# UI Bundle List Paging & Filtering — Design Spec

**Date:** 2026-07-16
**Status:** Approved
**Related issues:** [terramate-io/terramate#2371](https://github.com/terramate-io/terramate/issues/2371), [terramate-io/terramate#2372](https://github.com/terramate-io/terramate/issues/2372)

## Problem

The `terramate ui` (scaffolding) TUI has three bundle-list select screens — Scaffold/Create (`ViewCreateSelect`), Reconfigure (`ViewReconfigSelect`), and Promote (`ViewPromoteSelect`). All three only support moving the cursor one row at a time (Up/Down). In projects with hundreds or thousands of bundle instances this makes both finding a specific bundle and navigating the list tedious:

- #2371: no way to move by a full page (PgUp/PgDn).
- #2372: no way to filter/search the list by name.

## Goal

1. Add `PgUp`/`PgDn` paging to all three list views.
2. Add a `/`-triggered, live, free-text filter to all three list views.
3. Keep each view's implementation independent (per the codebase's existing convention of near-duplicated per-view logic, e.g. `buildReconfigFilters` / `buildPromoteFilters`) rather than introducing a new cross-view abstraction. Only the already-shared low-level plumbing (`scrollWindowVar`, `renderScrollbar`, the `renderedItem` type) is extended.

## Scope

Applies to `commands/ui/view_create_select.go` (flat list), `commands/ui/view_reconfig.go`, and `commands/ui/view_promote.go` (grouped lists). No changes to `ViewOverview` or the input-form views.

## Key bindings

Added to the global `keyMap` in `commands/ui/model.go`:

```go
PgUp: key.NewBinding(key.WithKeys("pgup", "b"), key.WithHelp("pgup", "page up")),
PgDn: key.NewBinding(key.WithKeys("pgdown", "f"), key.WithHelp("pgdn", "page down")),
```

Filter mode uses a literal `/` key check (not part of `keyMap`, since it's contextual to list views and must not fire while a filter text input is being typed into — see below).

## Paging (#2371)

Each of the three views already builds (or, for the Create/flat view, will factor out into a small per-view helper) the `[]renderedItem` slice used for rendering, and computes the current visible window via `scrollWindowVar(cursor, items, availableHeight, sep)`.

The `PgDn`/`PgUp` handlers reuse that exact same item-building + `scrollWindowVar` call to find the current window `(start, end)`:

- **PgDn**: cursor moves to the first selectable item at index `>= end` (i.e. the first bundle that would appear on the *next* screen). If none remain, cursor moves to the last bundle in the list.
- **PgUp**: cursor moves to the last selectable item at index `< start` (the last bundle that would appear on the *previous* screen). If none remain, cursor moves to the first bundle.

This makes PgDn/PgUp behave like "next/previous screen" consistent with what's already on screen, rather than an approximate fixed-count jump. Group header/separator rows do not count as landing targets.

### `renderedItem` change

`renderedItem` (defined in `view_create_select.go`, shared by all three view files within the package) gains one field:

```go
type renderedItem struct {
    content    string
    height     int
    selectable bool // false for group headers/separators
}
```

- Flat Create-view items: always `selectable: true` (no headers in that view).
- Grouped Reconfigure/Promote items: group header rows and blank separator rows get `selectable: false`; bundle instance rows get `selectable: true`.

### Per-view item builder extraction

`renderGroupedBundleItems` (Reconfigure) and `renderPromoteGroupedItems` (Promote) already exist as standalone functions returning `(selectedItemIdx, items)` — no extraction needed there, the PgUp/PgDn handlers just call them directly.

For the Create/flat view, the item-building loop currently lives inline inside `renderFlatBundleList`. It will be extracted into a new function `buildFlatBundleItems(items []flatBundleEntry, cursor, contentWidth int) (selectedItemIdx int, items []renderedItem)` in `view_create_select.go`, called from both `renderFlatBundleList` (rendering) and `updateCreateSelect` (paging). This is a within-file refactor only — it does not introduce sharing across the three views.

## Filtering (#2372)

### Activation and editing

- `/` (checked before other key handling in each view's `updateXSelect`, while not already in filter-edit mode) enters filter-edit mode: a single-line text input renders above the list, styled like the existing detail box's top border.
- While in filter-edit mode, all printed characters are appended to the query; `Backspace` removes the last rune. The live query re-filters the list on every keystroke (see matching rules below), and the cursor resets to `0` in the filtered list whenever the query changes.
- `Esc` while editing clears the query, exits filter-edit mode, and un-filters the list.
- `Enter` while editing keeps the current query, exits edit mode (input stops capturing characters), and returns focus to normal list navigation — Up/Down/PgUp/PgDn/Enter-to-select all resume their normal behavior. The filter stays applied until cleared. Pressing `/` again re-enters edit mode with the existing query pre-filled (cursor at end) so it can be refined or cleared.
- When not in filter-edit mode and the query is non-empty, `Esc` first clears the filter (mirroring the existing "esc clears env filter, then a second esc goes back" pattern already used for the `e` env filter in Reconfigure/Promote); a second `Esc` behaves as before (back to Overview, or reset env filter first if one is active — filter clears take priority in this order: text filter → env filter → back).

### Matching rules

Case-insensitive substring match against:
- The bundle definition name (e.g. `vpc`), and
- The instance alias (e.g. `prod-vpc-1`), where applicable (Create/flat view has no instance alias yet since it's pre-creation — only the definition name is matched there).

A bundle matches if the query is a substring of either field.

### Interaction with the existing `e` env-cycle filter (Reconfigure/Promote only)

The text filter and the `e` env filter combine with AND: the list first narrows by the active env filter (if any), then narrows further by the active text query (if any). Both are reflected in the breadcrumb, e.g.:

```
Reconfigure Bundle Instance in staging — filter: "vpc"
```

### Grouped views

A group (in Reconfigure/Promote) is omitted entirely if none of its instances match the current text filter (and env filter). Otherwise only the matching instances render under that group's header.

### New `Model` state

One filter-state struct, duplicated per view (per the "duplicate per view" decision — no shared type):

```go
// in view_create_select.go
type flatFilterState struct {
    query  string
    editing bool
}

// in view_reconfig.go
type reconfigFilterState struct {
    query   string
    editing bool
}

// in view_promote.go
type promoteFilterState struct {
    query   string
    editing bool
}
```

Added to `Model` (`commands/ui/model.go`):

```go
flatBundleFilter flatFilterState
reconfigFilter   reconfigFilterState
promoteFilter    promoteFilterState
```

`buildReconfigBundles` / `buildAllPromoteBundles` / the Create view's list-building gain the text-match narrowing step alongside their existing env-filter narrowing. Cursor is reset to `0` whenever the effective filtered list is rebuilt (query change, env filter change, or both).

## Rendering / help text

- The filter input row renders only when `editing` is true or `query != ""`; otherwise the view looks exactly as it does today.
- Help text hint changes:
  - Idle, no query: append `"/: filter"` to the existing help line.
  - Editing: help line becomes `"esc: clear • enter: apply"`.
  - Applied (not editing, query non-empty): append `"/: edit filter"` and the breadcrumb shows the active query as above.

## Testing

- Unit tests for page-jump math: page larger than the whole list, cursor already at top/bottom, windows containing multiple group headers, empty list.
- Unit tests for filter matching: case-insensitivity, name-only vs alias match, combined env+text filter narrowing, group hidden when no instance in it matches, cursor reset on query change.
- No `e2etests/` changes expected — the TUI key-loop isn't exercised by the binary-driven e2e suite.

## What is NOT changed

- The `e` env-cycle filter's own behavior (still cycles the same way).
- `ViewOverview`, `ViewCreateInput`, `ViewReconfigInput`, `ViewPromoteInput`, or any input-form widget.
- `scrollWindowVar`'s signature (only `renderedItem` gains a field; the windowing algorithm itself is unchanged).
