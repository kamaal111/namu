package cli

import (
	"bytes"
	"context"
	"github.com/kamaal111/namu/internal/testutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, value bool, message string) {
	t.Helper()
	if !value {
		t.Fatal(message)
	}
}
func execute(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	root := New(dir)
	var out, diagnostics bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&diagnostics)
	root.SetIn(strings.NewReader(""))
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return out.String(), diagnostics.String(), err
}
func TestHelpAndVersionOutsideRepository(t *testing.T) {
	dir := t.TempDir()
	out, _, err := execute(t, dir, "--version")
	must(t, err)
	check(t, out == "namu version "+Version+"\n", out)
	out, _, err = execute(t, dir, "--help")
	must(t, err)
	check(t, strings.Contains(out, "create"), out)
}
func TestNoninteractiveOpenRequiresBranch(t *testing.T) {
	out, _, err := execute(t, t.TempDir(), "open")
	check(t, err != nil && strings.Contains(err.Error(), "supply a branch"), "must provide actionable error")
	check(t, out == "", "must not emit path")
}
func TestCreatePrintsPathBeforeHookAndDoesNotLeakHookOutput(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	dir, physical := repo.Source, repo.Main
	must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[setup]\ncommand = ['sh', '-c', 'echo diagnostics; exit 1']\n"), 0600))
	out, diagnostics, err := execute(t, dir, "create", "feature/cards", "--base", "main")
	check(t, err != nil, "hook failure must propagate")
	check(t, out == filepath.Join(physical, ".agents/worktrees/feature/cards")+"\n", out)
	check(t, diagnostics == "diagnostics\n", diagnostics)
	listing, _, err := execute(t, dir, "list")
	must(t, err)
	check(t, strings.Contains(listing, "feature/cards"), listing)
}
func TestCreateRejectsEmptyBaseAndConflictingFlags(t *testing.T) {
	_, _, err := execute(t, t.TempDir(), "create", "feature/cards", "--base", "")
	check(t, err != nil && strings.Contains(err.Error(), "--base cannot be empty"), "empty base must fail")
	_, _, err = execute(t, t.TempDir(), "create", "feature/cards", "--base", "main", "--remote", "origin")
	check(t, err != nil, "conflicting flags must fail")
}

func TestDeleteMultipleManagedCheckoutsAndRetainBranches(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	branches := []string{"fix/initial-release-version", "feat/initial-release", "chore/mise-setup", "herdr-setup"}
	for _, branch := range branches {
		_, _, err := execute(t, repo.Source, "create", branch, "--base", "main")
		must(t, err)
	}
	out, diagnostics, err := execute(t, repo.Source, append([]string{"delete"}, branches...)...)
	must(t, err)
	check(t, out == "", out)
	check(t, strings.Count(diagnostics, "Removed ") == 4, diagnostics)
	for _, branch := range branches {
		_, err := os.Stat(filepath.Join(repo.Main, ".agents/worktrees", branch))
		check(t, os.IsNotExist(err), "checkout must be removed")
		check(t, testutil.Git(t, repo.Source, "branch", "--list", branch) != "", "branch must remain")
	}
}

func TestDeleteContinuesAfterFailuresAndDeduplicates(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	for _, branch := range []string{"feature/cards", "feature/dirty", "feature/locked", "feature/last"} {
		_, _, err := execute(t, repo.Source, "create", branch, "--base", "main")
		must(t, err)
	}
	dirty := filepath.Join(repo.Main, ".agents/worktrees/feature/dirty")
	testutil.Write(t, filepath.Join(dirty, "untracked"), "keep")
	locked := filepath.Join(repo.Main, ".agents/worktrees/feature/locked")
	testutil.Git(t, repo.Source, "worktree", "lock", locked)
	external := filepath.Join(t.TempDir(), "external")
	testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/external", external)
	out, diagnostics, err := execute(t, repo.Source, "delete", "feature/cards", "feature/cards", "feature/dirty", "feature/locked", "feature/external", "missing", "main", "feature/last")
	check(t, err != nil, "any failure must fail the batch")
	check(t, out == "", out)
	check(t, strings.Count(diagnostics, "Removed ") == 3, diagnostics)
	for _, branch := range []string{"feature/dirty", "feature/locked", "missing", "main"} {
		check(t, strings.Contains(err.Error(), branch), err.Error())
	}
	for _, path := range []string{dirty, locked, repo.Main} {
		_, err := os.Stat(path)
		must(t, err)
	}
	_, statErr := os.Stat(external)
	check(t, os.IsNotExist(statErr), "external checkout must be removed")
	check(t, testutil.Git(t, repo.Source, "branch", "--list", "feature/external") != "", "external branch must remain")
	_, _, err = execute(t, repo.Source, "delete")
	check(t, err != nil, "missing argument must fail")
}

func TestDeleteProtectsInvokingCheckoutAndContinues(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	for _, branch := range []string{"feature/current", "feature/other"} {
		_, _, err := execute(t, repo.Source, "create", branch, "--base", "main")
		must(t, err)
	}
	current := filepath.Join(repo.Main, ".agents/worktrees/feature/current")
	_, diagnostics, err := execute(t, current, "delete", "feature/current", "feature/other")
	check(t, err != nil && strings.Contains(err.Error(), "cannot delete the invoking checkout"), "must protect invoking checkout")
	check(t, strings.Contains(diagnostics, "feature/other retained"), diagnostics)
	_, err = os.Stat(current)
	must(t, err)
}

func TestHomePathsAcrossCheckoutCommands(t *testing.T) {
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	t.Setenv("HOME", repo.Main)
	target := "~/.agents/worktrees/feature/cards"
	out, _, err := execute(t, repo.Source, "create", "feature/cards", "--base", "main")
	must(t, err)
	check(t, out == target+"\n", out)
	out, _, err = execute(t, repo.Source, "open", "feature/cards")
	must(t, err)
	check(t, out == target+"\n", out)
	out, _, err = execute(t, repo.Source, "list")
	must(t, err)
	check(t, strings.Contains(out, target), out)
	check(t, !strings.Contains(out, repo.Main), out)
	_, _, err = execute(t, repo.Source, "create", "feature/other", "--base", "main")
	must(t, err)
	_, diagnostics, err := execute(t, repo.Source, "delete", "feature/cards")
	must(t, err)
	check(t, diagnostics == "Removed "+target+" (branch feature/cards retained)\n", diagnostics)
	must(t, os.WriteFile(filepath.Join(repo.Source, ".namu.toml"), []byte("[setup]\ncommand = ['sh', '-c', 'exit 1']\n"), 0600))
	out, _, err = execute(t, repo.Source, "create", "feature/fails", "--base", "main")
	check(t, err != nil, "setup must fail")
	check(t, out == "~/.agents/worktrees/feature/fails\n", out)
	check(t, strings.Contains(err.Error(), "checkout preserved at ~/.agents/worktrees/feature/fails; retry from ~:"), err.Error())
}
