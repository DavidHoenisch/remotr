# Getting started

This tutorial walks through the full Remotr loop on your machine using the Docker Compose dev stack: Postgres, the server, and two enrolled agents (Debian and Arch). By the end you will have run operator bootstrap, seen agents enroll with one-time tokens, and verified mTLS sync.

Estimated time: 15 minutes.

## Prerequisites

- Docker and Docker Compose v2
- Go 1.26+ (for running tests and the operator CLI from source)
- `make`, `git`

## Run the local stack

From the repository root:

```bash
make compose-up
```

This builds three binaries into container images, generates a Remotr CA and server certificate, applies the Postgres schema, seeds a `test-fleet` with enrollment tokens, starts `remotr-server` on `https://localhost:8443`, and starts two agents that enroll on first boot (CSR-based, same as production).

Verify health:

```bash
curl -k https://localhost:8443/healthz
```

Expected output: `ok`

Sample fleet configuration lives at `compose/config-repo/fleets/test-fleet/desired.yaml`.

## Bootstrap your operator credential

On first start the server emits a **one-time bootstrap token** to stdout and to `compose/runtime/bootstrap.token` (bind-mounted from the server container).

Exchange it for an operator client certificate:

```bash
TOKEN=$(tr -d ' \n\r' < compose/runtime/bootstrap.token)

go run -mod=vendor ./cmd/remotr bootstrap \
  --server-url https://localhost:8443 \
  --ca compose/runtime/certs/ca.crt \
  --token "$TOKEN" \
  --state-dir ./compose/runtime/operator
```

You should see `operator bootstrapped` and credentials under `./compose/runtime/operator/`.

The bootstrap token is invalidated after use. If the file is empty, the stack was already bootstrapped — tear down and recreate with `make compose-down && make compose-up`.

## Inspect enrolled endpoints

Agents enroll automatically in Compose using tokens from `compose/runtime/enroll-tokens/`. List endpoints with your operator credential:

```bash
go run -mod=vendor ./cmd/remotr endpoint list \
  --server-url https://localhost:8443 \
  --state-dir ./compose/runtime/operator
```

Each line shows endpoint UUID, fleet, certificate fingerprint, and any labels reported at sync.

Show detail for one endpoint:

```bash
go run -mod=vendor ./cmd/remotr endpoint show \
  --server-url https://localhost:8443 \
  --state-dir ./compose/runtime/operator \
  --json <endpoint-id>
```

## Run integration tests

The e2e suite exercises the same flows against the running stack:

```bash
make test-e2e
```

This runs `compose-down`, `compose-up`, and all tests under `test/e2e/`. For a faster iteration loop when the stack is already up:

```bash
make test-e2e-quick
```

## Scaffold your own configuration repository

Create a GitOps repo layout with a sample fleet artifact:

```bash
go run -mod=vendor ./cmd/remotr init -fleet engineering ./remotr-config
cd remotr-config
git init
```

The scaffold creates:

- `fleets/engineering/desired.yaml` — deployable artifact for the fleet
- `endpoints/` — optional per-machine overrides
- `remotr.yaml` — operator metadata (not served to agents)
- `server.env.example` — suggested server environment variables

Optional: register the fleet in Postgres and create an enrollment token in one step:

```bash
export REMOTR_DATABASE_URL='postgres://remotr:remotr@localhost:5432/remotr?sslmode=disable'
go run -mod=vendor ./cmd/remotr init ./remotr-config \
  --register-server \
  --enroll \
  --enroll-out ./enroll.token
```

Point a production or self-hosted server at the repo checkout with `REMOTR_CONFIG_REPO` and enroll real machines — see [Agent deployment](../guides/agent-deployment.md).

## Tear down

```bash
make compose-down
```

This removes containers, volumes, and runtime state (agent credentials, enrollment tokens).

## Next steps

- [Operator workflows](../guides/operator-workflows.md) — enrollment tokens, endpoint inventory, remediation policy
- [Configuration repository](../guides/configuration-repository.md) — Git layout, overrides, release ref
- [Configuration format reference](../reference/configuration-format.md) — packages, files, users, systemd, commands
- [Architecture](../explanation/architecture.md) — how identity, sync, and apply fit together
