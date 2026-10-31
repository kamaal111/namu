package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kamaal111/namu/internal/testutil"
)

const authenticated = `
case "$1 $2" in
  'repo view') printf '%s\n' '{"nameWithOwner":"team/project","url":"https://github.example.com/team/project"}'; exit 0 ;;
  'auth status') test "$GH_PROMPT_DISABLED" = 1; exit $? ;;
esac
`

func require(t *testing.T, ok bool, message any) {
	t.Helper()
	if !ok {
		t.Fatal(message)
	}
}

func calls(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require(t, err == nil, err)
	return string(b)
}

func TestPRNumbersAuthentication(t *testing.T) {
	for _, tt := range []struct {
		name, body, expectedCalls string
	}{
		{"failed auth", `
case "$1" in
  repo) echo '{"nameWithOwner":"team/project","url":"https://github.example.com/team/project"}' ;;
  *) echo 'not authenticated' >&2; exit 1 ;;
esac`, "repo view --json nameWithOwner,url\nauth status --active --hostname github.example.com\n"},
		{"unresolved repo", "exit 1", "repo view --json nameWithOwner,url\n"},
		{"malformed repo", "echo broken", "repo view --json nameWithOwner,url\n"},
		{"missing hostname", `echo '{"nameWithOwner":"team/project","url":""}'`, "repo view --json nameWithOwner,url\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := testutil.GitHub(t, tt.body)
			numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"feature/cards"})
			require(t, err == nil, err)
			require(t, len(numbers) == 0, numbers)
			require(t, calls(t, log) == tt.expectedCalls, calls(t, log))
		})
	}
}

func TestPRNumbersMissingExecutable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"feature/cards"})
	require(t, err == nil, err)
	require(t, len(numbers) == 0, numbers)
}

func TestPRNumbersSelection(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       map[string]int
		queries    int
	}{
		{"open including drafts", `echo '[{"number":42}]'`, map[string]int{"feature/cards": 42, "123": 42}, 2},
		{"merged or closed fallback", `case "$8" in open) echo '[]' ;; all) echo '[{"number":40}]' ;; esac`, map[string]int{"feature/cards": 40, "123": 40}, 4},
		{"empty", `echo '[]'`, map[string]int{}, 4},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require(t, err == nil, err)
			t.Setenv("GH_EXPECT_DIR", dir)
			log := testutil.GitHub(t, `test "$PWD" = "$GH_EXPECT_DIR" || exit 7
`+authenticated+tt.body)
			numbers, err := PRNumbers(context.Background(), dir, []string{"feature/cards", "", "123", "feature/cards"})
			require(t, err == nil, err)
			require(t, reflect.DeepEqual(numbers, tt.want), numbers)
			recorded := calls(t, log)
			require(t, strings.Contains(recorded, "auth status --active --hostname github.example.com\n"), recorded)
			require(t, strings.Count(recorded, "pr list ") == tt.queries, recorded)
			require(t, strings.Contains(recorded, "--repo github.example.com/team/project --head feature/cards --state open --search sort:updated-desc --limit 1 --json number"), recorded)
			require(t, strings.Contains(recorded, "--head 123 --state open"), recorded)
		})
	}
}

func TestPRNumbersFailures(t *testing.T) {
	for _, tt := range []struct{ name, response string }{
		{"command failure", "echo unavailable >&2; exit 1"},
		{"malformed JSON", "echo broken"},
		{"negative number", `echo '[{"number":-1}]'`},
		{"missing number", `echo '[{}]'`},
		{"string number", `echo '[{"number":"42"}]'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := testutil.GitHub(t, authenticated+`
case "$6" in
  feature/cards) echo '[{"number":42}]' ;;
  *) `+tt.response+` ;;
esac`)
			numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"feature/cards", "broken"})
			require(t, err == nil, err)
			require(t, reflect.DeepEqual(numbers, map[string]int{"feature/cards": 42}), numbers)
			require(t, strings.Count(calls(t, log), "--head broken") == 1, calls(t, log))
		})
	}
}

func TestPRNumbersTimeoutPreservesPartialSuccess(t *testing.T) {
	testutil.GitHub(t, authenticated+`
case "$6" in
  feature/cards) echo '[{"number":42}]' ;;
  *) exec /bin/sleep 60 ;;
esac`)
	start := time.Now()
	numbers, err := prNumbers(context.Background(), t.TempDir(), []string{"feature/cards", "slow"}, time.Second)
	require(t, err == nil, err)
	require(t, reflect.DeepEqual(numbers, map[string]int{"feature/cards": 42}), numbers)
	require(t, time.Since(start) < 5*time.Second, "lookup must be bounded")
}

func TestPRNumbersParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	testutil.GitHub(t, "exit 1")
	_, err := PRNumbers(ctx, t.TempDir(), []string{"feature/cards"})
	require(t, errors.Is(err, context.Canceled), err)
}

func TestPRNumbersParentDeadline(t *testing.T) {
	testutil.GitHub(t, "exec /bin/sleep 60")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := PRNumbers(ctx, t.TempDir(), []string{"feature/cards"})
	require(t, errors.Is(err, context.DeadlineExceeded), err)
}
