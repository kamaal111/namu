package guest

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/vm/credentials"
)

//go:embed testdata/read-api-key.sh
var readAPIKeyFixture string

func importProfile(t *testing.T, home string, bundle credentials.Bundle) string {
	t.Helper()
	payload, err := json.Marshal(profilePayload{Bundle: bundle, GitHelper: gitCredentialHelper})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "-c", profileScript)
	cmd.Env = append(os.Environ(), "HOME="+home)
	cmd.Stdin = bytes.NewReader(payload)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var path string
	if err := json.Unmarshal(output, &path); err != nil {
		t.Fatal("must return only profile path")
	}
	return path
}
func TestImportedCredentialsArePrivateAndDoNotReplaceGuestLogin(t *testing.T) {
	home := t.TempDir()
	guest := filepath.Join(home, ".codex")
	if err := os.Mkdir(guest, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(guest, "auth.json"), []byte("original guest login"), 0600); err != nil {
		t.Fatal(err)
	}
	profile := importProfile(t, home, credentials.Bundle{Codex: json.RawMessage(`{"OPENAI_API_KEY":"synthetic-key"}`), Environment: map[string]string{"CURSOR_AUTH_TOKEN": "synthetic-token"}})
	original, err := os.ReadFile(filepath.Join(guest, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != "original guest login" {
		t.Fatal("must preserve guest-owned login")
	}
	for _, path := range []string{profile, filepath.Join(profile, "cursor.env"), filepath.Join(home, ".namu-vm/host-auth/codex/auth.json")} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0077 != 0 {
			t.Fatal("credential state must be private")
		}
	}
}
func TestImportPreservesRefreshedNativeCredentialsAcrossSessions(t *testing.T) {
	home := t.TempDir()
	first := importProfile(t, home, credentials.Bundle{Codex: json.RawMessage(`{"OPENAI_API_KEY":"first"}`)})
	cache := filepath.Join(home, ".namu-vm/host-auth/codex/auth.json")
	if err := os.WriteFile(cache, []byte(`{"tokens":"guest-refreshed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	second := importProfile(t, home, credentials.Bundle{Codex: json.RawMessage(`{"OPENAI_API_KEY":"stale-host"}`)})
	data, err := os.ReadFile(cache)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("transient session profiles must be independent")
	}
	if string(data) != `{"tokens":"guest-refreshed"}` {
		t.Fatal("must preserve refreshed guest auth")
	}
}
func TestGitCredentialHelperIsScopedToGitHubHTTPS(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git.env"), []byte("export GH_TOKEN='synthetic-github-token'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", gitCredentialHelper, "helper", "get")
	cmd.Env = append(os.Environ(), "NAMU_HOST_CREDENTIALS_DIR="+dir)
	cmd.Stdin = strings.NewReader("protocol=https\nhost=example.com\n\n")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 0 {
		t.Fatal("must not expose GitHub tokens to other hosts")
	}
}
func TestGitCredentialHelperSuppliesGitHubTokenOnStdinRequest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git.env"), []byte("export GH_TOKEN='synthetic-github-token'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", gitCredentialHelper, "helper", "get")
	cmd.Env = append(os.Environ(), "NAMU_HOST_CREDENTIALS_DIR="+dir)
	cmd.Stdin = strings.NewReader("protocol=https\nhost=github.com\n\n")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "username=x-access-token\npassword=synthetic-github-token\n\n" {
		t.Fatal("must supply scoped credential response")
	}
}

func TestImportedGitIdentityPreservesUnicodeAndCanBeParsedByGit(t *testing.T) {
	home := t.TempDir()
	profile := importProfile(t, home, credentials.Bundle{GitName: "Ada 雪", GitEmail: "ada@example.com"})
	cmd := exec.Command("git", "config", "--file", filepath.Join(profile, "gitconfig"), "--get", "user.name")
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != "Ada 雪\n" {
		t.Fatal("must preserve Unicode author identity")
	}
}

func TestProfileCredentialValuesRemainShellData(t *testing.T) {
	home := t.TempDir()
	marker := filepath.Join(home, "must-not-execute")
	value := "token'; touch " + marker + "; echo '"
	profile := importProfile(t, home, credentials.Bundle{Environment: map[string]string{"OPENAI_API_KEY": value}})
	cmd := exec.Command("sh", "-c", readAPIKeyFixture, "test", profile)
	output, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(output) != value {
		t.Fatal("credential value must be preserved as data")
	}
	_, err = os.Stat(marker)
	if !os.IsNotExist(err) {
		t.Fatal("credential contents must never execute")
	}
}

func TestCredentialProfilesUseRegisteredGuestHomes(t *testing.T) {
	for _, tc := range []struct {
		osName, profile string
		valid           bool
	}{
		{"macos", "/Users/admin/.namu-vm/credentials/session.synthetic", true},
		{"linux", "/home/admin/.namu-vm/credentials/session.synthetic", true},
		{"macos", "/home/admin/.namu-vm/credentials/session.synthetic", false},
		{"windows", "/home/admin/.namu-vm/credentials/session.synthetic", false},
		{"", "/home/admin/.namu-vm/credentials/session.synthetic", false},
		{"linux", "/home/admin/.namu-vm/credentials/session.synthetic/../escape", false},
	} {
		t.Run(tc.osName+tc.profile, func(t *testing.T) {
			if got := validProfile(tc.profile, tc.osName); got != tc.valid {
				t.Fatal(got)
			}
		})
	}
}
