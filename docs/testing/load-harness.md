# Authenticated Sync load harness

`cmd/remotr-load` provisions a fresh one-time enrollment token and unique mTLS
identity for every requested endpoint, then sends one concurrent authenticated
Sync wave through the real server and configured Postgres registry. It does not
approximate fleet activity with health checks or shared credentials.

The command refuses to run without `--allow-load`, a positive `--endpoints`,
and explicit server URL, CA path, Postgres URL, and fleet. Set the equivalent
`REMOTR_LOAD_*` variables only for a disposable environment. The Postgres URL
is used to create one-time enrollment tokens, so it must never reference a
production database. Endpoint identities are prefixed `load-<run-id>-` and
their TLS material remains only in process memory.

Example for an already-running disposable Compose stack:

```sh
REMOTR_LOAD_SERVER_URL=https://localhost:8443 \
REMOTR_LOAD_CA="$PWD/compose/runtime/certs/ca.crt" \
REMOTR_LOAD_DATABASE_URL=postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable \
REMOTR_LOAD_FLEET=test-fleet \
go run -mod=vendor ./cmd/remotr-load --allow-load --endpoints 10 --concurrency 10
```

The JSON result reports client-observed request count, successes, errors,
artifact bytes, and p50/p95/max latency. Steady-poll, startup, fan-out,
telemetry, degradation, and soak modes build on this identity-provisioning
boundary rather than bypassing it.
