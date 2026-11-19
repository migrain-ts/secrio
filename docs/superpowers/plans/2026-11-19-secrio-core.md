# Secrio Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the Secrio Core: domain, use cases and infra adapters (crypto, SQLite, keychain, registry) so that CLI and Desktop can later be thin interfaces.

**Architecture:** Clean architecture under `internal/`: `domain` (entities, errors, no deps) ← `app` (use cases + ports) ← `infra` (adapters). Every entity is stored as one sealed `Record` (generic codec), so the SQLite adapter knows nothing about secrets. Keys: password → Argon2id → unlock key → vault key → one key per environment.

**Tech Stack:** Go 1.26, `modernc.org/sqlite` (no CGO), `golang.org/x/crypto` (argon2, chacha20poly1305), `github.com/zalando/go-keyring`, `gopkg.in/yaml.v3`, stdlib `testing`.

**Spec:** `docs/superpowers/specs/2026-11-19-secrio-design.md`

## Global Constraints

- Go only, module `secrio`, no CGO anywhere in Core.
- Argon2id KDF; XChaCha20-Poly1305 AEAD; all keys/nonces from `crypto/rand`; fresh 24-byte nonce per `Seal`.
- Every persisted ciphertext starts with a 1-byte format version (`0x01`); unknown versions are refused.
- Names and values of projects, environments, secrets and history are encrypted; only vault name, ids, kinds, parent ids, timestamps, KDF params, salt and format version are plaintext.
- One key per environment, wrapped by the vault key; changing the password re-wraps only the vault key.
- Secret names match `^[A-Za-z_][A-Za-z0-9_]*$`.
- Never put secret values, passwords or keys in errors, logs or `fmt` output.
- SQLite files created with mode `0600`, WAL mode, `busy_timeout` set.
- Core imports nothing from CLI/Desktop. `app` never imports `infra`.
- Commits: no `Co-Authored-By`, no "Generated with" line. Never push.
- Prefer small files; generics only where they remove real repetition (`ID[T]`, `seal/open[T]`).

## Review Focus

- Secret values with newlines, `=`, NUL, empty, or 1 MiB: must round-trip byte-exact (Task 7).
- Duplicate secret name in the same env, or names differing only by case: duplicate rejected, case-sensitive otherwise (Task 7).
- Desktop and CLI writing/reading the same file at once: `SQLITE_BUSY` retried within `busy_timeout`, no failure (Task 5).
- Truncated/garbage DB file or unknown format version: clean `ErrCorrupt`/`ErrUnsupportedFormat`, no panic, no data in message (Tasks 2, 5).
- Wrong password vs corrupted wrapped key vs expired session: three distinct errors (`ErrInvalidPassword`, `ErrCorrupt`, `ErrLocked`) (Tasks 4, 8).

---

### Task 1: Scaffold and domain

**Files:**
- Create: `go.mod`, `internal/domain/id.go`, `internal/domain/entities.go`, `internal/domain/errors.go`, `internal/domain/secretname.go`
- Test: `internal/domain/secretname_test.go`, `internal/domain/id_test.go`

**Interfaces:**
- Produces:
  - `type ID[T any] string`; `func NewID[T any]() ID[T]` (16 random bytes, hex).
  - `type Key [32]byte`
  - `type SecretName string`; `func ParseSecretName(s string) (SecretName, error)` → `ErrInvalidName`.
  - Entities: `Vault{ID ID[Vault]; Name string; CreatedAt time.Time}`, `Project{ID ID[Project]; Name, Description string; CreatedAt time.Time}`, `Environment{ID ID[Environment]; ProjectID ID[Project]; Name string}`, `SecretType` (`text|password|api_key|token|url|private_key|certificate|json`), `Secret{ID ID[Secret]; EnvID ID[Environment]; Name SecretName; Value []byte; Description string; Type SecretType; CreatedAt, UpdatedAt time.Time}`.
  - Errors: `ErrInvalidName, ErrLocked, ErrInvalidPassword, ErrNotFound, ErrDuplicate, ErrCorrupt, ErrUnsupportedFormat`.

- [ ] **Step 1: Write failing tests**: `TestParseSecretName` (table: `DATABASE_URL`, `_x`, `a1` valid; `""`, `1A`, `A-B`, `A B`, `é` → `ErrInvalidName`); `TestParseSecretNameCaseSensitive` (`a != A` kept as given); `TestNewIDUnique` (1000 ids distinct, length 32).
- [ ] **Step 2:** `go mod init secrio && go test ./internal/domain/...` → FAIL (undefined).
- [ ] **Step 3:** Implement the types above. Entities hold no behavior beyond `ParseSecretName`. `Secret.String()`/`GoString()` must return `Secret{<redacted>}` so `%v` never prints a value.
- [ ] **Step 4:** Add test `TestSecretFormatRedacted` (`fmt.Sprintf("%v %+v", s, s)` contains no value). Run `go test ./internal/domain/...` → PASS.
- [ ] **Step 5:** Commit `feat(domain): entities, typed ids, secret name validation`.

### Task 2: Crypto adapter (AEAD, KDF, zeroing)

**Files:**
- Create: `internal/infra/crypto/aead.go`, `internal/infra/crypto/kdf.go`
- Test: `internal/infra/crypto/aead_test.go`, `internal/infra/crypto/kdf_test.go`

**Interfaces:**
- Consumes: `domain.Key`, `domain.ErrCorrupt`, `domain.ErrUnsupportedFormat`.
- Produces (methods on `crypto.Suite`, which later satisfies `app.Cipher` and `app.KDF`):
  - `func (Suite) NewKey() (domain.Key, error)`
  - `func (Suite) Seal(k domain.Key, plaintext, aad []byte) ([]byte, error)` → `0x01 | nonce(24) | ciphertext`
  - `func (Suite) Open(k domain.Key, sealed, aad []byte) ([]byte, error)`
  - `func (Suite) RandomSalt() ([]byte, error)` (16 bytes)
  - `type KDFParams struct{ Time, MemoryKiB uint32; Threads uint8 }`; `func DefaultKDFParams() KDFParams`
  - `func (Suite) Derive(password, salt []byte, p KDFParams) domain.Key`
  - `func Zero(b []byte)`

- [ ] **Step 1: Write failing tests**: `TestSealOpenRoundTrip` (empty, 1 byte, 1 MiB); `TestSealNonceUnique` (two seals of same input differ); `TestOpenTamperedCiphertext` (flip each byte of a small blob → error `ErrCorrupt`); `TestOpenWrongKey`; `TestOpenWrongAAD`; `TestOpenUnknownVersion` (first byte `0x02` → `ErrUnsupportedFormat`); `TestOpenTruncated` (len<25 → `ErrCorrupt`); `TestDeriveDeterministic` (same inputs same key; different salt different key; uses cheap params `{1, 8, 1}`); `TestZero`.
- [ ] **Step 2:** Run `go test ./internal/infra/crypto/...` → FAIL.
- [ ] **Step 3:** Implement with `chacha20poly1305.NewX`. `DefaultKDFParams` benchmark-chosen: start at `{Time:3, MemoryKiB:64*1024, Threads:4}` and add `BenchmarkDerive`; target 200–500 ms on a recent laptop, adjust numbers if outside.
- [ ] **Step 4:** Run tests with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(crypto): xchacha20-poly1305 sealing, argon2id derivation`.

### Task 3: Ports and generic sealed codec

**Files:**
- Create: `internal/app/ports.go`, `internal/app/codec.go`
- Test: `internal/app/codec_test.go`

**Interfaces:**
- Consumes: Task 1 types.
- Produces:
  - `type Cipher interface{ NewKey() (domain.Key, error); Seal(domain.Key, plaintext, aad []byte) ([]byte, error); Open(domain.Key, sealed, aad []byte) ([]byte, error) }`
  - `type KDF interface{ RandomSalt() ([]byte, error); Derive(password, salt []byte, p KDFParams) domain.Key }` and `type KDFParams struct{ Time, MemoryKiB uint32; Threads uint8 }` (app-owned; infra converts).
  - `type Clock interface{ Now() time.Time }`
  - `type Kind string` consts `KindProject, KindEnvironment, KindSecret, KindHistory, KindEnvKey`.
  - `type Record struct{ Kind Kind; ID, ParentID string; Blob []byte; UpdatedAt time.Time }`
  - `type VaultMeta struct{ FormatVersion int; Name string; KDF KDFParams; Salt, WrappedVaultKey []byte; CreatedAt time.Time }`
  - `type RecordStore interface{ Meta(ctx) (VaultMeta, error); PutMeta(ctx, VaultMeta) error; Put(ctx, Record) error; Get(ctx, Kind, id string) (Record, error); List(ctx, Kind, parentID string) ([]Record, error); Delete(ctx, Kind, id string) error; Tx(ctx, func(RecordStore) error) error }` (`Get` miss → `domain.ErrNotFound`).
  - `type Session struct{ VaultKey domain.Key; ExpiresAt, LastUsed time.Time; Idle time.Duration }`; `type SessionStore interface{ Save(ctx, vaultID string, Session) error; Load(ctx, vaultID string) (Session, error); Delete(ctx, vaultID string) error }` (`Load` miss → `ErrLocked`).
  - `type VaultEntry struct{ ID, Name, Path string }`; `type Registry interface{ List(ctx) ([]VaultEntry, error); Add(ctx, VaultEntry) error; Remove(ctx, id string) error }`
  - `type VaultOpener interface{ Create(ctx, path string) (RecordStore, error); Open(ctx, path string) (RecordStore, error) }`
  - `func seal[T any](c Cipher, k domain.Key, kind Kind, id string, v T) ([]byte, error)` and `func open[T any](c Cipher, k domain.Key, kind Kind, id string, blob []byte) (T, error)`: JSON encode, AAD = `kind + "|" + id` so a blob cannot be swapped between records.

- [ ] **Step 1: Write failing tests** with a fake `Cipher` (XOR-free: wrap `crypto.Suite` is forbidden here; use a small in-test AEAD built from stdlib `crypto/cipher` AES-GCM): `TestSealOpenRoundTripGeneric` (Secret and Project), `TestOpenWrongKindFails` (AAD binds kind), `TestOpenWrongIDFails`.
- [ ] **Step 2:** `go test ./internal/app/...` → FAIL.
- [ ] **Step 3:** Implement ports and the two generic functions. `Secret.Value` is `[]byte`; JSON base64 is fine.
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5:** Commit `feat(app): ports and generic sealed codec`.

### Task 4: Key hierarchy

**Files:**
- Create: `internal/app/keys.go`
- Test: `internal/app/keys_test.go`

**Interfaces:**
- Consumes: Task 3 ports.
- Produces:
  - `func newVaultKeys(c Cipher, kdf KDF, p KDFParams, password []byte) (meta VaultMeta, vaultKey domain.Key, err error)`: salt, derive unlock key, generate vault key, wrap it with AAD `"vaultkey"`.
  - `func unwrapVaultKey(c Cipher, kdf KDF, meta VaultMeta, password []byte) (domain.Key, error)`: wrong password → `domain.ErrInvalidPassword`; a blob that fails with a *structurally* invalid header → `ErrCorrupt`.
  - `func rewrapVaultKey(c Cipher, kdf KDF, meta VaultMeta, oldPw, newPw []byte) (VaultMeta, error)` (new salt, same vault key).
  - `func newEnvKey(c Cipher, vaultKey domain.Key, envID string) (key domain.Key, wrapped []byte, err error)` / `func unwrapEnvKey(c Cipher, vaultKey domain.Key, envID string, wrapped []byte) (domain.Key, error)` (AAD = env id).

- [ ] **Step 1: Write failing tests** (cheap KDF params, AES-GCM test cipher from Task 3): `TestUnwrapWrongPassword` → `ErrInvalidPassword`; `TestRewrapKeepsVaultKey` (key before == key after with new pw; old pw now fails); `TestEnvKeyWrapBoundToEnvID` (unwrap with other env id fails); `TestTruncatedWrappedKeyIsCorrupt`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Zero intermediate unlock key after use (`crypto.Zero` equivalent: add local `zeroKey(*domain.Key)` in `app`; do not import infra).
- [ ] **Step 4:** Run with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(app): vault and environment key hierarchy`.

### Task 5: SQLite RecordStore

**Files:**
- Create: `internal/infra/sqlite/store.go`, `internal/infra/sqlite/migrate.go`, `internal/infra/sqlite/migrations/0001_init.sql` (embedded), `internal/infra/sqlite/opener.go`
- Test: `internal/infra/sqlite/store_test.go`

**Interfaces:**
- Consumes: `app.RecordStore`, `app.VaultOpener`, `app.VaultMeta`, `app.Record`.
- Produces: `type Opener struct{}` implementing `app.VaultOpener`; stores implement `app.RecordStore`.

Schema (migration 1): `meta(id INTEGER PRIMARY KEY CHECK(id=1), format_version, name, kdf_time, kdf_mem_kib, kdf_threads, salt BLOB, wrapped_vault_key BLOB, created_at)`; `records(kind TEXT, id TEXT, parent_id TEXT, blob BLOB, updated_at INTEGER, PRIMARY KEY(kind,id))` + index on `(kind,parent_id)`; `schema_migrations(version)`.

- [ ] **Step 1: Write failing tests**: `TestCreateSetsMode0600` (file and `-wal` after write); `TestPutGetListDelete`; `TestGetMissingIsNotFound`; `TestTxRollbackOnError`; `TestMigrationsIdempotent` (reopen); `TestOpenGarbageFile` (random bytes file → `ErrCorrupt`, error string contains no file content); `TestTwoConnectionsConcurrentWrites` (two `Opener.Open` on same path, 50 goroutines each writing, no error, `-race`); `TestFutureSchemaVersionRefused` (`schema_migrations` has version 99 → `ErrUnsupportedFormat`).
- [ ] **Step 2:** Run `go test ./internal/infra/sqlite/...` → FAIL.
- [ ] **Step 3:** Implement with `modernc.org/sqlite`: DSN pragmas `journal_mode(WAL)`, `busy_timeout(5000)`, `foreign_keys(1)`; `os.OpenFile(..., 0o600)` before opening; embed migrations with `embed.FS`; `Tx` wraps `BeginTx` and runs `fn` with a tx-bound store.
- [ ] **Step 4:** Run with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(sqlite): record store with migrations, WAL, 0600`.

### Task 6: Registry, session store, clock

**Files:**
- Create: `internal/infra/registry/registry.go`, `internal/infra/keychain/session.go`, `internal/infra/clock/clock.go`
- Test: `internal/infra/registry/registry_test.go`, `internal/infra/keychain/session_test.go`

**Interfaces:**
- Produces: `registry.New(dir string) *Registry` (JSON file `vaults.json`, mode `0600`, atomic write via temp+rename) implementing `app.Registry`; `keychain.New(service string) *SessionStore` implementing `app.SessionStore` (JSON-encodes `Session` into `go-keyring`); `keychain.NewMemory()` in-memory variant for tests and fallback; `clock.System{}`.

- [ ] **Step 1: Write failing tests**: registry `TestAddListRemove`, `TestAddDuplicateIDIsDuplicate`, `TestConcurrentAddNoLoss`; session (using `keyring.MockInit()`): `TestSaveLoadDelete`, `TestLoadMissingIsLocked`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. The registry holds only `{id,name,path}`; use a file lock via atomic rename and an in-process mutex.
- [ ] **Step 4:** Run with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(infra): vault registry, keychain session store, clock`.

### Task 7: Use cases, structure and secrets

**Files:**
- Create: `internal/app/service.go`, `internal/app/session.go`, `internal/app/structure.go`, `internal/app/secrets.go`
- Test: `internal/app/service_test.go`, `internal/app/secrets_test.go`, `internal/app/testsupport_test.go` (in-memory `RecordStore` fake + fixed `Clock`)

**Interfaces:**
- Consumes: Tasks 3–4.
- Produces:
  - `type Service struct` built by `NewService(c Cipher, kdf KDF, p KDFParams, opener VaultOpener, reg Registry, sessions SessionStore, clock Clock) *Service`
  - `func (s *Service) CreateVault(ctx, name, path string, password []byte) (domain.Vault, error)`
  - `func (s *Service) ListVaults(ctx) ([]VaultEntry, error)` (works while locked)
  - `func (s *Service) Unlock(ctx, vaultID string, password []byte, ttl, idle time.Duration) error` (persists a `Session`)
  - `func (s *Service) Lock(ctx, vaultID string) error`
  - `func (s *Service) Open(ctx, vaultID string) (*Unlocked, error)` (from `SessionStore`; expired TTL or idle → delete + `ErrLocked`; success refreshes `LastUsed`)
  - `func (s *Service) OpenWithPassword(ctx, vaultID string, password []byte) (*Unlocked, error)` (no persistence)
  - `func (s *Service) ChangePassword(ctx, vaultID string, oldPw, newPw []byte) error`
  - on `*Unlocked`: `CreateProject(ctx, name, desc string) (domain.Project, error)`, `ListProjects`, `RenameProject`, `DeleteProject` (cascades), `DuplicateProject`, `CreateEnvironment(ctx, projectID, name) (domain.Environment, error)` (generates the env key), `ListEnvironments(ctx, projectID)`, `DeleteEnvironment`, `SetSecret(ctx, envID, name domain.SecretName, value []byte, typ domain.SecretType, desc string) (domain.Secret, error)` (upsert), `GetSecret(ctx, envID, name) (domain.Secret, error)`, `ListSecrets(ctx, envID) ([]domain.Secret, error)`, `DeleteSecret(ctx, envID, name) error`.
  - Defaults `CreateVault` seeds nothing; predefined env names are exported as `var DefaultEnvironments = []string{"Development","Testing","Staging","Production"}` for interfaces to offer.

- [ ] **Step 1: Write failing tests** against the fakes: `TestCreateVaultThenUnlockOpen`; `TestOpenWithoutUnlockIsLocked`; `TestSessionExpiresAtTTL` and `TestSessionIdleTimeout` (advance fake clock → `ErrLocked`); `TestUnlockWrongPassword` → `ErrInvalidPassword`; `TestChangePasswordKeepsSecretsReadable` (existing session still works; old password rejected; no secret record rewritten: count `Put` calls on `KindSecret` = 0); `TestSecretValueRoundTrip` (table: `""`, `"a\nb"`, `"k=v"`, `"\x00x"`, 1 MiB random → byte-exact); `TestSetSecretDuplicateNameUpserts` and `TestCreateSecretCaseSensitive` (`a` and `A` coexist); `TestDeleteProjectCascades`; `TestEnvsHaveDistinctKeys` (secret sealed in env A cannot be opened with env B's key); `TestErrorsNeverContainValue` (trigger `ErrNotFound`/`ErrDuplicate`/`ErrCorrupt` paths, assert `err.Error()` lacks the secret value and password).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement. Secret lookup: list `KindSecret` by `ParentID==envID`, decrypt all, match name (scale target: hundreds/env). Each record uses `seal/open` with AAD kind+id. Write paths run inside `RecordStore.Tx`. Zero passwords/keys no longer needed where practical.
- [ ] **Step 4:** Run with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(app): vault, session, project, environment and secret use cases`.

### Task 8: History and restore

**Files:**
- Create: `internal/app/history.go`
- Modify: `internal/app/secrets.go`, `internal/app/structure.go` (record events inside the same `Tx`)
- Test: `internal/app/history_test.go`

**Interfaces:**
- Produces: `type HistoryOp string` (`secret.create|secret.update|secret.delete|env.create|env.delete`); `type HistoryEntry struct{ ID string; EnvID domain.ID[domain.Environment]; Op HistoryOp; SecretName domain.SecretName; At time.Time; PrevValue, NewValue []byte }` (values stay inside the sealed blob, sealed under the env key); `func (u *Unlocked) History(ctx, envID) ([]HistoryEntry, error)` (values omitted from the returned struct); `func (u *Unlocked) RestoreVersion(ctx, entryID string, password []byte) error` (re-verifies the password via `unwrapVaultKey`, wrong → `ErrInvalidPassword`).

- [ ] **Step 1: Write failing tests**: `TestSetSecretWritesHistory` (create then update → 2 entries, ops correct); `TestHistoryListHasNoValues`; `TestDeleteEnvironmentDeletesItsHistory`; `TestRestoreRequiresPassword` (wrong pw leaves value unchanged); `TestRestoreRestoresPreviousValue`; `TestHistoryAtomicWithWrite` (store `Put` fails on history → secret not written).
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement; history records use `KindHistory`, `ParentID = envID`.
- [ ] **Step 4:** Run with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(app): encrypted history and password-gated restore`.

### Task 9: Env comparison and process environment

**Files:**
- Create: `internal/app/compare.go`, `internal/app/procenv.go`
- Test: `internal/app/compare_test.go`, `internal/app/procenv_test.go`

**Interfaces:**
- Produces: `type Presence map[domain.SecretName]map[domain.ID[domain.Environment]]bool`; `func (u *Unlocked) CompareEnvs(ctx, envIDs ...domain.ID[domain.Environment]) (Presence, error)` (no values in the result type); `func (u *Unlocked) ProcessEnv(ctx, envID, base []string) ([]string, error)`: returns `base` with secrets applied; a secret replaces an existing variable of the same name, all other variables preserved in order; values may contain `=` and newlines.

- [ ] **Step 1: Write failing tests**: `TestCompareEnvsPresence` (REDIS_URL missing in staging); `TestCompareHasNoValues` (reflect: result type contains only bools); `TestProcessEnvOverridesOnlySameName`; `TestProcessEnvKeepsPath`; `TestProcessEnvValueWithEqualsAndNewline`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Run → PASS.
- [ ] **Step 5:** Commit `feat(app): environment comparison and process env builder`.

### Task 10: Import and export parsers

**Files:**
- Create: `internal/app/importexport/dotenv.go`, `json.go`, `yaml.go`, `importexport.go`
- Modify: `internal/app/secrets.go` (add `ImportPlan`/`ApplyImport`)
- Test: `internal/app/importexport/*_test.go`, `internal/app/import_test.go`

**Interfaces:**
- Produces: `type Format string` (`dotenv|json|yaml`); `type Entry struct{ Name domain.SecretName; Value []byte }`; `func Parse(f Format, r io.Reader) ([]Entry, error)` (rejects invalid names with the line number, never the value); `func Write(f Format, w io.Writer, entries []Entry) error`; `type Strategy string` (`skip|overwrite`); `func (u *Unlocked) PlanImport(ctx, envID, entries []Entry) (ImportPlan, error)` (lists new/duplicate names, no values); `func (u *Unlocked) ApplyImport(ctx, envID, plan ImportPlan, s Strategy) error`; `func (u *Unlocked) Export(ctx, envID, names []domain.SecretName, f Format, w io.Writer) error`.
- `.env` rules: `KEY=value`, optional `export `, `#` comments, single/double quotes, `\n` escapes in double quotes, blank lines skipped.

- [ ] **Step 1: Write failing tests**: dotenv table (quotes, `export`, comments, `=` in value, empty value, CRLF); `TestParseInvalidNameReportsLineNotValue`; JSON/YAML flat string maps round-trip; non-string values → error; `TestPlanImportFlagsDuplicates`; `TestApplyImportSkipVsOverwrite`; `TestExportSelectedNamesOnly`.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3:** Implement hand-written dotenv parser (no dependency), `encoding/json`, `yaml.v3`.
- [ ] **Step 4:** Run with `-race` → PASS.
- [ ] **Step 5:** Commit `feat(core): dotenv/json/yaml import and export`.

### Task 11: Wiring, end-to-end and security tests, architecture doc

**Files:**
- Create: `internal/core/core.go` (composition: `func New(dataDir string) (*app.Service, error)` wiring real adapters), `tests/security_test.go`, `tests/e2e_test.go`, `docs/architecture.md`
- Modify: `internal/infra/crypto` (add `ToApp` adapter if the `KDFParams` conversion needs it)

**Interfaces:**
- Produces: `core.New(dataDir string) (*app.Service, error)` — the only entry point CLI/Desktop will call.

- [ ] **Step 1: Write failing tests** (real adapters, `t.TempDir()`, mock keyring, cheap KDF override): `TestE2ECreateUnlockSetCloseReopenGet`; `TestSQLiteFileContainsNoPlaintext` (store a recognizable secret *name* and *value*, project/env names; `bytes.Contains` over the `.db`, `-wal` and `-shm` files finds none; vault name is allowed); `TestFilePermissions0600`; `TestCorruptedBlobDetected` (flip a byte of a `records.blob` via raw SQL → `GetSecret` returns `ErrCorrupt`); `TestSwapBlobsBetweenRecordsFails`; `TestLogsContainNoSecrets` (route `slog` to a buffer during a full flow, assert no value/password); `TestSessionSharedBetweenTwoServices` (two `core.New` on the same dir and mock keyring: unlock on one, open on the other); `TestConcurrentServicesRace` (`-race`).
- [ ] **Step 2:** Run `go test ./tests/...` → FAIL.
- [ ] **Step 3:** Implement `core.New`; write `docs/architecture.md` (layers, key hierarchy diagram, file layout, error semantics, how CLI/Desktop plug in) — project rule: keep this file current.
- [ ] **Step 4:** Run `go vet ./... && go test -race ./...` → all PASS.
- [ ] **Step 5:** Commit `feat(core): composition root, security and e2e tests, architecture doc`.

---

## Self-review notes

- Spec coverage: key hierarchy (T4), encrypted names (T3/T5/T11), per-env keys (T4/T7), password change without re-encrypting (T7), shared session + TTL/idle + clock port (T6/T7), history incl. restore with password (T8), compare without values (T9), `run` env building (T9; process spawning is the CLI plan), import/export formats (T10), plaintext vault names registry (T6), WAL/0600/migrations (T5), security tests (T11). CLI, Desktop, packaging, CI are separate plans.
- Calls made by the planner: module name `secrio`; code under `internal/` (not repo root) so the API is not importable externally; one generic `Record` table instead of per-entity tables; `ParentID` plaintext (leaks which records share a parent, not names); Argon2id start values to be tuned by benchmark; secrets override same-name parent variables in `ProcessEnv`.
