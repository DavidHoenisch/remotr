# Service and Reboot Management Specification

## Purpose

Define convergent service, unit, activation, reboot, and recovery behavior for managed Linux endpoints.

## Requirements

### Requirement: Service intent is provider-neutral
Service resources SHALL independently manage enabled-at-boot, active, and masked/blocked state where supported, using an explicitly resolved provider such as systemd, OpenRC, or SysV.

#### Scenario: Provider lacks mask semantics
<!-- verification-id: OS-SRM-001 -->
- **WHEN** a service requests masked state on a provider without masking
- **THEN** validation or Check returns `unsupported` rather than approximating it with stopped state

### Requirement: Service operations distinguish persistent state and actions
Restart, reload, try-restart, and daemon-reload SHALL be activation actions, not steady-state booleans. Actions SHALL occur only when triggered by a successful dependency/resource change or explicitly requested job.

#### Scenario: Compliant service has restart trigger
<!-- verification-id: OS-SRM-002 -->
- **WHEN** a dependent configuration resource changes and emits restart for a compliant active service
- **THEN** the engine restarts the service after the dependency succeeds

### Requirement: Systemd units and drop-ins are first-class
Systemd unit resources SHALL manage named unit or drop-in content, ownership, mode, presence, validation, and daemon-reload without requiring a generic file plus command resource.

#### Scenario: Drop-in is absent
<!-- verification-id: OS-SRM-003 -->
- **WHEN** a managed drop-in declares absent
- **THEN** Apply removes only that drop-in, performs one daemon-reload, and reconverges affected service state as declared

### Requirement: Unit changes are validated before activation
The provider SHALL validate staged unit content and relevant references before replacing active unit files. Failed validation SHALL leave prior content and service state intact.

#### Scenario: Invalid unit directive
<!-- verification-id: OS-SRM-004 -->
- **WHEN** staged unit content fails systemd verification
- **THEN** Apply fails before daemon-reload or service restart

### Requirement: Activation signals are ordered and coalesced
The engine SHALL coalesce identical activation requests, order daemon-reload before reload/restart, and execute activation only after all producing resources required for that action succeed.

#### Scenario: Two drop-ins request restart
<!-- verification-id: OS-SRM-005 -->
- **WHEN** two successful resources request restart of the same unit
- **THEN** the engine performs at most one restart after both resources apply

### Requirement: Service failures preserve diagnostics safely
Service Check and Apply SHALL report provider, unit identity, operation, exit status, and bounded redacted diagnostics without leaking environment secrets.

#### Scenario: Restart fails
<!-- verification-id: OS-SRM-006 -->
- **WHEN** service restart exits unsuccessfully
- **THEN** Apply reports the restart failure and does not mark the producing resources fully activated

### Requirement: Reboot-required is separate from reboot execution
Any applicator SHALL be able to report `reboot-required`; only a reboot resource or authorized maintenance job SHALL initiate reboot.

#### Scenario: Package reports reboot requirement
<!-- verification-id: OS-SRM-007 -->
- **WHEN** package Apply succeeds and reports reboot required but no reboot resource is eligible
- **THEN** state reporting retains the reboot requirement and the endpoint remains running

### Requirement: Reboot is coordinated and acknowledged
A reboot resource SHALL support preconditions, delay, deadline, maintenance window, user/workload inhibition policy, pre-reboot sync acknowledgement, and timeout. Completion SHALL be verified using boot ID or equivalent monotonic boot identity after the agent reconnects.

#### Scenario: Reboot completes
<!-- verification-id: OS-SRM-008 -->
- **WHEN** an authorized reboot begins and the agent later reconnects with a different boot ID before deadline
- **THEN** the reboot resource reports successful completion

#### Scenario: Boot ID does not change
<!-- verification-id: OS-SRM-009 -->
- **WHEN** the agent reconnects after a reboot attempt with the same boot ID
- **THEN** the reboot is not marked successful and the attempt reason is reported

### Requirement: Reboot defers safely
If maintenance, workload, power, or connectivity preconditions are not met, reboot SHALL report `deferred` rather than repeatedly attempting or forcing the endpoint down.

#### Scenario: Active inhibitor blocks reboot
<!-- verification-id: OS-SRM-010 -->
- **WHEN** a declared inhibitor is active
- **THEN** the resource remains deferred until policy permits or deadline handling applies

### Requirement: Reboot does not self-repeat
The reboot workflow SHALL persist intent/attempt state across restart so the same desired artifact cannot trigger an endless reboot loop.

#### Scenario: Agent returns after successful reboot
<!-- verification-id: OS-SRM-011 -->
- **WHEN** the desired reboot generation was already acknowledged for the new boot ID
- **THEN** Check reports the reboot resource compliant
