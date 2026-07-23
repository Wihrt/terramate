// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

// Package change implements the bundle-change domain of the interactive
// UI: building create/reconfigure/promote changes from form values and
// persisting them as YAML bundle instance files. It has no dependency on
// the terminal rendering layer.
package change

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/uuid"
	hhcl "github.com/terramate-io/hcl/v2"
	"github.com/terramate-io/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/hcl"
	"github.com/terramate-io/terramate/hcl/ast"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/stdlib"
	"github.com/terramate-io/terramate/typeschema"
)

// Kind indicates the type of change.
type Kind string

// KindCreate and the following constants enumerate the supported change kinds.
const (
	KindCreate   Kind = "change_create"
	KindReconfig Kind = "change_reconfig"
	KindPromote  Kind = "change_promote"
)

// Change represents a bundle change (create, reconfigure, or promote).
type Change struct {
	Kind        Kind
	HostPath    string
	ProjectPath string
	Name        string
	UUID        string
	Source      string
	Alias       string

	DisplayName string
	Metadata    config.Metadata

	BundleDefEntry *config.BundleDefinitionEntry
	InputDefs      []*config.InputDefinition // Original input definitions
	Values         map[string]cty.Value      // All values (user + defaults) for validation
	UserValues     map[string]cty.Value      // Only user-set values (written to YAML)

	Env     *config.Environment
	FromEnv *config.Environment

	OriginalBundle *config.Bundle       // Only set for reconfig and promote
	OriginalValues map[string]cty.Value // Baseline values from disk (for change detection)

	Warnings []string // Non-fatal warnings surfaced after resolving the change

}

// Session is the narrow slice of engine state the change layer reads.
// The TUI projects its EngineState onto it; tests can construct it
// directly.
type Session struct {
	Context    context.Context
	Registry   *config.Registry
	RootDir    string // host absolute path of the project root
	WorkingDir string
}

// NewCreate builds a Change that represents a new bundle creation.
func NewCreate(
	s Session,
	activeEnv *config.Environment,
	bde *config.BundleDefinitionEntry,
	schemactx typeschema.EvalContext,
	inputDefs []*config.InputDefinition,
	values map[string]cty.Value,
) (Change, error) {
	schemactx = schemactx.ChildContext()
	schemactx.Evalctx.SetFunction(stdlib.Name("bundle"), config.BundleFunc(s.Context, s.Registry, activeEnv, false))
	schemactx.Evalctx.SetFunction(stdlib.Name("bundles"), config.BundlesFunc(s.Registry, activeEnv))

	// The form may or may not contain values for all defaults.
	// In this step we re-run input evaluation like it would be done if this was a bundle instance that
	// has the form values as inputs. This will ensure we get all the inputs.
	allValues, err := reEvalAllInputs(
		schemactx,
		bde.Tree.HostDir(),
		bde.Define,
		values,
	)
	if err != nil {
		return Change{}, err
	}
	if err := checkBundleRefsResolved(inputDefs, allValues); err != nil {
		return Change{}, err
	}

	if err := config.LoadBundleLets(schemactx.Evalctx, bde.Define.Lets); err != nil {
		return Change{}, err
	}

	// We check if the bundle has an explicit alias and add it to the context if yes.
	alias, err := setupExplicitBundleAlias(schemactx.Evalctx, bde.Define)
	if err != nil {
		return Change{}, err
	}

	bundleDef, err := config.EvalBundleDefinition(schemactx.Evalctx, bde.Define)
	if err != nil {
		return Change{}, err
	}

	outputPath := bundleDef.ScaffoldingPath
	if outputPath == "" {
		outputPath = ExtractPseudoString(values, PseudoKeyOutputPath)
	}
	outputName := bundleDef.ScaffoldingName
	if outputName == "" {
		outputName = ExtractPseudoString(values, PseudoKeyOutputName)
	}

	if outputPath == "" {
		return Change{}, errors.E("Bundle path is not set.")
	}
	if outputName == "" {
		return Change{}, errors.E("Bundle name is not set.")
	}

	var hostPath string
	var projPath string
	if filepath.IsAbs(outputPath) {
		hostPath = project.NewPath(outputPath).HostPath(s.RootDir)
		projPath = outputPath
	} else {

		hostPath = filepath.Join(s.WorkingDir, outputPath)
		projPath = project.PrjAbsPath(s.RootDir, hostPath).String()
	}
	hostPath = fixupFileExtension("yaml", hostPath)

	// If there is no explicit alias, we fallback to the <path>:<name> default.
	// But we must do this only after these values are known.
	// If there was an explicit alias, it can actually be used in path and name.
	// Sounds circular but it isn't...
	if alias == "" {
		alias = fmt.Sprintf("%s:%s", filepath.Dir(outputPath), outputName)
	}

	var env *config.Environment
	if activeEnv != nil && BundleRequiresEnv(schemactx.Evalctx, bde.Define) {
		env = activeEnv
	}

	// Final check: Is the bundle unique?
	if err := isBundleUnique(s.Registry, alias, bde.Metadata.Class, hostPath, env); err != nil {
		return Change{}, err
	}

	return Change{
		Kind:           KindCreate,
		HostPath:       hostPath,
		ProjectPath:    projPath,
		Name:           outputName,
		UUID:           uuid.NewString(),
		Source:         bde.Source,
		Env:            env,
		Alias:          alias,
		DisplayName:    DisplayNameFromAlias(alias, outputName),
		Metadata:       *bde.Metadata,
		BundleDefEntry: bde,
		InputDefs:      inputDefs,
		Values:         allValues,
		UserValues:     values,
	}, nil
}

// NewReconfig builds a Change that represents reconfiguring an existing bundle.
func NewReconfig(
	s Session,
	bundle *config.Bundle,
	bde *config.BundleDefinitionEntry,
	schemactx typeschema.EvalContext,
	inputDefs []*config.InputDefinition,
	values map[string]cty.Value,
) (Change, error) {
	schemactx = schemactx.ChildContext()
	schemactx.Evalctx.SetFunction(stdlib.Name("bundle"), config.BundleFunc(s.Context, s.Registry, bundle.Environment, false))
	schemactx.Evalctx.SetFunction(stdlib.Name("bundles"), config.BundlesFunc(s.Registry, bundle.Environment))

	hostPath := bundle.Info.HostPath()
	projPath := project.PrjAbsPath(s.RootDir, hostPath).String()

	// The form may or may not contain values for all defaults.
	// In this step we re-run input evaluation like it would be done if this was a bundle instance that
	// has the form values as inputs. This will ensure we get all the inputs.
	allValues, err := reEvalAllInputs(
		schemactx,
		hostPath,
		bde.Define,
		values,
	)
	if err != nil {
		return Change{}, err
	}
	if err := checkBundleRefsResolved(inputDefs, allValues); err != nil {
		return Change{}, err
	}

	if err := config.LoadBundleLets(schemactx.Evalctx, bde.Define.Lets); err != nil {
		return Change{}, err
	}

	// This will only be set if there is an explicit alias.
	newAlias, err := setupExplicitBundleAlias(schemactx.Evalctx, bde.Define)
	if err != nil {
		return Change{}, err
	}

	var warnings []string
	if bundle.Alias != "" && newAlias != "" && bundle.Alias != newAlias {
		warnings = append(warnings, fmt.Sprintf("Alias will change from %q to %q", bundle.Alias, newAlias))
	}

	// The alias can change, if it's an explicit alias. If it's the implicit alias,
	// it just remains path:name since neither will change.
	var alias, displayName string
	if newAlias == "" {
		alias = bundle.Alias
		displayName = bundle.Name
	} else {
		alias = newAlias
		displayName = newAlias
	}

	return Change{
		Kind:           KindReconfig,
		HostPath:       hostPath,
		ProjectPath:    projPath,
		Name:           bundle.Name,
		UUID:           bundle.UUID,
		Source:         bundle.Source,
		Env:            bundle.Environment,
		Alias:          alias,
		DisplayName:    displayName,
		Metadata:       bundle.DefinitionMetadata,
		BundleDefEntry: bde,
		InputDefs:      inputDefs,
		Values:         allValues,
		UserValues:     values,
		OriginalBundle: bundle,
		OriginalValues: InputsToValueMap(bundle.Inputs),
		Warnings:       warnings,
	}, nil
}

// NewPromote builds a Change that represents promoting a bundle to another environment.
func NewPromote(
	s Session,
	env *config.Environment,
	bundle *config.Bundle,
	bde *config.BundleDefinitionEntry,
	schemactx typeschema.EvalContext,
	inputDefs []*config.InputDefinition,
	values map[string]cty.Value,
) (Change, error) {
	schemactx = schemactx.ChildContext()
	schemactx.Evalctx.SetFunction(stdlib.Name("bundle"), config.BundleFunc(s.Context, s.Registry, env, false))
	schemactx.Evalctx.SetFunction(stdlib.Name("bundles"), config.BundlesFunc(s.Registry, env))

	hostPath := bundle.Info.HostPath()
	projPath := project.PrjAbsPath(s.RootDir, hostPath).String()

	// The form may or may not contain values for all defaults.
	// In this step we re-run input evaluation like it would be done if this was a bundle instance that
	// has the form values as inputs. This will ensure we get all the inputs.
	allValues, err := reEvalAllInputs(
		schemactx,
		hostPath,
		bde.Define,
		values,
	)
	if err != nil {
		return Change{}, err
	}
	if err := checkBundleRefsResolved(inputDefs, allValues); err != nil {
		return Change{}, err
	}

	if err := config.LoadBundleLets(schemactx.Evalctx, bde.Define.Lets); err != nil {
		return Change{}, err
	}

	// This will only be set if there is an explicit alias.
	newAlias, err := setupExplicitBundleAlias(schemactx.Evalctx, bde.Define)
	if err != nil {
		return Change{}, err
	}

	var warnings []string
	if bundle.Alias != "" && newAlias != "" && bundle.Alias != newAlias {
		warnings = append(warnings, fmt.Sprintf("Alias will change from %q to %q", bundle.Alias, newAlias))
	}

	// The alias can change, if it's an explicit alias. If it's the implicit alias,
	// it just remains path:name since neither will change.
	var alias, displayName string
	if newAlias == "" {
		alias = bundle.Alias
		displayName = bundle.Name
	} else {
		alias = newAlias
		displayName = newAlias
	}

	return Change{
		Kind:           KindPromote,
		HostPath:       hostPath,
		ProjectPath:    projPath,
		Name:           bundle.Name,
		UUID:           bundle.UUID,
		Source:         bundle.Source,
		Env:            env,
		FromEnv:        bundle.Environment,
		Alias:          alias,
		DisplayName:    displayName,
		Metadata:       bundle.DefinitionMetadata,
		BundleDefEntry: bde,
		InputDefs:      inputDefs,
		Values:         allValues,
		UserValues:     values,
		OriginalBundle: bundle,
		OriginalValues: InputsToValueMap(bundle.Inputs),
		Warnings:       warnings,
	}, nil
}

// reEvalAllInputs evaluates the bundle's input definitions using the prompted
// values as a simulated inputs block, filling in defaults for any inputs that
// were not prompted.
func reEvalAllInputs(
	schemactx typeschema.EvalContext,
	filename string,
	defineBundleHCL *hcl.DefineBundle,
	values map[string]cty.Value,
) (map[string]cty.Value, error) {
	inst := &hcl.Bundle{
		Inputs: ast.NewMergedBlock("inputs", []string{}),
	}
	for k, v := range values {
		if v == cty.NilVal || IsPseudoKey(k) {
			continue
		}
		r := hhcl.Range{
			Filename: filename,
		}
		inst.Inputs.Attributes[k] = ast.NewAttribute(filename,
			&hhcl.Attribute{
				Name:  k,
				Expr:  &hclsyntax.LiteralValueExpr{Val: v},
				Range: r,
			})
	}

	tempInputs, err := config.EvalInputs(
		schemactx,
		"bundle",
		inst.Info,
		inst.Inputs,
		inst.InputsAttr,
		defineBundleHCL.Inputs,
	)
	if err != nil {
		return nil, err
	}

	result := make(map[string]cty.Value, len(values))
	for k, v := range values {
		if v != cty.NilVal {
			result[k] = v
		}
	}

	for k, v := range tempInputs {
		if _, exists := result[k]; exists {
			continue
		}
		vm := v.AsValueMap()
		result[k] = vm["value"]
	}
	return result, nil
}

// NormalizeBundleRefValues converts resolved bundle objects back to alias strings.
// When reconfiguring or promoting, bundle-ref inputs are loaded as full objects
// (with alias, uuid, etc.) from disk. The type system expects strings, so we
// extract the alias before the values enter the form.
func NormalizeBundleRefValues(inputDefs []*config.InputDefinition, values map[string]cty.Value) map[string]cty.Value {
	for _, def := range inputDefs {
		if _, isBundleType := def.Type.(*typeschema.BundleType); !isBundleType {
			continue
		}
		v, ok := values[def.Name]
		if !ok || v == cty.NilVal || v.IsNull() || !v.IsKnown() {
			continue
		}
		if v.Type().IsObjectType() && v.Type().HasAttribute("alias") {
			alias := v.GetAttr("alias")
			if alias.IsKnown() && alias.Type() == cty.String {
				values[def.Name] = alias
			}
		}
	}
	return values
}

// checkBundleRefsResolved verifies that all bundle-ref inputs resolved to non-null
// values. Returns a user-friendly error if any referenced bundle is missing.
func checkBundleRefsResolved(inputDefs []*config.InputDefinition, values map[string]cty.Value) error {
	for _, def := range inputDefs {
		if _, isBundleType := def.Type.(*typeschema.BundleType); !isBundleType {
			continue
		}
		v, ok := values[def.Name]
		if !ok || v == cty.NilVal {
			continue
		}
		if v.IsNull() {
			return errors.E("Input %q references a bundle that does not exist in this environment. Promote dependencies first.", def.Name)
		}
	}
	return nil
}
