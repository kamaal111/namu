package registry_test

import (
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/vm/platform"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"github.com/kamaal111/namu/internal/vm/resources"
)

func TestRegistryPreservesGuestContracts(t *testing.T) {
	for _, tc := range []struct {
		name, label, home, kernel, download string
		disk, minimumHost                   int
	}{
		{"macos", "macOS 27", "/Users/admin", "Darwin", "darwin", 80, 27},
		{"linux", "Linux (Ubuntu 24.04)", "/home/admin", "Linux", "linux", 60, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, err := registry.Resolve(tc.name)
			if err != nil {
				t.Fatal(err)
			}
			if d.Name != tc.name || d.Label != tc.label || d.DefaultVMName != "namu-"+tc.name || d.User != "admin" || d.Home != tc.home || d.Kernel != tc.kernel || d.DownloadPlatform != tc.download {
				t.Fatal(d)
			}
			if d.Resources != (resources.Settings{CPUs: 4, MemoryMB: 8192, DiskGB: tc.disk}) || d.Host.MinimumMacOSMajor != tc.minimumHost {
				t.Fatal(d)
			}
		})
	}
}

func TestRegistryNamesAndDefaultAreStableAndCallerOwned(t *testing.T) {
	if !reflect.DeepEqual(registry.Names(), []string{"macos", "linux"}) || registry.Default().Name != "macos" {
		t.Fatal("unexpected registry selection")
	}
	definitions := registry.All()
	definitions[0].Name = "modified"
	names := registry.Names()
	names[0] = "modified"
	if registry.All()[0].Name != "macos" || registry.Names()[0] != "macos" {
		t.Fatal("callers must not mutate the registry")
	}
}

func TestRegistryRejectsUnknownAndEmptyNames(t *testing.T) {
	for _, name := range []string{"", "windows", "MACOS", " linux"} {
		t.Run(name, func(t *testing.T) {
			_, err := registry.Resolve(name)
			if err == nil || !strings.Contains(err.Error(), "unsupported guest OS") {
				t.Fatal(err)
			}
		})
	}
}

func TestPlatformRejectsIncompleteDefinitions(t *testing.T) {
	d := registry.Default()
	d.Hooks.Mount = nil
	if err := d.Validate(); err == nil {
		t.Fatal("missing operation must fail")
	}
	if err := (platform.Definition{}).Validate(); err == nil {
		t.Fatal("missing metadata must fail")
	}
}

func TestPlatformScriptsParseForEveryRegisteredAdapter(t *testing.T) {
	for _, d := range registry.All() {
		t.Run(d.Name, func(t *testing.T) {
			for _, script := range []string{d.Hooks.Access("ssh-ed25519 test-key'with-quote"), d.Hooks.Bootstrap("ssh-ed25519 test-key"), d.Hooks.Essentials, d.Hooks.AgentPrerequisites, platform.RootShell() + d.Hooks.MountPrelude + d.Hooks.Mount("/Volumes/work/雪 'project", "test-tag")} {
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
		})
	}
}
