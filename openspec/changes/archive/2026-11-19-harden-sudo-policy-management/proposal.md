## Why

Remotr's `sudo` resource currently provides a solid minimal fragment baseline, but three real operational gaps remain for production Linux fleets:

1. **Preflight does not prevent sudo lockouts or catch host prerequisites early**: `Preflight()` only checks `user.Lookup` on `recoveryPrincipals`, so an unprivileged local account passes preflight, revoking the recovery user's own sudo fragment is not blocked, and missing `visudo` or broken `/etc/sudoers` include directives are only discovered during `Apply()`.
2. **The structured schema is too narrow for real hardening baselines**: each fragment renders a single rule line with no typed `Defaults` directives (`use_pty`, `timestamp_timeout`, `logfile`, `env_keep`, `secure_path`, etc.), no `user:group` `runAs` targets (`ALL:ALL`, `root:wheel`), no multi-rule fragments, and no structured command-argument restrictions (where bare sudoers paths silently permit arbitrary arguments unless `""` is specified).
3. **No precedence control or visibility into shadowing and unmanaged drop-ins**: sudoers evaluates `/etc/sudoers.d/*` lexically with last-match-wins semantics, yet fragments have no explicit priority ordering, no collision detection on rendered filenames, and no way to surface later-precedence shadowing or unmanaged `/etc/sudoers.d/` drop-ins in compliance checks.

## What Changes

- **Strengthen sudo preflight and recovery-access verification**:
  - Run prerequisite checks (`#includedir`/`@includedir`, regular-file fragment tree) and full staged `visudo -cf` validation during `Preflight()` as well as before activation in `Apply()`, redacting validator output so policy text is never leaked.
  - Verify that declared `recoveryPrincipals` not only resolve locally, but that at least one recovery principal retains an effective sudo grant outside the fragment being modified or removed (using a safe, injectable checker backed by `sudo -l -U <principal>` / staged-tree evaluation), blocking changes that would remove the last verified administrative sudo path.
- **Expand the backward-compatible `kind: sudo` structured model (`schemaVersion: 1`)**:
  - Add typed `defaults` entries (`scope`, `target`, `option`, `value`, `negate`) with a strict allowlist of hardening options and safe value validation.
  - Add multi-rule support via `rules[]` (mutually exclusive with top-level single-rule fields `subjects`/`runAs`/`commands`/`tags`, preserving full backward compatibility for existing single-rule resources).
  - Support `user:group` syntax in `runAs` targets (`ALL`, `ALL:ALL`, `user`, `%group`, `user:group`, `:group`).
  - Add structured `commandSpecs[]` (`path`, `args`, `noArgs`, `negate`) alongside `commands[]` so operators can explicitly enforce zero-argument (`""`) or argument-scoped commands.
- **Add explicit precedence control and unmanaged-fragment / shadowing visibility**:
  - Add optional `priority` (`0`–`99`) to render `<NN>-<name>` under `/etc/sudoers.d/`, with duplicate effective-filename collision detection in `remotr config validate`.
  - Extend `Check()` to inspect `/etc/sudoers.d/` safely (redacted, bounded subresults) to surface lexically later fragments that may shadow the managed fragment and, when opted in via `auditUnmanaged: true` (with optional `allowedUnmanagedFragments[]`), report unmanaged `/etc/sudoers.d/*` drop-ins as drift without modifying them.
  - Preserve full backward compatibility for all existing `kind: sudo` YAML definitions.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `local-identity-and-access`: strengthens sudo preflight to validate staged `visudo` readiness and effective recovery-principal sudo access before enforcement, extends structured sudo fragments with typed `Defaults`, multi-rule entries, `user:group` `runAs`, and argument-safe command specs, and adds explicit fragment priority plus redacted shadowing and unmanaged-fragment visibility.

## Impact

- `internal/models/sudo.go` and `internal/models/parser.go` (backward-compatible schema extensions, validation, and bounded fuzzing).
- `internal/configrepo/validate.go` (effective fragment filename collision validation across a configuration).
- `internal/applicators/sudo/applicator.go`, `applicator_test.go`, and `vm_safety_test.go` (preflight staging & recovery-grant check, rendering, priority path resolution, redacted shadowing/unmanaged checks, and native `visudo`/`sudo` VM safety tests).
- `internal/resourceregistry/field_policies.go` and `test/qualification/ubuntu-2404-applicators.yaml` (redaction classifications and accepted-fields alignment).
- `test/traceability.yaml`, `docs/reference/configuration-format.md`, and `docs/guides/local-administrator-access.md`.

## Non-goals

- Editing `/etc/sudoers` directly or bootstrapping missing `@includedir` lines.
- Deleting or modifying unmanaged `/etc/sudoers.d/*` files automatically (audit visibility is non-destructive).
- Adding `doas` or `polkit` policy applicators.
- Accepting raw/arbitrary unparsed sudoers text.
