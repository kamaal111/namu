package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
)

func TestMain(m *testing.M) {
	for _, key := range []string{"REX_SESSION", "REX_BLOCK", "REX_SERVER", "REX_CLIENT"} {
		_ = os.Unsetenv(key)
	}
	os.Exit(m.Run())
}

func TestRexSelectionAndFocusFlags(t *testing.T) {
	for _, tc := range []struct {
		name, env     string
		flags         []string
		called, focus bool
	}{
		{"automatic", "session:source", nil, true, true},
		{"explicit", "", []string{"--rex"}, true, true},
		{"disabled", "session:source", []string{"--rex=false"}, false, false},
		{"outside", "", nil, false, false},
		{"no-focus", "session:source", []string{"--no-focus"}, true, false},
		{"focus-false", "session:source", []string{"--focus=false"}, true, false},
		{"no-focus-false", "session:source", []string{"--no-focus=false"}, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Rex(t, testutil.RexResponses)
			t.Setenv("REX_SESSION", tc.env)
			out, diagnostics, err := execute(t, repo.Source, append([]string{"create", "feature/cards", "--base", "main"}, tc.flags...)...)
			must(t, err)
			check(t, diagnostics == "", diagnostics)
			check(t, out == filepath.Join(repo.Main, ".agents/worktrees/feature/cards")+"\n", out)
			calls, _ := os.ReadFile(log)
			check(t, strings.Contains(string(calls), "BEGIN\nnew\n") == tc.called, string(calls))
			check(t, strings.Contains(string(calls), "session.select") == tc.focus, string(calls))
		})
	}
}

func TestRexAndHerdrSelectionConflictBeforeCreation(t *testing.T) {
	repo := testutil.Repository(t)
	testutil.Rex(t, testutil.RexResponses)
	t.Setenv("HERDR_ENV", "1")
	out, _, err := execute(t, repo.Source, "create", "feature/cards", "--base", "main")
	check(t, err != nil && strings.Contains(err.Error(), "multiple integrations selected"), "must reject both integrations")
	check(t, out == "", out)
}

func TestRexFailureFallsBackToShellWithoutLeakingOutput(t *testing.T) {
	repo := testutil.Repository(t)
	testutil.Rex(t, "echo rejected >&2; echo private-output; exit 4")
	result := filepath.Join(t.TempDir(), "result")
	must(t, os.WriteFile(result, nil, 0600))
	t.Setenv("NAMU_SHELL_RESULT", result)
	out, diagnostics, err := execute(t, repo.Source, "open", "main")
	must(t, err)
	check(t, out == repo.Main+"\n", out)
	check(t, strings.Contains(diagnostics, "Warning: Rex open failed") && strings.Contains(diagnostics, "rejected"), diagnostics)
	destination, err := os.ReadFile(result)
	must(t, err)
	check(t, string(destination) == repo.Main+"\n", string(destination))
}

func TestRexShellIntegrationPreservesDirectoryAndFallsBackOnFailure(t *testing.T) {
	binaryDir := buildShellCLI(t)
	for _, shell := range []string{"bash", "zsh"} {
		for _, tc := range []struct {
			name, body        string
			flags             []string
			navigate, warning bool
		}{
			{"focused", testutil.RexResponses, nil, false, false},
			{"background", testutil.RexResponses, []string{"--no-focus"}, false, false},
			{"failure", "echo unavailable >&2; exit 4", nil, true, true},
			{"background-failure", "exit 4", []string{"--focus=false"}, false, true},
		} {
			t.Run(shell+"/"+tc.name, func(t *testing.T) {
				repo := testutil.Repository(t)
				testutil.Rex(t, tc.body)
				testutil.Git(t, repo.Source, "branch", "feature/cards")
				path := filepath.Join(repo.Main, ".agents/worktrees/feature/cards")
				program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu open feature/cards " + strings.Join(tc.flags, " ") + "\nnamu_code=$?\nprintf 'exit=%s\\npwd=%s\\n' \"$namu_code\" \"$(pwd -P)\"\n"
				cmd := exec.Command(shell, "-f", "-c", program)
				cmd.Dir = repo.Source
				cmd.Env = append(os.Environ(), "TEST_PATH="+binaryDir+string(os.PathListSeparator)+os.Getenv("PATH"))
				var diagnostics bytes.Buffer
				cmd.Stderr = &diagnostics
				output, err := cmd.Output()
				must(t, err)
				destination := map[bool]string{true: path, false: repo.Main}[tc.navigate]
				check(t, string(output) == path+"\nexit=0\npwd="+destination+"\n", string(output))
				check(t, strings.Contains(diagnostics.String(), "Warning: Rex") == tc.warning, diagnostics.String())
			})
		}
	}
}
