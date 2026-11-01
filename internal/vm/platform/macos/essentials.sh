set -eu
command -v /opt/homebrew/bin/brew >/dev/null || { echo 'Default macOS image requires Homebrew' >&2; exit 1; }
HOMEBREW_NO_AUTO_UPDATE=1 /opt/homebrew/bin/brew install git python
git --version
