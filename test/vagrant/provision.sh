#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt-get update
apt-get install -y --no-install-recommends \
    apparmor \
    auditd \
    firewalld \
    nftables \
    dbus \
    ca-certificates \
    curl \
    git \
    make \
    network-manager \
    libpam-modules-bin \
    libpam-pwquality \
    logrotate \
    pamtester \
    rsync \
    golang-go

if test "${REMOTR_VM_PROFILE:-default}" = desktop-session
then
    apt-get install -y --no-install-recommends \
        dbus-x11 \
        dconf-cli \
        gsettings-desktop-schemas \
        libglib2.0-bin

    for account in remotr-desktop-a:24001 remotr-desktop-b:24002
    do
        username=${account%%:*}
        uid=${account##*:}
        if ! id "$username" >/dev/null 2>&1
        then
            useradd --create-home --user-group --uid "$uid" --shell /bin/bash "$username"
        fi
        install -d -o "$username" -g "$username" -m 700 "/home/$username"
    done
fi

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
