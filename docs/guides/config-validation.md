# Validate configuration

Check configuration repository YAML locally before merge. No server connection required.

```bash
remotr config validate ./remotr-config
remotr config validate --json
```

![remotr config validate](../assets/demo/config-validate.gif)

Reports schema and convention issues in fleet and endpoint artifacts.

See also [Configuration repository — validate before push](configuration-repository.md#validate-before-push).

## Certificate maintenance

See [CA rotation runbook](../runbooks/ca-rotation.md) for full CA rotation, endpoint re-enrollment, and operator cert replacement.

Quick endpoint cert refresh (CA unchanged):

```bash
remotr enroll token create --server-url ... --fleet engineering
# on endpoint:
remotr-agent enroll --token ... --force --server-url ... --ca ...
```

## Related

- [Configuration format reference](../reference/configuration-format.md)
- [Operator overview — CLI layout](operator-overview.md#cli-layout)
