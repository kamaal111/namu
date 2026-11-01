package guest

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/kamaal111/namu/internal/vm/state"
)

type SSH struct {
	Runner                   host.Runner
	Data, Identity, User, IP string
	VM                       state.VM
}

func (s SSH) alias() string { return "namu-vm-" + s.VM.ID }
func (s SSH) options(tty bool) []string {
	a := []string{"-F", "/dev/null", "-o", "BatchMode=yes", "-o", "IdentitiesOnly=yes", "-o", "IdentityAgent=none", "-o", "ForwardAgent=no", "-o", "ClearAllForwardings=yes", "-o", "ConnectTimeout=5", "-o", "ServerAliveInterval=15", "-o", "ServerAliveCountMax=3", "-o", "StrictHostKeyChecking=yes", "-o", "UserKnownHostsFile=" + filepath.Join(s.Data, "known_hosts"), "-o", "GlobalKnownHostsFile=/dev/null", "-o", "HostKeyAlias=" + s.alias(), "-i", s.Identity}
	if tty {
		a = append(a, "-tt")
	}
	return append(a, s.User+"@"+s.IP)
}
func (s SSH) Run(ctx context.Context, script string, in io.Reader, out, stderr io.Writer, tty bool) ([]byte, error) {
	if s.Identity == "" {
		return nil, fmt.Errorf("identity_file is required: configure a dedicated SSH key and enroll the guest host key")
	}
	prefix := `export PATH="$HOME/.local/bin:$HOME/.npm-global/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"; `
	a := append(s.options(tty), host.Join([]string{"sh", "-c", prefix + script}))
	return s.Runner.Run(ctx, host.Command{Name: "ssh", Args: a, In: in, Out: out, Err: stderr})
}
func (s SSH) Ready(ctx context.Context) error {
	if s.Identity == "" {
		return fmt.Errorf("configure identity_file before connecting to the guest")
	}
	var last error
	var diagnostics strings.Builder
	for {
		diagnostics.Reset()
		if _, last = s.Run(ctx, "true", nil, nil, &diagnostics, false); last == nil {
			return nil
		}
		detail := strings.TrimSpace(diagnostics.String())
		if strings.Contains(detail, "Permission denied") || strings.Contains(detail, "Host key verification failed") || strings.Contains(detail, "REMOTE HOST IDENTIFICATION HAS CHANGED") {
			return fmt.Errorf("SSH authentication or trust failed for VM %s: %s; existing trust retained", s.VM.Name, detail)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("SSH readiness: %w; last attempt: %v; %s; inspect namu vm shell %s", ctx.Err(), last, strings.TrimSpace(diagnostics.String()), s.VM.Name)
		case <-time.After(time.Second):
		}
	}
}

func CheckSSH(ctx context.Context, r host.Runner) error {
	var stderr strings.Builder
	b, err := r.Run(ctx, host.Command{Name: "ssh", Args: []string{"-V"}, Err: &stderr})
	if err != nil {
		return err
	}
	if err = host.CheckVersion(string(b)+stderr.String(), "10.3", 10); err != nil {
		return fmt.Errorf("system OpenSSH: %w", err)
	}
	return nil
}

// EnrollPublicKey accepts a key obtained through the local Tart channel.
// An identical key is safe on bootstrap retry.
func (s SSH) EnrollPublicKey(ctx context.Context, publicKey string, retry bool) error {
	var key string
	for _, line := range strings.Split(strings.TrimSpace(publicKey), "\n") {
		f := strings.Fields(line)
		for i, field := range f {
			if field == "ssh-ed25519" && i+1 < len(f) {
				decoded, err := base64.StdEncoding.DecodeString(f[i+1])
				if err != nil || len(decoded) != 51 || binary.BigEndian.Uint32(decoded[:4]) != 11 || string(decoded[4:15]) != "ssh-ed25519" || binary.BigEndian.Uint32(decoded[15:19]) != 32 {
					return fmt.Errorf("invalid ed25519 public host key")
				}
				if key != "" {
					return fmt.Errorf("expected exactly one ed25519 public host key")
				}
				key = "ssh-ed25519 " + f[i+1]
			}
		}
	}
	if key == "" {
		return fmt.Errorf("no ed25519 host key returned")
	}
	unlock, err := host.Lock(ctx, filepath.Join(s.Data, "locks", "known-hosts.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(s.Data, "known_hosts")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	entry := s.alias() + " " + key
	for _, line := range strings.Split(string(existing), "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == s.alias() {
			if retry && line == entry {
				return nil
			}
			return fmt.Errorf("VM host key already enrolled; refusing replacement; verify deliberate rotation independently")
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.WriteString(entry + "\n")
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
