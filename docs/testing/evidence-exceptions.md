# Reviewed evidence exceptions

An intentionally manual, not-applicable, equivalent-mutant, or quarantined
evidence item must have a reviewed YAML record in `test/evidence-exceptions.yaml`.
Each record uses this shape:

```yaml
- id: EXC-001
  kind: manual # manual | not-applicable | equivalent-mutant | quarantine
  verification_ids: [OS-AEC-001]
  owner: team-or-github-handle
  issue: https://github.com/DavidHoenisch/remotr/issues/123
  reason: Why automated evidence is unavailable or unnecessary.
  equivalent_selector: go-test:./internal/example:^TestEquivalent$
  expires: 2026-12-31
```

`owner`, `issue`, `reason`, and `expires` are mandatory for every record.
`equivalent_selector` is mandatory for a quarantine and recommended whenever a
manual or not-applicable disposition still has a release-blocking substitute.
Expired records are invalid; renewals require a new review rather than changing
the date silently.
