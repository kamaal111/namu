# Executed as root through the owned VM's local Tart control channel.
test "$(uname -s):$(uname -m)" = Darwin:arm64 || { echo 'Guest OS/architecture does not match the selected image' >&2; exit 1; }
home=$(dscl . -read "/Users/$user" NFSHomeDirectory | cut -d ' ' -f 2-)
group=staff
