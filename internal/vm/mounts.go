package vm

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

func contains(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && (relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))))
}

// ShareRoots produces a stable minimal set without widening directory access.
func ShareRoots(paths []string) ([]string, error) {
	roots := append([]string(nil), paths...)
	for _, root := range roots {
		if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, ":,\x00\r\n") || root == "/" || root == "/Users" || root == "/Volumes" || root == "/private" || root == "/home" || root == "/var" || root == "/tmp" {
			return nil, fmt.Errorf("unsupported VM share root %q; use a canonical repository directory without colons, commas or newlines", root)
		}
	}
	sort.Strings(roots)
	result := []string{}
	for _, root := range roots {
		covered := false
		for _, parent := range result {
			if contains(parent, root) {
				covered = true
				break
			}
		}
		if !covered {
			result = append(result, root)
		}
	}
	return result, nil
}
