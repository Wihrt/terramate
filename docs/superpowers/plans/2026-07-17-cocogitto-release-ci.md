# Cocogitto-Driven Release CI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make cocogitto the source of truth for versioning, tagging, and changelog/release-notes generation in the OSS release pipeline, replacing the floating "dev" pre-release with real `-dev.N` tags, without any privileged CI token.

**Architecture:** `oss-main.yml` becomes self-contained: `cog bump` (auto dev profile) → `cog changelog --at` → `goreleaser release --release-notes=...`, all in one job, all pushed with the default `GITHUB_TOKEN`. `oss-release.yml` keeps its existing tag-push trigger (fed by the maintainer running `cog bump --hook-profile release` locally) and gains the same `cog changelog --at` → `goreleaser release --release-notes=...` sequence. Both workflows switch from `actions/setup-go` + `goreleaser-action` to `jdx/mise-action`, since `cocogitto` and `goreleaser` are already pinned in `.mise.toml`.

**Tech Stack:** GitHub Actions, `jdx/mise-action`, cocogitto 7.0.0 (`cog`), GoReleaser 2.17.0 (OSS), bash.

## Global Constraints

- No `workflow_dispatch` button, no PAT/GitHub App token — every CI push uses the default `GITHUB_TOKEN` (per `docs/superpowers/specs/2026-07-17-cocogitto-release-ci-design.md`, "Non-Goals").
- Every GitHub Action reference must be pinned to a full commit SHA with a trailing `# vX.Y.Z` comment, matching the existing convention in `.github/workflows/*.yml` (see `mise.toml`'s `pin-gha` task).
- Do not touch `ci-sync-preview.yml`, `ci-sync-deployment.yml`, the legacy Pro `release.yml`, `oss-pr.yml`, or `oss-build-test.yml`.
- Do not touch `.goreleaser.yaml`'s `builds`, `archives`, `checksum` sections, or the already-fixed `release.name_template` (commit `c552a6bf`).
- Do not touch `cog.toml`'s `tag_prefix`, `branch_whitelist`, `pre_bump_hooks`/`post_bump_hooks`, or `[bump_profiles.release]`.
- `cog bump`'s default-profile `post_bump_hooks` only pushes the tag (`git push origin {{version_tag}}`), never the `main` branch — this is existing, intentional `cog.toml` behavior (dev bumps must not spam `main` with a VERSION-bump commit) and must not be "fixed".

---

## Task 1: Enable cocogitto changelog generation

**Files:**
- Modify: `cog.toml`

**Interfaces:**
- Produces: `cog changelog --at <tag>` (no `--template` flag needed anywhere else in this plan — `full_hash` becomes the default via this config).

- [ ] **Step 1: Edit `cog.toml`**

Change line 7 from:
```toml
disable_changelog = true
```
to:
```toml
disable_changelog = false
```

Then add a new `[changelog]` section directly after the top-level keys (before `pre_bump_hooks`):
```toml
[changelog]
template = "full_hash"
```

Full resulting top section of `cog.toml`:
```toml
#:schema https://docs.cocogitto.io/cog-schema.json

tag_prefix = "v"
branch_whitelist = ["main"]
ignore_merge_commits = true
from_latest_tag = true
disable_changelog = false

[changelog]
template = "full_hash"

# Default profile: automatic "dev" pre-release path.
# {{version}} is the bare semver (no "v"), matching VERSION's existing convention.
# {{version_tag}} includes tag_prefix -- this is the actual git ref to push.
pre_bump_hooks = ["echo -n {{version}} > VERSION"]
post_bump_hooks = ["git push origin {{version_tag}}"]
```

- [ ] **Step 2: Verify the config parses and the template applies**

Run:
```bash
cog changelog --at v0.9.4 | head -5
```
Expected: non-empty markdown output starting with a `####` heading (e.g. `#### Miscellaneous Chores`) followed by lines of the form `- <40-char-hex-hash> - <subject> - <author>` (the `full_hash` template's signature — confirms `cog.toml`'s new `[changelog]` block is picked up without needing an explicit `--template` flag).

- [ ] **Step 3: Commit**

```bash
git add cog.toml
git commit -m "feat: enable cocogitto changelog generation with the full_hash template"
```

---

## Task 2: Remove dead changelog config from `.goreleaser.yaml`

**Files:**
- Modify: `.goreleaser.yaml:66-70`

**Interfaces:**
- Consumes: none.
- Produces: none (later tasks pass `--release-notes` on the CLI instead, making this config unnecessary — see Tasks 4 and 5).

- [ ] **Step 1: Remove the `changelog:` block**

Delete these lines from `.goreleaser.yaml`:
```yaml
changelog:
  sort: asc
  filters:
    exclude:
      - "^chore:"
```
so the file goes directly from the `checksum:` block to the `release:` block:
```yaml
checksum:
  name_template: "checksums.txt"

release:
  prerelease: auto
  name_template: >-
    {{ if contains .Tag "-dev." }}Development build {{ .Tag }}{{ else }}{{ .Tag }}{{ end }}
```

- [ ] **Step 2: Verify the config still validates**

Run:
```bash
goreleaser check
```
Expected:
```
  • checking                                  path=.goreleaser.yaml
  • 1 configuration file(s) validated
  • thanks for using GoReleaser!
```

- [ ] **Step 3: Commit**

```bash
git add .goreleaser.yaml
git commit -m "chore: drop goreleaser's own changelog config, notes now come from cocogitto"
```

---

## Task 3: Remove obsolete pre-cocogitto mise tasks

**Files:**
- Modify: `.mise.toml`

**Interfaces:**
- Consumes: none.
- Produces: none.

- [ ] **Step 1: Remove `tasks.release` and `tasks."release:tag"`**

Delete these two blocks from `.mise.toml`:
```toml
[tasks.release]
description = "Generate a terramate release"
run = "goreleaser release --rm-dist --key $GORELEASER_KEY"
```
```toml
[tasks."release:tag"]
description = "Create a new release tag"
run = '''
version=${VERSION:-v$(cat VERSION)}
git tag -s -a "$version" -m "Release $version"
git push origin "$version"
'''
```
(`--rm-dist` was renamed `--clean` in GoReleaser v2 and this task bypasses `cog bump`/`CHANGELOG.md` entirely — both are superseded by `cog bump` / `cog bump --hook-profile release`, already configured in `cog.toml`.)

- [ ] **Step 2: Verify the remaining tasks list is intact**

Run:
```bash
mise tasks ls | grep release
```
Expected: exactly one line, `release:dry-run            Dry run of the release process` — confirming `release` and `release:tag` are gone while the unrelated `release:dry-run` task is untouched.

- [ ] **Step 3: Commit**

```bash
git add .mise.toml
git commit -m "chore: remove mise tasks superseded by cog bump"
```

---

## Task 4: Rewrite `oss-main.yml` to self-contained cocogitto + goreleaser publish

**Files:**
- Modify: `.github/workflows/oss-main.yml`

**Interfaces:**
- Consumes: `cog.toml`'s default bump profile (Task 1), `.goreleaser.yaml`'s `release.name_template` (already fixed in commit `c552a6bf`).
- Produces: on a qualifying push to `main`, a real GitHub Release tagged `vX.Y.Z-dev.N`.

- [ ] **Step 1: Replace the `publish_dev` job**

Replace the entire `publish_dev` job in `.github/workflows/oss-main.yml` (currently lines 19-52) with:

```yaml
  publish_dev:
    name: Publish dev release
    needs: build_test
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4
        with:
          fetch-depth: 0

      - uses: jdx/mise-action@e6a8b3978addb5a52f2b4cd9d91eafa7f0ab959d # v4.2.0
        with:
          install: true

      - name: configure git identity for the bump commit
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

      - name: check whether there is anything to bump
        id: check
        run: |
          if mise exec -- cog bump --dry-run --auto --pre "dev.*"; then
            echo "should_bump=true" >> "$GITHUB_OUTPUT"
          else
            echo "should_bump=false" >> "$GITHUB_OUTPUT"
          fi

      - name: cog bump (dev pre-release)
        if: steps.check.outputs.should_bump == 'true'
        run: mise exec -- cog bump --auto --pre "dev.*"

      - name: generate release notes
        if: steps.check.outputs.should_bump == 'true'
        run: |
          tag=$(git describe --tags --exact-match)
          mise exec -- cog changelog --at "$tag" > /tmp/notes.md

      - name: goreleaser release
        if: steps.check.outputs.should_bump == 'true'
        run: mise exec -- goreleaser release --clean --release-notes=/tmp/notes.md
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

This drops the old `goreleaser-action`/`softprops/action-gh-release` steps and the floating `dev` tag entirely. `cog bump --dry-run` exits non-zero when there are no conventional commits to bump since the last tag (e.g. a `chore:`-only push) — `should_bump` gates every following step so those pushes succeed as a no-op instead of failing the job.

- [ ] **Step 2: Verify workflow YAML is well-formed**

Run:
```bash
python3 -c "import yaml, sys; yaml.safe_load(open('.github/workflows/oss-main.yml'))" && echo OK
```
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/oss-main.yml
git commit -m "feat: publish real -dev.N releases via cog bump instead of a floating dev pre-release"
```

---

## Task 5: Rewrite `oss-release.yml` to consume cocogitto-generated notes

**Files:**
- Modify: `.github/workflows/oss-release.yml`

**Interfaces:**
- Consumes: `cog.toml`'s `[changelog]` template (Task 1); the tag pushed by the maintainer's local `cog bump --hook-profile release` (unchanged, out of scope).
- Produces: on a maintainer-pushed `vX.Y.Z` tag, a real GitHub Release with cocogitto-generated notes.

- [ ] **Step 1: Replace the `publish_release` job**

Replace the entire `publish_release` job in `.github/workflows/oss-release.yml` (currently lines 19-42) with:

```yaml
  publish_release:
    name: Publish release binaries
    needs: build_test
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683 # v4
        with:
          fetch-depth: 0

      - uses: jdx/mise-action@e6a8b3978addb5a52f2b4cd9d91eafa7f0ab959d # v4.2.0
        with:
          install: true

      - name: generate release notes
        run: mise exec -- cog changelog --at "${{ github.ref_name }}" > /tmp/notes.md

      - name: Run GoReleaser (release)
        run: mise exec -- goreleaser release --clean --release-notes=/tmp/notes.md
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

- [ ] **Step 2: Verify workflow YAML is well-formed**

Run:
```bash
python3 -c "import yaml, sys; yaml.safe_load(open('.github/workflows/oss-release.yml'))" && echo OK
```
Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/oss-release.yml
git commit -m "feat: use cocogitto-generated notes for tagged releases"
```

---

## Task 6: End-to-end sandbox verification

Proves the `cog bump` → `cog changelog --at` chain used in Task 4 actually works end-to-end — including the tag-only push semantics — **without pushing anything to the real `Wihrt/terramate` GitHub remote**. This is done in a throwaway local sandbox with its own fake `origin`, never touching `git remote origin` of the real working copy.

**Files:** none (verification only).

- [ ] **Step 1: Build an isolated sandbox with a fake origin**

```bash
SANDBOX=/tmp/claude-1001/-home-arnaud-hatzenbuhler-Documents-Projects-github-terramate/8c05d0da-7c49-4cd1-9e2f-bee2523fe116/scratchpad/cog-e2e-sandbox
rm -rf "$SANDBOX"
mkdir -p "$SANDBOX"
git init --bare "$SANDBOX/fake-origin.git"
git clone /home/arnaud.hatzenbuhler/Documents/Projects/github/terramate "$SANDBOX/work"
cd "$SANDBOX/work"
git checkout main
git remote set-url origin "$SANDBOX/fake-origin.git"
git push origin main --tags
git config user.name "CI Bot"
git config user.email "ci-bot@example.invalid"
```

- [ ] **Step 2: Confirm the dry-run matches the design's expected version**

```bash
mise exec -- cog bump --dry-run --auto --pre "dev.*"
```
Expected: `v0.18.0-dev.1` (matches the value already confirmed against the real `main` branch during design — same commit history, since the sandbox was cloned from it).

- [ ] **Step 3: Run the real bump and confirm only the tag is pushed, not `main`**

```bash
mise exec -- cog bump --auto --pre "dev.*"
git describe --tags --exact-match
git rev-parse main
git --git-dir="$SANDBOX/fake-origin.git" rev-parse main
git --git-dir="$SANDBOX/fake-origin.git" tag -l 'v0.18.0-dev.1'
```
Expected: `git describe --tags --exact-match` prints `v0.18.0-dev.1`; the local `main` and the fake origin's `main` now **differ** (the bump commit exists locally but was never pushed — matches the `Global Constraints` note); `git tag -l` on the fake origin's bare repo lists `v0.18.0-dev.1` (the tag *was* pushed, per `post_bump_hooks`).

- [ ] **Step 4: Confirm changelog generation for the freshly created tag**

```bash
mise exec -- cog changelog --at v0.18.0-dev.1 | head -5
```
Expected: non-empty markdown starting with a `####` heading, same shape as Task 1 Step 2's output.

- [ ] **Step 5: Clean up the sandbox**

```bash
rm -rf "$SANDBOX"
```

No commit for this task — it's a verification-only step with no repository changes.
