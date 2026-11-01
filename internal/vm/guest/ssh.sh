# Retain this per-clone key across interrupted setup, never reuse the image key.
key="$home/.namu-vm/bootstrap/ssh_host_ed25519_key"
if [ ! -f "$key" ]; then
  rm -f "$key.pending" "$key.pending.pub"
  ssh-keygen -q -t ed25519 -N '' -f "$key.pending"
  mv "$key.pending.pub" "$key.pub"
  mv "$key.pending" "$key"
fi
cp "$key" /etc/ssh/ssh_host_ed25519_key
cp "$key.pub" /etc/ssh/ssh_host_ed25519_key.pub
chmod 600 /etc/ssh/ssh_host_ed25519_key
if ! grep -q '^# namu-vm managed SSH$' /etc/ssh/sshd_config; then
  {
    printf '%s\n' '# namu-vm managed SSH' 'PasswordAuthentication no' 'KbdInteractiveAuthentication no' 'PubkeyAuthentication yes'
    cat /etc/ssh/sshd_config
  } > /etc/ssh/sshd_config.namu-vm
  chmod 600 /etc/ssh/sshd_config.namu-vm
  mv /etc/ssh/sshd_config.namu-vm /etc/ssh/sshd_config
fi
if [ "$expected_os" = linux ]; then mkdir -p /run/sshd; chmod 755 /run/sshd; fi
/usr/sbin/sshd -t
if [ "$expected_os" = linux ]; then
  systemctl restart ssh
fi
cat /etc/ssh/ssh_host_ed25519_key.pub
