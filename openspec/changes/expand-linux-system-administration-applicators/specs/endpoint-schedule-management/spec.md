## ADDED Requirements

### Requirement: Endpoint schedules are distinct from server jobs
Persistent endpoint schedules SHALL use a separate resource kind and reporting model from server-dispatched cron work. Their contract SHALL state that endpoint execution continues without server connectivity.

#### Scenario: Endpoint is offline at due time
- **WHEN** a local schedule becomes due while the endpoint cannot reach Remotr
- **THEN** the local scheduler runs it according to native scheduler behavior without waiting for a server dispatch

### Requirement: Schedule backend is explicit
Each endpoint schedule SHALL select `cron` or `systemd-timer`, or a provider-neutral mode whose resolved backend is reported. Backend-specific fields SHALL be rejected by other backends.

#### Scenario: Systemd-only persistence field on cron
- **WHEN** a schedule selects cron and declares a systemd timer persistence field
- **THEN** validation rejects the incompatible field

### Requirement: Schedule identity and lifecycle are stable
Schedules SHALL manage `present`, `disabled`, or `absent` state by stable name and Remotr ownership marker, preserving unrelated cron entries and timer units.

#### Scenario: Schedule is removed
- **WHEN** a named schedule declares `absent`
- **THEN** Apply removes only its owned cron entry or timer/service units

### Requirement: Local schedule inputs are structured
Schedules SHALL declare schedule expression, execution user, argv or an explicitly selected shell command, working directory, environment references, timeout, overlap policy, and enabled state as supported by the backend.

#### Scenario: Argv command contains spaces
- **WHEN** an argv-based schedule is authored
- **THEN** the backend preserves argument boundaries without shell re-parsing

### Requirement: Schedule syntax and target identity are validated
The system SHALL validate schedule expressions, user existence or dependency, environment names, executable form, unit names, and backend availability before activation.

#### Scenario: Invalid cron expression
- **WHEN** a cron schedule does not contain a valid supported expression
- **THEN** author-time validation fails with the schedule address

### Requirement: Cron entries use owned fragments
Cron-backed schedules SHALL prefer a named `/etc/cron.d`-style fragment or a precisely marked user crontab entry with correct permissions and newline/syntax validation.

#### Scenario: Unrelated user crontab exists
- **WHEN** a Remotr schedule changes
- **THEN** Apply preserves all entries outside the resource's ownership marker

### Requirement: Systemd timers manage paired units
Systemd-timer schedules SHALL manage the timer and its paired service/drop-ins, validate them, daemon-reload once, and converge enablement and active state.

#### Scenario: Timer definition changes
- **WHEN** timer content changes
- **THEN** Apply stages and validates both units, activates them atomically as practical, daemon-reloads, and converges timer state

### Requirement: Secrets in schedules are references
Schedule environment secrets SHALL be resolved through the secret mechanism and SHALL not be embedded in unit/cron command lines, logs, drift, or world-readable environment files.

#### Scenario: Scheduled job needs credential
- **WHEN** a schedule references a credential
- **THEN** Apply uses a protected provider-supported credential mechanism and reports only the reference identifier

### Requirement: Schedule compliance is separate from run results
Schedule Check SHALL report configuration compliance. Execution history, exit status, and missed-run behavior SHALL be reported separately when runtime telemetry is supported.

#### Scenario: Last execution failed but configuration matches
- **WHEN** the local schedule definition is compliant and its last job run failed
- **THEN** schedule compliance remains compliant while runtime telemetry reports the failed execution

### Requirement: Overlap and persistence behavior are explicit
The resource SHALL define whether concurrent runs are allowed and, for capable backends, whether missed runs execute after downtime.

#### Scenario: Non-overlapping job is still running
- **WHEN** a second occurrence becomes due under non-overlap policy
- **THEN** the scheduler prevents a concurrent invocation and records the skipped or delayed occurrence when telemetry is supported

