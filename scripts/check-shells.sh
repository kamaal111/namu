#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
for script in scripts/*.sh internal/cli/scripts/*.sh internal/cli/testdata/*.sh internal/testutil/scripts/*.sh internal/vm/testdata/*.sh internal/vm/guest/*.sh internal/vm/guest/testdata/*.sh internal/vm/platform/*.sh internal/vm/platform/*/*.sh internal/vm/platform/*/testdata/*.sh internal/vm/agents/*.sh internal/vm/agents/testdata/*.sh; do sh -n "$script"; done
