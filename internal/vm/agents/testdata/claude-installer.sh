set -eu
printf 'claude %s\n' "$*" >> "$TEST_LOG"
mkdir -p "$HOME/.local/bin" "$HOME/.local/share/native-agents"
printf '%s\n' '#!/bin/sh' 'printf "Claude Code test home=%s args=%s\\n" "${CLAUDE_CONFIG_DIR-guest-default}" "$*"' > "$HOME/.local/share/native-agents/claude"
chmod 700 "$HOME/.local/share/native-agents/claude"
ln -sfn "$HOME/.local/share/native-agents/claude" "$HOME/.local/bin/claude"
