// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package change

import (
	"os"
	"strings"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/hcl"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/zclconf/go-cty/cty"
)

// PseudoKeyOutputName and PseudoKeyOutputPath are reserved input names
// injected by the UI for the instance name and output path form fields.
const (
	PseudoKeyOutputName = "__output_name__"
	PseudoKeyOutputPath = "__output_path__"
)

// IsPseudoKey returns true for reserved input names injected by the caller (e.g. "__output_name__", "__output_path__").
func IsPseudoKey(name string) bool {
	return strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__")
}

// ExtractPseudoString reads a synthetic string value from a values map.
func ExtractPseudoString(values map[string]cty.Value, key string) string {
	if v, ok := values[key]; ok && v != cty.NilVal {
		return v.AsString()
	}
	return ""
}

// InputsToValueMap converts a map[string]cty.Value to map[string]cty.Value,
// unwrapping the {"value": v} object that EvalInputs wraps each input in.
func InputsToValueMap(inputs map[string]cty.Value) map[string]cty.Value {
	out := make(map[string]cty.Value, len(inputs))
	for k, v := range inputs {
		out[k] = v.GetAttr("value")
	}
	return out
}

// DisplayNameFromAlias returns the short name when the alias is the implicit <path>:<name> form, the alias otherwise.
func DisplayNameFromAlias(alias, name string) string {
	if strings.HasSuffix(alias, ":"+name) {
		return name
	}
	return alias
}

// isBundleUnique checks that no existing bundle conflicts with the given alias and class.
func isBundleUnique(r *config.Registry, alias, classID, hostPath string, env *config.Environment) error {
	skipFileExistsCheck := false

	for _, b := range r.Bundles {
		bundleHostPath := b.Info.HostPath()
		if classID == b.DefinitionMetadata.Class && alias == b.Alias {
			if env != nil && b.Environment != nil {
				if env.ID == b.Environment.ID {
					return errors.E("A bundle with alias %q already exists for environment %s at %s", b.Alias, env.ID, bundleHostPath)
				}
				// Same alias+class, but different env. This is ok.
				// We have to assume the file exists already in this case.
				skipFileExistsCheck = true
			} else {
				return errors.E("A bundle with alias %q already exists at %s", b.Alias, bundleHostPath)
			}
		}
	}
	if hostPath != "" && !skipFileExistsCheck {
		_, err := os.Stat(hostPath)
		if err == nil {
			return errors.E("A file already exists at the target output path %s", hostPath)
		}
	}

	return nil
}

// BundleRequiresEnv reports whether the definition declares environments.required = true.
func BundleRequiresEnv(evalctx *eval.Context, def *hcl.DefineBundle) bool {
	if def.Environments.Required == nil {
		return false
	}
	envRequired, err := config.EvalBool(evalctx, def.Environments.Required.Expr, "environments.required")
	if err != nil {
		return false
	}
	return envRequired
}

// setupExplicitBundleAlias evaluates the definition's explicit alias, if any, and registers it in the bundle namespace.
func setupExplicitBundleAlias(evalctx *eval.Context, bundleDef *hcl.DefineBundle) (string, error) {
	if bundleDef.Alias != nil {
		alias, err := config.EvalString(evalctx, bundleDef.Alias.Expr, "alias")
		if err != nil {
			return "", err
		}
		if alias == "" {
			return "", nil
		}

		var bundleVals map[string]cty.Value
		if ns, ok := evalctx.GetNamespace("bundle"); ok {
			bundleVals = ns.AsValueMap()
		}
		if bundleVals == nil {
			bundleVals = map[string]cty.Value{}
		}
		bundleVals["alias"] = cty.StringVal(alias)
		evalctx.SetNamespace("bundle", bundleVals)

		return alias, nil
	}
	return "", nil
}
