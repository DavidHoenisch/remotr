## Context

Remotr-managed secrets currently carry either Fleet or Endpoint metadata authenticated with their ciphertext. Logical names and version histories are server-wide, references use the provider-neutral `remotr:<name>@<selector>` form, and resolution combines the stored scope with authenticated endpoint and active-artifact context. This makes operators duplicate intentionally shared credentials such as Ubuntu Pro enrollment tokens for every fleet.

The change crosses the operator CLI, Admin API, desktop parity surface, encrypted persistence, activation planning, authenticated resolution, configuration documentation, and provider evidence. It is security-sensitive because widening fleet eligibility must not weaken the remaining authorization dimensions or disclose secret existence.

## Goals / Non-Goals

**Goals:**

- Add an explicit, opt-in global scope to the existing Remotr secret lifecycle.
- Reuse one global version history across authorized resources in multiple fleets.
- Preserve endpoint identity, active artifact, exact resource address, declared purpose, version status, and rollout gates during resolution.
- Preserve the existing reference syntax and compatibility of fleet- and endpoint-scoped secrets.
- Make global lifecycle operations and cross-fleet impact visible through safe metadata, authorization, audit, and rollout planning.
- Make logical secrets discoverable before an operator knows a secret ID, while preserving safe metadata and non-interactive automation.
- Reject activation when complete active-use planning or required Change-request creation cannot be proven.

**Non-Goals:**

- Making global scope the default or automatically falling back from a missing fleet secret to a global secret.
- Adding plaintext retrieval, wildcard resource authorization, arbitrary operator sharing, or external vault providers.
- Promoting an existing logical secret between scopes in place.
- Changing Ubuntu Pro attachment behavior beyond allowing an authorized globally scoped token.

## Decisions

### Model global as a third explicit scope

The scope model will have a canonical discriminator (`global`, `fleet`, or `endpoint`) and an identifier required only for Fleet and Endpoint. The discriminator and identifier remain authenticated envelope metadata and safe API metadata. Validation rejects missing scope, unknown scope values, a global identifier, or a missing identifier for tighter scopes.

This is preferable to encoding global as empty Fleet/Endpoint fields, which makes omission indistinguishable from deliberate widening and complicates fail-closed validation.

### Keep scope immutable for a logical secret

A logical name has one scope for its complete version history. Uploading later versions must match it. Migrating scope requires creating a distinct logical name, updating reviewed configuration references, activating the replacement, and retiring the old secret.

This avoids mixed-scope active-version selection and makes activation, rollback retention, and audit semantics deterministic. In-place promotion was rejected because it could silently broaden every existing reference.

### Preserve reference syntax and resolve scope from stored metadata

References remain `remotr:<logical-name>@active` or an exact version. Since logical names are server-wide and scope is immutable, the selected stored version determines scope without adding scope syntax to every resource schema. There is no Fleet-first/global-fallback lookup: a name identifies exactly one logical secret.

Adding scope to reference strings was considered, but it would touch every consumer schema and duplicate authenticated registry metadata without resolving additional ambiguity under globally unique names.

### Treat global as widening only the fleet-membership check

Resolution will evaluate scope as one predicate within the existing authorization conjunction. Global makes that predicate true for any fleet, while authenticated endpoint identity, active artifact digest, exact resource address, declared purpose, selected version status, and rollout eligibility remain mandatory. Denials use the same bounded response regardless of whether the secret is absent, scoped elsewhere, revoked, or unauthorized.

This supplies cross-fleet reuse without turning global secrets into bearer values retrievable by arbitrary endpoints.

### Plan activation across the complete authorized use set

The server will discover global-secret consumers from active compiled artifacts across fleets through the same authenticated composition semantics used for scoped consumers. A global activation generation is single and monotonic for the logical secret; rollout planning groups uses by Fleet/resource while applying each resource's risk policy. Deletion considers rollback references from all fleets.

Bounded database queries and representative cross-fleet fixtures will prevent global activation from becoming an unbounded in-memory scan. The existing load and benchmark harnesses will measure activation-use discovery and resolution where this path is hot.

### Require elevated global-secret administration

Global create, activate, revoke, delete, and recovery-abandonment operations require an explicit server-wide secret permission rather than ordinary Fleet authority. Readable metadata remains authorization-filtered and contains no material. Every mutation records actor, scope, version, generation, and bounded affected-fleet counts; detailed inaccessible uses are not disclosed to unauthorized operators.

### Separate secret collection listing from per-secret inspection

`remotr secret list` will enumerate logical secrets visible to the operator without requiring an ID. Its stable table and structured output contain only the logical ID/name, explicit scope, active-version identity/status, bounded version count, and safe timestamps/fingerprint metadata that the caller is authorized to see. The backing Admin API is an authorization-filtered, bounded, paginated logical-secret collection endpoint rather than a scan assembled by the CLI.

`remotr secret show <secret-id>` will return the selected logical secret and its version metadata. When a human runs `secret show` without an ID on an interactive terminal, the CLI fetches the same authorized collection and opens the standard picker used by other resource-selection commands. Picker labels include scope and active status so similarly named entries remain distinguishable. Empty results, cancellation, and selection races return clear bounded outcomes. JSON or other non-interactive invocation without an ID fails with actionable usage guidance and never opens or waits for a picker.

Keeping `list LOGICAL-NAME` as an overloaded operation was rejected because it prevents discovery and conflicts with the collection semantics used by Fleet and Endpoint commands. Existing scripts migrate to `show LOGICAL-NAME`.

### Commit activation only after complete fail-closed planning

Activation is one atomic operation: discover the complete current `@active` consumer set, derive each effective identity and risk, create every required canonical Change request, validate one rollout binding for every use, and only then commit the active version and generation. A high-risk binding must carry a Change-request ID. Failure or uncertainty in consumer discovery, composition, endpoint evidence, planning, persistence, or binding validation rejects activation and leaves the previous active version unchanged.

Lower-risk uses may retain the documented audited activation path without a Change request, but they still require an explicit matching rollout binding. Resolution of `@active` fails closed when the active artifact's exact resource and purpose have no matching binding; high-risk resolution additionally requires an active authorized rollout. This closes the gap where a missing binding could be treated like an unrestricted use.

The execution-lease bootstrap must not require the endpoint to acknowledge the
artifact first. A high-risk Apply cannot complete—and therefore cannot produce
that acknowledgement—until the lease is delivered. Authenticated current-state
telemetry may establish the bootstrap identity from the exact current artifact
digest while `lastReleaseRef` is empty. A non-empty acknowledgement remains
strictly bound to the current Release ref, and stale digests remain ineligible.

## Risks / Trade-offs

- [A compromised global secret has a larger blast radius] → Keep global opt-in, require elevated authority, show safe affected-fleet impact, preserve risk-governed rollout, and document migration back to tighter scopes.
- [A resolution bug could bypass a non-scope authorization predicate] → Express authorization as an explicit conjunction and add negative tests and focused mutation evidence for endpoint, artifact, address, purpose, version, and rollout predicates.
- [Activation fan-out could amplify load across many fleets] → Use bounded indexed consumer discovery, deterministic planning, native benchmarks with allocations, and authenticated load-harness evidence.
- [Existing records do not contain a scope discriminator] → Migrate them deterministically from exactly one existing Fleet/Endpoint field and fail startup/migration on invalid or ambiguous rows.
- [Scope migration requires a new name] → Provide a documented create/update/activate/retire sequence; accept the operational step in exchange for preventing silent privilege widening.
- [Collection listing could disclose secret inventory] → Filter at the server by operator authorization, paginate and bound results, and expose only classified safe metadata.
- [Interactive selection can break automation] → Restrict the picker to an interactive terminal and require an explicit ID for structured/non-interactive output.
- [Incomplete consumer discovery could bypass change control] → Make discovery and binding validation mandatory and atomic; reject activation and `@active` resolution on missing or ambiguous evidence.

## Migration Plan

1. Add the scope discriminator and constraints to storage and API models. Backfill existing records as `fleet` or `endpoint`; reject rows with neither or both identifiers.
2. Deploy readers that understand all three scopes while writers still create only existing scopes, then enable global creation after migration verification.
3. Add the authorization-filtered logical-secret collection API, switch CLI collection semantics to `secret list`, and introduce `secret show [secret-id]` with interactive-only selection.
4. Add CLI, Admin API, and desktop validation/output for explicit global selection and elevated authorization.
5. Enable fail-closed cross-fleet consumer discovery, atomic activation planning, binding validation, and authenticated resolution after focused security, persistence, benchmark, load, and Ubuntu Pro evidence passes.
6. Document opt-in creation, `list` to `show` script migration, and the new-name scope migration workflow. Rollback disables new global mutations and resolution only if no active global references remain; database rollback is otherwise unsafe because global records cannot be represented by the old model.

## Open Questions

- What existing operator role/permission vocabulary should name the server-wide global-secret capability?
- Should the first release expose global-secret mutations in the desktop application simultaneously with CLI/Admin API support, or explicitly record temporary parity scope?
