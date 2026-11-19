# Secrio CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the `secrio` binary: `vault create|list`, `unlock`, `lock`, `project create`, `env create`, `set`, `get`, `list`, `run`, on top of the existing Core.

**Architecture:** `cmd/secrio/main.go` (minimal) → `internal/cli` (Cobra commands, one file per command, all I/O, prompts, env and process execution injected through a `Deps` struct) → `internal/core` / `internal/app` (unchanged). The CLI never imports `internal/infra`, and contains no business logic.

**Tech Stack:** Go 1.26, `github.com/spf13/cobra`, `golang.org/x/term`, `github.com/pelletier/go-toml/v2`, stdlib `os/exec`, `os/signal`.

**Spec:** `docs/superpowers/specs/2026-11-19-secrio-cli-design.md` (and `2026-11-19-secrio-design.md` §7).

## Global Constraints

- Binary `secrio`; no CGO; `internal/cli` imports `internal/core`, `internal/app`, `internal/domain` only (never `internal/infra`).
- Defaults: session `--ttl 12h`, `--idle 1h`; data dir `core.DefaultDataDir()` overridable by `SECRIO_HOME`.
- Env vars: `SECRIO_PASSWORD`, `SECRIO_VAULT`, `SECRIO_PROJECT`, `SECRIO_ENV`, `SECRIO_HOME`. Context file `.secrio.toml` with keys `vault`, `project`, `env`, searched from the current directory upwards. Priority: flags > env vars > file.
- Exit codes (outside `run`): 0 success, 1 error, 2 usage, 3 vault locked or invalid password. `run`: the child's code is propagated; Secrio failures before launching exit 125; command not found 127; not executable 126; a child killed by signal N exits 128+N.
- Vault names match `^[A-Za-z0-9._-]{1,64}$` and must not already exist (checked by the CLI).
- `set` takes the value from a masked prompt (stdin is a TTY) or from stdin (one trailing `\n` or `\r\n` removed); never from an argument or flag; an empty value is refused (exit 1); input over 1 MiB refused.
- `get` writes the raw value to stdout, plus `\n` only when stdout is a TTY. `list` never prints values.
- No prompt without a TTY: fail fast with a clear message (exit 1, or 3 for a missing password); never hang.
- Never put secret values, passwords, or keys in argv, errors, logs, or usage output.
- Commits: no `Co-Authored-By`, no "Generated with" line. Never push.

## Review Focus

- Values with newlines, `=`, a trailing `\r\n`, empty, 1 MiB, non-UTF-8: `set`/`get` round-trip byte-exact except the one documented trailing-newline strip (Tasks 5, 7).
- No TTY and no `SECRIO_PASSWORD`: `unlock`, `vault create`, `set` fail fast, no hang, nothing echoed (Tasks 3, 5).
- `run`: exit code propagation, child killed by signal, command not found (127), not executable (126), stdin/stdout/stderr passthrough, Ctrl-C relayed (Task 6).
- `.secrio.toml` malformed, unknown keys, found in a parent directory, vault/project/env names with odd characters: clear error, no panic, no path traversal (Task 2).
- Secrets never reach argv or stderr: `ps`-visible arguments contain no value; every error path is value-free (Tasks 5, 6, 7).

---

### Task 1: Scaffold, Deps, root command, exit codes

**Files:**
- Create: `cmd/secrio/main.go`, `internal/cli/cli.go`, `internal/cli/deps.go`, `internal/cli/errors.go`, `internal/cli/version.go`
- Test: `internal/cli/cli_test.go`

**Interfaces (produced):**
```go
type Deps struct {
    In  io.Reader; Out, Err io.Writer
    Getenv     func(string) string
    Environ    func() []string
    Getwd      func() (string, error)
    StdinTTY   func() bool
    StdoutTTY  func() bool
    ReadPassword func(prompt string) ([]byte, error) // masked, from the terminal
    NewService func() (*app.Service, error)          // wraps core.New(dataDir)
}
func RealDeps() Deps                                  // os.Stdin/Stdout/Stderr, x/term, core.New(SECRIO_HOME or core.DefaultDataDir())
func NewRoot(d Deps) *cobra.Command                   // adds subcommands registered by later tasks via an init list
func Execute(d Deps, args []string) int               // runs the root, maps errors to exit codes, prints "secrio: <msg>" to d.Err
type ExitError struct{ Code int; Err error }          // Unwrap() error; exit code carrier
func UsageErr(format string, a ...any) error          // code 2
func LockedErr(err error) error                       // code 3
const (ExitOK = 0; ExitFailure = 1; ExitUsage = 2; ExitLocked = 3)
var version = "dev"                                   // set by -ldflags
```
Cobra's own flag/arg errors map to exit 2; `SilenceUsage`/`SilenceErrors` set so Cobra prints nothing itself; `--version` prints `secrio <version>`.

- [ ] **Step 1: Write failing tests**: `TestExecuteUnknownCommandIsUsage` (exit 2), `TestExecuteVersion` (`--version` prints `secrio dev`, exit 0), `TestExitErrorMapping` (UsageErr→2, LockedErr→3, plain error→1, `domain.ErrLocked` wrapped→3, `domain.ErrInvalidPassword` wrapped→3), `TestErrorOutputHasPrefixAndNoStack` (stderr is exactly `secrio: <msg>\n`), `TestNoUsageDumpOnError`.
- [ ] **Step 2:** `go test ./internal/cli/...` → FAIL.
- [ ] **Step 3:** Implement; `main.go` is `os.Exit(cli.Execute(cli.RealDeps(), os.Args[1:]))`. Add the cobra and x/term deps via `go get`.
- [ ] **Step 4:** `go vet ./... && go test -race -count=1 ./...` → PASS; `CGO_ENABLED=0 go build ./cmd/secrio`.
- [ ] **Step 5:** Commit `feat(cli): scaffold, deps and exit codes`.

### Task 2: Context resolution and name resolvers

**Files:**
- Create: `internal/cli/context.go`, `internal/cli/resolve.go`
- Test: `internal/cli/context_test.go`, `internal/cli/resolve_test.go`

**Interfaces:**
- Consumes: Task 1 `Deps`, `UsageErr`.
- Produces:
```go
type Flags struct{ Vault, Project, Env string }
type Context struct{ Vault, Project, Env string }
func LoadContext(d Deps, f Flags) (Context, error)            // flags > SECRIO_* env > .secrio.toml (searched upward from d.Getwd)
func (c Context) NeedVault() (string, error)                  // UsageErr "no vault: use --vault, SECRIO_VAULT or .secrio.toml"
func (c Context) NeedProject() (string, error)
func (c Context) NeedEnv() (string, error)
func addContextFlags(cmd *cobra.Command, f *Flags)            // --vault --project --env (persistent on the root)
func resolveVault(ctx context.Context, svc *app.Service, name string) (app.VaultEntry, error)        // by exact name; unknown → error naming the vault
func resolveProject(ctx context.Context, u *app.Unlocked, name string) (domain.Project, error)
func resolveEnv(ctx context.Context, u *app.Unlocked, projectID domain.ID[domain.Project], name string) (domain.Environment, error)
func validVaultName(name string) error                        // regex from Global Constraints
```
Unknown keys in `.secrio.toml` and malformed TOML are errors (exit 1, message names the file path, never file content beyond the parser's line/column).

- [ ] **Step 1: Write failing tests**: priority table (flag beats env beats file); file found in a parent directory; stops at the filesystem root; malformed TOML → error mentioning the path; unknown key → error; `NeedVault` etc. return `UsageErr`; `validVaultName` table (`work`, `a.b-c_d` ok; ``, `a/b`, `..`, `a b`, 65 chars, `é` rejected); resolvers against a real Core in `t.TempDir()` (cheap KDF, `keychain.NewMemory`, fake clock): vault/project/env found by name, unknown → clear error, duplicate vault names listed → first-match is NOT acceptable: return an error "ambiguous vault name" (defensive, even though `vault create` prevents it).
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** `go test -race -count=1 ./...` → PASS. **Step 5:** Commit `feat(cli): context resolution and name resolvers`.

### Task 3: Vault commands, unlock/lock, password input, openVault

**Files:**
- Create: `internal/cli/password.go`, `internal/cli/session.go`, `internal/cli/vault.go`, `internal/cli/unlock.go`
- Test: `internal/cli/password_test.go`, `internal/cli/vault_test.go`, `internal/cli/unlock_test.go`

**Interfaces:**
- Consumes: Tasks 1–2.
- Produces:
```go
func readPassword(d Deps, prompt string, confirm bool) ([]byte, error)
    // SECRIO_PASSWORD wins (no prompt); else needs StdinTTY(): d.ReadPassword(prompt) (twice and compare when confirm); else LockedErr("no terminal and SECRIO_PASSWORD is not set")
func openVault(ctx context.Context, d Deps, svc *app.Service, entry app.VaultEntry) (*app.Unlocked, error)
    // svc.Open; on domain.ErrLocked: if SECRIO_PASSWORD set → svc.OpenWithPassword(...) else LockedErr("vault %q is locked: run `secrio unlock`")
// commands: `vault create <name>`, `vault list`, `unlock [vault] [--ttl 12h] [--idle 1h]`, `lock [vault]`
```
- `vault create`: `validVaultName`, refuse an existing name, path via `core.VaultPath(dataDir, name)` (get the dataDir through a small `Deps.DataDir func() (string, error)` field you add in this task, wired in `RealDeps`), `svc.CreateVault(ctx, name, path, password)`; prints `created vault <name>`; empty password refused by the Core → exit 1 with its message.
- `vault list`: one name per line, sorted.
- `unlock`: vault from the argument or the context; `svc.Unlock(ctx, id, pw, ttl, idle)`; wrong password → exit 3; prints `unlocked <name> (expires in 12h0m0s)`. `lock`: `svc.Lock`; prints `locked <name>`; any Lock error is reported as an error (exit 1) saying the vault may still be unlocked.

- [ ] **Step 1: Write failing tests** (real Core, in-memory I/O, fake `ReadPassword` returning scripted values, `StdinTTY` toggled): create+list; duplicate name refused; invalid name refused (`../x`); confirm mismatch refused; `SECRIO_PASSWORD` used without prompting; no TTY and no env → exit 3, `ReadPassword` never called; wrong password on `unlock` → exit 3 and no session (`openVault` still locked); `unlock` then `openVault` succeeds; `lock` then locked again; `--ttl 0` → usage error (exit 2) ; `TestPasswordNeverInOutput` (stdout/stderr never contain the password across all cases).
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** `go test -race -count=1 ./...` → PASS. **Step 5:** Commit `feat(cli): vault, unlock and lock commands`.

### Task 4: project create, env create, list

**Files:**
- Create: `internal/cli/project.go`, `internal/cli/env.go`, `internal/cli/list.go`
- Test: `internal/cli/structure_test.go`, `internal/cli/list_test.go`

**Interfaces:**
- Consumes: `openVault`, `resolveVault/Project/Env`, `LoadContext`.
- Produces commands: `project create <name>` (vault from context; prints `created project <name>`), `env create <name>` (project from context), `list` (hierarchical by context depth: no project → projects; project but no env → envs; env → secret names; one name per line, sorted; never values). Each command opens the vault with `openVault`, defers `u.Close()`.

- [ ] **Step 1: Write failing tests** (real Core): create project/env then `list` at each depth; duplicate project/env names → exit 1 with the Core's message; `env create` without a project in the context → exit 2; `list` on a locked vault → exit 3; `list` output contains no values (set a secret through the Core with a recognizable value and assert absence); names printed in sorted order.
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** `go test -race -count=1 ./...` → PASS. **Step 5:** Commit `feat(cli): project, env and list commands`.

### Task 5: set and get

**Files:**
- Create: `internal/cli/set.go`, `internal/cli/get.go`
- Test: `internal/cli/secrets_test.go`

**Interfaces:**
- Consumes: Task 4 patterns; `domain.ParseSecretName`; `Unlocked.SetSecret(ctx, envID, name, value, typ, desc)` (use `domain.TypeText` — check the exact constant name in `internal/domain/entities.go` — and empty description) and `GetSecret`.
- Produces: `set <NAME>` — exactly one positional argument (cobra `ExactArgs(1)`; there is no value flag); value source per Global Constraints via `readSecretValue(d Deps) ([]byte, error)`; invalid name → exit 1 with the Core's name error (no value shown); prints `set <NAME>`. `get <NAME>` — value to `d.Out`, `\n` only if `d.StdoutTTY()`; unknown name → exit 1 `secret not found`.

- [ ] **Step 1: Write failing tests**: set from piped stdin then get returns the exact bytes (table: `abc`, `a=b`, `line1\nline2`, `é`, 1 MiB, bytes `\xff\xfe` non-UTF-8, a value ending `\r\n` loses exactly that one terminator, a value ending `\n\n` keeps one `\n`); empty stdin refused (exit 1); input over 1 MiB refused; TTY stdin uses `ReadPassword` (masked, called once) and the value never appears in output; no TTY and empty/closed stdin → fast failure (use a reader that returns EOF; no hang); `get` adds `\n` only when `StdoutTTY` is true; unknown secret → exit 1; `set` with two positional args or a `--value` flag → usage error (exit 2); invalid name `1BAD` → exit 1; overwriting an existing name updates the value; errors and usage text never contain the value.
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** `go test -race -count=1 ./...` → PASS. **Step 5:** Commit `feat(cli): set and get commands`.

### Task 6: run

**Files:**
- Create: `internal/cli/run.go`, `internal/cli/runner.go`
- Test: `internal/cli/run_test.go` (unix only: `//go:build !windows` for tests that spawn `sh`)

**Interfaces:**
- Consumes: `Unlocked.ProcessEnv(ctx, envID, base []string)`; `d.Environ()`.
- Produces: `run [--env X] -- <cmd> [args...]`; zero command args → usage error (exit 2). `type runner func(ctx context.Context, name string, args, env []string, in io.Reader, out, errw io.Writer) (exitCode int, err error)` implemented in `runner.go` with `os/exec`: stdin/stdout/stderr passthrough, `signal.Notify` for SIGINT/SIGTERM (and `os.Interrupt` on Windows) relayed to the child, `signal.Stop` on return; exit code: normal exit → its code; killed by signal N → 128+N; `exec.ErrNotFound` → 127; permission denied → 126; any Secrio-side failure before launch (locked, unknown env, NUL value) → 125 with a message on `d.Err`. Add `Deps.Run runner` (default real) so tests can also inject a recorder.

- [ ] **Step 1: Write failing tests**: with the real runner and `sh`: secret reaches the child (`sh -c 'printf %s "$MY_SECRET"'` stdout equals the value, including a value with `=` and a newline); parent variables preserved, a secret overrides a same-name parent variable; child exit code 7 → secrio returns 7; `sh -c 'kill -TERM $$'` → 143; `nosuchcommand` → 127; a non-executable file → 126; locked vault → 125 with the `unlock` hint; unknown env → 125; no command → 2; the injected-runner test asserts the secret values are only in the `env` argument and NOT in `name`/`args`; stdout of the child passes through byte-exact; stderr of secrio on failure contains no value. A test sends SIGTERM to the test process's own handler path is NOT required; instead unit-test the relay by running `sh -c 'trap "exit 9" TERM; sleep 5'` and delivering the signal through an injectable `signals chan os.Signal` (add it as an unexported parameter of the runner constructor) → exit 9.
- [ ] **Step 2:** run → FAIL. **Step 3:** implement. **Step 4:** `go test -race -count=1 ./...` → PASS. **Step 5:** Commit `feat(cli): run command`.

### Task 7: Binary-level tests, completion, docs

**Files:**
- Create: `tests/cli_binary_test.go`, `README.md`
- Modify: `docs/architecture.md` (CLI section), `internal/cli/*` only if a test exposes a bug

- [ ] **Step 1: Write failing tests** (build the binary once in `TestMain` with `go build -o <tmp>/secrio ./cmd/secrio` and `CGO_ENABLED=0`; every test uses `SECRIO_HOME=<t.TempDir()>`, `SECRIO_PASSWORD`, and an in-memory-keychain-free flow: because the binary uses the real OS keychain, tests for session behaviour use `SECRIO_PASSWORD` only, no `unlock`): full flow `vault create` → `project create` → `env create` → `set` (piped) → `list` → `get` → `run` printing the secret via `sh`; `get` in a pipe has no trailing newline; exit codes 2/3/125/127; `secrio completion bash` exits 0 and prints a script; `--version`; run with no TTY and no `SECRIO_PASSWORD` exits 3 within a few seconds (timeout the test at 10 s); `ps`-style check: the secret never appears in the child's `os.Args` (the child script prints `"$*"` for its own args and the test asserts the value is absent); `.secrio.toml` in a temp project dir supplies vault/project/env so `secrio get NAME` works with no flags.
- [ ] **Step 2:** run → FAIL. **Step 3:** fix anything the tests expose in `internal/cli`; write `README.md` (what Secrio is, build with `go build ./cmd/secrio`, the command table, the `.secrio.toml` format, exit codes, a security note linking to `docs/architecture.md`), and add the CLI section to `docs/architecture.md` (package map entry for `cmd/secrio` and `internal/cli`, `Deps` injection, context resolution, exit codes, what the CLI does not do) — every claim verified against the code.
- [ ] **Step 4:** `gofmt -l .`, `go vet ./...`, `go test -race -count=1 ./...`, `CGO_ENABLED=0 go build ./cmd/secrio` → clean.
- [ ] **Step 5:** Commit `test: binary-level CLI tests; docs: README and CLI architecture`.

---

## Self-review notes

- Spec coverage: commands (T3–T6), context resolution (T2), codes and defaults (Global Constraints, T1/T3/T6), no-TTY behaviour (T3/T5/T7), `SECRIO_PASSWORD` path via `OpenWithPassword` (T3), `vault list`/`--version`/completion (T3/T1/T7), docs (T7).
- Planner calls: `Deps.DataDir` added in T3; the `list` depth rule; empty `set` value refused; ambiguous vault name is an error; the runner's signal channel injectable; binary tests avoid the OS keychain by using `SECRIO_PASSWORD`.
- Out of this plan: the Desktop, import/export, edit/delete, history, compare, change-password commands.
