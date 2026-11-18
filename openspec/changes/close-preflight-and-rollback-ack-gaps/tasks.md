## 1. Rollback acknowledgement

- [x] 1.1 For OS-AEC-122/123/124, add failing file-provider tests (successful change then compliant Check, record armed by an earlier store instance, non-compliant record) at the provider seam; implement `rollbackstore.Handle.AcknowledgeConverged` and the file provider acknowledgement.
- [x] 1.2 Add `executor.ConvergedRollbackAcknowledger`, call it from the engine after a compliant Check, and implement it on the `certificates`, `downloads`, and `trustanchors` providers (`filetx` is a helper used by wrapper providers, a design non-goal); add an engine test proving a compliant Check acknowledges the file record (the non-compliant case is covered at the provider seam by OS-AEC-124).

## 2. Preflight capability warnings

- [x] 2.1 For OS-AEC-120/121, add a failing `remotr config validate` test for a default-risk bootstrap and a normal-risk resource; implement the validation warning.
- [x] 2.2 Derive preflight capability from the real provider type (`Resource.ProvidesPreflight`) so no static table or drift test is needed; the CLI test covers capable and incapable providers.
- [x] 2.3 Log the same warnings during server composition.

## 3. Evidence and rollout

- [x] 3.1 Trace OS-AEC-120..124, run `make test`, and verify on `stllr-elastic-collector-t2` that its stuck file records clear without changing file content.
