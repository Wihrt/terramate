# mise.toml + pre-commit — Design Spec

**Date:** 2026-07-10
**Status:** Approved

## Problem

Development tooling and task running lives entirely in a hand-rolled Makefile system
(`Makefile` + `makefiles/{unix,windows,common,_mkconfig}.mk`) with OS-specific dispatch and no
built-in tool-version management — tool versions (golangci-lint, addlicense) are pinned as
Makefile variables and installed ad hoc via `go install`/`go run` on every invocation. There is
also no fast local feedback loop before commit: the only local safety net is `make test`, which
runs the full `go test -race` suite and is too slow to run on every commit.

## Goal

Add `mise.toml` as a tool-version-pinned task runner for local development onboarding, and a
`.pre-commit-config.yaml` giving a fast (seconds, not minutes) feedback loop at commit time
(fmt, vet, build, lint) — without removing the existing Makefile system.

## Non-Goals

- **Do not delete `Makefile` / `makefiles/`.** They remain the source of truth for
  `ci-experimental.yml`, `ci-sync-preview.yml`, `ci-sync-deployment.yml`, `benchmark.yml`,
  `interop-tests.yml`, `release.yml`, `.gitlab-ci.yml`, and `containers/test/Dockerfile` — none
  of which are modified by this work. (This reverses an earlier direction explored during
  brainstorming — full Makefile deletion + rewriting all 8 CI consumers — which the user
  explicitly walked back once every consumer except `oss-build-test.yml` turned out to be
  either legacy/disabled or intentionally left alone.)
- No `go test` in the pre-commit hooks — the full suite stays a CI/manual concern via
  `make test` / `mise run test`.
- No file-based `mise-tasks/` scripts — all tasks are inline `[tasks]` entries in `mise.toml`
  (explicit user choice, against the assistant's recommended file-based default).
- No reconciliation of the pre-existing `.gitlab-ci.yml` Go-version mismatch (`golang:1.21`
  image vs. `go 1.25.8` in `go.mod`) — flagged during brainstorming, left untouched.
- No Windows support in `mise.toml` tasks. The new OSS CI pipeline (`oss-*.yml`) already only
  targets `ubuntu-latest`; `mise.toml` tasks mirror `makefiles/unix.mk`/`common.mk` behavior
  only. Windows-local developers keep using `make` (`makefiles/windows.mk`), unchanged.

## Architecture

### Coexistence model

`mise.toml` and the Makefile system become two parallel, independently usable entry points to
the same underlying `go build`/`go test`/etc. commands:

- **`mise.toml`** — for local dev onboarding (`mise install` pins every tool version) and for
  the one CI file that adopts it, `oss-build-test.yml`.
- **`Makefile`** — unchanged, still authoritative for every other CI consumer and for Windows
  developers.

Each `mise.toml` task mirrors an existing Unix Makefile target 1:1 (same build flags, same
output paths), so a developer can use either and get identical binaries. The two systems are
not kept in lockstep automatically — if a Makefile target changes or a new one is added later,
the matching `mise.toml` task must be updated by hand.

### `[tools]`

| Tool | Version | Backend | Matches |
|---|---|---|---|
| `go` | `1.25.8` | core (default) | `go.mod` |
| `golangci-lint` | `2.6.1` | aqua (default registry mapping) | `makefiles/_mkconfig.mk` |
| `goreleaser` | `2.17.0` | aqua (default registry mapping) | latest stable; used for local `.goreleaser.yml` testing |
| `node` | `22` | core (default) | needed for `pin-gha` (`npm exec pin-github-action`) |
| `pre-commit` | `4.0.1` | aqua/pipx (default registry mapping) | hook runner itself |
| `"go:github.com/google/addlicense"` | `1.0.0` | go (explicit backend, no registry shortcut) | `makefiles/_mkconfig.mk`'s `ADDLICENSE_VERSION` |

Not tool-managed (documented as a manual prerequisite, no mise backend exists):
- `graphviz` (`dot` binary) — only needed for the optional `graph2png` task.

Exact aqua version-string formatting (e.g. whether a `v` prefix is required) will be confirmed
against the live aqua registry during implementation; the semver values above are the intended
pins carried over from the Makefile.

### `[tasks]` — mapping to existing Makefile targets

One inline `[tasks."name"]` block per row, using `:` as the namespace separator:

| Makefile target | mise task | Notes |
|---|---|---|
| `build` | `build` | depends on `build:terramate`, `build:terramate-ls` |
| `build/terramate` | `build:terramate` | `CGO_ENABLED=0 go build --ldflags '-extldflags "-static"' -o bin/terramate ./cmd/terramate` |
| `build/terramate-ls` | `build:terramate-ls` | same static-build flags, `./cmd/terramate-ls` |
| `build/pprof` | `build:pprof` | depends on `build:pprof:terramate` |
| `build/pprof/terramate` | `build:pprof:terramate` | adds `-tags profiler` |
| `build/tgdeps` | `build:tgdeps` | `./cmd/tgdeps` |
| `install` | `install` | depends on `install:terramate`, `install:terramate-ls` |
| `install/terramate` | `install:terramate` | `go install` variant |
| `install/pprof`, `install/pprof/terramate` | `install:pprof`, `install:pprof:terramate` | profiler variants |
| `install/terramate-ls` | `install:terramate-ls` | |
| `install/tgdeps` | `install:tgdeps` | |
| `generate` | `generate` | `./bin/terramate generate` |
| `fmt` | `fmt` | `go run golang.org/x/tools/cmd/goimports@v0.1.7 -w .` |
| `lint/install` | `lint:install` | `go install .../golangci-lint@$VERSION` — largely superseded by the pinned `[tools]` entry, kept for parity |
| `lint/all` | `lint:all` | `golangci-lint run ./...` (full repo, not the fast pre-commit subset) |
| `mod` | `mod` | `go mod tidy` |
| `mod/check` | `mod:check` | `./hack/mod-check`, reused unchanged |
| `license` | `license` | `addlicense -c "Terramate GmbH" .` |
| `license/check` | `license:check` | `addlicense --check .` |
| `pin-gha` | `pin-gha` | `npm exec --yes pin-github-action@3.5.1 -- ./.github/workflows/*.yml` |
| `coverage`, `coverage/show` | `coverage`, `coverage:show` | unchanged |
| `test/docker` | `test:docker` | `docker build ... -f containers/test/Dockerfile .` |
| `test/fuzz/eval`, `test/fuzz/fmt`, `test/fuzz/tokens-for-expr` | `test:fuzz:eval`, `test:fuzz:fmt`, `test:fuzz:tokens-for-expr` | unchanged |
| `bench`, `bench/all` | `bench`, `bench:all` | unchanged, default args preserved |
| `bench/check` | `bench:check` | **argument style changes**: Make's `pkg=X parallel=Y new=Z` becomes env vars — `PKG=X PARALLEL=Y NEW=Z mise run bench:check` — since inline mise tasks don't support Make-style key=value overrides |
| `bench/cleanup` | `bench:cleanup` | unchanged |
| `release/dry-run` | `release:dry-run` | `goreleaser release --snapshot --rm-dist` — kept byte-for-byte, including the deprecated `--rm-dist` flag, to avoid scope creep |
| `release` | `release` | `goreleaser release --rm-dist --key $GORELEASER_KEY` — same, unchanged |
| `cloud/sync/ok`, `cloud/sync/failed` | `cloud:sync:ok`, `cloud:sync:failed` | unchanged |
| `test/build` | `test:build` | depends on `test:testserver` (Unix only, per Non-Goals) |
| `test/testserver` | `test:testserver` | `go build -o bin/testserver ./cloud/testserver/cmd/testserver` |
| `test/helper` | `test:helper` | `go build -o bin/helper ./e2etests/cmd/helper` |
| `test` | `test` | tempdir via `./bin/helper tempdir`, `TM_TEST_ROOT_TEMPDIR=... ./bin/terramate run --no-recursive -- go test -race -count=1 -timeout 30m ./...` — mirrors `makefiles/unix.mk` exactly |
| `test/sync` | `test:sync` | unchanged (cloud staging sync test, not used by CI) |
| `test/interop` | `test:interop` | unchanged |
| `graph2png` | `graph2png` | requires system `dot` (graphviz), documented as prerequisite |
| `release/tag` | `release:tag` | `git tag -s -a $VERSION -m "Release $VERSION" && git push origin $VERSION` |
| `clean` | `clean` | `rm -rf bin/*` |
| `help` | — | dropped; replaced by mise's built-in `mise tasks` listing |

### `.pre-commit-config.yaml`

All hooks are **local** (`language: system`), invoking `mise exec --` / `mise run` rather than
pre-commit's own hook repos, so golangci-lint/go stay on the exact versions pinned in
`mise.toml` — no third version source to drift:

```yaml
repos:
  - repo: local
    hooks:
      - id: go-fmt
        name: goimports
        entry: mise run fmt
        language: system
        pass_filenames: false
        types: [go]

      - id: go-vet
        name: go vet
        entry: mise exec -- go vet ./...
        language: system
        pass_filenames: false
        types: [go]

      - id: go-build
        name: go build
        entry: mise run build
        language: system
        pass_filenames: false
        types: [go]

      - id: golangci-lint
        name: golangci-lint
        entry: mise exec -- golangci-lint run --new-from-rev=HEAD
        language: system
        pass_filenames: false
        types: [go]
```

- `types: [go]` — hooks only fire when staged files include `.go`, so a docs-only commit pays
  no cost.
- `golangci-lint run --new-from-rev=HEAD` scopes lint to the uncommitted diff, not the whole
  repo, keeping it fast. Full-repo lint stays `make lint/all` / `mise run lint:all`, run in CI.
- No `go test` hook, per explicit user decision.
- `pre-commit` itself is one of the `[tools]` entries, so `mise install` + `pre-commit install`
  is the entire bootstrap — no separate install step outside mise.

### CI change

Only `oss-build-test.yml` changes:

- Replace `actions/setup-go@... # pin@v5` with `jdx/mise-action@<pinned-sha> # pin@vX`
  (`install: true`) — installs Go + every other `[tools]` entry in one step, guaranteeing the
  same tool versions in CI as in local dev.
- `make build` → `mise run build`
- `make generate && git diff --exit-code` → `mise run generate && git diff --exit-code`
- `make test` → `mise run test`

Every other consumer of `make` (`ci-experimental.yml`, `ci-sync-preview.yml`,
`ci-sync-deployment.yml`, `benchmark.yml`, `interop-tests.yml`, `release.yml`, `.gitlab-ci.yml`,
`containers/test/Dockerfile`) is explicitly **out of scope** and stays exactly as-is.

## Data Flow

```
developer clone ──► mise install (pins go/golangci-lint/goreleaser/node/pre-commit/addlicense)
                       └─► pre-commit install (registers git hook)

git commit ──► pre-commit hooks: mise run fmt / mise exec -- go vet / mise run build /
               mise exec -- golangci-lint run --new-from-rev=HEAD
                       └─(all pass)─► commit proceeds
                       └─(any fail)─► commit blocked, fix and re-stage

push to PR ──► oss-build-test.yml: jdx/mise-action (installs pinned tools)
                 └─► mise run build / mise run generate && git diff --exit-code / mise run test
```

## Error Handling

- Pre-commit hooks fail the commit (non-zero exit) on any fmt/vet/build/lint failure — standard
  pre-commit behavior, no custom handling needed.
- `oss-build-test.yml`'s `mise run generate && git diff --exit-code` fails the job if generation
  drift is detected — unchanged behavior from the existing `make`-based step.
- `jdx/mise-action` failing to install a pinned tool fails the CI job before any build step
  runs, surfacing tool-version problems early rather than as a confusing later build failure.

## What Is NOT Changed

- `Makefile`, `makefiles/unix.mk`, `makefiles/windows.mk`, `makefiles/common.mk`,
  `makefiles/_mkconfig.mk` — all untouched.
- `ci-experimental.yml`, `ci-sync-preview.yml`, `ci-sync-deployment.yml`, `benchmark.yml`,
  `interop-tests.yml`, `release.yml`, `.gitlab-ci.yml`, `containers/test/Dockerfile` — all
  untouched, continue to call `make ...` exactly as today.
- `oss-pr.yml`, `oss-main.yml`, `oss-release.yml` — untouched; only the reusable
  `oss-build-test.yml` they call is modified.
- `goreleaser/goreleaser-action` usage inside `oss-main.yml`/`oss-release.yml` — untouched, not
  a Makefile wrapper, out of scope for this migration.

## Open Questions Resolved During Brainstorming

- **Replacement scope**: started as "full Makefile deletion + rewrite all 8 CI consumers",
  walked back to "Makefile stays, only `oss-build-test.yml` migrates" once the user confirmed
  the remaining 7 consumers are either legacy/disabled or deliberately left alone.
- **Tool management**: all tool versions pinned in `mise.toml`'s `[tools]` (not scattered across
  Makefile variables).
- **Pre-commit framework**: pre-commit.com, not a custom script.
- **Pre-commit scope**: fast checks only — fmt, vet, build, lint — no full test suite.
- **Task organization**: all tasks inline in `mise.toml` (no `mise-tasks/` files), per explicit
  user preference against the assistant's recommended default.
- **CI/local parity rationale**: mise.toml is used for *both* onboarding and the one active CI
  file specifically so there is no version drift between a developer's machine and CI.
