# Network and Firewall Management Specification

## Purpose

Define truthful, recovery-aware network and firewall management that preserves control connectivity.

## Requirements

### Requirement: Firewall rule lifecycle is explicit
Firewall rules SHALL manage `present` or `absent` independently from allow/deny/drop/reject action. Check and Apply SHALL support removal through every advertised backend.

#### Scenario: Present rule must be removed
<!-- verification-id: OS-NFM-001 -->
- **WHEN** a rule declares `absent` and the corresponding managed rule exists
- **THEN** Check reports lifecycle drift and Apply removes the rule using its stable backend identity

### Requirement: Firewall ownership modes are explicit
Firewall resources SHALL distinguish individual named rules, Remotr-owned chains/zones/fragments, and authoritative rule sets. Authoritative cleanup SHALL not remove rules outside the declared ownership boundary.

#### Scenario: Authoritative managed chain has extra rule
<!-- verification-id: OS-NFM-002 -->
- **WHEN** a Remotr-owned authoritative chain contains a stale managed rule
- **THEN** Apply removes the stale managed rule while preserving other tables and chains

### Requirement: Firewall audit is safe by default
New or migrated firewall enforcement SHALL default to audit/report behavior unless enforcement is explicitly requested. Audit SHALL produce a structured plan rather than treating a written local audit log as enforcement compliance.

#### Scenario: Default firewall rule is evaluated
<!-- verification-id: OS-NFM-003 -->
- **WHEN** a firewall rule omits enforcement mode
- **THEN** the endpoint reports the planned backend change without modifying packet-filter state

### Requirement: Firewall providers are truthful
Firewalld and nftables SHALL implement equivalent declared lifecycle and observation semantics where advertised. UFW or iptables SHALL not be selected until their provider contract and tests exist.

#### Scenario: Requested backend is unavailable
<!-- verification-id: OS-NFM-004 -->
- **WHEN** a resource requires firewalld and it is not available
- **THEN** Check returns `unsupported` instead of falling back to a backend with different semantics

### Requirement: Firewall changes preserve control connectivity
Enforced firewall changes SHALL evaluate the active Remotr sync destinations, routes, protocols, ports, established connections, and DNS dependencies. Risky transitions SHALL require a timed rollback or an explicitly authorized recovery path.

#### Scenario: Rule blocks sync destination
<!-- verification-id: OS-NFM-005 -->
- **WHEN** a proposed rule would block the active Remotr control path
- **THEN** preflight blocks Apply unless an approved timed recovery mechanism is active

#### Scenario: Post-apply acknowledgement fails
<!-- verification-id: OS-NFM-006 -->
- **WHEN** an enforced firewall transaction does not receive required connectivity acknowledgement before timeout
- **THEN** the endpoint restores the prior firewall state and reports rollback outcome

### Requirement: Network management uses separate resource kinds
Hosts entries, DNS resolver/search domains, routes, and interface/network profiles SHALL be separate resources with independent state and safety semantics rather than one unstructured network document.

#### Scenario: DNS-only change
<!-- verification-id: OS-NFM-007 -->
- **WHEN** only DNS servers/search domains are managed
- **THEN** Check and Apply leave interface addressing and routes unmanaged

### Requirement: Hosts entries are structurally owned
Hosts-entry resources SHALL manage address, canonical host, aliases, presence, and ownership marker while preserving unrelated `/etc/hosts` content.

#### Scenario: Alias differs
<!-- verification-id: OS-NFM-008 -->
- **WHEN** a managed hosts entry lacks a requested alias
- **THEN** Check reports drift and Apply updates only the managed entry

### Requirement: DNS and routes are provider-backed
DNS and route resources SHALL express portable intent and resolve to an advertised backend. Effective state, not merely configuration-file text, SHALL be checked where the backend exposes it.

#### Scenario: Runtime route differs from persistent profile
<!-- verification-id: OS-NFM-009 -->
- **WHEN** both runtime and persistence are managed and only runtime route state differs
- **THEN** Check reports runtime drift separately and Apply follows activation policy

### Requirement: Network profiles start audit-first
Interface/profile changes SHALL initially support audit/report and NetworkManager on declared distributions. Enforced activation SHALL require checkpoint/timed rollback support and explicit acknowledgement; netplan and systemd-networkd SHALL be advertised only after equivalent safety exists.

#### Scenario: NetworkManager checkpoint expires
<!-- verification-id: OS-NFM-010 -->
- **WHEN** profile activation loses control connectivity and no acknowledgement occurs
- **THEN** NetworkManager rollback restores the checkpoint and reports the failed transition

### Requirement: Network validation rejects ambiguity
The system SHALL reject ambiguous interface selectors, conflicting addresses/routes, invalid prefixes, unknown backends, and configuration that would remove all declared control routes without recovery.

#### Scenario: Interface selector matches multiple devices
<!-- verification-id: OS-NFM-011 -->
- **WHEN** a resource expected to own one profile matches more than one endpoint interface
- **THEN** Apply is blocked without changing any profile

### Requirement: Network reporting separates configured and effective state
Reports SHALL identify backend, configured state, effective runtime state, enforcement/audit mode, acknowledgement, and rollback outcome without disclosing network credentials.

#### Scenario: Wi-Fi credential-backed profile drifts
<!-- verification-id: OS-NFM-012 -->
- **WHEN** a managed profile differs only in secret material
- **THEN** the report identifies secret drift by safe reference/fingerprint and never exposes the credential
