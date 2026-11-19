# Secrio Desktop — UI rules we apply (Better Design)

Date: 2026-11-19. Source: Better Design MCP, `get-ui-principle` / `get-ux-principle`, loaded
before any styling or behaviour work (Task 9). Design system: catalog `linear` (dark, Inter,
indigo accent), installed with `get-design-system-kit` (target `react`, Base UI flavor). Icons:
the kit assigns **Iconoir (default)**; use `find-icons` with `libraryId: "iconoir"`, never another
pack, emoji or a hand-drawn SVG. Spinner: the kit's Motion `arc` spinner for every loading spinner.

Every screen task (10-13) follows these rules, then runs the review gate: `get-review-rules`,
`check-comprehension` per panel, `inspect-spacing` on rendered DOM; fix every critical/serious.

## Shell, sidebar, layout (ui: sidebar inset, product-page widths, shell precision pass)

- Sidebar has its own inset on both sides; selected-row fill keeps that inset (does not run to
  the edge). Every icon sits in a fixed-width slot so labels start on one x.
- Env secret counts go in a fixed-width trailing slot, right-aligned, `tabular-nums`.
- Separate groups with gaps, not extra headers or rules. Section captions in sentence case.
- Nav rows ~36px, one medium label weight for resting and active; selection is carried by
  surface colour, not a weight jump.
- Actions above the data (search, "New secret", "Import .env"); counts/paging below.
- Data views max ~75rem wide; a standalone form (unlock, create vault) max ~27.5rem.
- Title, then one supporting line, then content (~0.375rem / ~1.5rem gaps).

## Dense table (ui: differentiate dense content, product-page widths, tabular numbers)

- Differentiate before adding whitespace: a type badge per row is the visual anchor; names
  in a monospace face (identifiers), dates muted.
- Truncate long free text (~32ch) with a tooltip; missing value shown as an en dash.
- Numbers in columns and countdowns (clipboard clear) use `tabular-nums`.
- Hierarchy by weight and colour, not size: name = foreground, type/date = muted-foreground.
- Row hover = parent + 1 surface step; selected row = parent + 2 (hover must differ from selected).

## Dark theme and surfaces (ui: dark mode lift, surface steps)

- Dark only. Elevation = lighter fill + one 0.5px outline + short black drop; no white glow,
  no top highlight. Raised card = parent + 1 step, control = parent + 2.
- Theme tokens only (`bg-background`, `bg-card`, `text-muted-foreground`, `border-border`...),
  no hard-coded colours in components.
- Modal backdrop dims the page (dark: black ~90% with small blur). An overlay's edge is its
  shadow ring; do not add a border too.

## Actions and destructive flows (ui: semantics are secondary; ux: confirmation dialogs,
## ask in proportion to the damage, destructive copy)

- One primary action per screen. Destructive buttons use the secondary/destructive-quiet style;
  Cancel is the prominent, auto-focused option in a confirmation.
- Deleting a secret, env or project is irreversible here (no undo in the first cut), so it
  gets a confirmation dialog. Copy names the target and the consequence:
  "Delete environment Staging and its 12 secrets? This can't be undone." Never "Are you sure?".
- Deleting a project (cascades to all envs and secrets) uses type-to-confirm (type the
  project name); the only place a pre-disabled submit is allowed.
- Do not confirm ordinary, reversible changes (save, edit).

## Forms and dialogs (ux: forms, modal vs non-modal, focus management)

- Creation forms open behind an explicit CTA in a dialog; never a dormant inline form on a list.
- Every input has a visible `<label>`; inputs inside a `<form>` so Enter submits.
- Submit stays enabled until submission starts; then disabled + spinner + label kept.
- Errors next to their field (`aria-invalid`, `aria-describedby`), focus the first error.
- Trim names before validation (projects/envs: the Core rejects blank / control chars; secret
  names `^[A-Za-z_][A-Za-z0-9_]*$` validated client-side too, server is the authority).
- Secret value field: no trimming (a value is exact), paste allowed, `spellcheck=false`,
  `autocomplete="off"`. Password fields: `type=password`, `autocomplete="current-password"`
  (unlock) / `"new-password"` (create vault).
- Dialogs: focus moves in on open, is trapped, returns to the trigger on close; Escape closes.
- On a `locked` error mid-dialog: keep non-secret input, go to unlock, tell the user why;
  never silently drop what they typed (Review Focus). Passwords and values are cleared from
  state after submit.

## States (ux: four mandatory states, partial/degraded, page-level errors, honest loading)

- Every data region handles empty, loading, error, success explicitly; lists handle 0/1/N.
- Empty states name what is missing and offer the first action ("No secrets in Staging yet"
  + "New secret" / "Import .env").
- Loading: keep current content during refresh; skeleton only when structure is known and data
  genuinely absent; spinner inside the clicked button for mutations. No spinner theatre for
  local search/filter.
- Errors: plain cause + corrective action, scoped to the failing region. Never show raw error
  text: the UI maps `ApiError.code` to fixed copy (unknown → `internal` → generic message).
- Success feedback is contextual ("Copied. Clipboard clears in 30 s").

## Secrets and security UX (ux: sensitive content on screen, protect secrets)

- Values masked by default (fixed-length dots, never the real length). Reveal is a deliberate
  toggle with an accessible name ("Show value of STRIPE_KEY") and `aria-pressed`.
- A revealed value is kept in component state only briefly, then re-masked and dropped.
- Values never go into URLs, storage, logs, error messages, toasts or table DOM.
- Copy happens in Go; the UI shows the clear countdown, or, when `clearInSeconds` is 0, says
  "Clipboard won't be cleared automatically".
- Lock is always one click away in the top bar; after lock/expiry, return to unlock screen.

## Keyboard (ux: keyboard accessibility, keyboard path for frequent actions, navigation)

- Everything works with Tab/Shift+Tab; visible `:focus-visible` ring (kit `--ring`).
- Within lists/table: arrow keys move rows, Enter opens detail, Escape returns to the list
  with the same row focused. Tab moves between widgets, arrows within.
- `<nav aria-label>` for the sidebar; icon-only buttons get `aria-label` + tooltip.
- Single-key shortcuts only while the list has focus; never steal system/browser shortcuts.

## Trade-offs we own (ux: tradeoffs)

- We optimise for a fast, dense, keyboard-friendly pro tool at the cost of onboarding hand-holding.
- Dark only: less work and one tested palette, at the cost of users who prefer light mode.
- Confirmation dialogs instead of undo/trash: simpler facade (no soft delete), at the cost of a
  click per delete; type-to-confirm only for project deletion.
- Self-hosted Inter (bundle grows by the font files) to keep the strict CSP and offline use.
