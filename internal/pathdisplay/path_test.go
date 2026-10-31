package pathdisplay

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShorten(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tc := range []struct{ name, path, want string }{
		{"home", home, "~"},
		{"descendant", home + "/Projects/namu", "~/Projects/namu"},
		{"similar prefix", home + "-other/namu", home + "-other/namu"},
		{"outside", "/tmp/namu", "/tmp/namu"},
		{"literal suffix", home + "/checkout with spaces/$x;`cmd`", "~/checkout with spaces/$x;`cmd`"},
		{"relative", "Projects/namu", "Projects/namu"},
		{"empty", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := Shorten(tc.path); got != tc.want {
				t.Errorf("Shorten(%q) = %q; want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestShortenWithoutHome(t *testing.T) {
	t.Setenv("HOME", "")
	if got := Shorten("/tmp/namu"); got != "/tmp/namu" {
		t.Errorf("unexpected path: %q", got)
	}
}

func TestShortenSymlinkedHome(t *testing.T) {
	physical, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "home")
	if err := os.Symlink(physical, alias); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", alias)
	for _, path := range []string{alias + "/namu", physical + "/namu"} {
		if got := Shorten(path); got != "~/namu" {
			t.Errorf("Shorten(%q) = %q", path, got)
		}
	}
}
