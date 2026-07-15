# Evaluate Remotr locally

This tutorial runs the complete Remotr loop on one workstation: Postgres, the
HTTPS server, an operator, and Debian and Arch agents. You will bootstrap an
operator, inspect enrolled endpoints, validate a configuration repository, and
verify compliance reports.

Estimated time: 15–25 minutes.

!!! note "This is an evaluation environment"
    The Compose stack creates development certificates, predictable database
    credentials, and disposable state. Do not reuse its keys, tokens, or
    settings in production.

## Before you start

Run these commands from the Remotr source repository. You need:

- Docker Engine with the Compose v2 plugin;
- Go 1.26 or later;
- `make`, `git`, and `curl`;
- permission to run Docker and `sudo` to read the protected bootstrap token.

Confirm the tools are available:

```bash
docker version
docker compose version
go version
```

## 1. Start the stack

```bash
make compose-up
```

The first build can take several minutes. The command starts Postgres,
`remotr-server`, and two agents. The agents use the same CSR enrollment and
mTLS sync path as production endpoints.

Wait for the server health check:

```bash
curl -k https://localhost:8443/healthz
```

Expected output:

```text
ok
```

If it is not healthy, inspect the stack before continuing:

```bash
docker compose -f compose/docker-compose.yml ps
docker compose -f compose/docker-compose.yml logs remotr-server
```

## 2. Bootstrap the first operator

The server creates one bootstrap token on its first start. The token file is
root-owned and mode `0600`; read it with `sudo` and remove whitespace:

```bash
TOKEN=$(sudo cat compose/runtime/bootstrap.token | tr -d ' \n\r')
test -n "$TOKEN"
```

Set the connection once so later commands stay readable. `REMOTR_CA` must be
an absolute path.

```bash
export REMOTR_SERVER_URL=https://localhost:8443
export REMOTR_OPERATOR_STATE_DIR="$(pwd)/compose/runtime/operator"
export REMOTR_CA="$(pwd)/compose/runtime/certs/ca.crt"
```

Exchange the token for an operator certificate:

```bash
go run -mod=vendor ./cmd/remotr bootstrap --token "$TOKEN"
```

Expected output includes `operator bootstrapped`. The command writes
`operator.crt`, `operator.key`, `ca.crt`, and `state.json` below the operator state
directory. Protect that directory like an administrative credential.

The token is invalid after a successful exchange. To repeat the tutorial from
scratch, recreate all disposable state:

```bash
make compose-down
make compose-up
```

Then read the newly generated token. Do not keep retrying an already consumed
token.

## 3. Check the operator setup

```bash
go run -mod=vendor ./cmd/remotr doctor
```

Resolve warnings before continuing. In particular, confirm that the server
URL, CA, operator certificate, and private key all pass.

## 4. Inspect the enrolled endpoints

```bash
go run -mod=vendor ./cmd/remotr endpoint list
```

You should see the Debian and Arch agents in `test-fleet`. Copy one endpoint
ID and inspect it:

```bash
go run -mod=vendor ./cmd/remotr endpoint show <endpoint-id>
go run -mod=vendor ./cmd/remotr endpoint state report <endpoint-id>
```

The state report may initially be empty while the agent completes its first
sync. Wait one sync interval and run it again. `--json` is available when you
want stable machine-readable output.

## 5. Inspect and validate desired state

The example source is under `compose/config-repo/`:

```text
compose/config-repo/
├── fleets/test-fleet/manifest.yaml
└── modules/
```

The manifest selects reusable modules; the server composes those sources into
the artifact sent to agents. Generated `desired.yaml` and `crons.yaml` files
do not belong in the repository.

Validate and preview the exact fleet artifact:

```bash
go run -mod=vendor ./cmd/remotr config validate compose/config-repo
go run -mod=vendor ./cmd/remotr config discover --fleet test-fleet compose/config-repo
go run -mod=vendor ./cmd/remotr config render --fleet test-fleet compose/config-repo
```

`validate` should finish without errors. `render` writes to stdout only, which
makes it safe to use in review and CI.

## 6. Create your own repository

Choose one of these two paths. `remotr init` intentionally refuses to write
into a non-empty scaffold, so do not run the second form after the first.

### Repository only

Use this when the fleet will be registered later through your deployment
workflow:

```bash
go run -mod=vendor ./cmd/remotr init --fleet engineering ./remotr-config
git -C ./remotr-config init
go run -mod=vendor ./cmd/remotr config validate ./remotr-config
go run -mod=vendor ./cmd/remotr config render --fleet engineering ./remotr-config
```

### Repository, fleet registration, and first enrollment token

Use this alternative only when your workstation may connect directly to the
server database:

```bash
export REMOTR_DATABASE_URL='postgres://remotr:remotr@localhost:5432/remotr?sslmode=disable'
go run -mod=vendor ./cmd/remotr init \
  --fleet engineering \
  --register-server \
  --enroll \
  --enroll-out ./engineering.enroll.token \
  --quiet \
  ./remotr-config
```

The enrollment-token file is written mode `0600`. Move it through a protected
channel and delete it after enrollment.

The generated module uses canonical schema 1:

```yaml
kind: module
schemaVersion: 1
configurations:
  - name: base-packages
    targetDistros: [Debian, Arch]
    resources:
      - kind: package
        name: curl
        lifecycle: present
```

See [Write your first managed fleet](first-managed-fleet.md) for a complete
configuration-authoring workflow.

## 7. Optional: run the end-to-end checks

To destroy and recreate the stack before testing:

```bash
make test-e2e
```

When the stack is already healthy and you want the shorter loop:

```bash
make test-e2e-quick
```

## 8. Tear down

```bash
make compose-down
```

This deletes Compose containers, the Postgres volume, and generated agent and
enrollment-token state. Bind-mounted data under `compose/runtime/`—including
development certificates, operator credentials, and MinIO objects—can remain.
To erase the evaluation environment completely, inspect that directory and
delete it explicitly after `compose-down`; this is irreversible and invalidates
all local credentials and package objects.

## Where to go next

- [Write your first managed fleet](first-managed-fleet.md)
- [Production deployment](../guides/production-deployment.md)
- [Configuration repository workflow](../guides/configuration-repository.md)
- [Repository file kinds](../reference/repository-kinds.md)
- [Resource kinds](../reference/resource-kinds.md)
- [CLI reference](../reference/cli.md)
- [Troubleshooting](../guides/troubleshooting.md)
