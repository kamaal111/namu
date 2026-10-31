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
	if cmd.Flags().Lookup("herdr") != nil {
		enabled, _ := cmd.Flags().GetBool("herdr")
		if !cmd.Flags().Changed("herdr") {
			enabled = os.Getenv("HERDR_ENV") == "1"
		}
		focus, _ := cmd.Flags().GetBool("focus")
		noFocus, _ := cmd.Flags().GetBool("no-focus")
		s.Herdr, s.NoFocus = enabled, !focus || noFocus
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
		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "BRANCH\tPATH\tHEAD\tSTATE")
		for _, tree := range trees {
			branch := tree.Branch
			state := ""
			if branch == "" {
				branch = "(detached)"
				state = "detached"
			}
			if tree.Locked != "" {
				state += " locked"
			}
			if tree.Prunable != "" {
				state += " prunable"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", branch, pathdisplay.Shorten(tree.Path), tree.Head, state)
		}
		return w.Flush()
	}}
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
		cmd.Flags().Bool("herdr", false, "Use Herdr (automatic inside Herdr; --herdr=false disables)")
		cmd.Flags().Bool("focus", true, "Focus the workspace and change directory with shell integration")
		cmd.Flags().Bool("no-focus", false, "Keep current workspace and shell directory")
		cmd.MarkFlagsMutuallyExclusive("focus", "no-focus")
	}
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
