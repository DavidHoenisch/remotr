# Remotr Hub

Static community catalog for sharing importable Remotr configuration modules,
server-dispatched cron sources, and full configuration repositories.

Published at **https://davidhoenisch.github.io/remotr/hub/** as part of the combined docs + Hub GitHub Pages site.

Documentation home: **https://davidhoenisch.github.io/remotr/**

Pushes to `master` that touch `docs/`, `hub/`, or related paths deploy automatically via `.github/workflows/pages.yml`.

**One-time setup:** In the repository go to **Settings → Pages → Build and deployment** and set Source to **GitHub Actions**. Without this, the deploy workflow fails at the Configure Pages step.

## Contribute

1. Add one focused YAML source under `hub/snippets/`. Use `kind: module` with
   `schemaVersion: 1`, or `kind: crons`; do not add deployable `desired.yaml`
   or `crons.yaml` artifacts and do not add partial YAML fragments.
2. Register the entry in `hub/data/catalog.json`:
   - `id` — unique slug
   - `title`, `description`, `category` (`modules`, `crons`, or `repos`)
   - `tags`, `distros`, `author`
   - `snippetPath` — path relative to `hub/` (e.g. `snippets/my-job.yaml`)
   - `sourceUrl` — link to your repo or upstream source (optional)
   - `sourceCommit` — **required** when `sourceUrl` points at a third-party repository; full git commit hash (40 characters) that pins the linked content
   - `featured` — `true` to highlight on the grid (optional)
3. Open a pull request.

Operators import entries into their configuration repository:

```bash
remotr hub snippet import <entry-id>
```

Modules default to `modules/<entry-id>.yaml`; cron sources default to
`crons/<entry-id>.yaml`. Use `--out` to choose another repository-relative
source path, then reference that path from the fleet manifest.

For full configuration repositories, use category `repos` and point `sourceUrl` at the GitHub repo with a matching `sourceCommit`. Branch names such as `main` or `master` are not accepted for third-party entries — always pin to an immutable commit. Links to files in this repository may omit `sourceCommit`, but pinning is still recommended.

## Local preview

From the repository root:

```bash
make docs-build
cd site/hub && python -m http.server 8080
```

Open http://localhost:8080 — the catalog loads `data/catalog.json` and snippet files over HTTP.

Full docs preview (includes Hub at `/hub/`):

```bash
pip install -r requirements-docs.txt
make docs-serve
```

Validate catalog structure, registration, import paths, and every snippet as a
real repository source from the repository root:

```bash
go test -mod=vendor ./internal/hubcatalog/...
```
