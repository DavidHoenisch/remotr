# Secret management

Remotr-managed secrets are encrypted, versioned records. Configuration stores only an explicit reference: `remotr:<logical-name>@<version>` pins an exact version, while `remotr:<logical-name>@active` follows audited activation. An omitted selector is invalid; there is no implicit latest version.

## Upload an inactive version

Supply bytes through stdin or an owner-only regular file. The CLI does not accept secret material as an argument and prints only safe metadata.

```bash
remotr secret upload repositories/private --file /run/secrets/repository-token --fleet production
remotr secret upload services/api --file - --endpoint endpoint-1 --json
```

The input file must be owned by the invoking user, must not be a symlink, and must have mode `0600` or stricter. Upload creates a new inactive version, so resources following `@active` do not change yet.

List safe metadata without reading plaintext:

```bash
remotr secret list repositories/private
```

Remotr deliberately provides no operator plaintext-read command or API.

## Activate and revoke

Activate an exact version separately:

```bash
remotr secret activate repositories/private 2
```

Activation is audited. Remotr discovers resources following `@active`; high-risk references create hash-bound Change requests and remain unavailable until their rollout is authorized. Exact-version references remain pinned.

Revocation blocks future resolution:

```bash
remotr secret revoke repositories/private 1 --confirm repositories/private@1
```

Revocation does not erase copies already installed on endpoints. Metadata reports those copies as `rotation-or-removal-required`; desired state must rotate or remove them and Check must verify the result.
