package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kamaal111/namu/internal/testutil"
)

func TestProjectFocusDefaultsAndOverrides(t *testing.T) {
	for _, action := range []string{"create", "open"} {
		for _, tc := range []struct {
			name, config string
			flags        []string
			focus        bool
		}{
			{"missing-default", "", nil, true},
			{"disabled", "[defaults]\nfocus = false\n", nil, false},
			{"enabled", "[defaults]\nfocus = true\n", nil, true},
			{"focus-override", "[defaults]\nfocus = false\n", []string{"--focus"}, true},
			{"no-focus-false-override", "[defaults]\nfocus = false\n", []string{"--no-focus=false"}, true},
			{"no-focus-override", "[defaults]\nfocus = true\n", []string{"--no-focus"}, false},
			{"last-focus-wins", "[defaults]\nfocus = false\n", []string{"--no-focus", "--focus"}, true},
			{"last-no-focus-wins", "[defaults]\nfocus = true\n", []string{"--focus", "--no-focus"}, false},
			{"focus-false-override", "[defaults]\nfocus = true\n", []string{"--focus=false"}, false},
		} {
			t.Run(action+"/"+tc.name, func(t *testing.T) {
				repo := testutil.Repository(t)
				t.Setenv("HERDR_ENV", "")
				testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), tc.config)
				testutil.Git(t, repo.Source, "branch", "feature/existing")
				result := filepath.Join(t.TempDir(), "result")
				testutil.Write(t, result, "")
				t.Setenv("NAMU_SHELL_RESULT", result)
				args := map[string][]string{
					"create": {"create", "feature/cards", "--base", "main"},
					"open":   {"open", "feature/existing"},
				}[action]
				out, _, err := execute(t, repo.Source, append(args, tc.flags...)...)
				must(t, err)
				destination, err := os.ReadFile(result)
				must(t, err)
				check(t, (len(destination) > 0) == tc.focus, string(destination))
				check(t, strings.HasSuffix(out, "\n"), out)
			})
		}
	}
}

func TestProjectFocusUsesInvokingCheckoutRoot(t *testing.T) {
	repo := testutil.Repository(t)
	t.Setenv("HERDR_ENV", "")
	testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[defaults]\nfocus = true\n")
	linked := filepath.Join(t.TempDir(), "linked")
	testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/linked", linked)
	testutil.Write(t, filepath.Join(linked, ".namu.toml"), "[defaults]\nfocus = false\n")
	subdir := filepath.Join(linked, "nested")
	must(t, os.Mkdir(subdir, 0700))
	result := filepath.Join(t.TempDir(), "result")
	testutil.Write(t, result, "")
	t.Setenv("NAMU_SHELL_RESULT", result)
	out, _, err := execute(t, subdir, "open", "main")
	must(t, err)
	check(t, out == repo.Main+"\n", out)
	destination, err := os.ReadFile(result)
	must(t, err)
	check(t, len(destination) == 0, string(destination))
}

func TestInvalidProjectDefaultsFailBeforeNavigationSideEffects(t *testing.T) {
	for _, args := range [][]string{
		{"create", "feature/cards", "--base", "main"},
		{"open", "feature/new"},
		{"open", "main", "--focus"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			repo := testutil.Repository(t)
			log := testutil.Rex(t, testutil.RexResponses)
			testutil.Git(t, repo.Source, "branch", "feature/new")
			testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[defaults]\nfocus = 'false'\n")
			before := testutil.Git(t, repo.Source, "worktree", "list", "--porcelain")
			out, _, err := execute(t, repo.Source, args...)
			check(t, err != nil && strings.Contains(err.Error(), "read .namu.toml"), "invalid config must fail")
			check(t, out == "", out)
			check(t, testutil.Git(t, repo.Source, "worktree", "list", "--porcelain") == before, "worktrees changed")
			calls, _ := os.ReadFile(log)
			check(t, len(calls) == 0, string(calls))
		})
	}
}

func TestConfiguredFocusDoesNotSelectIntegrations(t *testing.T) {
	for _, tc := range []struct {
		name, env     string
		flags         []string
		called, focus bool
	}{
		{"outside-rex", "", nil, false, false},
		{"detected-rex-background", "session:source", nil, true, false},
		{"detected-rex-override", "session:source", []string{"--focus"}, true, true},
		{"explicitly-disabled-rex", "session:source", []string{"--rex=false"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			t.Setenv("HERDR_ENV", "")
			log := testutil.Rex(t, testutil.RexResponses)
			t.Setenv("REX_SESSION", tc.env)
			testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[defaults]\nfocus = false\n")
			out, _, err := execute(t, repo.Source, append([]string{"open", "main"}, tc.flags...)...)
			must(t, err)
			check(t, out == repo.Main+"\n", out)
			calls, _ := os.ReadFile(log)
			check(t, strings.Contains(string(calls), "BEGIN\nnew\n") == tc.called, string(calls))
			check(t, strings.Contains(string(calls), "session.select") == tc.focus, string(calls))
		})
	}
}

func TestProjectPRDefaultsAndOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, config string
		flags        []string
		enabled      bool
	}{
		{"omitted", "", nil, true},
		{"disabled", "[defaults]\npr = false\n", nil, false},
		{"enabled", "[defaults]\npr = true\n", nil, true},
		{"positive-override", "[defaults]\npr = false\n", []string{"--pr"}, true},
		{"negative-false-override", "[defaults]\npr = false\n", []string{"--no-pr=false"}, true},
		{"negative-override", "[defaults]\npr = true\n", []string{"--no-pr"}, false},
		{"positive-false-override", "[defaults]\npr = true\n", []string{"--pr=false"}, false},
		{"last-positive-wins", "[defaults]\npr = false\n", []string{"--no-pr", "--pr"}, true},
		{"last-negative-wins", "[defaults]\npr = true\n", []string{"--pr", "--no-pr"}, false},
		{"focus-independent", "[defaults]\nfocus = false\n", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := testutil.Repository(t)
			testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), tc.config)
			log := testutil.GitHub(t, githubReady+`echo '{"data":{"repository":{"open0":{"nodes":[{"number":42}]}}}}'`)
			out, diagnostics, err := execute(t, repo.Source, append([]string{"list"}, tc.flags...)...)
			must(t, err)
			calls, _ := os.ReadFile(log)
			check(t, (len(calls) > 0) == tc.enabled, string(calls))
			check(t, strings.Contains(out, "#42") == tc.enabled, out)
			header := strings.Join(strings.Fields(strings.Split(out, "\n")[0]), " ")
			check(t, header == map[bool]string{true: "BRANCH PATH PR HEAD NOTES", false: "BRANCH PATH HEAD NOTES"}[tc.enabled], out)
			check(t, diagnostics == "", diagnostics)
		})
	}
}

func TestInvalidPRDefaultsFailBeforeGitHubLookup(t *testing.T) {
	repo := testutil.Repository(t)
	testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[defaults]\npr = 'false'\n")
	log := testutil.GitHub(t, "exit 1")
	out, _, err := execute(t, repo.Source, "list", "--no-pr")
	check(t, err != nil && strings.Contains(err.Error(), "read .namu.toml"), "invalid PR default must fail")
	check(t, out == "", out)
	calls, _ := os.ReadFile(log)
	check(t, len(calls) == 0, string(calls))
}

func TestProjectPRUsesInvokingCheckoutRoot(t *testing.T) {
	repo := testutil.Repository(t)
	testutil.Write(t, filepath.Join(repo.Source, ".namu.toml"), "[defaults]\npr = true\n")
	linked := filepath.Join(t.TempDir(), "linked")
	testutil.Git(t, repo.Source, "worktree", "add", "-b", "feature/linked", linked)
	testutil.Write(t, filepath.Join(linked, ".namu.toml"), "[defaults]\npr = false\n")
	subdir := filepath.Join(linked, "nested")
	must(t, os.Mkdir(subdir, 0700))
	log := testutil.GitHub(t, "exit 1")
	out, _, err := execute(t, subdir, "list")
	must(t, err)
	check(t, strings.Contains(out, "feature/linked"), out)
	calls, _ := os.ReadFile(log)
	check(t, len(calls) == 0, string(calls))
}
