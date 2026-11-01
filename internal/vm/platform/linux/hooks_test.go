package linux

import (
	"strings"
	"testing"
)

func TestLinuxBootstrapValidatesSSHBeforeRestart(t *testing.T) {
	script := Definition().Hooks.Bootstrap("ssh-ed25519 test-key")
	prepare := strings.Index(script, "mkdir -p /run/sshd")
	validate := strings.Index(script, "/usr/sbin/sshd -t")
	restart := strings.Index(script, "systemctl restart ssh")
	publicKey := strings.Index(script, "cat /etc/ssh/ssh_host_ed25519_key.pub")
	if prepare < 0 || validate <= prepare || restart <= validate || publicKey <= restart {
		t.Fatal("SSH runtime preparation, validation, restart, and public key output must remain ordered")
	}
}
