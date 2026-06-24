# ADR 003: S3-compatible storage for custom app packages

## Status

Accepted

## Context

Operators need to distribute custom CLI tools and internal applications to endpoints via zip packages stored in private object storage. The server mints presigned download URLs at apply time; agents fetch packages over HTTPS.

Implementing S3 presign and upload without an SDK would duplicate AWS Signature Version 4 logic and increase maintenance risk.

## Decision

1. Allow minimal **aws-sdk-go-v2** modules for S3-compatible APIs:
   - `github.com/aws/aws-sdk-go-v2/config`
   - `github.com/aws/aws-sdk-go-v2/credentials`
   - `github.com/aws/aws-sdk-go-v2/service/s3`
2. Use standard AWS environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`) plus Remotr-specific `REMOTR_S3_BUCKET` and optional `REMOTR_S3_ENDPOINT` for MinIO/Tigris.
3. Vendor all new modules per ADR 001.

## Consequences

- Supply chain surface grows slightly; scope is limited to S3 client operations (PutObject, GetObject presign).
- Server and operator CLI can upload and presign without subprocess hacks.
- S3-compatible providers work via custom endpoint configuration.
