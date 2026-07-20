// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/terramate-io/terramate/config"
)

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
