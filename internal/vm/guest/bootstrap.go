package guest

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/tart"
)

//go:embed ssh.sh
var sshScript string

//go:embed access.sh
var accessScript string

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
func BootstrapScript(osName, publicKey string) string {
	return AccessScript(osName, publicKey) + sshScript
}

// AccessScript restores the approved public key after image boot services reset authorized_keys.
// Host-key trust is untouched; only initial bootstrap creates the per-clone host key.
func AccessScript(osName, publicKey string) string {
	return "set -eu\nexport PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin\nuser='admin'\nexpected_os=" + host.Quote(osName) + "\npublic_key=" + host.Quote(publicKey) + "\n" + accessScript
}

// Essentials runs as the guest user after SSH enrollment, never on the host.
func (s SSH) Essentials(ctx context.Context, out io.Writer) error {
	script := `set -eu
if [ "$(uname -s)" = Linux ]; then
  sudo -n apt-get update
  sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y git ca-certificates
else
  command -v /opt/homebrew/bin/brew >/dev/null || { echo 'Default macOS image requires Homebrew' >&2; exit 1; }
  HOMEBREW_NO_AUTO_UPDATE=1 /opt/homebrew/bin/brew install git
fi
git --version
`
	_, err := s.Run(ctx, script, nil, out, out, false)
	return err
}

// macOS reports a generic virtio-fs source, not the requested mount tag.
const macMountCheck = `mount | grep -F -- " on $root (" | grep -Ei '(virtiofs|virtio-fs)' >/dev/null`

func MountScript(osName string, roots []string) string {
	var b strings.Builder
	b.WriteString("set -eu\nexport PATH=/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin\n")
	for _, root := range roots {
		tag := tart.Tag(root)
		fmt.Fprintf(&b, "root=%s\ntag=%s\n", host.Quote(root), host.Quote(tag))
		// Only reuse mountpoints recorded by namu, never hide unrelated guest files.
		b.WriteString(`part="$root"
while [ "$part" != / ]; do
  test ! -L "$part" || { echo "Guest mount path contains symlink: $part" >&2; exit 1; }
  part=$(dirname "$part")
done
marker="/var/db/namu-vm/$tag"
mkdir -p /var/db/namu-vm
if [ ! -f "$marker" ]; then
  if [ -e "$root" ]; then
    test -d "$root" && test -z "$(ls -A "$root")" || { echo "Guest mount destination is not empty: $root" >&2; exit 1; }
  fi
  mkdir -p "$root"
  printf '%s\n' "$root" > "$marker"
fi
test "$(cat "$marker")" = "$root"
`)
		// Reuse only a recorded mountpoint with the expected filesystem. Linux also exposes its tag.
		if osName == "linux" {
			b.WriteString(`if ! findmnt -rn -t virtiofs -S "$tag" -M "$root" >/dev/null; then
  test -z "$(ls -A "$root")" || { echo "Guest mount destination is not empty: $root" >&2; exit 1; }
  mount -t virtiofs "$tag" "$root"
fi
`)
		} else {
			b.WriteString("if ! " + macMountCheck + "; then\n" + `
  test -z "$(ls -A "$root")" || { echo "Guest mount destination is not empty: $root" >&2; exit 1; }
  mount_virtiofs "$tag" "$root"
fi
`)
		}
	}
	return b.String()
}
