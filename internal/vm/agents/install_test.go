package agents

import (
	"embed"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/*.sh
var fixtureScripts embed.FS

func fixtureScript(t *testing.T, name string) string {
	t.Helper()
	content, err := fixtureScripts.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func writeInstallerFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
}

func nativeInstallationFixture(t *testing.T, osName string) (string, *exec.Cmd) {
	t.Helper()
	home := t.TempDir()
	fixtures := t.TempDir()
	writeInstallerFixture(t, filepath.Join(fixtures, "codex-installer.sh"), fixtureScript(t, "codex-installer.sh"))
	writeInstallerFixture(t, filepath.Join(fixtures, "claude-installer.sh"), fixtureScript(t, "claude-installer.sh"))
	writeInstallerFixture(t, filepath.Join(home, ".local/share/namu-agents", Revision, "cursor/complete"), "")
	writeInstallerFixture(t, filepath.Join(home, ".local/share/namu-agents", Revision, "cursor/cursor-agent"), fixtureScript(t, "cursor-agent.sh"))
	definition, err := registry.Resolve(osName)
	if err != nil {
		t.Fatal(err)
	}
	script, err := Script(definition)
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh")
	cmd.Stdin = strings.NewReader(fixtureScript(t, "commands.sh") + script)
	cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "TEST_FIXTURES=" + fixtures, "TEST_LOG=" + filepath.Join(home, "install.log")}
	return home, cmd
}

func TestNativeInstallersKeepCredentialLaunchersSeparateFromBinaries(t *testing.T) {
	for _, osName := range registry.Names() {
		t.Run(osName, func(t *testing.T) {
			home, cmd := nativeInstallationFixture(t, osName)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatal(err, string(output))
			}
			log, err := os.ReadFile(filepath.Join(home, "install.log"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(log), "codex  noninteractive=1\n") || !strings.Contains(string(log), "claude \n") {
				t.Fatal(string(log))
			}
			if strings.Contains(string(log), "npm") || strings.Contains(string(log), "nodejs.org") {
				t.Fatal(string(log))
			}
			for _, alias := range []string{"agent", "cursor-agent"} {
				invoke := exec.Command(filepath.Join(home, ".local/bin", alias), "--version")
				invoke.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
				result, err := invoke.CombinedOutput()
				if err != nil {
					t.Fatal(err, string(result))
				}
				if string(result) != "Cursor-test\n" {
					t.Fatal(string(result))
				}
			}
			for _, agent := range []string{"codex", "claude"} {
				target, err := os.Readlink(filepath.Join(home, ".local/bin", agent))
				if err != nil {
					t.Fatal(err)
				}
				if target != filepath.Join(home, ".local/share/native-agents", agent) {
					t.Fatal(target)
				}
				invoke := exec.Command(filepath.Join(home, ".local/share/namu-agents/bin", agent), "argument with spaces")
				invoke.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
				result, err := invoke.CombinedOutput()
				if err != nil {
					t.Fatal(err, string(result))
				}
				if !strings.Contains(string(result), "home=guest-default args=argument with spaces") {
					t.Fatal(string(result))
				}
			}
		})
	}
}

func TestFailedNativeInstallerDownloadDoesNotMarkInstallationComplete(t *testing.T) {
	home, cmd := nativeInstallationFixture(t, "linux")
	cmd.Env = append(cmd.Env, "TEST_FAIL_DOWNLOAD=1")
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("download failure must stop installation", string(output))
	}
	_, err = os.Stat(filepath.Join(home, ".local/share/namu-agents", Revision, "codex/complete"))
	if !os.IsNotExist(err) {
		t.Fatal("must not mark failed installation complete")
	}
}

func TestInstallationScriptsParseOnBothGuestOperatingSystems(t *testing.T) {
	for _, osName := range registry.Names() {
		definition, err := registry.Resolve(osName)
		if err != nil {
			t.Fatal(err)
		}
		script, err := Script(definition)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sh", "-n")
		cmd.Stdin = strings.NewReader(script)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatal(err, string(output))
		}
		if len(output) != 0 {
			t.Fatal(string(output))
		}
	}
}
func launcher(t *testing.T, agent string) (string, string) {
	t.Helper()
	home := t.TempDir()
	bin := filepath.Join(home, ".local/bin")
	writeInstallerFixture(t, filepath.Join(bin, agent), fixtureScript(t, "agent-profile.sh"))

	path := filepath.Join(home, agent)
	writeInstallerFixture(t, path, strings.ReplaceAll(fixtureScript(t, "wrapper-header.sh"), "@REVISION@", Revision)+wrapper)
	return home, path
}
func TestLauncherOptOutUsesGuestDefaultProfile(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			home, path := launcher(t, agent)
			cmd := exec.Command(path)
			cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}
			result, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(result) != "guest-default\n" {
				t.Fatal(string(result))
			}
		})
	}
}
func TestLauncherReuseSelectsSeparateNativeProfile(t *testing.T) {
	for _, agent := range []string{"codex", "claude"} {
		t.Run(agent, func(t *testing.T) {
			home, path := launcher(t, agent)
			profile := filepath.Join(home, "session")
			if err := os.Mkdir(profile, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(profile, agent+".env"), nil, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(path)
			cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "NAMU_HOST_CREDENTIALS_DIR=" + profile}
			result, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(result) != filepath.Join(home, ".namu-vm/host-auth", agent)+"\n" {
				t.Fatal(string(result))
			}
		})
	}
}
