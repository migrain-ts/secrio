# Secrio Desktop: review gates (Task 14, step 1)

Date: 2026-11-20. Build reviewed: `desktop/frontend` dev server with the mock API
(`VITE_MOCK_API=1 pnpm dev`), rendered in headless Chromium 1248 (the cached
Playwright browser, driven with the already-installed playwright-core 1.64.1;
nothing was downloaded). Widths: 1440×900 and 960×640. The window minimum is
960 px (`MinWidth: 960` in `cmd/secrio-desktop/main.go`), so the narrow check
uses 960 instead of the 900 suggested in the dispatch.

Method per panel: `get-review-rules` (once, for the whole review),
`check-comprehension` with the panel's real copy, then `inspect-spacing`
(`newScreen: true`) on DOM snapshots taken with the tool's own capture script.
Critical and serious findings were fixed, the screen re-rendered and
re-measured. Moderate findings were not chased (see "Accepted moderates").

Screenshots: `desktop-review-screens/<screen>-<width>.png` (final state, after
all fixes). Data comes from the mock seed, so every name and value is fake.

## Results per screen

| Screen | Comprehension | Last inspect-spacing (critical/serious) | Status |
|---|---|---|---|
| Unlock (default and wrong password) | pass | 0 / 0 (1440, 960) | done |
| Create vault | pass | 0 / 0 (1440, 960) | done |
| Projects, populated | pass | 0 / 0 at 1440 (after the single-column list, final round) and 960 | done |
| Projects, search no match | pass | 0 / 0 | done |
| Projects, empty vault | pass | 0 / 0 (transient toast left out of the snapshot) | done |
| New project dialog | pass | 0 / 0 | done |
| New environment dialog | pass | 0 / 0 | done |
| Project view: env sidebar, table, detail with a selected and revealed secret | pass (workspace) | 0 / 0 at 1440 (full page); 0 / 0 at 960 (scoped: table section + detail panel) | done |
| Project view: nothing selected | pass | 0 / 0 at 1440 (scoped: detail panel, the only region that differs) | done |
| Empty environment | pass | 0 / 0 at 1440 (scoped: content section) | done |
| Project with no environments | pass | 0 / 0 at 1440 (scoped: content section) | done |
| New / Edit secret dialog (with errors) | pass | 0 / 0 | done |
| Delete confirmations (secret, env type-to-confirm, project) | pass | 0 / 0 | done |
| Import: idle | pass | 0 / 0 | done |
| Import: preview with duplicates and Replace warning | pass (record names not counted, see below) | 0 / 0 | done |
| Import: nothing new to import | pass | 0 / 0 | done |
| Import: error | pass | 0 / 0 | done |

Dialogs were measured at 1440 only: they are fixed-width and centred, so the
960 rendering is the same geometry (screenshots at 960 are in the folder).

## Findings and fixes

1. **Secrets table broke below 1280 px** (seen in the 900/960 screenshots):
   the fixed columns left the Name column 0 px wide, the headers overlapped and
   the row actions were clipped. Fix: Value (always the same dots) and Updated
   (shown in the detail panel) hide below `xl`; the detail panel is `w-72`
   below `xl`; the table has `min-w-[24rem]` so Name can never collapse again
   (the area scrolls instead).
2. **Comprehension, import dialog (serious, 15-second test)**: the long
   description repeated on every step; "Done" did not name its outcome; the
   counts were said twice. Fix: description "Nothing changes until you
   confirm." on the idle step only; zero-import button "Close without
   importing"; the counts line is screen-reader only (still the live status)
   and each list heading shows its own count; radios "Skip" / "Replace" under
   the "Existing secrets" legend; warning "Replaces N values permanently."
   Judgment call: the file's secret names in the two lists are the user's
   records shown for review, not interface copy, so they were left out of the
   word count (as table rows were for the project view).
3. **Comprehension, Unlock**: the first call passed every error and notice
   state at once (they never show together) and failed; the worst real
   combination (involuntary-lock notice + wrong password) passes.
4. **`surface/drop-shadow` (serious)** on the card and primary button: the
   kit's linear look puts drop shadows on buttons, cards and the empty-state
   icon box. Fix in the kit (`button.tsx`, `card.tsx`, `empty.tsx`): drop
   shadows removed, the 1 px edge ring and inset highlight kept. Overlays
   (dialogs, menus, toasts) keep their shadows. Trade-off: a little of the
   kit's depth is lost on every screen; gained: flat surfaces as the rule asks.
5. **`spacing/touch-target` (serious)**: the dialog corner X was 16×16. Fix:
   32×32 target in the kit `dialog.tsx` (icon unchanged). The import strategy
   radios were 16×16: each row (circle + label) is now the radio element, 32 px
   tall, named by its text.
6. **`spacing/asymmetric-horizontal-inset` (serious)**: search fields (32/12
   because of the leading icon) are now 32/32; project cards (16/48 to leave
   room for the actions button) are 16/16 with the name width-limited (and,
   final round, listed one per row); env
   sidebar items (10/64) are 12/12 with the name width-limited.
7. **`spacing/density-role` (serious)**: page inset 24 → 16 px (compact
   density) on Projects and the project view; env sidebar width 224 → 184 px
   (`var(--layout-sidebar)`).
8. **`spacing/sibling-gap-drift` (serious)** in the detail card (8 vs 16 px
   between children): the secret name and description are one block, so the
   card keeps a single 16 px gap.
9. **Contrast (Part A, checked here)**: red text and error borders use the new
   `--destructive-text` (oklch 0.75 0.15 22) instead of the dark
   `--destructive` (about 2:1 on the background). Filled destructive buttons
   keep the dark red with white text.

## Final round

- **Projects, populated, 1440: `composition/lone-component` (serious)**,
  "content covers 22% of the screen and has nothing around it" (3 cards in a
  4-column grid on a large window). Minimal fix, attempt 1 of 2: the projects
  are a single-column list of full-width rows (`grid-cols-1`, same card
  component). Re-measured at 1440: 0 critical / 0 serious. No second attempt
  needed.
- **Re-measured after the final fixes** (real rendered DOM, `newScreen: true`):
  project view at 960, nothing selected, empty environment, no environments:
  0 critical / 0 serious. These runs used the capture script's `scope` (the
  region that differs from the already-measured full page: table section,
  detail panel, content section); the top bar and sidebar were measured on
  the full project view at 1440.

## Remaining (not done)

None at critical/serious level. Moderates below are accepted.

## Accepted moderates (not fixed)

- `spacing/touch-target` for 28–36 px controls: desktop, pointer-driven app
  with the kit's compact density; 44 px targets would change every control.
- `spacing/off-scale-value` 2/6/10/14/20/28 px: kit internals (field gap 6,
  sidebar item gap 10, table cell padding 6, alert icon inset 28).
- `spacing/density-role` card padding 25/48 px on the unlock card and the
  empty state (kit components).
- `spacing/baseline-drift` 2 px between the "Projects" crumb (12 px) and the
  project title (18 px).

## Not checked

- 390 / 1728 widths requested by the tool's defaults (desktop window, min
  960). Behaviour, motion and the real WKWebView rendering (Chromium only).
- The `find-design-system` suggestion the tool appends: the product already
  has its design system (Better Design "linear" kit), so it was not re-opened.

## Measurements

Date: 2026-11-20. Same machine as the spike (Apple M1 Max, 32 GB, macOS 26.6.2),
release build from `scripts/desktop-build.sh` (Wails v2.17.1, production mode,
devtools off). Data dir: a scratch `SECRIO_HOME` with one vault, so the app
opens on the unlock screen. Units: MiB = 2^20 bytes.

Method (same as the spike): a Python wrapper records `time.time_ns()` right
before `Popen` (no shell). For this measurement only, a temporary, uncommitted
patch to `src/main.tsx` called `window.runtime.LogPrint(mark)` inside a double
`requestAnimationFrame`: `PAINT_SHELL` right after the React mount (the spike's
mark) and `PAINT_SCREEN` once the unlock form (first `input`) was in the DOM.
Wails prints `LogPrint` on stderr; the wrapper timestamps the line on receipt
(the spike used a bound `Painted()` call and a Go timestamp; both include the
IPC trip). At +10 s: `ps -o rss=` and `footprint` (phys_footprint: private
dirty + compressed memory, what Activity Monitor shows) for the app pid and the
`com.apple.WebKit.*` pids that appeared after launch; then SIGTERM (exit 0 in
every run). One discarded first launch after the build (Gatekeeper: 895 ms),
then n = 10 runs. The patch was reverted and the app rebuilt before the checks
below.

| Metric (n = 10) | Min | Median | Max | Spike (v2, min of 5) |
|---|---|---|---|---|
| Launch to first paint (`PAINT_SHELL`) | 467 ms | 516 ms | 535 ms | 441 ms (441-536) |
| Launch to unlock screen painted (`PAINT_SCREEN`) | 507 ms | 564 ms | 583 ms | n/a (empty page) |
| Private memory, app + 3 WebKit helpers (footprint) | 84.3 MiB | 95.7 MiB | 97.7 MiB | not measured |
| Private memory, app process only | 33.0 MiB | 33.9 MiB | 34.3 MiB | not measured |
| RSS, app + 3 WebKit helpers | 205.0 MiB | 216.9 MiB | 218.3 MiB | 190.5 MiB |
| RSS, app process only | 106.9 MiB | 113.3 MiB | 113.9 MiB | 108.0 MiB |

Median per process (RSS / footprint): app 113.3 / 33.9; WebKit.WebContent
58.1 / 35.9; WebKit.GPU 30.0 / 20.4; WebKit.Networking 15.1 / 5.7. The gap
between RSS (217) and footprint (96) is shared pages: WebKit and system
frameworks mapped in every WebKit app. WebContent grew most against the spike
(37 -> 58 MiB RSS): it now holds the real UI (React, Base UI, 700 kB JS, fonts).

Size:

| | Bytes | MiB | Spike |
|---|---|---|---|
| Executable | 13,488,736 | 12.86 | 7.65 |
| `.app` (all files) | 13,751,660 | 13.11 | 7.9 |
| `frontend/dist` (embedded) | JS 699,831; CSS 60,050; 7 Inter woff2 (972 KiB on disk) | | |

The +5.2 MiB against the spike is the Core (modernc SQLite in pure Go, crypto,
keyring; the stripped CGO-free CLI alone is 9.08 MiB) plus about 1 MiB of
frontend.

Against the criteria (fast launch, low consumption): launch stays about half a
second; the usable unlock screen paints in 0.51-0.58 s, about 75 ms (median)
later than the spike's empty page. Memory: about 96 MiB private for app plus
WebKit, 34 MiB for the Go process. Worse than the spike, as expected with a
real UI and the Core, but in the same order.

Shipped binary checks (after the clean rebuild):

- Devtools off: `go version -m` shows tags `desktop,desktopapp,wv2runtime.download,production`
  (no `devtools`, no `debug`) and ldflags `-w -s`; Wails sets
  `developerExtrasEnabled` only when built with `devtools` (`IsDevtoolsEnabled`,
  `internal/app/app_devtools*.go` in Wails). The build log says `Devtools false`.
- Remote URLs in `dist` (JS, CSS, `index.html`), only never-fetched strings:
  `http://www.w3.org/{2000/svg,1999/xlink,1998/Math/MathML,XML/1998/namespace}`
  (XML namespaces in React DOM), `https://react.dev/errors/` and
  `https://base-ui.com/production-error` (minified error text),
  `https://tailwindcss.com` (CSS license banner). No protocol-relative URL;
  `index.html` has none.
- CSP meta present in `dist/index.html` (policy in architecture section 14).
- Source maps: none (no `.map` file, no `sourceMappingURL` in dist or binary).
- Mock: `__SECRIO_MOCK_BUNDLE__` (`MOCK_MARKER`), `createMockApi` and the seed
  strings (`hotenb`, `correct horse`, `fake-jwt-signing-secret-0000`) are absent
  from the JS and from the binary; `build:release` ran its 7 dist checks during
  the build. The binary embeds this exact dist (`index-7aBUv6h5.js` found in it).
- Signature: ad-hoc (`Signature=adhoc`), identifier `app.secrio.desktop`.

Not measured: a 500-row secrets table (no seeded large vault), memory after
use (only idle at +10 s), other machines or macOS versions.
