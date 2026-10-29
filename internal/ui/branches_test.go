package ui

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"
	"fmt"
	git "github.com/kamaal111/namu/internal/git"
	"io"
	"testing"
	"time"
)

func check(t *testing.T, value bool, message string) {
	t.Helper()
	if !value {
		t.Fatal(message)
	}
}
func TestBranchPickerFuzzySearchAndRemoteLabels(t *testing.T) {
	m := newModel([]git.Branch{{Ref: "refs/heads/main", Name: "main"}, {Ref: "refs/remotes/origin/feature/cards", Name: "feature/cards", Remote: "origin"}})
	m.input.SetValue("ORGFC")
	filtered := m.filtered()
	check(t, len(filtered) == 1, "expected fuzzy match")
	check(t, filtered[0].Label() == "origin/feature/cards", "must preserve remote label")
}
func TestBranchPickerNavigationAndSelection(t *testing.T) {
	m := newModel([]git.Branch{{Ref: "refs/heads/main", Name: "main"}, {Ref: "refs/heads/feature/cards", Name: "feature/cards"}})
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selected, _ := next.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	result, ok := selected.(model)
	check(t, ok, "unexpected model")
	check(t, result.selected == "refs/heads/feature/cards", "wrong selected branch")
}
func TestBranchPickerEscapeDoesNotSelect(t *testing.T) {
	m := newModel([]git.Branch{{Ref: "refs/heads/main", Name: "main"}})
	next, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	result, ok := next.(model)
	check(t, ok, "unexpected model")
	check(t, result.selected == "" && result.canceled, "cancel must not select")
}
func TestBranchPickerEmptySearchAndSmallWindow(t *testing.T) {
	m := newModel([]git.Branch{{Ref: "refs/heads/main", Name: "main"}})
	m.input.SetValue("unmatched")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 1, Height: 1})
	result, ok := next.(model)
	check(t, ok, "unexpected model")
	check(t, len(result.filtered()) == 0, "must be empty")
	check(t, result.View().Content != "", "must render safely")
}

func TestBranchPickerCtrlCMapsWrappedInterruptToCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	input, writer := io.Pipe()
	defer input.Close()
	go func() { defer writer.Close(); io.WriteString(writer, "\x03") }()
	selected, err := Pick(ctx, []git.Branch{{Ref: "refs/heads/main", Name: "main"}}, input, io.Discard)
	check(t, errors.Is(err, context.Canceled), fmt.Sprintf("expected cancellation; got %v", err))
	check(t, selected == "", "interrupt must not select")
}
