// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/terramate-io/terramate/commands/ui/change"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/project"
)

// promoteEnvFlow renders the "source → target" env annotation of the
// Promote list rows.
func promoteEnvFlow(m *Model, b *config.Bundle, globalIdx int) string {
	if globalIdx >= len(m.promoteTargetEnvs) || b.Environment == nil {
		return ""
	}
	envStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
	sourceEnvName := envNameForID(m.EngineState.Registry.Environments, b.Environment.ID)
	targetEnvName := m.promoteTargetEnvs[globalIdx].Name
	return envStyle.Render(sourceEnvName + " → " + targetEnvName)
}

// promoteBreadcrumb builds the Promote title reflecting active filters.
func (m *Model) promoteBreadcrumb() string {
	breadcrumb := "Promote Bundle Instance"
	if f := m.promoteEnvFilter.current(); f != nil {
		breadcrumb = "Promote Bundle Instance to " + f.label
	}
	if query := m.promoteFilter.input.Value(); query != "" {
		breadcrumb += fmt.Sprintf(" — filter: %q", query)
	}
	return breadcrumb
}

var promoteListViewCfg = listViewConfig{
	breadcrumb: func(m *Model) string { return m.promoteBreadcrumb() },
	helpLine: func(m *Model) string {
		return selectHelpLine(&m.promoteFilter, &m.promoteEnvFilter, "show target env ")
	},
	listHeader: func(m *Model, innerWidth int) string { return m.promoteListHeader(innerWidth) },
	buildItems: func(m *Model, contentWidth int) (int, []renderedItem) {
		return renderGroupedItems(m, groupBundles(m.promoteBundles), m.promoteCursor, contentWidth, groupedItemsOpts{padAliases: true, annotate: promoteEnvFlow})
	},
	itemCount:   func(m *Model) int { return len(m.promoteBundles) },
	cursor:      func(m *Model) int { return m.promoteCursor },
	setCursor:   func(m *Model, c int) { m.promoteCursor = c },
	textFilter:  func(m *Model) *textFilter { return &m.promoteFilter },
	applyFilter: func(m *Model) { m.applyPromoteFilter() },
	envFilter:   func(m *Model) *envFilterCycle { return &m.promoteEnvFilter },
	onEnter: func(m *Model) (tea.Model, tea.Cmd) {
		if m.promoteCursor < len(m.promoteBundles) {
			targetEnv := m.promoteTargetEnvs[m.promoteCursor]
			if err := m.loadPromoteBundle(m.promoteBundles[m.promoteCursor], targetEnv); err != nil {
				return m.updateError(err)
			}
			m.viewState = ViewPromoteInput
			return *m, nil
		}
		return *m, nil
	},
	exit:    func(m *Model) { m.viewState = ViewOverview },
	sep:     0,
	grouped: true,
}

func (m Model) updatePromoteSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateSelectView(promoteListViewCfg, msg)
}

// applyPromoteFilter rebuilds the bundle list based on the current filter position.
func (m *Model) applyPromoteFilter() {
	m.promoteBundles, m.promoteTargetEnvs = m.buildAllPromoteBundles()
	m.promoteCursor = 0
}

// loadPromoteBundle loads the bundle definition for the given bundle,
// evaluates its input definitions, and creates the inputs form pre-populated
// with the bundle's current values.
func (m *Model) loadPromoteBundle(b *config.Bundle, targetEnv *config.Environment) error {
	est := m.EngineState
	// We create a BundleDefinitionEntry
	bde := change.MakeBundleDefinitionEntry(est.Root, b)

	schemactx, err := m.EngineState.loadBundleEvalContext(bde, targetEnv)
	if err != nil {
		return err
	}

	inputDefs, err := config.EvalBundleInputDefinitions(schemactx, bde.Define)
	if err != nil {
		return errors.E(err, "failed to evaluate input definitions")
	}

	values := change.InputsToValueMap(b.Inputs)
	change.NormalizeBundleRefValues(inputDefs, values)

	m.promoteBundle = b
	m.selectedBundleDefEntry = bde
	m.inputsForm = NewInputsFormWithValues(inputDefs, schemactx, est.Registry, targetEnv, b.Environment, values, values, change.RawInputKeys(b, schemactx.Evalctx))
	m.inputsForm.confirmLabel = "Save"
	m.inputsForm.PanelWidth = m.effectiveWidth()
	m.inputsForm.PanelHeight = m.effectiveInputsPanelHeight()
	return nil
}

// buildPromoteFilters precomputes the list of target envs that have promotable bundles.
func (m Model) buildPromoteFilters() []envFilterState {
	// Build all promotable bundles unfiltered to find which target envs have results
	targetEnvHas := make(map[string]bool)
	for _, targetEnv := range m.EngineState.Registry.Environments {
		if targetEnv.PromoteFrom == "" {
			continue
		}
		// Temporarily check if this target env has promotable bundles
		envAliases := make(map[string]bool)
		for _, b := range m.EngineState.Registry.Bundles {
			if b.Environment != nil && b.Environment.ID == targetEnv.ID {
				envAliases[b.Alias] = true
			}
		}
		for _, b := range m.EngineState.Registry.Bundles {
			if b.Environment == nil || b.Environment.ID != targetEnv.PromoteFrom {
				continue
			}
			if !envAliases[b.Alias] && len(change.MissingBundleRefs(b, envAliases)) == 0 {
				targetEnvHas[targetEnv.ID] = true
				break
			}
		}
	}

	var states []envFilterState
	for _, env := range m.EngineState.Registry.Environments {
		if targetEnvHas[env.ID] {
			states = append(states, envFilterState{
				env:     env,
				label:   env.Name,
				shortID: env.ID,
			})
		}
	}
	return states
}

// buildAllPromoteBundles returns all promotable bundles across all environments.
// For each environment with PromoteFrom, finds bundles from the source env
// that don't already exist (by alias) in the target env.
// Returns parallel slices: bundles and their corresponding target environments.
func (m Model) buildAllPromoteBundles() ([]*config.Bundle, []*config.Environment) {
	est := m.EngineState

	// Build alias index per env: envID -> set of aliases
	envAliases := make(map[string]map[string]bool)
	for _, b := range est.Registry.Bundles {
		if b.Environment == nil {
			continue
		}
		if _, ok := envAliases[b.Environment.ID]; !ok {
			envAliases[b.Environment.ID] = make(map[string]bool)
		}
		envAliases[b.Environment.ID][b.Alias] = true
	}

	var bundles []*config.Bundle
	var targetEnvs []*config.Environment
	query := m.promoteFilter.query()

	for _, targetEnv := range est.Registry.Environments {
		if targetEnv.PromoteFrom == "" {
			continue
		}

		// Apply env filter: only show bundles promotable into the filtered target env
		if f := m.promoteEnvFilter.current(); f != nil && f.env.ID != targetEnv.ID {
			continue
		}

		existing := envAliases[targetEnv.ID]
		for _, b := range est.Registry.Bundles {
			if b.Environment == nil || b.Environment.ID != targetEnv.PromoteFrom {
				continue
			}
			if existing[b.Alias] {
				continue
			}
			if len(change.MissingBundleRefs(b, existing)) > 0 {
				continue
			}
			if !bundleMatchesFilter(b, query) {
				continue
			}
			bundles = append(bundles, b)
			targetEnvs = append(targetEnvs, targetEnv)
		}
	}

	// Sort into grouped display order so the flat cursor index matches
	// the visual position. Keep targetEnvs in sync.
	groups := groupBundles(bundles)
	sorted := make([]*config.Bundle, 0, len(bundles))
	sortedEnvs := make([]*config.Environment, 0, len(bundles))
	for _, g := range groups {
		for _, idx := range g.offsets {
			sorted = append(sorted, bundles[idx])
			sortedEnvs = append(sortedEnvs, targetEnvs[idx])
		}
	}
	return sorted, sortedEnvs
}

func envNameForID(envs []*config.Environment, envID string) string {
	for _, env := range envs {
		if env.ID == envID {
			return env.Name
		}
	}
	return ""
}

func (m Model) renderPromoteSelectView() string {
	return m.renderSelectView(promoteListViewCfg)
}

// promoteListHeader renders the detail box shown above the Promote bundle
// list. Used both for display and, via lipgloss.Height, to compute the
// available height for PgUp/PgDn page-jump math.
func (m Model) promoteListHeader(innerWidth int) string {
	est := m.EngineState
	var detailBox string
	if m.promoteCursor < len(m.promoteBundles) {
		b := m.promoteBundles[m.promoteCursor]
		fields := []detailField{
			{label: "Bundle", value: b.DefinitionMetadata.Name + " v" + b.DefinitionMetadata.Version, truncEnd: true},
		}
		if b.DefinitionMetadata.Class != "" {
			fields = append(fields, detailField{label: "Class", value: b.DefinitionMetadata.Class, truncEnd: true})
		}
		fields = append(fields, detailField{label: "Alias", value: change.DisplayNameFromAlias(b.Alias, b.Name), truncEnd: true})
		if m.promoteCursor < len(m.promoteTargetEnvs) {
			sourceEnvName := envNameForID(est.Registry.Environments, b.Environment.ID)
			targetEnvName := m.promoteTargetEnvs[m.promoteCursor].Name
			fields = append(fields, detailField{label: "Promote", value: sourceEnvName + " → " + targetEnvName, truncEnd: true})
		}
		fields = append(fields, detailField{}) // separator
		fields = append(fields, detailField{label: "Source", value: b.Source})
		hostPath := project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()).String()
		if hostPath != "" {
			fields = append(fields, detailField{label: "Config", value: hostPath})
		}
		detailBox = renderDetailBox(innerWidth, "Bundle Instance Details", fields)
	}
	var headerParts []string
	if m.promoteFilter.editing || m.promoteFilter.input.Value() != "" {
		filterStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
		headerParts = append(headerParts, filterStyle.Render(m.promoteFilter.input.View()), "")
	}
	headerParts = append(headerParts, detailBox, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}

func (m Model) renderPromoteInputView() string {
	panelWidth := m.effectiveWidth()
	helpStyle := lipgloss.NewStyle().
		Foreground(colorTextMuted).
		Width(panelWidth)

	b := m.promoteBundle
	aliasStyle := lipgloss.NewStyle().Foreground(colorCreate)
	alias := aliasStyle.Render(change.DisplayNameFromAlias(b.Alias, b.Name))
	var envTag string
	if m.promoteCursor < len(m.promoteTargetEnvs) {
		targetEnv := m.promoteTargetEnvs[m.promoteCursor]
		envStyle := lipgloss.NewStyle().Foreground(colorPromote)
		envTag = " " + envStyle.Render("["+targetEnv.Name+"]")
	}
	headerContext := "Promote " + b.DefinitionMetadata.Name + ": " + alias + envTag
	title := m.renderHeader(headerContext)

	formView := m.inputsForm.View()

	helpText := "esc: back"
	if m.inputsForm.ShowsTwoPanels() {
		helpText = "tab: switch section • esc: back"
	}
	if extra := m.inputsForm.ExtraHelpHints(); extra != "" {
		helpText += " • " + extra
	}
	help := helpStyle.Render(m.finalHelpText(helpText))

	content := lipgloss.JoinVertical(
		lipgloss.Left,
		title,
		formView,
		help,
	)

	return lipgloss.NewStyle().Padding(1, 2).Render(content)
}

func (m Model) updatePromoteInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	est := m.EngineState

	if key.Matches(msg, keys.Escape) {
		if handled, cmd := m.trySubFormEscape(); handled {
			return m, cmd
		}
	}

	if key.Matches(msg, keys.Escape) && len(m.objectEditStack) == 0 && !m.inputsForm.IsMultilineActive() && !m.inputsForm.confirmingDiscard {
		m.viewState = ViewPromoteSelect
		return m, nil
	}

	var targetEnv *config.Environment
	if m.promoteCursor < len(m.promoteTargetEnvs) {
		targetEnv = m.promoteTargetEnvs[m.promoteCursor]
	}

	var cmd tea.Cmd
	m.inputsForm, cmd = m.inputsForm.Update(msg)

	if handled, scmd := m.trySubFormStateTransition(targetEnv); handled {
		return m, scmd
	}

	switch m.inputsForm.State() {
	case InputsFormAccepted:
		ch, err := change.NewPromote(est.changeSession(), targetEnv, m.promoteBundle, m.selectedBundleDefEntry,
			m.inputsForm.Schemactx, m.inputsForm.InputDefs, m.inputsForm.UserValues(),
		)
		if err != nil {
			m.inputsForm.SetValidationError(err)
			m.inputsForm.state = InputsFormActive
			break
		}
		if err := ch.Save(est.Registry.Environments); err != nil {
			m.inputsForm.SetValidationError(err)
			m.inputsForm.state = InputsFormActive
			break
		}
		if err := m.reloadAll(); err != nil {
			m.inputsForm.SetValidationError(err)
			m.inputsForm.state = InputsFormActive
			break
		}
		m.recordSessionChange(ch)

		m.viewState = ViewOverview
	case InputsFormDiscarded:
		m.viewState = ViewPromoteSelect
	}

	return m, cmd
}
