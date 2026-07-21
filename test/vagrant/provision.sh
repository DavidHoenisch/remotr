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
        desktop-file-utils \
        dconf-cli \
        gsettings-desktop-schemas \
        libglib2.0-bin \
        xdg-utils

    cat > /usr/share/glib-2.0/schemas/com.remotr.Qualification.gschema.xml <<'EOF'
<schemalist>
  <schema id="com.remotr.Qualification" path="/com/remotr/qualification/">
    <key name="boolean" type="b"><default>false</default></key>
    <key name="string" type="s"><default>'before'</default></key>
    <key name="int32" type="i"><default>1</default></key>
    <key name="int64" type="x"><default>int64 2</default></key>
    <key name="uint32" type="u"><default>uint32 3</default></key>
    <key name="double" type="d"><default>2.0</default></key>
    <key name="string-list" type="as"><default>['before']</default></key>
    <key name="cleanup-gsettings" type="b"><default>false</default></key>
    <key name="malicious-gsettings" type="b"><default>false</default></key>
    <key name="mandatory" type="b"><default>true</default></key>
  </schema>
</schemalist>
EOF
    glib-compile-schemas /usr/share/glib-2.0/schemas

    cat > /usr/share/applications/remotr-browser.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Remotr Qualification Browser
Exec=/bin/true %u
MimeType=text/html;x-scheme-handler/http;
NoDisplay=true
EOF
    cat > /usr/share/applications/remotr-viewer.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Remotr Qualification Viewer
Exec=/bin/true %f
MimeType=application/pdf;image/png;
NoDisplay=true
EOF
    cat > /usr/share/applications/remotr-other.desktop <<'EOF'
[Desktop Entry]
Type=Application
Name=Remotr Qualification Other Handler
Exec=/bin/true %f
MimeType=text/html;application/pdf;
NoDisplay=true
EOF
    update-desktop-database /usr/share/applications

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
