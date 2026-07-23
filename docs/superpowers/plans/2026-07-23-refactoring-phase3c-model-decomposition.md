# Phase 3c — Model Decomposition Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Decompose the flat `Model` struct of the interactive TUI (`commands/ui/`) into per-view sub-states with a per-state dispatch table, move the pure bundle loaders into the `commands/ui/change` package, unexport `IsBundleUnique`, and repoint the stale `change.go:NNN` citations left in `change_test.go` by the Phase 3b move — all strictly iso-behavior.

**Architecture:** `Model` stays the single top-level `tea.Model` (forced by the `finalModel.(Model)` assertion in `ui.go:106`), but its 40+ flat fields are regrouped into four owned sub-structs (`overview`, `create`, `reconfig`, `promote`) plus an explicitly-documented shared core. The two `switch m.viewState` dispatches in `Update`/`View` are replaced by one `map[ViewState]viewHandler` table that pairs each state's update and render functions. Pure domain loaders (`newBundleEvalContext`, `makeBundleDefinitionEntry`, `rawInputKeys`, `missingBundleRefs`) move verbatim into `commands/ui/change/loaders.go`; `loadBundleEvalContext` becomes an `EngineState` method (it only ever used `m.EngineState`).

**Tech Stack:** Go 1.25.8 (via mise), Bubble Tea/lipgloss TUI, existing golden-test harness (`assertGolden`, `commands/ui/golden_select_test.go:23-42`).

## Global Constraints

- **Iso-behavior is absolute.** No user-visible behavior change of any kind. The goldens and existing tests are the arbiters.
- **Pre-existing goldens are NEVER regenerated, moved, or edited.** `testdata/golden/*.golden` files that exist at branch start must remain byte-identical. The ONLY golden additions are the four new `overview-*` goldens created in Task 1, generated with a **targeted** `-run TestGoldenOverviewView -update` invocation (never a bare `-update`, which would rewrite every golden whose test runs).
- **Bounded adaptation** for tests: symbol renames and struct-literal re-nesting only, with file:line proof; NEVER weaken or change an assertion or an expected value.
- **NEVER use `-race`.** The Go 1.25.8 TSan runtime dies probabilistically at startup on this kernel (silent SIGKILL). If a test binary dies with a `signal:` line, re-run the same package without extra flags before concluding anything.
- Always run tests with `LC_ALL=C`.
- **rtk output masking:** the `rtk` CLI proxy reformats `go test` output into "✓ Go test: N passed" — piping through `grep FAIL` on that output fails silently (rc=1, zero lines). Read the full plain output; never conclude from an empty grep.
- New files get the MPL header with **2026**; existing files keep their existing year:
  ```go
  // Copyright 2026 Terramate GmbH
  // SPDX-License-Identifier: MPL-2.0
  ```
- Conventional commits in English, footer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`. Pre-commit hooks run goimports/vet/build/golangci-lint — a failing hook means the change is broken, fix it, don't bypass.
- Work happens on branch `refactor/phase3c-model` (created from `refactor/maintenabilite` at `6862b222`).
- All file:line references below are as of `6862b222`.

## Non-Goals (deferred)

- Splitting `inputs_form.go` (1971 lines) — Phase 3d.
- Moving the form-building loaders (`loadBundleDef`, `finalizeBundleWithEnv`, `loadReconfigBundle`, `loadPromoteBundle`) or `buildAllPromoteBundles` out of the view files — they construct `InputsForm` or read filter state and stay UI-side.
- Any field logic change, any new feature, any rendering change.

## File Structure

| File | Action | Responsibility after 3c |
|---|---|---|
| `commands/ui/model.go` | Modify | Sub-state struct definitions, slimmed `Model`, dispatch table, `EngineState` (+ new `loadBundleEvalContext` method), global key handling |
| `commands/ui/change/loaders.go` | Create | Pure domain loaders: `NewBundleEvalContext`, `MakeBundleDefinitionEntry`, `RawInputKeys`, `MissingBundleRefs` |
| `commands/ui/change/helpers.go` | Modify | `IsBundleUnique` → `isBundleUnique` |
| `commands/ui/change/change.go` | Modify | Call site of `isBundleUnique` |
| `commands/ui/view_*.go`, `selectable_list_view.go` | Modify | Mechanical field re-qualification; loaders removed from view files |
| `commands/ui/golden_select_test.go` | Modify | + `TestGoldenOverviewView` (Task 1), literal re-nesting (Task 4) |
| `commands/ui/change_test.go` | Modify | Citation repointing (Task 2), loader symbol adaptation (Task 3) |
| `commands/ui/view_{create_select,reconfig,promote}_test.go` | Modify | Literal re-nesting (Task 4) |
| `commands/ui/testdata/golden/overview-*.golden` | Create (4) | New characterization goldens |

---

### Task 1: Characterize the overview view with goldens

The overview view (`renderOverviewView`, `view_overview.go:250`) has **no golden coverage** today, yet Tasks 4–5 restructure the fields it reads. Freeze its rendering first.

**Files:**
- Modify: `commands/ui/golden_select_test.go` (append test + imports)
- Create: `commands/ui/testdata/golden/overview-{basic,error,session-unfocused,session-focused}.golden`

**Interfaces:**
- Consumes: `assertGolden` (`golden_select_test.go:23`), `goldenEnvs` (`:44`), `goldenBundles` (`:65`), `sessionBundleKey` (`view_overview.go:189`).
- Produces: `TestGoldenOverviewView` + 4 goldens that Tasks 4–5 must keep byte-identical.

- [ ] **Step 1: Append the test to `golden_select_test.go`**

Add `"github.com/terramate-io/terramate/commands/ui/change"` and `"github.com/terramate-io/terramate/errors"` to the import block, then append:

```go
// TestGoldenOverviewView freezes the overview rendering (renderOverviewView,
// view_overview.go) before the phase-3c Model decomposition: command grid,
// inline error area, and the session-history panel in both focus states.
func TestGoldenOverviewView(t *testing.T) {
	s := sandbox.New(t)
	root := s.Config()

	staging, prod := goldenEnvs()
	bundles := goldenBundles(root.HostDir(), staging, prod)
	m := Model{
		EngineState: &EngineState{Root: root, Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:       100,
		height:      32,
		viewState:   ViewOverview,
		commands:    []string{"Scaffold", "Reconfigure", "Promote", "Quit"},
		focus:       FocusCommands,
	}

	assertGolden(t, "overview-basic", m.View())

	// Inline error area below the command grid.
	m.currentErr = errors.E("No bundles available.")
	assertGolden(t, "overview-error", m.View())
	m.currentErr = nil

	// Session-history panel: vpc-1 reconfigured this session and last saved.
	key := sessionBundleKey(bundles[0].Info.HostPath(), bundles[0].Environment)
	m.sessionChanges = map[string][]change.Kind{key: {change.KindReconfig}}
	m.lastSavedKey = key
	assertGolden(t, "overview-session-unfocused", m.View())

	// Same state with the summary panel focused.
	m.focus = FocusSummary
	assertGolden(t, "overview-session-focused", m.View())
}
```

- [ ] **Step 2: Run the test to verify it fails on missing goldens**

Run: `LC_ALL=C go test ./commands/ui/ -run TestGoldenOverviewView -v`
Expected: FAIL with `missing golden testdata/golden/overview-basic.golden`.

- [ ] **Step 3: Generate ONLY the new goldens (targeted -update)**

Run: `LC_ALL=C go test ./commands/ui/ -run TestGoldenOverviewView -update`
Then verify no pre-existing golden changed: `git status --short commands/ui/testdata/` must show exactly 4 untracked `overview-*.golden` files and zero modified files. If any existing golden shows as modified, restore it (`git checkout -- <file>`) and STOP — something is wrong.

Inspect the 4 new goldens by eye: `overview-basic` must show the "Terramate UI / Select an Action" header, the four commands with `› Scaffold` selected, and the help line; `overview-error` adds the error line; the two `overview-session-*` files add the "Recently Changed" panel with `vpc-1 [Staging] reconfigured ✓ saved`.

- [ ] **Step 4: Run the full ui package tests**

Run: `LC_ALL=C go test ./commands/ui/...`
Expected: all tests pass (rtk may render this as "✓ Go test: N passed").

- [ ] **Step 5: Commit**

```bash
git add commands/ui/golden_select_test.go commands/ui/testdata/golden/overview-basic.golden commands/ui/testdata/golden/overview-error.golden commands/ui/testdata/golden/overview-session-unfocused.golden commands/ui/testdata/golden/overview-session-focused.golden
git commit -m "test(ui): characterize overview view rendering with goldens"
```

---

### Task 2: Unexport IsBundleUnique and repoint stale citations

`IsBundleUnique`'s only caller repo-wide is `commands/ui/change/change.go:163` — same package, so the export is unjustified. Separately, `change_test.go` still cites `change.go:NNN` line numbers from the pre-3b monolithic file deleted at `6862b222`.

**Files:**
- Modify: `commands/ui/change/helpers.go:55-56`, `commands/ui/change/change.go:163`
- Modify: `commands/ui/change_test.go` (comments only)

**Interfaces:**
- Consumes: nothing from other tasks.
- Produces: `isBundleUnique` (unexported). No other task references it.

- [ ] **Step 1: Rename `IsBundleUnique` → `isBundleUnique`**

In `helpers.go:55-56` rename the function and its doc comment lead word; in `change.go:163` update the call. Then prove no other caller exists:

Run: `grep -rn "IsBundleUnique" --include="*.go" .`
Expected: zero hits (docs/plans markdown hits are fine and untouched).

- [ ] **Step 2: Repoint the stale citations in `change_test.go` (comments only — no code, no assertions)**

Apply exactly this mapping (line numbers are the comment's location in `change_test.go`; replace only the cited path:range, keep the rest of each comment intact):

| change_test.go line | Stale citation | Replace with |
|---|---|---|
| 133 | `change.go:535` | `change/yamlio.go:126` |
| 358 | `change.go:149-152` | `change/change.go:157-160` |
| 547 | `change.go:596-605` | `change/yamlio.go:159-200` |
| 554 | `change.go:612-622` | `change/yamlio.go:203` |
| 557-558 | `change.go:529, 610` | `change/yamlio.go:201, 120` |
| 558 | `change.go:574-582` | `change/yamlio.go:159-200` |
| 581 | `change.go:571-587` | `change/yamlio.go:159-200` |
| 622 | `change.go:571-587` | `change/yamlio.go:159-200` |
| 626 | `change.go:612-622` | `change/yamlio.go:203` |
| 628 | `change.go:529, 610` | `change/yamlio.go:201, 120` |
| 630 | `change.go:574-582` | `change/yamlio.go:159-200` |
| 666 | `change.go:468-474` | `change/change.go:402-410` (NormalizeBundleRefValues) |
| 351 | `view_promote.go:382-405` | `view_promote.go:357-380` (updatePromoteInput) |

Note: citations at lines 350 and 510 (`loadPromoteBundle (view_promote.go:87-112)`) will be repointed by Task 3 when that loader's neighbors move — leave them for now. Verify completeness:

Run: `grep -n "change\.go:[0-9]" commands/ui/change_test.go`
Expected: zero hits citing a bare `change.go:` (all now say `change/change.go:` or `change/yamlio.go:`).

- [ ] **Step 3: Run the change tests**

Run: `LC_ALL=C go test ./commands/ui/... ./commands/ui/change/...`
Expected: PASS; goldens untouched (`git status --short commands/ui/testdata/` empty).

- [ ] **Step 4: Commit**

```bash
git add commands/ui/change/helpers.go commands/ui/change/change.go commands/ui/change_test.go
git commit -m "refactor(ui): unexport isBundleUnique and repoint stale test citations"
```

---

### Task 3: Move the pure loaders into commands/ui/change

Four functions are pure domain logic (config/eval/cty only, zero TUI types) and one is `EngineState`-only. Move them out of the view files. **Every body moves verbatim** — the only permitted diffs are: function name (export), receiver removal, package-qualification of moved siblings, and the `est := m.EngineState` line dropped where the receiver changes.

**Files:**
- Create: `commands/ui/change/loaders.go`
- Modify: `commands/ui/model.go` (delete `rawInputKeys` :250-273; add `loadBundleEvalContext` method after `changeSession` :117)
- Modify: `commands/ui/view_create_select.go` (delete `newBundleEvalContext` :298-313; update callers :193, :243)
- Modify: `commands/ui/view_reconfig.go` (delete `loadBundleEvalContext` :151-163 and `makeBundleDefinitionEntry` :165-181; update callers :92, :94, :109)
- Modify: `commands/ui/view_promote.go` (delete `missingBundleRefs` :223-266; update callers :91, :93, :108, :134, :195; repoint the two `loadPromoteBundle` comment ranges cited from change_test.go:350/510 if the function's line numbers shift)
- Modify: `commands/ui/change_test.go` (symbol adaptation only, lines 245, 311, 315, 443, 512, 516)

**Interfaces:**
- Consumes: `change.Session` unchanged; `EngineState` fields `Evalctx`, `Root`, `ResolveAPI`, `Registry`.
- Produces (used by Tasks 4–5 unchanged, and by view files from now on):
  - `change.NewBundleEvalContext(evalctx *eval.Context, reg *config.Registry, env *config.Environment) *eval.Context`
  - `change.MakeBundleDefinitionEntry(root *config.Root, b *config.Bundle) *config.BundleDefinitionEntry`
  - `change.RawInputKeys(b *config.Bundle, evalctx *eval.Context) map[string]bool`
  - `change.MissingBundleRefs(b *config.Bundle, targetAliases map[string]bool) []string`
  - `(est *EngineState) loadBundleEvalContext(bde *config.BundleDefinitionEntry, env *config.Environment) (typeschema.EvalContext, error)` (package ui)

- [ ] **Step 1: Create `commands/ui/change/loaders.go`**

2026 MPL header, `package change`, then the four functions moved **verbatim** from their sources with these renames (keep each original doc comment, updating only the leading identifier):

| Old symbol | Source (verbatim body) | New symbol |
|---|---|---|
| `newBundleEvalContext` | `view_create_select.go:298-313` | `NewBundleEvalContext` |
| `makeBundleDefinitionEntry` | `view_reconfig.go:165-181` | `MakeBundleDefinitionEntry` |
| `rawInputKeys` | `model.go:250-273` | `RawInputKeys` |
| `missingBundleRefs` | `view_promote.go:223-266` | `MissingBundleRefs` |

Imports: whatever the four bodies need (`context`, `github.com/terramate-io/terramate/config`, `github.com/terramate-io/terramate/hcl/eval`, `github.com/terramate-io/terramate/stdlib` if used by `newBundleEvalContext`'s original import set, `github.com/zclconf/go-cty/cty`) — copy them from the source files' import blocks; goimports settles the rest.

- [ ] **Step 2: Convert `loadBundleEvalContext` to an `EngineState` method**

Delete `view_reconfig.go:151-163` and add to `model.go`, directly after `changeSession` (:117), the same body with receiver `(est *EngineState)`, the `est := m.EngineState` line removed, and `newBundleEvalContext(...)` → `change.NewBundleEvalContext(...)`. Add the imports `model.go` now needs (`typeschema`, `errors` — copy the exact error-wrapping line as-is from the source).

- [ ] **Step 3: Update all callers**

Production (mechanical, whole-word):
- `view_create_select.go:193, :243` — `newBundleEvalContext(` → `change.NewBundleEvalContext(`
- `view_reconfig.go:92`, `view_promote.go:91` — `makeBundleDefinitionEntry(` → `change.MakeBundleDefinitionEntry(`
- `view_reconfig.go:94`, `view_promote.go:93` — `m.loadBundleEvalContext(` → `m.EngineState.loadBundleEvalContext(`
- `view_reconfig.go:109`, `view_promote.go:108` — `rawInputKeys(` → `change.RawInputKeys(`
- `view_promote.go:134, :195` — `missingBundleRefs(` → `change.MissingBundleRefs(`

Tests (bounded adaptation, same renames): `change_test.go:245, :443` (`newBundleEvalContext`), `:311, :512` (`makeBundleDefinitionEntry`), `:315, :516` (`m.loadBundleEvalContext(` / `m2.loadBundleEvalContext(` → `est.loadBundleEvalContext(` / `est2.loadBundleEvalContext(` — the tests already hold the `*EngineState` variable). Update any adjacent comment that names the old file location of a moved function (e.g. `view_promote.go:87-112` for `loadPromoteBundle` at lines 350/510 — recheck the real new range with grep).

- [ ] **Step 4: Verify the move is byte-faithful and green**

Run: `go build ./commands/ui/... && go vet ./commands/ui/...`
Expected: clean.
Run: `LC_ALL=C go test ./commands/ui/... ./commands/ui/change/...`
Expected: PASS, `git status --short commands/ui/testdata/` empty (no golden churn).
Reviewer check (mechanical): applying the rename table as a sed transform to the deleted bodies must yield the added bodies byte-identically.

- [ ] **Step 5: Commit**

```bash
git add commands/ui/change/loaders.go commands/ui/model.go commands/ui/view_create_select.go commands/ui/view_reconfig.go commands/ui/view_promote.go commands/ui/change_test.go
git commit -m "refactor(ui): move pure bundle loaders into commands/ui/change"
```

---

### Task 4: Group Model state into per-view sub-states

Purely mechanical re-nesting: no field logic changes, no renames beyond stripping the group prefix, no behavior change. The compiler plus the goldens are the safety net.

**Files:**
- Modify: `commands/ui/model.go` (struct definitions + `NewModel`)
- Modify: `commands/ui/view_overview.go`, `view_create.go`, `view_create_select.go`, `view_reconfig.go`, `view_promote.go`, `selectable_list_view.go` (and any other `commands/ui/*.go` file the compiler flags) — apply the substitution table
- Modify: `commands/ui/golden_select_test.go`, `view_create_select_test.go`, `view_reconfig_test.go`, `view_promote_test.go` (literal re-nesting)

**Interfaces:**
- Consumes: Task 3's final `model.go` layout.
- Produces: sub-state types `overviewState`, `createState`, `reconfigState`, `promoteState`; Model fields `overview`, `create`, `reconfig`, `promote`. Task 5 relies on nothing from this task beyond compilation.

- [ ] **Step 1: Rewrite the state structs in `model.go`**

Replace the `Model` struct (`model.go:120-191` as of `6862b222`; adjust for Task 3's edits) with the following — field comments are carried over verbatim from the old struct:

```go
// overviewState groups the Model fields owned by the overview view.
type overviewState struct {
	focus         FocusArea
	commandIdx    int
	commands      []string
	summaryCursor int   // Selected row in the session bundles list
	currentErr    error // Shown in the overview error area, cleared on next keypress
}

// createState groups the Model fields owned by the create flow
// (flat bundle selection, environment selection, and the create wizard).
type createState struct {
	// Bundle selection state (flat list)
	allFlatBundles       []flatBundleEntry // Unfiltered master list, rebuilt each time Scaffold is entered
	flatBundles          []flatBundleEntry // Filtered view of allFlatBundles for the current filter query
	flatBundleFilter     textFilter        // Free-text filter state for the flat bundle list
	flatBundleCursor     int
	selectedCollIdx      int // Set by selectFlatBundle, used by loadBundleDef
	selectedBundleIdx    int // Set by selectFlatBundle, used by loadBundleDef
	selectedBundleSource string
	bundleSelectErr      string // Inline error shown in the bundle list view, cleared on cursor move
	envCursor            int    // Cursor for ViewCreateEnvSelect

	// Wizard exit confirmation
	confirmingExit bool // true when showing wizard exit confirmation
	exitConfirmIdx int  // 0 = Yes, 1 = No

	// Bundle reference / nested creation state
	stack          []CreateFrame // Stack of suspended wizard states
	nestedRefClass string        // When non-empty, we're creating a bundle for this class
}

// reconfigState groups the Model fields owned by the reconfigure flow.
type reconfigState struct {
	bundles      []*config.Bundle // Filtered bundles for current filter, rebuilt on filter change
	cursor       int              // Cursor in bundles
	bundle       *config.Bundle   // The bundle currently being reconfigured
	fromOverview bool             // true when reconfig was entered from session panel (skip ViewReconfigSelect on ESC)
	envFilter    envFilterCycle   // Precomputed valid env filter states + cycle position
	filter       textFilter       // Free-text filter state for the Reconfigure bundle list
}

// promoteState groups the Model fields owned by the promote flow.
type promoteState struct {
	bundles    []*config.Bundle      // Filtered bundles for current filter
	targetEnvs []*config.Environment // Target env per bundle (parallel to bundles)
	cursor     int                   // Cursor in bundles
	bundle     *config.Bundle        // The bundle currently being promoted
	envFilter  envFilterCycle        // Precomputed valid env filter states + cycle position
	filter     textFilter            // Free-text filter state for the Promote bundle list
}

// Model is the main BubbleTea model for the prompt UI.
type Model struct {
	// Layout
	width  int
	height int

	// Common state
	EngineState *EngineState

	// View state
	viewState ViewState

	// Per-view sub-states
	overview overviewState
	create   createState
	reconfig reconfigState
	promote  promoteState

	// Shared selection/form state, written by the select views and read by
	// the input views across the create/reconfig/promote flows.
	selectedEnv            *config.Environment
	selectedBundleDefEntry *config.BundleDefinitionEntry
	inputsForm             InputsForm
	objectEditStack        []ObjectEditFrame // Stack for nested object input editing

	// Session history, appended on every save; read by the overview panel
	// and by the CLI exit path (ui.go).
	changeLog      []string                 // cumulative log of all saved changes across the session (for CLI exit)
	sessionChanges map[string][]change.Kind // bundle key → ordered list of change kinds applied this session
	lastSavedKey   string                   // bundle key of the most recently saved change (cleared on next keypress)

	// Transient status
	ctrlCPending     bool   // true after first ctrl+c press, reset after 1s
	errorDialogTitle string // Title for the error dialog (e.g. "Bundle is not enabled")
	errorDialogText  string // When non-empty, shows a dismissible error dialog overlay

	// Result
	err       error
	cancelled bool
}
```

And `NewModel` becomes:

```go
// NewModel creates a new prompt model.
func NewModel(est *EngineState) Model {
	return Model{
		EngineState: est,
		viewState:   ViewOverview,
		overview: overviewState{
			commands: []string{
				"Scaffold",
				"Reconfigure",
				"Promote",
				"Quit",
			},
			focus: FocusCommands,
		},
	}
}
```

- [ ] **Step 2: Apply the substitution table across `commands/ui/*.go` (production + tests)**

Whole-word (`\b`) substitutions, **longest pattern first** within each group. Receivers are uniformly named `m` (including the `*Model` callbacks in the `listViewConfig` vars), so `m.` prefixes cover every access. `frame.`/`est.`/`c.` accesses and the `CreateFrame` struct are untouched.

| Old token | New token |
|---|---|
| `m.summaryCursor` | `m.overview.summaryCursor` |
| `m.currentErr` | `m.overview.currentErr` |
| `m.commandIdx` | `m.overview.commandIdx` |
| `m.commands` | `m.overview.commands` |
| `m.focus` | `m.overview.focus` |
| `m.allFlatBundles` | `m.create.allFlatBundles` |
| `m.flatBundleFilter` | `m.create.flatBundleFilter` |
| `m.flatBundleCursor` | `m.create.flatBundleCursor` |
| `m.flatBundles` | `m.create.flatBundles` |
| `m.selectedCollIdx` | `m.create.selectedCollIdx` |
| `m.selectedBundleIdx` | `m.create.selectedBundleIdx` |
| `m.selectedBundleSource` | `m.create.selectedBundleSource` |
| `m.bundleSelectErr` | `m.create.bundleSelectErr` |
| `m.createEnvCursor` | `m.create.envCursor` |
| `m.confirmingCreateExit` | `m.create.confirmingExit` |
| `m.createExitConfirmIdx` | `m.create.exitConfirmIdx` |
| `m.createStack` | `m.create.stack` |
| `m.nestedRefClass` | `m.create.nestedRefClass` |
| `m.reconfigBundles` | `m.reconfig.bundles` |
| `m.reconfigCursor` | `m.reconfig.cursor` |
| `m.reconfigBundle` | `m.reconfig.bundle` |
| `m.reconfigFromOverview` | `m.reconfig.fromOverview` |
| `m.reconfigEnvFilter` | `m.reconfig.envFilter` |
| `m.reconfigFilter` | `m.reconfig.filter` |
| `m.promoteBundles` | `m.promote.bundles` |
| `m.promoteTargetEnvs` | `m.promote.targetEnvs` |
| `m.promoteCursor` | `m.promote.cursor` |
| `m.promoteBundle` | `m.promote.bundle` |
| `m.promoteEnvFilter` | `m.promote.envFilter` |
| `m.promoteFilter` | `m.promote.filter` |

CRITICAL ordering traps: `m.reconfigBundles` before `m.reconfigBundle`; `m.promoteBundles` before `m.promoteBundle`; `m.flatBundleFilter`/`m.flatBundleCursor` before `m.flatBundles`; `m.selectedBundleDefEntry` must remain UNCHANGED (whole-word matching on the three `selectedBundle*` patterns above protects it — verify with `grep -n "m.create.selectedBundleDefEntry"` → must be zero hits). Model literals in tests do not use the `m.` prefix — Step 3 handles them by hand. Do NOT sed field names inside the new struct definitions from Step 1.

After substituting, fix the handful of non-`m.` sites the compiler flags (e.g. `m2.` in tests, if any) the same way, and update any comment that names a regrouped field where it would now mislead.

- [ ] **Step 3: Re-nest the test Model literals**

Exact replacements (assertions untouched):

`golden_select_test.go` — `TestGoldenReconfigSelectView`:
```go
	m := Model{
		EngineState: &EngineState{Root: root, Registry: &config.Registry{Bundles: bundles, Environments: []*config.Environment{staging, prod}}},
		width:       100,
		height:      32,
		viewState:   ViewReconfigSelect,
		reconfig: reconfigState{
			envFilter: envFilterCycle{filters: []envFilterState{{env: staging, label: "Staging", shortID: "staging"}}, pos: -1},
			filter:    newTextFilter(),
		},
	}
	m.reconfig.bundles = m.buildReconfigBundles()
```
(and below: `m.reconfig.envFilter.pos`, `m.reconfig.filter.input.SetValue`, `m.reconfig.cursor` — per the table.)

`TestGoldenPromoteSelectView`: same shape with `promote: promoteState{envFilter: …, filter: newTextFilter()}` then `m.promote.bundles, m.promote.targetEnvs = m.buildAllPromoteBundles()`, `m.promote.cursor`.

`TestGoldenCreateSelectView`:
```go
	m := Model{
		EngineState: &EngineState{Registry: &config.Registry{}, Collections: []*manifest.Collection{coll}},
		width:       100,
		height:      32,
		viewState:   ViewCreateSelect,
		create:      createState{allFlatBundles: entries, flatBundleFilter: newTextFilter()},
	}
```
(then `m.create.flatBundleFilter.input.SetValue`, `m.create.bundleSelectErr`.)

`TestGoldenOverviewView` (from Task 1):
```go
		overview: overviewState{
			commands: []string{"Scaffold", "Reconfigure", "Promote", "Quit"},
			focus:    FocusCommands,
		},
```
(then `m.overview.currentErr`, `m.overview.focus = FocusSummary`; `m.sessionChanges`/`m.lastSavedKey` stay flat.)

`view_create_select_test.go:70`:
```go
	m := Model{viewState: ViewCreateSelect, create: createState{allFlatBundles: entries, flatBundleFilter: newTextFilter()}}
```

`view_reconfig_test.go` (:24-25, :44-46) and `view_promote_test.go` (:34-35): re-nest their `reconfig*`/`promote*` literal fields into `reconfig: reconfigState{…}` / `promote: promoteState{…}` identically; direct post-literal assignments follow the substitution table. `change_test.go`'s `Model{EngineState: …}` literals use no regrouped fields — untouched.

- [ ] **Step 4: Compile-and-fix loop, then full test run**

Run: `go build ./commands/ui/...` — iterate until clean, fixing ONLY qualification (never logic).
Run: `LC_ALL=C go test ./commands/ui/... ./commands/ui/change/...`
Expected: PASS. Then: `git status --short commands/ui/testdata/` → empty; the 12 pre-existing + 4 new goldens are all byte-identical, proving iso-rendering.

- [ ] **Step 5: Commit**

```bash
git add -A commands/ui
git commit -m "refactor(ui): group Model state into per-view sub-states"
```

---

### Task 5: Route Update/View dispatch through a handler table

Replace the twin `switch m.viewState` blocks (`model.go:319-336` and `:353-370` pre-3c numbering) with one table pairing each state's update and render functions. Fallback semantics are preserved exactly: any state not in the table (i.e. `ViewOverview` or an out-of-range value) routes to the overview, matching today's `default:` branches.

**Files:**
- Modify: `commands/ui/model.go`

**Interfaces:**
- Consumes: the per-view methods exactly as they exist after Task 4 (`updateCreateSelect`, `renderBundleSelectView`, etc. — all value-receiver `Model` methods).
- Produces: `viewHandler`, `viewHandlers` (package-private; no other task uses them).

- [ ] **Step 1: Add the table to `model.go`** (place directly above `Update`)

```go
// viewHandler pairs the update and render functions of one view state.
type viewHandler struct {
	update func(Model, tea.KeyMsg) (tea.Model, tea.Cmd)
	render func(Model) string
}

// viewHandlers routes Update/View dispatch per view state. States absent
// from the table (ViewOverview, or an out-of-range value) fall back to the
// overview handlers, preserving the previous switch defaults.
var viewHandlers = map[ViewState]viewHandler{
	ViewCreateSelect:    {Model.updateCreateSelect, Model.renderBundleSelectView},
	ViewCreateEnvSelect: {Model.updateCreateEnvSelect, Model.renderCreateEnvSelectView},
	ViewCreateInput:     {Model.updateCreateInput, Model.renderCreateInputView},
	ViewReconfigSelect:  {Model.updateReconfigSelect, Model.renderReconfigSelectView},
	ViewReconfigInput:   {Model.updateReconfigInput, Model.renderReconfigInputView},
	ViewPromoteSelect:   {Model.updatePromoteSelect, Model.renderPromoteSelectView},
	ViewPromoteInput:    {Model.updatePromoteInput, Model.renderPromoteInputView},
}
```

- [ ] **Step 2: Replace the dispatch switches**

In `Update`, the `tea.KeyMsg` case's `switch m.viewState { … }` block becomes:

```go
		if h, ok := viewHandlers[m.viewState]; ok {
			return h.update(m, msg)
		}
		return m.updateOverview(msg)
```

In `View`, the `switch m.viewState { … }` block becomes:

```go
	var base string
	if h, ok := viewHandlers[m.viewState]; ok {
		base = h.render(m)
	} else {
		base = m.renderOverviewView()
	}
```

The `default:` (non-key message) branch of `Update` — the small `switch m.viewState { case ViewCreateInput, ViewReconfigInput, ViewPromoteInput: … }` forwarding to `inputsForm.Update` — is about message forwarding, not view dispatch, and stays EXACTLY as it is.

- [ ] **Step 3: Verify**

Run: `go build ./commands/ui/... && go vet ./commands/ui/...` — clean.
Run: `LC_ALL=C go test ./commands/ui/... ./commands/ui/change/...` — PASS, `git status --short commands/ui/testdata/` empty.

- [ ] **Step 4: Commit**

```bash
git add commands/ui/model.go
git commit -m "refactor(ui): route view dispatch through a per-state handler table"
```

---

### Task 6: Whole-phase verification (controller-direct, no subagent)

- [ ] `make build` — clean.
- [ ] `LC_ALL=C make test` — exit 0; inspect the full log for `FAIL` and `signal:` lines (never trust an rtk-piped grep; a TSan `signal: killed` on this kernel means re-run that package without `-race`).
- [ ] `make lint/all` — 0 issues.
- [ ] Goldens vs `6862b222`: `git diff --stat 6862b222 -- commands/ui/testdata/` shows exactly 4 added `overview-*.golden` files and zero modifications.
- [ ] Blast radius: `git diff --stat 6862b222` touches only `commands/ui/**`, `docs/**`, `.superpowers/**` — nothing outside the TUI, so the non-TUI CLI surface is untouched by construction.
- [ ] Final whole-branch review (opus) per subagent-driven-development, then the finishing-a-development-branch menu.
