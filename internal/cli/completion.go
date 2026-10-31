package cli

import (
	"path/filepath"
	"sort"
	"strings"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/spf13/cobra"
)

func completeRepository(dir, kind string) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		const directive = cobra.ShellCompDirectiveNoFileComp
		if (kind == "open" || kind == "setup") && len(args) > 0 {
			return nil, directive
		}
		repo, err := git.Discover(cmd.Context(), dir)
		if err != nil {
			return nil, directive
		}
		var candidates []string
		switch kind {
		case "open", "base":
			branches, err := git.Branches(cmd.Context(), repo.Source)
			if err != nil {
				return nil, directive
			}
			for _, branch := range branches {
				candidates = append(candidates, branch.Label())
			}
		case "remote":
			candidates, err = git.Remotes(cmd.Context(), repo.Source)
			if err != nil {
				return nil, directive
			}
		case "delete", "setup":
			trees, err := git.Worktrees(cmd.Context(), repo.Source)
			if err != nil {
				return nil, directive
			}
			for _, tree := range trees {
				if tree.Branch == "" || tree.Branch == "gitbutler/workspace" || tree.Path == repo.Main || tree.Path == repo.Source {
					continue
				}
				if kind == "setup" && (tree.Path != filepath.Join(repo.Main, ".agents", "worktrees", tree.Branch) || tree.Locked != "" || tree.Prunable != "") {
					continue
				}
				candidates = append(candidates, tree.Branch)
			}
		}
		seen := make(map[string]bool)
		if kind == "delete" {
			for _, arg := range args {
				seen[arg] = true
			}
		}
		var result []string
		for _, candidate := range candidates {
			if !seen[candidate] && strings.HasPrefix(candidate, prefix) {
				seen[candidate] = true
				result = append(result, candidate)
			}
		}
		sort.Strings(result)
		return result, directive
	}
}
