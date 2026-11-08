package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	git "github.com/kamaal111/namu/internal/git"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"text/tabwriter"

	"github.com/kamaal111/namu/internal/pathdisplay"
	"github.com/kamaal111/namu/internal/testutil"
)

const githubReady = `
case "$1" in
  repo) echo '{"nameWithOwner":"team/project","url":"https://github.example.com/team/project"}'; exit 0 ;;
  auth) exit 0 ;;
esac
`

func TestListWithoutGitHub(t *testing.T) {
	repo := testutil.Repository(t)
	realGit, err := exec.LookPath("git")
	must(t, err)
	bin := t.TempDir()
	must(t, os.Symlink(realGit, filepath.Join(bin, "git")))
	t.Setenv("PATH", bin)
	out, diagnostics, err := execute(t, repo.Source, "list")
	must(t, err)
	lines := strings.Split(out, "\n")
	check(t, strings.Join(strings.Fields(lines[0]), " ") == "BRANCH PATH HEAD NOTES", out)
	check(t, strings.Contains(out, "main"), out)
	check(t, diagnostics == "", diagnostics)
}

func TestListPRColumn(t *testing.T) {
	for _, tt := range []struct {
		name, body, header, blankCell, cardsCell string
	}{
		{"open preferred", githubReady + `echo '{"data":{"repository":{
			"open0":{"nodes":[]}, "closed0":{"nodes":[]},
			"open1":{"nodes":[{"number":42}]}, "closed1":{"nodes":[{"number":50}]},
			"open2":null
		}},"errors":[{"message":"unavailable","path":["repository","open2"]}]}'
exit 1`, "BRANCH\tPATH\tPR\tHEAD\tNOTES\n", "\t", "#42\t"},
		{"merged or closed", githubReady + `echo '{"data":{"repository":{
			"open0":{"nodes":[]}, "closed0":{"nodes":[]},
			"open1":{"nodes":[]}, "closed1":{"nodes":[{"number":40}]}
		}}}'`, "BRANCH\tPATH\tPR\tHEAD\tNOTES\n", "\t", "#40\t"},
		{"no matches", githubReady + `echo '{"data":{"repository":{}}}'`, "BRANCH\tPATH\tHEAD\tNOTES\n", "", ""},
		{"lookup failure", githubReady + `echo unavailable >&2; exit 1`, "BRANCH\tPATH\tHEAD\tNOTES\n", "", ""},
		{"unauthenticated", `case "$1" in repo) echo '{"nameWithOwner":"team/project","url":"https://github.example.com/team/project"}' ;; *) exit 1 ;; esac`, "BRANCH\tPATH\tHEAD\tNOTES\n", "", ""},
		{"unresolved repository", "exit 1", "BRANCH\tPATH\tHEAD\tNOTES\n", "", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.GitHub(t, tt.body)
			repo := testutil.Repository(t)
			parent, err := filepath.EvalSymlinks(t.TempDir())
			must(t, err)
			cards, failed, detached := filepath.Join(parent, "cards"), filepath.Join(parent, "failed"), filepath.Join(parent, "detached")
			testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/cards", cards)
			testutil.Git(t, repo.Source, "worktree", "lock", cards)
			testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/fails", failed)
			testutil.Git(t, repo.Source, "worktree", "add", "--detach", detached)
			head := testutil.Git(t, repo.Source, "rev-parse", "HEAD")
			before := testutil.Git(t, repo.Source, "worktree", "list", "--porcelain")
			out, diagnostics, err := execute(t, repo.Source, "list")
			must(t, err)
			var expected bytes.Buffer
			w := tabwriter.NewWriter(&expected, 0, 4, 2, ' ', 0)
			_, err = fmt.Fprint(w, tt.header)
			must(t, err)
			_, err = fmt.Fprintf(w, "main\t%s\t%s%s\t—\nfeature/cards\t%s\t%s%s\tLocked against removal\n(detached)\t%s\t%s%s\tNo branch checked out\nfeature/fails\t%s\t%s%s\t—\n",
				pathdisplay.Shorten(repo.Main), tt.blankCell, head,
				pathdisplay.Shorten(cards), tt.cardsCell, head,
				pathdisplay.Shorten(detached), tt.blankCell, head,
				pathdisplay.Shorten(failed), tt.blankCell, head)
			must(t, err)
			must(t, w.Flush())
			check(t, out == expected.String(), "got:\n"+out+"want:\n"+expected.String())
			check(t, diagnostics == "", diagnostics)
			check(t, testutil.Git(t, repo.Source, "worktree", "list", "--porcelain") == before, "list must preserve worktrees")
		})
	}
}

func TestListPRFlags(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   []string
		enabled bool
	}{
		{"default", nil, true},
		{"positive", []string{"--pr"}, true},
		{"negative", []string{"--no-pr"}, false},
		{"positive-false", []string{"--pr=false"}, false},
		{"negative-false", []string{"--no-pr=false"}, true},
		{"positive-last", []string{"--no-pr", "--pr"}, true},
		{"negative-last", []string{"--pr", "--no-pr"}, false},
		{"false-negative-last", []string{"--pr=false", "--no-pr=false"}, true},
		{"false-positive-last", []string{"--no-pr=false", "--pr=false"}, false},
		{"repeated-positive", []string{"--pr=false", "--pr"}, true},
		{"repeated-negative", []string{"--no-pr", "--no-pr=false"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := testutil.GitHub(t, githubReady+`echo '{"data":{"repository":{"open0":{"nodes":[{"number":42}]}}}}'`)
			repo := testutil.Repository(t)
			before := testutil.Git(t, repo.Source, "worktree", "list", "--porcelain")
			out, diagnostics, err := execute(t, repo.Source, append([]string{"list"}, tc.flags...)...)
			must(t, err)
			calls, _ := os.ReadFile(log)
			check(t, (len(calls) > 0) == tc.enabled, string(calls))
			check(t, strings.Contains(out, "#42") == tc.enabled, out)
			header := strings.Join(strings.Fields(strings.Split(out, "\n")[0]), " ")
			check(t, header == map[bool]string{true: "BRANCH PATH PR HEAD NOTES", false: "BRANCH PATH HEAD NOTES"}[tc.enabled], out)
			check(t, strings.Contains(out, "main"), out)
			check(t, diagnostics == "", diagnostics)
			check(t, testutil.Git(t, repo.Source, "worktree", "list", "--porcelain") == before, "list must preserve worktrees")
		})
	}
}

func TestListPRFlagsRejectInvalidBoolean(t *testing.T) {
	for _, flag := range []string{"--pr=invalid", "--no-pr=invalid"} {
		t.Run(flag, func(t *testing.T) {
			log := testutil.GitHub(t, "exit 1")
			out, _, err := execute(t, t.TempDir(), "list", flag)
			check(t, err != nil && strings.Contains(err.Error(), "invalid"), "must reject invalid boolean")
			check(t, out == "", out)
			calls, _ := os.ReadFile(log)
			check(t, len(calls) == 0, string(calls))
		})
	}
}

func TestListJSON(t *testing.T) {
	repo := testutil.Repository(t)
	parent, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	target := filepath.Join(parent, "cards with spaces")
	testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/cards", target)
	testutil.Git(t, repo.Source, "worktree", "lock", "--reason", "keep", target)
	head := testutil.Git(t, repo.Source, "rev-parse", "HEAD")
	out, diagnostics, err := execute(t, repo.Source, "list", "--json", "--no-pr")
	must(t, err)
	var rows []map[string]any
	must(t, json.Unmarshal([]byte(out), &rows))
	check(t, len(rows) == 2, out)
	check(t, len(rows[1]) == 5, out)
	check(t, rows[1]["branch"] == "feature/cards", out)
	check(t, rows[1]["path"] == target, out)
	check(t, rows[1]["head"] == head, out)
	check(t, rows[1]["pr"] == nil, out)
	notes, err := json.Marshal(rows[1]["notes"])
	must(t, err)
	check(t, string(notes) == `["Locked against removal"]`, out)
	check(t, diagnostics == "", diagnostics)
}

func TestListTableFlag(t *testing.T) {
	repo := testutil.Repository(t)
	plain, _, err := execute(t, repo.Source, "list", "--no-pr")
	must(t, err)
	table, _, err := execute(t, repo.Source, "list", "--table", "--no-pr")
	must(t, err)
	check(t, plain == table, table)
}

func TestListOutputFlags(t *testing.T) {
	out, _, err := execute(t, t.TempDir(), "list", "--table", "--json")
	check(t, err != nil && strings.Contains(err.Error(), "table") && strings.Contains(err.Error(), "json"), fmt.Sprint(err))
	check(t, out == "", out)
}

func TestListJSONEntries(t *testing.T) {
	trees := []git.Worktree{
		{Path: "/tmp/cards\twith\n雪", Branch: "feature/cards", Head: strings.Repeat("1", 40)},
		{Path: "/tmp/detached", Head: strings.Repeat("2", 40), Locked: "keep", Prunable: "missing"},
	}
	entries := listEntries(trees, map[string]int{"feature/cards": 42})
	encoded, err := json.Marshal(entries)
	must(t, err)
	expected := `[{"branch":"feature/cards","path":"/tmp/cards\twith\n雪","pr":42,"head":"1111111111111111111111111111111111111111","notes":[]},{"branch":null,"path":"/tmp/detached","pr":null,"head":"2222222222222222222222222222222222222222","notes":["No branch checked out","Locked against removal","Stale worktree registration"]}]`
	check(t, string(encoded) == expected, string(encoded))
	empty, err := json.Marshal(listEntries(nil, nil))
	must(t, err)
	check(t, string(empty) == "[]", string(empty))
}

func TestListJSONPRFlags(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
		want  string
		calls bool
	}{
		{"default", nil, `"pr":42`, true},
		{"disabled", []string{"--no-pr"}, `"pr":null`, false},
		{"positive-last", []string{"--no-pr", "--pr"}, `"pr":42`, true},
		{"negative-last", []string{"--pr", "--no-pr"}, `"pr":null`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := testutil.GitHub(t, githubReady+`echo '{"data":{"repository":{"open0":{"nodes":[{"number":42}]}}}}'`)
			repo := testutil.Repository(t)
			out, _, err := execute(t, repo.Source, append([]string{"list", "--json"}, tc.flags...)...)
			must(t, err)
			check(t, strings.Contains(out, tc.want), out)
			calls, _ := os.ReadFile(log)
			check(t, (len(calls) > 0) == tc.calls, string(calls))
		})
	}
}

type listFailWriter struct{}

func (listFailWriter) Write(p []byte) (int, error) { return 0, io.ErrClosedPipe }

func TestListWriteFailure(t *testing.T) {
	repo := testutil.Repository(t)
	for _, flag := range []string{"--json", "--table"} {
		t.Run(flag, func(t *testing.T) {
			cmd := New(repo.Source)
			cmd.SetOut(listFailWriter{})
			cmd.SetArgs([]string{"list", flag, "--no-pr"})
			err := cmd.Execute()
			check(t, errors.Is(err, io.ErrClosedPipe), fmt.Sprint(err))
		})
	}
}

func TestListJSONOutsideRepository(t *testing.T) {
	out, _, err := execute(t, t.TempDir(), "list", "--json")
	check(t, err != nil, "listing must fail outside a repository")
	check(t, out == "", out)
}
