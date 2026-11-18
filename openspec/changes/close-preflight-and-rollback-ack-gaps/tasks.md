## 1. Rollback acknowledgement

- [ ] 1.1 For OS-AEC-122/123/124, add failing file-provider tests (successful change then compliant Check, record armed by an earlier store instance, non-compliant record) at the provider seam; implement `rollbackstore.Handle.AcknowledgeConverged` and the file provider acknowledgement.
- [ ] 1.2 Add `executor.ConvergedRollbackAcknowledger`, call it from the engine after a compliant Check, and implement it on the `filetx`, `certificates`, `downloads`, and `trustanchors` providers; add an engine test proving a compliant Check acknowledges and a drifted one does not.

## 2. Preflight capability warnings

- [ ] 2.1 For OS-AEC-120/121, add a failing `remotr config validate` test for a default-risk bootstrap and a normal-risk resource; implement registry preflight capability metadata and the validation warning.
- [ ] 2.2 Add a registry contract test asserting the capability metadata matches each kind's handler type.
- [ ] 2.3 Log the same warnings during server composition.

## 3. Evidence and rollout

- [ ] 3.1 Trace OS-AEC-120..124, run `make test`, and verify on `stllr-elastic-collector-t2` that its stuck file records clear without changing file content.
