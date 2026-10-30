package cli

import (
	"bytes"
	"encoding/json"
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
				must(t, os.WriteFile(bin, []byte("#!/bin/sh\n[ \"$NAMU_SHELL_INTEGRATION\" = 1 ] || exit 99\nif [ -n \"$TEST_OUTPUT\" ]; then printf '%s\\n' \"$TEST_OUTPUT\"; fi\necho diagnostics >&2\nexit \"$TEST_CODE\"\n"), 0700))
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
	binaryDir := buildShellCLI(t)
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

func buildShellCLI(t *testing.T) string {
	t.Helper()
	binaryDir := t.TempDir()
	build := exec.Command("go", "build", "-o", filepath.Join(binaryDir, "namu"), "../../cmd/namu")
	output, err := build.CombinedOutput()
	check(t, err == nil, string(output))
	return binaryDir
}

func TestInstalledShellIntegrationNavigates(t *testing.T) {
	binaryDir := buildShellCLI(t)
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			home := shellHome(t)
			repo := testutil.Repository(t)
			_, _, err := execute(t, repo.Source, "shell-init", shell, "--install")
			must(t, err)
			startup := map[string]string{"bash": ".bashrc", "zsh": ".zshrc"}[shell]
			program := "export PATH=\"$TEST_PATH\"\n. \"$HOME/" + startup + "\"\nnamu create feature/cards --base main --herdr=false\nnamu_code=$?\nprintf 'exit=%s\\npwd=%s\\n' \"$namu_code\" \"$(pwd -P)\"\n"
			cmd := exec.Command(shell, "-f", "-c", program)
			cmd.Dir = repo.Source
			cmd.Env = append(os.Environ(), "TEST_PATH="+binaryDir+string(os.PathListSeparator)+os.Getenv("PATH"))
			output, err := cmd.CombinedOutput()
			must(t, err)
			target := filepath.Join(repo.Main, ".agents/worktrees/feature/cards")
			check(t, string(output) == target+"\nexit=0\npwd="+target+"\n", string(output))
			content, err := os.ReadFile(filepath.Join(home, startup))
			must(t, err)
			check(t, strings.Count(string(content), "eval ") == 1, string(content))
		})
	}
}

func TestAutomaticShellInstallInTerminal(t *testing.T) {
	binaryDir := buildShellCLI(t)
	for _, tc := range []struct {
		name, shell, answer, marker   string
		args                          []string
		redirected, prompt, installed bool
		code                          int
	}{
		{name: "accept", shell: "zsh", answer: "yes\n", args: []string{"open", "feature/cards"}, prompt: true, installed: true},
		{name: "accept-bash", shell: "bash", answer: "yes\n", args: []string{"create", "feature/new", "--base", "main"}, prompt: true, installed: true},
		{name: "decline", shell: "zsh", answer: "no\n", args: []string{"open", "feature/cards"}, prompt: true},
		{name: "eof", shell: "zsh", answer: "\x04", args: []string{"open", "feature/cards"}, prompt: true},
		{name: "interrupt", shell: "zsh", answer: "interrupt", args: []string{"open", "feature/cards"}, prompt: true, code: 130},
		{name: "loaded", shell: "zsh", marker: "1", args: []string{"open", "feature/cards"}},
		{name: "no-focus", shell: "zsh", args: []string{"open", "feature/cards", "--no-focus"}},
		{name: "focus-false", shell: "zsh", args: []string{"open", "feature/cards", "--focus=false"}},
		{name: "redirected", shell: "zsh", args: []string{"open", "feature/cards"}, redirected: true},
		{name: "unsupported", shell: "fish", args: []string{"open", "feature/cards"}},
		{name: "help", shell: "zsh", args: []string{"open", "--help"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := shellHome(t)
			repo := testutil.Repository(t)
			testutil.Git(t, repo.Source, "branch", "feature/cards")
			t.Setenv("SHELL", "/bin/"+tc.shell)
			t.Setenv("TERM", "xterm-256color")
			t.Setenv("NAMU_SHELL_INTEGRATION", tc.marker)
			t.Setenv("HERDR_ENV", "")
			settings, err := json.Marshal(struct {
				Args       []string
				Answer     string
				Redirected bool
			}{tc.args, tc.answer, tc.redirected})
			must(t, err)
			cmd := exec.Command("python3", "-c", terminalInstallProgram, filepath.Join(binaryDir, "namu"), string(settings))
			cmd.Dir = repo.Source
			output, err := cmd.CombinedOutput()
			check(t, err == nil, string(output))
			var result struct {
				Output string
				Code   int
			}
			must(t, json.Unmarshal(output, &result))
			check(t, result.Code == tc.code, result.Output)
			check(t, strings.Contains(result.Output, "Install shell integration with these changes?") == tc.prompt, result.Output)
			startup := map[string]string{"zsh": ".zshrc", "bash": ".bashrc", "fish": ".zshrc"}[tc.shell]
			_, err = os.Stat(filepath.Join(home, startup))
			check(t, (err == nil) == tc.installed, result.Output)
			check(t, strings.Contains(result.Output, "Added to:") == tc.installed, result.Output)
		})
	}
}

const terminalInstallProgram = `
import errno, json, os, pty, select, signal, subprocess, sys, time
settings = json.loads(sys.argv[2])
master, slave = pty.openpty()
proc = subprocess.Popen([sys.argv[1], *settings["Args"]], stdin=slave,
    stdout=subprocess.PIPE if settings["Redirected"] else slave, stderr=slave)
os.close(slave)
output = b""
answered = False
deadline = time.monotonic() + 15
while time.monotonic() < deadline:
    ready, _, _ = select.select([master], [], [], 0.1)
    if ready:
        try:
            chunk = os.read(master, 65536)
        except OSError as error:
            if error.errno == errno.EIO:
                break
            raise
        if not chunk:
            break
        output += chunk
    if not answered and b"[y/N] " in output:
        answered = True
        if settings["Answer"] == "interrupt":
            proc.send_signal(signal.SIGINT)
        else:
            os.write(master, settings["Answer"].encode())
    if proc.poll() is not None and not ready:
        break
else:
    proc.kill()
    raise RuntimeError("terminal command timed out: " + output.decode(errors="replace"))
proc.wait(timeout=5)
os.close(master)
if settings["Redirected"]:
    output += proc.stdout.read()
print(json.dumps({"Output": output.decode(errors="replace"), "Code": proc.returncode}))
`
