# Executed as root through the owned VM's local Tart control channel.
umask 077
case "$expected_os:$(uname -s):$(uname -m)" in
  linux:Linux:aarch64|macos:Darwin:arm64) ;;
  *) echo 'Guest OS/architecture does not match the selected image' >&2; exit 1;;
esac
if [ "$expected_os" = macos ]; then
  home=$(dscl . -read "/Users/$user" NFSHomeDirectory | cut -d ' ' -f 2-)
  group=staff
else
  home=$(getent passwd "$user" | cut -d: -f6)
  group=$(id -gn "$user")
fi
test -n "$home" && test -d "$home"
test ! -L "$home/.ssh"
mkdir -p "$home/.ssh" "$home/.namu-vm/bootstrap"
chmod 700 "$home/.ssh" "$home/.namu-vm" "$home/.namu-vm/bootstrap"
test ! -L "$home/.ssh/authorized_keys"
touch "$home/.ssh/authorized_keys"
grep -qxF "$public_key" "$home/.ssh/authorized_keys" || printf '%s\n' "$public_key" >> "$home/.ssh/authorized_keys"
chmod 600 "$home/.ssh/authorized_keys"
chown -R "$user:$group" "$home/.ssh" "$home/.namu-vm"
