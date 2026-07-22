# Manage encrypted secrets

Remotr-managed secrets are encrypted, versioned records in the server
registry. Configuration contains an explicit reference, never plaintext.

Two selectors exist:

- `remotr:<logical-name>@<version>` pins one exact version;
- `remotr:<logical-name>@active` follows a separately audited activation.

An omitted selector is invalid. There is no implicit “latest.”

## Before using server-managed secrets

The server requires Postgres and one external KEK keyring source:

```bash
REMOTR_SECRETS_ENABLED=true
REMOTR_SECRET_KEK_KEYRING=/etc/remotr/secrets/kek-keyring.json
```

or:

```bash
REMOTR_SECRETS_ENABLED=true
REMOTR_SECRET_KEK_KEYRING_B64=<strict-standard-base64-keyring-json>
```

Do not set both. The file source must be a root-owned regular file, not a
symlink, with mode `0600` or stricter. The current server enforces UID 0 for
that file; use the base64 deployment-secret source for a non-root server
instead of weakening permissions.

Keyring document:

```json
{
  "active": "kek-2026-07",
  "keys": {
    "kek-2026-07": "<base64-of-exactly-32-random-bytes>",
    "kek-2026-06": "<historical-key-still-needed-for-decryption>"
  }
}
```

Back up the complete keyring independently of Postgres. Keep old keys until no
encrypted record refers to them. Startup fails closed for missing, malformed,
permissive, symlinked, or incomplete configured key material.

## Choose scope

Every uploaded version has exactly one scope:

- `--global` allows the same logical version to satisfy otherwise authorized
  consumers in multiple fleets;
- `--fleet production` allows resolution only by endpoints in that fleet;
- `--endpoint endpoint-01` allows resolution only by that endpoint.

Global scope is opt-in and requires server-wide global-secret permission. It
only relaxes the Fleet-membership predicate: endpoint identity, active artifact
digest, exact resource address, declared purpose, version state, and rollout
authorization still apply. Scope is immutable for a logical name. Migrating to
a wider or narrower scope requires a new logical name and reviewed reference
updates; there is no scoped-to-global fallback.

## Upload an inactive version

From a protected file:

```bash
remotr secret upload repositories/private \
  --file /run/secrets/repository-token \
  --fleet production \
  --json
```

From stdin:

```bash
remotr secret upload services/api \
  --file - \
  --endpoint endpoint-01
```

The CLI never accepts secret bytes as a positional argument. A file must be a
regular file owned by the invoking UID, not a symlink, with mode `0600` or
stricter. Material must be non-empty and at most 1 MiB.

Upload creates a new inactive positive version. Save the returned safe
metadata: name, version, scope, and fingerprint. The command never prints the
plaintext.

Discover visible logical secrets without knowing an ID:

```bash
remotr secret list --json
```

Inspect one logical secret's versions:

```bash
remotr secret show repositories/private --json
```

Interactive `secret show` with no ID opens a picker. Non-interactive and JSON
invocations require the ID. Scripts that previously used
`secret list LOGICAL-NAME` must switch to `secret show LOGICAL-NAME`.

Remotr deliberately has no operator plaintext-read command. The Admin API's
plaintext-read path is denied.

## Author a reference

APT repository credential:

```yaml
- kind: aptRepository
  name: private-tools
  url: https://packages.example.internal/debian
  suites: [stable]
  components: [main]
  signingKey: private-tools
  credentialRef: remotr:repositories/private@active
```

Pinned user password hash:

```yaml
- kind: user
  name: recovery
  username: recovery
  present: true
  passwordHashRef: remotr:accounts/recovery-password@7
```

Network credential following activation:

```yaml
- kind: networkProfile
  name: office-wifi
  provider: network-manager
  selector: {type: wifi}
  profileName: office
  profileType: wifi
  ssid: company-office
  credentialRef: remotr:wifi/office@active
  audit: false
  enforce: true
  rollbackTimeout: 2m
```

Ubuntu Pro enrollment token following activation:

```yaml
- kind: ubuntuPro
  name: primary
  lifecycle: attached
  tokenRef: remotr:ubuntu-pro/production@active
  services:
    - name: esm-apps
      state: enabled
```

Supported discovery for `@active` activation planning currently covers APT
repository credentials, user password hashes, endpoint-schedule environment
secrets, network-profile credentials, certificate/key/chain references, and
trust anchors, plus Ubuntu Pro enrollment tokens. Each typed provider supplies
a purpose string so a value authorized for one use cannot be resolved as
another. Ubuntu Pro uses `ubuntu-pro-token` and preserves the resource's
computed sensitive, boot, or destructive risk in its Change request.

`local-file:/absolute/path` is a separate provider for material provisioned
independently on the endpoint. It does not upload or store bytes on the server.
Some older resource contracts still use the spelling `file:/absolute/path`;
follow the resource-specific reference.

## Activate a version

```bash
remotr secret activate repositories/private 2 --json
```

Activation increments the logical secret's activation generation and switches
the registry's active version only after complete fail-closed planning.
Exact-version references remain unchanged.

For every current `@active` use, the server records the fleet, resource
address, purpose, risk, provider, active release ref, artifact digest, frozen
endpoint list, and an effective hash that includes the activation generation.

If any use is high risk, activation creates a canonical Change request before
committing the new generation. A global activation creates the required
per-Fleet requests and one exact binding per resource/purpose. Incomplete
discovery, stale artifacts, ambiguous consumers, a missing request binding, or
persistence failure rejects the entire activation and retains the prior active
version. After a successful activation, endpoint resolution for a high-risk
binding remains unauthorized until its request has an active rollout
authorization:

```bash
remotr change show <changeRequestId> --json
remotr change authorize <changeRequestId> \
  --attempt-limit 1 \
  --max-concurrency 1 \
  --justification "CHG-4821: rotate repository credential"
```

Read [Change control](change-control.md) before operating this gate. Requests,
approvals, rollout authorizations, leases, and attempt counts are persisted in
Postgres and restored on an ordinary single-server restart. Back up the
`change_control_state` row with secret versions and KEKs, and verify the same
change ID after recovery rather than creating a replacement approval. A
successful `pause` or `revoke` is committed with its audit entry and restored
after restart.

Lower-risk uses can activate without a change request. Activation is still
audited and versioned.

## Rotate safely

1. Upload the new bytes as an inactive version.
2. Record and compare the safe fingerprint.
3. Validate the configuration repository and current `@active` uses.
4. Activate the exact new version.
5. Review and authorize every returned high-risk change request.
6. Verify endpoint state and the protected service, access, or connectivity
   path independently.
7. Keep the prior version available while rollback references or offline
   endpoints require it.
8. Revoke only after the new state is verified.

Use a pinned reference when Git review should control the exact version. Use
`@active` when secret rotation must occur without editing Git, accepting the
separate activation/change workflow.

## Revoke

```bash
remotr secret revoke repositories/private 1 \
  --confirm repositories/private@1 \
  --json
```

Revocation blocks all future resolution of that version, including exact
references. It does not erase plaintext already written to an endpoint or
held by another process. Metadata reports endpoint copies as
`rotation-or-removal-required`.

After revocation:

1. change desired state to the replacement or absence;
2. sync affected endpoints;
3. verify protected paths/services no longer use the value;
4. investigate offline endpoints separately.

Do not revoke an active recovery value before confirming an alternative
recovery path.

## Recovery and deletion constraints

Transactional providers may retain a server-side reference to a prior secret
version for bounded rollback. An active version cannot be deleted, and a
version retained for recovery cannot be deleted without an explicitly
authorized recovery-abandonment path. The public CLI currently exposes upload,
list, activate, and revoke—not arbitrary plaintext retrieval or force deletion.

Database recovery requires every KEK referenced by stored versions. Restore
Postgres and the matching keyring before starting the server. Remotr will not
generate a replacement key that could decrypt old records.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Server will not start | Exactly one KEK source; valid strict base64/JSON; file UID/mode; all historical keys present. |
| Upload rejects a file | Regular file, invoking-user ownership, no symlink, mode `0600` or stricter, size at most 1 MiB. |
| `@active` resolution denied | Scope, resource address, purpose, revocation, change request state, rollout validity, and server restart. |
| Exact version denied | Version exists, scope matches, not revoked, and provider purpose is correct. |
| Revocation did not erase endpoint data | Expected: revocation blocks future resolution; desired state must rotate/remove and verify the copy. |

See [Configuration secret references](../reference/configuration-format.md#secret-references),
[Ubuntu Pro management](ubuntu-pro-management.md),
[CLI reference](../reference/cli.md#secrets), and [HTTP API](../reference/http-api.md).
