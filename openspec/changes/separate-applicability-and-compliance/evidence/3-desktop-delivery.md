# Independent desktop delivery evidence

## Red

- `EndpointTable` was given an Endpoint whose active Release was `release-42`, whose target was `release-43`, and whose target was capability-blocked. The table had no Delivery column and could only render `Compliant`.
- `EndpointInvestigation` received the same independent compliance and delivery fields. Its summary rendered `Compliant` but omitted `Capability blocked`.

These intended failures established the endpoint inventory and detail views as the public desktop seams for OS-DFV-036. OS-DFV-037 is composed with the authenticated State-report regression because historical apply failures are deliberately excluded from the safe desktop row model; the desktop must render the canonical current compliance supplied by that API.

## Green

Inventory and detail now render an independent Delivery status from target, offered, active, capability-blocked, and unmanaged evidence. A compliant Endpoint blocked from a newer Release simultaneously displays `Compliant` and `Capability blocked`, with active and target Release refs kept distinct. Table-driven status tests cover unmanaged, capability-blocked, offered, current, and not-reported precedence.

The focused three-file frontend run passed 13 tests. The complete frontend run passed 80 tests, followed by clean lint, type-check, and production build checks.
