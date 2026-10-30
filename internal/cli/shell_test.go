package cli

import (
	"bytes"
	"github.com/kamaal111/namu/internal/testutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellInitOutsideRepository(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			out, _, err := execute(t, t.TempDir(), "shell-init", shell)
			must(t, err)
			check(t, out == shellIntegration, "must print shell function")
		})
	}
	_, _, err := execute(t, t.TempDir(), "shell-init", "fish")
	check(t, err != nil, "unsupported shell must fail")
}

func TestShellNavigationAndForwarding(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		for _, tc := range []struct {
			name, args, output, code string
			navigate                 bool
		}{
			{"create", "create feature/cards", "checkout", "0", true},
			{"open", "open feature/cards", "checkout", "0", true},
			{"focus", "create feature/cards --focus", "checkout", "0", true},
			{"no-focus", "create feature/cards --no-focus", "checkout", "0", false},
			{"focus-false", "create feature/cards --focus=false", "checkout", "0", false},
			{"no-focus-false", "open feature/cards --no-focus=false", "checkout", "0", true},
			{"setup-failure", "create feature/cards", "checkout", "1", false},
			{"cancel", "open", "", "0", false},
			{"interrupt", "open", "", "130", false},
			{"help", "open --help", "help", "0", false},
			{"other", "list", "listing", "0", false},
			{"flag-as-value", "create cards --base --no-focus", "checkout", "0", true},
			{"end-options", "create -- --no-focus", "checkout", "0", true},
		} {
			t.Run(shell+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				target := filepath.Join(root, "checkout with spaces")
				must(t, os.Mkdir(target, 0700))
				bin := filepath.Join(root, "namu")
				must(t, os.WriteFile(bin, []byte("#!/bin/sh\nif [ -n \"$TEST_OUTPUT\" ]; then printf '%s\\n' \"$TEST_OUTPUT\"; fi\necho diagnostics >&2\nexit \"$TEST_CODE\"\n"), 0700))
				output := strings.ReplaceAll(tc.output, "checkout", target)
				program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu " + tc.args + "\nnamu_code=$?\nprintf 'exit=%s\\npwd=%s\\n' \"$namu_code\" \"$(pwd -P)\"\n"
				cmd := exec.Command(shell, "-f", "-c", program)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "TEST_PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_OUTPUT="+output, "TEST_CODE="+tc.code)
				var diagnostics bytes.Buffer
				cmd.Stderr = &diagnostics
				result, err := cmd.Output()
				must(t, err)
				check(t, diagnostics.String() == "diagnostics\n", diagnostics.String())
				actualRoot, err := filepath.EvalSymlinks(root)
				must(t, err)
				expected := map[bool]string{false: actualRoot, true: filepath.Join(actualRoot, "checkout with spaces")}[tc.navigate]
				prefix := map[bool]string{true: output + "\n", false: ""}[output != ""]
				check(t, string(result) == prefix+"exit="+tc.code+"\npwd="+expected+"\n", string(result))
			})
		}
	}
}

func TestShellIntegrationWithRealCLIAndBothBackends(t *testing.T) {
	binaryDir := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(binaryDir, "namu"), "../../cmd/namu")
	buildOutput, err := build.CombinedOutput()
	check(t, err == nil, string(buildOutput))
	for _, shell := range []string{"bash", "zsh"} {
		for _, backend := range []string{"false", "true"} {
			for _, action := range []string{"create", "open"} {
				t.Run(shell+"/herdr="+backend+"/"+action, func(t *testing.T) {
					repo := testutil.Repository(t)
					log := testutil.Herdr(t, `if [ "$action" = create ]; then
 if "$REAL_GIT" -C "$source" show-ref --verify --quiet "refs/heads/$branch"; then
  "$REAL_GIT" -C "$source" worktree add "$target" "$branch"
 else
  "$REAL_GIT" -C "$source" worktree add -b "$branch" "$target" "$base"
 fi
fi`)
					testutil.Git(t, repo.Source, "branch", "feature/cards")
					createArgs := map[string]string{"create": "feature/pricing --base main", "open": "feature/cards"}[action]
					expectedBranch := map[string]string{"create": "feature/pricing", "open": "feature/cards"}[action]
					program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu " + action + " " + createArgs + " --herdr=" + backend + "\nprintf 'pwd=%s\\n' \"$(pwd -P)\"\n"
					cmd := exec.Command(shell, "-f", "-c", program)
					cmd.Dir = repo.Source
					cmd.Env = append(os.Environ(), "TEST_PATH="+binaryDir+string(os.PathListSeparator)+os.Getenv("PATH"))
					result, err := cmd.CombinedOutput()
					must(t, err)
					expectedPath := filepath.Join(repo.Main, ".agents/worktrees", expectedBranch)
					check(t, string(result) == expectedPath+"\npwd="+expectedPath+"\n", string(result))
					calls, _ := os.ReadFile(log)
					check(t, (len(calls) > 0) == (backend == "true"), string(calls))
				})
			}
		}
	}
}
