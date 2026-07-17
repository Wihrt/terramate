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
