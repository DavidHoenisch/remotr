## 1. Current State Report Evidence

- [x] 1.1 For OS-AEC-118 and OS-AEC-119, add focused Postgres State-report seam tests proving that newer compliant evidence supersedes an older apply failure while a same-Release newer failure remains current; record the intended red failures.
- [x] 1.2 Correlate persisted apply failures by Release and evidence time, retain historical endpoint detail, and make the focused State-report tests green.
- [x] 1.3 Add Admin API regression evidence for contradictory `in_compliance: true` / `status: apply_failed` output and the superseded failure's omission from current State-report output.

## 2. Exact Pop!_OS Applicability

- [x] 2.1 For OS-LPC-011 and OS-LPC-028, update the public fact-discovery test to require exact `PopOS` identity, Debian family lineage, and absence of inherited Ubuntu/Debian provider capabilities; record the intended red failure.
- [x] 2.2 Add canonical `PopOS` parsing, validation, capability facts, and local resolution while retaining Debian-family lineage; make the focused identity tests green.
- [x] 2.3 For OS-AEC-117, add configuration CLI/composition and authenticated Sync tests proving exact PopOS targeting excludes Ubuntu and Debian requirements without partial desired-state mutation; record red then implement target selection to green.
- [x] 2.4 Extend parser/target fuzz seeds and configuration documentation for canonical `PopOS`, exact `targetDistros` semantics, and fail-closed unqualified providers.

## 3. Independent Desktop Delivery Status

- [x] 3.1 For OS-DFV-036 and OS-DFV-037, add frontend tests for simultaneous compliant/capability-blocked presentation and for historical failures not changing current compliance; record the intended red failure.
- [x] 3.2 Add a compact independent Delivery status to endpoint inventory and detail using existing target, offered, active, blocked, and unmanaged fields; make focused frontend and desktop service tests green.

## 4. Verification

- [x] 4.1 Run focused negative/boundary tests, parser and target fuzz smoke checks, Go and desktop frontend suites, and `make test`.
- [x] 4.2 Validate the OpenSpec change and update traceability/documentation selectors for OS-AEC-117 through OS-AEC-119, OS-LPC-028, and OS-DFV-036 through OS-DFV-037.
