#!/bin/bash
set -e

apt-get update
apt-get install -y --no-install-recommends \
    firewalld \
    nftables \
    dbus \
    ca-certificates \
    curl \
    git \
    make \
    golang-go

systemctl enable firewalld || true
systemctl start firewalld || true

# Verify backends
firewall-cmd --version || true
nft --version || true

echo "Firewall test VM provisioned. Run:"
echo "  vagrant ssh -c 'cd /workspace && go test -mod=vendor ./internal/applicators/firewall/ -tags=firewall -v'"
