#!/usr/bin/env bash
set -euo pipefail

report=

while test "$#" -gt 0
do
  case "$1" in
    --report)
      report=$2
      shift 2
      ;;
    *)
      echo "unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

test "$(id -u)" = 0
test -n "$report"

work_dir=$(mktemp -d /var/tmp/remotr-system-safety.XXXXXX)
disk_image="$work_dir/mount.img"
mount_dir="$work_dir/mount"
recovery_user=remotr-vm-recovery
loop_device=
mounted=false
original_ip_forward=$(sysctl -n net.ipv4.ip_forward)

cleanup() {
  status=$?
  sysctl -w "net.ipv4.ip_forward=$original_ip_forward" >/dev/null || true
  if test "$mounted" = true
  then
    umount "$mount_dir" || true
  fi
  if test -n "$loop_device"
  then
    losetup --detach "$loop_device" || true
  fi
  if id "$recovery_user" >/dev/null 2>&1
  then
    userdel --remove "$recovery_user" || true
  fi
  rm -rf "$work_dir"
  exit "$status"
}
trap cleanup EXIT INT TERM

truncate -s 16M "$disk_image"
loop_device=$(losetup --find --show "$disk_image")
mkfs.ext4 -F "$loop_device" >/dev/null
mkdir -p "$mount_dir"
mount "$loop_device" "$mount_dir"
mounted=true
mountpoint -q "$mount_dir"

modprobe loop
test -d /sys/module/loop
sysctl -w net.ipv4.ip_forward=1 >/dev/null
test "$(sysctl -n net.ipv4.ip_forward)" = 1

if test -d /sys/kernel/security/apparmor
then
  apparmor=available
else
  apparmor=unavailable
fi

useradd --create-home --shell /bin/sh "$recovery_user"
test "$(runuser -u "$recovery_user" -- id -un)" = "$recovery_user"

umount "$mount_dir"
mounted=false
losetup --detach "$loop_device"
loop_device=
userdel --remove "$recovery_user"

sysctl -w "net.ipv4.ip_forward=$original_ip_forward" >/dev/null
test "$(sysctl -n net.ipv4.ip_forward)" = "$original_ip_forward"

mkdir -p "$(dirname "$report")"
printf 'loop_module=available\nsysctl=restored\napparmor=%s\nrecovery_principal=verified\n' "$apparmor" > "$report"
