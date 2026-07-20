# Phase 2 — Composant `selectableListView` générique : Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Unifier les 3 vues de sélection de bundles de la TUI (create-select, promote, reconfig) sur un moteur de liste sélectionnable unique (update clavier + rendu), paramétré par vue, en éliminant les ~400-500 lignes de copié-collé restantes — sous protection de tests golden capturés AVANT la restructuration.

**Architecture:** Un nouveau fichier `commands/ui/selectable_list_view.go` porte : `listViewConfig` (callbacks par vue), `updateSelectView` (le switch clavier commun : édition de filtre, Escape à étages, `/`, `e`, PgUp/PgDn, Up/Down, Enter), `renderSelectView` (le squelette de rendu commun : breadcrumb, header, fenêtre d'items, scrollbar, ligne d'aide), `selectHelpLine` (aide commune promote/reconfig) et `renderGroupedItems` (fusion des deux renderers groupés, paramétré par padding d'alias et annotation). Les `updateXSelect`/`renderXSelectView` deviennent des wrappers d'une ligne sur leur config. Le `Model` n'est PAS restructuré (c'est la Phase 3) : les configs accèdent aux champs existants via closures. Spec : `docs/superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md` (Phase 2).

**Tech Stack:** Go 1.25.8, Bubble Tea/lipgloss, tests golden (testdata/ + flag `-update`).

## Global Constraints

- Refactoring **iso-comportement** strict. Les tests golden de la Task 1, capturés sur le code actuel, sont l'arbitre : ils doivent passer **inchangés** après chaque migration (Tasks 2-4). Interdiction de régénérer un golden pour "faire passer" une migration — un golden qui change = régression à corriger dans le code migré.
- Les tests existants (`TestUpdateReconfigSelectFilter`, `TestUpdateCreateSelectFilter`, `TestBuildAllPromoteBundlesCombinesEnvAndTextFilter`, `TestBuildReconfigBundlesCombinesEnvAndTextFilter`, etc.) restent verts, assertions inchangées.
- Nouveaux fichiers : en-tête `// Copyright 2026 Terramate GmbH` + `// SPDX-License-Identifier: MPL-2.0`.
- Messages de commit : conventional commits (`test:`, `refactor:`), en anglais, footer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Branche : `refactor/phase2-selectable-list` (empilée sur `refactor/maintenabilite`).
- Après chaque tâche : `go build ./commands/ui/ && go test -race ./commands/ui/`. Fin de phase : `make build`, `LC_ALL=C make test`, `make lint/all` (ne pas lancer lint et tests en parallèle ; les éventuels `signal: killed` de binaires `-race` sont l'incompatibilité TSan/noyau 7.0 connue — re-vérifier ces packages sans `-race` et le noter, ne pas les traiter comme des régressions).

---

### Task 1: Tests golden de caractérisation des 3 vues de sélection

**Files:**
- Create: `commands/ui/golden_select_test.go`
- Create: `commands/ui/testdata/golden/` (fichiers `.golden` générés puis committés)

**Interfaces:**
- Consumes: `Model`, `EngineState` (`model.go:96`), `config.Registry`, `config.Bundle`, `config.Environment` — mêmes fixtures que les tests existants (`view_reconfig_test.go:14-35` est le modèle à suivre).
- Produces: les goldens de référence que les Tasks 2-4 doivent préserver, et le helper `assertGolden(t, name, got string)`.

**Contexte pour l'implémenteur.** Les trois fonctions de rendu à caractériser sont `renderReconfigSelectView` (`view_reconfig.go:288`), `renderPromoteSelectView` (`view_promote.go:303`), `renderBundleSelectView` (`view_create_select.go:507`). Elles sont appelées via le switch de `Model.View` (`model.go:352-377`) selon `m.viewState`. En test (pas de TTY), lipgloss rend sans couleurs — les goldens sont du texte stable.

- [ ] **Step 1: Écrire le harnais golden**

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/terramate-io/terramate/config"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files with current output")

// assertGolden compares got against testdata/golden/<name>.golden,
// rewriting the file when -update is passed.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run: go test ./commands/ui/ -run TestGolden -update): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("rendered output differs from %s.\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}
```

- [ ] **Step 2: Écrire les fixtures et les tests golden**

Fixture commune (mêmes types que `view_reconfig_test.go:16-28`) :

```go
func goldenEnvs() (*config.Environment, *config.Environment) {
	staging := &config.Environment{ID: "staging", Name: "Staging", PromoteFrom: ""}
	prod := &config.Environment{ID: "prod", Name: "Production", PromoteFrom: "staging"}
	return staging, prod
}

func goldenBundles(staging, prod *config.Environment) []*config.Bundle {
	return []*config.Bundle{
		{DefinitionMetadata: config.Metadata{Name: "vpc", Version: "1.0.0", Class: "network"}, Alias: "vpc-1", Environment: staging, Source: "git::https://example.com/vpc"},
		{DefinitionMetadata: config.Metadata{Name: "vpc", Version: "1.0.0", Class: "network"}, Alias: "vpc-2", Environment: staging, Source: "git::https://example.com/vpc"},
		{DefinitionMetadata: config.Metadata{Name: "ecs", Version: "2.1.0", Class: "compute"}, Alias: "ecs-1", Environment: staging, Source: "git::https://example.com/ecs"},
	}
}
```

Tests (un golden par état représentatif) :

```go
func TestGoldenReconfigSelectView(t *testing.T) {
	staging, prod := goldenEnvs()
	bundles := goldenBundles(staging, prod)
	m := Model{
		EngineState:       &EngineState{Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:             100,
		height:            32,
		viewState:         ViewReconfigSelect,
		reconfigEnvFilter: envFilterCycle{filters: []envFilterState{{env: staging, label: "Staging", shortID: "staging"}}, pos: -1},
		reconfigFilter:    newTextFilter(),
	}
	m.reconfigBundles = m.buildReconfigBundles()

	assertGolden(t, "reconfig-select-basic", m.View())

	// Env filter active
	m.reconfigEnvFilter.pos = 0
	m.reconfigBundles = m.buildReconfigBundles()
	assertGolden(t, "reconfig-select-envfilter", m.View())

	// Text filter narrowing + cursor on second row
	m.reconfigEnvFilter.pos = -1
	m.reconfigFilter.input.SetValue("vpc")
	m.reconfigBundles = m.buildReconfigBundles()
	m.reconfigCursor = 1
	assertGolden(t, "reconfig-select-textfilter", m.View())
}

func TestGoldenPromoteSelectView(t *testing.T) {
	staging, prod := goldenEnvs()
	bundles := goldenBundles(staging, prod)
	m := Model{
		EngineState:      &EngineState{Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:            100,
		height:           32,
		viewState:        ViewPromoteSelect,
		promoteEnvFilter: envFilterCycle{filters: []envFilterState{{env: prod, label: "Production", shortID: "prod"}}, pos: -1},
		promoteFilter:    newTextFilter(),
	}
	m.promoteBundles, m.promoteTargetEnvs = m.buildAllPromoteBundles()

	assertGolden(t, "promote-select-basic", m.View())

	// Cursor on the last promotable bundle
	if len(m.promoteBundles) > 1 {
		m.promoteCursor = len(m.promoteBundles) - 1
	}
	assertGolden(t, "promote-select-cursor-last", m.View())
}

func TestGoldenCreateSelectView(t *testing.T) {
	// flatBundleEntry (model.go:70) wraps *manifest.Bundle + collection
	// origin. isLocal=false makes flatBundleListHeader resolve Source via
	// bundleSourceFromManifest(coll, bundle) — no est.Root needed.
	coll := &manifest.Collection{Name: "Core Bundles", Location: "git::https://example.com/bundles"}
	entries := []flatBundleEntry{
		{collIdx: 0, bundleIdx: 0, collName: "Core Bundles", bundle: &manifest.Bundle{Path: "vpc", Name: "vpc", Class: "network", Version: "1.0.0", Description: "A virtual private cloud bundle."}},
		{collIdx: 0, bundleIdx: 1, collName: "Core Bundles", bundle: &manifest.Bundle{Path: "ecs", Name: "ecs", Class: "compute", Version: "2.1.0"}},
	}
	m := Model{
		EngineState:      &EngineState{Registry: &config.Registry{}, Collections: []*manifest.Collection{coll}},
		width:            100,
		height:           32,
		viewState:        ViewCreateSelect,
		allFlatBundles:   entries,
		flatBundleFilter: newTextFilter(),
	}
	m.applyFlatBundleFilter()

	assertGolden(t, "create-select-basic", m.View())

	// Text filter narrowing
	m.flatBundleFilter.input.SetValue("vpc")
	m.applyFlatBundleFilter()
	assertGolden(t, "create-select-textfilter", m.View())

	// Inline error box
	m.flatBundleFilter.input.SetValue("")
	m.applyFlatBundleFilter()
	m.bundleSelectErr = "boom: could not load bundle definition"
	assertGolden(t, "create-select-error", m.View())
}
```

(Import supplémentaire : `"github.com/terramate-io/terramate/scaffold/manifest"`.) Si un champ de la fixture s'avère malgré tout insuffisant pour rendre sans panic (ex. un accès à `est.Root` non anticipé), NE PAS mettre de garde nil dans le code de production : compléter la fixture (au besoin via `test/sandbox` pour un `*config.Root` réel) et le signaler dans le rapport. Acceptance : les goldens contiennent la boîte de détails (lignes Bundle/Collection/Source), l'item `vpc` sur deux lignes (description) et `ecs` sur une, la ligne de filtre dans `create-select-textfilter`, la boîte d'erreur dans `create-select-error`.

- [ ] **Step 3: Générer les goldens et vérifier la stabilité**

Run: `go test ./commands/ui/ -run TestGolden -update && go test -count=2 ./commands/ui/ -run TestGolden`
Expected: première commande génère les fichiers ; seconde PASSE deux fois (sortie déterministe — si un golden est instable entre deux runs, corriger la fixture, pas le golden).

- [ ] **Step 4: Vérifier la sensibilité (le harnais détecte bien un changement)**

Modifier temporairement une chaîne de `renderReconfigSelectView` (ex. le breadcrumb), lancer `go test ./commands/ui/ -run TestGoldenReconfig` → doit FAIL ; annuler la modification → PASS. Consigner ce contrôle dans le rapport.

- [ ] **Step 5: Commit**

```bash
git add commands/ui/golden_select_test.go commands/ui/testdata/
git commit -m "test(ui): golden characterization of the three select views"
```

---

### Task 2: Moteur `selectableListView` + migration de la vue Reconfigure

**Files:**
- Create: `commands/ui/selectable_list_view.go`
- Modify: `commands/ui/view_reconfig.go` (remplacement de `updateReconfigSelect` :21-102, `renderGroupedBundleItems` :238-286, `renderReconfigSelectView` :288-376 par des wrappers/configs)

**Interfaces:**
- Consumes: `pageCursor`, `cursorForItem`, `renderedItem`, `scrollWindowVar`, `renderScrollbar`, `groupBundles`, `bundleGroup` (`list_component.go`) ; `textFilter`, `envFilterCycle` (`filter.go`) ; `keys`, `m.effectiveWidth()`, `m.effectiveContentHeight()`, `m.renderHeader`, `m.finalHelpText`.
- Produces (utilisés par Tasks 3-4) :
  - `type listViewConfig struct` (champs ci-dessous)
  - `func (m Model) updateSelectView(cfg listViewConfig, msg tea.KeyMsg) (tea.Model, tea.Cmd)`
  - `func (m Model) renderSelectView(cfg listViewConfig) string`
  - `func selectHelpLine(f *textFilter, ef *envFilterCycle, envHint string) string`
  - `type groupedItemsOpts struct { padAliases bool; annotate func(m *Model, b *config.Bundle, globalIdx int) string }`
  - `func renderGroupedItems(m *Model, groups []bundleGroup, cursor, contentWidth int, opts groupedItemsOpts) (int, []renderedItem)`

- [ ] **Step 1: Créer `commands/ui/selectable_list_view.go`**

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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
				w := lipgloss.Width(displayNameFromAlias(b.Alias, b.Name))
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

			displayName := displayNameFromAlias(b.Alias, b.Name)
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
```

**Point de vigilance iso-comportement** (à vérifier contre les goldens, pas à corriger dans les goldens) : dans l'original reconfig, la ligne sélectionnée est `selectedStyle.Render("  › " + displayName)` **sans** pad (`view_reconfig.go:273`) et l'annotation env est `" " + envStyle.Render("[...]")` avec l'espace DANS l'annotation ; dans promote le pad remplace cet espace. Avec `padAliases:false` et l'annotation reconfig préfixée d'un espace, la sortie est identique à l'original — c'est le contrat.

- [ ] **Step 2: Migrer Reconfigure**

Dans `view_reconfig.go` :

1. Ajouter la config et les helpers par vue :

```go
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
```

**Attention aux receivers** : `m.updateError(err)` et `m.loadReconfigBundle(...)` — vérifier leurs receivers actuels (`loadReconfigBundle` est `*Model`, `updateError` à vérifier) ; dans un callback `func(m *Model)`, `return *m, nil` retourne la copie mutée. Si `updateError` a un value receiver retournant `(tea.Model, tea.Cmd)`, `return m.updateError(err)` fonctionne tel quel sur `*m` auto-déréférencé — reproduire exactement le flux de l'original (`view_reconfig.go:92-99`).

2. Remplacer `updateReconfigSelect` (:21-102) par :

```go
func (m Model) updateReconfigSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return m.updateSelectView(reconfigListViewCfg, msg)
}
```

3. Remplacer `renderReconfigSelectView` (:288-376) par :

```go
func (m Model) renderReconfigSelectView() string {
	return m.renderSelectView(reconfigListViewCfg)
}
```

4. Supprimer `renderGroupedBundleItems` (:238-286). Nettoyer les imports devenus inutiles.

- [ ] **Step 3: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/`
Expected: PASS — en particulier les 3 goldens `reconfig-select-*` **inchangés** et `TestUpdateReconfigSelectFilter` vert sans modification.

- [ ] **Step 4: Commit**

```bash
git add commands/ui/
git commit -m "refactor(ui): introduce selectableListView engine, migrate reconfigure view"
```

---

### Task 3: Migration de la vue Promote

**Files:**
- Modify: `commands/ui/view_promote.go` (remplacement de `updatePromoteSelect` :21-103, `renderPromoteGroupedItems` :429-497, `renderPromoteSelectView` :303-387)

**Interfaces:**
- Consumes: tout Task 2 (`listViewConfig`, `updateSelectView`, `renderSelectView`, `selectHelpLine`, `renderGroupedItems`, `groupedItemsOpts`).
- Produces: `promoteListViewCfg`, annotation `promoteEnvFlow`.

- [ ] **Step 1: Config et helpers Promote**

```go
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
```

- [ ] **Step 2: Remplacer les 3 fonctions**

`updatePromoteSelect` → wrapper une ligne sur `m.updateSelectView(promoteListViewCfg, msg)` ; `renderPromoteSelectView` → wrapper sur `m.renderSelectView(promoteListViewCfg)` ; supprimer `renderPromoteGroupedItems` (:429-497). Nettoyer les imports.

- [ ] **Step 3: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/`
Expected: PASS — goldens `promote-select-*` **inchangés**, `TestBuildAllPromoteBundlesCombinesEnvAndTextFilter` intact.

- [ ] **Step 4: Commit**

```bash
git add commands/ui/
git commit -m "refactor(ui): migrate promote view to selectableListView"
```

---

### Task 4: Migration de la vue Create-Select (mode flat)

**Files:**
- Modify: `commands/ui/view_create_select.go` (remplacement de `updateCreateSelect` :79-144, `renderBundleSelectView` :507-541, `renderFlatBundleList` :567-595)

**Interfaces:**
- Consumes: Task 2. `buildFlatBundleItems`, `flatBundleListHeader`, `flatBundleFilterHelp`, `selectFlatBundle` restent tels quels (spécifiques au flat).
- Produces: `createSelectListViewCfg`.

- [ ] **Step 1: Config Create-Select**

```go
var createSelectListViewCfg = listViewConfig{
	breadcrumb: func(m *Model) string { return "Scaffold Bundle Instance" },
	helpLine:   func(m *Model) string { return m.flatBundleFilterHelp() },
	listHeader: func(m *Model, innerWidth int) string { return m.flatBundleListHeader(innerWidth) },
	buildItems: func(m *Model, contentWidth int) (int, []renderedItem) {
		items := buildFlatBundleItems(m.flatBundles, m.flatBundleCursor, contentWidth)
		return m.flatBundleCursor, items
	},
	itemCount:   func(m *Model) int { return len(m.flatBundles) },
	cursor:      func(m *Model) int { return m.flatBundleCursor },
	setCursor:   func(m *Model, c int) { m.flatBundleCursor = c },
	textFilter:  func(m *Model) *textFilter { return &m.flatBundleFilter },
	applyFilter: func(m *Model) { m.applyFlatBundleFilter() },
	envFilter:   func(m *Model) *envFilterCycle { return nil },
	onCursorMove: func(m *Model) { m.bundleSelectErr = "" },
	onEnter:     func(m *Model) (tea.Model, tea.Cmd) { return m.selectFlatBundle() },
	exit:        func(m *Model) { m.viewState = ViewOverview },
	sep:         1,
	grouped:     false,
}
```

**Vigilance iso-comportement** :
- L'original clear `m.bundleSelectErr` sur PgUp/PgDn **et** sur Up/Down (dans la branche du if), pas sur Enter/`/` — le moteur reproduit exactement cela via `onCursorMove` (voir Task 2 Step 1). L'original clear aussi l'erreur dans `applyFlatBundleFilter` (`view_create_select.go:91`) — inchangé, la fonction est réutilisée telle quelle.
- L'original ne gère pas la touche `e` (elle tombait dans le `switch` sans case → `return m, nil` inchangé). Le moteur avec `envFilter` nil fait exactement cela.
- Scrollbar : l'original teste `len(m.flatBundles) > end-start` là où le moteur teste `len(items) > visibleCount` — équivalent car `buildFlatBundleItems` produit exactement un item par entrée. Les goldens le confirment.

- [ ] **Step 2: Remplacer les fonctions**

`updateCreateSelect` → wrapper sur `m.updateSelectView(createSelectListViewCfg, msg)` ; `renderBundleSelectView` → wrapper sur `m.renderSelectView(createSelectListViewCfg)` ; supprimer `renderFlatBundleList` (:567-595). `flatBundleFilterHelp`, `flatBundleListHeader`, `buildFlatBundleItems`, `selectFlatBundle` sont conservés. Nettoyer les imports.

- [ ] **Step 3: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/`
Expected: PASS — goldens `create-select-*` **inchangés**, `TestUpdateCreateSelectFilter` et `TestBuildFlatBundleItems` verts sans modification.

- [ ] **Step 4: Commit**

```bash
git add commands/ui/
git commit -m "refactor(ui): migrate create-select view to selectableListView"
```

---

### Task 5: Vérification de fin de phase

**Files:** aucun (vérification seule).

**Interfaces:**
- Consumes: tout le travail des tasks 1-4.
- Produces: Phase 2 livrée.

- [ ] **Step 1: Build + suite complète + lint (séquentiellement)**

Run: `make build`, puis `LC_ALL=C make test`, puis `make lint/all`
Expected: verts (modulo l'artefact TSan/noyau connu — re-vérifier les packages tués sans `-race` et le documenter).

- [ ] **Step 2: Contrôle du gain net**

Run: `git diff --stat refactor/maintenabilite..HEAD -- 'commands/ui/*.go' ':!*_test.go'`
Expected: net négatif d'au moins ~250 lignes sur le code de production UI (les 3 switchs clavier + 2 renderers groupés + 3 squelettes de rendu fusionnés).

- [ ] **Step 3: Smoke test binaire sur données réelles**

Compiler le binaire (`go build -o <scratchpad>/terramate-p2 ./cmd/terramate`) et re-dérouler la comparaison de la Phase 1 sur les copies du dépôt `infrastructure-infra-terraform-scw` (list/generate/fmt/run-graph/script list) : sorties attendues identiques à la base. (La TUI elle-même est couverte par les goldens ; une vérification manuelle interactive de `terramate ui` par l'utilisateur est recommandée en fin de phase — la proposer dans le rapport final.)
