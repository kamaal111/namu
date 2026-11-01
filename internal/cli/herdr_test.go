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
		{"focus-last", "1", []string{"--no-focus", "--focus"}, true, true},
		{"no-focus-last", "1", []string{"--focus", "--no-focus"}, true, false},
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

func TestFocusFlagsLastWins(t *testing.T) {
	for _, action := range []string{"create", "open"} {
		for _, tc := range []struct {
			name  string
			flags []string
			focus bool
		}{
			{"focus-last", []string{"--no-focus", "--focus"}, true},
			{"no-focus-last", []string{"--focus", "--no-focus"}, false},
			{"negative-false-last", []string{"--focus=false", "--no-focus=false"}, true},
			{"positive-false-last", []string{"--no-focus=false", "--focus=false"}, false},
			{"repeated-positive", []string{"--focus=false", "--focus"}, true},
			{"repeated-negative", []string{"--no-focus", "--no-focus=false"}, true},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				repo := testutil.Repository(t)
				testutil.Git(t, repo.Source, "branch", "feature/cards")
				root := New(repo.Source)
				cmd, _, err := root.Find([]string{action})
				must(t, err)
				cmd.SetContext(t.Context())
				must(t, cmd.ParseFlags(tc.flags))
				s, err := service(cmd, repo.Source)
				must(t, err)
				check(t, s.NoFocus == !tc.focus, "incorrect effective focus")
				focus, err := cmd.Flags().GetBool("focus")
				must(t, err)
				check(t, focus == tc.focus, "positive flag must expose effective focus")
			})
		}
	}
}
