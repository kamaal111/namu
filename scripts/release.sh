#!/bin/sh
# Build deterministic archives for all supported targets; optionally upload them.
set -eu
cd "$(dirname "$0")/.."
tag=${1:?Usage: sh scripts/release.sh v0.1.0 [--publish]}
mode=${2:-}
test "$#" -le 2 || { echo 'Too many arguments' >&2; exit 1; }
test "$mode" = '' || test "$mode" = --publish || { echo 'Unknown option' >&2; exit 1; }
python3 scripts/release.py validate-tag "$tag"
version=${tag#v}
test "$(cat version.txt)" = "$version" || { echo 'Tag must match version.txt' >&2; exit 1; }
if test "$mode" = --publish; then
  test "$(git rev-parse HEAD)" = "$(git rev-parse "refs/tags/$tag^{commit}")" || { echo 'Check out the release tag before publishing' >&2; exit 1; }
  test -z "$(git status --porcelain --untracked-files=normal)" || { echo 'Publishing requires a clean checkout' >&2; exit 1; }
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
mkdir -p bin/release
for target in $(python3 scripts/release.py targets); do
  system=${target%_*}
  arch=${target#*_}
  CGO_ENABLED=0 GOOS="$system" GOARCH="$arch" go build -buildvcs=false -trimpath \
    -ldflags "-s -w -X github.com/kamaal111/namu/internal/cli.Version=$version" \
    -o "$work/namu" ./cmd/namu
  python3 scripts/release.py package "$work/namu" "bin/release/namu_${version}_${target}.tar.gz"
done
CGO_ENABLED=0 go build -buildvcs=false -trimpath \
  -ldflags "-X github.com/kamaal111/namu/internal/cli.Version=$version" -o "$work/native-namu" ./cmd/namu
test "$("$work/native-namu" --version)" = "namu version $version"
"$work/native-namu" --help >/dev/null
python3 scripts/release.py checksums "$tag" bin/release
if test "$mode" = --publish; then
  mkdir "$work/existing"
  python3 scripts/release.py publish "$tag" bin/release "$work/existing"
fi
printf 'Packaged namu %s for macOS and Linux (ARM64 and AMD64) in bin/release\n' "$version"
