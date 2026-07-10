# Remove Terramate Cloud — Design Spec

**Date:** 2026-07-10
**Status:** Approved

## Problem

The repository currently ships as a combined CLI + Terramate Cloud client: a large fraction of
the codebase (~23,000 lines across ~165 files, plus smaller amounts interleaved into otherwise
generic packages) exists solely to support Terramate Cloud — a paid, hosted
observability/drift-detection/deployment-sync product. The repo owner wants to run this as a
CLI-and-code-generation-only tool and does not want the Cloud product, its API client, its auth
flows, or its CI/tooling/documentation surface in the codebase at all.

## Goal

Remove every Terramate-Cloud-coupled package, command, config block, CI workflow, dependency,
and doc section from the repo, while keeping the CLI and code-generation functionality (HCL
parsing, globals, `generate`, stack management, orchestration/`run`, the LSP, the local TUI
dashboard) fully working and tested.

## Non-Goals

- Not touching anything unrelated to Terramate Cloud: Terraform/OpenTofu/Terragrunt support,
  code generation, HCL parsing/formatting, the language server, stack orchestration/scheduling,
  the mise.toml/pre-commit tooling added in the prior PR.
- Not removing `terramate ui` (the TUI dashboard) as a whole — only its Cloud-login gate.
- Not touching `benchmark.yml` — it's disabled due to paid Blacksmith CI runners, a separate,
  unrelated concern from the Terramate Cloud *product* coupling this spec addresses.
- Not attempting to preserve backward compatibility for anyone currently using
  `--sync-deployment`/`--sync-drift-status`/`--sync-preview`/`--target`/`terramate cloud *` — all
  of it is being removed, not deprecated.
- Not rewriting the `mise.toml`/Makefile task *names* beyond removing the Cloud-only ones — the
  rest of the task inventory from the prior PR stays as-is.

## Architecture

### Branch / PR strategy

New branch `feat/remove-terramate-cloud`, created from (stacked on top of)
`feat/oss-github-actions-pipelines` — not from `main`. The PR targets
`feat/oss-github-actions-pipelines` as its base, since that branch's PR (#1) is not yet merged.

### Removal categories

**1. Wholesale directory/file deletions** (~23,000 lines, ~165 files — no surgery, no callers
outside the deleted tree once cross-cutting references in category 2 are removed):

- `cloud/` — API client, typed API resources, PR/MR integration clients, fake-cloud test server
- `cloudsync/` — deployment/drift/preview sync logic, VCS/CI metadata detection
- `commands/cloud/` — `login`/`info`/`drift show` subcommands
- `ui/tui/cliauth/` — credential storage + all auth-provider flows (GitHub/GitLab/Google/SSO/OIDC)
- `ui/tui/clitest/messages.go` — Cloud-specific error kinds/messages (whole file)
- `test/cloud/` — cloud test fixture helper
- `e2etests/cloud/` (including `interop/`) — all Cloud e2e tests, including the `-tags interop`
  suite that hits a real staging API
- `commands/ui/view_cloud_login.go` — the TUI's cloud-login screen (single file inside the
  otherwise-generic `commands/ui/` package)

**2. Surgical removal from shared/generic files** (the real engineering work — each of these
files has non-Cloud responsibilities and must keep working correctly after Cloud code is
excised):

| File | What's removed |
|---|---|
| `ui/tui/cli_spec.go` | `cloudFilterFlags`/`cloudTargetFlags`/`cloudSyncFlags` structs and their embeds in `run`/`script run`/`list`/`trigger`; the deprecated `Cloud` sub-struct and `experimental cloud login` alias |
| `ui/tui/cli_handler.go` | `"cloud login"`/`"cloud info"`/`"cloud drift show"` dispatch cases; `cloudStatus`/`expStatus` filter-building in `list`/`trigger` handling |
| `engine/engine.go` | `ListStacks`'s `target`/`resources.StatusFilters` parameters (simplify signature and all callers); `CheckTargetsConfiguration` and the `terramate.config.cloud.targets.enabled` gate; the `cloud CloudState` field on `Engine` |
| `engine/cloud.go` | entire file deleted (all `Engine.Cloud*`/`SelectCloudStackTasks`/`Is*Task` methods) |
| `engine/run.go` | `CloudTarget`/`CloudFromTarget`/`CloudSync*`/`CloudPlanFile*` fields from `StackRunTask`; the `isCloudSync` output-buffering branch in `Run()` collapses to the single remaining path; `LogSyncer`/`LogSyncCondition` if they turn out to be Cloud-only (confirm during implementation) |
| `commands/run/cloud.go` | entire file deleted |
| `commands/run/run.go` | `SyncDeployment`/`SyncDriftStatus`/`SyncPreview` fields and their flag-conflict validation |
| `commands/script/run/run.go` | same pattern as `commands/run/run.go`, script-command-scoped |
| `config/script.go` | `CloudSyncDeployment`/`DriftStatus`/`Preview`, `CloudSyncLayer`, `CloudTerraformPlanFile`/`CloudTofuPlanFile`, `CloudPlanRenderTimeout` fields; `cloud_sync_*` HCL attribute parsing |
| `config/config.go` | `IsTargetsEnabled()` |
| `hcl/hcl.go` | `CloudConfig`/`TargetsConfig` structs, `"cloud"` entry in `ValidateSubBlocks`, `parseCloudConfig`/`parseTargetsConfig` functions — the entire `terramate.config.cloud { }` block stops being valid HCL |
| `http/http.go` | its `cloud/api/resources` error-type dependency |
| `commands/ui/model.go` | `ViewCloudLogin` as initial view state; `cloudLoginButtonIdx`/`cloudLoginLoading`/`cloudSignupMsg` fields — dashboard now opens directly to the local overview |
| `commands/trigger/*`, `commands/stack/{create,list}`, `commands/script/info/info.go`, `commands/debug/show/*/*.go` | drop now-unnecessary `cloud/api/{resources,stack}` imports, a side effect of simplifying `engine.ListStacks`'s signature |
| `ui/tui/telemetry/` | the dead `DetectAuthTypeFromEnv` helper that read `cliauth`'s credentials file — telemetry itself (anonymous usage analytics, unrelated product) stays |

**3. Test adjustments** (beyond the wholesale `e2etests/cloud/` deletion):

- `e2etests/core/exp_trigger_test.go` — remove/rewrite the assertion that
  `--status` conflicts with `--recursive` (that cloud-filter flag no longer exists)
- `e2etests/core/run_test.go` — `TestRunIOBuffering`'s "non-cloud, non-parallel is unbuffered"
  subtest no longer names a meaningful distinction once the cloud branch is gone; rename/simplify
- `e2etests/internal/runner/runner.go` — remove the fake-JWT/`credentials.tmrc.json`-writing
  cloud-auth test helper (shared infra file used by all e2e tests — only this one helper goes)
- `config/script_test.go` — remove `cloud_sync_*` HCL attribute parsing test cases

**4. CI/tooling:**

- Delete outright (not merely left disabled, since there's nothing left to ever re-enable once
  Cloud is gone): `ci-sync-preview.yml`, `ci-sync-deployment.yml`, the old Pro `release.yml`
  (already superseded by `oss-release.yml` from the prior PR), `interop-tests.yml`
- Leave `benchmark.yml` untouched (disabled for an unrelated reason — paid runners, not Cloud)
- `Makefile` / `makefiles/common.mk` / `makefiles/unix.mk` / `makefiles/windows.mk`: remove
  `cloud/sync/ok`, `cloud/sync/failed`, `test/testserver`, `test/sync`, `test/interop`,
  `test/fakecloud` targets; adjust `test/build`'s dependency (currently on
  `test/testserver`/`test/fakecloud`) to whatever `test/build` actually needs once those are gone
  — confirm during implementation whether `test/build`'s "telemetry sent to localhost" build tag
  still needs a local server binary for any surviving (non-cloud) e2e test, or whether the
  dependency can simply be dropped
- `mise.toml`: mirror the same Makefile task removals (`test:testserver`, `test:sync`,
  `test:interop`, `cloud:sync:ok`, `cloud:sync:failed`) and the same `test:build` dependency
  adjustment
- `bitbucket-pipelines.yml`: strip `--sync-preview`/`--sync-deployment`/`--terraform-plan-file`
  flags from both the PR and `main` pipelines, replacing with plain `terraform plan`/`apply` so
  the pipeline still exercises something real without depending on Cloud

**5. `go.mod` cleanup:**

Run `go mod tidy` after the code removal (categories 1-3) to drop dependencies that exist only to
support Cloud: `github.com/google/go-github` (+indirect), `github.com/shurcooL/githubv4`,
`github.com/cli/go-gh/v2`, `github.com/cli/safeexec`, `github.com/pkg/browser`,
`golang.org/x/oauth2`, `github.com/julienschmidt/httprouter`,
`github.com/terramate-io/tfjson`(+`sanitize`). Two dependencies need a closer check before
removal since they also touch shared test infra: `github.com/golang-jwt/jwt/v5` (used by
`cliauth`, being deleted, but also referenced by `e2etests/internal/runner`'s cloud-auth helper,
also being deleted in category 3 — confirm nothing else in the surviving test infra needs it) and
`github.com/hashicorp/go-uuid` (used by `cloudsync`/`commands/run/cloud.go`/
`commands/script/run/run.go` for deployment run-UUIDs — confirm no surviving non-cloud code path
also needs run-UUID generation before removing).

**6. Documentation:**

- `CLAUDE.md` — remove the "Terramate Cloud integration" bullet from Project Overview, the
  `cloud/` entry from Core Packages, and the cloud-sync mention from the `engine/` description
- `AGENTS.md` — remove the `/cloud/` repo-structure bullet
- `README.md` — remove the `terramate cloud login` quickstart section, the "Terramate CLI vs
  Terramate Cloud" section and its platform-overview images
  (`terramate_platform_overview_{dark,light}.png`), and Cloud-only feature bullets (Drift
  Management, Observability, Misconfiguration Detection, Asset Management, Audit Trail, Slack
  Integrations) — keep genuine CLI/codegen feature bullets (e.g. Scaffolding)

## Data Flow

```
feat/oss-github-actions-pipelines (PR #1, open, not yet merged)
        └─► feat/remove-terramate-cloud (new branch, stacked)
                 ├─ delete wholesale Cloud packages/dirs (category 1)
                 ├─ surgically remove Cloud code from shared files (category 2)
                 ├─ adjust/remove Cloud-touching tests (category 3)
                 ├─ delete dead Cloud-only CI workflows, trim Makefile/mise.toml/
                 │  bitbucket-pipelines.yml (category 4)
                 ├─ go mod tidy (category 5)
                 ├─ update docs (category 6)
                 └─► PR opened targeting feat/oss-github-actions-pipelines as base
```

## Error Handling

- Every surgical-removal file (category 2) must leave the surrounding non-Cloud logic working
  identically to today — the implementation plan's tasks each need a verification step that
  proves the touched command/package still does its non-Cloud job correctly (e.g. `run`/`script
  run`/`list`/`trigger` still work with no Cloud flags at all).
- `go build ./...` and the full non-Cloud test suite (`go test -race ./...`, now with
  `e2etests/cloud/` gone) must pass after every category, not just at the very end — this is a
  large enough change that catching a broken shared file early (e.g. `engine.ListStacks`'s
  signature change breaking a caller) is much cheaper than finding it at the end.
- The two `go.mod` dependencies needing a closer check (`golang-jwt/jwt/v5`,
  `hashicorp/go-uuid`) must be verified unused by `grep` across the whole repo (not just the
  deleted directories) before `go mod tidy` removes them, in case something not yet identified in
  this spec depends on them.

## What Is NOT Changed

- `Makefile`/`makefiles/*.mk` structure itself (still exists, still used for the surviving
  non-Cloud targets) — only Cloud-specific targets are removed from it.
- `mise.toml`/`.pre-commit-config.yaml` structure from the prior PR — only Cloud-specific tasks
  are removed.
- `oss-pr.yml`/`oss-main.yml`/`oss-release.yml`/`oss-build-test.yml` — already Cloud-free,
  untouched.
- `benchmark.yml`, `.gitlab-ci.yml`, `ci-experimental.yml` — untouched (verified Cloud-free or
  out of scope per Non-Goals).
- `di/di.go` and the `stack/` package proper — verified during exploration to have zero Cloud
  coupling, nothing to remove.
- Anything Terraform/OpenTofu/Terragrunt-related in `go.mod` (the large indirect dependency list
  for `tgdeps`) — unrelated to Cloud, stays.

## Open Questions Resolved During Brainstorming

- **Branch strategy**: stacked on `feat/oss-github-actions-pipelines` (not yet merged), not on
  `main` — confirmed explicitly.
- **`terramate ui` dashboard**: kept, only its Cloud-login gate is removed — the rest of the
  dashboard has zero Cloud coupling per exploration, confirmed by the user as the reasonable
  choice over deleting the whole TUI.
- **Already-disabled Cloud-coupled workflows** (`ci-sync-preview.yml`, `ci-sync-deployment.yml`,
  old `release.yml`, `interop-tests.yml`): deleted outright rather than left disabled, since full
  Cloud removal means there's nothing left to ever re-enable them for.
- **`benchmark.yml`**: explicitly out of scope — its disabled state is about paid Blacksmith
  runners, a separate axis from Terramate Cloud product coupling.
- **`bitbucket-pipelines.yml`**: rewritten to plain `terraform plan`/`apply` rather than deleted,
  so the repo's own live CI pipeline still exercises something real.
- **PR count**: one PR for this entire removal (not split into multiple), executed as many
  ordered tasks within a single implementation plan.
