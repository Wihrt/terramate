# OSS GitHub Actions Pipelines Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three independent GitHub Actions workflows (PR, main, release) that build/test the Go binaries and publish multi-arch binaries as GitHub Releases, using GoReleaser OSS only — no Docker, no Terramate Cloud dependency, no paid services.

**Architecture:** A reusable `workflow_call` job (`oss-build-test.yml`) does `make build` / `make generate` (drift check) / `make test` and is called by three thin trigger workflows. `oss-main.yml` and `oss-release.yml` each add a publish job that runs GoReleaser OSS and uploads archives to a GitHub Release — a floating `dev` pre-release on every `main` push, a real versioned release on every `vX.Y.Z` tag push.

**Tech Stack:** GitHub Actions, GoReleaser OSS v2, `goreleaser/goreleaser-action`, `softprops/action-gh-release`, existing repo `Makefile` targets (`build`, `generate`, `test`).

## Global Constraints

- Every new file gets the repo's copyright header: `# Copyright 2026 Terramate GmbH` / `# SPDX-License-Identifier: MPL-2.0` (per `CLAUDE.md`).
- Do not modify `ci-sync-preview.yml`, `ci-sync-deployment.yml`, `release.yml`, or `containers/test/Dockerfile` — these stay untouched.
- No Docker image, no external container registry.
- Binaries are published exclusively as GitHub Releases/pre-releases (not plain workflow artifacts).
- GoReleaser **OSS** edition only — no Cosign signing, no Discord/Twitter/Fury integrations, no license key.
- Build/test runs on `ubuntu-latest` only (no OS matrix).
- PR pipeline is Build + Test only — no lint / `mod tidy` / license-check steps.
- Release build matrix: `linux` (amd64, arm64), `darwin` (amd64, arm64), `windows` (amd64 only) — no `386` anywhere, no `windows/arm64`.
- Third-party GitHub Actions (anything not already pinned elsewhere in this repo) must be pinned to a commit SHA with a `# pin@vX` comment, using the repo's own `pin-github-action` tool — not hand-typed SHAs.
- Reuse the exact existing pins already present in other workflows for `actions/checkout` (`11bd71901bbe5b1630ceea73d27597364c9af683 # pin@v4`) and `actions/setup-go` (`0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # pin@v5`) — don't re-pin those.
- The design spec is `docs/superpowers/specs/2026-07-06-oss-github-actions-pipelines-design.md` — consult it for the full rationale.

---

## Task 1: GoReleaser OSS config (`.goreleaser.yml`)

**Files:**
- Create: `.goreleaser.yml` (repo root)

**Interfaces:**
- Produces: a GoReleaser config that Tasks 4 and 5 invoke via `goreleaser/goreleaser-action`. Task 4 uses `goreleaser release --snapshot --clean --skip=publish`; Task 5 uses `goreleaser release --clean`. Both expect `dist/*.tar.gz`, `dist/*.zip`, and `dist/checksums.txt` to exist afterwards.

There is no existing `.goreleaser.yml` in this repository to copy from (the current Pro-based `release.yml` config lives outside this repo). This task creates a new, from-scratch OSS config.

- [ ] **Step 1: Write `.goreleaser.yml`**

```yaml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

version: 2

project_name: terramate

builds:
  - id: terramate
    main: ./cmd/terramate
    binary: terramate
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -extldflags "-static"
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64

  - id: terramate-ls
    main: ./cmd/terramate-ls
    binary: terramate-ls
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -extldflags "-static"
    goos:
      - linux
      - darwin
      - windows
    goarch:
      - amd64
      - arm64
    ignore:
      - goos: windows
        goarch: arm64

archives:
  - id: default
    ids:
      - terramate
      - terramate-ls
    formats:
      - tar.gz
    format_overrides:
      - goos: windows
        formats:
          - zip
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}

checksum:
  name_template: "checksums.txt"

changelog:
  sort: asc
  filters:
    exclude:
      - "^chore:"

release:
  prerelease: auto
```

- [ ] **Step 2: Validate the config schema**

Run:
```bash
go run github.com/goreleaser/goreleaser/v2@latest check
```
Expected: prints `1 configuration file(s) validated` (or similar) with no errors. If it reports a schema error (e.g. a field renamed between GoReleaser versions), fix the reported field in `.goreleaser.yml` and re-run this command until it passes clean — do not proceed to Step 3 until this is a clean pass.

- [ ] **Step 3: Run a full snapshot build to prove cross-compilation and archiving actually work**

Run:
```bash
go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean --skip=publish
```
Expected: exits 0. If any `goos`/`goarch` combination fails to compile, fix the build (this should not happen since `CGO_ENABLED=0` cross-compiles cleanly for all these targets in pure Go).

- [ ] **Step 4: Verify the expected artifacts exist**

Run:
```bash
ls dist/*.tar.gz dist/*.zip dist/checksums.txt
```
Expected: 4 `.tar.gz` files (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64), 1 `.zip` file (windows/amd64), and `dist/checksums.txt` — 6 lines total.

- [ ] **Step 5: Clean up local build artifacts and commit**

```bash
rm -rf dist/
git add .goreleaser.yml
git commit -m "feat: add GoReleaser OSS config for binary releases"
```

---

## Task 2: Reusable build/test workflow (`oss-build-test.yml`)

**Files:**
- Create: `.github/workflows/oss-build-test.yml`

**Interfaces:**
- Consumes: `make build`, `make generate`, `make test` (existing `Makefile` targets — no changes to them).
- Produces: a `workflow_call` reusable workflow with job id `build_test`, invoked by Tasks 3, 4, and 5 as `uses: ./.github/workflows/oss-build-test.yml`.

- [ ] **Step 1: Write the workflow**

```yaml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

name: OSS / Build & Test

on:
  workflow_call:

jobs:
  build_test:
    name: Build and Test
    runs-on: ubuntu-latest
    timeout-minutes: 30

    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # pin@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # pin@v5
        with:
          go-version-file: 'go.mod'
          check-latest: true

      - name: make build
        run: make build

      - name: make generate (fail on drift)
        run: make generate && git diff --exit-code

      - name: make test
        run: make test
```

This mirrors the existing `ci-experimental.yml` steps (minus the Windows-only `core.autocrlf` step, since this targets Linux only). It needs no OpenTofu/Terraform/Terragrunt setup steps: `e2etests/internal/runner/installers.go` self-installs Terraform/OpenTofu on demand via `hc-install` when they're not already on `PATH`, which is exactly how `ci-experimental.yml` already gets away with not installing them either.

- [ ] **Step 2: Validate workflow YAML syntax and semantics**

Run:
```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/oss-build-test.yml
```
Expected: no output (actionlint prints nothing and exits 0 when there are no issues). Fix any reported issues before continuing.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/oss-build-test.yml
git commit -m "feat: add reusable OSS build/test workflow"
```

---

## Task 3: Pull Request workflow (`oss-pr.yml`)

**Files:**
- Create: `.github/workflows/oss-pr.yml`

**Interfaces:**
- Consumes: the `oss-build-test.yml` reusable workflow from Task 2.

- [ ] **Step 1: Write the workflow**

```yaml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

name: OSS / Pull Request

on:
  pull_request:

permissions:
  contents: read

jobs:
  build_test:
    name: Build and Test
    uses: ./.github/workflows/oss-build-test.yml
```

- [ ] **Step 2: Validate**

Run:
```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/oss-pr.yml
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/oss-pr.yml
git commit -m "feat: add OSS pull request pipeline (build+test)"
```

---

## Task 4: Main branch workflow with dev binary publishing (`oss-main.yml`)

**Files:**
- Create: `.github/workflows/oss-main.yml`

**Interfaces:**
- Consumes: `oss-build-test.yml` (Task 2), `.goreleaser.yml` (Task 1).
- Produces: a floating GitHub pre-release tagged `dev`, whose assets are replaced on every push to `main`.

- [ ] **Step 1: Write the workflow**

```yaml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

name: OSS / Main

on:
  push:
    branches:
      - main

permissions:
  contents: read

jobs:
  build_test:
    name: Build and Test
    uses: ./.github/workflows/oss-build-test.yml

  publish_dev:
    name: Publish dev binaries
    needs: build_test
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # pin@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # pin@v5
        with:
          go-version-file: 'go.mod'
          check-latest: true

      - name: Run GoReleaser (snapshot build, no publish to GitHub by goreleaser itself)
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: latest
          args: release --snapshot --clean --skip=publish

      - name: Update floating dev pre-release
        uses: softprops/action-gh-release@v2
        with:
          tag_name: dev
          name: "Development build (main@${{ github.sha }})"
          prerelease: true
          make_latest: false
          files: |
            dist/*.tar.gz
            dist/*.zip
            dist/checksums.txt
```

`--skip=publish` makes GoReleaser run its full local pipeline (build, archive, checksum, changelog) without trying to create/update a GitHub Release itself — `softprops/action-gh-release` owns that step instead, and it overwrites the existing `dev` release's assets on every run because it always targets the same `tag_name: dev`.

- [ ] **Step 2: Validate**

Run:
```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/oss-main.yml
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/oss-main.yml
git commit -m "feat: add OSS main pipeline (build+test+dev binaries)"
```

---

## Task 5: Release workflow (`oss-release.yml`)

**Files:**
- Create: `.github/workflows/oss-release.yml`

**Interfaces:**
- Consumes: `oss-build-test.yml` (Task 2), `.goreleaser.yml` (Task 1).
- Produces: a real GitHub Release for the pushed `vX.Y.Z` tag, with archives + `checksums.txt` attached and an auto-generated changelog.

- [ ] **Step 1: Write the workflow**

```yaml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

name: OSS / Release

on:
  push:
    tags:
      - 'v*.*.*'

permissions:
  contents: read

jobs:
  build_test:
    name: Build and Test
    uses: ./.github/workflows/oss-build-test.yml

  publish_release:
    name: Publish release binaries
    needs: build_test
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # pin@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@0a12ed9d6a96ab950c8f026ed9f722fe0da7ef32 # pin@v5
        with:
          go-version-file: 'go.mod'
          check-latest: true

      - name: Run GoReleaser (release)
        uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: latest
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

Unlike Task 4, this is a plain `release --clean` (no `--snapshot`, no `--skip=publish`): GoReleaser OSS builds, archives, checksums, generates the changelog from git log, and creates the GitHub Release itself using `GITHUB_TOKEN`.

- [ ] **Step 2: Validate**

Run:
```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/oss-release.yml
```
Expected: no output.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/oss-release.yml
git commit -m "feat: add OSS release pipeline (build+test+release binaries)"
```

---

## Task 6: Pin the new third-party actions to commit SHAs

**Files:**
- Modify: `.github/workflows/oss-main.yml` (pins `goreleaser/goreleaser-action@v6` and `softprops/action-gh-release@v2`)
- Modify: `.github/workflows/oss-release.yml` (pins `goreleaser/goreleaser-action@v6`)

**Interfaces:** none (mechanical rewrite of `uses:` refs to `owner/repo@<sha> # pin@vX`, same convention already used throughout this repo's other workflows).

`actions/checkout` and `actions/setup-go` are already pinned (reused from the existing repo convention in Task 2–5); only the two new third-party actions introduced in this plan need pinning.

- [ ] **Step 1: Confirm the `pin-github-action` tool is available**

Run:
```bash
npx --yes pin-github-action --version
```
Expected: prints a version number. (This is the same tool the repo's own `make pin-gha` target uses — see `makefiles/common.mk`.)

- [ ] **Step 2: Pin only the two new workflow files**

Run:
```bash
npx --yes pin-github-action .github/workflows/oss-main.yml .github/workflows/oss-release.yml
```
Expected: the tool rewrites `goreleaser/goreleaser-action@v6` and `softprops/action-gh-release@v2` in place to `owner/repo@<full-sha> # pin@v6` / `# pin@v2`. It must NOT touch `actions/checkout` or `actions/setup-go` lines (already pinned) or any file outside these two.

- [ ] **Step 3: Verify only the intended lines changed**

Run:
```bash
git diff .github/workflows/oss-main.yml .github/workflows/oss-release.yml
```
Expected: only the `goreleaser/goreleaser-action` and `softprops/action-gh-release` `uses:` lines changed, each now `owner/repo@<40-char-sha> # pin@vX`. No other file in the repo shows as modified (`git status`).

- [ ] **Step 4: Re-run actionlint on the two changed files**

Run:
```bash
go run github.com/rhysd/actionlint/cmd/actionlint@latest .github/workflows/oss-main.yml .github/workflows/oss-release.yml
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/oss-main.yml .github/workflows/oss-release.yml
git commit -m "chore: pin new third-party GitHub Actions to commit SHAs"
```

---

## Task 7: End-to-end verification on GitHub Actions

**Files:** none (verification only).

**Interfaces:** none.

GitHub Actions workflows can only be truly verified by actually running them on GitHub — local YAML/schema validation (Tasks 1–6) catches syntax errors but not runtime behavior. This task proves the PR pipeline (the only one that's safe to trigger without publishing a real release or touching `main`) actually runs and passes.

- [ ] **Step 1: Push the branch**

Run:
```bash
git push -u origin feat/oss-github-actions-pipelines
```
Expected: push succeeds.

- [ ] **Step 2: Open a pull request**

Run:
```bash
gh pr create --title "feat: add independent OSS GitHub Actions pipelines" --body "$(cat <<'EOF'
## Summary
- Adds oss-build-test.yml (reusable), oss-pr.yml, oss-main.yml, oss-release.yml
- Adds .goreleaser.yml (GoReleaser OSS, no Docker, GitHub Releases only)
- Fully independent from the existing Terramate-Cloud-coupled workflows

See docs/superpowers/specs/2026-07-06-oss-github-actions-pipelines-design.md
and docs/superpowers/plans/2026-07-06-oss-github-actions-pipelines.md for the
full design and implementation plan.

## Test plan
- [ ] `OSS / Pull Request` check passes on this PR
EOF
)"
```
Expected: prints the created PR URL.

- [ ] **Step 3: Watch the PR check run to completion**

Run:
```bash
gh pr checks --watch
```
Expected: the `Build and Test` check (from `oss-pr.yml` → `oss-build-test.yml`) eventually shows `pass`. If it fails, read the failing step's log with `gh run view --log-failed`, fix the root cause in the relevant workflow/config file, commit, push, and re-run this step.

- [ ] **Step 4: Report status to the user**

Do not merge the PR, push to `main`, or create a release tag — those are the `oss-main.yml` and `oss-release.yml` triggers, and pushing to `main` or tagging a release are actions the user should explicitly approve first (per this repo's git safety conventions). Report the PR URL and the passing check to the user, and note that `oss-main.yml` (dev pre-release) and `oss-release.yml` (real release) will only be exercised once the user merges to `main` / pushes a `vX.Y.Z` tag respectively.

---

## Self-Review Notes

- **Spec coverage:** PR → Build/Test (Task 3), main → Build/Test/dev binaries (Task 4), Release → Build/Test/release binaries (Task 5), no Docker (confirmed absent throughout), GitHub Releases as the sole distribution mechanism (Tasks 4–5), GoReleaser OSS only (Task 1, no Pro features), build matrix linux/darwin amd64+arm64 + windows amd64 with no 386 (Task 1 `.goreleaser.yml`), existing workflows untouched (no task modifies them), third-party actions pinned via the repo's own tool (Task 6). All spec requirements are covered.
- **Placeholder scan:** no TBD/TODO markers; every step has literal file content or an exact runnable command.
- **Type/name consistency:** job id `build_test` is used identically in Tasks 2–5; the `uses: ./.github/workflows/oss-build-test.yml` path in Tasks 3–5 matches the file created in Task 2; archive/checksum filenames referenced in Task 4's `files:` block (`dist/*.tar.gz`, `dist/*.zip`, `dist/checksums.txt`) match the `archives`/`checksum` `name_template`s defined in Task 1.
