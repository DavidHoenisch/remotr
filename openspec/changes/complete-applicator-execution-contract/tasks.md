## 1. Establish Traceability and Migration Baselines

- [x] 1.1 Register this child as an authorized modifier of the umbrella capability, support canonical verification-ID lineage for one authorized delta, register OS-AEC-080 through OS-AEC-087, and reconcile OS-AEC-068 through OS-AEC-074 in `test/traceability.yaml` with truthful planned selectors, public seams, and required provider/VM/mutation/performance layers.
- [x] 1.2 Inventory every provider that reports `transactional` or `best_effort`, every resource schema field, every generic output/persistence/backup sink, and every producer of `changecontrol.FleetPlan`; record the migration owner and current unsafe or partial behavior for each.
- [x] 1.3 Add a compatibility fixture for current rollback records and persisted Change-control plans so migration failures and intentionally non-enforcing legacy state have independently known expected results.

## 2. Complete the Protected Transaction Store

- [x] 2.1 For OS-AEC-080, write and observe a focused red restart-recovery test at the system-safety recovery seam, then implement the minimum versioned transaction lifecycle and startup scan needed to recover one armed record green.
- [x] 2.2 For OS-AEC-069, write and observe a focused red capacity-reservation test, then implement pre-mutation reservation covering encrypted payload, metadata, filesystem allowance, and protected armed records.
- [x] 2.3 For OS-AEC-081, write and observe deterministic boundary/property tests for attempt, successful-state, age, sensitivity, supersession, and disk limits, then implement pruning without wall-clock sleeps or armed-record eviction.
- [x] 2.4 Replace separately activated rollback payload/metadata files with one authenticated, checksummed, fsynced, atomically renamed transaction envelope and add injected crash-point tests for every durability boundary.
- [x] 2.5 For OS-AEC-070 and OS-AEC-082, write red key-selection/downgrade tests, implement a concrete capability-gated TPM key provider plus versioned root-file fallback, and report the selected protection class without exposing key material.
- [x] 2.6 Add key-rotation and historical decrypt-only retention tests proving that no armed or retained record is orphaned when the active rollback key identity changes.
- [x] 2.7 Migrate firewall and network-profile acknowledgement/rollback to transaction handles and prove Apply, restart, authenticated acknowledgement, timeout rollback, cleanup, and second Check through the provider and VM safety seams.
- [x] 2.8 Migrate file/download and certificate/trust rollback-advertising providers to transaction handles, removing generic adjacent backups and proving original-error plus rollback-outcome reporting.
- [x] 2.9 Migrate access, boot, and remaining rollback-advertising providers identified by task 1.2; downgrade any provider that cannot meet the contract to an honest rollback class and update its advertisement evidence.

## 3. Enforce Schema-Field Sensitivity

- [x] 3.1 For OS-AEC-083, write and observe a focused red resource-registration test for one unclassified accepted field, then add field descriptors and fail registration/validation for missing classifications.
- [x] 3.2 Classify every accepted field public, sensitive-metadata, or secret, including nested collections, provider options, secret references, and safe metadata projections; add a completeness linter derived from strict schema types.
- [x] 3.3 Replace arbitrary desired/observed serialization in agent logs, Check/Apply reports, Sync payloads, and provider errors with classified safe summary types and exact negative canary assertions.
- [x] 3.4 Route Postgres state-report/change-control persistence and Admin API/CLI output through classified safe types, proving the canary is absent after durable round trip and restart.
- [x] 3.5 For OS-AEC-084, write and observe a focused red generic backup/diagnostic canary test, then enforce classified projections in diagnostic bundles, database backup/restore metadata, and rollback metadata.
- [x] 3.6 Run focused mutation tests against classification, safe projection, provider-error conversion, and sink admission so no new relevant redaction bypass survives unexplained.

## 4. Derive Canonical Hashes and High-Risk Plans

- [x] 4.1 For OS-AEC-085, write and observe table/property tests for ordering, omitted versus explicit values, defaults, provider revisions, nested collections, and secret version changes, then implement one versioned canonical effective-hash package.
- [x] 4.2 Integrate canonical hash generation into composition and agent resolution, and reject mismatches at Change request, baseline, Execution lease, agent, and report trust boundaries.
- [x] 4.3 Add a provider plan-descriptor contract for bounded typed effects, rollback class, activation targets, and baseline eligibility; reject free-form or secret-bearing plan evidence.
- [x] 4.4 For OS-AEC-086, write and observe a red Admin API test accepting a caller-supplied conflicting hash, then replace caller-authored authoritative `FleetPlan` data with server-derived plans from composed registered Resources.
- [x] 4.5 Join current authenticated capability and non-enforcing endpoint Check/preflight evidence to derived plans before target freeze, preserving exact Release, artifact, provider-revision, and endpoint evidence.
- [x] 4.6 For OS-AEC-087, write and observe red dependency/reservation/break-glass cases, then integrate normal dependency closure and non-bypassable hash, redaction, preflight, and rollback-reservation blocks.
- [x] 4.7 Migrate existing authorizations through visible non-enforcing comparison and explicit regeneration; never silently bind a legacy request or baseline to a new canonical hash.

## 5. Verify Recovery, Safety, and Performance

- [ ] 5.1 Run the end-to-end secret-canary path through agent logs, Sync, Postgres, Admin API, CLI, diagnostics, backup/restore, rollback recovery, and server/agent restart; retain negative persistence and cleanup evidence.
- [ ] 5.2 Extend the Ubuntu VM fixtures for interrupted connectivity, access, boot, and secret-bearing attempts, proving restart recovery, acknowledgement, timeout rollback, abandoned recovery authorization, and second Check.
- [ ] 5.3 Add bounded fuzz properties for transaction-envelope decoding, retention cleanup, schema classification, canonical hashing, and plan dependency graphs; commit every discovered crash as a seed regression.
- [ ] 5.4 Add allocation-reporting benchmarks for transaction reservation/recovery, classified serialization, canonical hashing, and plan construction using representative 10/100/500/1,000-resource fixtures.
- [ ] 5.5 Run focused tests after every red/green slice, then provider/VM/mutation/benchmark checks selected by risk, `make test`, strict OpenSpec validation, traceability validation, and documentation validation.
- [ ] 5.6 Promote governing traceability entries only after their required selectors pass, record any expiring evidence exception, and close umbrella tasks 2.9–2.11 only after this change is accepted.
