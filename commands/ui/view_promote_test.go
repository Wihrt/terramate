// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	"github.com/terramate-io/terramate/config"
)

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
