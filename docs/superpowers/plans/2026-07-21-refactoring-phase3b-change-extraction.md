# Refactoring Phase 3b — Change Domain Extraction Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract the bundle-change business logic (`commands/ui/change.go` + its pure helper satellites) out of the Bubble Tea TUI package into a new `commands/ui/change` package, strictly iso-behavior, after first closing the promote-leg characterization gaps left by Phase 3a.

**Architecture:** Phase 3a proved `change.go` has zero Bubble Tea dependencies; its only coupling is package-private helpers scattered across TUI files (`model.go`, `view_create.go`, `view_create_select.go`, `inputs_form.go`) — all pure. We move the whole Change domain (constructors, YAML reader/writer, pure helpers) into `commands/ui/change` (package `change`), replace the `*EngineState` parameter with a narrow `change.Session` value (the 4 fields the constructors actually read), and update TUI call sites mechanically. The Phase 3a characterization tests **stay in package `ui`** and keep exercising the flow through the new package exactly like the views do — no golden file moves, no assertion changes.

**Tech Stack:** Go 1.25.8 (via mise), Bubble Tea TUI, forked HCL, go-cty, sandbox test infra, golden-file characterization harness from Phase 3a.

## Global Constraints

- **Iso-behavior is absolute.** No observable behavior change: identical YAML output, identical error messages, identical TUI rendering. The Phase 3a characterization tests + goldens are the arbiters.
- **Golden files are never regenerated and never moved.** They stay at `commands/ui/testdata/golden/*.golden`. At the end of the phase, `git diff <base> -- commands/ui/testdata/golden/` restricted to pre-existing goldens must be empty. New goldens may be *added* (Tasks 1-2) but existing ones must be byte-identical.
- **Bounded adaptation rule (tests):** existing test assertions may only be adapted for *renamed/moved symbols* (e.g. `NewCreateChange(est, …)` → `change.NewCreate(est.changeSession(), …)`); assertion values, golden names, and error-message expectations must not change. If a test fails for any other reason, STOP and report — it is a behavior regression, not something to fix in the test.
- **License headers:** every NEW file starts with:
  ```go
  // Copyright 2026 Terramate GmbH
  // SPDX-License-Identifier: MPL-2.0
  ```
  (existing files keep their existing year).
- Conventional commits in English; every commit message ends with the footer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Verification commands: `make build`, then `LC_ALL=C go test ./commands/ui/... -count=1` (per task) — **never use `-race`** (the TSan runtime is broken on this kernel: silent SIGKILL, a known environmental artifact). Full-suite verification happens in Task 4 only.
- `make fmt` before each commit if imports changed (pre-commit hooks run goimports/vet/build/golangci-lint and will block otherwise).
- The rtk shell proxy masks pipeline exit codes behind `| tail`; when checking a test run for failures, grep the full output for `FAIL` / `signal:` instead of trusting the last line.

## Non-goals (deferred)

- Extracting the view-side loaders (`loadReconfigBundle`, `loadPromoteBundle`, `makeBundleDefinitionEntry`, `missingBundleRefs`, `rawInputKeys`, `buildAllPromoteBundles`) — they restructure naturally with the Model decomposition in **Phase 3c**.
- Splitting `inputs_form.go` — **Phase 3d**.
- Any behavior fix, even for quirks (e.g. `mergeBundleYAMLEnv` deduping only against top-level spec inputs is FROZEN behavior).

## File Structure (end state)

```
commands/ui/change/                 # NEW package "change" — pure business logic, no tea/lipgloss
  change.go       # Kind, Change, Session, NewCreate/NewReconfig/NewPromote,
                  # reEvalAllInputs, NormalizeBundleRefValues, checkBundleRefsResolved
  yamlio.go       # Save, generateBundleYAML, loadBundleYAMLConfig, hasYAMLConfigExt,
                  # mergeBundleYAMLEnv, trailingWSRE, formatTmdoc, writeBundleInstance,
                  # fixupFileExtension
  helpers.go      # IsPseudoKey, ExtractPseudoString, PseudoKeyOutput{Name,Path},
                  # InputsToValueMap, DisplayNameFromAlias, IsBundleUnique,
                  # BundleRequiresEnv, setupExplicitBundleAlias
commands/ui/change.go               # DELETED (content moved)
commands/ui/change_test.go          # stays in package ui; symbol renames only
commands/ui/testdata/golden/        # existing goldens untouched; new ones added by Tasks 1-2
```

---

### Task 1: Characterize the promote leg end-to-end (create env-scoped → promote)

Closes the gap explicitly documented at `commands/ui/change_test.go:168-172` and in the Phase 3a final review: `NewPromoteChange` + `Save` merging a new environment block into an existing on-disk file has no test.

**Files:**
- Modify: `commands/ui/change_test.go` (append one test; update the promote-gap doc comment at lines 168-172 and step-4 comment at lines 340-341 to point at the new test)
- Create (via `-update` run, then commit): `commands/ui/testdata/golden/change-roundtrip-env-created.golden`, `commands/ui/testdata/golden/change-roundtrip-promoted.golden`

**Interfaces:**
- Consumes: everything `TestChangeCreateReconfigRoundTrip` (change_test.go:173-342) already uses — reuse its wiring verbatim where indicated.
- Produces: `TestChangePromoteRoundTrip` — an arbiter Task 3 must keep green.

**Facts established by controller research (verify, don't re-derive):**
- `environment` is a top-level, label-less HCL block with attributes `id`, `name`, `description`, `promote_from` (hcl/block_environment_parser.go:27-64).
- `reg.Environments` is populated by `engine.EvalProjectBundles` via `config.EvalEnvironments(root, evalctx)` (engine/bundles.go:28).
- `define "bundle"` accepts an `environments { … }` sub-block parsed by `parseDefineEnvironmentsBlock` (hcl/block_define_parser.go:300-303); `bundleRequiresEnv` (view_create_select.go:290-299) evaluates `def.Environments.Required`. **Verify the attribute syntax (`required = true`) against `parseDefineEnvironmentsBlock` before writing the fixture.**
- `NewCreateChange` only attaches the env when `activeEnv != nil && bundleRequiresEnv(...)` (change.go:149-152), so the fixture's define MUST have `environments { required = true }` for the env-scoped create leg.
- The promote UI wiring being characterized: `loadPromoteBundle` (view_promote.go:87-112) evaluates the form context against the TARGET env, seeds `inputsToValueMap(b.Inputs)` + `normalizeBundleRefValues`, then `updatePromoteInput` calls `NewPromoteChange(est, targetEnv, bundle, …)` and `change.Save(est.Registry.Environments)` (view_promote.go:382-405).

**Bounded adaptation:** if any step of the fixture fails against real parser/engine behavior, adjust the FIXTURE to reality with a file:line citation in a comment (as done throughout Phase 3a) — never relax an assertion. If the promote output itself looks surprising (e.g. input dedup, env ordering), that is exactly what we are freezing: golden it and document with a comment.

- [ ] **Step 1: Write the failing test**

Append to `commands/ui/change_test.go`:

```go
// TestChangePromoteRoundTrip characterizes the promote leg end-to-end,
// closing the gap documented by TestChangeCreateReconfigRoundTrip: an
// env-scoped create into "staging", reload from disk, then
// NewPromoteChange into "prod" merging a second environment block into
// the existing file (mergeBundleYAMLEnv append + registry-order sort).
//
// Wiring mirrors loadPromoteBundle (view_promote.go:87-112) and
// updatePromoteInput (view_promote.go:382-405).
func TestChangePromoteRoundTrip(t *testing.T) {
	t.Parallel()

	// Same skeleton as TestChangeCreateReconfigRoundTrip, plus:
	// two top-level environment blocks (prod promotes from staging) and
	// environments { required = true } on the definition so the create
	// leg binds to an environment (change.go:149-152).
	s := sandbox.NoGit(t, true)
	s.BuildTree([]string{
		"f:/envs.tm:" + `environment {
  id   = "staging"
  name = "Staging"
}
environment {
  id           = "prod"
  name         = "Production"
  promote_from = "staging"
}`,
		"f:/bundles/vpc/define.tm:" + `define "bundle" {
  metadata {
    class   = "network"
    name    = "vpc"
    version = "1.0.0"
  }
  scaffolding {
    path = "stacks/vpc.tm"
    name = "vpc"
  }
  environments {
    required = true
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

	// di-bound resolve.API — same rationale as TestChangeCreateReconfigRoundTrip.
	bindings := di.NewBindings(context.Background())
	if err := di.Bind(bindings, resolve.NewAPI(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	ctx := di.WithBindings(context.Background(), bindings)
	resolveAPI, err := di.Get[resolve.API](ctx)
	if err != nil {
		t.Fatal(err)
	}

	evalctx := newRootEvalctx(root)

	// Registry with 0 bundles but both environments (engine/bundles.go:28).
	reg, err := engine.EvalProjectBundles(root, resolveAPI, evalctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg.Environments) != 2 {
		t.Fatalf("expected 2 environments in the registry, got %d", len(reg.Environments))
	}
	staging := reg.Environments[0]
	prod := reg.Environments[1]
	if staging.ID != "staging" || prod.ID != "prod" {
		t.Fatalf("unexpected registry env order: %q, %q", staging.ID, prod.ID)
	}

	localDefs, err := config.ListLocalBundleDefinitions(root, evalctx, project.NewPath("/"))
	if err != nil {
		t.Fatal(err)
	}
	if len(localDefs) != 1 {
		t.Fatalf("expected exactly 1 local bundle definition, got %d", len(localDefs))
	}
	bde := &localDefs[0]

	est := &EngineState{
		Context:    ctx,
		WorkingDir: root.HostDir(),
		Root:       root,
		Evalctx:    evalctx,
		ResolveAPI: resolveAPI,
		Registry:   reg,
	}

	// Create leg, env-scoped into staging (view_create_select.go:190-233 wiring).
	bundleEvalctx := newBundleEvalContext(est.Evalctx, est.Registry, staging)
	schemas, err := config.EvalBundleSchemaNamespaces(est.Root, est.ResolveAPI, bundleEvalctx, bde.Define, true)
	if err != nil {
		t.Fatal(err)
	}
	schemactx := typeschema.EvalContext{Evalctx: bundleEvalctx, Schemas: schemas}

	inputDefs, err := config.EvalBundleInputDefinitions(schemactx, bde.Define)
	if err != nil {
		t.Fatal(err)
	}

	createValues := map[string]cty.Value{"region": cty.StringVal("fr-par")}
	createChange, err := NewCreateChange(est, staging, bde, schemactx, inputDefs, createValues)
	if err != nil {
		t.Fatal(err)
	}
	if createChange.Env == nil || createChange.Env.ID != "staging" {
		t.Fatalf("expected the create change to bind to staging, got %+v", createChange.Env)
	}
	if err := createChange.Save(reg.Environments); err != nil {
		t.Fatal(err)
	}
	createdContent, err := os.ReadFile(createChange.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-roundtrip-env-created", normalizeUUID(t, string(createdContent)))

	// Reload from disk through engine.Load — same rationale as the
	// create/reconfigure round trip (config.LoadRoot never re-parses .tm.yml).
	eng, found, err := engine.Load(ctx, root.HostDir(), false, cliconfig.Config{}, 0, printer.Printers{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("engine.Load: project not found on reload")
	}
	root2 := eng.Config()
	evalctx2 := newRootEvalctx(root2)

	reg2, err := engine.EvalProjectBundles(root2, resolveAPI, evalctx2, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(reg2.Bundles) != 1 {
		t.Fatalf("expected exactly 1 bundle in the reloaded registry, got %d", len(reg2.Bundles))
	}
	bundle := reg2.Bundles[0]
	if bundle.Environment == nil || bundle.Environment.ID != "staging" {
		t.Fatalf("expected the reloaded bundle bound to staging, got %+v", bundle.Environment)
	}
	prod2 := reg2.Environments[1]
	if prod2.ID != "prod" {
		t.Fatalf("unexpected reloaded env order, got %q", prod2.ID)
	}

	est2 := &EngineState{
		Context:    ctx,
		WorkingDir: root2.HostDir(),
		Root:       root2,
		Evalctx:    evalctx2,
		ResolveAPI: resolveAPI,
		Registry:   reg2,
	}
	m2 := Model{EngineState: est2}

	// Promote leg — mirrors loadPromoteBundle (view_promote.go:87-112):
	// the eval context is built against the TARGET env.
	bde2 := makeBundleDefinitionEntry(est2.Root, bundle)
	if bde2 == nil {
		t.Fatal("makeBundleDefinitionEntry returned nil for the reloaded bundle")
	}
	schemactx2, err := m2.loadBundleEvalContext(bde2, prod2)
	if err != nil {
		t.Fatal(err)
	}
	inputDefs2, err := config.EvalBundleInputDefinitions(schemactx2, bde2.Define)
	if err != nil {
		t.Fatal(err)
	}

	promoteValues := inputsToValueMap(bundle.Inputs)
	normalizeBundleRefValues(inputDefs2, promoteValues)
	promoteValues["region"] = cty.StringVal("us-east")

	promoteChange, err := NewPromoteChange(est2, prod2, bundle, bde2, schemactx2, inputDefs2, promoteValues)
	if err != nil {
		t.Fatal(err)
	}
	if promoteChange.Kind != ChangePromote {
		t.Fatalf("expected ChangePromote, got %v", promoteChange.Kind)
	}
	if promoteChange.FromEnv == nil || promoteChange.FromEnv.ID != "staging" {
		t.Fatalf("expected FromEnv staging, got %+v", promoteChange.FromEnv)
	}
	if err := promoteChange.Save(reg2.Environments); err != nil {
		t.Fatal(err)
	}
	promotedContent, err := os.ReadFile(promoteChange.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	// Freezes: both env blocks present, sorted in registry order
	// (staging before prod, mergeBundleYAMLEnv change.go:596-605), env
	// inputs deduped only against the (empty) top-level spec inputs.
	assertGolden(t, "change-roundtrip-promoted", normalizeUUID(t, string(promotedContent)))
}
```

Also update the stale gap comments in the same file:
- Lines 168-172 (doc comment of `TestChangeCreateReconfigRoundTrip`): replace the sentence `That gap is documented for Phase 3b.` with `That leg is covered by TestChangePromoteRoundTrip below.`
- Lines 340-341 (step-4 comment): replace both lines with `// 4. Promote leg covered by TestChangePromoteRoundTrip.`

- [ ] **Step 2: Run to verify it fails for the right reason**

Run: `cd /home/arnaud.hatzenbuhler/Documents/Projects/github/terramate && LC_ALL=C go test ./commands/ui/ -run TestChangePromoteRoundTrip -count=1 -v 2>&1 | tail -30`

Expected: FAIL with `missing golden testdata/golden/change-roundtrip-env-created.golden`. If it fails EARLIER (fixture parse error, env not bound, nil bde2…), debug the fixture against the cited parsers — the assertions must not be weakened.

- [ ] **Step 3: Record the goldens (once) and inspect them**

Run: `LC_ALL=C go test ./commands/ui/ -run TestChangePromoteRoundTrip -update -count=1`
Then READ both new goldens and sanity-check: env-created has `environments:` with only `staging:`; promoted has `staging:` before `prod:`, the prod block carrying `source:` and `inputs:` with `region: us-east`, staging's `region: fr-par` intact, one `<UUID>` placeholder. Paste both goldens into your task report.

- [ ] **Step 4: Run the full ui package to prove no interference**

Run: `LC_ALL=C go test ./commands/ui/ -count=1 2>&1 | grep -E "FAIL|ok|signal" `
Expected: `ok` (49 tests total after this task; all previous 48 still green).

- [ ] **Step 5: Commit**

```bash
git add commands/ui/change_test.go commands/ui/testdata/golden/change-roundtrip-env-created.golden commands/ui/testdata/golden/change-roundtrip-promoted.golden
git commit -m "test(ui): characterize promote change round trip (env create + env merge)"
```

---

### Task 2: Characterize the dedup-active merge branch and bundle-ref alias write-out

Closes the two remaining unit-level gaps from the Phase 3a final review: (1) the `mergeBundleYAMLEnv` branch that actually FILTERS an env input because an identical entry exists in the top-level `spec` inputs (change.go:571-587) — the existing two-envs test only exercises the empty-spec case; (2) the outbound bundle-ref conversion in `generateBundleYAML` (change.go:468-474), where a resolved bundle object is written as its alias string.

**Files:**
- Modify: `commands/ui/change_test.go` (append two tests)
- Create (via `-update`, then commit): `commands/ui/testdata/golden/change-save-dedup-spec.golden`, `commands/ui/testdata/golden/change-save-bundleref-alias.golden`

**Interfaces:**
- Consumes: `testChange(hostPath)` fixture (change_test.go:29-50), `strInput` (inputs_form_test.go helper), `assertGolden`.
- Produces: two more arbiters for Task 3.

- [ ] **Step 1: Write the two failing tests**

Append to `commands/ui/change_test.go`:

```go
// TestChangeSaveDedupsEnvInputAgainstSpec characterizes the ACTIVE branch
// of mergeBundleYAMLEnv's dedup (change.go:571-587): an env input is
// dropped when an identical entry (same key, same comments, deep-equal
// value) already exists in the TOP-LEVEL spec inputs. The first save is
// top-level (spec inputs populated); the second save is env-scoped with
// one identical input ("region") and one new input ("name" changed), so
// only the changed one survives into the env block.
func TestChangeSaveDedupsEnvInputAgainstSpec(t *testing.T) {
	t.Parallel()
	staging := &config.Environment{ID: "staging", Name: "Staging"}
	dir := t.TempDir()
	path := filepath.Join(dir, "vpc-main.tm.yml")

	// 1. Top-level save: spec.inputs = {region: fr-par, name: main}.
	c1 := testChange(path)
	if err := c1.Save(nil); err != nil {
		t.Fatal(err)
	}

	// 2. Env-scoped save on the same file: region identical to spec
	// (deduped away), name differs (kept in the env block).
	c2 := testChange(path)
	c2.Env = staging
	c2.UserValues["name"] = cty.StringVal("staging-main")
	if err := c2.Save([]*config.Environment{staging}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-save-dedup-spec", string(got))
}

// TestChangeSaveWritesBundleRefAsAlias characterizes the outbound
// bundle-ref conversion (change.go:468-474): inputs typed BundleType hold
// resolved objects internally but are written to YAML as their alias
// string.
func TestChangeSaveWritesBundleRefAsAlias(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.tm.yml")

	c := testChange(path)
	c.InputDefs = append(c.InputDefs, &config.InputDefinition{
		Name:        "network",
		Description: "Upstream network bundle",
		Type:        &typeschema.BundleType{ClassID: "network"},
		Prompt:      config.PromptConfig{Text: "Network?"},
	})
	c.UserValues["network"] = cty.ObjectVal(map[string]cty.Value{
		"alias": cty.StringVal("vpc-main"),
		"uuid":  cty.StringVal("00000000-0000-0000-0000-000000000002"),
	})

	if err := c.Save(nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-save-bundleref-alias", string(got))
}
```

Adaptation note: `config.InputDefinition`/`config.PromptConfig` field names must match `strInput` in inputs_form_test.go and pseudoStringInput (inputs_form.go:1860-1868) — verify there before compiling; if `Prompt` is not needed for Save, it may be dropped.

- [ ] **Step 2: Run to verify both fail on missing goldens**

Run: `LC_ALL=C go test ./commands/ui/ -run "TestChangeSaveDedupsEnvInputAgainstSpec|TestChangeSaveWritesBundleRefAsAlias" -count=1 -v 2>&1 | tail -20`
Expected: both FAIL with `missing golden`.

- [ ] **Step 3: Record goldens and inspect**

Run: `LC_ALL=C go test ./commands/ui/ -run "TestChangeSaveDedupsEnvInputAgainstSpec|TestChangeSaveWritesBundleRefAsAlias" -update -count=1`
READ both goldens. Sanity: dedup golden's `staging:` env block must contain `name: staging-main` and must NOT contain a `region:` line inside the env block (region only at top level). Alias golden must contain `network: vpc-main` (a string, not an object). Paste both in your report. If the dedup golden DOES contain region in the env block, STOP: re-check comment equality (change.go:574-582) — the dedup requires identical tmdoc comments; report what you find instead of tweaking the test silently.

- [ ] **Step 4: Full package run**

Run: `LC_ALL=C go test ./commands/ui/ -count=1 2>&1 | grep -E "FAIL|ok|signal"`
Expected: `ok`.

- [ ] **Step 5: Commit**

```bash
git add commands/ui/change_test.go commands/ui/testdata/golden/change-save-dedup-spec.golden commands/ui/testdata/golden/change-save-bundleref-alias.golden
git commit -m "test(ui): characterize env-input dedup against spec and bundle-ref alias write-out"
```

---

### Task 3: Extract the Change domain into `commands/ui/change`

Pure move + rename, guarded by all Task 1/2/3a arbiters. NO logic edits of any kind.

**Files:**
- Create: `commands/ui/change/change.go`, `commands/ui/change/yamlio.go`, `commands/ui/change/helpers.go`
- Delete: `commands/ui/change.go`
- Modify: `commands/ui/model.go`, `commands/ui/view_create.go`, `commands/ui/view_create_select.go`, `commands/ui/view_reconfig.go`, `commands/ui/view_promote.go`, `commands/ui/view_overview.go`, `commands/ui/inputs_form.go`, `commands/ui/filter.go`, `commands/ui/selectable_list_view.go`, `commands/ui/change_test.go` (symbol renames only)

**Interfaces:**
- Produces (new package `github.com/terramate-io/terramate/commands/ui/change`):
  - `type Kind string` with consts `KindCreate Kind = "change_create"`, `KindReconfig Kind = "change_reconfig"`, `KindPromote Kind = "change_promote"` (string values UNCHANGED)
  - `type Change struct { … }` — identical fields to today's `ui.Change` (change.go:42-67), with `Kind Kind`
  - `type Session struct { Context context.Context; Registry *config.Registry; RootDir string; WorkingDir string }`
  - `func NewCreate(s Session, activeEnv *config.Environment, bde *config.BundleDefinitionEntry, schemactx typeschema.EvalContext, inputDefs []*config.InputDefinition, values map[string]cty.Value) (Change, error)`
  - `func NewReconfig(s Session, bundle *config.Bundle, bde *config.BundleDefinitionEntry, schemactx typeschema.EvalContext, inputDefs []*config.InputDefinition, values map[string]cty.Value) (Change, error)`
  - `func NewPromote(s Session, env *config.Environment, bundle *config.Bundle, bde *config.BundleDefinitionEntry, schemactx typeschema.EvalContext, inputDefs []*config.InputDefinition, values map[string]cty.Value) (Change, error)`
  - `func (c *Change) Save(envs []*config.Environment) error`
  - `func NormalizeBundleRefValues(inputDefs []*config.InputDefinition, values map[string]cty.Value) map[string]cty.Value`
  - `func IsPseudoKey(name string) bool`, `func ExtractPseudoString(values map[string]cty.Value, key string) string`, consts `PseudoKeyOutputName = "__output_name__"`, `PseudoKeyOutputPath = "__output_path__"`
  - `func InputsToValueMap(inputs map[string]cty.Value) map[string]cty.Value`
  - `func DisplayNameFromAlias(alias, name string) string`
  - `func IsBundleUnique(r *config.Registry, alias, classID, hostPath string, env *config.Environment) error`
  - `func BundleRequiresEnv(evalctx *eval.Context, def *hcl.DefineBundle) bool`
- Produces (in package `ui`, model.go): `func (est *EngineState) changeSession() change.Session`

- [ ] **Step 1: Create `commands/ui/change/change.go`**

New file. Header:

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

// Package change implements the bundle-change domain of the interactive
// UI: building create/reconfigure/promote changes from form values and
// persisting them as YAML bundle instance files. It has no dependency on
// the terminal rendering layer.
package change
```

Content = MOVED VERBATIM from `commands/ui/change.go` (use `git show HEAD:commands/ui/change.go` as the source of truth): lines 31-432 (ChangeKind/consts through `checkBundleRefsResolved`), applying ONLY these mechanical substitutions (whole-word):

| old | new |
|---|---|
| `ChangeKind` | `Kind` |
| `ChangeCreate` / `ChangeReconfig` / `ChangePromote` | `KindCreate` / `KindReconfig` / `KindPromote` |
| `NewCreateChange` / `NewReconfigChange` / `NewPromoteChange` | `NewCreate` / `NewReconfig` / `NewPromote` |
| param `est *EngineState` | `s Session` |
| `est.Context` | `s.Context` |
| `est.Registry` | `s.Registry` |
| `est.Root.HostDir()` | `s.RootDir` |
| `est.WorkingDir` | `s.WorkingDir` |
| `normalizeBundleRefValues` | `NormalizeBundleRefValues` |
| `isPseudoKey` | `IsPseudoKey` |
| `extractPseudoString` | `ExtractPseudoString` |
| `pseudoKeyOutputPath` / `pseudoKeyOutputName` | `PseudoKeyOutputPath` / `PseudoKeyOutputName` |
| `inputsToValueMap` | `InputsToValueMap` |
| `displayNameFromAlias` | `DisplayNameFromAlias` |
| `bundleRequiresEnv` | `BundleRequiresEnv` |

(`IsBundleUnique`, `setupExplicitBundleAlias`, `reEvalAllInputs`, `checkBundleRefsResolved` keep their names.) The doc comments move with their functions; comment TEXT referencing old names is updated to the new names (e.g. `// NewCreate builds a Change …`).

Plus, immediately after the `Change` struct, add:

```go
// Session is the narrow slice of engine state the change layer reads.
// The TUI projects its EngineState onto it; tests can construct it
// directly.
type Session struct {
	Context    context.Context
	Registry   *config.Registry
	RootDir    string // host absolute path of the project root
	WorkingDir string
}
```

Imports: same list as today's change.go lines 6-29 MINUS the ones now used only by yamlio.go, PLUS `"context"`. Let goimports settle it (`make fmt`); do not hand-tune.

- [ ] **Step 2: Create `commands/ui/change/yamlio.go`**

Header:

```go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package change
```

Content = lines 434-666 of the old change.go MOVED VERBATIM (from `// Save writes the change…` through `fixupFileExtension`), with the same substitution table (only `isPseudoKey` → `IsPseudoKey` actually occurs in this range). No other edits.

- [ ] **Step 3: Create `commands/ui/change/helpers.go`**

Header as in Step 2, then the following functions MOVED VERBATIM from their current locations (delete them at the source in Step 5), renamed per the table:

- from inputs_form.go:1853-1856: the two consts →
  ```go
  // PseudoKeyOutputName and PseudoKeyOutputPath are reserved input names
  // injected by the UI for the instance name and output path form fields.
  const (
  	PseudoKeyOutputName = "__output_name__"
  	PseudoKeyOutputPath = "__output_path__"
  )
  ```
- from inputs_form.go:567-571: `isPseudoKey` → `IsPseudoKey` (doc comment moves along, reworded to exported form: `// IsPseudoKey returns true for reserved input names injected by the caller (e.g. "__output_name__", "__output_path__").`)
- from inputs_form.go:1882-1888: `extractPseudoString` → `ExtractPseudoString`
- from model.go:241-249: `inputsToValueMap` → `InputsToValueMap`
- from model.go:487-493: `displayNameFromAlias` → `DisplayNameFromAlias` (add a doc comment: `// DisplayNameFromAlias returns the short name when the alias is the implicit <path>:<name> form, the alias otherwise.`)
- from model.go:519-546: `IsBundleUnique` (name unchanged, doc comment unchanged)
- from view_create_select.go:290-299: `bundleRequiresEnv` → `BundleRequiresEnv` (add doc comment: `// BundleRequiresEnv reports whether the definition declares environments.required = true.`)
- from view_create.go:128-151: `setupExplicitBundleAlias` (unexported, name unchanged; add doc comment: `// setupExplicitBundleAlias evaluates the definition's explicit alias, if any, and registers it in the bundle namespace.`)

NOTE: `rawInputKeys`, `MatchingBundleOptions`, `checkEnvRequired`, `newBundleEvalContext`, `pseudoStringInput`, `pseudoOutputPathInput` do NOT move — they stay in package ui (Phase 3c scope).

- [ ] **Step 4: Delete `commands/ui/change.go`**

```bash
git rm commands/ui/change.go
```

- [ ] **Step 5: Update package ui call sites**

Add `"github.com/terramate-io/terramate/commands/ui/change"` to the imports of each file below. IMPORTANT: local variables named `change` shadow the package — rename those variables to `ch` as listed.

**model.go:**
- Delete the moved functions: `inputsToValueMap` (:241-249), `displayNameFromAlias` (:487-493), `IsBundleUnique` (:519-546). Keep `rawInputKeys` and `MatchingBundleOptions` (update the latter's body if it calls none of the moved — it doesn't).
- `:132` — `sessionChanges map[string][]ChangeKind` → `sessionChanges map[string][]change.Kind`
- Add next to `EngineState` (after its struct definition):
  ```go
  // changeSession projects the engine state onto the narrow dependency
  // set of the change package.
  func (est *EngineState) changeSession() change.Session {
  	return change.Session{
  		Context:    est.Context,
  		Registry:   est.Registry,
  		RootDir:    est.Root.HostDir(),
  		WorkingDir: est.WorkingDir,
  	}
  }
  ```

**view_create.go:**
- Delete `setupExplicitBundleAlias` (:128-151).
- `:76` — `change, err := NewCreateChange(` → `ch, err := change.NewCreate(est.changeSession(),` keeping the remaining arguments unchanged except dropping the old `est` first argument (check the actual receiver: if the call reads `NewCreateChange(m.EngineState, …)` use `m.EngineState.changeSession()`). Rename every use of the old `change` variable in that block to `ch` (`change.Save(…)` → `ch.Save(…)`, `m.recordSessionChange(change)` → `m.recordSessionChange(ch)`, `change.Alias` → `ch.Alias`, `change.Warnings` if present).
- `:258` — `bundleRequiresEnv(` → `change.BundleRequiresEnv(`.

**view_create_select.go:**
- Delete `bundleRequiresEnv` (:290-299).
- `:123`, `:184` — `bundleRequiresEnv(` → `change.BundleRequiresEnv(`.
- `:302` (inside `checkEnvRequired`) — `bundleRequiresEnv(` → `change.BundleRequiresEnv(`.
- `:215`, `:265` — `pseudoKeyOutputName` → `change.PseudoKeyOutputName`.

**view_reconfig.go:**
- `:103` — `inputsToValueMap(` → `change.InputsToValueMap(`.
- `:104` — `normalizeBundleRefValues(` → `change.NormalizeBundleRefValues(`.
- `:228`, `:259` — `displayNameFromAlias(` → `change.DisplayNameFromAlias(`.
- `:327` — `change, err := NewReconfigChange(` → `ch, err := change.NewReconfig(est.changeSession(),` (same first-argument treatment as view_create.go; rename block-local `change` var uses to `ch`, incl. `:346 m.recordSessionChange(ch)`).

**view_promote.go:**
- `:102` — `change.InputsToValueMap(`; `:103` — `change.NormalizeBundleRefValues(`.
- `:294`, `:325` — `change.DisplayNameFromAlias(`.
- `:384` — `ch, err := change.NewPromote(est.changeSession(),` (+ block-local var renames, incl. `:403 m.recordSessionChange(ch)`).

**view_overview.go:**
- `:196` — `func (m *Model) recordSessionChange(c Change)` → `(c change.Change)`.
- `:226` — `func changeLogEntry(c Change) string` → `(c change.Change)`.
- `:230/:235/:240` — `case ChangeCreate:` etc. → `case change.KindCreate:` / `change.KindReconfig` / `change.KindPromote`.
- `:522` — `map[ChangeKind]struct` → `map[change.Kind]struct`; `:526-528` keys → `change.KindCreate` etc.
- `:552` — `change.DisplayNameFromAlias(`.
- `:573` — `map[ChangeKind]bool` → `map[change.Kind]bool`.

**inputs_form.go:**
- Delete `isPseudoKey` (:567-571), the pseudo consts (:1853-1856), `extractPseudoString` (:1882-1888).
- `:419`, `:590`, `:600` — `isPseudoKey(` → `change.IsPseudoKey(`.
- `:1871` (inside `pseudoOutputPathInput`) — `pseudoKeyOutputPath` → `change.PseudoKeyOutputPath`; find any remaining `pseudoKeyOutputName` uses in this file (e.g. a `pseudoOutputNameInput` helper) → `change.PseudoKeyOutputName`.

**filter.go** `:67` and **selectable_list_view.go** `:263`, `:295` — `displayNameFromAlias(` → `change.DisplayNameFromAlias(`.

Then sweep for stragglers (must return no hits outside `commands/ui/change/`):

```bash
grep -rn -E "\b(ChangeKind|ChangeCreate|ChangeReconfig|ChangePromote|NewCreateChange|NewReconfigChange|NewPromoteChange|isPseudoKey|extractPseudoString|pseudoKeyOutput(Name|Path)|inputsToValueMap|displayNameFromAlias|bundleRequiresEnv|normalizeBundleRefValues|setupExplicitBundleAlias)\b" commands/ui --include="*.go" | grep -v "commands/ui/change/"
```

(Only change_test.go hits remain at this point — handled in Step 6.)

- [ ] **Step 6: Adapt `commands/ui/change_test.go` (symbol renames ONLY)**

Apply the same substitutions to the test file — `testChange` returns `*change.Change` with `Kind: change.KindCreate`; `NewCreateChange(est, …)` → `change.NewCreate(est.changeSession(), …)`; `NewReconfigChange(est2, …)` → `change.NewReconfig(est2.changeSession(), …)`; `NewPromoteChange(est2, …)` → `change.NewPromote(est2.changeSession(), …)`; `ChangePromote` → `change.KindPromote`; `inputsToValueMap` → `change.InputsToValueMap`; `normalizeBundleRefValues` → `change.NormalizeBundleRefValues`; `&typeschema.BundleType{…}` untouched. Add the change import. **Do not touch golden names, assertion values, error substrings, or any fixture strings.** Also check `inputs_form_test.go` and `view_reconfig_test.go` for moved-symbol references (grep from Step 5); adapt identically if any.

- [ ] **Step 7: Format, build, run the arbiters**

```bash
make fmt
make build
LC_ALL=C go test ./commands/ui/... -count=1 2>&1 | grep -E "FAIL|ok|signal"
```

Expected: build clean; BOTH packages `ok` (`commands/ui` and `commands/ui/change` — the latter with "no test files" is fine). Any test failure = STOP, report (behavior regression or bad move), never adjust an assertion.

- [ ] **Step 8: Prove goldens untouched**

```bash
git status --porcelain commands/ui/testdata/golden/
```

Expected: EMPTY output (no modification, no rename, no deletion). Include this proof in your report.

- [ ] **Step 9: Commit**

```bash
git add -A commands/ui
git commit -m "refactor(ui): extract change domain into commands/ui/change package"
```

---

### Task 4: Whole-phase verification (controller-direct — do not delegate)

- [ ] **Step 1:** `make build` — clean.
- [ ] **Step 2:** `LC_ALL=C make test` — grep the full log for `FAIL` and `signal:`; re-verify any TSan-killed packages without `-race` per the established protocol.
- [ ] **Step 3:** `make lint/all` — 0 issues.
- [ ] **Step 4:** Golden integrity: `git diff 7ab56a3a -- commands/ui/testdata/golden/` must show ONLY the 4 new goldens from Tasks 1-2 (added), zero modifications to pre-existing ones.
- [ ] **Step 5:** Coverage: `go test ./commands/ui/... -coverprofile` — record `commands/ui` + `commands/ui/change` combined coverage vs the 30.0% baseline.
- [ ] **Step 6:** Binary smoke test: build and run the phase binary vs the pre-phase binary on `/home/arnaud.hatzenbuhler/Documents/Projects/gitlab-of/additi/infra/infrastructure-infra-terraform-scw` copies (version/list/generate/fmt/run-graph/script-list), diff outputs and trees — byte-identical expected.
- [ ] **Step 7:** Update `.superpowers/sdd/progress.md`, then final whole-branch review.
