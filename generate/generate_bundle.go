// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package generate

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/errors"
	genreport "github.com/terramate-io/terramate/generate/report"
	"github.com/terramate-io/terramate/hcl"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/stack"
)

func (g *gState) generateBundleStacks(report *genreport.Report, allowCreate bool) {
	for _, bundle := range g.registry.Bundles {
		for _, stack := range bundle.Stacks {
			g.generateBundleStack(bundle, stack, report, allowCreate)
		}
	}
}

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
		g.updateExistingBundleStack(logger, bundle, stackMeta, stackTree, report)
		return
	}

	if !allowCreate {
		report.AddFailure(stackMeta.Dir, errors.E("stack not generated"))
		return
	}

	g.createBundleStack(logger, bundle, stackMeta, report)
}

// updateExistingBundleStack refreshes the stack metadata of an already
// existing stack from its bundle definition and attaches the bundle's
// runtime components.
func (g *gState) updateExistingBundleStack(logger zerolog.Logger, bundle *config.Bundle, stackMeta config.StackMetadata, stackTree *config.Tree, report *genreport.Report) {
	stackFilePath := filepath.Join(stackMeta.Dir.HostPath(g.root.HostDir()), stack.DefaultFilename)
	if _, statErr := os.Lstat(stackFilePath); statErr != nil {
		logger.Debug().Msg("stack already exists but stack.tm.hcl not found: skipping metadata update")
		attachBundleComponents(stackTree, stackMeta, bundle)
		return
	}
	logger.Debug().Msg("stack already exists: updating metadata from bundle")
	changed, err := stack.UpdateMetadata(g.root, stackMeta)
	if err != nil {
		report.AddFailure(stackMeta.Dir, err)
		return
	}
	if changed {
		dirReport := genreport.Dir{}
		dirReport.AddChangedFile(stack.DefaultFilename)
		report.AddDirReport(stackMeta.Dir, dirReport)
	}
	attachBundleComponents(stackTree, stackMeta, bundle)
}

// createBundleStack creates a new stack on disk from the bundle's stack
// metadata, loads it into the config tree and attaches the bundle's runtime
// components.
func (g *gState) createBundleStack(logger zerolog.Logger, bundle *config.Bundle, stackMeta config.StackMetadata, report *genreport.Report) {
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

	stackTree, ok := g.root.Lookup(stackMeta.Dir)
	if !ok {
		panic(errors.E(errors.ErrInternal, "just created stack %s cannot be loaded", stackMeta.Dir))
	}

	logger.Debug().Msg("adding created file to report")

	dirReport := genreport.Dir{}
	dirReport.AddCreatedFile(stack.DefaultFilename)
	report.AddDirReport(stackMeta.Dir, dirReport)

	attachBundleComponents(stackTree, stackMeta, bundle)
}

// attachBundleComponents attaches the bundle's runtime components to the
// stack's config node.
func attachBundleComponents(stackTree *config.Tree, stackMeta config.StackMetadata, bundle *config.Bundle) {
	stackTree.Node.Components = mergeComponentList(stackTree.Node.Components, stackMeta.Components, bundle.Source)
}

func mergeComponentList(dst, src []*hcl.Component, bundleSource string) []*hcl.Component {
	for _, srcComp := range src {
		contains := false
		for _, dstComp := range dst {
			if srcComp.Name == dstComp.Name {
				contains = true
				break
			}
		}
		if contains {
			continue
		}
		// Clone
		compCopy := *srcComp
		compCopy.FromBundleSource = bundleSource
		dst = append(dst, &compCopy)
	}
	return dst
}
