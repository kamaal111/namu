package tart

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/vm/host"
)

type runner struct {
	run    func(host.Command) ([]byte, error)
	detach func(host.Command, string) error
}

func (r runner) Run(_ context.Context, c host.Command) ([]byte, error) { return r.run(c) }
func (r runner) Detach(_ context.Context, c host.Command, path string) error {
	return r.detach(c, path)
}
func TestInstallRejectsChecksumMismatchBeforeExtraction(t *testing.T) {
	extracted := false
	p := Provider{Data: t.TempDir(), Runner: runner{run: func(c host.Command) ([]byte, error) {
		switch c.Name {
		case "curl":
			return nil, os.WriteFile(c.Args[len(c.Args)-1], []byte("untrusted archive"), 0600)
		case "tar":
			extracted = true
			return nil, errors.New("must not extract")
		}
		return nil, errors.New("unexpected command")
	}}}
	err := p.Install(context.Background(), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatal(err)
	}
	if extracted {
		t.Fatal("must not extract unverified archive")
	}
	entries, err := os.ReadDir(filepath.Join(p.Data, "tools"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("failed download must be cleaned up")
	}
}
func TestStartPassesDistinctTagsWithoutShellExpansion(t *testing.T) {
	var actual host.Command
	p := Provider{Data: t.TempDir(), Runner: runner{detach: func(c host.Command, _ string) error { actual = c; return nil }}}
	root := "/Users/ada/雪 project"
	err := p.Start(context.Background(), "sandbox", []string{root})
	if err != nil {
		t.Fatal(err)
	}
	if actual.Args[len(actual.Args)-1] != "sandbox" {
		t.Fatal(actual)
	}
	if actual.Args[5] != root+":tag="+Tag(root) {
		t.Fatal(actual.Args)
	}
	if Tag(root) == Tag(root+"2") {
		t.Fatal("distinct shares need distinct tags")
	}
}
func TestHostCheckOffersLinuxOnOlderSupportedMac(t *testing.T) {
	p := Provider{Host: func() (string, string) { return "darwin", "arm64" }, Runner: runner{run: func(host.Command) ([]byte, error) { return []byte("26.3"), nil }}}
	err := p.HostCheck(context.Background(), "macos")
	if err == nil || !strings.Contains(err.Error(), "--os linux") {
		t.Fatal(err)
	}
	if err := p.HostCheck(context.Background(), "linux"); err != nil {
		t.Fatal(err)
	}
}
func TestListRejectsMalformedProviderOutput(t *testing.T) {
	p := Provider{Runner: runner{run: func(host.Command) ([]byte, error) { return []byte(`[{"Name":""}]`), nil }}}
	_, err := p.List(context.Background())
	if err == nil {
		t.Fatal("must reject unnamed VM")
	}
}
