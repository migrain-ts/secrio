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
| Projects, populated | pass | 0 / **1** at 1440 (`composition/lone-component`), 0 / 0 at 960 | **not done**, see below |
| Projects, search no match | pass | 0 / 0 | done |
| Projects, empty vault | pass | 0 / 0 (transient toast left out of the snapshot) | done |
| New project dialog | pass | 0 / 0 | done |
| New environment dialog | pass | 0 / 0 | done |
| Project view: env sidebar, table, detail with a selected and revealed secret | pass (workspace) | 0 / 0 at 1440 | done at 1440; 960 measured before the last two fixes only (screenshot checked) |
| Project view: nothing selected | pass | measured before the final fixes only | **unverified** (same layout as the revealed state minus the detail card) |
| Empty environment | pass | not measured | **unverified** (screenshot only) |
| Project with no environments | pass | not measured | **unverified** (screenshot only) |
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
   room for the actions button) are 16/16 with the name width-limited; env
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

## Remaining (not done)

- **Projects, populated, 1440: `composition/lone-component` (serious)**,
  "content covers 22% of the screen and has nothing around it". It does not
  fire at 960 nor on the no-match/empty states; it depends on the seed (3
  projects on a large window). The grid now uses `auto-fit` so the cards fill
  the row, but clearing the rule would need extra content or navigation on
  the picker (a redesign, out of this task's scope). Decision for the
  controller/user.
- **Not re-measured after the final fixes**: project view at 960, project view
  with nothing selected, empty environment, project with no environments
  (screenshots reviewed; `inspect-spacing` not run on the final DOM because of
  the review session's context budget). Treat them as unverified.

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
