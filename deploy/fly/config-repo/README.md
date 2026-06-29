# Fly bootstrap configuration repository

Bundled at `/config-repo` in the Fly `remotr-server` image. Same kind-tagged layout as `remotr init`:

- `modules/` — `kind: module` configuration slices
- `fleets/default/manifest.yaml` — `kind: manifest` fleet entry point
- `applications/`, `crons/` — optional shared sources

The server composes deployable artifacts when git sync advances the release ref. Preview locally with `remotr config render --fleet default`.

Replace this tree with your own GitOps repository by setting `REMOTR_GIT_REMOTE_URL` on Fly and redeploying. See [Configuration repository](https://davidhoenisch.github.io/remotr/guides/configuration-repository/).
