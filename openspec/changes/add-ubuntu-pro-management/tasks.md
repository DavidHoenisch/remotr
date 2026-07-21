## 1. Traceability and qualification inventory

- [x] 1.1 Register the `OS-UPM` verification prefix and add planned traceability records for every scenario retained by this change, without marking any behavior verified before its public evidence exists.
- [x] 1.2 Add an Ubuntu Pro qualification manifest with exact Ubuntu 20.04, 22.04, 24.04, and 26.04 LTS amd64 base-attachment rows initially `untested`, required VM selectors, negative derivative cases, secret-canary obligations, and explicit non-claims for every out-of-scope platform.
- [x] 1.3 Add separate initially `untested` capability rows for every cataloged service/release/architecture/API-revision tuple and each applicable enable mode, variant, and retain/purge disable behavior; prove that no base attachment row manufactures these service capabilities.

## 2. Exact Ubuntu identity slice

- [x] 2.1 For OS-UPM-004, OS-UPM-005, OS-LPC-011, and OS-LPC-012, add one focused facts/capability test at a time for exact Ubuntu, Pop!_OS `ID_LIKE`, a second derivative, conflicting `/etc/os-release` and `/usr/lib/os-release`, malformed/duplicate keys, and exact `dpkg-vendor` evidence; run each and record the intended red failure before production changes.
- [x] 2.2 Implement the minimum strict os-release parser and exact-identity facts needed to make each identity test green while preserving existing Debian-family compatibility; add table-driven negative/boundary coverage and a bounded os-release fuzz property.
- [x] 2.3 For OS-UPM-006, OS-UPM-008, OS-UPM-009, OS-LPC-013, and OS-LPC-014, add focused capability-document tests proving exact row selection, 26.04 capability isolation, interim/future release rejection, and no sibling capability uplift; record red, implement the minimum resource-specific advertisement gate, then rerun green.

## 3. Canonical resource and validation slice

- [x] 3.1 For OS-UPM-001 through OS-UPM-003 and OS-UPM-034/035/058, add one focused configuration-CLI test at a time for lifecycle/token consistency, duplicate services, strict unknown-field and raw-argument rejection, the full stable service catalog, precise historical-service diagnostics, typed options, and explicit rejection of client-setting/event fields; record red before production changes.
- [x] 3.2 Add `UbuntuProResource` and typed service-option models plus a checked-in service contract catalog for `esm-infra`, `esm-apps`, `livepatch`, `usg`, `fips`, `fips-updates`, `realtime-kernel`, `ros`, `ros-updates`, and `anbox-cloud`; add recognized-but-unqualified historical aliases and only enough strict decoding/encoding to make each schema test green.
- [x] 3.3 Add table-driven service/option/release boundary tests and a bounded canonical-parser fuzz property covering unknown and duplicate fields, invalid enum combinations, alias confusion, option cross-product rejection, oversized lists/strings, and deterministic round trips; rerun the affected parser/config suites.
- [x] 3.4 Add field descriptors that classify `tokenRef` appropriately; prove registry completeness and safe plan/report projection through focused tests.
- [x] 3.5 Derive base, service, mode, variant, and disable-behavior capability requirements from the catalog and add a checked-in source-only config repository; prove public `config discover`, `config validate`, deterministic `config render`, stable resource identity, precise dependency requirements, and absence of generated `desired.yaml`/`crons.yaml` artifacts.

## 4. Secret authorization and redaction slice

- [x] 4.1 For OS-UPM-010 through OS-UPM-013 and OS-UPM-016, add focused authenticated secret-resolution, authorization, effective-hash, process-boundary, error, audit, plan, rollback, and report tests using a unique token canary; record red before adding the consumer.
- [x] 4.2 Authorize `ubuntuPro.tokenRef` only for purpose `ubuntu-pro-token` at the active artifact's exact endpoint/fleet/resource scope, project only safe version metadata into effective hashes, and make the focused authorization tests green.
- [x] 4.3 Wire attachment to `executil.InputRunner` and add exact argv assertions proving typed JSON uses protected stdin with `auto_enable_services: false`; verify unsupported and already-attached paths issue zero resolver/InputRunner calls and rerun all token-canary tests.

## 5. Versioned Ubuntu Pro API slice

- [x] 5.1 For OS-UPM-037 through OS-UPM-040 and OS-LPC-019/020, add one focused process-boundary test at a time for each literal endpoint, the common envelope, `--data -`, bounded typed JSON stdin, and the absence of `--args`, shell use, ordinary `pro` subcommands, localized-message control flow, and legacy fallback; record red before adapter code.
- [x] 5.2 Implement a bounded common `/usr/bin/pro api` adapter for `u.pro.version.v1`, `u.pro.status.is_attached.v1`, `u.pro.status.enabled_services.v1`, `u.pro.services.dependencies.v1`, `u.pro.attach.token.full_token_attach.v1`, `u.pro.services.enable.v1`, `u.pro.services.disable.v1`, `u.pro.security.status.reboot_required.v1`, and `u.pro.detach.v1`, adding only the endpoint needed for the current red test.
- [x] 5.3 Add table-driven and bounded-fuzz coverage for common-envelope schema/version/result/errors/warnings, stable code mapping, unknown fields, duplicate or missing members, invalid endpoint attributes, oversized output, and translated titles; fail closed without copying raw messages.
- [x] 5.4 For OS-UPM-014/015/020/021/028/029/031/032/038, add read-only provider-contract tests for unattached, attached, stable invalid/expired-contract errors, warning, unavailable/unentitled, native lock, malformed state, USG/CIS alias normalization, and bounded reports; make them green using only versioned APIs.
- [x] 5.5 Add deterministic cancellation and timeout tests using injected process/clock boundaries with no wall-clock sleeps, implement the minimum bounded execution behavior, and rerun shared provider conformance checks.
- [x] 5.6 For OS-UPM-060 and OS-LPC-022, add tests proving an invocation-only success cannot qualify a service or mode whose desired state is not durably observable; keep those tuple selectors `untested` or `unsupported` until a reviewed API or provider-native Check seam exists.

## 6. Attachment convergence slice

- [x] 6.1 For OS-UPM-010, OS-UPM-011, OS-UPM-014, and OS-UPM-024, add public provider tests proving invalid-token no-change, unattached attach, already-attached no-token behavior, post-attach Check, and second-Apply idempotence; record the focused red failure.
- [x] 6.2 Implement attachment through `pro api u.pro.attach.token.full_token_attach.v1 --data -`, zero provider-owned input buffers on all paths, require `auto_enable_services: false`, verify the returned enabled set is empty, re-probe structured API state, and add only enough behavior to make the attachment tests green.
- [x] 6.3 Add negative tests for resolver denial, network failure, stable Canonical API errors, missing endpoint, process cancellation, lost response after native success, and ambiguous post-state; implement bounded recovery checks without retrying a potentially successful attach or falling back to ordinary `pro attach`.

## 7. Service convergence slice

- [x] 7.1 For OS-UPM-017 through OS-UPM-021, OS-UPM-024, and OS-UPM-042, add one focused public provider test at a time for enable, retain-packages disable, omitted-service preservation, unentitled/unavailable state, stable warnings, USG/CIS alias normalization, and second Check; capture red then green per behavior.
- [x] 7.2 Implement partial-ownership Check and `u.pro.services.enable.v1`/`u.pro.services.disable.v1` calls with literal endpoint argv and typed protected-stdin requests, adding one ordinary service contract at a time and defaulting disable to `purge: false`.
- [x] 7.3 Drive separate red-green slices for every observable qualified row of `esm-infra`, `esm-apps`, `livepatch`, `usg`, `ros`, `ros-updates`, and `anbox-cloud`; require a post-operation Check seam for each and leave any service whose state cannot be re-observed unadvertised.
- [x] 7.4 Add unavailable, unentitled, beta/unknown, historical-name, unsupported-release, endpoint-version, malformed-result, and unexpected-message cases for each applicable service adapter; rerun the provider contract after every minimal implementation.

## 8. Service options, dependencies, and incompatibilities slice

- [x] 8.1 For OS-UPM-043 through OS-UPM-045 and OS-UPM-060, add focused configuration and provider tests for full/access-only, each cataloged variant, retain/purge, exact tuple capability requirements, no implicit downgrade, and durable observation; record red before implementing each option row.
- [x] 8.2 Implement catalog-driven typed API request construction and option-specific Check contracts; explicitly prove that enabled-service name alone does not distinguish access-only mode and keep any mode without a stable observation seam unadvertised.
- [x] 8.3 For OS-UPM-046 through OS-UPM-049, add focused tests for declared/already-satisfied dependencies, omitted disabled dependencies, declared incompatibility transitions, omitted enabled conflicts, deterministic graph order, cycles or unknown graph members, and response-set mismatches; record red per behavior.
- [x] 8.4 Implement bounded `u.pro.services.dependencies.v1` parsing, checked-in graph reconciliation, dependency-safe transition planning, and exact enabled/disabled response-set verification; block mutation unless every native automatic transition is explicitly owned or already satisfied.
- [x] 8.5 Add drift-during-Apply and native-side-effect fault cases; implement applicable best-effort restoration without silently expanding ownership and rerun partial-ownership regressions.

## 9. Specialized services slice

- [x] 9.1 For OS-UPM-050 through OS-UPM-053, add one red-green provider slice for USG's tooling-only claim, FIPS, FIPS Updates, real-time-kernel variants, Livepatch incompatibilities, reboot signals, access-only behavior, explicit purge, and no-automatic-rollback boundaries.
- [x] 9.2 Implement cataloged risk, locks, native observations, activation, and recovery metadata for each specialized service tuple; prove impossible FIPS/FIPS Updates/real-time-kernel/Livepatch combinations are rejected before mutation.
- [x] 9.3 Add behavior-specific VM fixtures for FIPS streams, real-time-kernel variants, Livepatch interactions, Anbox Cloud, ROS, USG, and purge. Exercise public control flow, boot/recovery, and residual-effect reporting through deterministic API doubles without claiming live entitled package, snap, repository, kernel, or compliance-tool effects.

## 10. Risk, locking, activation, and state slice

- [x] 10.1 Add focused public plan tests proving attach and ordinary enablement default to `sensitive`, service-specific boot/install operations take their cataloged maximum risk, disable/purge/detach are destructive, authors cannot lower computed risk, and secret/contract details never enter the plan; record red and implement dynamic descriptors.
- [x] 10.2 Add lock tests proving mandatory `ubuntu-pro` and `package-manager:apt` domains plus cataloged snap and boot domains survive authored inputs and serialize with competing work; implement registry wiring and make contention/cancellation evidence green.
- [x] 10.3 For OS-UPM-033, add focused typed-operation and reboot-status API tests for every result value, record red, implement the standard reboot-required activation signal, and prove the provider never invokes a reboot command.
- [x] 10.4 Add fleet state-report tests for bounded attachment, only API-established contract/entitlement outcomes, declared service state, warnings, last outcome, rollback/residual-effects class, and reboot status; implement only the safe structured projection and rerun canary/redaction coverage.
- [x] 10.5 For OS-UPM-058/059, add regression tests proving client settings, security fixes, package-upgrade policy, hardening execution, and reboot execution are rejected or reported as separate needs rather than smuggled into the Ubuntu Pro service provider.

## 11. Recovery and explicit detachment slice

- [x] 11.1 For OS-UPM-022 through OS-UPM-024 and OS-UPM-048/057, add fault-injected provider tests for post-attach entitlement failure, unexpected native side effects, a later service failure, reverse dependency restoration, post-rollback Check, cataloged no-automatic-rollback operations, residual-artifact reporting, and rollback failure; record red per case.
- [x] 11.2 Implement centralized non-secret rollback snapshots and catalog-driven recovery for attachment and services, including detaching only an attachment created by the failed Apply; make focused recovery tests green without claiming filesystem transactionality.
- [x] 11.3 For OS-UPM-025 through OS-UPM-027, add focused tests for authorized `u.pro.detach.v1`, already-detached idempotence, absent-resource no-op semantics, partial detach failure, reboot reporting, and no automatic rollback; record red then implement the minimum detach lifecycle without ordinary-command fallback.
- [x] 11.4 Add persistence/restart tests proving rollback records never contain token or contract material; reconstructed providers can complete only applicable recovery; and terminal cleanup removes rollback payloads according to the execution contract.

## 12. Real platform qualification

- [x] 12.1 Build isolated negative VM fixtures for Pop!_OS and another Ubuntu-derived identity, plus controlled conflicting-identity and interim-Ubuntu cases; prove no capability advertisement, secret resolution, API invocation, or mutation before marking OS-UPM-004/005/008/009 and OS-LPC-011/012 verified.
- [x] 12.2 Build the serialized, credential-free Ubuntu Pro VM harness with protected synthetic-token injection, deterministic independently specified API responses, the exact public provider seam and process request contract, bounded logs, canary scanning, recovery/cleanup checks, VM destruction verification, and failure-artifact retention rules. Record explicitly that no live Canonical subscription is exercised.
- [x] 12.3 On pinned Ubuntu 20.04 LTS amd64, qualify the base attachment row and then each applicable service/mode/variant/disable row independently through mock-API compliant/drifted/Apply/second-Check/recovery evidence; promote no untested sibling tuple and make no live subscription or entitled native-effect claim.
- [x] 12.4 Repeat tuple-by-tuple qualification on pinned Ubuntu 22.04 LTS amd64.
- [x] 12.5 Repeat tuple-by-tuple qualification on pinned Ubuntu 24.04 LTS amd64.
- [x] 12.6 Repeat tuple-by-tuple qualification on pinned Ubuntu 26.04 LTS amd64, proving unrelated 24.04-only capabilities and every unproven Ubuntu Pro tuple remain absent.
- [x] 12.7 Run deterministic behavior-specific high-risk fixtures for FIPS/FIPS Updates, real-time-kernel variants, Livepatch conflicts, purge, Anbox Cloud, ROS, and USG on every claimed applicable row; verify provider boot/recovery signaling without claiming live entitled native effects.
- [x] 12.8 Run invalid/expired synthetic token, unentitled service, missing endpoint, native lock, network loss, cancellation, timeout, malformed/oversized envelope, graph drift, unexpected side effect, partial failure, rollback failure, and all secret-canary scenarios on applicable real VM rows through deterministic API doubles; leave incomplete rows unadvertised.

## 13. Public workflow, documentation, and release gates

- [x] 13.1 Add declarative Godog scenarios using only public authoring, authenticated Sync/apply, and fleet-state vocabulary to cover synthetic token upload/activation, attachment, ordinary service convergence, dependency/conflict planning, idempotent resync, specialized service capability blocking, reboot signal, and explicit detachment.
- [x] 13.2 Document the complete stable catalog and typed options, bootstrap-only token semantics, partial ownership, API-first boundary, observable-mode limitation, destructive transitions, exact per-tuple 20.04-through-26.04 amd64 support table, derivative rejection, redacted reporting, rollback classes, mock-qualification limitation, and every client-setting/event non-claim.
- [ ] 13.3 Update every retained OS-UPM and OS-LPC traceability record with its actual public test, environment selector, and passing disposition; run traceability and provider-matrix audits and keep unproven IDs/rows planned or unadvertised.
- [x] 13.4 Run focused mutation testing over identity gating, API fallback prevention, envelope parsing, secret authorization/redaction, attachment branching, service graph ownership, side-effect verification, and rollback; kill critical mutants or record only reviewed expiring exceptions permitted by test policy.
- [ ] 13.5 Run the narrow focused suites, configuration acceptance, authenticated mock integration/e2e checks, every selected credential-free Ubuntu Pro VM selector, `make gosec`, and finally `make test`; record exact commands and results before handoff.
