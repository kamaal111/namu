#!/bin/sh
printf '%s\n' "$@" >> "$HERDR_CALLS"
action=$2
shift 2
while [ "$#" -gt 0 ]; do
 case "$1" in
  --cwd) source=$2; shift 2 ;;
  --path) target=$2; shift 2 ;;
  --branch) branch=$2; shift 2 ;;
  --base) base=$2; shift 2 ;;
  *) shift ;;
 esac
done
