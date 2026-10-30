package worktree

import (
	"context"
	"errors"
	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/testutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHerdrCreatesCheckoutAndRunsSetupOnce(t *testing.T) {
	s, out, diagnostics := fixture(t)
	s.Herdr = true
	log := testutil.Herdr(t, `"$REAL_GIT" -C "$source" worktree add -b "$branch" "$target" "$base"
echo '{"result":"workspace"}'`)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo setup >> count']\n")
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	calls, err := os.ReadFile(log)
	must(t, err)
	ok(t, strings.Contains(string(calls), "--focus\n"), string(calls))
	ok(t, out.String() == path+"\n", out.String())
	ok(t, diagnostics.Len() == 0, diagnostics.String())
	count, err := os.ReadFile(filepath.Join(path, "count"))
	must(t, err)
	ok(t, string(count) == "setup\n", string(count))
}

func TestHerdrFailureFallsBackToGit(t *testing.T) {
	for _, body := range []string{
		"exit 7",
		`"$REAL_GIT" -C "$source" branch "$branch" "$base"; exit 7`,
		`"$REAL_GIT" -C "$source" worktree add -b "$branch" "$target" "$base"; exit 7`,
		`echo '{"ok":true}'`,
	} {
		t.Run(body, func(t *testing.T) {
			s, out, diagnostics := fixture(t)
			s.Herdr = true
			testutil.Herdr(t, body)
			must(t, s.Create(context.Background(), "feature/cards", "main", ""))
			path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
			ok(t, out.String() == path+"\n", out.String())
			ok(t, strings.Contains(diagnostics.String(), "Warning:"), diagnostics.String())
			ok(t, run(t, path, "branch", "--show-current") == "feature/cards", "checkout must exist")
		})
	}
}

func TestHerdrOpenExistingNoFocusDoesNotRunSetup(t *testing.T) {
	s, out, diagnostics := fixture(t)
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	out.Reset()
	s.Herdr, s.NoFocus = true, true
	log := testutil.Herdr(t, "echo workspace")
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "invalid toml!!!")
	must(t, s.Open(context.Background(), "feature/cards"))
	calls, err := os.ReadFile(log)
	must(t, err)
	ok(t, strings.HasPrefix(string(calls), "worktree\nopen\n"), string(calls))
	ok(t, strings.Contains(string(calls), "--no-focus\n"), string(calls))
	ok(t, diagnostics.Len() == 0, diagnostics.String())
	ok(t, strings.HasSuffix(out.String(), "feature/cards\n"), out.String())
}

func TestHerdrOpenExistingFailureStillPrintsPath(t *testing.T) {
	s, out, diagnostics := fixture(t)
	s.Herdr = true
	testutil.Herdr(t, "exit 7")
	must(t, s.Open(context.Background(), "main"))
	ok(t, out.String() == s.Repo.Main+"\n", out.String())
	ok(t, strings.Contains(diagnostics.String(), "Warning:"), diagnostics.String())
}

func TestHerdrConflictingPartialStateIsPreserved(t *testing.T) {
	s, out, diagnostics := fixture(t)
	s.Herdr = true
	testutil.Herdr(t, `mkdir -p "$target"; echo keep > "$target/file"; exit 7`)
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil, "must fail on conflicting destination")
	ok(t, out.Len() == 0, "must not print checkout")
	ok(t, strings.Contains(diagnostics.String(), "Warning:"), diagnostics.String())
	content, err := os.ReadFile(filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards/file"))
	must(t, err)
	ok(t, string(content) == "keep\n", string(content))
}

func TestHerdrRemoteOpenPreservesTracking(t *testing.T) {
	s, _, _ := fixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	run(t, s.Repo.Main, "clone", "--bare", s.Repo.Main, remote)
	run(t, remote, "branch", "feature/search")
	run(t, s.Repo.Main, "remote", "add", "origin", remote)
	run(t, s.Repo.Main, "fetch", "origin")
	s.Herdr = true
	testutil.Herdr(t, `"$REAL_GIT" -C "$source" worktree add -b "$branch" "$target" "$base"`)
	must(t, s.Open(context.Background(), "origin/feature/search"))
	ok(t, run(t, s.Repo.Main, "config", "branch.feature/search.remote") == "origin", "must track remote")
	ok(t, run(t, s.Repo.Main, "config", "branch.feature/search.merge") == "refs/heads/feature/search", "must track branch")
}

func TestHerdrLocalBranchOpenUsesExistingBranch(t *testing.T) {
	s, out, diagnostics := fixture(t)
	run(t, s.Repo.Source, "branch", "feature/cards")
	s.Herdr = true
	testutil.Herdr(t, `"$REAL_GIT" -C "$source" worktree add "$target" "$branch"`)
	must(t, s.Open(context.Background(), "feature/cards"))
	ok(t, diagnostics.Len() == 0, diagnostics.String())
	ok(t, out.String() == filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")+"\n", out.String())
}

func TestHerdrLinkedInvocationUsesMainManagedPath(t *testing.T) {
	s, out, diagnostics := fixture(t)
	ctx := context.Background()
	must(t, s.Create(ctx, "feature/cards", "main", ""))
	repo, err := git.Discover(ctx, filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards"))
	must(t, err)
	s.Repo, s.Herdr = repo, true
	out.Reset()
	log := testutil.Herdr(t, `"$REAL_GIT" -C "$source" worktree add -b "$branch" "$target" "$base"`)
	must(t, s.Create(ctx, "feature/pricing", "feature/cards", ""))
	calls, err := os.ReadFile(log)
	must(t, err)
	ok(t, strings.Contains(string(calls), "--cwd\n"+repo.Source+"\n"), string(calls))
	ok(t, out.String() == filepath.Join(repo.Main, ".agents/worktrees/feature/pricing")+"\n", out.String())
	ok(t, diagnostics.Len() == 0, diagnostics.String())
}

func TestHerdrSetupFailureDoesNotFallbackOrRecreate(t *testing.T) {
	s, out, diagnostics := fixture(t)
	s.Herdr = true
	testutil.Herdr(t, `"$REAL_GIT" -C "$source" worktree add -b "$branch" "$target" "$base"`)
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo failed >&2; exit 7']\n")
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil, "setup must fail")
	ok(t, strings.Contains(err.Error(), "namu setup feature/cards"), err.Error())
	ok(t, diagnostics.String() == "failed\n", diagnostics.String())
	ok(t, out.String() == filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")+"\n", out.String())
}

func TestHerdrMissingExecutableFallsBack(t *testing.T) {
	s, out, diagnostics := fixture(t)
	s.Herdr = true
	bin := t.TempDir()
	realGit, err := exec.LookPath("git")
	must(t, err)
	must(t, os.Symlink(realGit, filepath.Join(bin, "git")))
	t.Setenv("PATH", bin)
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	ok(t, strings.Contains(diagnostics.String(), "executable file not found"), diagnostics.String())
	ok(t, out.String() == filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")+"\n", out.String())
}

func TestHerdrCancellationDoesNotFallback(t *testing.T) {
	s, out, diagnostics := fixture(t)
	s.Herdr = true
	testutil.Herdr(t, "exit 7")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Create(ctx, "feature/cards", "main", "")
	ok(t, errors.Is(err, context.Canceled), "must return cancellation")
	ok(t, out.Len() == 0 && diagnostics.Len() == 0, "must not warn or print a path")
	exists, err := git.LocalBranchExists(context.Background(), s.Repo.Source, "feature/cards")
	must(t, err)
	ok(t, !exists, "must not create branch")
}

func TestHerdrConflictingBranchOrCheckoutIsPreserved(t *testing.T) {
	for _, tc := range []struct{ name, body, ref string }{
		{"wrong-path", `"$REAL_GIT" -C "$source" worktree add -b "$branch" "$target-other" "$base"; exit 7`, "refs/heads/feature/cards"},
		{"wrong-branch", `"$REAL_GIT" -C "$source" worktree add -b other "$target" "$base"; exit 7`, "refs/heads/other"},
		{"wrong-commit", `"$REAL_GIT" -C "$source" commit --allow-empty -qm changed; "$REAL_GIT" -C "$source" branch "$branch" HEAD; exit 7`, "refs/heads/feature/cards"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, out, diagnostics := fixture(t)
			s.Herdr = true
			testutil.Herdr(t, tc.body)
			err := s.Create(context.Background(), "feature/cards", "main", "")
			ok(t, err != nil, "must reject conflicting state")
			ok(t, strings.Contains(err.Error(), "preserved"), err.Error())
			ok(t, out.Len() == 0, "must not print checkout")
			ok(t, strings.Contains(diagnostics.String(), "Warning:"), diagnostics.String())
			ok(t, run(t, s.Repo.Source, "show-ref", "--verify", tc.ref) != "", "must preserve branch")
		})
	}
}
