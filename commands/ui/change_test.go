// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/di"
	"github.com/terramate-io/terramate/engine"
	"github.com/terramate-io/terramate/generate/resolve"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/terramate-io/terramate/printer"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/stdlib"
	"github.com/terramate-io/terramate/test/sandbox"
	"github.com/terramate-io/terramate/typeschema"
	"github.com/terramate-io/terramate/ui/tui/cliconfig"
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

// uuidRE matches a canonical UUID so goldens stay stable across runs
// (NewCreateChange/generateBundleYAML mint a fresh uuid.NewString() each time).
var uuidRE = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// normalizeUUID replaces every canonical UUID with <UUID>. It fails the test
// when no UUID is present, so normalization can never mask a missing UUID.
func normalizeUUID(t *testing.T, s string) string {
	t.Helper()
	if !uuidRE.MatchString(s) {
		t.Fatalf("expected at least one canonical UUID in content, found none:\n%s", s)
	}
	return uuidRE.ReplaceAllString(s, "<UUID>")
}

// newRootEvalctx builds the base eval.Context for a config.Root, mirroring
// the bootstrap in commands/ui/ui.go:53-55 (minus setupRootGlobals, which is
// a best-effort globals load not needed by this fixture).
func newRootEvalctx(root *config.Root) *eval.Context {
	evalctx := eval.NewContext(stdlib.Functions(root.HostDir(), root.Tree().Node.Experiments()))
	evalctx.SetNamespace("terramate", root.Runtime())
	return evalctx
}

// TestChangeCreateReconfigRoundTrip characterizes the create -> reconfigure
// change path end-to-end on a real bundle definition: a sandbox with a
// minimal `define "bundle"` (one prompted string input + static
// scaffolding), through NewCreateChange -> Save -> reload from disk ->
// NewReconfigChange -> Save.
//
// The promote leg is NOT covered here (see task-4-report.md): NewPromoteChange
// requires a *config.Environment with a PromoteFrom lineage plus an
// `environments {}` block on the bundle definition, which is a materially
// bigger fixture (environments + promotion registry wiring) than the
// create/reconfigure path below. That gap is documented for Phase 3b.
func TestChangeCreateReconfigRoundTrip(t *testing.T) {
	t.Parallel()

	// 1. Sandbox with a minimal bundle definition carrying one prompted
	//    string input and a static scaffolding path/name. Syntax verified
	//    directly against hcl/block_define_parser.go (parseMetadataBlock,
	//    parseDefineBundleScaffoldingBlock, parseInputBody, parsePromptBlock).
	s := sandbox.NoGit(t, true)
	s.BuildTree([]string{
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

	// A real resolve.API is required: EvalProjectBundles refuses a nil one
	// (engine/bundles.go), and our bundle's source is a local "/"-prefixed
	// path, which resolve.Resolver.Resolve short-circuits without any
	// network/cache access (generate/resolve/resolver.go:68-71). It must be
	// DI-bound (not just constructed) because the reload step below goes
	// through engine.Load, whose internal applyBundleStacks fetches it via
	// di.Get[resolve.API](ctx) (engine/bundles.go:100).
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
		Registry:   &config.Registry{},
	}

	// Mirrors the bundle-selection wiring in view_create_select.go:190-233.
	bundleEvalctx := newBundleEvalContext(est.Evalctx, est.Registry, nil)
	schemas, err := config.EvalBundleSchemaNamespaces(est.Root, est.ResolveAPI, bundleEvalctx, bde.Define, true)
	if err != nil {
		t.Fatal(err)
	}
	schemactx := typeschema.EvalContext{Evalctx: bundleEvalctx, Schemas: schemas}

	inputDefs, err := config.EvalBundleInputDefinitions(schemactx, bde.Define)
	if err != nil {
		t.Fatal(err)
	}

	// 2. Create with region=fr-par, save, reload the written file, and
	//    assert against a UUID-normalized golden.
	createValues := map[string]cty.Value{"region": cty.StringVal("fr-par")}
	createChange, err := NewCreateChange(est, nil, bde, schemactx, inputDefs, createValues)
	if err != nil {
		t.Fatal(err)
	}
	if err := createChange.Save(nil); err != nil {
		t.Fatal(err)
	}

	createdContent, err := os.ReadFile(createChange.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-roundtrip-created", normalizeUUID(t, string(createdContent)))

	// 3. Reload the registry from disk, mirroring reloadAll (view_overview.go:167)
	//    and its est.CLI.Reload() call. config.LoadRoot alone is NOT enough here:
	//    the just-written .tm.yml is only merged into the config tree
	//    (Node.Bundles) by engine's internal loadYAMLConfigs pass, which runs
	//    inside engine.Load/engine.NewProject (engine/project.go:85) — not
	//    inside config.LoadRoot itself. So the reload must go through
	//    engine.Load, exactly like the real CLI.Reload() does.
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

	est2 := &EngineState{
		Context:    ctx,
		WorkingDir: root2.HostDir(),
		Root:       root2,
		Evalctx:    evalctx2,
		ResolveAPI: resolveAPI,
		Registry:   reg2,
	}
	m2 := Model{EngineState: est2}

	// Mirrors loadReconfigBundle, view_reconfig.go:88-113.
	bde2 := makeBundleDefinitionEntry(est2.Root, bundle)
	if bde2 == nil {
		t.Fatal("makeBundleDefinitionEntry returned nil for the reloaded bundle")
	}
	schemactx2, err := m2.loadBundleEvalContext(bde2, bundle.Environment)
	if err != nil {
		t.Fatal(err)
	}
	inputDefs2, err := config.EvalBundleInputDefinitions(schemactx2, bde2.Define)
	if err != nil {
		t.Fatal(err)
	}

	reconfigValues := inputsToValueMap(bundle.Inputs)
	reconfigValues["region"] = cty.StringVal("us-east")

	reconfigChange, err := NewReconfigChange(est2, bundle, bde2, schemactx2, inputDefs2, reconfigValues)
	if err != nil {
		t.Fatal(err)
	}
	if err := reconfigChange.Save(nil); err != nil {
		t.Fatal(err)
	}

	reconfiguredContent, err := os.ReadFile(reconfigChange.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	assertGolden(t, "change-roundtrip-reconfigured", normalizeUUID(t, string(reconfiguredContent)))

	// 4. Promote leg intentionally omitted — see the doc comment above and
	//    task-4-report.md for the documented gap.
}
