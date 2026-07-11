## ADDED Requirements

### Requirement: Sysctl runtime and persistence are independent
Sysctl resources SHALL manage a key and value with explicit runtime and persistent booleans. Check SHALL evaluate every enabled scope, and persistence SHALL use a named Remotr-owned drop-in.

#### Scenario: Persistent value matches but runtime differs
<!-- verification-id: OS-KHB-001 -->
- **WHEN** both scopes are managed, the drop-in is correct, and the live kernel value differs
- **THEN** Check reports runtime drift and Apply updates the live value according to reload policy

#### Scenario: Runtime-only setting
<!-- verification-id: OS-KHB-002 -->
- **WHEN** runtime is true and persistent is false
- **THEN** Apply changes `/proc/sys` state without creating a boot-time drop-in

### Requirement: Unsupported sysctl keys are explicit
The sysctl provider SHALL distinguish unknown/unsupported keys, permission failures, invalid values, and ordinary value drift.

#### Scenario: Kernel lacks key
<!-- verification-id: OS-KHB-003 -->
- **WHEN** the requested sysctl key does not exist on the running kernel
- **THEN** Check returns `unsupported` with the key reason rather than drift

### Requirement: Sysctl reload behavior is controlled
Sysctl resources SHALL declare whether to apply the single runtime key immediately, reload the Remotr-owned drop-in set, or defer until next boot. Reload SHALL not reapply unrelated distribution-owned files.

#### Scenario: Deferred setting changes
<!-- verification-id: OS-KHB-004 -->
- **WHEN** persistence changes with next-boot activation
- **THEN** Apply writes the drop-in and returns `next-boot` while leaving live state unchanged

### Requirement: Kernel module state includes boot behavior
Kernel-module resources SHALL independently manage current loaded state and boot persistence, with presence, module name, parameters, and optional blacklist behavior.

#### Scenario: Module must be loaded and persistent
<!-- verification-id: OS-KHB-005 -->
- **WHEN** the module is not loaded and no Remotr-owned boot declaration exists
- **THEN** Check reports both scopes and Apply loads the module then writes its boot declaration

### Requirement: Module unload and blacklist are safety-gated
Unload or blacklist SHALL require provider support and preflight that the module is not required by the active root filesystem, network control path, or declared protected subsystem.

#### Scenario: Active network driver would be unloaded
<!-- verification-id: OS-KHB-006 -->
- **WHEN** a module removal targets the driver carrying Remotr connectivity
- **THEN** Apply is blocked as connectivity/boot risk

### Requirement: Hostname has persistent and active state
Hostname resources SHALL manage the canonical static hostname and, where supported, the active transient hostname without conflating `/etc/hosts` ownership.

#### Scenario: Static hostname differs
<!-- verification-id: OS-KHB-007 -->
- **WHEN** the managed static hostname differs
- **THEN** Check reports drift and Apply uses the selected host provider, returning any activation requirement

### Requirement: Timezone, locale, and keymap are separate fields
Host-locale resources SHALL manage timezone, system locale variables, and console keymap as independently optional fields through provider-supported mechanisms.

#### Scenario: Only timezone is managed
<!-- verification-id: OS-KHB-008 -->
- **WHEN** timezone is specified and locale/keymap are omitted
- **THEN** Check and Apply leave locale and keymap unchanged

### Requirement: Time synchronization is provider-neutral
Time-synchronization resources SHALL separately express whether synchronization is enabled and the desired server/pool configuration, and SHALL select an advertised provider such as systemd-timesyncd or chrony.

#### Scenario: Existing provider cannot manage servers
<!-- verification-id: OS-KHB-009 -->
- **WHEN** synchronization enablement is supported but custom servers are not supported by the active provider
- **THEN** validation or Check reports the server-list capability unsupported instead of accepting partial state

### Requirement: Host baseline reports activation
Host and kernel Apply SHALL report reload, restart, logout, or reboot requirements without performing an undeclared reboot or session termination.

#### Scenario: Locale requires new login
<!-- verification-id: OS-KHB-010 -->
- **WHEN** locale configuration changes successfully
- **THEN** Apply returns `logout-required` and does not terminate active users
