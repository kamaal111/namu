# namu

A Go CLI to create, open, list and delete Git worktrees from any project.
Git is the source of truth; no daemon, database, or project configuration is required.

## Install

Requires Git on `PATH`. Supported release targets are macOS and Linux, ARM64 and AMD64.

Once the first release and tap formula are published:

```sh
brew install kamaal111/tap/namu
```

Or install with Go 1.27.3 or later after this repository is published:

```sh
go install github.com/kamaal111/namu/cmd/namu@latest
```

Release archives contain a single executable. Verify the archive against
`checksums.txt`, extract it, and install the executable in a directory on your
`PATH`.

For contributor setup and source builds, see the [development docs](docs/development.md).

## Worktrees

Run commands from anywhere inside a normal Git checkout, including a linked worktree.

```sh
namu create feature/cards
namu create feature/pricing --base feature/cards
namu create feature/release --remote upstream
namu open feature/cards
namu open origin/feature/search
namu open
namu list
namu delete feature/cards
namu delete feature/pricing feature/release
```

- `create BRANCH` creates a **new** branch. It fetches `origin` (or the sole remote)
  and branches from that remote's advertised default branch. Multiple remotes without
  `origin` require `--remote NAME`. Offline work uses `--base REF`, which resolves a
  local branch, remote-tracking ref, tag, or commit without fetching. `--base` and
  `--remote` cannot be combined.
- New checkouts live under the **main checkout's** `.agents/worktrees/BRANCH`.
  Slashes are preserved: `feature/cards` becomes `.agents/worktrees/feature/cards`.
  Namu adds this directory to the shared local Git excludes, leaving tracked files
  unchanged. Existing destination paths and symlink components are refused.
- `open BRANCH` opens an existing local or known remote branch. It prints the path
  of any registered checkout already using the branch, including external checkouts
  and the main checkout. Otherwise it creates a managed checkout. A remote-only
  branch creates a local tracking branch; local branches take precedence. Explicit
  arguments never fetch. Ambiguous remote-only names require a remote-qualified name.
- `open` without an argument fetches configured remotes and shows a fuzzy branch
  picker. Failed fetches warn and preserve known refs. Use arrows or Ctrl+P/Ctrl+N
  to move, Enter to select, Escape to leave, or Ctrl+C to cancel. Symbolic remote HEAD
  refs and `gitbutler/workspace` are excluded. Input and stderr must be terminals;
  stdout can be redirected.
- `list` shows every registered worktree, its branch, home-abbreviated path, commit, and
  a `NOTES` column. Notes explain unusual conditions: `No branch checked out`,
  `Locked against removal`, or `Stale worktree registration`. Multiple notes are
  separated by semicolons; `—` means none of these conditions apply, not that the
  checkout has no uncommitted changes. It does not fetch or prune anything.
- `delete BRANCH [BRANCH...]` removes matching registered linked checkouts at any
  location, including checkouts created outside Namu, and **retains their branches**. Each unique branch is attempted in argument order;
  failures are reported together and make the command exit 1, while remaining
  branches are still attempted. Dirty tracked files, untracked files, locks,
  prunable registrations, the main checkout, and the invoking checkout prevent
  removal. Multiple registered checkouts using the same branch are rejected as
  ambiguous. Ignored files are removed with the checkout. Stop checkout-specific
  services yourself before deleting it.

Bare repositories are unsupported. Create an initial commit before creating/opening
branches. Git locks and conflicts are surfaced without force, branch deletion,
checkout replacement, or automatic rollback.

## Herdr integration

Inside a Herdr pane (`HERDR_ENV=1`), `create` and `open` automatically use Herdr
and focus the checkout's workspace. The `herdr` executable must be on `PATH` and
able to reach its server. Namu inherits Herdr's session and socket environment.
Force or disable this integration explicitly:

```sh
namu create feature/cards --herdr
namu open feature/cards --herdr=false
namu open feature/cards --no-focus
```

Herdr creates checkouts at Namu's usual `.agents/worktrees/BRANCH` path and opens
existing checkouts as workspaces. `--focus` is the default; `--no-focus` preserves
Herdr's current workspace. These flags are mutually exclusive and also control
shell navigation when the integration below is installed. `--focus=false` and
`--no-focus=false` are supported.

If Herdr is unavailable or fails, Namu warns on stderr and falls back to Git.
If Herdr already created the requested checkout or branch, Namu verifies its path,
branch, and commit before reusing it. Conflicting partial state is preserved and
reported for inspection. Cancellation stops the operation. Setup failures preserve
the checkout and fail normally; they never trigger a second creation attempt.
Opening an existing checkout never reruns setup, even when Herdr fails.

Herdr documents detection and worktree commands in its
[CLI reference](https://herdr.chefgroep.nl/docs/cli-reference/).

## Paths and shell use

`create` and `open` write exactly one checkout path and a newline to stdout as soon
as Git creates or identifies the checkout, before file copying or setup. Paths
beneath your home directory use `~` (for example, `~/Projects/namu`); paths outside
home remain absolute. This applies to listing, status messages, and Namu's own
path fields in errors. Underlying tool output and error details are unchanged.
Scripts consuming checkout output must expand a leading `~` before using it.
All picker, setup, progress, and error output goes to stderr.

On your first interactive `create` or `open` with focus enabled, Namu offers to
install shell integration so future commands enter the checkout automatically.
It shows each startup file's home-abbreviated path and the exact text it will append,
then asks for confirmation (default: No). After installation it reports each
changed file and the exact text added. Existing configuration is preserved;
repeated installation does not add duplicate initialization.

Zsh uses `$ZDOTDIR/.zshrc` or `~/.zshrc`. Bash uses `~/.bashrc` and the first
existing login file among `~/.bash_profile`, `~/.bash_login`, and `~/.profile`,
creating `~/.bash_profile` if none exists. Symlinked startup files are updated
at their resolved destinations, which are shown in the preview and report.
Declining leaves startup files untouched and continues the worktree command.
Scripts, redirected commands, unsupported shells, and `--no-focus` never trigger
automatic installation.

After updating Namu, reload existing shell sessions to load the updated integration.
Reload once with `exec zsh` or `exec bash`, as instructed after installation.
The installing invocation cannot change its parent shell's directory. You can
also install explicitly; these commands authorize the displayed file edits:

```sh
namu shell-init zsh --install
namu shell-init bash --install
```

For a temporary setup without editing startup files, load the integration in your
current shell:

```sh
# Zsh (~/.zshrc)
eval "$(namu shell-init zsh)"
# Bash (~/.bashrc)
eval "$(namu shell-init bash)"
```

The wrapper prints the checkout path and changes directory after setup succeeds
when using Git. When Herdr successfully handles navigation, the invoking shell
stays in its current directory. If Herdr fails and Namu safely recovers, shell
navigation is used instead. Automatic shell installation is not offered while
Herdr is selected. Use `--no-focus` to keep your shell directory. Failures and
picker cancellation never change directory. The wrapper buffers stdout until the
command finishes, while diagnostics remain on stderr. Other commands pass through.
Use `command namu` to bypass the wrapper in scripts or command substitutions.

Without shell integration, the executable cannot change its parent shell's
directory. Use the exit status when composing commands, because setup can fail
**after** a path has been printed:

```sh
if checkout=$(namu open feature/cards); then
  case "$checkout" in
    '~') checkout=$HOME ;;
    '~/'*) checkout="$HOME/${checkout#\~/}" ;;
  esac
  cd -- "$checkout"
fi
```

Successful commands and Escape cancellation exit 0; errors exit 1; Ctrl+C exits 130.
Escape cancellation prints no path. Repository paths containing newlines can be
listed, but newline-terminated path output is intended for ordinary checkout paths.

## Optional project setup

Add `.namu.toml` to the checkout from which you invoke Namu:

```toml
[setup]
command = ["just", "setup-worktree"]
```

The command is an argv array, executed directly in the new checkout with the inherited
environment. Shell expressions require an explicit shell command, such as
`["sh", "-c", "pnpm install && pnpm build"]`. Setup receives no interactive stdin.
Missing configuration means no hook; unknown keys or invalid TOML fail before creation.
The hook's executable and supporting files must be present in the selected base.

To copy ignored local files before the hook runs, add `.worktreeinclude`:

```gitignore
# Independent copies of local development configuration
.env*
!.env.production
config/secrets.json
```

Rules support gitignore-style globs, anchors, comments, directory patterns, and `!`
exclusions, matching Herdr's opt-in workflow. Only matching **gitignored, untracked,
regular files** from the invoking checkout are copied, even when that checkout is
linked. Files inside wholly ignored directories can match. File permissions are
preserved, and newly created directories are private. Existing destination entries
and destination tracked paths are preserved, including deleted tracked files.
Symlinks are never followed; special files, nested repositories, and Git metadata
are excluded. Missing or empty rules do nothing.

Copy failures preserve successful copies, skip the hook, and fail the command.
Hook failures also fail the command. In either case the branch and worktree remain
available, and the path has already been printed. Fix the issue and retry from the
original source checkout:

```sh
namu setup feature/cards
```

An explicit retry reruns copying and the hook. Hooks should therefore tolerate
repeated execution. Opening an existing checkout does not copy or run setup again.
Project-specific ports, environment rewriting, dependency installation, and services
belong in the project hook; Namu does not implement them itself.
