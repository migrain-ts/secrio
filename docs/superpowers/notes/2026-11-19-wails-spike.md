# Wails spike: v2 vs v3 (Task 1)

Date: 2026-11-19. All spike code lived outside the repo (scratchpad); only this note is committed.
"verified" = observed by running code on this machine; "docs" = read from docs/source only.

## Environment

- macOS 26.6.2 (25G83), Apple M1 Max, 32 GB. Go 1.26.1 darwin/arm64, Node 26.5.0, npm 11.17.0, pnpm 10.14.0.
- Xcode 27.1 + CLT 2416 (`xcode-select -p` = `/Applications/Xcode.app/Contents/Developer`).
- `wails doctor` (CLI v2.17.1, latest v2): "Your system is ready for Wails development!" (upx, nsis optional, absent).
- `wails3 doctor` (CLI v3.1.0, latest stable v3): "No issues found", CGO_ENABLED=1, macOS signing not configured.
- Install: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`, `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`.
  `wails3 build` runs Task, which calls `wails3` by name, so `$(go env GOPATH)/bin` must be on PATH.

## Results

Same minimal React+TS frontend in both (title + one button, Vite release build), release build, `-tags desktopapp`.

| Criterion | Wails v2 (v2.17.1) | Wails v3 (v3.1.0) |
|---|---|---|
| Cold start to first paint, best of 5 (verified) | **441 ms** (441-536) | **454 ms** (454-519) |
| Idle RSS at +10 s, app + WebKit helpers (verified) | **190.5 MB** (app 108 + GPU 30 + Networking 15 + WebContent 37) | **188.9 MB** (app 107 + 29 + 15 + 38) |
| Binary size (verified) | 7.7 MB exe (8,021,968 B), 8.0 MB `.app` | 8.9 MB exe (9,362,226 B), bare binary (`.app` via `wails3 package`) |
| (a) Clipboard read+write from Go | `runtime.ClipboardSetText(ctx, s) error`, `runtime.ClipboardGetText(ctx) (string, error)` - verified round trip | `app.Clipboard.SetText(s) bool`, `app.Clipboard.Text() (string, bool)` - verified |
| (b) Close / shutdown hooks | `options.App.OnBeforeClose func(ctx) (prevent bool)`, `OnShutdown func(ctx)` - verified | `Options.ShouldQuit func() bool`, `Options.OnShutdown func()`, `ServiceShutdown() error`, window hook `RegisterHook(events.Common.WindowClosing, func(e){e.Cancel()})` - verified |
| (c) Open-file dialog with filter | `runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Filters: []runtime.FileFilter{{DisplayName, Pattern: "*.env"}}}) (string, error)` - compiled; dialog not clicked | `app.Dialog.OpenFile().AddFilter("Env", "*.env").PromptForSingleSelection() (string, error)` - compiled; not clicked |
| (d) TS bindings for `(Thing, error)` | `Promise<desktop.Thing>`; class `Thing` with `createFrom` in `wailsjs/go/models.ts` - verified | `$CancellablePromise<$models.Thing>`, `interface Thing` in `bindings/<import path>/models.ts` - verified |
| (d) Go error in JS (verified) | rejects with `Error`; `e.message` = exact `err.Error()` string | rejects with `RuntimeError` (`e.name`); `e.message` = exact `err.Error()` string |
| (e) `-tags desktopapp` (verified) | accepted; final tags `desktop,wv2runtime.download,production,desktopapp`; `wails.json` `"build:tags"` also works | accepted; final tags `production,desktopapp` (passed to bindings gen too) |
| (f) main in `cmd/secrio-desktop` (verified) | works: `wails.json` in `cmd/secrio-desktop`, run `wails build` there | works only after editing the generated Taskfiles (`go build ... ../cmd/secrio-desktop`, bindings pattern) |
| (g) Go 1.26 / CGO (verified) | builds with 1.26.1; CGO required on macOS (CGO_ENABLED=0 with Wails tags fails) | builds with 1.26.1; CGO by default; experimental `-tags purego` CGO-free backend (not tested) |
| (g) untagged repo, CGO_ENABLED=0 (verified) | `go build ./...` and `go vet ./...` OK, with Wails in go.mod | same, OK |
| (h) Browser without Wails (verified, headless Chromium + Vite dev) | importing bindings OK; calls throw `TypeError` (no `window.go`); setting `window.go.desktop.App = {...}` mocks them | importing OK; calls reject (no backend); no global to mock, so wrap the bindings |
| Close/quit interception (verified via System Events) | close button, Cmd+Q and SIGTERM all call `OnBeforeClose`; `true` blocks all three | close button: `WindowClosing` (cancellable) then `ShouldQuit`; Cmd+Q / SIGTERM: `ShouldQuit` only |

Measurement method: a Python wrapper records `time.time_ns()` right before `Popen` (no shell); JS calls a bound
`Painted()` inside a double `requestAnimationFrame` after the React mount, and Go prints `time.Now().UnixNano()`. Cold start =
the difference, so it includes the bound-call trip. 5 alternating runs per version, with the first run after each fresh build
thrown away (Gatekeeper first-launch cost: 900-1070 ms). Memory: `ps -o rss=` for the app pid plus the
`com.apple.WebKit.{GPU,Networking,WebContent}` pids that were not there before launch, 10 s after launch, then
SIGTERM. RSS counts shared WebKit/system pages, so it overstates the app's private memory. The ~1 % differences between
versions are noise.

## DECISION: Wails v2 (v2.17.1)

v3 does not clearly win. Startup and memory are equal, v3 is 1.2 MB bigger, and both have every API we need. v3 needs a
Taskfile plus a `build/` tree and edits to the generated Taskfiles to put main under `cmd/`; v2 needs one `wails.json`.
v2 is the default under the plan's rule.

## Exact usage for later tasks (v2)

- go.mod: `go get github.com/wailsapp/wails/v2@v2.17.1` (`wails build` refuses "Unable to find Wails in go.mod"). It
  runs `go mod tidy` and syncs the Wails version to the CLI unless you pass `-m` / `-nosyncgomod`; pin the CLI to v2.17.1.
- Layout:
  - `cmd/secrio-desktop/main.go` (`//go:build desktopapp`, the only Wails importer) holds `wails.json`.
  - `desktop/frontend/` is the Vite app.
  - `desktop/assets.go` (`//go:build desktopapp`, package e.g. `desktopassets`) holds `//go:embed all:frontend/dist` +
    `var Assets embed.FS`, and main imports it. go:embed cannot use `..`, so the embed must sit in the package next to
    `frontend/`, not in main. The tag guard keeps untagged `go build ./...` from failing when `dist` is missing.
  - `internal/desktop` (no Wails import) holds the bound struct; v2 bound it fine from that package.
- `cmd/secrio-desktop/wails.json` (verified):
  `{"name":"secrio-desktop","outputfilename":"secrio-desktop","frontend:dir":"../../desktop/frontend",
  "frontend:install":"pnpm install","frontend:build":"pnpm run build","frontend:dev:watcher":"pnpm run dev",
  "frontend:dev:serverUrl":"auto","build:dir":"../../desktop/build","build:tags":"desktopapp"}`
  (the spike used npm and `build:dir` `../../build/desktop`; pnpm and the dir name are untested choices). `wailsjsdir`
  defaults to `frontend:dir`. Wails writes `appicon.png` and `darwin/Info.plist` into `build:dir` on first build and the
  app into `<build:dir>/bin/`, so gitignore `desktop/build/bin/`.
- Build (from `cmd/secrio-desktop`): `wails build -tags desktopapp` (or rely on `build:tags`; giving both repeats the
  tag, which is harmless). Output: `desktop/build/bin/secrio-desktop.app`. Devtools stay off unless `-debug`/`-devtools`.
- Bindings: `wails build` generates them; standalone `wails generate module -tags desktopapp` (run in
  `cmd/secrio-desktop`). This command does NOT read `build:tags`; without `-tags` it fails with "build constraints exclude
  all Go files". Output: `desktop/frontend/wailsjs/go/desktop/App.{js,d.ts}`, `wailsjs/go/models.ts` (`namespace desktop`),
  `wailsjs/runtime/*`. JS calls `window['go']['desktop']['App']['Method'](...)`.
- Errors: a Go `error` rejects the promise with an `Error` whose `.message` is exactly `err.Error()`. So the
  `apiError.Error()` JSON comes back via `JSON.parse(e.message)`.
- Clipboard: `runtime.ClipboardSetText(ctx, v)` / `runtime.ClipboardGetText(ctx)`; `ctx` comes from `OnStartup`. Inject it
  into the facade as a small interface adapter in main.
- Dialog: `runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "Import .env", Filters:
  []runtime.FileFilter{{DisplayName: "Env files (*.env)", Pattern: "*.env"}}})`. It returns `"", nil` on cancel (docs;
  the cancel path was not exercised).
- Hooks: lock and clear the clipboard in `OnShutdown(ctx)`. It runs after the close button, Cmd+Q and SIGTERM (verified),
  not on SIGKILL. Do NOT return `true` from `OnBeforeClose`: it blocks the close button, Cmd+Q and even SIGTERM. Use
  `OnStartup` for ctx and `OnDomReady` if needed.
- Browser mock: the generated JS has no import-time side effects (verified). Route all calls through one `src/api.ts`
  that uses the generated bindings when `window.go` exists and a mock otherwise. Alternative: set
  `window.go = {desktop: {App: mock}}` before the first call. Do not call `wailsjs/runtime` in the browser, because it
  throws there.
- CGO: `wails build` needs CGO plus Xcode CLT on macOS. Keep core checks with `CGO_ENABLED=0 go build/vet ./...`
  untagged (verified OK).

## Open risks

- Fresh clone: v2 bindings generation compiles the `desktopapp` embed before the frontend build, and fails with
  "pattern all:frontend/dist: no matching files found" if `desktop/frontend/dist` is missing (verified). Vite empties
  `dist`, so a committed placeholder gets deleted locally. Mitigation: `mkdir -p desktop/frontend/dist && touch
  desktop/frontend/dist/.keep` before `wails build` (script/Make target), with `dist` gitignored.
- Plain `go build -tags desktopapp ./cmd/secrio-desktop` compiles without the `desktop,production` tags but is not a
  working app. Always build through `wails build`.
- Native dialog and cancel path not clicked through (needs a human); APIs compiled and the signatures were checked.
- RSS is dominated by shared WebKit pages (~190 MB for an empty window). If "low memory" needs a private-memory
  number, use `footprint`/`vmmap` in Task 14.
- v2 is in maintenance while v3 is the active line. Revisit if a needed fix lands only in v3.
