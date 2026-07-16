# Public test seams

Tests verify behavior at one declared public seam. A test may use fakes only at
an operating-system, clock, randomness, network, persistence, or external
service boundary; owned Remotr modules should normally compose for real.

| Seam | Good evidence | Implementation-coupled evidence to avoid |
| --- | --- | --- |
| Configuration authoring | `remotr config validate`, render, or discover output | Calling an unexported parser helper |
| Operator administration | CLI output or authenticated Admin API response | Inspecting private client fields |
| Desktop user/Wails bridge | A complete user workflow rendered against the generated typed binding contract with an injected deterministic bridge fixture | Calling component hooks directly, asserting private frontend state, or mocking Remotr-owned application-service collaborators |
| Agent/server protocol | Authenticated Sync request, response, and state transition | Reading Postgres to bypass the API |
| Agent execution | Composed artifact plus controlled host boundary to Check/Apply result | Asserting engine collaborator call order |
| Provider contract | Declared intent to observed sandboxed/real host state | Provider private helper expectations |
| System safety | Complete recovery across network, reboot, storage, or access boundary | Command construction alone as recovery proof |
| Performance | Observable request/cycle cost and bounded resources | Benchmarking an unexported implementation detail |

The desktop user/Wails bridge seam begins at an operator-visible action and ends
at rendered, accessible state or a purpose-specific generated Wails binding.
Component and browser-mode tests may replace that native boundary with an
injectable deterministic fixture, provided the fixture implements the same
typed binding contract and the test exercises the complete user workflow.
Tests should assert visible labels, semantic roles, focus, confirmation,
classified errors, and retained context rather than component internals or
calls between Remotr-owned frontend modules.

This seam complements rather than replaces the authenticated Admin API seam.
It proves desktop presentation and interaction behavior; backend claims about
mTLS identity, RBAC, server state transitions, audit records, or secret handling
must compose the real desktop application service and Admin client against a
controlled network/TLS boundary or the authenticated in-process Admin API.
Likewise, browser-rendered evidence does not prove Wails asset embedding,
remote-navigation policy, native window behavior, or Linux packaging. Those
claims require the focused bridge-security, production-build, or native Linux
smoke evidence selected by their risk.

Exact argv assertions remain appropriate at the external-process boundary when
they prove shell avoidance, argument separation, noninteractive behavior, or
the absence of an unsafe flag.
