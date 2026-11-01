package macos

import (
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

//go:embed testdata/mount-spaces.sh
var mountSpacesFixture string

//go:embed testdata/mount-other.sh
var mountOtherFixture string

//go:embed testdata/mount-check.sh
var mountCheckFixture string

func TestMacMountCheckRecognizesAppleVirtIOFSWithSpaces(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mount")
	if err := os.WriteFile(fake, []byte(mountSpacesFixture), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh")
	cmd.Stdin = strings.NewReader(mountCheck + mountCheckFixture)
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_MOUNT_ROOT=/Users/ada/project with spaces")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(string(output), err)
	}
	if len(output) != 0 {
		t.Fatal(string(output))
	}
}
func TestMacMountCheckRejectsAnotherMountpoint(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mount")
	if err := os.WriteFile(fake, []byte(mountOtherFixture), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh")
	cmd.Stdin = strings.NewReader(mountCheck + mountCheckFixture)
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TEST_MOUNT_ROOT=/Users/ada/project")
	err := cmd.Run()
	if err == nil {
		t.Fatal("must not reuse a different mountpoint")
	}
}

func TestMacBootstrapUsesHostKeysWithoutLinuxServiceManagement(t *testing.T) {
	script := Definition().Hooks.Bootstrap("ssh-ed25519 test-key")
	if !strings.Contains(script, "/usr/sbin/sshd -t") || !strings.Contains(script, "cat /etc/ssh/ssh_host_ed25519_key.pub") {
		t.Fatal("must validate SSH and return the per-clone host key")
	}
	if strings.Contains(script, "systemctl") || strings.Contains(script, "/run/sshd") {
		t.Fatal("Linux service management must not run in macOS guests")
	}
}
