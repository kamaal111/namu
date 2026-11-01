package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripAndPrivatePermissions(t *testing.T) {
	s := Store{Data: filepath.Join(t.TempDir(), "state")}
	unlock, err := s.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	expected := VM{ID: "unique", Name: "sandbox", OS: "linux", Image: "pinned", Stage: "ready"}
	if err := s.Write(Registry{Version: 1, VMs: []VM{expected}}); err != nil {
		t.Fatal(err)
	}
	r, err := s.Read()
	if err != nil {
		t.Fatal(err)
	}
	if len(r.VMs) != 1 || r.VMs[0].ID != expected.ID {
		t.Fatal(r)
	}
	for _, path := range []string{s.Data, filepath.Join(s.Data, "registry.json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatal("VM state must be private")
		}
	}
}
func TestStoreRejectsUnknownVersionAndInvalidOwnership(t *testing.T) {
	for _, content := range []string{`{"version":2}`, `{"version":1,"vms":[{"id":"../escape","name":"sandbox","os":"linux","image":"pinned","stage":"ready"}]}`, `{"version":1,"vms":[{"id":"unique","name":"sandbox","os":"windows","image":"pinned","stage":"ready"}]}`, `invalid`} {
		t.Run(content, func(t *testing.T) {
			s := Store{Data: t.TempDir()}
			if err := os.WriteFile(filepath.Join(s.Data, "registry.json"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := s.Read()
			if err == nil {
				t.Fatal("must reject corrupt registry")
			}
		})
	}
}
