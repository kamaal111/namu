package herdr

import (
	"context"
	"os"
	"testing"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/integration"
	"github.com/kamaal111/namu/internal/testutil"
)

func TestHooksUseDistinctCommandsAndHonorFocus(t *testing.T) {
	for _, tc := range []struct {
		name, flag string
		focus      bool
	}{
		{name: "focus", flag: "--focus", focus: true},
		{name: "no-focus", flag: "--no-focus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := testutil.Herdr(t, "exit 0")
			hooks := Hooks()
			result, err := hooks.Create(context.Background(), integration.CreateRequest{
				Source: "/repo", Worktree: git.WorktreeRequest{Path: "/repo/cards", Branch: "feature/cards"}, Revision: "abc123", Focus: tc.focus,
			})
			must(t, err)
			check(t, result.OpenHandled, "Herdr creation must complete opening for either focus preference")
			must(t, hooks.Open(context.Background(), integration.OpenRequest{Source: "/repo", Path: "/repo/cards", Branch: "feature/cards", Focus: tc.focus}))
			calls, err := os.ReadFile(log)
			must(t, err)
			expected := "worktree\ncreate\n--cwd\n/repo\n--path\n/repo/cards\n" + tc.flag + "\n--branch\nfeature/cards\n--base\nabc123\nworktree\nopen\n--cwd\n/repo\n--path\n/repo/cards\n" + tc.flag + "\n"
			check(t, string(calls) == expected, string(calls))
		})
	}
}

func TestCreateFailureDoesNotClaimOpenHandled(t *testing.T) {
	testutil.Herdr(t, "exit 7")
	result, err := Hooks().Create(context.Background(), integration.CreateRequest{})
	check(t, err != nil, "creation must report failure")
	check(t, !result.OpenHandled, "failed creation cannot complete opening")
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func check(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Fatal(message)
	}
}
