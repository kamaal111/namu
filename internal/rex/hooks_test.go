package rex

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kamaal111/namu/internal/integration"
	"github.com/kamaal111/namu/internal/testutil"
)

func TestRexCreatesThenReusesRenamedSessionAndFocusesPaneOwner(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	request := integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "feature/cards", Focus: true}
	must(t, Hooks().Open(context.Background(), request))
	must(t, Hooks().Open(context.Background(), request))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 1, string(data))
	check(t, strings.Contains(string(data), "new\nfeature/cards\n--cwd\n"+repo.Main+"\n--json\n"), string(data))
	check(t, strings.Count(string(data), "-C\nclient:owner\ndo\nsession.select\n--args\n{\"session_id\":\"session:created\"}") == 2, string(data))
	entries, err := filepath.Glob(filepath.Join(repo.Common, "namu", "rex", "*.json"))
	must(t, err)
	check(t, len(entries) == 1, "must persist one mapping")
}

func TestRexBackgroundDoesNotDiscoverOrSelectApp(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "feature/cards"}))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Contains(string(data), "BEGIN\nnew\n"), string(data))
	check(t, !strings.Contains(string(data), "session.select") && !strings.Contains(string(data), "BEGIN\nblock\n") && !strings.Contains(string(data), "BEGIN\nclient\n"), string(data))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, condition bool, message string) {
	t.Helper()
	if !condition {
		t.Fatal(message)
	}
}

func TestRexExplicitClientOverridesPaneOwner(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	t.Setenv("REX_CLIENT", "client:chosen")
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main", Focus: true}))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Contains(string(data), "BEGIN\ndo\nsession.select"), string(data))
	check(t, !strings.Contains(string(data), "BEGIN\nblock\n") && !strings.Contains(string(data), "BEGIN\nclient\n"), string(data))
}

func TestRexUnavailableOwnerUsesDefaultClient(t *testing.T) {
	for _, response := range []string{
		`{"block":{}}`,
		`{"block":{"owner":"client:missing"}}`,
		`{"block":{"owner":"client:cli"}}`,
		`{"block":{"owner":"client:offline"}}`,
	} {
		t.Run(response, func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Rex(t, `case "$1" in
block) printf '%s\n' '`+response+`'; exit 0 ;;
client) printf '%s\n' '{"clients":[{"client_id":"client:cli","connection_state":"connected","info":{"kind":"cli"}},{"client_id":"client:offline","connection_state":"disconnected","info":{"kind":"app"}}]}'; exit 0 ;;
esac
`+testutil.RexResponses)
			must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main", Focus: true}))
			data, err := os.ReadFile(log)
			must(t, err)
			check(t, strings.Contains(string(data), "BEGIN\ndo\nsession.select"), string(data))
		})
	}
}

func TestRexClosedSessionIsReplaced(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	request := integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"}
	must(t, Hooks().Open(context.Background(), request))
	must(t, os.Remove(log+".created"))
	must(t, Hooks().Open(context.Background(), request))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 2, string(data))
}

func TestRexFocusFailureRetainsMappingForRetry(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, `case "$1" in -C|do) echo ambiguous >&2; exit 3 ;; esac
`+testutil.RexResponses)
	request := integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main", Focus: true}
	err := Hooks().Open(context.Background(), request)
	check(t, err != nil && strings.Contains(err.Error(), "ambiguous"), "focus must report diagnostics")
	request.Focus = false
	must(t, Hooks().Open(context.Background(), request))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 1, string(data))
}

func TestRexSeparatePathsAndHostsHaveSeparateMappings(t *testing.T) {
	repo := testutil.Repository(t)
	other := filepath.Join(repo.Main, "checkout with spaces")
	must(t, os.Mkdir(other, 0700))
	log := testutil.Rex(t, `case "$1" in whoami) printf '{"host_id":"%s"}\n' "$TEST_REX_HOST"; exit 0 ;; esac
`+testutil.RexResponses)
	t.Setenv("TEST_REX_HOST", "host:first")
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"}))
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: other, Branch: "main"}))
	t.Setenv("TEST_REX_HOST", "host:second")
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: other, Branch: "main"}))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 3, string(data))
	check(t, strings.Contains(string(data), "--cwd\n"+other+"\n"), string(data))
}

func TestRexEndpointAliasesReuseHostMapping(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	request := integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"}
	t.Setenv("REX_SERVER", "unix:///server.sock")
	must(t, Hooks().Open(context.Background(), request))
	t.Setenv("REX_SERVER", "my-host")
	must(t, Hooks().Open(context.Background(), request))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 1, string(data))
}

func TestRexRejectsMalformedResponsesWithoutSpeculativeCreation(t *testing.T) {
	for _, tc := range []struct{ name, body, message string }{
		{"identity-json", `echo invalid; exit 0`, "invalid rex"},
		{"identity-field", `echo '{}'; exit 0`, "host_id"},
		{"list-json", `case "$1" in ls) echo invalid; exit 0 ;; esac`, "invalid rex"},
		{"list-field", `case "$1" in ls) echo '{}'; exit 0 ;; esac`, "missing sessions"},
		{"list-id", `case "$1" in ls) echo '{"sessions":[{}]}'; exit 0 ;; esac`, "empty session_id"},
		{"list-error", `case "$1" in ls) echo unreachable >&2; exit 4 ;; esac`, "unreachable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Rex(t, tc.body+"\n"+testutil.RexResponses)
			err := Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"})
			check(t, err != nil && strings.Contains(err.Error(), tc.message), fmt.Sprint(err))
			data, readErr := os.ReadFile(log)
			must(t, readErr)
			check(t, !strings.Contains(string(data), "BEGIN\nnew\n"), string(data))
		})
	}
}

func TestRexCreationFailuresAreNotRetried(t *testing.T) {
	for _, body := range []string{`echo invalid`, `echo '{}'`, `echo rejected >&2; exit 7`} {
		t.Run(body, func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Rex(t, "case \"$1\" in new) "+body+"; exit 0 ;; esac\n"+testutil.RexResponses)
			err := Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"})
			check(t, err != nil, "must reject failed creation")
			data, readErr := os.ReadFile(log)
			must(t, readErr)
			check(t, strings.Count(string(data), "BEGIN\nnew\n") == 1, string(data))
		})
	}
}

func TestRexCorruptMappingAndLockContentionPreventCreation(t *testing.T) {
	for _, tc := range []struct{ name, suffix, content string }{
		{"corrupt", "", "invalid"},
		{"wrong-version", "", `{"version":2}`},
		{"locked", ".lock", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Rex(t, testutil.RexResponses)
			request := integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"}
			must(t, Hooks().Open(context.Background(), request))
			entries, err := filepath.Glob(filepath.Join(repo.Common, "namu", "rex", "*.json"))
			must(t, err)
			check(t, len(entries) == 1, "must persist mapping")
			must(t, os.WriteFile(entries[0]+tc.suffix, []byte(tc.content), 0600))
			err = Hooks().Open(context.Background(), request)
			check(t, err != nil, "must reject unavailable state")
			data, readErr := os.ReadFile(log)
			must(t, readErr)
			check(t, strings.Count(string(data), "BEGIN\nnew\n") == 1, string(data))
		})
	}
}

func TestRexLabelLimitPreventsExternalCalls(t *testing.T) {
	log := testutil.Rex(t, testutil.RexResponses)
	err := Hooks().Open(context.Background(), integration.OpenRequest{Branch: strings.Repeat("界", 129)})
	check(t, err != nil && strings.Contains(err.Error(), "128"), fmt.Sprint(err))
	_, err = os.Stat(log)
	check(t, errors.Is(err, os.ErrNotExist), "must not invoke Rex")
}

func TestRexCancellationAndTimeout(t *testing.T) {
	repo := testutil.Repository(t)
	testutil.Rex(t, "exec /bin/sleep 60")
	request := integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Hooks().Open(ctx, request)
	check(t, errors.Is(err, context.Canceled), fmt.Sprint(err))
	err = openWithTimeout(context.Background(), request, 10*time.Millisecond)
	check(t, errors.Is(err, context.DeadlineExceeded), fmt.Sprint(err))
}

func TestRexMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	err := Hooks().Open(context.Background(), integration.OpenRequest{Branch: "main"})
	check(t, err != nil && strings.Contains(err.Error(), "executable file not found"), fmt.Sprint(err))
}

func TestRexSameBranchInSeparateRepositoriesCreatesSeparateSessions(t *testing.T) {
	first := testutil.Repository(t)
	second := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: first.Source, Path: first.Main, Branch: "main"}))
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: second.Source, Path: second.Main, Branch: "main"}))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 2, string(data))
}

func TestRexUnicodeLabelAtLimitIsAccepted(t *testing.T) {
	repo := testutil.Repository(t)
	log := testutil.Rex(t, testutil.RexResponses)
	branch := strings.Repeat("界", 128)
	must(t, Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: branch}))
	data, err := os.ReadFile(log)
	must(t, err)
	check(t, strings.Contains(string(data), "new\n"+branch+"\n"), string(data))
}

func TestRexMappingWriteFailureDoesNotSelectOrRemoveCreatedSession(t *testing.T) {
	repo := testutil.Repository(t)
	t.Setenv("TEST_REX_STATE", filepath.Join(repo.Common, "namu", "rex"))
	log := testutil.Rex(t, `case "$1" in new) rm "$TEST_REX_STATE/"*.lock; rmdir "$TEST_REX_STATE"; printf blocked > "$TEST_REX_STATE" ;; esac
`+testutil.RexResponses)
	err := Hooks().Open(context.Background(), integration.OpenRequest{Source: repo.Source, Path: repo.Main, Branch: "main", Focus: true})
	check(t, err != nil && strings.Contains(err.Error(), "session:created created but mapping could not be saved"), fmt.Sprint(err))
	data, readErr := os.ReadFile(log)
	must(t, readErr)
	check(t, strings.Count(string(data), "BEGIN\nnew\n") == 1 && !strings.Contains(string(data), "session.select") && !strings.Contains(string(data), "kill"), string(data))
}
