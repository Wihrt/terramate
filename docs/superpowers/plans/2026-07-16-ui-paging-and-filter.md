# UI Bundle List Paging & Filtering Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add PgUp/PgDn page-jump navigation and a `/`-triggered live text filter to the three bundle-list select screens in the `terramate ui` TUI (Scaffold/Create, Reconfigure, Promote), closing terramate-io/terramate#2371 and #2372.

**Architecture:** Each of the three views (`view_create_select.go`, `view_reconfig.go`, `view_promote.go`) gets its own paging math function and its own filter-state/matching logic — no new cross-view abstraction, per the approved design spec. Only trivial index-space plumbing (`renderedItem.selectable`, `firstSelectableIndex`/`lastSelectableIndex`) is added next to the pre-existing shared `scrollWindowVar`/`renderScrollbar` helpers in `view_create_select.go`.

**Tech Stack:** Go, `charmbracelet/bubbletea` v1.3.4, `charmbracelet/bubbles` v0.21.0 (`key`, `textinput`), `charmbracelet/lipgloss`. Tests use stdlib `testing` with table-driven cases (this package, `commands/ui`, currently has zero test files — this plan establishes the pattern).

## Global Constraints

- Every new/modified Go file keeps the copyright header: `// Copyright 2026 Terramate GmbH` / `// SPDX-License-Identifier: MPL-2.0` (see `CLAUDE.md`).
- Test files use `package ui` (internal test package, matching `commands/scaffold/scaffold_test.go`'s convention) since tests need access to unexported `Model` fields and helper functions.
- Run `make fmt` (goimports) and ensure `go vet ./commands/ui/...` is clean before each commit.
- Design spec of record: `docs/superpowers/specs/2026-07-16-ui-paging-and-filter-design.md`. If any step here appears to contradict it, the spec wins — stop and flag it rather than guessing.
- No cross-view shared filter/paging business logic — each view's `<view>PageCursor`, `<view>MatchesFilter`, and filter-state type are separate, independent functions/types, even where their bodies end up textually similar. This was an explicit, deliberate project decision (see spec's "Scope" section) — do not "clean this up" into a shared helper.

---

### Task 1: Shared plumbing — PgUp/PgDn key bindings and `renderedItem.selectable`

**Files:**
- Modify: `commands/ui/model.go` (`keyMap` struct and `var keys`)
- Modify: `commands/ui/view_create_select.go` (`renderedItem` struct, its one literal, adds `firstSelectableIndex`/`lastSelectableIndex`)
- Modify: `commands/ui/view_reconfig.go` (3 `renderedItem` literals)
- Modify: `commands/ui/view_promote.go` (3 `renderedItem` literals)
- Test: `commands/ui/model_test.go` (new)

**Interfaces:**
- Produces: `keys.PgUp`, `keys.PgDn key.Binding`; `renderedItem.selectable bool` field; `firstSelectableIndex(items []renderedItem) int`, `lastSelectableIndex(items []renderedItem) int`. Consumed by Tasks 2-7.

- [ ] **Step 1: Write the failing test for the new key bindings**

Create `commands/ui/model_test.go`:

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

func TestPagingKeyBindings(t *testing.T) {
	t.Parallel()

	if !key.Matches(tea.KeyMsg{Type: tea.KeyPgDown}, keys.PgDn) {
		t.Fatal("expected a pgdown key message to match keys.PgDn")
	}
	if !key.Matches(tea.KeyMsg{Type: tea.KeyPgUp}, keys.PgUp) {
		t.Fatal("expected a pgup key message to match keys.PgUp")
	}
	if key.Matches(tea.KeyMsg{Type: tea.KeyPgDown}, keys.PgUp) {
		t.Fatal("did not expect a pgdown key message to match keys.PgUp")
	}
}

func TestFirstLastSelectableIndex(t *testing.T) {
	t.Parallel()

	items := []renderedItem{
		{selectable: false},
		{selectable: true},
		{selectable: false},
		{selectable: true},
		{selectable: false},
	}

	if got := firstSelectableIndex(items); got != 1 {
		t.Fatalf("firstSelectableIndex() = %d, want 1", got)
	}
	if got := lastSelectableIndex(items); got != 3 {
		t.Fatalf("lastSelectableIndex() = %d, want 3", got)
	}
	if got := firstSelectableIndex(nil); got != 0 {
		t.Fatalf("firstSelectableIndex(nil) = %d, want 0", got)
	}
	if got := lastSelectableIndex(nil); got != 0 {
		t.Fatalf("lastSelectableIndex(nil) = %d, want 0", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./commands/ui/... -run 'TestPagingKeyBindings|TestFirstLastSelectableIndex' -v`
Expected: build FAILURE — `undefined: keys.PgDn` (or similar), since `keyMap` doesn't have `PgUp`/`PgDn` yet and `firstSelectableIndex`/`lastSelectableIndex` don't exist.

- [ ] **Step 3: Add `PgUp`/`PgDn` to the key map**

In `commands/ui/model.go`, find:

```go
// keyMap defines the key bindings for the prompt UI.
type keyMap struct {
	Quit   key.Binding
	Tab    key.Binding
	Enter  key.Binding
	Escape key.Binding
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
}
```

Replace with:

```go
// keyMap defines the key bindings for the prompt UI.
type keyMap struct {
	Quit   key.Binding
	Tab    key.Binding
	Enter  key.Binding
	Escape key.Binding
	Up     key.Binding
	Down   key.Binding
	Left   key.Binding
	Right  key.Binding
	PgUp   key.Binding
	PgDn   key.Binding
}
```

Then find the end of `var keys = keyMap{...}`:

```go
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
}
```

Replace with:

```go
	Right: key.NewBinding(
		key.WithKeys("right", "l"),
		key.WithHelp("→/l", "right"),
	),
	PgUp: key.NewBinding(
		key.WithKeys("pgup", "b"),
		key.WithHelp("pgup", "page up"),
	),
	PgDn: key.NewBinding(
		key.WithKeys("pgdown", "f"),
		key.WithHelp("pgdn", "page down"),
	),
}
```

`b`/`f` are included as `less`-style aliases per the design spec. They're safe: outside filter-edit mode (the only place `b`/`f` would otherwise be typed as filter text), these two views have no other single-letter binding that collides except `e` (env-cycle in Reconfigure/Promote), which is untouched.

- [ ] **Step 4: Add `selectable` to `renderedItem` and the index helpers**

In `commands/ui/view_create_select.go`, find:

```go
type renderedItem struct {
	content string
	height  int
}
```

Replace with:

```go
type renderedItem struct {
	content    string
	height     int
	selectable bool // false for non-selectable rows such as group headers/separators
}

// firstSelectableIndex returns the index of the first selectable item, or 0 if none.
func firstSelectableIndex(items []renderedItem) int {
	for i, it := range items {
		if it.selectable {
			return i
		}
	}
	return 0
}

// lastSelectableIndex returns the index of the last selectable item, or 0 if none.
func lastSelectableIndex(items []renderedItem) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].selectable {
			return i
		}
	}
	return 0
}
```

- [ ] **Step 5: Update the one `renderedItem{}` literal in `view_create_select.go`**

Find:

```go
		items = append(items, renderedItem{content: block, height: lipgloss.Height(block)})
```

Replace with:

```go
		items = append(items, renderedItem{content: block, height: lipgloss.Height(block), selectable: true})
```

- [ ] **Step 6: Update the 3 `renderedItem{}` literals in `view_reconfig.go`**

Find (appears once):

```go
		// Empty line before group (except first)
		if gi > 0 {
			items = append(items, renderedItem{content: "", height: 1})
		}

		// Group header: non-selectable
		headerLine := headerNameStyle.Render(g.name) + " " + versionStyle.Render("v"+b0.DefinitionMetadata.Version)
		items = append(items, renderedItem{content: lineStyle.Render(headerLine), height: 1})
```

Replace with:

```go
		// Empty line before group (except first)
		if gi > 0 {
			items = append(items, renderedItem{content: "", height: 1, selectable: false})
		}

		// Group header: non-selectable
		headerLine := headerNameStyle.Render(g.name) + " " + versionStyle.Render("v"+b0.DefinitionMetadata.Version)
		items = append(items, renderedItem{content: lineStyle.Render(headerLine), height: 1, selectable: false})
```

Find (appears once, inside the instance-row loop):

```go
			items = append(items, renderedItem{content: lineStyle.Render(line), height: 1})
```

Replace with:

```go
			items = append(items, renderedItem{content: lineStyle.Render(line), height: 1, selectable: true})
```

- [ ] **Step 7: Update the 3 `renderedItem{}` literals in `view_promote.go`**

Apply the exact same two replacements as Step 6 (the surrounding code is textually identical) to `commands/ui/view_promote.go`.

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestPagingKeyBindings|TestFirstLastSelectableIndex' -v`
Expected: PASS (2 tests)

- [ ] **Step 9: Confirm the package still builds**

Run: `go build ./commands/ui/...`
Expected: no output, exit code 0

- [ ] **Step 10: Commit**

```bash
git add commands/ui/model.go commands/ui/view_create_select.go commands/ui/view_reconfig.go commands/ui/view_promote.go commands/ui/model_test.go
git commit -m "feat(ui): add PgUp/PgDn key bindings and selectable rendered items"
```

---

### Task 2: Flat bundle list (Scaffold/Create) paging

**Files:**
- Modify: `commands/ui/view_create_select.go`
- Test: `commands/ui/view_create_select_test.go` (new)

**Interfaces:**
- Consumes: `renderedItem{selectable bool}`, `firstSelectableIndex`, `lastSelectableIndex`, `scrollWindowVar` (Task 1).
- Produces: `buildFlatBundleItems(entries []flatBundleEntry, cursor, contentWidth int) []renderedItem`, `flatBundleListHeader(innerWidth int) string` (method on `Model`), `flatBundlePageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int`. Consumed by Task 5.

- [ ] **Step 1: Write the failing tests**

Create `commands/ui/view_create_select_test.go`:

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	"github.com/terramate-io/terramate/scaffold/manifest"
)

func TestFlatBundlePageCursor(t *testing.T) {
	t.Parallel()

	items := make([]renderedItem, 20)
	for i := range items {
		items[i] = renderedItem{content: "x", height: 1, selectable: true}
	}

	testcases := []struct {
		name            string
		cursor          int
		availableHeight int
		down            bool
		want            int
	}{
		{name: "page down from top", cursor: 0, availableHeight: 5, down: true, want: 3},
		{name: "page down near end clamps to last", cursor: 17, availableHeight: 5, down: true, want: 19},
		{name: "page up from middle", cursor: 10, availableHeight: 5, down: false, want: 8},
		{name: "page up clamps to first", cursor: 2, availableHeight: 5, down: false, want: 0},
		{name: "page larger than whole list clamps down to last", cursor: 3, availableHeight: 100, down: true, want: 19},
		{name: "page larger than whole list clamps up to first", cursor: 3, availableHeight: 100, down: false, want: 0},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := flatBundlePageCursor(items, tc.cursor, tc.availableHeight, 1, tc.down)
			if got != tc.want {
				t.Fatalf("flatBundlePageCursor(cursor=%d, height=%d, down=%v) = %d, want %d",
					tc.cursor, tc.availableHeight, tc.down, got, tc.want)
			}
		})
	}
}

func TestFlatBundlePageCursorEmptyList(t *testing.T) {
	t.Parallel()
	if got := flatBundlePageCursor(nil, 0, 10, 1, true); got != 0 {
		t.Fatalf("expected 0 for an empty list, got %d", got)
	}
}

func TestBuildFlatBundleItems(t *testing.T) {
	t.Parallel()

	entries := []flatBundleEntry{
		{bundle: &manifest.Bundle{Name: "vpc", Version: "1.0.0"}, collName: "local"},
		{bundle: &manifest.Bundle{Name: "ecs", Version: "2.0.0", Description: "Elastic Container Service"}, collName: "local"},
	}

	items := buildFlatBundleItems(entries, 0, 60)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !items[0].selectable || !items[1].selectable {
		t.Fatal("expected all flat bundle items to be selectable")
	}
	if items[0].height != 1 {
		t.Fatalf("expected the item without a description to have height 1, got %d", items[0].height)
	}
	if items[1].height != 2 {
		t.Fatalf("expected the item with a description to have height 2, got %d", items[1].height)
	}
}
```

The expected cursor values in `TestFlatBundlePageCursor` were computed by hand-tracing `scrollWindowVar` with 20 one-line items and `sep=1` — availableHeight 5 fits exactly 3 items per window (`2k-1 <= 5` → `k=3`); do not "fix" these numbers without re-deriving them the same way.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./commands/ui/... -run 'TestFlatBundlePageCursor|TestBuildFlatBundleItems' -v`
Expected: build FAILURE — `undefined: flatBundlePageCursor` / `undefined: buildFlatBundleItems`

- [ ] **Step 3: Extract `buildFlatBundleItems` and `flatBundleListHeader`, add `flatBundlePageCursor`**

In `commands/ui/view_create_select.go`, find `renderFlatBundleList` in full:

```go
func (m Model) renderFlatBundleList(innerWidth int) string {
	scrollbarGutter := 4
	contentWidth := innerWidth - scrollbarGutter

	// Detail box for the highlighted bundle
	var detailBox string
	if m.flatBundleCursor < len(m.flatBundles) {
		est := m.EngineState
		entry := m.flatBundles[m.flatBundleCursor]
		fields := []detailField{
			{label: "Bundle", value: entry.bundle.Name + " v" + entry.bundle.Version, truncEnd: true},
		}
		if entry.bundle.Class != "" {
			fields = append(fields, detailField{label: "Class", value: entry.bundle.Class, truncEnd: true})
		}
		fields = append(fields, detailField{}) // separator
		// Collection: name and location
		coll := est.Collections[entry.collIdx]
		fields = append(fields, detailField{label: "Collection", value: coll.Name})
		// Source: the real resolved source path to the bundle definition
		var source string
		if entry.isLocal && entry.bundleIdx < len(est.LocalBundleDefs) {
			source = est.LocalBundleDefs[entry.bundleIdx].Tree.Dir().String()
		} else {
			source = bundleSourceFromManifest(coll, entry.bundle)
		}
		fields = append(fields, detailField{label: "Source", value: source})
		detailBox = renderDetailBox(innerWidth, "Bundle Details", fields)
	}

	headerParts := []string{detailBox}
	if m.bundleSelectErr != "" {
		headerParts = append(headerParts, renderErrorBox(innerWidth, m.bundleSelectErr))
	}
	headerParts = append(headerParts, "")
	header := lipgloss.JoinVertical(lipgloss.Left, headerParts...)
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight

	itemStyle := lipgloss.NewStyle().
		Bold(true).
		Width(contentWidth)

	selectedStyle := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Width(contentWidth)

	versionStyle := lipgloss.NewStyle().
		Foreground(colorTextSubtle)

	descStyle := lipgloss.NewStyle().
		PaddingLeft(6).
		Foreground(colorTextMuted).
		Width(contentWidth)

	collStyle := lipgloss.NewStyle().Foreground(colorTextMuted)

	var items []renderedItem
	for i, entry := range m.flatBundles {
		displayName := entry.bundle.Name + " " + versionStyle.Render("v"+entry.bundle.Version) + " " + collStyle.Render("• "+entry.collName)

		var line string
		if i == m.flatBundleCursor {
			line = selectedStyle.Render("› " + displayName)
		} else {
			line = itemStyle.Render("  " + displayName)
		}

		block := line
		if entry.bundle.Description != "" {
			block += "\n" + descStyle.Render(summaryLine(strings.TrimSpace(entry.bundle.Description)))
		}
		items = append(items, renderedItem{content: block, height: lipgloss.Height(block), selectable: true})
	}

	start, end := scrollWindowVar(m.flatBundleCursor, items, availableHeight, 1)
```

Replace the whole function body down to (and including) that `start, end := ...` line with:

```go
func (m Model) renderFlatBundleList(innerWidth int) string {
	scrollbarGutter := 4
	contentWidth := innerWidth - scrollbarGutter

	header := m.flatBundleListHeader(innerWidth)
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight

	items := buildFlatBundleItems(m.flatBundles, m.flatBundleCursor, contentWidth)

	start, end := scrollWindowVar(m.flatBundleCursor, items, availableHeight, 1)
```

(Leave the rest of `renderFlatBundleList` — the scrollbar/`sb.WriteString` loop below — untouched; it still refers to `items`, `start`, `end`, `header`, which all still exist.)

Now add the two new functions immediately after the closing `}` of `renderFlatBundleList` (i.e., right before `type renderedItem struct`):

```go
// flatBundleListHeader renders the detail box (and any inline error) shown
// above the flat bundle list. Used both for display and, via lipgloss.Height,
// to compute the available height for PgUp/PgDn page-jump math.
func (m Model) flatBundleListHeader(innerWidth int) string {
	var detailBox string
	if m.flatBundleCursor < len(m.flatBundles) {
		est := m.EngineState
		entry := m.flatBundles[m.flatBundleCursor]
		fields := []detailField{
			{label: "Bundle", value: entry.bundle.Name + " v" + entry.bundle.Version, truncEnd: true},
		}
		if entry.bundle.Class != "" {
			fields = append(fields, detailField{label: "Class", value: entry.bundle.Class, truncEnd: true})
		}
		fields = append(fields, detailField{}) // separator
		// Collection: name and location
		coll := est.Collections[entry.collIdx]
		fields = append(fields, detailField{label: "Collection", value: coll.Name})
		// Source: the real resolved source path to the bundle definition
		var source string
		if entry.isLocal && entry.bundleIdx < len(est.LocalBundleDefs) {
			source = est.LocalBundleDefs[entry.bundleIdx].Tree.Dir().String()
		} else {
			source = bundleSourceFromManifest(coll, entry.bundle)
		}
		fields = append(fields, detailField{label: "Source", value: source})
		detailBox = renderDetailBox(innerWidth, "Bundle Details", fields)
	}

	headerParts := []string{detailBox}
	if m.bundleSelectErr != "" {
		headerParts = append(headerParts, renderErrorBox(innerWidth, m.bundleSelectErr))
	}
	headerParts = append(headerParts, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}

// buildFlatBundleItems renders each flat bundle entry into a renderedItem,
// used both for display and for PgUp/PgDn page-jump math.
func buildFlatBundleItems(entries []flatBundleEntry, cursor, contentWidth int) []renderedItem {
	itemStyle := lipgloss.NewStyle().
		Bold(true).
		Width(contentWidth)

	selectedStyle := lipgloss.NewStyle().
		Foreground(colorPrimary).
		Bold(true).
		Width(contentWidth)

	versionStyle := lipgloss.NewStyle().
		Foreground(colorTextSubtle)

	descStyle := lipgloss.NewStyle().
		PaddingLeft(6).
		Foreground(colorTextMuted).
		Width(contentWidth)

	collStyle := lipgloss.NewStyle().Foreground(colorTextMuted)

	var items []renderedItem
	for i, entry := range entries {
		displayName := entry.bundle.Name + " " + versionStyle.Render("v"+entry.bundle.Version) + " " + collStyle.Render("• "+entry.collName)

		var line string
		if i == cursor {
			line = selectedStyle.Render("› " + displayName)
		} else {
			line = itemStyle.Render("  " + displayName)
		}

		block := line
		if entry.bundle.Description != "" {
			block += "\n" + descStyle.Render(summaryLine(strings.TrimSpace(entry.bundle.Description)))
		}
		items = append(items, renderedItem{content: block, height: lipgloss.Height(block), selectable: true})
	}
	return items
}

// flatBundlePageCursor returns the new cursor position for a PgUp/PgDn jump
// over the flat bundle list. down selects PgDn (next page) vs PgUp (previous page).
func flatBundlePageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int {
	if len(items) == 0 {
		return 0
	}
	start, end := scrollWindowVar(cursor, items, availableHeight, sep)
	if down {
		for i := end; i < len(items); i++ {
			if items[i].selectable {
				return i
			}
		}
		return lastSelectableIndex(items)
	}
	for i := start - 1; i >= 0; i-- {
		if items[i].selectable {
			return i
		}
	}
	return firstSelectableIndex(items)
}
```

- [ ] **Step 4: Wire PgUp/PgDn into `updateCreateSelect`**

Find:

```go
func (m Model) updateCreateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.viewState = ViewOverview
		return m, nil

	case key.Matches(msg, keys.Up):
```

Replace with:

```go
func (m Model) updateCreateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.viewState = ViewOverview
		return m, nil

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
		down := key.Matches(msg, keys.PgDn)
		innerWidth := m.effectiveWidth() - 4
		contentWidth := innerWidth - 4 // scrollbarGutter, matches renderFlatBundleList
		availableHeight := m.effectiveContentHeight() - lipgloss.Height(m.flatBundleListHeader(innerWidth))
		items := buildFlatBundleItems(m.flatBundles, m.flatBundleCursor, contentWidth)
		m.flatBundleCursor = flatBundlePageCursor(items, m.flatBundleCursor, availableHeight, 1, down)
		m.bundleSelectErr = ""
		return m, nil

	case key.Matches(msg, keys.Up):
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestFlatBundlePageCursor|TestBuildFlatBundleItems' -v`
Expected: PASS (all subtests)

- [ ] **Step 6: Build and run the full package test suite**

Run: `go build ./commands/ui/... && go test ./commands/ui/... -v`
Expected: PASS, no build errors

- [ ] **Step 7: Commit**

```bash
git add commands/ui/view_create_select.go commands/ui/view_create_select_test.go
git commit -m "feat(ui): add PgUp/PgDn paging to the Scaffold bundle list"
```

---

### Task 3: Reconfigure bundle list paging

**Files:**
- Modify: `commands/ui/view_reconfig.go`
- Test: `commands/ui/view_reconfig_test.go` (new)

**Interfaces:**
- Consumes: `renderedItem`, `firstSelectableIndex`, `lastSelectableIndex`, `scrollWindowVar`, `groupBundles`, `renderGroupedBundleItems` (all pre-existing or Task 1).
- Produces: `reconfigListHeader(innerWidth int) string` (method on `Model`), `reconfigPageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int`, `reconfigCursorForItem(items []renderedItem, itemIdx int) int`. Consumed by Task 6.

- [ ] **Step 1: Write the failing tests**

Create `commands/ui/view_reconfig_test.go`:

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import "testing"

// reconfigTestItems builds a synthetic 2-group, 6-instance rendered item
// list matching the shape renderGroupedBundleItems produces: each group is
// [blank?, header, instance...], all height 1.
//
//	0: header A (not selectable)
//	1: A1
//	2: A2
//	3: A3
//	4: blank (not selectable)
//	5: header B (not selectable)
//	6: B1
//	7: B2
//	8: B3
func reconfigTestItems() []renderedItem {
	return []renderedItem{
		{content: "Group A", height: 1, selectable: false},
		{content: "A1", height: 1, selectable: true},
		{content: "A2", height: 1, selectable: true},
		{content: "A3", height: 1, selectable: true},
		{content: "", height: 1, selectable: false},
		{content: "Group B", height: 1, selectable: false},
		{content: "B1", height: 1, selectable: true},
		{content: "B2", height: 1, selectable: true},
		{content: "B3", height: 1, selectable: true},
	}
}

func TestReconfigPageCursor(t *testing.T) {
	t.Parallel()
	items := reconfigTestItems()

	testcases := []struct {
		name            string
		itemIdx         int
		availableHeight int
		down            bool
		want            int
	}{
		{name: "page down skips the group boundary, lands on first bundle of next group", itemIdx: 1, availableHeight: 4, down: true, want: 6},
		{name: "page up skips the group boundary, lands on last bundle of previous group", itemIdx: 7, availableHeight: 4, down: false, want: 3},
		{name: "page down at the last item clamps", itemIdx: 8, availableHeight: 4, down: true, want: 8},
		{name: "page up at the first item clamps", itemIdx: 1, availableHeight: 4, down: false, want: 1},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reconfigPageCursor(items, tc.itemIdx, tc.availableHeight, 0, tc.down)
			if got != tc.want {
				t.Fatalf("reconfigPageCursor(itemIdx=%d, height=%d, down=%v) = %d, want %d",
					tc.itemIdx, tc.availableHeight, tc.down, got, tc.want)
			}
		})
	}
}

func TestReconfigCursorForItem(t *testing.T) {
	t.Parallel()
	items := reconfigTestItems()

	testcases := []struct {
		name    string
		itemIdx int
		want    int
	}{
		{name: "first selectable item (A1)", itemIdx: 1, want: 0},
		{name: "third selectable item (A3)", itemIdx: 3, want: 2},
		{name: "first item of second group (B1)", itemIdx: 6, want: 3},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := reconfigCursorForItem(items, tc.itemIdx); got != tc.want {
				t.Fatalf("reconfigCursorForItem(%d) = %d, want %d", tc.itemIdx, got, tc.want)
			}
		})
	}
}
```

These expected values were hand-traced through `scrollWindowVar` with `sep=0` over the 9-item fixture above; do not adjust them without re-tracing the same way (see the design spec's paging section for the general algorithm).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./commands/ui/... -run 'TestReconfigPageCursor|TestReconfigCursorForItem' -v`
Expected: build FAILURE — `undefined: reconfigPageCursor` / `undefined: reconfigCursorForItem`

- [ ] **Step 3: Extract `reconfigListHeader`, add `reconfigPageCursor` and `reconfigCursorForItem`**

In `commands/ui/view_reconfig.go`, find inside `renderReconfigSelectView`:

```go
	// Detail box for highlighted bundle
	var detailBox string
	if m.reconfigCursor < len(m.reconfigBundles) {
		b := m.reconfigBundles[m.reconfigCursor]
		fields := []detailField{
			{label: "Bundle", value: b.DefinitionMetadata.Name + " v" + b.DefinitionMetadata.Version, truncEnd: true},
		}
		if b.DefinitionMetadata.Class != "" {
			fields = append(fields, detailField{label: "Class", value: b.DefinitionMetadata.Class, truncEnd: true})
		}
		fields = append(fields, detailField{label: "Alias", value: displayNameFromAlias(b.Alias, b.Name), truncEnd: true})
		envName := "n/a"
		if b.Environment != nil {
			envName = b.Environment.Name
		}
		fields = append(fields, detailField{label: "Environment", value: envName, truncEnd: true})
		fields = append(fields, detailField{}) // separator
		fields = append(fields, detailField{label: "Source", value: b.Source})
		hostPath := project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()).String()
		if hostPath != "" {
			fields = append(fields, detailField{label: "Config", value: hostPath})
		}
		detailBox = renderDetailBox(innerWidth, "Bundle Instance Details", fields)
	}

	header := lipgloss.JoinVertical(lipgloss.Left, detailBox, "")
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight
```

Replace with:

```go
	header := m.reconfigListHeader(innerWidth)
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight
```

Then add the extracted helper and the two new pure functions right after the closing `}` of `renderReconfigSelectView`:

```go
// reconfigListHeader renders the detail box shown above the Reconfigure
// bundle list. Used both for display and, via lipgloss.Height, to compute
// the available height for PgUp/PgDn page-jump math.
func (m Model) reconfigListHeader(innerWidth int) string {
	est := m.EngineState
	var detailBox string
	if m.reconfigCursor < len(m.reconfigBundles) {
		b := m.reconfigBundles[m.reconfigCursor]
		fields := []detailField{
			{label: "Bundle", value: b.DefinitionMetadata.Name + " v" + b.DefinitionMetadata.Version, truncEnd: true},
		}
		if b.DefinitionMetadata.Class != "" {
			fields = append(fields, detailField{label: "Class", value: b.DefinitionMetadata.Class, truncEnd: true})
		}
		fields = append(fields, detailField{label: "Alias", value: displayNameFromAlias(b.Alias, b.Name), truncEnd: true})
		envName := "n/a"
		if b.Environment != nil {
			envName = b.Environment.Name
		}
		fields = append(fields, detailField{label: "Environment", value: envName, truncEnd: true})
		fields = append(fields, detailField{}) // separator
		fields = append(fields, detailField{label: "Source", value: b.Source})
		hostPath := project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()).String()
		if hostPath != "" {
			fields = append(fields, detailField{label: "Config", value: hostPath})
		}
		detailBox = renderDetailBox(innerWidth, "Bundle Instance Details", fields)
	}
	return lipgloss.JoinVertical(lipgloss.Left, detailBox, "")
}

// reconfigPageCursor returns the new item-list index for a PgUp/PgDn jump
// over the Reconfigure bundle list. down selects PgDn vs PgUp. cursor and
// the return value are indices into items (as returned by
// renderGroupedBundleItems), not bundle indices — see reconfigCursorForItem.
func reconfigPageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int {
	if len(items) == 0 {
		return 0
	}
	start, end := scrollWindowVar(cursor, items, availableHeight, sep)
	if down {
		for i := end; i < len(items); i++ {
			if items[i].selectable {
				return i
			}
		}
		return lastSelectableIndex(items)
	}
	for i := start - 1; i >= 0; i-- {
		if items[i].selectable {
			return i
		}
	}
	return firstSelectableIndex(items)
}

// reconfigCursorForItem converts an index into the rendered items list back
// into a bundle-index (m.reconfigCursor space) by counting selectable items
// before it.
func reconfigCursorForItem(items []renderedItem, itemIdx int) int {
	rank := 0
	for i := 0; i < itemIdx; i++ {
		if items[i].selectable {
			rank++
		}
	}
	return rank
}
```

- [ ] **Step 4: Wire PgUp/PgDn into `updateReconfigSelect`**

Find:

```go
	case key.Matches(msg, keys.Up):
		if m.reconfigCursor > 0 {
			m.reconfigCursor--
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.reconfigCursor < len(m.reconfigBundles)-1 {
			m.reconfigCursor++
		}
		return m, nil

	case msg.String() == "e":
```

Replace with:

```go
	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
		down := key.Matches(msg, keys.PgDn)
		innerWidth := m.effectiveWidth() - 4
		contentWidth := innerWidth - 4 // scrollbarGutter, matches renderReconfigSelectView
		availableHeight := m.effectiveContentHeight() - lipgloss.Height(m.reconfigListHeader(innerWidth))
		groups := groupBundles(m.reconfigBundles)
		selectedItemIdx, items := m.renderGroupedBundleItems(groups, m.reconfigCursor, contentWidth)
		newItemIdx := reconfigPageCursor(items, selectedItemIdx, availableHeight, 0, down)
		m.reconfigCursor = reconfigCursorForItem(items, newItemIdx)
		return m, nil

	case key.Matches(msg, keys.Up):
		if m.reconfigCursor > 0 {
			m.reconfigCursor--
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.reconfigCursor < len(m.reconfigBundles)-1 {
			m.reconfigCursor++
		}
		return m, nil

	case msg.String() == "e":
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestReconfigPageCursor|TestReconfigCursorForItem' -v`
Expected: PASS (all subtests)

- [ ] **Step 6: Build and run the full package test suite**

Run: `go build ./commands/ui/... && go test ./commands/ui/... -v`
Expected: PASS, no build errors

- [ ] **Step 7: Commit**

```bash
git add commands/ui/view_reconfig.go commands/ui/view_reconfig_test.go
git commit -m "feat(ui): add PgUp/PgDn paging to the Reconfigure bundle list"
```

---

### Task 4: Promote bundle list paging

**Files:**
- Modify: `commands/ui/view_promote.go`
- Test: `commands/ui/view_promote_test.go` (new)

**Interfaces:**
- Consumes: same as Task 3, plus `renderPromoteGroupedItems` (pre-existing).
- Produces: `promoteListHeader(innerWidth int) string` (method on `Model`), `promotePageCursor(...)`, `promoteCursorForItem(...)` — same shapes as the Reconfigure equivalents. Consumed by Task 7.

- [ ] **Step 1: Write the failing tests**

Create `commands/ui/view_promote_test.go`:

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import "testing"

// promoteTestItems mirrors reconfigTestItems (see view_reconfig_test.go) —
// a synthetic 2-group, 6-instance rendered item list.
func promoteTestItems() []renderedItem {
	return []renderedItem{
		{content: "Group A", height: 1, selectable: false},
		{content: "A1", height: 1, selectable: true},
		{content: "A2", height: 1, selectable: true},
		{content: "A3", height: 1, selectable: true},
		{content: "", height: 1, selectable: false},
		{content: "Group B", height: 1, selectable: false},
		{content: "B1", height: 1, selectable: true},
		{content: "B2", height: 1, selectable: true},
		{content: "B3", height: 1, selectable: true},
	}
}

func TestPromotePageCursor(t *testing.T) {
	t.Parallel()
	items := promoteTestItems()

	testcases := []struct {
		name            string
		itemIdx         int
		availableHeight int
		down            bool
		want            int
	}{
		{name: "page down skips the group boundary, lands on first bundle of next group", itemIdx: 1, availableHeight: 4, down: true, want: 6},
		{name: "page up skips the group boundary, lands on last bundle of previous group", itemIdx: 7, availableHeight: 4, down: false, want: 3},
		{name: "page down at the last item clamps", itemIdx: 8, availableHeight: 4, down: true, want: 8},
		{name: "page up at the first item clamps", itemIdx: 1, availableHeight: 4, down: false, want: 1},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := promotePageCursor(items, tc.itemIdx, tc.availableHeight, 0, tc.down)
			if got != tc.want {
				t.Fatalf("promotePageCursor(itemIdx=%d, height=%d, down=%v) = %d, want %d",
					tc.itemIdx, tc.availableHeight, tc.down, got, tc.want)
			}
		})
	}
}

func TestPromoteCursorForItem(t *testing.T) {
	t.Parallel()
	items := promoteTestItems()

	testcases := []struct {
		name    string
		itemIdx int
		want    int
	}{
		{name: "first selectable item (A1)", itemIdx: 1, want: 0},
		{name: "third selectable item (A3)", itemIdx: 3, want: 2},
		{name: "first item of second group (B1)", itemIdx: 6, want: 3},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := promoteCursorForItem(items, tc.itemIdx); got != tc.want {
				t.Fatalf("promoteCursorForItem(%d) = %d, want %d", tc.itemIdx, got, tc.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./commands/ui/... -run 'TestPromotePageCursor|TestPromoteCursorForItem' -v`
Expected: build FAILURE — `undefined: promotePageCursor` / `undefined: promoteCursorForItem`

- [ ] **Step 3: Extract `promoteListHeader`, add `promotePageCursor` and `promoteCursorForItem`**

In `commands/ui/view_promote.go`, find inside `renderPromoteSelectView`:

```go
	// Detail box for highlighted bundle
	var detailBox string
	if m.promoteCursor < len(m.promoteBundles) {
		b := m.promoteBundles[m.promoteCursor]
		fields := []detailField{
			{label: "Bundle", value: b.DefinitionMetadata.Name + " v" + b.DefinitionMetadata.Version, truncEnd: true},
		}
		if b.DefinitionMetadata.Class != "" {
			fields = append(fields, detailField{label: "Class", value: b.DefinitionMetadata.Class, truncEnd: true})
		}
		fields = append(fields, detailField{label: "Alias", value: displayNameFromAlias(b.Alias, b.Name), truncEnd: true})
		if m.promoteCursor < len(m.promoteTargetEnvs) {
			sourceEnvName := envNameForID(est.Registry.Environments, b.Environment.ID)
			targetEnvName := m.promoteTargetEnvs[m.promoteCursor].Name
			fields = append(fields, detailField{label: "Promote", value: sourceEnvName + " → " + targetEnvName, truncEnd: true})
		}
		fields = append(fields, detailField{}) // separator
		fields = append(fields, detailField{label: "Source", value: b.Source})
		hostPath := project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()).String()
		if hostPath != "" {
			fields = append(fields, detailField{label: "Config", value: hostPath})
		}
		detailBox = renderDetailBox(innerWidth, "Bundle Instance Details", fields)
	}

	header := lipgloss.JoinVertical(lipgloss.Left, detailBox, "")
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight
```

Replace with:

```go
	header := m.promoteListHeader(innerWidth)
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight
```

Then add the extracted helper and the two new pure functions right after the closing `}` of `renderPromoteSelectView`:

```go
// promoteListHeader renders the detail box shown above the Promote bundle
// list. Used both for display and, via lipgloss.Height, to compute the
// available height for PgUp/PgDn page-jump math.
func (m Model) promoteListHeader(innerWidth int) string {
	est := m.EngineState
	var detailBox string
	if m.promoteCursor < len(m.promoteBundles) {
		b := m.promoteBundles[m.promoteCursor]
		fields := []detailField{
			{label: "Bundle", value: b.DefinitionMetadata.Name + " v" + b.DefinitionMetadata.Version, truncEnd: true},
		}
		if b.DefinitionMetadata.Class != "" {
			fields = append(fields, detailField{label: "Class", value: b.DefinitionMetadata.Class, truncEnd: true})
		}
		fields = append(fields, detailField{label: "Alias", value: displayNameFromAlias(b.Alias, b.Name), truncEnd: true})
		if m.promoteCursor < len(m.promoteTargetEnvs) {
			sourceEnvName := envNameForID(est.Registry.Environments, b.Environment.ID)
			targetEnvName := m.promoteTargetEnvs[m.promoteCursor].Name
			fields = append(fields, detailField{label: "Promote", value: sourceEnvName + " → " + targetEnvName, truncEnd: true})
		}
		fields = append(fields, detailField{}) // separator
		fields = append(fields, detailField{label: "Source", value: b.Source})
		hostPath := project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()).String()
		if hostPath != "" {
			fields = append(fields, detailField{label: "Config", value: hostPath})
		}
		detailBox = renderDetailBox(innerWidth, "Bundle Instance Details", fields)
	}
	return lipgloss.JoinVertical(lipgloss.Left, detailBox, "")
}

// promotePageCursor returns the new item-list index for a PgUp/PgDn jump
// over the Promote bundle list. See reconfigPageCursor for the algorithm.
func promotePageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int {
	if len(items) == 0 {
		return 0
	}
	start, end := scrollWindowVar(cursor, items, availableHeight, sep)
	if down {
		for i := end; i < len(items); i++ {
			if items[i].selectable {
				return i
			}
		}
		return lastSelectableIndex(items)
	}
	for i := start - 1; i >= 0; i-- {
		if items[i].selectable {
			return i
		}
	}
	return firstSelectableIndex(items)
}

// promoteCursorForItem converts an index into the rendered items list back
// into a bundle-index (m.promoteCursor space) by counting selectable items
// before it.
func promoteCursorForItem(items []renderedItem, itemIdx int) int {
	rank := 0
	for i := 0; i < itemIdx; i++ {
		if items[i].selectable {
			rank++
		}
	}
	return rank
}
```

- [ ] **Step 4: Wire PgUp/PgDn into `updatePromoteSelect`**

Find:

```go
	case key.Matches(msg, keys.Up):
		if m.promoteCursor > 0 {
			m.promoteCursor--
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.promoteCursor < len(m.promoteBundles)-1 {
			m.promoteCursor++
		}
		return m, nil

	case msg.String() == "e":
```

Replace with:

```go
	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
		down := key.Matches(msg, keys.PgDn)
		innerWidth := m.effectiveWidth() - 4
		contentWidth := innerWidth - 4 // scrollbarGutter, matches renderPromoteSelectView
		availableHeight := m.effectiveContentHeight() - lipgloss.Height(m.promoteListHeader(innerWidth))
		groups := groupBundles(m.promoteBundles)
		selectedItemIdx, items := m.renderPromoteGroupedItems(groups, m.promoteCursor, contentWidth)
		newItemIdx := promotePageCursor(items, selectedItemIdx, availableHeight, 0, down)
		m.promoteCursor = promoteCursorForItem(items, newItemIdx)
		return m, nil

	case key.Matches(msg, keys.Up):
		if m.promoteCursor > 0 {
			m.promoteCursor--
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if m.promoteCursor < len(m.promoteBundles)-1 {
			m.promoteCursor++
		}
		return m, nil

	case msg.String() == "e":
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestPromotePageCursor|TestPromoteCursorForItem' -v`
Expected: PASS (all subtests)

- [ ] **Step 6: Build and run the full package test suite**

Run: `go build ./commands/ui/... && go test ./commands/ui/... -v`
Expected: PASS, no build errors

- [ ] **Step 7: Commit**

```bash
git add commands/ui/view_promote.go commands/ui/view_promote_test.go
git commit -m "feat(ui): add PgUp/PgDn paging to the Promote bundle list"
```

---

### Task 5: Flat bundle list (Scaffold/Create) free-text filter

**Files:**
- Modify: `commands/ui/view_create_select.go` (add `textinput` import)
- Modify: `commands/ui/model.go` (`Model` struct fields)
- Modify: `commands/ui/view_overview.go` (`executeCommand`'s `"Scaffold"` case)
- Test: `commands/ui/view_create_select_test.go` (append to Task 2's file)

**Interfaces:**
- Consumes: `flatBundleEntry`, `buildFlatBundles` (pre-existing).
- Produces: `flatFilterState` type, `newFlatFilterState() flatFilterState`, `flatBundleMatchesFilter(entry flatBundleEntry, query string) bool`, `(m *Model) applyFlatBundleFilter()`, `Model.allFlatBundles`, `Model.flatBundleFilter`.

- [ ] **Step 1: Write the failing tests**

Append to `commands/ui/view_create_select_test.go`:

```go
func TestFlatBundleMatchesFilter(t *testing.T) {
	t.Parallel()
	entry := flatBundleEntry{bundle: &manifest.Bundle{Name: "VPC-Network"}}

	testcases := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "empty query matches everything", query: "", want: true},
		{name: "exact case match", query: "VPC", want: true},
		{name: "case-insensitive match", query: "vpc-network", want: true},
		{name: "substring match", query: "network", want: true},
		{name: "no match", query: "ecs", want: false},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := flatBundleMatchesFilter(entry, tc.query); got != tc.want {
				t.Fatalf("flatBundleMatchesFilter(query=%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestUpdateCreateSelectFilter(t *testing.T) {
	t.Parallel()

	entries := []flatBundleEntry{
		{bundle: &manifest.Bundle{Name: "vpc", Version: "1.0.0"}, collName: "local"},
		{bundle: &manifest.Bundle{Name: "ecs", Version: "1.0.0"}, collName: "local"},
	}
	m := Model{viewState: ViewCreateSelect, allFlatBundles: entries, flatBundleFilter: newFlatFilterState()}
	m.applyFlatBundleFilter()

	// "/" enters filter-edit mode.
	updated, _ := m.updateCreateSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(Model)
	if !m.flatBundleFilter.editing {
		t.Fatal("expected filter mode to be active after '/'")
	}

	// Typing narrows the list live.
	updated, _ = m.updateCreateSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
	m = updated.(Model)
	if len(m.flatBundles) != 1 || m.flatBundles[0].bundle.Name != "vpc" {
		t.Fatalf("expected filter %q to narrow to [vpc], got %v", m.flatBundleFilter.input.Value(), m.flatBundles)
	}

	// Enter keeps the filter applied and exits edit mode.
	updated, _ = m.updateCreateSelect(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.flatBundleFilter.editing {
		t.Fatal("expected filter mode to be inactive after enter")
	}
	if len(m.flatBundles) != 1 {
		t.Fatal("expected the filter to remain applied after enter")
	}

	// First esc (not editing, query set) clears the filter but stays on this view.
	updated, _ = m.updateCreateSelect(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.flatBundleFilter.input.Value() != "" || len(m.flatBundles) != 2 {
		t.Fatal("expected the first esc to clear the filter and restore the full list")
	}
	if m.viewState != ViewCreateSelect {
		t.Fatalf("expected the first esc to stay on ViewCreateSelect, got %v", m.viewState)
	}

	// Second esc (no filter active) goes back to the overview.
	updated, _ = m.updateCreateSelect(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.viewState != ViewOverview {
		t.Fatalf("expected the second esc to return to ViewOverview, got %v", m.viewState)
	}
}
```

Add `tea "github.com/charmbracelet/bubbletea"` to this test file's import block (it wasn't needed by Task 2's tests).

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./commands/ui/... -run 'TestFlatBundleMatchesFilter|TestUpdateCreateSelectFilter' -v`
Expected: build FAILURE — `undefined: flatBundleMatchesFilter` / `undefined: newFlatFilterState` / `m.allFlatBundles` (unknown field)

- [ ] **Step 3: Add `flatFilterState`, `flatBundleMatchesFilter`, `applyFlatBundleFilter`**

In `commands/ui/view_create_select.go`, add `"github.com/charmbracelet/bubbles/textinput"` to the import block:

```go
import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/generate/resolve"
	"github.com/terramate-io/terramate/hcl"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/terramate-io/terramate/scaffold/manifest"
	"github.com/terramate-io/terramate/stdlib"
	"github.com/terramate-io/terramate/typeschema"
	"github.com/zclconf/go-cty/cty"
)
```

Then, right after `buildFlatBundles` (before `func (m Model) updateCreateSelect`), add:

```go
// flatFilterState holds the free-text filter editing state for the flat
// (Scaffold/Create) bundle list.
type flatFilterState struct {
	input   textinput.Model
	editing bool
}

// newFlatFilterState creates a fresh, unfocused filter input.
func newFlatFilterState() flatFilterState {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 128
	return flatFilterState{input: ti}
}

// flatBundleMatchesFilter reports whether entry's bundle name contains query
// (case-insensitive). An empty query matches everything.
func flatBundleMatchesFilter(entry flatBundleEntry, query string) bool {
	if query == "" {
		return true
	}
	return strings.Contains(strings.ToLower(entry.bundle.Name), strings.ToLower(query))
}

// applyFlatBundleFilter recomputes m.flatBundles from m.allFlatBundles using
// the current filter query, and resets the cursor.
func (m *Model) applyFlatBundleFilter() {
	query := strings.TrimSpace(m.flatBundleFilter.input.Value())
	m.flatBundles = nil
	for _, entry := range m.allFlatBundles {
		if flatBundleMatchesFilter(entry, query) {
			m.flatBundles = append(m.flatBundles, entry)
		}
	}
	m.flatBundleCursor = 0
	m.bundleSelectErr = ""
}
```

- [ ] **Step 4: Add `allFlatBundles` and `flatBundleFilter` to `Model`**

In `commands/ui/model.go`, find:

```go
	// Bundle selection state (flat list)
	flatBundles            []flatBundleEntry
	flatBundleCursor       int
	selectedCollIdx        int // Set by selectFlatBundle, used by loadBundleDef
```

Replace with:

```go
	// Bundle selection state (flat list)
	allFlatBundles         []flatBundleEntry // Unfiltered master list, rebuilt each time Scaffold is entered
	flatBundles            []flatBundleEntry // Filtered view of allFlatBundles for the current filter query
	flatBundleFilter       flatFilterState   // Free-text filter state for the flat bundle list
	flatBundleCursor       int
	selectedCollIdx        int // Set by selectFlatBundle, used by loadBundleDef
```

- [ ] **Step 5: Populate the filter state when entering Scaffold**

In `commands/ui/view_overview.go`, find:

```go
	case "Scaffold":
		if len(est.Collections) == 0 {
			m.currentErr = errors.E("No collections available. Configure package sources to get started.")
			return
		}
		m.flatBundles = buildFlatBundles(est)
		if len(m.flatBundles) == 0 {
			m.currentErr = errors.E("No bundles available.")
			return
		}
		m.viewState = ViewCreateSelect
		m.flatBundleCursor = 0
		m.bundleSelectErr = ""
```

Replace with:

```go
	case "Scaffold":
		if len(est.Collections) == 0 {
			m.currentErr = errors.E("No collections available. Configure package sources to get started.")
			return
		}
		m.allFlatBundles = buildFlatBundles(est)
		if len(m.allFlatBundles) == 0 {
			m.currentErr = errors.E("No bundles available.")
			return
		}
		m.flatBundleFilter = newFlatFilterState()
		m.applyFlatBundleFilter()
		m.viewState = ViewCreateSelect
```

- [ ] **Step 6: Wire filter-mode key handling and `/` activation into `updateCreateSelect`**

Find the full `updateCreateSelect` function (after Task 2's changes):

```go
func (m Model) updateCreateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		m.viewState = ViewOverview
		return m, nil

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
```

Replace with:

```go
func (m Model) updateCreateSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.flatBundleFilter.editing {
		switch {
		case key.Matches(msg, keys.Escape):
			m.flatBundleFilter.input.SetValue("")
			m.flatBundleFilter.input.Blur()
			m.flatBundleFilter.editing = false
			m.applyFlatBundleFilter()
			return m, nil
		case key.Matches(msg, keys.Enter):
			m.flatBundleFilter.input.Blur()
			m.flatBundleFilter.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.flatBundleFilter.input, cmd = m.flatBundleFilter.input.Update(msg)
		m.applyFlatBundleFilter()
		return m, cmd
	}

	switch {
	case key.Matches(msg, keys.Escape):
		if m.flatBundleFilter.input.Value() != "" {
			m.flatBundleFilter.input.SetValue("")
			m.applyFlatBundleFilter()
			return m, nil
		}
		m.viewState = ViewOverview
		return m, nil

	case msg.String() == "/":
		m.flatBundleFilter.editing = true
		m.flatBundleFilter.input.Width = m.effectiveWidth() - 8
		m.flatBundleFilter.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
```

- [ ] **Step 7: Show the filter row and update help text**

In `commands/ui/view_create_select.go`, find in `renderBundleSelectView`:

```go
	help := helpStyle.Render(m.finalHelpText("esc: back"))
```

Replace with:

```go
	help := helpStyle.Render(m.finalHelpText(m.flatBundleFilterHelp()))
```

Add the helper right after `renderBundleSelectView`'s closing `}`:

```go
// flatBundleFilterHelp returns the help-line hint reflecting the current
// filter state of the flat bundle list.
func (m Model) flatBundleFilterHelp() string {
	switch {
	case m.flatBundleFilter.editing:
		return "esc: clear • enter: apply"
	case m.flatBundleFilter.input.Value() != "":
		return "/: edit filter • esc: back"
	default:
		return "/: filter • esc: back"
	}
}
```

Then, in `flatBundleListHeader` (added in Task 2), find:

```go
	headerParts := []string{detailBox}
	if m.bundleSelectErr != "" {
		headerParts = append(headerParts, renderErrorBox(innerWidth, m.bundleSelectErr))
	}
	headerParts = append(headerParts, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}
```

Replace with:

```go
	var headerParts []string
	if m.flatBundleFilter.editing || m.flatBundleFilter.input.Value() != "" {
		filterStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
		headerParts = append(headerParts, filterStyle.Render(m.flatBundleFilter.input.View()), "")
	}
	headerParts = append(headerParts, detailBox)
	if m.bundleSelectErr != "" {
		headerParts = append(headerParts, renderErrorBox(innerWidth, m.bundleSelectErr))
	}
	headerParts = append(headerParts, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestFlatBundleMatchesFilter|TestUpdateCreateSelectFilter' -v`
Expected: PASS (all subtests)

- [ ] **Step 9: Build and run the full package test suite**

Run: `go build ./commands/ui/... && go test ./commands/ui/... -v`
Expected: PASS, no build errors

- [ ] **Step 10: Commit**

```bash
git add commands/ui/view_create_select.go commands/ui/model.go commands/ui/view_overview.go commands/ui/view_create_select_test.go
git commit -m "feat(ui): add free-text filter to the Scaffold bundle list"
```

---

### Task 6: Reconfigure bundle list free-text filter

**Files:**
- Modify: `commands/ui/view_reconfig.go` (add `textinput` import)
- Modify: `commands/ui/model.go`
- Modify: `commands/ui/view_overview.go` (`executeCommand`'s `"Reconfigure"` case)
- Test: `commands/ui/view_reconfig_test.go` (append to Task 3's file)

**Interfaces:**
- Consumes: `buildReconfigBundles`, `applyReconfigFilter`, `currentReconfigFilter` (pre-existing), `displayNameFromAlias` (pre-existing).
- Produces: `reconfigFilterState`, `newReconfigFilterState()`, `reconfigBundleMatchesFilter(b *config.Bundle, query string) bool`, `Model.reconfigFilter`.

- [ ] **Step 1: Write the failing tests**

Append to `commands/ui/view_reconfig_test.go`:

```go
func TestReconfigBundleMatchesFilter(t *testing.T) {
	t.Parallel()
	b := &config.Bundle{
		DefinitionMetadata: config.Metadata{Name: "VPC-Network"},
		Alias:              "prod-vpc-1",
		Name:               "vpc",
	}

	testcases := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "empty query matches everything", query: "", want: true},
		{name: "matches definition name", query: "network", want: true},
		{name: "matches instance alias case-insensitively", query: "PROD-VPC", want: true},
		{name: "no match", query: "ecs", want: false},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := reconfigBundleMatchesFilter(b, tc.query); got != tc.want {
				t.Fatalf("reconfigBundleMatchesFilter(query=%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestBuildReconfigBundlesCombinesEnvAndTextFilter(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	prod := &config.Environment{ID: "prod", Name: "Production"}
	bundles := []*config.Bundle{
		{DefinitionMetadata: config.Metadata{Name: "vpc"}, Alias: "vpc-1", Environment: staging},
		{DefinitionMetadata: config.Metadata{Name: "ecs"}, Alias: "ecs-1", Environment: staging},
		{DefinitionMetadata: config.Metadata{Name: "vpc"}, Alias: "vpc-2", Environment: prod},
	}

	m := Model{
		EngineState:       &EngineState{Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		reconfigFilters:   []envFilterState{{env: staging, label: "Staging", shortID: "staging"}},
		reconfigFilterPos: 0,
		reconfigFilter:    newReconfigFilterState(),
	}
	m.reconfigFilter.input.SetValue("vpc")

	got := m.buildReconfigBundles()
	if len(got) != 1 || got[0].Alias != "vpc-1" {
		t.Fatalf("expected the env+text filter to narrow to [vpc-1], got %v", got)
	}
}

func TestUpdateReconfigSelectFilter(t *testing.T) {
	t.Parallel()

	bundles := []*config.Bundle{
		{DefinitionMetadata: config.Metadata{Name: "vpc", Version: "1.0.0"}, Alias: "prod-vpc-1"},
		{DefinitionMetadata: config.Metadata{Name: "ecs", Version: "1.0.0"}, Alias: "prod-ecs-1"},
	}
	m := Model{
		EngineState:       &EngineState{Registry: &config.Registry{Bundles: bundles}},
		viewState:         ViewReconfigSelect,
		reconfigFilterPos: -1,
		reconfigFilter:    newReconfigFilterState(),
	}
	m.reconfigBundles = m.buildReconfigBundles()

	// "/" enters filter-edit mode.
	updated, _ := m.updateReconfigSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(Model)
	if !m.reconfigFilter.editing {
		t.Fatal("expected filter mode to be active after '/'")
	}

	// Typing narrows the list live. Sending all 3 runes in one KeyMsg is a
	// test simplification — textinput.Update appends the whole Runes slice
	// in one call, which is equivalent to 3 separate keystrokes here.
	updated, _ = m.updateReconfigSelect(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("vpc")})
	m = updated.(Model)
	if len(m.reconfigBundles) != 1 || m.reconfigBundles[0].Alias != "prod-vpc-1" {
		t.Fatalf("expected the filter to narrow to [prod-vpc-1], got %v", m.reconfigBundles)
	}

	// esc clears the filter and stays on this view.
	updated, _ = m.updateReconfigSelect(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if len(m.reconfigBundles) != 2 {
		t.Fatal("expected esc to clear the filter and restore both bundles")
	}
	if m.viewState != ViewReconfigSelect {
		t.Fatalf("expected esc to stay on ViewReconfigSelect, got %v", m.viewState)
	}
}
```

Add `"github.com/terramate-io/terramate/config"` and `tea "github.com/charmbracelet/bubbletea"` to this test file's import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./commands/ui/... -run 'TestReconfigBundleMatchesFilter|TestBuildReconfigBundlesCombinesEnvAndTextFilter|TestUpdateReconfigSelectFilter' -v`
Expected: build FAILURE — `undefined: reconfigBundleMatchesFilter` / `undefined: newReconfigFilterState` / `m.reconfigFilter` (unknown field)

- [ ] **Step 3: Add `reconfigFilterState`, `reconfigBundleMatchesFilter`; wire into `buildReconfigBundles`**

In `commands/ui/view_reconfig.go`, add `"github.com/charmbracelet/bubbles/textinput"` to the import block:

```go
import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/typeschema"
)
```

Right after `applyReconfigFilter` (before `loadReconfigBundle`), add:

```go
// reconfigFilterState holds the free-text filter editing state for the
// Reconfigure bundle list.
type reconfigFilterState struct {
	input   textinput.Model
	editing bool
}

// newReconfigFilterState creates a fresh, unfocused filter input.
func newReconfigFilterState() reconfigFilterState {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 128
	return reconfigFilterState{input: ti}
}

// reconfigBundleMatchesFilter reports whether b's definition name or
// instance alias contains query (case-insensitive). An empty query matches
// everything.
func reconfigBundleMatchesFilter(b *config.Bundle, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(b.DefinitionMetadata.Name), q) {
		return true
	}
	return strings.Contains(strings.ToLower(displayNameFromAlias(b.Alias, b.Name)), q)
}
```

Now find `buildReconfigBundles`:

```go
func (m Model) buildReconfigBundles() []*config.Bundle {
	f := m.currentReconfigFilter()
	var filtered []*config.Bundle
	for _, b := range m.EngineState.Registry.Bundles {
		if f != nil {
			if f.envLess {
				if b.Environment != nil {
					continue
				}
			} else if b.Environment == nil || b.Environment.ID != f.env.ID {
				continue
			}
		}
		filtered = append(filtered, b)
	}
```

Replace with:

```go
func (m Model) buildReconfigBundles() []*config.Bundle {
	f := m.currentReconfigFilter()
	query := strings.TrimSpace(m.reconfigFilter.input.Value())
	var filtered []*config.Bundle
	for _, b := range m.EngineState.Registry.Bundles {
		if f != nil {
			if f.envLess {
				if b.Environment != nil {
					continue
				}
			} else if b.Environment == nil || b.Environment.ID != f.env.ID {
				continue
			}
		}
		if !reconfigBundleMatchesFilter(b, query) {
			continue
		}
		filtered = append(filtered, b)
	}
```

(`applyReconfigFilter` and grouping below are unchanged — `applyReconfigFilter` already just calls `buildReconfigBundles()` and resets the cursor, so it picks up the text filter automatically. Groups with no matching instances simply won't appear, since `groupBundles` only groups the bundles present in its filtered input — no separate "hide empty groups" code is needed.)

- [ ] **Step 4: Add `reconfigFilter` to `Model`**

In `commands/ui/model.go`, find:

```go
	// Reconfigure state
	reconfigBundles      []*config.Bundle // Filtered bundles for current filter, rebuilt on filter change
	reconfigCursor       int              // Cursor in reconfigBundles
	reconfigBundle       *config.Bundle   // The bundle currently being reconfigured
	reconfigFromOverview bool             // true when reconfig was entered from session panel (skip ViewReconfigSelect on ESC)
	reconfigFilters      []envFilterState // Precomputed valid filter states
	reconfigFilterPos    int              // Current position in reconfigFilters (-1 = all/no filter)
```

Replace with:

```go
	// Reconfigure state
	reconfigBundles      []*config.Bundle // Filtered bundles for current filter, rebuilt on filter change
	reconfigCursor       int              // Cursor in reconfigBundles
	reconfigBundle       *config.Bundle   // The bundle currently being reconfigured
	reconfigFromOverview bool             // true when reconfig was entered from session panel (skip ViewReconfigSelect on ESC)
	reconfigFilters      []envFilterState // Precomputed valid filter states
	reconfigFilterPos    int              // Current position in reconfigFilters (-1 = all/no filter)
	reconfigFilter       reconfigFilterState // Free-text filter state for the Reconfigure bundle list
```

- [ ] **Step 5: Reset the filter state when entering Reconfigure**

In `commands/ui/view_overview.go`, find:

```go
	case "Reconfigure":
		m.reconfigFilterPos = -1
		m.reconfigFilters = m.buildReconfigFilters()
		m.reconfigBundles = m.buildReconfigBundles()
```

Replace with:

```go
	case "Reconfigure":
		m.reconfigFilterPos = -1
		m.reconfigFilter = newReconfigFilterState()
		m.reconfigFilters = m.buildReconfigFilters()
		m.reconfigBundles = m.buildReconfigBundles()
```

- [ ] **Step 6: Wire filter-mode key handling, `/` activation, and Escape priority into `updateReconfigSelect`**

Find the full `updateReconfigSelect` function:

```go
func (m Model) updateReconfigSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		if m.reconfigFilterPos >= 0 {
			m.reconfigFilterPos = -1
			m.applyReconfigFilter()
			return m, nil
		}
		m.viewState = ViewOverview
		return m, nil

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
```

Replace with:

```go
func (m Model) updateReconfigSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.reconfigFilter.editing {
		switch {
		case key.Matches(msg, keys.Escape):
			m.reconfigFilter.input.SetValue("")
			m.reconfigFilter.input.Blur()
			m.reconfigFilter.editing = false
			m.applyReconfigFilter()
			return m, nil
		case key.Matches(msg, keys.Enter):
			m.reconfigFilter.input.Blur()
			m.reconfigFilter.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.reconfigFilter.input, cmd = m.reconfigFilter.input.Update(msg)
		m.applyReconfigFilter()
		return m, cmd
	}

	switch {
	case key.Matches(msg, keys.Escape):
		if m.reconfigFilter.input.Value() != "" {
			m.reconfigFilter.input.SetValue("")
			m.applyReconfigFilter()
			return m, nil
		}
		if m.reconfigFilterPos >= 0 {
			m.reconfigFilterPos = -1
			m.applyReconfigFilter()
			return m, nil
		}
		m.viewState = ViewOverview
		return m, nil

	case msg.String() == "/":
		m.reconfigFilter.editing = true
		m.reconfigFilter.input.Width = m.effectiveWidth() - 8
		m.reconfigFilter.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
```

- [ ] **Step 7: Show the filter row, breadcrumb, and help text**

In `commands/ui/view_reconfig.go`, find in `renderReconfigSelectView`:

```go
	breadcrumb := "Reconfigure Bundle Instance"
	if f := m.currentReconfigFilter(); f != nil {
		if f.envLess {
			breadcrumb = "Reconfigure Bundle Instance Without Environment"
		} else {
			breadcrumb = "Reconfigure Bundle Instance in " + f.label
		}
	}
	title := m.renderHeader(breadcrumb)
```

Replace with:

```go
	breadcrumb := "Reconfigure Bundle Instance"
	if f := m.currentReconfigFilter(); f != nil {
		if f.envLess {
			breadcrumb = "Reconfigure Bundle Instance Without Environment"
		} else {
			breadcrumb = "Reconfigure Bundle Instance in " + f.label
		}
	}
	if query := m.reconfigFilter.input.Value(); query != "" {
		breadcrumb += fmt.Sprintf(" — filter: %q", query)
	}
	title := m.renderHeader(breadcrumb)
```

Find:

```go
	escLabel := "esc: back"
	if m.reconfigFilterPos >= 0 {
		escLabel = "esc: reset filter"
	}
	helpParts := escLabel
	if len(m.reconfigFilters) > 0 {
		helpParts += " • e: show only " + m.nextReconfigFilterName()
	}
	help := helpStyle.Render(m.finalHelpText(helpParts))
```

Replace with:

```go
	escLabel := "esc: back"
	if m.reconfigFilterPos >= 0 {
		escLabel = "esc: reset filter"
	}
	if m.reconfigFilter.input.Value() != "" {
		escLabel = "esc: clear filter"
	}
	helpParts := escLabel
	if len(m.reconfigFilters) > 0 {
		helpParts += " • e: show only " + m.nextReconfigFilterName()
	}
	switch {
	case m.reconfigFilter.editing:
		helpParts = "esc: clear • enter: apply"
	case m.reconfigFilter.input.Value() == "":
		helpParts += " • /: filter"
	default:
		helpParts += " • /: edit filter"
	}
	help := helpStyle.Render(m.finalHelpText(helpParts))
```

Then, in `reconfigListHeader` (added in Task 3), find:

```go
	return lipgloss.JoinVertical(lipgloss.Left, detailBox, "")
}
```

Replace with:

```go
	var headerParts []string
	if m.reconfigFilter.editing || m.reconfigFilter.input.Value() != "" {
		filterStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
		headerParts = append(headerParts, filterStyle.Render(m.reconfigFilter.input.View()), "")
	}
	headerParts = append(headerParts, detailBox, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestReconfigBundleMatchesFilter|TestBuildReconfigBundlesCombinesEnvAndTextFilter|TestUpdateReconfigSelectFilter' -v`
Expected: PASS (all subtests)

- [ ] **Step 9: Build and run the full package test suite**

Run: `go build ./commands/ui/... && go test ./commands/ui/... -v`
Expected: PASS, no build errors

- [ ] **Step 10: Commit**

```bash
git add commands/ui/view_reconfig.go commands/ui/model.go commands/ui/view_overview.go commands/ui/view_reconfig_test.go
git commit -m "feat(ui): add free-text filter to the Reconfigure bundle list"
```

---

### Task 7: Promote bundle list free-text filter

**Files:**
- Modify: `commands/ui/view_promote.go` (add `textinput` and `fmt` imports)
- Modify: `commands/ui/model.go`
- Modify: `commands/ui/view_overview.go` (`executeCommand`'s `"Promote"` case)
- Test: `commands/ui/view_promote_test.go` (append to Task 4's file)

**Interfaces:**
- Consumes: `buildAllPromoteBundles`, `applyPromoteFilter`, `currentPromoteFilter` (pre-existing), `displayNameFromAlias` (pre-existing).
- Produces: `promoteFilterState`, `newPromoteFilterState()`, `promoteBundleMatchesFilter(b *config.Bundle, query string) bool`, `Model.promoteFilter`.

- [ ] **Step 1: Write the failing tests**

Append to `commands/ui/view_promote_test.go`:

```go
func TestPromoteBundleMatchesFilter(t *testing.T) {
	t.Parallel()
	b := &config.Bundle{
		DefinitionMetadata: config.Metadata{Name: "VPC-Network"},
		Alias:              "prod-vpc-1",
		Name:               "vpc",
	}

	testcases := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "empty query matches everything", query: "", want: true},
		{name: "matches definition name", query: "network", want: true},
		{name: "matches instance alias case-insensitively", query: "PROD-VPC", want: true},
		{name: "no match", query: "ecs", want: false},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := promoteBundleMatchesFilter(b, tc.query); got != tc.want {
				t.Fatalf("promoteBundleMatchesFilter(query=%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestBuildAllPromoteBundlesCombinesEnvAndTextFilter(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	prod := &config.Environment{ID: "prod", Name: "Production", PromoteFrom: "staging"}
	bundles := []*config.Bundle{
		{DefinitionMetadata: config.Metadata{Name: "vpc"}, Alias: "vpc-1", Environment: staging},
		{DefinitionMetadata: config.Metadata{Name: "ecs"}, Alias: "ecs-1", Environment: staging},
	}

	m := Model{
		EngineState: &EngineState{Registry: &config.Registry{
			Bundles:      bundles,
			Environments: []*config.Environment{staging, prod},
		}},
		promoteFilter: newPromoteFilterState(),
	}
	m.promoteFilter.input.SetValue("vpc")

	got, targetEnvs := m.buildAllPromoteBundles()
	if len(got) != 1 || got[0].Alias != "vpc-1" {
		t.Fatalf("expected the text filter to narrow to [vpc-1], got %v", got)
	}
	if len(targetEnvs) != 1 || targetEnvs[0].ID != "prod" {
		t.Fatalf("expected the target environment to be prod, got %v", targetEnvs)
	}
}
```

Add `"github.com/terramate-io/terramate/config"` to this test file's import block.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./commands/ui/... -run 'TestPromoteBundleMatchesFilter|TestBuildAllPromoteBundlesCombinesEnvAndTextFilter' -v`
Expected: build FAILURE — `undefined: promoteBundleMatchesFilter` / `undefined: newPromoteFilterState`

- [ ] **Step 3: Add `promoteFilterState`, `promoteBundleMatchesFilter`; wire into `buildAllPromoteBundles`**

In `commands/ui/view_promote.go`, find:

```go
import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/project"
)
```

Replace with:

```go
import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/project"
)
```

Right after `applyPromoteFilter` (before `loadPromoteBundle`), add:

```go
// promoteFilterState holds the free-text filter editing state for the
// Promote bundle list.
type promoteFilterState struct {
	input   textinput.Model
	editing bool
}

// newPromoteFilterState creates a fresh, unfocused filter input.
func newPromoteFilterState() promoteFilterState {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 128
	return promoteFilterState{input: ti}
}

// promoteBundleMatchesFilter reports whether b's definition name or
// instance alias contains query (case-insensitive). An empty query matches
// everything.
func promoteBundleMatchesFilter(b *config.Bundle, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(b.DefinitionMetadata.Name), q) {
		return true
	}
	return strings.Contains(strings.ToLower(displayNameFromAlias(b.Alias, b.Name)), q)
}
```

Now find, inside `buildAllPromoteBundles`:

```go
		existing := envAliases[targetEnv.ID]
		for _, b := range est.Registry.Bundles {
			if b.Environment == nil || b.Environment.ID != targetEnv.PromoteFrom {
				continue
			}
			if existing[b.Alias] {
				continue
			}
			if len(missingBundleRefs(b, existing)) > 0 {
				continue
			}
			bundles = append(bundles, b)
			targetEnvs = append(targetEnvs, targetEnv)
		}
```

Replace with:

```go
		existing := envAliases[targetEnv.ID]
		query := strings.TrimSpace(m.promoteFilter.input.Value())
		for _, b := range est.Registry.Bundles {
			if b.Environment == nil || b.Environment.ID != targetEnv.PromoteFrom {
				continue
			}
			if existing[b.Alias] {
				continue
			}
			if len(missingBundleRefs(b, existing)) > 0 {
				continue
			}
			if !promoteBundleMatchesFilter(b, query) {
				continue
			}
			bundles = append(bundles, b)
			targetEnvs = append(targetEnvs, targetEnv)
		}
```

(`applyPromoteFilter` is unchanged — it already just calls `buildAllPromoteBundles()` and resets the cursor.)

- [ ] **Step 4: Add `promoteFilter` to `Model`**

In `commands/ui/model.go`, find:

```go
	// Promote state
	promoteBundles    []*config.Bundle      // Filtered bundles for current filter
	promoteTargetEnvs []*config.Environment // Target env per bundle (parallel to promoteBundles)
	promoteCursor     int                   // Cursor in promoteBundles
	promoteBundle     *config.Bundle        // The bundle currently being promoted
	promoteFilters    []envFilterState      // Precomputed valid filter states
	promoteFilterPos  int                   // Current position in promoteFilters (-1 = all/no filter)
```

Replace with:

```go
	// Promote state
	promoteBundles    []*config.Bundle      // Filtered bundles for current filter
	promoteTargetEnvs []*config.Environment // Target env per bundle (parallel to promoteBundles)
	promoteCursor     int                   // Cursor in promoteBundles
	promoteBundle     *config.Bundle        // The bundle currently being promoted
	promoteFilters    []envFilterState      // Precomputed valid filter states
	promoteFilterPos  int                   // Current position in promoteFilters (-1 = all/no filter)
	promoteFilter     promoteFilterState    // Free-text filter state for the Promote bundle list
```

- [ ] **Step 5: Reset the filter state when entering Promote**

In `commands/ui/view_overview.go`, find:

```go
	case "Promote":
		m.promoteFilterPos = -1
		m.promoteFilters = m.buildPromoteFilters()
		m.promoteBundles, m.promoteTargetEnvs = m.buildAllPromoteBundles()
```

Replace with:

```go
	case "Promote":
		m.promoteFilterPos = -1
		m.promoteFilter = newPromoteFilterState()
		m.promoteFilters = m.buildPromoteFilters()
		m.promoteBundles, m.promoteTargetEnvs = m.buildAllPromoteBundles()
```

- [ ] **Step 6: Wire filter-mode key handling, `/` activation, and Escape priority into `updatePromoteSelect`**

Find the full `updatePromoteSelect` function:

```go
func (m Model) updatePromoteSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, keys.Escape):
		if m.promoteFilterPos >= 0 {
			m.promoteFilterPos = -1
			m.applyPromoteFilter()
			return m, nil
		}
		m.viewState = ViewOverview
		return m, nil

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
```

Replace with:

```go
func (m Model) updatePromoteSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.promoteFilter.editing {
		switch {
		case key.Matches(msg, keys.Escape):
			m.promoteFilter.input.SetValue("")
			m.promoteFilter.input.Blur()
			m.promoteFilter.editing = false
			m.applyPromoteFilter()
			return m, nil
		case key.Matches(msg, keys.Enter):
			m.promoteFilter.input.Blur()
			m.promoteFilter.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.promoteFilter.input, cmd = m.promoteFilter.input.Update(msg)
		m.applyPromoteFilter()
		return m, cmd
	}

	switch {
	case key.Matches(msg, keys.Escape):
		if m.promoteFilter.input.Value() != "" {
			m.promoteFilter.input.SetValue("")
			m.applyPromoteFilter()
			return m, nil
		}
		if m.promoteFilterPos >= 0 {
			m.promoteFilterPos = -1
			m.applyPromoteFilter()
			return m, nil
		}
		m.viewState = ViewOverview
		return m, nil

	case msg.String() == "/":
		m.promoteFilter.editing = true
		m.promoteFilter.input.Width = m.effectiveWidth() - 8
		m.promoteFilter.input.Focus()
		return m, textinput.Blink

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
```

- [ ] **Step 7: Show the filter row, breadcrumb, and help text**

In `commands/ui/view_promote.go`, find in `renderPromoteSelectView`:

```go
	breadcrumb := "Promote Bundle Instance"
	if f := m.currentPromoteFilter(); f != nil {
		breadcrumb = "Promote Bundle Instance to " + f.label
	}
	title := m.renderHeader(breadcrumb)
```

Replace with:

```go
	breadcrumb := "Promote Bundle Instance"
	if f := m.currentPromoteFilter(); f != nil {
		breadcrumb = "Promote Bundle Instance to " + f.label
	}
	if query := m.promoteFilter.input.Value(); query != "" {
		breadcrumb += fmt.Sprintf(" — filter: %q", query)
	}
	title := m.renderHeader(breadcrumb)
```

Find:

```go
	escLabel := "esc: back"
	if m.promoteFilterPos >= 0 {
		escLabel = "esc: reset filter"
	}
	helpParts := escLabel
	if len(m.promoteFilters) > 0 {
		helpParts += " • e: show target env " + m.nextPromoteFilterName()
	}
	help := helpStyle.Render(m.finalHelpText(helpParts))
```

Replace with:

```go
	escLabel := "esc: back"
	if m.promoteFilterPos >= 0 {
		escLabel = "esc: reset filter"
	}
	if m.promoteFilter.input.Value() != "" {
		escLabel = "esc: clear filter"
	}
	helpParts := escLabel
	if len(m.promoteFilters) > 0 {
		helpParts += " • e: show target env " + m.nextPromoteFilterName()
	}
	switch {
	case m.promoteFilter.editing:
		helpParts = "esc: clear • enter: apply"
	case m.promoteFilter.input.Value() == "":
		helpParts += " • /: filter"
	default:
		helpParts += " • /: edit filter"
	}
	help := helpStyle.Render(m.finalHelpText(helpParts))
```

Then, in `promoteListHeader` (added in Task 4), find:

```go
	return lipgloss.JoinVertical(lipgloss.Left, detailBox, "")
}
```

Replace with:

```go
	var headerParts []string
	if m.promoteFilter.editing || m.promoteFilter.input.Value() != "" {
		filterStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
		headerParts = append(headerParts, filterStyle.Render(m.promoteFilter.input.View()), "")
	}
	headerParts = append(headerParts, detailBox, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}
```

- [ ] **Step 8: Run the tests to verify they pass**

Run: `go test ./commands/ui/... -run 'TestPromoteBundleMatchesFilter|TestBuildAllPromoteBundlesCombinesEnvAndTextFilter' -v`
Expected: PASS (all subtests)

- [ ] **Step 9: Build and run the full package test suite**

Run: `go build ./commands/ui/... && go test ./commands/ui/... -v`
Expected: PASS, no build errors

- [ ] **Step 10: Commit**

```bash
git add commands/ui/view_promote.go commands/ui/model.go commands/ui/view_overview.go commands/ui/view_promote_test.go
git commit -m "feat(ui): add free-text filter to the Promote bundle list"
```

---

### Task 8: Full verification and manual smoke test

**Files:**
- None modified — this task verifies Tasks 1-7 together.

**Interfaces:**
- Consumes: everything produced by Tasks 1-7.
- Produces: nothing new; final gate before considering the feature done.

- [ ] **Step 1: Format and vet the whole module**

Run: `make fmt && go vet ./...`
Expected: `make fmt` reports no changes needed (or auto-fixes are clean to review); `go vet` produces no output.

- [ ] **Step 2: Run the full `commands/ui` test suite with the race detector**

Run: `go test -race -count=1 ./commands/ui/... -v`
Expected: PASS, every test from Tasks 1-7 listed and green.

- [ ] **Step 3: Build both binaries**

Run: `make build`
Expected: `bin/terramate` and `bin/terramate-ls` build successfully.

- [ ] **Step 4: Run lint**

Run: `make lint/all`
Expected: no new findings introduced by `commands/ui/*.go`. (If `golangci-lint` isn't installed, run `make lint/install` first per `CLAUDE.md`.)

- [ ] **Step 5: Manual smoke test (human or agent with a real TTY)**

This is a `bubbletea` full-screen TUI — it cannot be driven headlessly by an automated test in this repo. A human (or an agent with terminal/PTY access) must verify by hand, in a project directory with several dozen+ bundles configured:

1. Run `./bin/terramate ui`, choose **Scaffold**.
2. Confirm `PgDn`/`PgUp` jump the cursor by roughly a screenful and the detail box updates to match the new selection.
3. Press `/`, type a partial bundle name, confirm the list narrows on every keystroke and the cursor is on the first match.
4. Press `Enter`, confirm the filter stays applied and `Up`/`Down`/`PgUp`/`PgDn` still navigate the narrowed list.
5. Press `Esc` once — filter clears, full list returns. Press `Esc` again — returns to the Overview.
6. Repeat steps 2-5 for **Reconfigure** and **Promote**, and additionally verify: pressing `e` to cycle the environment filter still works, and combining it with `/` narrows further (not replaces).
7. Confirm a group in Reconfigure/Promote fully disappears from the list when none of its instances match the current text filter, and reappears when the filter is cleared.

Record the outcome in the PR description or commit message body — this plan cannot claim the feature works end-to-end without this step having been performed.

- [ ] **Step 6: Final commit (only if Steps 1-5 turned up fixes)**

If any step above required a code change, stage and commit it with a clear message before moving on; otherwise there is nothing to commit for this task.
