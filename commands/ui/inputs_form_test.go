// Copyright 2026 Terramate GmbH
// SPDX-License-Identifier: MPL-2.0

package ui

import (
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/zclconf/go-cty/cty"

	"github.com/terramate-io/terramate/config"
	"github.com/terramate-io/terramate/hcl/eval"
	"github.com/terramate-io/terramate/typeschema"
)

func strInput(name, prompt string) *config.InputDefinition {
	return &config.InputDefinition{
		Name:   name,
		Type:   &typeschema.PrimitiveType{Name: "string"},
		Prompt: config.PromptConfig{Text: prompt},
	}
}

func boolInput(name, prompt string) *config.InputDefinition {
	return &config.InputDefinition{
		Name:   name,
		Type:   &typeschema.PrimitiveType{Name: "bool"},
		Prompt: config.PromptConfig{Text: prompt},
	}
}

func newTestForm(defs ...*config.InputDefinition) InputsForm {
	return NewInputsForm(defs, typeschema.EvalContext{Evalctx: eval.NewContext(nil)}, &config.Registry{}, nil)
}

func pressKeys(f InputsForm, msgs ...tea.Msg) InputsForm {
	for _, msg := range msgs {
		f, _ = f.Update(msg)
	}
	return f
}

func runes(s string) tea.Msg      { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func enter() tea.Msg              { return tea.KeyMsg{Type: tea.KeyEnter} }
func keyOf(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func TestInputsFormFiltersUnpromptedInputs(t *testing.T) {
	t.Parallel()
	silent := &config.InputDefinition{Name: "silent", Type: &typeschema.PrimitiveType{Name: "string"}}
	f := newTestForm(strInput("shown", "Shown?"), boolInput("flag", "Flag?"), silent)
	if got := len(f.InputDefs); got != 2 {
		t.Fatalf("expected only prompted inputs to be kept, got %d defs", got)
	}
	if f.InputDefs[0].Name != "shown" || f.InputDefs[1].Name != "flag" {
		t.Fatalf("expected 'shown' and 'flag' to survive filtering in order, got [%q %q]",
			f.InputDefs[0].Name, f.InputDefs[1].Name)
	}
}

func TestInputsFormTypeConfirmAdvancesAndAccepts(t *testing.T) {
	t.Parallel()
	f := newTestForm(strInput("name", "Name?"), strInput("region", "Region?"))
	if f.State() != InputsFormActive {
		t.Fatalf("fresh form should be active, got %v", f.State())
	}

	// Fill input 1, confirm; fill input 2, confirm.
	f = pressKeys(f, runes("my-bundle"), enter(), runes("fr-par"), enter())
	if f.State() != InputsFormActive {
		t.Fatalf("form should still be active on the buttons panel, got %v", f.State())
	}

	// All inputs done: Enter on the (default) Confirm button accepts.
	f = pressKeys(f, enter())
	if f.State() != InputsFormAccepted {
		t.Fatalf("expected InputsFormAccepted after confirming, got %v", f.State())
	}

	vals := f.Values()
	if got := vals["name"]; got != cty.StringVal("my-bundle") {
		t.Fatalf("expected name=my-bundle, got %#v", got)
	}
	if got := vals["region"]; got != cty.StringVal("fr-par") {
		t.Fatalf("expected region=fr-par, got %#v", got)
	}
}

// TestInputsFormUserValuesOnlyContainUserSetKeys characterizes NewInputsFormWithValues.
//
// ADJUSTMENT vs the brief: NewInputsFormWithValues sets activeIdx = len(prompted)
// unconditionally (inputs_form.go:234), so the form starts directly on the
// completed/buttons panel (focus = InputFocusCompleted) — there is no sequential
// per-input "confirm the pre-filled entry" step to drive here, even though
// "typed" has no value yet (allInputsDone() is purely activeIdx-based, see
// inputs_form.go:369-371). Pressing Enter on an entry in the completed panel
// calls ReenterAt (inputs_form.go:1015), which re-opens it for editing — it does
// not "confirm" it. So instead of pressing Enter on "kept" first, we navigate
// down to "typed" and edit only that one.
func TestInputsFormUserValuesOnlyContainUserSetKeys(t *testing.T) {
	t.Parallel()
	// Seed one value as pre-existing (reconfigure-style), set the other by typing.
	defs := []*config.InputDefinition{strInput("kept", "Kept?"), strInput("typed", "Typed?")}
	seeded := map[string]cty.Value{"kept": cty.StringVal("from-disk")}
	f := NewInputsFormWithValues(defs, typeschema.EvalContext{Evalctx: eval.NewContext(nil)}, &config.Registry{}, nil, nil, seeded, seeded, map[string]bool{"kept": true})

	f = pressKeys(f, keyOf(tea.KeyDown))    // move cursor from "kept" to "typed"
	f = pressKeys(f, enter())               // re-enter "typed" (unfilled) for editing
	f = pressKeys(f, runes("abc"), enter()) // type + confirm "typed"

	user := f.UserValues()
	if _, ok := user["typed"]; !ok {
		t.Fatalf("expected typed key in UserValues, got %#v", user)
	}
	if _, ok := user["kept"]; !ok {
		t.Fatalf("expected user-flagged seeded key in UserValues, got %#v", user)
	}
}

// TestInputsFormDiscardFlow characterizes the two-step discard confirmation.
//
// ADJUSTMENT vs the brief: the confirmation prompt defaults its selection to
// "No" (discardConfirmIdx = 1, see inputs_form.go:1169 and :625), so a second
// bare Enter cancels the discard rather than confirming it. Confirming requires
// first moving the selection to "Yes" (KeyLeft, inputs_form.go:1079-1082) before
// pressing Enter.
func TestInputsFormDiscardFlow(t *testing.T) {
	t.Parallel()
	f := newTestForm(strInput("only", "Only?"))
	f = pressKeys(f, runes("v"), enter()) // reach the buttons panel

	// Navigate to the Discard button, then Enter triggers the confirmation prompt.
	f = pressKeys(f, keyOf(tea.KeyRight), enter())
	if f.State() == InputsFormDiscarded {
		t.Fatal("discard must require a confirmation step, got discarded on first Enter")
	}
	if !f.IsConfirmingDiscard() {
		t.Fatal("expected the discard confirmation prompt to be active")
	}

	// The confirmation prompt defaults to "No" — a bare Enter here would cancel.
	// Select "Yes" explicitly before confirming.
	f = pressKeys(f, keyOf(tea.KeyLeft), enter())
	if f.State() != InputsFormDiscarded {
		t.Fatalf("expected InputsFormDiscarded after confirming, got %v", f.State())
	}
}

// TestInputsFormTabTogglesFocusAndReeditWorks characterizes Tab-driven navigation
// combined with re-editing an entry from the completed panel.
//
// ADJUSTMENT vs the brief: Tab does not put the cursor on the first completed
// entry — it positions the cursor on whatever input is currently active
// (inputs_form.go:840-848: "Position cursor on the currently active input"),
// which here is the still-pending "second" input, not the completed "first"
// one. An extra Up press is needed to move the cursor onto "first" before
// Enter re-edits it (updateCompleted's KeyUp handling, inputs_form.go:928-936).
func TestInputsFormTabTogglesFocusAndReeditWorks(t *testing.T) {
	t.Parallel()
	f := newTestForm(strInput("first", "First?"), strInput("second", "Second?"))
	f = pressKeys(f, runes("one"), enter()) // "first" completed, "second" active

	// Tab moves focus to the completed panel, cursor lands on the active
	// ("second") entry; Up moves it onto the completed "first" entry. Enter
	// re-edits the entry under the cursor; typing + Enter updates it and
	// returns to the pending input.
	f = pressKeys(f, keyOf(tea.KeyTab))
	f = pressKeys(f, keyOf(tea.KeyUp))
	f = pressKeys(f, enter())             // re-enter "first"
	f = pressKeys(f, runes("!"), enter()) // append and confirm
	got := f.Values()["first"]
	if got != cty.StringVal("one!") {
		t.Fatalf("expected re-edited value 'one!', got %#v", got)
	}
}

// TestGoldenInputsFormViews captures the rendered View() of InputsForm across
// five representative states, so the upcoming split of inputs_form.go (phase
// 3d) can be checked against these goldens for regressions.
//
// ADJUSTMENT vs the brief: SetValidationError takes an `error`, not a
// string (inputs_form.go:631: `func (f *InputsForm) SetValidationError(err
// error)`; callers in view_create.go/view_promote.go/view_reconfig.go all
// pass a genuine error value) — the brief's snippet calls it with a string
// literal, which does not compile. Wrapped the fixture message with
// errors.New instead. The bool-widget Enter sequence in state 3 needed no
// adjustment: BoolWidget.Update (widget_primitive.go:169-182) confirms
// whatever w.cursor currently is on tea.KeyEnter (defaulting to false/"No"
// via Prepare, widget_primitive.go:152-166), so a bare enter() after the
// first string input does confirm the bool value and advance, exactly as
// the brief assumed.
//
// Second ADJUSTMENT: the brief's state-5 sequence (just one input
// completed) never renders the error at all. View() (inputs_form.go:1206)
// only takes the renderSingleCompletedPanel path — the sole place that
// prints f.validationErr (inputs_form.go:1446-1449) — when
// f.allInputsDone() is true; otherwise it renders the two-panel
// active/completed layout, which has no error slot. This matches the real
// call sites (view_create.go:85, view_promote.go:389, view_reconfig.go:332):
// they call SetValidationError only after Confirm is pressed on a fully
// completed form. So state 5 reuses the state-3 "all inputs done" sequence
// before setting the error, instead of stopping after the first input.
func TestGoldenInputsFormViews(t *testing.T) {
	mk := func() InputsForm {
		f := newTestForm(
			strInput("name", "Bundle name?"),
			boolInput("enabled", "Enable the thing?"),
			strInput("region", "Region?"),
		)
		f.PanelWidth = 100
		f.PanelHeight = 24
		return f
	}

	// 1. Fresh form, first input active.
	f := mk()
	assertGolden(t, "inputs-form-active-first", f.View())

	// 2. One input completed, second active.
	f = pressKeys(mk(), runes("my-bundle"), enter())
	assertGolden(t, "inputs-form-one-completed", f.View())

	// 3. All inputs done: buttons panel.
	f = pressKeys(mk(), runes("my-bundle"), enter(), enter(), runes("fr-par"), enter())
	assertGolden(t, "inputs-form-buttons", f.View())

	// 4. Focus on the completed panel (Tab from state 2).
	f = pressKeys(mk(), runes("my-bundle"), enter(), keyOf(tea.KeyTab))
	assertGolden(t, "inputs-form-completed-focus", f.View())

	// 5. Validation error banner. The error only renders on the
	// all-inputs-done (buttons) panel — see the ADJUSTMENT note above — so
	// this reuses state 3's sequence before setting the error.
	f = pressKeys(mk(), runes("my-bundle"), enter(), enter(), runes("fr-par"), enter())
	f.SetValidationError(errors.New("something went wrong: characterization fixture"))
	assertGolden(t, "inputs-form-validation-error", f.View())
}
