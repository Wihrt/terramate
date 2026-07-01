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
	"github.com/terramate-io/terramate/test"
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
				Tags:        []string{"env-prod", "team-platform"},
				After:       []string{"/other"},
				Before:      []string{"/another"},
				Wants:       []string{"/wanted"},
				WantedBy:    []string{"/wanting"},
				Watch:       []string{"/path/to/watched"},
			},
			wantStack: config.Stack{
				Dir:         project.NewPath("/mystack"),
				Name:        "My Stack",
				Description: "A test stack",
				Tags:        []string{"env-prod", "team-platform"},
				After:       []string{"/other"},
				Before:      []string{"/another"},
				Wants:       []string{"/wanted"},
				WantedBy:    []string{"/wanting"},
				Watch:       project.Paths{project.NewPath("/path/to/watched")},
			},
		},
		{
			name: "clears optional fields when empty",
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
			name: "preserves existing stack ID",
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

			_, err = stack.UpdateMetadata(root, tc.stackMeta)
			assert.NoError(t, err)

			// Idempotency: second call with same metadata should report no change.
			changed, err := stack.UpdateMetadata(root, tc.stackMeta)
			assert.NoError(t, err)
			if changed {
				t.Errorf("expected no change on second call with same metadata, got changed=true")
			}

			got := s.LoadStack(tc.stackMeta.Dir)
			if tc.wantStack.Name == "" {
				tc.wantStack.Name = filepath.Base(tc.stackMeta.Dir.String())
			}
			tc.wantStack.Dir = tc.stackMeta.Dir
			assert.EqualStrings(t, tc.wantStack.Name, got.Name, "name mismatch")
			assert.EqualStrings(t, tc.wantStack.Description, got.Description, "description mismatch")
			if tc.wantStack.ID != "" {
				assert.EqualStrings(t, tc.wantStack.ID, got.ID, "ID must be preserved")
			}
			test.AssertEqualSets[string](t, got.Tags, tc.wantStack.Tags)
			test.AssertEqualSets[string](t, got.After, tc.wantStack.After)
			test.AssertEqualSets[string](t, got.Before, tc.wantStack.Before)
			test.AssertEqualSets[string](t, got.Wants, tc.wantStack.Wants)
			test.AssertEqualSets[string](t, got.WantedBy, tc.wantStack.WantedBy)
			test.AssertEqualSets[project.Path](t, got.Watch, tc.wantStack.Watch)
		})
	}
}
