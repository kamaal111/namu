package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		name, open, closed string
		want               map[string]int
	}{
		{"open including drafts", `[{"number":42}]`, `[{"number":50}]`, map[string]int{"feature/cards": 42, "123": 42}},
		{"merged or closed fallback", `[]`, `[{"number":50}]`, map[string]int{"feature/cards": 50, "123": 50}},
		{"empty", `[]`, `[]`, map[string]int{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require(t, err == nil, err)
			t.Setenv("GH_EXPECT_DIR", dir)
			response := fmt.Sprintf(`echo '{"data":{"repository":{
				"open0":{"nodes":%s}, "closed0":{"nodes":%s},
				"open1":{"nodes":%s}, "closed1":{"nodes":%s}
			}}}'`, tt.open, tt.closed, tt.open, tt.closed)
			log := testutil.GitHub(t, `test "$PWD" = "$GH_EXPECT_DIR" || exit 7
`+authenticated+response)
			numbers, err := PRNumbers(context.Background(), dir, []string{"feature/cards", "", "123", "feature/cards"})
			require(t, err == nil, err)
			require(t, reflect.DeepEqual(numbers, tt.want), numbers)
			recorded := calls(t, log)
			require(t, strings.Contains(recorded, "auth status --active --hostname github.example.com\n"), recorded)
			require(t, strings.Count(recorded, "api graphql --hostname github.example.com --input -") == 1, recorded)
			var request struct {
				Query     string            `json:"query"`
				Variables map[string]string `json:"variables"`
			}
			require(t, json.Unmarshal([]byte(calls(t, log+".requests")), &request) == nil, "valid GraphQL request")
			require(t, reflect.DeepEqual(request.Variables, map[string]string{"owner": "team", "name": "project", "branch0": "feature/cards", "branch1": "123"}), request.Variables)
			require(t, strings.Contains(request.Query, "states: [OPEN]"), request.Query)
			require(t, strings.Contains(request.Query, "states: [CLOSED, MERGED]"), request.Query)
			require(t, strings.Count(request.Query, "first: 1") == 4, request.Query)
			require(t, strings.Count(request.Query, "orderBy: {field: UPDATED_AT, direction: DESC}") == 4, request.Query)
		})
	}
}

func TestPRNumbersBatchCount(t *testing.T) {
	for _, tt := range []struct{ branches, queries int }{{20, 1}, {25, 1}, {26, 2}, {51, 3}} {
		t.Run(fmt.Sprint(tt.branches), func(t *testing.T) {
			log := testutil.GitHub(t, authenticated+`echo '{"data":{"repository":{}}}'`)
			branches := make([]string, tt.branches)
			for i := range branches {
				branches[i] = fmt.Sprintf("feature/%d", i)
			}
			numbers, err := PRNumbers(context.Background(), t.TempDir(), branches)
			require(t, err == nil, err)
			require(t, len(numbers) == 0, numbers)
			require(t, strings.Count(calls(t, log), "api graphql ") == tt.queries, calls(t, log))
			decoder := json.NewDecoder(strings.NewReader(calls(t, log+".requests")))
			var actual []string
			for batch := 0; batch < tt.queries; batch++ {
				var request struct {
					Variables map[string]string `json:"variables"`
				}
				require(t, decoder.Decode(&request) == nil, "valid batch")
				for i := 0; i < len(request.Variables)-2; i++ {
					actual = append(actual, request.Variables[fmt.Sprintf("branch%d", i)])
				}
			}
			require(t, reflect.DeepEqual(actual, branches), actual)
		})
	}
}

func TestPRNumbersEmptyBranches(t *testing.T) {
	log := testutil.GitHub(t, "exit 1")
	numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"", ""})
	require(t, err == nil, err)
	require(t, len(numbers) == 0, numbers)
	_, err = os.Stat(log)
	require(t, errors.Is(err, os.ErrNotExist), err)
}

func TestPRNumbersBranchEncoding(t *testing.T) {
	log := testutil.GitHub(t, authenticated+`echo '{"data":{"repository":{}}}'`)
	branch := "feature/\"quoted\\branch"
	_, err := PRNumbers(context.Background(), t.TempDir(), []string{branch})
	require(t, err == nil, err)
	var request struct {
		Query     string            `json:"query"`
		Variables map[string]string `json:"variables"`
	}
	require(t, json.Unmarshal([]byte(calls(t, log+".requests")), &request) == nil, "valid request")
	require(t, request.Variables["branch0"] == branch, request.Variables)
	require(t, !strings.Contains(request.Query, branch), request.Query)
}

func TestPRNumbersFailures(t *testing.T) {
	for _, tt := range []struct{ name, response string }{
		{"command failure", "echo unavailable >&2; exit 1"},
		{"malformed JSON", "echo broken"},
		{"null repository", `echo '{"data":{"repository":null}}'`},
		{"negative number", `echo '{"data":{"repository":{"open0":{"nodes":[{"number":-1}]}}}}'`},
		{"missing number", `echo '{"data":{"repository":{"open0":{"nodes":[{}]}}}}'`},
		{"string number", `echo '{"data":{"repository":{"open0":{"nodes":[{"number":"42"}]}}}}'`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.GitHub(t, authenticated+tt.response)
			numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"broken"})
			require(t, err == nil, err)
			require(t, len(numbers) == 0, numbers)
		})
	}
}

func TestPRNumbersPartialData(t *testing.T) {
	for _, broken := range []string{
		`"open1":null, "closed1":{"nodes":[{"number":50}]}`,
		`"closed1":{"nodes":[{"number":50}]}`,
		`"open1":{"nodes":[{"number":"bad"}]}, "closed1":{"nodes":[{"number":50}]}`,
		`"open1":{}, "closed1":{"nodes":[{"number":50}]}`,
		`"open1":{"nodes":null}, "closed1":{"nodes":[{"number":50}]}`,
		`"open1":{"nodes":[{"number":-1}]}, "closed1":{"nodes":[{"number":50}]}`,
	} {
		t.Run(broken, func(t *testing.T) {
			testutil.GitHub(t, authenticated+`echo '{"data":{"repository":{
				"open0":{"nodes":[{"number":42}]}, `+broken+`
			}}}'`)
			numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"feature/cards", "broken"})
			require(t, err == nil, err)
			require(t, reflect.DeepEqual(numbers, map[string]int{"feature/cards": 42}), numbers)
		})
	}
}

func TestPRNumbersErroredFieldsAreUnresolved(t *testing.T) {
	testutil.GitHub(t, authenticated+`echo '{
		"data":{"repository":{
			"open0":{"nodes":[{"number":42}]},
			"open1":{"nodes":[]}, "closed1":{"nodes":[{"number":50}]},
			"open2":{"nodes":[]}, "closed2":{"nodes":[{"number":60}]}
		}},
		"errors":[
			{"message":"unavailable", "path":["repository","open1"]},
			{"message":"unavailable", "path":["repository","closed2","nodes",0,"number"]}
		]
	}'
exit 1`)
	numbers, err := PRNumbers(context.Background(), t.TempDir(), []string{"feature/cards", "broken-open", "broken-closed"})
	require(t, err == nil, err)
	require(t, reflect.DeepEqual(numbers, map[string]int{"feature/cards": 42}), numbers)
}

func TestPRNumbersLaterBatchFailurePreservesSuccess(t *testing.T) {
	for _, tt := range []struct {
		name, failure string
		queries       int
	}{
		{"command failure", "exit 1", 3},
		{"timeout", "exec /bin/sleep 60", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			log := testutil.GitHub(t, authenticated+`
case "$(wc -l < "$GH_CALLS")" in
  *3) echo '{"data":{"repository":{"open0":{"nodes":[{"number":42}]}}}}' ;;
  *) `+tt.failure+` ;;
esac`)
			branches := make([]string, 51)
			for i := range branches {
				branches[i] = fmt.Sprintf("feature/%d", i)
			}
			start := time.Now()
			numbers, err := prNumbers(context.Background(), t.TempDir(), branches, time.Second)
			require(t, err == nil, err)
			require(t, reflect.DeepEqual(numbers, map[string]int{"feature/0": 42}), numbers)
			require(t, time.Since(start) < 5*time.Second, "lookup must be bounded")
			require(t, strings.Count(calls(t, log), "api graphql ") == tt.queries, calls(t, log))
		})
	}
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
