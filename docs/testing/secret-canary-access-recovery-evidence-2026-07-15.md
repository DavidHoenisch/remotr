# Secret-canary and access-recovery evidence — 2026-07-15

This evidence run covers task 12.15 and verification ID OS-LSM-031. It uses
synthetic canaries only; no production credentials or endpoint data are part
of the fixtures.

## Apply, failure, rollback, and diagnostics

The certificate provider test injects a recognizable canary into an
untrusted secret-backend error, executes the provider through
`executor.ApplyState`, and observes a failed Apply plus an explicit no-op
rollback. The retained error is a typed, bounded resolution failure and does
not contain the backend diagnostic. The central resource registry applies the
same redaction boundary to string and byte secret resolvers used by repository,
schedule, user, certificate, and trust-anchor providers.

The focused red test failed because the complete backend diagnostic crossed
the Apply result. After the redaction boundary was added, these suites passed:

```sh
go test -mod=vendor ./internal/secrets ./internal/applicators/certificates ./internal/resourceregistry
go test -mod=vendor ./internal/applicators/aptrepositories ./internal/applicators/downloads \
  ./internal/applicators/endpointschedules/cron \
  ./internal/applicators/endpointschedules/systemdtimer \
  ./internal/applicators/trustanchors ./internal/applicators/users \
  ./internal/applicators/certificates
go test -mod=vendor ./internal/agent/engine \
  -run '^(TestEngineReportsServiceActionFailureWithoutLeakingStderr|Test.*Secret.*|Test.*Redact.*)$'
```

## Sync, storage, API, and CLI

The following focused suites passed:

```sh
go test -mod=vendor ./internal/server \
  -run '^(TestSecretCanaryIsAbsentFromLogsSyncAPIAndCLI|TestAdminSecretUploadReturnsOnlyInactiveMetadataAndRefusesPlaintextReadback|TestAdminSecretActivationCreatesConnectivityChangeBeforeResolution)$'
go test -mod=vendor ./internal/store/postgres ./cmd/remotr ./cmd/remotr-agent \
  -run '^(TestStorePersistsEncryptedSecretEnvelopeWithoutExternalKEK|TestSecretUploadReadsProtectedInputFileAndNeverAcceptsArgvMaterial|TestSyncRunStateRedactsRebootCommandFailure)$'
```

A disposable local Compose Postgres instance then ran the build-tagged storage
integration test:

```sh
go test -mod=vendor -tags=e2e ./test/e2e \
  -run '^TestStateReportRedactionPersistsNoDesiredSecretCanary$'
```

It verified canary absence in the agent's pending report, the persisted
`drift_reports.report_json` value, the store model readback, and the serialized
API model. The Compose stack and its volumes were removed after the run, and
the tracked runtime fixture was restored.

## Access recovery and real-system safety

The existing PAM/login-policy VM suite had already passed in the immediately
preceding task 12.12 run, covering activation failure, rollback, and a PAM-backed
recovery principal. Task 12.15 additionally ran:

```sh
make provider-matrix-vm-negative-safety
```

The disposable Debian Vagrant/libvirt guest verified that the last
administrative recovery principal is blocked before mutation and that its
retained provider diagnostic contains a bounded redaction marker instead of
the synthetic canary. It also verified the boot and ambiguous-device negative
safety guards. The guest, overlay disk, and libvirt network were destroyed by
the harness after the passing run.
