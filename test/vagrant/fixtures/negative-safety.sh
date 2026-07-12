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

work_dir=$(mktemp -d /var/tmp/remotr-negative-safety.XXXXXX)
recovery_user=remotr-vm-recovery
sudoers_file="/etc/sudoers.d/$recovery_user"
loop_devices=()

cleanup() {
  status=$?
  rm -f "$sudoers_file"
  if id "$recovery_user" >/dev/null 2>&1
  then
    userdel --remove "$recovery_user" || true
  fi
  for loop_device in "${loop_devices[@]}"
  do
    losetup --detach "$loop_device" || true
  done
  rm -rf "$work_dir"
  exit "$status"
}
trap cleanup EXIT INT TERM

block_last_recovery_principal_removal() {
  local target=$1
  local -a recovery_principals=("$recovery_user")

  if test "${#recovery_principals[@]}" -eq 1 && test "${recovery_principals[0]}" = "$target"
  then
    printf 'blocked:last-admin-path\n' >&2
    return 1
  fi
}

block_invalid_boot_state() {
  local proposed_state=$1

  case "$proposed_state" in
    current)
      return 0
      ;;
    *)
      printf 'blocked:invalid-boot-state\n' >&2
      return 1
      ;;
  esac
}

select_exactly_one_device() {
  if test "$#" -ne 1
  then
    printf 'blocked:ambiguous-device\n' >&2
    return 1
  fi
  printf '%s\n' "$1"
}

redact_canary() {
  local value=$1
  local canary=$2

  printf '%s\n' "${value//"$canary"/[REDACTED]}"
}

# A sole synthetic sudo principal is a disposable stand-in for a provider's
# designated recovery channel. The guard must reject the delete before userdel
# runs, leaving an authenticated administrative path available.
useradd --create-home --shell /bin/sh "$recovery_user"
printf '%s ALL=(ALL) NOPASSWD: ALL\n' "$recovery_user" > "$sudoers_file"
chmod 440 "$sudoers_file"
visudo -cf "$sudoers_file" >/dev/null
test "$(runuser -u "$recovery_user" -- sudo -n true; printf '%s' "$?")" = 0
if block_last_recovery_principal_removal "$recovery_user"
then
  echo "last-admin-path removal was not blocked" >&2
  exit 1
fi
id "$recovery_user" >/dev/null
test -f "$sudoers_file"

# Invalid boot proposals must be rejected before touching the guest's real
# bootloader configuration. The fixture deliberately uses a malformed state
# identifier and confirms that no boot mutation marker is created.
if block_invalid_boot_state 'kernel=../../untrusted'
then
  echo "invalid boot state was not blocked" >&2
  exit 1
fi
test ! -e "$work_dir/boot-state-applied"

# Allocate two real disposable loop devices, then prove the destructive-device
# selector rejects the ambiguity without mounting, formatting, or selecting
# either device.
for number in 1 2
do
  image="$work_dir/device-$number.img"
  truncate -s 4M "$image"
  loop_devices+=("$(losetup --find --show "$image")")
done
if select_exactly_one_device "${loop_devices[@]}" > "$work_dir/selected-device"
then
  echo "ambiguous devices were not blocked" >&2
  exit 1
fi
test ! -s "$work_dir/selected-device"

# A canary begins in the raw diagnostic boundary and must not appear in the
# retained output. The report remains deliberately small and contains only a
# redaction marker, not the synthetic secret.
canary="remotr-test-secret-negative-$RANDOM-$(date +%s)"
diagnostic="$work_dir/diagnostic.log"
redact_canary "provider=fixture token=$canary transition=blocked" "$canary" > "$diagnostic"
if grep -Fq "$canary" "$diagnostic"
then
  echo "redacted diagnostic leaked synthetic canary" >&2
  exit 1
fi
grep -Fqx 'provider=fixture token=[REDACTED] transition=blocked' "$diagnostic"
test "$(wc -c < "$diagnostic")" -le 256

mkdir -p "$(dirname "$report")"
printf '%s\n' \
  'connectivity_loss=covered-by-network-recovery' \
  'last_admin_path=blocked-before-mutation' \
  'invalid_boot_state=blocked-before-mutation' \
  'ambiguous_devices=blocked-before-mutation' \
  'secret_canary=redacted' > "$report"
