package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func shellHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	return home
}

func TestShellInstallReportsExactChangesAndPreservesConfiguration(t *testing.T) {
	home := shellHome(t)
	path := filepath.Join(home, ".zshrc")
	must(t, os.WriteFile(path, []byte("export EDITOR=vim"), 0640))
	edits, err := prepareShellInstall("zsh")
	must(t, err)
	check(t, len(edits) == 1, "one startup file")
	check(t, edits[0].Path == path, edits[0].Path)
	var preview, report bytes.Buffer
	must(t, previewShellInstall(edits, &preview))
	must(t, applyShellInstall(edits, &report))
	content, err := os.ReadFile(path)
	must(t, err)
	check(t, string(content) == "export EDITOR=vim"+edits[0].Addition, string(content))
	check(t, strings.Contains(preview.String(), path+"\n"+edits[0].Addition), preview.String())
	check(t, strings.Contains(report.String(), path+"\n"+edits[0].Addition), report.String())
	info, err := os.Stat(path)
	must(t, err)
	check(t, info.Mode().Perm() == 0640, "preserve permissions")
	again, err := prepareShellInstall("zsh")
	must(t, err)
	check(t, again[0].Addition == "", "must not duplicate integration")
	report.Reset()
	must(t, applyShellInstall(again, &report))
	check(t, strings.Contains(report.String(), "Already configured: "+path), report.String())
}

func TestShellInstallDestinations(t *testing.T) {
	for _, tc := range []struct {
		name, shell, existing string
		files                 []string
	}{
		{"zsh", "zsh", "", []string{".zshrc"}},
		{"bash-new", "bash", "", []string{".bashrc", ".bash_profile"}},
		{"bash-login", "bash", ".bash_login", []string{".bashrc", ".bash_login"}},
		{"bash-profile", "bash", ".profile", []string{".bashrc", ".profile"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := shellHome(t)
			createStartupFixture(t, home, tc.existing)
			edits, err := prepareShellInstall(tc.shell)
			must(t, err)
			check(t, len(edits) == len(tc.files), "destination count")
			for i, file := range tc.files {
				check(t, edits[i].Path == filepath.Join(home, file), edits[i].Path)
			}
		})
	}
}

func createStartupFixture(t *testing.T, home, name string) {
	t.Helper()
	if name != "" {
		must(t, os.WriteFile(filepath.Join(home, name), []byte("# existing\n"), 0600))
	}
}

func TestShellInstallRespectsZdotdirAndSymlink(t *testing.T) {
	home := shellHome(t)
	dot := filepath.Join(home, "zsh")
	must(t, os.Mkdir(dot, 0700))
	target := filepath.Join(home, "actual-zshrc")
	must(t, os.WriteFile(target, []byte("# config\n"), 0600))
	must(t, os.Symlink(target, filepath.Join(dot, ".zshrc")))
	t.Setenv("ZDOTDIR", dot)
	edits, err := prepareShellInstall("zsh")
	must(t, err)
	check(t, edits[0].Path == target, edits[0].Path)
	must(t, applyShellInstall(edits, &bytes.Buffer{}))
	link, err := os.Readlink(filepath.Join(dot, ".zshrc"))
	must(t, err)
	check(t, link == target, "preserve symlink")
}

func TestShellInstallRecognizesExistingEval(t *testing.T) {
	home := shellHome(t)
	must(t, os.WriteFile(filepath.Join(home, ".zshrc"), []byte("eval \"$(namu shell-init zsh)\"\n"), 0600))
	edits, err := prepareShellInstall("zsh")
	must(t, err)
	check(t, edits[0].Addition == "", "existing initialization must not be duplicated")
}

func TestShellInstallRejectsUnsupportedShellAndDirectory(t *testing.T) {
	home := shellHome(t)
	_, err := prepareShellInstall("fish")
	check(t, err != nil, "unsupported shell must fail")
	must(t, os.Mkdir(filepath.Join(home, ".zshrc"), 0700))
	_, err = prepareShellInstall("zsh")
	check(t, err != nil, "directory must fail before installation")
}

func TestShellInstallReportsPartialFailure(t *testing.T) {
	home := shellHome(t)
	edits, err := prepareShellInstall("bash")
	must(t, err)
	must(t, os.Mkdir(filepath.Join(home, ".bash_profile"), 0700))
	var report bytes.Buffer
	err = applyShellInstall(edits, &report)
	check(t, err != nil, "second destination must fail")
	content, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	must(t, err)
	check(t, string(content) == edits[0].Addition, string(content))
	check(t, strings.Contains(report.String(), "Added to: "+edits[0].Path), report.String())
	check(t, !strings.Contains(report.String(), "Added to: "+edits[1].Path), report.String())
}

func TestShellInstallRejectsChangedConfigurationAfterPreview(t *testing.T) {
	home := shellHome(t)
	path := filepath.Join(home, ".zshrc")
	must(t, os.WriteFile(path, []byte("# original\n"), 0600))
	edits, err := prepareShellInstall("zsh")
	must(t, err)
	must(t, os.WriteFile(path, []byte("# changed\n"), 0600))
	err = applyShellInstall(edits, &bytes.Buffer{})
	check(t, err != nil, "concurrent changes require a fresh preview")
	content, err := os.ReadFile(path)
	must(t, err)
	check(t, string(content) == "# changed\n", string(content))
}

func TestShellInstallConfirmation(t *testing.T) {
	for _, tc := range []struct {
		input    string
		accepted bool
	}{{"yes\n", true}, {"Y\n", true}, {"no\n", false}, {"\n", false}, {"", false}, {"yes", false}} {
		t.Run(tc.input, func(t *testing.T) {
			accepted, err := confirmShellInstall(context.Background(), strings.NewReader(tc.input), &bytes.Buffer{})
			must(t, err)
			check(t, accepted == tc.accepted, "confirmation result")
		})
	}
}

func TestShellInstallConfirmationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	accepted, err := confirmShellInstall(ctx, strings.NewReader("yes\n"), &bytes.Buffer{})
	check(t, errors.Is(err, context.Canceled), "must preserve cancellation")
	check(t, !accepted, "cancellation must not authorize installation")
}

func TestOfferShellInstallAcceptAndDecline(t *testing.T) {
	for _, accepted := range []bool{true, false} {
		t.Run(map[bool]string{true: "accept", false: "decline"}[accepted], func(t *testing.T) {
			home := shellHome(t)
			var report bytes.Buffer
			err := offerShellInstall(context.Background(), "zsh", &report, func() (bool, error) { return accepted, nil })
			must(t, err)
			check(t, strings.Contains(report.String(), filepath.Join(home, ".zshrc")), report.String())
			_, err = os.Stat(filepath.Join(home, ".zshrc"))
			check(t, (err == nil) == accepted, "only acceptance may write configuration")
			check(t, strings.Contains(report.String(), "Reload") == accepted, report.String())
		})
	}
}

func TestExplicitShellInstallerOutsideRepository(t *testing.T) {
	home := shellHome(t)
	out, diagnostics, err := execute(t, t.TempDir(), "shell-init", "zsh", "--install")
	must(t, err)
	check(t, out == "", out)
	content, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	must(t, err)
	check(t, strings.Contains(diagnostics, "Added to: "+filepath.Join(home, ".zshrc")+"\n"+string(content)), diagnostics)
}
