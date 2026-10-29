# Releases and Homebrew

Namu follows agent-vm's Release Please → GitHub release archives → reviewed Homebrew
formula workflow. The first stable version is `v0.1.0`.

## Repository setup

Publish this repository as `kamaal111/namu` on GitHub with `main` as its default branch.
Enable **Settings → Actions → General → Workflow permissions → Allow GitHub Actions
to create and approve pull requests**. Require the `check` workflow on pull requests.

Use Conventional Commits. Release Please updates `version.txt`, the manifest, and
`CHANGELOG.md`, and creates a stable `vX.Y.Z` tag when its release PR is merged.
`fix:` increments patch and `feat:` increments minor. Do not leave a persistent
`release-as` override in the release configuration.

The manifest's initial `0.1.0` is the baseline used to calculate the first automated
release; without a bootstrap override, initial features can therefore propose a later
version. To publish exactly `v0.1.0` first, add `release-as: 0.1.0` to the package's
Release Please configuration for the bootstrap release PR only, then remove it for
subsequent releases. The archive scripts always require the tag and version file to agree.

The release workflow explicitly dispatches checks on generated release branches,
because PRs created with the default Actions token do not trigger ordinary PR workflows.
Publishing rechecks the tagged source and builds these CGO-disabled assets:

```text
namu_0.1.0_darwin_arm64.tar.gz
namu_0.1.0_darwin_amd64.tar.gz
namu_0.1.0_linux_arm64.tar.gz
namu_0.1.0_linux_amd64.tar.gz
checksums.txt
```

Archives have normalized metadata and contain only the executable. The release
build embeds the version in `namu --version`; compiler versions are pinned in CI.

## Local packaging

From the repository root with a matching `version.txt`:

```sh
just check
just package v0.1.0
```

Assets appear under `bin/release/`. No GitHub publication occurs unless the release
script receives `--publish`; publication requires a clean checkout at the release tag.

## Recover failed uploads

```sh
gh workflow run release.yml --repo kamaal111/namu -f release_tag=v0.1.0
```

Recovery builds the existing tag, checks every existing expected asset byte-for-byte
before uploading anything, and uploads only missing assets. It refuses to overwrite
conflicting content. New source changes need a new release, not replacement tags.

## Publish the Homebrew formula

After all release assets are uploaded, generate a formula from verified downloads:

```sh
python3 scripts/update_homebrew.py --version v0.1.0 --output /tmp/namu.rb
```

Omit `--version` to select the latest published stable release. The updater checks
release completeness and all four archive SHA-256 values before writing the formula.
It requires `gh` with access to the public release; it does not push or open a PR.

In `kamaal111/homebrew-tap`, add the generated file as `Formula/namu.rb`, validate
it with `brew style Formula/namu.rb`, then install and test the local formula:

```sh
brew install --formula ./Formula/namu.rb
brew test namu
```

Open a formula PR following the existing agent-vm update workflow. Review and merge
that PR before advertising `brew install kamaal111/tap/namu`. Subsequent releases
regenerate the same formula. The template supports native binaries for macOS and
Linux on ARM64 and AMD64; no source compilation or bottles are required.

On both macOS and Linux, manually smoke-test the installed binary with `--version`,
`--help`, creation, the branch picker, listing, setup retry, and deletion inside a
throwaway repository. Publishing releases, opening the tap PR, and validating an
actual Homebrew installation require the repository and first release to exist.
