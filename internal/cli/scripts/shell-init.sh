namu() {
  case "${1-}" in
    create|open) ;;
    *) NAMU_SHELL_INTEGRATION=1 command namu "$@"; return $? ;;
  esac
  local namu_arg namu_path namu_result namu_focus=1 namu_skip=0
  for namu_arg in "$@"; do
    if [ "$namu_skip" = 1 ]; then
      namu_skip=0
      continue
    fi
    case "$namu_arg" in
      --) break ;;
      --base|--remote) namu_skip=1 ;;
      --help|--help=true|-h|-h=true) NAMU_SHELL_INTEGRATION=1 command namu "$@"; return $? ;;
      --no-focus|--no-focus=true|--focus=false) namu_focus=0 ;;
      --focus|--focus=true|--no-focus=false) namu_focus=1 ;;
    esac
  done
  if [ "$namu_focus" = 0 ]; then
    NAMU_SHELL_INTEGRATION=1 command namu "$@"
    return $?
  fi
  if namu_path=$(NAMU_SHELL_INTEGRATION=1 command namu "$@"); then
    namu_result=0
  else
    namu_result=$?
  fi
  if [ -n "$namu_path" ]; then printf '%s\n' "$namu_path"; fi
  if [ "$namu_result" = 0 ] && [ -n "$namu_path" ]; then
    case "$namu_path" in
      '~') namu_path=$HOME ;;
      '~/'*) namu_path="$HOME/${namu_path#\~/}" ;;
    esac
    cd -- "$namu_path" || return $?
  fi
  return "$namu_result"
}
