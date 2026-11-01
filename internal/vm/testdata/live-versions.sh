set -eu
. "$NAMU_HOST_CREDENTIALS_DIR/codex.env"
test -n "$OPENAI_API_KEY"
test "$(git config --get user.name)" = 'Namu Smoke'
codex --version
claude --version
agent --version
