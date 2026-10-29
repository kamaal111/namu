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
