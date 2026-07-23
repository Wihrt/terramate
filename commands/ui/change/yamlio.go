// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package change

import (
	"cmp"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	hhcl "github.com/terramate-io/hcl/v2"
	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/typeschema"
	"github.com/terramate-io/terramate/yaml"
)

// Save writes the change to disk as a YAML bundle instance file.
func (c *Change) Save(envs []*config.Environment) error {
	var existing *yaml.BundleInstance

	_, err := os.Stat(c.HostPath)
	overwrite := err == nil

	// If the change doesn't have an env, the bundle doesn't have envs, so no need for merging.
	if overwrite && c.Env != nil {
		var err error
		existing, err = loadBundleYAMLConfig(c.HostPath)
		if err != nil {
			return err
		}
	}

	content, err := c.generateBundleYAML(existing, envs)
	if err != nil {
		return err
	}
	return writeBundleInstance(c.HostPath, content)
}

func (c *Change) generateBundleYAML(existing *yaml.BundleInstance, envs []*config.Environment) (string, error) {
	inputs := yaml.Map[any]{}
	for _, def := range c.InputDefs {
		if IsPseudoKey(def.Name) {
			continue
		}
		v, found := c.UserValues[def.Name]
		if !found || v == cty.NilVal {
			continue
		}

		// Bundle-ref values are stored as resolved objects internally
		// but must be written as alias strings in the YAML config.
		if _, isBundleType := def.Type.(*typeschema.BundleType); isBundleType {
			if v.IsKnown() && !v.IsNull() && v.Type().IsObjectType() && v.Type().HasAttribute("alias") {
				v = v.GetAttr("alias")
			}
		}

		yv, err := yaml.ConvertFromCty(v)
		if err != nil {
			return "", err
		}
		inputs = append(inputs, yaml.MapItem[any]{
			Key:   yaml.Attr(def.Name, 0, 0, formatTmdoc(def.Description)),
			Value: yaml.Attr(yv),
		})
	}

	var envID string
	if c.Env != nil {
		envID = c.Env.ID
	}

	if c.UUID == "" {
		c.UUID = uuid.NewString()
	}

	var bundle yaml.BundleInstance
	if envID != "" {
		envBlock := &yaml.BundleEnvironment{
			Source: yaml.Attr[any](c.Source),
			Inputs: yaml.Attr(inputs),
		}
		if existing != nil {
			bundle = mergeBundleYAMLEnv(*existing, envID, envBlock, envs)
		} else {
			bundle = yaml.BundleInstance{
				Name: yaml.Attr(c.Name),
				UUID: yaml.Attr(c.UUID),
				Environments: yaml.Attr(
					yaml.Map[*yaml.BundleEnvironment]{
						{Key: yaml.Attr(envID), Value: yaml.Attr(envBlock)},
					},
				),
			}
		}
	} else {
		bundle = yaml.BundleInstance{
			Name:   yaml.Attr(c.Name),
			UUID:   yaml.Attr(c.UUID),
			Source: yaml.Attr[any](c.Source),
			Inputs: yaml.Attr(inputs),
		}
	}

	var b strings.Builder
	err := yaml.Encode(&bundle, &b)
	if err != nil {
		return "", err
	}
	// Strip any trailling whitespace.
	output := trailingWSRE.ReplaceAllString(b.String(), "")
	return output, nil
}

func loadBundleYAMLConfig(p string) (*yaml.BundleInstance, error) {
	if !hasYAMLConfigExt(p) {
		return nil, errors.E("File %q is not a .tm.yml file.", p)
	}

	r, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = r.Close()
	}()

	var bundle yaml.BundleInstance
	err = yaml.Decode(r, &bundle)
	if err != nil {
		loc := hhcl.Range{
			Filename: p,
		}
		var yamlErr yaml.Error
		if errors.As(err, &yamlErr) {
			loc.Start = hhcl.Pos{Line: yamlErr.Line, Column: yamlErr.Column}
			loc.End = hhcl.Pos{Line: yamlErr.Line, Column: yamlErr.Column}
		}

		return nil, errors.E(err, loc)
	}

	return &bundle, nil
}

func hasYAMLConfigExt(fn string) bool {
	return strings.HasSuffix(fn, ".tm.yml") || strings.HasSuffix(fn, ".tm.yaml")
}

func mergeBundleYAMLEnv(existing yaml.BundleInstance, envID string, env *yaml.BundleEnvironment, envs []*config.Environment) yaml.BundleInstance {
	merged := existing

	// If the same entry already is in spec of the existing bundle, then don't write it.
	var filteredInputs yaml.Map[any]
	for _, input := range env.Inputs.V {
		if !slices.ContainsFunc(merged.Inputs.V, func(other yaml.MapItem[any]) bool {
			return input.Key.V == other.Key.V &&
				input.Key.HeadComment == other.Key.HeadComment &&
				input.Key.LineComment == other.Key.LineComment &&
				input.Key.FootComment == other.Key.FootComment &&
				input.Value.HeadComment == other.Value.HeadComment &&
				input.Value.LineComment == other.Value.LineComment &&
				input.Value.FootComment == other.Value.FootComment &&
				reflect.DeepEqual(input.Value.V, other.Value.V)
		}) {
			filteredInputs = append(filteredInputs, input)
		}
	}
	env.Inputs.V = filteredInputs

	newMapItem := yaml.MapItem[*yaml.BundleEnvironment]{Key: yaml.Attr(envID), Value: yaml.Attr(env)}

	replaceIndex := slices.IndexFunc(merged.Environments.V, func(a yaml.MapItem[*yaml.BundleEnvironment]) bool {
		return a.Key.V == envID
	})
	if replaceIndex != -1 {
		merged.Environments.V[replaceIndex] = newMapItem
	} else {
		indexMap := make(map[string]int, len(envs))
		for i, e := range envs {
			indexMap[e.ID] = i
		}
		merged.Environments.V = append(merged.Environments.V, newMapItem)
		slices.SortFunc(merged.Environments.V, func(a, b yaml.MapItem[*yaml.BundleEnvironment]) int {
			return cmp.Compare(indexMap[a.Key.V], indexMap[b.Key.V])
		})
	}

	return merged
}

var trailingWSRE = regexp.MustCompile(`(?m)[ \t]+$`)

func formatTmdoc(in string) string {
	lines := strings.Split(in, "\n")
	// Remove empty last line.
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for i, s := range lines {
		lines[i] = "# tmdoc: " + s
	}
	return strings.Join(lines, "\n")
}

func writeBundleInstance(outpath string, content string) error {
	if err := os.MkdirAll(filepath.Dir(outpath), 0755); err != nil {
		return errors.E(err, "failed to create directory")
	}

	f, err := os.Create(outpath)
	if err != nil {
		return errors.E(err, "creating/truncating file")
	}

	defer func() {
		errClose := f.Close()
		if errClose != nil {
			err = errors.L(err, errClose)
		}
	}()

	_, err = f.WriteString(content)
	return err
}

func fixupFileExtension(format, fn string) string {
	switch format {
	case "yaml":
		if strings.HasSuffix(fn, ".hcl") {
			return strings.TrimSuffix(fn, "hcl") + "yml"
		}
		if strings.HasSuffix(fn, ".tm") {
			return fn + ".yml"
		}
	case "hcl":
		if strings.HasSuffix(fn, ".yml") {
			return strings.TrimSuffix(fn, "yml") + "hcl"
		}
		if strings.HasSuffix(fn, ".yaml") {
			return strings.TrimSuffix(fn, "yaml") + "hcl"
		}
		if strings.HasSuffix(fn, ".tm") {
			return fn + ".hcl"
		}
	}
	return fn
}
