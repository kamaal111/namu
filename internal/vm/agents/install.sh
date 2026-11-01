base="$HOME/.local/share/namu-agents/$revision"
wrapper_dir="$HOME/.local/share/namu-agents/bin"
mkdir -p "$base" "$HOME/.local/bin" "$wrapper_dir"
tmp=$(mktemp -d "$base/install.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
download() {
  curl --disable --fail --location --retry 3 --connect-timeout 20 --proto '=https' --proto-redir '=https' "$1" -o "$2"
}
fetch() {
  download "$1" "$2"
  printf '%s  %s\n' "$3" "$2" > "$tmp/checksum"
  if command -v sha256sum >/dev/null; then sha256sum -c "$tmp/checksum"; else shasum -a 256 -c "$tmp/checksum"; fi
}
for selected in codex claude cursor; do
  target="$base/$selected"
  if [ ! -f "$target/complete" ]; then
    rm -rf "$target.incomplete"
    mkdir "$target.incomplete"
    case "$selected" in
      codex)
        download https://chatgpt.com/codex/install.sh "$tmp/codex-install.sh"
        CODEX_HOME="$HOME/.codex" CODEX_NON_INTERACTIVE=1 sh "$tmp/codex-install.sh"
        "$HOME/.local/bin/codex" --version;;
      claude)
        download https://claude.ai/install.sh "$tmp/claude-install.sh"
        bash "$tmp/claude-install.sh"
        "$HOME/.local/bin/claude" --version;;
      cursor)
        fetch "https://downloads.cursor.com/lab/$cursor_version/$platform/arm64/agent-cli-package.tar.gz" "$tmp/cursor.tar.gz" "$cursor_sha"
        tar -xzf "$tmp/cursor.tar.gz" --strip-components=1 -C "$target.incomplete";;
    esac
    touch "$target.incomplete/complete"
    mv "$target.incomplete" "$target"
  fi
done
