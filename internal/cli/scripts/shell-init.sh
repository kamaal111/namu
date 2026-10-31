namu() {
  case "${1-}" in
    create|open) ;;
    *) NAMU_SHELL_INTEGRATION=1 command namu "$@"; return $? ;;
  esac
  local namu_path namu_result namu_file namu_destination
  if ! namu_file=$(mktemp "${TMPDIR:-/tmp}/namu-navigation.XXXXXXXXXX"); then
    printf '%s\n' 'Warning: cannot prepare shell navigation' >&2
    NAMU_SHELL_INTEGRATION=1 NAMU_SHELL_RESULT= command namu "$@"
    return $?
  fi
  if namu_path=$(NAMU_SHELL_INTEGRATION=1 NAMU_SHELL_RESULT="$namu_file" command namu "$@"); then
    namu_result=0
  else
    namu_result=$?
  fi
  namu_destination=$(cat -- "$namu_file")
  rm -f -- "$namu_file"
  if [ -n "$namu_path" ]; then printf '%s\n' "$namu_path"; fi
  if [ "$namu_result" = 0 ] && [ -n "$namu_destination" ]; then
    cd -- "$namu_destination" || return $?
  fi
  return "$namu_result"
}
