set -eu
sudo -n apt-get update
sudo -n env DEBIAN_FRONTEND=noninteractive apt-get install -y git ca-certificates python3 curl
git --version
