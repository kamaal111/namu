package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, value bool, message string) {
	t.Helper()
	if !value {
		t.Fatal(message)
	}
}
func TestMissingConfigHasNoHook(t *testing.T) {
	c, err := Load(t.TempDir())
	must(t, err)
	check(t, len(c.Setup.Command) == 0, "unexpected hook")
}
func TestLoadCommandPreservesArgumentBoundaries(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[setup]\ncommand = ['just', 'setup-worktree', 'a b']\n"), 0600))
	c, err := Load(dir)
	must(t, err)
	check(t, len(c.Setup.Command) == 3, "wrong argument count")
	check(t, c.Setup.Command[2] == "a b", "argument was split")
}
func TestRejectUnknownConfig(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[setup]\ncommnad = ['true']\n"), 0600))
	_, err := Load(dir)
	check(t, err != nil, "unknown config must fail")
}
func TestRejectSymlinkConfig(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "target"), []byte(""), 0600))
	must(t, os.Symlink("target", filepath.Join(dir, ".namu.toml")))
	_, err := Load(dir)
	check(t, err != nil, "symlink config must fail")
}

func TestLoadFocusDefaults(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		want          *bool
	}{
		{"absent", "", nil},
		{"enabled", "[defaults]\nfocus = true\n", boolPointer(true)},
		{"disabled-with-setup", "[defaults]\nfocus = false\n[setup]\ncommand = ['true']\n", boolPointer(false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte(tc.content), 0600))
			c, err := Load(dir)
			must(t, err)
			check(t, reflect.DeepEqual(c.Defaults.Focus, tc.want), "unexpected focus default")
		})
	}
}

func boolPointer(value bool) *bool { return &value }

func TestRejectInvalidDefaults(t *testing.T) {
	for _, content := range []string{
		"[defaults]\nfocus = 'false'\n",
		"[defaults]\npr = 'false'\n",
		"[defaults]\nrex = false\n",
		"[defaults]\nherdr = true\n",
		"[defaults]\nfocus =",
	} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte(content), 0600))
			_, err := Load(dir)
			check(t, err != nil, "invalid defaults must fail")
		})
	}
}

func TestLoadPRDefaults(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			dir := t.TempDir()
			must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[defaults]\npr = "+strconv.FormatBool(enabled)+"\nfocus = false\n"), 0600))
			c, err := Load(dir)
			must(t, err)
			check(t, reflect.DeepEqual(c.Defaults.PR, &enabled), "unexpected PR default")
			check(t, reflect.DeepEqual(c.Defaults.Focus, boolPointer(false)), "focus default changed")
		})
	}
}

func TestLoadVMAndCredentialDefaults(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[defaults]\nvm = true\nreuse_credentials = false\n"), 0600))
	cfg, err := Load(dir)
	must(t, err)
	check(t, reflect.DeepEqual(cfg.Defaults.VM, boolPointer(true)), "must load VM default")
	check(t, reflect.DeepEqual(cfg.Defaults.ReuseCredentials, boolPointer(false)), "must load credential default")
}
func TestMissingVMDefaultRemainsUnspecified(t *testing.T) {
	cfg, err := Load(t.TempDir())
	must(t, err)
	check(t, cfg.Defaults.VM == nil && cfg.Defaults.ReuseCredentials == nil, "must preserve built-in defaults")
}
func TestRejectInvalidVMAndCredentialDefaults(t *testing.T) {
	for _, content := range []string{"[defaults]\nvm = 'true'\n", "[defaults]\nreuse_credentials = 'false'\n"} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte(content), 0600))
			_, err := Load(dir)
			check(t, err != nil, "must reject non-boolean defaults")
		})
	}
}

func TestLoadVMSettings(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[vm]\nos = 'linux'\nname = 'sandbox'\ncpus = 2\nmemory_mb = 4096\ndisk_gb = 90\n"), 0600))
	cfg, err := Load(dir)
	must(t, err)
	check(t, cfg.VM.OS == "linux" && cfg.VM.Name == "sandbox", "VM selection must load")
	check(t, reflect.DeepEqual(cfg.VM.CPUs, new(2)), "CPU count must load")
	check(t, reflect.DeepEqual(cfg.VM.MemoryMB, new(4096)), "memory must load")
	check(t, reflect.DeepEqual(cfg.VM.DiskGB, new(90)), "disk must load")
}

func TestRejectInvalidVMSettings(t *testing.T) {
	for _, field := range []string{"cpus = 0", "memory_mb = -1", "disk_gb = 65536", "cpus = '2'", "os = 'windows'", "name = '../unsafe'", "memory = 4096"} {
		t.Run(field, func(t *testing.T) {
			dir := t.TempDir()
			must(t, os.WriteFile(filepath.Join(dir, ".namu.toml"), []byte("[vm]\n"+field+"\n"), 0600))
			_, err := Load(dir)
			check(t, err != nil, "invalid VM setting must fail")
		})
	}
}
