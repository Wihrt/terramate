// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

// Package run provides the script run command.
package run

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/terramate-io/terramate/cloud/api/status"
	"github.com/terramate-io/terramate/commands"

	runcmd "github.com/terramate-io/terramate/commands/run"
	"github.com/terramate-io/terramate/commands/script"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/engine"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/globals"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/terramate-io/terramate/printer"
	"github.com/terramate-io/terramate/project"
	"github.com/terramate-io/terramate/stdlib"
	"github.com/zclconf/go-cty/cty"

	tel "github.com/terramate-io/terramate/ui/tui/telemetry"
)

// Spec represents the script run specification.
type Spec struct {
	Safeguards      runcmd.Safeguards
	DryRun          bool
	Quiet           bool
	Reverse         bool
	Parallel        int
	ContinueOnError bool
	GitFilter       engine.GitFilter
	NoRecursive     bool
	NoTags          []string
	Tags            []string
	engine.DependencyFilters

	Labels []string

	workingDir string
	engine     *engine.Engine
	printers   printer.Printers

	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}

// Name returns the name of the script run command.
func (s *Spec) Name() string { return "script run" }

// Requirements returns the requirements of the command.
func (s *Spec) Requirements(context.Context, commands.CLI) any {
	return commands.RequireEngine(
		commands.WithTerragrunt(s.GitFilter.IsChanged || s.HasDependencyFilters()),
		commands.WithExperiments(script.ExperimentName),
	)
}

// Exec executes the script run command.
func (s *Spec) Exec(ctx context.Context, cli commands.CLI) error {
	s.workingDir = cli.WorkingDir()
	s.engine = cli.Engine()
	s.printers = cli.Printers()
	s.stdout = cli.Stdout()
	s.stderr = cli.Stderr()
	s.stdin = cli.Stdin()

	err := runcmd.CheckOutdatedGeneratedCode(ctx, s.engine, s.Safeguards, s.workingDir)
	if err != nil {
		return err
	}

	root := s.engine.Config()

	var stacks config.List[*config.SortableStack]
	if s.NoRecursive {
		st, found, err := config.TryLoadStack(root, project.PrjAbsPath(root.HostDir(), s.workingDir))
		if err != nil {
			return errors.E(err, "failed to load stack in current directory")
		}

		if !found {
			return errors.E("--no-recursive provided but no stack found in the current directory")
		}

		stacks = append(stacks, st.Sortable())
		stacks, err = s.engine.AddOutputDependencies(s.DependencyFilters, stacks, "")
		if err != nil {
			return err
		}
	} else {
		noFilters, err := status.ParseFilters("", "", "")
		if err != nil {
			return err
		}
		tags, err := engine.ParseFilterTags(s.Tags, s.NoTags)
		if err != nil {
			return err
		}
		stacks, err = s.engine.ComputeSelectedStacks(s.GitFilter, tags, s.DependencyFilters, "", noFilters)
		if err != nil {
			return err
		}

		if !s.DryRun {
			err = runcmd.GitFileSafeguards(s.engine, true, s.Safeguards)
			if err != nil {
				return err
			}
		}
	}

	err = runcmd.GitSafeguardDefaultBranchIsReachable(s.engine, s.Safeguards)
	if err != nil {
		return err
	}

	// search for the script and prepare a list of script/stack entries
	m := script.NewMatcher(s.Labels)
	m.Search(root, stacks)

	if len(m.Results) == 0 {
		return errors.E(color.RedString("script not found: ") + strings.Join(s.Labels, " "))
	}

	if s.DryRun {
		s.printers.Stderr.Println("This is a dry run, commands will not be executed.")
	}

	var runs []engine.StackRun

	for scriptIdx, result := range m.Results {
		if len(result.Stacks) == 0 {
			continue
		}

		if !s.Quiet {
			s.printers.Stderr.Println(fmt.Sprintf("Script %s at %s having %s job(s)",
				color.GreenString(fmt.Sprintf("%d", scriptIdx)),
				color.BlueString(result.ScriptCfg.Range.String()),
				color.BlueString(fmt.Sprintf("%d", len(result.ScriptCfg.Jobs))),
			))
		}

		for _, st := range result.Stacks {
			run := engine.StackRun{Stack: st.Stack}

			ectx, err := scriptEvalContext(root, st.Stack, "")
			if err != nil {
				return errors.E(err, "failed to get context")
			}

			evalScript, err := config.EvalScript(ectx, *result.ScriptCfg)
			if err != nil {
				return errors.E(err, "failed to eval script")
			}

			for jobIdx, job := range evalScript.Jobs {
				for cmdIdx, cmd := range job.Commands() {
					task := engine.StackRunTask{
						Cmd:          cmd.Args,
						ScriptIdx:    scriptIdx,
						ScriptJobIdx: jobIdx,
						ScriptCmdIdx: cmdIdx,
					}

					if cmd.Options != nil {
						task.UseTerragrunt = cmd.Options.UseTerragrunt
						task.EnableSharing = cmd.Options.EnableSharing
						task.MockOnFail = cmd.Options.MockOnFail

						tel.DefaultRecord.Set(
							tel.BoolFlag("terragrunt", cmd.Options.UseTerragrunt),
							tel.BoolFlag("output-sharing", cmd.Options.EnableSharing),
							tel.BoolFlag("output-mocks", cmd.Options.MockOnFail),
						)
					}
					run.Tasks = append(run.Tasks, task)
				}
			}

			runs = append(runs, run)
		}
	}

	err = s.engine.RunAll(runs, engine.RunAllOptions{
		ScriptRun:       true,
		Quiet:           s.Quiet,
		DryRun:          s.DryRun,
		Reverse:         s.Reverse,
		ContinueOnError: s.ContinueOnError,
		Parallel:        s.Parallel,
		Stdout:          s.stdout,
		Stderr:          s.stderr,
		Stdin:           s.stdin,
	})
	if err != nil {
		return errors.D("%s", "one or more commands failed").WithError(err)
	}
	return nil
}

func scriptEvalContext(root *config.Root, st *config.Stack, target string) (*eval.Context, error) {
	globalsReport := globals.ForStack(root, st)
	if err := globalsReport.AsError(); err != nil {
		return nil, err
	}

	evalctx := eval.NewContext(stdlib.Functions(st.HostDir(root), root.Tree().Node.Experiments()))
	runtime := root.Runtime()
	runtime.Merge(st.RuntimeValues(root))

	if target != "" {
		runtime["target"] = cty.StringVal(target)
	}

	evalctx.SetNamespace("terramate", runtime)
	evalctx.SetNamespace("global", globalsReport.Globals.AsValueMap())
	evalctx.SetEnv(os.Environ())

	return evalctx, nil
}
