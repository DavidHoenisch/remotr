# Fly bootstrap configuration repository

Bundled at `/config-repo` in the Fly `remotr-server` image. Same modular layout as `remotr init`:

- `modules/` — reusable configuration slices
- `fleets/default/manifest.yaml` — lists modules for the default fleet
- `fleets/default/desired.yaml` — generated deployable artifact (run `remotr config compose .` after edits)

Replace this tree with your own GitOps repository by setting `REMOTR_GIT_REMOTE_URL` on Fly and redeploying. See [Configuration repository](https://davidhoenisch.github.io/remotr/guides/configuration-repository/).
