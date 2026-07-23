// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import "testing"

// groupedTestItems builds a synthetic 2-group, 6-instance rendered item list
// matching the shape renderGroupedBundleItems / renderPromoteGroupedItems
// produce: each group is [blank?, header, instance...], all height 1.
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
func groupedTestItems() []renderedItem {
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

func TestPageCursor(t *testing.T) {
	t.Parallel()

	flatItems := make([]renderedItem, 20)
	for i := range flatItems {
		flatItems[i] = renderedItem{content: "x", height: 1, selectable: true}
	}
	groupedItems := groupedTestItems()

	testcases := []struct {
		name            string
		items           []renderedItem
		cursor          int
		availableHeight int
		sep             int
		down            bool
		want            int
	}{
		// From TestFlatBundlePageCursor (view_create_select_test.go).
		{name: "page down from top", items: flatItems, cursor: 0, availableHeight: 5, sep: 1, down: true, want: 3},
		{name: "page down near end clamps to last", items: flatItems, cursor: 17, availableHeight: 5, sep: 1, down: true, want: 19},
		{name: "page up from middle", items: flatItems, cursor: 10, availableHeight: 5, sep: 1, down: false, want: 8},
		{name: "page up clamps to first", items: flatItems, cursor: 2, availableHeight: 5, sep: 1, down: false, want: 0},
		{name: "page larger than whole list clamps down to last", items: flatItems, cursor: 3, availableHeight: 100, sep: 1, down: true, want: 19},
		{name: "page larger than whole list clamps up to first", items: flatItems, cursor: 3, availableHeight: 100, sep: 1, down: false, want: 0},
		// From TestPromotePageCursor / TestReconfigPageCursor (identical
		// cases in both view_promote_test.go and view_reconfig_test.go),
		// deduplicated to a single copy.
		{name: "page down skips the group boundary, lands on first bundle of next group", items: groupedItems, cursor: 1, availableHeight: 4, sep: 0, down: true, want: 6},
		{name: "page up skips the group boundary, lands on last bundle of previous group", items: groupedItems, cursor: 7, availableHeight: 4, sep: 0, down: false, want: 3},
		{name: "page down at the last item clamps", items: groupedItems, cursor: 8, availableHeight: 4, sep: 0, down: true, want: 8},
		{name: "page up at the first item clamps", items: groupedItems, cursor: 1, availableHeight: 4, sep: 0, down: false, want: 1},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := pageCursor(tc.items, tc.cursor, tc.availableHeight, tc.sep, tc.down)
			if got != tc.want {
				t.Fatalf("pageCursor(cursor=%d, height=%d, down=%v) = %d, want %d",
					tc.cursor, tc.availableHeight, tc.down, got, tc.want)
			}
		})
	}
}

func TestPageCursorEmptyList(t *testing.T) {
	t.Parallel()
	if got := pageCursor(nil, 0, 10, 1, true); got != 0 {
		t.Fatalf("expected 0 for an empty list, got %d", got)
	}
}

func TestCursorForItem(t *testing.T) {
	t.Parallel()
	items := groupedTestItems()

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
			if got := cursorForItem(items, tc.itemIdx); got != tc.want {
				t.Fatalf("cursorForItem(%d) = %d, want %d", tc.itemIdx, got, tc.want)
			}
		})
	}
}
