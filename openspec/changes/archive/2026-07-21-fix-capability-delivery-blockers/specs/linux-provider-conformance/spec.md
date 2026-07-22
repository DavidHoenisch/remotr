## ADDED Requirements

### Requirement: Production capability catalogs are frozen evidence outputs
Every production agent release SHALL contain a canonical immutable capability catalog generated deterministically from checked-in passing qualification rows. The production default capability generator SHALL evaluate that embedded catalog against normalized runtime facts. Test-only constructor wiring, runtime reads from test fixtures, implementation presence, executable presence, and remotely fetched data SHALL NOT add advertised capabilities. Agent release metadata used by the server SHALL be generated from the same release inputs and SHALL distinguish protocol or binary eligibility from runtime provider evidence.

#### Scenario: Passing Ubuntu Pro manifest is packaged
<!-- verification-id: OS-LPC-023 -->
- **WHEN** an exact Ubuntu Pro qualification row passes all required selectors and is included in a release
- **THEN** the production default generator advertises only that row on matching runtime facts without requiring a test-only constructor or runtime test-file access

#### Scenario: Generated catalog is stale or conflicting
<!-- verification-id: OS-LPC-024 -->
- **WHEN** regeneration changes packaged output, a row lacks required evidence selectors, duplicate rows conflict, or server release metadata disagrees with the agent payload
- **THEN** validation or release packaging fails and no affected row is published

#### Scenario: Agent version is known to the server
<!-- verification-id: OS-LPC-025 -->
- **WHEN** the server recognizes an approved agent release and considers an upgrade for a blocked endpoint
- **THEN** release metadata may prove binary and protocol eligibility but does not prove that the endpoint runtime satisfies a provider capability

### Requirement: A target release is advertised only with complete applicable provider evidence
Before Remotr advertises support for a distribution release and architecture in a representative public configuration, every resource and provider row applicable to that target SHALL either have its own complete required evidence and frozen catalog entry or SHALL be rejected at author-time validation. Evidence for a specialized provider SHALL NOT promote sibling core providers, and evidence for a core provider SHALL NOT promote specialized capabilities.

#### Scenario: Ubuntu Pro passes but a core provider does not
<!-- verification-id: OS-LPC-026 -->
- **WHEN** Ubuntu Pro passes on Ubuntu 26.04 amd64 but an applicable file, command, package, init, download, or other core provider row lacks its governing evidence
- **THEN** only Ubuntu Pro's passing rows may be advertised and validation rejects configuration requiring the unqualified core row

#### Scenario: Representative target inventory is complete
<!-- verification-id: OS-LPC-027 -->
- **WHEN** every row required by the Ubuntu 26.04 amd64 public qualification configuration has passed its own selected provider, safety, redaction, and cleanup evidence
- **THEN** the frozen catalog contains exactly those applicable rows and authenticated Sync can evaluate the configuration without manufacturing support
