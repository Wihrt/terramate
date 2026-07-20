# Terramate (fork)

> This is a fork of [terramate-io/terramate](https://github.com/terramate-io/terramate),
> maintained at [Wihrt/terramate](https://github.com/Wihrt/terramate).
>
> **Differences from upstream:**
>
> - **Terramate Cloud support has been removed** — this fork is fully standalone,
>   no SaaS integration, no telemetry endpoint.
> - **Interactive TUI added** — `terramate ui` provides an interactive terminal UI
>   to scaffold, reconfigure and promote infrastructure bundles.
> - **Bundles, components and packages** — define reusable infrastructure bundles
>   with typed inputs, generate full stack metadata from bundle definitions
>   (`scaffold`, `component create`, `package create` commands).

## What is Terramate?

Terramate CLI is an open-source orchestration and code generation engine that allows
Infrastructure as Code (IaC) such as Terraform, OpenTofu and Terragrunt to scale:

1. Break up large, monolithic state files into multiple smaller stacks to limit blast
   radius, reduce runtimes and unlock better collaboration.
2. Reduce code duplication by programmatically generating native Terraform backend and
   provider configurations or any other arbitrary files using the Terramate compiler.
3. Using a graph-based orchestration engine, orchestrate any command such as
   `terraform apply` in stacks. Only deploy stacks that contain changes using change
   detection built on top of Git.

The [upstream documentation](https://terramate.io/docs/cli) remains valid for the core
CLI features (stacks, orchestration, change detection, code generation, globals).
Cloud-related sections of the upstream documentation do not apply to this fork.

## Installation

This fork is not published to package managers (`brew install terramate` and
`go install github.com/terramate-io/...` install the **upstream** project, not this fork).
Build from source:

```sh
git clone https://github.com/Wihrt/terramate.git
cd terramate
make build          # or: mise run build
```

Prerequisites: Go 1.25.x and `make` (or [mise](https://mise.jdx.dev/), which pins all
tool versions from `mise.toml`).

Produced binaries:

- `bin/terramate` — the CLI
- `bin/terramate-ls` — the language server

## Features

**Core (inherited from upstream):**

- **Stacks & Orchestration:** Run any command and configurable workflows in stacks with
  unlimited concurrency.
- **Change Detection:** Only execute stacks that contain changes, built on top of Git.
  Detects changes in referenced Terraform/OpenTofu modules and Terragrunt dependencies.
- **Code Generation:** Generate HCL, JSON and YAML to keep your stacks DRY.
- **Globals & Hierarchical Configuration:** Variables merged hierarchically across
  directories.

**Fork additions:**

- **Interactive TUI:** `terramate ui` — browse bundles, scaffold new stacks from bundle
  definitions, reconfigure existing ones, and promote bundles across environments, all
  from an interactive terminal interface.
- **Bundles:** Define reusable infrastructure bundles with typed inputs
  (`typeschema`); stack metadata (name, description, tags, dependencies) is generated
  from bundle definitions.
- **Scaffolding commands:** `terramate scaffold`, `terramate component create`,
  `terramate package create`.

## Development

```sh
make build        # build bin/terramate and bin/terramate-ls
make test         # full test suite (requires prior make build)
make fmt          # format code (goimports)
make lint/all     # golangci-lint
mise tasks        # list all mise tasks (build, install, test, ...)
```

See [CLAUDE.md](./CLAUDE.md) and [AGENTS.md](./AGENTS.md) for detailed guidance aimed
at AI coding agents (architecture, code patterns, testing conventions).

## License

MPL-2.0 — see the [LICENSE](./LICENSE) file. This fork retains the upstream license
and copyright headers (`Copyright Terramate GmbH`).
