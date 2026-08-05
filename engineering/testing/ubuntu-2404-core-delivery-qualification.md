# Ubuntu 24.04 core delivery qualification

- **Date:** 2026-08-05
- **Verification:** OS-LPC-029
- **Public seam:** Production endpoint capability document generated from exact Ubuntu 24.04 LTS amd64, systemd, and APT facts
- **Provider seam:** Real provider contracts in the pinned Ubuntu 24.04 LTS amd64 Vagrant guest

## Red

`go test ./internal/capabilitydoc -run '^TestDefaultGeneratorPublishesQualifiedUbuntu2404CoreDelivery$' -count=1`

failed because the production capability document omitted these independently expected requirements:

- `resource:bootstrap@bootstrap-v1`
- `resource:command@command-v1`
- `resource:systemd@systemd-v1`

## Green

The provider matrix now contains three exact Ubuntu 24.04 LTS amd64 VM rows
selected by `make provider-matrix-vm-core-delivery-ubuntu-24-04`. The focused
production capability-document test passes.

The pinned `cloud-image/ubuntu-24.04@20260705.0.0` VM selector passed the
command, bootstrap, and systemd provider contracts. Each contract exercised an
initial drift observation, Apply, a compliant second Check, no-change replay,
and cleanup or revert behavior applicable to that provider. The harness then
removed the disposable VM and libvirt network.

No Ubuntu ARM, derivative, family, or other-release support is inferred by
these exact rows.
