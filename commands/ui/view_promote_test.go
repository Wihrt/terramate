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
