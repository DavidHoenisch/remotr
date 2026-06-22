# Remotr Hub

Static community catalog for sharing Remotr manifests, cron jobs, and configuration snippets.

Published via GitHub Pages from the `hub/` directory. Pushes to `master` that touch `hub/` deploy automatically.

**One-time setup:** In the repository go to **Settings → Pages → Build and deployment** and set Source to **GitHub Actions**. Without this, the deploy workflow fails at the Configure Pages step.

Live site: **https://davidhoenisch.github.io/remotr/**

## Contribute

1. Add a YAML file under `hub/snippets/` (optional but recommended for copy-paste previews).
2. Register the entry in `hub/data/catalog.json`:
   - `id` — unique slug
   - `title`, `description`, `category` (`manifests`, `crons`, `snippets`, or `repos`)
   - `tags`, `distros`, `author`
   - `snippetPath` — path relative to `hub/` (e.g. `snippets/my-job.yaml`)
   - `sourceUrl` — link to your repo or upstream source (optional)
   - `sourceCommit` — **required** when `sourceUrl` points at a third-party repository; full git commit hash (40 characters) that pins the linked content
   - `featured` — `true` to highlight on the grid (optional)
3. Open a pull request.

For full configuration repositories, use category `repos` and point `sourceUrl` at the GitHub repo with a matching `sourceCommit`. Branch names such as `main` or `master` are not accepted for third-party entries — always pin to an immutable commit. Links to files in this repository may omit `sourceCommit`, but pinning is still recommended.

## Local preview

From the repository root:

```bash
cd hub && python -m http.server 8080
```

Open http://localhost:8080 — the catalog loads `data/catalog.json` and snippet files over HTTP.

Validate catalog changes from the repository root:

```bash
go test -mod=vendor ./internal/hubcatalog/...
```
