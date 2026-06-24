#!/bin/sh
set -eu

MINIO_ALIAS="${MINIO_ALIAS:-local}"
MINIO_URL="${MINIO_URL:-http://minio:9000}"
MINIO_ROOT_USER="${MINIO_ROOT_USER:-remotr}"
MINIO_ROOT_PASSWORD="${MINIO_ROOT_PASSWORD:-remotremotr}"
BUCKET="${REMOTR_S3_BUCKET:-remotr-packages}"
KEY="${REMOTR_E2E_APP_S3_KEY:-app-packages/e2e/test-cli/1.0.0/e2e_test-cli-1.0.0.zip}"
PACKAGE_ZIP="${PACKAGE_ZIP:-/package.zip}"

mc alias set "$MINIO_ALIAS" "$MINIO_URL" "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD"
mc mb --ignore-existing "$MINIO_ALIAS/$BUCKET"
mc cp "$PACKAGE_ZIP" "$MINIO_ALIAS/$BUCKET/$KEY"
echo "uploaded s3://$BUCKET/$KEY"
