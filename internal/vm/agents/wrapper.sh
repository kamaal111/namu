set -eu
name=$(basename "$0")
base="$HOME/.local/share/namu-agents/$NAMU_AGENT_REVISION"
profile=${NAMU_HOST_CREDENTIALS_DIR-}
case "$name" in
  codex)
    if [ -n "$profile" ]; then
      . "$profile/codex.env"
      export CODEX_HOME="$HOME/.namu-vm/host-auth/codex"
    fi
    exec "$HOME/.local/bin/codex" "$@";;
  claude)
    if [ -n "$profile" ]; then
      . "$profile/claude.env"
      export CLAUDE_CONFIG_DIR="$HOME/.namu-vm/host-auth/claude"
    fi
    exec "$HOME/.local/bin/claude" "$@";;
  agent|cursor-agent)
    if [ -n "$profile" ]; then . "$profile/cursor.env"; fi
    exec "$base/cursor/cursor-agent" "$@";;
esac
exit 1
