// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package stack

import (
	"bytes"
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
// It returns true if the file was actually written (content changed), false if
// the file was already up to date.
func UpdateMetadata(root *config.Root, stackMeta config.StackMetadata) (bool, error) {
	hostpath := stackMeta.Dir.HostPath(root.HostDir())
	stackFilePath := filepath.Join(hostpath, DefaultFilename)

	st, err := os.Lstat(stackFilePath)
	if err != nil {
		return false, errors.E(err, "stating the stack file")
	}
	originalFileMode := st.Mode()

	stackContents, err := os.ReadFile(stackFilePath)
	if err != nil {
		return false, errors.E(err, "reading stack file")
	}

	parsed, diags := hclwrite.ParseConfig(stackContents, stackFilePath, hhcl.InitialPos)
	if diags.HasErrors() {
		return false, errors.E(diags, "parsing stack file")
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

		newContents := parsed.Bytes()
		if bytes.Equal(newContents, stackContents) {
			return false, nil
		}
		return true, os.WriteFile(stackFilePath, newContents, originalFileMode)
	}

	return false, errors.E("stack block not found in %s", stackFilePath)
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
