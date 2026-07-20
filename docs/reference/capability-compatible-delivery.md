# Capability-compatible delivery protocol

Remotr agents that implement capability reporting send a capability document
on every authenticated `POST /v1/sync`. The endpoint certificate authenticates
the document: the protocol does not accept a separate bearer assertion for
capability evidence.

## Version 1 bounds

All limits apply before a document can be persisted or used for selection.

| Field or collection | Bound | Canonical form |
| --- | ---: | --- |
| Encoded document | 65,536 bytes | UTF-8 JSON body without the submitted digest |
| Artifact schemas | 8 entries | Unique ascending non-negative integers |
| Capabilities | 512 entries | Unique by capability ID, sorted by ID then revision |
| Normalized facts | 128 entries | Unique keys, sorted by key then value |
| Capability ID | 128 bytes | Lowercase namespaced identifier |
| Contract revision | 32 bytes | One to three dot-separated unsigned integers |
| Fact key | 64 bytes | Lowercase namespaced identifier |
| Fact value | 256 bytes | A normalized non-secret enum or stable identifier |
| Agent version | 128 bytes | Printable release identifier |
| Missing-requirement diagnostics | 32 entries | Sorted IDs/revisions; never fact values |

An empty capability list is valid and means the endpoint currently qualifies
no artifact or provider contract. It fails closed during artifact selection.

Capability and fact identifiers use lowercase ASCII segments beginning with a
letter and separated by `.`, `_`, `-`, `:`, or `/`. Contract revisions are
either `MAJOR`, `MAJOR.MINOR`, or `MAJOR.MINOR.PATCH`, or a stable registered
contract token such as `package-v1`; leading/trailing separators and whitespace
are rejected.

Facts are an allowlisted description of runtime provider selection, such as
distribution, architecture, package manager, init system, firewall, network,
security, desktop, and browser backends. Free-form environment values,
credentials, tokens, paths, command output, and other secret-bearing or
operator-authored values are not capability facts.

A discovered fact never authorizes a provider capability by itself. Resource
and provider entries are emitted only when an exact passing matrix row matches
the endpoint platform, contract revision, evidence environment, accepted
dependency gates, and any applicable runtime provider fact.

## Canonical document and digest

Document version 1 contains `documentVersion`, `artifactSchemaVersions`,
`capabilities`, `facts`, and `agentVersion`. The canonical body omits `digest`,
sorts all set-like collections as described above, and uses compact JSON with
the declared field order. Its digest is lowercase hexadecimal SHA-256 prefixed
with `sha256:`. Duplicate IDs, conflicting revisions, unsupported document
versions, non-normalized values, impossible schema declarations, and digest
mismatches fail closed.

The server persists the latest valid canonical document, digest, authenticated
endpoint identity, and server receive time for readiness and reporting. The
current authenticated request remains the only evidence used for artifact
selection; stored evidence is never substituted for an omitted or invalid
modern request document.

## Artifact requirements and delivery state

Each compiled artifact variant has a versioned requirement set covering its
schema version, resource capability IDs and revisions, and provider
requirements. Remotr composes only canonical schema 1 and a schema-0 variant
when conversion is behaviorally lossless. It never removes a resource or field
to manufacture an endpoint-specific variant.

Endpoint delivery reports separate target, offered, and active Release and
artifact state. An offer becomes active only when a later request acknowledges
successful processing of the exact digest. An incompatible existing endpoint
retains its active artifact; an incompatible new endpoint remains unmanaged.
Both receive the successful authenticated `capability_blocked` outcome with a
bounded list of missing requirements and the ordinary polling cadence.
