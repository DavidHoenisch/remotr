# Remotr documentation

Remotr is pull-based MDM for Linux: desired state lives in Git, the server serves deployable artifacts over mTLS, and agents sync on a schedule without inbound ports.

For domain terminology (fleet, deployable artifact, release ref, drift, and similar), see [CONTEXT.md](../CONTEXT.md) in the repository root.

## Documentation map

| Type | Audience | Start here |
|------|----------|------------|
| **Tutorial** | New operators and contributors | [Getting started](tutorial/getting-started.md) |
| **How-to guides** | Day-to-day operations | [Operator workflows](guides/operator-workflows.md), [Installing the CLI](guides/installing-cli.md), [Installing the agent](guides/installing-agent.md), [Agent deployment](guides/agent-deployment.md), [Configuration repository](guides/configuration-repository.md), [Production deployment](guides/production-deployment.md), [Fly.io bootstrap](../deploy/fly/README.md) |
| **Reference** | Lookup while working | [Configuration format](reference/configuration-format.md), [Environment variables](reference/environment-variables.md), [HTTP API](reference/http-api.md) |
| **Explanation** | Design and security model | [Architecture](explanation/architecture.md) |
| **Runbooks** | Production maintenance | [CA rotation](runbooks/ca-rotation.md) |

## Quick links

- [Build and run the local Docker stack](tutorial/getting-started.md#run-the-local-stack)
- [Bootstrap the first operator](guides/operator-workflows.md#bootstrap-the-first-operator) (includes CLI terminal recordings)
- [Install an endpoint agent](guides/installing-agent.md) (paste-and-run script)
- [Enroll a new endpoint](guides/agent-deployment.md#enrollment)
- [Upgrade agents in-band](guides/agent-deployment.md#agent-upgrades) (`remotr fleet agent upgrade`)
- [Validate configuration YAML](guides/operator-workflows.md#validate-configuration-before-merge) (`remotr config validate`)
- [Author fleet desired state](guides/configuration-repository.md#fleet-artifacts)
- [Fly.io one-command bootstrap](../deploy/fly/README.md)
- [Troubleshooting](guides/troubleshooting.md)

## Architecture decisions

- [ADR 001: Vendored allowlist](adr/001-vendored-allowlist.md)
- [ADR 002: Postgres server registry](adr/002-postgres-server-registry.md)

## Binaries

| Binary | Role |
|--------|------|
| `remotr` | Operator CLI — GitOps scaffolding and server registry admin |
| `remotr-server` | HTTPS API — enroll, sync, admin, Git webhook |
| `remotr-agent` | Endpoint agent — enroll once, then periodic mTLS sync |

Build from the repository root:

```bash
go build -mod=vendor -o bin/remotr ./cmd/remotr
go build -mod=vendor -o bin/remotr-server ./cmd/remotr-server
go build -mod=vendor -o bin/remotr-agent ./cmd/remotr-agent
```

Requires **Go 1.26+**. Dependencies are vendored under `vendor/`.
