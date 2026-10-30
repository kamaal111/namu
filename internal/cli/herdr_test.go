package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
)

func TestHerdrSelectionAndFocusFlags(t *testing.T) {
	for _, tc := range []struct {
		name, env     string
		flags         []string
		called, focus bool
	}{
		{"automatic", "1", nil, true, true},
		{"explicit", "", []string{"--herdr"}, true, true},
		{"disabled", "1", []string{"--herdr=false"}, false, false},
		{"outside", "", nil, false, false},
		{"not-one", "true", nil, false, false},
		{"no-focus", "1", []string{"--no-focus"}, true, false},
		{"focus-false", "1", []string{"--focus=false"}, true, false},
		{"no-focus-false", "1", []string{"--no-focus=false"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Herdr(t, "exit 7")
			t.Setenv("HERDR_ENV", tc.env)
			out, diagnostics, err := execute(t, repo.Source, append([]string{"create", "feature/cards", "--base", "main"}, tc.flags...)...)
			must(t, err)
			okCall := strings.Contains(diagnostics, "Warning:")
			check(t, okCall == tc.called, diagnostics)
			check(t, out == filepath.Join(repo.Main, ".agents/worktrees/feature/cards")+"\n", out)
			calls, _ := os.ReadFile(log)
			check(t, strings.Contains(string(calls), "--focus\n") == tc.focus, string(calls))
		})
	}
}

func TestFocusFlagsAreMutuallyExclusive(t *testing.T) {
	for _, action := range []string{"create", "open"} {
		t.Run(action, func(t *testing.T) {
			out, _, err := execute(t, t.TempDir(), action, "feature/cards", "--focus", "--no-focus")
			check(t, err != nil, "conflicting flags must fail")
			check(t, out == "", "must not print path")
		})
	}
}
