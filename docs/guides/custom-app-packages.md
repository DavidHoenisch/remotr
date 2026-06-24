# Custom app packages

Publish zip-based CLI tools and internal apps to a private S3-compatible bucket. Endpoints install them via the `customApps` desired-state resource; the server mints presigned download URLs at apply time.

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

Requires operator credentials and S3 env (`REMOTR_S3_BUCKET` or `BUCKET_NAME`, plus AWS credentials). Optional: `REMOTR_S3_REGION`, `REMOTR_S3_ENDPOINT` (or Fly’s `AWS_REGION`, `AWS_ENDPOINT_URL_S3`).

```bash
export REMOTR_S3_BUCKET=my-private-bucket
remotr app publish ./mycli-1.4.0.zip
remotr app list
remotr app show internal/mycli 1.4.0
```

## Assign to endpoints

Reference the catalog entry in Git (`desired.yaml`):

```yaml
customApps:
  - name: mycli
    package: internal/mycli
    version: "1.4.0"
    present: true
```

On sync the agent checks `/var/lib/remotr/apps/<package>/version`. When drift is detected and remediation policy is `auto`, the agent requests `POST /v1/app-packages/download-url`, downloads the zip, and runs the manifest install steps.

## Server configuration

| Variable | Purpose |
|----------|---------|
| `REMOTR_S3_BUCKET` | Private bucket for package zips (fallback: `BUCKET_NAME` from Fly Tigris) |
| `REMOTR_S3_REGION` | AWS region (fallback: `AWS_REGION`, default `us-east-1`) |
| `REMOTR_S3_ENDPOINT` | Optional S3-compatible endpoint (fallback: `AWS_ENDPOINT_URL_S3`) |
| `REMOTR_S3_PRESIGN_TTL` | Presigned URL lifetime (default 30m) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | Credentials for upload/presign |

On [Fly.io bootstrap](../../deploy/fly/README.md), `fly storage create` sets `BUCKET_NAME` and the `AWS_*` secrets; the server enables app packages automatically without extra `REMOTR_S3_*` secrets.

See [ADR 003](../adr/003-s3-app-packages.md) for the AWS SDK allowlist.
