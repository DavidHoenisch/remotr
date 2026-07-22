# Manage Ubuntu Pro attachment and services

Use an `ubuntuPro` resource to attach qualified Ubuntu endpoints to an Ubuntu
Pro subscription and manage only the services you explicitly declare. The
subscription token remains in Remotr's encrypted secret registry; Git contains
only its versioned reference.

This guide is for fleet operators managing Ubuntu 20.04, 22.04, 24.04, or
26.04 LTS on amd64. For the complete field, service, risk, API, and support
tables, see the [Ubuntu Pro resource reference](../reference/configuration-format.md#ubuntu-pro-resources).

## Before you begin

Confirm that:

- the endpoint reports exact Canonical Ubuntu identity, not an Ubuntu-derived
  distribution;
- the endpoint is amd64 and runs a qualified Ubuntu LTS release;
- Ubuntu Pro Client is installed and exposes the required versioned `pro api`
  endpoints;
- [server-managed secrets](secret-management.md#before-using-server-managed-secrets)
  are enabled;
- your operator can upload and activate secrets and review Change requests;
- your configuration repository is connected to the server; and
- the endpoint has reported a current capability document and preflight state.

Remotr does not install Ubuntu Pro Client. Install and verify it through your
image or another separately managed resource before delivering `ubuntuPro`.

## 1. Upload the token

Put the Canonical token in a protected regular file owned by your current user
and readable only by that user. Upload it for the fleet that will consume it:

```bash
remotr secret upload ubuntu-pro/production \
  --file /run/secrets/ubuntu-pro-token \
  --fleet production \
  --json
```

The upload creates an inactive version and returns safe metadata, including
its version and fingerprint. Remotr does not print the token. Record the
returned version for the activation step.

Use endpoint scope instead of fleet scope only when one subscription token is
intentionally restricted to one endpoint:

```bash
remotr secret upload ubuntu-pro/special-host \
  --file /run/secrets/ubuntu-pro-token \
  --endpoint <endpoint-id> \
  --json
```

## 2. Author a deployable resource

Add a module such as `modules/ubuntu-pro.yaml`:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: subscriptions
    targetDistros: [Ubuntu]
    targetArch: [X86]
    resources:
      - kind: ubuntuPro
        name: primary
        lifecycle: attached
        tokenRef: remotr:ubuntu-pro/production@active
        services:
          - name: esm-apps
            state: enabled
```

Reference the module from the fleet's `kind: manifest` file. Omitted services
remain outside Remotr ownership.

The currently advertised service workflow is:

- attachment on qualified Ubuntu 20.04, 22.04, 24.04, and 26.04 LTS amd64;
- enabled catalog services using the default `full` behavior (omit
  `enableMode`, as above, or write `enableMode: full`); and
- qualified `realtime-kernel` variants `intel-iotg` and `raspi`.

Although the schema recognizes `enableMode: access-only`, disabled service
state, `retain-packages`, and `purge`, those tuples are currently
unadvertised. A repository containing them can validate and render, but
capability-compatible delivery blocks it before token resolution or mutation.
Do not put those options in a production artifact until their exact capability
rows are advertised.

## 3. Validate the repository and inspect requirements

Run discovery first so the selected module paths and exact capability
requirements are visible:

```bash
remotr config discover --fleet production /path/to/remotr-config
remotr config validate /path/to/remotr-config
remotr config render --fleet production /path/to/remotr-config
```

For the example above, discovery includes `resource:ubuntu-pro`, the selected
service capability, and the default-full option capability. If an endpoint
lacks any requirement, it becomes capability blocked and continues using its
last compatible artifact.

Review the rendered YAML, commit the source files, and advance the server's
release ref through the normal Git sync workflow. The `@active` reference must
be present in the current composed artifact before activation so the server
can bind the secret version to its exact fleet, artifact digest, resource
address, purpose, and endpoint set.

## 4. Activate and authorize the token version

Activate the exact uploaded version:

```bash
remotr secret activate ubuntu-pro/production <version> --json
```

Attachment is sensitive, so activation discovers the current
`ubuntu-pro-token` use and creates a Change request. The response contains a
rollout binding and `changeRequestId`. Until that request has an active rollout
authorization, affected endpoints cannot resolve the active token.

Review the request before authorizing it:

```bash
remotr change show <change-request-id> --json
remotr change authorize <change-request-id> \
  --attempt-limit 1 \
  --max-concurrency 1 \
  --justification "CHG-4821: attach production Ubuntu Pro subscription"
```

FIPS, FIPS Updates, and real-time-kernel resources retain `boot` risk during
token activation. A resource that disables a service or requests detachment
retains `destructive` risk. Authors can raise these classes but cannot lower
them.

See [Change control](change-control.md#workflow-authorize-a-high-risk-active-secret)
for approval thresholds, execution windows, pause/revoke controls, and
recovery procedures.

## 5. Verify convergence

After the endpoint syncs, inspect its complete state report:

```bash
remotr endpoint state report <endpoint-id> --json
```

Verify the `subscriptions/primary` item, attachment state, each declared
service, warning codes, rollback/residual-effects class, and any
`reboot-required` activation signal. Remotr reports reboot requirements but
never reboots the endpoint.

An already attached endpoint does not resolve the token again or replace its
contract solely because the active secret version changes. A second Apply on
compliant state performs no attachment or service mutation.

## Rotate the token

Upload a new inactive version under the same logical name, verify its safe
fingerprint, then activate that exact version. The activation creates a new
Change request for current `@active` uses.

Rotation does not force a healthy, already attached endpoint to detach or
reattach. Keep the prior version until offline endpoints and retained recovery
references no longer need it, then revoke it according to the
[secret rotation workflow](secret-management.md#rotate-safely).

## Detach explicitly

Detachment must be an explicit resource state with no token or services:

```yaml
- kind: ubuntuPro
  name: primary
  lifecycle: detached
```

Commit and release this destructive change, then review and authorize its
Change request. Removing the resource from Git only relinquishes Remotr
ownership; it does not detach the subscription or disable services.

## Troubleshooting

| Symptom | What to check |
| --- | --- |
| Endpoint is capability blocked | Exact Ubuntu identity, LTS release, amd64 architecture, Ubuntu Pro API availability, and every capability printed by `config discover`. |
| Access-only or disabled service is blocked | Expected today: access-only and disable-behavior rows are unadvertised even though their syntax validates. |
| Activation returns no Ubuntu Pro rollout binding | The current release ref must contain the matching `@active` reference, logical secret name, fleet, and resource address. Confirm the fleet has enrolled endpoints and current reports. |
| Token resolution is unauthorized | Secret scope, active version, artifact digest, `ubuntu-pro-token` purpose, Change request approvals, rollout window, and endpoint membership. |
| Attachment reports invalid or expired token | Upload a valid token as a new version; do not place it in argv, environment variables, Git, or logs. |
| Provider reports a missing endpoint | Upgrade Ubuntu Pro Client. Remotr does not fall back to ordinary `pro attach`, `status`, `enable`, or `disable` commands. |
| Service reports unavailable or unentitled | Confirm the subscription entitlement and the exact service/release capability; Remotr does not report the service compliant. |
| Reboot is required | Coordinate the reboot through a separate maintenance workflow; `ubuntuPro` never executes it. |
| Pop!_OS or another derivative is rejected | Expected: Ubuntu lineage is insufficient. Ubuntu Pro requires exact, consistent Canonical Ubuntu identity. |

## Scope and qualification limits

The provider manages subscription attachment, explicit detachment, and
qualified service state only. It does not manage Landscape, install Ubuntu Pro
Client, change Pro client settings, apply security fixes or hardening
profiles, upgrade packages, or execute reboots.

Remotr's qualification suite uses pinned Ubuntu VMs, deterministic API doubles,
and synthetic tokens. It does not consume a live Canonical token or claim that
CI observed entitled package, snap, repository, kernel, boot-artifact, or
compliance-tool effects. Production operation uses the Canonical token that
you upload to the encrypted secret registry.
