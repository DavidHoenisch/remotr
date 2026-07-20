#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y --no-install-recommends \
    firewalld \
    nftables \
    dbus \
    ca-certificates \
    curl \
    git \
    make \
    network-manager \
    rsync \
    golang-go

systemctl enable firewalld || true
systemctl start firewalld || true
install -d -o root -g root -m 755 /etc/NetworkManager/conf.d
printf '%s\n' '[keyfile]' 'unmanaged-devices=*,except:interface-name:remotr-dns0' > /etc/NetworkManager/conf.d/90-remotr-provider-safety.conf
systemctl enable NetworkManager
systemctl start NetworkManager

# Verify backends
firewall-cmd --version || true
nft --version || true

echo "Firewall test VM provisioned. Run:"
echo "  vagrant ssh -c 'cd /workspace && go test -mod=vendor ./internal/applicators/firewall/ -tags=firewall -v'"
