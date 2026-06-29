# Server-side composition with self-describing config files

We moved configuration composition from Git/CI to the server, eliminated generated `desired.yaml` files from the repository, and introduced self-describing config files via a `kind:` field.

## Context

The previous model required authors to run `remotr config compose` to generate `desired.yaml` from manifests and modules before committing. This created confusion:

1. **Ambiguous authoring paths** — you could edit `desired.yaml` directly or use the composition system, with no clear guidance on when to use which
2. **Magic file names** — the system inferred file type from names (`manifest.yaml`, `applications.manifest.yaml`) rather than content, requiring rigid folder structures within fleets
3. **Generated files in Git** — `desired.yaml` looked editable but was meant to be generated, confusing users coming from Ansible/Helm

## Decision

**Self-describing files:** All config files have a required `kind:` field (`manifest`, `module`, `application`, `crons`). The tool determines file type from content, not path. This enables flexible folder structures within fleet/endpoint directories while keeping prescribed top-level folders (`fleets/`, `endpoints/`, `applications/`, `modules/`).

**Server-side composition:** The server composes deployable artifacts when the Release ref advances (git push), caching results in Postgres keyed by `(fleet_or_endpoint, release_ref)`. No generated files in Git — the repository contains only source manifests and modules. `remotr config render` provides a CLI dry-run preview.

**One manifest per fleet/endpoint:** Each fleet or endpoint folder must contain exactly one file with `kind: manifest` as its entry point. Manifests reference modules via explicit paths or folder references (no globs), support single inheritance via `extends`, and allow `overrides` at the manifest level only.

## Consequences

- Breaking change to config format — existing repos must migrate (no backwards compatibility period)
- Server becomes stateful for compiled artifacts (already uses Postgres for registry)
- Simpler authoring experience — matches Helm mental model
- Git history shows intent (sources) not generated output
- `remotr config compose` command removed; replaced by `remotr config render` for preview
