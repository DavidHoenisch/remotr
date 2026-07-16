# Config Ergonomics Redesign — Implementation Plan

This plan implements the architectural changes described in ADR-004: server-side composition with self-describing config files.

## Summary of Changes

1. All config files require a `kind:` field (`manifest`, `module`, `application`, `crons`)
2. Server composes deployable artifacts at Release ref advance (not in Git/CI)
3. Compiled artifacts cached in Postgres
4. No `desired.yaml` or `crons.yaml` in Git repositories
5. `remotr config compose` replaced by `remotr config render` (preview only)
6. Flexible folder structure within fleet/endpoint directories

## Phase 1: Schema and Validation

### 1.1 Define Kind Types

File: `internal/types/kind.go` (new file)

```go
type Kind string

const (
    KindManifest    Kind = "manifest"
    KindModule      Kind = "module"
    KindApplication Kind = "application"
    KindCrons       Kind = "crons"
)
```

### 1.2 Update YAML Structures

**Manifest** (`internal/configcompose/manifest.go`):
- Add `Kind Kind `yaml:"kind"`` field
- Validation: `kind: manifest` required
- Keep: `extends`, `modules`, `applications`, `crons`, `overrides`
- `applications` and `crons` are now direct references (paths/folders), not separate manifest files

**Module** (update `internal/models/models.go` or new file):
- Add `Kind Kind `yaml:"kind"`` field to State struct
- Validation: `kind: module` required
- Keep: `configurations` list

**Application** (`internal/configcompose/application_manifest.go`):
- Update application module parsing to require `kind: application`
- Single app definition with `name`, `packageManager`, `present`, etc.

**Crons** (update `internal/configcompose/cron_manifest.go`):
- Add `Kind Kind `yaml:"kind"`` field
- Validation: `kind: crons` required
- Contains cron definitions directly

### 1.3 Update Validation

File: `internal/configrepo/validate.go`

- Add `validateKind()` function that checks for required `kind:` field
- Update `validateDesiredFile()` → rename to `validateModuleFile()` 
- Add `validateManifestFile()`, `validateApplicationFile()`, `validateCronsFile()`
- All validation functions should reject files missing `kind:`
- Error messages should be clear: "file missing required 'kind' field"

### 1.4 Update File Discovery

File: `internal/configcompose/discover.go` (new file)

Create a discovery system that:
1. Walks a directory tree
2. Reads each `.yaml` file
3. Parses just the `kind:` field first
4. Returns files grouped by kind

```go
type DiscoveredFiles struct {
    Manifests    []string // paths to kind: manifest files
    Modules      []string // paths to kind: module files
    Applications []string // paths to kind: application files
    Crons        []string // paths to kind: crons files
    Unknown      []string // files missing kind or with unknown kind
}

func DiscoverFiles(root string) (DiscoveredFiles, error)
```

## Phase 2: Database Changes

### 2.1 Add Compiled Artifacts Table

File: `internal/store/postgres/migrations/` (new migration)

```sql
CREATE TABLE compiled_artifacts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    fleet_name      TEXT,
    endpoint_id     UUID REFERENCES endpoints(id),
    release_ref     TEXT NOT NULL,
    artifact_type   TEXT NOT NULL CHECK (artifact_type IN ('desired', 'crons')),
    artifact        BYTEA NOT NULL,
    digest          TEXT NOT NULL,
    compiled_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT fleet_or_endpoint CHECK (
        (fleet_name IS NOT NULL AND endpoint_id IS NULL) OR
        (fleet_name IS NULL AND endpoint_id IS NOT NULL)
    ),
    UNIQUE (fleet_name, release_ref, artifact_type) WHERE fleet_name IS NOT NULL,
    UNIQUE (endpoint_id, release_ref, artifact_type) WHERE endpoint_id IS NOT NULL
);

CREATE INDEX idx_compiled_artifacts_fleet ON compiled_artifacts(fleet_name, release_ref) WHERE fleet_name IS NOT NULL;
CREATE INDEX idx_compiled_artifacts_endpoint ON compiled_artifacts(endpoint_id, release_ref) WHERE endpoint_id IS NOT NULL;
CREATE INDEX idx_compiled_artifacts_compiled_at ON compiled_artifacts(compiled_at);
```

### 2.2 Add SQLC Queries

File: `internal/store/postgres/queries/compiled_artifacts.sql` (new file)

```sql
-- name: UpsertCompiledArtifact :one
INSERT INTO compiled_artifacts (fleet_name, endpoint_id, release_ref, artifact_type, artifact, digest, compiled_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
ON CONFLICT ... 
DO UPDATE SET artifact = $5, digest = $6, compiled_at = NOW()
RETURNING *;

-- name: GetCompiledArtifactForFleet :one
SELECT * FROM compiled_artifacts
WHERE fleet_name = $1 AND release_ref = $2 AND artifact_type = $3;

-- name: GetCompiledArtifactForEndpoint :one
SELECT * FROM compiled_artifacts
WHERE endpoint_id = $1 AND release_ref = $2 AND artifact_type = $3;

-- name: PruneOldArtifacts :exec
DELETE FROM compiled_artifacts
WHERE compiled_at < $1;
```

### 2.3 Update Store Interface

File: `internal/registry/registry.go`

Add methods:
- `StoreCompiledArtifact(ctx, fleetOrEndpoint, releaseRef, artifactType, artifact, digest) error`
- `GetCompiledArtifact(ctx, fleetOrEndpoint, releaseRef, artifactType) ([]byte, string, error)`
- `PruneOldArtifacts(ctx, olderThan time.Time) error`

## Phase 3: Server-Side Composition

### 3.1 Create Composition Service

File: `internal/server/composition.go` (new file)

```go
type CompositionService struct {
    registry Registry
    repoRoot string
}

// ComposeFleet composes the deployable artifact for a fleet at a given release ref
func (s *CompositionService) ComposeFleet(ctx context.Context, fleetName, releaseRef string) error {
    // 1. Find the manifest in fleets/<fleetName>/
    // 2. Discover files by kind
    // 3. Validate exactly one manifest exists
    // 4. Resolve extends chain
    // 5. Load all referenced modules (explicit paths + folder refs)
    // 6. Load all referenced applications
    // 7. Load all referenced crons
    // 8. Merge configurations, apply overrides
    // 9. Marshal to YAML
    // 10. Compute digest
    // 11. Store in compiled_artifacts table
}

// ComposeEndpoint composes the deployable artifact for an endpoint override
func (s *CompositionService) ComposeEndpoint(ctx context.Context, endpointID, releaseRef string) error

// ComposeAll composes all fleets and endpoint overrides for a release
func (s *CompositionService) ComposeAll(ctx context.Context, releaseRef string) error
```

### 3.2 Update Git Sync Handler

File: `internal/gitsync/sync.go`

When Release ref advances:
1. After successful git fetch/checkout
2. Call `compositionService.ComposeAll(ctx, newReleaseRef)`
3. Log any composition errors (don't block release, but record issues)
4. Consider: should composition failure block the release ref advance?

### 3.3 Update Sync Handler

File: `internal/server/sync.go`

Update the endpoint sync handler:
1. Determine if endpoint has override path → use endpoint artifact
2. Otherwise use fleet artifact
3. Fetch from `compiled_artifacts` table instead of reading Git files
4. Return cached artifact + digest

```go
func (s *Server) handleSync(ctx context.Context, endpointID uuid.UUID) (*SyncResponse, error) {
    endpoint, err := s.registry.GetEndpoint(ctx, endpointID)
    // ...
    
    // Check for endpoint override first
    artifact, digest, err := s.registry.GetCompiledArtifact(ctx, endpointID, s.releaseRef, "desired")
    if err == ErrNotFound {
        // Fall back to fleet artifact
        artifact, digest, err = s.registry.GetCompiledArtifact(ctx, endpoint.FleetName, s.releaseRef, "desired")
    }
    // ...
}
```

## Phase 4: CLI Changes

### 4.1 Replace `config compose` with `config render`

File: `cmd/remotr/cmd_config.go`

Remove or deprecate `config compose` subcommand.

Add `config render` subcommand:
- `--fleet <name>` — render a specific fleet
- `--endpoint <id>` — render a specific endpoint  
- `--ref <sha>` — render at a specific git ref (default: HEAD)
- `--output <path>` — write to file instead of stdout
- `--format yaml|json` — output format

```go
func configRenderCmd() *cobra.Command {
    // 1. Run composition logic locally (same code as server)
    // 2. Output to stdout or file
    // 3. Do NOT write to repo or cache
}
```

### 4.2 Update `config validate`

File: `cmd/remotr/cmd_config.go`

Update validation to:
1. Check for `kind:` field in all config files
2. Validate manifest references resolve
3. Check for exactly one manifest per fleet/endpoint
4. Run composition as validation (catch merge errors)

### 4.3 Add `config discover`

File: `cmd/remotr/cmd_config.go` (optional, useful for debugging)

New subcommand that shows discovered files:

```
$ remotr config discover --fleet marketing
Manifest: fleets/marketing/fleet.yaml (kind: manifest)
Modules:
  - fleets/marketing/security/hardening.yaml (kind: module)
  - modules/base-packages.yaml (kind: module)
Applications:
  - applications/slack.yaml (kind: application)
  - fleets/marketing/apps/analytics.yaml (kind: application)
Crons:
  - fleets/marketing/jobs/cleanup.yaml (kind: crons)
```

## Phase 5: Update Composition Logic

### 5.1 Refactor `internal/configcompose/`

Current files to update:
- `compose.go` — main entry point, update to use kind-based discovery
- `manifest.go` — add kind validation, update parsing
- `application_manifest.go` — merge into manifest, applications are now a section
- `application_resolve.go` — update to work with kind-based discovery
- `cron_manifest.go` — merge into manifest, crons are now a section

New files:
- `discover.go` — kind-based file discovery
- `resolve.go` — resolve module/application/cron references (paths + folders)

### 5.2 Update Reference Resolution

Module references can be:
- Explicit path: `./security/hardening.yaml`
- Folder reference: `./security/` (include all `kind: module` files recursively)
- Relative to repo root: `../../modules/shared/`

```go
func resolveModuleRefs(repoRoot, manifestDir string, refs []string) ([]string, error) {
    var resolved []string
    for _, ref := range refs {
        path := resolveRelativePath(manifestDir, ref)
        if isDirectory(path) {
            // Discover all kind: module files in directory
            files, err := discoverFilesOfKind(path, KindModule)
            resolved = append(resolved, files...)
        } else {
            // Single file reference
            resolved = append(resolved, path)
        }
    }
    return resolved, nil
}
```

### 5.3 Merge Configuration Logic

Keep existing merge logic in `manifest.go`:
- `mergeConfigurations()` — merge module configs
- `mergeConfiguration()` — apply overrides to a single config

Update to handle the new structure where applications and crons come from separate sections.

## Phase 6: Testing

### 6.1 Unit Tests

Update/add tests for:
- `internal/configcompose/` — all composition logic
- `internal/configrepo/validate.go` — kind validation
- `internal/store/postgres/` — compiled artifacts CRUD

### 6.2 Integration Tests

- Server composes on release ref advance
- Sync returns cached artifact
- Endpoint override takes precedence over fleet
- Composition errors are handled gracefully

### 6.3 E2E Tests

Update `test/e2e/`:
- Remove references to `desired.yaml` in Git
- Update test config repos to use `kind:` fields
- Test full flow: push → compose → sync → apply

## Phase 7: Migration and Cleanup

### 7.1 Update Test Fixtures

- `compose/config-repo/` — add `kind:` fields, remove `desired.yaml`
- `deploy/fly/config-repo/` — add `kind:` fields, remove `desired.yaml`

### 7.2 Remove Deprecated Code

- Remove `remotr config compose` command (or keep as alias to `render`)
- Remove `discoverManifests()` that looks for `manifest.yaml` by name
- Remove `desiredPathForManifest()` that generates `desired.yaml` paths
- Remove any code that writes `desired.yaml` to disk

### 7.3 Update Documentation

- Update `AGENTS.md` with new config workflow
- Update any README or docs referencing `desired.yaml`
- Add examples of new config format with `kind:` fields

## Implementation Order

Recommended sequence to minimize breaking changes during development:

1. **Phase 1.1-1.2**: Add `kind:` field support (backwards compatible — field is optional initially)
2. **Phase 2**: Database changes (no runtime impact)
3. **Phase 5**: Refactor composition to support both old and new formats
4. **Phase 3**: Server-side composition (behind feature flag initially)
5. **Phase 4**: CLI changes
6. **Phase 1.3-1.4**: Make `kind:` required, add discovery
7. **Phase 6**: Testing
8. **Phase 7**: Migration and cleanup

## Example Config Files

### Manifest (`kind: manifest`)

```yaml
kind: manifest
extends: ../../base-fleet/manifest.yaml
modules:
  - ./security/           # folder reference
  - ./packages/tools.yaml # explicit path
  - ../../modules/compliance/
applications:
  - slack                 # basename lookup in applications/
  - ./apps/analytics.yaml # fleet-local app
crons:
  - ./jobs/               # folder reference
overrides:
  - name: screen-lock
    files:
      - name: timeout
        content: "TIMEOUT=600"
```

### Module (`kind: module`)

```yaml
kind: module
configurations:
  - name: screen-lock
    description: Enforce screen lock policy
    targetDistros:
      - Debian
      - Arch
    packages:
      - name: xautolock
        present: true
        packageManager: apt
    files:
      - name: autolock-config
        path: /etc/xautolock.conf
        content: |
          TIMEOUT=300
```

### Application (`kind: application`)

```yaml
kind: application
name: slack
present: true
packageManager: flatpak
flatpakRemote: flathub
```

### Crons (`kind: crons`)

```yaml
kind: crons
crons:
  - name: weekly-update
    schedule: "0 2 * * 0"
    command: ["/usr/bin/apt", "update"]
  - name: cleanup-tmp
    schedule: "0 3 * * *"
    command: ["/usr/bin/find", "/tmp", "-mtime", "+7", "-delete"]
```

## Open Questions

1. **Composition failure handling**: Should a composition error block Release ref advance, or just log and continue?
2. **Artifact pruning**: How many old release refs to keep? Time-based or count-based?
3. **Cron artifact**: Separate `crons` artifact type, or merge into main `desired` artifact?
4. **Validation strictness**: Warn or error on unknown fields in config files?
