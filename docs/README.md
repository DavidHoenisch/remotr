# Remotr documentation

Published documentation: **https://davidhoenisch.github.io/remotr/**

Community snippet catalog: **https://davidhoenisch.github.io/remotr/hub/**

## Local preview

```bash
pip install -r requirements-docs.txt
make docs-serve
```

Open http://127.0.0.1:8000 — Hub catalog at http://127.0.0.1:8000/hub/ after `make docs-build`.

## Build the static site

```bash
make docs-build
```

Output: `site/` (MkDocs docs at root, Hub at `site/hub/`).

## Source layout

| Path | Purpose |
|------|---------|
| `docs/index.md` | Documentation home (production-first paths) |
| `docs/tutorial/` | Learning-oriented walkthroughs |
| `docs/guides/` | Task-focused how-to guides |
| `docs/reference/` | Lookup reference |
| `docs/explanation/` | Architecture and terminology |
| `docs/runbooks/` | Production maintenance |
| `docs/adr/` | Architecture decision records |
| `docs/contributing/checklist.md` | Symlink to `CHECKLIST.md` |
| `docs/explanation/terminology.md` | Symlink to `CONTEXT.md` |
| `docs/guides/fly-io.md` | Symlink to `deploy/fly/README.md` |
| `docs/assets/demo/` | Symlink to `demo/assets/` (CLI terminal recordings) |

Markdown in `docs/` is the source for the published site. Edit there (or via symlinked files) and push to `master` to deploy.
