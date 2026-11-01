#!/bin/sh
[ "$1" = open ] && [ "$3" = --vm ] || exit 9
printf 'guest-ready\n' >&2
IFS= read -r answer
[ "$answer" = from-terminal ] || exit 8
printf 'guest-output:%s\n' "$answer" >&2
printf '/host/checkout\n'
