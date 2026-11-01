# macOS reports a generic virtio-fs source, not the requested mount tag.
mac_mount_check() {
  mount | grep -F -- " on $root (" | grep -Ei '(virtiofs|virtio-fs)' >/dev/null
}
