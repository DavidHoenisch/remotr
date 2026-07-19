# Disposable provider-safety VM

This directory contains the Vagrant/libvirt foundation for provider behavior
that containers cannot prove: reboot, network recovery, mounts, kernel state,
MAC policy, and authentication recovery.

The default provider guest is `generic/debian12`, pinned to `4.3.12`, for the
Debian-specific fixtures. The system-safety target overrides that default with
the amd64/libvirt `cloud-image/ubuntu-24.04` box pinned to `20260705.0.0`.
Its 128-GiB virtual capacity is explicit in the Vagrantfile; the qcow2 overlay
is sparse and is destroyed after each lifecycle. The NAT-only
`remotr-provider-safety` network uses the `remotrbr0` bridge and is never
attached to a contributor LAN. The guest has an explicit ten-minute boot
budget so slower cloud-image first boots and controlled reloads do not inherit
Vagrant's shorter implicit timeout.

Vagrant replaces the box's bootstrap key with a generated per-machine SSH key.
The harness checks that the key is mode `0600` and that teardown removes it;
no VM credential is committed to this repository.

## Host prerequisites

Install Vagrant, the `vagrant-libvirt` plugin, libvirt's system QEMU/network
services, `qemu-img`, and `qemu-system-x86`. The invoking user needs access to
`qemu:///system` through libvirt.

Hosts using UFW must allow DHCP from the isolated bridge and its outbound NAT
traffic before starting the VM. Replace `wlan0` with the host's default-route
interface when it differs:

```bash
sudo ufw allow in on remotrbr0 to any port 67 proto udp comment 'Remotr VM DHCP'
sudo ufw route allow in on remotrbr0 out on wlan0 comment 'Remotr VM NAT'
sudo ufw enable
```

The rules expose only libvirt's DHCP service to VMs on `remotrbr0`; they do not
open a service on the contributor LAN. Remove them when this harness is no
longer needed:

```bash
sudo ufw delete allow in on remotrbr0 to any port 67 proto udp
sudo ufw delete route allow in on remotrbr0 out on wlan0
```

## Lifecycle commands

```bash
make provider-matrix-vm-up
make provider-matrix-vm-restore
make provider-matrix-vm-destroy
make provider-matrix-vm-lifecycle
make provider-matrix-vm-network-recovery
make provider-matrix-vm-login-policy-safety
make provider-matrix-vm-system-safety
make provider-matrix-vm-negative-safety
make provider-matrix-vm-failure-artifacts
```

`provider-matrix-vm-lifecycle` proves the full lifecycle: it creates the
isolated network and VM, saves a baseline snapshot, verifies snapshot restore
by removing a post-snapshot probe, and verifies removal of the domain, sparse
overlay, network, and generated SSH key.

`provider-matrix-vm-network-recovery` starts a host-side controlled peer and
generates a one-time synthetic token. The guest independently breaks and
recovers its default route, resolver, outbound firewall, and control interface.
Each failure must block the control probe, each watchdog must restore it, and a
mode-`0600` report must contain all four recovery outcomes before the guest may
send the authenticated acknowledgement. This VM boundary complements the real
provider transaction and rollback contract tests; it does not replace them.

`provider-matrix-vm-system-safety` runs on the pinned Ubuntu 24.04 guest. It
first proves the boot-risk foundation: a disposable loopback mount, reversible
`net.ipv4.ip_forward` state, the `loop` module, AppArmor availability reporting,
and a synthetic recovery principal. It then arms real nftables connectivity,
authoritative authorized-key access, private-key certificate, and coordinated
reboot attempts before one controlled VM reboot. Reconstructed providers prove
timeout rollback and authenticated acknowledgement, exact access and secret
recovery, authorized-only abandonment, changed boot-ID completion, terminal
no-replay, and the required second Checks. It reports AppArmor as unavailable
when the guest kernel does not expose it rather than treating availability as
provider qualification.

`provider-matrix-vm-login-policy-safety` runs the real Debian
`pam-auth-update` provider against a benign provider-owned session profile,
exercises the declared recovery principal through the resulting PAM-backed
`su` stack, rolls the profile back, and verifies the recovery path again. This
is technical stack/recovery evidence and does not claim that a human login was
tested.

`provider-matrix-vm-negative-safety` completes the negative fixture suite.
The network-recovery target supplies its real control-path-loss case. This
target then proves that the fixture blocks removal of its sole synthetic sudo
recovery principal and an invalid boot proposal before mutation, refuses an
ambiguous pair of real disposable loop devices, and redacts a synthetic secret
canary from its retained diagnostic. These are test-harness contracts for
future destructive providers; they do not advertise user, boot, or storage
provider support that Remotr has not implemented.

`provider-matrix-vm-failure-artifacts` intentionally runs a failing guest
fixture. Before teardown, the harness writes a mode-`0600`, bounded diagnostic
under `test/vagrant/artifacts/`: redacted provider facts and state transitions,
safe argv, and guest boot/network diagnostics. It proves that the synthetic
canary emitted by the failed command is absent from the retained artifact.
Artifacts and Vagrant machine metadata are ignored by Git but remain available
for local failure triage.
