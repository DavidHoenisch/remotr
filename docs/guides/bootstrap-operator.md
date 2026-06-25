# Bootstrap the operator

When the server starts with Postgres and no registered operators, it generates a **bootstrap token**.

## Steps

1. Read the token from server logs or the bootstrap file (default `/var/lib/remotr/bootstrap.token` on the server host).
2. Exchange it:

```bash
remotr bootstrap \
  --server-url https://remotr.example:8443 \
  --ca /etc/remotr/ca.crt \
  --token "$BOOTSTRAP_TOKEN" \
  --state-dir ~/.config/remotr
```

![remotr bootstrap](../assets/demo/bootstrap.gif)

3. Confirm credentials exist:

```bash
ls ~/.config/remotr/
# operator.crt  operator.key  ca.crt  state.json
```

The bootstrap token and file are invalidated after a successful exchange. Issue additional operator credentials with `remotr admin credential stamp` (see [RBAC](rbac.md#use-stamped-credentials-on-a-new-computer)).

## Fly.io quick path

If you used the [Fly.io bootstrap](fly-io.md), the installer runs bootstrap for you and writes credentials locally.

## Next steps

- [Create enrollment tokens](enrollment-tokens.md)
- [Register fleets in Postgres](enrollment-tokens.md#register-a-fleet-before-enrolling) before enrolling endpoints
- [Production hardening](production-deployment.md#8-hardening-checklist)
