## Why

Some credentials, such as an Ubuntu Pro enrollment token, are intentionally shared across multiple fleets, but Remotr secrets can currently be authorized only within tighter fleet or endpoint boundaries. Operators need an explicit global scope so they can manage one shared secret lifecycle without duplicating the same credential per fleet, while retaining fleet- and endpoint-scoped secrets as the default choices.

## What Changes

- Add `global` as an explicit secret scope alongside the existing fleet and endpoint scopes.
- Keep tighter scopes available and make global selection opt-in rather than an automatic fallback or promotion.
- Allow authorized resources in multiple fleets to reference a global secret while preserving all existing endpoint identity, active artifact, resource address, and purpose authorization checks.
- Extend secret creation, listing, activation, rotation, deletion, audit, and safe metadata behavior to understand global scope.
- Make `secret list` enumerate authorized logical secrets, move per-secret version metadata to `secret show`, and open the standard interactive picker when `secret show` omits its secret ID.
- Make activation planning fail closed: every current `@active` use must receive a rollout binding, and every high-risk use must receive a Change request before the selected version can become active.
- Permit Ubuntu Pro resources in separate fleets to consume the same globally scoped token reference without exposing or copying token bytes.
- Reject ambiguous, unauthorized, or invalid cross-scope references without revealing whether inaccessible secrets exist.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `linux-security-and-secret-management`: Add opt-in global secret scope and define its lifecycle, authorization, audit, and non-disclosure behavior.
- `ubuntu-pro-management`: Allow an Ubuntu Pro attachment resource to consume either a tightly scoped token or an explicitly global token while retaining resource- and purpose-bound authorization.

## Impact

- Operator CLI and Admin API secret commands and representations gain an explicit global scope value plus a discoverable logical-secret collection/list surface.
- Secret persistence, uniqueness, lookup, authorization, activation/rotation rollout targeting, deletion safety, and audit paths must distinguish global, fleet, and endpoint scopes.
- Authenticated Sync/secret resolution must authorize global references across fleets without weakening endpoint, artifact, resource-address, or purpose binding.
- Configuration validation and Ubuntu Pro provider-contract evidence must cover global references, cross-fleet reuse, denial cases, redaction, persistence, and cleanup.
- Existing fleet- and endpoint-scoped secret behavior remains compatible; no dependency or wire-format change is expected beyond the additive scope value.
- Existing scripts using `secret list LOGICAL-NAME` must migrate to `secret show LOGICAL-NAME`; human invocation of `secret show` may select from authorized secrets interactively.
