## Context

Remotr manages sudo policy through the `kind: sudo` (`schemaVersion: 1`) resource defined in `internal/models/sudo.go` and enforced by `internal/applicators/sudo/applicator.go`. Today:

- `Preflight()` only calls `user.Lookup` on each declared `recoveryPrincipal`. Staged `visudo -cf` validation and `/etc/sudoers` `#includedir`/`@includedir` checks only happen inside `Apply()`, and no check verifies that a declared recovery principal actually holds an effective sudo grant that survives the candidate change.
- `SudoResource` models only a single rule line (`<subjects> ALL=(<runAs>) [<tags>: ]<commands>`), rejecting `user:group` `runAs` syntax, `Defaults` directives, multi-rule fragments, and structured argument restrictions.
- `State()` / `Check()` inspects only `/etc/sudoers.d/<name>` with no explicit precedence ordering (`priority`), no filename collision validation across a configuration, no shadowing awareness for lexically later fragments, and no opt-in audit of unmanaged drop-ins.
- Every field on `SudoResource` is subject to `resourceregistry` strict-schema field-policy linting (`OS-AEC-083`), safe-redaction reporting, and Ubuntu qualification accepted-field checks (`test/qualification/ubuntu-2404-applicators.yaml`).

## Goals / Non-Goals

**Goals:**

- Make `Preflight()` a truthful readiness and anti-lockout gate: verify local account existence, `/etc/sudoers` include-directory prerequisites, full staged `visudo -cf` syntax, and surviving effective sudo access for every declared `recoveryPrincipal`, with redacted error diagnostics.
- Extend `SudoResource` backward-compatibly with typed `defaults[]`, multi-rule `rules[]`, `user:group` `runAs` targets, structured `commandSpecs[]` (`path`, `args`, `noArgs`, `negate`), and optional `priority` (`0`–`99`).
- Provide redacted, bounded compliance visibility into lexically later shadowing fragments and opt-in unmanaged `/etc/sudoers.d/` drop-in auditing without mutating unmanaged files.
- Maintain full backward compatibility for existing `kind: sudo` manifests, provider contracts, change-control effects, and transactional rollback semantics.

**Non-Goals:**

- Mutating `/etc/sudoers` directly or inserting missing `#includedir`/`@includedir` lines.
- Deleting or rewriting unmanaged `/etc/sudoers.d/*` files automatically.
- Adding `doas` or `polkit` policy applicators.
- Accepting arbitrary unparsed sudoers text or shell expressions.

## Decisions

### 1. Stage-and-verify in `Preflight()` with injectable recovery-grant verification

- **Decision:** Move staged candidate construction and `visudo -cf` validation into a shared helper called by both `Preflight(ctx)` and `Apply(ctx)`. Rewrite *all* `#includedir <SudoersDir>` and `@includedir <SudoersDir>` lines in the staged `/etc/sudoers` copy (rather than only the first occurrence). After `visudo` succeeds, verify every `recoveryPrincipal`:
  1. Resolve the local account (`LookupRecovery`, defaulting to `user.Lookup`).
  2. Verify effective recovery access via `VerifyRecoveryAccess(ctx, principal, stagedSudoers, stagedDir)` when injected, or the default verifier which:
     - Scans the staged effective sudoers tree (`stagedSudoers` + active regular files in `stagedDir`) to confirm that `principal` (or any of its local groups `%<group>`, or `root`) still has at least one matching grant rule after applying the candidate change (catching `lifecycle: absent` or narrowing of the recovery user's own fragment).
     - Unless `principal == "root"`, runs `sudo -l -U <principal>` via `a.Runner` to confirm the live host recognizes an active sudo privilege for that account (catching unprivileged accounts like `nobody`), returning a redacted error without copying `sudo` output.
- **Alternative considered:** Relying solely on `sudo -l -U` without scanning the staged tree. Rejected because `sudo -l -U` evaluates the *live* `/etc/sudoers` and cannot detect when the pending candidate change itself deletes or revokes the recovery principal's last grant.
- **Alternative considered:** Parsing full sudoers grammar in Go. Rejected as fragile and redundant with `visudo`; `visudo -cf` remains the authoritative syntax validator, while a focused, conservative token matcher on non-comment rule lines checks staged recovery principal/group presence.

### 2. Backward-compatible schema extensions on `SudoResource`

- **Decision:** Keep existing top-level single-rule fields (`subjects`, `runAs`, `commands`, `tags`) intact and add optional fields:
  - `priority *int` (`0`–`99`): formats fragment filename as `%02d-%s` via `FragmentFilename()`. When `nil`, filename remains `<name>`. `configrepo.ValidateState` rejects duplicate effective `FragmentFilename()` values within a configuration.
  - `defaults []SudoDefault`: typed `Defaults` directives (`scope`: `""|global|user|runAs|command`, `target`, `option`, `value`, `values`, `negate`) gated by a strict allowlist of hardening options:
    - Booleans (support `negate: true`, forbid `value`/`values`): `use_pty`, `env_reset`, `requiretty`, `insults`, `mail_badpass`, `log_input`, `log_output`, `pwfeedback`, `rootpw`, `targetpw`, `stay_setuid`, `noexec`, `fqdn`.
    - Non-negative integers: `timestamp_timeout`, `passwd_tries`, `passwd_timeout`.
    - Validated paths/enums: `logfile` (abs path), `iolog_dir` (abs path), `secure_path` (colon-separated abs paths), `umask` (4-digit octal `0000`–`0777`), `lecture` (`always|once|never`), `timestamp_type` (`global|ppid|tty|kernel`).
    - Env-var lists (`values[]` matching `[A-Za-z_][A-Za-z0-9_]*`, rendered as `option += "V1 V2"`): `env_keep`, `env_delete`.
  - `rules []SudoRule`: multi-rule list (`subjects`, `runAs`, `commands`, `commandSpecs`, `tags`). Mutually exclusive with top-level `subjects`, `runAs`, `commands`, `commandSpecs`, `tags`. A `present` fragment must declare at least one `default` or one rule (so pure hardening-defaults fragments are valid).
  - `commandSpecs []SudoCommandSpec`: structured command entries (`path`, `args[]`, `noArgs`, `negate`), available on both top-level single-rule resources and inside `rules[]`. `noArgs: true` renders `<path> ""` (explicit zero-argument restriction); `negate: true` prefixes `!`; `args` and `noArgs` are mutually exclusive; unsafe delimiters (`,`, `:`, `=`, `\`, quotes, whitespace in `path`, control chars) are rejected.
  - `runAs` targets extended to accept `user:group` syntax (`ALL`, `ALL:ALL`, `ALL:<group>`, `<user>:ALL`, `<user>:<group>`, `:<group>`, `:ALL`, `<user>`, `%<group>`).
- **Alternative considered:** Introducing a new `kind: sudoDefaults` resource. Rejected because sudoers hardening (`Defaults`) and role rules routinely share a fragment and atomic `visudo` validation boundary; keeping one `kind: sudo` resource preserves a single lock domain (`sudo-policy`), change-control effect, and capability identity.

### 3. Precedence awareness and non-destructive unmanaged-fragment auditing

- **Decision:** Add optional `auditUnmanaged bool` and `allowedUnmanagedFragments []string` (valid only when `auditUnmanaged: true`, each entry a safe active fragment filename).
  - Active drop-ins in `/etc/sudoers.d/` follow sudoers' own include rule: regular files whose names do not contain `.` and do not end in `~`.
  - When `lifecycle: present` and the managed fragment matches desired content and `root:root 0440` metadata:
    - **Shadowing visibility**: Scan active files in `SudoersDir` lexically *after* `FragmentFilename()`. If any later file contains a rule matching one of the resource's managed subjects, record bounded, redacted `Subresults` (`Status: Drifted`, `ReasonCode: "sudo_fragment_shadowed"`, `Target: <filename>`) and note the count in `ObservedSummary`, while keeping top-level `Status: Compliant` so the agent does not spin retrying an already-converged `Apply()`.
    - **Unmanaged-fragment auditing (`auditUnmanaged: true`)**: If any active file in `SudoersDir` is neither `FragmentFilename()` nor listed in `allowedUnmanagedFragments`, report top-level `Status: Drifted` with `ReasonCode: "sudo_unmanaged_fragments"` and bounded `Subresults` (capped at `executor.MaxCheckSubresults`, filenames only, never file contents). `Apply()` remains strictly scoped to its own fragment and never deletes unmanaged files.
- **Alternative considered:** Making shadowing mark top-level `Status: Drifted`. Rejected because `Apply()` cannot safely modify unrelated fragments and would loop indefinitely (`Drifted` -> `Apply` -> `NoChange`).

### 4. Strict field-policy classification and redaction

- **Decision:** Classify every new leaf YAML path in `internal/resourceregistry/field_policies.go` and update `test/qualification/ubuntu-2404-applicators.yaml`:
  - Public values: `auditUnmanaged`, `priority`, `defaults[].scope`, `defaults[].option`, `defaults[].negate`, `commandSpecs[].noArgs`, `commandSpecs[].negate`, `rules[].tags[]`, `rules[].commandSpecs[].noArgs`, `rules[].commandSpecs[].negate`.
  - Counts: `allowedUnmanagedFragments[]`, `commandSpecs[]`, `commandSpecs[].args[]`, `defaults[]`, `defaults[].values[]`, `rules[]`, `rules[].subjects[]`, `rules[].runAs[]`, `rules[].commands[]`, `rules[].commandSpecs[]`, `rules[].commandSpecs[].args[]`.
  - Sensitive-omit (never leaked in reports): `commandSpecs[].path`, `rules[].commandSpecs[].path`, `defaults[].target`, `defaults[].value`.

## Risks / Trade-offs

- **[Risk] False-negative staged recovery check on hosts using `User_Alias` in `/etc/sudoers`** → Mitigation: match direct username, all resolved local groups (`%group`), and `User_Alias` definitions in the staged tree, plus allow explicit `root` recovery; keep `VerifyRecoveryAccess` cleanly testable and fail-closed on ambiguous configurations.
- **[Risk] Existing unit or acceptance tests that inject `LookupRecovery` and `ValidateEffective` without a real `sudo` binary** → Mitigation: update affected unit/acceptance tests to supply explicit recovery fragments or `VerifyRecoveryAccess` / mock runner stubs so tests stay deterministic with zero host-sudo dependency.
- **[Risk] Schema additions drifting from `field_policies.go` or `ubuntu-2404-applicators.yaml`** → Mitigation: run `TestDefaultFieldPoliciesCoverStrictSchemas` and `internal/ubuntuqualification` tests as part of every TDD slice.

## Migration Plan

- All changes are strictly additive on `schemaVersion: 1`; existing `kind: sudo` resources continue to parse, validate, render identically, and pass existing tests without modification.
- Existing fragments without `priority` keep their `<name>` path; operators adopt `priority`, `defaults`, `rules`, `commandSpecs`, or `auditUnmanaged` incrementally.
- Rollback remains transactional per fragment via `rollbackstore` / `filetx`.
