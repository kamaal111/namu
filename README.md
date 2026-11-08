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
  With an authenticated GitHub CLI (`gh`), it also shows a `PR` column between
  `PATH` and `HEAD` when at least one branch's PR number can be determined.
  Open PRs (including drafts) take precedence; otherwise the most recently
  updated merged or closed PR is shown. Unresolved and detached rows are blank.
  Missing `gh`, failed authentication, or lookup failures silently omit PR data;
  the column is hidden when no numbers resolve. PR lookup is limited to ten seconds.
  Use `namu list --no-pr` or `namu list --pr=false` to skip GitHub requests and
  hide the `PR` column. The last PR flag wins: `namu list --no-pr --pr` enables
  lookup, and `--no-pr=false` also enables it.
  Use `namu list --table | less` for a readable paged table, or
  `namu list --json | jq -r '.[].branch'` for JSON processing. Both formats
  work in every supported shell. `--table` and `--json` are mutually exclusive;
  piping or redirecting alone does not change the executable's text output.
  JSON is an array of records with `branch` (string or null for detached HEAD),
  absolute `path`, full `head`, `pr` (integer or null), and `notes` (string array).
  Disabled or unavailable PR lookup produces null PRs; ordinary rows have empty
  notes arrays. Record order matches the text table.
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
Herdr's current workspace. These flags also control shell navigation when the
integration below is installed. The last flag wins: `--no-focus --focus` enables
focus, and `--focus --no-focus` disables it. `--focus=false` disables focus;
`--no-focus=false` enables it. This lets wrappers supply defaults that later
arguments can override.

If Herdr is unavailable or fails, Namu warns on stderr and falls back to Git.
If Herdr already created the requested checkout or branch, Namu verifies its path,
branch, and commit before reusing it. Conflicting partial state is preserved and
reported for inspection. Cancellation stops the operation. Setup failures preserve
the checkout and fail normally; they never trigger a second creation attempt.
Opening an existing checkout never reruns setup, even when Herdr fails.

Herdr documents detection and worktree commands in its
[CLI reference](https://herdr.chefgroep.nl/docs/cli-reference/).

## Rex integration

Inside a Rex terminal (`REX_SESSION` is nonempty), `create` and `open`
automatically create or reuse a Rex session named after the branch. New
sessions start an interactive shell in the checkout directory after setup
succeeds. The `rex` executable must be on `PATH`.

> **Before using Rex integration:** In **Rex Settings → Rex Server**, enable
> **Remote Control**. This lets Namu switch the app to the worktree's session.

```sh
namu create feature/cards --rex
namu open feature/cards --no-focus
namu open feature/cards --rex=false
```

Focus is enabled by default. Namu honors `REX_CLIENT`; otherwise it selects
the connected app owning the current Rex pane, falling back to Rex's default
single-app selection. Enable Remote Control as described above for app focus.
`--no-focus` and `--focus=false` create or reuse the session in the background.
A reused terminal retains its current directory and running processes.

Session IDs are stored under the common Git directory's `namu/rex/` directory,
keyed by worktree path and Rex host ID. Repeated opens reuse mapped live
sessions even after renaming; closed sessions are recreated. Unmapped sessions
are not adopted by label. Different repositories and hosts remain separate.

Rex failures warn and use existing shell fallback without removing the
checkout. A focus failure retains the mapping for the next open. Session
creation is never automatically retried after an uncertain failure; inspect
Rex before retrying. State errors and lock contention also select fallback.
After an interrupted process leaves a stale `.lock` file, remove that file
only once the associated operation has stopped. Branch names longer than
Rex's 128-character label limit select fallback. If both Herdr and Rex are
detected, disable one explicitly with `--herdr=false` or `--rex=false`.

### Troubleshooting

If Namu reports `Rex.app is not accepting actions`, enable **Remote Control**
in **Rex Settings → Rex Server**. The Git checkout and Rex session can already
exist even though switching to the session failed. After enabling the setting,
open the existing checkout again:

```sh
namu open failing-to-open
```

Namu reuses the saved live Rex session. To intentionally create or reuse the
session in the background without displaying it, use:

```sh
namu open failing-to-open --no-focus
```

## Paths and shell use

Bash, Zsh, and Nushell support directory navigation and command, flag, and branch
completion through `shell-init`. For Nushell 0.116.1 or newer, install with:

```nu
namu shell-init nu --install
```

Restart Nu after installation. The installer previews and writes a managed
`namu.nu` in Nu's user autoload directory, respecting Nu's platform-specific
configuration paths and `XDG_CONFIG_HOME`. It preserves unrelated configuration
and refuses to replace an unmanaged file. Run the installer again after updating
Namu to refresh the script. `namu shell-init nu` prints the sourceable script
without installing it; save it to a file and `source` that file in a later command
if managing your own configuration.

With the integration loaded, plain `namu list` returns native Nu records:

```nu
namu list | where pr != null | select branch pr
namu list | sort-by branch | table | less -R
namu list | explore
```

Explicit formats return text, so external tools work the same way in every shell:

```nu
namu list --table | less
namu list --json | jq -r '.[].branch'
```

Use `^namu` to bypass the Nu wrapper. Create/open navigate only when Namu reports
a successful shell-navigation result; `--no-focus` keeps the current directory.

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
Shell initialization loads both directory navigation and command, flag, and branch
completion. Existing installations gain completion after updating Namu and
reloading their shell; no additional startup entry or reinstallation is needed.
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

Completion uses local and known remote branches without fetching. `open` suggests
branches, `delete` suggests registered checkout branches, and `setup` suggests
managed checkouts. `create --base` suggests branches and `create --remote`
suggests remote names; the new branch name itself has no suggestions.

For completion without directory navigation, load the standalone script instead:

```sh
# Zsh (initialize completion first if your shell does not already do so)
autoload -Uz compinit; compinit -i
source <(namu completion zsh)
# Bash
source <(namu completion bash)
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

## Project defaults

Add `.namu.toml` at the root of the checkout from which you invoke Namu:

```toml
[defaults]
focus = false
pr = false
vm = false
reuse_credentials = true

[setup]
command = ["just", "setup-worktree"]
```

`defaults.focus` controls workspace focus and shell navigation for `create` and
`open`. Setting it to `false` also skips automatic shell-install prompting.
Explicit `--focus` or `--no-focus=false` enables focus for an invocation;
`--no-focus` or `--focus=false` disables it. Without this setting, focus is enabled.
When multiple focus flags are supplied, the last one wins.
`defaults.pr = false` skips GitHub lookup and hides the PR column in `list`.
Explicit `--pr` or `--no-pr=false` enables lookup; `--no-pr` or `--pr=false`
disables it. Without this setting, PR lookup is enabled. The last supplied PR
flag wins. Invalid configuration fails before lookup or navigation.

`defaults.vm = true` selects VM execution for `create`, `open`, and `setup`.
Without this setting, worktrees use the host. Override it with `--vm`,
`--vm=false`, or `--no-vm`; the last VM flag wins. Setting
`defaults.reuse_credentials = false` selects guest-only authentication in VM mode.
`--reuse-credentials` overrides that setting; `--reuse-credentials=false` and
`--no-reuse-credentials` are equivalent opt-outs. These flags also use last-wins
precedence. Credential defaults never enable VM execution by themselves.

Namu reads the invoking checkout's root configuration even from a subdirectory
or linked worktree. Rex and Herdr selection still uses flags and terminal detection;
base and remote selection still uses flags. These settings cannot be configured.

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

## Optional VM worktrees

Pass `--vm` to run setup and open a guest shell in a persistent Tart VM on an
Apple Silicon Mac, or set `[defaults] vm = true` in `.namu.toml`. Without an
explicit flag or configured VM default, worktrees use the host.
No separate `agent-vm` installation is needed.

```sh
namu create feature/cards --vm
namu open feature/cards --vm
namu open feature/cards --vm --os linux
namu open feature/cards --vm --no-focus
namu setup feature/cards --vm --vm-name namu-linux
```

The default is **macOS 27**, using the shared `namu-macos` VM. When that default
has not been created, interactive first use offers macOS or Ubuntu 24.04 Linux
before asking `[y/N]` to create it. `--os linux` selects the shared `namu-linux`
VM directly. `--os` overrides the project's `vm.os` setting for that invocation.
Without either, macOS remains the default.

Creation requires confirmation, including any private Tart download, the pinned
upstream image, dedicated SSH access, Git, mounting prerequisites, and the Claude,
Codex, and Cursor CLIs. Default resources are 4 CPUs, 8 GiB RAM, and an 80 GB
macOS or 60 GB Linux disk. Downloads
can be several GB. Linux guests require macOS 14+; macOS 27 guests require a
macOS 27+ host. Compatible Apple Silicon hardware, system OpenSSH 10.3+ within
major version 10, sufficient disk space/RAM, and network access are required.
A compatible Tart 2.41.1+ within major version 2 is reused; otherwise approved
setup downloads a checksum-verified private copy. Existing VM images are retained
when embedded image defaults change.

Codex and Claude Code use their official native installers, which select the
current release at installation time and retain their native update behavior.
Namu does not provision a separate Node.js/npm runtime for agent installation.
Cursor uses a versioned, checksum-verified archive. Credential launchers live
separately from the native binaries so native updates do not overwrite them.
VM sessions put these launchers first on `PATH`.

Configure VM selection and resources in the invoking checkout's `.namu.toml`:

```toml
[defaults]
vm = true

[vm]
os = "linux"       # macos or linux
# name = "sandbox" # optional existing namu-owned VM, instead of the shared default
cpus = 2
memory_mb = 4096   # MiB
disk_gb = 90       # capacity, not physical host storage used
```

Override individual settings with flags:

```sh
namu create feature/cards --vm --vm-cpus 6 --vm-memory 12288 --vm-disk 100
namu vm create sandbox --os linux --vm-cpus 2 --vm-memory 4096 --vm-disk 90
namu vm start sandbox --vm-memory 8192
```

Resource flags are available on `create`, `open`, `setup`, and `vm create/start/shell`.
`--os` and `--vm-name` override `vm.os` and `vm.name` on worktree commands;
`vm create NAME` also honors `vm.os`, and explicit management command names
always select the named VM. `[vm]` settings alone never enable VM execution;
worktree commands still require `--vm` or `defaults.vm = true`.

Flags take precedence over project settings. Omitted resource settings preserve
an existing VM's values; new VMs use the defaults above. Approved resource changes
persist on the shared VM and affect every project using it. Changing resources
requires `[y/N]` confirmation, including a restart when the VM is running, which
stops all guest processes. Choosing a separate named VM avoids changing another
project's shared resources.

Disk capacity can only grow. Requests below an existing VM's recorded capacity
fail before stopping it. A new VM also cannot shrink its cloned base image, so
Tart rejects a requested capacity below that image's capacity. Lowering `disk_gb`
does not reclaim host storage. Invalid settings fail before worktree creation.

The guest shares the **main repository directory**, including its worktrees and
Git metadata, read/write. External worktrees and external common Git directories
are shared separately when needed. Every process in that VM can access these
registered shares. Host and guest edits affect the same files immediately; there
is no snapshot or export step. Host absolute paths are preserved inside the guest
so linked-worktree Git references work without rewriting host metadata. Colons,
commas, and newlines in share roots are unsupported. Conflicting guest mount paths
are rejected rather than hiding existing guest files.

Each OS's default VM is reused across worktrees and repositories. Sharing another
worktree beneath an already shared main checkout needs no restart. Adding a new
repository share to a running VM requires a restart, with a separate `[y/N]`
confirmation warning that **all guest processes will stop**. Declining preserves
the running VM and its existing shares. Use a separate named VM when needed:

```sh
namu vm create sandbox --os linux
namu open feature/cards --vm --vm-name sandbox
namu vm list
namu vm shell sandbox
namu vm stop sandbox
namu vm start sandbox
namu vm delete sandbox
```

Management commands work outside repositories. Named selection requires an
existing namu-owned VM; unrelated Tart VMs are never adopted automatically.
Deletion requires a stopped VM and confirmation, destroys guest files and installed
tools, and retains host repositories. Removing a worktree with `namu delete`
does not delete or stop its shared VM. Stop guest services before removing a
worktree. Do not rename, replace, or delete owned VMs behind namu's registry.

With VM execution enabled, included ignored files are copied on the host, then
`.namu.toml`'s setup argv runs **inside the guest** with no stdin. Project setup
can use the installed `claude`, `codex`, and Cursor `agent` / `cursor-agent`
commands. Opening an existing checkout does not rerun setup; use an explicit
`namu setup BRANCH --vm --vm-name NAME` retry from the source checkout. Use
`--vm=false` to force a host retry when project configuration enables VMs.

Host credential reuse is enabled by default for VM setup and shells:

```sh
namu open feature/cards --vm                       # reuse supported host logins
namu open feature/cards --vm --no-reuse-credentials # use guest-only logins
namu open feature/cards --vm --reuse-credentials=false # equivalent opt-out
namu vm shell sandbox --no-reuse-credentials
```

Supported sources are the explicit agent environment variables (`OPENAI_API_KEY`,
`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`,
`CURSOR_API_KEY`, and `CURSOR_AUTH_TOKEN`), Codex's file cache or standard macOS
Keychain entry, Claude's `.credentials.json` or default macOS Keychain entry,
and Cursor CLI's auth file or macOS Keychain. `CODEX_HOME`, `CLAUDE_CONFIG_DIR`,
and Cursor's Linux `XDG_CONFIG_HOME` paths are honored. Custom Claude Keychain
names and other unsupported credential backends require guest login. Cursor IDE
state, entire home directories, and arbitrary environment variables are not copied.

Codex and Claude login caches seed a separate guest profile under
`~/.namu-vm/host-auth/` once. Existing guest logins and refreshed imported caches
are preserved; namu never copies refreshed credentials back to the host. OAuth
refresh-token sharing across machines can require reauthentication; cache reuse
is not a guarantee that every vendor account remains signed in. Cursor's host
access token or API key is supplied to its CLI through the session environment.
Missing or inaccessible host logins require native login inside the guest using
`namu vm shell NAME --no-reuse-credentials`. On headless macOS, if Cursor reports
that the guest Keychain is locked, use
`AGENT_CLI_CREDENTIAL_STORE=file NO_OPEN_BROWSER=1 agent login` and retain that
credential-store setting for subsequent Cursor commands.

Agent-specific environment credentials travel over SSH stdin into private session
files and are loaded only by the corresponding agent wrapper. They are not put in
command arguments, diagnostics, or the host registry. Normal command completion
removes transient session files; abrupt termination can leave private files in
`~/.namu-vm/credentials/` for manual cleanup. Opting out skips host credential reads,
selects the guest's own native profile, and disables SSH agent forwarding. It does
not erase previously imported caches or isolate processes sharing the guest user.

For Git, reuse forwards the host SSH agent when available, without copying private
SSH keys. GitHub HTTPS uses `GH_TOKEN`, `GITHUB_TOKEN`, or the authenticated host
GitHub CLI, scoped strictly to `https://github.com`. The invoking repository's Git
author name and email are applied to the session. Other HTTPS credential helpers,
SSH hosts without available agent keys, and commit signing need guest setup.
Host-side Git commands continue to use the host's own configuration and credentials.

VM mode opens SSH in the current terminal and bypasses automatic Rex/Herdr
detection. Explicitly combining `--vm` with `--rex` or `--herdr` is rejected.
The parent shell never changes directory. Stdout remains the single host checkout
path; guest shell output, setup diagnostics, and prompts use stderr, including
when the shell wrapper captures stdout. `--no-focus` prepares and verifies the
VM worktree without attaching. Noninteractive use requires `--no-focus` for
create/open and an already prepared VM; creation, restart, and deletion still
require interactive confirmation. There is no `--yes` bypass.

Failures never fall back to host execution. Created worktrees and owned VMs are
preserved for inspection and retry. Partial VM setup can be retried with
`namu vm start NAME`; `namu vm list` reports incomplete stages or missing disks.
A missing previously completed VM is not silently replaced. Restore missing
registered share directories before starting a stopped VM. Persistent VM registry,
SSH identity, trust, and logs live under `~/.local/share/namu/vm/`, separate from
agent-vm state. Configuration defaults affect only the invoking project.

Opt-in live verification uses `just vm-smoke NAME` against a prepared, stopped
namu VM. It
creates a temporary repository, verifies guest setup, shared files and Git commits,
checks interactive Bash/Zsh guest shells, then stops the VM and restores its
previous share registration. Live checks use `--no-reuse-credentials` and synthetic
authentication fixtures; they never inspect personal logins. Normal tests never
boot VMs or download images.

Guest operating systems follow the integration adapter pattern. The shared
contract lives in `internal/vm/platform`, with implementations and embedded
OS-specific scripts in its `macos` and `linux` packages. The explicit `registry`
package supplies selection, defaults, validation, and CLI choices. To add a guest,
implement its definition and hooks, register it, and supply its image/download
pins. Keep common lifecycle, transport, and security checks shared. Both guest
adapters compile on every supported host; OS-specific tests and test-only embedded
fixtures live beside their adapter. Host credential-store adapters are separate.
