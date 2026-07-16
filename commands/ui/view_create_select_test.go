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
