# Executed as root through the owned VM's local Tart control channel.
test "$(uname -s):$(uname -m)" = Linux:aarch64 || { echo 'Guest OS/architecture does not match the selected image' >&2; exit 1; }
home=$(getent passwd "$user" | cut -d: -f6)
group=$(id -gn "$user")
