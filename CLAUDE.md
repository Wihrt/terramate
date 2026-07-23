# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Terramate** is an orchestration, code generation, and change management tool for IaC (Terraform, OpenTofu, Terragrunt). It consists of:
- A CLI (`terramate`) — stack orchestration, change detection, code generation
- A Language Server (`terramate-ls`) — LSP support for HCL editing

**This repo is a fork** of `terramate-io/terramate` (upstream remote configured as `upstream`). Fork specifics:
- **Terramate Cloud has been removed** entirely (no `cloud/` package, no SaaS sync).
- **Interactive TUI added**: `terramate ui` (Bubble Tea app in `commands/ui/`).
- **Bundles/components/packages**: reusable infrastructure definitions with typed
  inputs; stack metadata is generated from bundle definitions.
- Upstream merges are not expected; the fork owns its divergence.

**License**: MPL-2.0. All source files must have a copyright header:
```go
// Copyright 2025 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0
```

## Commands

### Build
```bash
make build              # Build all binaries (bin/terramate, bin/terramate-ls)
make build/terramate    # Build CLI only
make build/terramate-ls # Build language server only
```

> `mise` is the task runner pinning tool versions (`mise.toml`, Go 1.25.8). Common make
> targets (build, test, lint, generate, fmt) have mise equivalents: `mise run build`, `mise run test`, etc. (`mise tasks`
> to list all available tasks).

### Test
```bash
make test                               # Full test suite (requires prior build)
go test -race -count=1 ./hcl/...       # Specific package
go test -race ./stack/... -run TestName # Single test
go test -race ./hcl/eval -fuzz=FuzzPartialEval # Fuzz testing
```

> `make test` uses the built `bin/terramate` binary itself to orchestrate test runs across stacks. Run `make build` first.

### Lint & Format
```bash
make fmt        # Format using goimports (not just gofmt)
make lint/all   # Run golangci-lint
make lint/install # Install golangci-lint
```

### Code Generation
```bash
./bin/terramate generate  # Regenerate files from .tm templates
make generate             # Same via make
```

The repo uses Terramate itself for internal code generation. If generated files are outdated, `make test` will fail.

## Architecture

### Entry Points (`cmd/`)
- `cmd/terramate/` — CLI main
- `cmd/terramate-ls/` — Language server main
- `cmd/tgdeps/` — Terragrunt dependency tool

### Core Packages
- **`engine/`** — Central orchestration: stack selection, change detection, run orchestration. `engine.go` is the main entry point; `run.go` handles execution, `dependencies.go` handles stack ordering, `bundles.go` handles bundle resolution.
- **`hcl/`** — HCL parsing and evaluation using a forked `github.com/terramate-io/hcl/v2`. Defines all Terramate-specific HCL block parsers (`block_*.go`), including the fork's `block_define_parser.go` / `block_bundle_parser.go` (bundles). `hcl.go` is the primary parser.
- **`config/`** — Loads and represents the parsed Terramate configuration: stacks, scripts, components, globals, sharing backends, bundles (`bundle.go`).
- **`typeschema/`** — Type system for bundle/component inputs (parsing, validation).
- **`globals/`** — Global variable evaluation. Implements Terramate's hierarchical variable merging across directories.
- **`generate/`** — Code generation: `genfile/` (arbitrary files), `genhcl/` (HCL output), `sharing/` (output sharing between stacks), `generate_bundle.go` (stack metadata generated from bundle definitions).
- **`stack/`** — Stack lifecycle: create, clone, manager (stack discovery and ordering), `update.go` (metadata sync from bundles).
- **`commands/`** — CLI command implementations, grouped by feature (`run/`, `stack/`, `generate/`, `fmt/`, `ui/`, `component/`, `package/`, `clone/`, `trigger/`, `script/`, `debug/`, …).
- **`commands/ui/`** — Interactive TUI (`terramate ui`), a Bubble Tea app: views (`view_*.go`), form widgets (`widget_*.go`), business logic (`change.go`), central model (`model.go`).
- **`ui/tui/`** — CLI bootstrap: kong flag parsing (`cli_spec.go`), command dispatch (`cli_handler.go`), telemetry.
- **`ls/`** — Language server implementation: definitions, references, rename, imports resolution (see AGENTS.md for details).
- **`scaffold/`** — Scaffolding of bundles/components from manifests.
- **`git/`** — Git integration for change detection.
- **`scheduler/`** — Parallel execution scheduler for stack orchestration.

### Test Infrastructure (`test/`)
- **`test/sandbox/`** — File-system sandbox for isolated tests. Always prefer this over `os.TempDir()`.
- **`test/hclutils/`** — HCL parsing test helpers.
- **`test/hclwrite/`** — HCL write helpers.
- **`e2etests/`** — End-to-end tests that run the actual binary.

## Code Patterns

### Error Handling
```go
// Wrap with context
return nil, errors.E(err, "description of what failed")

// Collect multiple errors
errs := errors.L()
errs.Append(err1)
errs.Append(err2)
return errs.AsError()
```

### Logging (zerolog)
```go
log.Debug().Str("key", value).Msg("description")
log.Info().Int("count", n).Msg("operation complete")
log.Error().Err(err).Msg("operation failed")
```

### Testing
```go
s := sandbox.New(t)
s.BuildTree([]string{
    `f:globals.tm:globals { var = "value" }`,
    `s:stacks/prod`,  // Create a stack
})
```

## Terramate-Specific Concepts

### Variable Namespaces
- `global.*` — Globals (defined in `globals {}` blocks, hierarchically merged)
- `let.*` — Scoped to `generate_hcl`/`generate_file` blocks
- `terramate.stack.*` — Built-in stack metadata (`name`, `id`, `path`, etc.)
- `env.*` — Environment variables via `terramate.config.run.env`
- **`stack.*` is NOT valid** — use `terramate.stack.*`

### Stack Configuration (`.tm.hcl` files)
```hcl
stack {
  id          = "uuid-here"
  name        = "my-stack"
  description = "..."
  tags        = ["tag1", "tag2"]
}
```

### Import Resolution
Globals can be defined in imported files. Imports are resolved recursively and must not escape the workspace root. Absolute paths start with `/` (workspace-relative); relative paths are directory-relative.

### HCL Coordinate System
HCL uses **1-indexed** positions; LSP protocol uses **0-indexed**. Convert when bridging the two:
```go
lspLine = hclLine - 1
lspChar = hclChar - 1
```

## Language Server Development

Key files in the LS implementation handle: definition lookup, references, rename, label rename, import resolution. Debug with:
```bash
./bin/terramate-ls --log-level debug --log-fmt console 2> ls-debug.log
```

## Fork Maintenance

- Refactoring specs and plans live in `docs/superpowers/specs/` and `docs/superpowers/plans/`.
- Technical debt backlog: `docs/BACKLOG.md`.
