set shell := ["mise", "exec", "--", "sh", "-cu"]

# List project workflows
@_default:
    just --list --unsorted

# Build the native executable
build:
    CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/namu ./cmd/namu

# Run Go tests
test package="./...":
    go test {{ quote(package) }}

# Run all checks, including cross-platform builds
check:
    sh scripts/check.sh

# Format Go source
format:
    gofmt -w cmd internal

# Build stable release archives without publishing
package tag:
    sh scripts/release.sh {{ quote(tag) }}

# Run matching Go tests in a package
test-run pattern package="./...":
    go test {{ quote(package) }} -run {{ quote(pattern) }}

# Check Go source without changing it
vet:
    go vet ./...

# Validate project shell scripts
check-shells:
    sh scripts/check-shells.sh
