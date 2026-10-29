# List project workflows
@_default:
    just --list --unsorted

# Build the native executable
build:
    CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/namu ./cmd/namu

# Run Go tests
test:
    go test ./...

# Run all checks, including cross-platform builds
check:
    sh scripts/check.sh

# Format Go source
format:
    gofmt -w cmd internal

# Build stable release archives without publishing
package tag:
    sh scripts/release.sh {{quote(tag)}}
