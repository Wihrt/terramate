# Phase 1 — Factorisations sûres : Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Éliminer les duplications mécaniques de la TUI (helpers de liste, pagination, filtres, boilerplate widgets) et les factorisations mineures hors UI (generateBundleStack, await keys, tests BufferGroup), en refactoring iso-comportement validé par les tests existants.

**Architecture:** Aucun changement de comportement. Les fonctions byte-identiques des 3 vues (`view_create_select.go`, `view_promote.go`, `view_reconfig.go`) sont fusionnées dans deux nouveaux fichiers `commands/ui/list_component.go` (liste/pagination/rendu) et `commands/ui/filter.go` (états de filtre). Un `baseWidget` embarqué supprime le boilerplate des widgets. Hors UI : découpage de `generateBundleStack`, helper d'await key, renforcement des tests `BufferGroup`. Spec : `docs/superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md` (Phase 1).

**Tech Stack:** Go 1.25.8, Bubble Tea/lipgloss (TUI), make + mise.

## Global Constraints

- Refactoring **iso-comportement** strict : aucune fonctionnalité ajoutée ni modifiée ; les tables de cas des tests existants sont conservées à l'identique (elles sont la preuve d'iso-comportement).
- Tout fichier Go doit porter l'en-tête copyright : `// Copyright 2026 Terramate GmbH` + `// SPDX-License-Identifier: MPL-2.0` pour les fichiers créés ; ne pas modifier les en-têtes existants (certains datent de 2025).
- Messages de commit : conventional commits (`refactor:`, `test:`), en anglais, footer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Branche de travail : `refactor/phase1-factorisations` (empilée sur `refactor/maintenabilite`, PR #7 encore ouverte).
- Après chaque tâche UI : `go build ./commands/ui/` + `go test -race ./commands/ui/`. Fin de phase : `make build`, `LC_ALL=C make test`, `make lint/all` verts (le `LC_ALL=C` évite le faux négatif connu de `TestParallelBug1828Regression` sur machine en locale française).
- Chaque déplacement de code est un déplacement **à l'identique** (mêmes corps, mêmes commentaires) sauf renommage explicitement spécifié.

---

### Task 1: `list_component.go` — helpers de liste partagés + pagination unifiée

**Files:**
- Create: `commands/ui/list_component.go`
- Create: `commands/ui/list_component_test.go`
- Modify: `commands/ui/view_create_select.go` (retraits + 1 call site)
- Modify: `commands/ui/view_promote.go` (retraits + 1 call site)
- Modify: `commands/ui/view_reconfig.go` (retraits + 1 call site)
- Modify: `commands/ui/view_create_select_test.go`, `commands/ui/view_promote_test.go`, `commands/ui/view_reconfig_test.go`

**Interfaces:**
- Consumes: types/fonctions existants du package `ui` (déplacés, pas modifiés).
- Produces: `pageCursor(items []renderedItem, cursor, availableHeight, sep int, down bool) int` et `cursorForItem(items []renderedItem, itemIdx int) int` — utilisés par les 3 vues (et par la Phase 2 à venir).

- [ ] **Step 1: Créer `commands/ui/list_component.go` et y déplacer à l'identique les helpers partagés**

En-tête du fichier :

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui
```

Déplacer (corps et commentaires inchangés, imports ajustés) :

| Symbole | Source actuelle |
|---|---|
| `type renderedItem` | `view_create_select.go:879` |
| `firstSelectableIndex` | `view_create_select.go:886` |
| `lastSelectableIndex` | `view_create_select.go:896` |
| `scrollWindowVar` | `view_create_select.go:908` |
| `renderScrollbar` | `view_create_select.go:948` |
| `type detailField` | `view_create_select.go:572` |
| `renderDetailBox` | `view_create_select.go:580` |
| `renderErrorBox` | `view_create_select.go:670` |
| `truncateEnd` | `view_create_select.go:719` |
| `truncateStart` | `view_create_select.go:731` |
| `type bundleGroup` | `view_reconfig.go:283` |
| `groupBundles` | `view_reconfig.go:292` |

- [ ] **Step 2: Ajouter les fonctions unifiées de pagination dans `list_component.go`**

Les 3 fonctions `flatBundlePageCursor` (`view_create_select.go:858`), `promotePageCursor` (`view_promote.go:474`) et `reconfigPageCursor` (`view_reconfig.go:527`) sont **byte-identiques** ; `promoteCursorForItem` (`view_promote.go:498`) et `reconfigCursorForItem` (`view_reconfig.go:551`) aussi. Les remplacer par :

```go
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
```

- [ ] **Step 3: Supprimer les 5 variantes et mettre à jour les call sites**

- Supprimer `flatBundlePageCursor`, `promotePageCursor`, `promoteCursorForItem`, `reconfigPageCursor`, `reconfigCursorForItem`.
- `view_create_select.go:136` : `flatBundlePageCursor(...)` → `pageCursor(...)`.
- `view_promote.go:69-70` : `promotePageCursor(...)` → `pageCursor(...)` ; `promoteCursorForItem(...)` → `cursorForItem(...)`.
- `view_reconfig.go:71-72` : `reconfigPageCursor(...)` → `pageCursor(...)` ; `reconfigCursorForItem(...)` → `cursorForItem(...)`.

- [ ] **Step 4: Consolider les tests de pagination**

Créer `commands/ui/list_component_test.go` (en-tête copyright 2026) et y fusionner les tests existants **en conservant l'union de leurs tables de cas à l'identique** :

- `TestPromotePageCursor` (`view_promote_test.go`), `TestReconfigPageCursor` (`view_reconfig_test.go`), `TestFlatBundlePageCursor` + `TestFlatBundlePageCursorEmptyList` (`view_create_select_test.go`) → un seul `TestPageCursor` (+ `TestPageCursorEmptyList`) appelant `pageCursor`. Si deux tables contiennent des cas identiques, dédupliquer ; si un cas n'existe que dans une table, le garder.
- `TestPromoteCursorForItem` + `TestReconfigCursorForItem` → un seul `TestCursorForItem` appelant `cursorForItem`.
- Supprimer les tests d'origine des 3 fichiers de test.

- [ ] **Step 5: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/`
Expected: build OK, tous les tests PASS (aucun test perdu : comparer `go test ./commands/ui/ -list '.*' | wc -l` avant/après — la baisse doit correspondre exactement aux fusions de ce step).

- [ ] **Step 6: Commit**

```bash
git add commands/ui/
git commit -m "refactor(ui): extract shared list helpers into list_component.go"
```

---

### Task 2: `filter.go` — états de filtre unifiés (`textFilter`, `envFilterCycle`, `bundleMatchesFilter`)

**Files:**
- Create: `commands/ui/filter.go`
- Create: `commands/ui/filter_test.go`
- Modify: `commands/ui/model.go:139-172` (champs), `commands/ui/view_create_select.go`, `commands/ui/view_promote.go`, `commands/ui/view_reconfig.go`, `commands/ui/view_overview.go:130-148`, les 3 fichiers de test.

**Interfaces:**
- Consumes: `envFilterState` (`model.go:79`, inchangé), `textinput.Model` (Bubble Tea).
- Produces:
  - `type textFilter struct { input textinput.Model; editing bool }`, `newTextFilter() textFilter`, méthode `query() string`
  - `type envFilterCycle struct { filters []envFilterState; pos int }`, méthodes `current() *envFilterState`, `nextName() string`
  - `bundleMatchesFilter(b *config.Bundle, query string) bool`

- [ ] **Step 1: Créer `commands/ui/filter.go`**

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"

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
	return strings.Contains(strings.ToLower(displayNameFromAlias(b.Alias, b.Name)), q)
}
```

- [ ] **Step 2: Remplacer les triplets dupliqués — remplacements mécaniques (le compilateur signale tout site oublié)**

Suppressions :
- `flatFilterState` + `newFlatFilterState` (`view_create_select.go:58-69`)
- `promoteFilterState` + `newPromoteFilterState` (`view_promote.go:113-124`), `promoteBundleMatchesFilter` (`view_promote.go:129`), `currentPromoteFilter` (`view_promote.go:171`), `nextPromoteFilterName` (`view_promote.go:179`)
- `reconfigFilterState` + `newReconfigFilterState` (`view_reconfig.go:114-125`), `reconfigBundleMatchesFilter` (`view_reconfig.go:130`), `currentReconfigFilter` (`view_reconfig.go:172`), `nextReconfigFilterName` (`view_reconfig.go:180`)

`flatBundleMatchesFilter` (`view_create_select.go:73`) est **conservé** : il filtre des `flatBundleEntry` sur `bundle.Name` seul — sémantique différente.

Champs de `Model` (`model.go`) :

| Avant | Après |
|---|---|
| `flatBundleFilter flatFilterState` | `flatBundleFilter textFilter` |
| `reconfigFilter reconfigFilterState` | `reconfigFilter textFilter` |
| `promoteFilter promoteFilterState` | `promoteFilter textFilter` |
| `reconfigFilters []envFilterState` + `reconfigFilterPos int` | `reconfigEnvFilter envFilterCycle` |
| `promoteFilters []envFilterState` + `promoteFilterPos int` | `promoteEnvFilter envFilterCycle` |

Renommage des usages (dans les 3 vues, `view_overview.go` et les tests) :
- `newPromoteFilterState()` / `newReconfigFilterState()` / `newFlatFilterState()` → `newTextFilter()`
- `m.promoteFilters` → `m.promoteEnvFilter.filters` ; `m.promoteFilterPos` → `m.promoteEnvFilter.pos` (idem reconfig)
- `m.currentPromoteFilter()` → `m.promoteEnvFilter.current()` ; `m.nextPromoteFilterName()` → `m.promoteEnvFilter.nextName()` (idem reconfig)
- `promoteBundleMatchesFilter(...)` / `reconfigBundleMatchesFilter(...)` → `bundleMatchesFilter(...)`
- `strings.TrimSpace(m.promoteFilter.input.Value())` → `m.promoteFilter.query()` (idem reconfig et flat, ex. `view_create_select.go:83`, `view_promote.go:247`)

`buildPromoteFilters` / `buildReconfigFilters` restent dans leurs vues (logiques réellement différentes) ; leurs résultats alimentent `m.xEnvFilter.filters`.

- [ ] **Step 3: Consolider les tests de filtre**

Créer `commands/ui/filter_test.go` (copyright 2026) : fusionner `TestPromoteBundleMatchesFilter` et `TestReconfigBundleMatchesFilter` (tables identiques ou quasi — union des cas, à l'identique) en un `TestBundleMatchesFilter` appelant `bundleMatchesFilter` ; supprimer les originaux. `TestFlatBundleMatchesFilter` reste inchangé. Les autres tests (`TestBuildAllPromoteBundlesCombinesEnvAndTextFilter`, `TestBuildReconfigBundlesCombinesEnvAndTextFilter`, `TestUpdateCreateSelectFilter`, `TestUpdateReconfigSelectFilter`) sont adaptés aux nouveaux noms de champs **sans changer leurs assertions**.

- [ ] **Step 4: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/`
Expected: PASS, assertions inchangées.

- [ ] **Step 5: Commit**

```bash
git add commands/ui/
git commit -m "refactor(ui): unify filter state into textFilter and envFilterCycle"
```

---

### Task 3: `baseWidget` — suppression du boilerplate des widgets

**Files:**
- Modify: `commands/ui/widget.go` (ajout du type), `commands/ui/widget_primitive.go`, `commands/ui/widget_select.go`, `commands/ui/widget_list.go`, `commands/ui/widget_map.go`, `commands/ui/widget_object.go`

**Interfaces:**
- Consumes: `InputWidget` (`widget.go:26`), `WidgetContext`, `SubFormResult` (existants).
- Produces: `type baseWidget struct { wctx *WidgetContext }` avec les implémentations par défaut, embarqué par tous les widgets.

- [ ] **Step 1: Ajouter `baseWidget` dans `widget.go`** (après la définition de l'interface `InputWidget`)

```go
// baseWidget carries the widget context and the default implementations of
// the InputWidget methods most widgets share. Widgets embed it and override
// only the methods they actually customize.
type baseWidget struct {
	wctx *WidgetContext
}

// WidgetContext returns the widget's context.
func (w *baseWidget) WidgetContext() *WidgetContext { return w.wctx }

// ForwardMsg ignores forwarded messages by default.
func (w *baseWidget) ForwardMsg(tea.Msg) tea.Cmd { return nil }

// AcceptSubFormResult accepts any sub-form result by default.
func (w *baseWidget) AcceptSubFormResult(SubFormResult) bool { return true }
```

- [ ] **Step 2: Embarquer `baseWidget` dans chaque widget — transformation exacte, widget par widget**

Règle appliquée aux 11 widgets (`TextWidget`, `BoolWidget`, `MultilineWidget` dans `widget_primitive.go` ; `SelectWidget`, `MultiSelectWidget`, `BundleRefWidget` dans `widget_select.go` ; `InlineListWidget`, `SubFormListWidget` dans `widget_list.go` ; `InlineMapWidget`, `SubFormMapWidget` dans `widget_map.go` ; `ObjectWidget` dans `widget_object.go`) :

1. Dans la struct : remplacer le champ `wctx *WidgetContext` par un champ embarqué `baseWidget` (en première position). Les accès internes `w.wctx` continuent de compiler (champ promu).
2. Dans le constructeur : `XWidget{wctx: wctx, ...}` → `XWidget{baseWidget: baseWidget{wctx: wctx}, ...}`. Exemple complet (`widget_primitive.go:48`) :

```go
	return &TextWidget{
		baseWidget: baseWidget{wctx: wctx},
		valueType:  valueType,
		textInput:  ti,
		numberMode: numberMode,
	}
```

3. Supprimer la méthode `WidgetContext()` du widget (toutes sont identiques : `return w.wctx`).
4. Supprimer `ForwardMsg` **uniquement si** son corps est exactement le no-op `return nil` ; sinon la conserver (elle masque le défaut de l'embed).
5. Supprimer `AcceptSubFormResult` **uniquement si** son corps est exactement `return true` (cas connus : `InlineListWidget:306`, `InlineMapWidget:481`, `TextWidget:144`, `BoolWidget:229`, `MultilineWidget:414`, `BundleRefWidget:423`) ; conserver les implémentations réelles (`SubFormListWidget:448`, `SubFormMapWidget:751`, `ObjectWidget:122`, et `SelectWidget:124`/`MultiSelectWidget:279` si leur corps n'est pas `return true` — vérifier chaque corps avant suppression).

- [ ] **Step 3: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/ && go vet ./commands/ui/`
Expected: PASS. Contrôle du gain : `git diff --stat` doit montrer un net négatif (~80-120 lignes supprimées).

- [ ] **Step 4: Commit**

```bash
git add commands/ui/
git commit -m "refactor(ui): embed baseWidget to remove widget boilerplate"
```

---

### Task 4: Découper `generateBundleStack` (generate/generate_bundle.go)

**Files:**
- Modify: `generate/generate_bundle.go:29-119`

**Interfaces:**
- Consumes: `gState`, `stack.UpdateMetadata`, `stack.Create`, `mergeComponentList` (existants).
- Produces: `(*gState) updateExistingBundleStack(...)`, `(*gState) createBundleStack(...)`, `attachBundleComponents(stackTree *config.Tree, stackMeta config.StackMetadata, bundle *config.Bundle)` — usage interne au package.

- [ ] **Step 1: Restructurer**

Remplacer `generateBundleStack` (lignes 29-119) par :

```go
func (g *gState) generateBundleStack(bundle *config.Bundle, stackMeta config.StackMetadata, report *genreport.Report, allowCreate bool) {
	logger := log.With().
		Str("action", "generate.generateBundleStack()").
		Str("bundle", bundle.Name).
		Str("stack", stackMeta.Name).
		Logger()

	if stackMeta.Skipped {
		logger.Debug().Msg("skipping stack because of condition attribute")
		return
	}

	stackTree, ok := g.root.Lookup(stackMeta.Dir)
	if ok && stackTree.IsStack() {
		g.updateExistingBundleStack(logger, bundle, stackMeta, stackTree, report)
		return
	}

	if !allowCreate {
		report.AddFailure(stackMeta.Dir, errors.E("stack not generated"))
		return
	}

	g.createBundleStack(logger, bundle, stackMeta, report)
}

// updateExistingBundleStack refreshes the stack metadata of an already
// existing stack from its bundle definition and attaches the bundle's
// runtime components.
func (g *gState) updateExistingBundleStack(logger zerolog.Logger, bundle *config.Bundle, stackMeta config.StackMetadata, stackTree *config.Tree, report *genreport.Report) {
	stackFilePath := filepath.Join(stackMeta.Dir.HostPath(g.root.HostDir()), stack.DefaultFilename)
	if _, statErr := os.Lstat(stackFilePath); statErr != nil {
		logger.Debug().Msg("stack already exists but stack.tm.hcl not found: skipping metadata update")
		attachBundleComponents(stackTree, stackMeta, bundle)
		return
	}
	logger.Debug().Msg("stack already exists: updating metadata from bundle")
	changed, err := stack.UpdateMetadata(g.root, stackMeta)
	if err != nil {
		report.AddFailure(stackMeta.Dir, err)
		return
	}
	if changed {
		dirReport := genreport.Dir{}
		dirReport.AddChangedFile(stack.DefaultFilename)
		report.AddDirReport(stackMeta.Dir, dirReport)
	}
	attachBundleComponents(stackTree, stackMeta, bundle)
}

// createBundleStack creates a new stack on disk from the bundle's stack
// metadata, loads it into the config tree and attaches the bundle's runtime
// components.
func (g *gState) createBundleStack(logger zerolog.Logger, bundle *config.Bundle, stackMeta config.StackMetadata, report *genreport.Report) {
	watch := make(project.Paths, len(stackMeta.Watch))
	for i, w := range stackMeta.Watch {
		watch[i] = project.NewPath(w)
	}

	stackCfg := config.Stack{
		Dir:         stackMeta.Dir,
		ID:          uuid.NewString(),
		Name:        stackMeta.Name,
		Description: stackMeta.Description,
		Tags:        stackMeta.Tags,
		After:       stackMeta.After,
		Before:      stackMeta.Before,
		Wants:       stackMeta.Wants,
		WantedBy:    stackMeta.WantedBy,
		Watch:       watch,
	}

	logger.Debug().Msg("creating stack")

	err := stack.Create(g.root, stackCfg)
	if err != nil {
		report.AddFailure(stackMeta.Dir, err)
		return
	}

	logger.Debug().Msg("loading stack")

	err = g.root.LoadSubTree(stackMeta.Dir)
	if err != nil {
		report.AddFailure(stackMeta.Dir, err)
		return
	}

	logger.Debug().Msg("stack loaded successfully")

	stackTree, ok := g.root.Lookup(stackMeta.Dir)
	if !ok {
		panic(errors.E(errors.ErrInternal, "just created stack %s cannot be loaded", stackMeta.Dir))
	}

	logger.Debug().Msg("adding created file to report")

	dirReport := genreport.Dir{}
	dirReport.AddCreatedFile(stack.DefaultFilename)
	report.AddDirReport(stackMeta.Dir, dirReport)

	attachBundleComponents(stackTree, stackMeta, bundle)
}

// attachBundleComponents attaches the bundle's runtime components to the
// stack's config node.
func attachBundleComponents(stackTree *config.Tree, stackMeta config.StackMetadata, bundle *config.Bundle) {
	stackTree.Node.Components = mergeComponentList(stackTree.Node.Components, stackMeta.Components, bundle.Source)
}
```

Ajouter l'import `"github.com/rs/zerolog"` (le package importe déjà `zerolog/log`).

- [ ] **Step 2: Vérifier**

Run: `go build ./generate/ && go test -race ./generate/...`
Expected: PASS (la génération bundle→stack est couverte par les tests existants du package).

- [ ] **Step 3: Commit**

```bash
git add generate/generate_bundle.go
git commit -m "refactor(generate): split generateBundleStack into focused helpers"
```

---

### Task 5: Helper d'await key (config/hcl_func.go)

**Files:**
- Modify: `config/hcl_func.go:103-132`
- Create ou Modify: `config/hcl_func_test.go` (créer si absent, en-tête copyright 2026, `package config`)

**Interfaces:**
- Produces: `bundleAwaitKey(classScoped, isUUID bool, class, key, envID string) string` — usage interne au package.

- [ ] **Step 1: Écrire le test (TDD)**

Dans `config/hcl_func_test.go` :

```go
func TestBundleAwaitKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		classScoped bool
		isUUID      bool
		want        string
	}{
		{name: "plain alias", classScoped: false, isUUID: false, want: "env1:web"},
		{name: "class-scoped alias", classScoped: true, isUUID: false, want: "env1:vpc:web"},
		{name: "plain uuid", classScoped: false, isUUID: true, want: "env1:web"},
		{name: "class-scoped uuid", classScoped: true, isUUID: true, want: "env1:vpc:web"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := bundleAwaitKey(tc.classScoped, tc.isUUID, "vpc", "web", "env1")
			if got != tc.want {
				t.Fatalf("bundleAwaitKey(%v, %v) = %q, want %q", tc.classScoped, tc.isUUID, got, tc.want)
			}
		})
	}
}
```

Run: `go test ./config/ -run TestBundleAwaitKey`
Expected: FAIL — `undefined: bundleAwaitKey`.

- [ ] **Step 2: Implémenter le helper et l'utiliser**

Dans `hcl_func.go`, après `BundleClassUUIDAwaitKey` :

```go
// bundleAwaitKey computes the preempt await key for key (an alias, or a UUID
// when isUUID is true). When classScoped is true the class-scoped variant is
// used, so bundles sharing the same key but a different class cannot
// prematurely unblock the waiter.
func bundleAwaitKey(classScoped, isUUID bool, class, key, envID string) string {
	switch {
	case classScoped && isUUID:
		return BundleClassUUIDAwaitKey(class, key, envID)
	case classScoped:
		return BundleClassAliasAwaitKey(class, key, envID)
	case isUUID:
		return BundleUUIDAwaitKey(key, envID)
	default:
		return BundleAliasAwaitKey(key, envID)
	}
}
```

Dans `BundleFunc.Impl` (lignes 103-132), remplacer les deux branchements dupliqués par :

```go
			var keyKind string
			var pred func(*Bundle) bool

			isUUID := uuid.Validate(key) == nil
			if isUUID {
				keyKind = "UUID"
				pred = func(b *Bundle) bool {
					return b.UUID == key
				}
			} else {
				keyKind = "alias"
				pred = func(b *Bundle) bool {
					return b.Alias == key
				}
			}
			awaitKey := bundleAwaitKey(useAwait, isUUID, class, key, envID)
```

(Le commentaire « Use the class-scoped key so bundles with the same … » vit désormais sur le helper ; ne pas le dupliquer aux call sites.)

- [ ] **Step 3: Vérifier**

Run: `go test -race ./config/...`
Expected: PASS, y compris `TestBundleAwaitKey`.

- [ ] **Step 4: Commit**

```bash
git add config/hcl_func.go config/hcl_func_test.go
git commit -m "refactor(config): factor await-key selection into bundleAwaitKey"
```

---

### Task 6: Renforcer les tests de `BufferGroup` (engine/buffer_group_test.go)

**Files:**
- Modify: `engine/buffer_group_test.go`

**Interfaces:**
- Consumes: `NewBufferGroup`, `(*BufferGroup).NewBuffer`, `(*BufferGroup).Wait`, `readLines` (existants, `engine/buffer_group.go`). **Aucun changement de code de production.**

- [ ] **Step 1: Ajouter les tests suivants** (le `lockedWriter` existant est réutilisé)

```go
func TestBufferGroupReassemblesPartialWrites(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w := bg.NewBuffer(safeOut)

	for _, chunk := range []string{"hel", "lo\nwor", "ld\n"} {
		if _, err := w.Write([]byte(chunk)); err != nil {
			t.Fatal(err)
		}
	}
	bg.Wait()

	if got := out.String(); got != "hello\nworld\n" {
		t.Fatalf("expected reassembled lines, got: %q", got)
	}
}

func TestBufferGroupFlushesTrailingDataWithoutNewline(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w := bg.NewBuffer(safeOut)

	if _, err := w.Write([]byte("no trailing newline")); err != nil {
		t.Fatal(err)
	}
	bg.Wait()

	if got := out.String(); got != "no trailing newline" {
		t.Fatalf("expected trailing data flushed on EOF, got: %q", got)
	}
}

func TestBufferGroupInterleavedPartialWritesKeepLinesIntact(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w1 := bg.NewBuffer(safeOut)
	w2 := bg.NewBuffer(safeOut)

	writes := []struct {
		w     io.Writer
		chunk string
	}{
		{w1, "a"}, {w2, "x"}, {w1, "b\n"}, {w2, "y\n"},
	}
	for _, wr := range writes {
		if _, err := wr.w.Write([]byte(wr.chunk)); err != nil {
			t.Fatal(err)
		}
	}
	bg.Wait()

	got := out.String()
	if got != "ab\nxy\n" && got != "xy\nab\n" {
		t.Fatalf("expected complete lines ab/xy in either order, got: %q", got)
	}
}

// failingWriter fails every write, exercising the error-logging path of the
// buffer goroutine.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("sink failed")
}

func TestBufferGroupWriteErrorDoesNotDeadlock(t *testing.T) {
	t.Parallel()

	bg := NewBufferGroup()
	w := bg.NewBuffer(failingWriter{})

	if _, err := w.Write([]byte("line\n")); err != nil {
		t.Fatal(err)
	}
	// Wait must return despite the sink error (which is only logged).
	bg.Wait()
}

// misbehavingReader returns (0, nil), which readLines must treat as EOF
// instead of spinning forever.
type misbehavingReader struct{}

func (misbehavingReader) Read([]byte) (int, error) { return 0, nil }

func TestReadLinesMisbehavingReaderReturnsEOF(t *testing.T) {
	t.Parallel()

	lines, rest, err := readLines(misbehavingReader{}, nil)
	if err != io.EOF {
		t.Fatalf("expected io.EOF for misbehaving reader, got: %v", err)
	}
	if lines != nil || rest != nil {
		t.Fatalf("expected no data, got lines=%v rest=%q", lines, rest)
	}
}

func TestReadLinesKeepsPendingAcrossCalls(t *testing.T) {
	t.Parallel()

	lines, rest, err := readLines(strings.NewReader("tail"), []byte("head-"))
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got: %v", err)
	}
	if len(lines) != 0 {
		t.Fatalf("expected no complete line, got: %q", lines)
	}
	if string(rest) != "head-tail" {
		t.Fatalf("expected pending bytes preserved, got: %q", rest)
	}
}
```

Imports à ajouter au fichier de test : `"errors"`, `"io"`, `"strings"`.

- [ ] **Step 2: Vérifier (les tests doivent passer immédiatement — ils caractérisent le comportement actuel ; un échec = découverte à remonter, pas à corriger silencieusement)**

Run: `go test -race -count=1 ./engine/ -run 'TestBufferGroup|TestReadLines'`
Expected: PASS. En cas de FAIL : STOP, rapporter le comportement observé (c'est peut-être un bug réel de `BufferGroup` — décision au contrôleur, pas de modification du code de production dans cette tâche).

- [ ] **Step 3: Commit**

```bash
git add engine/buffer_group_test.go
git commit -m "test(engine): strengthen BufferGroup coverage (partial lines, EOF, errors)"
```

---

### Task 7: Vérification de fin de phase

**Files:** aucun (vérification seule).

**Interfaces:**
- Consumes: tout le travail des tasks 1-6.
- Produces: Phase 1 livrée — build, tests, lint verts.

- [ ] **Step 1: Build complet**

Run: `make build`
Expected: succès.

- [ ] **Step 2: Suite de tests complète**

Run: `LC_ALL=C make test`
Expected: PASS (le `LC_ALL=C` neutralise le test sensible à la locale ; ne pas lancer lint en parallèle de cette étape).

- [ ] **Step 3: Lint**

Run: `make lint/all`
Expected: 0 issue.

- [ ] **Step 4: Contrôle du gain net**

Run: `git diff --stat refactor/maintenabilite..HEAD | tail -1`
Expected: net négatif sur le code de production (~250-400 lignes supprimées au total, hors nouveaux tests).
