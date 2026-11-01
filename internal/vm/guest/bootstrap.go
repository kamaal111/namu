package guest

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/platform"
	"github.com/kamaal111/namu/internal/vm/platform/registry"
	"github.com/kamaal111/namu/internal/vm/tart"
)

func Identity(ctx context.Context, data string, r host.Runner) (string, error) {
	path := filepath.Join(data, "ssh", "id_ed25519")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return "", err
		}
		tmp, err := os.MkdirTemp(filepath.Dir(path), ".key-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)
		key := filepath.Join(tmp, "id_ed25519")
		if _, err = r.Run(ctx, host.Command{Name: "ssh-keygen", Args: []string{"-q", "-t", "ed25519", "-N", "", "-f", key}}); err != nil {
			return "", err
		}
		if err = os.Link(key, path); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	} else if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return "", fmt.Errorf("VM identity must be a private regular file with permissions 0600 or stricter")
	}
	b, err := r.Run(ctx, host.Command{Name: "ssh-keygen", Args: []string{"-y", "-P", "", "-f", path}})
	return strings.TrimSpace(string(b)), err
}
func BootstrapScript(definition platform.Definition, publicKey string) string {
	return definition.Hooks.Bootstrap(publicKey)
}

// AccessScript restores the approved public key without changing host-key trust.
func AccessScript(definition platform.Definition, publicKey string) string {
	return definition.Hooks.Access(publicKey)
}

// Essentials runs as the guest user after SSH enrollment, never on the host.
func (s SSH) Essentials(ctx context.Context, out io.Writer) error {
	definition, err := registry.Resolve(s.VM.OS)
	if err != nil {
		return err
	}
	_, err = s.Run(ctx, definition.Hooks.Essentials, nil, out, out, false)
	return err
}

func MountScript(definition platform.Definition, roots []string) string {
	var b strings.Builder
	b.WriteString(platform.RootShell())
	b.WriteString(definition.Hooks.MountPrelude)
	for _, root := range roots {
		b.WriteString(definition.Hooks.Mount(root, tart.Tag(root)))
	}
	return b.String()
}
