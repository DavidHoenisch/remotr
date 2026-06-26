# Remotr Hub

Static community catalog for sharing Remotr manifests, cron jobs, and configuration snippets.

Published at **https://davidhoenisch.github.io/remotr/hub/** as part of the combined docs + Hub GitHub Pages site.

Documentation home: **https://davidhoenisch.github.io/remotr/**

Pushes to `master` that touch `docs/`, `hub/`, or related paths deploy automatically via `.github/workflows/pages.yml`.

**One-time setup:** In the repository go to **Settings → Pages → Build and deployment** and set Source to **GitHub Actions**. Without this, the deploy workflow fails at the Configure Pages step.

## Contribute

1. Add a YAML file under `hub/snippets/` (optional but recommended for previews and `remotr hub snippet import`).
2. Register the entry in `hub/data/catalog.json`:
   - `id` — unique slug
   - `title`, `description`, `category` (`manifests`, `crons`, `snippets`, or `repos`)
   - `tags`, `distros`, `author`
   - `snippetPath` — path relative to `hub/` (e.g. `snippets/my-job.yaml`)
   - `sourceUrl` — link to your repo or upstream source (optional)
   - `sourceCommit` — **required** when `sourceUrl` points at a third-party repository; full git commit hash (40 characters) that pins the linked content
   - `featured` — `true` to highlight on the grid (optional)
3. Open a pull request.

Operators import snippets into their config repo:

```bash
remotr hub snippet import <entry-id> -o modules/my-module.yaml
```

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

Validate catalog changes from the repository root:

```bash
go test -mod=vendor ./internal/hubcatalog/...
```
