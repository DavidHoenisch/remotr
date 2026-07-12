# Godog Pilot Review

## Decision

Accepted for the current public workflow boundary. The pilot remains limited to
the five implemented workflows listed below. Future M1–M5 acceptance features
must wait until their corresponding operator-visible behavior exists.

## Covered workflows

| Feature | Public seam | Evidence command |
| --- | --- | --- |
| Configuration authoring | `remotr config validate` and `remotr config render` | `go test -mod=vendor ./test/acceptance -run TestConfigurationAuthoringFeature -count=1` |
| Operator bootstrap | `remotr bootstrap` and `remotr endpoint list` | `make test-e2e` |
| Enrollment and Sync | `remotr enroll token create`, `remotr-agent enroll`, and mTLS `POST /v1/sync` | `make test-e2e` |
| Application package catalog | `remotr app list --json` | `make test-e2e` |
| Endpoint labels | authenticated `POST /v1/sync` and `remotr endpoint list --json` | `make test-e2e` |

## Review findings

- Godog remains isolated to `test/acceptance`; the Compose E2E suite uses the
  package's small `ScenarioSteps` facade and has no direct Godog dependency.
- The feature text is domain-readable and each scenario is directly tagged
  with `@os_OS-TQG-006`. Traceability lint rejects missing or inactive tags.
- Steps cross only public operator CLI, agent CLI, or Sync HTTP boundaries.
  The single stored-credential assertion verifies an explicit CLI result,
  rather than using persistence as the workflow's success shortcut.
- The only shared setup is ensuring an authenticated operator. It uses the
  one-time bootstrap workflow, which lets an individually tag-filtered E2E
  scenario run against a fresh Compose stack without relying on another
  scenario's order.
- The deterministic local feature completed in under one second. The four
  Compose scenarios completed in about one second after the stack was ready;
  image build and stack startup remain outside the ordinary in-process PR
  acceptance target.
- Failures report the Gherkin scenario and failing public step. Enrollment
  tokens use the CLI's `--out --quiet` handoff so test output does not expose
  a secret token.

## Expansion boundary

Do not add Gherkin for private algorithms, provider permutations, parser
matrices, or future rollout/rollback/secret/reboot workflows. Those remain in
focused Go tests until a cross-component, operator-visible, or safety-critical
public workflow is implemented and needs a readable executable example.
