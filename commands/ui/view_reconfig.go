// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/typeschema"
)

// reconfigEnvTag renders the single [Env] tag annotation of the
// Reconfigure list rows.
func reconfigEnvTag(_ *Model, b *config.Bundle, _ int) string {
	if b.Environment == nil {
		return ""
	}
	envStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
	return " " + envStyle.Render("["+b.Environment.Name+"]")
}

// reconfigBreadcrumb builds the Reconfigure title reflecting active filters.
func (m *Model) reconfigBreadcrumb() string {
	breadcrumb := "Reconfigure Bundle Instance"
	if f := m.reconfigEnvFilter.current(); f != nil {
		if f.envLess {
			breadcrumb = "Reconfigure Bundle Instance Without Environment"
		} else {
			breadcrumb = "Reconfigure Bundle Instance in " + f.label
		}
	}
	if query := m.reconfigFilter.input.Value(); query != "" {
		breadcrumb += fmt.Sprintf(" — filter: %q", query)
	}
	return breadcrumb
}

var reconfigListViewCfg = listViewConfig{
	breadcrumb: func(m *Model) string { return m.reconfigBreadcrumb() },
	helpLine: func(m *Model) string {
		return selectHelpLine(&m.reconfigFilter, &m.reconfigEnvFilter, "show only ")
	},
	listHeader: func(m *Model, innerWidth int) string { return m.reconfigListHeader(innerWidth) },
	buildItems: func(m *Model, contentWidth int) (int, []renderedItem) {
		return renderGroupedItems(m, groupBundles(m.reconfigBundles), m.reconfigCursor, contentWidth, groupedItemsOpts{annotate: reconfigEnvTag})
	},
	itemCount:   func(m *Model) int { return len(m.reconfigBundles) },
	cursor:      func(m *Model) int { return m.reconfigCursor },
	setCursor:   func(m *Model, c int) { m.reconfigCursor = c },
	textFilter:  func(m *Model) *textFilter { return &m.reconfigFilter },
	applyFilter: func(m *Model) { m.applyReconfigFilter() },
	envFilter:   func(m *Model) *envFilterCycle { return &m.reconfigEnvFilter },
	onEnter: func(m *Model) (tea.Model, tea.Cmd) {
		if m.reconfigCursor < len(m.reconfigBundles) {
			if err := m.loadReconfigBundle(m.reconfigBundles[m.reconfigCursor]); err != nil {
				return m.updateError(err)
			}
			m.viewState = ViewReconfigInput
			return *m, nil
		}
		return *m, nil
	},
	exit:    func(m *Model) { m.viewState = ViewOverview },
	sep:     0,
	grouped: true,
}

func (m Model) updateReconfigSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateSelectView(reconfigListViewCfg, msg)
}

// applyReconfigFilter rebuilds the bundle list based on the current filter position.
func (m *Model) applyReconfigFilter() {
	m.reconfigBundles = m.buildReconfigBundles()
	m.reconfigCursor = 0
}

// loadReconfigBundle loads the bundle definition for the given bundle,
// evaluates its input definitions, and creates the inputs form pre-populated
// with the bundle's current values.
func (m *Model) loadReconfigBundle(b *config.Bundle) error {
	est := m.EngineState
	// We create a BundleDefinitionEntry
	bde := makeBundleDefinitionEntry(est.Root, b)

	schemactx, err := m.loadBundleEvalContext(bde, b.Environment)
	if err != nil {
		return err
	}

	inputDefs, err := config.EvalBundleInputDefinitions(schemactx, bde.Define)
	if err != nil {
		return errors.E(err, "failed to evaluate input definitions")
	}

	values := inputsToValueMap(b.Inputs)
	normalizeBundleRefValues(inputDefs, values)

	m.reconfigBundle = b
	m.selectedBundleDefEntry = bde
	m.inputsForm = NewInputsFormWithValues(inputDefs, schemactx, est.Registry, b.Environment, nil, values, values, rawInputKeys(b, schemactx.Evalctx))
	m.inputsForm.confirmLabel = "Save"
	m.inputsForm.PanelWidth = m.effectiveWidth()
	m.inputsForm.PanelHeight = m.effectiveInputsPanelHeight()
	return nil
}

// buildReconfigFilters precomputes the list of valid filter states
// (environments that have reconfigurable bundles, plus env-less if applicable).
func (m Model) buildReconfigFilters() []envFilterState {
	// Check which envs have bundles, and whether env-less bundles exist
	envHas := make(map[string]bool)
	hasEnvLess := false
	for _, b := range m.EngineState.Registry.Bundles {
		if b.Environment == nil {
			hasEnvLess = true
		} else {
			envHas[b.Environment.ID] = true
		}
	}

	var states []envFilterState
	for _, env := range m.EngineState.Registry.Environments {
		if envHas[env.ID] {
			states = append(states, envFilterState{
				env:     env,
				label:   env.Name,
				shortID: env.ID,
			})
		}
	}
	if hasEnvLess {
		states = append(states, envFilterState{
			envLess: true,
			label:   "Without Environment",
			shortID: "env-less",
		})
	}
	return states
}

// loadBundleEvalContext creates a bundle eval context and loads the schema namespaces for the given bundle.
func (m Model) loadBundleEvalContext(bde *config.BundleDefinitionEntry, env *config.Environment) (typeschema.EvalContext, error) {
	est := m.EngineState
	evalctx := newBundleEvalContext(est.Evalctx, est.Registry, env)
	schemas, err := config.EvalBundleSchemaNamespaces(est.Root, est.ResolveAPI, evalctx, bde.Define, true)
	if err != nil {
		return typeschema.EvalContext{}, errors.E(err, "Failed to load bundle schema.")
	}
	return typeschema.EvalContext{
		Evalctx: evalctx,
		Schemas: schemas,
	}, nil
}

// makeBundleDefinitionEntry constructs a BundleDefinitionEntry from an existing, already loaded bundle.
func makeBundleDefinitionEntry(root *config.Root, b *config.Bundle) *config.BundleDefinitionEntry {
	// This cannot fail. If we have the evaluated config.Bundle already, the HCL define must exist.
	tree, _ := root.Lookup(b.ResolvedSource)
	for _, def := range tree.Node.Defines {
		if def.Bundle != nil {
			return &config.BundleDefinitionEntry{
				Tree:     tree,
				Metadata: &b.DefinitionMetadata,
				Define:   def.Bundle,
			}
		}
	}
	return nil
}

// buildReconfigBundles returns bundles that do not already have a pending
// ChangeReconfig entry, sorted into grouped display order so that
// the flat cursor index matches the visual position.
func (m Model) buildReconfigBundles() []*config.Bundle {
	f := m.reconfigEnvFilter.current()
	query := m.reconfigFilter.query()
	var filtered []*config.Bundle
	for _, b := range m.EngineState.Registry.Bundles {
		if f != nil {
			if f.envLess {
				if b.Environment != nil {
					continue
				}
			} else if b.Environment == nil || b.Environment.ID != f.env.ID {
				continue
			}
		}
		if !bundleMatchesFilter(b, query) {
			continue
		}
		filtered = append(filtered, b)
	}

	groups := groupBundles(filtered)
	sorted := make([]*config.Bundle, 0, len(filtered))
	for _, g := range groups {
		sorted = append(sorted, g.bundles...)
	}
	return sorted
}

func (m Model) renderReconfigSelectView() string {
	return m.renderSelectView(reconfigListViewCfg)
}

// reconfigListHeader renders the detail box shown above the Reconfigure
// bundle list. Used both for display and, via lipgloss.Height, to compute
// the available height for PgUp/PgDn page-jump math.
func (m Model) reconfigListHeader(innerWidth int) string {
	est := m.EngineState
	var detailBox string
	if m.reconfigCursor < len(m.reconfigBundles) {
		b := m.reconfigBundles[m.reconfigCursor]
		fields := []detailField{
			{label: "Bundle", value: b.DefinitionMetadata.Name + " v" + b.DefinitionMetadata.Version, truncEnd: true},
		}
		if b.DefinitionMetadata.Class != "" {
			fields = append(fields, detailField{label: "Class", value: b.DefinitionMetadata.Class, truncEnd: true})
		}
		fields = append(fields, detailField{label: "Alias", value: displayNameFromAlias(b.Alias, b.Name), truncEnd: true})
		envName := "n/a"
		if b.Environment != nil {
			envName = b.Environment.Name
		}
		fields = append(fields, detailField{label: "Environment", value: envName, truncEnd: true})
		fields = append(fields, detailField{}) // separator
		fields = append(fields, detailField{label: "Source", value: b.Source})
		hostPath := project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()).String()
		if hostPath != "" {
			fields = append(fields, detailField{label: "Config", value: hostPath})
		}
		detailBox = renderDetailBox(innerWidth, "Bundle Instance Details", fields)
	}
	var headerParts []string
	if m.reconfigFilter.editing || m.reconfigFilter.input.Value() != "" {
		filterStyle := lipgloss.NewStyle().Foreground(colorTextMuted)
		headerParts = append(headerParts, filterStyle.Render(m.reconfigFilter.input.View()), "")
	}
	headerParts = append(headerParts, detailBox, "")
	return lipgloss.JoinVertical(lipgloss.Left, headerParts...)
}

func (m Model) renderReconfigInputView() string {
	panelWidth := m.effectiveWidth()
	helpStyle := lipgloss.NewStyle().
		Foreground(colorTextMuted).
		Width(panelWidth)

	b := m.reconfigBundle
	aliasStyle := lipgloss.NewStyle().Foreground(colorCreate)
	alias := aliasStyle.Render(displayNameFromAlias(b.Alias, b.Name))
	envStyle := lipgloss.NewStyle().Foreground(colorPromote)
	var envTag string
	if b.Environment != nil {
		envTag = " " + envStyle.Render("["+b.Environment.Name+"]")
	} else {
		envTag = " " + envStyle.Render("[Without Environment]")
	}
	headerContext := "Reconfigure " + b.DefinitionMetadata.Name + ": " + alias + envTag
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

func (m Model) updateReconfigInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	est := m.EngineState

	if key.Matches(msg, keys.Escape) {
		if handled, cmd := m.trySubFormEscape(); handled {
			return m, cmd
		}
	}

	if key.Matches(msg, keys.Escape) && len(m.objectEditStack) == 0 && !m.inputsForm.IsMultilineActive() && !m.inputsForm.confirmingDiscard {
		if m.inputsForm.HasPendingChanges() {
			m.inputsForm.preDiscardFocus = m.inputsForm.focus
			m.inputsForm.confirmingDiscard = true
			m.inputsForm.discardConfirmIdx = 1
			m.inputsForm.focus = InputFocusActive
			return m, nil
		}
		if m.reconfigFromOverview {
			m.reconfigFromOverview = false
			m.viewState = ViewOverview
		} else {
			m.viewState = ViewReconfigSelect
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.inputsForm, cmd = m.inputsForm.Update(msg)

	if handled, scmd := m.trySubFormStateTransition(m.reconfigBundle.Environment); handled {
		return m, scmd
	}

	switch m.inputsForm.State() {
	case InputsFormAccepted:
		if m.inputsForm.HasPendingChanges() {
			change, err := NewReconfigChange(
				est, m.reconfigBundle, m.selectedBundleDefEntry,
				m.inputsForm.Schemactx, m.inputsForm.InputDefs, m.inputsForm.UserValues(),
			)
			if err != nil {
				m.inputsForm.SetValidationError(err)
				m.inputsForm.state = InputsFormActive
				break
			}
			if err := change.Save(est.Registry.Environments); err != nil {
				m.inputsForm.SetValidationError(err)
				m.inputsForm.state = InputsFormActive
				break
			}
			if err := m.reloadAll(); err != nil {
				m.inputsForm.SetValidationError(err)
				m.inputsForm.state = InputsFormActive
				break
			}
			m.recordSessionChange(change)
		}
		m.reconfigFromOverview = false
		m.viewState = ViewOverview
	case InputsFormDiscarded:
		if m.reconfigFromOverview {
			m.reconfigFromOverview = false
			m.viewState = ViewOverview
		} else {
			m.viewState = ViewReconfigSelect
		}
	}

	return m, cmd
}
