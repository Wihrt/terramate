# Phase 3a — Tests de caractérisation (`inputs_form.go`, `change.go`) : Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Figer le comportement observable actuel des ~2 650 lignes non testées de `commands/ui/inputs_form.go` (state machine du formulaire, rendu) et `commands/ui/change.go` (génération/écriture YAML des changements de bundles) AVANT toute restructuration des phases 3b-3d.

**Architecture:** Tests de caractérisation purs : aucun changement de code de production. Trois angles : (1) la state machine d'`InputsForm` pilotée en mémoire (constructeurs + séquences de touches → assertions sur `State()`/`Values()`), (2) des goldens de `View()` (harnais `assertGolden` de la Phase 2 réutilisé), (3) `change.go` testé par ses fonctions d'entrée (`(*Change).Save` en standalone sur `t.TempDir()`, puis le chemin complet `NewCreateChange`→`Save`→`NewReconfigChange`/`NewPromoteChange` sur sandbox). Un échec de test = découverte d'un comportement, jamais une correction silencieuse. Spec : `docs/superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md` (Phase 3, étape 1).

**Tech Stack:** Go 1.25.8, Bubble Tea (KeyMsg synthétiques), `test/sandbox`, goldens testdata/.

## Global Constraints

- **AUCUN changement de code de production.** Si un test révèle un comportement surprenant (ex. la bizarrerie de dédup de `mergeBundleYAMLEnv`), on le FIGE tel quel avec un commentaire `// Characterizes current behavior: ...` — la décision de le corriger appartient aux phases suivantes.
- Si un test échoue de façon inattendue : STOP, rapporter le comportement observé vs attendu (DONE_WITH_CONCERNS/BLOCKED) — ne pas modifier le test pour le faire passer sans comprendre, ne pas toucher la production.
- Aléa contrôlé : `NewCreateChange` appelle `uuid.NewString()` (`change.go:164`) — les goldens de YAML issus de ce chemin normalisent l'UUID (remplacement regex par `<UUID>`), les `Change` construits à la main fixent un UUID littéral.
- Nouveaux fichiers : en-tête `// Copyright 2026 Terramate GmbH` + `// SPDX-License-Identifier: MPL-2.0`.
- Messages de commit : conventional commits (`test:`), en anglais, footer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Branche : `refactor/phase3a-characterization` (empilée sur `refactor/maintenabilite`).
- Après chaque tâche : `go build ./commands/ui/ && go test -race ./commands/ui/`. Fin de phase : `make build`, `LC_ALL=C make test`, `make lint/all` (les `signal: killed` de binaires `-race` = artefact TSan/noyau 7.0 connu — re-vérifier sans `-race`, ne pas traiter en régression).

## Références de fixtures (communes à toutes les tâches)

- **InputsForm en mémoire** (aucun disque) : `NewInputsForm(defs, typeschema.EvalContext{Evalctx: eval.NewContext(nil)}, &config.Registry{}, nil)`. Une définition d'input se construit en littéral : `&config.InputDefinition{Name: "x", Type: &typeschema.PrimitiveType{Name: "string"}, Prompt: config.PromptConfig{Text: "X?"}}` (précédent : `widget_list.go:487`). Les inputs sans `Prompt.Text` sont filtrés par `filterPrompted` (`inputs_form.go:254`).
- **Limite connue** : defaults/options/conditions de prompt reposent sur des champs non exportés (`defaultExpr`, `optionsExpr`, `conditionExpr` — `config/bundle.go:92-99,125-126`) → impossibles en littéral depuis `package ui`. Ces comportements sont couverts par la Task 4 (fixture HCL réelle) via `config.EvalBundleInputDefinitions`.
- **Fixture HCL réelle** : pattern de `config/bundle_test.go:20-35` — `sandbox.NoGit(t, true)` + `s.BuildTree([]string{"f:/bundles/a/define.tm:" + <HCL define bundle>})` + `config.LoadRoot(s.RootDir(), false)` + evalctx `eval.NewContext(stdlib.Functions(root.HostDir(), root.Tree().Node.Experiments()))` avec `evalctx.SetNamespace("terramate", root.Runtime())` + `config.ListLocalBundleDefinitions(root, evalctx, project.NewPath("/"))`.
- **Touches synthétiques** : `tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")}`, `tea.KeyMsg{Type: tea.KeyEnter}`, `{Type: tea.KeyTab}`, `{Type: tea.KeyEsc}`, `{Type: tea.KeyUp}`, `{Type: tea.KeyDown}` (précédent : `view_reconfig_test.go:53-69`).
- **Goldens** : harnais `assertGolden(t, name, got)` existant (`golden_select_test.go`), génération via `go test ./commands/ui/ -run <Test> -update`.

---

### Task 1: State machine d'`InputsForm` — caractérisation en mémoire

**Files:**
- Create: `commands/ui/inputs_form_test.go`

**Interfaces:**
- Consumes: `NewInputsForm` (`inputs_form.go:128`), `NewInputsFormWithValues` (`:174`), `(InputsForm).Update(tea.Msg) (InputsForm, tea.Cmd)` (`:825`), `State()` (`:491`), `Values()` (`:576`), `UserValues()` (`:587`).
- Produces: le harnais `newTestForm(t, defs...)` et `pressKeys(f, msgs...)` réutilisés par la Task 2.

- [ ] **Step 1: Écrire le harnais et les tests de cycle de vie**

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/terramate-io/terramate/typeschema"
)

func strInput(name, prompt string) *config.InputDefinition {
	return &config.InputDefinition{
		Name:   name,
		Type:   &typeschema.PrimitiveType{Name: "string"},
		Prompt: config.PromptConfig{Text: prompt},
	}
}

func boolInput(name, prompt string) *config.InputDefinition {
	return &config.InputDefinition{
		Name:   name,
		Type:   &typeschema.PrimitiveType{Name: "bool"},
		Prompt: config.PromptConfig{Text: prompt},
	}
}

func newTestForm(defs ...*config.InputDefinition) InputsForm {
	return NewInputsForm(defs, typeschema.EvalContext{Evalctx: eval.NewContext(nil)}, &config.Registry{}, nil)
}

func pressKeys(f InputsForm, msgs ...tea.Msg) InputsForm {
	for _, msg := range msgs {
		f, _ = f.Update(msg)
	}
	return f
}

func runes(s string) tea.Msg   { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func enter() tea.Msg           { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyOf(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func TestInputsFormFiltersUnpromptedInputs(t *testing.T) {
	t.Parallel()
	silent := &config.InputDefinition{Name: "silent", Type: &typeschema.PrimitiveType{Name: "string"}}
	f := newTestForm(strInput("shown", "Shown?"), silent)
	if got := len(f.InputDefs); got != 1 {
		t.Fatalf("expected only prompted inputs to be kept, got %d defs", got)
	}
	if f.InputDefs[0].Name != "shown" {
		t.Fatalf("expected 'shown' to survive filtering, got %q", f.InputDefs[0].Name)
	}
}

func TestInputsFormTypeConfirmAdvancesAndAccepts(t *testing.T) {
	t.Parallel()
	f := newTestForm(strInput("name", "Name?"), strInput("region", "Region?"))
	if f.State() != InputsFormActive {
		t.Fatalf("fresh form should be active, got %v", f.State())
	}

	// Fill input 1, confirm; fill input 2, confirm.
	f = pressKeys(f, runes("my-bundle"), enter(), runes("fr-par"), enter())
	if f.State() != InputsFormActive {
		t.Fatalf("form should still be active on the buttons panel, got %v", f.State())
	}

	// All inputs done: Enter on the (default) Confirm button accepts.
	f = pressKeys(f, enter())
	if f.State() != InputsFormAccepted {
		t.Fatalf("expected InputsFormAccepted after confirming, got %v", f.State())
	}

	vals := f.Values()
	if got := vals["name"]; got != cty.StringVal("my-bundle") {
		t.Fatalf("expected name=my-bundle, got %#v", got)
	}
	if got := vals["region"]; got != cty.StringVal("fr-par") {
		t.Fatalf("expected region=fr-par, got %#v", got)
	}
}

func TestInputsFormUserValuesOnlyContainUserSetKeys(t *testing.T) {
	t.Parallel()
	// Seed one value as pre-existing (reconfigure-style), set the other by typing.
	defs := []*config.InputDefinition{strInput("kept", "Kept?"), strInput("typed", "Typed?")}
	seeded := map[string]cty.Value{"kept": cty.StringVal("from-disk")}
	f := NewInputsFormWithValues(defs, typeschema.EvalContext{Evalctx: eval.NewContext(nil)}, &config.Registry{}, nil, nil, seeded, seeded, map[string]bool{"kept": true})

	f = pressKeys(f, enter())               // confirm "kept" (pre-filled)
	f = pressKeys(f, runes("abc"), enter()) // type + confirm "typed"

	user := f.UserValues()
	if _, ok := user["typed"]; !ok {
		t.Fatalf("expected typed key in UserValues, got %#v", user)
	}
	if _, ok := user["kept"]; !ok {
		t.Fatalf("expected user-flagged seeded key in UserValues, got %#v", user)
	}
}

func TestInputsFormDiscardFlow(t *testing.T) {
	t.Parallel()
	f := newTestForm(strInput("only", "Only?"))
	f = pressKeys(f, runes("v"), enter()) // reach the buttons panel

	// Navigate to the Discard button, then Enter twice (two-step confirm).
	f = pressKeys(f, keyOf(tea.KeyRight), enter())
	if f.State() == InputsFormDiscarded {
		t.Fatal("discard must require a confirmation step, got discarded on first Enter")
	}
	f = pressKeys(f, enter())
	if f.State() != InputsFormDiscarded {
		t.Fatalf("expected InputsFormDiscarded after confirming, got %v", f.State())
	}
}

func TestInputsFormTabTogglesFocusAndReeditWorks(t *testing.T) {
	t.Parallel()
	f := newTestForm(strInput("first", "First?"), strInput("second", "Second?"))
	f = pressKeys(f, runes("one"), enter()) // "first" completed, "second" active

	// Tab moves focus to the completed panel; Enter re-edits the entry under
	// the cursor; typing + Enter updates it and returns to the pending input.
	f = pressKeys(f, keyOf(tea.KeyTab))
	f = pressKeys(f, enter())            // re-enter "first"
	f = pressKeys(f, runes("!"), enter()) // append and confirm
	got := f.Values()["first"]
	if got != cty.StringVal("one!") {
		t.Fatalf("expected re-edited value 'one!', got %#v", got)
	}
}
```

**Règle d'adaptation bornée** : ces tests caractérisent — si une assertion échoue parce que le comportement réel diffère de l'attente écrite ici (ex. la navigation boutons demande `Down` au lieu de `Right`, la ré-édition insère au lieu d'apposer, `UserValues` exclut la clé seedée), l'implémenteur **lit le code** (`updateButtons` `inputs_form.go:1069`, `updateCompleted` `:907`, `ReenterAt`, `UserValues` `:587`), ajuste la séquence de touches ou l'assertion pour refléter le comportement RÉEL, et documente l'écart dans son rapport. Interdit : modifier la production, ou affaiblir une assertion en simple non-panic.

- [ ] **Step 2: Vérifier**

Run: `go build ./commands/ui/ && go test -race ./commands/ui/ -run TestInputsForm`
Expected: PASS (comportements réels figés).

- [ ] **Step 3: Commit**

```bash
git add commands/ui/inputs_form_test.go
git commit -m "test(ui): characterize InputsForm state machine (lifecycle, values, discard, re-edit)"
```

---

### Task 2: Goldens de `View()` d'`InputsForm`

**Files:**
- Modify: `commands/ui/inputs_form_test.go` (ajout des tests goldens)
- Create: `commands/ui/testdata/golden/inputs-form-*.golden` (générés puis committés)

**Interfaces:**
- Consumes: harnais Task 1 (`newTestForm`, `pressKeys`, `runes`, `enter`, `keyOf`), `assertGolden` (`golden_select_test.go`), `(InputsForm).View()` (`inputs_form.go:1180`), `SetValidationError`.
- Produces: goldens de référence pour les phases 3b-3d (le découpage d'`inputs_form.go` en 3d devra les préserver).

- [ ] **Step 1: Écrire les tests goldens**

```go
func TestGoldenInputsFormViews(t *testing.T) {
	mk := func() InputsForm {
		f := newTestForm(
			strInput("name", "Bundle name?"),
			boolInput("enabled", "Enable the thing?"),
			strInput("region", "Region?"),
		)
		f.PanelWidth = 100
		f.PanelHeight = 24
		return f
	}

	// 1. Fresh form, first input active.
	f := mk()
	assertGolden(t, "inputs-form-active-first", f.View())

	// 2. One input completed, second active.
	f = pressKeys(mk(), runes("my-bundle"), enter())
	assertGolden(t, "inputs-form-one-completed", f.View())

	// 3. All inputs done: buttons panel.
	f = pressKeys(mk(), runes("my-bundle"), enter(), enter(), runes("fr-par"), enter())
	assertGolden(t, "inputs-form-buttons", f.View())

	// 4. Focus on the completed panel (Tab from state 2).
	f = pressKeys(mk(), runes("my-bundle"), enter(), keyOf(tea.KeyTab))
	assertGolden(t, "inputs-form-completed-focus", f.View())

	// 5. Validation error banner.
	f = pressKeys(mk(), runes("my-bundle"), enter())
	f.SetValidationError("something went wrong: characterization fixture")
	assertGolden(t, "inputs-form-validation-error", f.View())
}
```

Note : l'étape 3 suppose que `enter()` sur le `boolInput` confirme la valeur courante du widget bool — si le widget bool exige une autre touche (espace/flèches) pour basculer puis confirmer, adapter la séquence au comportement réel (règle d'adaptation bornée de la Task 1) et le documenter. `SetValidationError` : vérifier le nom exact de la méthode (`grep -n 'func (f \*InputsForm) SetValidation' commands/ui/inputs_form.go`) — les callers l'utilisent (`view_create.go`), reprendre leur appel exact.

- [ ] **Step 2: Générer, vérifier déterminisme et sensibilité**

Run: `go test ./commands/ui/ -run TestGoldenInputsForm -update && go test -count=2 ./commands/ui/ -run TestGoldenInputsForm`
Expected: goldens générés, deux runs PASS. Sensibilité : altérer temporairement une chaîne du rendu (ex. un titre dans `View`), vérifier FAIL, annuler, vérifier PASS — consigner au rapport.

- [ ] **Step 3: Commit**

```bash
git add commands/ui/inputs_form_test.go commands/ui/testdata/
git commit -m "test(ui): golden characterization of InputsForm rendering states"
```

---

### Task 3: `change.go` — caractérisation de `Save` en standalone

**Files:**
- Create: `commands/ui/change_test.go`
- Create: `commands/ui/testdata/golden/change-*.golden`

**Interfaces:**
- Consumes: `Change` (struct exportée, `change.go:42-67`), `(*Change).Save(envs []*config.Environment) error` (`:435`). Aucune dépendance à `Model`/tea.
- Produces: la preuve du format YAML généré (goldens) pour les refontes 3b+.

- [ ] **Step 1: Écrire les tests**

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
)

func testChange(hostPath string) *Change {
	return &Change{
		Kind:     ChangeCreate,
		HostPath: hostPath,
		Name:     "my-bundle",
		UUID:     "00000000-0000-0000-0000-000000000001",
		Source:   "/bundles/vpc",
		Alias:    "vpc-main",
		InputDefs: []*config.InputDefinition{
			strInput("region", "Region?"),
			strInput("name", "Name?"),
		},
		Values: map[string]cty.Value{
			"region": cty.StringVal("fr-par"),
			"name":   cty.StringVal("main"),
		},
		UserValues: map[string]cty.Value{
			"region": cty.StringVal("fr-par"),
			"name":   cty.StringVal("main"),
		},
	}
}

func TestChangeSaveWritesTopLevelBundleYAML(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "vpc-main.tm.yml")
	c := testChange(path)

	if err := c.Save(nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-save-toplevel", string(got))
}

func TestChangeSaveEnvScopedBundleYAML(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	dir := t.TempDir()
	path := filepath.Join(dir, "vpc-main.tm.yml")
	c := testChange(path)
	c.Env = staging

	if err := c.Save([]*config.Environment{staging}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-save-envscoped", string(got))
}

func TestChangeSaveMergesSecondEnvIntoExistingFile(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	prod := &config.Environment{ID: "prod", Name: "Production"}
	envs := []*config.Environment{staging, prod}
	dir := t.TempDir()
	path := filepath.Join(dir, "vpc-main.tm.yml")

	c1 := testChange(path)
	c1.Env = staging
	if err := c1.Save(envs); err != nil {
		t.Fatal(err)
	}

	// Promote-style: same file, second env, same input values.
	// Characterizes current behavior: mergeBundleYAMLEnv dedups the new
	// env's inputs against the TOP-LEVEL inputs (empty here), not the
	// existing env's inputs — so both envs keep their full input maps.
	c2 := testChange(path)
	c2.Env = prod
	c2.FromEnv = staging
	if err := c2.Save(envs); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-save-two-envs", string(got))
}

func TestChangeSaveRejectsNonYAMLExistingFileForEnvMerge(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	dir := t.TempDir()
	path := filepath.Join(dir, "vpc-main.tm.hcl") // wrong extension
	if err := os.WriteFile(path, []byte("# not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := testChange(path)
	c.Env = staging

	err := c.Save([]*config.Environment{staging})
	if err == nil {
		t.Fatal("expected an error when merging into a non-.tm.yml file")
	}
	// Characterizes the exact user-facing message (change.go:535).
	wantSub := "is not a .tm.yml file"
	if !strings.Contains(err.Error(), wantSub) {
		t.Fatalf("expected error containing %q, got %q", wantSub, err.Error())
	}
}
```

(Ajouter l'import `"strings"`.) Règle d'adaptation bornée identique à la Task 1 : si `Save` exige un champ supplémentaire non fourni (panic/erreur sur fixture incomplète), compléter la fixture d'après le code (`generateBundleYAML` `:457`) et documenter — jamais de garde dans la production.

- [ ] **Step 2: Générer les goldens, vérifier le contenu**

Run: `go test ./commands/ui/ -run TestChangeSave -update && go test -count=2 ./commands/ui/ -run TestChangeSave`
Expected: PASS ×2. Contrôles sur les goldens : `change-save-toplevel` contient `kind: BundleInstance`, `metadata` avec le nom, `spec.source` et `spec.inputs` avec les deux valeurs ; `change-save-envscoped` place les inputs sous `spec.environments.staging` ; `change-save-two-envs` contient les DEUX environnements avec leurs inputs complets (bizarrerie de dédup figée).

- [ ] **Step 3: Commit**

```bash
git add commands/ui/change_test.go commands/ui/testdata/
git commit -m "test(ui): characterize Change.Save YAML generation and env merging"
```

---

### Task 4: Chemin complet create → reconfigure → promote (fixture HCL réelle)

**Files:**
- Modify: `commands/ui/change_test.go` (ajout du test bout-en-bout)

**Interfaces:**
- Consumes: `NewCreateChange` (`change.go:70`), `NewReconfigChange` (`:178`), `NewPromoteChange` (`:257`), `config.ListLocalBundleDefinitions`, `config.EvalBundleInputDefinitions` (`config/bundle.go:871`), `config.EvalBundleSchemaNamespaces`, `test/sandbox`.
- Produces: la caractérisation du chemin métier complet que la Phase 3b extraira vers la couche application.

- [ ] **Step 1: Écrire le test bout-en-bout**

Structure imposée (le code exact des appels intermédiaires suit les patterns de production — `ui.go:43-97` pour le bootstrap, `view_create_select.go:190-233` pour la construction des defs — et `config/bundle_test.go:20-35` pour la fixture) :

```go
func TestChangeCreateReconfigPromoteRoundTrip(t *testing.T) {
	t.Parallel()

	// 1. Sandbox avec une définition de bundle minimale portant un input
	//    string prompté et un scaffolding path/name.
	s := sandbox.NoGit(t, true)
	s.BuildTree([]string{
		"f:/bundles/vpc/define.tm:" + `define "bundle" {
  metadata {
    class   = "network"
    name    = "vpc"
    version = "1.0.0"
  }
  scaffolding {
    path = "stacks/{{ .name }}"
    name = "{{ .name }}"
  }
  input "region" {
    type = string
    prompt {
      text = "Region?"
    }
  }
}`,
	})
	root, err := config.LoadRoot(s.RootDir(), false)
	if err != nil {
		t.Fatal(err)
	}
	// ... evalctx (pattern config/bundle_test.go:31-35), ListLocalBundleDefinitions,
	// EvalBundleSchemaNamespaces + EvalBundleInputDefinitions sur l'entrée trouvée,
	// puis :

	// 2. NewCreateChange avec values{"region": "fr-par"} (+ __output_path__/__output_name__
	//    si le scaffolding de la fixture ne suffit pas), Save, relire le fichier →
	//    assertGolden(t, "change-roundtrip-created", <contenu avec UUID normalisé en <UUID>>).

	// 3. Recharger le registry depuis le disque (pattern reloadAll/EvalProjectBundles),
	//    retrouver le bundle créé, NewReconfigChange avec region modifiée → Save →
	//    assertGolden(t, "change-roundtrip-reconfigured", <contenu normalisé>).

	// 4. Si la mécanique d'environnements est trop lourde à fixturer ici (PromoteFrom,
	//    envs déclarés en config), le volet promote peut être remplacé par un test de
	//    NewPromoteChange retournant une erreur explicite sur bundle sans env — à
	//    documenter. L'objectif ferme : create + reconfigure caractérisés bout en bout.
}
```

**Cette tâche est exploratoire par nature** : la syntaxe HCL exacte du bloc `define bundle` (noms des sous-blocs `scaffolding`/`input`/`prompt`, templating du path) doit être vérifiée contre le parser (`hcl/block_define_parser.go`) et les fixtures existantes (`config/bundle_test.go`, e2e testdata, `grep -rn 'scaffolding' --include='*.tm'`). Si la fixture ci-dessus ne parse pas, la corriger d'après le parser — c'est le comportement du parser qui fait foi. Si `NewPromoteChange` est infixturable sans un vrai système d'environnements, livrer create+reconfigure et documenter le manque (il sera couvert en 3b quand la logique sera extraite et injectable). Escalader NEEDS_CONTEXT si le chemin `EvalBundleSchemaNamespaces` exige `resolve.API` d'une façon qu'un stub ne satisfait pas — indiquer alors précisément la signature bloquante.

- [ ] **Step 2: Générer goldens (UUID normalisé), vérifier déterminisme**

Run: `go test ./commands/ui/ -run TestChangeCreateReconfigPromote -update && go test -count=2 ./commands/ui/ -run TestChangeCreateReconfigPromote`
Expected: PASS ×2 (la normalisation `<UUID>` rend le golden stable).

- [ ] **Step 3: Commit**

```bash
git add commands/ui/change_test.go commands/ui/testdata/
git commit -m "test(ui): characterize create/reconfigure change round trip on real bundle definition"
```

---

### Task 5: Vérification de fin de phase

**Files:** aucun (vérification seule).

- [ ] **Step 1:** `make build` → succès.
- [ ] **Step 2:** `LC_ALL=C make test` → vert (artefact TSan connu : re-vérifier les packages tués sans `-race`, documenter).
- [ ] **Step 3:** `make lint/all` → 0 issue.
- [ ] **Step 4:** Bilan de couverture : `go test ./commands/ui/ -cover` avant/après la phase — consigner le delta de couverture du package dans le rapport final (attendu : hausse sensible, `inputs_form.go` et `change.go` passant de 0 test à couverts sur leurs chemins principaux).
