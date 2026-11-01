package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestBoolPairLastWins(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   []string
		enabled bool
	}{
		{"default", nil, true},
		{"positive-true", []string{"--pr=true"}, true},
		{"negative-true", []string{"--no-pr=true"}, false},
		{"positive-last", []string{"--no-pr", "--pr"}, true},
		{"negative-last", []string{"--pr", "--no-pr"}, false},
		{"positive-false-last", []string{"--no-pr=false", "--pr=false"}, false},
		{"negative-false-last", []string{"--pr=false", "--no-pr=false"}, true},
		{"repeated-positive", []string{"--pr", "--pr=false", "--pr"}, true},
		{"repeated-negative", []string{"--no-pr", "--no-pr=false", "--no-pr"}, false},
		{"end-options", []string{"--", "--no-pr"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "list"}
			registerBoolPair(cmd, "pr", true, "Look up PRs", "Skip PRs")
			must(t, cmd.ParseFlags(tc.flags))
			enabled, err := cmd.Flags().GetBool("pr")
			must(t, err)
			check(t, enabled == tc.enabled, "incorrect effective value")
			disabled, err := cmd.Flags().GetBool("no-pr")
			must(t, err)
			check(t, disabled == !tc.enabled, "negative flag must invert effective value")
		})
	}
}

func TestBoolPairCommandIsolation(t *testing.T) {
	root := New(t.TempDir())
	create, _, err := root.Find([]string{"create"})
	must(t, err)
	open, _, err := root.Find([]string{"open"})
	must(t, err)
	must(t, create.ParseFlags([]string{"--no-focus"}))
	focus, err := open.Flags().GetBool("focus")
	must(t, err)
	check(t, focus, "create flags must not change open defaults")
}

func TestBoolPairHelpAndCompletion(t *testing.T) {
	dir := t.TempDir()
	out, _, err := execute(t, dir, "list", "--help")
	must(t, err)
	check(t, strings.Contains(out, "--pr") && strings.Contains(out, "--no-pr"), out)
	check(t, strings.Contains(out, "(default true)"), out)
	out, _, err = execute(t, dir, "__completeNoDesc", "list", "--no-p")
	must(t, err)
	check(t, strings.Contains(out, "--no-pr\n"), out)
}
