// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/terramate-io/terramate/commands/ui/change"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/hcl/info"
	"github.com/terramate-io/terramate/scaffold/manifest"
	"github.com/terramate-io/terramate/test/hclutils"
	"github.com/terramate-io/terramate/test/sandbox"
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

func goldenEnvs() (*config.Environment, *config.Environment) {
	staging := &config.Environment{ID: "staging", Name: "Staging", PromoteFrom: ""}
	prod := &config.Environment{ID: "prod", Name: "Production", PromoteFrom: "staging"}
	return staging, prod
}

// goldenBundleInfo builds an info.Range whose HostPath() is an absolute path
// rooted at rootdir, so that reconfigListHeader/promoteListHeader's call to
// project.PrjAbsPath(est.Root.HostDir(), b.Info.HostPath()) does not panic
// (PrjAbsPath panics on a non-absolute or out-of-root path, and the zero
// value of config.Bundle.Info.HostPath() is ""). See task-1-report.md for
// the fixture-adaptation note.
func goldenBundleInfo(rootdir, relpath string) info.Range {
	fname := filepath.Join(rootdir, relpath)
	return info.NewRange(rootdir, hclutils.Mkrange(fname, hclutils.Start(1, 1, 0), hclutils.End(1, 1, 0)))
}

// goldenBundles builds the shared bundle fixture used by the Reconfigure and
// Promote golden tests. rootdir must be the HostDir() of the *config.Root
// used in the same test, so that each bundle's Info field resolves to a
// valid absolute path under it (see goldenBundleInfo).
func goldenBundles(rootdir string, staging, _ *config.Environment) []*config.Bundle {
	return []*config.Bundle{
		{DefinitionMetadata: config.Metadata{Name: "vpc", Version: "1.0.0", Class: "network"}, Alias: "vpc-1", Environment: staging, Source: "git::https://example.com/vpc", Info: goldenBundleInfo(rootdir, "vpc-1.tm.hcl")},
		{DefinitionMetadata: config.Metadata{Name: "vpc", Version: "1.0.0", Class: "network"}, Alias: "vpc-2", Environment: staging, Source: "git::https://example.com/vpc", Info: goldenBundleInfo(rootdir, "vpc-2.tm.hcl")},
		{DefinitionMetadata: config.Metadata{Name: "ecs", Version: "2.1.0", Class: "compute"}, Alias: "ecs-1", Environment: staging, Source: "git::https://example.com/ecs", Info: goldenBundleInfo(rootdir, "ecs-1.tm.hcl")},
	}
}

func TestGoldenReconfigSelectView(t *testing.T) {
	s := sandbox.New(t)
	root := s.Config()

	staging, prod := goldenEnvs()
	bundles := goldenBundles(root.HostDir(), staging, prod)
	m := Model{
		EngineState: &EngineState{Root: root, Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:       100,
		height:      32,
		viewState:   ViewReconfigSelect,
		reconfig: reconfigState{
			envFilter: envFilterCycle{filters: []envFilterState{{env: staging, label: "Staging", shortID: "staging"}}, pos: -1},
			filter:    newTextFilter(),
		},
	}
	m.reconfig.bundles = m.buildReconfigBundles()

	assertGolden(t, "reconfig-select-basic", m.View())

	// Env filter active
	m.reconfig.envFilter.pos = 0
	m.reconfig.bundles = m.buildReconfigBundles()
	assertGolden(t, "reconfig-select-envfilter", m.View())

	// Text filter narrowing + cursor on second row
	m.reconfig.envFilter.pos = -1
	m.reconfig.filter.input.SetValue("vpc")
	m.reconfig.bundles = m.buildReconfigBundles()
	m.reconfig.cursor = 1
	assertGolden(t, "reconfig-select-textfilter", m.View())
}

func TestGoldenPromoteSelectView(t *testing.T) {
	s := sandbox.New(t)
	root := s.Config()

	staging, prod := goldenEnvs()
	bundles := goldenBundles(root.HostDir(), staging, prod)
	m := Model{
		EngineState: &EngineState{Root: root, Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:       100,
		height:      32,
		viewState:   ViewPromoteSelect,
		promote: promoteState{
			envFilter: envFilterCycle{filters: []envFilterState{{env: prod, label: "Production", shortID: "prod"}}, pos: -1},
			filter:    newTextFilter(),
		},
	}
	m.promote.bundles, m.promote.targetEnvs = m.buildAllPromoteBundles()

	assertGolden(t, "promote-select-basic", m.View())

	// Cursor on the last promotable bundle
	if len(m.promote.bundles) > 1 {
		m.promote.cursor = len(m.promote.bundles) - 1
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
		EngineState: &EngineState{Registry: &config.Registry{}, Collections: []*manifest.Collection{coll}},
		width:       100,
		height:      32,
		viewState:   ViewCreateSelect,
		create:      createState{allFlatBundles: entries, flatBundleFilter: newTextFilter()},
	}
	m.applyFlatBundleFilter()

	assertGolden(t, "create-select-basic", m.View())

	// Text filter narrowing
	m.create.flatBundleFilter.input.SetValue("vpc")
	m.applyFlatBundleFilter()
	assertGolden(t, "create-select-textfilter", m.View())

	// Inline error box
	m.create.flatBundleFilter.input.SetValue("")
	m.applyFlatBundleFilter()
	m.create.bundleSelectErr = "boom: could not load bundle definition"
	assertGolden(t, "create-select-error", m.View())
}

// TestGoldenOverviewView freezes the overview rendering (renderOverviewView,
// view_overview.go) before the phase-3c Model decomposition: command grid,
// inline error area, and the session-history panel in both focus states.
func TestGoldenOverviewView(t *testing.T) {
	s := sandbox.New(t)
	root := s.Config()

	staging, prod := goldenEnvs()
	bundles := goldenBundles(root.HostDir(), staging, prod)
	m := Model{
		EngineState: &EngineState{Root: root, Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:       100,
		height:      32,
		viewState:   ViewOverview,
		overview: overviewState{
			commands: []string{"Scaffold", "Reconfigure", "Promote", "Quit"},
			focus:    FocusCommands,
		},
	}

	assertGolden(t, "overview-basic", m.View())

	// Inline error area below the command grid.
	m.overview.currentErr = errors.E("No bundles available.")
	assertGolden(t, "overview-error", m.View())
	m.overview.currentErr = nil

	// Session-history panel: vpc-1 reconfigured this session and last saved.
	key := sessionBundleKey(bundles[0].Info.HostPath(), bundles[0].Environment)
	m.sessionChanges = map[string][]change.Kind{key: {change.KindReconfig}}
	m.lastSavedKey = key
	assertGolden(t, "overview-session-unfocused", m.View())

	// Same state with the summary panel focused.
	m.overview.focus = FocusSummary
	assertGolden(t, "overview-session-focused", m.View())
}
