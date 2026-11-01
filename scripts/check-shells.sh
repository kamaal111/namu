#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
for script in scripts/*.sh internal/cli/scripts/*.sh internal/testutil/scripts/*.sh internal/vm/guest/*.sh; do sh -n "$script"; done
