package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
)

func runNu(t *testing.T, dir, program string) (string, string, error) {
	t.Helper()
	script := filepath.Join(t.TempDir(), "init.nu")
	must(t, os.WriteFile(script, []byte(nuShellIntegration), 0600))
	source, err := json.Marshal(script)
	must(t, err)
	cmd := exec.Command("nu", "--no-config-file", "--commands", "source "+string(source)+"; "+program)
	cmd.Dir = dir
	var diagnostics bytes.Buffer
	cmd.Stderr = &diagnostics
	out, err := cmd.Output()
	return string(out), diagnostics.String(), err
}

func TestNuListAndFormats(t *testing.T) {
	testutil.GitHub(t, "exit 1")
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := testutil.Repository(t)
	for _, tc := range []struct{ name, program, want string }{
		{"records", `namu list --no-pr | where branch == 'main' | get branch | to json --raw`, `["main"]`},
		{"json", `namu list --json --no-pr | jq -c 'map(.branch)'`, `["main"]`},
		{"table", `namu list --table --no-pr | ^cat`, "BRANCH"},
		{"json-false", `namu list --json=false --no-pr | ^cat`, "BRANCH"},
		{"table-false", `namu list --table=false --no-pr | ^cat`, "BRANCH"},
		{"help", `namu list --help`, "List all registered checkouts"},
		{"root", `namu --version`, "namu version"},
		{"root-help", `namu --help`, "Create, open, list and delete Git worktrees"},
		{"root-no-args", `namu`, "Create, open, list and delete Git worktrees"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, diagnostics, err := runNu(t, repo.Source, tc.program)
			check(t, err == nil, fmt.Sprint(err)+diagnostics)
			check(t, strings.Contains(out, tc.want), out)
			check(t, diagnostics == "", diagnostics)
		})
	}
}

func TestNuListErrors(t *testing.T) {
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, args := range []string{"list", "list --unknown", "list --table --json"} {
		t.Run(args, func(t *testing.T) {
			out, diagnostics, err := runNu(t, t.TempDir(), "namu "+args)
			check(t, err != nil, diagnostics)
			check(t, out == "", out)
			check(t, !strings.Contains(diagnostics, "from json"), diagnostics)
		})
	}
}

func TestNuNavigation(t *testing.T) {
	testutil.GitHub(t, "exit 1")
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REX_SESSION", "")
	t.Setenv("HERDR_ENV", "")
	for _, tc := range []struct{ name, args, destination string }{
		{"create", "create feature/new --base main", "cards"},
		{"open", "open feature/cards", "cards"},
		{"no-focus", "open feature/cards --no-focus", "source"},
		{"focus-false", "open feature/cards --focus=false", "source"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			target := filepath.Join(repo.Main, ".agents", "worktrees", strings.Fields(tc.args)[1])
			testutil.Git(t, repo.Source, "branch", "feature/cards")
			program := "print (namu " + tc.args + "); print $env.PWD"
			out, diagnostics, err := runNu(t, repo.Source, program)
			check(t, err == nil, fmt.Sprint(err)+diagnostics)
			expected := map[string]string{"cards": target, "source": repo.Main}[tc.destination]
			check(t, strings.TrimSpace(out) == target+"\n"+expected, out+diagnostics)
		})
	}
}

func TestNuCompletion(t *testing.T) {
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := testutil.Repository(t)
	testutil.Git(t, repo.Source, "branch", "feature/cards")
	testutil.Git(t, repo.Source, "remote", "add", "origin", "https://invalid.example/repo")
	testutil.Git(t, repo.Source, "worktree", "add", filepath.Join(repo.Main, ".agents", "worktrees", "feature/cards"), "feature/cards")
	before := testutil.Git(t, repo.Source, "worktree", "list", "--porcelain")
	for _, tc := range []struct{ input, want string }{
		{"namu op", "open"},
		{"namu list --j", "--json"},
		{"namu list --t", "--table"},
		{"namu open feature/", "feature/cards"},
		{"namu open ", "feature/cards"},
		{"namu open 'feature/'", "feature/cards"},
		{"namu open 'feature/", "feature/cards"},
		{"namu delete feature/cards ", "[]"},
		{"namu create new --base feature/", "feature/cards"},
		{"namu create new --remote or", "origin"},
		{"echo x | namu open feature/", "feature/cards"},
		{"namu shell-init n", "nu"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			input, err := json.Marshal(tc.input)
			must(t, err)
			out, diagnostics, err := runNu(t, repo.Source, string(input)+" | commandline complete | to json --raw")
			check(t, err == nil, fmt.Sprint(err)+diagnostics)
			check(t, strings.Contains(out, tc.want), out+diagnostics)
		})
	}
	check(t, testutil.Git(t, repo.Source, "worktree", "list", "--porcelain") == before, "completion must not alter worktrees")
}

func TestListPortablePipelines(t *testing.T) {
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	repo := testutil.Repository(t)
	for _, shell := range []string{"bash", "zsh"} {
		for _, tc := range []struct{ args, want string }{
			{`--json --no-pr | jq -c 'map(.branch)'`, "[\"main\"]\n"},
			{`--table --no-pr | cat`, "BRANCH"},
		} {
			t.Run(shell+tc.args, func(t *testing.T) {
				cmd := exec.Command(shell, "-f", "-c", "namu list "+tc.args)
				cmd.Dir = repo.Source
				out, err := cmd.CombinedOutput()
				check(t, err == nil, fmt.Sprint(err)+string(out))
				check(t, strings.Contains(string(out), tc.want), string(out))
			})
		}
	}
}

func TestNuShellInitOutsideRepository(t *testing.T) {
	out, diagnostics, err := execute(t, t.TempDir(), "shell-init", "nu")
	must(t, err)
	check(t, out == nuShellIntegration, out)
	check(t, diagnostics == "", diagnostics)
}

func TestNuInstallAndUpgrade(t *testing.T) {
	home := shellHome(t)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config with spaces"))
	edits, err := prepareShellInstall("nu")
	must(t, err)
	check(t, len(edits) == 1 && edits[0].Replace, fmt.Sprint(edits))
	check(t, strings.HasPrefix(edits[0].Path, filepath.Join(home, "config with spaces")), edits[0].Path)
	var report bytes.Buffer
	must(t, applyShellInstall(edits, &report))
	check(t, strings.Contains(report.String(), "Wrote:"), report.String())
	content, err := os.ReadFile(edits[0].Path)
	must(t, err)
	check(t, string(content) == nuShellIntegration, string(content))
	again, err := prepareShellInstall("nu")
	must(t, err)
	check(t, again[0].Addition == "", "installation must be idempotent")
	old := "# Managed by namu shell-init nu --install.\n# old version\n"
	must(t, os.WriteFile(edits[0].Path, []byte(old), 0640))
	must(t, os.Chmod(edits[0].Path, 0640))
	upgrade, err := prepareShellInstall("nu")
	must(t, err)
	must(t, applyShellInstall(upgrade, &report))
	content, err = os.ReadFile(upgrade[0].Path)
	must(t, err)
	check(t, string(content) == nuShellIntegration, string(content))
	info, err := os.Stat(upgrade[0].Path)
	must(t, err)
	check(t, info.Mode().Perm() == 0640, "upgrade preserves permissions")
}

func TestNuInstallRejectsUnmanagedAndConcurrentChanges(t *testing.T) {
	home := shellHome(t)
	t.Setenv("XDG_CONFIG_HOME", home)
	edits, err := prepareShellInstall("nu")
	must(t, err)
	must(t, os.MkdirAll(filepath.Dir(edits[0].Path), 0700))
	must(t, os.WriteFile(edits[0].Path, []byte("# my own integration\n"), 0600))
	_, err = prepareShellInstall("nu")
	check(t, err != nil && strings.Contains(err.Error(), "unmanaged"), fmt.Sprint(err))
	must(t, os.WriteFile(edits[0].Path, []byte(nuShellIntegration+"\n"), 0600))
	upgrade, err := prepareShellInstall("nu")
	must(t, err)
	must(t, os.WriteFile(edits[0].Path, []byte("# concurrent changes\n"), 0600))
	err = applyShellInstall(upgrade, &bytes.Buffer{})
	check(t, err != nil && strings.Contains(err.Error(), "changed since preview"), fmt.Sprint(err))
	content, err := os.ReadFile(edits[0].Path)
	must(t, err)
	check(t, string(content) == "# concurrent changes\n", string(content))
}

func TestNuInstallPreservesSymlink(t *testing.T) {
	home := shellHome(t)
	t.Setenv("XDG_CONFIG_HOME", home)
	edits, err := prepareShellInstall("nu")
	must(t, err)
	must(t, os.MkdirAll(filepath.Dir(edits[0].Path), 0700))
	target := filepath.Join(home, "actual.nu")
	must(t, os.WriteFile(target, []byte("# Managed by namu shell-init nu --install.\n"), 0600))
	must(t, os.Symlink(target, edits[0].Path))
	upgrade, err := prepareShellInstall("nu")
	must(t, err)
	must(t, applyShellInstall(upgrade, &bytes.Buffer{}))
	link, err := os.Readlink(edits[0].Path)
	must(t, err)
	check(t, link == target, link)
	content, err := os.ReadFile(target)
	must(t, err)
	check(t, string(content) == nuShellIntegration, string(content))
}

func TestNuInstalledIntegration(t *testing.T) {
	binary := buildShellCLI(t)
	home := shellHome(t)
	t.Setenv("XDG_CONFIG_HOME", home)
	testutil.GitHub(t, "exit 1")

	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REX_SESSION", "")
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	_, diagnostics, err := execute(t, repo.Source, "shell-init", "nu", "--install")
	must(t, err)
	check(t, strings.Contains(diagnostics, "exec nu"), diagnostics)
	cmd := exec.Command("nu", "--execute", "print (namu create feature/cards --base main); print (namu list --no-pr | where branch == 'feature/cards' | get path | first); print $env.PWD; exit")
	cmd.Dir = repo.Source
	out, err := cmd.CombinedOutput()
	check(t, err == nil, fmt.Sprint(err)+string(out))
	target := filepath.Join(repo.Main, ".agents", "worktrees", "feature/cards")
	check(t, strings.TrimSpace(string(out)) == target+"\n"+target+"\n"+target, string(out))
}

func TestNuNavigationFailureCleansUp(t *testing.T) {
	testutil.GitHub(t, "exit 1")
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("REX_SESSION", "")
	t.Setenv("HERDR_ENV", "")
	temp, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	t.Setenv("TMPDIR", temp)
	repo := testutil.Repository(t)
	must(t, os.WriteFile(filepath.Join(repo.Source, ".namu.toml"), []byte("[setup]\ncommand = ['sh', '-c', 'echo hook-failed >&2; exit 7']\n"), 0600))
	out, diagnostics, err := runNu(t, repo.Source, "namu create feature/cards --base main")
	check(t, err != nil, out+diagnostics)
	check(t, strings.Contains(diagnostics, "hook-failed"), diagnostics)
	files, err := filepath.Glob(filepath.Join(temp, "namu-navigation.*"))
	must(t, err)
	check(t, len(files) == 0, fmt.Sprint(files))
}

func TestNuNavigationUnavailableStillForwards(t *testing.T) {
	nu, err := exec.LookPath("nu")
	must(t, err)
	bin := t.TempDir()
	must(t, os.Symlink(nu, filepath.Join(bin, "nu")))
	must(t, os.WriteFile(filepath.Join(bin, "namu"), []byte("#!/bin/sh\n[ \"$NAMU_SHELL_INTEGRATION\" = 1 ] || exit 99\nprintf '%s\\n' \"$@\"\n"), 0700))
	t.Setenv("PATH", bin)
	out, diagnostics, err := runNu(t, bin, "namu open 'feature/cards'")
	check(t, err == nil, fmt.Sprint(err)+diagnostics)
	check(t, out == "open\nfeature/cards\n", out)
	check(t, strings.Contains(diagnostics, "Warning: cannot prepare shell navigation"), diagnostics)
}

func TestNuForwardingPreservesStdin(t *testing.T) {
	bin := t.TempDir()
	must(t, os.WriteFile(filepath.Join(bin, "namu"), []byte("#!/bin/sh\n[ \"$NAMU_SHELL_INTEGRATION\" = 1 ] || exit 99\ncat\n"), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, diagnostics, err := runNu(t, bin, "'hello' | namu version")
	check(t, err == nil, fmt.Sprint(err)+diagnostics)
	check(t, strings.TrimSpace(out) == "hello", out)
}

func TestNuHerdrNavigation(t *testing.T) {
	binary := buildShellCLI(t)
	for _, tc := range []struct {
		name, body, flags string
		navigate, warning bool
	}{
		{"handled", `"$REAL_GIT" -C "$source" worktree add "$target" "$branch"`, "", false, false},
		{"fallback", "exit 7", "", true, true},
		{"no-focus", `"$REAL_GIT" -C "$source" worktree add "$target" "$branch"`, "--no-focus", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Herdr(t, tc.body)
			t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
			repo := testutil.Repository(t)
			testutil.Git(t, repo.Source, "branch", "feature/cards")
			target := filepath.Join(repo.Main, ".agents", "worktrees", "feature/cards")
			out, diagnostics, err := runNu(t, repo.Source, "print (namu open feature/cards --herdr "+tc.flags+"); print $env.PWD")
			check(t, err == nil, fmt.Sprint(err)+diagnostics)
			expected := map[bool]string{true: target, false: repo.Main}[tc.navigate]
			check(t, strings.TrimSpace(out) == target+"\n"+expected, out)
			check(t, strings.Contains(diagnostics, "Warning:") == tc.warning, diagnostics)
		})
	}
}

func TestNuRexNavigation(t *testing.T) {
	binary := buildShellCLI(t)
	for _, tc := range []struct {
		name, body string
		navigate   bool
	}{
		{"handled", testutil.RexResponses, false},
		{"fallback", "exit 7", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Rex(t, tc.body)
			t.Setenv("HERDR_ENV", "")
			t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
			repo := testutil.Repository(t)
			testutil.Git(t, repo.Source, "branch", "feature/cards")
			target := filepath.Join(repo.Main, ".agents", "worktrees", "feature/cards")
			out, diagnostics, err := runNu(t, repo.Source, "print (namu open feature/cards); print $env.PWD")
			check(t, err == nil, fmt.Sprint(err)+diagnostics)
			expected := map[bool]string{true: target, false: repo.Main}[tc.navigate]
			check(t, strings.TrimSpace(out) == target+"\n"+expected, out)
			check(t, strings.Contains(diagnostics, "Warning:") == tc.navigate, diagnostics)
		})
	}
}

func TestNuNavigationExistingSpacePath(t *testing.T) {
	binary := buildShellCLI(t)
	t.Setenv("PATH", binary+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HERDR_ENV", "")
	repo := testutil.Repository(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	target := filepath.Join(parent, "checkout with spaces")
	testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/cards", target)
	t.Setenv("HOME", parent)
	out, diagnostics, err := runNu(t, repo.Source, "print (namu open feature/cards); print $env.PWD")
	check(t, err == nil, fmt.Sprint(err)+diagnostics)
	check(t, strings.TrimSpace(out) == "~/checkout with spaces\n"+target, out)
}

func TestNuNavigationPreservesFailureCode(t *testing.T) {
	bin := t.TempDir()
	must(t, os.WriteFile(filepath.Join(bin, "namu"), []byte("#!/bin/sh\necho failed >&2\nexit 7\n"), 0700))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	temp, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	t.Setenv("TMPDIR", temp)
	out, diagnostics, err := runNu(t, bin, "namu open feature/cards")
	var exitError *exec.ExitError
	check(t, errors.As(err, &exitError), fmt.Sprint(err)+diagnostics)
	check(t, exitError.ExitCode() == 7, diagnostics)
	check(t, out == "", out)
	files, err := filepath.Glob(filepath.Join(temp, "namu-navigation.*"))
	must(t, err)
	check(t, len(files) == 0, fmt.Sprint(files))
}
