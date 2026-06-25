# Git sync workflow

Desired state changes never go through the operator CLI. Operators edit the configuration repository in Git; the server advances the **release ref** when Git sync runs; agents pick up new artifact digests on their next sync.

## Publish configuration changes

1. Edit `fleets/<fleet>/desired.yaml` (or an endpoint override) in the configuration repository.
2. Open a pull request; review in Git as usual.
3. Merge to the tracked branch (for example `main`).
4. Git sync advances the **release ref** on the server (webhook or poll).
5. Agents pick up the new artifact digest on the next sync.

See [Configuration repository](configuration-repository.md) for layout and override semantics.

## Git sync and release ref

The server serves artifacts from a checkout at `REMOTR_CONFIG_REPO`. The **release ref** is the Git commit SHA agents receive with each artifact.

Configure Git sync on the server:

| Variable | Purpose |
|----------|---------|
| `REMOTR_GIT_REMOTE_URL` | Remote to fetch (HTTPS URL without embedded credentials) |
| `REMOTR_GIT_TOKEN` | GitHub/Git HTTPS PAT for private repos |
| `REMOTR_GIT_USERNAME` | HTTPS username (default `x-access-token` for GitHub) |
| `REMOTR_GIT_BRANCH` | Branch to track (default `main`) |
| `REMOTR_GIT_SYNC_POLL_INTERVAL` | Periodic fetch (for example `5m`); `0` disables polling |
| `REMOTR_GIT_WEBHOOK_SECRET` | Shared secret for `X-Remotr-Git-Webhook-Secret` header |

Webhook endpoints:

- `/v1/webhooks/git` — for GitHub/forge hooks (requires `X-Remotr-Git-Webhook-Secret` when configured)
- `/v1/admin/git-sync` — operator mTLS (use `remotr git sync`)

Example forge hook (GitHub webhook):

```bash
curl -X POST https://remotr.example:8443/v1/webhooks/git \
  -H "X-Remotr-Git-Webhook-Secret: $SECRET"
```

Trigger sync manually as an operator:

```bash
remotr git sync
```

![remotr git sync](../assets/demo/git-sync.gif)

### Private GitHub repositories

Set a **read-only** GitHub PAT on the server (Fly secret, systemd env, etc.):

```bash
fly secrets set \
  REMOTR_GIT_REMOTE_URL=https://github.com/your-org/remotr-config.git \
  REMOTR_GIT_TOKEN=ghp_xxxxxxxx \
  REMOTR_GIT_BRANCH=main \
  -a your-app
```

On first sync the server clones (or replaces a bundled starter checkout) from the private remote. The PAT is passed via Git `http.extraHeader` and is **not** written to `.git/config`.

Use a fine-grained PAT or classic token with **Contents: read** on the config repo only.

If the config repo is not a Git checkout (plain directory mount), set `REMOTR_RELEASE_REF` to a static label; the server will not advance ref automatically.

## Related

- [Architecture — release ref](../explanation/architecture.md#release-ref)
- [Terminology — Git sync](../explanation/terminology.md#language)
- [Production deployment — webhook setup](production-deployment.md#6-git-sync-webhook)
