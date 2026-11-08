package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/github"
	"github.com/kamaal111/namu/internal/pathdisplay"
	"github.com/spf13/cobra"
)

type listEntry struct {
	Branch *string  `json:"branch"`
	Path   string   `json:"path"`
	PR     *int     `json:"pr"`
	Head   string   `json:"head"`
	Notes  []string `json:"notes"`
}

func listCommand(dir string) *cobra.Command {
	var asJSON, table bool
	cmd := &cobra.Command{Use: "list", Short: "List all registered checkouts", Args: cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmd.ValidateFlagGroups(); err != nil {
				return err
			}
			return applyProjectDefaults(cmd, dir)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := service(cmd, dir)
			if err != nil {
				return err
			}
			trees, err := git.Worktrees(cmd.Context(), s.Repo.Source)
			if err != nil {
				return err
			}
			var prs map[string]int
			pr, _ := cmd.Flags().GetBool("pr")
			if pr {
				branches := make([]string, 0, len(trees))
				for _, tree := range trees {
					branches = append(branches, tree.Branch)
				}
				prs, err = github.PRNumbers(cmd.Context(), s.Repo.Source, branches)
				if err != nil {
					return err
				}
			}
			entries := listEntries(trees, prs)
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(entries)
			}
			return writeListTable(cmd.OutOrStdout(), entries, len(prs) > 0)
		}}
	registerBoolPair(cmd, "pr", true, "Look up GitHub PR numbers", "Skip GitHub PR lookup and hide the PR column")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Print a JSON array for structured processing")
	cmd.Flags().BoolVar(&table, "table", false, "Print a text table for display or paging")
	cmd.MarkFlagsMutuallyExclusive("json", "table")
	return cmd
}

func listEntries(trees []git.Worktree, prs map[string]int) []listEntry {
	entries := make([]listEntry, 0, len(trees))
	for _, tree := range trees {
		entry := listEntry{Path: tree.Path, Head: tree.Head, Notes: []string{}}
		if tree.Branch == "" {
			entry.Notes = append(entry.Notes, "No branch checked out")
		} else {
			entry.Branch = &tree.Branch
		}
		if tree.Locked != "" {
			entry.Notes = append(entry.Notes, "Locked against removal")
		}
		if tree.Prunable != "" {
			entry.Notes = append(entry.Notes, "Stale worktree registration")
		}
		if number := prs[tree.Branch]; number > 0 {
			entry.PR = &number
		}
		entries = append(entries, entry)
	}
	return entries
}

func writeListTable(out io.Writer, entries []listEntry, showPR bool) error {
	var table bytes.Buffer
	w := tabwriter.NewWriter(&table, 0, 4, 2, ' ', 0)
	if showPR {
		fmt.Fprintln(w, "BRANCH\tPATH\tPR\tHEAD\tNOTES")
	} else {
		fmt.Fprintln(w, "BRANCH\tPATH\tHEAD\tNOTES")
	}
	for _, entry := range entries {
		branch := "(detached)"
		if entry.Branch != nil {
			branch = *entry.Branch
		}
		notes := strings.Join(entry.Notes, "; ")
		if notes == "" {
			notes = "—"
		}
		fmt.Fprintf(w, "%s\t%s\t", branch, pathdisplay.Shorten(entry.Path))
		if showPR {
			pr := ""
			if entry.PR != nil {
				pr = fmt.Sprintf("#%d", *entry.PR)
			}
			fmt.Fprintf(w, "%s\t", pr)
		}
		fmt.Fprintf(w, "%s\t%s\n", entry.Head, notes)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := io.Copy(out, &table)
	return err
}
