set shell := ["mise", "exec", "--", "sh", "-cu"]

# List project workflows
@_default:
    just --list --unsorted

# Build the native executable
build:
    CGO_ENABLED=0 go build -buildvcs=false -trimpath -o bin/namu ./cmd/namu

# Run Namu from source
run +args:
    go run ./cmd/namu {{args}}

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

# Inspect prerequisites for opt-in real VM verification
vm-prerequisites:
    uname -sm
    sw_vers -productVersion
    tart --version
    ssh -V

# Validate sharing and guest Git against an already prepared namu VM
vm-smoke name:
    NAMU_VM_SMOKE={{ quote(name) }} go test ./internal/vm -run '^TestLiveVMWorktree$' -v

# Run the VM lifecycle regression under the race detector
vm-race:
    go test -race ./internal/vm/... -run 'TestConcurrent|TestEnsure|TestStore'

# Inspect SSH readiness inside a running validation VM without exposing keys
vm-diagnose name:
    tart ip {{ quote(name) }} --wait 10
    tart exec {{ quote(name) }} sudo -n /bin/sh -c 'uname -s; /sbin/mount | grep virtiofs || true; /usr/sbin/sshd -t; if command -v systemctl >/dev/null; then systemctl --no-pager status ssh; ls -ld /home/admin /home/admin/.ssh /home/admin/.ssh/authorized_keys; ssh-keygen -lf /home/admin/.ssh/authorized_keys; fi'
