## Why

Remotr has broad M1–M5 applicator code, but the exit audit found no milestone release-complete: checked-in configuration repositories do not compose the non-package baseline, Ubuntu 24.04 provider rows are missing or untested, and high-risk resources lack the release-specific VM evidence needed for an honest support claim. The umbrella needs one bounded qualification change that turns existing Ubuntu behavior into traceable, real-environment evidence without expanding into the future CMMC/Hub feature roadmap.

## What Changes

- Define Ubuntu 24.04 amd64 as the first qualification target for existing non-package M1–M5 applicators, with provider/backend rows granular enough that one passing resource family cannot imply support for another.
- Add a checked-in schema-1 Ubuntu M1–M5 configuration repository that validates, discovers, and renders representative existing resource kinds through the public operator CLI without committing generated artifacts or enabling high-risk changes by default.
- Audit every accepted field and provider represented by that baseline against Check, Apply, second Check, absence, unsupported, failure, activation, redaction, rollback, and preservation requirements selected by risk.
- Run ordinary provider behavior in pinned Ubuntu 24.04 containers and run access, connectivity, boot, storage, firewall, authentication, and other destructive-safety behavior in an Ubuntu 24.04 VM with explicit recovery evidence.
- Promote only exact provider/distribution/release/architecture/backend/contract rows whose complete selectors pass; leave missing, partial, or planned rows unadvertised and keep the gap report explicit.
- Re-run the M1–M5 composed-repository, provider-matrix, traceability, documentation, safety, and release audits after the execution-contract, capability-delivery, package-provider, and testing-foundation dependencies are accepted.
- Close umbrella qualification and archive gates only when the audit can distinguish implemented, composed, qualified, and deferred behavior with no inferred support claim.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `applicator-execution-contract`: Add a release-specific Ubuntu 24.04 qualification contract tying checked-in composition, provider evidence, risk-selected VM recovery, traceability, and capability advertisement to every support claim.

## Impact

- Checked-in configuration fixtures, public `config validate`/`discover`/`render` acceptance coverage, and M1–M5 audit documentation.
- Provider matrix schema/rows/selectors, capability advertisement, traceability, evidence exceptions, and release validation.
- Ubuntu 24.04 provider containers plus Vagrant/libvirt safety and recovery fixtures for high-risk providers.
- Existing non-package M1–M5 applicators and their tests when qualification exposes a real behavior gap; each correction remains a focused TDD slice governed by the existing capability requirement.
- Package and repository implementation remains owned by `complete-core-package-providers`; new CMMC/Hub capabilities remain in the future roadmap.
- This child remains linked to the active umbrella and is not archived ahead of the umbrella capability baseline.
