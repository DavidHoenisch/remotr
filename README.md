# Remotr

<div align=center>
  <p>There are many MDMs, but this one is mine</p>
</div>

## Early days

Remotr is young. The core loop works—GitOps config, enroll, sync, apply—but there is still a lot to improve, and rough edges are expected.

I'd love contributions: bug reports, docs fixes, ideas, and pull requests all help. If you want to get involved, start with [Development](#development) and the [implementation checklist](CHECKLIST.md).

Remotr is an MDM solution (mainly focused on config management at this phase) for Linux.
Remotr focuses on giving admins a DevOps way of managing devices. Very similar to
Ansible, configs are written in YAML and can be version controlled with Git.
Unlike Ansible, however, the infrastructure requirements are much less. Ansible usually requires:

1. A central server to run Ansible playbooks from
2. Inbound network allowlists so you can reach devices over SSH
   (you can use ZTNA solutions for this... but that just adds more required infrastructure to the setup)

Remotr still needs a central server (and a Postgres database), but it does not
require special networking or ZTNA solutions. Remotr agents phone home on set schedules
to pick up the latest configurations and instructions. That makes it easy to
whitelist outbound requests to your Remotr server without ever handling incoming requests from it.

Remotr also gives you native compliance tracking, multi-user/multi-admin tooling, and audit logs so you can hit the ground running. We provide one-click installs for:

1. fly.io + Neon Postgres
2. ...more to come!

The installer script automates setting up the admin server, the Postgres database, and bootstrapping the initial admin endpoint—allowing you to go from zero to up-and-running in under two minutes.

I built this for one of my startups because we use Linux heavily (for example, Omarchy and
Ubuntu derivatives). We had used Ansible but did not like maintaining all the
infrastructure, and we really felt the lack of features like compliance/drift
reporting and audit logs.

## How it works

At a high level: you keep desired state in Git, the server syncs from your **configuration repository**, and enrolled endpoints pull deployable artifacts over mTLS on a schedule. No inbound ports, no SSH.

```text
  Configuration repo (Git)          Remotr server              Endpoint
  fleets/<fleet>/desired.yaml  -->  syncs release ref    -->   remotr-agent (systemd)
  endpoints/<id>/desired.yaml     Postgres registry          CSR enroll → mTLS sync
                                  issues endpoint certs        resolve → check → apply
```

Here's the loop in plain terms:

1. Desired state lives in Git at `fleets/<fleet-name>/desired.yaml`, with optional per-endpoint overrides.
2. The server tracks a **release ref** (commit SHA) and serves artifact bytes plus a digest to agents.
3. Each endpoint runs `remotr-agent`, which syncs over mTLS. Identity comes from the client certificate—not from self-reported IDs in the request body.
4. New machines enroll with a one-time token via `POST /v1/enroll` (CSR by default) and receive an **endpoint credential**.
5. The agent resolves **in-document targeting** locally, then runs **Check** and **Apply** per fleet remediation policy.

Domain terminology lives in [CONTEXT.md](CONTEXT.md). For tutorials, operator guides, reference, and architecture, see [docs/](docs/README.md)—operator CLI walkthroughs include terminal recordings (for example [bootstrap](docs/guides/operator-workflows.md#bootstrap-the-first-operator)).

## What's in the box

Three binaries, three jobs:

| Binary | Path | Role |
|--------|------|------|
| `remotr` | `cmd/remotr` | Operator CLI — GitOps scaffolding, admin API, fleet agent upgrades ([urfave/cli](https://github.com/urfave/cli)) |
| `remotr-server` | `cmd/remotr-server` | HTTPS API: health, enroll, sync, admin, Git webhook |
| `remotr-agent` | `cmd/remotr-agent` | Enroll once, then periodic mTLS sync and apply |

Build them locally (requires **Go 1.26+**):

```bash
go build -mod=vendor -o bin/remotr ./cmd/remotr
go build -mod=vendor -o bin/remotr-server ./cmd/remotr-server
go build -mod=vendor -o bin/remotr-agent ./cmd/remotr-agent
```

## Get started

**Try it locally** with the Docker stack—Postgres, server, and Debian + Arch agents with production-like enrollment:

```bash
make compose-up
make test-e2e
make compose-down
```

Server listens at `https://localhost:8443`. Step-by-step walkthrough: [Getting started](docs/tutorial/getting-started.md).

**Scaffold a configuration repository** when you're ready to define fleet state:

```bash
go run -mod=vendor ./cmd/remotr init -fleet engineering ./remotr-config
```

For Postgres registration and enrollment tokens, see [Operator workflows](docs/guides/operator-workflows.md).

**Deploy to production** without Compose: [Production deployment](docs/guides/production-deployment.md). For the fastest path, the [Fly.io bootstrap](deploy/fly/README.md) script (`curl | bash`) wires up Fly + Neon in one shot—the same flow the one-click installer uses.

Install the operator CLI from GitHub Releases: [Installing the CLI](docs/guides/installing-cli.md). Enroll Linux endpoints with: [Installing the agent](docs/guides/installing-agent.md).

## Documentation

| Guide | Description |
|-------|-------------|
| [Getting started](docs/tutorial/getting-started.md) | First run with Compose |
| [Operator workflows](docs/guides/operator-workflows.md) | Bootstrap, tokens, endpoints, Git sync, agent upgrades |
| [Installing the agent](docs/guides/installing-agent.md) | Paste-and-run install script, CA auto-fetch, deployment tokens |
| [Agent deployment](docs/guides/agent-deployment.md) | Enroll, systemd, sync loop, re-enrollment |
| [Installing the CLI](docs/guides/installing-cli.md) | Download releases, semver, verify checksums |
| [Configuration repository](docs/guides/configuration-repository.md) | Git layout and GitOps workflow |
| [Configuration format](docs/reference/configuration-format.md) | YAML reference |
| [Environment variables](docs/reference/environment-variables.md) | Server, agent, CLI |
| [HTTP API](docs/reference/http-api.md) | REST endpoints |
| [Architecture](docs/explanation/architecture.md) | Design and security model |
| [Production deployment](docs/guides/production-deployment.md) | Server + Postgres + agents outside Compose |
| [Fly.io bootstrap](deploy/fly/README.md) | One-command Fly + Neon deploy (`curl \| bash`) |
| [Troubleshooting](docs/guides/troubleshooting.md) | Common failures |
| [CA rotation](docs/runbooks/ca-rotation.md) | Certificate maintenance |

## Configuration format (at a glance)

Artifacts are YAML with a `configurations` list. Each slice can declare packages, files, downloads, users, systemd (system and user), bootstrap steps, agent install, and commands with optional `targetDistros` / `targetArch`.

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

## Server container image

Production image: `docker/remotr-server/Dockerfile` (Alpine 3.21 runtime). Published to Docker Hub via GitHub Actions when server code changes.

```bash
make docker-server-build
docker pull <dockerhub-user>/remotr-server:latest
```

See [Production deployment](docs/guides/production-deployment.md#docker-hub-image) for secrets and run configuration.

## Development

If you're hacking on Remotr itself:

```bash
make test          # unit tests
make test-e2e      # Docker stack + integration tests
make fuzz-short    # short fuzz run
make gosec         # static analysis
make vendor        # refresh vendor/
```

**Documentation GIFs** (requires [VHS](https://github.com/charmbracelet/vhs), `ttyd`, and `ffmpeg`): operator CLI demos use fixture-backed `REMOTR_DEMO` mode (set by Make, not shown in recordings). Regenerate fixtures and all tapes with `make demo-record-all`, or one tape with `make demo-record TAPE=init`. Output: `demo/assets/`.

Progress tracker: [CHECKLIST.md](CHECKLIST.md).

## Architecture decisions

- [ADR 001: Vendored allowlist](docs/adr/001-vendored-allowlist.md)
- [ADR 002: Postgres server registry](docs/adr/002-postgres-server-registry.md)

## Dependencies

Vendored allowlist: [chi](https://github.com/go-chi/chi), [urfave/cli](https://github.com/urfave/cli), [yaml.v3](https://gopkg.in/yaml.v3), [pgx](https://github.com/jackc/pgx), [uuid](https://github.com/google/uuid).

## License

See repository license file if present; otherwise treat as unpublished work in active development.
