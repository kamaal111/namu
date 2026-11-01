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
