#!/bin/sh
set -eu
[ "${1-}" = get ] || exit 0
protocol= host=
while IFS='=' read -r name value && [ -n "$name" ]; do
  case "$name" in protocol) protocol=$value;; host) host=$value;; esac
done
[ "$protocol" = https ] && [ "$host" = github.com ] || exit 0
[ -n "${NAMU_HOST_CREDENTIALS_DIR-}" ] || exit 0
. "$NAMU_HOST_CREDENTIALS_DIR/git.env"
[ -n "${GH_TOKEN-}" ] || exit 0
printf 'username=x-access-token\npassword=%s\n\n' "$GH_TOKEN"
