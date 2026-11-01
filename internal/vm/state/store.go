// Package state owns namu's private VM registry and lifecycle serialization.
package state

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/kamaal111/namu/internal/vm/host"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func ValidateName(name string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid VM name %q: use 1–64 letters, digits, underscores or hyphens", name)
	}
	return nil
}

type VM struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	OS      string   `json:"os"`
	Image   string   `json:"image"`
	Stage   string   `json:"stage"`
	Shares  []string `json:"shares"`
	Applied []string `json:"applied"`
}
type Registry struct {
	Version int  `json:"version"`
	VMs     []VM `json:"vms"`
}
type Store struct{ Data string }

func (s Store) Lock(ctx context.Context) (func(), error) {
	if err := os.MkdirAll(s.Data, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(s.Data, 0700); err != nil {
		return nil, err
	}
	return host.Lock(ctx, filepath.Join(s.Data, "lifecycle.lock"))
}
func (s Store) Read() (Registry, error) {
	b, err := os.ReadFile(filepath.Join(s.Data, "registry.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Registry{Version: 1}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("read VM registry: %w", err)
	}
	if r.Version != 1 {
		return r, fmt.Errorf("unsupported VM registry version %d", r.Version)
	}
	names, ids := map[string]bool{}, map[string]bool{}
	for _, v := range r.VMs {
		if err := ValidateName(v.Name); err != nil {
			return r, err
		}
		if !validName.MatchString(v.ID) || v.Image == "" || (v.OS != "macos" && v.OS != "linux") || (v.Stage != "creating" && v.Stage != "cloned" && v.Stage != "ssh" && v.Stage != "ready") || names[v.Name] || ids[v.ID] {
			return r, fmt.Errorf("invalid VM registry entry for %s", v.Name)
		}
		names[v.Name], ids[v.ID] = true, true
	}
	return r, nil
}

// Write must be called under Lock; the old registry survives failed writes.
func (s Store) Write(r Registry) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.Data, ".registry-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(append(b, '\n'))
	syncErr := f.Sync()
	if err := errors.Join(writeErr, syncErr, f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(s.Data, "registry.json"))
}
func Find(r Registry, name string) (int, error) {
	for i, v := range r.VMs {
		if v.Name == name {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unknown namu VM %q; create it with: namu vm create %s", name, name)
}
