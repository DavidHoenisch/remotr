# Ubuntu 24.04 M1-M5 qualification repository

This source-only repository exercises Remotr configuration validation,
discovery, and deterministic composition for the Ubuntu 24.04 amd64
qualification target. Values are inert and test-only. High-risk resources are
report-only unless a disposable qualification fixture explicitly authorizes
enforcement.

Do not add real credentials, deployable placeholders, `desired.yaml`, or
`crons.yaml` to this repository. Package, signing-key, and repository examples
are owned by the `complete-core-package-providers` change.

## OS-AEC-095 TDD evidence

- Public seam: `remotr config discover`, `config validate`, and `config render`.
- Expected result: every exact composed manifest address and authored field is
  preserved, discovery reports every expected capability requirement, and all
  guarded examples retain `policy: report` with `enforce: false`.
- Observed red: the focused acceptance scenario reported that
  `m3-storage/managed-mount` rendered `enforce: false` without the required
  report-only policy.
- Green result: adding explicit report policy to every guarded source resource
  made `TestUbuntu2404QualificationRepository` pass without changing runtime
  provider code.
- Broader checks: the full acceptance package, traceability lint, qualification
  manifest validator, strict OpenSpec validation, and repeated public render
  all pass; the fixture remains free of generated composed artifacts.
