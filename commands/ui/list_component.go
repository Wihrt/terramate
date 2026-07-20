// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/terramate-io/terramate/config"
)

type renderedItem struct {
	content    string
	height     int
	selectable bool // false for non-selectable rows such as group headers/separators
}

// firstSelectableIndex returns the index of the first selectable item, or 0 if none.
func firstSelectableIndex(items []renderedItem) int {
	for i, it := range items {
		if it.selectable {
			return i
		}
	}
	return 0
}

// lastSelectableIndex returns the index of the last selectable item, or 0 if none.
func lastSelectableIndex(items []renderedItem) int {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].selectable {
			return i
		}
	}
	return 0
}

// scrollWindowVar computes a visible window of variable-height items that fits
// within availableHeight, keeping the selected item visible.
// sep is the number of visual lines between items (1 for "\n\n", 0 for "\n").
func scrollWindowVar(selectedIdx int, items []renderedItem, availableHeight, sep int) (start, end int) {
	total := len(items)
	if total == 0 {
		return 0, 0
	}
	totalH := 0
	for i, it := range items {
		totalH += it.height
		if i > 0 {
			totalH += sep
		}
	}
	if totalH <= availableHeight {
		return 0, total
	}

	// Start from selected, expand downward then upward.
	start = selectedIdx
	end = selectedIdx + 1
	usedH := items[selectedIdx].height

	for {
		expanded := false
		if end < total && usedH+sep+items[end].height <= availableHeight {
			usedH += sep + items[end].height
			end++
			expanded = true
		}
		if start > 0 && usedH+sep+items[start-1].height <= availableHeight {
			start--
			usedH += sep + items[start].height
			expanded = true
		}
		if !expanded {
			break
		}
	}
	return
}

func renderScrollbar(totalItems, visibleCount, offset, trackHeight int) string {
	if totalItems <= visibleCount || trackHeight <= 0 {
		return ""
	}

	thumbSize := max(1, trackHeight*visibleCount/totalItems)
	maxOff := totalItems - visibleCount
	thumbPos := 0
	if maxOff > 0 {
		thumbPos = (trackHeight - thumbSize) * offset / maxOff
	}
	if thumbPos+thumbSize > trackHeight {
		thumbPos = trackHeight - thumbSize
	}

	trackStyle := lipgloss.NewStyle().Foreground(colorScrollTrack)
	thumbStyle := lipgloss.NewStyle().Foreground(colorScrollThumb)

	var sb strings.Builder
	for i := range trackHeight {
		if i > 0 {
			sb.WriteByte('\n')
		}
		if i >= thumbPos && i < thumbPos+thumbSize {
			sb.WriteString(thumbStyle.Render("┃"))
		} else {
			sb.WriteString(trackStyle.Render("│"))
		}
	}
	return sb.String()
}

// detailField represents a labeled field in the detail box.
type detailField struct {
	label    string
	value    string
	truncEnd bool // true: truncate end ("long..."), false: truncate start ("...long")
}

// renderDetailBox renders a sticky detail box with a titled border and labeled fields.
// The boxTitle appears on the top border line. The first field is the main title (bold).
func renderDetailBox(innerWidth int, boxTitle string, fields []detailField) string {
	borderColor := lipgloss.NewStyle().Foreground(colorBorder)
	titleColor := lipgloss.NewStyle().Foreground(colorTextMuted)
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	labelStyle := lipgloss.NewStyle().Foreground(colorTextSubtle)
	valueStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
	versionStyle := lipgloss.NewStyle().Foreground(colorTextSubtle)

	// Content width inside the box (border 1 + padding 1 on each side)
	contentWidth := innerWidth - 4
	if contentWidth < 20 {
		contentWidth = 20
	}

	// Build the top border with title: ╭─ Title ───────────╮
	titleText := titleColor.Render(" " + boxTitle + " ")
	titleVisualWidth := lipgloss.Width(titleText)
	fillWidth := innerWidth - 2 - 1 - titleVisualWidth // 2 for corners, 1 for dash before title
	if fillWidth < 0 {
		fillWidth = 0
	}
	topBorder := borderColor.Render("╭─") + titleText + borderColor.Render(strings.Repeat("─", fillWidth)+"╮")

	// Find max label width for alignment
	maxLabelWidth := 0
	for _, f := range fields {
		if f.label != "" && lipgloss.Width(f.label)+2 > maxLabelWidth {
			maxLabelWidth = lipgloss.Width(f.label) + 2 // +2 for ": "
		}
	}

	// Build content lines
	var contentLines []string
	for _, f := range fields {
		// Separator line
		if f.label == "" && f.value == "" {
			sepLine := borderColor.Render("├" + strings.Repeat("─", innerWidth-2) + "┤")
			contentLines = append(contentLines, sepLine)
			continue
		}
		var line string
		if f.label != "" {
			paddedLabel := f.label + ": " + strings.Repeat(" ", maxLabelWidth-lipgloss.Width(f.label)-2)
			label := labelStyle.Render(paddedLabel)
			labelWidth := lipgloss.Width(label)
			availWidth := contentWidth - labelWidth
			if availWidth < 10 {
				availWidth = 10
			}
			// Split name+version for the Bundle field
			name, version := splitNameVersion(f.value)
			if version != "" {
				val := truncateEnd(name, availWidth-lipgloss.Width(version)-1)
				line = label + nameStyle.Render(val) + " " + versionStyle.Render(version)
			} else {
				var val string
				if f.truncEnd {
					val = truncateEnd(f.value, availWidth)
				} else {
					val = truncateStart(f.value, availWidth)
				}
				line = label + valueStyle.Render(val)
			}
		} else {
			name, version := splitNameVersion(f.value)
			line = nameStyle.Render(truncateEnd(name, contentWidth-lipgloss.Width(version)-1))
			if version != "" {
				line += " " + versionStyle.Render(version)
			}
		}
		// Pad each line to full width and wrap with border chars
		lineWidth := lipgloss.Width(line)
		pad := contentWidth - lineWidth
		if pad < 0 {
			pad = 0
		}
		contentLines = append(contentLines, borderColor.Render("│")+" "+line+strings.Repeat(" ", pad)+" "+borderColor.Render("│"))
	}

	// Bottom border
	bottomBorder := borderColor.Render("╰" + strings.Repeat("─", innerWidth-2) + "╯")

	all := []string{topBorder}
	all = append(all, contentLines...)
	all = append(all, bottomBorder)
	return strings.Join(all, "\n")
}

// renderErrorBox renders an error message in a red-bordered box matching the
// dimensions of the detail box.
func renderErrorBox(innerWidth int, msg string) string {
	borderColor := lipgloss.NewStyle().Foreground(colorError)
	textStyle := lipgloss.NewStyle().Foreground(colorError)

	contentWidth := innerWidth - 4 // border 1 + padding 1 on each side
	if contentWidth < 20 {
		contentWidth = 20
	}

	titleText := borderColor.Render(" Error ")
	titleVisualWidth := lipgloss.Width(titleText)
	fillWidth := innerWidth - 2 - 1 - titleVisualWidth
	if fillWidth < 0 {
		fillWidth = 0
	}
	topBorder := borderColor.Render("╭─") + titleText + borderColor.Render(strings.Repeat("─", fillWidth)+"╮")

	// Word-wrap the message to fit inside the box.
	wrapped := lipgloss.NewStyle().Width(contentWidth).Render(msg)
	var contentLines []string
	for _, line := range strings.Split(wrapped, "\n") {
		lineWidth := lipgloss.Width(line)
		pad := contentWidth - lineWidth
		if pad < 0 {
			pad = 0
		}
		contentLines = append(contentLines, borderColor.Render("│")+" "+textStyle.Render(line)+strings.Repeat(" ", pad)+" "+borderColor.Render("│"))
	}

	bottomBorder := borderColor.Render("╰" + strings.Repeat("─", innerWidth-2) + "╯")

	all := []string{topBorder}
	all = append(all, contentLines...)
	all = append(all, bottomBorder)
	return strings.Join(all, "\n")
}

// truncateEnd truncates a string at the end: "very long string" → "very long st..."
func truncateEnd(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return string(runes[:maxWidth-3]) + "..."
}

// truncateStart truncates a string at the start: "/very/long/path/file" → ".../long/path/file"
func truncateStart(s string, maxWidth int) string {
	if maxWidth <= 3 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	return "..." + string(runes[len(runes)-maxWidth+3:])
}

type bundleGroup struct {
	name    string
	detail  string
	bundles []*config.Bundle
	offsets []int // cursor positions in the flat list
}

// groupBundles groups bundles by definition identity (name + version + source),
// sorted alphabetically by group name, with instances sorted by alias within each group.
func groupBundles(bundles []*config.Bundle) []bundleGroup {
	var groups []bundleGroup
	seen := map[string]int{}

	for i, b := range bundles {
		key := b.DefinitionMetadata.Name + "\x00" + b.DefinitionMetadata.Version + "\x00" + b.Source
		if gIdx, ok := seen[key]; ok {
			groups[gIdx].bundles = append(groups[gIdx].bundles, b)
			groups[gIdx].offsets = append(groups[gIdx].offsets, i)
			continue
		}
		seen[key] = len(groups)
		groups = append(groups, bundleGroup{
			name:    b.DefinitionMetadata.Name,
			detail:  fmt.Sprintf("v%s from %s", b.DefinitionMetadata.Version, b.Source),
			bundles: []*config.Bundle{b},
			offsets: []int{i},
		})
	}

	// Sort groups deterministically by name, then detail (version+source) as tiebreaker
	slices.SortFunc(groups, func(a, b bundleGroup) int {
		if c := cmp.Compare(a.name, b.name); c != 0 {
			return c
		}
		return cmp.Compare(a.detail, b.detail)
	})

	// Sort instances within each group by alias
	for i := range groups {
		g := &groups[i]
		indices := make([]int, len(g.bundles))
		for j := range indices {
			indices[j] = j
		}
		slices.SortFunc(indices, func(a, b int) int {
			return cmp.Compare(g.bundles[a].Alias, g.bundles[b].Alias)
		})
		sortedBundles := make([]*config.Bundle, len(g.bundles))
		sortedOffsets := make([]int, len(g.offsets))
		for j, idx := range indices {
			sortedBundles[j] = g.bundles[idx]
			sortedOffsets[j] = g.offsets[idx]
		}
		g.bundles = sortedBundles
		g.offsets = sortedOffsets
	}

	return groups
}

// pageCursor returns the new item-list index for a PgUp/PgDn jump over a
// rendered list. down selects PgDn vs PgUp. cursor and the return value are
// indices into items, not bundle indices — see cursorForItem.
func pageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int {
	if len(items) == 0 {
		return 0
	}
	start, end := scrollWindowVar(cursor, items, availableHeight, sep)
	if down {
		for i := end; i < len(items); i++ {
			if items[i].selectable {
				return i
			}
		}
		return lastSelectableIndex(items)
	}
	for i := start - 1; i >= 0; i-- {
		if items[i].selectable {
			return i
		}
	}
	return firstSelectableIndex(items)
}

// cursorForItem converts an index into the rendered items list back into a
// bundle-index (cursor space) by counting selectable items before it.
func cursorForItem(items []renderedItem, itemIdx int) int {
	rank := 0
	for i := 0; i < itemIdx; i++ {
		if items[i].selectable {
			rank++
		}
	}
	return rank
}
