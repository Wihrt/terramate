// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

// Package run provides the run command.
package run

import (
	"context"
	"io"

	"github.com/terramate-io/terramate/cloud/api/status"
	"github.com/terramate-io/terramate/commands"
	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/engine"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/hcl/ast"
	"github.com/terramate-io/terramate/printer"
	"github.com/terramate-io/terramate/project"
	"github.com/zclconf/go-cty/cty"
)

const (
	// ErrConflictOptions tells if the error is related to conflicting options in the command spec.
	ErrConflictOptions errors.Kind = "conflicting arguments"
	// ErrCurrentHeadIsOutOfDate indicates the local HEAD revision is outdated.
	ErrCurrentHeadIsOutOfDate errors.Kind = "current HEAD is out-of-date with the remote base branch"
	// ErrOutdatedGenCodeDetected indicates outdated generated code detected.
	ErrOutdatedGenCodeDetected errors.Kind = "outdated generated code detected"
)

// Spec is the command specification for the run command.
type Spec struct {
	// Behavior control options
	Command         []string
	Quiet           bool
	DryRun          bool
	Reverse         bool
	ScriptRun       bool
	ContinueOnError bool
	Parallel        int
	NoRecursive     bool

	Terragrunt    bool
	EnableSharing bool
	MockOnFail    bool
	EvalCmd       bool

	GitFilter engine.GitFilter
	Tags      []string
	NoTags    []string

	engine.DependencyFilters

	Safeguards Safeguards

	workingDir string
	engine     *engine.Engine
	printers   printer.Printers
	stdout     io.Writer
	stderr     io.Writer
	stdin      io.Reader
}

// Safeguards holds the safeguard options for the run command.
type Safeguards struct {
	DisableCheckGitUntracked          bool
	DisableCheckGitUncommitted        bool
	DisableCheckGitRemote             bool
	DisableCheckGenerateOutdatedCheck bool

	ReEnabled bool
}

// Name returns the name of the command.
func (s *Spec) Name() string { return "run" }

// Requirements returns the requirements of the command.
func (s *Spec) Requirements(context.Context, commands.CLI) any {
	return commands.RequireEngine(
		commands.WithTerragrunt(s.GitFilter.IsChanged || s.HasDependencyFilters()),
	)
}

// Exec executes the run command.
func (s *Spec) Exec(ctx context.Context, cli commands.CLI) error {
	s.workingDir = cli.WorkingDir()
	s.engine = cli.Engine()
	s.printers = cli.Printers()
	s.stdout = cli.Stdout()
	s.stderr = cli.Stderr()
	s.stdin = cli.Stdin()

	if len(s.Command) == 0 {
		return errors.E("run expects a command")
	}

	err := CheckOutdatedGeneratedCode(ctx, s.engine, s.Safeguards, s.workingDir)
	if err != nil {
		return err
	}

	cfg := s.engine.Config()
	rootdir := cfg.HostDir()
	var stacks config.List[*config.SortableStack]
	if s.NoRecursive {
		st, found, err := config.TryLoadStack(cfg, project.PrjAbsPath(rootdir, s.workingDir))
		if err != nil {
			return errors.E(err, "loading stack in current directory")
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
			err = GitFileSafeguards(s.engine, true, s.Safeguards)
			if err != nil {
				return err
			}
		}
	}

	err = GitSafeguardDefaultBranchIsReachable(s.engine, s.Safeguards)
	if err != nil {
		return err
	}

	var runs []engine.StackRun
	for _, st := range stacks {
		run := engine.StackRun{
			SyncTaskIndex: -1,
			Stack:         st.Stack,
			Tasks: []engine.StackRunTask{
				{
					Cmd:           s.Command,
					UseTerragrunt: s.Terragrunt,
					EnableSharing: s.EnableSharing,
					MockOnFail:    s.MockOnFail,
				},
			},
		}
		if s.EvalCmd {
			run.Tasks[0].Cmd, err = s.evalRunArgs(run.Stack, run.Tasks[0].Cmd)
			if err != nil {
				return errors.D("%s", "unable to evaluate command").WithError(err)
			}
		}
		runs = append(runs, run)
	}

	err = s.engine.RunAll(runs, engine.RunAllOptions{
		Quiet:           s.Quiet,
		DryRun:          s.DryRun,
		Reverse:         s.Reverse,
		ScriptRun:       false,
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

func (s *Spec) evalRunArgs(st *config.Stack, cmd []string) ([]string, error) {
	ctx, err := s.engine.SetupEvalContext(st.HostDir(s.engine.Config()), st, "", map[string]string{})
	if err != nil {
		return nil, err
	}
	var newargs []string
	for _, arg := range cmd {
		exprStr := `"` + arg + `"`
		expr, err := ast.ParseExpression(exprStr, "<cmd arg>")
		if err != nil {
			return nil, errors.E(err, "parsing %s", exprStr)
		}
		val, err := ctx.Eval(expr)
		if err != nil {
			return nil, errors.E(err, "eval %s", exprStr)
		}
		if !val.Type().Equals(cty.String) {
			return nil, errors.E("cmd line evaluates to type %s but only string is permitted", val.Type().FriendlyName())
		}

		newargs = append(newargs, val.AsString())
	}
	return newargs, nil
}
