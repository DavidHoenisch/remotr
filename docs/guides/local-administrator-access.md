# Manage a local administrator safely

Use the M2 access resources to create and later revoke a local administrator
without a generic command, an unmanaged `authorized_keys` edit, or an
unscoped sudoers change. This guide is for operators who already maintain a
Remotr configuration repository and need a recoverable access baseline.

## Before you apply the baseline

Choose an existing local `recovery` account that remains outside the access
policy you are changing. It must be a real local account on every target
endpoint. Keep an independent console, hypervisor, or other out-of-band path
available while validating an access change.

Use a real public key and calculate its SHA-256 fingerprint before committing
the configuration. Never put private keys, passwords, or password hashes in
the repository.

## Add the canonical M2 configuration

Create a schema-1 module such as `modules/local-admin.yaml`:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: local-admin
    resources:
      - kind: group
        name: administrators
        lifecycle: present
        group: administrators

      - kind: user
        name: developer
        username: developer
        present: true
        dependsOn: [local-admin/administrators]
        primaryGroup: administrators
        home: /home/developer
        createHome: true
        shell: /bin/bash
        comment: Managed local administrator

      - kind: authorizedKey
        name: developer-admin-key
        lifecycle: present
        ownership: authoritative
        enforce: true
        dependsOn: [local-admin/developer]
        user: developer
        recoveryPrincipals: [recovery]
        entries:
          - type: ssh-ed25519
            key: AAAAC3NzaC1lZDI1NTE5AAAAIPTCEW4tXxI1a3nVVLmEEu2WADFX6GeP0HeZg2N5DR9W
            fingerprint: SHA256:YX/1T3lbmFP3mL3tZEfnRA79p12FyzmdPJnh4P7TLd4
            comment: developer admin
            restrictions: [no-agent-forwarding]

      - kind: sudo
        name: developer-admin
        lifecycle: present
        ownership: fragment
        enforce: true
        dependsOn: [local-admin/developer]
        subjects: [developer]
        runAs: [ALL]
        commands: [/usr/bin/id]
        tags: [NOPASSWD]
        recoveryPrincipals: [recovery]
```

Reference the module from the target Fleet manifest, then validate and render
before pushing:

```bash
remotr config validate ./remotr-config
remotr config render --fleet engineering ./remotr-config
```

The rendered artifact must contain `authorizedKey` and `sudo` resources. Do
not add a `command` resource to edit passwd, SSH, or sudo policy as a
workaround.

## Provider matrix

| Resource | Endpoint provider | Required endpoint capability | Ownership boundary |
| --- | --- | --- | --- |
| `group` | Local account database (`getent`, `groupadd`, `groupmod`, `groupdel`) | Linux local accounts | One named group under the account-database lock |
| `user` | Local account database (`useradd`, `usermod`, `userdel`) | Linux local accounts | One named account under the account-database lock |
| `authorizedKey` | OpenSSH `authorized_keys` format | Selected account home | One Remotr marker inside that user’s `authorized_keys` file |
| `knownHost` | OpenSSH `known_hosts` format | User home or system known-host path | One named Remotr marker; optional explicit conflict replacement |
| `sudo` | `sudoers.d` and `visudo` | `/etc/sudoers` with the managed include directory and `visudo` | One named sudoers fragment |

The account-database commands are supported on the currently targeted
Debian/Ubuntu and Arch systems when their standard account-management tools are
available. SSH resources operate on OpenSSH-compatible file formats; sudo
requires a working `visudo` binary and a sudoers configuration that includes
the managed fragment directory. If a required local provider is unavailable,
do not replace it with a generic command—resolve the endpoint prerequisite or
keep the resource out of that Fleet.

## Revoke the administrator

First revoke the access grants, then remove the account in a later reviewed
change. Set the SSH and sudo resources to `lifecycle: absent`; this removes
only their named Remotr-owned blocks/fragments. Once the access removal is
confirmed, change the user resource to `present: false` and choose the
appropriate `removeHome` and `forceRemoval` policy.

Do not rely on omitting a resource: omission preserves existing state. A
separate named absence declaration is the deliberate revocation operation.

## Recover from a blocked access change

Remotr keeps the active sudo fragment in place when staged effective `visudo`
validation fails. It also refuses authoritative SSH or sudo enforcement when a
declared recovery principal cannot be resolved. These protections do not prove
that a human can log in, so keep the out-of-band path identified above.

If the endpoint reports a failed or blocked access resource:

1. Use the independent console or the unchanged `recovery` principal; do not
   keep retrying the same access artifact.
2. Inspect the rendered artifact and the resource-level diagnostic. Correct
   the structured key restriction, sudo subject/run-as/command/tag, or
   recovery-principal declaration in Git.
3. Re-run `remotr config validate` and `remotr config render` before releasing
   the correction.
4. If recovery itself is no longer viable, use your platform’s out-of-band
   recovery process. A Remotr break-glass or ordinary retry is not a substitute
   for an independent endpoint recovery channel.

## Compatibility and migration

The M2 `authorizedKey`, `knownHost`, and `sudo` kinds require
`schemaVersion: 1`; there is no lossless schema-0 representation for their
structured ownership and recovery semantics. Existing schema-0 package, file,
and systemd collections remain readable during the documented compatibility
window, but are not a migration path for M2 access resources.

When migrating an existing host, leave unrelated SSH and sudo lines in place.
Remotr owns only its markers and named fragments. Model new policy through the
typed resources, deploy it alongside the existing access path, verify it, and
only then retire legacy manual policy through an explicit reviewed change.
