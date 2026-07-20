# Ubuntu 24.04 M1-M5 qualification repository

This source-only repository exercises Remotr configuration validation,
discovery, and deterministic composition for the Ubuntu 24.04 amd64
qualification target. Values are inert and test-only. High-risk resources are
report-only unless a disposable qualification fixture explicitly authorizes
enforcement.

Do not add real credentials, deployable placeholders, `desired.yaml`, or
`crons.yaml` to this repository. Package, signing-key, and repository examples
are owned by the `complete-core-package-providers` change.
