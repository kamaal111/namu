package cli

import (
	"bytes"
	"encoding/json"
	"github.com/kamaal111/namu/internal/pathdisplay"
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
			check(t, strings.HasPrefix(out, shellIntegration), "must print shell function")
			check(t, strings.Contains(out, "__namu"), "must include completion")
		})
	}
	_, _, err := execute(t, t.TempDir(), "shell-init", "fish")
	check(t, err != nil, "unsupported shell must fail")
}

func TestShellInitRegistersCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, shell, before, assertion string
	}{
		{"bash", "bash", "", "complete -p namu"},
		{"zsh", "zsh", "", "print -r -- ${_comps[namu]}"},
		{"zsh-insecure-directory", "zsh", "fpath=(\"$TEST_COMPLETION_DIR\" $fpath)\n", "print -r -- ${_comps[namu]}"},
		{"zsh-initialized", "zsh", "autoload -Uz compinit; compinit -i\ncompinit() { return 99; }\n", "print -r -- ${_comps[namu]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := shellHome(t)
			t.Setenv("ZDOTDIR", home)
			insecure := filepath.Join(home, "insecure-completions")
			must(t, os.Mkdir(insecure, 0700))
			must(t, os.Chmod(insecure, 0777))
			t.Setenv("TEST_COMPLETION_DIR", insecure)
			out, _, err := execute(t, home, "shell-init", tc.shell)
			must(t, err)
			program := "set -e\n" + tc.before + out + "\n" + out + "\n" + tc.assertion
			cmd := exec.Command(tc.shell, "-f", "-c", program)
			cmd.Dir = home
			result, err := cmd.CombinedOutput()
			must(t, err)
			check(t, strings.Contains(string(result), "namu"), string(result))
		})
	}
}

func TestShellNavigationAndForwarding(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		for _, tc := range []struct {
			name, args, output, code, destination string
			navigate                              bool
		}{
			{"create", "create feature/cards", "{checkout}", "0", "checkout with spaces", true},
			{"open", "open feature/cards", "{checkout}", "0", "checkout with spaces", true},
			{"home", "open main", "~", "0", "", true},
			{"home-child", "create feature/cards", "~/checkout with spaces", "0", "checkout with spaces", true},
			{"home-no-focus", "open main --no-focus", "~", "0", "", false},
			{"home-failure", "open main", "~", "1", "", false},
			{"focus", "create feature/cards --focus", "{checkout}", "0", "checkout with spaces", true},
			{"no-focus", "create feature/cards --no-focus", "{checkout}", "0", "checkout with spaces", false},
			{"focus-false", "create feature/cards --focus=false", "{checkout}", "0", "checkout with spaces", false},
			{"no-focus-false", "open feature/cards --no-focus=false", "{checkout}", "0", "checkout with spaces", true},
			{"setup-failure", "create feature/cards", "{checkout}", "1", "checkout with spaces", false},
			{"cancel", "open", "", "0", "checkout with spaces", false},
			{"interrupt", "open", "", "130", "checkout with spaces", false},
			{"help", "open --help", "help", "0", "checkout with spaces", false},
			{"other", "list", "listing", "0", "checkout with spaces", false},
			{"flag-as-value", "create cards --base --no-focus", "{checkout}", "0", "checkout with spaces", true},
			{"end-options", "create -- --no-focus", "{checkout}", "0", "checkout with spaces", true},
		} {
			t.Run(shell+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				target := filepath.Join(root, "checkout with spaces")
				must(t, os.Mkdir(target, 0700))
				bin := filepath.Join(root, "namu")
				must(t, os.WriteFile(bin, []byte("#!/bin/sh\n[ \"$NAMU_SHELL_INTEGRATION\" = 1 ] || exit 99\nif [ -n \"$TEST_OUTPUT\" ]; then printf '%s\\n' \"$TEST_OUTPUT\"; fi\nif [ -n \"$TEST_DESTINATION\" ]; then printf '%s\\n' \"$TEST_DESTINATION\" > \"$NAMU_SHELL_RESULT\"; fi\necho diagnostics >&2\nexit \"$TEST_CODE\"\n"), 0700))
				output := strings.ReplaceAll(tc.output, "{checkout}", target)
				program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu " + tc.args + "\nnamu_code=$?\nprintf 'exit=%s\\npwd=%s\\n' \"$namu_code\" \"$(pwd -P)\"\n"
				cmd := exec.Command(shell, "-f", "-c", program)
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "TEST_PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_OUTPUT="+output, "TEST_CODE="+tc.code, "TEST_DESTINATION="+map[bool]string{true: filepath.Join(root, tc.destination), false: ""}[tc.navigate || tc.name == "setup-failure"], "TMPDIR="+root, "HOME="+root)
				var diagnostics bytes.Buffer
				cmd.Stderr = &diagnostics
				result, err := cmd.Output()
				must(t, err)
				check(t, diagnostics.String() == "diagnostics\n", diagnostics.String())
				actualRoot, err := filepath.EvalSymlinks(root)
				must(t, err)
				expected := map[bool]string{false: actualRoot, true: filepath.Join(actualRoot, tc.destination)}[tc.navigate]
				prefix := map[bool]string{true: output + "\n", false: ""}[output != ""]
				check(t, string(result) == prefix+"exit="+tc.code+"\npwd="+expected+"\n", string(result))
				files, err := filepath.Glob(filepath.Join(root, "namu-navigation.*"))
				must(t, err)
				check(t, len(files) == 0, "navigation files must be removed")
			})
		}
	}
}

func TestShellIntegrationWithRealCLIAndBothBackends(t *testing.T) {
	binaryDir := buildShellCLI(t)
	for _, shell := range []string{"bash", "zsh"} {
		for _, tc := range []struct {
			name              string
			flags             []string
			body              string
			navigate, warning bool
		}{
			{name: "git", flags: []string{"--herdr=false"}, body: "exit 7", navigate: true},
			{name: "herdr", flags: []string{"--herdr"}, body: `"$REAL_GIT" -C "$source" worktree add "$target" "$branch"`},
			{name: "herdr-fallback", flags: []string{"--herdr"}, body: "exit 7", navigate: true, warning: true},
			{name: "herdr-no-focus", flags: []string{"--herdr", "--no-focus"}, body: `"$REAL_GIT" -C "$source" worktree add "$target" "$branch"`},
			{name: "git-no-focus", flags: []string{"--herdr=false", "--focus=false"}},
		} {
			for _, action := range []string{"create", "open", "existing"} {
				t.Run(shell+"/"+tc.name+"/"+action, func(t *testing.T) {
					repo := testutil.Repository(t)
					body := map[string]string{"create": `"$REAL_GIT" -C "$source" branch "$branch" "$base"; ` + tc.body, "open": tc.body, "existing": map[bool]string{true: "exit 7", false: "exit 0"}[tc.warning]}[action]
					testutil.Herdr(t, body)
					testutil.Git(t, repo.Source, "branch", "feature/cards")
					t.Setenv("HOME", repo.Main)
					existing := filepath.Join(repo.Main, "existing checkout")
					testutil.Git(t, repo.Source, "branch", "feature/existing")
					testutil.Git(t, repo.Source, "worktree", "add", existing, "feature/existing")
					args := map[string]string{"create": "create feature/pricing --base main", "open": "open feature/cards", "existing": "open feature/existing"}[action]
					expectedPath := map[string]string{"create": filepath.Join(repo.Main, ".agents/worktrees/feature/pricing"), "open": filepath.Join(repo.Main, ".agents/worktrees/feature/cards"), "existing": existing}[action]
					program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu " + args + " " + strings.Join(tc.flags, " ") + "\nprintf 'pwd=%s\\n' \"$(pwd -P)\"\n"
					cmd := exec.Command(shell, "-f", "-c", program)
					cmd.Dir = repo.Source
					cmd.Env = append(os.Environ(), "TEST_PATH="+binaryDir+string(os.PathListSeparator)+os.Getenv("PATH"))
					var diagnostics bytes.Buffer
					cmd.Stderr = &diagnostics
					result, err := cmd.Output()
					must(t, err)
					navigate := tc.navigate
					expectedPWD := map[bool]string{true: expectedPath, false: repo.Source}[navigate]
					check(t, string(result) == pathdisplay.Shorten(expectedPath)+"\npwd="+expectedPWD+"\n", string(result))
					check(t, strings.Contains(diagnostics.String(), "Warning:") == tc.warning, diagnostics.String())
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
		name, shell, answer, marker, herdr string
		args                               []string
		redirected, prompt, installed      bool
		code                               int
	}{
		{name: "accept", shell: "zsh", answer: "yes\n", args: []string{"open", "feature/cards"}, prompt: true, installed: true},
		{name: "accept-bash", shell: "bash", answer: "yes\n", args: []string{"create", "feature/new", "--base", "main"}, prompt: true, installed: true},
		{name: "decline", shell: "zsh", answer: "no\n", args: []string{"open", "feature/cards"}, prompt: true},
		{name: "eof", shell: "zsh", answer: "\x04", args: []string{"open", "feature/cards"}, prompt: true},
		{name: "interrupt", shell: "zsh", answer: "interrupt", args: []string{"open", "feature/cards"}, prompt: true, code: 130},
		{name: "herdr", shell: "zsh", herdr: "1", args: []string{"open", "feature/cards"}},
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
			t.Setenv("HERDR_ENV", tc.herdr)
			testutil.Herdr(t, `"$REAL_GIT" -C "$source" worktree add "$target" "$branch"`)
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

func TestShellNavigationUnavailableStillRunsCommand(t *testing.T) {
	for _, shell := range []string{"bash", "zsh"} {
		t.Run(shell, func(t *testing.T) {
			root := t.TempDir()
			must(t, os.WriteFile(filepath.Join(root, "namu"), []byte("#!/bin/sh\n[ -z \"$NAMU_SHELL_RESULT\" ] || exit 99\nprintf 'checkout\\n'\nexit 7\n"), 0700))
			program := "export PATH=\"$TEST_PATH\"\n" + shellIntegration + "\nnamu open feature/cards\ncode=$?\nprintf 'exit=%s\\npwd=%s\\n' \"$code\" \"$(pwd -P)\"\n"
			cmd := exec.Command(shell, "-f", "-c", program)
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "TEST_PATH="+root+string(os.PathListSeparator)+os.Getenv("PATH"), "TMPDIR="+filepath.Join(root, "missing"), "NAMU_SHELL_RESULT=stale")
			var diagnostics bytes.Buffer
			cmd.Stderr = &diagnostics
			result, err := cmd.Output()
			must(t, err)
			actualRoot, err := filepath.EvalSymlinks(root)
			must(t, err)
			check(t, string(result) == "checkout\nexit=7\npwd="+actualRoot+"\n", string(result))
			check(t, strings.Contains(diagnostics.String(), "Warning: cannot prepare shell navigation"), diagnostics.String())
		})
	}
}
