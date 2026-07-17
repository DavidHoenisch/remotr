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

The advertisement gate evaluates the evidence union for the selected
change/capability and runs every selector. A new field or provider is blocked
unless verified entries collectively include unit, contract, acceptance,
container, VM safety, fuzz, mutation, performance, and reviewed manual
documentation evidence. These classes cover the required schema, validation,
composition, provider, engine, telemetry, traceability, migration, integration,
safety, and release checks without pretending one test proves every layer.
