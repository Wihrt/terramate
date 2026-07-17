# Phase 0 — Documentation + hygiène : Plan d'implémentation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mettre la documentation (README.md, CLAUDE.md, AGENTS.md) en accord avec l'état réel du fork, créer `docs/BACKLOG.md`, et supprimer le code mort résiduel du retrait de Terramate Cloud.

**Architecture:** Aucun changement structurel — suppression de code mort (package `http/` orphelin, fichiers résiduels), deux micro-corrections de code (`Spec.Name()`, un commentaire), et réécriture/correction de 3 documents + création d'un backlog. Spec : `docs/superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md` (Phase 0).

**Tech Stack:** Go 1.25.8, make + mise (task runners), Markdown.

## Global Constraints

- Refactoring iso-comportement : aucune fonctionnalité ajoutée ni modifiée (seule exception actée par le spec : `Spec.Name()` retourne `"ui"` au lieu de `"prompt"`).
- Tout fichier Go doit garder son en-tête copyright : `// Copyright 2025 Terramate GmbH` + `// SPDX-License-Identifier: MPL-2.0` (ne pas modifier les en-têtes existants).
- Messages de commit : conventional commits (`docs:`, `chore:`, `fix:`), en anglais, comme l'historique du repo.
- La branche de travail est `refactor/maintenabilite` (déjà créée).
- Version Go de référence : **1.25.8** (`go.mod:3`). Task runner : **mise** (`mise.toml`), make en équivalent.
- Fin de phase : `make build`, `make test` et `make lint/all` verts.

---

### Task 1: Supprimer le code mort résiduel du Cloud

**Files:**
- Delete: `http/` (répertoire complet : `http.go`, `stack.tm.hcl`, `_test_mock.tf`)
- Delete: `.tmtriggers/cloud/` (répertoire complet, contient `changed-a9fe0083-ed01-4ee7-b407-b367413071d1.tm.hcl`)
- Delete: `.goreleaser.yaml.bak`

**Interfaces:**
- Consumes: rien.
- Produces: rien — le package `http/` a 0 importeur dans tout le repo (vérifié : `grep -rl "terramate/http" --include='*.go' .` ne retourne rien). Son `stack.tm.hcl` référence même encore le package supprimé `resources` dans sa description.

- [ ] **Step 1: Vérifier que le package http/ est bien orphelin**

Run: `grep -rln 'terramate-io/terramate/http"' --include='*.go' . ; echo "exit=$?"`
Expected: aucun fichier listé, `exit=1` (grep ne trouve rien).

- [ ] **Step 2: Supprimer les fichiers morts**

```bash
git rm -r http/ .tmtriggers/cloud/
git rm .goreleaser.yaml.bak
```

- [ ] **Step 3: Vérifier que le build passe**

Run: `make build`
Expected: succès, binaires `bin/terramate` et `bin/terramate-ls` produits.

- [ ] **Step 4: Vérifier que la génération Terramate est cohérente**

Le repo utilise Terramate pour sa propre génération ; la suppression du stack `http/` ne doit rien laisser en attente :

Run: `./bin/terramate generate`
Expected: aucun fichier généré/modifié (sortie vide ou "Nothing to do"). Si des fichiers sont modifiés par la commande, les examiner : seuls des retraits liés à `http/` sont acceptables — les ajouter au commit.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "chore: remove dead http package and cloud leftovers"
```

---

### Task 2: Micro-corrections de code (Spec.Name, résidus TMC)

**Files:**
- Modify: `commands/ui/ui.go:37`
- Modify: `ui/tui/telemetry/telemetry.go:51`
- Modify: `e2etests/internal/runner/runner.go:87-99`

**Interfaces:**
- Consumes: interface `commands.Executor` (`commands/commands.go:18`, méthode `Name() string`).
- Produces: `Spec.Name()` de la TUI retourne `"ui"`. Aucun call site de `Name()` en production ne dépend de l'ancienne valeur (vérifié : aucun appel `.Name()` sur les executors dans `ui/`, `engine/`, `commands/`).

- [ ] **Step 1: Corriger Spec.Name()**

Dans `commands/ui/ui.go:37`, remplacer :

```go
func (s *Spec) Name() string { return "prompt" }
```

par :

```go
func (s *Spec) Name() string { return "ui" }
```

- [ ] **Step 2: Corriger le commentaire résiduel TMC**

Dans `ui/tui/telemetry/telemetry.go:51`, remplacer :

```go
	// AuthUser is the TMC user UUID.
```

par :

```go
	// AuthUser is the authenticated user UUID.
```

- [ ] **Step 3: Retirer le scrubbing d'environnement TMC mort du runner e2e**

Dans `e2etests/internal/runner/runner.go`, fonction `NewCLI` (lignes 87-99), supprimer les deux blocs devenus morts depuis le retrait du Cloud :

```go
	// environments below are never used in automation.
	env = RemoveEnv(env, "TMC_API_HOST", "TMC_API_IDP_KEY")
```

et :

```go
	// sanity check for cases where user has this configured in their environment.
	if index := slices.Index(env, "TMC_API_URL"); index >= 0 {
		apiURL := env[index]
		if !strings.HasPrefix(apiURL, "http://") {
			panic("tests are picking up the wrong API URL")
		}
	}
```

Conserver les lignes voisines (`RemoveEnv(env, "ACTIONS_ID_TOKEN_...")`, `CHECKPOINT_DISABLE=1`). Si l'import `slices` devient inutilisé après suppression, le retirer (le build le signalera).

- [ ] **Step 4: Vérifier build et tests des packages touchés**

Run: `go build ./commands/ui/... ./ui/tui/... ./e2etests/internal/... && go vet ./e2etests/internal/runner/ && go test -race -count=1 ./commands/ui/... ./ui/tui/...`
Expected: build OK, vet OK, tests PASS.

- [ ] **Step 5: Commit**

```bash
git add commands/ui/ui.go ui/tui/telemetry/telemetry.go e2etests/internal/runner/runner.go
git commit -m "fix: align ui command Name() with CLI name and drop TMC leftovers"
```

---

### Task 3: Réécrire README.md pour le fork

**Files:**
- Modify: `README.md` (remplacement complet)

**Interfaces:**
- Consumes: rien.
- Produces: rien (documentation).

- [ ] **Step 1: Remplacer intégralement le contenu de README.md par :**

````markdown
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
````

- [ ] **Step 2: Vérifier que les commandes documentées existent**

Run: `make -n build test fmt lint/all >/dev/null && echo OK && ./bin/terramate --help 2>&1 | grep -E '^\s+(ui|scaffold)' | head -5`
Expected: `OK`, puis les lignes d'aide mentionnant les commandes `ui` et `scaffold`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs: rewrite README for the fork (build from source, TUI, bundles)"
```

---

### Task 4: Mettre à niveau CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: rien.
- Produces: rien (documentation).

- [ ] **Step 1: Appliquer les modifications suivantes à CLAUDE.md**

**(a)** Remplacer la section "Project Overview" (le paragraphe sous `## Project Overview`, avant `**License**`) par :

````markdown
**Terramate** is an orchestration, code generation, and change management tool for IaC (Terraform, OpenTofu, Terragrunt). It consists of:
- A CLI (`terramate`) — stack orchestration, change detection, code generation
- A Language Server (`terramate-ls`) — LSP support for HCL editing

**This repo is a fork** of `terramate-io/terramate` (upstream remote configured as `upstream`). Fork specifics:
- **Terramate Cloud has been removed** entirely (no `cloud/` package, no SaaS sync).
- **Interactive TUI added**: `terramate ui` (Bubble Tea app in `commands/ui/`).
- **Bundles/components/packages**: reusable infrastructure definitions with typed
  inputs; stack metadata is generated from bundle definitions.
- Upstream merges are not expected; the fork owns its divergence.
````

**(b)** Dans la section "Commands", ajouter après le bloc Build :

````markdown
> `mise` is the task runner pinning tool versions (`mise.toml`, Go 1.25.8). Every make
> target has a mise equivalent: `mise run build`, `mise run test`, etc. (`mise tasks`
> to list them).
````

**(c)** Remplacer la liste de la section "### Core Packages" par :

````markdown
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
````

**(d)** À la fin du fichier, ajouter :

````markdown
## Fork Maintenance

- Refactoring specs and plans live in `docs/superpowers/specs/` and `docs/superpowers/plans/`.
- Technical debt backlog: `docs/BACKLOG.md`.
````

- [ ] **Step 2: Vérifier les affirmations du document**

Run: `make -n build build/terramate build/terramate-ls test fmt lint/all lint/install generate >/dev/null && echo MAKE-OK && ls commands/ui/model.go ui/tui/cli_spec.go ls/ls.go typeschema/ scaffold/ >/dev/null && echo PATHS-OK`
Expected: `MAKE-OK` puis `PATHS-OK`.

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: update CLAUDE.md with fork architecture (TUI, bundles, ls, mise)"
```

---

### Task 5: Corriger AGENTS.md

**Files:**
- Modify: `AGENTS.md`

**Interfaces:**
- Consumes: rien.
- Produces: rien (documentation).

- [ ] **Step 1: Appliquer les corrections factuelles**

**(a)** Remplacer la liste "Repository Structure" (lignes 11-18) par :

````markdown
**Repository Structure**:
- `/cmd/` - Main binaries (terramate CLI, terramate-ls language server, tgdeps)
- `/ls/` - Language Server Protocol implementation
- `/hcl/` - HCL parsing and evaluation
- `/config/` - Configuration management (incl. bundles)
- `/typeschema/` - Type system for bundle/component inputs
- `/engine/` - Orchestration engine
- `/commands/` - CLI command implementations (incl. `commands/ui/`, the interactive TUI)
- `/ui/tui/` - CLI bootstrap (flag parsing, command dispatch)
- `/stack/` - Stack lifecycle
- `/generate/` - Code generation
- `/e2etests/` - End-to-end tests
- `/test/` - Test utilities

**Fork note**: this repo is a fork of `terramate-io/terramate` with Terramate Cloud
removed, an interactive TUI (`terramate ui`) and bundles/components/packages added.
````

**(b)** Remplacer `**Language**: Go 1.24+` par :

````markdown
**Language**: Go 1.25.8 (pinned in `go.mod` and `mise.toml`)
````

**(c)** Remplacer le bloc "Prerequisites" (lignes 29-36) par :

````markdown
```bash
# Install all dependencies using mise (https://mise.jdx.dev/)
mise install

# Check versions
go version  # Should be 1.25.x
make --version
```
````

**(d)** Remplacer le bloc "Build" (lignes 40-48) par :

````markdown
```bash
# Build all binaries
make build          # or: mise run build

# Output:
# - bin/terramate (CLI)
# - bin/terramate-ls (Language Server)

# Test helper binary (built separately, needed by e2e tests):
make test/helper    # -> bin/helper
```
````

- [ ] **Step 2: Vérifier les affirmations corrigées**

Run: `grep -q 'go 1.25.8' go.mod && test -f mise.toml && ! test -f .tool-versions && make -n test/helper >/dev/null && echo OK`
Expected: `OK`.

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: fix AGENTS.md tooling facts (mise, Go 1.25.8, build outputs)"
```

---

### Task 6: Créer docs/BACKLOG.md

**Files:**
- Create: `docs/BACKLOG.md`

**Interfaces:**
- Consumes: Annexe A du spec `docs/superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md`.
- Produces: rien (documentation).

- [ ] **Step 1: Créer docs/BACKLOG.md avec ce contenu :**

````markdown
# Backlog — dette technique

État des lieux réalisé le 2026-07-17 (4 audits : TUI, code bundle, structure globale,
documentation). Deux chantiers de refactorisation sont spécifiés :

- **Chantier 1 — code fork** : [spec](superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md)
  (docs, hygiène, factorisation TUI, Clean Architecture TUI).
- **Chantier 2 — code upstream** : [spec](superpowers/specs/2026-07-17-refactoring-upstream-design.md)
  (registre CLI, unification des parsers, décomposition `hcl/hcl.go`, découplage
  engine/generate). Exécuté après le chantier 1.

## Items (par priorité)

| Prio | Dette | Localisation | Effort | Couvert par |
|---|---|---|---|---|
| P1 | Duplication des 3 vues TUI (~500-700 lignes), god object `Model`, logique métier dans les vues, ~7k lignes sans test | `commands/ui/` | L | Chantier 1, phases 1-3 |
| P1 | Dispatch CLI géant : `SelectCommand`, 438 lignes, 36 `case` | `ui/tui/cli_handler.go:75` | M | Chantier 2, U1 |
| P1 | God file parser : 2 210 lignes (parsing, merge, git config, imports, glob) | `hcl/hcl.go` | L | Chantier 2, U3 |
| P2 | 3 signatures `Parse` différentes sur 16 parsers `block_*_parser.go`, 4 types de handlers | `hcl/block_*.go` | M | Chantier 2, U2 |
| P2 | `di/` utilisé en service locator transversal (dépendances masquées) | `engine`, `generate`, `ui/tui`, `commands` | M/L | Chantier 2, U4 |
| P2 | Couplage bidirectionnel `engine` ↔ `generate` | `engine/bundles.go`, `generate/generate.go` | M | Chantier 2, U4 |
| P2 | Fonctions longues : `ApplyDependencyFilters` (192 l.), `generateRootFiles` (136 l.), `stackGenerate` (127 l.), `loadStackCodeCfgs` (110 l.) | `engine/engine.go:353`, `generate/generate.go` | M | Chantier 2, U4 |
| P2 | Parser `define` : `Parse` 266 l. + helpers 100-163 l. | `hcl/block_define_parser.go:224`, `config/bundle.go` | M | Chantier 2, U1 |
| P2 | Couverture insuffisante de `BufferGroup` (1 seul test, code sur le chemin de tous les runs parallèles) | `engine/buffer_group_test.go` | S | Chantier 1, phase 1 |
| P3 | Tests e2e avec `t.Skip` à trier | `e2etests/core/{exp_trigger,general,list_git}_test.go` | S | Chantier 2, U5 |
| P3 | Await keys « plates » émises en plus des clés scopées par classe | `config/hcl_func.go:60-72` | S | Chantier 2, U5 |

## Hors backlog (déjà traité)

- Résidus du retrait Cloud (package `http/` orphelin, `.tmtriggers/cloud/`,
  `.goreleaser.yaml.bak`, commentaire TMC, `Spec.Name()`) — traités en Phase 0 du
  chantier 1.
````

- [ ] **Step 2: Vérifier les liens relatifs**

Run: `test -f docs/superpowers/specs/2026-07-17-refactoring-maintenabilite-design.md && test -f docs/superpowers/specs/2026-07-17-refactoring-upstream-design.md && echo OK`
Expected: `OK`.

- [ ] **Step 3: Commit**

```bash
git add docs/BACKLOG.md
git commit -m "docs: add technical debt backlog"
```

---

### Task 7: Vérification de fin de phase

**Files:** aucun (vérification seule).

**Interfaces:**
- Consumes: tout le travail des tasks 1-6.
- Produces: Phase 0 livrée — build, tests, lint verts.

- [ ] **Step 1: Build complet**

Run: `make build`
Expected: succès.

- [ ] **Step 2: Suite de tests complète**

Run: `make test`
Expected: PASS (la suite utilise `bin/terramate` construit à l'étape 1).

- [ ] **Step 3: Lint**

Run: `make lint/all`
Expected: aucun problème signalé.

- [ ] **Step 4: Contrôle final des résidus cloud**

Run: `grep -rn "TMC_\|Terramate Cloud" --include='*.go' . | grep -v _test.go | grep -v testdata ; echo "exit=$?"`
Expected: aucune ligne de code de production, `exit=1`.
