# Session Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop storing the raw permanent vault key in the OS keychain. The keychain keeps only a random per-session key; the vault key lives wrapped under it in a `sessions` row inside the vault's SQLite file, which `Lock` and expiry delete.

**Architecture:** `RecordStore` gains four session methods (atomic in SQL). `SessionStore` (keychain) shrinks to a per-vault session-key store. `Service.Unlock/Open/Lock` orchestrate both. Cross-process `Lock` vs `Open` becomes atomic, so the in-process `sessionMu` goes away.

**Tech Stack:** unchanged (Go 1.26, `modernc.org/sqlite`, `go-keyring`).

**Spec:** `docs/superpowers/specs/2026-11-19-secrio-design.md` §11 (session bullet amended by this plan).

## Global Constraints

- Same as `2026-11-19-secrio-core.md` Global Constraints (no CGO; `app` never imports `infra`; no secrets in errors/logs/fmt; commits without `Co-Authored-By`/"Generated with"; never push).
- The keychain never holds the vault key, an env key, or a password.
- Session blob: `Cipher.Seal(sessionKey, vaultKey, aad = "session|" + vaultID)`.
- Fail closed: any missing half (row without keychain key, key without row), authentication failure, or expiry yields `domain.ErrLocked` and deletes whatever remains.
- Migration `0002_sessions.sql` through the existing embedded migration runner (first real use of a second migration).

## Review Focus

- `Lock` in one process while `Open` runs in another: `Open` never resurrects a deleted session (Task 2).
- Row present but keychain key missing (or the reverse), tampered blob, session key from another vault: `ErrLocked`, remnants deleted (Task 2).
- Expired row is purged by the next `Open` and its keychain key deleted (Task 2).
- Old vault files without the table: migration 0002 applies cleanly on `Open`; a vault with no session reports `ErrLocked` (Task 1).
- `ChangePassword` keeps existing sessions valid (same vault key) (Task 2).

---

### Task 1: SQLite session row and RecordStore methods

**Files:**
- Create: `internal/infra/sqlite/migrations/0002_sessions.sql`
- Modify: `internal/app/ports.go` (RecordStore + `SessionRow`), `internal/infra/sqlite/store.go`, `internal/app/testsupport_test.go` (fake store)
- Test: `internal/infra/sqlite/session_row_test.go`, `internal/infra/sqlite/migrate_internal_test.go` (0002 applies on a v1 file)

**Interfaces:**
- Produces in `app`: `type SessionRow struct{ Blob []byte; ExpiresAt, LastUsed time.Time; Idle time.Duration }` and on `RecordStore`:
  - `PutSession(ctx, SessionRow) error` (upsert, single row id=1)
  - `GetSession(ctx) (SessionRow, error)` (none → `domain.ErrLocked`)
  - `TouchSession(ctx, now time.Time) (bool, error)`: ONE statement `UPDATE sessions SET last_used=? WHERE id=1 AND expires_at>? AND (idle_ns=0 OR ?-last_used<idle_ns)`; returns whether a row was updated
  - `DeleteSession(ctx) error` (idempotent)
- Table: `sessions(id INTEGER PRIMARY KEY CHECK(id=1), blob BLOB NOT NULL, expires_at INTEGER NOT NULL, last_used INTEGER NOT NULL, idle_ns INTEGER NOT NULL)`, times in unix milliseconds.

- [ ] **Step 1: Write failing tests**: `TestPutGetSessionRoundTrip` (all fields), `TestGetSessionNoneIsLocked`, `TestTouchSessionUpdatesLastUsed`, `TestTouchSessionFalseWhenExpired` and `...WhenIdleElapsed` and `...WhenDeleted`, `TestDeleteSessionIdempotent`, `TestTouchVsDeleteAcrossTwoStores` (two stores on one file: Delete then Touch → false; 50 racing goroutines of Touch against one Delete end with the row gone and no Touch true after the Delete completed), `TestMigration0002AppliesToV1File` (create a v1-only file with the injected migration list, reopen → table exists), session methods work inside `Tx`.
- [ ] **Step 2:** `go test ./internal/infra/sqlite/...` → FAIL.
- [ ] **Step 3:** Implement the migration and the four methods (plain parameterized SQL; `GetSession` maps `sql.ErrNoRows` to `ErrLocked`). The in-memory fake store in `testsupport_test.go` gets the same four methods with the same semantics.
- [ ] **Step 4:** `go vet ./... && go test -race -count=1 ./...` → PASS.
- [ ] **Step 5:** Commit `feat(sqlite): sessions table and atomic session row operations`.

### Task 2: Service session flow with a session key

**Files:**
- Modify: `internal/app/ports.go` (replace `Session`/`SessionStore` by `SessionKeys`), `internal/app/service.go` (`Unlock`, `Lock`, drop `sessionMu`), `internal/app/session.go` (`Open`), `internal/app/testsupport_test.go`
- Test: `internal/app/service_test.go`, `internal/app/session_test.go`

**Interfaces:**
- Consumes: Task 1 methods.
- Produces: `type SessionKeys interface{ Save(ctx, vaultID string, k domain.Key) error; Load(ctx, vaultID string) (domain.Key, error) /* miss → domain.ErrLocked */; Delete(ctx, vaultID string) error /* idempotent */ }`; `NewService(c, kdf, p, opener, reg, keys SessionKeys, clock)`. Public `Service` method signatures do not change.
- Flow:
  - `Unlock(ctx, vaultID, password, ttl, idle)`: verify password → `NewKey()` as session key → seal vault key (AAD `"session|"+vaultID`) → `store.PutSession` → `keys.Save`. If `keys.Save` fails, delete the row.
  - `Open(ctx, vaultID)`: open store → `keys.Load` (miss → delete row, `ErrLocked`) → `store.TouchSession(now)`; `false` → `Lock`-equivalent cleanup (delete row + key), `ErrLocked` → `store.GetSession` (miss → `ErrLocked`) → `Cipher.Open` (auth failure → cleanup, `ErrLocked`) → return `*Unlocked`. Zero the session key and temp vault-key copy on every path.
  - `Lock(ctx, vaultID)`: `store.DeleteSession` FIRST, then `keys.Delete`.

- [ ] **Step 1: Write failing tests** (fake store, in-memory `SessionKeys` fake, fake clock): `TestUnlockThenOpen`; `TestOpenAfterLockIsLocked`; `TestSessionExpiresAtTTL` and `TestSessionIdleTimeout` (expired row purged and key deleted: assert both gone); `TestOpenRefreshesLastUsedNotExpiry`; `TestRowWithoutKeyIsLocked` and `TestKeyWithoutRowIsLocked` (remnants deleted); `TestTamperedSessionBlobIsLocked`; `TestSessionKeyFromOtherVaultIsLocked` (AAD binding); `TestChangePasswordKeepsSessionValid`; `TestKeychainNeverReceivesVaultKey` (the `SessionKeys` fake records every key; none equals the vault key); `TestLockNotUndoneByConcurrentOpen` rewritten against the atomic touch (fake store whose `TouchSession` blocks while `Lock` runs → final `Open` is `ErrLocked`); remove the old `sessionMu` test and mutex.
- [ ] **Step 2:** run → FAIL. **Step 3:** implement as above. **Step 4:** `go vet ./... && go test -race -count=1 ./...` → PASS.
- [ ] **Step 5:** Commit `feat(app): session key in keychain, vault key wrapped in the vault file`.

### Task 3: Keychain adapter, wiring, black-box tests, docs

**Files:**
- Modify: `internal/infra/keychain/session.go` (store only the 32-byte session key, base64, service `secrio`, user = vaultID; `MemorySessionStore` adapted), `internal/core/core.go` (`WithSessionStore` → `WithSessionKeys(app.SessionKeys)`), `tests/*` (session tests), `docs/architecture.md`, `docs/superpowers/specs/2026-11-19-secrio-design.md` §11
- Test: `internal/infra/keychain/session_test.go`, `tests/security_test.go`

- [ ] **Step 1: Write failing tests**: keychain round trip of a key; miss → `ErrLocked`; undecodable payload or wrong length → `ErrCorrupt` (no payload in message); idempotent delete; black-box `TestKeychainHoldsNoVaultKey` (after `Unlock` through `core.New`, scan the keychain mock's stored bytes: not the vault key, only 32 bytes of key material; the vault file's `sessions.blob` is ciphertext and differs from the vault key), `TestSessionSharedBetweenTwoServices` (existing, adapt), `TestLockInOneServiceKillsSessionInOther`, `TestLeakedKeychainItemUselessAfterLock` (copy the keychain value before `Lock`; after `Lock` and re-`Unlock`, the old session key opens nothing), `TestOldVaultFileGainsSessionsTable` (a vault created with only migration 0001 opens and unlocks).
- [ ] **Step 2:** run → FAIL. **Step 3:** implement; update docs: architecture §sessions/§10 threat model (what the keychain holds now; TTL enforced by data not only by app; remaining limit: a same-user attacker reading BOTH keychain and vault file during a live session still gets the vault key; handles do not observe Lock — unchanged), remove the cross-process Lock-vs-Open limit, update spec §11 session bullet.
- [ ] **Step 4:** `gofmt -l .`, `go vet ./...`, `go test -race -count=1 ./...`, `CGO_ENABLED=0 go build ./...` → all clean.
- [ ] **Step 5:** Commit `feat(keychain): store only a per-session key; wire and document`.

---

## Self-review notes

- Spec coverage: session shared between clients (T2/T3), TTL/idle in the Core via clock port (T2), no raw vault key in the keychain (T2/T3), cross-process Lock race closed (T1/T2). The CLI/Desktop plans are untouched.
- Honest limit kept in the docs: no protection against a same-user attacker who reads both the keychain item and the vault file while a session is live; what improves is that a stale or backed-up keychain item (or vault file alone) is useless, and `Lock`/expiry make the credential dead.
- Planner calls: session row lives in the vault's own SQLite file (so `Lock` is one transactional `DELETE`); one session per vault; times in unix ms.
