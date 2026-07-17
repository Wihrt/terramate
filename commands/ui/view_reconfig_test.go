// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/terramate-io/terramate/config"
)

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
