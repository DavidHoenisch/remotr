# Use Remotr Desktop

Remotr Desktop is an additive native Linux administration workspace. It talks
to the same authenticated Admin API as the `remotr` Admin CLI and reuses the
same Operator credentials. It does not host a browser service or replace the
CLI for automation, headless work, or recovery.

The currently evidenced package is an unsigned Linux/amd64 DEB development
snapshot. Use it only through an approved development channel together with its
checked manifest. See the [support reference](../reference/remotr-desktop.md)
before treating an artifact as available.

## Reuse the default Operator profile

The standard Operator configuration defaults to
`~/.config/remotr/config.yaml`. If no named desktop profiles have been saved,
Remotr Desktop resolves that file as a profile named **Default**. The normal
precedence remains explicit flags, environment, configuration file, then
credential defaults.

A profile contains references only:

- a name;
- an absolute HTTPS Remotr server URL;
- an **absolute Operator state directory**;
- an optional absolute `ca.crt` path; and
- an optional default Fleet.

The default credential directory is `~/.config/remotr/` unless
`REMOTR_OPERATOR_STATE_DIR` or the configuration selects another absolute
path. A complete credential layout contains `operator.crt`, `operator.key`,
`ca.crt`, and `state.json` with owner-only permissions.

Remotr Desktop stores named profile references in
`~/.config/remotr/desktop-profiles.json` with mode `0600`. It **does not copy credential**
certificates or private keys into that file. Selecting a profile
constructs a new mTLS client and verifies the Operator identity with the server;
the presence of files alone does not mean the profile is connected.

## Add or switch a profile

1. Open the setup and maintenance view.
2. Enter a unique profile name and the server's absolute HTTPS URL.
3. Select the existing absolute Operator state directory.
4. Select an absolute CA path when the credential directory's `ca.crt` is not
   the desired trust reference.
5. Save and connect.

Switching profiles cancels obsolete requests and clears the previous Operator
identity, cached snapshots, selections, overlays, and transient action results
before loading the new server.

## Bootstrap the first Operator

Use desktop bootstrap only when the server has no registered Operator and the
selected profile points to an empty state directory.

1. Obtain the **one-time bootstrap token** from the protected server channel.
2. Select the server URL, CA certificate, and a new empty Operator state
   directory.
3. Enter the token in the focused bootstrap form and submit it once.
4. Wait for the desktop to save the issued credential and verify the returned
   Operator identity.

The desktop clears the token input and **never stores the token** in profile
settings, browser storage, logs, or ordinary result state. The server
invalidates a successful token. If credential persistence or identity
verification fails, the desktop removes partial credential fragments; obtain a
new token rather than reusing the previous value.

Do not bootstrap into a directory that already contains credential state. Use
the existing profile or choose a new empty directory.

## Keep desired state Git-bound

Remotr Desktop preserves the same deployment boundary as the Admin CLI:

- Server Git sync asks the server to fetch and compose its configured
  Configuration repository.
- Local configuration init, validate, discover, render, package, and Hub import
  workflows operate only inside an explicitly selected working tree.
- The desktop **does not stage, commit, push, merge, or apply** generated
  Configuration content.
- No desktop control writes uncommitted desired state directly to the server.

Review local changes, commit them through your normal Git process, and then use
the desktop Git sync action. If the desktop is unavailable, use `remotr git
sync` with the same Operator profile.

## CLI fallback and recovery

The Admin CLI remains supported and uses the same configuration and credential
directory. Put global flags before the subcommand when supplying them
explicitly:

```bash
remotr config show
remotr doctor

remotr \
  --server-url https://remotr.example:8443 \
  --state-dir ~/.config/remotr \
  --ca ~/.config/remotr/ca.crt \
  endpoint list

remotr \
  --server-url https://remotr.example:8443 \
  --state-dir ~/.config/remotr \
  --ca ~/.config/remotr/ca.crt \
  git sync
```

Use the CLI for automation, scripting, headless servers, credential recovery,
and any workflow whose current parity entry remains planned. For example,
`remotr endpoint list` provides inventory when the native window is unavailable. Consult the
[Operator CLI reference](../reference/cli.md) and the machine-readable parity
inventory linked from the [support reference](../reference/remotr-desktop.md).

If named desktop profile settings are damaged, close the app and move
`~/.config/remotr/desktop-profiles.json` aside. On the next launch, the desktop
can resolve the standard `~/.config/remotr/config.yaml` again. This does not
delete or revoke `operator.crt`, `operator.key`, `ca.crt`, or `state.json`; test
the same credential with `remotr doctor` before changing it.

## Troubleshooting

| Symptom | Resolution |
| --- | --- |
| No Default profile appears | Run `remotr config show`, confirm `~/.config/remotr/config.yaml` resolves an HTTPS server and absolute state directory, then correct it with the CLI configuration workflow. |
| `Operator credentials missing` | Confirm all four credential files exist in the selected state directory. Bootstrap into a new empty directory only when this is the server's first Operator. |
| TLS or server trust fails | Verify the absolute CA path and that the server hostname matches its certificate. Do not bypass verification. |
| The profile has files but will not connect | Run `remotr doctor` with the same references. Connection requires a successful authenticated identity check, not file presence alone. |
| An action is forbidden | Keep the session connected and ask an administrator to review the Operator's server-side RBAC roles. Hidden or disabled controls are not an authorization boundary. |
| Data is stale after refresh failure | Read the displayed failure time and reason. The desktop keeps the last successful snapshot for context but does not present it as current. |
| Git sync fails or the Release ref does not advance | Run Configuration validation, inspect server Git/composition logs, and use `remotr git sync` for the same authenticated request. Composition failure keeps the prior release active. |
| The native app will not start | Use the Admin CLI immediately, then follow the contributor guide's GTK/WebKit and native-smoke troubleshooting. |

For server, agent, and CLI symptoms, see the general [Troubleshooting
guide](troubleshooting.md).
