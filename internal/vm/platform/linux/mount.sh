# Linux exposes the share tag as well as the filesystem type.
if ! findmnt -rn -t virtiofs -S "$tag" -M "$root" >/dev/null; then
  test -z "$(ls -A "$root")" || { echo "Guest mount destination is not empty: $root" >&2; exit 1; }
  mount -t virtiofs "$tag" "$root"
fi
