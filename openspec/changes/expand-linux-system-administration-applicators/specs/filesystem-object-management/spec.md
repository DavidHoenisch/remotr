## ADDED Requirements

### Requirement: Filesystem object kinds are explicit
The filesystem contract SHALL distinguish regular files, directories, symbolic links, and hard links, with lifecycle `present` or `absent`. The requested kind SHALL be part of Check.

#### Scenario: Wrong object kind exists
- **WHEN** desired state requires a directory but a regular file exists at the path
- **THEN** Check reports kind drift and Apply refuses replacement unless type replacement is explicitly permitted

#### Scenario: Object is absent
- **WHEN** an object declares `absent`
- **THEN** Apply removes that object only within its declared recursive and safety policy

### Requirement: Content and metadata converge independently
Regular files SHALL support managed content and all filesystem kinds SHALL support managed owner, group, and mode where applicable. Check SHALL detect metadata drift even when content is correct.

#### Scenario: Only mode differs
- **WHEN** file content matches but the managed mode differs
- **THEN** Check reports mode drift and Apply corrects the mode without rewriting content unnecessarily

#### Scenario: Owner field is omitted
- **WHEN** owner is omitted and other managed fields match
- **THEN** Check does not consider current ownership drift

### Requirement: File replacement is atomic and durable
Whole-file content changes SHALL be written to a same-filesystem temporary file with requested metadata, validated when required, fsynced as appropriate, and atomically renamed. Failure SHALL leave the previous file active.

#### Scenario: Validation fails before rename
- **WHEN** staged content fails its declared validator
- **THEN** the existing path remains unchanged and the staged file is removed

### Requirement: Structured edits preserve their current semantics
Line-regex and future managed-block/template modes SHALL be explicit content strategies. Existing line-edit behavior SHALL remain compatible during migration and SHALL not be conflated with whole-file ownership.

#### Scenario: Managed line is missing
- **WHEN** line-edit Check cannot find the desired expression
- **THEN** Apply replaces the declared matching line or appends the desired line according to the resource strategy

### Requirement: Link targets converge safely
Symbolic and hard-link resources SHALL check the declared target and SHALL not follow an existing untrusted symlink while changing a protected path.

#### Scenario: Symlink target differs
- **WHEN** a managed symbolic link points to a different target
- **THEN** Check reports target drift and Apply atomically replaces the link when replacement policy permits

### Requirement: Path traversal and symlink attacks are prevented
System and per-user filesystem operations SHALL use validated absolute or home-relative paths, no-follow operations, and safe parent traversal. Per-user operations SHALL remain confined to the selected home directory.

#### Scenario: User path escapes home
- **WHEN** a user-relative path contains traversal or a symlinked parent escapes the home
- **THEN** validation or Apply rejects the operation without touching the escaped target

### Requirement: Directory recursion has bounded ownership
Recursive directory management SHALL require explicit recursion, purge, and cross-filesystem policies. Authoritative cleanup SHALL apply only below the declared root and SHALL exclude agent state and protected mount boundaries by default.

#### Scenario: Extra file under non-purging directory
- **WHEN** an unmanaged child exists and purge is not enabled
- **THEN** Check and Apply leave that child unchanged

### Requirement: Extended metadata is capability-gated
The filesystem metadata contract SHALL distinguish ACLs, extended attributes, Linux file capabilities, and SELinux labels and SHALL reject unsupported metadata for the selected provider/filesystem. This change SHALL advertise ACL and extended-attribute management only when implemented and tested; Linux file capabilities remain an M6 roadmap capability and SELinux labels remain part of the deferred RPM-family roadmap.

#### Scenario: Filesystem lacks required ACL support
- **WHEN** a resource manages an ACL on a filesystem/provider without ACL capability
- **THEN** Check returns `unsupported` rather than repeatedly reporting ordinary drift

### Requirement: Rollback preserves applicable prior state
When filesystem rollback is advertised, prior kind, content, ownership, mode, link target, and managed extended metadata SHALL be captured in protected agent state. Secret content SHALL follow the secret rollback policy rather than generic backup files adjacent to the target.

#### Scenario: Apply fails after metadata staging
- **WHEN** a transactional file Apply fails before activation
- **THEN** the previous active object and its managed metadata remain intact

### Requirement: Remote files are verified before activation
Remote-file resources SHALL explicitly manage lifecycle, URL, destination, checksum, optional signature and trusted signer, authentication reference, redirect policy, timeout, mode, owner, and group. Downloaded bytes SHALL be verified in protected staging before atomic destination replacement.

#### Scenario: Signature verification fails
- **WHEN** downloaded content has a valid transport response but its signature does not match a declared trusted signer
- **THEN** Apply rejects the staged content, preserves the active destination, and reports verification failure without credentials

#### Scenario: Authenticated download is compliant
- **WHEN** the destination checksum and managed metadata already match desired state
- **THEN** Check reports compliant without fetching secret-authenticated content solely to rewrite the destination

### Requirement: Remote-file activation uses common signals
Remote-file resources SHALL emit structured activation signals instead of executing ad hoc notification fields. Compatible legacy `notifySystemd` and `reloadExec` input SHALL map to canonical activation behavior during migration.

#### Scenario: Verified remote file replaces active policy
- **WHEN** a remote file changes successfully and declares a service reload
- **THEN** the engine activates the verified file atomically and orders the reload through the shared activation queue

### Requirement: Archive deployment is an optional structured strategy
When archive deployment is advertised, it SHALL verify checksum/signature, bound extraction paths, support strip-components and ownership, stage into a new directory, and atomically switch the destination according to explicit retention/removal policy.

#### Scenario: Archive contains traversal entry
- **WHEN** an archive member would escape the staging root
- **THEN** extraction fails and no destination content is activated

### Requirement: VCS deployment is revision-pinned
When VCS deployment is advertised, the desired revision, remote identity, clean/dirty policy, credential reference, and activation strategy SHALL be explicit; floating branches SHALL not satisfy a pinned revision.

#### Scenario: Checkout has local modifications
- **WHEN** authoritative clean policy is declared and the checkout is dirty
- **THEN** Check reports drift and Apply follows the declared discard-or-block policy
