# Traceability manifest

`test/traceability.yaml` is a version-1 manifest validated against
`test/traceability.schema.json`. It maps each immutable OpenSpec verification
ID to its canonical change/capability, lifecycle disposition, verification
classes, selectors, and required environments.

`verified` entries require concrete selectors and environments. `planned`,
`deferred`, `not-applicable`, and `removed` entries require a durable
disposition reason and are not passing evidence for advertised behavior. A
scenario may list multiple selectors and verification classes when its evidence
must compose across layers.
