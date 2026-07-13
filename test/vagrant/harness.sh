#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
vagrant_dir="$root/test/vagrant"
network=remotr-provider-safety
domain=vagrant_default
snapshot=remotr-baseline
machine_key="$vagrant_dir/.vagrant/machines/default/libvirt/private_key"
probe=/tmp/remotr-snapshot-probe
recovery_pid=
recovery_runtime=
failure_runtime=
user_safety_runtime=

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

network_active() {
  virsh -c qemu:///system net-info "$network" | awk '/^Active:/ { print $2 }' | grep -qx yes
}

prepare_network() {
  if ! virsh -c qemu:///system net-info "$network" >/dev/null 2>&1
  then
    virsh -c qemu:///system net-define "$vagrant_dir/remotr-provider-safety-network.xml"
  fi

  if ! network_active
  then
    virsh -c qemu:///system net-start "$network"
  fi
}

save_baseline() {
  if ! (
    cd "$vagrant_dir"
    vagrant snapshot list | grep -qx "$snapshot"
  )
  then
    (
      cd "$vagrant_dir"
      vagrant snapshot save "$snapshot"
    )
  fi
}

up() {
  prepare_network
  (
    cd "$vagrant_dir"
    vagrant up --provider=libvirt
  )

  test -s "$machine_key"
  test "$(stat -c '%a' "$machine_key")" = 600
  save_baseline
}

restore() {
  (
    cd "$vagrant_dir"
    vagrant snapshot restore "$snapshot"
  )
}

destroy() {
  if test -e "$vagrant_dir/.vagrant/machines/default/libvirt/id"
  then
    (
      cd "$vagrant_dir"
      vagrant destroy -f
    )
  fi

  if virsh -c qemu:///system net-info "$network" >/dev/null 2>&1
  then
    virsh -c qemu:///system net-destroy "$network" || true
    virsh -c qemu:///system net-undefine "$network"
  fi

  if virsh -c qemu:///system dominfo "$domain" >/dev/null 2>&1
  then
    echo "VM teardown retained domain: $domain" >&2
    exit 1
  fi

  if virsh -c qemu:///system vol-list default | grep -q "${domain}.img"
  then
    echo "VM teardown retained overlay disk: ${domain}.img" >&2
    exit 1
  fi

  test ! -e "$machine_key"
}

lifecycle() {
  up
  (
    cd "$vagrant_dir"
    vagrant ssh -c "sudo touch $probe"
    vagrant ssh -c "test -f $probe"
  )
  restore
  (
    cd "$vagrant_dir"
    vagrant ssh -c "test ! -e $probe"
  )
  destroy
}

recovery_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$recovery_pid"
  then
    kill "$recovery_pid" 2>/dev/null || true
    wait "$recovery_pid" 2>/dev/null || true
  fi
  if test -n "$recovery_runtime"
  then
    rm -rf "$recovery_runtime"
  fi
  destroy || status=1
  exit "$status"
}

network_recovery() {
  require_command go
  require_command curl

  recovery_runtime=$(mktemp -d)
  trap recovery_cleanup EXIT INT TERM
  umask 077
  token_file="$recovery_runtime/token"
  result_file="$recovery_runtime/acknowledgement"
  server_log="$recovery_runtime/server.log"
  head -c 32 /dev/urandom | base64 | tr -d '\n' > "$token_file"

  control_ip=$(ip route get 1.1.1.1 | awk 'NR == 1 { for (i = 1; i <= NF; i++) if ($i == "src") { print $(i + 1); exit } }')
  test -n "$control_ip"

  go build -mod=vendor -o "$recovery_runtime/server" "$root/test/vagrant/fixture-server"
  REMOTR_VM_FIXTURE_TOKEN="$(cat "$token_file")" "$recovery_runtime/server" --advertise "$control_ip" --result "$result_file" > "$server_log" 2>&1 &
  recovery_pid=$!

  control_url=
  for attempt in $(seq 1 30)
  do
    control_url=$(awk '/^READY / { print $2; exit }' "$server_log")
    if test -n "$control_url"
    then
      break
    fi
    if ! kill -0 "$recovery_pid" 2>/dev/null
    then
      cat "$server_log" >&2
      exit 1
    fi
    sleep 1
  done
  test -n "$control_url"

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$token_file" /tmp/remotr-vm-recovery-token
    vagrant ssh -c 'sudo install -o root -g root -m 600 /tmp/remotr-vm-recovery-token /run/remotr-vm-recovery-token'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-recovery-token'
    vagrant ssh -c "sudo /workspace/test/vagrant/fixtures/network-recovery.sh --health-url $control_url/health --ack-url $control_url/ack --token-file /run/remotr-vm-recovery-token"
  )
  grep -qx acknowledged "$result_file"
  echo "network recovery fixture verified"
}

system_safety_cleanup() {
  status=$?
  trap - EXIT INT TERM
  destroy || status=1
  exit "$status"
}

boot_id() {
  (
    cd "$vagrant_dir"
    vagrant ssh -c 'cat /proc/sys/kernel/random/boot_id'
  ) | tr -d '\r' | awk '/^[[:xdigit:]-]+$/ { id = $0 } END { if (id == "") exit 1; print id }'
}

system_safety() {
  trap system_safety_cleanup EXIT INT TERM
  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant ssh -c 'sudo /workspace/test/vagrant/fixtures/system-safety.sh --report /tmp/remotr-system-safety.report'
    vagrant ssh -c 'sudo test -s /tmp/remotr-system-safety.report'
    vagrant ssh -c 'sudo grep -Fqx reboot_pre_ack=ready /tmp/remotr-system-safety.report'
  )

  boot_before=$(boot_id)
  test -n "$boot_before"
  (
    cd "$vagrant_dir"
    vagrant reload --no-provision
  )
  boot_after=$(boot_id)
  test -n "$boot_after"
  test "$boot_before" != "$boot_after"
  echo "system safety fixture verified"
}

negative_safety() {
  trap system_safety_cleanup EXIT INT TERM
  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant ssh -c 'sudo /workspace/test/vagrant/fixtures/negative-safety.sh --report /tmp/remotr-negative-safety.report'
    vagrant ssh -c 'sudo test -s /tmp/remotr-negative-safety.report'
    vagrant ssh -c 'sudo grep -Fqx remotr_connectivity_loss=covered-by-network-recovery /tmp/remotr-negative-safety.report'
    vagrant ssh -c 'sudo grep -Fqx ssh_sudo_lockout=blocked-before-mutation /tmp/remotr-negative-safety.report'
    vagrant ssh -c 'sudo grep -Fqx invalid_boot_state=blocked-before-mutation /tmp/remotr-negative-safety.report'
    vagrant ssh -c 'sudo grep -Fqx ambiguous_devices=blocked-before-mutation /tmp/remotr-negative-safety.report'
    vagrant ssh -c 'sudo grep -Fqx secret_canary=redacted /tmp/remotr-negative-safety.report'
  )
  echo "negative safety fixture verified"
}

user_safety_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$user_safety_runtime"
  then
    rm -rf "$user_safety_runtime"
  fi
  destroy || status=1
  exit "$status"
}

user_safety() {
  require_command go

  user_safety_runtime=$(mktemp -d)
  trap user_safety_cleanup EXIT INT TERM
  user_safety_binary="$user_safety_runtime/remotr-vm-user-safety.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$user_safety_binary" ./internal/applicators/users
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$user_safety_binary" /tmp/remotr-vm-user-safety.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-user-safety.test /usr/local/lib/remotr-vm-user-safety.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-user-safety.test'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-user-safety.test -test.run '^TestUserRemovalSafetyVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-user-safety.test'
  )
  echo "user removal safety fixture verified"
}

failure_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$failure_runtime"
  then
    rm -rf "$failure_runtime"
  fi
  destroy || status=1
  exit "$status"
}

append_redacted_bounded() {
  local source=$1
  local destination=$2
  local canary=$3

  while IFS= read -r line || test -n "$line"
  do
    line=${line%$'\r'}
    printf '%s\n' "${line//"$canary"/[REDACTED]}" >> "$destination"
  done < <(head -c 8192 "$source")
}

failure_artifacts() {
  local artifact_dir artifact raw_output system_output token_file guest_status

  failure_runtime=$(mktemp -d)
  umask 077
  token_file="$failure_runtime/token"
  printf 'remotr-test-secret-failure-%s-%s\n' "$RANDOM" "$(date +%s)" > "$token_file"
  artifact_dir="$vagrant_dir/artifacts"
  mkdir -p "$artifact_dir"
  artifact=$(mktemp "$artifact_dir/failure.XXXXXX.log")
  raw_output="$failure_runtime/guest.raw"
  system_output="$failure_runtime/system.raw"
  trap failure_cleanup EXIT INT TERM

  up
  if (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$token_file" /tmp/remotr-vm-failure-token
    vagrant ssh -c 'sudo install -o root -g root -m 600 /tmp/remotr-vm-failure-token /run/remotr-vm-failure-token'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-failure-token'
    vagrant ssh -c 'sudo /workspace/test/vagrant/fixtures/failure-artifacts.sh --canary-file /run/remotr-vm-failure-token'
  ) > "$raw_output" 2>&1
  then
    echo "fixture failure unexpectedly succeeded" >&2
    exit 1
  else
    guest_status=$?
  fi
  test "$guest_status" -ne 0

  (
    cd "$vagrant_dir"
    vagrant ssh -c 'sudo rm -f /run/remotr-vm-failure-token'
    vagrant ssh -c 'printf "boot_id="; cat /proc/sys/kernel/random/boot_id; ip -4 route'
  ) > "$system_output" 2>&1 || true

  printf '%s\n' \
    'environment=vm' \
    "vagrant_exit=$guest_status" \
    'fixture_expected_exit=42' \
    'retention_limit_bytes=16384' \
    '--- guest failure ---' > "$artifact"
  append_redacted_bounded "$raw_output" "$artifact" "$(cat "$token_file")"
  printf '%s\n' '--- system diagnostics ---' >> "$artifact"
  append_redacted_bounded "$system_output" "$artifact" "$(cat "$token_file")"

  if grep -Fq "$(cat "$token_file")" "$artifact"
  then
    echo "failure artifact leaked synthetic canary" >&2
    exit 1
  fi
  grep -Fqx 'provider=fixture' "$artifact"
  grep -Fqx 'state_transition=failed' "$artifact"
  grep -Fqx 'safe_argv=/usr/bin/false --fixture-failure' "$artifact"
  grep -Fq 'boot_id=' "$artifact"
  test "$(wc -c < "$artifact")" -le 16384
  echo "failure artifact fixture verified: $artifact"
}

require_command vagrant
require_command virsh

case "${1:-}" in
  up) up ;;
  restore) restore ;;
  destroy) destroy ;;
  lifecycle) lifecycle ;;
  network-recovery) network_recovery ;;
  system-safety) system_safety ;;
  negative-safety) negative_safety ;;
	user-safety) user_safety ;;
  failure-artifacts) failure_artifacts ;;
  *)
    echo "usage: $0 {up|restore|destroy|lifecycle|network-recovery|system-safety|negative-safety|user-safety|failure-artifacts}" >&2
    exit 2
    ;;
esac
