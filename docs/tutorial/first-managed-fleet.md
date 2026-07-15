# Write your first managed fleet

This tutorial starts with an empty directory and builds a small, reviewable
configuration repository for Debian, Ubuntu, and Arch endpoints. It uses
canonical schema 1 throughout.

By the end you will understand the three layers that matter during authoring:

1. a fleet `manifest` selects source files;
2. `module` files declare typed desired-state resources;
3. the server composes and releases an artifact that agents check and apply.

You need an installed `remotr` CLI or a source checkout from which you can run
`go run -mod=vendor ./cmd/remotr`.

## 1. Scaffold the repository

```bash
remotr init --fleet workstations ./remotr-config
cd ./remotr-config
git init
```

The important files are:

```text
modules/base-packages.yaml
fleets/workstations/manifest.yaml
```

`remotr.yaml` is operator metadata. It is not desired state and is not sent to
agents.

## 2. Write a portable base module

Replace `modules/base-packages.yaml` with:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: workstation-base
    description: Small baseline shared by supported workstation distributions
    targetDistros:
      - Debian
      - Ubuntu
      - Arch
    resources:
      - kind: package
        name: curl
        lifecycle: present

      - kind: package
        name: git
        lifecycle: present

      - kind: file
        name: managed-motd
        path: /etc/motd
        content: |
          This endpoint is managed by Remotr.
```

The package provider is intentionally omitted. Normalized endpoint facts
select APT on Debian/Ubuntu and Pacman on Arch.

Every resource has a stable address of
`<configuration-name>/<resource-name>`, for example
`workstation-base/managed-motd`.

## 3. Select the module from the fleet manifest

Set `fleets/workstations/manifest.yaml` to:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
```

Paths are repository-relative. A manifest can select individual files or a
directory; directory discovery is recursive and includes files of the expected
kind only.

## 4. Validate before reviewing

```bash
remotr config validate .
remotr config discover --fleet workstations .
remotr config render --fleet workstations .
```

Use the commands for different questions:

- `validate` answers “can every source be parsed, resolved, and composed?”;
- `discover` answers “which files, kinds, and capabilities did this fleet
  select?”;
- `render` answers “what exact desired-state artifact will the agent receive?”

Do not redirect rendered output into `desired.yaml` inside the repository.
Rendered artifacts are server-owned release products, not authoring inputs.

## 5. Add an explicit dependency

Suppose a service must not be managed until its configuration file succeeds.
Add the following resources to the same `resources` list:

```yaml
      - kind: file
        name: telemetry-config
        path: /etc/telemetry/config.ini
        content: |
          endpoint=https://telemetry.example.internal

      - kind: service
        name: telemetry-running
        dependsOn:
          - workstation-base/telemetry-config
        provider: systemd
        scope: system
        service: telemetry.service
        enabled: true
        active: true
```

Dependencies always use complete resource addresses, even inside the same
configuration. Missing addresses and dependency cycles fail validation.

Run the three review commands again after every meaningful edit.

## 6. Split distro-specific state

Do not duplicate a whole fleet just because one package name differs. Add a
second configuration slice to the module:

```yaml
  - name: debian-utilities
    targetDistros: [Debian, Ubuntu]
    resources:
      - kind: package
        name: apt-transport-https
        lifecycle: present

  - name: arch-utilities
    targetDistros: [Arch]
    resources:
      - kind: package
        name: reflector
        lifecycle: present
```

The full artifact is sent to the endpoint; the agent filters configuration
slices using normalized distribution and architecture facts.

## 7. Review the first release

```bash
git add .
git diff --cached
remotr config validate .
git commit -m "Add workstation baseline"
```

In a connected deployment, push the commit to the branch watched by the
server, then trigger or wait for Git sync:

```bash
git push origin main
remotr git sync
```

Composition must succeed before the server advances its release ref. A failed
composition leaves the prior release active.

## 8. Observe without immediately enforcing

For the first endpoint cohort, register the fleet with remediation policy
`report`. When creating the repository and registry together, do this at the
initial scaffold with `remotr init --register-server --policy report`. This
tutorial used the repository-only path, so the database deployment can perform
the equivalent idempotent registration:

```sql
INSERT INTO fleet_settings (fleet, remediation_policy)
VALUES ('workstations', 'report')
ON CONFLICT (fleet) DO UPDATE
SET remediation_policy = EXCLUDED.remediation_policy,
    updated_at = now();
```

Agents will report drift but will not apply it. Inspect results:

```bash
remotr fleet state report workstations
remotr fleet state report workstations --verbose
remotr endpoint state report <endpoint-id> --json
```

After reviewing the report, change the fleet's server-side remediation policy
to `auto` through your database deployment procedure:

```sql
UPDATE fleet_settings
SET remediation_policy = 'auto', updated_at = now()
WHERE fleet = 'workstations';
```

The current operator CLI does not expose a fleet policy setter. The policy
belongs to the server registry; editing `remotr.yaml` alone does not change
enforcement. Confirm the intended policy through deployment evidence before
allowing the next agent sync.

## 9. Add one endpoint override

Endpoint overrides replace the fleet artifact. To retain the fleet state,
extend its manifest explicitly:

```text
endpoints/<endpoint-id>/manifest.yaml
```

```yaml
kind: manifest
extends: fleets/workstations/manifest.yaml
modules:
  - modules/design-tools.yaml
```

`modules/design-tools.yaml`:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: design-tools
    resources:
      - kind: package
        name: inkscape
        lifecycle: present
```

Validate and render the endpoint-specific result:

```bash
remotr config validate .
remotr config render --endpoint <endpoint-id> .
```

Never create a partial endpoint manifest without `extends` unless replacing
the entire fleet artifact is intentional.

## Next steps

- [Configuration repository workflow](../guides/configuration-repository.md)
- [Repository file kinds](../reference/repository-kinds.md)
- [Manifest format](../reference/manifest-format.md)
- [All resource kinds](../reference/resource-kinds.md)
- [Applications format](../reference/applications-format.md)
- [Scheduled jobs](../reference/crons-format.md)
- [Change control](../guides/change-control.md)
