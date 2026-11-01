curl() {
  printf 'curl %s\n' "$*" >> "$TEST_LOG"
  if [ "${TEST_FAIL_DOWNLOAD-}" = 1 ]; then return 22; fi
  for arg do destination="$arg"; done
  case "$*" in
    *https://chatgpt.com/codex/install.sh*) cp "$TEST_FIXTURES/codex-installer.sh" "$destination";;
    *https://claude.ai/install.sh*) cp "$TEST_FIXTURES/claude-installer.sh" "$destination";;
    *) return 1;;
  esac
}
sudo() { printf 'sudo %s\n' "$*" >> "$TEST_LOG"; }
