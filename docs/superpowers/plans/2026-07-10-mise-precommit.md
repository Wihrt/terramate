# mise.toml + pre-commit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `mise.toml` as a tool-version-pinned task runner mirroring the existing Makefile (kept, not replaced), and a `.pre-commit-config.yaml` giving a fast fmt/vet/build/lint feedback loop at commit time, then migrate the one active CI consumer that should share versions with local dev (`oss-build-test.yml`) onto mise.

**Architecture:** `mise.toml` lives at the repo root alongside the untouched `Makefile`/`makefiles/`. Every `[tasks]` entry mirrors a `makefiles/unix.mk`/`common.mk` target 1:1 (same flags, same output paths). `.pre-commit-config.yaml` wraps `mise run`/`mise exec` calls as local hooks so there's a single source of truth for tool versions. Only `oss-build-test.yml` is edited to install tools via `jdx/mise-action` and call `mise run` instead of `make`.

**Tech Stack:** mise (tool/task manager), pre-commit (pre-commit.com), Go 1.25.8, golangci-lint v2, GoReleaser OSS, GitHub Actions.

## Global Constraints

- Do NOT delete or modify `Makefile` / `makefiles/*.mk`. They stay exactly as-is.
- Do NOT modify `ci-experimental.yml`, `ci-sync-preview.yml`, `ci-sync-deployment.yml`, `benchmark.yml`, `interop-tests.yml`, `release.yml`, `.gitlab-ci.yml`, `containers/test/Dockerfile`. Only `oss-build-test.yml` changes.
- All mise tasks are inline `[tasks."name"]` blocks in `mise.toml` — no `mise-tasks/` files.
- mise tasks mirror Unix (`makefiles/unix.mk`) behavior only — no Windows branching in `mise.toml`.
- Every new/modified file needs the MPL-2.0 header where the file type supports comments:
  ```
  # Copyright 2026 Terramate GmbH
  # SPDX-License-Identifier: MPL-2.0
  ```
- Preserve exact existing flags/behavior for `release`/`release:dry-run` (including the deprecated `--rm-dist` GoReleaser flag) — do not "fix" it as part of this work.
- `bench:check`'s argument style changes from Make's `key=value` to environment variables (e.g. `PKG=./foo mise run bench:check`) — this is an intentional, approved behavior change, not a bug.
- Third-party GitHub Actions must be SHA-pinned with a `# pin@vX` trailing comment, matching the repo's existing convention.

---

### Task 1: `mise.toml` — tools + build/install tasks

**Files:**
- Create: `mise.toml` (repo root)

**Interfaces:**
- Produces: `mise run build`, `mise run build:terramate`, `mise run build:terramate-ls`, `mise run build:pprof`, `mise run build:pprof:terramate`, `mise run build:tgdeps`, `mise run install`, `mise run install:terramate`, `mise run install:terramate-ls`, `mise run install:pprof`, `mise run install:pprof:terramate`, `mise run install:tgdeps` — all later tasks that need built binaries (Task 3) depend on `build`.

- [ ] **Step 1: Write `mise.toml` with `[tools]` and build/install tasks**

```toml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

[tools]
go = "1.25.8"
golangci-lint = "2.6.1"
goreleaser = "2.17.0"
node = "22"
pre-commit = "4.6.0"
"go:github.com/google/addlicense" = "1.0.0"

[tasks.build]
description = "Build terramate + terramate-ls"
depends = ["build:terramate", "build:terramate-ls"]

[tasks."build:terramate"]
description = "Build the terramate binary"
env = { CGO_ENABLED = "0" }
run = "go build --ldflags '-extldflags \"-static\"' -o bin/terramate ./cmd/terramate"

[tasks."build:terramate-ls"]
description = "Build the terramate-ls binary"
env = { CGO_ENABLED = "0" }
run = "go build --ldflags '-extldflags \"-static\"' -o bin/terramate-ls ./cmd/terramate-ls"

[tasks."build:pprof"]
description = "Build terramate with profiler"
depends = ["build:pprof:terramate"]

[tasks."build:pprof:terramate"]
description = "Build the terramate binary with profiler"
env = { CGO_ENABLED = "0" }
run = "go build --ldflags '-extldflags \"-static\"' -tags profiler -o bin/terramate ./cmd/terramate"

[tasks."build:tgdeps"]
description = "Build tgdeps"
env = { CGO_ENABLED = "0" }
run = "go build --ldflags '-extldflags \"-static\"' -o bin/tgdeps ./cmd/tgdeps"

[tasks.install]
description = "Install terramate + terramate-ls on the host"
depends = ["install:terramate", "install:terramate-ls"]

[tasks."install:terramate"]
description = "Install the terramate binary"
env = { CGO_ENABLED = "0" }
run = "go install --ldflags '-extldflags \"-static\"' ./cmd/terramate"

[tasks."install:terramate-ls"]
description = "Install the terramate-ls binary"
env = { CGO_ENABLED = "0" }
run = "go install --ldflags '-extldflags \"-static\"' ./cmd/terramate-ls"

[tasks."install:pprof"]
description = "Install terramate with profiler"
depends = ["install:pprof:terramate"]

[tasks."install:pprof:terramate"]
description = "Install the terramate binary with profiler"
env = { CGO_ENABLED = "0" }
run = "go install --ldflags '-extldflags \"-static\"' -tags profiler ./cmd/terramate"

[tasks."install:tgdeps"]
description = "Install tgdeps"
env = { CGO_ENABLED = "0" }
run = "go install --ldflags '-extldflags \"-static\"' ./cmd/tgdeps"
```

- [ ] **Step 2: Install pinned tools**

Run: `mise install`
Expected: downloads/activates Go 1.25.8, golangci-lint 2.6.1, goreleaser 2.17.0, node 22, pre-commit 4.6.0, and addlicense 1.0.0 with no errors.

- [ ] **Step 3: Verify `mise run build` matches `make build`**

Run:
```bash
rm -rf bin/*
mise run build
./bin/terramate version
./bin/terramate-ls --help >/dev/null && echo "terramate-ls OK"
```
Expected: `terramate version` prints `0.17.2-dev` (the current `VERSION` file content), `terramate-ls OK` is printed, both binaries exist under `bin/`.

- [ ] **Step 4: Diff against the Makefile build for equivalence**

Run:
```bash
mv bin/terramate bin/terramate-mise
mv bin/terramate-ls bin/terramate-ls-mise
make build
cmp bin/terramate bin/terramate-mise && echo "terramate binaries identical"
cmp bin/terramate-ls bin/terramate-ls-mise && echo "terramate-ls binaries identical"
rm -f bin/terramate-mise bin/terramate-ls-mise
```
Expected: both `cmp` calls report identical (exit 0) — proves the mise task produces byte-identical output to the existing Make target.

- [ ] **Step 5: Commit**

```bash
git add mise.toml
git commit -m "feat: add mise.toml with pinned tools and build/install tasks"
```

---

### Task 2: `mise.toml` — dev-loop tasks (generate, fmt, lint, mod, license, pin-gha, coverage)

**Files:**
- Modify: `mise.toml`

**Interfaces:**
- Consumes: nothing from Task 1 at the TOML level (independent `[tasks]` entries), but `generate` requires `bin/terramate` to already exist (built in Task 1).
- Produces: `mise run generate`, `mise run fmt`, `mise run lint:install`, `mise run lint:all`, `mise run mod`, `mise run mod:check`, `mise run license`, `mise run license:check`, `mise run pin-gha`, `mise run coverage`, `mise run coverage:show` — Task 5's CI migration depends on `generate` existing with this exact name.

- [ ] **Step 1: Append dev-loop tasks to `mise.toml`**

```toml
[tasks.generate]
description = "Regenerate files from .tm templates"
run = "./bin/terramate generate"

[tasks.fmt]
description = "Format go code"
run = "go run golang.org/x/tools/cmd/goimports@v0.1.7 -w ."

[tasks."lint:install"]
description = "Install the linter"
run = "go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.1"

[tasks."lint:all"]
description = "Lint all code"
run = "golangci-lint run ./..."

[tasks.mod]
description = "Tidy up go modules"
run = "go mod tidy"

[tasks."mod:check"]
description = "Check go modules are tidy"
run = "./hack/mod-check"

[tasks.license]
description = "Add license headers to code"
run = "go run github.com/google/addlicense@v1.0.0 -l mpl -s=only -ignore 'docs/**' -ignore '.tmtriggers/**' -ignore '**/*.tf' -c \"Terramate GmbH\" ."

[tasks."license:check"]
description = "Check code is licensed properly"
run = "go run github.com/google/addlicense@v1.0.0 -l mpl -s=only -ignore 'docs/**' -ignore '.tmtriggers/**' -ignore '**/*.tf' --check ."

[tasks."pin-gha"]
description = "Pin github actions to SHAs"
run = "npm exec --yes pin-github-action@3.5.1 -- ./.github/workflows/*.yml"

[tasks.coverage]
description = "Generate coverage report"
run = "go test -count=1 -coverprofile=coverage.txt -coverpkg=./... ./..."

[tasks."coverage:show"]
description = "Generate coverage report and open it in the browser"
depends = ["coverage"]
run = "go tool cover -html=coverage.txt"
```

- [ ] **Step 2: Verify `generate` matches `make generate`**

Run:
```bash
mise run generate
git status --porcelain
```
Expected: no output from `git status --porcelain` (repo already up to date, matches the existing `make generate && git diff --exit-code` CI check).

- [ ] **Step 3: Verify `fmt` is a no-op on already-formatted code**

Run:
```bash
mise run fmt
git status --porcelain
```
Expected: no output — repo code is already goimports-clean, so this task should not modify anything.

- [ ] **Step 4: Verify `lint:all` runs cleanly**

Run: `mise run lint:all`
Expected: exits 0 with no lint findings (same result as `make lint/all` on this branch).

- [ ] **Step 5: Verify `mod:check` and `license:check` pass**

Run:
```bash
mise run mod:check
mise run license:check
```
Expected: both exit 0 (matches current `make mod/check` / `make license/check` state on this branch).

- [ ] **Step 6: Commit**

```bash
git add mise.toml
git commit -m "feat: add mise dev-loop tasks (generate, fmt, lint, mod, license, pin-gha, coverage)"
```

---

### Task 3: `mise.toml` — test, bench, release, and misc tasks

**Files:**
- Modify: `mise.toml`

**Interfaces:**
- Consumes: `build` task from Task 1 (`test` and `cloud:sync:*` depend on it).
- Produces: `mise run test`, `mise run test:build`, `mise run test:helper`, `mise run test:testserver`, `mise run test:sync`, `mise run test:interop`, `mise run test:docker`, `mise run test:fuzz:eval`, `mise run test:fuzz:fmt`, `mise run test:fuzz:tokens-for-expr`, `mise run bench`, `mise run bench:all`, `mise run bench:check`, `mise run bench:cleanup`, `mise run release:dry-run`, `mise run release`, `mise run cloud:sync:ok`, `mise run cloud:sync:failed`, `mise run graph2png`, `mise run release:tag`, `mise run clean` — Task 5's CI migration depends on `test` existing with this exact name and behavior.

- [ ] **Step 1: Append test/bench/release/misc tasks to `mise.toml`**

```toml
[tasks."test:testserver"]
description = "Build bin/testserver"
run = "go build -o bin/testserver ./cloud/testserver/cmd/testserver"

[tasks."test:helper"]
description = "Build the helper binary"
run = "go build -o bin/helper ./e2etests/cmd/helper"

[tasks."test:build"]
description = "Build a test binary -- not static, telemetry sent to localhost, etc"
depends = ["test:testserver"]
run = "go build -tags localhostEndpoints -o bin/test-terramate ./cmd/terramate"

[tasks.test]
description = "Run the test suite"
depends = ["test:helper", "build"]
run = '''
tempdir=$(./bin/helper tempdir)
TM_TEST_ROOT_TEMPDIR=$tempdir ./bin/terramate run --no-recursive -- go test -race -count=1 -timeout 30m ./... || ./bin/helper rm $tempdir
'''

[tasks."test:sync"]
description = "Run the cloud preview sync test"
depends = ["test:helper", "build"]
run = '''
tempdir=$(./bin/helper tempdir)
TMC_API_HOST=api.stg.terramate.io \
TM_TEST_ROOT_TEMPDIR=$tempdir \
TM_CLOUD_ORGANIZATION=test \
GITHUB_TOKEN=$(cat ../my_github_token.txt) \
NO_COLOR=1 \
CI=1 \
./bin/terramate script run --tags golang --parallel=10 preview || ./bin/helper rm $tempdir
'''

[tasks."test:interop"]
description = "Run interop tests"
run = "TM_CLOUD_ORGANIZATION=${ORG:-test} TMC_API_HOST=${BACKEND_HOST:-api.stg.terramate.io} go test -v -count=1 -tags interop ./e2etests/cloud/interop/..."

[tasks."test:docker"]
description = "Run tests within docker"
run = "docker build --progress=plain --rm -f containers/test/Dockerfile ."

[tasks."test:fuzz:eval"]
description = "Fuzz partial eval"
run = "go test ./hcl/eval -fuzz=FuzzPartialEval"

[tasks."test:fuzz:fmt"]
description = "Fuzz formatting"
run = "go test ./hcl -fuzz=FuzzFormatMultiline"

[tasks."test:fuzz:tokens-for-expr"]
description = "Fuzz ast.TokensForExpression"
run = "go test ./hcl/ast -fuzz=FuzzTokensForExpression"

[tasks.bench]
description = "Run all benchmarks on the given 'pkg', or a specific benchmark"
run = "go test -bench=${NAME:-.} -count=5 -benchtime=${TIME:-1s} -benchmem ${PKG:-.}"

[tasks."bench:all"]
description = "Benchmark all packages"
run = '''
for benchfile in $(find ${DIR:-.} | grep _bench_); do
  go test -bench=. -benchtime=${TIME:-1s} -benchmem $(dirname $benchfile)
done
'''

[tasks."bench:check"]
description = "Check benchmark regressions"
run = '''
go run github.com/madlambda/benchcheck/cmd/benchcheck@743137fbfd827958b25ab6b13fa1180e0e933eb1 \
  -mod ${NAME:-github.com/terramate-io/terramate} -pkg ${PKG:-./...} \
  -go-test-flags "-benchmem,-count=${COUNT:-20},-run=Bench,-parallel=${PARALLEL:-1}" \
  -old ${OLD:-main} -new ${NEW:-$(git rev-parse HEAD)} \
  -check allocs/op=${ALLOCDELTA:-+20%} \
  -check time/op=${TIMEDELTA:-+20%}
'''

[tasks."bench:cleanup"]
description = "Cleanup artifacts produced by benchmarking"
run = '''
rm -f *.prof
rm -f *.test
'''

[tasks."release:dry-run"]
description = "Dry run of the release process"
run = "goreleaser release --snapshot --rm-dist"

[tasks.release]
description = "Generate a terramate release"
run = "goreleaser release --rm-dist --key $GORELEASER_KEY"

[tasks."cloud:sync:ok"]
description = "Sync the Terramate example stack with a success status"
depends = ["build", "test:helper"]
run = '''
./bin/terramate --log-level=info \
  --disable-check-git-untracked \
  --disable-check-git-uncommitted \
  --tags test \
  run --sync-deployment -- \
  $(pwd)/bin/helper true
'''

[tasks."cloud:sync:failed"]
description = "Sync the Terramate example stack with a failed status"
depends = ["build", "test:helper"]
run = '''
./bin/terramate --log-level=info \
  --disable-check-git-untracked \
  --disable-check-git-uncommitted \
  --tags test \
  run --sync-deployment -- \
  $(pwd)/bin/helper false
'''

[tasks.graph2png]
description = "Render the stack run-graph to graph.png (requires graphviz)"
run = '''
./bin/terramate experimental run-graph | dot -Tpng > graph.png
echo "check the image: graph.png"
'''

[tasks."release:tag"]
description = "Create a new release tag"
run = '''
version=${VERSION:-v$(cat VERSION)}
git tag -s -a "$version" -m "Release $version"
git push origin "$version"
'''

[tasks.clean]
description = "Remove build artifacts"
run = "rm -rf bin/*"
```

- [ ] **Step 2: Verify `test:build` matches `make test/build`**

Run:
```bash
rm -f bin/test-terramate bin/testserver
mise run test:build
ls bin/test-terramate bin/testserver
```
Expected: both files exist under `bin/`.

- [ ] **Step 3: Verify `test` matches `make test`**

Run: `mise run test`
Expected: same pass/fail result as `make test` on this branch (full `go test -race` suite passes, matching current CI state on this branch).

- [ ] **Step 4: Verify `coverage`/fuzz/bench tasks at least start correctly (smoke check, not full run)**

Run:
```bash
timeout 5 mise run test:fuzz:eval || true
timeout 5 mise run bench PKG=./stack/... NAME=. TIME=1s || true
```
Expected: both commands start executing the underlying `go test` invocation (visible output like `fuzzing ...` or benchmark names) rather than erroring on unknown flags/tasks — confirms the env-var substitution and task wiring work. (`timeout` intentionally kills them early since a full fuzz/bench run isn't needed to prove wiring.)

- [ ] **Step 5: Commit**

```bash
git add mise.toml
git commit -m "feat: add mise test, bench, release, and misc tasks"
```

---

### Task 4: `.pre-commit-config.yaml`

**Files:**
- Create: `.pre-commit-config.yaml` (repo root)

**Interfaces:**
- Consumes: `mise run fmt` (Task 2), `mise run build` (Task 1) — both must already work.

- [ ] **Step 1: Write `.pre-commit-config.yaml`**

```yaml
# Copyright 2026 Terramate GmbH
# SPDX-License-Identifier: MPL-2.0

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

- [ ] **Step 2: Install the git hook**

Run: `mise exec -- pre-commit install`
Expected: `pre-commit installed at .git/hooks/pre-commit`

- [ ] **Step 3: Verify hooks pass on clean code**

Run: `mise exec -- pre-commit run --all-files`
Expected: all four hooks (`goimports`, `go vet`, `go build`, `golangci-lint`) report `Passed`.

- [ ] **Step 4: Verify the go-vet hook actually catches a violation**

Introduce a deliberate `go vet` violation:
```bash
cat >> stack/stack.go <<'EOF'

func brokenVetCheck() {
	var x fmt.Stringer
	fmt.Printf("%d", x)
}
EOF
```
Run: `mise exec -- pre-commit run go-vet --all-files`
Expected: hook reports `Failed`, output includes a `Printf` format-verb mismatch.

- [ ] **Step 5: Revert the deliberate violation**

Run: `git checkout -- stack/stack.go`
Expected: `git status --porcelain stack/stack.go` prints nothing.

- [ ] **Step 6: Verify the goimports hook actually catches a violation**

Introduce a deliberate formatting violation:
```bash
printf '\n\n\nfunc extraBlankLines() {}\n' >> stack/stack.go
```
Run: `mise exec -- pre-commit run go-fmt --all-files`
Expected: hook reports `Failed` (goimports rewrites the file in place, exiting non-zero because it modified a tracked file).

- [ ] **Step 7: Revert the deliberate violation**

Run: `git checkout -- stack/stack.go`
Expected: `git status --porcelain stack/stack.go` prints nothing.

- [ ] **Step 8: Commit**

```bash
git add .pre-commit-config.yaml
git commit -m "feat: add pre-commit config (fmt, vet, build, lint)"
```

---

### Task 5: Migrate `oss-build-test.yml` to mise

**Files:**
- Modify: `.github/workflows/oss-build-test.yml`

**Interfaces:**
- Consumes: `mise run build` (Task 1), `mise run generate` (Task 2), `mise run test` (Task 3) — all must already exist and work locally before this task runs.

- [ ] **Step 1: Replace `actions/setup-go` + `make` steps with `jdx/mise-action` + `mise run`**

Modify `.github/workflows/oss-build-test.yml` — replace lines 20-32:

```yaml
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

with:

```yaml
      - uses: jdx/mise-action@e6a8b3978addb5a52f2b4cd9d91eafa7f0ab959d # pin@v4.2.0
        with:
          install: true

      - name: mise run build
        run: mise run build

      - name: mise run generate (fail on drift)
        run: mise run generate && git diff --exit-code

      - name: mise run test
        run: mise run test
```

- [ ] **Step 2: Validate workflow syntax**

Run: `actionlint .github/workflows/oss-build-test.yml`
Expected: no new errors (same warning set as before the edit — pre-existing warnings unrelated to this change, if any, are unchanged).

- [ ] **Step 3: Confirm the SHA pin is correct**

Run: `git ls-remote --tags https://github.com/jdx/mise-action | grep v4.2.0`
Expected: `e6a8b3978addb5a52f2b4cd9d91eafa7f0ab959d	refs/tags/v4.2.0` — matches the SHA used in Step 1.

- [ ] **Step 4: Commit**

```bash
git add .github/workflows/oss-build-test.yml
git commit -m "feat: migrate oss-build-test.yml from make to mise"
```

- [ ] **Step 5: Push and verify on the real PR**

Run:
```bash
git push
gh pr checks --repo Wihrt/terramate --watch
```
Expected: `OSS / Pull Request` (which calls `oss-build-test.yml`) passes, confirming `jdx/mise-action` installs tools correctly and `mise run build`/`generate`/`test` succeed in the actual GitHub Actions environment, not just locally.

---

## Self-Review

**Spec coverage:** `[tools]` (Task 1) ✓, full `[tasks]` mapping table (Tasks 1-3, every row from the spec's table has a corresponding task) ✓, `.pre-commit-config.yaml` (Task 4) ✓, CI change to `oss-build-test.yml` only (Task 5) ✓, `release`/`release:dry-run` preserved verbatim including `--rm-dist` (Task 3) ✓, `bench:check` env-var argument style (Task 3) ✓, no Windows branching (all tasks are Unix-only, matching `makefiles/unix.mk`) ✓, Makefile/makefiles left untouched (no task modifies them) ✓, other 7 CI consumers left untouched (no task modifies them) ✓.

**Placeholder scan:** no TBD/TODO; every step has literal runnable commands or complete TOML/YAML content.

**Type/name consistency:** task names used across tasks match — `build` (Task 1) is referenced by `test` and `cloud:sync:*` (Task 3) and by `oss-build-test.yml` (Task 5); `test:helper` (Task 3) referenced by `test` and `cloud:sync:*` in the same task; `generate` (Task 2) referenced by Task 5's CI step — all names are identical strings throughout.
