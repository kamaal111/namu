// Package credentials reads only supported host login sources for an explicit VM session.
// Secret bytes are transferred through stdin, never arguments, diagnostics or the VM registry.
package credentials

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kamaal111/namu/internal/vm/host"
	"github.com/pelletier/go-toml/v2"
	"golang.org/x/sys/unix"
)

const maxCredentialSize = 1 << 20

type Bundle struct {
	Codex       json.RawMessage   `json:"codex,omitempty"`
	Claude      json.RawMessage   `json:"claude,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	GitName     string            `json:"git_name,omitempty"`
	GitEmail    string            `json:"git_email,omitempty"`
}
type Source struct {
	Home, Directory, System string
	Runner                  host.Runner
	Getenv                  func(string) string
}

func Default(r host.Runner, directory string) (Source, error) {
	home, err := os.UserHomeDir()
	return Source{Home: home, Directory: directory, System: runtime.GOOS, Runner: r, Getenv: os.Getenv}, err
}
func (s Source) environment(name string) string {
	if s.Getenv == nil {
		return ""
	}
	return s.Getenv(name)
}
func (s Source) directory(variable, fallback string) string {
	if value := s.environment(variable); value != "" {
		return value
	}
	return filepath.Join(s.Home, fallback)
}
func readBounded(path, label string, private bool) ([]byte, error) {
	flags := unix.O_RDONLY | unix.O_NONBLOCK | unix.O_CLOEXEC
	if private {
		flags |= unix.O_NOFOLLOW
	}
	fd, err := unix.Open(path, flags, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s credentials; use --no-reuse-credentials to skip host access", label)
	}
	f := os.NewFile(uintptr(fd), label)
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot inspect %s credentials", label)
	}
	if !info.Mode().IsRegular() || (private && info.Mode().Perm()&0077 != 0) || info.Size() > maxCredentialSize {
		return nil, fmt.Errorf("%s credentials must be a private regular file no larger than 1 MiB", label)
	}
	b, err := io.ReadAll(io.LimitReader(f, maxCredentialSize+1))
	if err != nil || len(b) > maxCredentialSize {
		return nil, fmt.Errorf("cannot read bounded %s credentials", label)
	}
	return b, nil
}
func readPrivateJSON(path, label string) ([]byte, error) {
	b, err := readBounded(path, label, true)
	if err != nil || len(b) == 0 {
		return b, err
	}
	if !json.Valid(b) {
		return nil, fmt.Errorf("invalid %s credential JSON; contents withheld", label)
	}
	return b, nil
}
func (s Source) keychain(ctx context.Context, service, account string) []byte {
	if s.System != "darwin" {
		return nil
	}
	args := []string{"find-generic-password", "-s", service, "-w"}
	if account != "" {
		args = append(args, "-a", account)
	}
	b, err := s.Runner.Run(ctx, host.Command{Name: "security", Args: args})
	if err != nil || len(b) > maxCredentialSize {
		return nil
	}
	return []byte(strings.TrimSpace(string(b)))
}
func filterObject(b []byte, label string, fields ...string) (json.RawMessage, error) {
	if len(b) == 0 {
		return nil, nil
	}
	var object map[string]json.RawMessage
	if json.Unmarshal(b, &object) != nil || object == nil {
		return nil, fmt.Errorf("invalid %s credential object; contents withheld", label)
	}
	selected := map[string]json.RawMessage{}
	for _, field := range fields {
		if value, ok := object[field]; ok && string(value) != "null" {
			selected[field] = value
		}
	}
	if len(selected) == 0 {
		return nil, nil
	}
	result, err := json.Marshal(selected)
	return result, err
}
func (s Source) codex(ctx context.Context) (json.RawMessage, error) {
	dir := s.directory("CODEX_HOME", ".codex")
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("CODEX_HOME must be absolute for credential reuse")
	}
	var config struct {
		Store string `toml:"cli_auth_credentials_store"`
	}
	settings, err := readBounded(filepath.Join(dir, "config.toml"), "Codex auth configuration", false)
	if err != nil {
		return nil, err
	}
	if len(settings) > 0 && toml.Unmarshal(settings, &config) != nil {
		return nil, fmt.Errorf("invalid Codex auth configuration; contents withheld")
	}
	if config.Store == "ephemeral" {
		return nil, nil
	}
	keyring := func() []byte {
		canonical, err := filepath.EvalSymlinks(dir)
		if err == nil {
			dir = canonical
		}
		sum := sha256.Sum256([]byte(dir))
		return s.keychain(ctx, "Codex Auth", fmt.Sprintf("cli|%x", sum)[:20])
	}
	var raw []byte
	switch config.Store {
	case "keyring":
		raw = keyring()
	case "auto":
		raw = keyring()
		if len(raw) == 0 {
			raw, err = readPrivateJSON(filepath.Join(dir, "auth.json"), "Codex")
		}
	case "file":
		raw, err = readPrivateJSON(filepath.Join(dir, "auth.json"), "Codex")
	case "":
		raw, err = readPrivateJSON(filepath.Join(dir, "auth.json"), "Codex")
		if err == nil && len(raw) == 0 {
			raw = keyring()
		}
	default:
		return nil, fmt.Errorf("unsupported Codex credential store; use guest login or --no-reuse-credentials")
	}
	if err != nil {
		return nil, err
	}
	return filterObject(raw, "Codex", "auth_mode", "OPENAI_API_KEY", "tokens", "last_refresh")
}

func (s Source) claude(ctx context.Context) (json.RawMessage, error) {
	dir := s.directory("CLAUDE_CONFIG_DIR", ".claude")
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("CLAUDE_CONFIG_DIR must be absolute for credential reuse")
	}
	var raw []byte
	if s.environment("CLAUDE_CONFIG_DIR") == "" {
		raw = s.keychain(ctx, "Claude Code-credentials", "")
	}
	if len(raw) == 0 {
		var err error
		raw, err = readPrivateJSON(filepath.Join(dir, ".credentials.json"), "Claude")
		if err != nil {
			return nil, err
		}
	}
	return filterObject(raw, "Claude", "claudeAiOauth")
}

func (s Source) cursor(ctx context.Context, env map[string]string) error {
	if env["CURSOR_API_KEY"] != "" || env["CURSOR_AUTH_TOKEN"] != "" {
		return nil
	}
	store := s.environment("AGENT_CLI_CREDENTIAL_STORE")
	if store == "memory" {
		return nil
	}
	if store != "file" && s.System == "darwin" {
		if token := s.keychain(ctx, "cursor-access-token", "cursor-user"); len(token) > 0 {
			env["CURSOR_AUTH_TOKEN"] = string(token)
			return nil
		}
		if key := s.keychain(ctx, "cursor-api-key", "cursor-user"); len(key) > 0 {
			env["CURSOR_API_KEY"] = string(key)
			return nil
		}
	}
	paths := []string{filepath.Join(s.Home, ".cursor", "auth.json")}
	if s.System != "darwin" {
		paths = append([]string{filepath.Join(s.directory("XDG_CONFIG_HOME", ".config"), "cursor", "auth.json")}, paths...)
	}
	for _, path := range paths {
		b, err := readPrivateJSON(path, "Cursor")
		if err != nil {
			return err
		}
		if len(b) == 0 {
			continue
		}
		var record struct {
			AccessToken string `json:"accessToken"`
			APIKey      string `json:"apiKey"`
		}
		if json.Unmarshal(b, &record) != nil {
			return fmt.Errorf("invalid Cursor credential object; contents withheld")
		}
		if record.AccessToken != "" {
			env["CURSOR_AUTH_TOKEN"] = record.AccessToken
			return nil
		}
		if record.APIKey != "" {
			env["CURSOR_API_KEY"] = record.APIKey
			return nil
		}
	}

	return nil
}
func (s Source) gitValue(ctx context.Context, key string) string {
	dir := s.Directory
	if dir == "" {
		dir = s.Home
	}
	b, err := s.Runner.Run(ctx, host.Command{Name: "git", Args: []string{"-C", dir, "config", "--get", key}})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func (s Source) Collect(ctx context.Context) (Bundle, error) {
	if err := ctx.Err(); err != nil {
		return Bundle{}, err
	}
	env := map[string]string{}
	for _, name := range []string{"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "CLAUDE_CODE_OAUTH_TOKEN", "CURSOR_API_KEY", "CURSOR_AUTH_TOKEN"} {
		if value := s.environment(name); value != "" {
			if len(value) > maxCredentialSize {
				return Bundle{}, fmt.Errorf("host credential exceeds size limit")
			}
			env[name] = value
		}
	}
	codex, err := s.codex(ctx)
	if err != nil {
		return Bundle{}, err
	}
	claude, err := s.claude(ctx)
	if err != nil {
		return Bundle{}, err
	}
	if err := s.cursor(ctx, env); err != nil {
		return Bundle{}, err
	}
	token := s.environment("GH_TOKEN")
	if token == "" {
		token = s.environment("GITHUB_TOKEN")
	}
	if token == "" {
		b, err := s.Runner.Run(ctx, host.Command{Name: "gh", Args: []string{"auth", "token", "--hostname", "github.com"}})
		if err == nil && len(b) <= maxCredentialSize {
			token = strings.TrimSpace(string(b))
		}
	}
	if token != "" {
		env["GH_TOKEN"] = token
	}
	if err := ctx.Err(); err != nil {
		return Bundle{}, err
	}
	return Bundle{Codex: codex, Claude: claude, Environment: env, GitName: s.gitValue(ctx, "user.name"), GitEmail: s.gitValue(ctx, "user.email")}, nil
}
