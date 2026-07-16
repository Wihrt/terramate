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
