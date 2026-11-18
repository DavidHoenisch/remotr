## ADDED Requirements

### Requirement: Unsatisfiable preflight requirements are reported to authors
Configuration validation and server composition SHALL warn for every composed resource whose effective risk requires a preflight that its selected provider does not implement. The warning SHALL name the resource address, its effective risk, and the remedy (declare a lower risk where the change is routine, or use a provider that supports preflight). Warnings SHALL NOT fail validation or block release, and the engine SHALL continue to refuse to apply such a resource without a provider preflight.

#### Scenario: Bootstrap left at its default risk
<!-- verification-id: OS-AEC-120 -->
- **WHEN** a configuration declares a bootstrap resource without an explicit risk
- **THEN** `remotr config validate` reports a warning naming the resource, risk `boot`, and the remedy, and still exits successfully

#### Scenario: Resource with a preflight-capable provider
<!-- verification-id: OS-AEC-121 -->
- **WHEN** a configuration declares a non-normal-risk resource whose provider implements a preflight, or a normal-risk resource of any kind
- **THEN** validation reports no preflight warning for it

### Requirement: Converged rollback transactions are acknowledged
When a rollback-advertising provider's Check reports a resource compliant, the agent SHALL acknowledge every armed rollback record for that resource address, regardless of the artifact digest or attempt that armed it, so that a converged resource no longer blocks later mutation. Acknowledgement SHALL NOT restore prior content. A resource whose Check is not compliant SHALL keep its armed records and remain blocked pending recovery. Revert after a restart SHALL remain available until the first compliant Check.

#### Scenario: Successful change is later checked compliant
<!-- verification-id: OS-AEC-122 -->
- **WHEN** a file resource applies successfully and a later Check reports it compliant
- **THEN** its rollback record is no longer armed and the next rollback preflight for that resource succeeds

#### Scenario: Record armed by an earlier agent process
<!-- verification-id: OS-AEC-123 -->
- **WHEN** an armed record left by an earlier agent process exists for a resource whose current content is compliant
- **THEN** the Check acknowledges it without changing the managed file, and the resource is no longer reported as `rollback_reservation_failed`

#### Scenario: Interrupted change is not compliant
<!-- verification-id: OS-AEC-124 -->
- **WHEN** an armed record exists for a resource whose current content is not compliant
- **THEN** the record remains armed and rollback preflight for that resource still fails
