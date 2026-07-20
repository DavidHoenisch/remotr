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
login_policy_safety_runtime=
kernel_module_safety_runtime=
host_locale_runtime=
time_sync_runtime=
mount_runtime=
reboot_safety_runtime=

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
    vagrant ssh -c "sudo /workspace/test/vagrant/fixtures/network-recovery.sh --health-url $control_url/health --ack-url $control_url/ack --token-file /run/remotr-vm-recovery-token --report /tmp/remotr-network-recovery.report"
    vagrant ssh -c 'sudo grep -Fqx route_recovery=verified /tmp/remotr-network-recovery.report'
    vagrant ssh -c 'sudo grep -Fqx dns_recovery=verified /tmp/remotr-network-recovery.report'
    vagrant ssh -c 'sudo grep -Fqx firewall_recovery=verified /tmp/remotr-network-recovery.report'
    vagrant ssh -c 'sudo grep -Fqx profile_recovery=verified /tmp/remotr-network-recovery.report'
  )
  grep -qx acknowledged "$result_file"
  echo "network recovery fixture verified"
}

system_safety_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$reboot_safety_runtime"
  then
    rm -rf "$reboot_safety_runtime"
  fi
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
  require_command go

  export REMOTR_VM_BOX=cloud-image/ubuntu-24.04
  export REMOTR_VM_BOX_VERSION=20260705.0.0
  export REMOTR_VM_HOSTNAME=remotr-ubuntu-system-safety
  reboot_safety_runtime=$(mktemp -d)
  trap system_safety_cleanup EXIT INT TERM
  reboot_safety_binary="$reboot_safety_runtime/remotr-vm-reboot-safety.test"
  firewall_safety_binary="$reboot_safety_runtime/remotr-vm-firewall-recovery.test"
  access_safety_binary="$reboot_safety_runtime/remotr-vm-access-recovery.test"
  certificate_safety_binary="$reboot_safety_runtime/remotr-vm-certificate-recovery.test"
  sysctl_safety_binary="$reboot_safety_runtime/remotr-vm-sysctl-safety.test"
  hostname_safety_binary="$reboot_safety_runtime/remotr-vm-hostname-safety.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$reboot_safety_binary" ./internal/applicators/reboots
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$firewall_safety_binary" ./internal/applicators/firewall
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$access_safety_binary" ./internal/applicators/authorizedkeys
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$certificate_safety_binary" ./internal/applicators/certificates
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$sysctl_safety_binary" ./internal/applicators/sysctl
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$hostname_safety_binary" ./internal/applicators/hostname
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$reboot_safety_binary" /tmp/remotr-vm-reboot-safety.test
    vagrant upload "$firewall_safety_binary" /tmp/remotr-vm-firewall-recovery.test
    vagrant upload "$access_safety_binary" /tmp/remotr-vm-access-recovery.test
    vagrant upload "$certificate_safety_binary" /tmp/remotr-vm-certificate-recovery.test
    vagrant upload "$sysctl_safety_binary" /tmp/remotr-vm-sysctl-safety.test
    vagrant upload "$hostname_safety_binary" /tmp/remotr-vm-hostname-safety.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-reboot-safety.test /usr/local/lib/remotr-vm-reboot-safety.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-firewall-recovery.test /usr/local/lib/remotr-vm-firewall-recovery.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-access-recovery.test /usr/local/lib/remotr-vm-access-recovery.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-certificate-recovery.test /usr/local/lib/remotr-vm-certificate-recovery.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-sysctl-safety.test /usr/local/lib/remotr-vm-sysctl-safety.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-hostname-safety.test /usr/local/lib/remotr-vm-hostname-safety.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-reboot-safety.test /tmp/remotr-vm-firewall-recovery.test /tmp/remotr-vm-access-recovery.test /tmp/remotr-vm-certificate-recovery.test /tmp/remotr-vm-sysctl-safety.test /tmp/remotr-vm-hostname-safety.test'
    vagrant ssh -c '. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04'
    vagrant ssh -c 'sudo /workspace/test/vagrant/fixtures/system-safety.sh --report /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo test -s /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx reboot_pre_ack=ready /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-sysctl-safety.test -test.run '^TestSysctlProviderContractVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-hostname-safety.test -test.run '^TestHostnameProviderContractVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_ACCESS_VM_PHASE=prepare REMOTR_ACCESS_VM_STATE_DIR=/var/lib/remotr-vm-access-safety /usr/local/lib/remotr-vm-access-recovery.test -test.run '^TestAuthorizedKeyInterruptedRecoveryVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_CERTIFICATE_VM_PHASE=prepare REMOTR_CERTIFICATE_VM_STATE_DIR=/var/lib/remotr-vm-certificate-safety /usr/local/lib/remotr-vm-certificate-recovery.test -test.run '^TestCertificateSecretInterruptedRecoveryVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_FIREWALL_VM_PHASE=prepare REMOTR_FIREWALL_VM_STATE_DIR=/var/lib/remotr-vm-firewall-safety /usr/local/lib/remotr-vm-firewall-recovery.test -test.run '^TestFirewallInterruptedRecoveryVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_REBOOT_VM_PHASE=prepare REMOTR_REBOOT_VM_STATE_DIR=/var/lib/remotr-vm-reboot-safety /usr/local/lib/remotr-vm-reboot-safety.test -test.run '^TestCoordinatedRebootSafetyVM$' -test.count=1"
    vagrant ssh -c "sudo sh -c 'printf \"guest=ubuntu-24.04\\nconnectivity_interruption=armed\\naccess_interruption=armed\\nsecret_interruption=armed\\n\" >> /var/lib/remotr-vm-system-safety/report'"
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
  (
    cd "$vagrant_dir"
    vagrant ssh -c "sudo env REMOTR_REBOOT_VM_PHASE=verify REMOTR_REBOOT_VM_STATE_DIR=/var/lib/remotr-vm-reboot-safety /usr/local/lib/remotr-vm-reboot-safety.test -test.run '^TestCoordinatedRebootSafetyVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_FIREWALL_VM_PHASE=verify REMOTR_FIREWALL_VM_STATE_DIR=/var/lib/remotr-vm-firewall-safety /usr/local/lib/remotr-vm-firewall-recovery.test -test.run '^TestFirewallInterruptedRecoveryVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_ACCESS_VM_PHASE=verify REMOTR_ACCESS_VM_STATE_DIR=/var/lib/remotr-vm-access-safety /usr/local/lib/remotr-vm-access-recovery.test -test.run '^TestAuthorizedKeyInterruptedRecoveryVM$' -test.count=1"
    vagrant ssh -c "sudo env REMOTR_CERTIFICATE_VM_PHASE=verify REMOTR_CERTIFICATE_VM_STATE_DIR=/var/lib/remotr-vm-certificate-safety /usr/local/lib/remotr-vm-certificate-recovery.test -test.run '^TestCertificateSecretInterruptedRecoveryVM$' -test.count=1"
    vagrant ssh -c "sudo sh -c 'printf \"boot_restart_recovery=verified\\nboot_acknowledgement=verified\\nboot_second_check=compliant\\nconnectivity_restart_recovery=verified\\nconnectivity_timeout_rollback=verified\\nconnectivity_acknowledgement=authenticated\\nconnectivity_second_check=compliant\\naccess_restart_recovery=verified\\naccess_second_check=drifted-after-rollback\\nsecret_restart_recovery=verified\\nsecret_abandonment=authorized-only\\nsecret_second_check=drifted-after-rollback\\n\" >> /var/lib/remotr-vm-system-safety/report'"
    vagrant ssh -c 'sudo grep -Fqx guest=ubuntu-24.04 /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx boot_restart_recovery=verified /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx boot_acknowledgement=verified /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx boot_second_check=compliant /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx connectivity_restart_recovery=verified /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx connectivity_timeout_rollback=verified /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx connectivity_acknowledgement=authenticated /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx connectivity_second_check=compliant /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx access_restart_recovery=verified /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx access_second_check=drifted-after-rollback /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx secret_restart_recovery=verified /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx secret_abandonment=authorized-only /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo grep -Fqx secret_second_check=drifted-after-rollback /var/lib/remotr-vm-system-safety/report'
    vagrant ssh -c 'sudo rm -rf /var/lib/remotr-vm-reboot-safety /var/lib/remotr-vm-system-safety /usr/local/lib/remotr-vm-reboot-safety.test /usr/local/lib/remotr-vm-firewall-recovery.test /usr/local/lib/remotr-vm-access-recovery.test /usr/local/lib/remotr-vm-certificate-recovery.test /usr/local/lib/remotr-vm-sysctl-safety.test /usr/local/lib/remotr-vm-hostname-safety.test'
  )
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

  export REMOTR_VM_BOX=cloud-image/ubuntu-24.04
  export REMOTR_VM_BOX_VERSION=20260705.0.0
  export REMOTR_VM_HOSTNAME=remotr-ubuntu-user-safety
  user_safety_runtime=$(mktemp -d)
  trap user_safety_cleanup EXIT INT TERM
  user_safety_binary="$user_safety_runtime/remotr-vm-user-safety.test"
  group_safety_binary="$user_safety_runtime/remotr-vm-group-safety.test"
  authorized_key_safety_binary="$user_safety_runtime/remotr-vm-authorized-key-safety.test"
  sudo_safety_binary="$user_safety_runtime/remotr-vm-sudo-safety.test"
  user_file_safety_binary="$user_safety_runtime/remotr-vm-user-file-safety.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$user_safety_binary" ./internal/applicators/users
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$group_safety_binary" ./internal/applicators/groups
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$authorized_key_safety_binary" ./internal/applicators/authorizedkeys
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$sudo_safety_binary" ./internal/applicators/sudo
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$user_file_safety_binary" ./internal/applicators/userfiles
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$user_safety_binary" /tmp/remotr-vm-user-safety.test
    vagrant upload "$group_safety_binary" /tmp/remotr-vm-group-safety.test
    vagrant upload "$authorized_key_safety_binary" /tmp/remotr-vm-authorized-key-safety.test
    vagrant upload "$sudo_safety_binary" /tmp/remotr-vm-sudo-safety.test
    vagrant upload "$user_file_safety_binary" /tmp/remotr-vm-user-file-safety.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-user-safety.test /usr/local/lib/remotr-vm-user-safety.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-group-safety.test /usr/local/lib/remotr-vm-group-safety.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-authorized-key-safety.test /usr/local/lib/remotr-vm-authorized-key-safety.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-sudo-safety.test /usr/local/lib/remotr-vm-sudo-safety.test'
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-user-file-safety.test /usr/local/lib/remotr-vm-user-file-safety.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-user-safety.test /tmp/remotr-vm-group-safety.test /tmp/remotr-vm-authorized-key-safety.test /tmp/remotr-vm-sudo-safety.test /tmp/remotr-vm-user-file-safety.test'
    vagrant ssh -c '. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-group-safety.test -test.run '^TestGroupProviderContractVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-user-safety.test -test.run '^TestUserProviderContractVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-user-safety.test -test.run '^TestUserRemovalSafetyVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-authorized-key-safety.test -test.run '^TestAuthorizedKeyProviderContractVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-sudo-safety.test -test.run '^TestSudoProviderContractVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-user-file-safety.test -test.run '^TestUserFileProviderContractVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-user-safety.test /usr/local/lib/remotr-vm-group-safety.test /usr/local/lib/remotr-vm-authorized-key-safety.test /usr/local/lib/remotr-vm-sudo-safety.test /usr/local/lib/remotr-vm-user-file-safety.test'
  )
  echo "group, user, authorized-key, sudo, and user-file safety fixture verified"
}

login_policy_safety_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$login_policy_safety_runtime"
  then
    rm -rf "$login_policy_safety_runtime"
  fi
  destroy || status=1
  exit "$status"
}

login_policy_safety() {
  require_command go

  login_policy_safety_runtime=$(mktemp -d)
  trap login_policy_safety_cleanup EXIT INT TERM
  login_policy_safety_binary="$login_policy_safety_runtime/remotr-vm-login-policy-safety.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$login_policy_safety_binary" ./internal/applicators/loginpolicy
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$login_policy_safety_binary" /tmp/remotr-vm-login-policy-safety.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-login-policy-safety.test /usr/local/lib/remotr-vm-login-policy-safety.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-login-policy-safety.test'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-login-policy-safety.test -test.run '^TestLoginPolicyRecoverySafetyVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-login-policy-safety.test'
  )
  echo "login policy recovery safety fixture verified"
}

kernel_module_safety_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$kernel_module_safety_runtime"
  then
    rm -rf "$kernel_module_safety_runtime"
  fi
  destroy || status=1
  exit "$status"
}

kernel_module_safety() {
  require_command go

  export REMOTR_VM_BOX=cloud-image/ubuntu-24.04
  export REMOTR_VM_BOX_VERSION=20260705.0.0
  export REMOTR_VM_HOSTNAME=remotr-ubuntu-kernel-module-safety
  kernel_module_safety_runtime=$(mktemp -d)
  trap kernel_module_safety_cleanup EXIT INT TERM
  kernel_module_safety_binary="$kernel_module_safety_runtime/remotr-vm-kernel-module-safety.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$kernel_module_safety_binary" ./internal/applicators/kernelmodules
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$kernel_module_safety_binary" /tmp/remotr-vm-kernel-module-safety.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-kernel-module-safety.test /usr/local/lib/remotr-vm-kernel-module-safety.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-kernel-module-safety.test'
    vagrant ssh -c '. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-kernel-module-safety.test -test.run '^TestKernelModuleProviderContractVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-kernel-module-safety.test'
  )
  echo "kernel module safety fixture verified"
}

host_locale_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$host_locale_runtime"
  then
    rm -rf "$host_locale_runtime"
  fi
  destroy || status=1
  exit "$status"
}

host_locale() {
  require_command go

  export REMOTR_VM_BOX=cloud-image/ubuntu-24.04
  export REMOTR_VM_BOX_VERSION=20260705.0.0
  export REMOTR_VM_HOSTNAME=remotr-ubuntu-host-locale
  host_locale_runtime=$(mktemp -d)
  trap host_locale_cleanup EXIT INT TERM
  host_locale_binary="$host_locale_runtime/remotr-vm-host-locale.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$host_locale_binary" ./internal/applicators/hostlocale
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$host_locale_binary" /tmp/remotr-vm-host-locale.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-host-locale.test /usr/local/lib/remotr-vm-host-locale.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-host-locale.test'
    vagrant ssh -c '. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-host-locale.test -test.run '^TestHostLocaleNativeKeymapValidationVM$' -test.count=1"
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-host-locale.test -test.run '^TestHostLocaleProviderVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-host-locale.test'
  )
  echo "host locale provider fixture verified"
}

time_sync_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$time_sync_runtime"
  then
    rm -rf "$time_sync_runtime"
  fi
  destroy || status=1
  exit "$status"
}

time_sync() {
  require_command go

  export REMOTR_VM_BOX=cloud-image/ubuntu-24.04
  export REMOTR_VM_BOX_VERSION=20260705.0.0
  export REMOTR_VM_HOSTNAME=remotr-ubuntu-time-sync
  time_sync_runtime=$(mktemp -d)
  trap time_sync_cleanup EXIT INT TERM
  time_sync_binary="$time_sync_runtime/remotr-vm-time-sync.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$time_sync_binary" ./internal/applicators/timesync
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$time_sync_binary" /tmp/remotr-vm-time-sync.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-time-sync.test /usr/local/lib/remotr-vm-time-sync.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-time-sync.test'
    vagrant ssh -c '. /etc/os-release; test "$ID" = ubuntu; test "$VERSION_ID" = 24.04'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-time-sync.test -test.run '^TestTimeSyncProviderVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-time-sync.test'
  )
  echo "time-sync provider fixture verified"
}

mount_cleanup() {
  status=$?
  trap - EXIT INT TERM
  if test -n "$mount_runtime"
  then
    rm -rf "$mount_runtime"
  fi
  destroy || status=1
  exit "$status"
}

mount_provider() {
  require_command go

  mount_runtime=$(mktemp -d)
  trap mount_cleanup EXIT INT TERM
  mount_binary="$mount_runtime/remotr-vm-mount.test"
  (
    cd "$root"
    CGO_ENABLED=0 go test -mod=vendor -tags=vmsafety -c -o "$mount_binary" ./internal/applicators/mounts
  )

  up
  (
    cd "$vagrant_dir"
    vagrant rsync
    vagrant upload "$mount_binary" /tmp/remotr-vm-mount.test
    vagrant ssh -c 'sudo install -o root -g root -m 700 /tmp/remotr-vm-mount.test /usr/local/lib/remotr-vm-mount.test'
    vagrant ssh -c 'sudo rm -f /tmp/remotr-vm-mount.test'
    vagrant ssh -c "sudo /usr/local/lib/remotr-vm-mount.test -test.run '^TestMountProviderVM$' -test.count=1"
    vagrant ssh -c 'sudo rm -f /usr/local/lib/remotr-vm-mount.test'
  )
  echo "mount provider fixture verified"
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
  login-policy-safety) login_policy_safety ;;
  kernel-module-safety) kernel_module_safety ;;
  host-locale) host_locale ;;
  time-sync) time_sync ;;
	 mount) mount_provider ;;
  failure-artifacts) failure_artifacts ;;
  *)
    echo "usage: $0 {up|restore|destroy|lifecycle|network-recovery|system-safety|negative-safety|user-safety|login-policy-safety|kernel-module-safety|host-locale|time-sync|mount|failure-artifacts}" >&2
    exit 2
    ;;
esac
