# Only reuse mountpoints recorded by namu; never hide unrelated guest files.
part="$root"
while [ "$part" != / ]; do
  test ! -L "$part" || { echo "Guest mount path contains symlink: $part" >&2; exit 1; }
  part=$(dirname "$part")
done
marker="/var/db/namu-vm/$tag"
mkdir -p /var/db/namu-vm
if [ ! -f "$marker" ]; then
  if [ -e "$root" ]; then
    test -d "$root" && test -z "$(ls -A "$root")" || { echo "Guest mount destination is not empty: $root" >&2; exit 1; }
  fi
  mkdir -p "$root"
  printf '%s\n' "$root" > "$marker"
fi
test "$(cat "$marker")" = "$root"
