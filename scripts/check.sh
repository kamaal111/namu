#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
test -z "$(gofmt -l cmd internal)" || { echo 'Run gofmt -w cmd internal' >&2; exit 1; }
go mod verify
go vet ./...
go test ./...
go test -race ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
python3 -m unittest discover -s scripts -p 'test_*.py'
for script in scripts/*.sh; do sh -n "$script"; done
mkdir -p bin
for target in $(python3 scripts/release.py targets); do
  system=${target%_*}
  arch=${target#*_}
  CGO_ENABLED=0 GOOS="$system" GOARCH="$arch" go build -buildvcs=false -trimpath -o "bin/namu_${target}" ./cmd/namu
done
CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/namu ./cmd/namu
bin/namu --version
bin/namu --help >/dev/null
