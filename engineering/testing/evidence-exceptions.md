# Reviewed evidence exceptions

An intentionally manual, not-applicable, equivalent-mutant, or quarantined
evidence item must have a reviewed YAML record in `test/evidence-exceptions.yaml`.
Review metadata applies to the complete versioned snapshot; each record remains
individually owned, traceable, justified, and expiring:

```yaml
version: 1
review:
  reviewed_by: team-or-github-handle
  reviewed_at: 2026-07-20
  scope: openspec:example-change#task
records:
- id: EXC-001
  kind: manual # manual | not-applicable | equivalent-mutant | quarantine
  verification_ids: [OS-AEC-001] # optional when the issue already identifies the governing task
  owner: team-or-github-handle
  issue: https://github.com/DavidHoenisch/remotr/issues/123
  reason: Why automated evidence is unavailable or unnecessary.
  expires: 2026-12-31
```

`reviewed_by`, `reviewed_at`, and `scope` are mandatory for the snapshot.
`owner`, a non-placeholder `issue`, `reason`, and `expires` are mandatory for
every record. `equivalent_selector` is mandatory for both a quarantine and an
equivalent mutant, and recommended whenever a manual or not-applicable
disposition still has a release-blocking substitute. Expired records are
invalid; renewals require a new review rather than changing the date silently.

An exception is disposition metadata, never passing evidence. Provider-matrix
selectors remain restricted to executable `make:` and `go-test:` selectors,
and a qualification row in the `not-applicable` TDD phase must remain
unadvertised.
