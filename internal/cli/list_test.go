package cli

import (
	"bytes"
	"fmt"
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
