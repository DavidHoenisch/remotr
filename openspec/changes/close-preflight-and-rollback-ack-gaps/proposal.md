## Why

Two gaps leave resources permanently stuck on real endpoints:

- The engine refuses to apply any resource whose effective risk is not
  `normal` unless its provider implements a preflight. Many kinds default to
  such a risk while their providers implement no preflight (bootstrap,
  command, critical file, user, mount, certificate, and others). On
  `stllr-elastic-collector-t2` this blocked `install-elastic-agent`,
  `sshd-hardening-conf`, `load-audit-rules`, and other resources indefinitely,
  and configuration authors had no signal that the resources could never
  apply.
- Rollback-advertising providers arm a durable record before mutation, but
  nothing acknowledges it after a successful change, so the record stays armed
  and blocks every later mutation of that resource (`rollback_reservation_failed`).
  Every endpoint in the fleet has 3–8 such blocked file resources.

## What Changes

- `remotr config validate` and server composition warn for every composed
  resource whose effective risk requires a preflight that its provider does not
  implement, naming the resource, its risk, and the remedy. The engine's
  preflight requirement is unchanged.
- After a provider's Check proves a resource compliant, the agent acknowledges
  that resource's armed rollback records, transitioning them out of the armed
  state without restoring prior content. A non-compliant resource keeps its
  armed record and remains blocked for recovery as before. Restart-safe revert
  between Apply and the next compliant Check is unchanged.

## Capabilities

### New Capabilities

None.

### Modified Capabilities
- `applicator-execution-contract`: adds a configuration warning for
  unsatisfiable preflight requirements and defines when a converged
  rollback-advertising resource's armed transaction is acknowledged.

## Impact

- `internal/resourceregistry` (provider preflight capability per kind),
  `internal/configrepo` validation warnings, `internal/server` composition log.
- `internal/rollbackstore` (`Handle.AcknowledgeConverged`), the engine Check
  path, and the `files`, `filetx`, `certificates`, `downloads`, and
  `trustanchors` providers.
- Existing stuck records clear on the first compliant Check after agents
  upgrade; no endpoint-side manual cleanup is required.
