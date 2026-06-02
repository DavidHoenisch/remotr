# Remotr

Pull-based MDM and state management for Linux. Admins publish desired state through Git; the server syncs from a **configuration repository** and serves deployable artifacts to enrolled endpoints over mTLS. Agents phone home on a schedule—no inbound ports or SSH required.

**Documentation:** [docs/](docs/README.md) — tutorials, operator guides, reference, and architecture.

Domain terminology: [CONTEXT.md](CONTEXT.md).

## How it works

```text
  Configuration repo (Git)          Remotr server              Endpoint
  fleets/<fleet>/desired.yaml  -->  syncs release ref    -->   remotr-agent (systemd)
  endpoints/<id>/desired.yaml     Postgres registry          CSR enroll → mTLS sync
                                  issues endpoint certs        resolve → check → apply
```

1. Desired state lives in Git at `fleets/<fleet-name>/desired.yaml` (optional per-endpoint overrides).
2. The server tracks a **release ref** (commit SHA) and serves artifact bytes plus a digest to agents.
3. Each endpoint runs `remotr-agent`, which syncs over mTLS. Identity comes from the client certificate—never from self-reported IDs in the request body.
4. New machines enroll with a one-time token via `POST /v1/enroll` (CSR by default) and receive an **endpoint credential**.
5. The agent resolves **in-document targeting** locally, then runs **Check** and **Apply** per fleet remediation policy.

## Binaries

| Binary | Path | Role |
|--------|------|------|
| `remotr` | `cmd/remotr` | Operator CLI — GitOps scaffolding and admin API |
| `remotr-server` | `cmd/remotr-server` | HTTPS API: health, enroll, sync, admin, Git webhook |
| `remotr-agent` | `cmd/remotr-agent` | Enroll once, then periodic mTLS sync and apply |

```bash
go build -mod=vendor -o bin/remotr ./cmd/remotr
go build -mod=vendor -o bin/remotr-server ./cmd/remotr-server
go build -mod=vendor -o bin/remotr-agent ./cmd/remotr-agent
```

Requires **Go 1.26+**.

## Quick start

**Local Docker stack** (Postgres, server, Debian + Arch agents with production-like enrollment):

```bash
make compose-up
make test-e2e
make compose-down
```

Server: `https://localhost:8443`. Walkthrough: [Getting started](docs/tutorial/getting-started.md).

**Scaffold a configuration repository:**

```bash
go run -mod=vendor ./cmd/remotr init -fleet engineering ./remotr-config
```

With Postgres registration and enrollment token: see [Operator workflows](docs/guides/operator-workflows.md).

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting started](docs/tutorial/getting-started.md) | First run with Compose |
| [Operator workflows](docs/guides/operator-workflows.md) | Bootstrap, tokens, endpoints, Git sync |
| [Agent deployment](docs/guides/agent-deployment.md) | Install, enroll, systemd |
| [Configuration repository](docs/guides/configuration-repository.md) | Git layout and GitOps workflow |
| [Configuration format](docs/reference/configuration-format.md) | YAML reference |
| [Environment variables](docs/reference/environment-variables.md) | Server, agent, CLI |
| [HTTP API](docs/reference/http-api.md) | REST endpoints |
| [Architecture](docs/explanation/architecture.md) | Design and security model |
| [Production deployment](docs/guides/production-deployment.md) | Server + Postgres + agents outside Compose |
| [Troubleshooting](docs/guides/troubleshooting.md) | Common failures |
| [CA rotation](docs/runbooks/ca-rotation.md) | Certificate maintenance |

## State format (summary)

Artifacts are YAML with a `configurations` list. Each slice can declare packages, files, users, systemd units, and commands with optional `targetDistros` / `targetArch`.

```yaml
configurations:
  - name: base-packages
    targetDistros: [Debian, Arch]
    packages:
      - name: curl
        present: true
        packageManager: apt
      - name: curl
        present: true
        packageManager: pacman
```

Full reference: [Configuration format](docs/reference/configuration-format.md).

Build locally:

```bash
make docker-server-build
```

### Container image

Production image: `docker/remotr-server/Dockerfile` (Alpine 3.21 runtime). Published to Docker Hub via GitHub Actions on server code changes.

```bash
make docker-server-build
docker pull <dockerhub-user>/remotr-server:latest
```

See [Production deployment](docs/guides/production-deployment.md#docker-hub-image) for secrets and run configuration.

## Development

```bash
make test          # unit tests
make test-e2e      # Docker stack + integration tests
make fuzz-short    # short fuzz run
make gosec         # static analysis
make vendor        # refresh vendor/
```

Progress: [CHECKLIST.md](CHECKLIST.md).

## Architecture decisions

- [ADR 001: Vendored allowlist](docs/adr/001-vendored-allowlist.md)
- [ADR 002: Postgres server registry](docs/adr/002-postgres-server-registry.md)

## Dependencies

Vendored allowlist: [chi](https://github.com/go-chi/chi), [yaml.v3](https://gopkg.in/yaml.v3), [pgx](https://github.com/jackc/pgx), [uuid](https://github.com/google/uuid).

## License

See repository license file if present; otherwise treat as unpublished work in active development.
