set -eu
cd "$worktree_path"
test -w .
git -c "safe.directory=$worktree_path" rev-parse --is-inside-work-tree >/dev/null
git config --global --get-all safe.directory | grep -qxF -- "$worktree_path" || git config --global --add safe.directory "$worktree_path"
