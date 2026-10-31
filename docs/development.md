# Development

## Tool setup

Install [mise](https://mise.jdx.dev/getting-started.html) and Git, then run
these commands from the checkout you will work in:

```sh
mise trust
mise install
mise exec -- just
```

The repository's `mise.toml` pins Go 1.27.3, Python 3.15.2, and just 1.59.0.
Git, Bash, and Zsh remain system prerequisites. Bash and Zsh are used by the
shell integration tests. Tool installation and vulnerability checks
require network access.

Every just recipe runs through `mise exec`, so it loads the project tools even
in a shell without mise activation. If just itself is not on your `PATH`, use
`mise exec -- just …`. Interactive shell activation is optional.

## Build and test

```sh
mise exec -- just test
mise exec -- just test-run TestShorten ./internal/pathdisplay
mise exec -- just vet
mise exec -- just check-shells
mise exec -- just check
mise exec -- just build
mise exec -- just format
mise exec -- just package "v$(cat version.txt)"
```

`just test` runs the Go tests. `just check` checks formatting, verifies modules,
runs vet, Go tests, race tests, `govulncheck`, Python release-tool tests, and
CGO-disabled builds for macOS and Linux on ARM64 and AMD64. Integration tests
create isolated local Git repositories and bare remotes; they do not use
accounts or network remotes.

Use existing `just` recipes for repeatable development and verification commands.
Add or extend a recipe before running a missing workflow, so other agents can
reuse it. The root `AGENTS.md` records this policy. `just test PACKAGE` selects
a package; `just test-run PATTERN PACKAGE` selects matching tests. Both default
to all packages when the package argument is omitted.

To install a local source build:

```sh
mise exec -- just build
mkdir -p "$HOME/.local/bin"
install -m 755 bin/namu "$HOME/.local/bin/namu"
```

Add `~/.local/bin` to your `PATH`.

## CI and verification

The macOS and Linux check jobs install tools from the same `mise.toml` and run
`just check`. Release publishing installs tools from the checked-out release
tag's config, runs `just check`, and invokes the release script through mise.
CI uses mise 2026.10.24.

A mise installation or configuration failure blocks verification. Report the
failed command and its output; do not substitute a system or temporary
toolchain and claim that the project setup passed.

See [releases and Homebrew](releases.md) for packaging, publishing, and recovery.

## Implementation notes

The optional picker uses the same Cobra/Charm stack as `agent-vm`; only its
Gitignore matcher is imported from go-git, while Git itself performs repository
operations.

## Adding an integration

See [Integration hooks](integrations.md) for the typed hook contract, adapter
examples, lifecycle ordering, recovery behavior, and registration instructions.
