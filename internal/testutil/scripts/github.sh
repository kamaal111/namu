#!/bin/sh
printf '%s\n' "$*" >> "$GH_CALLS"
case "$1" in
  api) cat >> "$GH_CALLS.requests"; printf '\n' >> "$GH_CALLS.requests" ;;
esac
