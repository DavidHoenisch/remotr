# Provider behavior containers

These immutable amd64 images provide the Debian 12.11 and Ubuntu 24.04
package-manager/filesystem boundary used by the initial provider matrix.

They may prove user-space behavior exposed through `apt-get`, `dpkg`, and
ordinary filesystem operations. They do not prove systemd service management,
firewall enforcement or recovery, mounts, boot/reboot, kernel settings,
AppArmor, authentication recovery, or destructive-device behavior; those need
the VM matrix introduced by later tasks.

`Dockerfile.arch-2026-07-06` is a separately pinned rolling-release snapshot.
It proves only the `pacman` boundary; it deliberately does not install or
assert an AUR helper such as `yay`.

The versioned matrix also records planned identity, service, and repository
rows for all three images. Those rows stay `untested` until their provider
selectors execute successfully in an environment capable of proving the
behavior; a container build or backend probe never promotes them to passing.
