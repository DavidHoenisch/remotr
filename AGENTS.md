# AGENTS.md

## Cursor Cloud specific instructions

### Product overview

Remotr is a Go monorepo for pull-based Linux MDM: `remotr-server` (HTTPS API + Postgres), `remotr-agent` (endpoint sync/apply), and `remotr` (operator CLI). Local development uses Docker Compose under `compose/`.

### System dependencies (VM snapshot)

- **Go 1.26+** — preinstalled on the VM; builds use vendored modules (`-mod=vendor`).
- **Docker** — required for the Compose stack. In this Cloud VM, `dockerd` must run with `fuse-overlayfs` (see daemon.json). Start it in a tmux session if not already running: `sudo dockerd`. Use `sudo docker` / `sudo make compose-up` unless the user is in the `docker` group.

### Services

| Service | Port | Start |
|---------|------|-------|
| Postgres 16 | 5432 | `make compose-up` |
| remotr-server | 8443 (HTTPS) | `make compose-up` |
| agent-debian, agent-arch | — | `make compose-up` |

One command for the full stack: `make compose-up` from repo root. Tear down: `make compose-down`.

Verify server: `curl -k https://localhost:8443/healthz` → `ok`.

### Operator CLI in Compose

Global flags (`--server-url`, `--state-dir`, `--ca`) must appear **before** the subcommand, or use environment variables:

```bash
export REMOTR_SERVER_URL=https://localhost:8443
export REMOTR_OPERATOR_STATE_DIR=/workspace/compose/runtime/operator
export REMOTR_CA=/workspace/compose/runtime/certs/ca.crt   # absolute paths required for --ca
TOKEN=$(sudo cat compose/runtime/bootstrap.token | tr -d ' \n\r')
go run -mod=vendor ./cmd/remotr bootstrap --token "$TOKEN"
go run -mod=vendor ./cmd/remotr endpoint list
```

`compose/runtime/bootstrap.token` is root-owned (`600`); read it with `sudo cat`. After bootstrap the token is invalidated; recreate the stack with `make compose-down && make compose-up` for a fresh token.

### Tests and lint

| Command | Purpose |
|---------|---------|
| `make test` | Unit tests (no Docker) |
| `make test-e2e` | `compose-down` + `compose-up` + e2e tests (`-tags=e2e`) |
| `make test-e2e-quick` | E2e against an already-running stack |
| `make gosec` | Static analysis (install: `go install github.com/securego/gosec/v2/cmd/gosec@latest`) |

Stack/agent sync e2e tests pass from a clean Compose stack.

### Test integrity

Implement one vertical red-green slice at a time: name its OpenSpec
verification ID and public seam, add a focused behavioral test, record the
intended red result, then add only the behavior needed to pass. Do not weaken
or delete evidence without the governing OpenSpec/traceability update. Do not
derive expectations from current output, use owned-internal mock call counts,
or add undocumented skips. See `docs/testing/public-seams.md` for approved
observable boundaries.

### Gotchas

- First `compose-up` after cert generation may show agent TLS errors until keys are chmod'd; `make test-e2e` runs the chmod steps automatically.
- Agent containers may log apply failures for packages (e.g. `curl`) in the slim test images; sync/enrollment still works.
- Dependencies are fully vendored in `vendor/`; no `go mod download` is needed for routine dev.

### Configuration repository workflow

Config repos use **kind-tagged YAML** (`kind: manifest`, `kind: module`, `kind: application`, `kind: crons`). The server composes deployable artifacts on every git sync (stored in Postgres `compiled_artifacts`) or on-demand when no cache exists.

| Command | Purpose |
|---------|---------|
| `go run -mod=vendor ./cmd/remotr config validate <repo>` | Validate kinds, references, and composition |
| `go run -mod=vendor ./cmd/remotr config render --fleet <name> <repo>` | Preview composed desired/crons YAML (stdout only) |
| `go run -mod=vendor ./cmd/remotr config discover --fleet <name> <repo>` | List discovered files by kind under a fleet |

Do not commit `desired.yaml` or `crons.yaml` — they are server-composed artifacts. CI validates with `config validate` (see `.github/workflows/config-repo.yml`). Composition failure **blocks** release ref advance on git sync.
