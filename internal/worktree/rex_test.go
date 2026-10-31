package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/rex"
	"github.com/kamaal111/namu/internal/testutil"
)

func TestRexOpensAfterSetupAndReusesExistingCheckoutWithoutSetup(t *testing.T) {
	s, out, diagnostics := fixture(t)
	log := testutil.Rex(t, `case "$1" in new) [ -f "$4/ready" ] || exit 9 ;; esac
`+testutil.RexResponses)
	s.Integration = rex.Hooks()
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'echo ready > ready']\n")
	must(t, s.Create(context.Background(), "feature/cards", "main", ""))
	path := filepath.Join(s.Repo.Main, ".agents/worktrees/feature/cards")
	ok(t, out.String() == path+"\n" && diagnostics.Len() == 0, diagnostics.String())
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "invalid toml")
	must(t, s.Open(context.Background(), "feature/cards"))
	calls, err := os.ReadFile(log)
	must(t, err)
	ok(t, strings.Count(string(calls), "BEGIN\nnew\n") == 1, string(calls))
	ok(t, strings.Count(string(calls), "session.select") == 2, string(calls))
}

func TestRexDoesNotRunAfterSetupFailure(t *testing.T) {
	s, out, _ := fixture(t)
	log := testutil.Rex(t, testutil.RexResponses)
	s.Integration = rex.Hooks()
	write(t, filepath.Join(s.Repo.Source, ".namu.toml"), "[setup]\ncommand = ['sh', '-c', 'exit 7']\n")
	err := s.Create(context.Background(), "feature/cards", "main", "")
	ok(t, err != nil, "setup must fail")
	ok(t, strings.HasSuffix(out.String(), "/feature/cards\n"), out.String())
	_, err = os.Stat(log)
	ok(t, os.IsNotExist(err), "Rex must not run")
}
