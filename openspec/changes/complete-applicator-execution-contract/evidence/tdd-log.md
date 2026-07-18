# TDD evidence log

## Task 2.1 — OS-AEC-080 restart recovery

- Public seam: system-safety recovery through `rollbackstore.New`, mutation
  admission, and `RecoverArmed`.
- Selected evidence: focused restart contract; corrupt-record and failed-provider
  negatives; rollback-store and network-state package regressions. Provider and
  Ubuntu VM interruption evidence remains owned by tasks 2.7 through 2.9 and
  scenario promotion remains blocked until task 5.6.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRecoversArmedTransactionAfterRestart -count=1`.
- Intended red observed: the test did not compile because `ErrArmedRecovery`,
  `Recovery`, and `RecoverArmed` did not exist.
- Green command: the same focused command passed after adding lifecycle version
  1, startup validation, per-resource blocking, recovery delivery, terminal
  transition, and payload cleanup.
- Negative evidence: the package suite proves corrupt legacy ciphertext blocks
  startup and a failed recovery callback remains armed, retryable, and absent
  from the safe returned diagnostic.

## Task 2.2 — OS-AEC-069 pre-mutation reservation

- Public seam: provider-contract reservation through `Store.Reserve` and the
  single-use `Reservation.Arm` handle.
- Selected evidence: focused configured-cap refusal, protected armed-record
  preservation, ciphertext/metadata/filesystem boundary cases, outstanding
  reservation accounting, and oversized-payload cleanup.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreReservesCompleteRecoveryBeforeMutation -count=1`.
- Intended red observed: the test did not compile because `AvailableBytes`,
  `ReservationRequest`, and `Store.Reserve` did not exist.
- Green command: the same focused command passed after reservation began
  accounting for AEAD overhead, encoded metadata, filesystem allowance,
  configured usage, available filesystem bytes, protected armed records, and
  concurrent outstanding reservations before `Arm` permits mutation.

## Task 2.3 — OS-AEC-081 deterministic retention

- Public seam: transaction retention through `Store.Cleanup` and the
  payload-free `Store.Records` metadata view.
- Selected evidence: exact attempt and successful-prior-state limits, exact
  age boundary, sensitive expiry, incomplete-attempt supersession, configured
  disk pressure, armed-record preservation, and a deterministic 0–20 attempt
  property loop under an injected clock.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRetentionPrunesAttemptsAndSuccessfulPayloads -count=1`.
- Intended red observed: the test did not compile because `Store.Cleanup` and
  `Store.Records` did not exist.
- Green command: the focused command passed after retention began keeping at
  most the configured attempt metadata and three successful non-secret prior
  payloads per resource.
- Boundary evidence: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRetention -count=1` passed for age, armed, sensitivity,
  supersession, disk, and bounded-count cases without wall-clock sleeps.

## Task 2.4 — OS-AEC-080 atomic transaction envelope

- Public seam: system-safety recovery through durable `Store.Save`, startup
  validation in `rollbackstore.New`, and `RecoverArmed` after an injected
  process stop.
- Selected evidence: deterministic injection after temporary-file creation,
  envelope write, envelope fsync, atomic activation, and directory fsync;
  authenticated metadata and ciphertext tamper negatives; legacy split-record
  migration; rollback-store and network-state package regressions.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreActivatesOneEnvelopeAtEveryDurabilityBoundary -count=1`.
- Intended red observed: the test did not compile because `DurabilityPoint`,
  its five durability-boundary constants, and `Options.CrashInjector` did not
  exist.
- Green command: the same focused command passed after payload and lifecycle
  metadata were sealed into one checksummed AEAD envelope, fsynced, atomically
  renamed, and followed by a parent-directory fsync. Stops before activation
  leave no recoverable transaction; stops after activation leave exactly one
  complete recoverable transaction.
- Negative and migration evidence: `TestEnvelopeAuthenticatesMetadataAndCiphertext`
  rejects modified headers and ciphertext at startup without leaking the
  payload, and `TestLegacySplitRecordCompatibilityFixture` migrates the frozen
  split-format fixture to one envelope while corrupt legacy ciphertext remains
  startup-blocking.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.
