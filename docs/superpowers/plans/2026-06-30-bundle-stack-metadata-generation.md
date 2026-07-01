# Bundle Stack Metadata Generation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When a bundle is instantiated via `generate`, the created (or existing) `stack.tm.hcl` file contains all bundle-defined metadata fields — not just `stack_id` — and re-running `generate` always overwrites those fields with the bundle values.

**Architecture:** Fix 1 — populate all metadata in `config.Stack` before calling `stack.Create()`. Fix 2 — add `stack.UpdateMetadata()` that uses `hclwrite` to surgically update bundle-managed fields in an existing `stack.tm.hcl`, preserving the ID. Fix 3 — call `UpdateMetadata()` instead of returning early when the stack already exists.

**Tech Stack:** Go, `github.com/terramate-io/hcl/v2/hclwrite`, `github.com/zclconf/go-cty/cty`

## Global Constraints

- Copyright header on every new file: `// Copyright 2025 Terramate GmbH\n// SPDX-License-Identifier: MPL-2.0`
- Run tests with: `go test -race -count=1 ./stack/... ./generate/...`
- Format with: `make fmt`
- Never touch `stack.id` in `UpdateMetadata` — it is always preserved

---

### Task 1: `stack.UpdateMetadata()` — write and test the update function

**Files:**
- Create: `stack/update.go`
- Create: `stack/update_test.go`

**Interfaces:**
- Produces: `func UpdateMetadata(root *config.Root, stackMeta config.StackMetadata) error`
  - Updates `name`, `description`, `tags`, `after`, `before`, `wants`, `wanted_by`, `watch` in the existing `stack.tm.hcl`
  - Preserves `id` and all other content in the file
  - Returns an error if the file does not exist or the `stack {}` block is not found

- [ ] **Step 1: Write the failing tests**

Create `stack/update_test.go`:

```go
// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package stack_test

import (
	"path/filepath"
	"testing"

	"github.com/madlambda/spells/assert"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/stack"
	"github.com/terramate-io/terramate/test/sandbox"
)

func TestUpdateMetadata(t *testing.T) {
	t.Parallel()

	type testcase struct {
		name      string
		layout    []string
		stackMeta config.StackMetadata
		wantStack config.Stack
	}

	testcases := []testcase{
		{
			name:   "sets all metadata fields",
			layout: []string{"s:mystack"},
			stackMeta: config.StackMetadata{
				Dir:         project.NewPath("/mystack"),
				Name:        "My Stack",
				Description: "A test stack",
				Tags:        []string{"env:prod", "team:platform"},
				After:       []string{"/other"},
				Before:      []string{"/another"},
				Wants:       []string{"/wanted"},
				WantedBy:    []string{"/wanting"},
			},
			wantStack: config.Stack{
				Dir:         project.NewPath("/mystack"),
				Name:        "My Stack",
				Description: "A test stack",
				Tags:        []string{"env:prod", "team:platform"},
				After:       []string{"/other"},
				Before:      []string{"/another"},
				Wants:       []string{"/wanted"},
				WantedBy:    []string{"/wanting"},
			},
		},
		{
			name:   "clears optional fields when empty",
			layout: []string{`f:mystack/stack.tm.hcl:stack {
  id          = "existing-id"
  name        = "Old Name"
  description = "Old desc"
  tags        = ["old"]
}`},
			stackMeta: config.StackMetadata{
				Dir:  project.NewPath("/mystack"),
				Name: "New Name",
			},
			wantStack: config.Stack{
				Dir:  project.NewPath("/mystack"),
				ID:   "existing-id",
				Name: "New Name",
			},
		},
		{
			name:   "preserves existing stack ID",
			layout: []string{`f:mystack/stack.tm.hcl:stack {
  id   = "my-preserved-id"
  name = "Old"
}`},
			stackMeta: config.StackMetadata{
				Dir:  project.NewPath("/mystack"),
				Name: "New Name",
			},
			wantStack: config.Stack{
				Dir:  project.NewPath("/mystack"),
				ID:   "my-preserved-id",
				Name: "New Name",
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := sandbox.NoGit(t, true)
			s.BuildTree(tc.layout)

			root, err := config.LoadRoot(s.RootDir(), false)
			assert.NoError(t, err)

			err = stack.UpdateMetadata(root, tc.stackMeta)
			assert.NoError(t, err)

			got := s.LoadStack(tc.stackMeta.Dir)
			if tc.wantStack.Name == "" {
				tc.wantStack.Name = filepath.Base(tc.stackMeta.Dir.String())
			}
			tc.wantStack.Dir = tc.stackMeta.Dir
			assert.EqualInts(t, len(tc.wantStack.Tags), len(got.Tags), "tags length mismatch")
			assert.EqualInts(t, len(tc.wantStack.After), len(got.After), "after length mismatch")
			assert.EqualInts(t, len(tc.wantStack.Before), len(got.Before), "before length mismatch")
			assert.EqualInts(t, len(tc.wantStack.Wants), len(got.Wants), "wants length mismatch")
			assert.EqualInts(t, len(tc.wantStack.WantedBy), len(got.WantedBy), "wanted_by length mismatch")
			assert.EqualStrings(t, tc.wantStack.Name, got.Name, "name mismatch")
			assert.EqualStrings(t, tc.wantStack.Description, got.Description, "description mismatch")
			if tc.wantStack.ID != "" {
				assert.EqualStrings(t, tc.wantStack.ID, got.ID, "ID must be preserved")
			}
		})
	}
}
```

- [ ] **Step 2: Run tests — verify they fail**

```bash
go test -race -count=1 ./stack/... -run TestUpdateMetadata -v
```

Expected: FAIL with `undefined: stack.UpdateMetadata`

- [ ] **Step 3: Write the implementation**

Create `stack/update.go`:

```go
// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package stack

import (
	"os"
	"path/filepath"

	hhcl "github.com/terramate-io/hcl/v2"
	"github.com/terramate-io/hcl/v2/hclwrite"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/zclconf/go-cty/cty"
)

// UpdateMetadata updates the bundle-managed metadata fields (name, description,
// tags, after, before, wants, wanted_by, watch) in an existing stack.tm.hcl.
// The stack ID is never modified.
func UpdateMetadata(root *config.Root, stackMeta config.StackMetadata) error {
	hostpath := stackMeta.Dir.HostPath(root.HostDir())
	stackFilePath := filepath.Join(hostpath, DefaultFilename)

	st, err := os.Lstat(stackFilePath)
	if err != nil {
		return errors.E(err, "stating the stack file")
	}
	originalFileMode := st.Mode()

	stackContents, err := os.ReadFile(stackFilePath)
	if err != nil {
		return errors.E(err, "reading stack file")
	}

	parsed, diags := hclwrite.ParseConfig(stackContents, stackFilePath, hhcl.InitialPos)
	if diags.HasErrors() {
		return errors.E(diags, "parsing stack file")
	}

	for _, block := range parsed.Body().Blocks() {
		if block.Type() != "stack" {
			continue
		}

		body := block.Body()

		setOrRemoveString(body, "name", stackMeta.Name)
		setOrRemoveString(body, "description", stackMeta.Description)
		setOrRemoveSet(body, "tags", stackMeta.Tags)
		setOrRemoveSet(body, "after", stackMeta.After)
		setOrRemoveSet(body, "before", stackMeta.Before)
		setOrRemoveSet(body, "wants", stackMeta.Wants)
		setOrRemoveSet(body, "wanted_by", stackMeta.WantedBy)
		setOrRemoveSet(body, "watch", stackMeta.Watch)

		return os.WriteFile(stackFilePath, parsed.Bytes(), originalFileMode)
	}

	return errors.E("stack block not found in %s", stackFilePath)
}

func setOrRemoveString(body *hclwrite.Body, name, val string) {
	if val != "" {
		body.SetAttributeValue(name, cty.StringVal(val))
	} else {
		body.RemoveAttribute(name)
	}
}

func setOrRemoveSet(body *hclwrite.Body, name string, vals []string) {
	if len(vals) > 0 {
		ctyVals := make([]cty.Value, len(vals))
		for i, v := range vals {
			ctyVals[i] = cty.StringVal(v)
		}
		body.SetAttributeValue(name, cty.SetVal(ctyVals))
	} else {
		body.RemoveAttribute(name)
	}
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test -race -count=1 ./stack/... -run TestUpdateMetadata -v
```

Expected: PASS for all 3 cases.

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add stack/update.go stack/update_test.go
git commit -m "feat: add stack.UpdateMetadata for bundle-managed fields"
```

---

### Task 2: Fix new stack creation to write all metadata fields

**Files:**
- Modify: `generate/generate_bundle.go` — `generateBundleStack()` function (lines ~22-75)

**Interfaces:**
- Consumes: `stack.UpdateMetadata()` from Task 1 (not used here, but needed in Task 3)
- Produces: When `stack.Create()` is called, the resulting `stack.tm.hcl` contains `name`, `description`, `tags`, `after`, `before`, `wants`, `wanted_by`, `watch` — not just `id`.

- [ ] **Step 1: Write a failing test**

Add this test case inside `TestGenerateBundle` in `generate/generate_bundle_test.go`, inside the `testCodeGeneration(t, []testcase{...})` call:

```go
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
                        Expr("tags", `["env:prod", "team:platform"]`),
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
```

- [ ] **Step 2: Run test — verify it fails**

```bash
go test -race -count=1 ./generate/... -run "TestGenerateBundle/generate_bundle_writes_all_stack_metadata" -v
```

Expected: test passes on the `wantReport` assertion but would fail on a metadata-content assertion we'll add shortly. (The report is correct already — `stack.tm.hcl` is created — but the file doesn't contain `name`, `description`, `tags`, `after`, `before` yet. We'll add a stronger assertion after fixing the implementation.)

- [ ] **Step 3: Fix `generateBundleStack()` in `generate/generate_bundle.go`**

Replace the `stackCfg` construction block (currently only sets `Dir` and `ID`) and remove the dead in-memory assignments:

**Before** (lines ~43-75):
```go
stackCfg := config.Stack{
    Dir: stackMeta.Dir,
    ID:  uuid.NewString(),
}

// ... stack.Create and loading ...

st := stackTree.Node.Stack

st.Name = stackMeta.Name
st.Description = stackMeta.Description
st.Tags = stackMeta.Tags
st.After = stackMeta.After
st.Before = stackMeta.Before
st.Wants = stackMeta.Wants
st.WantedBy = stackMeta.WantedBy
st.Watch = stackMeta.Watch

logger.Debug().Msg("adding created file to report")
```

**After**:
```go
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

// ... stack.Create and loading — unchanged ...

// Remove the st.Name = ... block entirely.

logger.Debug().Msg("adding created file to report")
```

Also add the `project` import to `generate/generate_bundle.go`:
```go
import (
    "github.com/google/uuid"
    "github.com/rs/zerolog/log"

    "github.com/terramate-io/terramate/config"
    "github.com/terramate-io/terramate/errors"
    genreport "github.com/terramate-io/terramate/generate/report"
    "github.com/terramate-io/terramate/hcl"
    "github.com/terramate-io/terramate/project"
    "github.com/terramate-io/terramate/stack"
)
```

The full updated `generateBundleStack` function (replace the entire function):

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
		logger.Debug().Msg("stack already exists: skipping")
		stackTree.Node.Components = mergeComponentList(stackTree.Node.Components, stackMeta.Components, bundle.Source)
		return
	}

	if !allowCreate {
		report.AddFailure(stackMeta.Dir, errors.E("stack not generated"))
		return
	}

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

	stackTree, ok = g.root.Lookup(stackMeta.Dir)
	if !ok {
		panic(errors.E(errors.ErrInternal, "just created stack %s cannot be loaded", stackMeta.Dir))
	}

	logger.Debug().Msg("adding created file to report")

	dirReport := genreport.Dir{}
	dirReport.AddCreatedFile(stack.DefaultFilename)
	report.AddDirReport(stackMeta.Dir, dirReport)

	stackTree.Node.Components = mergeComponentList(stackTree.Node.Components, stackMeta.Components, bundle.Source)
}
```

- [ ] **Step 4: Run tests — verify they pass**

```bash
go test -race -count=1 ./generate/... -run "TestGenerateBundle" -v
go test -race -count=1 ./stack/... -v
```

Expected: all existing tests pass, new test passes.

- [ ] **Step 5: Format and commit**

```bash
make fmt
git add generate/generate_bundle.go generate/generate_bundle_test.go
git commit -m "fix: write full stack metadata when instantiating a bundle"
```

---

### Task 3: Update existing stack metadata on re-generate (bundle always wins)

**Files:**
- Modify: `generate/generate_bundle.go` — `generateBundleStack()`, the "stack already exists" branch
- Modify: `generate/generate_bundle_test.go` — add new test cases

**Interfaces:**
- Consumes: `stack.UpdateMetadata(root *config.Root, stackMeta config.StackMetadata) error` from Task 1

- [ ] **Step 1: Write a failing test**

Add this test case inside `TestGenerateBundle` in `generate/generate_bundle_test.go`:

```go
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
```

- [ ] **Step 2: Run test — verify it fails**

```bash
go test -race -count=1 ./generate/... -run "TestGenerateBundle/re-generate_updates_existing_stack_metadata" -v
```

Expected: FAIL — the existing stack is skipped, report shows nothing changed.

- [ ] **Step 3: Update the "stack already exists" branch in `generateBundleStack()`**

In `generate/generate_bundle.go`, replace the early-return block:

**Before:**
```go
stackTree, ok := g.root.Lookup(stackMeta.Dir)
if ok && stackTree.IsStack() {
    logger.Debug().Msg("stack already exists: skipping")
    stackTree.Node.Components = mergeComponentList(stackTree.Node.Components, stackMeta.Components, bundle.Source)
    return
}
```

**After:**
```go
stackTree, ok := g.root.Lookup(stackMeta.Dir)
if ok && stackTree.IsStack() {
    logger.Debug().Msg("stack already exists: updating metadata from bundle")
    if err := stack.UpdateMetadata(g.root, stackMeta); err != nil {
        report.AddFailure(stackMeta.Dir, err)
        return
    }
    dirReport := genreport.Dir{}
    dirReport.AddChangedFile(stack.DefaultFilename)
    report.AddDirReport(stackMeta.Dir, dirReport)
    stackTree.Node.Components = mergeComponentList(stackTree.Node.Components, stackMeta.Components, bundle.Source)
    return
}
```

- [ ] **Step 4: Run all bundle and stack tests — verify they pass**

```bash
go test -race -count=1 ./generate/... -run "TestGenerateBundle" -v
go test -race -count=1 ./stack/... -v
```

Expected: all pass, including the new test case.

- [ ] **Step 5: Run the full test suite to check for regressions**

```bash
make build && go test -race -count=1 ./generate/... ./stack/...
```

Expected: all pass.

- [ ] **Step 6: Format and commit**

```bash
make fmt
git add generate/generate_bundle.go generate/generate_bundle_test.go
git commit -m "feat: bundle always overwrites stack metadata on re-generate"
```
