package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
)

func TestCompletion(t *testing.T) {
	repo := testutil.Repository(t)
	testutil.Git(t, repo.Source, "branch", "feature/cards")
	testutil.Git(t, repo.Source, "remote", "add", "origin", "https://invalid.example/repo")
	testutil.Git(t, repo.Source, "update-ref", "refs/remotes/origin/feature/search", "HEAD")
	testutil.Git(t, repo.Source, "update-ref", "refs/remotes/origin/feature/cards", "HEAD")
	testutil.Git(t, repo.Source, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/feature/search")
	testutil.Git(t, repo.Source, "branch", "gitbutler/workspace")
	managed := filepath.Join(repo.Main, ".agents", "worktrees", "feature/cards")
	testutil.Git(t, repo.Source, "worktree", "add", managed, "feature/cards")
	external := filepath.Join(t.TempDir(), "external")
	testutil.Git(t, repo.Source, "worktree", "add", "-b", "external", external)
	before := testutil.Git(t, repo.Source, "show-ref")
	trees := testutil.Git(t, repo.Source, "worktree", "list", "--porcelain")
	for _, tc := range []struct {
		name string
		dir  string
		args []string
		want string
	}{
		{"local", repo.Source, []string{"open", "feature/"}, "feature/cards\n:4\n"},
		{"remote", repo.Source, []string{"open", "origin/feature/"}, "origin/feature/search\n:4\n"},
		{"open-limit", repo.Source, []string{"open", "feature/cards", ""}, ":4\n"},
		{"delete", repo.Source, []string{"delete", "feature/"}, "feature/cards\n:4\n"},
		{"delete-repeat", repo.Source, []string{"delete", "feature/cards", "feature/"}, ":4\n"},
		{"delete-external", repo.Source, []string{"delete", "ext"}, "external\n:4\n"},
		{"delete-invoking", managed, []string{"delete", "feature/"}, ":4\n"},
		{"setup", repo.Source, []string{"setup", "feature/"}, "feature/cards\n:4\n"},
		{"setup-external", repo.Source, []string{"setup", "ext"}, ":4\n"},
		{"create", repo.Source, []string{"create", ""}, ":4\n"},
		{"base", repo.Source, []string{"create", "new", "--base", "feature/"}, "feature/cards\n:4\n"},
		{"remote-flag", repo.Source, []string{"create", "new", "--remote", "or"}, "origin\n:4\n"},
		{"outside", t.TempDir(), []string{"open", ""}, ":4\n"},
		{"shell-limit", repo.Source, []string{"shell-init", "zsh", ""}, ":4\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _, err := execute(t, tc.dir, append([]string{"__completeNoDesc"}, tc.args...)...)
			must(t, err)
			check(t, out == tc.want, out)
		})
	}
	check(t, testutil.Git(t, repo.Source, "show-ref") == before, "completion changed refs")
	check(t, testutil.Git(t, repo.Source, "worktree", "list", "--porcelain") == trees, "completion changed checkouts")
	testutil.Git(t, repo.Source, "worktree", "lock", managed)
	out, _, err := execute(t, repo.Source, "__completeNoDesc", "setup", "feature/")
	must(t, err)
	check(t, out == ":4\n", "locked checkout must not be suggested")
	testutil.Git(t, repo.Source, "worktree", "unlock", managed)
	must(t, os.RemoveAll(managed))
	out, _, err = execute(t, repo.Source, "__completeNoDesc", "setup", "feature/")
	must(t, err)
	check(t, out == ":4\n", "prunable checkout must not be suggested")
}

func TestCompletionCommandsOutsideRepository(t *testing.T) {
	out, _, err := execute(t, t.TempDir(), "__completeNoDesc", "op")
	must(t, err)
	check(t, strings.Contains(out, "open\n"), out)
	out, _, err = execute(t, t.TempDir(), "completion", "zsh")
	must(t, err)
	check(t, strings.Contains(out, "compdef _namu namu"), out)
}
