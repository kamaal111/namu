set -eu
printf 'codex %s noninteractive=%s\n' "$*" "$CODEX_NON_INTERACTIVE" >> "$TEST_LOG"
mkdir -p "$HOME/.local/bin" "$HOME/.local/share/native-agents"
printf '%s\n' '#!/bin/sh' 'printf "codex-cli test home=%s args=%s\\n" "${CODEX_HOME-guest-default}" "$*"' > "$HOME/.local/share/native-agents/codex"
chmod 700 "$HOME/.local/share/native-agents/codex"
ln -sfn "$HOME/.local/share/native-agents/codex" "$HOME/.local/bin/codex"
