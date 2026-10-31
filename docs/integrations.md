# Integration hooks

Integrations implement `integration.Hooks` and register a definition in
`internal/cli/integration.go`. Each definition supplies its flag, help text,
detection function, and hook factory. Explicit flag values override detection;
selecting multiple integrations is an error.

The complete list of operations is `integration.Hooks.Create` and
`integration.Hooks.Open`. Each has its own input type and signature; there are
no action names to pass as strings. Start with an open-only adapter:

```go
func Hooks() integration.Hooks {
    return integration.Hooks{Name: "editor", Open: open}
}

func open(ctx context.Context, request integration.OpenRequest) error {
    cmd := exec.CommandContext(ctx, "editor", "--path", request.Path,
        fmt.Sprintf("--focus=%t", request.Focus))
    return cmd.Run()
}
```

Namu creates with Git, prints the checkout path, runs setup, and then calls
`Open`. Opening an existing checkout calls only `Open`, without copying files
or running setup.
An open hook returns nil when it has fulfilled the requested focus behavior;
`Focus: false` means open without switching the current workspace. Errors warn
and select shell fallback; cancellation stops the operation.

For a technology with separate creation and opening commands, add `Create`:

```go
func Hooks() integration.Hooks {
    return integration.Hooks{Name: "editor", Create: create, Open: open}
}

func create(ctx context.Context, request integration.CreateRequest) (integration.CreateResult, error) {
    cmd := exec.CommandContext(ctx, "editor", "create",
        "--path", request.Worktree.Path, "--branch", request.Worktree.Branch,
        "--base", request.Revision)
    return integration.CreateResult{}, cmd.Run()
}
```

These examples illustrate tools that accept the shown flags. Implement their
actual CLI arguments in the adapter, including existing-branch creation where
needed (`request.Worktree.CreateBranch` is false in that case). Route command
diagnostics to stderr or capture them in returned errors; stdout belongs to
Namu's checkout path. Herdr demonstrates captured output and timeouts.

The zero `CreateResult` means opening remains necessary: after verifying the
checkout, configuring tracking, printing the path, and running setup, Namu calls
`Open` once. A missing `Create` uses Git. A missing `Open` selects shell fallback
when opening remains necessary. Register the adapter in the `integrations`
slice with its flag, help text, detection function, and hook factory; shared
worktree code needs no technology-specific changes.

If one command both creates and opens, return
`integration.CreateResult{OpenHandled: true}` to skip the second open. This
means opening with the requested focus preference is complete, including when
focus is disabled. Herdr uses this result because `worktree create` already
opens its workspace. It still supports the separate `Open` hook for existing
checkouts.

Namu verifies actual Git state regardless of a creation hook's reported
success. On failure or missing checkout registration, it performs safe Git
recovery and selects shell fallback without retrying the integration's open
command. Completion claims are ignored during recovery. Conflicting state is
preserved and reported. Setup failures and cancellation prevent subsequent open
and shell hooks. A combined creation/open command may already have opened its
workspace before setup fails; Namu does not undo that external side effect.

Namu owns path validation, verification, Git fallback, remote tracking, path
output, ignored-file copying, and setup. These do not belong in an adapter.

The Bash/Zsh wrapper passes a private temporary result file in
`NAMU_SHELL_RESULT`. Only successful operations selecting shell navigation with
focus enabled write a destination there. The wrapper preserves stdout and the
exit status, removes the file, and changes directory only after success. This
keeps navigation decisions in Go without changing path-only stdout. Reload an
existing shell wrapper with `exec bash` or `exec zsh` after upgrading.
