package guest

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/vm/state"
)

func publicKey(seed byte) string {
	b := make([]byte, 51)
	binary.BigEndian.PutUint32(b[:4], 11)
	copy(b[4:15], "ssh-ed25519")
	binary.BigEndian.PutUint32(b[15:19], 32)
	b[19] = seed
	return "ssh-ed25519 " + base64.StdEncoding.EncodeToString(b)
}
func TestEnrollPublicKeyPreservesTrustAndRejectsReplacement(t *testing.T) {
	s := SSH{Data: t.TempDir(), VM: state.VM{ID: "unique"}}
	if err := s.EnrollPublicKey(context.Background(), publicKey(1), true); err != nil {
		t.Fatal(err)
	}
	if err := s.EnrollPublicKey(context.Background(), publicKey(1), true); err != nil {
		t.Fatal(err)
	}
	err := s.EnrollPublicKey(context.Background(), publicKey(2), true)
	if err == nil || !strings.Contains(err.Error(), "refusing replacement") {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(s.Data, "known_hosts"))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "namu-vm-unique "+publicKey(1)+"\n" {
		t.Fatal(string(b))
	}
}
func TestEnrollPublicKeyRejectsMalformedAndMultipleKeys(t *testing.T) {
	for _, key := range []string{"ssh-ed25519 invalid", publicKey(1) + "\n" + publicKey(2), ""} {
		t.Run(key, func(t *testing.T) {
			s := SSH{Data: t.TempDir(), VM: state.VM{ID: "unique"}}
			err := s.EnrollPublicKey(context.Background(), key, true)
			if err == nil {
				t.Fatal("must reject invalid host keys")
			}
			_, err = os.Stat(filepath.Join(s.Data, "known_hosts"))
			if !os.IsNotExist(err) {
				t.Fatal("must not enroll invalid key")
			}
		})
	}
}
func TestSSHOptionsDisableHostCredentialAndConfigInheritance(t *testing.T) {
	s := SSH{Data: "/private/state", Identity: "/private/state/key", VM: state.VM{ID: "unique"}, IP: "192.0.2.1", User: "admin"}
	args := strings.Join(s.options(true), " ")
	for _, required := range []string{"-F /dev/null", "IdentitiesOnly=yes", "StrictHostKeyChecking=yes", "HostKeyAlias=namu-vm-unique", "-tt", "admin@192.0.2.1"} {
		if !strings.Contains(args, required) {
			t.Fatal(args)
		}
	}
}
