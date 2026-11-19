# Secrio Desktop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the Secrio desktop app (Wails): unlock/create a vault, pick a project, manage envs and secrets, import a `.env`, copy with auto-clearing clipboard, lock — in a dark UI based on the Better Design `linear` system and the user's reference screenshot (variant A: project picker, then sidebar of envs + secret table + detail panel).

**Architecture:** `internal/desktop` is a pure-Go facade (no Wails import) over `internal/core`/`internal/app`: each bound method opens the vault, acts, closes, and returns value-free DTOs. `cmd/secrio-desktop` (build tag `desktopapp`) is the only Wails importer: it wires real adapters (clipboard, file dialog, startup/shutdown hooks) into the facade. `desktop/frontend` is React + TypeScript + Vite + Tailwind with components from the Better Design kit.

**Tech Stack:** Go 1.26, Wails (v2 by default, decided by Task 1), React + TypeScript + Vite + Tailwind, Better Design kit `linear` (shadcn install), Vitest + Testing Library, pnpm.

**Spec:** `docs/superpowers/specs/2026-11-19-secrio-desktop-design.md`

## Global Constraints

- Core packages stay CGO-free. `internal/desktop` must not import Wails or `internal/infra`; only `cmd/secrio-desktop` (guarded by build tag `desktopapp`) imports Wails, so `go build ./...`, `go vet ./...` and `go test -race ./...` keep working with `CGO_ENABLED=0`. Wails builds pass `-tags desktopapp`.
- Facade DTOs, errors and logs never contain secret values or passwords. Only `RevealSecret` returns a value. `CopySecret` writes the clipboard in Go; the value never reaches JS.
- Every bound method opens the vault per call (`svc.Open`, then `defer u.Close()`); no handle is kept between calls. `domain.ErrLocked` surfaces as error code `locked`.
- Errors returned to JS are `*apiError{Code, Message}` whose `Error()` is the JSON `{"code":"…","message":"…"}`; codes: `locked, invalid_password, not_found, duplicate, invalid_name, invalid_value, conflict, unsupported, no_vault, internal`.
- Session defaults: ttl 12h, idle 1h (as the CLI). Clipboard auto-clear 30 s, only if the clipboard still holds our value (compare SHA-256 of the current text with the stored hash; never keep the plaintext).
- Webview: only embedded local assets, no network access, strict CSP, no devtools in release builds, no `eval`, no secrets in URL/storage.
- UI: Better Design workflow is mandatory for every screen: kit installed (`get-design-system-kit`, designSystemId `linear`, target `react`), `get-ui-principle` before styling, `get-ux-principle` before behaviour, review (`get-review-rules`, `check-comprehension` per panel, `inspect-spacing` on rendered DOM) with every critical/serious finding fixed. Implementers load these tools with ToolSearch. Dark theme; use the kit's `Sidebar`, `Table`, dialogs; no hand-rolled replacements.
- No Zustand, no router: local React state and a small state machine (`unlock | projects | project`).
- Measure and report cold start time and idle memory of the built binary (Tasks 1 and 14).
- Commits: no `Co-Authored-By`, no "Generated with" line. Never push.

## Review Focus

- A project/env/secret list response never contains a value, and `RevealSecret`/`SaveSecret` errors never echo one (Tasks 5, 9).
- Clipboard: a value copied by the user from another app within the 30 s is never cleared by us; closing the app and locking clear our own copy; unreadable clipboard → no clearing and the UI says so (Task 6).
- Import: preview then apply must refuse when the file or vault changed in between (Task 7).
- A session that expires or is locked by the CLI mid-use returns to the unlock screen on the next action without losing unsaved dialog input silently (Tasks 3, 10, 12).
- Starting the app with an existing session opens directly on the project picker; closing the app locks the vault (Tasks 8, 10).
- Passwords and typed values are cleared from component state right after submit (Tasks 10, 12).

---

## Phase A — Go side

### Task 1: Wails spike and version decision

**Files:** Create `docs/superpowers/notes/2026-11-19-wails-spike.md` (the result). Work in a throwaway directory OUTSIDE the repo (scratchpad); nothing else is committed.

- [ ] **Step 1:** Install the Wails CLI for v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`) and, separately, v3 if its CLI installs cleanly (`wails3`); record `wails doctor` output (CGO/Xcode CLT status on macOS, Node, pnpm).
- [ ] **Step 2:** For each version, scaffold a React+TypeScript template, build a release binary, run it, and record: cold start to first paint (best of 5, state how measured), resident memory 10 s after launch (`ps -o rss`), binary size. If a GUI cannot be launched in the agent environment, say so and leave those cells "unverified".
- [ ] **Step 3:** Verify and record for each version: (a) clipboard read AND write from Go (exact API or absence), (b) a shutdown/before-close hook where Go code can run, (c) native open-file dialog from Go, (d) TypeScript bindings generated from a Go struct with a method returning a struct and an error, (e) `-tags desktopapp` accepted by the build command, (f) builds with Go 1.26 and the repo's module layout (a main package under `cmd/` inside the existing module).
- [ ] **Step 4:** Write the note: table of results, the decision (v2 unless v3 clearly wins on these criteria), and the exact APIs to use for clipboard, dialogs, hooks. Commit `docs: wails spike result`. The controller records the version ruling in the ledger; later tasks use that version.

### Task 2: Core `SecretCount`

**Files:** Modify `internal/app/secrets.go`; Test `internal/app/secrets_test.go`

**Interfaces:** Produces `func (u *Unlocked) SecretCount(ctx context.Context, envID domain.ID[domain.Environment]) (int, error)` — counts the env's secret records WITHOUT decrypting values or names; unknown env → `domain.ErrNotFound`; after `Close` → `domain.ErrLocked` (use `enter()` like every method).

- [ ] **Step 1:** Failing tests: `TestSecretCountEmptyAndFilled` (0, then after 3 `SetSecret`s → 3), `TestSecretCountUnknownEnv`, `TestSecretCountDoesNotDecrypt` (a fake cipher whose `Open` panics/fails after setup: count still works), `TestSecretCountAfterClose` (ErrLocked), `TestSecretCountPerEnvIsolated`.
- [ ] **Step 2:** run → FAIL. **Step 3:** implement via `RecordStore.List(KindSecret, envID)` and `len` (check the env exists first, as `ListSecrets` does). **Step 4:** `go vet ./... && go test -race -count=1 ./...`. **Step 5:** Commit `feat(app): SecretCount without decrypting`.

### Task 3: Facade core — deps, errors, vault session

**Files:** Create `internal/desktop/{app.go,deps.go,errors.go,dto.go,prefs.go}`; Test `internal/desktop/{vault_test.go,errors_test.go,testhelpers_test.go}`

**Interfaces (produced):**
```go
type Deps struct {
    NewService func() (*app.Service, error)
    DataDir    func() (string, error)
    Clipboard  Clipboard                       // Get/Set text; Task 6
    Dialogs    Dialogs                         // OpenFile; Task 7
    AfterFunc  func(d time.Duration, f func()) (stop func() bool)  // injectable timer (default time.AfterFunc)
}
type App struct{ /* deps, current vault id (guarded by a mutex) */ }
func New(d Deps) *App
type VaultDTO struct{ ID, Name string }
func (a *App) ListVaults() ([]VaultDTO, error)
func (a *App) CreateVault(name, password string) (VaultDTO, error)   // validates name like the CLI (letters/digits/._-, 1..64, not . / .. / leading . or -, no Windows reserved, case-insensitive duplicate refused), creates via core.VaultPath, then unlocks (ttl 12h/idle 1h) and selects it
func (a *App) Unlock(vaultID, password string) error                 // selects the vault on success
func (a *App) Lock() error                                           // Lock current vault + clears our clipboard copy (Task 6 hook)
func (a *App) SelectVault(vaultID string) error                      // remembers it in desktop.json
func (a *App) CurrentVault() (vault VaultDTO, unlocked bool, err error)  // last selected (from desktop.json); unlocked = svc.Open succeeds (closed immediately)
func (a *App) Shutdown()                                             // Lock current vault + clipboard clear; idempotent
type apiError struct{ Code, Message string }                         // Error() = JSON; Unwrap-free
func toAPIError(err error) error                                     // maps domain errors to codes; never includes the original message for internal errors beyond a fixed text
```
`desktop.json` (`{"lastVault":"<id>"}`, mode 0600, atomic write) lives in the data dir; a missing/corrupt file means "no last vault". The `vault`-name validation helpers may be shared with the CLI only by copying the small rules (do NOT import `internal/cli`).

- [ ] **Step 1: Write failing tests** (real Core via `core.New(t.TempDir(), cheap KDF, memory session keys, fake clock)` — test files may import `internal/infra` fixtures like the CLI tests): create+list; creating selects and unlocks (`CurrentVault` unlocked=true); duplicate/case-duplicate/invalid/reserved names refused (`invalid_name`/`duplicate`); empty password refused (`invalid_value`); wrong password → `invalid_password` and stays locked; `Lock` then any `Open`-based call → `locked`; session created by another service on the same dir (as the CLI would) → `CurrentVault` unlocked=true without a password; `desktop.json` round-trip, corrupt file ignored, mode 0600; `Shutdown` locks and is idempotent; `apiError` JSON shape; the password never appears in any error string; guard test: non-test imports of `internal/desktop` contain no `internal/infra`, no Wails.
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** `go vet ./... && go test -race -count=1 ./...`; `CGO_ENABLED=0 go build ./...`. **Step 5:** Commit `feat(desktop): facade core, errors and vault session`.

### Task 4: Facade — projects and envs

**Files:** Create `internal/desktop/structure.go`; Test `internal/desktop/structure_test.go`

**Interfaces:** `type ProjectDTO struct{ ID, Name string; EnvCount, SecretCount int }`, `type EnvDTO struct{ ID, Name string; SecretCount int }`; methods `ListProjects() ([]ProjectDTO, error)` (name-sorted; counts via `ListEnvironments` + `SecretCount`), `CreateProject(name string) (ProjectDTO, error)`, `DeleteProject(projectID string) error`, `ListEnvironments(projectID string) ([]EnvDTO, error)`, `CreateEnvironment(projectID, name string) (EnvDTO, error)`, `DeleteEnvironment(envID string) error`. IDs are the Core's string ids. No current vault → `no_vault`; vault locked → `locked`.

- [ ] **Step 1: Failing tests:** create/list with correct counts (3 secrets seeded through the Core across 2 envs); delete project cascades (secrets gone, counts updated); delete env; duplicate and invalid names (`duplicate`, `invalid_name` incl. control characters); `no_vault`; `locked` after `Lock`; IDs of another project rejected as `not_found`; no value anywhere in the DTO JSON (seed `s3cr3t-VALUE`, marshal all DTOs, assert absent).
- [ ] **Step 2–5:** run FAIL → implement → `go test -race -count=1 ./...` → commit `feat(desktop): project and env methods`.

### Task 5: Facade — secrets

**Files:** Create `internal/desktop/secrets.go`; Test `internal/desktop/secrets_test.go`

**Interfaces:** `type SecretDTO struct{ Name, Type, Description string; CreatedAt, UpdatedAt string /*RFC3339 UTC*/ }`; methods `ListSecrets(envID string) ([]SecretDTO, error)` (name-sorted, value cleared immediately after listing), `SaveSecret(envID, name, value, typ string) error` (create or update; `typ` one of the eight Core types, empty → `text`; name via `domain.ParseSecretName` → `invalid_name`; value empty or containing NUL or over 1 MiB → `invalid_value`), `DeleteSecret(envID, name string) error`, `RevealSecret(envID, name string) (string, error)` (unknown → `not_found`).

- [ ] **Step 1: Failing tests:** list DTOs have no value (marshal-and-search); save then reveal round-trips (table: `abc`, `a=b`, `l1\nl2`, `é`, 1 MiB); empty/NUL/1 MiB+1 → `invalid_value`; invalid name `1BAD`/`a-b` → `invalid_name` and the value never in the error; unknown type → `invalid_value`; update changes `UpdatedAt` and keeps `CreatedAt`; delete then reveal → `not_found`; `locked` after Lock; env of another vault/unknown → `not_found`; every error string free of the value (table over failures).
- [ ] **Step 2–5:** FAIL → implement (zero buffers after use) → pass → commit `feat(desktop): secret methods`.

### Task 6: Facade — clipboard copy with conditional clearing

**Files:** Create `internal/desktop/clipboard.go`; Test `internal/desktop/clipboard_test.go`

**Interfaces:** `type Clipboard interface{ GetText() (string, error); SetText(string) error }` (a `GetText` that returns `ErrClipboardUnreadable` models the unsupported-read case); `type CopyResult struct{ ClearInSeconds int }` (0 = will not be cleared automatically); method `CopySecret(envID, name string) (CopyResult, error)`: opens the vault, reads the value, `SetText`, stores only `sha256(value)` and arms the 30 s timer (previous timer stopped); on fire: `GetText`, clear (`SetText("")`) only if `sha256(current) == stored`; `clearOurClipboard()` used by `Lock` and `Shutdown` with the same condition; if `GetText` is unreadable at copy time → `ClearInSeconds: 0` and never clear.

- [ ] **Step 1: Failing tests** (fake `Clipboard`, fake `AfterFunc` that lets the test fire timers): copy sets the text and returns 30; firing clears when unchanged; firing does NOT clear when the clipboard now holds something else; copying twice re-arms (first timer stopped); `Lock` and `Shutdown` clear when still ours and not otherwise; unreadable clipboard → `ClearInSeconds == 0`, no clear ever; the plaintext is not retained (a reflection/`%v` dump of the `App` after copy contains no value); copy of an unknown secret → `not_found` and clipboard untouched; `SetText` failure → `internal` error without the value.
- [ ] **Step 2–5:** FAIL → implement → pass → commit `feat(desktop): clipboard copy with conditional clearing`.

### Task 7: Facade — `.env` import without server state

**Files:** Create `internal/desktop/import.go`; Test `internal/desktop/import_test.go`

**Interfaces:** `type Dialogs interface{ OpenFile(title string) (string, error) }`; `type ImportPreview struct{ New, Duplicate []string }`; methods `PickEnvFile() (string, error)` (via `Dialogs`; empty string = cancelled), `PreviewImport(envID, path string) (ImportPreview, error)` (reads the file through `importexport.Parse(dotenv)`, plans with `PlanImport`, returns names only), `ApplyImport(envID, path, strategy string) error` (`strategy` = `skip|overwrite`; re-reads the file, re-plans and returns error code `conflict` when the new/duplicate name sets differ from what the preview showed — the facade stores only the SHA-256 of the previous preview's sorted name sets per (envID, path), never values).

- [ ] **Step 1: Failing tests:** preview lists new vs duplicate names, no values anywhere; apply skip/overwrite behaviour (overwrite keeps type/description, history is the Core's); apply without a prior preview → `conflict`; file changed between preview and apply (add a key) → `conflict` and nothing imported; vault changed in between (another process adds a duplicate) → `conflict`; invalid name in the file → error naming the LINE only (no value); file larger than the importexport cap → `invalid_value`; unreadable path → `not_found`; `locked`; the apply is atomic (the Core's Tx).
- [ ] **Step 2–5:** FAIL → implement → pass → commit `feat(desktop): env import preview and apply`.

## Phase B — Wails shell and UI

### Task 8: Wails shell, adapters, frontend scaffold

**Files:** Create `cmd/secrio-desktop/{main.go,adapters.go}` (`//go:build desktopapp`), `desktop/wails.json`, `desktop/frontend/*` scaffold (Vite + React + TS + Tailwind, pnpm), `desktop/README` section in the top README

Uses the Wails version decided in Task 1. For v2: `options.App{ Bind: []any{app}, AssetServer: embed of desktop/frontend/dist, OnStartup, OnBeforeClose/OnShutdown: app.Shutdown, Frameless:false, BackgroundColour dark }`; adapters implement `desktop.Clipboard` and `desktop.Dialogs` with the runtime APIs found in the spike. If v3 was chosen, port the same structure (application services, window close event) — the controller rules on the mapping.

- [ ] **Step 1: Tests:** Go-side: `TestDesktopMainBuildTagOnly` (`go list -tags desktopapp ./cmd/secrio-desktop` works; without the tag `go build ./...` ignores it) and the facade guard from Task 3 still passes; a unit test for the adapter error mapping if separable. Frontend: Vitest smoke test renders `<App/>` with the mock API.
- [ ] **Step 2:** scaffold; strict CSP meta/headers; no remote URLs; devtools only in dev builds. **Step 3:** `wails build -tags desktopapp` produces the app; generated TypeScript bindings are committed (or generated in CI: decide per spike and state in README). **Step 4:** `go vet ./... && go test -race -count=1 ./... && (cd desktop/frontend && pnpm test)`; `CGO_ENABLED=0 go build ./...`. **Step 5:** Commit `feat(desktop): wails shell and frontend scaffold`.

### Task 9: Design system, principles, API client and mock

**Files:** Create/modify `desktop/frontend/src/{api/client.ts,api/mock.ts,api/types.ts,api/errors.ts}`, `components/ui/*` (kit), theme tokens/fonts, `desktop/frontend/src/lib/*`

- [ ] **Step 1:** Load Better Design tools (ToolSearch); call `get-ui-principle` and `get-ux-principle` for the relevant topics (dense table app, destructive actions, forms, empty states, lock/security UX) and record the key rules you apply in a short comment block in `docs/superpowers/notes/2026-11-19-desktop-ui-notes.md`. Run `get-design-system-kit` (designSystemId `linear`, target `react`), run its install command, keep existing dependency versions (report conflicts and stop). Await/verify the installed font loads.
- [ ] **Step 2:** `api/types.ts` mirrors the Go DTOs exactly (VaultDTO, ProjectDTO, EnvDTO, SecretDTO, ImportPreview, CopyResult, ApiError). `api/client.ts` wraps the generated bindings: every method returns typed data or throws `ApiError` parsed from the JSON error string (unknown strings → code `internal`). `api/mock.ts` is an in-memory implementation of the SAME interface (used by Vitest and by the browser dev server via `VITE_MOCK_API=1`) including `locked` after a mock lock, a mock clipboard, and seed data (projects hotenb/portfolio/infra, envs Development/Staging/Production, realistic secrets).
- [ ] **Step 3: Tests:** `ApiError` parsing table (valid JSON, garbage, empty); client maps a thrown JSON string to the right code; mock behaviours (reveal returns the seeded value only through `revealSecret`; list responses have no `value` key).
- [ ] **Step 4:** `pnpm test`, `pnpm build`. **Step 5:** Commit `feat(desktop): design system kit, api client and mock`.

### Task 10: App shell, unlock and first-run screens

**Files:** Create `desktop/frontend/src/{App.tsx,state/machine.ts,screens/Unlock.tsx,screens/CreateVault.tsx,components/TopBar.tsx}`; Tests alongside

State machine: `boot → (CurrentVault unlocked ? projects : unlock)`; `unlock | create-vault | projects | project(projectId)`; any `locked` ApiError from any call → `unlock` (keep the current selection to return to); the top bar has the vault name, a vault switcher when several vaults exist, and **Lock**.

- [ ] **Step 1: Failing tests (Vitest + Testing Library, mock API):** boot with an existing session lands on projects; boot locked shows unlock with the last vault preselected; no vault → create-vault form (name, password, confirm; mismatch and empty refused client-side with clear messages); wrong password shows an inline error and clears the field; the password field is cleared from state after submit (success and failure); Lock returns to unlock; a `locked` error thrown by any later call returns to unlock; keyboard: Enter submits, focus lands on the password field.
- [ ] **Step 2:** run → FAIL. **Step 3:** build with kit components only (`Sidebar`, `Table`, `Dialog`, form inputs/buttons from `components/ui`), app-scale titles, one primary action per view. **Step 4:** `pnpm test && pnpm build`. **Step 5:** Commit `feat(desktop): shell, unlock and first-run screens`.

### Task 11: Project picker

**Files:** Create `desktop/frontend/src/screens/Projects.tsx`, `dialogs/NewProject.tsx`, `dialogs/ConfirmDelete.tsx`; Tests alongside

- [ ] **Step 1: Failing tests:** cards show name, env count, secret count, env names; search filters; opening a card goes to the project screen; "New project" dialog (validation message from `invalid_name`/`duplicate` shown inline, dialog stays open with its input); delete project requires typing nothing but a confirm dialog naming the project and the number of secrets; empty state with the primary action; loading and error states; keyboard navigation of cards.
- [ ] **Step 2–5:** FAIL → implement (cards use the kit's raised surface) → `pnpm test && pnpm build` → commit `feat(desktop): project picker`.

### Task 12: Project view — envs, secrets, detail panel

**Files:** Create `desktop/frontend/src/screens/Project.tsx`, `components/{EnvSidebar,SecretTable,SecretDetail}.tsx`, `dialogs/{SecretForm,NewEnv}.tsx`, `hooks/useReveal.ts`; Tests alongside

Behaviour: sidebar (kit `Sidebar`) lists envs with counts, "+ env", delete env (confirm); the table (kit `Table`) shows name, masked value placeholder (a fixed run of dots, NOT derived from the length), type badge, updated date, actions view/copy/edit/delete, one line per row; selecting a row opens the detail panel on the right (name, masked value with reveal and copy, type, created/updated). **Reveal**: `revealSecret` on demand, value held only in component state, auto-hidden after 10 s and when the selection changes, the window loses focus, or the vault locks; **Copy**: `copySecret` → toast "Copied, clears in 30 s" or "Copied, cannot auto-clear on this system" from `ClearInSeconds`; **Create/edit**: dialog with name (disabled when editing), value (masked input with show toggle), type select (eight types, `text` default); **delete** with confirm; search filters by name; empty env state with primary action "New secret" and secondary "Import .env" (Task 13 wires it).

- [ ] **Step 1: Failing tests:** table never contains a value in the DOM or accessible text (seed `s3cr3t-VALUE`, assert absent after render and after selection, before reveal); reveal shows the value then hides after 10 s (fake timers), on selection change, on blur, on lock; copy toasts for both `ClearInSeconds` cases and never renders the value; create → row appears, edit keeps the name fixed and updates, delete confirm removes; `invalid_name`, `invalid_value` (empty), `duplicate`-like errors shown inline with input preserved; the value input is cleared from state after a successful save and after the dialog closes; switching env reloads the table; `locked` mid-action returns to unlock and, after unlocking again, the dialog input is NOT silently restored (a short notice says the action was not saved); a sidebar count updates after create/delete; large list (500 rows) renders without long frames (virtualise or paginate if the kit table does not cope — measure and decide, note it).
- [ ] **Step 2:** FAIL. **Step 3:** build with kit components; handle focus management in dialogs (focus trap, Escape closes, focus returns). **Step 4:** `pnpm test && pnpm build`. **Step 5:** Commit `feat(desktop): project view with env sidebar, secret table and detail panel`.

### Task 13: Import dialog

**Files:** Create `desktop/frontend/src/dialogs/ImportEnv.tsx`; wire into Project screen; Tests alongside

- [ ] **Step 1: Failing tests:** flow pick file → preview (new vs duplicate lists, counts, no values) → choose Skip/Overwrite (default skip, overwrite shows a plain warning that existing values change) → apply → table reloads and a toast reports counts; cancelled picker does nothing; `conflict` on apply shows "the file or the vault changed, preview again" and returns to the preview; error states (invalid name line, too large, unreadable); nothing about values anywhere in the DOM.
- [ ] **Step 2–5:** FAIL → implement → `pnpm test && pnpm build` → commit `feat(desktop): env import dialog`.

### Task 14: Review gates, measurements, docs

**Files:** Modify `docs/architecture.md` (new Desktop section), `README.md`; Create `docs/superpowers/notes/2026-11-19-desktop-review.md`

- [ ] **Step 1:** Serve the frontend with the mock API (`VITE_MOCK_API=1 pnpm dev`) and run the Better Design review gates on every screen/panel: `get-review-rules`, `check-comprehension` once per panel with its real copy, `inspect-spacing` on the rendered page (`newScreen: true`) at desktop width; fix every critical and serious finding; the LAST `inspect-spacing` run must report none (otherwise name the remaining finding in the note and do not call the screen done). Record screenshots/paths and results in the review note.
- [ ] **Step 2:** Build the release binary (`wails build -tags desktopapp`), measure cold start and idle memory (same method as the spike), compare with the spike's numbers and the criteria (fast, light), record in the note and README.
- [ ] **Step 3:** Docs: architecture section for the Desktop (package map entries, facade contract, DTOs, error codes, value flow, clipboard rules, lock on close, mock API, build tag, packaging commands, known limits: password/value pass through JS memory when typed; copy/reveal windows; no history/compare/settings yet) verified against the code; README Desktop section (build, run, dev with mock).
- [ ] **Step 4:** `gofmt -l .`, `go vet ./...`, `go test -race -count=1 ./...`, `CGO_ENABLED=0 go build ./...`, `(cd desktop/frontend && pnpm test && pnpm build)`, `wails build -tags desktopapp`. **Step 5:** Commit `docs: desktop architecture, review notes and measurements`.

---

## Self-review notes

- Spec coverage: unlock/create (T3, T10), projects/envs (T4, T11, T12), secrets CRUD + reveal/copy (T5, T12), import (T7, T13), lock + close (T3, T8, T10), clipboard rules (T6), measurements (T1, T14), design system + review gates (T9, T14).
- Planner calls: build tag `desktopapp` to keep CGO-free repo checks; hashes (not plaintext) for clipboard and import conflict detection; error code `conflict` added for import; `ListProjects` counts via `SecretCount`; the mock API doubles as browser dev server and test double; v3 branch is ruled by the controller after Task 1.
- Out of this plan: comparison, history/restore, settings, export, palette, renames, Windows/Linux packaging, signing/notarisation.
