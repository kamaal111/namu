package cli

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

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
	selected, err := selectedIntegration(cmd)
	if err != nil {
		return nil, err
	}
	s.Integration, s.Fallback = selected, shellIntegrationHooks()
	if cmd.Flags().Lookup("focus") != nil {
		focus, _ := cmd.Flags().GetBool("focus")
		s.NoFocus = !focus
	}
	if err := configureVM(cmd, s); err != nil {
		return nil, err
	}
	return s, nil
}
func New(dir string) *cobra.Command {
	root := &cobra.Command{Use: "namu", Short: "Create, open, list and delete Git worktrees", Version: version(), Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true}
	var base, remote string
	create := &cobra.Command{Use: "create BRANCH", Short: "Create a new branch and worktree, then print its path", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) (result error) {
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, s.Close()) }()
		return s.Create(cmd.Context(), args[0], base, remote)
	}}
	create.Flags().StringVar(&base, "base", "", "Exact local base ref; skips fetching")
	create.Flags().StringVar(&remote, "remote", "", "Remote to fetch for its default branch")
	create.MarkFlagsMutuallyExclusive("base", "remote")
	create.ValidArgsFunction = cobra.NoFileCompletions
	_ = create.RegisterFlagCompletionFunc("base", completeRepository(dir, "base"))
	_ = create.RegisterFlagCompletionFunc("remote", completeRepository(dir, "remote"))
	open := &cobra.Command{Use: "open [BRANCH]", Short: "Create or reuse a branch checkout and print its path", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) (result error) {
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, s.Close()) }()
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
	list := listCommand(dir)
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
	setup := &cobra.Command{Use: "setup BRANCH", Short: "Retry copying and setup from the original source checkout", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) (result error) {
		s, err := service(cmd, dir)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, s.Close()) }()
		return s.Setup(cmd.Context(), args[0])
	}}
	setup.PreRunE = func(cmd *cobra.Command, _ []string) error { return applyProjectDefaults(cmd, dir) }
	for _, cmd := range []*cobra.Command{create, open} {
		cmd.PreRunE = func(cmd *cobra.Command, args []string) error {
			return prepareNavigation(cmd, dir, args)
		}
		registerIntegrations(cmd)
		registerBoolPair(cmd, "focus", true, "Focus the workspace and change directory with shell integration", "Keep current workspace and shell directory")
	}
	for _, cmd := range []*cobra.Command{create, open, setup} {
		registerVM(cmd)
	}
	open.ValidArgsFunction = completeRepository(dir, "open")
	remove.ValidArgsFunction = completeRepository(dir, "delete")
	setup.ValidArgsFunction = completeRepository(dir, "setup")
	list.ValidArgsFunction = cobra.NoFileCompletions
	root.AddCommand(create, open, list, remove, setup, shellInit(), vmCommand(dir))
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
