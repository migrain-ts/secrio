# secrio architecture

Keep this file current: any change to layers, storage, key handling or error semantics updates it in the same commit.

## 1. Purpose

secrio is a local secrets manager. Secrets (name, value, type, description) are grouped as vault > project > environment > secret and stored in one encrypted SQLite file per vault. The core is a Go library (`internal/`); the CLI (`secrio`, section 13) is a front end on top of it, and the Desktop app will be another. No network, no CGO, no server.

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
| `app` | `Service` (create/list/unlock/lock/open/change password), `Unlocked` (projects, environments, secrets, history, compare, `ProcessEnv`, import/export), ports (including `SessionKeys` and the four session methods of `RecordStore`), sealed-record codec, key hierarchy (`keys.go`), session flow (`service.go`, `session.go`) |
| `app/importexport` | Pure parse/write of dotenv, JSON, YAML entry lists |
| `infra/crypto` | `Suite`: XChaCha20-Poly1305 sealing and Argon2id derivation; satisfies both `app.Cipher` and `app.KDF` |
| `infra/sqlite` | `Opener` / `Store`: record store on a SQLite file (modernc, pure Go), embedded migrations (`0001_init`, `0002_sessions`), WAL, 0600; atomic `PutSession/GetSession/TouchSession/DeleteSession` on the single-row `sessions` table (`session.go`) |
| `infra/registry` | `Registry`: list of known vaults in `<dataDir>/vaults.json` |
| `infra/keychain` | `KeyStore` (OS keychain via go-keyring; holds only a 32-byte session key) and `MemoryKeyStore`; both implement `app.SessionKeys` |
| `infra/clock` | `System` (UTC now) and `Fake` |
| `core` | `New`, options, `DefaultDataDir`, `VaultPath` |
| `cmd/secrio` | The CLI binary: `main` only calls `cli.Execute(cli.RealDeps(), os.Args[1:])` and exits with its code |
| `cli` (`internal/cli`) | Cobra command layer: one file per command, all I/O through `Deps`, no business logic; imports `core`, `app` and `domain`, never `infra` (section 13) |
| `tests` | Black-box end-to-end and security tests through `core.New`, plus tests of the real `secrio` binary |

`core.New(dataDir, opts...)` creates `dataDir` (0700) and builds: `crypto.Suite{}` as cipher and KDF, `sqlite.Opener{}`, `registry.New(dataDir)`, `keychain.New("secrio")`, `clock.System{}`, `crypto.DefaultKDFParams()`. Options: `WithKDFParams`, `WithSessionKeys`, `WithClock`. `VaultPath(dataDir, name)` returns `<dataDir>/vaults/<name>.db` (vaults dir 0700) and rejects empty names, `/`, `\`, `..` and NUL.

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

A session has two halves, both needed to open the vault:

- Half 1, the OS keychain (`app.SessionKeys`, production `keychain.KeyStore`): service `secrio`, user = vault id, value = the base64 of a fresh random 32-byte session key. Nothing else; never the vault key, an env key or a password.
- Half 2, the `sessions` table of the vault file (migration `0002_sessions.sql`, single row `id = 1`): `blob` = `Cipher.Seal(sessionKey, vaultKey, aad = "session|" + vaultID + "|" + <expires_at ms> + "|" + <idle_ms>)` (`sessionAAD` in `keys.go`, used by both `Unlock` and `Open`), plus `expires_at`, `last_used` and `idle_ms`. All times are unix milliseconds; `idle_ms = 0` means no idle limit; a positive idle below 1 ms is stored as 1 ms.

Flow:

- `Unlock(vaultID, password, ttl, idle)` verifies the password, draws a new session key, writes the row (`PutSession`), then saves the key (`SessionKeys.Save`). If the save fails the row is deleted again. `ttl <= 0` or `idle < 0` is rejected before anything is touched with an error wrapping `ErrInvalidArgument` (the message carries no values); `idle = 0` means no idle timeout. A second `Unlock` replaces both halves, so earlier session keys stop working.
- `Open(vaultID)` loads the session key, then runs `TouchSession`, one conditional `UPDATE ... SET last_used=now WHERE row exists AND expires_at > now AND (idle_ms = 0 OR now - last_used < idle_ms)`, then reads the row and opens the blob with the AAD rebuilt from that row (the Touch never changes `expires_at` or `idle_ms`). Success bumps `LastUsed` only, never `ExpiresAt`. `Open` therefore WRITES to the vault file on every call: the file mtime and WAL change on every `run` (this can churn sync tools and create conflict copies), and session use fails on a read-only vault file with an error that is not `ErrLocked`.
- Fail closed: no key, no row, expired, idle, a blob that does not authenticate under the key and AAD (tampered, truncated, from another vault, or with `expires_at` / `idle_ms` edited in the file), a keychain item that decodes as `ErrCorrupt` (damaged, or a legacy raw-vault-key item) or a wrong-length plaintext all give `ErrLocked`, and whatever is left of the session (row and/or key) is deleted. Exceptions that keep the row: any other `keys.Load` error (for example keychain unavailable) is returned as is; and when `TouchSession` succeeds but the row is gone by the time it is read (a concurrent `Lock`), `Open` returns `ErrLocked` without further cleanup because the `Lock` owns it.
- `Lock(vaultID)` deletes the row first (it is the credential), then the keychain key; it is idempotent. `OpenWithPassword` never touches session state.
- Atomicity across processes: `Lock` and `Open` meet in the vault file. Once a `Lock` has deleted the row, the conditional `UPDATE` cannot match, so an `Open` can never resurrect a session another process locked. Nothing in `app` serialises session calls (`TestLockNotUndoneByConcurrentOpen`).
- Sessions are shared between processes: two `core.New` on the same keychain and vault file see each other's unlock and lock (`TestSessionSharedBetweenTwoServices`, `TestSessionSharedAndKilledAcrossServices`).
- Time comes from the `Clock` port, so TTL and idle are testable (`TestSessionExpiryEndToEnd`).
- `ChangePassword` keeps existing sessions valid: it re-wraps the same vault key, and the session row seals that key, not the password.
- Unknown vault id: `Open` and `Lock` return `ErrNotFound` (`Lock` still deletes any keychain key stored under that id). A garbage vault file makes `Open` return `ErrCorrupt`, not `ErrLocked`: the file is read before the keychain.
- A damaged keychain item (not base64 of 32 bytes, or a go-keyring prefix decode failure) makes `Open` return `ErrCorrupt` and the row is kept; `Lock` or the next `Unlock` replaces the item.
- `Open` racing another process's `Unlock` can delete the freshly written session in two ways: it sees the new row with the old key, fails to open it and cleans up; or it finds no key (Unlock B has not saved it yet), then deletes the row after Unlock B wrote row and key, so the vault ends locked although `Unlock` succeeded. Both fail closed; retry `Unlock`.
- `errors.Join` results can match two sentinels (for example `ErrLocked` plus a cleanup error from the row or key delete). Front ends must test `ErrLocked` first.
- Timing fields: `expires_at` and `idle_ms` are bound into the blob's AAD, so a file-only writer who edits them makes `Open` fail closed (`ErrLocked`, both halves removed). `last_used` is the only unauthenticated timing field: an attacker holding the keychain key AND write access to the vault file still cannot extend the TTL, they can only roll `last_used` forward (keeping an idle session alive until `expires_at`).
- Expiry is lazy: nothing is deleted until the next `Open`, `Lock` or `Unlock` touches the vault. After the TTL a stale keychain item and session row persist until then, and anyone holding BOTH halves can still decrypt the blob offline; that window lasts until the next `Open`/`Lock` after expiry, not until the TTL. The clock is the local system clock: turning it back extends a session.
- `Lock` returns an error if the vault file cannot be opened or `DeleteSession` fails, and also joins a `keychain` delete failure; a store `Close` error can also make `Lock` return an error after both deletes succeeded. In every case `Lock` still attempts the keychain delete, and either half missing makes the session unusable, so one failure alone leaves no usable session (the leftover half is cleaned by the next `Open`/`Unlock`). Only when both halves fail to be removed (for example store open and keychain delete both fail) can a session survive; a caller cannot tell these apart, so it should treat any `Lock` error as "maybe still unlocked" and retry.
- Handle lifecycle: an open `*Unlocked` does NOT observe `Lock`, TTL expiry or idle expiry; those are checked only inside `Service.Open`. The handle keeps working on its in-memory key until `Close`, and its own use does not refresh the session's `LastUsed`. Front ends must therefore call `Open` per operation or short burst, `Close` handles promptly, and never cache a handle across user idle time.
- After `Close` every `Unlocked` method returns `ErrLocked` (one guard, `enter`); `Close` is idempotent and safe against a concurrent call (it waits for calls in flight).
- `CreateVault` registers the absolute path. If it fails after the file was created, it closes the store and removes the file and its `-wal`/`-shm`.

## 7. Error semantics

| Error | Meaning | Raised when |
|---|---|---|
| `ErrInvalidPassword` | Wrong password | Wrapped vault key fails to authenticate (`Unlock`, `OpenWithPassword`, `ChangePassword`, `RestoreVersion`) |
| `ErrLocked` | No usable session | No session, TTL expired, idle expired, row without keychain key or key without row, session blob that does not authenticate under the keychain key and row timing, keychain item that is `ErrCorrupt`. May be joined with a cleanup error, so test it first |
| `ErrInvalidArgument` | Invalid argument | `Unlock` with `ttl <= 0` or `idle < 0` |
| `ErrCorrupt` | Stored data is damaged or tampered | Record or env key fails to authenticate under the right key, JSON undecodable, truncated blob, KDF params out of range, not a SQLite file (also from `Open`, which reads the file before the keychain), vault id differs from registry, secret in the wrong env, keychain item that is not a base64 32-byte key (including a go-keyring prefix decode failure; `Open` converts it to `ErrLocked` and purges the session) |
| `ErrAuthFailed` | Cipher-level authentication failure | Returned by `Cipher.Open`; `app` converts it to `ErrInvalidPassword` (vault key) or `ErrCorrupt` (any record), so front ends normally do not see it |
| `ErrUnsupportedFormat` | Format not understood | Unknown `format_version`, newer migrations, unknown or malformed import/export input |
| `ErrUnsupportedValue` | Value cannot be used as asked | NUL in a value passed to `ProcessEnv`; non-UTF-8 value for JSON/YAML export |
| `ErrNotFound` | Missing item | Unknown vault id (also from `Open` and `Lock`), project, environment, secret, history entry; missing vault file |
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
- Sessions (amended 2026-11-19 after the final review). The keychain holds only a random per-session key; the vault key lives sealed under it in the `sessions` row of the vault file (section 6). What this improves: the keychain item alone is useless (without the vault file it opens nothing, and it is not the vault key); a vault file or backup alone is useless (the row is sealed under a key that is not in it); `Lock` deletes the row (and expiry does so lazily, at the next `Open`), so a keychain item leaked after a `Lock` is dead (`TestLeakedKeychainItemUselessAfterLock`); TTL and idle are enforced by the data (an `UPDATE` that matches no expired or idle row), not only by app code; `Lock` versus `Open` is atomic across processes; the keychain never receives the vault key (`TestKeychainHoldsNoVaultKey`, `TestKeychainNeverReceivesVaultKey`).
- What does NOT improve: a same-user attacker who reads BOTH the keychain item and the vault file during a live session recovers the vault key (on macOS go-keyring calls the `security` CLI, so any process running as the user may read the item). Handles still do not observe `Lock`. `ChangePassword` keeps sessions valid. Expiry is lazy: nothing is deleted until the next `Open`, `Lock` or `Unlock`, so after the TTL the keychain item and the row persist, and anyone holding both halves (a backup of the file plus the keychain item) can still decrypt the blob offline until the next `Open`/`Lock` after expiry, not merely until the TTL. The keychain key is gone only after a `Lock` or a purge by `Open`. `expires_at` and `idle_ms` are authenticated through the AAD; `last_used` stays plaintext and unauthenticated (a holder of the keychain key with file write access can only roll it forward, not extend the TTL). Turning the system clock back extends a session. Recommend short TTLs and calling `Lock` on exit.
- `core.New` only creates a missing data directory with 0700; it does not tighten an already-existing wider one.
- `CompareEnvs` and `ProcessEnv` decrypt every value of the environments involved. `ProcessEnv` fails the whole environment on a NUL value and deliberately does not name it. Restoring a value equal to the current one is a silent no-op (no event). Multi-line quoted dotenv values are not supported in v1; import treats an unquoted `#` as a comment only after whitespace (`KEY=#abc` is the literal `#abc`) and strips a leading UTF-8 BOM from all formats.
- `secure_delete(1)` zeroes freed pages only. Old ciphertext (for example a previous password's wrapped vault key) may linger in backups, copied WAL files or filesystem snapshots.
- Registry: `vaults.json` is replaced atomically, but its read-modify-write cycle is guarded by an in-process mutex only; two processes adding vaults at once can lose an entry.
- SQLite: concurrent writers (several `Store`s or processes on one file) are serialised by SQLite locks plus `busy_timeout`.
- A user who knows the password can read everything; there is no recovery if it is lost.

## 11. Testing strategy

- Unit tests per package with fakes: `app` runs on an in-memory record store, cheap test cipher and KDF, fake clock, fake registry and sessions; `infra/*` test their adapters (including real SQLite files in temp dirs and the go-keyring mock).
- `tests/` is black-box: real adapters through `core.New`, `t.TempDir()`, `keyring.MockInit()` and cheap KDF params `{Time:1, MemoryKiB:8, Threads:1}`. It covers the end-to-end flow, plaintext absence in the `.db`, `-wal` and `-shm` files, file modes, blob corruption and blob swapping via raw SQL, future format versions, garbage files, log hygiene, session hardening (what the keychain and the `sessions` row hold, a leaked key after `Lock`, orphan halves, TTL and idle expiry with a fake clock, shared and killed sessions across two services; `tests/session_test.go`), concurrent writers (run under `-race`; the go-keyring mock is not goroutine-safe, so that test shares a `MemoryKeyStore`) and a real child process receiving secrets.
- `internal/cli` unit tests run the commands in-process with injected `Deps` (fake TTY flags, in-memory keychain, a fake or real runner). `tests/cli_binary_test.go` builds the real binary once in `TestMain` (`CGO_ENABLED=0`) and runs it with an explicit minimal environment (`SECRIO_HOME`, `SECRIO_PASSWORD`, `HOME`, `PATH`): full flow, exit codes, `.secrio.toml` lookup from a subdirectory, no secret in argv, error output free of passwords and values. These tests never call `unlock` or `lock`, because the binary would use the real OS keychain; session behaviour is covered in-process.
- Checks before every commit: `gofmt -l .` empty, `go vet ./...`, `go test -race -count=1 ./...`.

## 12. How CLI and Desktop plug in

Both front ends:

1. Resolve the data dir (`core.DefaultDataDir()` or a flag) and call `core.New(dir)`; they get an `*app.Service`.
2. Create vaults at `core.VaultPath(dir, name)`, then `Unlock`, `Open` (or `OpenWithPassword` for one-shot use), call `Unlocked` methods, and `Close` the handle.
3. Map the errors in section 7 to messages and exit codes; they never import `internal/infra`.

CLI `run` = `Open` + `ProcessEnv(os.Environ())` + `os/exec`, propagating the child's exit code (details in section 13). Desktop keeps a long-lived `Service` and calls `Lock` on demand, but must not keep a long-lived `Unlocked`: an open handle ignores `Lock`, TTL and idle expiry (section 6) and does not refresh `LastUsed`, so it calls `Open` per operation or short burst and closes the handle right after. The CLI exists (section 13); the Desktop does not yet.

## 13. CLI

Commands: `vault create|list`, `unlock`, `lock`, `project create`, `env create`, `list`, `set`, `get`, `run`, plus Cobra's `completion` and `help`. User-facing documentation is in `README.md`.

- Wiring: `cmd/secrio/main.go` calls `cli.Execute(cli.RealDeps(), os.Args[1:])`. `Deps` (`deps.go`) injects every side effect: `In`/`Out`/`Err`, `Getenv`, `Environ`, `Getwd`, `StdinTTY`, `StdoutTTY`, `ReadPassword` (masked, from the terminal), `DataDir`, `NewService` (wraps `core.New`) and `Run` (the child-process runner). `RealDeps` binds them to the process; the data dir is `SECRIO_HOME` if set, else `core.DefaultDataDir()`. Tests pass fakes.
- Commands: `NewRoot` (`cli.go`) builds the root and attaches the subcommands in one explicit constructor block, one `newXxxCmd(d, &flags)` per line. There is no registry or `init()` list.
- Argument errors: `finalize` runs `usageArgs` over the whole tree, including Cobra's built-in `completion` and `help` commands. It makes every argument-validation error exit 2: a group command (has subcommands, no `Args`) rejects positional arguments as an unknown command; a leaf without `Args` defaults to `cobra.NoArgs`; any other `Args` error is wrapped as a usage error unless it already carries an `*ExitError`. Flag-parse errors are mapped to usage errors through `SetFlagErrorFunc`. Required flags are not used: commands validate the context themselves and return `UsageErr`.
- Context: `--vault`, `--project` and `--env` are persistent flags. `LoadContext` resolves each field on its own: flag, then `SECRIO_VAULT` / `SECRIO_PROJECT` / `SECRIO_ENV`, then the nearest `.secrio.toml` (keys `vault`, `project`, `env`), searched from the working directory upwards, first file found wins. The file is read only when a field is still empty, is decoded strictly (unknown keys are an error) and errors name the path and position, never file content. A missing field gives a usage error (exit 2) naming the flag, variable and file.
- Unlocking: `openVault` tries the session first (`Service.Open`). If `SECRIO_PASSWORD` is set it is used whenever no usable session exists, including when the OS keychain is unavailable: any `Open` error leads to `OpenWithPassword`, which creates no session and reports a wrong password as exit 3. A live session wins over the variable. A rejected `SECRIO_PASSWORD` reads `invalid password (from SECRIO_PASSWORD)` (`envPasswordErr` in `password.go`, still exit 3). Without the variable, `ErrLocked` is exit 3 (with a `secrio unlock` hint) and any other `Open` error (for example a keychain failure) keeps its text and ends with `(set SECRIO_PASSWORD to open without the OS keychain)`, exit 1. `unlock` and `vault create` read the password from `SECRIO_PASSWORD` or, with a terminal, a masked prompt (`vault create` asks twice); without a terminal and without the variable they fail with exit 3 and never wait for input. `vault create` checks the name against `^[A-Za-z0-9._-]{1,64}$`, refuses names starting with `.` or containing `..` and Windows device names (`con prn aux nul com1-9 lpt1-9`, case-insensitive, also with an extension), and refuses a name already registered, compared case-insensitively (`strings.EqualFold`) before any prompt. On success it prints `created vault <name>` and `next: secrio unlock <name>`, because creating a vault opens no session. The Core's `validName` additionally rejects control characters in vault, project and environment names. `project create` / `env create` map `ErrDuplicate` to `project "x" already exists` / `environment "x" already exists` (exit 1).
- Values: `set` reads a masked prompt when stdin is a terminal, else stdin (at most 1 MiB; one trailing `\n` or `\r\n` removed); it ignores `SECRIO_PASSWORD`, refuses an empty value or one containing a NUL byte (exit 1; a NUL would make `ProcessEnv` fail every later `run` of the environment) and never takes a value from an argument. `get` writes the raw bytes and a `\n` only when stdout is a terminal. `list` prints sorted names at the depth the context gives (vaults need `vault list`; no project: projects; project: environments; project and env: secret names) and never values. `--env` without a project is a usage error.

Exit codes (outside `run`): 0 success; 1 any other error; 2 usage error (`ExitError` code 2, or an error wrapping `ErrInvalidArgument`); 3 locked or wrong password (`ExitError` code 3, `ErrLocked`, `ErrInvalidPassword`). `Execute` prints `secrio: <message>` to stderr, except for a code-only `ExitError` (the child's status).

`run` (`run.go`, `runner.go`):

- Needs `--` before the command. Every secrio failure before the child starts exits 125, usage errors included (a missing `--` or command, a missing vault/project/env context), as well as an unknown project or environment, a locked vault or a wrong password: exit 2 would collide with a child's own exit 2. The one exception is Cobra's flag-parse error and the root's unknown-command error, which stay exit 2 (`run --nosuch -- true`).
- The child's environment is `Environ()` without `SECRIO_PASSWORD`, merged with the environment's secrets by `ProcessEnv`. The vault is closed (`defer u.Close()` in `runEnvironment`) before the runner starts the child, so the vault key is not held while the child runs.
- The command is looked up with `exec.Command` using secrio's own `PATH`, not a `PATH` secret of the environment. stdin, stdout and stderr are passed straight through.
- Exit code: the child's own code; 127 when the command is not found; 126 when it cannot be executed; 128+N when killed by signal N (Unix; Windows has no signal case); 125 for a failure of the runner itself such as a stdio copy error.
- Signals: while the child runs, secrio catches SIGTERM and relays it always, so the child can shut down and secrio reports its status. SIGINT is relayed only when no terminal is attached to stdin, stdout or stderr: on a terminal, Ctrl-C already reaches the child through the foreground process group, and relaying it again would deliver it twice.

Known limits (also in the README): Two concurrent `vault create` runs can leave an orphaned vault file: the registry is last-writer-wins across processes. A project or environment provided by `.secrio.toml` cannot be cleared from the command line: an empty `--project ""` counts as unset and the file's value applies. Vault, project and environment names cannot contain control characters (newline, ESC, ...). Vault names are case-insensitive on macOS and Windows file systems, so `vault create` refuses a name that differs from an existing one only by case; reserved Windows names (`con`, `prn`, `aux`, `nul`, `com1`-`com9`, `lpt1`-`lpt9`, also with an extension) and `..` inside a name are refused everywhere. `SECRIO_PASSWORD` exported in a shell profile costs one Argon2id run (about a second and 256 MiB of memory) for every command that has no session.

What the CLI deliberately does not do: put values, passwords or keys in argv, errors or logs; prompt without a terminal; offer import, export, edit, delete, history, compare or change-password commands (the library supports them, no command exists yet); read a value from a flag or argument; keep a long-lived `Unlocked` (each command opens, works and closes); import `internal/infra`.
