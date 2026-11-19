# Wails spike: v2 vs v3 (Task 1)

Date: 2026-11-19. All spike code lived outside the repo (scratchpad); only this note is committed.

How each fact was checked:
- **verified**: observed by running code on this machine.
- **compiled**: the code builds but was not exercised.
- **docs**: read from docs or source only.
- **partial**: some parts were run, others were not.

## Environment

- macOS 26.6.2 (25G83), Apple M1 Max, 32 GB. Go 1.26.1 darwin/arm64, Node 26.5.0, npm 11.17.0, pnpm 10.14.0.
- Xcode 27.1 + CLT 2416 (`xcode-select -p` = `/Applications/Xcode.app/Contents/Developer`).
- `wails doctor` (CLI v2.17.1, latest v2): "Your system is ready for Wails development!" (upx and nsis are optional and absent).
- `wails3 doctor` (CLI v3.1.0, latest stable v3): "No issues found", CGO_ENABLED=1, macOS signing not configured.
- Install: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`, `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`.
  `wails3 build` runs Task, which calls `wails3` by name, so `$(go env GOPATH)/bin` must be on PATH.

## Results

Same minimal React+TS frontend in both (title + one button, Vite release build), release build, `-tags desktopapp`.
Memory and size units: MiB (2^20 bytes); RSS comes from `ps -o rss=` (KiB) divided by 1024.

| Criterion | Wails v2 (v2.17.1) | Wails v3 (v3.1.0) |
|---|---|---|
| Cold start to first paint, min of 5 (verified) | **441 ms** (range 441-536) | **454 ms** (range 454-519) |
| Idle RSS at +10 s, app + WebKit helpers, min of 5 (verified) | **190.5 MiB** (app 108.0 + GPU 30.2 + Networking 15.3 + WebContent 37.0) | **188.9 MiB** (app 106.7 + 29.3 + 15.4 + 37.5) |
| Binary size (verified) | 8,021,968 B = 7.65 MiB exe; `.app` 7.9 MiB | 9,362,226 B = 8.93 MiB exe (bare binary; `.app` via `wails3 package`) |
| (a) Clipboard read+write from Go (verified) | `runtime.ClipboardSetText(ctx, s) error`, `runtime.ClipboardGetText(ctx) (string, error)`; round trip at startup and inside `OnShutdown` | `app.Clipboard.SetText(s) bool`, `app.Clipboard.Text() (string, bool)`; round trip at startup only |
| (b) Close / shutdown hooks (verified) | `options.App.OnBeforeClose func(ctx) (prevent bool)`, `OnShutdown func(ctx)` | `Options.ShouldQuit func() bool`, `Options.OnShutdown func()`, `ServiceShutdown() error`, window hook `RegisterHook(events.Common.WindowClosing, func(e){e.Cancel()})` |
| (c) Open-file dialog with filter (docs + compiled) | `runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Filters: []runtime.FileFilter{{DisplayName, Pattern: "*.env"}}}) (string, error)` | `app.Dialog.OpenFile().AddFilter("Env", "*.env").PromptForSingleSelection() (string, error)` |
| (d) TS bindings for `(Thing, error)` (verified) | `Promise<desktop.Thing>`; class `Thing` with `createFrom` in `wailsjs/go/models.ts` | `$CancellablePromise<$models.Thing>`; `interface Thing` in `bindings/<import path>/models.ts` |
| (d) Go error in JS (verified) | rejects with `Error`; `e.message` = exact `err.Error()` string | rejects with `RuntimeError` (`e.name`); `e.message` = exact `err.Error()` string |
| (e) `-tags desktopapp` (verified) | accepted; final `go build -tags desktop,wv2runtime.download,production,desktopapp`; `"build:tags":"desktopapp"` alone (no `-tags`) gives `-tags desktop,desktopapp,wv2runtime.download,production` | accepted; final `go build -tags production,desktopapp` (passed to bindings gen too) |
| (f) main in `cmd/secrio-desktop` (verified) | works: `wails.json` in `cmd/secrio-desktop`, run `wails build` there | works after editing 2 generated Taskfile lines (below); build output `go build -tags production,desktopapp ... -o "bin/v3app" ../cmd/secrio-desktop` |
| (g) Go 1.26 / CGO (verified) | builds with 1.26.1; CGO required on macOS (CGO_ENABLED=0 with Wails tags fails) | builds with 1.26.1; CGO by default; experimental `-tags purego` CGO-free backend (docs, not tested) |
| (g) untagged repo, CGO_ENABLED=0 (verified) | `go build ./...` and `go vet ./...` OK, with Wails in go.mod | same, OK |
| (h) Browser without Wails (verified, headless Chromium + Vite dev) | importing bindings OK; calls throw `TypeError` (no `window.go`); setting `window.go.desktop.App = {...}` mocks them | importing OK; calls reject (no backend); no documented global to mock (the runtime creates `window._wails`), so wrap the bindings |
| Close/quit interception (verified via System Events) | close button, Cmd+Q and SIGTERM all call `OnBeforeClose`; returning `true` blocks all three | close button: `WindowClosing` (cancellable), then `ShouldQuit`; Cmd+Q / SIGTERM: `ShouldQuit` only |

Close/quit runs: the hook matrix ran 1 time per path per version for each of prevent on/off, plus one manual v2 run.
In the v2 prevent=1 Cmd+Q run the captured log was empty because of a script timing race; the earlier manual run
showed `BEFORECLOSE` and the app kept running. The v2 clipboard-in-`OnShutdown` test ran 3 times per path (9 runs).

Edited v3 Taskfile lines for (f), with the Wails project in `desktop/`:
- `build/darwin/Taskfile.yml`: `... go build {{.BUILD_FLAGS}} -o "{{.OUTPUT}}" ../cmd/secrio-desktop`
- `build/Taskfile.yml`: `wails3 generate bindings -f '{{.BUILD_FLAGS}}' -clean=true ... -ts -i ../cmd/secrio-desktop`

Measurement method:
- **Cold start.** A Python wrapper records `time.time_ns()` right before `Popen` (no shell). JS calls a bound
  `Painted()` inside a double `requestAnimationFrame` after the React mount, and Go prints `time.Now().UnixNano()`. Cold
  start is the difference, so it includes the bound-call trip.
- **Runs.** 5 alternating runs per version. The first launch after each fresh build was excluded (Gatekeeper cost,
  900-1070 ms).
- **Memory.** 10 s after launch, `ps -o rss=` for the app pid plus the `com.apple.WebKit.{GPU,Networking,WebContent}`
  pids that were not there before launch; then SIGTERM. RSS counts shared WebKit/system pages, so it overstates the
  app's private memory. The ~1 % differences between versions are noise.

## DECISION: Wails v2 (v2.17.1)

v3 does not clearly win:
- Startup and memory are equal.
- v3's exe is 1.28 MiB (1,340,258 B) bigger.
- Both have every API we need.
- v3 needs a Taskfile plus a `build/` tree, and edits to the generated Taskfiles to put main under `cmd/`; v2 needs one
  `wails.json`.

So v2, the default under the plan's rule.

## Exact usage for later tasks (v2)

- **How Wails finds `main`.** It builds the package in the directory where `wails build` runs, which must hold
  `wails.json`; there is no separate main-dir setting.
- **go.mod.** Run `go get github.com/wailsapp/wails/v2@v2.17.1` first, or `wails build` refuses with "Unable to find
  Wails in go.mod". `wails build` runs `go mod tidy` and syncs the Wails version to the CLI unless you pass `-m` /
  `-nosyncgomod`. Pin the CLI to v2.17.1.
- **Layout.**
  - `cmd/secrio-desktop/main.go` (`//go:build desktopapp`): the only Wails importer, next to `wails.json`.
  - `cmd/secrio-desktop/adapters.go` (`//go:build desktopapp`): clipboard and dialog adapters over the Wails `ctx`.
  - `desktop/frontend/`: the Vite app.
  - `desktop/assets.go` (`//go:build desktopapp`, package e.g. `desktopassets`): `//go:embed all:frontend/dist` +
    `var Assets embed.FS`; main imports it. go:embed cannot use `..`, so the embed sits next to `frontend/`, not in
    main. The tag guard keeps untagged `go build ./...` from failing when `dist` is missing.
  - `internal/desktop` (no Wails import): the bound struct; v2 bound it fine from that package.
- **`cmd/secrio-desktop/wails.json`** (verified with npm):
  `{"name":"secrio-desktop","outputfilename":"secrio-desktop","frontend:dir":"../../desktop/frontend",
  "frontend:install":"pnpm install","frontend:build":"pnpm run build","frontend:dev:watcher":"pnpm run dev",
  "frontend:dev:serverUrl":"auto","build:dir":"../../desktop/build","build:tags":"desktopapp"}`
  - pnpm and the `build:dir` value are untested choices; the spike used npm and `../../build/desktop`.
  - `wailsjsdir` defaults to `frontend:dir`.
  - On first build Wails writes `appicon.png` and `darwin/Info.plist` into `build:dir`, and the app into
    `<build:dir>/bin/`. Gitignore `desktop/build/bin/`.
- **Build** (from `cmd/secrio-desktop`). Run `wails build`: `build:tags` adds `desktopapp`; `-tags desktopapp` also works,
  and giving both repeats the tag harmlessly. Output: `desktop/build/bin/secrio-desktop.app`. Devtools stay off unless
  `-debug` / `-devtools`.
- **Bindings.**
  - `wails build` generates them. Standalone: run `wails generate module -tags desktopapp` in `cmd/secrio-desktop`. This
    command ignores `build:tags`; without `-tags` it fails with "build constraints exclude all Go files".
  - Output: `desktop/frontend/wailsjs/go/desktop/App.{js,d.ts}`, `wailsjs/go/models.ts` (`namespace desktop`), and
    `wailsjs/runtime/*`. JS calls `window['go']['desktop']['App']['Method'](...)`.
- **Errors.** A Go `error` rejects the promise with an `Error` whose `.message` is exactly `err.Error()`. The
  `apiError.Error()` JSON comes back via `JSON.parse(e.message)`.
- **Clipboard.** Use `runtime.ClipboardSetText(ctx, v)` / `runtime.ClipboardGetText(ctx)` with the `ctx` from `OnStartup`,
  wrapped in `adapters.go` and injected into the facade as a small interface.
- **Dialog.** `runtime.OpenFileDialog(ctx, runtime.OpenDialogOptions{Title: "Import .env", Filters:
  []runtime.FileFilter{{DisplayName: "Env files (*.env)", Pattern: "*.env"}}})`. It returns `"", nil` on cancel
  (docs; not exercised).
- **Hooks.** Lock the vault and clear the clipboard conditionally in `OnShutdown(ctx)`.
  - Verified 9/9 runs (3 each: close button, Cmd+Q, SIGTERM). Inside `OnShutdown`, `ctx.Err()` was nil,
    `ClipboardGetText` returned the pre-set sentinel, `ClipboardSetText` succeeded, and `pbpaste` after exit showed the
    new value.
  - It does not run on SIGKILL.
  - Never return `true` from `OnBeforeClose`: it blocks the close button, Cmd+Q and even SIGTERM. Use `OnStartup` for
    ctx and `OnDomReady` if needed.
- **Browser mock.** The generated JS has no import-time side effects (verified). Route all calls through one
  `src/api.ts`, which uses the bindings when `window.go` exists and a mock otherwise. Alternative: set
  `window.go = {desktop: {App: mock}}` before the first call. Do not call `wailsjs/runtime` in the browser; it throws.
- **CGO.** `wails build` needs CGO plus Xcode CLT on macOS. Keep core checks with untagged `CGO_ENABLED=0 go build/vet
  ./...` (verified OK).

## Open risks

- **Fresh clone.** v2 bindings generation compiles the `desktopapp` embed before the frontend build. It fails with
  "pattern all:frontend/dist: no matching files found" if `desktop/frontend/dist` is missing (verified). Vite empties
  `dist`, so a committed placeholder gets deleted. Mitigation: `mkdir -p desktop/frontend/dist && touch
  desktop/frontend/dist/.keep` before `wails build` (script/Make target), with `dist` gitignored.
- **Plain `go build`.** `go build -tags desktopapp ./cmd/secrio-desktop` compiles without the `desktop,production` tags
  but is not a working app. Always build through `wails build`.
- **go.mod changes on build.** `wails build` runs `go mod tidy` and rewrites the Wails version in go.mod to match the
  CLI. In CI, pin the CLI version and consider `-m -nosyncgomod` for reproducible builds.
- **Clipboard on SIGKILL or crash.** `OnShutdown` does not run, so our copied value stays on the clipboard. The 30 s
  auto-clear timer is the only protection.
- **Dialog untested.** The native dialog and its cancel path were not clicked through (needs a human); the APIs
  compiled and the signatures were checked.
- **Untested here:** a strict CSP against the scripts Wails v2's asset server injects (`/wails/ipc.js`,
  `/wails/runtime.js`); `wails dev` (hot reload); Windows and Linux.
- **No signing or notarisation.** The first-launch Gatekeeper cost (900-1070 ms) is excluded from the timings above.
- **Memory metric.** RSS is mostly shared WebKit pages (~190 MiB for an empty window). If "low memory" needs a
  private-memory number, use `footprint`/`vmmap` in Task 14.
- **v2 is in maintenance** and v3 is the active line. Revisit if a needed fix lands only in v3.
