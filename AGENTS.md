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

### Mandatory TDD and test evidence

TDD is required for every behavior change. Work one vertical **red → green**
slice at a time; do not write a batch of imagined tests or implementation.

1. Before production code, name the OpenSpec verification ID (when one exists),
   the public seam, and the test layers selected below.
2. Write one focused behavioral test with an independently known expected
   result, run it, and record the intended **red** failure.
3. Add only the minimum implementation needed to make that test **green**.
4. Run the focused test again, then the required risk-based checks and finally
   the relevant broader suite. Refactoring follows successful verification; it
   must not weaken the test's public behavioral assertion.

Tests verify observable behavior at an approved public seam, not private
helpers or interactions between Remotr-owned modules. Use the seams in
`docs/testing/public-seams.md`: configuration CLI, operator CLI/Admin API,
authenticated Sync, composed agent execution, provider contract, system-safety
recovery, and observable performance. Fakes are appropriate only at an OS,
clock, randomness, network, persistence, or other external-service boundary.

Choose evidence by risk; a higher-risk row includes the lower-risk evidence
where it applies.

| Changed behavior | Required evidence |
| --- | --- |
| Pure validation, model, or configuration behavior | Focused seam test; negative and boundary cases; table tests where inputs vary. Add or strengthen a bounded fuzz property for parser/schema input. |
| Public CLI, Admin API, enrollment, or Sync behavior | Focused public-interface test plus malformed, unauthenticated, authorization, and regression cases that apply. Use authenticated protocol integration rather than a database side channel. |
| Agent execution or provider behavior | Provider-contract compliant/drifted/Apply/second-check evidence; exact argv assertions at process boundaries; real provider container evidence before claiming container support. |
| Connectivity, boot, storage, firewall, identity, or other destructive safety behavior | Provider evidence plus the relevant Vagrant VM safety/recovery fixture. Docker is not a substitute for VM safety evidence. |
| Time, retry, polling, overload, concurrency, or ordering behavior | Inject clock and randomness; use deterministic unit/property tests with no wall-clock sleeps. Add an authenticated load-harness scenario when behavior can synchronize or amplify fleet requests. |
| Hot parsing, composition, Sync, or database path | Native benchmark with allocation reporting and representative fixture size; use controlled Postgres/load evidence when the path crosses those boundaries. |
| Secret, credential, authorization, redaction, or rollback behavior | Negative tests, secret-canary/redaction checks, and persistence/cleanup evidence. Add focused mutation evidence when the critical logic is in mutation scope. |
| Implemented public end-to-end workflow | Add or extend a Godog scenario only through the declarative public step vocabulary. Do not add Gherkin for private algorithms or unimplemented roadmap behavior. |

Coverage, a mock call count, or a passing unit test is never a substitute for
provider, safety, mutation, or performance evidence selected above. Do not
derive expected output from the implementation under test; do not mock a
Remotr-owned collaborator merely to assert its call order; and do not weaken or
delete evidence without the governing OpenSpec/traceability update. New fuzz
crashes become committed seed regressions. Skips, quarantines, manual evidence,
and equivalent mutants require a reviewed, expiring record in
`test/evidence-exceptions.yaml`.

Run the narrowest useful command first, then `make test` before handoff. Use
`make test-e2e-quick`, provider/Vagrant fixtures, benchmark collection, and the
authenticated load targets only when their selected evidence layer requires
them. See `docs/testing/foundation-operations.md` for commands, controlled
environment safety, CI ownership, failure triage, and baseline changes.

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
