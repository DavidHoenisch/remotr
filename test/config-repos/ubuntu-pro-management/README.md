# Ubuntu Pro configuration fixture

This source-only configuration repository exercises the typed Ubuntu Pro
resource and its exact capability requirements through `remotr config
discover`, `validate`, and `render`.

It intentionally contains no generated `desired.yaml` or `crons.yaml` files.
The token, Landscape registration key, and Landscape CA values are logical
Remotr secret references, not credentials.
