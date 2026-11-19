## 1. Schema, Validation, and Field Policies

- [x] 1.1 Add `Priority`, `Defaults` (`SudoDefault`), `Rules` (`SudoRule`), `CommandSpecs` (`SudoCommandSpec`), `AuditUnmanaged`, and `AllowedUnmanagedFragments` fields to `models.SudoResource` with `FragmentFilename()`, `user:group` `runAs` validation, strict option/delimiter validation, and focused unit + fuzz tests (`OS-LIA-014`, `OS-LIA-015`).
- [x] 1.2 Add effective fragment filename collision validation to `configrepo.validateSudo` with unit tests (`OS-LIA-015`).
- [x] 1.3 Classify all new `SudoResource` schema paths in `internal/resourceregistry/field_policies.go` and align `accepted_fields` in `test/qualification/ubuntu-2404-applicators.yaml`, verifying `TestDefaultFieldPoliciesCoverStrictSchemas` and qualification manifest tests pass.

## 2. Applicator Rendering, Preflight Hardening, and Recovery Verification

- [x] 2.1 Implement canonical rendering for `Defaults`, multi-rule `rules[]`, `user:group` `runAs`, and `commandSpecs[]` (`noArgs` `""`, explicit `args`, `!` `negate`), plus `priority`-prefixed fragment path resolution in `internal/applicators/sudo/applicator.go`, with focused unit tests (`OS-LIA-014`, `OS-LIA-015`).
- [x] 2.2 Move staged `visudo -cf` validation (rewriting all `#includedir`/`@includedir` directives) into `Preflight()` alongside `Apply()` with canary redaction tests (`OS-LIA-010`, `OS-LIA-017`).
- [x] 2.3 Implement effective recovery-principal verification in `Preflight()` (staged-tree grant preservation + live `sudo -l -U <principal>` check via injectable hook/runner) so unprivileged recovery users and self-revoking fragments are rejected before mutation (`OS-LIA-011`, `OS-LIA-016`).

## 3. Precedence Shadowing and Unmanaged Drop-In Visibility

- [x] 3.1 Implement lexically-later shadowing detection in `Check()` returning redacted, bounded `sudo_fragment_shadowed` subresults without breaking `Compliant` convergence (`OS-LIA-018`).
- [x] 3.2 Implement opt-in `auditUnmanaged` drop-in detection in `Check()` returning `Drifted` (`sudo_unmanaged_fragments`) with bounded, filename-only subresults while ensuring `Apply()` never mutates unmanaged files (`OS-LIA-019`).

## 4. Acceptance, Qualification, Documentation, and Verification

- [x] 4.1 Update `test/acceptance/local_administrator_test.go`, `internal/applicators/sudo/vm_safety_test.go`, and `test/traceability.yaml` for the new verification IDs (`OS-LIA-014` through `OS-LIA-019`).
- [x] 4.2 Update `docs/reference/configuration-format.md` and `docs/guides/local-administrator-access.md` with examples for `priority`, `defaults`, `rules`, `commandSpecs`, `user:group` `runAs`, recovery preflight, and `auditUnmanaged`.
- [x] 4.3 Run focused suites, fuzz regression checks, `openspec validate`, and full `make test` to verify all checks are green.
