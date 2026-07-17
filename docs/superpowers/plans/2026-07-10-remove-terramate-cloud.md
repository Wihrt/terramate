# Remove Terramate Cloud Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove every Terramate-Cloud-coupled package, command, config block, CI workflow, dependency, and doc section from the repo, keeping the CLI and code-generation functionality (including the local TUI dashboard) fully working and tested.

**Architecture:** Go forces whole-package compilation, so the removal proceeds in dependency order: CLI-facing commands and flags first (their removal only requires downstream fields to go unpopulated, not yet deleted), then the `engine` package (the hub every command calls into), then wholesale deletion of now-uncalled packages (`cloud/`, `cloudsync/`, `ui/tui/cliauth/`), then tests, then CI/tooling/docs. Each task ends with `go build ./...` passing; the full test suite runs at the end of the source-code tasks (Task 11) and again after CI/tooling changes (Task 12).

**Tech Stack:** Go 1.25.8, Kong (CLI flag parsing), Bubbletea (TUI), HCL v2 (fork).

## Global Constraints

- Do not touch anything unrelated to Terramate Cloud: Terraform/OpenTofu/Terragrunt support, code generation, HCL parsing/formatting (beyond the `cloud` block), the language server, stack orchestration/scheduling, `mise.toml`/`.pre-commit-config.yaml`.
- Do not remove `terramate ui` as a whole — only its Cloud-login gate. The rest of the TUI dashboard (`view_overview.go`, `change.go`, wizard flows, etc.) is untouched.
- Do not touch `benchmark.yml` — disabled for an unrelated reason (paid Blacksmith runners).
- Do not preserve backward compatibility for `--sync-*`/`--target`/`terramate cloud *` — remove outright, do not deprecate.
- Every removal must leave `go build ./...` passing before moving to the next task.
- MPL-2.0 header required on any new file:
  ```
  // Copyright 2026 Terramate GmbH
  // SPDX-License-Identifier: MPL-2.0
  ```

---

### Task 1: Extract the non-cloud output-buffering utility out of the `cloud` package

**Files:**
- Create: `engine/buffer_group.go`
- Test: `engine/buffer_group_test.go`

**Interfaces:**
- Produces: `engine.NewBufferGroup() *engine.BufferGroup`, `(*BufferGroup).NewBuffer(out io.Writer) io.Writer`, `(*BufferGroup).Wait()` — Task 8 (engine/run.go) depends on these existing.

**Context:** `cloud/log_syncer.go` defines `BufferGroup`, used by `engine/run.go` to interleave parallel stacks' output (`if isCloudSync || opts.Parallel > 1`). This buffering has nothing to do with Terramate Cloud — it's needed purely because concurrent processes writing directly to the same stdout/stderr would interleave mid-line. Since `cloud/` is being deleted wholesale later (Task 10), this utility must move to a package that survives. The original also accepts an optional `*LogSyncer` for streaming lines to Terramate Cloud — that part is dropped entirely since cloud sync is being removed.

- [ ] **Step 1: Write the extracted, simplified `BufferGroup`**

```go
// engine/buffer_group.go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package engine

import (
	"bytes"
	"io"
	"sync"

	"github.com/rs/zerolog/log"
	"github.com/terramate-io/terramate/errors"
)

// BufferGroup manages a group of synchronized buffers so that concurrent
// writers (e.g. parallel stack runs) don't interleave mid-line.
type BufferGroup struct {
	fds []io.Closer
	wg  sync.WaitGroup
}

// NewBufferGroup creates a new buffer group.
func NewBufferGroup() *BufferGroup {
	return &BufferGroup{}
}

// Wait waits for the processing of all output for the buffers within this group.
// After calling this method, it's not safe to call any other method, as it
// closes the internal channels and shuts down all goroutines.
func (s *BufferGroup) Wait() {
	for _, writerFD := range s.fds {
		// only returns an error when readerFD.CloseWithError(err) is called,
		// but this is not the case.
		_ = writerFD.Close()
	}
	s.wg.Wait()
}

// NewBuffer creates a new synchronized, line-buffered writer.
func (s *BufferGroup) NewBuffer(out io.Writer) io.Writer {
	r, w := io.Pipe()
	s.fds = append(s.fds, w)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		var pending []byte
		errs := errors.L()
		for {
			lines, rest, readErr := readLines(r, pending)
			if readErr != nil && readErr != io.EOF {
				errs.Append(readErr)
				break
			}
			if readErr == io.EOF && len(rest) > 0 {
				lines = [][]byte{rest}
			}
			for _, line := range lines {
				_, err := out.Write(line)
				if err != nil {
					errs.Append(errors.E(err, "writing to terminal"))
				}
			}
			if readErr == io.EOF {
				break
			}
			pending = rest
		}

		errs.Append(r.Close())
		errs.Append(w.Close())
		if err := errs.AsError(); err != nil {
			log.Error().Err(err).Msg("synchronizing command output")
		}
	}()
	return w
}

func readLines(r io.Reader, pending []byte) (line [][]byte, rest []byte, err error) {
	const readSize = 1024

	var buf [readSize]byte
	rest = pending
	for {
		n, err := r.Read(buf[:])
		if n > 0 {
			rest = append(rest, buf[:n]...)
			var lines [][]byte

			var nlpos int
			for nlpos != -1 {
				nlpos = bytes.IndexByte(rest, '\n')
				if nlpos >= 0 {
					lines = append(lines, rest[:nlpos+1]) // line includes ln
					rest = rest[nlpos+1:]
				}
			}
			if len(lines) > 0 {
				return lines, rest, err
			}
		} else if err == nil {
			// misbehaving reader
			return nil, nil, io.EOF
		}
		if err != nil {
			return nil, rest, err
		}

		// line ending not found, continue reading.
	}
}
```

- [ ] **Step 2: Write a test proving it buffers and interleaves correctly**

```go
// engine/buffer_group_test.go
// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package engine

import (
	"bytes"
	"sync"
	"testing"
)

func TestBufferGroupWritesCompleteLines(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	var mu sync.Mutex
	safeOut := &lockedWriter{w: &out, mu: &mu}

	bg := NewBufferGroup()
	w1 := bg.NewBuffer(safeOut)
	w2 := bg.NewBuffer(safeOut)

	_, err := w1.Write([]byte("hello from w1\n"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = w2.Write([]byte("hello from w2\n"))
	if err != nil {
		t.Fatal(err)
	}

	bg.Wait()

	got := out.String()
	if got != "hello from w1\nhello from w2\n" && got != "hello from w2\nhello from w1\n" {
		t.Fatalf("expected two complete, non-interleaved lines, got: %q", got)
	}
}

type lockedWriter struct {
	w  *bytes.Buffer
	mu *sync.Mutex
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
```

- [ ] **Step 3: Run the test**

Run: `mise exec -- go test -race -count=1 ./engine/... -run TestBufferGroupWritesCompleteLines -v`
Expected: `--- PASS: TestBufferGroupWritesCompleteLines` and `ok`.

- [ ] **Step 4: Verify the whole repo still builds (this is purely additive so far)**

Run: `mise run build`
Expected: exits 0, `bin/terramate` and `bin/terramate-ls` produced.

- [ ] **Step 5: Commit**

```bash
git add engine/buffer_group.go engine/buffer_group_test.go
git commit -m "feat: extract non-cloud output buffering from cloud package into engine"
```

---

### Task 2: Remove `terramate cloud login/info/drift show` commands

**Files:**
- Delete: `commands/cloud/` (recursively — `login/{github,google,sso}.go`, `info/info.go`, `drift/show/show.go`)
- Modify: `ui/tui/cli_spec.go`
- Modify: `ui/tui/cli_handler.go`

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces: nothing consumed by later tasks — this is a leaf removal (the `cloud login/info/drift show` command tree has no other callers in the repo).

- [ ] **Step 1: Delete the command package**

```bash
git rm -r commands/cloud
```

- [ ] **Step 2: Remove the two Kong command structs from `ui/tui/cli_spec.go`**

Remove the top-level `Cloud` struct (`ui/tui/cli_spec.go:99-111`):
```go
	Cloud struct {
		Login struct {
			Google bool `optional:"true" help:"authenticate with google credentials"`
			Github bool `optional:"true" help:"authenticate with github credentials"`
			SSO    bool `optional:"true" help:"authenticate with SSO credentials"`
		} `cmd:"" help:"Sign in to Terramate Cloud."`
		Info  struct{} `cmd:"" help:"Show your current Terramate Cloud login status."`
		Drift struct {
			Show struct {
				Target string `help:"Show stacks from the given deployment target."`
			} `cmd:"" help:"Show the current drift of a stack."`
		} `cmd:"" help:"Interact with Terramate Cloud Drift Detection."`
	} `cmd:"" help:"Interact with Terramate Cloud"`
```

Remove the deprecated `Experimental.Cloud` struct (`ui/tui/cli_spec.go:193-200`):
```go
		Cloud struct {
			Login struct{} `cmd:"" help:"login for cloud.terramate.io  (DEPRECATED)"`
			Info  struct{} `cmd:"" help:"cloud information status (DEPRECATED)"`
			Drift struct {
				Show struct {
				} `cmd:"" help:"show drifts  (DEPRECATED)"`
			} `cmd:"" help:"manage cloud drifts  (DEPRECATED)"`
		} `cmd:"" hidden:"" help:"Terramate Cloud commands (DEPRECATED)"`
```

- [ ] **Step 3: Remove the three dispatch cases and their imports from `ui/tui/cli_handler.go`**

Remove the import lines:
```go
	clouddriftshowcmd "github.com/terramate-io/terramate/commands/cloud/drift/show"
	cloudinfocmd "github.com/terramate-io/terramate/commands/cloud/info"
	logincmd "github.com/terramate-io/terramate/commands/cloud/login"
```

Remove the `cloud login` / `experimental cloud login` case:
```go
	case "experimental cloud login": // Deprecated: use cloud login
		fallthrough
	case "cloud login":
		if parsedArgs.Cloud.Login.Github {
			return &logincmd.GithubSpec{
				Verbosity: parsedArgs.Verbose,
			}, nil
		} else if parsedArgs.Cloud.Login.SSO {
			return &logincmd.SSOSpec{
				Verbosity: parsedArgs.Verbose,
			}, nil
		}
		return &logincmd.GoogleSpec{
			Verbosity: parsedArgs.Verbose,
		}, nil
```

Remove the `cloud info` case:
```go
	case "cloud info":
		c.SetCommandAnalytics("cloud-info")
		return &cloudinfocmd.Spec{
			Verbosity: parsedArgs.Verbose,
		}, nil
```

Remove the `cloud drift show` case:
```go
	case "cloud drift show":
		c.SetCommandAnalytics("cloud-drift-show")
		return &clouddriftshowcmd.Spec{
			Verbosiness: parsedArgs.Verbose,
			Target:      parsedArgs.Cloud.Drift.Show.Target,
		}, nil
```

- [ ] **Step 4: Verify the repo builds**

Run: `mise run build`
Expected: exits 0.

- [ ] **Step 5: Verify the commands are actually gone**

Run: `./bin/terramate cloud login 2>&1; echo "exit: $?"`
Expected: Kong reports an unknown command (non-zero exit), not a working login flow.

Run: `./bin/terramate --help 2>&1 | grep -i cloud`
Expected: no output (no `cloud` command listed).

- [ ] **Step 6: Commit**

```bash
git add -A -- commands/cloud ui/tui/cli_spec.go ui/tui/cli_handler.go
git commit -m "feat: remove terramate cloud login/info/drift show commands"
```

---

### Task 3: Remove the Cloud-login screen from the TUI dashboard

**Files:**
- Delete: `commands/ui/view_cloud_login.go`
- Modify: `commands/ui/model.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `terramate ui` now always opens directly to `ViewOverview`.

- [ ] **Step 1: Delete the login-screen file**

```bash
git rm commands/ui/view_cloud_login.go
```

- [ ] **Step 2: Remove the three cloud fields from `Model`**

Remove from the `Model` struct (`commands/ui/model.go`):
```go
	// Cloud login state

	cloudLoginButtonIdx int
	cloudLoginLoading   bool
	cloudSignupMsg      string
```

- [ ] **Step 3: Simplify the view-state enum — keep the iota slot reserved (matches the existing `ViewEnvSelect` "unused, kept to preserve iota values" pattern already in this file), don't renumber**

Change:
```go
const (
	ViewCloudLogin      ViewState = iota // Cloud login prompt (shown first)
	ViewEnvSelect                        // Unused, kept to preserve iota values
	ViewOverview                         // Main overview
```
to:
```go
const (
	_removedCloudLogin  ViewState = iota // Unused, kept to preserve iota values (was cloud login prompt)
	ViewEnvSelect                        // Unused, kept to preserve iota values
	ViewOverview                         // Main overview (now the initial view)
```

- [ ] **Step 4: Simplify `NewModel` to always start at the overview**

Change:
```go
func NewModel(est *EngineState) Model {
	var initialViewState ViewState

	if _, err := os.Stat(cliauth.CredentialFile(est.CLIConfig)); err != nil && shouldAskForLogin(est.CLIConfig) {
		initialViewState = ViewCloudLogin
	} else {
		initialViewState = ViewOverview
	}

	return Model{
		EngineState: est,
		viewState:   initialViewState,
		commands: []string{
			"Scaffold",
			"Reconfigure",
			"Promote",
			"Quit",
		},
		focus: FocusCommands,
	}
}
```
to:
```go
func NewModel(est *EngineState) Model {
	return Model{
		EngineState: est,
		viewState:   ViewOverview,
		commands: []string{
			"Scaffold",
			"Reconfigure",
			"Promote",
			"Quit",
		},
		focus: FocusCommands,
	}
}
```

- [ ] **Step 5: Remove the `cloudLoginResultMsg` case from `Update()`**

Remove:
```go
	case cloudLoginResultMsg:
		m.cloudLoginLoading = false
		if msg.err != nil {
			m.currentErr = msg.err
			return m, nil
		}
		m.viewState = ViewOverview
		return m, textarea.Blink
```

- [ ] **Step 6: Remove the `ViewCloudLogin` case from the keyboard-input switch in `Update()`**

Remove:
```go
		case ViewCloudLogin:
			return m.updateCloudLogin(msg)
```

- [ ] **Step 7: Remove the `ViewCloudLogin` case from `View()`**

Remove:
```go
	case ViewCloudLogin:
		base = m.renderCloudLoginView()
```

- [ ] **Step 8: Remove `shouldAskForLogin`, `setLoginSkipped`, and `loginSkipFileData`**

Remove the full block:
```go
type loginSkipFileData struct {
	Timestamp uint64 `json:"timestamp"`
	Version   string `json:"version"`
}

func shouldAskForLogin(clicfg cliconfig.Config) bool {
	loginSkipFile := filepath.Join(clicfg.UserTerramateDir, "login_skip")
	data, err := os.ReadFile(loginSkipFile)
	if err != nil {
		return true
	}
	var skip loginSkipFileData
	if err := json.Unmarshal(data, &skip); err != nil {
		return true
	}
	if skip.Version != terramate.Version() {
		return true
	}
	skippedAt := time.Unix(int64(skip.Timestamp), 0)
	return time.Since(skippedAt) > 7*24*time.Hour
}

func setLoginSkipped(clicfg cliconfig.Config) error {
	skip := loginSkipFileData{
		Timestamp: uint64(time.Now().Unix()),
		Version:   terramate.Version(),
	}
	data, err := json.Marshal(skip)
	if err != nil {
		return errors.E(err, "marshaling login skip data")
	}
	if err := os.MkdirAll(clicfg.UserTerramateDir, 0o700); err != nil {
		return errors.E(err, "creating user terramate dir")
	}
	loginSkipFile := filepath.Join(clicfg.UserTerramateDir, "login_skip")
	return os.WriteFile(loginSkipFile, data, 0o600)
}
```

- [ ] **Step 9: Remove now-unused imports from `model.go`**

Remove the `cliauth` and `cliconfig` imports (double-check `terramate`, `os`, `json`, `time`, `filepath` aren't used elsewhere in the file before removing them — likely still needed by other, non-cloud parts of `model.go`; only remove an import if `goimports`/the compiler confirms it's now unused).

- [ ] **Step 10: Verify the repo builds**

Run: `mise run build`
Expected: exits 0.

- [ ] **Step 11: Run goimports to clean up any now-unused imports precisely**

Run: `mise run fmt`
Expected: exits 0; `git diff commands/ui/model.go` shows only import-list changes beyond what you already made by hand.

- [ ] **Step 12: Commit**

```bash
git add -A -- commands/ui/view_cloud_login.go commands/ui/model.go
git commit -m "feat: remove cloud-login screen from TUI dashboard"
```

---

### Task 4: Remove Cloud CLI flags from run/script run/list/trigger/debug show

**Files:**
- Modify: `ui/tui/cli_spec.go`
- Modify: `ui/tui/cli_handler.go`

**Interfaces:**
- Consumes: nothing from Tasks 1-3.
- Produces: `parsedArgs.Run`/`parsedArgs.Script.Run`/`parsedArgs.List`/`parsedArgs.Trigger`/`parsedArgs.Debug.Show.*` no longer have `Status`/`DriftStatus`/`DeploymentStatus`/`Target`/`FromTarget`/`SyncDeployment`/`SyncDriftStatus`/`SyncPreview`/`Layer`/`TerraformPlanFile`/`TofuPlanFile`/`PlanRenderTimeout`/`DebugPreviewURL` fields. Task 5/6/7's Spec-struct field removals depend on these flags no longer existing (so nothing constructs a Spec with a removed field from a flag that no longer exists).

- [ ] **Step 1: Delete the three flag structs from `ui/tui/cli_spec.go`**

Delete:
```go
type cloudFilterFlags struct {
	ExperimentalStatus string `hidden:"" help:"Filter by status (Deprecated)"`
	CloudStatus        string `hidden:""`
	Status             string `help:"Filter by Terramate Cloud status of the stack."`
	DeploymentStatus   string `help:"Filter by Terramate Cloud deployment status of the stack"`
	DriftStatus        string `help:"Filter by Terramate Cloud drift status of the stack"`
}
```
```go
type cloudTargetFlags struct {
	Target     string `env:"TARGET" help:"Set the deployment target for stacks synchronized to Terramate Cloud."`
	FromTarget string `env:"FROM_TARGET" help:"Migrate stacks from given deployment target."`
}
```
```go
type cloudSyncFlags struct {
	CloudSyncDeployment  bool `hidden:""`
	SyncDeployment       bool `env:"SYNC_DEPLOYMENT" default:"false" help:"Synchronize the command as a new deployment to Terramate Cloud."`
	CloudSyncDriftStatus bool `hidden:""`
	SyncDriftStatus      bool `env:"SYNC_DRIFT_STATUS" default:"false" help:"Synchronize the command as a new drift run to Terramate Cloud."`
	CloudSyncPreview     bool `hidden:""`
	SyncPreview          bool `env:"SYNC_PREVIEW" default:"false" help:"Synchronize the command as a new preview to Terramate Cloud."`

	CloudSyncLayer             preview.Layer `hidden:""`
	Layer                      preview.Layer `env:"LAYER" default:"" help:"Set a customer layer for synchronizing a preview to Terramate Cloud."`
	CloudSyncTerraformPlanFile string        `hidden:""`
	PlanRenderTimeout          int           `env:"PLAN_RENDER_TIMEOUT" default:"300" help:"Timeout (in seconds) for internal commands that render changes from plan files."`
}
```

Remove the now-unused `"github.com/terramate-io/terramate/cloud/api/preview"` import if `preview.Layer` is no longer referenced anywhere in this file.

- [ ] **Step 2: Remove the `cloudFilterFlags` embed from `List`, `Debug.Show.{Metadata,Globals,GenerateOrigins,RuntimeEnv}`, `Trigger`, and `Experimental.Trigger`**

In each of the following, delete the `cloudFilterFlags` line:
```go
	List struct {
		Why bool `help:"Shows the reason why the stack has changed."`

		cloudFilterFlags
		Target   string `help:"Select the deployment target of the filtered stacks."`
```
(also delete the `Target` field here — it's cloud-only, used only to scope the cloud status filter)
```go
	Debug struct {
		Show struct {
			Metadata struct {
				cloudFilterFlags
			} `cmd:"" help:"Show metadata available in stacks."`
			Globals struct {
				cloudFilterFlags
			} `cmd:"" help:"Show globals available in stacks."`
			GenerateOrigins struct {
				cloudFilterFlags
			} `cmd:"" help:"Show details about generated code in stacks."`
			RuntimeEnv struct {
				cloudFilterFlags
			} `cmd:"" help:"Show available run-time environment variables (ENV) in stacks."`
```
```go
	Trigger struct {
		Stack        string `arg:"" optional:"true" name:"stack" predictor:"file" help:"The stacks path."`
		Recursive    bool   `default:"false" help:"Recursively triggers all child stacks of the given path"`
		Change       bool   `default:"false" help:"Trigger stacks as changed"`
		IgnoreChange bool   `default:"false" help:"Trigger stacks to be ignored by change detection"`
		Reason       string `default:"" name:"reason" help:"Set a reason for triggering the stack."`
		cloudFilterFlags
```
```go
		Trigger struct {
			Stack        string `arg:"" optional:"true" name:"stack" predictor:"file" help:"The stacks path."`
			Recursive    bool   `default:"false" help:"Recursively triggers all child stacks of the given path"`
			Change       bool   `default:"false" help:"Trigger stacks as changed"`
			IgnoreChange bool   `default:"false" help:"Trigger stacks to be ignored by change detection"`
			Reason       string `default:"" name:"reason" help:"Set a reason for triggering the stack."`
			cloudFilterFlags
```

- [ ] **Step 3: Remove `cloudFilterFlags`, `cloudTargetFlags`, `cloudSyncFlags` from `runCommandFlags`**

Change:
```go
type runCommandFlags struct {
	cloudFilterFlags
	changeDetectionFlags
	cloudTargetFlags

	EnableSharing bool `env:"ENABLE_SHARING" help:"Enable sharing of stack outputs as stack inputs."`
	MockOnFail    bool `env:"MOCK_ON_FAIL" help:"Mock the output values if command fails."`

	cloudSyncFlags

	TerraformPlanFile string `env:"TERRAFORM_PLAN_FILE" default:"" help:"Add details of the Terraform Plan file to the synchronization to Terramate Cloud."`
	TofuPlanFile      string `env:"TOFU_PLAN_FILE" default:"" help:"Add details of the OpenTofu Plan file to the synchronization to Terramate Cloud."`
	DebugPreviewURL   string `hidden:"true" default:"" help:"Create a debug preview URL to Terramate Cloud details."`

	commonRunFlags

	Eval       bool     `env:"EVAL" default:"false" help:"Evaluate command arguments as HCL strings interpolating Globals, Functions and Metadata."`
	Terragrunt bool     `env:"TERRAGRUNT" default:"false" help:"Use terragrunt when generating planfile for Terramate Cloud sync."`
	Command    []string `arg:"" name:"cmd" predictor:"file" passthrough:"" help:"Command to execute"`
}
```
to:
```go
type runCommandFlags struct {
	changeDetectionFlags

	EnableSharing bool `env:"ENABLE_SHARING" help:"Enable sharing of stack outputs as stack inputs."`
	MockOnFail    bool `env:"MOCK_ON_FAIL" help:"Mock the output values if command fails."`

	commonRunFlags

	Eval       bool     `env:"EVAL" default:"false" help:"Evaluate command arguments as HCL strings interpolating Globals, Functions and Metadata."`
	Terragrunt bool     `env:"TERRAGRUNT" default:"false" help:"Use terragrunt when generating planfile."`
	Command    []string `arg:"" name:"cmd" predictor:"file" passthrough:"" help:"Command to execute"`
}
```

- [ ] **Step 4: Remove `cloudFilterFlags`/`cloudTargetFlags` from `runScriptFlags`**

Change:
```go
type runScriptFlags struct {
	cloudFilterFlags
	changeDetectionFlags
	cloudTargetFlags
	commonRunFlags

	Cmds []string `arg:"" optional:"true" passthrough:"" help:"Script to execute."`
}
```
to:
```go
type runScriptFlags struct {
	changeDetectionFlags
	commonRunFlags

	Cmds []string `arg:"" optional:"true" passthrough:"" help:"Script to execute."`
}
```

- [ ] **Step 5: Simplify `migrateFlagAliases` — remove all cloud-related migrations**

Change:
```go
func migrateFlagAliases(parsedArgs *FlagSpec) {
	// list
	migrateStringFlag(&parsedArgs.List.Status, parsedArgs.List.CloudStatus)

	// run
	migrateStringFlag(&parsedArgs.Run.Status, parsedArgs.Run.CloudStatus)
	migrateBoolFlag(&parsedArgs.Run.SyncDeployment, parsedArgs.Run.CloudSyncDeployment)
	migrateBoolFlag(&parsedArgs.Run.SyncDriftStatus, parsedArgs.Run.CloudSyncDriftStatus)
	migrateBoolFlag(&parsedArgs.Run.SyncPreview, parsedArgs.Run.CloudSyncPreview)
	migrateStringFlag(&parsedArgs.Run.TerraformPlanFile, parsedArgs.Run.CloudSyncTerraformPlanFile)
	if parsedArgs.Run.CloudSyncLayer != "" && parsedArgs.Run.Layer == "" {
		parsedArgs.Run.Layer = parsedArgs.Run.CloudSyncLayer
	}

	// script run
	migrateStringFlag(&parsedArgs.Script.Run.Status, parsedArgs.Script.Run.CloudStatus)

	// experimental trigger
	migrateStringFlag(&parsedArgs.Experimental.Trigger.Status, parsedArgs.Experimental.Trigger.CloudStatus)

	// trigger
	migrateStringFlag(&parsedArgs.Trigger.Status, parsedArgs.Trigger.CloudStatus)
}
```
to a no-op-free removal: delete the function entirely and its single call site (find via `grep -rn "migrateFlagAliases(" ui/tui/`). If `migrateStringFlag`/`migrateBoolFlag` helper functions become unused as a result, remove them too (`grep -rn "func migrateStringFlag\|func migrateBoolFlag" ui/tui/`).

- [ ] **Step 6: Update `ui/tui/cli_handler.go`'s `list` case**

Change:
```go
	case "list":
		c.SetCommandAnalytics("list",
			tel.BoolFlag("filter-changed", parsedArgs.Changed),
			tel.BoolFlag("filter-tags", len(parsedArgs.Tags) != 0),
			tel.StringFlag("filter-status", parsedArgs.List.Status),
			tel.StringFlag("filter-drift-status", parsedArgs.List.DriftStatus),
			tel.StringFlag("filter-deployment-status", parsedArgs.List.DeploymentStatus),
			tel.StringFlag("filter-target", parsedArgs.List.Target),
			tel.BoolFlag("run-order", parsedArgs.List.RunOrder),
		)
		expStatus := parsedArgs.List.ExperimentalStatus
		cloudStatus := parsedArgs.List.Status
		if expStatus != "" && cloudStatus != "" {
			return nil, errors.E("--experimental-status and --status cannot be used together")
		}

		var statusStr string
		if cloudStatus != "" {
			statusStr = cloudStatus
		} else if expStatus != "" {
			statusStr = expStatus
		}

		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			parsedArgs.List.EnableChangeDetection,
			parsedArgs.List.DisableChangeDetection,
		)
		if err != nil {
			return nil, err
		}
		return &listcmd.Spec{
			GitFilter: gitfilter,
			Reason:    parsedArgs.List.Why,
			Target:    parsedArgs.List.Target,
			StatusFilters: listcmd.StatusFilters{
				StackStatus:      statusStr,
				DeploymentStatus: parsedArgs.List.DeploymentStatus,
				DriftStatus:      parsedArgs.List.DriftStatus,
			},
			RunOrder: parsedArgs.List.RunOrder,
			Tags:     parsedArgs.Tags,
			NoTags:   parsedArgs.NoTags,
			DependencyFilters: engine.DependencyFilters{
				...
			},
		}, nil
```
to (drop the status/target variables and the `Target`/`StatusFilters` fields on `listcmd.Spec` — the `StatusFilters` and `Target` fields on `listcmd.Spec` itself are removed in Task 8):
```go
	case "list":
		c.SetCommandAnalytics("list",
			tel.BoolFlag("filter-changed", parsedArgs.Changed),
			tel.BoolFlag("filter-tags", len(parsedArgs.Tags) != 0),
			tel.BoolFlag("run-order", parsedArgs.List.RunOrder),
		)
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			parsedArgs.List.EnableChangeDetection,
			parsedArgs.List.DisableChangeDetection,
		)
		if err != nil {
			return nil, err
		}
		return &listcmd.Spec{
			GitFilter: gitfilter,
			Reason:    parsedArgs.List.Why,
			RunOrder:  parsedArgs.List.RunOrder,
			Tags:      parsedArgs.Tags,
			NoTags:    parsedArgs.NoTags,
			DependencyFilters: engine.DependencyFilters{
				...
			},
		}, nil
```
(keep the `DependencyFilters` block exactly as it is today — it is unrelated to cloud).

- [ ] **Step 7: Update the `trigger`/`experimental trigger` cases**

Change (both the bare `trigger`/`experimental trigger` case and the `trigger <stack>`/`experimental trigger <stack>` case):
```go
	case "experimental trigger": // Deprecated
		parsedArgs.Trigger = parsedArgs.Experimental.Trigger
		fallthrough
	case "trigger":
		c.SetCommandAnalytics("trigger")
		gitfilter, err := engine.NewGitFilter(
			parsedArgs.Changed,
			parsedArgs.GitChangeBase,
			nil,
			nil,
		)
		if err != nil {
			return nil, err
		}
		expStatus := parsedArgs.Trigger.ExperimentalStatus
		cloudStatus := parsedArgs.Trigger.Status
		if expStatus != "" && cloudStatus != "" {
			return nil, errors.E("--experimental-status and --status cannot be used together")
		}

		var statusStr string
		if cloudStatus != "" {
			statusStr = cloudStatus
		} else if expStatus != "" {
			statusStr = expStatus
		}

		if statusStr == "" && parsedArgs.Trigger.DeploymentStatus == "" && parsedArgs.Trigger.DriftStatus == "" {
			return nil, errors.E("trigger command expects either a stack path or a cloud filter such as --status")
		}
		if parsedArgs.Trigger.Recursive {
			return nil, errors.E("cloud filters such as --status are incompatible with --recursive flag")
		}
		return &triggercmd.FilterSpec{
			GitFilter: gitfilter,
			StatusFilters: triggercmd.StatusFilters{
				StackStatus:      statusStr,
				DeploymentStatus: parsedArgs.Trigger.DeploymentStatus,
				DriftStatus:      parsedArgs.Trigger.DriftStatus,
			},
			Change:       parsedArgs.Trigger.Change,
			IgnoreChange: parsedArgs.Trigger.IgnoreChange,
			Tags:         parsedArgs.Tags,
			NoTags:       parsedArgs.NoTags,
			Reason:       parsedArgs.Trigger.Reason,
		}, nil
```
to (this case now REQUIRES a stack argument since the cloud-filter path is gone — return an error if `Stack` is empty, since previously `trigger` with no stack path only made sense via a cloud filter):
```go
	case "experimental trigger": // Deprecated
		parsedArgs.Trigger = parsedArgs.Experimental.Trigger
		fallthrough
	case "trigger":
		c.SetCommandAnalytics("trigger")
		return nil, errors.E("trigger command expects a stack path")
```
And for `trigger <stack>` / `experimental trigger <stack>`, remove the now-dead cloud-status/recursive check:
```go
	case "experimental trigger <stack>": // Deprecated
		parsedArgs.Trigger = parsedArgs.Experimental.Trigger
		fallthrough
	case "trigger <stack>":
		c.SetCommandAnalytics("trigger",
			tel.StringFlag("stack", parsedArgs.Trigger.Stack),
			tel.BoolFlag("change", parsedArgs.Trigger.Change),
			tel.BoolFlag("ignore-change", parsedArgs.Trigger.IgnoreChange),
		)
		if parsedArgs.Trigger.Status != "" && parsedArgs.Trigger.Recursive {
			return nil, errors.E("cloud filters such as --status are incompatible with --recursive flag")
		}
		return &triggercmd.PathSpec{
```
to:
```go
	case "experimental trigger <stack>": // Deprecated
		parsedArgs.Trigger = parsedArgs.Experimental.Trigger
		fallthrough
	case "trigger <stack>":
		c.SetCommandAnalytics("trigger",
			tel.StringFlag("stack", parsedArgs.Trigger.Stack),
			tel.BoolFlag("change", parsedArgs.Trigger.Change),
			tel.BoolFlag("ignore-change", parsedArgs.Trigger.IgnoreChange),
		)
		return &triggercmd.PathSpec{
```
(the rest of the `PathSpec{...}` construction is unchanged — it has no cloud fields).

**Note:** `triggercmd.FilterSpec` (the filter-only trigger path) becomes entirely unused once this case is simplified — Task 5's collateral cleanup in `commands/trigger/filter.go` should confirm whether `FilterSpec` and its handling function are now dead code and remove them if so.

- [ ] **Step 8: Update the `run <cmd>` and `script run <cmds>` Spec-construction to stop reading the removed flags**

For `run <cmd>` (find via `grep -n 'case "run <cmd>"' ui/tui/cli_handler.go`), remove every field in the constructed `&runcmd.Spec{...}` literal that reads `parsedArgs.Run.SyncDeployment`, `.SyncDriftStatus`, `.SyncPreview`, `.Status`, `.DriftStatus`, `.DeploymentStatus`, `.Target`, `.FromTarget`, `.Layer`, `.TerraformPlanFile`, `.TofuPlanFile`, `.PlanRenderTimeout`, `.DebugPreviewURL` — these fields no longer exist on `parsedArgs.Run` after Step 3. The `Spec` struct itself still has these fields until Task 5 removes them; simply stop setting them here (they'll default to zero values, which is fine since Task 5 removes the fields before this dead state matters).

Apply the same treatment to `script run <cmds>` (find via `grep -n 'case "script run <cmds>"' ui/tui/cli_handler.go`) for `parsedArgs.Script.Run.Status/.DriftStatus/.DeploymentStatus/.Target/.FromTarget`.

- [ ] **Step 9: Update the four `debug show *` cases to stop building `StatusFilters` from removed flags**

Each of the four cases (`debug show metadata`, `debug show globals`, `debug show generate-origins`, `debug show runtime-env`) currently does:
```go
		statusFilters, err := status.ParseFilters(
			parsedArgs.Debug.Show.Globals.Status,
			parsedArgs.Debug.Show.Globals.DriftStatus,
			parsedArgs.Debug.Show.Globals.DeploymentStatus,
		)
```
(with the corresponding sub-struct name per case). Remove this call and the `err` check that follows it, and remove `StatusFilters: statusFilters` from the constructed `Spec` literal in each case (the `StatusFilters` field itself is removed from these four `Spec` structs in Task 8's collateral cleanup).

Also remove the now-unused `"github.com/terramate-io/terramate/cloud/api/status"` import from `ui/tui/cli_handler.go` if nothing else in the file uses the `status` package after this change.

- [ ] **Step 10: Verify the repo builds**

Run: `mise run build`
Expected: exits 0. (There will likely be compile errors pointing at `commands/run`, `commands/script/run`, `commands/stack/list`, `commands/trigger`, `commands/debug/show/*` still trying to read fields that Step 8/9 stopped populating incorrectly, or Spec struct fields that reference removed types — fix each reported error by removing the specific dead reference; do not add new cloud logic back.)

- [ ] **Step 11: Commit**

```bash
git add -A -- ui/tui/cli_spec.go ui/tui/cli_handler.go
git commit -m "feat: remove cloud flags from run, script run, list, trigger, debug show"
```

---

### Task 5: Remove Cloud fields/logic from `commands/run`

**Files:**
- Delete: `commands/run/cloud.go`
- Modify: `commands/run/run.go`

**Interfaces:**
- Consumes: Task 4 (flags no longer populate these fields).
- Produces: `runcmd.Spec` no longer has `SyncDeployment`/`SyncDriftStatus`/`SyncPreview`/`DebugPreviewURL`/`TechnologyLayer`/`TerraformPlanFile`/`PlanRenderTimeout`/`TofuPlanFile`/`Target`/`FromTarget`/`StatusFilters`/`state cloudsync.CloudRunState` — Task 8 (engine) can now safely remove the matching `StackRunTask` fields since this is the last non-`cloudsync` writer of them.

- [ ] **Step 1: Delete `commands/run/cloud.go`**

```bash
git rm commands/run/cloud.go
```

- [ ] **Step 2: Remove the call to the deleted file's method**

Remove from `Exec` (or wherever `s.checkCloudSync()` is called, `commands/run/run.go:131`):
```go
	err = s.checkCloudSync()
	if err != nil {
		return err
	}
```

- [ ] **Step 3: Remove cloud-sync fields from `Spec`**

Change:
```go
type Spec struct {
	// Behavior control options
	Command         []string
	Quiet           bool
	DryRun          bool
	Reverse         bool
	ScriptRun       bool
	ContinueOnError bool
	Parallel        int
	NoRecursive     bool

	SyncDeployment    bool
	SyncDriftStatus   bool
	SyncPreview       bool
	DebugPreviewURL   string
	TechnologyLayer   preview.Layer
	TerraformPlanFile string
	PlanRenderTimeout time.Duration
	TofuPlanFile      string
	Terragrunt        bool
	EnableSharing     bool
	MockOnFail        bool
	EvalCmd           bool

	GitFilter     engine.GitFilter
	StatusFilters StatusFilters
	Target        string
	FromTarget    string
	Tags          []string
	NoTags        []string

	engine.DependencyFilters

	Safeguards Safeguards

	state cloudsync.CloudRunState

	workingDir string
	engine     *engine.Engine
	printers   printer.Printers
	stdout     io.Writer
	stderr     io.Writer
	stdin      io.Reader
}
```
to:
```go
type Spec struct {
	// Behavior control options
	Command         []string
	Quiet           bool
	DryRun          bool
	Reverse         bool
	ScriptRun       bool
	ContinueOnError bool
	Parallel        int
	NoRecursive     bool

	Terragrunt    bool
	EnableSharing bool
	MockOnFail    bool
	EvalCmd       bool

	GitFilter engine.GitFilter
	Tags      []string
	NoTags    []string

	engine.DependencyFilters

	Safeguards Safeguards

	workingDir string
	engine     *engine.Engine
	printers   printer.Printers
	stdout     io.Writer
	stderr     io.Writer
	stdin      io.Reader
}
```
Also delete the now-unused `StatusFilters` struct:
```go
type StatusFilters struct {
	StackStatus      string
	DeploymentStatus string
	DriftStatus      string
}
```

- [ ] **Step 4: Remove the cloud-sync validation block**

Remove:
```go
	if s.SyncDeployment && s.SyncDriftStatus {
		return errors.E(ErrConflictOptions, "--sync-deployment conflicts with --sync-drift-status")
	}

	if s.SyncPreview && (s.SyncDeployment || s.SyncDriftStatus) {
		return errors.E(ErrConflictOptions, "cannot use --sync-preview with --sync-deployment or --sync-drift-status")
	}

	if s.TerraformPlanFile != "" && s.TofuPlanFile != "" {
		return errors.E(ErrConflictOptions, "--terraform-plan-file conflicts with --tofu-plan-file")
	}

	planFile, planProvisioner := SelectPlanFile(s.TerraformPlanFile, s.TofuPlanFile)

	if planFile == "" && s.SyncPreview {
		return errors.E(ErrConflictOptions, "--sync-preview requires --terraform-plan-file or -tofu-plan-file")
	}

	cloudSyncEnabled := s.SyncDeployment || s.SyncDriftStatus || s.SyncPreview

	if s.TerraformPlanFile != "" && !cloudSyncEnabled {
		return errors.E(ErrConflictOptions, "--terraform-plan-file requires flags --sync-deployment or --sync-drift-status or --sync-preview")
	} else if s.TofuPlanFile != "" && !cloudSyncEnabled {
		return errors.E(ErrConflictOptions, "--tofu-plan-file requires flags --sync-deployment or --sync-drift-status or --sync-preview")
	}

	err = s.engine.CheckTargetsConfiguration(s.Target, s.FromTarget, func(isTargetSet bool) error {
		isStatusSet := s.StatusFilters.StackStatus != ""
		isUsingCloudFeat := cloudSyncEnabled || isStatusSet

		if isTargetSet && !isUsingCloudFeat {
			return errors.E(ErrConflictOptions, "--target must be used together with --sync-deployment, --sync-drift-status, --sync-preview, or --status")
		} else if !isTargetSet && isUsingCloudFeat {
			return errors.E(ErrConflictOptions, "--sync-*/--status flags require --target when terramate.config.cloud.targets.enabled is true")
		}
		return nil
	})

	if err != nil {
		return err
	}

	if s.FromTarget != "" && !cloudSyncEnabled {
		return errors.E(ErrConflictOptions, "--from-target must be used together with --sync-deployment, --sync-drift-status, or --sync-preview")
	}
```
Since `SelectPlanFile` is still used elsewhere for a legitimate purpose, check whether it is: per research, `SelectPlanFile`'s only callers are this deleted block and `commands/script/run/run.go` (removed in Task 6) — if no callers remain after this task and Task 6, delete `SelectPlanFile` too (grep to confirm before deleting).

- [ ] **Step 5: Remove the cloud-metadata/preview-CICD-warning block**

Remove:
```go
	if cloudSyncEnabled {
		if !s.engine.Project().IsRepo() {
			return errors.E("cloud features requires a git repository")
		}
		err = s.engine.EnsureAllStackHaveIDs(stacks)
		if err != nil {
			return err
		}

		cloudsync.DetectCloudMetadata(s.engine, &s.state)
	}

	isCICD := os.Getenv("GITHUB_ACTIONS") != "" || os.Getenv("GITLAB_CI") != "" || os.Getenv("BITBUCKET_BUILD_NUMBER") != ""
	if s.SyncPreview && !isCICD {
		printer.Stderr.Warn(cloudSyncPreviewCICDWarning)
		s.engine.DisableCloudFeatures(errors.E(cloudSyncPreviewCICDWarning))
	}
```
(check whether `s.engine.EnsureAllStackHaveIDs(stacks)` is needed for a non-cloud reason elsewhere before removing this call entirely — if stack-ID assignment is a cloud-only concern for `run`, remove it; if any other code path in this file needs stacks to have IDs, keep the call outside the `cloudSyncEnabled` guard.)

- [ ] **Step 6: Remove the `Cloud*` fields from the `engine.StackRunTask{...}` literal**

Change:
```go
		run := engine.StackRun{
			SyncTaskIndex: -1,
			Stack:         st.Stack,
			Tasks: []engine.StackRunTask{
				{
					Cmd:                    s.Command,
					CloudTarget:            s.Target,
					CloudFromTarget:        s.FromTarget,
					CloudSyncDeployment:    s.SyncDeployment,
					CloudSyncDriftStatus:   s.SyncDriftStatus,
					CloudSyncPreview:       s.SyncPreview,
					CloudPlanFile:          planFile,
					CloudPlanProvisioner:   planProvisioner,
					CloudPlanRenderTimeout: s.PlanRenderTimeout,
					CloudSyncLayer:         s.TechnologyLayer,
					UseTerragrunt:          s.Terragrunt,
					EnableSharing:          s.EnableSharing,
					MockOnFail:             s.MockOnFail,
				},
			},
		}
```
to:
```go
		run := engine.StackRun{
			SyncTaskIndex: -1,
			Stack:         st.Stack,
			Tasks: []engine.StackRunTask{
				{
					Cmd:           s.Command,
					UseTerragrunt: s.Terragrunt,
					EnableSharing: s.EnableSharing,
					MockOnFail:    s.MockOnFail,
				},
			},
		}
```

- [ ] **Step 7: Remove the deployment/preview creation calls**

Remove:
```go
	if s.SyncDeployment {
		// This will just select all runs, since the CloudSyncDeployment was set just above.
		// Still, it's convenient to re-use this function here.
		deployRuns := engine.SelectCloudStackTasks(runs, engine.IsDeploymentTask)
		err := cloudsync.CreateCloudDeployment(s.engine, s.workingDir, deployRuns, &s.state)
		if err != nil {
			return err
		}
	}

	if s.SyncPreview && s.cloudEnabled() {
		// See comment above.
		previewRuns := engine.SelectCloudStackTasks(runs, engine.IsPreviewTask)
		for metaID, previewID := range cloudsync.CreateCloudPreview(s.engine, s.GitFilter, previewRuns, s.Target, s.FromTarget, &s.state) {
			s.state.SetMeta2PreviewID(metaID, previewID)
		}

		if s.DebugPreviewURL != "" {
			s.writePreviewURL()
		}
	}
```

- [ ] **Step 8: Simplify the `RunAll` hooks — remove the cloud-sync `Before`/`After`/`LogSyncer`/`LogSyncCondition` wiring**

Change:
```go
		Hooks: &engine.Hooks{
			Before: func(e *engine.Engine, run engine.StackCloudRun) {
				cloudsync.BeforeRun(e, run, &s.state)
			},
			After: func(e *engine.Engine, run engine.StackCloudRun, res engine.RunResult, err error) {
				cloudsync.AfterRun(e, run, &s.state, res, err)
			},
			LogSyncCondition: func(task engine.StackRunTask, _ engine.StackRun) bool {
				return task.CloudSyncDeployment || task.CloudSyncPreview || task.CloudSyncDriftStatus
			},
			LogSyncer: func(logger *zerolog.Logger, e *engine.Engine, run engine.StackRun, task engine.StackRunTask, logs resources.CommandLogs) {
				cloudsync.Logs(logger, e, run, task, &s.state, logs)
			},
		},
```
to simply omitting the `Hooks` field entirely from the `RunAllOptions{...}` literal (Task 8 makes `Hooks` optional / gives it a safe nil-check default, matching what `engine/run.go` already does when `opts.Hooks == nil`).

- [ ] **Step 9: Remove `cloudEnabled()` and `writePreviewURL()`**

Remove both methods entirely (their only callers were removed in Step 5/7).

- [ ] **Step 10: Remove now-dead imports**

Remove `cloudsync`, `cloud/api/resources` (if `resources.CommandLogs` was the only use), `cloud/api/preview` (if `preview.Layer` was the only use), `zerolog` (if `*zerolog.Logger` was the only use in this file) from `commands/run/run.go`'s import block — verify each with `goimports`/build errors rather than guessing.

- [ ] **Step 11: Verify the repo builds**

Run: `mise run build`
Expected: exits 0 (fix any remaining compile errors surfaced by these removals — they should all be in `commands/run/run.go` itself at this point).

- [ ] **Step 12: Commit**

```bash
git add -A -- commands/run/run.go commands/run/cloud.go
git commit -m "feat: remove cloud sync fields and logic from commands/run"
```

---

### Task 6: Remove Cloud fields/logic from `commands/script/run`

**Files:**
- Modify: `commands/script/run/run.go`

**Interfaces:**
- Consumes: Task 4 (flags no longer populate `parsedArgs.Script.Run.*` cloud fields), Task 5's pattern (same shape of removal).
- Produces: `Spec` no longer carries `Target`/`FromTarget`/`StatusFilters`/`state cloudsync.CloudRunState`.

- [ ] **Step 1: Remove cloud fields from `Spec`**

Change:
```go
type Spec struct {
	Safeguards      runcmd.Safeguards
	DryRun          bool
	Quiet           bool
	Reverse         bool
	Parallel        int
	ContinueOnError bool
	GitFilter       engine.GitFilter
	Target          string
	FromTarget      string
	NoRecursive     bool
	NoTags          []string
	Tags            []string
	engine.DependencyFilters
	StatusFilters runcmd.StatusFilters

	Labels []string

	state cloudsync.CloudRunState

	workingDir string
	engine     *engine.Engine
	printers   printer.Printers

	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}
```
to:
```go
type Spec struct {
	Safeguards      runcmd.Safeguards
	DryRun          bool
	Quiet           bool
	Reverse         bool
	Parallel        int
	ContinueOnError bool
	GitFilter       engine.GitFilter
	NoRecursive     bool
	NoTags          []string
	Tags            []string
	engine.DependencyFilters

	Labels []string

	workingDir string
	engine     *engine.Engine
	printers   printer.Printers

	stdout io.Writer
	stderr io.Writer
	stdin  io.Reader
}
```

- [ ] **Step 2: Remove the target-configuration check**

Remove:
```go
	err = s.engine.CheckTargetsConfiguration(s.Target, s.FromTarget, func(isTargetSet bool) error {
		if !isTargetSet {
			// We don't check here if any script has any sync command options enabled.
			// We assume yes and so --target must be set.
			return errors.E("--target is required when terramate.config.cloud.targets.enabled is true")
		}
		return nil
	})

	if err != nil {
		return err
	}
```

- [ ] **Step 3: Simplify the per-task construction to drop cloud fields**

Change:
```go
			for jobIdx, job := range evalScript.Jobs {
				for cmdIdx, cmd := range job.Commands() {
					task := engine.StackRunTask{
						Cmd:             cmd.Args,
						CloudTarget:     s.Target,
						CloudFromTarget: s.FromTarget,
						ScriptIdx:       scriptIdx,
						ScriptJobIdx:    jobIdx,
						ScriptCmdIdx:    cmdIdx,
					}

					if cmd.Options != nil {
						planFile, planProvisioner := runcmd.SelectPlanFile(
							cmd.Options.CloudTerraformPlanFile,
							cmd.Options.CloudTofuPlanFile)

						task.CloudSyncDeployment = cmd.Options.CloudSyncDeployment
						task.CloudSyncDriftStatus = cmd.Options.CloudSyncDriftStatus
						task.CloudSyncPreview = cmd.Options.CloudSyncPreview
						task.CloudSyncLayer = cmd.Options.CloudSyncLayer
						task.CloudPlanFile = planFile
						task.CloudPlanProvisioner = planProvisioner
						task.CloudPlanRenderTimeout = cmd.Options.CloudPlanRenderTimeout
						task.UseTerragrunt = cmd.Options.UseTerragrunt
						task.EnableSharing = cmd.Options.EnableSharing
						task.MockOnFail = cmd.Options.MockOnFail

						tel.DefaultRecord.Set(
							tel.BoolFlag("sync-deployment", cmd.Options.CloudSyncDeployment),
							tel.BoolFlag("sync-drift", cmd.Options.CloudSyncDriftStatus),
							tel.BoolFlag("sync-preview", cmd.Options.CloudSyncPreview),
							tel.StringFlag("terraform-planfile", cmd.Options.CloudTerraformPlanFile),
							tel.StringFlag("tofu-planfile", cmd.Options.CloudTofuPlanFile),
							tel.StringFlag("layer", string(cmd.Options.CloudSyncLayer)),
							tel.BoolFlag("terragrunt", cmd.Options.UseTerragrunt),
							tel.BoolFlag("output-sharing", cmd.Options.EnableSharing),
							tel.BoolFlag("output-mocks", cmd.Options.MockOnFail),
						)
					}
					run.Tasks = append(run.Tasks, task)
					if task.CloudSyncDeployment || task.CloudSyncDriftStatus || task.CloudSyncPreview {
						run.SyncTaskIndex = len(run.Tasks) - 1
					}
				}
			}
```
to:
```go
			for jobIdx, job := range evalScript.Jobs {
				for cmdIdx, cmd := range job.Commands() {
					task := engine.StackRunTask{
						Cmd:          cmd.Args,
						ScriptIdx:    scriptIdx,
						ScriptJobIdx: jobIdx,
						ScriptCmdIdx: cmdIdx,
					}

					if cmd.Options != nil {
						task.UseTerragrunt = cmd.Options.UseTerragrunt
						task.EnableSharing = cmd.Options.EnableSharing
						task.MockOnFail = cmd.Options.MockOnFail

						tel.DefaultRecord.Set(
							tel.BoolFlag("terragrunt", cmd.Options.UseTerragrunt),
							tel.BoolFlag("output-sharing", cmd.Options.EnableSharing),
							tel.BoolFlag("output-mocks", cmd.Options.MockOnFail),
						)
					}
					run.Tasks = append(run.Tasks, task)
				}
			}
```
(`run.SyncTaskIndex` field on `engine.StackRun` becomes dead once nothing ever sets it to something other than its zero value — check in Task 8 whether `SyncTaskIndex` has any remaining reader; if not, remove the field from `engine.StackRun` too.)

- [ ] **Step 4: Delete `prepareScriptForCloudSync` entirely and its call site**

Remove the call:
```go
	err = s.prepareScriptForCloudSync(runs)
	if err != nil {
		return err
	}
```
Remove the whole method (`commands/script/run/run.go`, the `prepareScriptForCloudSync` function body) and the three `cloudFeatScriptSync*` constants plus `cloudSyncPreviewCICDWarning` at the top of the file.

- [ ] **Step 5: Simplify the `RunAll` hooks the same way as Task 5's Step 8**

Change:
```go
	err = s.engine.RunAll(runs, engine.RunAllOptions{
		ScriptRun:       true,
		Quiet:           s.Quiet,
		DryRun:          s.DryRun,
		Reverse:         s.Reverse,
		ContinueOnError: s.ContinueOnError,
		Parallel:        s.Parallel,
		Stdout:          s.stdout,
		Stderr:          s.stderr,
		Stdin:           s.stdin,
		Hooks: &engine.Hooks{
			Before: func(e *engine.Engine, run engine.StackCloudRun) {
				cloudsync.BeforeRun(e, run, &s.state)
			},
			After: func(e *engine.Engine, run engine.StackCloudRun, res engine.RunResult, err error) {
				cloudsync.AfterRun(e, run, &s.state, res, err)
			},
			LogSyncCondition: func(task engine.StackRunTask, _ engine.StackRun) bool {
				return task.CloudSyncDeployment || task.CloudSyncPreview || task.CloudSyncDriftStatus
			},
			LogSyncer: func(logger *zerolog.Logger, e *engine.Engine, run engine.StackRun, task engine.StackRunTask, logs resources.CommandLogs) {
				cloudsync.Logs(logger, e, run, task, &s.state, logs)
			},
		},
	})
```
to:
```go
	err = s.engine.RunAll(runs, engine.RunAllOptions{
		ScriptRun:       true,
		Quiet:           s.Quiet,
		DryRun:          s.DryRun,
		Reverse:         s.Reverse,
		ContinueOnError: s.ContinueOnError,
		Parallel:        s.Parallel,
		Stdout:          s.stdout,
		Stderr:          s.stderr,
		Stdin:           s.stdin,
	})
```

- [ ] **Step 6: Remove now-dead imports**

Remove `cloudsync`, `hashicorp/go-uuid`, `cloud/api/resources`, `zerolog` from this file's imports if they're no longer referenced (verify with build errors).

- [ ] **Step 7: Verify the repo builds**

Run: `mise run build`
Expected: exits 0.

- [ ] **Step 8: Remove `cloud_sync_*` HCL parsing tests that exercised this now-removed path, if any live in this package's own test file** (the main HCL-side removal is Task 7 — this step only concerns anything test-local to `commands/script/run`)

Run: `grep -rn "cloud_sync\|CloudSync" commands/script/run/*_test.go`
If any hits, remove those test cases/assertions.

- [ ] **Step 9: Commit**

```bash
git add -A -- commands/script/run/run.go
git commit -m "feat: remove cloud sync fields and logic from commands/script/run"
```

---

### Task 7: Remove Cloud config from `config/script.go`, `config/config.go`, and `hcl/hcl.go`

**Files:**
- Modify: `config/script.go`
- Modify: `config/config.go`
- Modify: `hcl/hcl.go`
- Modify: `config/script_test.go`

**Interfaces:**
- Consumes: nothing from prior tasks directly (this is the HCL/config-schema layer).
- Produces: `terramate.config.cloud { }` is no longer valid HCL; `config.ScriptCmdOptions` no longer has `CloudSync*`/`CloudTerraformPlanFile`/`CloudTofuPlanFile`/`CloudPlanRenderTimeout`; `config.Root.IsTargetsEnabled()` is removed — Task 8's `engine.CheckTargetsConfiguration` depends on this being gone first (or removed in the same task if easier to keep green).

- [ ] **Step 1: Remove `CloudSync*` fields from `ScriptCmdOptions`**

Change:
```go
type ScriptCmdOptions struct {
	CloudSyncDeployment    bool
	CloudSyncDriftStatus   bool
	CloudSyncPreview       bool
	CloudSyncLayer         preview.Layer
	CloudTerraformPlanFile string
	CloudTofuPlanFile      string
	CloudPlanRenderTimeout time.Duration
	UseTerragrunt          bool
	EnableSharing          bool
	MockOnFail             bool
}
```
to:
```go
type ScriptCmdOptions struct {
	UseTerragrunt bool
	EnableSharing bool
	MockOnFail    bool
}
```

- [ ] **Step 2: Remove the cloud-only cases from `unmarshalScriptCommandOptions`'s switch**

Remove the `sync_deployment`/`cloud_sync_deployment`, `sync_drift_status`/`cloud_sync_drift_status`, `sync_preview`/`cloud_sync_preview`, `layer`/`sync_layer`/`cloud_sync_layer`, `terraform_plan_file`/`sync_terraform_plan_file`/`cloud_sync_terraform_plan_file`, `tofu_plan_file`, and `plan_render_timeout` cases entirely (all of them — none of these keys have a non-cloud meaning). Keep `terragrunt`, `enable_sharing`, `mock_on_fail` exactly as they are.

Remove the two now-dead validation checks at the end of the loop:
```go
		if r.CloudSyncDeployment && r.CloudSyncDriftStatus {
			errs.Append(errors.E(ErrScriptInvalidCmdOptions, expr.Range(),
				"'sync_deployment' and 'sync_drift_status' are conflicting options in the same command"))
		}

		if r.CloudTerraformPlanFile != "" && r.CloudTofuPlanFile != "" {
			errs.Append(errors.E(ErrScriptInvalidCmdOptions, expr.Range(),
				"'terraform_plan_file' and 'tofu_plan_file' are conflicting options in the same command"))
		}
```

- [ ] **Step 3: Remove the script-wide cloud validation in `EvalScript`**

Remove both blocks:
```go
	var cmdsWithCloudSyncDeployment []string
	for jobIdx, job := range evaluatedScript.Jobs {
		for cmdIdx, cmd := range job.Commands() {
			if cmd.Options != nil && cmd.Options.CloudSyncDeployment {
				cmdsWithCloudSyncDeployment = append(cmdsWithCloudSyncDeployment, fmt.Sprintf("job:%d.%d", jobIdx, cmdIdx))
			}
		}
	}
	if len(cmdsWithCloudSyncDeployment) > 1 {
		errs.Append(errors.E(ErrScriptInvalidCmdOptions,
			"only a single command per script may have 'sync_deployment' enabled, but was enabled by: %v",
			strings.Join(cmdsWithCloudSyncDeployment, " "),
		))
	}

	var cmdsWithCloudSyncPreview []string
	for jobIdx, job := range evaluatedScript.Jobs {
		for cmdIdx, cmd := range job.Commands() {
			if cmd.Options != nil && cmd.Options.CloudSyncPreview {
				cmdsWithCloudSyncPreview = append(cmdsWithCloudSyncPreview, fmt.Sprintf("job:%d.%d", jobIdx, cmdIdx))
			}
		}
	}
	if len(cmdsWithCloudSyncPreview) > 1 {
		errs.Append(errors.E(ErrScriptInvalidCmdOptions,
			"only a single command per script may have 'sync_preview' enabled, but was enabled by: %v",
			strings.Join(cmdsWithCloudSyncDeployment, " "),
		))
	}
```

- [ ] **Step 4: Remove the cloud cross-field check in `unmarshalScriptJobCommand`**

Change:
```go
			if elem.Type().IsObjectType() {
				var err error
				r.Options, err = unmarshalScriptCommandOptions(elem, expr)
				if r.Options != nil &&
					r.Options.CloudSyncPreview &&
					(r.Options.CloudSyncDriftStatus || r.Options.CloudSyncDeployment) {
					errs.Append(errors.E(ErrScriptInvalidCmdOptions, expr.Range(),
						"sync_preview cannot be used with sync_deployment or sync_drift_status"))
				}
				errs.Append(err)
			} else {
```
to:
```go
			if elem.Type().IsObjectType() {
				var err error
				r.Options, err = unmarshalScriptCommandOptions(elem, expr)
				errs.Append(err)
			} else {
```

- [ ] **Step 5: Remove `IsTargetsEnabled()` from `config/config.go`**

Remove:
```go
// IsTargetsEnabled returns the configured `terramate.config.cloud.targets.enabled` option.
func (root *Root) IsTargetsEnabled() bool {
	if root.tree.Node.Terramate != nil &&
		root.tree.Node.Terramate.Config != nil &&
		root.tree.Node.Terramate.Config.Cloud != nil &&
		root.tree.Node.Terramate.Config.Cloud.Targets != nil {
		return root.tree.Node.Terramate.Config.Cloud.Targets.Enabled
	}
	return false
}
```
(Task 8 removes its only call site, in `engine/engine.go`'s `CheckTargetsConfiguration` — do that in the same task if it's simpler to keep the build green, or leave `IsTargetsEnabled` as temporarily unused until Task 8, since an unused exported method is not a compile error.)

- [ ] **Step 6: Remove the `CloudConfig`/`TargetsConfig` types and their parsing from `hcl/hcl.go`**

Remove the two struct definitions and the `Cloud *CloudConfig` field from `RootConfig`:
```go
// CloudConfig represents Terramate cloud configuration.
type CloudConfig struct {
	// Organization is the name of the cloud organization
	Organization string

	Targets *TargetsConfig

	Location cloud.Region
}

// TargetsConfig represents Terramate targets configuration.
type TargetsConfig struct {
	Enabled bool
}
```
Change:
```go
type RootConfig struct {
	Git               *GitConfig
	Generate          *GenerateRootConfig
	ChangeDetection   *ChangeDetectionConfig
	Run               *RunConfig
	Cloud             *CloudConfig
	Experiments       []string
	DisableSafeguards safeguard.Keywords
	Telemetry         *TelemetryConfig
}
```
to:
```go
type RootConfig struct {
	Git               *GitConfig
	Generate          *GenerateRootConfig
	ChangeDetection   *ChangeDetectionConfig
	Run               *RunConfig
	Experiments       []string
	DisableSafeguards safeguard.Keywords
	Telemetry         *TelemetryConfig
}
```

Remove `"cloud"` (and the stray, pre-existing `"targets"` entry — it's only ever valid nested under `cloud`, not at this level, so it should never have been here either) from the `ValidateSubBlocks` call:
```go
	errs.AppendWrap(ErrTerramateSchema, block.ValidateSubBlocks("git", "generate", "change_detection", "run", "cloud", "targets", "telemetry"))
```
to:
```go
	errs.AppendWrap(ErrTerramateSchema, block.ValidateSubBlocks("git", "generate", "change_detection", "run", "telemetry"))
```

Remove the dispatch block:
```go
	cloudBlock, ok := block.Blocks[ast.NewEmptyLabelBlockType("cloud")]
	if ok {
		cfg.Cloud = &CloudConfig{}

		errs.Append(parseCloudConfig(cfg.Cloud, cloudBlock))
	}
```

Remove the two parsing functions in full: `parseCloudConfig` and `parseTargetsConfig` (both shown in full in the research — delete both function bodies entirely).

Remove the now-unused `"github.com/terramate-io/terramate/cloud"` import from `hcl/hcl.go` if `cloud.Region`/`cloud.ParseRegion`/`cloud.AvailableRegions` were its only uses in this file.

- [ ] **Step 7: Remove `cloud_sync_*` tests from `config/script_test.go`**

Run: `grep -n "cloud_sync\|CloudSync\|sync_deployment\|sync_preview\|sync_drift" config/script_test.go`
Remove every test case exercising these HCL attributes.

- [ ] **Step 8: Verify the repo builds**

Run: `mise run build`
Expected: exits 0 (fix any remaining references in `engine/engine.go` to `IsTargetsEnabled()`/`Config.Cloud` here if the build fails on them — otherwise leave for Task 8).

- [ ] **Step 9: Run the config and hcl package tests**

Run: `mise exec -- go test -race -count=1 ./config/... ./hcl/...`
Expected: `ok` for both, no failures.

- [ ] **Step 10: Commit**

```bash
git add -A -- config/script.go config/config.go config/script_test.go hcl/hcl.go
git commit -m "feat: remove terramate.config.cloud block and cloud_sync_* script options"
```

---

### Task 8: Remove Cloud from the `engine` package and fix collateral imports

**Files:**
- Delete: `engine/cloud.go`
- Modify: `engine/engine.go`
- Modify: `engine/run.go`
- Modify: `commands/trigger/filter.go`, `commands/trigger/stack.go`
- Modify: `commands/stack/create/create.go`, `commands/stack/list/list.go`
- Modify: `commands/script/info/info.go`
- Modify: `commands/debug/show/generate_origins/generate_origins.go`, `commands/debug/show/globals/globals.go`, `commands/debug/show/metadata/metadata.go`, `commands/debug/show/runtime_env/runtime_env.go`
- Modify: `http/http.go`

**Interfaces:**
- Consumes: Task 1's `engine.NewBufferGroup()`/`(*BufferGroup).NewBuffer(io.Writer) io.Writer`/`Wait()`; Tasks 5-7 having already stopped populating the cloud fields this task deletes.
- Produces: `engine.ListStacks(gitfilter GitFilter, checkRepo bool) (*stack.Report, error)` (2 params dropped) — this is the last task that changes this signature; nothing later depends on the old one.

- [ ] **Step 1: Delete `engine/cloud.go`**

```bash
git rm engine/cloud.go
```

- [ ] **Step 2: Simplify `ListStacks`**

Change:
```go
// ListStacks returns the list of stacks based on filters.
func (e *Engine) ListStacks(gitfilter GitFilter, target string, stackFilters resources.StatusFilters, checkRepo bool) (*stack.Report, error) {
	var report *stack.Report

	err := e.setupGit(gitfilter)
	if err != nil {
		return nil, err
	}

	mgr := e.StackManager()
	if gitfilter.IsChanged {
		report, err = mgr.ListChanged(stack.ChangeConfig{
			BaseRef:            e.project.baseRef,
			UntrackedChanges:   gitfilter.EnableUntracked,
			UncommittedChanges: gitfilter.EnableUncommitted,
		})
	} else {
		report, err = mgr.List(checkRepo)
	}

	if err != nil {
		return nil, err
	}

	// memoize the list of affected stacks so they can be retrieved later
	// without computing the list again
	e.state.affectedStacks = report.Stacks

	if stackFilters.HasFilter() {
		if !e.project.isRepo {
			return nil, errors.E("cloud filters requires a git repository")
		}
		err := e.SetupCloudConfig([]string{cloudFeatStatus})
		if err != nil {
			return nil, err
		}

		repository, err := e.project.Repo()
		if err != nil {
			return nil, err
		}
		if repository.Host == "local" {
			return nil, errors.E("status filters does not work with filesystem based remotes")
		}

		ctx, cancel := context.WithTimeout(context.Background(), defaultCloudTimeout)
		defer cancel()
		cloudStacks, err := e.state.cloud.client.StacksByStatus(ctx, e.state.cloud.Org.UUID, repository.Repo, target, stackFilters)
		if err != nil {
			return nil, err
		}

		cloudStacksMap := map[string]bool{}
		for _, stack := range cloudStacks {
			cloudStacksMap[stack.MetaID] = true
		}

		localStacks := report.Stacks
		var stacks []stack.Entry

		for _, stack := range localStacks {
			if cloudStacksMap[strings.ToLower(stack.Stack.ID)] {
				stacks = append(stacks, stack)
			}
		}
		report.Stacks = stacks
	}

	e.state.repoChecks = report.Checks
	return report, nil
}
```
to:
```go
// ListStacks returns the list of stacks based on filters.
func (e *Engine) ListStacks(gitfilter GitFilter, checkRepo bool) (*stack.Report, error) {
	var report *stack.Report

	err := e.setupGit(gitfilter)
	if err != nil {
		return nil, err
	}

	mgr := e.StackManager()
	if gitfilter.IsChanged {
		report, err = mgr.ListChanged(stack.ChangeConfig{
			BaseRef:            e.project.baseRef,
			UntrackedChanges:   gitfilter.EnableUntracked,
			UncommittedChanges: gitfilter.EnableUncommitted,
		})
	} else {
		report, err = mgr.List(checkRepo)
	}

	if err != nil {
		return nil, err
	}

	// memoize the list of affected stacks so they can be retrieved later
	// without computing the list again
	e.state.affectedStacks = report.Stacks
	e.state.repoChecks = report.Checks
	return report, nil
}
```

Update every call site (drop the two now-removed arguments):
```
engine/engine.go:286     e.ListStacks(gitfilter, target, stackFilters, true)        -> e.ListStacks(gitfilter, true)
engine/engine.go:997     e.ListStacks(gitfilter, cloudstack.AnyTarget, resources.NoStatusFilters(), false)  -> e.ListStacks(gitfilter, false)
commands/trigger/filter.go:60      s.engine.ListStacks(s.GitFilter, cloudstack.AnyTarget, cloudFilters, false)  -> s.engine.ListStacks(s.GitFilter, false)
commands/trigger/stack.go:117      s.engine.ListStacks(engine.NoGitFilter(), cloudstack.AnyTarget, resources.NoStatusFilters(), false) -> s.engine.ListStacks(engine.NoGitFilter(), false)
commands/stack/list/list.go:82     s.engine.ListStacks(s.GitFilter, s.Target, cloudFilters, false) -> s.engine.ListStacks(s.GitFilter, false)
commands/stack/create/create.go:414 s.engine.ListStacks(engine.NoGitFilter(), cloudstack.AnyTarget, resources.NoStatusFilters(), false) -> s.engine.ListStacks(engine.NoGitFilter(), false)
commands/script/info/info.go:57    s.engine.ListStacks(s.GitFilter, cloudstack.AnyTarget, resources.NoStatusFilters(), false) -> s.engine.ListStacks(s.GitFilter, false)
commands/debug/show/generate_origins/generate_origins.go:46  s.engine.ListStacks(s.GitFilter, cloudstack.AnyTarget, s.StatusFilters, false) -> s.engine.ListStacks(s.GitFilter, false)
commands/debug/show/globals/globals.go:45   (same pattern) -> s.engine.ListStacks(s.GitFilter, false)
commands/debug/show/runtime_env/runtime_env.go:45  (same pattern) -> s.engine.ListStacks(s.GitFilter, false)
commands/debug/show/metadata/metadata.go:48  (same pattern) -> s.engine.ListStacks(s.GitFilter, false)
```

For the four `debug/show/*` files, also remove the `StatusFilters resources.StatusFilters` field from their `Spec` structs and the now-unused `cloud/api/resources`/`cloud/api/stack` imports.

For `commands/stack/list/list.go`, remove its own `StatusFilters`/`Target` struct fields, the `status.ParseFilters(...)` call producing `cloudFilters`, and the `CheckTargetsConfiguration` callback block (`list.go:61-75`) — this file's cloud-target validation is being removed entirely, matching the flag removal from Task 4. Remove the `cloud/api/status` import.

For `commands/stack/create/create.go` and `commands/script/info/info.go`, just drop the two now-unused imports (`cloud/api/resources`, `cloud/api/stack`) — they had no other cloud logic.

For `commands/trigger/filter.go`, remove the `status.ParseFilters(...)` call and the `cloudFilters` variable, and the `cloud/api/status`/`cloud/api/stack` imports. Check whether `FilterSpec` (the whole cloud-filter-based trigger path) is now entirely dead code following Task 4's Step 7 simplification of the `trigger` case — if `s.StatusFilters` was the only reason `FilterSpec` existed as a distinct type from `PathSpec`, remove `FilterSpec` and its handling function from `commands/trigger/filter.go` (or the whole file, if nothing else in it survives), and confirm `commands/trigger/stack.go` doesn't also need equivalent trimming.

For `commands/trigger/stack.go`, remove the `cloud/api/stack`/`cloud/api/resources` imports (only used for `cloudstack.AnyTarget`/`resources.NoStatusFilters()` in the `ListStacks` call now being simplified).

- [ ] **Step 3: Remove `CheckTargetsConfiguration` and the `cloud CloudState` field**

Remove the whole `CheckTargetsConfiguration` method:
```go
// CheckTargetsConfiguration checks the target configuration of the project.
func (e *Engine) CheckTargetsConfiguration(targetArg, fromTargetArg string, cloudCheckFn func(bool) error) error {
	... (full body from research) ...
}
```
Its remaining call sites (`commands/stack/list/list.go` and `commands/script/run/run.go`) were already removed in Step 2 and Task 6 respectively — `commands/run/run.go`'s call was removed in Task 5. Confirm via `grep -rn "CheckTargetsConfiguration" .` that no callers remain before deleting.

Remove the `cloud CloudState` field from `Engine.state`:
```go
state struct {
    affectedStacks config.List[stack.Entry]
    repoChecks     stack.RepoChecks

    cloud CloudState
}
```
to:
```go
state struct {
    affectedStacks config.List[stack.Entry]
    repoChecks     stack.RepoChecks
}
```

Remove the now-dead `cloudFeatStatus`/`cloudFeatSyncDeployment`/`cloudFeatSyncDriftStatus`/`cloudFeatSyncPreview` constants and the `handleCriticalError`/`DisableCloudFeatures`-calling helper at the bottom of `engine.go` (`func (e *Engine) handleCriticalError(err error) error { ... }`) along with its call site in `EnsureAllStackHaveIDs`. Check whether `EnsureAllStackHaveIDs` itself is still needed for a non-cloud purpose before deciding whether to simplify or remove it (per Task 5's note — confirm no surviving caller needs stack-ID assignment for a non-cloud reason; if `EnsureAllStackHaveIDs` becomes fully unused, remove it too).

Remove the now-unused `"github.com/terramate-io/terramate/cloud/api/resources"` and `cloudstack "github.com/terramate-io/terramate/cloud/api/stack"` imports from `engine/engine.go`.

- [ ] **Step 4: Remove `Cloud*` fields from `StackRunTask`**

Change:
```go
type StackRunTask struct {
	Cmd []string

	ScriptIdx    int
	ScriptJobIdx int
	ScriptCmdIdx int

	CloudTarget     string
	CloudFromTarget string

	CloudSyncDeployment  bool
	CloudSyncDriftStatus bool
	CloudSyncPreview     bool
	CloudSyncLayer       preview.Layer

	CloudPlanFile          string
	CloudPlanProvisioner   string
	CloudPlanRenderTimeout time.Duration

	UseTerragrunt bool
	EnableSharing bool
	MockOnFail    bool
}
```
to:
```go
type StackRunTask struct {
	Cmd []string

	ScriptIdx    int
	ScriptJobIdx int
	ScriptCmdIdx int

	UseTerragrunt bool
	EnableSharing bool
	MockOnFail    bool
}
```

- [ ] **Step 5: Simplify `isSuccessExit`**

Change:
```go
func (t StackRunTask) isSuccessExit(exitCode int) bool {
	if exitCode == 0 {
		return true
	}
	if t.CloudSyncDriftStatus || (t.CloudSyncPreview && t.CloudPlanFile != "") {
		return exitCode == 2
	}
	return false
}
```
to:
```go
func (t StackRunTask) isSuccessExit(exitCode int) bool {
	return exitCode == 0
}
```

- [ ] **Step 6: Remove `LogSyncer`/`LogSyncCondition` and simplify the buffering branch using Task 1's `engine.NewBufferGroup()`**

Change the `Hooks`/`RunAllOptions` types:
```go
// Hooks contains hooks that can be used to extend the behavior of the run engine.
type Hooks struct {
	Before           RunBeforeHook
	After            RunAfterHook
	LogSyncer        LogSyncer
	LogSyncCondition LogSyncCondition
}

// RunBeforeHook is a function that is called before a stack is executed by the run engine.
type RunBeforeHook func(engine *Engine, run StackCloudRun)

// RunAfterHook is a function that is called after a stack is executed by the run engine.
type RunAfterHook func(engine *Engine, run StackCloudRun, res RunResult, err error)

// LogSyncer is a function that is called when the cloud API is enabled and the log sync condition is met.
type LogSyncer func(logger *zerolog.Logger, e *Engine, run StackRun, task StackRunTask, logs resources.CommandLogs)

// LogSyncCondition is a function that is used to determine if the log syncer should be enabled for a given task.
type LogSyncCondition func(task StackRunTask, run StackRun) bool
```
to:
```go
// Hooks contains hooks that can be used to extend the behavior of the run engine.
type Hooks struct {
	Before RunBeforeHook
	After  RunAfterHook
}

// RunBeforeHook is a function that is called before a stack is executed by the run engine.
type RunBeforeHook func(engine *Engine, run StackRun, task StackRunTask)

// RunAfterHook is a function that is called after a stack is executed by the run engine.
type RunAfterHook func(engine *Engine, run StackRun, task StackRunTask, res RunResult, err error)
```
(the `StackCloudRun` type — check via `grep -rn "StackCloudRun" .` whether it's now unused anywhere outside `cloudsync/` after this change; if so, remove it too, and update `Before`/`After`'s signature as shown above, which drops the cloud-shaped wrapper type in favor of the plain `StackRun`/`StackRunTask` values it always wrapped.)

Update the default-hooks wiring:
```go
if opts.Hooks == nil {
	opts.Hooks = &Hooks{
		Before:           func(_ *Engine, _ StackCloudRun) {},
		After:            func(_ *Engine, _ StackCloudRun, _ RunResult, _ error) {},
		LogSyncCondition: func(_ StackRunTask, _ StackRank) bool { return false },
	}
}
```
to:
```go
if opts.Hooks == nil {
	opts.Hooks = &Hooks{
		Before: func(_ *Engine, _ StackRun, _ StackRunTask) {},
		After:  func(_ *Engine, _ StackRun, _ StackRunTask, _ RunResult, _ error) {},
	}
}
```

Update every `opts.Hooks.Before(...)`/`opts.Hooks.After(...)` call site in `engine/run.go` to pass `run, task` instead of a constructed `StackCloudRun{...}` value (find via `grep -n "Hooks.Before\|Hooks.After" engine/run.go`).

Change the buffering branch:
```go
	isCloudSync := e.IsCloudEnabled() && opts.Hooks.LogSyncCondition(task, run)

	var logSyncer *cloud.LogSyncer
	if isCloudSync {
		logSyncer = cloud.NewLogSyncer(func(logs resources.CommandLogs) {
			opts.Hooks.LogSyncer(&logger, e, run, task, logs)
		})
	}

	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	waitForBuffersFunc := func() {}

	// In case of cloud sync or parallel, use line buffering for output of child commands
	// so they can interleave.
	if isCloudSync || opts.Parallel > 1 {
		outputBuffers := cloud.NewBufferGroup(logSyncer)
		cmd.Stdout = outputBuffers.NewBuffer(resources.StdoutLogChannel, cmd.Stdout)
		cmd.Stderr = outputBuffers.NewBuffer(resources.StderrLogChannel, cmd.Stderr)
		waitForBuffersFunc = func() { outputBuffers.Wait() }
	}
```
to:
```go
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	waitForBuffersFunc := func() {}

	// When running in parallel, use line buffering for output of child commands
	// so they can interleave without garbling lines.
	if opts.Parallel > 1 {
		outputBuffers := NewBufferGroup()
		cmd.Stdout = outputBuffers.NewBuffer(cmd.Stdout)
		cmd.Stderr = outputBuffers.NewBuffer(cmd.Stderr)
		waitForBuffersFunc = func() { outputBuffers.Wait() }
	}
```
Remove the now-unused `"github.com/terramate-io/terramate/cloud"` and `"github.com/terramate-io/terramate/cloud/api/resources"` imports from `engine/run.go` if nothing else in the file needs them.

- [ ] **Step 7: Remove `http/http.go`'s dependency on `cloud/api/resources`**

Change the generic type constraint from `resources.Resource` to a local, cloud-agnostic constraint. Add near the top of `http/http.go`:
```go
// Resource is a minimal constraint for types that can be sent/received
// through the generic HTTP helpers below.
type Resource any
```
Replace every `resources.Resource` type-parameter constraint (`Get[T resources.Resource]`, `Post[T resources.Resource]`, `Patch[T resources.Resource]`, `Put[T resources.Resource]`, `Delete[T resources.Resource]`, `Request[T resources.Resource]`) with `T Resource` (the new local type). Remove the `"github.com/terramate-io/terramate/cloud/api/resources"` import.

- [ ] **Step 8: Verify the repo builds**

Run: `mise run build`
Expected: exits 0. Resolve any remaining compile errors — at this point they should only be inside `cloudsync/`, `commands/cloud/` (already deleted), or `cloud/` itself, none of which matter since they're deleted wholesale in Task 10. If `go build ./...` (not just `mise run build`, which only builds the two main binaries) still tries to compile `cloudsync/`/`cloud/` and fails, that's expected and resolved by Task 10 — run `go build $(go list ./... | grep -v -e /cloud -e /cloudsync -e /commands/cloud -e /ui/tui/cliauth -e /e2etests/cloud)` instead for this task's checkpoint.

- [ ] **Step 9: Run the engine and commands package tests (excluding cloud-only ones, not yet deleted)**

Run: `mise exec -- go test -race -count=1 ./engine/... ./commands/... ./http/... ./config/...`
Expected: `ok` for all listed packages (compile errors in `commands/cloud/...` won't occur since that directory no longer exists; if `commands/...` still recurses into something cloud-only that fails, note it and proceed — Task 10/11 clean up the remainder).

- [ ] **Step 10: Commit**

```bash
git add -A -- engine/ commands/trigger commands/stack commands/script/info commands/debug/show http/http.go
git commit -m "feat: remove cloud coupling from engine package and its callers"
```

---

### Task 9: Delete `cloud/`, `cloudsync/`, `ui/tui/cliauth/`, `ui/tui/clitest/messages.go`, `test/cloud/`

**Files:**
- Delete: `cloud/`, `cloudsync/`, `ui/tui/cliauth/`, `ui/tui/clitest/messages.go`, `test/cloud/`

**Interfaces:**
- Consumes: Tasks 2-8 having removed every real caller.
- Produces: nothing — this is the payoff wholesale deletion once nothing calls into these packages.

- [ ] **Step 1: Confirm nothing outside these directories still imports them**

Run:
```bash
grep -rln 'terramate-io/terramate/cloud"' --include="*.go" . | grep -v '^./cloud/' | grep -v '^./cloudsync/'
grep -rln 'terramate-io/terramate/cloudsync"' --include="*.go" . | grep -v '^./cloudsync/'
grep -rln 'terramate-io/terramate/ui/tui/cliauth"' --include="*.go" . | grep -v '^./ui/tui/cliauth/'
grep -rln 'terramate-io/terramate/ui/tui/clitest"' --include="*.go" . | grep -v '^./ui/tui/clitest/'
```
Expected: every command's output is empty (no remaining external importers). If any file shows up, stop and go fix that reference in the appropriate earlier task's file rather than proceeding.

- [ ] **Step 2: Delete the packages**

```bash
git rm -r cloud cloudsync ui/tui/cliauth test/cloud
git rm ui/tui/clitest/messages.go
```

- [ ] **Step 3: Check whether `ui/tui/clitest` has any surviving content**

Run: `ls ui/tui/clitest/`
If the directory is now empty, it will already be gone after `git rm` of its only file; confirm with `git status`.

- [ ] **Step 4: Verify the full repo builds**

Run: `mise run build && go build ./...`
Expected: both exit 0.

- [ ] **Step 5: Commit**

```bash
git add -A -- cloud cloudsync ui/tui/cliauth ui/tui/clitest test/cloud
git commit -m "feat: delete cloud, cloudsync, cliauth, and cloud test-fixture packages"
```

---

### Task 10: Delete `e2etests/cloud/` and fix collateral e2e tests, runner infra, and telemetry

**Files:**
- Delete: `e2etests/cloud/` (including `interop/`)
- Modify: `e2etests/core/exp_trigger_test.go`
- Modify: `e2etests/core/run_test.go`
- Modify: `e2etests/internal/runner/runner.go`
- Modify: `ui/tui/telemetry/telemetry.go`
- Modify: `ui/tui/telemetry/telemetry_test.go`

**Interfaces:**
- Consumes: Task 9 (packages these tests exercised are gone).

- [ ] **Step 1: Delete the cloud e2e test directory**

```bash
git rm -r e2etests/cloud
```

- [ ] **Step 2: Simplify `TestTriggerWorksRecursivelyFromRelativeStackPath`**

Change:
```go
func TestTriggerWorksRecursivelyFromRelativeStackPath(t *testing.T) {
	t.Parallel()

	s := sandbox.New(t)
	s.CreateStack("dir/stacks/stack1")
	s.CreateStack("dir/stacks/stack2")
	s.CreateStack("other-stack")
	git := s.Git()
	git.CommitAll("all")
	git.Push("main")
	git.CheckoutNew("trigger-the-stack")

	cli := NewCLI(t, filepath.Join(s.RootDir(), "dir"))
	AssertRunResult(t, cli.Trigger("--recursive", "--status=ok"), RunExpected{
		Status:      1,
		StderrRegex: regexp.QuoteMeta("cloud filters such as --status are incompatible with --recursive flag"),
	})

	AssertRunResult(t, cli.Trigger("--recursive", "--status=ok", "stacks"), RunExpected{
		Status:      1,
		StderrRegex: regexp.QuoteMeta("cloud filters such as --status are incompatible with --recursive flag"),
	})

	AssertRunResult(t, cli.Trigger("--changed", "stacks"), RunExpected{
		Status:      1,
		StderrRegex: regexp.QuoteMeta("path is not a stack and --recursive is not provided"),
	})

	AssertRunResult(t, cli.TriggerRecursively(trigger.Changed, "stacks"), RunExpected{
		StdoutRegex: "Created change trigger",
	})

	git.CommitAll("commit the trigger file")
	want := RunExpected{Stdout: nljoin("stacks/stack1", "stacks/stack2")}
	AssertRunResult(t, cli.ListChangedStacks(), want)
}
```
to (drop the two `--status=ok` cloud-filter assertions, keep everything else):
```go
func TestTriggerWorksRecursivelyFromRelativeStackPath(t *testing.T) {
	t.Parallel()

	s := sandbox.New(t)
	s.CreateStack("dir/stacks/stack1")
	s.CreateStack("dir/stacks/stack2")
	s.CreateStack("other-stack")
	git := s.Git()
	git.CommitAll("all")
	git.Push("main")
	git.CheckoutNew("trigger-the-stack")

	cli := NewCLI(t, filepath.Join(s.RootDir(), "dir"))
	AssertRunResult(t, cli.Trigger("--changed", "stacks"), RunExpected{
		Status:      1,
		StderrRegex: regexp.QuoteMeta("path is not a stack and --recursive is not provided"),
	})

	AssertRunResult(t, cli.TriggerRecursively(trigger.Changed, "stacks"), RunExpected{
		StdoutRegex: "Created change trigger",
	})

	git.CommitAll("commit the trigger file")
	want := RunExpected{Stdout: nljoin("stacks/stack1", "stacks/stack2")}
	AssertRunResult(t, cli.ListChangedStacks(), want)
}
```

- [ ] **Step 3: Rename `TestRunIOBuffering`'s subtest for accuracy**

Change the subtest name (this is cosmetic only — confirmed via research there's no actual cloud logic in this test, the name was just imprecise):
```go
	t.Run("non-cloud, non-parallel is unbuffered", func(t *testing.T) {
```
to:
```go
	t.Run("non-parallel is unbuffered", func(t *testing.T) {
```
Leave the rest of `TestRunIOBuffering` (both subtests, in full) exactly as-is.

- [ ] **Step 4: Remove the fake-credentials block from `e2etests/internal/runner/runner.go`**

Remove from `NewCmd`:
```go
	// fake credentials
	type MyCustomClaims struct {
		Email string `json:"email"`
		jwt.RegisteredClaims
	}

	claims := MyCustomClaims{
		"batman@terramate.io",
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			Issuer:    "terramate-tests",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	fakeJwt, err := token.SignedString([]byte("test"))
	assert.NoError(t, err)
	test.WriteFile(t, tm.userDir, "credentials.tmrc.json", fmt.Sprintf(`{"id_token": "%s", "refresh_token": "abcd", "provider": "Google"}`, fakeJwt))
```

Before removing, run a repo-wide check that nothing depends on this file existing:
```bash
grep -rn "credentials.tmrc.json\|batman@terramate.io" e2etests/
```
If any test asserts on the presence/content of this fake credentials file, that test is itself cloud-coupled and should have been caught in Task 10 Step 1's deletion or needs its own removal here — investigate and remove it alongside this change rather than leaving a broken assertion.

Remove the now-unused `"time"` and `"github.com/golang-jwt/jwt/v5"` imports from `runner.go` if nothing else in the file uses them.

- [ ] **Step 5: Remove `DetectAuthTypeFromEnv`'s Google/GitHub credential-file detection, keep the CI-env-var detection**

Change:
```go
// DetectAuthTypeFromEnv detects AuthType based on environment variables and credentials.
func DetectAuthTypeFromEnv(credpath string) AuthType {
	if isEnvVarSet("ACTIONS_ID_TOKEN_REQUEST_TOKEN") {
		return AuthOIDCGithub
	} else if isEnvVarSet("TM_GITLAB_ID_TOKEN") {
		return AuthOIDCGitlab
	}
	return getAuthProviderFromCredentials(credpath)
}
```
to:
```go
// DetectAuthTypeFromEnv detects AuthType based on environment variables.
func DetectAuthTypeFromEnv() AuthType {
	if isEnvVarSet("ACTIONS_ID_TOKEN_REQUEST_TOKEN") {
		return AuthOIDCGithub
	} else if isEnvVarSet("TM_GITLAB_ID_TOKEN") {
		return AuthOIDCGitlab
	}
	return AuthNone
}
```
Remove `getAuthProviderFromCredentials` entirely (it read the now-deleted `credentials.tmrc.json`, written only by the now-deleted `cliauth` package). Update the caller:
```go
msg.Auth = DetectAuthTypeFromEnv(credfile)
```
to:
```go
msg.Auth = DetectAuthTypeFromEnv()
```
Check `DetectFromEnv`'s own signature (`func DetectFromEnv(credfile, cpsigfile, anasigfile string, ...)`) — if `credfile` is now unused by anything else in that function, remove the parameter and update its callers accordingly (`grep -rn "DetectFromEnv(" ui/tui/`).

- [ ] **Step 6: Update `ui/tui/telemetry/telemetry_test.go`**

Update `TestDetectAuthTypeFromEnv` to match the new signature (drop the credential-file-path argument and the `AuthIDPGoogle`/`AuthIDPGithub` test cases that exercised the now-removed credential-file detection; keep the CI-env-var-based cases).

- [ ] **Step 7: Verify the repo builds**

Run: `mise run build && go build ./...`
Expected: both exit 0.

- [ ] **Step 8: Run the full test suite**

Run: `mise run test`
Expected: passes (matching the same pre-existing, unrelated locale-dependent flake noted in the prior PR's Task 3, if it recurs — that's environment-specific, not caused by this change).

- [ ] **Step 9: Commit**

```bash
git add -A -- e2etests/ ui/tui/telemetry
git commit -m "feat: delete cloud e2e tests, fix collateral test/telemetry cloud coupling"
```

---

### Task 11: CI/tooling cleanup

**Files:**
- Delete: `.github/workflows/ci-sync-preview.yml`, `.github/workflows/ci-sync-deployment.yml`, `.github/workflows/release.yml`, `.github/workflows/interop-tests.yml`
- Modify: `Makefile`, `makefiles/common.mk`, `makefiles/unix.mk`, `makefiles/windows.mk`
- Modify: `mise.toml`
- Modify: `bitbucket-pipelines.yml`

**Interfaces:**
- Consumes: Task 10 (the underlying `test/interop`/`test/sync` targets exercised now-deleted code).

- [ ] **Step 1: Delete the dead Cloud-coupled workflows**

```bash
git rm .github/workflows/ci-sync-preview.yml .github/workflows/ci-sync-deployment.yml .github/workflows/release.yml .github/workflows/interop-tests.yml
```

- [ ] **Step 2: Remove Cloud-only targets from `makefiles/common.mk`**

Remove:
```makefile
## sync the Terramate example stack with a success status.
.PHONY: cloud/sync/ok
cloud/sync/ok: build test/helper
	./bin/terramate --log-level=info		\
			--disable-check-git-untracked   \
			--disable-check-git-uncommitted \
			--tags test \
			run --sync-deployment --  \
			$(PWD)/bin/helper true

## sync the Terramate example stack with a failed status.
.PHONY: cloud/sync/failed
cloud/sync/failed: build test/helper
	./bin/terramate --log-level=info		\
			--disable-check-git-untracked   \
			--disable-check-git-uncommitted \
			--tags test \
			run --sync-deployment --  \
			$(PWD)/bin/helper false
```

- [ ] **Step 3: Remove Cloud-only targets from `makefiles/unix.mk` and adjust `test/build`'s dependency**

Change:
```makefile
## build a test binary -- not static, telemetry sent to localhost, etc
.PHONY: test/build
test/build: test/testserver
	go build -tags localhostEndpoints -o bin/test-terramate ./cmd/terramate

## build bin/testserver
.PHONY: test/testserver
test/testserver:
	go build -o bin/testserver ./cloud/testserver/cmd/testserver
```
to:
```makefile
## build a test binary -- not static, telemetry sent to localhost, etc
.PHONY: test/build
test/build:
	go build -tags localhostEndpoints -o bin/test-terramate ./cmd/terramate
```
Remove:
```makefile
## test/sync code
.PHONY: test/sync
tempdir=$(shell ./bin/helper tempdir)
test/sync: test/helper build
# 	Using `terramate` because it detects and fails if the generated files are outdated.
	TMC_API_HOST=api.stg.terramate.io \
	TM_TEST_ROOT_TEMPDIR=$(tempdir)   \
	TM_CLOUD_ORGANIZATION=test        \
	GITHUB_TOKEN=$(shell cat ../my_github_token.txt) \
	NO_COLOR=1 \
	CI=1 \
	./bin/terramate script run --tags golang --parallel=10 preview || ./bin/helper rm $(tempdir)

## test/interop
.PHONY: test/interop
test/interop: org?=test
test/interop: backend_host?=api.stg.terramate.io
test/interop:
	TM_CLOUD_ORGANIZATION=$(org) TMC_API_HOST=$(backend_host) go test -v -count=1 -tags interop ./e2etests/cloud/interop/...
```

- [ ] **Step 4: Remove Cloud-only target from `makefiles/windows.mk` and adjust `test/build`'s dependency**

Change:
```makefile
## build a test binary -- not static, telemetry sent to localhost, etc
.PHONY: test/build
test/build: test/fakecloud
	go build -tags localhostEndpoints -o bin/test-terramate.exe ./cmd/terramate

## build bin/fakecloud
.PHONY: test/fakecloud
test/fakecloud:
	go build -o bin/fakecloud.exe ./cloud/testserver/cmd/fakecloud
```
to:
```makefile
## build a test binary -- not static, telemetry sent to localhost, etc
.PHONY: test/build
test/build:
	go build -tags localhostEndpoints -o bin/test-terramate.exe ./cmd/terramate
```

- [ ] **Step 5: Mirror the same removals in `mise.toml`**

Remove the `test:testserver`, `test:sync`, `test:interop`, `cloud:sync:ok`, `cloud:sync:failed` task blocks. Update `[tasks."test:build"]` to drop its `depends = ["test:testserver"]` line entirely.

- [ ] **Step 6: Rewrite `bitbucket-pipelines.yml` to drop Cloud sync flags**

Run: `cat bitbucket-pipelines.yml` first to see its current exact content (this file wasn't fully captured during brainstorming research — read it directly before editing). Replace every `terramate run ... --sync-preview ...` / `terramate run ... --sync-deployment --terraform-plan-file ...` invocation with the equivalent plain `terramate run -- terraform plan`/`terramate run -- terraform apply` (no cloud flags), preserving the rest of the pipeline structure (stages, triggers, caches) unchanged.

- [ ] **Step 7: Validate the remaining workflows**

Run: `actionlint .github/workflows/*.yml`
Expected: no new errors compared to before this task (only the four deleted files disappear from the warning set).

- [ ] **Step 8: Verify `mise run build`/`mise run test:build` still work**

Run: `mise run build && mise run test:build`
Expected: both exit 0.

- [ ] **Step 9: Commit**

```bash
git add -A -- .github/workflows Makefile makefiles mise.toml bitbucket-pipelines.yml
git commit -m "chore: remove cloud-coupled CI workflows and Makefile/mise targets"
```

---

### Task 12: `go.mod` cleanup

**Files:**
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Consumes: Tasks 2-11 (all Cloud-only code deleted).

- [ ] **Step 1: Confirm the two ambiguous dependencies are truly unused now**

Run:
```bash
grep -rn "golang-jwt/jwt" --include="*.go" .
grep -rn "hashicorp/go-uuid" --include="*.go" .
```
Expected: no output (both were confirmed cloud-only during brainstorming — `golang-jwt/jwt/v5` was used only by the now-deleted `ui/tui/cliauth` and the now-removed fake-credentials block in `e2etests/internal/runner/runner.go`; `hashicorp/go-uuid` only by the now-deleted `commands/run/cloud.go` and the now-removed `prepareScriptForCloudSync` in `commands/script/run/run.go`).

- [ ] **Step 2: Run `go mod tidy`**

Run: `go mod tidy`
Expected: exits 0.

- [ ] **Step 3: Verify the expected dependencies were dropped**

Run: `git diff go.mod | grep '^-'`
Expected: includes `github.com/google/go-github`, `github.com/shurcooL/githubv4`, `github.com/cli/go-gh/v2`, `github.com/cli/safeexec`, `github.com/pkg/browser`, `golang.org/x/oauth2`, `github.com/julienschmidt/httprouter`, `github.com/terramate-io/tfjson`, `github.com/golang-jwt/jwt/v5`, `github.com/hashicorp/go-uuid` (exact set may vary slightly based on transitive-dependency resolution — review the diff and confirm nothing Terraform/OpenTofu/Terragrunt-related or otherwise unrelated to Cloud was accidentally removed).

- [ ] **Step 4: Verify the repo still builds and tests pass**

Run: `mise run build && mise run test`
Expected: both pass.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: go mod tidy after removing cloud-only dependencies"
```

---

### Task 13: Documentation updates

**Files:**
- Modify: `CLAUDE.md`, `AGENTS.md`, `README.md`

**Interfaces:**
- Consumes: nothing (pure doc cleanup, done last so it reflects the final code state).

- [ ] **Step 1: Update `CLAUDE.md`**

Remove the "Terramate Cloud integration — observability, drift detection, deployment sync" bullet from the Project Overview list. Remove the `cloud/` entry from the Core Packages list. Remove any cloud-sync mention from the `engine/` package description (e.g. "cloud sync" responsibility phrasing) — read the current file first (`cat CLAUDE.md`) since exact wording wasn't captured during brainstorming, and edit precisely rather than guessing.

- [ ] **Step 2: Update `AGENTS.md`**

Remove the `` `/cloud/` - Terramate Cloud integration`` repo-structure bullet (confirmed at `AGENTS.md:17` during brainstorming — verify the line still exists at/near that location before removing, since line numbers may have shifted).

- [ ] **Step 3: Update `README.md`**

Read the full current file first (`cat README.md`) since it wasn't fully captured verbatim during brainstorming — only specific line numbers/regions were identified. Remove:
- The `terramate cloud login` quickstart section (around the previously-identified lines 84-90).
- The "Terramate CLI vs Terramate Cloud" section and its platform-overview images (around line 123, plus the `terramate_platform_overview_{dark,light}.png` image files under whatever `docs`/`assets` directory holds them — find with `git ls-files | grep platform_overview` and `git rm` them too).
- Cloud-only feature bullets: Drift Management, Observability, Misconfiguration Detection, Asset Management, Audit Trail, Slack Integrations (around the previously-identified lines 108-118).
- Keep genuine CLI/codegen feature bullets (e.g. Scaffolding) and the general product description, trimmed of Cloud-specific claims.

- [ ] **Step 4: Verify no more Terramate Cloud references remain in these three files beyond what's intentionally kept**

Run: `grep -n -i "terramate cloud\|cloud sync\|cloud login" CLAUDE.md AGENTS.md README.md`
Expected: no output (or only legitimate remaining mentions you've deliberately decided to keep — review each hit).

- [ ] **Step 5: Commit**

```bash
git add -A -- CLAUDE.md AGENTS.md README.md
git commit -m "docs: remove Terramate Cloud references from CLAUDE.md, AGENTS.md, README.md"
```

---

## Self-Review

**Spec coverage:** Wholesale deletions (spec §1) ✓ Task 9/10. Surgical removals (spec §2, all 15 files) ✓ Tasks 2-8. Test adjustments (spec §3) ✓ Task 10. CI/tooling (spec §4) ✓ Task 11. `go.mod` cleanup (spec §5) ✓ Task 12. Docs (spec §6) ✓ Task 13. The `cloud.BufferGroup` extraction need (discovered during research, not in the original spec) is covered by Task 1 — this is a necessary consequence of the spec's own "keep `--parallel` output buffering working" implicit requirement (never stated as removable) and doesn't contradict the spec's Non-Goals.

**Placeholder scan:** every step has literal code or an exact shell command; the handful of "read the file first, exact wording wasn't captured" notes (Task 11 Step 6, Task 13 Steps 1-3) are honest acknowledgments of files not fully read during brainstorming research, not vague placeholders — each still names the exact file, the exact section to find, and the exact removal criterion.

**Type/name consistency:** `engine.NewBufferGroup()`/`NewBuffer()`/`Wait()` (Task 1) match their exact call shape in Task 8 Step 6. `ListStacks`'s final 2-argument signature (Task 8 Step 2) is used consistently across all 11 call-site updates in the same step. `StackRunTask`'s field removal (Task 8 Step 4) matches every construction site fixed in Tasks 5 and 6. `Hooks.Before`/`After`'s new signature (Task 8 Step 6) is used consistently; the old `StackCloudRun`-based hook wiring in Tasks 5/6 is already replaced by the simpler "omit Hooks" form before Task 8 changes the type, avoiding an ordering conflict (Tasks 5/6 stop constructing `Hooks` with cloud closures; Task 8 changes what `Hooks` even accepts — since Tasks 5/6 no longer set `Hooks` at all by the time Task 8 runs, there's no stale reference to fix).
