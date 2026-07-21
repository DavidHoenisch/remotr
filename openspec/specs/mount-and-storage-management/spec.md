# Mount and Storage Management Specification

## Purpose

Define explicit, safety-gated lifecycle and activation behavior for Linux mounts, swap, and storage resources.

## Requirements

### Requirement: Mount runtime and persistence are independent
Mount resources SHALL separately manage configured-at-boot state and current mounted state, with source, target, filesystem type, normalized options, dump, pass, and lifecycle.

#### Scenario: Configured but not mounted
<!-- verification-id: OS-MSM-001 -->
- **WHEN** both scopes are required, the persistent declaration matches, and the target is not mounted
- **THEN** Check reports runtime drift and Apply mounts the target after preflight

#### Scenario: Persistent-only mount
<!-- verification-id: OS-MSM-002 -->
- **WHEN** configured is true and mounted is unmanaged
- **THEN** Apply updates the persistent declaration without changing current mount state

### Requirement: Mount declarations have explicit ownership
Persistent mounts SHALL use a provider-supported named ownership mechanism or a precisely managed entry. Removal SHALL affect only the declaration with the resource's stable identity.

#### Scenario: Unrelated fstab entry exists
<!-- verification-id: OS-MSM-003 -->
- **WHEN** a Remotr mount is removed
- **THEN** Apply preserves unrelated entries and comments outside the owned entry

### Requirement: Mount activation is preflighted
Before mounting, remounting, or unmounting, the provider SHALL validate the target, source availability where safely probeable, filesystem support, option syntax, directory readiness, and protected-path policy.

#### Scenario: Target is Remotr state directory
<!-- verification-id: OS-MSM-004 -->
- **WHEN** a mount change would hide or detach the active Remotr state directory
- **THEN** Apply is blocked as a control-path risk

### Requirement: Unmount semantics are explicit
Unmount behavior SHALL declare normal, lazy, or forced semantics, and busy-target handling. Destructive or forced unmount SHALL require explicit authorization.

#### Scenario: Normal unmount target is busy
<!-- verification-id: OS-MSM-005 -->
- **WHEN** the target cannot be normally unmounted
- **THEN** Apply fails without escalating to lazy or forced behavior unless explicitly declared

### Requirement: Swap lifecycle is first-class
Swap resources SHALL manage swap files or devices with independent active and persistent state, priority, size for created files, mode/ownership, and safe removal behavior.

#### Scenario: Swap file must be created
<!-- verification-id: OS-MSM-006 -->
- **WHEN** a managed swap file is absent
- **THEN** Apply creates it without sparse-hole hazards, sets protected permissions, formats it, and activates it according to desired scopes

### Requirement: Root and boot-critical storage are protected
Changes affecting root, boot, the agent binary/state, or the active control path SHALL require boot-risk preflight, maintenance authorization, and a verified recovery plan.

#### Scenario: Root mount options require reboot
<!-- verification-id: OS-MSM-007 -->
- **WHEN** a safe persistent root option changes but cannot be activated online
- **THEN** Apply records `reboot-required` or `next-boot` and does not attempt an unsafe live remount

### Requirement: Destructive storage is demand-gated
Filesystem creation, partitioning, LVM, RAID, encryption, and quota resources SHALL not be advertised until an explicit approval workflow, inventory preconditions, audit plan, recovery metadata, and VM integration suite exist.

#### Scenario: Destructive capability is unavailable
<!-- verification-id: OS-MSM-008 -->
- **WHEN** configuration authors request partition creation before the endpoint advertises the destructive-storage capability
- **THEN** release validation rejects the resource

### Requirement: Destructive changes bind to stable device identity
When destructive storage is advertised, resources SHALL identify devices by stable identifiers and SHALL fail closed on ambiguity, size/model mismatch, mounted use, or unexpected existing signatures.

#### Scenario: Device identity no longer matches
<!-- verification-id: OS-MSM-009 -->
- **WHEN** a requested device path resolves to hardware different from the approved inventory identity
- **THEN** Apply is blocked without modifying the device

### Requirement: Storage outcomes include deferred maintenance
Storage Check and Apply SHALL distinguish ordinary drift, blocked destructive work, deferred maintenance-window work, reboot-required state, and failure.

#### Scenario: Authorized change is outside window
<!-- verification-id: OS-MSM-010 -->
- **WHEN** a destructive or boot-risk change is otherwise valid but outside its maintenance window
- **THEN** the resource reports `deferred` with the next eligible window
