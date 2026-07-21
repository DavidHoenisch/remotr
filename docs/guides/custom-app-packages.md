# Custom app packages

Publish zip-based CLI tools and internal apps to a private S3-compatible bucket. Endpoints install them via `packages` with `packageManager: remotr`; the server mints presigned download URLs at apply time.

## Package layout

Each zip must contain `remotr-package.yaml` at the archive root:

```yaml
schemaVersion: 1
name: internal/mycli
version: "1.4.0"
install:
  mode: binary
  files:
    - src: bin/mycli-linux-amd64
      dest: /usr/local/bin/mycli
      mode: "0755"
      arch: x86
check:
  command: ["/usr/local/bin/mycli", "--version"]
  expect: "1.4.0"
```

Install modes:

| Mode | Purpose |
|------|---------|
| `binary` | Copy prebuilt files from the zip (Go/Rust/Zig CLIs) |
| `script` | Run `install.script` after extract |
| `build` | Run `install.build` steps, then optional `install.script` (Python/JS/TS) |

Validate a built zip or build from source:

```bash
remotr package create --path ./mycli --name internal/mycli --mode binary
remotr package build --path ./mycli --output ./mycli-0.1.0.zip
remotr package build --path ./mycli --push
```

Legacy zip-only commands still work:

```bash
remotr app package validate ./mycli-1.4.0.zip
```

## Publish

Requires operator credentials only. The server stores the zip in S3 and registers the catalog entry.

For CI or release automation, stamp a dedicated operator with the `package_manager` role instead of `global_admin`. See [RBAC](rbac.md).

```bash
remotr app publish ./mycli-1.4.0.zip
remotr app list
remotr app show internal/mycli 1.4.0
```

`remotr package build --push` uses the same server upload path. Operators never need bucket URLs or AWS keys locally — only the server holds S3 credentials.

The server chooses the immutable object key from package name and version.
Omit `--s3-key`; despite the compatibility flag's current help text, an
arbitrary custom key is rejected. Uploading an existing name/version returns a
conflict rather than replacing bytes in place.

## Assign to endpoints

Add deploy assignments as `kind: application` files under `applications/` and list them from the fleet manifest `applications:` field:

```yaml
kind: application
name: internal/mycli
present: true
packageManager: remotr
version: "1.4.0"
```

Fleet selection:

```yaml
kind: manifest
modules:
  - modules/base-packages.yaml
applications:
  - internal/mycli
  - slack
```

Legacy: a full configuration slice still works (`kind: module` with `configurations:`).

Run `remotr config validate .` after editing modules or manifests.

The `customApps` resource type is no longer supported.

On sync the agent checks `/var/lib/remotr/apps/<package>/version`. When drift is detected and remediation policy is `auto`, the agent requests `POST /v1/app-packages/download-url`, downloads the zip, and runs the manifest install steps.

Current removal is intentionally limited: changing the application to absent
removes the Remotr version marker but does not execute `uninstall.script` or
remove installed files. Also, `check.versionFile` is validated while authoring
but the steady-state applicator currently checks the default marker path.
Script/build commands do not change into the extracted package directory, so
scripts must anchor payload paths to their own `$0` location. A failed
post-install check leaves the marker in place and can appear compliant on the
next marker-only check.
Plan explicit cleanup through another managed resource until these behaviors
are implemented. See the [custom package manifest reference](../reference/custom-package-format.md)
for full fields, modes, validation, and examples.

## Server configuration

S3 credentials belong on the **server** only. Agents and operators talk to Remotr over mTLS; the server uploads zips and mints presigned download URLs.

| Variable | Purpose |
|----------|---------|
| `REMOTR_S3_BUCKET` | Private bucket for package zips (fallback: `BUCKET_NAME` from Fly Tigris) |
| `REMOTR_S3_REGION` | AWS region (fallback: `AWS_REGION`, default `us-east-1`) |
| `REMOTR_S3_ENDPOINT` | Optional S3-compatible endpoint (fallback: `AWS_ENDPOINT_URL_S3`) |
| `REMOTR_S3_PRESIGN_TTL` | Presigned URL lifetime (default 30m) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Server credentials for upload/presign |

On [Fly.io bootstrap](fly-io.md), `fly storage create` sets `BUCKET_NAME` and the `AWS_*` secrets on the app.

Rotate server credentials without exposing secrets: `deploy/fly/rotate-tigris-credentials.sh` (see Fly deploy README).

See [ADR 003](https://github.com/DavidHoenisch/remotr/blob/master/engineering/adr/003-s3-app-packages.md) for the AWS SDK allowlist.
