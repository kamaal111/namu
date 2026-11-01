if ! mac_mount_check; then
  test -z "$(ls -A "$root")" || { echo "Guest mount destination is not empty: $root" >&2; exit 1; }
  mount_virtiofs "$tag" "$root"
fi
