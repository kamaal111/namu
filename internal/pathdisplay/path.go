// Package pathdisplay abbreviates home directories for display only.
package pathdisplay

import (
	"os"
	"path/filepath"
	"strings"
)

// Shorten replaces the current user's home directory with ~.
// Operational paths must continue to use their original absolute values.
func Shorten(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return path
	}
	homes := []string{filepath.Clean(home)}
	if physical, err := filepath.EvalSymlinks(home); err == nil && physical != homes[0] {
		homes = append(homes, physical)
	}
	for _, home := range homes {
		if path == home {
			return "~"
		}
		prefix := home
		if !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += string(filepath.Separator)
		}
		if strings.HasPrefix(path, prefix) {
			return "~" + string(filepath.Separator) + strings.TrimPrefix(path, prefix)
		}
	}
	return path
}
