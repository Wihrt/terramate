# OSS GitHub Actions Pipelines — Design Spec

**Date:** 2026-07-06
**Status:** Approved

## Problem

The repository's existing GitHub Actions workflows (`ci-sync-preview.yml`, `ci-sync-deployment.yml`, `release.yml`) are tightly coupled to Terramate Cloud infrastructure: paid Blacksmith runners, a licensed GoReleaser Pro binary, and secrets for Cosign signing, Discord, Twitter, and Fury (packagecloud). There is no lightweight, self-contained pipeline that only needs a stock `GITHUB_TOKEN` and GitHub-hosted runners.

## Goal

Add three new, fully independent workflows that coexist with (do not modify or replace) the existing ones:

1. **Pull Request** → Build + Test.
2. **`main` branch** → Build + Test + publish binaries to a floating "dev" GitHub pre-release.
3. **Release tag** (`vX.Y.Z`) → Build + Test + publish binaries to a real GitHub Release.

No Docker image is in scope — binaries only, published exclusively as GitHub Releases/pre-releases (no external registries, no plain workflow artifacts as the primary distribution mechanism).

## Non-Goals

- Do not touch or reuse `ci-sync-preview.yml`, `ci-sync-deployment.yml`, or the existing `release.yml`.
- No Docker image / container registry publishing.
- No GoReleaser Pro features (Cosign signing, Discord/Twitter announcements, Fury/packagecloud publishing). GoReleaser **OSS** only.
- No matrix of OS runners for build/test — `ubuntu-latest` only. Can be extended later if needed.
- No lint / `go mod tidy` / license-header checks on PR — the user asked for Build + Test only.

## Architecture

### File layout (all new files, `.github/workflows/`)

- `oss-build-test.yml` — reusable workflow (`workflow_call`) containing the build/test job. Avoids duplicating the same steps across the three trigger workflows.
- `oss-pr.yml` — trigger `pull_request`. Calls `oss-build-test.yml`.
- `oss-main.yml` — trigger `push: branches: [main]`. Calls `oss-build-test.yml`, then runs the "publish dev binaries" job on success.
- `oss-release.yml` — trigger `push: tags: ["v*.*.*"]`. Calls `oss-build-test.yml`, then runs the "publish release binaries" job on success.
- `.goreleaser.yml` — new GoReleaser OSS config at repo root (does not exist today) describing build targets and archive format.

Distinct `oss-` filename prefix and distinct `name:` fields (e.g. "OSS / Pull Request") avoid any confusion with the existing Terramate-Cloud-coupled workflows in the Actions UI.

### `oss-build-test.yml` (reusable)

Single job `build_test` on `ubuntu-latest`:

1. `actions/checkout` (`fetch-depth: 0` — needed later for GoReleaser changelog generation from git tags).
2. `actions/setup-go` with `go-version-file: go.mod`.
3. `make build` — builds `bin/terramate` and `bin/terramate-ls`.
4. `make generate && git diff --exit-code` — regenerate and fail if generated files are stale (matches existing `ci-experimental.yml` pattern).
5. `make test` — runs `go test -race` orchestrated via the freshly built `terramate` binary (existing `test` Makefile target already handles building `bin/helper` etc.).

This mirrors the existing `ci-experimental.yml` pattern minus the Windows-specific `core.autocrlf` step, since we're targeting Linux only.

### `oss-pr.yml`

```yaml
on: pull_request
jobs:
  build_test:
    uses: ./.github/workflows/oss-build-test.yml
```

### `oss-main.yml`

```yaml
on:
  push:
    branches: [main]
jobs:
  build_test:
    uses: ./.github/workflows/oss-build-test.yml
  publish_dev:
    needs: build_test
    runs-on: ubuntu-latest
    permissions:
      contents: write   # to create/update the GitHub pre-release
    steps:
      - checkout (fetch-depth: 0)
      - setup-go
      - install goreleaser (OSS, via `go install github.com/goreleaser/goreleaser/v2@latest` or the official action)
      - run: goreleaser build --snapshot --clean
      - publish/update the floating "dev" pre-release via `softprops/action-gh-release@<pinned-sha>`
          with: tag_name: dev, prerelease: true, name: "Development build (main@<short-sha>)",
                files: dist/*.tar.gz, dist/*.zip, dist/*checksums.txt
```

`softprops/action-gh-release` overwrites the existing `dev` release's assets when re-run against the same `tag_name`, giving us the "always latest main build" semantics without needing GoReleaser Pro's nightly feature.

### `oss-release.yml`

```yaml
on:
  push:
    tags: ["v*.*.*"]
jobs:
  build_test:
    uses: ./.github/workflows/oss-build-test.yml
  publish_release:
    needs: build_test
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - checkout (fetch-depth: 0)
      - setup-go
      - install goreleaser (OSS)
      - run: goreleaser release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

`goreleaser release --clean` (OSS edition) builds all archives, generates checksums and a changelog from git log, and creates/publishes the GitHub Release for the pushed tag directly — no extra action needed for this path.

### `.goreleaser.yml` (new)

Minimal OSS config:

- `builds`: one entry per binary (`terramate`, `terramate-ls`), reusing the existing static-build flags from `makefiles/unix.mk` (`CGO_ENABLED=0`, `-extldflags "-static"`). Target matrix:
  - `linux`: `amd64`, `arm64`
  - `darwin`: `amd64`, `arm64`
  - `windows`: `amd64`
  - No `386` (legacy 32-bit x86) on any OS — negligible real-world usage for a modern IaC CLI, and `darwin/386` is not buildable at all (Go and macOS both dropped 32-bit support). Use `ignore:` entries to exclude the `windows/arm64`, `linux/386`, `darwin/386`, `windows/386` combinations GoReleaser would otherwise generate from the default matrix.
  - No existing `.goreleaser.yml` was found in the repository to copy this matrix from — the current Pro-based release pipeline's config lives outside this repo (private/vault). This matrix is inferred from the existing CI evidence (the `ci-sync-*` workflows test on `macos-15`, an arm64 runner) and standard Go CLI conventions, then confirmed with the user.
- `archives`: `tar.gz` for linux/darwin, `zip` for windows.
- `checksum`: default (`checksums.txt`).
- `changelog`: commit-log-based changelog, excluding commits with a `chore:` prefix (the repo's existing convention, confirmed via `git log`: `feat:`, `fix:`, `chore:`, `x:` prefixes are in active use).
- No `signs`, `dockers`, `announce`, or `publishers` sections (those are the Pro/paid-integration parts we're explicitly dropping).

## Data Flow

```
pull_request ──► oss-build-test.yml (build/test) ──► pass/fail check

push main ──► oss-build-test.yml (build/test)
                 └─(success)─► goreleaser build --snapshot ──► dist/*
                                  └─► softprops/action-gh-release (tag: dev, prerelease)
                                        overwrites assets on the floating "dev" release

push tag vX.Y.Z ──► oss-build-test.yml (build/test)
                       └─(success)─► goreleaser release --clean
                                        └─► creates GitHub Release "vX.Y.Z" with archives + checksums
```

## Error Handling

- `make generate && git diff --exit-code` fails the job (non-zero exit) if generation drift is detected — same as existing CI.
- `publish_dev` / `publish_release` jobs use `needs: build_test`, so they never run if build/test fails.
- GoReleaser's own failure modes (build errors, GitHub API errors) fail the job naturally; no custom error handling needed beyond default `set -e` shell behavior.

## What Is NOT Changed

- `ci-sync-preview.yml`, `ci-sync-deployment.yml`, `release.yml` — untouched, continue to run in parallel for their existing purpose (Terramate Cloud preview/deployment sync and the Pro GoReleaser release with Cosign signing).
- `containers/test/Dockerfile` — untouched (used only for `make test/docker`, unrelated to this work).
- No changes to `VERSION` file semantics or `make release/tag`.

## Open Questions Resolved During Brainstorming

- **Scope**: independent/autonomous workflows, not a replacement of the existing Cloud-coupled ones.
- **Docker**: dropped entirely per user follow-up — binaries only.
- **Distribution mechanism**: GitHub Releases (real release on tag, floating `dev` pre-release on main) — not plain workflow artifacts.
- **GoReleaser edition**: OSS (free), since Pro requires a paid license this independent pipeline shouldn't depend on.
