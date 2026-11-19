# secrio architecture

Keep this file current: any change to layers, storage, key handling or error semantics updates it in the same commit.

## 1. Purpose

secrio is a local secrets manager. Secrets (name, value, type, description) are grouped as vault > project > environment > secret and stored in one encrypted SQLite file per vault. The core is a Go library (`internal/`); the CLI and the Desktop app are separate front ends that will sit on top of it. No network, no CGO, no server.

## 2. Layers and dependency rule

```
domain  <-  app  <-  core (composition root)  ->  infra/*
                 ^                                  |
                 +---- ports (interfaces) ----------+
```

- `internal/domain`: entities, typed IDs, errors, `Key`, `KDFParams`, secret-name validation. Imports nothing from the project.
- `internal/app`: use cases (`Service`, `Unlocked`) plus the ports they need (`ports.go`). Imports only `domain` (and its own `app/importexport`).
- `internal/infra/*`: adapters implementing the ports. They import `app` and `domain`, never each other's internals.
- `internal/core`: the only non-test package importing both `app` and `infra`. `core.New` wires everything.
- Rule: `app` must never import `infra`. Front ends call `core.New` and then use `app` and `domain` types only.

## 3. Package map

| Package | Role |
|---|---|
| `domain` | `Vault`, `Project`, `Environment`, `Secret` (redacted under every fmt verb), `ID[T]`, `Key` (32 bytes), `KDFParams`, `SecretName` (`[A-Za-z_][A-Za-z0-9_]*`), sentinel errors |
| `app` | `Service` (create/list/unlock/lock/open/change password), `Unlocked` (projects, environments, secrets, history, compare, `ProcessEnv`, import/export), ports, sealed-record codec, key hierarchy (`keys.go`) |
| `app/importexport` | Pure parse/write of dotenv, JSON, YAML entry lists |
| `infra/crypto` | `Suite`: XChaCha20-Poly1305 sealing and Argon2id derivation; satisfies both `app.Cipher` and `app.KDF` |
| `infra/sqlite` | `Opener` / `Store`: record store on a SQLite file (modernc, pure Go), embedded migrations, WAL, 0600 |
| `infra/registry` | `Registry`: list of known vaults in `<dataDir>/vaults.json` |
| `infra/keychain` | `SessionStore` (OS keychain via go-keyring) and `MemorySessionStore` |
| `infra/clock` | `System` (UTC now) and `Fake` |
| `core` | `New`, options, `DefaultDataDir`, `VaultPath` |
| `tests` | Black-box end-to-end and security tests through `core.New` |

`core.New(dataDir, opts...)` creates `dataDir` (0700) and builds: `crypto.Suite{}` as cipher and KDF, `sqlite.Opener{}`, `registry.New(dataDir)`, `keychain.New("secrio")`, `clock.System{}`, `crypto.DefaultKDFParams()`. Options: `WithKDFParams`, `WithSessionStore`, `WithClock`. `VaultPath(dataDir, name)` returns `<dataDir>/vaults/<name>.db` (vaults dir 0700) and rejects empty names, `/`, `\`, `..` and NUL.

## 4. Key hierarchy

```
password --Argon2id(salt, per-vault params)--> unlock key
unlock key  --seals-->  vault key            (meta.wrapped_vault_key, AAD "vaultkey")
vault key   --seals-->  project records, environment records   (AAD "<kind>|<id>")
vault key   --wraps-->  one env key per environment             (envkey record, AAD = env id)
env key     --seals-->  secret records, history records         (AAD "<kind>|<id>")
```

- Seal format: `0x01 | nonce(24) | ciphertext+tag` (XChaCha20-Poly1305, random nonce per seal). The AEAD authenticates `0x01 | aad`: the version byte is bound into the tag as well as being stored in clear.
- The AAD binds each blob to its record kind and id, so blobs swapped between rows fail authentication.
- Empty master passwords are refused (`ErrInvalidPassword`) by `CreateVault` and as the new password of `ChangePassword`, before any file is created or KDF run.
- Changing the password only re-wraps the vault key under a new salt and unlock key; no data record is rewritten.
- Per-environment keys keep environments cryptographically separate; `DuplicateProject` generates new env keys and re-seals the copies (there is no environment-level duplicate).
- Default Argon2id cost: time 4, memory 256 MiB, 4 threads. The cost used for a vault is stored in its header (`meta.kdf_*`).

## 5. Storage schema

One SQLite file per vault, opened with pragmas `busy_timeout(5000)` (first, so the WAL switch also waits), `journal_mode(WAL)`, `foreign_keys(1)`, `secure_delete(1)`, immediate write transactions, one pooled connection per `Store`. Files are created 0600 (the `-wal` and `-shm` files are chmod-ed to 0600 on open). Timestamps are unix milliseconds. Migrations are embedded SQL files applied in order, each in its own transaction.

| Table | Columns | Plaintext? |
|---|---|---|
| `schema_migrations` | `version` | yes |
| `meta` (single row, `id = 1`) | `format_version`, `vault_id`, `name`, `kdf_time`, `kdf_mem_kib`, `kdf_threads`, `salt`, `wrapped_vault_key`, `created_at` | yes, except `wrapped_vault_key` (sealed) |
| `records` | `kind`, `id`, `parent_id`, `blob`, `updated_at`; PK `(kind, id)`; index `(kind, parent_id)` | `kind`, `id`, `parent_id`, `updated_at` plaintext; `blob` sealed |

Record kinds and their sealing key:

| kind | id / parent_id | blob |
|---|---|---|
| `project` | project id / "" | JSON sealed with vault key (name, description) |
| `environment` | env id / project id | JSON sealed with vault key (name) |
| `envkey` | env id / project id | env key wrapped by vault key |
| `secret` | secret id / env id | JSON sealed with env key (name, value, type, description, timestamps) |
| `history` | random id / env id | JSON sealed with env key |

Plaintext leaks by design: the vault name, KDF parameters, salt, vault id, record ids, which records share a parent (and how many), and update times. Secret names, values, descriptions, project and environment names are never plaintext (checked by `TestSQLiteFileContainsNoPlaintext`). The vault list (id, name, file path) is plaintext in `vaults.json`.

`meta.format_version` must equal 1 (`app.formatVersion`); any other value is refused with `ErrUnsupportedFormat` on unlock, open-with-password and change-password. A database whose `schema_migrations` is ahead of the embedded migrations is refused the same way at open.

## 6. Session model

- `Unlock(vaultID, password, ttl, idle)` verifies the password and stores a `Session` (vault key, `ExpiresAt`, `LastUsed`, `Idle`) in the `SessionStore`. `ttl` must be positive; `idle <= 0` disables the idle timeout.
- The production store is the OS keychain: service `secrio`, user = vault id, value = JSON with the base64 key and RFC 3339 times.
- `Open(vaultID)` loads the session. Missing, past `ExpiresAt`, or idle for `Idle` or longer gives `ErrLocked` (an expired session is deleted). Success bumps `LastUsed`. `Lock` deletes the session. `OpenWithPassword` never touches the session store.
- All of `Open`, `Lock`, `Unlock` take one in-process mutex (`sessionMu`) so a `Lock` cannot be undone by a concurrent `Open` in the same process.
- Limit: the mutex does not span processes. A `Lock` from another process during the short Load-to-Save window of an `Open` can be undone; the session then lives until TTL or idle expiry.
- Sessions are shared between processes through the keychain: two `core.New` on the same keychain see each other's unlock and lock (`TestSessionSharedBetweenTwoServices`).
- Time comes from the `Clock` port so TTL/idle are testable.
- Handle lifecycle: an open `*Unlocked` does NOT observe `Lock`, TTL expiry or idle expiry; those are checked only inside `Service.Open`. The handle keeps working on its in-memory key until `Close`, and its own use does not refresh the session's `LastUsed`. Front ends must therefore call `Open` per operation or short burst, `Close` handles promptly, and never cache a handle across user idle time.
- After `Close` every `Unlocked` method returns `ErrLocked` (one guard, `enter`); `Close` is idempotent and safe against a concurrent call (it waits for calls in flight).
- `CreateVault` registers the absolute path. If it fails after the file was created, it closes the store and removes the file and its `-wal`/`-shm`.

## 7. Error semantics

| Error | Meaning | Raised when |
|---|---|---|
| `ErrInvalidPassword` | Wrong password | Wrapped vault key fails to authenticate (`Unlock`, `OpenWithPassword`, `ChangePassword`, `RestoreVersion`) |
| `ErrLocked` | No usable session | No session, TTL expired, idle expired |
| `ErrCorrupt` | Stored data is damaged or tampered | Record or env key fails to authenticate under the right key, JSON undecodable, truncated blob, KDF params out of range, not a SQLite file, vault id differs from registry, secret in the wrong env |
| `ErrAuthFailed` | Cipher-level authentication failure | Returned by `Cipher.Open`; `app` converts it to `ErrInvalidPassword` (vault key) or `ErrCorrupt` (any record), so front ends normally do not see it |
| `ErrUnsupportedFormat` | Format not understood | Unknown `format_version`, newer migrations, unknown or malformed import/export input |
| `ErrUnsupportedValue` | Value cannot be used as asked | NUL in a value passed to `ProcessEnv`; non-UTF-8 value for JSON/YAML export |
| `ErrNotFound` | Missing item | Unknown vault, project, environment, secret, history entry; missing vault file |
| `ErrDuplicate` | Name or path taken | Project or environment name, vault path already exists, duplicate vault id, repeated import name |
| `ErrInvalidName` | Rejected name | Blank display name, bad secret name, unknown secret type, bad `VaultPath` file name |

Error text never contains secret names, values, passwords or file content (driver errors are mapped to sentinels; import errors give line or position only).

## 8. History model

- Every `SetSecret`, `DeleteSecret`, import and restore that changes something records a `history` record sealed with the env key; creating an environment records `env.create`. Ops: `secret.create`, `secret.update`, `secret.delete`, `env.create`.
- The record holds previous and new values (and previous type and description), a sequence number and timestamp. `History` returns entries newest first, ordered by sequence number alone (the clock may tie or go backwards), with `PrevValue` and `NewValue` always nil.
- `RestoreVersion(entryID, password)` needs the vault password again, re-applies the previous value of an update or delete entry, and records itself as a normal event.
- History is deleted with its environment or project.

## 9. Compare, run, import/export

- `CompareEnvs` returns, per secret name, presence in each environment (no values).
- `ProcessEnv(envID, base)` returns a new `KEY=VALUE` slice: secrets replace same-named base entries in place and are otherwise appended sorted by name; names compare case-insensitively on Windows only. The caller starts the child process (`TestRunEnvEndToEnd` does it with `os/exec`) and must not log the result.
- Import/export formats (`importexport`): dotenv (`KEY=VALUE`, comments, optional `export`), JSON (flat object of scalar values), YAML (flat mapping). Parsing is capped at 8 MiB, duplicate names keep the last value at the first position, and output is sorted by name. `PlanImport` classifies entries as new or duplicate; `ApplyImport` stores them as text secrets with strategy `skip` or `overwrite` in one transaction (overwrite keeps type and description). `Export` writes all or named secrets; a missing name is `ErrNotFound`.

## 10. Security properties and known limits

- Confidentiality at rest: all names (except the vault name) and values are sealed; integrity and record binding come from the AEAD and the AAD.
- Files: vault files, WAL/SHM and `vaults.json` are 0600; data and vaults directories created by secrio are 0700 (`TestFilePermissions0600`; POSIX only).
- Hostile vault files: stored KDF parameters are bounded (time 1-64, threads 1-64, memory 8 KiB-1 GiB) before any derivation; out-of-range values give `ErrCorrupt`. A non-empty file shorter than the 100-byte SQLite header is refused instead of being overwritten.
- Logging: `Secret`, `HistoryEntry`, `ImportPlan` and `importexport.Entry` print as `<redacted>` for every fmt verb; the app layer does not log (`TestLogsContainNoSecrets`).
- Go memory: keys are zeroed after use (`zeroKey`) and password-derived keys are zeroed, but Go gives no guarantee: the GC may have copied buffers, and secret values returned to callers are ordinary byte slices. Swap and core dumps are out of scope.
- Session key in the OS keychain: the item holds the RAW permanent vault key (not a per-session key), so anyone who reads it can decrypt the vault file, and the key stays valid even after `ChangePassword` (which only re-wraps it). TTL and idle expiry are enforced by the app only; an expired item stays in the keychain until some `Open` or `Lock` runs. On macOS go-keyring calls the `security` CLI, so any process running as the user may read the item. Recommend short TTLs and calling `Lock` on exit.
- `core.New` only creates a missing data directory with 0700; it does not tighten an already-existing wider one.
- `CompareEnvs` and `ProcessEnv` decrypt every value of the environments involved. `ProcessEnv` fails the whole environment on a NUL value and deliberately does not name it. Restoring a value equal to the current one is a silent no-op (no event). Multi-line quoted dotenv values are not supported in v1; import treats an unquoted `#` as a comment only after whitespace (`KEY=#abc` is the literal `#abc`) and strips a leading UTF-8 BOM from all formats.
- `secure_delete(1)` zeroes freed pages only. Old ciphertext (for example a previous password's wrapped vault key) may linger in backups, copied WAL files or filesystem snapshots.
- Registry: `vaults.json` is replaced atomically, but its read-modify-write cycle is guarded by an in-process mutex only; two processes adding vaults at once can lose an entry.
- SQLite: concurrent writers (several `Store`s or processes on one file) are serialised by SQLite locks plus `busy_timeout`.
- A user who knows the password can read everything; there is no recovery if it is lost.

## 11. Testing strategy

- Unit tests per package with fakes: `app` runs on an in-memory record store, cheap test cipher and KDF, fake clock, fake registry and sessions; `infra/*` test their adapters (including real SQLite files in temp dirs and the go-keyring mock).
- `tests/` is black-box: real adapters through `core.New`, `t.TempDir()`, `keyring.MockInit()` and cheap KDF params `{Time:1, MemoryKiB:8, Threads:1}`. It covers the end-to-end flow, plaintext absence in the `.db`, `-wal` and `-shm` files, file modes, blob corruption and blob swapping via raw SQL, future format versions, garbage files, log hygiene, shared sessions across two services, concurrent writers (run under `-race`; the go-keyring mock is not goroutine-safe, so that test shares a `MemorySessionStore`) and a real child process receiving secrets.
- Checks before every commit: `gofmt -l .` empty, `go vet ./...`, `go test -race -count=1 ./...`.

## 12. How CLI and Desktop plug in

Both front ends:

1. Resolve the data dir (`core.DefaultDataDir()` or a flag) and call `core.New(dir)`; they get an `*app.Service`.
2. Create vaults at `core.VaultPath(dir, name)`, then `Unlock`, `Open` (or `OpenWithPassword` for one-shot use), call `Unlocked` methods, and `Close` the handle.
3. Map the errors in section 7 to messages and exit codes; they never import `internal/infra`.

CLI `run` = `Open` + `ProcessEnv(os.Environ())` + `os/exec`, propagating the child's exit code. Desktop keeps a long-lived `Service` and calls `Lock` on demand, but must not keep a long-lived `Unlocked`: an open handle ignores `Lock`, TTL and idle expiry (section 6) and does not refresh `LastUsed`, so it calls `Open` per operation or short burst and closes the handle right after. Neither front end exists yet.
