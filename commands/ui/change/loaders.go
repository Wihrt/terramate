// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package change

import (
	"context"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/terramate-io/terramate/stdlib"
	"github.com/zclconf/go-cty/cty"
)

// NewBundleEvalContext returns a child eval context with the bundle namespace
// populated for env and the bundle/bundles functions registered against reg.
func NewBundleEvalContext(evalctx *eval.Context, reg *config.Registry, env *config.Environment) *eval.Context {
	evalctx = evalctx.ChildContext()

	var bundleVals map[string]cty.Value
	if bundleNS, ok := evalctx.GetNamespace("bundle"); ok {
		bundleVals = bundleNS.AsValueMap()
	} else {
		bundleVals = map[string]cty.Value{}
	}
	bundleVals["environment"] = config.MakeEnvObject(env)
	evalctx.SetNamespace("bundle", bundleVals)

	evalctx.SetFunction(stdlib.Name("bundle"), config.BundleFunc(context.TODO(), reg, env, false))
	evalctx.SetFunction(stdlib.Name("bundles"), config.BundlesFunc(reg, env))
	return evalctx
}

// MakeBundleDefinitionEntry constructs a BundleDefinitionEntry from an existing, already loaded bundle.
func MakeBundleDefinitionEntry(root *config.Root, b *config.Bundle) *config.BundleDefinitionEntry {
	// This cannot fail. If we have the evaluated config.Bundle already, the HCL define must exist.
	tree, _ := root.Lookup(b.ResolvedSource)
	for _, def := range tree.Node.Defines {
		if def.Bundle != nil {
			return &config.BundleDefinitionEntry{
				Tree:     tree,
				Metadata: &b.DefinitionMetadata,
				Define:   def.Bundle,
			}
		}
	}
	return nil
}

// RawInputKeys returns the set of input names that were explicitly provided
// in the bundle's YAML file (before default evaluation).
func RawInputKeys(b *config.Bundle, evalctx *eval.Context) map[string]bool {
	keys := make(map[string]bool)
	if b.Inst == nil {
		return keys
	}
	// Block-style: inputs { key = val }
	if b.Inst.Inputs != nil {
		for name := range b.Inst.Inputs.Attributes {
			keys[name] = true
		}
	}
	// Attribute-style: inputs = { key = val }
	if b.Inst.InputsAttr != nil && evalctx != nil {
		val, err := evalctx.Eval(b.Inst.InputsAttr.Expr)
		if err == nil && val.Type().IsObjectType() {
			for name := range val.AsValueMap() {
				keys[name] = true
			}
		}
	}
	return keys
}

// MissingBundleRefs walks the inputs of b and returns the aliases of any
// referenced bundles (cty objects with alias, class, environment.available==true)
// that are absent from targetAliases.
func MissingBundleRefs(b *config.Bundle, targetAliases map[string]bool) []string {
	seen := make(map[string]bool)
	var missing []string

	var walk func(v cty.Value)
	walk = func(v cty.Value) {
		if !v.IsKnown() || v.IsNull() {
			return
		}
		t := v.Type()
		if t.IsObjectType() &&
			t.HasAttribute("alias") &&
			t.HasAttribute("class") &&
			t.HasAttribute("environment") {
			envVal := v.GetAttr("environment")
			if envVal.IsKnown() && !envVal.IsNull() &&
				envVal.Type().IsObjectType() &&
				envVal.Type().HasAttribute("available") {
				avail := envVal.GetAttr("available")
				if avail.IsKnown() && !avail.IsNull() && avail.True() {
					alias := v.GetAttr("alias").AsString()
					if !targetAliases[alias] && !seen[alias] {
						seen[alias] = true
						missing = append(missing, alias)
					}
				}
			}
			return // do not descend into the referenced bundle's embedded inputs/exports
		}

		if t.IsObjectType() || t.IsMapType() ||
			t.IsListType() || t.IsTupleType() || t.IsSetType() {
			for it := v.ElementIterator(); it.Next(); {
				_, elem := it.Element()
				walk(elem)
			}
		}
	}

	for _, v := range b.Inputs {
		walk(v)
	}
	return missing
}
