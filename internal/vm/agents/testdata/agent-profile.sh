#!/bin/sh
case "$(basename "$0")" in
  codex) printf '%s\n' "${CODEX_HOME-guest-default}";;
  claude) printf '%s\n' "${CLAUDE_CONFIG_DIR-guest-default}";;
esac
