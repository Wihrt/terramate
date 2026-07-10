// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package tui

import (
	"context"
	"fmt"

	"github.com/terramate-io/terramate/commands"
	clonecmd "github.com/terramate-io/terramate/commands/clone"
	compcmd "github.com/terramate-io/terramate/commands/completions"
	componentcreatecmd "github.com/terramate-io/terramate/commands/component/create"
	generateoriginscmd "github.com/terramate-io/terramate/commands/debug/show/generate_origins"
	debugglobalscmd "github.com/terramate-io/terramate/commands/debug/show/globals"
	debugshowmetadatacmd "github.com/terramate-io/terramate/commands/debug/show/metadata"
	debugshowruntimeenv "github.com/terramate-io/terramate/commands/debug/show/runtime_env"
	evalcmd "github.com/terramate-io/terramate/commands/experimental/eval"
	rungraphcmd "github.com/terramate-io/terramate/commands/experimental/rungraph"
	vendordownloadcmd "github.com/terramate-io/terramate/commands/experimental/vendordownload"
	fmtcmd "github.com/terramate-io/terramate/commands/fmt"
	gencmd "github.com/terramate-io/terramate/commands/generate"
	pkgcreatecmd "github.com/terramate-io/terramate/commands/package/create"
	runcmd "github.com/terramate-io/terramate/commands/run"
	scaffoldcmd "github.com/terramate-io/terramate/commands/scaffold"
	scriptinfocmd "github.com/terramate-io/terramate/commands/script/info"
	scriptlistcmd "github.com/terramate-io/terramate/commands/script/list"
	scriptruncmd "github.com/terramate-io/terramate/commands/script/run"
	scripttreecmd "github.com/terramate-io/terramate/commands/script/tree"
	createcmd "github.com/terramate-io/terramate/commands/stack/create"
	listcmd "github.com/terramate-io/terramate/commands/stack/list"
	triggercmd "github.com/terramate-io/terramate/commands/trigger"
	uicmd "github.com/terramate-io/terramate/commands/ui"
	"github.com/terramate-io/terramate/commands/version"
	"github.com/terramate-io/terramate/engine"
	"github.com/terramate-io/terramate/errors"
	"github.com/terramate-io/terramate/safeguard"
	"github.com/terramate-io/terramate/ui/tui/clitest"

	"github.com/alecthomas/kong"

	tel "github.com/terramate-io/terramate/ui/tui/telemetry"
)

// ErrSetup is the error returned when the CLI fails to setup its initial values.
const ErrSetup errors.Kind = "failed to setup Terramate"

func handleRootVersionFlagAlone(parsedSpec any, _ *CLI) (name string, val any, run func(c *CLI, value any) error, isset bool) {
	p := AsFlagSpec[FlagSpec](parsedSpec)
	if p == nil {
		panic(errors.E(errors.ErrInternal, "please report this as a bug"))
	}

	if p.VersionFlag {
		return "--version", p.VersionFlag, func(c *CLI, _ any) error {
			fmt.Println(c.Version())
			return nil
		}, true
	}
	return "", nil, nil, false
}

// DefaultRootFlagHandlers returns the CLI default flag handlers for global flags
// that can be used alone (without a command).
// For example: terramate --version
func DefaultRootFlagHandlers() []RootFlagHandlers {
	return []RootFlagHandlers{
		handleRootVersionFlagAlone, // handles: terramate --version
	}
}

// SelectCommand selects the command handler and extracts the parameters.
func SelectCommand(ctx context.Context, c *CLI, command string, flags any) (cmd commands.Command, err error) {
	parsedArgs := AsFlagSpec[FlagSpec](flags)
	if parsedArgs == nil {
		panic(errors.E(errors.ErrInternal, "please report this as a bug"))
	}

	// TODO: This function also sets the analytics. Maybe it should be done somewhere else.

	switch command {
	case "version":
		return &version.Spec{
			InfoChan: c.checkpointResponse,
			Full:     parsedArgs.Version.Full,
		}, nil

	case "install-completions":
		kctx := ctx.Value(KongContext).(*kong.Context)

		return &compcmd.Spec{
			Installer: parsedArgs.InstallCompletions,
			KongCtx:   kctx,
		}, nil

	case "fmt", "fmt <files>":
		c.SetCommandAnalytics("fmt",
			tel.BoolFlag("detailed-exit-code", parsedArgs.Fmt.DetailedExitCode),
		)
		return &fmtcmd.Spec{
			Check:            parsedArgs.Fmt.Check,
			DetailedExitCode: parsedArgs.Fmt.DetailedExitCode,
			Files:            parsedArgs.Fmt.Files,
		}, nil

	case "create <path>":
		c.SetCommandAnalytics("create")
		return &createcmd.Spec{
			Path:             parsedArgs.Create.Path,
			IgnoreExisting:   parsedArgs.Create.IgnoreExisting,
			AllTerraform:     parsedArgs.Create.AllTerraform,
			AllTerragrunt:    parsedArgs.Create.AllTerragrunt,
			EnsureStackIDs:   parsedArgs.Create.EnsureStackIDs,
			NoGenerate:       parsedArgs.Create.NoGenerate,
			Imports:          parsedArgs.Create.Import,
			StackID:          parsedArgs.Create.ID,
			StackName:        parsedArgs.Create.Name,
			StackDescription: parsedArgs.Create.Description,
			StackTags:        parsedArgs.Tags,
			StackAfter:       parsedArgs.Create.After,
			StackBefore:      parsedArgs.Create.Before,
			StackWatch:       parsedArgs.Create.Watch,
			StackWants:       parsedArgs.Create.Wants,
			StackWantedBy:    parsedArgs.Create.WantedBy,
			Verbosity:        parsedArgs.Verbose,
		}, nil
	case "create":
		c.SetCommandAnalytics("create",
			tel.BoolFlag("all-terragrunt", parsedArgs.Create.AllTerragrunt),
			tel.BoolFlag("all-terraform", parsedArgs.Create.AllTerraform),
		)
		return &createcmd.Spec{
			IgnoreExisting:   parsedArgs.Create.IgnoreExisting,
			AllTerraform:     parsedArgs.Create.AllTerraform,
			AllTerragrunt:    parsedArgs.Create.AllTerragrunt,
			EnsureStackIDs:   parsedArgs.Create.EnsureStackIDs,
			NoGenerate:       parsedArgs.Create.NoGenerate,
			Imports:          parsedArgs.Create.Import,
			StackID:          parsedArgs.Create.ID,
			StackName:        parsedArgs.Create.Name,
			StackDescription: parsedArgs.Create.Description,
			StackTags:        parsedArgs.Tags,
			StackAfter:       parsedArgs.Create.After,
			StackBefore:      parsedArgs.Create.Before,
			StackWatch:       parsedArgs.Create.Watch,
			StackWants:       parsedArgs.Create.Wants,
			StackWantedBy:    parsedArgs.Create.WantedBy,
			Verbosity:        parsedArgs.Verbose,
		}, nil

	case "list":
		c.SetCommandAnalytics("list",
			tel.BoolFlag("filter-changed", parsedArgs.Changed),
			tel.BoolFlag("filter-tags", len(parsedArgs.Tags) != 0),
			tel.BoolFlag("run-order", parsedArgs.List.RunOrder),
		)
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			parsedArgs.List.EnableChangeDetection,
			parsedArgs.List.DisableChangeDetection,
		)
		if err != nil {
			return nil, err
		}
		return &listcmd.Spec{
			GitFilter: gitfilter,
			Reason:    parsedArgs.List.Why,
			RunOrder:  parsedArgs.List.RunOrder,
			Tags:      parsedArgs.Tags,
			NoTags:    parsedArgs.NoTags,
			DependencyFilters: engine.DependencyFilters{
				IncludeOutputDependencies: parsedArgs.List.IncludeOutputDependencies,
				OnlyOutputDependencies:    parsedArgs.List.OnlyOutputDependencies,
				IncludeAllDependencies:    parsedArgs.List.IncludeAllDependencies,
				IncludeDirectDependencies: parsedArgs.List.IncludeDirectDependencies,
				OnlyAllDependencies:       parsedArgs.List.OnlyAllDependencies,
				OnlyDirectDependencies:    parsedArgs.List.OnlyDirectDependencies,
				ExcludeAllDependencies:    parsedArgs.List.ExcludeAllDependencies,
				IncludeAllDependents:      parsedArgs.List.IncludeAllDependents,
				IncludeDirectDependents:   parsedArgs.List.IncludeDirectDependents,
				OnlyDirectDependents:      parsedArgs.List.OnlyDirectDependents,
				OnlyAllDependents:         parsedArgs.List.OnlyAllDependents,
				ExcludeAllDependents:      parsedArgs.List.ExcludeAllDependents,
			},
		}, nil

	case "generate":
		c.SetCommandAnalytics("generate",
			tel.BoolFlag("detailed-exit-code", parsedArgs.Generate.DetailedExitCode),
			tel.BoolFlag("parallel", parsedArgs.Generate.Parallel > 0),
		)
		return &gencmd.Spec{
			DetailedExitCode: parsedArgs.Generate.DetailedExitCode,
			Parallel:         parsedArgs.Generate.Parallel,
			PrintReport:      true,
		}, nil

	case "scaffold":
		c.SetCommandAnalytics("scaffold")
		return &scaffoldcmd.Spec{
			OutputFormat: parsedArgs.Scaffold.OutputFormat,
			Generate:     parsedArgs.Scaffold.Generate,
		}, nil

	case "ui":
		c.SetCommandAnalytics("ui")
		return &uicmd.Spec{}, nil

	case "component create", "component create <path>":
		c.SetCommandAnalytics("component-create")
		path := parsedArgs.Component.Create.Path
		return &componentcreatecmd.Spec{
			Path: path,
		}, nil

	case "package create <output-dir>":
		c.SetCommandAnalytics("package-create")
		return &pkgcreatecmd.Spec{
			OutputDir: parsedArgs.Package.Create.OutputDir,

			ManifestOnly: parsedArgs.Package.Create.ManifestOnly,

			PackageLocation:    parsedArgs.Package.Create.Location,
			PackageName:        parsedArgs.Package.Create.Name,
			PackageDescription: parsedArgs.Package.Create.Description,
		}, nil

	case "experimental clone <srcdir> <destdir>":
		c.SetCommandAnalytics("clone")
		return &clonecmd.Spec{
			SrcDir:          parsedArgs.Experimental.Clone.SrcDir,
			DstDir:          parsedArgs.Experimental.Clone.DestDir,
			SkipChildStacks: parsedArgs.Experimental.Clone.SkipChildStacks,
			NoGenerate:      parsedArgs.Experimental.Clone.NoGenerate,
		}, nil

	case "experimental vendor download <source> <ref>":
		c.SetCommandAnalytics("vendor-download")
		return &vendordownloadcmd.Spec{
			Source:    parsedArgs.Experimental.Vendor.Download.Source,
			Reference: parsedArgs.Experimental.Vendor.Download.Reference,
			Dir:       parsedArgs.Experimental.Vendor.Download.Dir,
		}, nil

	case "run":
		return nil, errors.E("no command specified")
	case "run <cmd>":
		c.SetCommandAnalytics("run",
			tel.BoolFlag("filter-changed", parsedArgs.Changed),
			tel.BoolFlag("filter-tags", len(parsedArgs.Tags) != 0),
			tel.BoolFlag("terragrunt", parsedArgs.Run.Terragrunt),
			tel.BoolFlag("reverse", parsedArgs.Run.Reverse),
			tel.BoolFlag("parallel", parsedArgs.Run.Parallel > 0),
			tel.BoolFlag("output-sharing", parsedArgs.Run.EnableSharing),
			tel.BoolFlag("output-mocks", parsedArgs.Run.MockOnFail),
		)
		sf, err := setupSafeguards(parsedArgs, parsedArgs.Run.runSafeguardsCliSpec)
		if err != nil {
			return nil, err
		}
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			parsedArgs.Run.EnableChangeDetection,
			parsedArgs.Run.DisableChangeDetection,
		)
		if err != nil {
			return nil, err
		}
		return &runcmd.Spec{
			Safeguards:      sf,
			GitFilter:       gitfilter,
			Command:         parsedArgs.Run.Command,
			Quiet:           parsedArgs.Quiet,
			DryRun:          parsedArgs.Run.DryRun,
			Reverse:         parsedArgs.Run.Reverse,
			ScriptRun:       false,
			ContinueOnError: parsedArgs.Run.ContinueOnError,
			Parallel:        parsedArgs.Run.Parallel,
			NoRecursive:     parsedArgs.Run.NoRecursive,
			Terragrunt:      parsedArgs.Run.Terragrunt,
			EnableSharing:   parsedArgs.Run.EnableSharing,
			MockOnFail:      parsedArgs.Run.MockOnFail,
			EvalCmd:         parsedArgs.Run.Eval,
			Tags:            parsedArgs.Tags,
			NoTags:          parsedArgs.NoTags,
			DependencyFilters: engine.DependencyFilters{
				IncludeOutputDependencies: parsedArgs.Run.IncludeOutputDependencies,
				OnlyOutputDependencies:    parsedArgs.Run.OnlyOutputDependencies,
				IncludeAllDependencies:    parsedArgs.Run.IncludeAllDependencies,
				IncludeDirectDependencies: parsedArgs.Run.IncludeDirectDependencies,
				OnlyAllDependencies:       parsedArgs.Run.OnlyAllDependencies,
				OnlyDirectDependencies:    parsedArgs.Run.OnlyDirectDependencies,
				ExcludeAllDependencies:    parsedArgs.Run.ExcludeAllDependencies,
				IncludeAllDependents:      parsedArgs.Run.IncludeAllDependents,
				IncludeDirectDependents:   parsedArgs.Run.IncludeDirectDependents,
				OnlyDirectDependents:      parsedArgs.Run.OnlyDirectDependents,
				OnlyAllDependents:         parsedArgs.Run.OnlyAllDependents,
				ExcludeAllDependents:      parsedArgs.Run.ExcludeAllDependents,
			},
		}, nil

	case "experimental eval":
		return nil, errors.E("no expression specified")
	case "experimental eval <expr>":
		return &evalcmd.Spec{
			Exprs:   parsedArgs.Experimental.Eval.Exprs,
			Globals: parsedArgs.Experimental.Eval.Global,
			AsJSON:  parsedArgs.Experimental.Eval.AsJSON,
		}, nil

	case "experimental partial-eval":
		return nil, errors.E("no expression specified")
	case "experimental partial-eval <expr>":
		return &evalcmd.PartialSpec{
			Exprs:   parsedArgs.Experimental.PartialEval.Exprs,
			Globals: parsedArgs.Experimental.PartialEval.Global,
		}, nil

	case "experimental get-config-value":
		return nil, errors.E("no variable specified")
	case "experimental get-config-value <var>":
		return &evalcmd.GetConfigValueSpec{
			Vars:    parsedArgs.Experimental.GetConfigValue.Vars,
			Globals: parsedArgs.Experimental.GetConfigValue.Global,
			AsJSON:  parsedArgs.Experimental.GetConfigValue.AsJSON,
		}, nil

	case "experimental trigger": // Deprecated
		parsedArgs.Trigger = parsedArgs.Experimental.Trigger
		fallthrough
	case "trigger":
		c.SetCommandAnalytics("trigger")
		return nil, errors.E("trigger command expects a stack path")

	case "experimental trigger <stack>": // Deprecated
		parsedArgs.Trigger = parsedArgs.Experimental.Trigger
		fallthrough
	case "trigger <stack>":
		c.SetCommandAnalytics("trigger",
			tel.StringFlag("stack", parsedArgs.Trigger.Stack),
			tel.BoolFlag("change", parsedArgs.Trigger.Change),
			tel.BoolFlag("ignore-change", parsedArgs.Trigger.IgnoreChange),
		)
		return &triggercmd.PathSpec{
			Change:       parsedArgs.Trigger.Change,
			IgnoreChange: parsedArgs.Trigger.IgnoreChange,
			Tags:         parsedArgs.Tags,
			NoTags:       parsedArgs.NoTags,
			Reason:       parsedArgs.Trigger.Reason,
			Path:         parsedArgs.Trigger.Stack,
			Recursive:    parsedArgs.Trigger.Recursive,
		}, nil

	case "script list":
		c.SetCommandAnalytics("script-list")
		return &scriptlistcmd.Spec{}, nil

	case "script tree":
		c.SetCommandAnalytics("script-tree")
		return &scripttreecmd.Spec{}, nil

	case "script info":
		return nil, errors.E("no script specified")

	case "script info <cmds>":
		c.SetCommandAnalytics("script-info")
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			nil, nil,
		)
		if err != nil {
			return nil, err
		}
		return &scriptinfocmd.Spec{
			GitFilter: gitfilter,
			Labels:    parsedArgs.Script.Info.Cmds,
		}, nil

	case "script run":
		return nil, errors.E("no script specified")
	case "script run <cmds>":
		c.SetCommandAnalytics("script-run",
			tel.BoolFlag("filter-changed", parsedArgs.Changed),
			tel.BoolFlag("filter-tags", len(parsedArgs.Tags) != 0),
			tel.BoolFlag("reverse", parsedArgs.Script.Run.Reverse),
			tel.BoolFlag("parallel", parsedArgs.Script.Run.Parallel > 0),
		)
		sf, err := setupSafeguards(parsedArgs, parsedArgs.Script.Run.runSafeguardsCliSpec)
		if err != nil {
			return nil, err
		}
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			parsedArgs.Script.Run.EnableChangeDetection,
			parsedArgs.Script.Run.DisableChangeDetection,
		)
		if err != nil {
			return nil, err
		}
		return &scriptruncmd.Spec{
			Safeguards: sf,
			GitFilter:  gitfilter,

			Quiet:           parsedArgs.Quiet,
			DryRun:          parsedArgs.Script.Run.DryRun,
			Reverse:         parsedArgs.Script.Run.Reverse,
			ContinueOnError: parsedArgs.Script.Run.ContinueOnError,
			Parallel:        parsedArgs.Script.Run.Parallel,
			NoRecursive:     parsedArgs.Script.Run.NoRecursive,
			Tags:            parsedArgs.Tags,
			NoTags:          parsedArgs.NoTags,
			DependencyFilters: engine.DependencyFilters{
				IncludeOutputDependencies: parsedArgs.Script.Run.IncludeOutputDependencies,
				OnlyOutputDependencies:    parsedArgs.Script.Run.OnlyOutputDependencies,
				IncludeAllDependencies:    parsedArgs.Script.Run.IncludeAllDependencies,
				IncludeDirectDependencies: parsedArgs.Script.Run.IncludeDirectDependencies,
				OnlyAllDependencies:       parsedArgs.Script.Run.OnlyAllDependencies,
				OnlyDirectDependencies:    parsedArgs.Script.Run.OnlyDirectDependencies,
				ExcludeAllDependencies:    parsedArgs.Script.Run.ExcludeAllDependencies,
				IncludeAllDependents:      parsedArgs.Script.Run.IncludeAllDependents,
				IncludeDirectDependents:   parsedArgs.Script.Run.IncludeDirectDependents,
				OnlyDirectDependents:      parsedArgs.Script.Run.OnlyDirectDependents,
				OnlyAllDependents:         parsedArgs.Script.Run.OnlyAllDependents,
				ExcludeAllDependents:      parsedArgs.Script.Run.ExcludeAllDependents,
			},
			Labels: parsedArgs.Script.Run.Cmds,
		}, nil

	case "debug show globals":
		c.SetCommandAnalytics("debug-show-globals")
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			nil, nil,
		)
		if err != nil {
			return nil, err
		}

		return &debugglobalscmd.Spec{
			GitFilter: gitfilter,
			Tags:      parsedArgs.Tags,
			NoTags:    parsedArgs.NoTags,
		}, nil

	case "debug show generate-origins":
		c.SetCommandAnalytics("debug-show-generate-origins")
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			nil, nil,
		)
		if err != nil {
			return nil, err
		}

		return &generateoriginscmd.Spec{
			GitFilter: gitfilter,
			Tags:      parsedArgs.Tags,
			NoTags:    parsedArgs.NoTags,
		}, nil

	case "debug show metadata":
		c.SetCommandAnalytics("debug-show-metadata")
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			nil, nil,
		)
		if err != nil {
			return nil, err
		}

		return &debugshowmetadatacmd.Spec{
			GitFilter: gitfilter,
			Tags:      parsedArgs.Tags,
			NoTags:    parsedArgs.NoTags,
		}, nil

	case "debug show runtime-env":
		c.SetCommandAnalytics("debug-show-runtime-env")
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			nil, nil,
		)
		if err != nil {
			return nil, err
		}
		return &debugshowruntimeenv.Spec{
			GitFilter: gitfilter,
			Tags:      parsedArgs.Tags,
			NoTags:    parsedArgs.NoTags,
		}, nil

	case "experimental run-graph":
		c.SetCommandAnalytics("graph")
		return &rungraphcmd.Spec{
			Label:      parsedArgs.Experimental.RunGraph.Label,
			OutputFile: parsedArgs.Experimental.RunGraph.Outfile,
		}, nil
	default:
		return nil, errors.E("unexpected command sequence")
	}
}

func envVarIsSet(val string) bool {
	return val != "" && val != "0" && val != "false"
}

func setupSafeguards(parsedArgs *FlagSpec, runflags runSafeguardsCliSpec) (sf runcmd.Safeguards, err error) {
	global := parsedArgs.deprecatedGlobalSafeguardsCliSpec

	// handle deprecated flags as --disable-safeguards
	if global.DeprecatedDisableCheckGitUncommitted {
		runflags.DisableSafeguards = append(runflags.DisableSafeguards, "git-uncommitted")
	}
	if global.DeprecatedDisableCheckGitUntracked {
		runflags.DisableSafeguards = append(runflags.DisableSafeguards, "git-untracked")
	}
	if runflags.DeprecatedDisableCheckGitRemote {
		runflags.DisableSafeguards = append(runflags.DisableSafeguards, "git-out-of-sync")
	}
	if runflags.DeprecatedDisableCheckGenCode {
		runflags.DisableSafeguards = append(runflags.DisableSafeguards, "outdated-code")
	}
	if runflags.DisableSafeguardsAll {
		runflags.DisableSafeguards = append(runflags.DisableSafeguards, "all")
	}

	if runflags.DisableSafeguards.Has(safeguard.All) && runflags.DisableSafeguards.Has(safeguard.None) {
		return runcmd.Safeguards{}, errors.E(
			errors.E(clitest.ErrSafeguardKeywordValidation,
				`the safeguards keywords "all" and "none" are incompatible`),
			"Disabling safeguards",
		)
	}

	sf.DisableCheckGitUncommitted = runflags.DisableSafeguards.Has(safeguard.GitUncommitted, safeguard.All, safeguard.Git)
	sf.DisableCheckGitUntracked = runflags.DisableSafeguards.Has(safeguard.GitUntracked, safeguard.All, safeguard.Git)
	sf.DisableCheckGitRemote = runflags.DisableSafeguards.Has(safeguard.GitOutOfSync, safeguard.All, safeguard.Git)
	sf.DisableCheckGenerateOutdatedCheck = runflags.DisableSafeguards.Has(safeguard.Outdated, safeguard.All)
	if runflags.DisableSafeguards.Has("none") {
		sf = runcmd.Safeguards{}
		sf.ReEnabled = true
	}
	return sf, nil
}
