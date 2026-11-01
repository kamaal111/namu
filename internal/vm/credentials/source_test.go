package credentials

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/vm/host"
)

type fakeRunner struct{ calls []host.Command }

func (f *fakeRunner) Run(_ context.Context, c host.Command) ([]byte, error) {
	f.calls = append(f.calls, c)
	return nil, os.ErrNotExist
}
func (f *fakeRunner) Detach(context.Context, host.Command, string) error { return nil }

func TestSourceReadsOnlyAgentAuthenticationFields(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude/.credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"synthetic-token","refreshToken":"synthetic-refresh"},"mcpOAuth":{"unrelated":"must-not-copy"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	source := Source{Home: home, Runner: &fakeRunner{}, Getenv: func(string) string { return "" }, System: "linux"}
	bundle, err := source.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(bundle.Claude) != `{"claudeAiOauth":{"accessToken":"synthetic-token","refreshToken":"synthetic-refresh"}}` {
		t.Fatal("must copy only agent login data")
	}
}
func TestSourceRejectsPublicCredentialFileWithoutPrintingContents(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".codex/auth.json"), []byte(`{"OPENAI_API_KEY":"synthetic-secret"}`), 0644); err != nil {
		t.Fatal(err)
	}
	source := Source{Home: home, Runner: &fakeRunner{}, Getenv: func(string) string { return "" }, System: "linux"}
	_, err := source.Collect(context.Background())
	if err == nil {
		t.Fatal("must reject publicly readable credentials")
	}
}

func TestCodexEphemeralStoreDoesNotReadStaleFileCache(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".codex")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("cli_auth_credentials_store = 'ephemeral'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "auth.json"), []byte("stale invalid cache"), 0644); err != nil {
		t.Fatal(err)
	}
	source := Source{Home: home, Runner: &fakeRunner{}, Getenv: func(string) string { return "" }, System: "linux"}
	bundle, err := source.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Codex) != 0 {
		t.Fatal("ephemeral host login must not reuse stale file credentials")
	}
}

type keychainRunner struct{ calls []host.Command }

func (f *keychainRunner) Run(_ context.Context, c host.Command) ([]byte, error) {
	f.calls = append(f.calls, c)
	if c.Name != "security" {
		return nil, os.ErrNotExist
	}
	switch c.Args[2] {
	case "Codex Auth":
		return []byte(`{"auth_mode":"chatgpt","tokens":{"access_token":"synthetic-current"}}`), nil
	case "Claude Code-credentials":
		return []byte(`{"claudeAiOauth":{"accessToken":"synthetic-current"},"mcpOAuth":{"unrelated":"must-not-copy"}}`), nil
	case "cursor-access-token":
		return []byte("synthetic-current"), nil
	}
	return nil, os.ErrNotExist
}
func (*keychainRunner) Detach(context.Context, host.Command, string) error { return nil }
func TestNativeMacKeychainCredentialsTakePrecedenceOverStaleFiles(t *testing.T) {
	home := t.TempDir()
	for _, dir := range []string{".codex", ".claude", ".cursor"} {
		if err := os.Mkdir(filepath.Join(home, dir), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".codex/config.toml"), []byte("cli_auth_credentials_store = 'keyring'\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{".codex/auth.json", ".claude/.credentials.json", ".cursor/auth.json"} {
		if err := os.WriteFile(filepath.Join(home, file), []byte("stale public cache must not be read"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runner := &keychainRunner{}
	source := Source{Home: home, Runner: runner, Getenv: func(string) string { return "" }, System: "darwin"}
	bundle, err := source.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Codex) == 0 || len(bundle.Claude) == 0 || bundle.Environment["CURSOR_AUTH_TOKEN"] != "synthetic-current" {
		t.Fatal("must reuse current native Keychain credentials")
	}
	if strings.Contains(string(bundle.Claude), "must-not-copy") {
		t.Fatal("must exclude unrelated MCP authentication")
	}
}
