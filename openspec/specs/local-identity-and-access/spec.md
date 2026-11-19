# Local Identity and Access Specification

## Purpose

Define safe, convergent local identity, authentication, authorization, and recovery-access management.

## Requirements

### Requirement: Local groups are convergent resources
Group resources SHALL manage presence, name, optional fixed GID, system-group class, and explicitly owned membership semantics under the account-database lock.

#### Scenario: Fixed GID differs
<!-- verification-id: OS-LIA-001 -->
- **WHEN** a present group exists with a different GID
- **THEN** Check reports drift and Apply changes it only when safe reassignment is explicitly allowed

### Requirement: Local users expose complete account state
User resources SHALL support presence, username, UID, primary group, supplementary groups, home path and creation policy, shell, comment, system-account class, password/lock state, expiry, and removal policy. Every specified field SHALL be checked.

#### Scenario: Requested UID differs
<!-- verification-id: OS-LIA-002 -->
- **WHEN** a user exists with a UID different from the managed UID
- **THEN** Check reports drift and Apply follows the explicit reassignment policy rather than treating presence as compliant

#### Scenario: Shell is unmanaged
<!-- verification-id: OS-LIA-003 -->
- **WHEN** shell is omitted
- **THEN** the existing shell does not affect compliance

### Requirement: Group membership ownership is explicit
Supplementary membership SHALL declare merge or authoritative semantics. Authoritative membership SHALL remove only memberships inside its declared user/group ownership boundary.

#### Scenario: Merge membership
<!-- verification-id: OS-LIA-004 -->
- **WHEN** a user requests groups in merge mode and already belongs to another group
- **THEN** Apply adds missing requested groups and preserves the unrelated membership

### Requirement: Account removal is safety-controlled
User absence SHALL explicitly declare whether to retain or remove the home, mail spool, processes, and owned files. Protected system accounts and the active Remotr runtime identity SHALL not be removed without an explicit supported override and recovery preflight.

#### Scenario: User removal omits home policy
<!-- verification-id: OS-LIA-005 -->
- **WHEN** a user declares absent without home removal
- **THEN** Apply removes the account while retaining home content

### Requirement: Passwords use secret references
Password hashes, unlock credentials, and other authentication secrets SHALL be supplied only through secret references. Reports SHALL expose only lock/expiry/health and safe hash metadata.

#### Scenario: Password hash changes
<!-- verification-id: OS-LIA-006 -->
- **WHEN** the referenced desired hash differs from the local account hash
- **THEN** Check reports password drift without revealing either hash and Apply updates it through a non-logging path

### Requirement: Authorized keys are structured entries
Authorized-key resources SHALL manage key type/data fingerprint, comment, options/restrictions, principals where applicable, expiry metadata, target user, and presence. The resource SHALL support merge and authoritative-set ownership.

#### Scenario: Key options differ
<!-- verification-id: OS-LIA-007 -->
- **WHEN** the same public key exists without the required source or forwarding restriction
- **THEN** Check reports drift and Apply writes the canonical restricted entry

#### Scenario: Authoritative key set revokes key
<!-- verification-id: OS-LIA-008 -->
- **WHEN** a previously managed key is absent from an authoritative set
- **THEN** Apply removes that key while preserving entries outside the set's ownership marker

### Requirement: Known hosts are structured and hashed-aware
Known-host resources SHALL manage host patterns, key type, fingerprint, presence, target scope, and hashing policy without replacing unrelated entries.

#### Scenario: Host key fingerprint changes
<!-- verification-id: OS-LIA-009 -->
- **WHEN** a managed host pattern resolves to a different stored fingerprint
- **THEN** Check reports drift and Apply changes it only according to explicit replacement policy

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

### Requirement: Account limits are first-class fragments
Account-limit resources SHALL manage named `limits.d`-style entries with domain, limit type, item, value, and presence, and SHALL report whether re-login is required.

#### Scenario: Limit changes
<!-- verification-id: OS-LIA-012 -->
- **WHEN** a managed limit differs
- **THEN** Apply changes its owned fragment and returns `logout-required` without terminating sessions

### Requirement: PAM and login policy are provider-owned
When authentication-policy management is advertised, it SHALL use a distro-aware provider, validate the complete effective stack, preserve recovery, and SHALL not perform generic unscoped line edits on provider-generated files. Authselect support SHALL remain unadvertised until the future RPM-family change implements and tests its provider contract.

#### Scenario: Authselect owns PAM configuration
<!-- verification-id: OS-LIA-013 -->
- **WHEN** a Fedora/RHEL endpoint uses authselect
- **THEN** the provider applies a supported authselect profile operation or reports `unsupported` rather than editing generated PAM files directly
