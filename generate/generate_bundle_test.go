// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package generate_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/madlambda/spells/assert"
	"github.com/terramate-io/terramate/config"
	genreport "github.com/terramate-io/terramate/generate/report"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/test"
	. "github.com/terramate-io/terramate/test/hclwrite/hclutils"
	"github.com/terramate-io/terramate/test/sandbox"
)

func TestGenerateBundleLets(t *testing.T) {
	t.Parallel()

	testCodeGeneration(t, []testcase{
		{
			name: "bundle lets computes values from inputs, available in component inputs",
			layout: []string{
				"s:stacks/stack-1",
			},
			configs: []hclconfig{
				{
					path: "/components/my-comp/v1",
					add: Doc(
						Block("define",
							Labels("component", "metadata"),
							Str("class", "my-comp"),
							Str("name", "my-comp"),
							Str("version", "1.0.0"),
							Str("description", "My component"),
						),
						Block("define",
							Labels("component", "input", "full_name"),
							Str("prompt", "Full name"),
							Str("description", "Full name"),
						),
						Block("generate_hcl",
							Labels("main.tf"),
							Block("content",
								Expr("full_name", "component.input.full_name.value"),
							),
						),
					),
				},
				{
					path: "/bundles/my-bundle/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-bundle"),
							Str("version", "1.0.0"),
							Str("description", "My bundle"),
						),
						Block("define",
							Labels("bundle", "input", "name"),
							Str("prompt", "Name"),
							Str("description", "Name"),
						),
						Block("define",
							Labels("bundle", "lets"),
							Expr("full_name", `"prefix-${bundle.input.name.value}"`),
						),
						Block("define",
							Labels("bundle", "stack", "main"),
							Block("metadata",
								Str("path", "main"),
								Str("name", "main"),
							),
							Block("component",
								Labels("my-comp"),
								Str("source", "/components/my-comp/v1"),
								Block("inputs",
									Expr("full_name", "bundle.let.full_name"),
								),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/bundles/my-bundle/v1"),
						Block("inputs",
							Str("name", "my-app"),
						),
					),
				},
			},
			want: []generatedFile{
				{
					dir: "/stacks/stack-1/main",
					files: map[string]fmt.Stringer{
						"component_my-comp_main.tf": stringer(`full_name = "prefix-my-app"`),
					},
				},
			},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/stacks/stack-1/main"),
						Created: []string{"component_my-comp_main.tf", "stack.tm.hcl"},
					},
				},
			},
		},
		{
			name: "bundle lets values are available in exports",
			layout: []string{
				"s:stacks/stack-1",
			},
			configs: []hclconfig{
				{
					path: "/components/my-comp/v1",
					add: Doc(
						Block("define",
							Labels("component", "metadata"),
							Str("class", "my-comp"),
							Str("name", "my-comp"),
							Str("version", "1.0.0"),
							Str("description", "My component"),
						),
						Block("define",
							Labels("component", "input", "computed"),
							Str("prompt", "Computed"),
							Str("description", "Computed"),
						),
						Block("generate_hcl",
							Labels("out.tf"),
							Block("content",
								Expr("computed", "component.input.computed.value"),
							),
						),
					),
				},
				{
					path: "/bundles/provider/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "provider"),
							Str("name", "provider"),
							Str("version", "1.0.0"),
							Str("description", "Provider bundle"),
						),
						Block("define",
							Labels("bundle", "input", "prefix"),
							Str("prompt", "Prefix"),
							Str("description", "Prefix"),
						),
						Block("define",
							Labels("bundle", "lets"),
							Expr("computed", `"${bundle.input.prefix.value}-computed"`),
						),
						Block("define",
							Labels("bundle", "export", "computed"),
							Expr("value", "bundle.let.computed"),
						),
						Block("define",
							Labels("bundle", "stack", "main"),
							Block("metadata",
								Str("path", "main"),
								Str("name", "main"),
							),
							Block("component",
								Labels("my-comp"),
								Str("source", "/components/my-comp/v1"),
								Block("inputs",
									Expr("computed", "bundle.let.computed"),
								),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("prov"),
						Str("source", "/bundles/provider/v1"),
						Block("inputs",
							Str("prefix", "hello"),
						),
					),
				},
			},
			want: []generatedFile{
				{
					dir: "/stacks/stack-1/main",
					files: map[string]fmt.Stringer{
						"component_my-comp_out.tf": stringer(`computed = "hello-computed"`),
					},
				},
			},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/stacks/stack-1/main"),
						Created: []string{"component_my-comp_out.tf", "stack.tm.hcl"},
					},
				},
			},
		},
	})
}

func TestGenerateBundle(t *testing.T) {
	t.Parallel()

	testCodeGeneration(t, []testcase{
		{
			name: "generate bundle",
			layout: []string{
				"s:stacks/stack-1",
			},
			configs: []hclconfig{
				{
					path: "/components/example.com/my-component/v1",
					add: Doc(
						Block("define",
							Labels("component", "metadata"),
							Str("class", "my-component"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My component"),
						),

						Block("define",
							Labels("component", "input", "test"),
							Str("prompt", "Test value"),
							Str("description", "Test value"),
						),

						Block("generate_hcl",
							Labels("main.tf"),
							Block("content",
								Expr("value", "component.input.test.value"),
							),
						),
					),
				},
				{
					path: "/bundles/example.com/my-bundle/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My bundle"),
						),

						Block("define",
							Labels("bundle", "input", "test"),
							Str("prompt", "Test value"),
							Str("description", "Test value"),
						),

						Block("define",
							Labels("bundle", "stack", "my-stack"),
							Block("metadata",
								Str("path", "my-stack"),
								Str("name", "my-stack"),
							),
							Block("component",
								Labels("my-comp"),
								Str("source", "/components/example.com/my-component/v1"),
								Block("inputs",
									Expr("test", "bundle.input.test.value"),
								),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/bundles/example.com/my-bundle/v1"),
						Block("inputs",
							Str("test", `some_input`),
						),
					),
				},
			},
			want: []generatedFile{
				{
					dir: "/stacks/stack-1/my-stack",
					files: map[string]fmt.Stringer{
						"component_my-comp_main.tf": stringer(`value = "some_input"`),
					},
				},
			},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/stacks/stack-1/my-stack"),
						Created: []string{"component_my-comp_main.tf", "stack.tm.hcl"},
					},
				},
			},
		},
		{
			name: "generate bundle with conditions",
			layout: []string{
				"s:stacks/stack-1",
				"s:stacks/stack-2",
				"s:stacks/stack-3",
			},
			configs: []hclconfig{
				{
					path: "/components/example.com/my-component/v1",
					add: Doc(
						Block("define",
							Labels("component", "metadata"),
							Str("class", "my-component"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My component"),
						),

						Block("define",
							Labels("component", "input", "test"),
							Str("prompt", "Test value"),
							Str("description", "Test value"),
						),

						Block("generate_hcl",
							Labels("main.tf"),
							Block("content",
								Expr("value", "component.input.test.value"),
							),
						),
					),
				},
				{
					path: "/bundles/example.com/my-bundle/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My bundle"),
						),
						Block("define",
							Labels("bundle", "input", "test"),
						),
						Block("define",
							Labels("bundle", "input", "with_stack"),
							Bool("default", false),
						),
						Block("define",
							Labels("bundle", "input", "with_component"),
							Bool("default", true),
						),
						Block("define",
							Labels("bundle", "stack", "my-stack"),
							Expr("condition", "bundle.input.with_stack.value"),
							Block("metadata",
								Str("path", "my-stack"),
								Str("name", "my-stack"),
							),
							Block("component",
								Labels("my-comp"),
								Expr("condition", "bundle.input.with_component.value"),
								Str("source", "/components/example.com/my-component/v1"),
								Block("inputs",
									Expr("test", "bundle.input.test.value"),
								),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/bundles/example.com/my-bundle/v1"),
						Block("inputs",
							Str("test", `some_input1`),
						),
					),
				},
				{
					path: "/stacks/stack-2",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/bundles/example.com/my-bundle/v1"),
						Block("inputs",
							Str("test", `some_input2`),
							Bool("with_stack", true),
						),
					),
				},
				{
					path: "/stacks/stack-3",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/bundles/example.com/my-bundle/v1"),
						Block("inputs",
							Str("test", `some_input2`),
							Bool("with_stack", true),
							Bool("with_component", false),
						),
					),
				},
			},
			want: []generatedFile{
				{
					dir: "/stacks/stack-2/my-stack",
					files: map[string]fmt.Stringer{
						"component_my-comp_main.tf": stringer(`value = "some_input2"`),
					},
				},
			},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/stacks/stack-2/my-stack"),
						Created: []string{"component_my-comp_main.tf", "stack.tm.hcl"},
					},
					{
						Dir:     project.NewPath("/stacks/stack-3/my-stack"),
						Created: []string{"stack.tm.hcl"},
					},
				},
			},
		},
		{
			name: "generate bundle stack with absolute path",
			layout: []string{
				"s:stacks/stack-1",
			},
			configs: []hclconfig{
				{
					path: "/components/example.com/my-component/v1",
					add: Doc(
						Block("define",
							Labels("component", "metadata"),
							Str("class", "my-component"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My component"),
						),

						Block("define",
							Labels("component", "input", "test"),
							Str("prompt", "Test value"),
							Str("description", "Test value"),
						),

						Block("generate_hcl",
							Labels("main.tf"),
							Block("content",
								Expr("value", "component.input.test.value"),
							),
						),
					),
				},
				{
					path: "/bundles/example.com/my-bundle/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My bundle"),
						),
						Block("define",
							Labels("bundle", "input", "test"),
						),
						Block("define",
							Labels("bundle", "stack", "my-stack"),
							Block("metadata",
								Str("path", "/generated_stacks/my-stack"),
								Str("name", "my-stack"),
							),
							Block("component",
								Labels("my-comp"),
								Str("source", "/components/example.com/my-component/v1"),
								Block("inputs",
									Expr("test", "bundle.input.test.value"),
								),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/bundles/example.com/my-bundle/v1"),
						Block("inputs",
							Str("test", `some_input1`),
						),
					),
				},
			},
			want: []generatedFile{
				{
					dir: "/generated_stacks/my-stack",
					files: map[string]fmt.Stringer{
						"component_my-comp_main.tf": stringer(`value = "some_input1"`),
					},
				},
			},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/generated_stacks/my-stack"),
						Created: []string{"component_my-comp_main.tf", "stack.tm.hcl"},
					},
				},
			},
		},
		{
			name: "generate bundle writes all stack metadata to stack file",
			layout: []string{
				"s:stacks/stack-1",
			},
			configs: []hclconfig{
				{
					path: "/bundles/my-bundle/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-bundle"),
							Str("version", "1.0.0"),
							Str("description", "My bundle"),
						),
						Block("define",
							Labels("bundle", "stack", "app"),
							Block("metadata",
								Str("path", "app"),
								Str("name", "App Stack"),
								Str("description", "The application stack"),
								Expr("tags", `["env-prod", "team-platform"]`),
								Expr("after", `["/infra"]`),
								Expr("before", `["/destroy"]`),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("my-bundle"),
						Str("source", "/bundles/my-bundle/v1"),
					),
				},
			},
			want: []generatedFile{},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/stacks/stack-1/app"),
						Created: []string{"stack.tm.hcl"},
					},
				},
			},
		},
		{
			name: "re-generate updates existing stack metadata from bundle",
			layout: []string{
				"s:stacks/stack-1",
				// Pre-existing stack created by a previous generate, but with stale/manual metadata
				`f:stacks/stack-1/app/stack.tm.hcl:stack {
  id          = "stable-existing-id"
  name        = "Old Manual Name"
  description = "Old description"
  tags        = ["old-tag"]
}`,
			},
			configs: []hclconfig{
				{
					path: "/bundles/my-bundle/v1",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-bundle"),
							Str("version", "1.0.0"),
							Str("description", "My bundle"),
						),
						Block("define",
							Labels("bundle", "stack", "app"),
							Block("metadata",
								Str("path", "app"),
								Str("name", "Bundle Stack Name"),
								Str("description", "Bundle description"),
								Expr("tags", `["bundle-tag"]`),
							),
						),
					),
				},
				{
					path: "/stacks/stack-1",
					add: Block("bundle",
						Labels("my-bundle"),
						Str("source", "/bundles/my-bundle/v1"),
					),
				},
			},
			want: []generatedFile{},
			wantReport: genreport.Report{
				Successes: []genreport.Result{
					{
						Dir:     project.NewPath("/stacks/stack-1/app"),
						Changed: []string{"stack.tm.hcl"},
					},
				},
			},
		},
		{
			name: "don't generate files in .terramate/ for bundle references",
			layout: []string{
				`s:.terramate/stack:id=s1;tags=["genstack"]`,
				"s:other",
			},
			configs: []hclconfig{
				{
					path: "/",
					add: GenerateHCL(
						Labels("file.hcl"),
						Content(
							Str("data", "data"),
						),
						Expr("condition", `tm_contains(terramate.stack.tags, "genstack")`),
					),
				},
				{
					path:     "/.terramate/stack",
					filename: "bundle_def.tm.hcl",
					add: Doc(
						Block("define",
							Labels("bundle", "metadata"),
							Str("class", "my-bundle"),
							Str("name", "my-name"),
							Str("version", "1.2.3"),
							Str("description", "My bundle"),
						),

						Block("define",
							Labels("bundle", "input", "test"),
							Str("prompt", "Test value"),
							Str("description", "Test value"),
						),
					),
				},
				{
					path:     "/other",
					filename: "bundle_use.tm.hcl",
					add: Block("bundle",
						Labels("name"),
						Str("source", "/.terramate/stack"),
						Block("inputs",
							Str("test", `some_input`),
						),
					),
				},
			},
			wantReport: genreport.Report{},
		},
	})
}

func TestGenerateBundleStackMetadataContent(t *testing.T) {
	t.Parallel()

	s := sandbox.NoGit(t, true)
	s.BuildTree([]string{"s:stacks/stack-1"})

	// Bundle definition: a stack with name, description, tags, after, before.
	test.AppendFile(t, filepath.Join(s.RootDir(), "bundles/my-bundle/v1"), "bundle.tm.hcl", Doc(
		Block("define",
			Labels("bundle", "metadata"),
			Str("class", "my-bundle"),
			Str("name", "my-bundle"),
			Str("version", "1.0.0"),
			Str("description", "My bundle"),
		),
		Block("define",
			Labels("bundle", "stack", "app"),
			Block("metadata",
				Str("path", "app"),
				Str("name", "App Stack"),
				Str("description", "The application stack"),
				Expr("tags", `["env-prod", "team-platform"]`),
				Expr("after", `["/infra"]`),
				Expr("before", `["/destroy"]`),
			),
		),
	).String())

	// Bundle usage in stack-1.
	test.AppendFile(t, filepath.Join(s.RootDir(), "stacks/stack-1"), "terramate.tm.hcl", Block("bundle",
		Labels("my-bundle"),
		Str("source", "/bundles/my-bundle/v1"),
	).String())

	generateAPI := newGenerateAPIForTest(t)
	cfg, err := config.LoadRoot(s.RootDir(), false)
	assert.NoError(t, err)

	generateAPI.Do(cfg, project.NewPath("/"), 0, project.NewPath("/modules"), nil)

	// Read the generated stack.tm.hcl and verify all metadata fields are present.
	stackEntry := s.StackEntry("stacks/stack-1/app")
	content := stackEntry.ReadFile("stack.tm.hcl")

	for _, want := range []string{
		"App Stack",             // name value
		"The application stack", // description value
		"env-prod",              // tag
		"team-platform",         // tag
		"/infra",                // after entry
		"/destroy",              // before entry
	} {
		if !strings.Contains(content, want) {
			t.Errorf("stack.tm.hcl missing %q\ngot:\n%s", want, content)
		}
	}
}

func TestGenerateBundleReGenerateOverwritesMetadata(t *testing.T) {
	t.Parallel()

	s := sandbox.NoGit(t, true)
	s.BuildTree([]string{
		"s:stacks/stack-1",
		// Pre-existing stack file with stale metadata from a previous generate run.
		`f:stacks/stack-1/app/stack.tm.hcl:stack {
  id          = "stable-existing-id"
  name        = "Old Manual Name"
  description = "Old description"
  tags        = ["old-tag"]
}`,
	})

	// Bundle definition that specifies updated metadata.
	test.AppendFile(t, filepath.Join(s.RootDir(), "bundles/my-bundle/v1"), "bundle.tm.hcl", Doc(
		Block("define",
			Labels("bundle", "metadata"),
			Str("class", "my-bundle"),
			Str("name", "my-bundle"),
			Str("version", "1.0.0"),
			Str("description", "My bundle"),
		),
		Block("define",
			Labels("bundle", "stack", "app"),
			Block("metadata",
				Str("path", "app"),
				Str("name", "Bundle Stack Name"),
				Str("description", "Bundle description"),
				Expr("tags", `["bundle-tag"]`),
			),
		),
	).String())

	// Bundle usage in stack-1.
	test.AppendFile(t, filepath.Join(s.RootDir(), "stacks/stack-1"), "terramate.tm.hcl", Block("bundle",
		Labels("my-bundle"),
		Str("source", "/bundles/my-bundle/v1"),
	).String())

	generateAPI := newGenerateAPIForTest(t)
	cfg, err := config.LoadRoot(s.RootDir(), false)
	assert.NoError(t, err)

	generateAPI.Do(cfg, project.NewPath("/"), 0, project.NewPath("/modules"), nil)

	// Read the regenerated stack.tm.hcl and verify bundle values overwrote stale ones.
	stackEntry := s.StackEntry("stacks/stack-1/app")
	content := stackEntry.ReadFile("stack.tm.hcl")

	for _, want := range []string{
		"Bundle Stack Name",  // bundle name must replace stale name
		"Bundle description", // bundle description must replace stale description
		"bundle-tag",         // bundle tag must replace stale tag
	} {
		if !strings.Contains(content, want) {
			t.Errorf("stack.tm.hcl missing bundle value %q; bundle values should overwrite stale metadata\ngot:\n%s", want, content)
		}
	}

	if !strings.Contains(content, "stable-existing-id") {
		t.Errorf("stack.tm.hcl should preserve the existing stack id; got:\n%s", content)
	}

	for _, stale := range []string{
		"Old Manual Name",
		"Old description",
		"old-tag",
	} {
		if strings.Contains(content, stale) {
			t.Errorf("stack.tm.hcl still contains stale value %q; bundle should have overwritten it\ngot:\n%s", stale, content)
		}
	}
}
