# Remotr

Pull-based MDM for Linux: admins publish desired state through Git; the server syncs from a configuration repository and serves ready-to-apply artifacts to enrolled endpoints over mTLS.

## Language

### Source of truth

**Configuration repository**:
The Git repository where all desired state lives. Admins merge, branch, and review changes here (GitOps). What lands on the default branch (or a release ref) is already composed and safe to deploy—not merged again by the server.
_Avoid_: Config store, backend, database of policies

**Release ref**:
The Git commit SHA on the tracked branch (for example `main`) that the server currently serves to all fleets. One global ref for v1: when it advances, every endpoint's next sync can receive updated artifacts from that commit.
_Avoid_: Latest, HEAD (too vague in conversation)

**Git sync**:
How the server updates its view of the configuration repository: a forge webhook on push triggers an immediate fetch and advances **Release ref**; a periodic poll (for example every 5–15 minutes) runs as fallback if a webhook is missed.
_Avoid_: Pull, CI deploy

**Deployable artifact**:
A single file at a **Fleet path** or **Endpoint override path** (for example `desired.yaml`) that holds every **Configuration** slice assigned to that deployment target. One file per endpoint/fleet; composition across concerns happens inside the file, not by merging multiple Git paths on the server.
_Avoid_: Effective desired state (old server-merge meaning), bundle

**Fleet path**:
The canonical Git path for a fleet's deployable artifact: `fleets/<fleet-name>/desired.yaml`. Enrollment binds an **Endpoint** to a **Fleet**; the server serves the artifact at that **Fleet path** for the current **Release ref**.
_Avoid_: Config path, policy path

**Endpoint override path**:
Optional Git path `endpoints/<endpoint-id>/desired.yaml` for a single machine that must diverge from its fleet. When present, it replaces (not merges with) the fleet artifact for that endpoint only.
_Avoid_: Per-host playbook, snowflake config

**In-document targeting**:
Fields inside a deployable artifact (such as `targetDistro`, `targetArch`) that limit which stanzas apply on a given endpoint. Used when one **Fleet path** serves multiple distros or architectures without separate Git directories.
_Avoid_: Server-side filter, selector merge

**Artifact format**:
Deployable artifacts are authored and transmitted as YAML. The server serves the same bytes as the **Configuration repository** at the **Release ref** plus a digest; the agent parses YAML locally.
_Avoid_: JSON policy (except future optional API versions)

**Resolved desired state**:
The **Configuration** slices and stanzas that apply to this endpoint after the agent applies **In-document targeting** using local OS facts (for example `/etc/os-release`, `uname -m`). **Check** and **Apply** run against this, not the raw file.
_Avoid_: Filtered config, effective state (old server-merge term)

### Fleet and assignment

**Endpoint**:
A single enrolled Linux machine that runs the Remotr agent.
_Avoid_: Node, host, device (unless talking to operators who already say "device")

**Fleet**:
A named group of endpoints that share the same baseline configurations.
_Avoid_: Group, pool, cluster

**Label**:
A key-value tag an endpoint reports during **Sync** for inventory and admin UI (for example `site=berlin`, `owner=platform`). v1 does not use labels to select **Deployable artifact** paths; assignment is **Fleet** enrollment plus optional **Endpoint override path** only.
_Avoid_: Tag (acceptable in UI copy, but prefer Label in domain discussion), selector, routing label

**Configuration**:
One named slice inside a **Deployable artifact**—for example "screen lock after 5 minutes idle" or "install nmap." Each slice declares its own packages, files, and similar, and may use **In-document targeting**. The agent resolves targeting per slice, then **Check** / **Apply** across all slices in the file.
_Avoid_: Policy, playbook, manifest (use for the whole artifact only informally)

**Builtin resource**:
A **Resource** applied by a first-class applicator with structured fields and dedicated **Check** logic (packages, files, local users). Preferred for operations that will be common across fleets.
_Avoid_: Module, plugin, native (vague)

**Escape hatch resource**:
A **Resource** for operations not yet modeled as **Builtin resource** kinds—initially arbitrary commands and systemd actions. Used while discovering what deserves native support; expected to shrink as builtins cover most day-to-day ops.
_Avoid_: Raw exec, script resource, generic task

**Command resource**:
An **Escape hatch resource** with explicit `check`, `apply`, and optional `revert` steps (argv or script). **Drift** is detected only when `check` is defined and exits non-zero; **Apply** runs `apply` when check fails or apply is forced. Authors own idempotency.
_Avoid_: Shell resource, run-once task

**Systemd resource**:
A **Builtin resource** (v1) with structured fields such as unit name, enabled/masked, and active state. **Check** compares against `systemctl` state; use **Command resource** for edge cases (timers, drop-ins, `daemon-reload`).
_Avoid_: Service resource (ambiguous with package services)

### Compliance and apply

**Drift**:
Actual state on an endpoint does not match the resolved desired state from its **Deployable artifact** at the current **Release ref**. Detected by a **Check** on sync, not synonymous with a failed apply.
_Avoid_: Out of sync (too vague), error, failed deploy

**Check**:
Read-only comparison of the system against resolved desired state (packages installed, file content, and similar). Produces a drift report without mutating the system.
_Avoid_: Dry-run apply (implies mutation path), audit (too broad)

**Apply**:
Mutating run that brings the endpoint toward desired state. On failure, applicators **Revert** per resource; a failed apply is an **Apply failure**, not drift by definition.
_Avoid_: Enforce, converge, remediate (use only if you add a named policy mode)

**Apply failure**:
The last apply attempt did not complete successfully (rollback may have run). Distinct from **Drift**—you can have drift without a recent apply failure (for example manual `nmap` install).
_Avoid_: Drift, desync

**Remediation policy**:
Per-**Fleet** setting stored on the server: `auto` (default) runs **Apply** after **Check** when drift is found; `report` records drift only and does not mutate the system until an operator triggers apply or Git advances with a separate policy.
_Avoid_: Mode, enforcement level, dry-run fleet

**Apply order**:
Default sequence when no explicit dependencies are set: flatten all applicable **Configuration** slices from **Resolved desired state**, classify each **Resource**, then apply packages first, non-critical files next, critical files last. **Configuration** slices are for human organization; execution order is global unless overridden.
_Avoid_: Pipeline, stages (okay informally)

**Resource dependency**:
Optional explicit ordering on a **Resource** (Terraform-style `depends_on`): the resource waits until named dependencies have applied successfully. Overrides default **Apply order** when present; cycles are invalid and must fail at parse or pre-apply time.
_Avoid_: Priority, weight, requires (too vague)

**Resource address**:
Stable reference `configuration-name/resource-name` used in `depends_on`. Resource `name` is unique within its **Configuration** slice; configuration `name` is unique within the **Deployable artifact**. Duplicate addresses are a parse error.
_Avoid_: ID, URI, path

**Resource**:
One intended change applied by a single applicator—**Builtin resource** (package, file, user) or **Escape hatch resource** (command, systemd). Identified by a stable name within the **Deployable artifact** for **Check**, **Apply**, **Revert**, and **Resource dependency** references. **Apply** is atomic at the **Resource** level: failure triggers **Revert** for that resource only.
_Avoid_: Task, step, handler (implementation word)

**Pre-apply validation**:
Checks before mutating critical resources (for example `sshd -t` before reloading sshd). Failed validation aborts that **Resource** without applying it.
_Avoid_: Atomic apply (whole artifact—see **Resource**)

### Server registry

**Server registry**:
Operational data that must not live in the **Configuration repository**: enrolled **Endpoint** records, **Endpoint credential** metadata, **Fleet** assignment, **Remediation policy**, **Labels** reported at check-in, drift and apply telemetry, and the server's current **Release ref** / **Git sync** settings.
_Avoid_: Database, Postgres (implementation)

### Admin surfaces

**Admin CLI**:
A separate Remotr package used by operators—no browser **Admin UI**. Desired state changes go through the **Configuration repository** (Git only). Server operations—enrollment tokens, endpoint lifecycle, drift views, **Remediation policy**, **Remotr CA** / cert rotation—go through the **Admin CLI** over mTLS using **Operator credential**s, not **Endpoint credential**s.
_Avoid_: Web console, dashboard UI, kubectl-style TUI (unless added later)

**Operator credential**:
Client TLS certificate for a human or automation principal using the **Admin CLI**. Distinct from **Endpoint credential**; same **Remotr CA** (or a separate operator CA) but different ACL on the server API.
_Avoid_: Admin API key, shared password

**Operator bootstrap token**:
A one-time random secret emitted when the server cold-starts or initializes. Delivered once to stdout and to a root-only file (for example under `/var/lib/remotr/`); both are invalidated after the **Admin CLI** exchanges it for the first **Operator credential**. Must not appear in structured logs after emission.
_Avoid_: Admin password, API key, permanent bootstrap file

**Operator issuance**:
After bootstrap, existing **Operator credential**s create or revoke additional operators via the **Admin CLI** over mTLS. Bootstrap is not repeated for each new operator.
_Avoid_: Re-bootstrap per user, shared admin cert

**Remotr binaries**:
Three programs in one repository: `remotr` (**Admin CLI**), `remotr-agent`, `remotr-server` (each under `cmd/`). Shared libraries live in `internal/`.
_Avoid_: Single combined binary, top-level `agent/` and `server/` cmd trees (legacy layout)

**Admin UI**:
Not planned. Administration is Git for desired state plus **Admin CLI** for the **Server registry** and PKI.
_Avoid_: Portal, web app

### Allowlisted dependencies (contributor policy)

Third-party imports are minimized: prefer the Go stdlib. Explicit allowlist for v1 includes **chi** (HTTP routing) and **yaml.v3** (YAML parse/emit). Dependencies are **vendored**; production builds compile from `vendor/`, not against mutable upstream module proxies.
_Avoid_: go get at build time, floating versions

### Agent runtime

**Agent service account**:
The agent systemd unit runs as root (`User=root`). **Apply** requires root for packages, `/etc`, `systemctl`, and **Command resource**; a dedicated user with passwordless sudo was rejected as not meaningfully safer.
_Avoid_: Dedicated agent user, NOPASSWD sudo, sudo elevation

### Enrollment and access

**Enrollment token**:
A short-lived, one-time (or few-use) secret an admin creates to bind a new endpoint to a Fleet. The agent exchanges it during enroll; it is not used on ongoing syncs.
_Avoid_: API key, invite code

**Endpoint credential**:
The client TLS certificate issued to an endpoint at enrollment. The server derives **Authenticated endpoint identity** from this credential on every request—never from fields the agent asserts in the request body.
_Avoid_: API key, auth token, device secret

**Authenticated endpoint identity**:
The endpoint record the server selects after mTLS, by mapping the presented **Endpoint credential** (for example certificate fingerprint or subject) to exactly one row in the **Server registry**. A compromised agent cannot claim another endpoint's identity in JSON to receive another fleet's **Deployable artifact**.
_Avoid_: Self-reported ID, device_id header, endpoint_id in body

**Remotr CA**:
Certificate authority operated by the server (v1). Issues **Endpoint credential** client certificates at enroll and terminates mTLS from agents. Enterprise PKI integration is out of scope for v1.
_Avoid_: step-ca, corporate CA (unless integrated later)

**Sync**:
One agent check-in over mTLS: server resolves **Authenticated endpoint identity**, compares **Release ref** / artifact digest, returns the **Deployable artifact** inline (with digest and **Remediation policy**) or unchanged. Large YAML is fine with HTTP compression (for example gzip) on the response. Request body carries telemetry (drift, **Label**s)—not endpoint identity.
_Avoid_: Self-identified sync, poll API, second-hop blob fetch (v1)

**Endpoint identity**:
The server-generated UUID for an enrolled endpoint. Encoded in the **Endpoint credential** (for example cert SAN) and recorded with the certificate fingerprint in the **Server registry** so each sync maps mTLS to exactly one row without self-reported IDs.
_Avoid_: Hostname as primary key, self-reported device_id

### Flagged ambiguities

**State** (in code):
The parsed contents of a **Deployable artifact**: a `Configurations` list of **Configuration** slices. Prefer **Deployable artifact** when talking about the file on disk, **State** when talking about the parsed structure, and **Resolved desired state** after targeting on the agent.

**Server-side merge** (deprecated intent):
Earlier design assumed the server layered global, fleet, label, and endpoint configs. With GitOps, layering is done in Git (directories, overlays, or CI render). The server syncs Git and maps each endpoint to an already-resolved path.

## Example dialogue

**Dev:** When a new laptop enrolls, what does it pull on the first sync?

**Expert:** `engineering-laptops` is `fleets/engineering-laptops/desired.yaml` in the **Configuration repository**. Arch and Debian laptops share that **Fleet path**; the server sends the full file and each agent builds **Resolved desired state** locally—pacman stanzas never run on Debian. One laptop needs a unique package? Add `endpoints/<id>/desired.yaml`—that **Endpoint override path** replaces the fleet file for that machine only. PR merges to `main`; a **Git sync** webhook advances the global **Release ref**; agents pull over mTLS and skip work if the artifact digest is unchanged. Each sync runs a **Check** first: if someone installed `nmap` by hand but desired says absent, that's **Drift** reported to the server—separate from an **Apply failure** when a bad sshd regex breaks **Apply**. Production fleets use default **Remediation policy** `auto` to self-heal; the lab fleet uses `report` so drift is visible without ripping out someone's manual install. **Apply** follows **Apply order**; a bad sshd stanza fails **Pre-apply validation** and only that **Resource** is reverted—not nine successful package installs from the same sync.
