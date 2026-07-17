# Cocogitto-Driven Release CI — Design Spec

**Date:** 2026-07-17
**Status:** Approved

## Problem

Two issues in the current OSS release pipeline (introduced in
[2026-07-06-oss-github-actions-pipelines-design.md](2026-07-06-oss-github-actions-pipelines-design.md)):

1. **Immediate bug**: `.goreleaser.yaml`'s `release.name_template` calls `regexMatch`, a function
   GoReleaser has never defined (it's a Sprig/Helm function, not a GoReleaser template function).
   Every real release — dev or clean — fails at the "scm releases: publishing" step because
   GoReleaser cannot even parse the template. This is what broke the `v0.18.0` release
   (https://github.com/Wihrt/terramate/actions/runs/29594928056/job/87935614917).
2. **Architectural gap**: `cog.toml` already declares a cocogitto bump/tag workflow (a `dev`
   default profile and a manual `release` profile), but nothing in CI or the release pipeline
   actually uses cocogitto. Tags are created ad hoc, the "dev" pre-release on `main` is a single
   floating GitHub pre-release (not a real semver tag), and GoReleaser generates its own
   changelog from raw commit log instead of from conventional-commit-aware cocogitto output.

## Goal

- Fix the `regexMatch` bug so releases (dev and clean) succeed again.
- Make cocogitto the single source of truth for version bumping, tagging, and changelog/release
  notes generation, GitLab-CI style: a linear, self-contained pipeline per trigger, no cross-workflow
  re-triggering games.
- Keep the two existing workflow entry points (`oss-main.yml` for automatic dev builds, `oss-release.yml`
  for real releases) but make each one fully self-contained.

## Non-Goals

- No `workflow_dispatch` "push a button to release" job. Real releases are triggered by the
  maintainer running `cog bump --hook-profile release` **locally**, exactly as `cog.toml` already
  documents. This was considered (see "Options Considered" below) and explicitly rejected in favor
  of the simpler, already-working tag-push trigger.
- No PAT / GitHub App token provisioning. Every push made from CI keeps using the default
  `GITHUB_TOKEN`.
- No change to `ci-sync-preview.yml`, `ci-sync-deployment.yml`, or the legacy Pro `release.yml`
  (untouched Terramate Cloud pipeline, out of scope as established in the prior OSS pipelines spec).
- No change to the build matrix, archive formats, or checksum logic in `.goreleaser.yaml` beyond
  what's described below.

## Key Constraint Discovered

**A `git push` performed with the default `GITHUB_TOKEN` does not trigger other GitHub Actions
workflows** (GitHub's built-in loop-prevention). This means if `cog bump` runs inside
`oss-main.yml` and pushes a new tag, `oss-release.yml`'s `push: tags: v*.*.*` trigger will
**not** fire for that tag. This is treated as a *feature*, not a bug to work around: it gives us,
for free, the invariant that `oss-release.yml` only ever runs for tags a human actually pushed
(i.e. real releases via `cog bump --hook-profile release` run locally), never for CI-generated
dev tags. If a future change swaps the dev job's token for a PAT/GitHub App, this invariant breaks
silently and `oss-release.yml` would start double-firing for dev tags — call this out in a code
comment at the point the token is used.

## Options Considered

| Option | Description | Verdict |
|---|---|---|
| A. Single unified workflow, manual button, everything inline | `workflow_dispatch` runs build+test+bump+release in one job for real releases | Rejected — user prefers running `cog bump --hook-profile release` locally, no need for an in-CI manual trigger |
| B. Two workflows + PAT/GitHub App so CI-pushed tags cross-trigger | Keeps today's two-workflow shape, adds a privileged token | Rejected — extra secret to provision/rotate for no benefit once real releases are local |
| C. `repository_dispatch` to explicitly re-trigger | Bump job calls the GitHub API to kick off the release workflow | Rejected — more custom plumbing than B for the same result |
| **D. Two workflows, both self-contained (chosen)** | `oss-main.yml` does bump+publish inline for dev tags; `oss-release.yml` keeps its existing tag-push trigger, fed by a human running `cog bump` locally | **Chosen** — no new secrets, no cross-workflow dependency, matches how the maintainer already works today |

## Architecture

### `.goreleaser.yaml`

- Fix the bug: replace the undefined `regexMatch` with the OSS-available `contains` function.
  ```
  # before (broken — regexMatch is not a GoReleaser function)
  {{ if regexMatch `-dev\.\d+$` .Tag }}Development build {{ .Tag }}{{ else }}{{ .Tag }}{{ end }}
  # after
  {{ if contains .Tag "-dev." }}Development build {{ .Tag }}{{ else }}{{ .Tag }}{{ end }}
  ```
  Verified by rendering both branches of the template against real and synthetic tags
  (`v0.18.0`, `v0.18.0-dev.1`, `v0.17.2-dev.42`, `v0.9.0-rc1`) — behavior matches intent.
- Remove the `changelog:` block (current lines 66-70, the `sort`/`filters` config). It becomes
  dead configuration once release notes are supplied externally via `--release-notes` (see
  below) — GoReleaser skips its own changelog generation entirely when that flag is set, so
  leaving stale config in place would misleadingly suggest it's still the source of truth.

### `cog.toml`

- `disable_changelog = true` → `false`, so cocogitto maintains a real `CHANGELOG.md` on every
  bump (dev and release), not just an in-memory diff.
- Add a `[changelog]` section pinning `template = "full_hash"` — cocogitto's built-in template
  designed for GitHub release bodies (full commit hashes, GitHub-flavored markdown) — as the
  default, so `cog changelog --at <tag>` needs no extra flags in CI.

### `.github/workflows/oss-main.yml` (automatic, `push: branches: [main]`)

Replaces the current "build snapshot, overwrite floating `dev` pre-release" job with:

1. `build_test` (unchanged, reused via `oss-build-test.yml`).
2. `publish_dev` job, `needs: build_test`, `permissions: contents: write`:
   - `actions/checkout` (`fetch-depth: 0`, needed for cocogitto/goreleaser history).
   - `jdx/mise-action` (replaces `actions/setup-go` + `goreleaser/goreleaser-action` — `cocogitto`
     and `goreleaser` are already pinned in `mise.toml`, so this also removes a version drift
     risk between local dev and CI).
   - `mise exec -- cog bump` — uses `cog.toml`'s default profile, auto-detects whether a
     dev bump is warranted from conventional commits since the last tag, creates and pushes a
     real `vX.Y.Z-dev.N` tag using `GITHUB_TOKEN` (does not cross-trigger `oss-release.yml`, see
     "Key Constraint" above). No-ops cleanly if there's nothing to bump (e.g. a `chore:`-only push).
   - `tag=$(git describe --tags --exact-match)` — read back the tag `cog bump` just created
     (HEAD is exactly on it after a bump), rather than reconstructing it from `VERSION`.
   - `mise exec -- cog changelog --at "$tag" > /tmp/notes.md` — the notes for just the version
     that was bumped.
   - `mise exec -- goreleaser release --clean --release-notes=/tmp/notes.md`.
   - Drop the `softprops/action-gh-release` floating-`dev`-tag step entirely — every push to
     `main` that warrants a release now produces its own real, permanent GitHub Release instead
     of overwriting a single floating one.

### `.github/workflows/oss-release.yml` (`push: tags: ["v*.*.*"]`, triggered by a human)

1. `build_test` (unchanged).
2. `publish_release` job, `needs: build_test`:
   - `actions/checkout` (`fetch-depth: 0`).
   - `jdx/mise-action` (same swap as above).
   - `mise exec -- cog changelog --at "${{ github.ref_name }}" > /tmp/notes.md`.
   - `mise exec -- goreleaser release --clean --release-notes=/tmp/notes.md`.
   - The trigger itself is unchanged — a maintainer runs `cog bump --hook-profile release`
     locally (per `cog.toml`'s existing `[bump_profiles.release]`), which pushes to `main` and
     pushes the tag with their own git credentials, firing this workflow exactly as it does
     today.

### `mise.toml` cleanup

Remove two tasks made obsolete/contradictory by this change:

- `tasks.release` — calls `goreleaser release --rm-dist --key $GORELEASER_KEY`; `--rm-dist` was
  removed in GoReleaser v2 (renamed `--clean`), and it bypasses cocogitto entirely.
- `tasks."release:tag"` — manually creates a tag with `git tag -s -a`, bypassing `cog bump` and
  therefore never updating `VERSION` or `CHANGELOG.md`.

Both predate cocogitto adoption and would let a maintainer accidentally cut a release that skips
version bumping and changelog generation. `cog bump` (default and `release` profiles, already
configured) is the only supported path going forward.

## Data Flow

```
push main ──► oss-build-test.yml (build/test)
                └─(success)─► cog bump (dev profile)
                                 └─(if bump occurred)─► push vX.Y.Z-dev.N tag [GITHUB_TOKEN, no cross-trigger]
                                 └─► cog changelog --at <tag> ──► notes.md
                                 └─► goreleaser release --release-notes=notes.md
                                        └─► real, permanent GitHub (pre-)Release "vX.Y.Z-dev.N"

(local) cog bump --hook-profile release
  └─► push origin main
  └─► push origin vX.Y.Z tag [maintainer's own credentials]
        └─► triggers push tag v*.*.*
              └─► oss-build-test.yml (build/test)
                    └─(success)─► cog changelog --at <tag> ──► notes.md
                                    └─► goreleaser release --release-notes=notes.md
                                           └─► GitHub Release "vX.Y.Z"
```

## Error Handling

- `cog bump` on `main` is a no-op (non-error exit) when there's nothing to bump (e.g. only
  `chore:`/`docs:` commits since the last tag) — the `publish_dev` job should treat "nothing to
  bump" as success, not failure, so routine `chore:` merges to `main` don't red the pipeline.
  This needs a guard (e.g. check `cog.toml`'s bump exit semantics / compare `VERSION` before and
  after) rather than assuming `cog bump` always produces a new tag.
- `mise exec -- cog changelog --at <tag>` failing (e.g. malformed conventional commits) fails the
  job before any GitHub Release is created — no partial/half-published release.
- `goreleaser release` failure modes are unchanged from today (build errors, GitHub API errors
  fail the job naturally).

## What Is NOT Changed

- `ci-sync-preview.yml`, `ci-sync-deployment.yml`, legacy Pro `release.yml` — untouched.
- `oss-pr.yml`, `oss-build-test.yml` — untouched.
- Build matrix, archive formats, checksum config in `.goreleaser.yaml` — untouched.
- `cog.toml`'s `tag_prefix`, `branch_whitelist`, `pre_bump_hooks`/`post_bump_hooks`, and the
  `[bump_profiles.release]` profile itself — untouched (the maintainer's local release flow keeps
  working exactly as configured today).

## Open Questions Resolved During Brainstorming

- **Release trigger**: manual, but "manual" means the maintainer runs `cog bump` locally, not a
  GitHub Actions `workflow_dispatch` button — this was the maintainer's stated preference after
  learning about the `GITHUB_TOKEN` cross-trigger limitation.
- **Changelog ownership**: cocogitto generates it entirely (`cog changelog --at`); GoReleaser only
  consumes it via `--release-notes`, its own changelog logic is unused.
- **Dev pre-release mechanism**: the floating single `dev` GitHub pre-release is replaced by real,
  permanent `vX.Y.Z-dev.N` tags/releases created automatically by `cog bump` on every qualifying
  push to `main`.
