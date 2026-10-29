package config

import (
	"os"
	"path/filepath"
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
