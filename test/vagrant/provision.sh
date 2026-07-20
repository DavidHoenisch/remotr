#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y --no-install-recommends \
    apparmor \
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
printf '%s\n' \
    '[keyfile]' \
    'unmanaged-devices=*,except:interface-name:remotr-dns0' \
    '' \
    '[device-remotr-dns0]' \
    'match-device=interface-name:remotr-dns0' \
    'managed=1' \
    > /etc/NetworkManager/conf.d/99-remotr-provider-safety.conf
systemctl enable NetworkManager
systemctl restart NetworkManager

# Verify backends
firewall-cmd --version || true
nft --version || true

echo "Firewall test VM provisioned. Run:"
echo "  vagrant ssh -c 'cd /workspace && go test -mod=vendor ./internal/applicators/firewall/ -tags=firewall -v'"
