// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"os"
	"path/filepath"
	"strings"
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
