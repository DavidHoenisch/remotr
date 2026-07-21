# Remotr documentation

Remotr is pull-based MDM for Linux: desired state lives in Git, the server serves deployable artifacts over mTLS, and agents sync on a schedule without inbound ports.

## Choose your path

=== "Deploy to production"

    Most operators start here.

    1. **[Production deployment](guides/production-deployment.md)** — architecture, Postgres, server hardening, bootstrap.
    2. **[Fly.io bootstrap](guides/fly-io.md)** — one-command server on Fly + Neon Postgres.
    3. **[Install the operator CLI](guides/installing-cli.md)** — bootstrap credentials and day-two commands.
    4. **[Install agents](guides/installing-agent.md)** — paste-and-run enrollment on endpoints.
    5. **[Configuration repository](guides/configuration-repository.md)** — fleet layout and GitOps workflow.

=== "Operate a fleet"

    You already have a server and enrolled machines.

    - **[Operator overview](guides/operator-overview.md)** — credentials, CLI layout, quick links.
    - **[Use Remotr Desktop](guides/use-remotr-desktop.md)** — install and operate the native Linux Fleet workspace.
    - **[Enrollment tokens](guides/enrollment-tokens.md)** — single-machine and bulk deployment tokens.
    - **[Endpoint management](guides/endpoint-management.md)** — inventory, upgrades, cron reports.
    - **[Git sync workflow](guides/git-sync-workflow.md)** — release ref, webhooks, private repos.
    - **[Change control](guides/change-control.md)** — review boundary, approvals, rollouts, baselines, and current limitations.
    - **[Secret management](guides/secret-management.md)** — encrypted versions, references, activation, rotation, and recovery.
    - **[Troubleshooting](guides/troubleshooting.md)** — common failures and diagnostics.

=== "Develop locally"

    For contributors and evaluators who want the Docker Compose stack.

    - **[Getting started (local)](tutorial/getting-started.md)** — Postgres, server, two test agents, bootstrap.
    - **[Write your first managed fleet](tutorial/first-managed-fleet.md)** — canonical manifests, modules, resources, validation, and release.

## Quick tasks

| I want to… | Go to |
|------------|-------|
| Bootstrap the first operator | [Bootstrap operator](guides/bootstrap-operator.md) |
| Enroll a new machine | [Installing the agent](guides/installing-agent.md) |
| Author fleet configuration | [First managed fleet](tutorial/first-managed-fleet.md), [Configuration repository](guides/configuration-repository.md) |
| Choose a repository file kind | [Repository file kinds](reference/repository-kinds.md) |
| Look up any of the 47 resources | [Resource kinds](reference/resource-kinds.md) |
| Understand Check, Apply, ownership, or rollback | [Applicator execution contract](reference/applicator-execution.md) |
| Diagnose a capability-blocked endpoint | [Endpoint management](guides/endpoint-management.md#distinguish-target-offered-and-active-artifacts), [Capability-compatible delivery](reference/capability-compatible-delivery.md) |
| Check an exact Linux/provider support claim | [Ubuntu 24.04 support](reference/ubuntu-2404-applicator-support.md), [Package provider qualification](testing/package-provider-qualification.md) |
| Author firewall rules | [Configuration format — Firewall](reference/configuration-format.md#firewall-resources) |
| Compose modules into artifacts | [Manifest format](reference/manifest-format.md) |
| Validate YAML before merge | [Config validation](guides/config-validation.md) |
| Review a high-risk activation | [Change control](guides/change-control.md) |
| Upload or rotate a secret | [Secret management](guides/secret-management.md) |
| Look up a CLI command | [CLI reference](reference/cli.md) |
| Upgrade agents in-band | [Endpoint management](guides/endpoint-management.md#request-in-band-agent-upgrades) |
| Inspect cron job status | [Endpoint management](guides/endpoint-management.md#cron-job-status) |
| Inspect firewall rules and audit logs | [Endpoint management](guides/endpoint-management.md#firewall-inspection) |
| Understand security model | [Architecture](explanation/architecture.md) |
| Look up env vars or API | [Environment variables](reference/environment-variables.md), [HTTP API](reference/http-api.md) |
| Rotate the Remotr CA | [CA rotation](runbooks/ca-rotation.md) |
| Browse community snippets | [Hub catalog](/hub/) |

## Terminology

Domain terms (configuration repository, release ref, deployable artifact, fleet path, drift, and more) are defined in the **[terminology glossary](explanation/terminology.md)**.

## Binaries

| Binary | Role |
|--------|------|
| `remotr` | Operator CLI — GitOps scaffolding and server registry admin |
| `remotr-server` | HTTPS API — enroll, sync, admin, Git webhook |
| `remotr-agent` | Endpoint agent — enroll once, then periodic mTLS sync |
| `remotr-desktop` | Native Linux administration workspace over the same Admin API |

Build from the repository root (Go 1.26+, vendored modules):

```bash
go build -mod=vendor -o bin/remotr ./cmd/remotr
go build -mod=vendor -o bin/remotr-server ./cmd/remotr-server
go build -mod=vendor -o bin/remotr-agent ./cmd/remotr-agent
```

Remotr Desktop is an isolated Wails module with native Linux prerequisites.
Build it with `make desktop-build DESKTOP_VERSION=0.0.0-local.1`; see
[Develop Remotr Desktop](guides/develop-remotr-desktop.md).
