# Optional Workload Management Specification

## Purpose

Define capability-gated contracts and graduation criteria for optional Linux workload-management resources.

## Requirements

### Requirement: Optional workload features are capability-gated
Container, alternatives, file-capability, environment-policy, and transient-path resources SHALL be rejected unless the endpoint/provider advertises the specific optional capability. They SHALL not block completion of the core M1–M5 roadmap.

#### Scenario: Optional provider is absent
<!-- verification-id: OS-OWM-001 -->
- **WHEN** configuration selects a container provider not advertised for the fleet
- **THEN** release validation rejects the resource without affecting core resource support

### Requirement: M6 providers are roadmap-only in this change
Archive/VCS deployment, destructive storage, containers, alternatives, Linux file capabilities, environment fragments, and transient paths SHALL remain unadvertised by this implementation change. Each SHALL require a demand-backed child OpenSpec change with a concrete Fleet use case, provider and ownership boundaries, security review, maintenance ownership, and suitable integration tests before promotion.

#### Scenario: Roadmap resource is authored
<!-- verification-id: OS-OWM-002 -->
- **WHEN** configuration selects an M6 resource before a child OpenSpec change advertises it
- **THEN** validation rejects it with a roadmap diagnostic rather than constructing a placeholder provider

#### Scenario: Contributor wants to promote an M6 resource
<!-- verification-id: OS-OWM-003 -->
- **WHEN** a concrete Fleet use case demonstrates that existing primitives cannot safely compose the outcome
- **THEN** contributors create a focused child OpenSpec change and satisfy its provider, security, ownership, maintenance, and test gates

### Requirement: Container state is provider-backed
When advertised, container resources SHALL explicitly manage provider, image digest/tag policy, lifecycle, command, environment secret references, networks, volumes, restart/auto-update policy, health, and ownership. Shell wrappers SHALL not count as a container provider.

#### Scenario: Image tag moves under digest policy
<!-- verification-id: OS-OWM-004 -->
- **WHEN** a container requires a pinned digest and the running container uses another digest
- **THEN** Check reports image drift and Apply follows the declared replacement/activation policy

### Requirement: Registry authentication is secret-backed
Container registry credentials SHALL use secret references and provider-specific protected auth storage; credentials SHALL not appear in image references, argv, logs, or reports.

#### Scenario: Registry pull fails authentication
<!-- verification-id: OS-OWM-005 -->
- **WHEN** an authenticated image pull is denied
- **THEN** Apply reports registry authorization failure without exposing the credential

### Requirement: Container replacement preserves declared data
Volume and bind-mount ownership SHALL be explicit, and container replacement/removal SHALL not delete persistent data unless a separately authorized lifecycle policy requests it.

#### Scenario: Container is absent
<!-- verification-id: OS-OWM-006 -->
- **WHEN** a container declares absent and delete-volumes is false
- **THEN** Apply removes the container while preserving declared volumes

### Requirement: Alternatives selection converges
When advertised, alternatives resources SHALL manage a named alternatives group, selected target, priority where provider-supported, and auto/manual mode.

#### Scenario: Wrong alternative is selected
<!-- verification-id: OS-OWM-007 -->
- **WHEN** manual mode specifies a target different from the active alternative
- **THEN** Check reports drift and Apply selects the requested registered target

### Requirement: Linux file capabilities are explicit
When advertised, file-capability resources SHALL manage an authoritative or merge set of capabilities on a validated regular file and SHALL distinguish absent capability state from an absent file.

#### Scenario: Extra capability exists in authoritative mode
<!-- verification-id: OS-OWM-008 -->
- **WHEN** a file carries a capability outside the authoritative desired set
- **THEN** Check reports drift and Apply removes the extra capability without changing file content

### Requirement: Environment policy has owned scope
When advertised, environment resources SHALL manage named variables, target system/user/service scope, literal or secret-reference value, presence, and a provider-owned fragment. They SHALL not rewrite unrelated global environment configuration.

#### Scenario: Secret environment value changes
<!-- verification-id: OS-OWM-009 -->
- **WHEN** a secret-backed variable differs
- **THEN** Check reports safe secret drift and Apply changes protected provider storage without exposing the value

### Requirement: Transient paths use declarative lifecycle
When advertised, transient-path resources SHALL manage tmpfiles-style directory/file/link creation, ownership, mode, age cleanup, and removal through validated provider fragments.

#### Scenario: Cleanup age is invalid
<!-- verification-id: OS-OWM-010 -->
- **WHEN** transient-path policy contains an unsupported age expression
- **THEN** validation fails before installing the fragment

### Requirement: Optional resources meet the common contract
Every optional provider SHALL pass the same structured outcome, idempotence, locking, redaction, rollback disclosure, documentation, and integration requirements as core applicators before advertisement.

#### Scenario: Provider only implements Apply
<!-- verification-id: OS-OWM-011 -->
- **WHEN** an optional provider cannot accurately Check every accepted field
- **THEN** its capability remains unadvertised and configuration validation rejects it

### Requirement: Optional graduation is evidence-based
An optional resource SHALL graduate into the required roadmap only through a proposal update that records fleet demand, supported provider matrix, security review, and maintenance ownership.

#### Scenario: New optional primitive is proposed
<!-- verification-id: OS-OWM-012 -->
- **WHEN** contributors want to make it part of the core baseline
- **THEN** the OpenSpec change is updated before implementation is treated as required scope
