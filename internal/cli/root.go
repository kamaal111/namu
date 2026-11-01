package cli

import (
	"errors"
	"fmt"
	"github.com/kamaal111/namu/internal/pathdisplay"
	"os"
	"runtime/debug"
	"strings"
	"text/tabwriter"

	git "github.com/kamaal111/namu/internal/git"
	"github.com/kamaal111/namu/internal/github"
	"github.com/kamaal111/namu/internal/ui"
	"github.com/kamaal111/namu/internal/worktree"
	"github.com/spf13/cobra"
)

// Version is set by release builds.
var Version = "0.1.0-dev"

func service(cmd *cobra.Command, dir string) (*worktree.Service, error) {
	repo, err := git.Discover(cmd.Context(), dir)
	if err != nil {
		return nil, err
	}
	s := &worktree.Service{Repo: repo, Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()}
	selected, err := selectedIntegration(cmd)
	if err != nil {
		return nil, err
	}
	s.Integration, s.Fallback = selected, shellIntegrationHooks()
	if cmd.Flags().Lookup("focus") != nil {
		focus, _ := cmd.Flags().GetBool("focus")
		s.NoFocus = !focus
	}
	return s, nil
}
func New(dir string) *cobra.Command {
	root := &cobra.Command{Use: "namu", Short: "Create, open, list and delete Git worktrees", Version: version(), Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true}
	var base, remote string
	create := &cobra.Command{Use: "create BRANCH", Short: "Create a new branch and worktree, then print its path", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("base") && base == "" {
			return fmt.Errorf("--base cannot be empty")
		}
		if cmd.Flags().Changed("remote") && remote == "" {
			return fmt.Errorf("--remote cannot be empty")
		}
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		return s.Create(cmd.Context(), args[0], base, remote)
	}}
	create.Flags().StringVar(&base, "base", "", "Exact local base ref; skips fetching")
	create.Flags().StringVar(&remote, "remote", "", "Remote to fetch for its default branch")
	create.MarkFlagsMutuallyExclusive("base", "remote")
	create.ValidArgsFunction = cobra.NoFileCompletions
	_ = create.RegisterFlagCompletionFunc("base", completeRepository(dir, "base"))
	_ = create.RegisterFlagCompletionFunc("remote", completeRepository(dir, "remote"))
	open := &cobra.Command{Use: "open [BRANCH]", Short: "Create or reuse a branch checkout and print its path", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 && !interactive(cmd) {
			return fmt.Errorf("interactive terminal required; supply a branch: namu open feature/cards")
		}
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		if len(args) == 1 {
			return s.Open(cmd.Context(), args[0])
		}
		remotes, err := git.Remotes(cmd.Context(), s.Repo.Source)
		if err != nil {
			return err
		}
		for _, r := range remotes {
			if err := git.Fetch(cmd.Context(), s.Repo.Source, r); err != nil {
				if cmd.Context().Err() != nil {
					return cmd.Context().Err()
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: fetch %s failed; using known refs: %v\n", r, err)
			}
		}
		branches, err := git.Branches(cmd.Context(), s.Repo.Source)
		if err != nil {
			return err
		}
		selected, err := ui.Pick(cmd.Context(), branches, cmd.InOrStdin(), cmd.ErrOrStderr())
		if err != nil {
			return err
		}
		if selected == "" {
			return nil
		}
		return s.Open(cmd.Context(), selected)
	}}
	list := &cobra.Command{Use: "list", Short: "List all registered checkouts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
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
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		if len(prs) > 0 {
			fmt.Fprintln(w, "BRANCH\tPATH\tPR\tHEAD\tNOTES")
		} else {
			fmt.Fprintln(w, "BRANCH\tPATH\tHEAD\tNOTES")
		}
		for _, tree := range trees {
			branch := tree.Branch
			var notes []string
			if branch == "" {
				branch = "(detached)"
				notes = append(notes, "No branch checked out")
			}
			if tree.Locked != "" {
				notes = append(notes, "Locked against removal")
			}
			if tree.Prunable != "" {
				notes = append(notes, "Stale worktree registration")
			}
			note := strings.Join(notes, "; ")
			if note == "" {
				note = "—"
			}
			fmt.Fprintf(w, "%s\t%s\t", branch, pathdisplay.Shorten(tree.Path))
			if len(prs) > 0 {
				pr := ""
				if number := prs[tree.Branch]; number > 0 {
					pr = fmt.Sprintf("#%d", number)
				}
				fmt.Fprintf(w, "%s\t", pr)
			}
			fmt.Fprintf(w, "%s\t%s\n", tree.Head, note)
		}
		return w.Flush()
	}}
	registerBoolPair(list, "pr", true, "Look up GitHub PR numbers", "Skip GitHub PR lookup and hide the PR column")
	remove := &cobra.Command{Use: "delete BRANCH [BRANCH...]", Short: "Remove clean registered checkouts, retaining their branches", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		seen := make(map[string]bool)
		var failures []error
		for _, branch := range args {
			if err := cmd.Context().Err(); err != nil {
				return errors.Join(append(failures, err)...)
			}
			if seen[branch] {
				continue
			}
			seen[branch] = true
			if err := s.Delete(cmd.Context(), branch); err != nil {
				failures = append(failures, fmt.Errorf("%s: %w", branch, err))
			}
		}
		return errors.Join(failures...)
	}}
	setup := &cobra.Command{Use: "setup BRANCH", Short: "Retry copying and setup from the original source checkout", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		return s.Setup(cmd.Context(), args[0])
	}}
	for _, cmd := range []*cobra.Command{create, open} {
		cmd.PreRunE = autoShellInstall
		registerIntegrations(cmd)
		registerBoolPair(cmd, "focus", true, "Focus the workspace and change directory with shell integration", "Keep current workspace and shell directory")
	}
	open.ValidArgsFunction = completeRepository(dir, "open")
	remove.ValidArgsFunction = completeRepository(dir, "delete")
	setup.ValidArgsFunction = completeRepository(dir, "setup")
	list.ValidArgsFunction = cobra.NoFileCompletions
	root.AddCommand(create, open, list, remove, setup, shellInit())
	return root
}
func interactive(cmd *cobra.Command) bool {
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return terminalStream(cmd.InOrStdin()) && terminalStream(cmd.ErrOrStderr())
}

func version() string {
	if Version != "0.1.0-dev" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && strings.HasPrefix(info.Main.Version, "v") {
		return strings.TrimPrefix(info.Main.Version, "v")
	}
	return Version
}
