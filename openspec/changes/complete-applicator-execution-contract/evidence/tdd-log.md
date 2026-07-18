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
