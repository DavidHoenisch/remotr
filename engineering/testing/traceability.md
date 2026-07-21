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

## Ubuntu 24.04 qualification coherence

Ubuntu applicator support advances only as an exact row across three checked-in
sources: `test/qualification/ubuntu-2404-applicators.yaml`,
`test/provider-matrix.yaml`, and this traceability manifest. A `qualified` row
must have one matching `passing` matrix row, every matrix selector must be part
of the row's recorded evidence, and every governing verification ID must be
`verified`. An `unadvertised` row must have no passing matrix evidence.

`TestQualifiedRowsStayCoherentAcrossReleaseEvidence` enforces those joins and
also rejects broad Ubuntu family rows. Consequently, the verified lifecycle of
OS-AEC-093, OS-AEC-094, OS-AEC-097, OS-AEC-098, OS-AEC-099, or OS-AEC-102 is
evidence that the selective gate works; it is not permission to infer support
for a sibling or future-roadmap backend.

`make ubuntu-2404-applicator-qualification-audit` is the evidence-derived exit
audit for OS-AEC-101 and OS-AEC-103. It reports every exact qualified and
explicitly descoped row, retains blocked/planned/missing/skipped/failing/
untested rows in both milestone and umbrella decisions, requires all M1-M5
inventories, and checks the provider-matrix plus completed task state of the
four sibling workstreams before reporting archive eligibility.
