# Interactive User Policy Specification

## Purpose

Define provider-aware, multi-user policy management that remains safe across active and inactive Linux sessions.

## Requirements

### Requirement: Interactive-user selection is explicit
Interactive-user resources SHALL select users through a documented selector such as all interactive local users, explicit usernames, or supported labels, and SHALL report unmatched explicit users without applying to unintended accounts.

#### Scenario: Explicit user does not exist
<!-- verification-id: OS-IUP-001 -->
- **WHEN** a policy targets one explicit username that is absent and has no user-resource dependency
- **THEN** Check reports the target unresolved rather than silently applying to all interactive users

### Requirement: Policies work without active login
Providers SHALL apply persistent per-user policy without requiring the user to be logged in, or SHALL report `deferred` when the provider fundamentally requires a session.

#### Scenario: Dconf policy for logged-out user
<!-- verification-id: OS-IUP-002 -->
- **WHEN** a supported system database/override mechanism exists
- **THEN** Apply updates persistent policy with correct ownership without starting an interactive session

### Requirement: Per-user filesystem safety is preserved
Interactive-user policy SHALL use no-follow home traversal, correct UID/GID ownership, bounded paths, and protected permissions for every per-user artifact.

#### Scenario: User policy parent is a malicious symlink
<!-- verification-id: OS-IUP-003 -->
- **WHEN** a policy path under a home resolves through a symlink outside that home
- **THEN** Apply rejects the user target and does not modify the external path

### Requirement: Dconf and GSettings state is provider-aware
Desktop-setting resources SHALL manage schema, key, typed value, lock/mandatory state where supported, and scope through a dconf/GSettings provider. Values SHALL be checked using their native type.

#### Scenario: Boolean setting stored as string
<!-- verification-id: OS-IUP-004 -->
- **WHEN** desired state declares a boolean but the backend value is a string with similar text
- **THEN** Check reports type/value drift rather than treating them as equal

### Requirement: Session and login policy is structured
When advertised, session policy SHALL manage screen lock, idle timeout, login/session restrictions, proxy settings, default applications, and related settings as explicit fields or provider resources rather than arbitrary shell commands.

#### Scenario: Lock timeout changes
<!-- verification-id: OS-IUP-005 -->
- **WHEN** persistent lock policy differs for a selected user scope
- **THEN** Apply updates the provider-owned policy and reports whether logout is required

### Requirement: Browser policy uses managed scope
Browser-policy resources SHALL identify browser/provider, policy name, typed value, mandatory/recommended level, scope, and presence, and SHALL write only supported managed-policy locations.

#### Scenario: Unsupported browser policy
<!-- verification-id: OS-IUP-006 -->
- **WHEN** a policy name or level is unsupported by the selected browser provider
- **THEN** validation or Check returns `unsupported` rather than writing an ignored JSON key

### Requirement: Desktop certificate policy composes with trust resources
Browser or desktop certificate assignment SHALL reference certificate/trust resources and SHALL not duplicate private material or inline secret values.

#### Scenario: Browser trust depends on CA
<!-- verification-id: OS-IUP-007 -->
- **WHEN** a browser policy depends on a managed CA trust anchor
- **THEN** dependency ordering installs and verifies the anchor before browser policy activation

### Requirement: Active sessions are not disrupted implicitly
Policy Apply SHALL return activation such as `logout-required` or application restart and SHALL not terminate sessions unless a separately authorized action declares that behavior.

#### Scenario: Setting requires logout
<!-- verification-id: OS-IUP-008 -->
- **WHEN** the provider cannot activate a successful policy change in the current session
- **THEN** the result records `logout-required` and leaves the session running

### Requirement: Per-user results are aggregate and inspectable
Check SHALL provide an aggregate resource outcome and bounded per-user subresults so one failed or unsupported user is visible without leaking user secrets.

#### Scenario: One of three users fails
<!-- verification-id: OS-IUP-009 -->
- **WHEN** policy is compliant for two selected users and Check fails for one
- **THEN** the aggregate is not compliant and identifies the affected username and safe reason

### Requirement: Removed users do not leave owned policy unintentionally
Authoritative user-scope policy SHALL define cleanup behavior for users who leave the selector, while merge-scoped policy SHALL leave their prior state unchanged.

#### Scenario: User leaves authoritative selector
<!-- verification-id: OS-IUP-010 -->
- **WHEN** a previously selected user is no longer in an authoritative policy scope
- **THEN** Apply removes only that policy's owned artifacts for the user according to retention policy
