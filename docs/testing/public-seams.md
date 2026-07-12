# Public test seams

Tests verify behavior at one declared public seam. A test may use fakes only at
an operating-system, clock, randomness, network, persistence, or external
service boundary; owned Remotr modules should normally compose for real.

| Seam | Good evidence | Implementation-coupled evidence to avoid |
| --- | --- | --- |
| Configuration authoring | `remotr config validate`, render, or discover output | Calling an unexported parser helper |
| Operator administration | CLI output or authenticated Admin API response | Inspecting private client fields |
| Agent/server protocol | Authenticated Sync request, response, and state transition | Reading Postgres to bypass the API |
| Agent execution | Composed artifact plus controlled host boundary to Check/Apply result | Asserting engine collaborator call order |
| Provider contract | Declared intent to observed sandboxed/real host state | Provider private helper expectations |
| System safety | Complete recovery across network, reboot, storage, or access boundary | Command construction alone as recovery proof |
| Performance | Observable request/cycle cost and bounded resources | Benchmarking an unexported implementation detail |

Exact argv assertions remain appropriate at the external-process boundary when
they prove shell avoidance, argument separation, noninteractive behavior, or
the absence of an unsafe flag.
