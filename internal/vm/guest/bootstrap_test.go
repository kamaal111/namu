package guest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedGuestScriptsParseForBothOperatingSystems(t *testing.T) {
	for _, osName := range []string{"macos", "linux"} {
		for _, script := range []string{AccessScript(osName, publicKey(1)), BootstrapScript(osName, publicKey(1)), MountScript(osName, []string{"/Users/ada/雪 project", "/Volumes/work/quote'project"})} {
			cmd := exec.Command("sh", "-n")
			cmd.Stdin = strings.NewReader(script)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatal(string(output), err)
			}
			if len(output) != 0 {
				t.Fatal(string(output))
			}
		}
	}
}

func TestMacMountCheckRecognizesAppleVirtIOFSWithSpaces(t *testing.T) {
	dir := t.TempDir()
	fake := filepath.Join(dir, "mount")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' 'virtio-fs on /Users/ada/project with spaces (AppleVirtIOFS, nodev, nosuid)'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `root='/Users/ada/project with spaces'; `+macMountCheck)
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
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
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nprintf '%s\\n' 'virtio-fs on /Users/ada/project-other (AppleVirtIOFS, nodev, nosuid)'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", `root='/Users/ada/project'; `+macMountCheck)
	cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	err := cmd.Run()
	if err == nil {
		t.Fatal("must not reuse a different mountpoint")
	}
}
