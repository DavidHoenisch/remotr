# Validate configuration

Check configuration repository YAML locally before merge. No server connection required.

```bash
remotr config validate ./remotr-config
remotr config validate --json
remotr config validate . --skip-render-check   # kinds and schema only
```

Use `config validate` in CI (see `.github/workflows/config-repo.yml`). Run `remotr config render --fleet <name>` to preview composed output after editing modules or manifests.

![remotr config validate](../assets/demo/config-validate.gif)

Reports invalid kinds, unresolved references, composition errors, schema issues, and convention problems.

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

- [Manifest format reference](../reference/manifest-format.md)
- [Configuration format reference](../reference/configuration-format.md)
- [Operator overview — CLI layout](operator-overview.md#cli-layout)
