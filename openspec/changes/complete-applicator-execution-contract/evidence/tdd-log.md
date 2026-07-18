# TDD evidence log

## Task 2.1 — OS-AEC-080 restart recovery

- Public seam: system-safety recovery through `rollbackstore.New`, mutation
  admission, and `RecoverArmed`.
- Selected evidence: focused restart contract; corrupt-record and failed-provider
  negatives; rollback-store and network-state package regressions. Provider and
  Ubuntu VM interruption evidence remains owned by tasks 2.7 through 2.9 and
  scenario promotion remains blocked until task 5.6.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRecoversArmedTransactionAfterRestart -count=1`.
- Intended red observed: the test did not compile because `ErrArmedRecovery`,
  `Recovery`, and `RecoverArmed` did not exist.
- Green command: the same focused command passed after adding lifecycle version
  1, startup validation, per-resource blocking, recovery delivery, terminal
  transition, and payload cleanup.
- Negative evidence: the package suite proves corrupt legacy ciphertext blocks
  startup and a failed recovery callback remains armed, retryable, and absent
  from the safe returned diagnostic.

## Task 2.2 — OS-AEC-069 pre-mutation reservation

- Public seam: provider-contract reservation through `Store.Reserve` and the
  single-use `Reservation.Arm` handle.
- Selected evidence: focused configured-cap refusal, protected armed-record
  preservation, ciphertext/metadata/filesystem boundary cases, outstanding
  reservation accounting, and oversized-payload cleanup.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreReservesCompleteRecoveryBeforeMutation -count=1`.
- Intended red observed: the test did not compile because `AvailableBytes`,
  `ReservationRequest`, and `Store.Reserve` did not exist.
- Green command: the same focused command passed after reservation began
  accounting for AEAD overhead, encoded metadata, filesystem allowance,
  configured usage, available filesystem bytes, protected armed records, and
  concurrent outstanding reservations before `Arm` permits mutation.

## Task 2.3 — OS-AEC-081 deterministic retention

- Public seam: transaction retention through `Store.Cleanup` and the
  payload-free `Store.Records` metadata view.
- Selected evidence: exact attempt and successful-prior-state limits, exact
  age boundary, sensitive expiry, incomplete-attempt supersession, configured
  disk pressure, armed-record preservation, and a deterministic 0–20 attempt
  property loop under an injected clock.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRetentionPrunesAttemptsAndSuccessfulPayloads -count=1`.
- Intended red observed: the test did not compile because `Store.Cleanup` and
  `Store.Records` did not exist.
- Green command: the focused command passed after retention began keeping at
  most the configured attempt metadata and three successful non-secret prior
  payloads per resource.
- Boundary evidence: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreRetention -count=1` passed for age, armed, sensitivity,
  supersession, disk, and bounded-count cases without wall-clock sleeps.

## Task 2.4 — OS-AEC-080 atomic transaction envelope

- Public seam: system-safety recovery through durable `Store.Save`, startup
  validation in `rollbackstore.New`, and `RecoverArmed` after an injected
  process stop.
- Selected evidence: deterministic injection after temporary-file creation,
  envelope write, envelope fsync, atomic activation, and directory fsync;
  authenticated metadata and ciphertext tamper negatives; legacy split-record
  migration; rollback-store and network-state package regressions.
- Red command: `go test -mod=vendor ./internal/rollbackstore -run
  TestStoreActivatesOneEnvelopeAtEveryDurabilityBoundary -count=1`.
- Intended red observed: the test did not compile because `DurabilityPoint`,
  its five durability-boundary constants, and `Options.CrashInjector` did not
  exist.
- Green command: the same focused command passed after payload and lifecycle
  metadata were sealed into one checksummed AEAD envelope, fsynced, atomically
  renamed, and followed by a parent-directory fsync. Stops before activation
  leave no recoverable transaction; stops after activation leave exactly one
  complete recoverable transaction.
- Negative and migration evidence: `TestEnvelopeAuthenticatesMetadataAndCiphertext`
  rejects modified headers and ciphertext at startup without leaking the
  payload, and `TestLegacySplitRecordCompatibilityFixture` migrates the frozen
  split-format fixture to one envelope while corrupt legacy ciphertext remains
  startup-blocking.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.

## Task 2.5 — OS-AEC-070 and OS-AEC-082 protected-key selection

- Public seam: protected-key provider selection through `KeyProvider`,
  `CapabilityKeyProvider`, the concrete `TPM2ToolsKeyProvider`, store startup,
  and the key-free `Store.Protection` report.
- Selected evidence: unsupported-TPM root fallback, selected-TPM failure and
  restart downgrade negatives, pre-marker upgrade compatibility, ambiguous
  provider-state refusal, versioned root-file persistence and raw-key migration,
  live TPM capability gating, exact TPM process argv, stdin-only sealing input,
  safe protection reports, and rollback/network-state package regressions.
  Real TPM VM and mutation evidence remains owned by tasks 5.2 and 5.6.
- First red command: `go test -mod=vendor ./internal/rollbackstore -run
  'TestStoreSelectsExplicitRootFallbackAndReportsReducedProtection|TestSelectedTPMFailureBlocksWithoutSilentRootDowngrade|TestRootKeyProviderPersistsVersionedRootOnlyMaterial'
  -count=1`.
- First intended red observed: the tests did not compile because `KeyMaterial`,
  `CapabilityKeyProvider`, protection classes, the blocking key-protection
  error, and `Store.Protection` did not exist.
- First green command: the same focused command passed after persisting the
  protection choice before provider use, adding a versioned root-only key
  envelope with legacy migration, binding transaction envelopes to a safe key
  identity, and exposing a key-free report with the explicit endpoint-root
  compromise limitation.
- Concrete-provider red command: `go test -mod=vendor ./internal/rollbackstore
  -run 'TestTPM2ToolsKeyProviderCapabilityGate|TestTPM2ToolsKeyProviderSealsAndReloadsWithoutArgumentLeakage|TestStoreReportsSelectedTPMClassWithoutKeyMaterial'
  -count=1`.
- Concrete-provider intended red observed: the tests did not compile because
  `NewTPM2ToolsKeyProvider` and `TPM2ToolsOptions` did not exist.
- Concrete-provider green command: the same focused command passed after the
  TPM2-tools provider began requiring a device, every required executable, and
  a live capability probe; sealing AES-256 key bytes through stdin; persisting
  only versioned TPM public/private blobs; unsealing on restart; flushing loaded
  contexts; and reporting only the safe TPM class and random key identity.
- Upgrade-boundary red command: `go test -mod=vendor
  ./internal/rollbackstore -run
  TestCapabilitySelectionPreservesPreexistingProtectionState -count=1`.
- Upgrade-boundary intended red observed: newly detected TPM capability won
  over a pre-marker root key, and ambiguous root-plus-TPM state reached the TPM
  provider instead of blocking.
- Upgrade-boundary green command: the same command passed after preexisting
  single-provider state became the initial durable selection and ambiguous
  state became startup-blocking.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.

## Task 2.6 — versioned key rotation and decrypt-only history

- Public seam: protected-key rotation through `Store.RotateKey`,
  `RotatingKeyProvider`, transaction-envelope key identities, startup
  validation, and retained-record `Store.Load`.
- Selected evidence: one armed recovery and one retained successful prior state
  encrypted before rotation, a newly armed recovery encrypted afterward,
  restart decryption through the historical identity, unavailable-history
  startup refusal, root keyring rotation, TPM-sealed keyring rotation, version 1
  migration for both provider files, and package regressions.
- Store red command: `go test -mod=vendor ./internal/rollbackstore -run
  'TestStoreKeyRotationRetainsHistoricalDecryptOnlyRecovery|TestStoreBlocksWhenReferencedHistoricalKeyIsUnavailable'
  -count=1`.
- Store intended red observed: the tests did not compile because
  `Store.RotateKey` did not exist.
- Store green command: the same focused command passed after rotation began
  binding legacy envelopes to the current identity, retaining the former key
  in a decrypt-only resolver, using only the new active identity for subsequent
  envelopes, and preserving the key-protection cause when missing history
  blocks restart recovery.
- Root-provider red command: `go test -mod=vendor ./internal/rollbackstore
  -run 'TestRootKeyProviderPersistsVersionedRootOnlyMaterial/rotation' -count=1`.
- Root-provider intended red observed: `RootKeyProvider.Rotate` and
  `RootKeyProvider.LoadByID` did not exist.
- Root-provider green command: the same focused command passed after the
  version 2 root-only keyring began naming one active key while retaining prior
  identities for decryption, including migration from raw and version 1 files.
- TPM-provider red command: `go test -mod=vendor ./internal/rollbackstore
  -run TestTPM2ToolsKeyProviderRotationRetainsSealedHistory -count=1`.
- TPM-provider intended red observed: `TPM2ToolsKeyProvider.Rotate` and
  `TPM2ToolsKeyProvider.LoadByID` did not exist.
- TPM-provider green command: the same focused command passed after the
  version 2 TPM keyring began retaining separately sealed historical blobs,
  selecting one active identity, and unsealing a requested old identity only
  for decryption.
- Regression command: `go test -mod=vendor ./internal/rollbackstore
  ./internal/agent/networkstate -count=1` passed.

## Task 2.7 — firewall and network-profile transaction handles

- Public seams: provider contract through firewall, NetworkManager, netplan,
  and systemd-networkd `ApplyResult`/`Check`; authenticated network transaction
  acknowledgement and timeout recovery through `networkstate`; destructive
  connectivity recovery through the isolated Vagrant VM safety fixture.
- Selected evidence: capacity refusal before mutation; one protected handle for
  every backend; process restart; protected checkpoint/snapshot use; plaintext
  state redirection resistance; legacy NetworkManager checkpoint migration;
  orphaned-state startup refusal; authenticated acknowledgement; timeout and
  explicit rollback; payload cleanup; second Check; exact protected nftables
  stdin; and real route, DNS, firewall, and interface loss/recovery.
- First red command: `go test -mod=vendor ./internal/agent/networkstate -run
  'TestStoreArmsNetworkManagerRecoveryHandleAcrossRestart|TestStoreRefusesNetworkTransactionWhenRecoveryReservationUnavailable'
  -count=1`.
- First intended red observed: the tests did not compile because
  `networkstate.Options.RollbackOptions` did not exist; before the implementation
  NetworkManager also had no transaction envelope and plaintext checkpoint
  state selected the rollback target.
- First green command: the same focused command passed after `Prepare` began
  reserving complete capacity and arming one encrypted versioned recovery
  handle for every backend before provider mutation. Restart and reconciliation
  now recover backend parameters and snapshots only from that handle, and
  acknowledgement or completed rollback drops its payload.
- Startup-coherence red command: `go test -mod=vendor
  ./internal/agent/networkstate -run
  'TestStoreBlocksOrphanedArmedRecoveryAfterStateLoss|TestStoreMigratesLegacyNetworkManagerIntentToProtectedHandle'
  -count=1`.
- Startup-coherence intended red observed: startup accepted an armed nftables
  recovery after its state file was removed, and legacy NetworkManager intent
  remained without a protected transaction envelope.
- Startup-coherence green command: the same command passed after startup began
  requiring a unique match between awaiting status and armed recovery, blocking
  orphaned or ambiguous state, and migrating one valid legacy NetworkManager
  checkpoint before reconciliation.
- Provider commands: `go test -mod=vendor ./internal/applicators/firewall -run
  TestApplicator_EnforcedNftablesArmsTimedRollback -count=1`, `go test
  -mod=vendor ./internal/applicators/networkmanager -run
  TestProfileApplyRestartAcknowledgementCleanupAndSecondCheck -count=1`, and
  `go test -mod=vendor ./internal/applicators/networkfiles -run
  TestFileBackedProfilesConvergeThroughArmedRollbackTransaction -count=1` all
  passed. Together they prove Apply, reconstructed-store restart, authenticated
  acknowledgement, timeout/explicit rollback, cleanup, and compliant or
  correctly drifted second Check at the provider seam.
- Relevant regression command: `go test -mod=vendor
  ./internal/agent/networkstate ./internal/applicators/firewall
  ./internal/applicators/networkmanager ./internal/applicators/networkfiles
  ./cmd/remotr-agent -count=1` passed.
- VM safety command: `mise exec go -- make
  provider-matrix-vm-network-recovery` passed against the disposable isolated
  Debian VM. The fixture observed failed control probes during real route, DNS,
  nftables, and interface disruption; restored every path; completed the
  authenticated acknowledgement; and destroyed the VM and libvirt network.

## Task 2.8 — file, download, certificate, and trust transaction handles

- Public seams: file, download, certificate, and trust-anchor provider
  `Apply`/`Rollback`/second-`Check` behavior through reconstructed providers;
  structured Apply failure reporting through `executor.ApplyState`; and the
  protected rollback-store reservation/envelope boundary.
- Selected evidence: reservation and arm before managed-state mutation;
  restart reconstruction; exact prior bytes, mode, and ownership metadata;
  absence of `.remotr.bak`; sensitive certificate/private-key encryption and
  24-hour retention admission; rollback cleanup; drifted second Check; honest
  `none` advertisement when no transaction handle is supplied; and separate
  original-Apply-error and rollback-outcome reporting.
- File red command: `go test -mod=vendor ./internal/applicators/files -run
  TestApplicatorProtectedRollbackSurvivesRestartWithoutAdjacentBackup
  -count=1`.
- File intended red observed: the test did not compile because
  `files.Applicator.ConfigureRollback` did not exist, and the provider still
  created `<path>.remotr.bak`.
- File green command: the same focused command passed after the file provider
  began arming a versioned path-bound snapshot through `rollbackstore.Handle`
  before removal, replacement, or metadata mutation and restoring it after
  provider reconstruction without an adjacent backup.
- Download red command: `go test -mod=vendor
  ./internal/applicators/downloads -run
  TestApplicator_ProtectedRollbackSurvivesRestartWithoutAdjacentBackup
  -count=1`.
- Download intended red observed: the test did not compile because
  `downloads.Applicator.ConfigureRollback` did not exist, and the provider
  still depended on `<destination>.remotr.bak`.
- Download green command: the same focused command passed after destination
  content, mode, and ownership became a protected attempt payload and Revert
  began resolving the armed handle after restart.
- Reservation regression red command: `go test -mod=vendor
  ./internal/rollbackstore -run
  TestReservationEstimateCoversTimestampEncodingGrowthBeforeArm -count=1`.
- Reservation intended red observed: a complete reserved payload was refused
  at Arm because fractional timestamp JSON grew between the estimate and the
  final envelope.
- Reservation green command: the same command passed after the complete-footprint
  estimate gained an explicit bound for variable-width metadata; repeated
  download package runs no longer failed intermittently.
- Certificate red command: `go test -mod=vendor
  ./internal/applicators/certificates -run
  TestApplicatorProtectedSensitiveRollbackSurvivesRestart -count=1`.
- Certificate intended red observed: the test did not compile because the
  certificate provider had no transaction-handle configuration and its
  certificate/private-key snapshot existed only in process memory.
- Certificate green command: the same focused command passed after the pair
  snapshot became a sensitive path-bound protected payload. Raw transaction
  files excluded the private-key canary, a reconstructed provider restored
  both files, and the second Check reported expected renewal drift.
- Trust-anchor red command: `go test -mod=vendor
  ./internal/applicators/trustanchors -run
  TestApplicatorProtectedRollbackSurvivesRestart -count=1`.
- Trust-anchor intended red observed: the test did not compile because the
  provider had no transaction handle and prior anchor state was process-local.
- Trust-anchor green command: the same focused command passed after the named
  anchor's bytes, mode, and ownership were protected and restart-restorable.
- Failure-reporting command: `go test -mod=vendor
  ./internal/applicators/files -run
  TestApplicatorReportsOriginalApplyErrorSeparatelyFromProtectedRollback
  -count=1` passed. A real procfs mutation refusal remained the primary Apply
  error while the armed recovery was independently reported as `reverted`.
- Relevant regression command: `go test -mod=vendor ./internal/applicators/...
  ./internal/agent/engine ./internal/resourceregistry
  ./internal/rollbackstore ./internal/executor -count=1` passed.

## Task 2.9 — access, package-manager, boot, and remaining advertisements

- Public seams: provider `ApplyResult`/`Revert` through reconstructed
  transaction stores; registry-created providers with Resource/artifact
  identity; the executor legacy-handler adapter; and isolated Linux VM access
  and system-safety fixtures.
- Selected evidence: APT signing-key and sensitive multi-file repository
  restart recovery; protected access/policy/logging file state; descriptor-safe
  SSH user-path recovery; valid no-rollback swap results; honest downgrade of
  no-op/arbitrary legacy handlers; explicit AppArmor and audit-rules downgrade;
  exact bytes/mode restoration; sensitive retention; registry wiring; provider
  package regression; real PAM recovery before and after rollback; and real
  boot/reboot, loop-device, sysctl, AppArmor-capability, and recovery-principal
  VM evidence.
- Legacy-adapter red command: `go test -mod=vendor ./internal/executor -run
  TestLegacyHandlerDoesNotInheritUnprovenBestEffortRollback -count=1`.
  Intended red observed: a legacy no-op-capable handler still inherited
  `best_effort`. The green implementation changed the compatibility adapter to
  `none`, covering Flatpak, PWA, directory, link, group, user file, user,
  systemd, service, bootstrap, agent-install, and command providers unless they
  later adopt an explicit structured contract.
- Package-manager red commands: `go test -mod=vendor
  ./internal/applicators/aptkeys -run
  TestApplicatorProtectedRollbackSurvivesRestart -count=1` and `go test
  -mod=vendor ./internal/applicators/aptrepositories -run
  TestApplicatorProtectedMultiFileRollbackSurvivesRestart -count=1`.
  Intended reds observed: neither provider exposed transaction configuration;
  signing-key state was process-local and repository Revert was a no-op. The
  green implementations restore keyring, source, preference, and credential
  files from protected envelopes after reopening the store, with repository
  credentials classified sensitive.
- Access-provider red commands: `go test -mod=vendor
  ./internal/applicators/authorizedkeys -run
  TestAuthorizedKeyApplicatorRestoresProtectedStateAfterRestart -count=1` and
  `go test -mod=vendor ./internal/applicators/knownhosts -run
  TestKnownHostApplicatorRestoresProtectedStateAfterRestart -count=1`.
  Intended reds observed: both providers lacked `ConfigureRollback`. The green
  providers delegate snapshot and restore to the descriptor-safe file seam,
  advertise transactional only with a handle, mark access/trust payloads
  sensitive, and restore exact prior bytes and modes after provider/store
  reconstruction. Focused registry tests prove production configuration.
- Remaining protected migrations: focused restart tests in account limits,
  browser policy, sudo, login policy, journald, logrotate, and hosts entries
  passed after each provider armed path-bound protected state before mutation.
  Activation failure remains inside the rollback callback where applicable so
  failed restoration stays armed and retryable.
- Downgrade red command: `go test -mod=vendor
  ./internal/applicators/apparmor ./internal/applicators/auditrules -count=1`.
  Intended reds observed: failed, changed, no-change, mutable, and
  immutable/reboot-required outcomes still reported `best_effort`. The green
  result is `none` on every path because process-local file restoration cannot
  guarantee loaded kernel-state recovery after restart; activation and reboot
  reporting remain unchanged.
- Advertisement audit: `rg -n RollbackBestEffort internal/applicators`
  returned no matches. Every explicitly transactional non-network provider
  found by `rg -l RollbackTransactional internal/applicators` has both a
  `ConfigureRollback` implementation and matching
  `configureProtectedRollback` registry wiring. The final provider-by-provider
  outcome is recorded in `1.2-migration-inventory.md`.
- Regression command: `go test -mod=vendor ./internal/applicators/...
  ./internal/agent/engine ./internal/executor ./internal/resourceregistry
  ./internal/rollbackstore -count=1` passed, including APT, Flatpak, PWA,
  access, security, logging, boot, network, and package provider suites.
- VM access command: `mise exec go -- make
  provider-matrix-vm-login-policy-safety` passed. The disposable Debian guest
  activated a real `pam-auth-update` profile, exercised its recovery principal,
  rolled back, exercised recovery again, and destroyed its domain and network.
- VM system-safety command: `mise exec go -- make
  provider-matrix-vm-system-safety` passed. The guest proved loop-device and
  sysctl recovery, recovery-principal viability, truthful AppArmor capability,
  coordinated reboot persistence, post-restart completion and second Check,
  then destroyed its domain and network.

## Task 3.1 — OS-AEC-083 field-classification registration gate

- Public seam: resource registration through `resourceregistry.New`, whose
  default registry is also the source of truth for strict repository decode and
  validation.
- Selected evidence: one independently known accepted `file.content` field;
  inline shared metadata; nested struct, sequence, and arbitrary map path
  discovery from the exact Go type supplied to strict YAML decode; missing and
  invalid classification refusal; unknown descriptor refusal; immutable
  registry copies; registry/model regressions; and focused vet.
- Red command: `go test -mod=vendor ./internal/resourceregistry -run
  TestRegistryRejectsUnclassifiedAcceptedField -count=1`.
- Intended red observed: the test did not compile because
  `FieldDescriptors`, `Definition.FieldDescriptors`, and a registration
  coverage contract did not exist.
- Green command: the same focused command passed after registration began
  deriving every accepted leaf path from the strict schema type, requiring a
  valid public, sensitive-metadata, or secret descriptor for each, and rejecting
  descriptors for fields the strict decoder cannot accept. The canonical
  discriminator `kind` is included explicitly.
- Immutability red command: `go test -mod=vendor
  ./internal/resourceregistry -run
  TestRegistryReturnsImmutableFieldDescriptorCopies -count=1`.
- Immutability intended red observed: mutating the map returned from
  `Registry.Definition` changed the registered classification. The green
  registry clones descriptor maps at admission and every public read.
- Regression command: `go test -mod=vendor ./internal/resourceregistry
  ./internal/models -count=1` passed, followed by `go vet
  ./internal/resourceregistry`.
- Migration boundary: task 3.1 seeds accepted fields from the prior
  whole-resource class only to establish a complete fail-closed descriptor
  surface. Task 3.2 owns replacing every coarse seed with reviewed explicit
  leaf classifications and safe projection rules.

## Task 3.2 — explicit strict-schema classifications and projections

- Public seam: immutable `Definition.FieldDescriptors` admitted by
  `resourceregistry.New`, consumed by the same default registry used for strict
  repository validation and composition.
- Selected evidence: every shared and kind-specific accepted leaf; inline
  metadata; nested structs and collections; arbitrary provider-option maps;
  public, sensitive-metadata, and secret classes; value, metadata, fingerprint,
  count, presence, reference, and omission projections; invalid class/projection
  pair refusal; strict-schema completeness lint; repository validation,
  composition, model, operator-config, vet, and documentation regressions.
- Red command: `go test -mod=vendor ./internal/resourceregistry -run
  TestDefaultRegistryClassifiesNestedFieldsAndSafeProjections -count=1`.
- Intended red observed: the test did not compile because `SafeProjection` and
  the required value/reference/fingerprint/omit projection constants did not
  exist; all fields still inherited a coarse whole-resource sensitivity.
- Green command: the same focused command passed after adding explicit
  per-kind policy for every reflected accepted path. Focused cases prove raw
  file content and environment values are omitted secrets, secret references
  expose only reference metadata, SSH fingerprints use the fingerprint shape,
  SSH key bytes are omitted sensitive metadata, provider-option wildcards are
  omitted secrets, and `kind` remains a public typed value.
- Completeness-linter command: `go test -mod=vendor
  ./internal/resourceregistry -run
  TestDefaultFieldPoliciesCoverStrictSchemas -count=1` passed. For each kind,
  the test independently derives and sorts paths from the strict Go schema,
  compares them to the explicit policy table, and rejects a missing, extra,
  duplicate, or invalid descriptor.
- Negative/boundary evidence: registration tests reject secret-as-raw-value,
  public-as-omit, missing, invalid, and unknown field descriptors. Descriptor
  maps remain immutable after registration.
- Regression command: `go test -mod=vendor ./internal/resourceregistry
  ./internal/models ./internal/configrepo ./internal/configcompose
  ./internal/operator/config -count=1` passed, followed by focused `go vet`.
- Documentation command: `uv run --offline --with
  'mkdocs-material>=9.6,<10' -- bash scripts/build-docs-site.sh` passed from the
  pinned ephemeral cache. Existing unrelated missing-nav warnings remained
  warnings; the site built successfully.

## Task 3.3 — OS-AEC-074 agent report and Sync sink admission

- Public seams: composed agent execution through `engine.NewForExecution` and
  `CheckAll`/`ApplyAll`, followed by authenticated Sync request construction
  through `sync.Pending.SetFromPipeline` and `Pending.Request`.
- Selected evidence: a single malicious provider injects an independently
  known canary into desired and observed Check summaries, child summaries,
  Apply diagnostics, and the Apply error. The test inspects the engine report,
  a representative operational error log, and the JSON Sync request. A
  schema-derived file-resource projection separately proves classified public
  and metadata retention plus secret omission.
- Red command: `go test -mod=vendor ./internal/agent/engine -run
  '^TestEngineAndSyncRejectArbitraryProviderCanary$' -count=1`.
- Intended red observed: the engine report copied every malicious provider
  string, including all four summary/diagnostic positions, and retained the raw
  Apply error. Those values were therefore available to log and Sync sinks.
- Green command: the same focused command passed after engine admission began
  accepting only registry-produced `SafeSummary`, stable typed health, and
  `SafeError` values with no raw message/cause. `SafeSummary.MarshalJSON`
  revalidates every class/projection/value-shape combination at sink entry.
- Projection command: `go test -mod=vendor ./internal/resourceregistry -run
  '^TestResourceSafeSummaryProjectsClassifiedFieldsAndOmitsSecretCanary$'
  -count=1` passed. It proves the literal file-content canary is absent while
  public `kind` and sensitive path metadata retain their approved shapes.
- Regression command: `go test -mod=vendor ./internal/executor
  ./internal/resourceregistry ./internal/agent/engine ./internal/agent/sync
  ./internal/agent/pipeline ./internal/agent/cronexec ./cmd/remotr-agent
  -count=1` passed with local test sockets enabled. A repository-wide compile
  run also passed.
- End-to-end compatibility command: `go test -mod=vendor ./internal/server
  -run '^TestSecretCanaryIsAbsentFromLogsSyncAPIAndCLI$' -count=1` initially
  failed at the Admin API because the legacy state-report reader expected
  strings. It passed after adding a narrow reader that validates classified
  version-7 objects and renders them for the legacy model. The test covers
  agent logs, authenticated Sync ingestion, durable in-memory round trip,
  Admin API output, and the real CLI formatter without exposing the canary.

## Task 3.4 (state-report slice) — durable classified round trip

- Public seams: authenticated `POST /v1/sync`, `SyncTelemetry`, Postgres
  `InsertDriftReport`/`GetEndpointStateReport`, Admin state-report JSON, the
  operator client, and human/JSON CLI formatting.
- Selected evidence: invalid version-7 secret-as-value refusal before any
  telemetry call; direct Postgres store refusal before its query boundary;
  classified JSONB serialization and restart read; legacy unclassified summary
  omission; typed Apply-failure persistence; Admin/client/CLI regressions; the
  existing full secret-canary path; docs build.
- Red command: `go test -mod=vendor ./internal/server -run
  '^TestSyncRejectsUnclassifiedVersion7StateReportBeforePersistence$'
  -count=1`.
- Intended red observed: authenticated Sync returned `200` and passed the
  invalid secret raw-value summary to the telemetry mock unchanged.
- Green behavior: Sync parses a typed `registry.StateReportPayload` before
  artifact/persistence work and returns `400` for an invalid classified shape.
  `SyncTelemetry` and the Postgres store accept the typed payload rather than
  arbitrary bytes; the store validates before JSONB serialization and validates
  again on read. Legacy summary strings are deliberately removed.
- Durable negative command: `go test -mod=vendor
  ./internal/store/postgres -run
  '^TestInsertDriftReportRejectsInvalidClassifiedSummaryBeforeDatabase$'
  -count=1` passed and proved the query was never called.
- Regression command: `go test -mod=vendor ./internal/executor
  ./internal/registry ./internal/store/postgres ./internal/server
  ./internal/admin ./cmd/remotr -count=1` passed with local test sockets.
- Documentation command: `uv run --offline --with
  'mkdocs-material>=9.6,<10' -- bash scripts/build-docs-site.sh` passed with the
  pre-existing missing-nav warnings. The API reference now documents the
  classified summary object and legacy omission behavior.
- Remaining task 3.4 work: migrate Change-control persistence, Admin API, and
  CLI output through classified safe types and run its restart canary.

## Task 3.4 (predicted-effect slice) — classified Change-control plans

- Public seams: baseline-adoption Admin API, persistent Change-control
  registry reconstruction, Admin detail JSON, operator human/JSON formatting,
  and desktop typed-to-view projection.
- Selected evidence: unclassified string refusal at the Admin boundary;
  invalid secret-as-value refusal before the state-store write; classified
  effect persistence and reconstruction; exact canary absence from durable
  bytes and post-restart Admin output; human/JSON CLI rendering; restore-only
  compatibility for old version-1 plans; desktop and docs regressions.
- Red commands: `go test -mod=vendor ./internal/changecontrol -run
  '^TestPersistentRegistryRejectsUnclassifiedPredictedEffectCanary$' -count=1`
  first accepted and persisted the canary; `go test -mod=vendor
  ./internal/server -run '^TestAdminChangeControlSurvivesServerRestart$'
  -count=1` then returned `200` for a public JSON string effect and restored it
  as plan evidence; `go test -mod=vendor ./cmd/remotr -run
  '^TestChangeOutputSupportsHumanAndJSONContracts$' -count=1` omitted effects
  from human output.
- Green behavior: `PredictedEffect` admits only a closed code and validated
  `SafeSummary`. Registry validation runs before mutation/persistence and JSON
  marshal/unmarshal revalidates the shape. Legacy version-1 strings are
  discarded only inside durable restoration and become the closed
  `legacy_unclassified_effect` marker; the public API rejects them.
- Regression commands: `go test -mod=vendor ./internal/changecontrol
  ./internal/server ./internal/admin ./cmd/remotr -count=1` passed with local
  test sockets; the separate desktop `go test ./...` suite passed with local
  sockets after its typed fixtures were updated. The offline docs build passed
  with only the pre-existing missing-nav warnings.
- Remaining task 3.4 work: classify related Change-control evidence and the
  durable generic audit-detail sink before marking the task complete.

## Task 3.4 (audit-detail slice) — classified durable API audit fields

- Public seams: semantic handler annotation, Postgres `RecordAuditEvent` and
  durable row reconstruction, Admin list/export JSON, typed operator-client
  JSON, and desktop Activity projection.
- Selected evidence: invalid secret-as-value refusal before the database query;
  classified secret-presence durable encode/read; legacy arbitrary map
  omission; an endpoint-label value canary projected only as presence; exact
  canary absence from Admin and CLI JSON; all producer migration; server,
  client, CLI, desktop, and docs regressions.
- Red command: `go test -mod=vendor ./internal/store/postgres -run
  '^TestRecordAuditEventRejectsUnclassifiedDetailsBeforeDatabase$' -count=1`.
  Intended red observed: the arbitrary map, including the literal canary, was
  marshalled and passed to `InsertAuditEvent` with no error.
- Green behavior: `audit.Event.Details` is a typed `SafeSummary`; every handler
  supplies explicit classified fields and projections. Postgres validates
  before insert and after durable read. Legacy detail maps are not
  reclassified—the containing events remain visible while their details are
  omitted. Admin/client JSON retains the typed shape, the CLI refuses an
  invalid hand-built shape, and desktop renders only the typed projection.
- Focused commands: the persistence rejection/round-trip tests, endpoint-label
  canary test, Admin list-output test, and CLI audit-output test all passed.
  `go test -mod=vendor ./internal/audit ./internal/store/postgres
  ./internal/server ./internal/admin ./cmd/remotr -count=1` passed with local
  sockets, followed by the separate desktop `go test ./...` suite.
- Task 3.4 completion boundary: state reports, Apply failures,
  Change-control predicted effects, and durable audit details now enter
  persistence and Admin/CLI output only through classified safe types. The
  provider-derived activation-target, rollback-class, baseline-eligibility,
  and authoritative-plan contract remains explicitly assigned to tasks
  4.3-4.4.

## Task 3.5 (diagnostic slice) — metadata-only classified bundles

- Verification and public seams: OS-AEC-084 through agent diagnostic
  collection, the upload boundary, authenticated diagnostic-result Sync,
  Postgres request completion/restart read, server object admission/download,
  Admin/client/CLI/Desktop failure output, and the archive validator.
- Red command: `go test ./internal/agent/diagnostics -run
  '^TestCollectProjectsSecretBearingSourcesIntoClassifiedMetadata$' -count=1`.
  Intended red observed: both `journal/remotr-agent.log` and
  `remotr/state.json` retained the exact source canary in the tar.gz artifact.
- Green behavior: every collector now emits only a validated `SafeSummary`
  containing byte/line counts, collection presence, and a source fingerprint.
  A closed manifest-to-file validator rejects raw/unknown entries and invalid
  summary shapes before upload and before ready/download admission.
- Parser-boundary red command: `go test ./internal/diagnostics -run
  '^TestValidateBundleRejectsTrailingManifestData$' -count=1` initially
  accepted a second JSON document after the manifest; the focused command
  passed after exact EOF admission was added.
- Durable-failure red command: `go test ./internal/store/postgres -run
  '^TestCompleteDiagnosticRequestRejectsUnclassifiedFailureBeforeDatabase$'
  -count=1` initially wrote a raw failure string. It passed after diagnostic
  failures became `SafeError` through agent logs, Sync, persistence, Admin,
  CLI, and desktop projections; legacy unclassified strings are discarded on
  restart read.
- Regression command: `go test ./internal/diagnostics
  ./internal/agent/diagnostics ./internal/agent/sync ./cmd/remotr-agent
  ./internal/store/postgres ./internal/server ./internal/admin ./cmd/remotr
  -count=1` passed with loopback test sockets enabled. Root and desktop
  repository-wide compile runs also passed.

## Task 3.5 (backup/rollback slice) — classified recovery metadata

- Verification and public seams: OS-AEC-084 through server startup after a
  Postgres/keyring restore, `CheckKeyCoverage`, the rollback transaction
  envelope, `rollbackstore.Records`, and server secret rollback references.
- Rollback red command: `go test ./internal/rollbackstore -run
  '^TestSensitiveRollbackMetadataOmitsPayloadFingerprintAndSerializesClassified$'
  -count=1`. Intended red observed: the envelope header exposed the exact
  SHA-256 of the sensitive payload and `RecordInfo` used ordinary struct JSON.
- Green behavior: new envelope version 2 relies on AES-GCM authentication and
  omits the unkeyed payload checksum. Version-1 envelopes remain recoverable
  and transition to checksum-free form. Agent and server rollback metadata
  marshal only through classified `SafeSummary` projections.
- Restore-projection red command: `go test ./internal/secrets -run
  '^TestKeyCoverageAndRemovalProtectionIdentifyReferencedHistoricalKEKs$'
  -count=1` initially produced an unclassified gap object. Coverage gaps now
  retain only classified KEK metadata and secret references.
- Startup red command: `go test ./cmd/remotr-server -run
  '^TestSecretProviderStartupValidatesRestoredDatabaseKeyCoverage$' -count=1`
  initially failed to compile because no restore admission existed. With
  secrets enabled, startup now validates every encrypted Postgres record and
  refuses to serve if a referenced current/historical KEK is missing.
- Provider-error red command: `go test ./internal/secrets -run
  '^TestKeyCoverageClassifiesProviderFailure$' -count=1` initially returned the
  injected provider canary in the error chain. It passed after conversion to a
  stable `SafeError` with classified coverage details.
- Regression command: `go test ./internal/rollbackstore ./internal/secrets
  ./internal/store/postgres ./cmd/remotr-server -count=1` passed, followed by a
  repository-wide compile. The offline documentation build passed with only
  the pre-existing missing-nav warnings.

## Task 3.6 — focused classified-projection mutation evidence

- Verification and public seams: OS-AEC-083 and OS-AEC-084 through
  `SafeSummary`/`SafeError` JSON admission, `Resource.SafeSummary`, diagnostic
  tar.gz validation, Postgres diagnostic completion/restart read, restored-key
  coverage, rollback `RecordInfo`, and server diagnostic-result completion.
- Mutation red: initial Mewt 3.0.1 runs left security-relevant survivors that
  could remove closed matrix checks, share mutable classified pointers, omit
  presence/count metadata, weaken diagnostic manifest and source-summary
  validation, strip recovery context, or bypass the ready-object validation
  block. In particular, server ER mutant 5284 survived because no test drove a
  ready result through object retrieval and durable completion.
- Green evidence: focused public-seam tests now cover the complete
  sensitivity/projection matrix, deep-copy and JSON behavior, nested/wildcard
  projection, exact presence semantics, archive size/path/count/manifest/file
  and source-summary boundaries, classified Postgres round trips, optional
  recovery metadata, provider-error detail conversion, and rollback metadata.
  `TestPersistDiagnosticResultAdmitsOnlyClassifiedBundleBeforeReady` uses a
  test-only S3 endpoint to prove that a valid classified bundle reaches
  `ready`, while a raw/invalid object becomes `failed`, loses claimed
  digest/size, receives a stable `SafeError`, and is deleted.
- Campaign result: 1,997 current mutants were evaluated across seven focused
  targets: 819 caught, 175 uncaught, 1,002 severity-short-circuited, and one
  unrelated retention timeout. Individual survivor inspection found zero
  unexplained redaction bypasses. The surviving task-adjacent variants remain
  fail-closed through a second validator or alter only invariant/no-op/logging
  behavior; they are documented in
  `engineering/testing/mutation-testing-pilot.md` and are not accepted into a
  mutation gate or survivor baseline. Broader non-redaction survivors remain
  pilot backlog.
- Focused regressions: `go test -mod=vendor ./internal/executor
  ./internal/resourceregistry ./internal/diagnostics
  ./internal/store/postgres ./internal/secrets ./internal/rollbackstore`
  passed. `go test -mod=vendor ./internal/server` passed with loopback enabled.
  Mewt restored every production target after each run, and `git diff --check`
  passed before the test checkpoint commit.

## Task 4.1 — versioned canonical effective hash

- Verification and public seam: OS-AEC-085 through the shared
  `internal/effectivehash` `Canonical` and `Sum` API that composition and agent
  resolution consume in task 4.2.
- Canonical-contract red command: `go test -mod=vendor
  ./internal/effectivehash` initially failed because the package did not exist.
  The green test locks independently specified version-1 canonical JSON and a
  separately calculated SHA-256 digest.
- Default-semantics red command: the focused package test next failed to
  compile because no schema-default input existed. The green implementation
  recursively applies only caller-declared managed defaults, so omitted and
  explicit defaults converge while an omitted unmanaged field remains
  distinct from an explicit zero value.
- Scalar-domain red command: the focused package test failed to compile before
  unsigned and floating-point canonical values existed. The green
  implementation admits the complete finite JSON scalar domain, normalizes
  negative zero, and rejects NaN and infinity.
- Property and negative evidence: 100 deterministic permutations prove nested
  object, set, and secret-identity ordering invariance while ordered-list
  permutations change the hash. Table cases prove provider contract revision,
  resolved secret version, and activation generation changes invalidate the
  prior hash. The secret identity type has no material field, and the
  canonical secret-canary assertion proves raw bytes are absent.
- Focused regression: `go test -mod=vendor ./internal/effectivehash` passed.

## Task 4.2 — composition, agent, and trust-boundary hash enforcement

- Verification and public seams: OS-AEC-085 through composed registered
  Resources, agent resolution/engine Check and Apply, authenticated Sync state
  reports, Change-request creation, baseline eligibility, preflight/lease
  issuance, and agent lease admission.
- Resource-derivation red command: `go test -mod=vendor
  ./internal/resourceregistry -run
  '^TestDecodedResourceDerivesSharedCanonicalEffectiveHash$' -count=1`
  initially failed because Resources had no effective-hash API or registered
  provider contract revision. The green path preserves source presence,
  merges typed normalization/defaults, and calls the shared package.
- Secret-identity red command: the focused registry test initially hashed the
  authored `@active` reference alongside supplied metadata. The green path
  removes every secret-classified reference from desired values, requires a
  matching provider/name/version/generation identity and purpose, and rejects
  missing/extra identities. Agent resolution extracts that identity through
  the provider resolver and clears returned material before hashing.
- Agent-resolution red command: `go test -mod=vendor ./internal/agent/engine
  -run '^TestEngineReportsCanonicalHashFromParsedResolvedResource$' -count=1`
  initially had no hash/revision report fields. Schema-1 source nodes now
  survive parse and target filtering; Check and Apply results carry the same
  canonical identity. Direct schema-0/test fixtures remain visible as legacy
  hash-contract version 0 rather than being silently rebound.
- Report red commands: the structured Sync test initially emitted schema 7
  without resource identities, and the registry version-8 negative table
  initially accepted missing, malformed, duplicate, and Check/Apply-conflicting
  hashes. Canonical reports now emit schema 8 and durable admission requires a
  lowercase SHA-256, provider revision, unique address, and identical Apply
  evidence; legacy reports remain readable under their prior version.
- Lease/agent red commands: focused Change-control and engine tests initially
  issued a lease from stale preflight hashes and ran a high-risk provider with
  a stale delivered lease. Version-1 plans, preflights, baselines, leases, and
  agents now bind exact hashes and contract/provider revisions. The agent
  validates lease structure/expiry under an injected clock and rejects before
  dependency mutation, preflight, or Apply.
- Change-request boundary red command: a syntactically valid caller SHA-256
  initially could claim canonical authority. The legacy constructor now
  rejects version-1 claims; the canonical constructor requires an exact
  one-to-one match with trusted composition address/provider/revision/hash
  evidence.
- Composition red command: `go test -mod=vendor ./internal/configcompose -run
  '^TestEffectiveResourcesDeriveCompositionHashesFromTrustedProviderSelections$'
  -count=1` initially failed because no composition identity API existed. The
  green API iterates composed registered Resources, requires exact trusted
  provider selections and schema-1 presence evidence, resolves safe secret
  identities, and returns a stable address-sorted identity set for task 4.4.
- Focused regressions passed for effective hashes, models, resource registry,
  composition, secrets, agent resolution/engine/Sync, state-report registry,
  Postgres, Change control, agent command, and server. Sync/server regressions
  ran with loopback sockets enabled.

## Task 4.3 — bounded provider plan descriptors

- Verification and public seam: OS-AEC-086 through the registered Resource
  provider contract consumed by server-derived planning in task 4.4.
- Closed-evidence red command: `go test -mod=vendor
  ./internal/providercontract -run
  '^TestPlanDescriptorRejectsFreeFormAndSecretBearingEvidence$' -count=1`
  initially failed to compile because no plan descriptor or typed effect
  contract existed. The green contract admits only six closed effect codes,
  validated `SafeSummary` parameters, three rollback classes, nine typed
  activation kinds, and bounded effect/detail/activation/target counts.
- Registration red command: `go test -mod=vendor
  ./internal/resourceregistry -run
  '^TestRegistryRejectsMissingProviderPlanDescriptor$' -count=1` initially
  failed to compile because a registered Resource definition did not own plan
  evidence. Registration now fails when the descriptor factory is absent, and
  `Resource.PlanDescriptor` validates every provider-produced value before it
  can cross the registry seam.
- Provider-adoption reds proved typed sudo, browser, firewall, DNS, default
  route, notification, next-boot, service restart, logout, daemon reload,
  trust-store refresh, transactional rollback, and baseline eligibility
  evidence. Firewall and network-profile rollback claims follow audit versus
  enforced mode; one-shot reboot and destructive command resources cannot
  become baselines.
- Free-form activation red command: `go test -mod=vendor
  ./internal/resourceregistry -run
  '^TestDownloadPlanDescriptorRejectsFreeFormActivationEvidence$' -count=1`
  initially omitted even the recognized typed activation. The green adapter
  translates only the existing closed `systemctl` reload/restart forms and
  rejects arbitrary argv without copying it into the error or plan. A
  malicious registered descriptor carrying a raw secret projection is also
  rejected at `Resource.PlanDescriptor`.
- Focused regressions passed for provider contract, resource registry,
  composition, agent resolution, and agent engine. Task 4.4 remains the owner
  of converting these validated descriptors into authoritative server plans.

## Task 4.4 — server-derived baseline-adoption plans

- Verification and public seams: OS-AEC-086 through the authenticated Admin
  baseline-adoption route, canonical Change-control admission, operator client
  and CLI, and desktop confirmation/transport boundary.
- Admin authority red command: `go test -mod=vendor ./internal/server -run
  '^TestAdminBaselineAdoptionRejectsCallerSuppliedConflictingHash$' -count=1`
  initially returned `200` and persisted the caller's conflicting hash. The
  route now accepts only an empty body and rejects every caller plan field.
- Derivation green command: `go test -mod=vendor ./internal/server -run
  '^TestAdminBaselineAdoptionDerivesCanonicalPlanFromServerArtifact$'
  -count=1` passed. The server resolves its current composed Fleet artifact,
  requires server-owned provider selections, derives canonical hashes and
  registered provider descriptors, and uses exact trusted-identity admission.
  Provider-owned baseline ineligibility is preserved.
- Client migration evidence: focused Admin client, operator command-shape, and
  desktop parity tests prove that only `{}` reaches the mutation route. The CLI
  no longer accepts `--file`; the desktop no longer opens, parses, previews, or
  retains a plan document and binds its one-use confirmation token only to the
  exact Fleet.
- Persistence regression: older restart and failure tests now use the
  server-derived API seam, or directly seed a legacy registry plan when the
  subject is lease persistence rather than plan authority. The complete
  `internal/server` suite passed with loopback sockets enabled; focused
  `internal/admin`, `cmd/remotr`, and desktop parity tests passed. The required
  `make test` run passed with loopback enabled, including all 37 discovered
  fuzz seed corpora and the complete vendored Go suite. All 33 frontend test
  files (73 tests) passed.
- Contract and documentation validation: strict OpenSpec validation and the
  traceability linter passed. The offline MkDocs build passed with only its
  pre-existing missing-nav warnings; the operator guide, API reference, and
  desktop parity contract now describe the server-derived request boundary.
- This slice deliberately left endpoint evidence to task 4.5. Task 4.5 closed
  that boundary with authenticated schema-9 reports and removed the temporary
  provider-selection source.

## Task 4.5 — authenticated endpoint evidence and producer completion

- Verification and public seams: OS-AEC-086 through agent provider Check,
  authenticated Sync, durable state-report admission, the Admin baseline and
  secret-activation routes, registered provider contracts, and canonical
  Change-control admission. Evidence layers are focused agent/server contract
  tests, persistence round trips, negative mismatch cases, and the repository
  regression suite.
- Non-enforcing preflight red command: `go test -mod=vendor
  ./internal/agent/engine -run
  '^TestEngineCheckAllReportsNonEnforcingHighRiskPreflight$' -count=1`
  initially had no provider preflight evidence in Check results. The green
  engine calls high-risk preflight without mutation and emits only the closed
  `not_required`, `ready`, or `blocked` status and stable reason code. A
  companion malicious-provider test proves raw error text is discarded.
- Durable evidence red command: `go test -mod=vendor ./internal/registry -run
  '^TestParseStateReportPayloadVersion9RequiresClosedPreflightEvidence$'
  -count=1` initially had no schema-9 contract. The green agent emits schema 9,
  and memory/Postgres admission preserves the authenticated schema version
  while rejecting absent, unknown, or unbounded preflight evidence.
- Endpoint-join red command: `go test -mod=vendor ./internal/server -run
  '^TestAdminBaselineAdoptionDerivesCanonicalPlanFromServerArtifact$'
  -count=1` initially depended on a stubbed server provider-selection source
  and could not freeze current endpoint evidence. The green deriver groups
  authenticated report cohorts deterministically, recomputes the registered
  canonical plan, requires exact Release, artifact, provider revision, and
  effective hash, and freezes every Fleet endpoint as ready, blocked, missing,
  stale, or incompatible. No matching current cohort fails closed.
- Release-identity red command: `go test -mod=vendor ./internal/server -run
  '^TestSyncPersistsStateReportUnderAgentReportedRelease$' -count=1` initially
  labeled Check evidence with the server's newest Release. The green Sync path
  stores it under the bounded agent-reported Release, preventing stale evidence
  from being promoted into a current plan.
- Remaining-producer red command: `go test -mod=vendor ./internal/server -run
  '^TestAdminSecretActivationCreatesConnectivityChangeBeforeResolution$'
  -count=1` initially produced a legacy hash-contract-version-0 request by
  copying or synthesizing activation inputs. The green coordinator resolves a
  proposed safe secret-version identity through the shared deriver, keeps the
  affected dependency closure, retains provider-owned typed effects and
  rollback evidence, appends a classified secret-activation effect, and uses
  trusted canonical admission. The public test permits only the expected
  affected-resource hash change, rejects a stale provider revision with no
  request, freezes ready endpoint evidence, and proves no secret material is
  exposed.
- Focused regressions passed for executor, agent engine/Sync, state-report
  memory and Postgres stores, resource registry, server, secrets, composition,
  Change control, and the server command. The required `make test` run passed
  with loopback enabled, including all 37 discovered fuzz seed corpora and the
  complete vendored Go suite.
- Contract and documentation validation: strict OpenSpec validation and the
  traceability linter passed. The offline MkDocs build passed with only the
  pre-existing missing-nav/link warnings. The operator guide, API reference,
  design explanation, migration inventory, and traceability disposition now
  describe authenticated endpoint evidence as implemented rather than a stub.

## Task 4.6 — dependency, reservation, and break-glass safety

- Verification and public seams: OS-AEC-087 through provider Check and
  rollback-reservation preflight, authenticated schema-9 Sync evidence, the
  Admin baseline-adoption route, canonical Change-control admission, execution
  lease issuance, and the persisted break-glass model. Evidence layers are
  provider-contract tests, real filesystem/network recovery-store probes,
  authenticated Admin API regression, negative hash/redaction/preflight cases,
  and the repository regression suite. VM, focused mutation, and final
  lifecycle promotion evidence remain owned by tasks 5.2, 5.5, and 5.6.
- Lease red command: `go test -mod=vendor ./internal/changecontrol -run
  '^TestExecutionLeaseRejectsFrozenRollbackReservationBlock$' -count=1`
  initially issued a lease for a frozen target whose fresh attempt report was
  ready but whose frozen rollback reservation was blocked. The green lease
  gate requires the frozen target itself to remain compatible and ready.
- Dependency/reservation red command: `go test -mod=vendor
  ./internal/agent/engine -run
  '^TestEngineCheckAllBlocksHighRiskDependentWhenRollbackReservationFails$'
  -count=1` initially failed to compile because the provider contract had no
  rollback-reservation preflight and no closed reservation/dependency reason
  codes. The green engine probes rollback capacity before Apply, emits
  `rollback_reservation_failed`, and propagates `dependency_blocked` through
  normal-risk prerequisites into affected high-risk resources without copying
  provider error text.
- Real reservation probes: focused tests for
  `TestApplicatorPreflightRollbackProbesCapacityWithoutArming` and
  `TestStorePreflightProbesNetworkReservationWithoutArming` initially failed
  because file and network transaction stores had no non-mutating reservation
  API. The green path reserves and releases the exact protected payload size,
  leaves no armed record or target mutation, and is implemented by every
  provider still advertising `transactional` or `best_effort` rollback.
- Authorization-group red command: `go test -mod=vendor
  ./internal/changecontrol -run
  '^TestRegistryScopesDependencyPreflightBlocksToAffectedChangeGroup$'
  -count=1` initially failed to compile because frozen targets had only one
  aggregate readiness value. Per-resource evidence now preserves the affected
  dependency closure while leaving unrelated authorization groups ready.
- Break-glass red command: `go test -mod=vendor ./internal/changecontrol -run
  '^TestBreakGlassCannotBypassCanonicalRequestSafetyEvidence$' -count=1`
  initially failed to compile because callers supplied Fleet, risk, hashes,
  and safeguard booleans without a canonical Change request. Creation and use
  now bind to a version-1 request, exact server-derived hashes and targets,
  validated classified effects, current resource preflights, rollback
  evidence, expiry, and attempt bounds. Legacy records with no request binding
  remain readable but cannot enforce.
- Authenticated Admin red command: `go test -mod=vendor ./internal/server -run
  '^TestAdminBaselineAdoptionPreservesNormalDependencyReservationBlock$'
  -count=1` proved that a normal file reservation failure and its dependent
  high-risk sudo block survive schema-9 ingestion, server derivation,
  authorization-group scoping, and target freeze.
- Focused regressions passed for executor, agent engine and network recovery,
  rollback store, every rollback-advertising provider, Change control, Sync,
  state-report memory/Postgres registries, server, composition, and the
  registered Resource contract. The required `make test` run passed with
  loopback enabled, including all 37 discovered fuzz seed corpora and the
  complete vendored Go suite.
- Contract and documentation validation: strict OpenSpec validation and the
  traceability linter passed. The offline MkDocs build passed with only its
  pre-existing missing-nav/link warnings. The operator guide, API reference,
  design explanation, migration inventory, and traceability disposition now
  describe dependency/reservation and break-glass safety as implemented while
  preserving the remaining VM, mutation, and promotion work as planned.

## Task 4.7 — visible legacy quarantine and explicit regeneration

- Verification and public seams: persisted Change-control restore, rollout and
  baseline authorization, execution lease issuance, authenticated Admin API,
  operator client and CLI, and restart durability. Evidence layers are the
  independently authored task-1.3 compatibility fixture, negative persisted
  state admission, authenticated route tests, CLI/parity contracts, and the
  repository regression suite.
- Legacy-quarantine red command: `go test -mod=vendor
  ./internal/changecontrol -run
  '^TestLegacyPersistedPlanCompatibilityFixture$' -count=1` initially failed to
  compile because a restored request had no visible migration status. The
  green restore path annotates the legacy request, rollout, and baseline with
  `non_enforcing`, preserves the old `authorized` lifecycle and caller hash as
  audit evidence, and proves the rollout, lease, and baseline gates all reject
  it. Persistent lifecycle tests now seed canonical identities so restart
  continuity is never proved with caller-authored authority.
- Explicit-regeneration red command: `go test -mod=vendor ./internal/server
  -run
  '^TestAdminRegenerateLegacyAuthorizationCreatesSeparateCanonicalPendingRequest$'
  -count=1` initially returned `404`. The green authenticated route accepts
  only an empty body, obtains the Fleet from the legacy request, derives current
  composition and schema-9 endpoint evidence, records a closed deterministic
  comparison, and creates a distinct canonical pending request. The old hash,
  approval, rollout, and baseline are not rewritten or copied; comparison and
  replacement lineage survive registry reconstruction in one durable commit.
- Operator-seam red command: focused `internal/admin` and `cmd/remotr` tests
  initially failed to compile because `RegenerateChangeRequest` and `change
  regenerate` did not exist. The green client sends only `{}` for the escaped
  legacy ID; the CLI accepts no Fleet, file, hash, provider, or effect flags and
  makes `non_enforcing` visible in list/show output. The desktop parity inventory
  records this workflow as planned and makes no unsupported desktop claim.
- Tamper red command: `go test -mod=vendor ./internal/changecontrol -run
  '^TestPersistentRegistryRejectsLegacyMigrationClaimingEnforcement$'
  -count=1` initially restored attacker-authored `enforcing` migration state
  without error. Startup now admits only the closed non-enforcing reason and
  replacement states, requires regenerated lineage to reference a canonical
  request, recomputes the stored comparison, and requires rollout/baseline
  migration status to match the owning legacy request.
- Focused regressions passed for Change control, Admin client, operator CLI,
  CLI/desktop parity, server restart and Sync lease persistence, the full
  authenticated server suite, and desktop documentation/release-review count
  gates.
- The required `make test` run passed with loopback enabled after the parity
  inventory and its published count were reconciled, including all 37
  discovered fuzz seed corpora and the complete vendored Go suite.
- Contract and documentation validation: strict OpenSpec validation and the
  traceability linter passed. The offline MkDocs build passed with only its
  pre-existing missing-nav/link warnings. The operator guide, CLI and HTTP
  references, design explanation, desktop parity inventory, migration
  inventory, and traceability dispositions now describe the explicit
  non-rebinding legacy workflow.
