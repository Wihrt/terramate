// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

	"github.com/terramate-io/terramate/commands/ui/change"
	"github.com/terramate-io/terramate/config"
)

// textFilter holds the free-text filter editing state shared by the bundle
// list views (create-select, promote, reconfigure).
type textFilter struct {
	input   textinput.Model
	editing bool
}

// newTextFilter creates a fresh, unfocused filter input.
func newTextFilter() textFilter {
	ti := textinput.New()
	ti.Prompt = "/ "
	ti.CharLimit = 128
	return textFilter{input: ti}
}

// query returns the trimmed current filter text.
func (f textFilter) query() string {
	return strings.TrimSpace(f.input.Value())
}

// envFilterCycle cycles through the precomputed env filter options of a
// view. pos == -1 means "show all" (no env filter active).
type envFilterCycle struct {
	filters []envFilterState
	pos     int
}

// current returns the active filter state, or nil if showing all.
func (c envFilterCycle) current() *envFilterState {
	if c.pos >= 0 && c.pos < len(c.filters) {
		return &c.filters[c.pos]
	}
	return nil
}

// nextName returns the short ID of the next filter in the cycle.
func (c envFilterCycle) nextName() string {
	if len(c.filters) == 0 {
		return ""
	}
	return c.filters[(c.pos+1)%len(c.filters)].shortID
}

// bundleMatchesFilter reports whether b's definition name or instance alias
// contains query (case-insensitive). An empty query matches everything.
func bundleMatchesFilter(b *config.Bundle, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(b.DefinitionMetadata.Name), q) {
		return true
	}
	return strings.Contains(strings.ToLower(change.DisplayNameFromAlias(b.Alias, b.Name)), q)
}
