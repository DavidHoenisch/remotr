## MODIFIED Requirements

### Requirement: Sudo policy uses validated fragments
Sudo resources SHALL create named Remotr-owned fragments with explicit presence, optional lexical priority (`0`–`99`), and structured policy declarations — supporting single-rule fields, multi-rule lists, `user:group` run-as targets, argument-scoped command specifications (`path`, `args`, `noArgs`, `negate`), and allowlisted typed `Defaults` hardening directives — without accepting raw sudoers text. Configuration validation SHALL reject duplicate effective fragment filenames within a configuration. The complete effective sudo configuration SHALL pass `visudo`-equivalent validation during both Preflight and Apply before activation, with redacted diagnostics that never reproduce staged policy content.

#### Scenario: Fragment would invalidate sudoers
<!-- verification-id: OS-LIA-010 -->
- **WHEN** staged sudo policy fails effective syntax validation
- **THEN** Preflight and Apply reject the candidate, leave the active policy unchanged, and report a redacted access-risk failure without leaking staged policy text

#### Scenario: Typed Defaults and multi-rule fragment with argument-safe commands
<!-- verification-id: OS-LIA-014 -->
- **WHEN** a sudo resource declares allowlisted `Defaults` options, multiple structured rules with `user:group` `runAs` targets, and `commandSpecs` using `noArgs` or `negate`
- **THEN** validation accepts the resource, renders canonical sudoers lines with explicit `""` zero-argument constraints and `!` negations, and rejects unsupported options or unsafe delimiters

#### Scenario: Fragment priority ordering and filename collision
<!-- verification-id: OS-LIA-015 -->
- **WHEN** a sudo resource sets `priority` in `0`–`99`
- **THEN** it manages `/etc/sudoers.d/<NN>-<name>`, and `remotr config validate` rejects two sudo resources in the same configuration that resolve to the same effective fragment filename

### Requirement: Privilege changes preserve recovery access
Authoritative SSH or sudo changes SHALL verify a declared recovery principal/path and SHALL default to report-only until access preflight succeeds. For sudo resources, Preflight SHALL verify that every declared recovery principal resolves locally, that the host sudoers configuration includes the managed fragment directory and passes staged `visudo` validation, and that every declared recovery principal retains verified effective sudo access after the staged change.

#### Scenario: Last administrative path would be removed
<!-- verification-id: OS-LIA-011 -->
- **WHEN** a change would remove or invalidate the only verified administrative access path for a declared recovery principal
- **THEN** Preflight and Apply block enforcement without modifying the active sudo policy

#### Scenario: Unprivileged local account declared as recovery principal
<!-- verification-id: OS-LIA-016 -->
- **WHEN** a sudo resource declares a local account that exists on the host but has no verified effective sudo grant
- **THEN** Preflight fails with a redacted diagnostic and blocks enforcement

#### Scenario: Missing visudo or include directive caught during Preflight
<!-- verification-id: OS-LIA-017 -->
- **WHEN** `/etc/sudoers` does not include the managed fragment directory or `visudo` rejects the staged effective configuration
- **THEN** non-enforcing Preflight fails before any Apply is attempted

## ADDED Requirements

### Requirement: Sudo precedence shadowing and unmanaged drop-in visibility
When a present sudo fragment is compliant, Check SHALL inspect active `/etc/sudoers.d/` entries lexically after the managed fragment and report redacted, bounded subresults if a later fragment matches a managed subject without causing a non-convergent Apply loop. When a sudo resource opts into `auditUnmanaged: true`, Check SHALL report unexpected active `/etc/sudoers.d/` files outside the resource's own fragment and `allowedUnmanagedFragments` as drift using redacted, bounded subresults, and Apply SHALL NOT delete or modify unmanaged fragments.

#### Scenario: Lexically later fragment shadows a managed subject
<!-- verification-id: OS-LIA-018 -->
- **WHEN** a managed sudo fragment is compliant and a lexically later active file in `/etc/sudoers.d/` grants rules to a managed subject
- **THEN** Check reports a compliant resource with a redacted `sudo_fragment_shadowed` subresult naming only the later fragment filename and without leaking fragment contents

#### Scenario: Opt-in audit detects unmanaged sudoers.d drop-ins
<!-- verification-id: OS-LIA-019 -->
- **WHEN** a sudo resource sets `auditUnmanaged: true` and an unexpected active file exists in `/etc/sudoers.d/` outside `allowedUnmanagedFragments`
- **THEN** Check reports `drifted` with reason code `sudo_unmanaged_fragments` and bounded redacted subresults naming only the unexpected filenames, while Apply leaves unmanaged files untouched
