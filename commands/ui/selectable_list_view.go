// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/terramate-io/terramate/commands/ui/change"
	"github.com/terramate-io/terramate/config"
)

// listViewConfig parameterizes the shared bundle-selection list behavior
// (keyboard handling and rendering) for one view. All callbacks receive the
// in-flight Model copy and mutate it through the pointer.
type listViewConfig struct {
	// breadcrumb returns the title text (may reflect active filters).
	breadcrumb func(m *Model) string
	// helpLine returns the help text (before finalHelpText decoration).
	helpLine func(m *Model) string
	// listHeader renders the block above the list (filter input line,
	// detail box, optional error box). Its lipgloss.Height feeds the
	// PgUp/PgDn page math, exactly as in the original views.
	listHeader func(m *Model, innerWidth int) string
	// buildItems renders the current list as renderedItems and returns the
	// index of the selected item within them.
	buildItems func(m *Model, contentWidth int) (selectedItemIdx int, items []renderedItem)
	// itemCount bounds the cursor (number of selectable entries).
	itemCount func(m *Model) int

	cursor      func(m *Model) int
	setCursor   func(m *Model, c int)
	textFilter  func(m *Model) *textFilter
	applyFilter func(m *Model)
	// envFilter returns nil for views without env cycling (create-select).
	envFilter func(m *Model) *envFilterCycle
	// onCursorMove, when non-nil, runs after any cursor change
	// (create-select clears its inline error there).
	onCursorMove func(m *Model)
	onEnter      func(m *Model) (tea.Model, tea.Cmd)
	// exit is the final Escape stage (all filters already cleared).
	exit func(m *Model)

	// sep is the number of blank lines between items (scroll math + join):
	// 0 for grouped lists, 1 for the flat list.
	sep int
	// grouped: cursor is a bundle-index that must be mapped through
	// cursorForItem after a page jump; flat lists use item indices directly.
	grouped bool
}

// updateSelectView is the shared keyboard handler of the three
// bundle-selection views.
func (m Model) updateSelectView(cfg listViewConfig, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	f := cfg.textFilter(&m)
	if f.editing {
		switch {
		case key.Matches(msg, keys.Escape):
			f.input.SetValue("")
			f.input.Blur()
			f.editing = false
			cfg.applyFilter(&m)
			return m, nil
		case key.Matches(msg, keys.Enter):
			f.input.Blur()
			f.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		f.input, cmd = f.input.Update(msg)
		cfg.applyFilter(&m)
		return m, cmd
	}

	switch {
	case key.Matches(msg, keys.Escape):
		if f.input.Value() != "" {
			f.input.SetValue("")
			cfg.applyFilter(&m)
			return m, nil
		}
		if ef := cfg.envFilter(&m); ef != nil && ef.pos >= 0 {
			ef.pos = -1
			cfg.applyFilter(&m)
			return m, nil
		}
		cfg.exit(&m)
		return m, nil

	case msg.String() == "/":
		f.editing = true
		f.input.Width = m.effectiveWidth() - 8
		f.input.Focus()
		return m, textinput.Blink

	case msg.String() == "e":
		if ef := cfg.envFilter(&m); ef != nil && len(ef.filters) > 0 {
			ef.pos = (ef.pos + 1) % len(ef.filters)
			cfg.applyFilter(&m)
		}
		return m, nil

	case key.Matches(msg, keys.PgUp), key.Matches(msg, keys.PgDn):
		down := key.Matches(msg, keys.PgDn)
		innerWidth := m.effectiveWidth() - 4
		contentWidth := innerWidth - 4 // scrollbarGutter, matches renderSelectView
		availableHeight := m.effectiveContentHeight() - lipgloss.Height(cfg.listHeader(&m, innerWidth))
		selectedItemIdx, items := cfg.buildItems(&m, contentWidth)
		newItemIdx := pageCursor(items, selectedItemIdx, availableHeight, cfg.sep, down)
		if cfg.grouped {
			cfg.setCursor(&m, cursorForItem(items, newItemIdx))
		} else {
			cfg.setCursor(&m, newItemIdx)
		}
		if cfg.onCursorMove != nil {
			cfg.onCursorMove(&m)
		}
		return m, nil

	case key.Matches(msg, keys.Up):
		if cfg.cursor(&m) > 0 {
			cfg.setCursor(&m, cfg.cursor(&m)-1)
			if cfg.onCursorMove != nil {
				cfg.onCursorMove(&m)
			}
		}
		return m, nil

	case key.Matches(msg, keys.Down):
		if cfg.cursor(&m) < cfg.itemCount(&m)-1 {
			cfg.setCursor(&m, cfg.cursor(&m)+1)
			if cfg.onCursorMove != nil {
				cfg.onCursorMove(&m)
			}
		}
		return m, nil

	case key.Matches(msg, keys.Enter):
		return cfg.onEnter(&m)
	}
	return m, nil
}

// renderSelectView is the shared rendering skeleton of the three
// bundle-selection views: title, bordered panel (list header + windowed
// items + scrollbar), help line.
func (m Model) renderSelectView(cfg listViewConfig) string {
	panelWidth := m.effectiveWidth()
	innerWidth := panelWidth - 4
	scrollbarGutter := 4 // left gap(1) + scrollbar(1) + right gap(2)
	contentWidth := innerWidth - scrollbarGutter

	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorderFocus).
		Padding(1, 2).
		Width(panelWidth).
		Height(m.effectiveContentHeight() + 2)

	helpStyle := lipgloss.NewStyle().
		Foreground(colorTextMuted).
		Width(panelWidth)

	contentStyle := lipgloss.NewStyle().Width(innerWidth)

	title := m.renderHeader(cfg.breadcrumb(&m))

	header := cfg.listHeader(&m, innerWidth)
	headerHeight := lipgloss.Height(header)
	availableHeight := m.effectiveContentHeight() - headerHeight

	selectedItemIdx, items := cfg.buildItems(&m, contentWidth)

	start, end := scrollWindowVar(selectedItemIdx, items, availableHeight, cfg.sep)

	itemSep := "\n" + strings.Repeat("\n", cfg.sep)
	var sb strings.Builder
	for i := start; i < end; i++ {
		if i > start {
			sb.WriteString(itemSep)
		}
		sb.WriteString(items[i].content)
	}
	listContent := sb.String()

	visibleCount := end - start
	if len(items) > visibleCount {
		trackHeight := lipgloss.Height(listContent)
		scrollbar := renderScrollbar(len(items), visibleCount, start, trackHeight)
		listContent = lipgloss.JoinHorizontal(lipgloss.Top, listContent, " ", scrollbar, "  ")
	}

	inner := contentStyle.Render(lipgloss.JoinVertical(lipgloss.Left, header, listContent))

	help := helpStyle.Render(m.finalHelpText(cfg.helpLine(&m)))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		borderStyle.Render(inner),
		help,
	)

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

// selectHelpLine builds the help text shared by the grouped selection views
// (promote/reconfig): staged esc label, env-cycle hint, filter hint.
func selectHelpLine(f *textFilter, ef *envFilterCycle, envHint string) string {
	escLabel := "esc: back"
	if ef != nil && ef.pos >= 0 {
		escLabel = "esc: reset filter"
	}
	if f.input.Value() != "" {
		escLabel = "esc: clear filter"
	}
	helpParts := escLabel
	if ef != nil && len(ef.filters) > 0 {
		helpParts += " • e: " + envHint + ef.nextName()
	}
	switch {
	case f.editing:
		helpParts = "esc: clear • enter: apply"
	case f.input.Value() == "":
		helpParts += " • /: filter"
	default:
		helpParts += " • /: edit filter"
	}
	return helpParts
}

// groupedItemsOpts parameterizes renderGroupedItems for the per-view
// differences of the grouped selection lists.
type groupedItemsOpts struct {
	// padAliases right-pads instance names to the widest alias (promote,
	// to align the env-flow annotations).
	padAliases bool
	// annotate returns the fully styled trailing annotation for a row
	// ("" for none). globalIdx is the bundle's index in the ungrouped
	// list (bundleGroup.offsets), as needed by promote's target-env lookup.
	annotate func(m *Model, b *config.Bundle, globalIdx int) string
}

// renderGroupedItems renders grouped bundles as a flat list of
// renderedItems. Group headers are non-selectable separator items,
// instances are individual items. Returns the index of the selected item
// in the flat list, suitable for scrollWindowVar.
func renderGroupedItems(m *Model, groups []bundleGroup, cursor, contentWidth int, opts groupedItemsOpts) (int, []renderedItem) {
	selectedStyle := lipgloss.NewStyle().Foreground(colorPrimary).Bold(true)
	headerNameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	versionStyle := lipgloss.NewStyle().Foreground(colorTextSubtle)

	lineStyle := lipgloss.NewStyle().Width(contentWidth)

	maxAliasWidth := 0
	if opts.padAliases {
		for _, g := range groups {
			for _, b := range g.bundles {
				w := lipgloss.Width(change.DisplayNameFromAlias(b.Alias, b.Name))
				if w > maxAliasWidth {
					maxAliasWidth = w
				}
			}
		}
	}

	var items []renderedItem
	selectedItemIdx := 0
	visualIdx := 0

	for gi, g := range groups {
		b0 := g.bundles[0]

		// Empty line before group (except first)
		if gi > 0 {
			items = append(items, renderedItem{content: "", height: 1, selectable: false})
		}

		// Group header: non-selectable
		headerLine := headerNameStyle.Render(g.name) + " " + versionStyle.Render("v"+b0.DefinitionMetadata.Version)
		items = append(items, renderedItem{content: lineStyle.Render(headerLine), height: 1, selectable: false})

		// Instance rows
		for i, b := range g.bundles {
			isSelected := visualIdx == cursor
			if isSelected {
				selectedItemIdx = len(items)
			}
			visualIdx++

			displayName := change.DisplayNameFromAlias(b.Alias, b.Name)
			pad := ""
			if opts.padAliases {
				pad = strings.Repeat(" ", maxAliasWidth-lipgloss.Width(displayName)+2)
			}

			var line string
			if isSelected {
				line = selectedStyle.Render("  › "+displayName) + pad
			} else {
				line = "    " + displayName + pad
			}
			if opts.annotate != nil {
				line += opts.annotate(m, b, g.offsets[i])
			}

			items = append(items, renderedItem{content: lineStyle.Render(line), height: 1, selectable: true})
		}
	}

	return selectedItemIdx, items
}
