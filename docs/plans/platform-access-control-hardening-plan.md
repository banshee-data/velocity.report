# Access control hardening

This proposal gives viewing, report downloads, configuration changes and maintenance
one permission model. Tailscale supplies authenticated callers now; future users and
groups must use the same authorisation boundary.

- **Status:** Agreed policy; core hardening implemented on PR #503. Appliance/live-tailnet acceptance and future native authentication remain outstanding.
- **Scope:** Main HTTP API, its alternate LiDAR HTTP entry point, local recovery and a future authenticated gRPC service.
- **Related:** [v0.5.1 backlog](../BACKLOG.md), [networking](../radar/architecture/networking.md), [security surface](../../.github/knowledge/security-surface.md), [TENETS.md](../../TENETS.md).

## Implementation boundary on PR #503

The hardening was developed as PR #684 on `dd/api/tailscale-acls-503`, which kept #503's
original commits, then moved onto #503's branch so the original work and its review land
together. References to #684 below name that earlier head and its audit.

The branch implements the transport-independent operation/resource contract, anonymous
LAN viewing/PDF policy, direct `WhoIs` adapter, dedicated Serve capability backend,
five-second cache without stale fallback, status/site/report redaction, origin checks,
caller-permission UI and loopback containment of alternate LiDAR HTTP/full gRPC.
The legacy `off`/`on` profiles remain explicit compatibility choices; `off` stays the
installation default. Routine admin includes configuration, report creation and
export, while HTTP maintenance and access management remain disabled in hardened mode.

Following #503's original review, every listener (main HTTP, Serve backend,
alternate LiDAR HTTP and gRPC) drops connections from the host while tailscaled
forwards to its port through a Serve handler the manager did not install: such a
forward carries client-written forwarding and capability headers. The image unit
selects the profile through `VELOCITY_ACCESS_PROFILE`, so an operator's drop-in
survives later changes to its command line.

OS CLI enrolment, persistent systemd activation and rollback are documented in the
[operator runbook](../platform/operations/tailscale-remote-access.md#hardened-profile).
Local tests and builds are software evidence; the Pi/live-tailnet acceptance matrix
has not run. This branch does not implement native users/groups, private resource
visibility, authenticated LAN gRPC, a passive gRPC profile or remote maintenance.

Local verification passed the complete maintained suites: Go, 153 Python tests,
916 web tests, nine offline-documentation tests, 800 macOS tests, 75 S2 Hilbert tests
and 51 scene-capture tests. Svelte checking reported zero errors or warnings; lint,
web/server builds and embedded documentation links passed. Branch-added internal Go
files reached 98.9%–100% coverage with the real `pcap` tag. Race checks passed for the
API, Tailscale cache, server command, LiDAR HTTP and gRPC packages. The root tooling
suites used the working pnpm 10.14.0 launcher after the locally cached pnpm 12 launcher
failed; the dependency lock remained unchanged. These are local software results,
not Pi, live-tailnet, multi-user or physical-sensor acceptance.

## Decision and rationale

Adopt operation permissions independent of the caller's network address. An
authentication adapter establishes a principal; a shared policy decides whether that
principal may perform an operation on a resource through the chosen ingress.

For the hardened profile, anonymous LAN callers may view aggregate charts and
download existing ordinary PDF reports. They may not change configuration, generate
reports, retrieve raw exports or use maintenance tools. Tailscale membership alone
does not confer administration: application grants determine permissions.

This deliberately replaces the LAN-admin premise in
[PR #503](https://github.com/banshee-data/velocity.report/pull/503) and
[PR #684](https://github.com/banshee-data/velocity.report/pull/684). Preserve existing
deployments through an explicitly documented compatibility profile during migration;
do not quietly change an installed device's access policy on upgrade.

Native user and group authentication is future work. The present change must leave
room for authenticated LAN users without adding an account database, password flow,
group editor or general policy language now. Local capture, viewing and recovery must
remain usable without a cloud service.

## Historical boundary before this change

The original sensitive routes use `tsweb.Debugger`: database backup, SQL, profiling,
serial commands and serial tail. Ordinary LAN addresses are refused; loopback and
direct Tailscale addresses are admitted, subject to the library's debug overrides.
This is a source check, not an application user-role system. The boundary dates to
[the original database debug routes](https://github.com/banshee-data/velocity.report/commit/d3c213fa460e021a18e5e4c16a759e4a4efdad69)
and remains documented in [networking](../radar/architecture/networking.md).

Ordinary settings APIs evolved outside that boundary. The audited #684 head added an optional outer
gate around the main HTTP mux, while keeping the inner debug checks. It consequently
admitted LAN configuration changes but refuses LAN debug access; Tailscale Serve admins
also fail the inner debug check because it rejects `X-Forwarded-For`.

PR #684's source classifier does not establish that an address is a private LAN
address. Direct public addresses and non-loopback proxies also fall into its local
class. Neither a non-Tailscale address nor a loopback proxy connection proves
administrative authority.

The LiDAR route set also has a separate HTTP listener without the new gate. Some
LiDAR handlers under `/debug/lidar/` are ordinary handlers, without `tsweb` protection.
Listener containment is therefore part of the security boundary, not an incidental
deployment detail.

### Review evidence and limits

The PR comparison used #503 head `c93272a4d3f5992c9c10fb33a79cf604fa501b57` and
#684 head `192ebc868537e4240437291034be47fb8aad2074`. Both original #503 commits
are ancestors of #684. This establishes commit preservation and code supersession;
it does not establish completion of the hardened policy agreed here.

A deterministic regression against the unmodified #684 implementation exercised
one peer and three resolver responses: an older admin lookup held in flight, a newer
authoritative not-found response, then a timeout after the short negative-cache entry
was aged beyond its five-second TTL. Releasing the old success between denial and
timeout restored the last-good admin identity. The regression expected an error and
no view/admin permissions; it failed in all 10 repetitions under Go's race detector.
The test advanced the cache timestamp rather than waiting five real seconds. This
proves the tested ordering permits stale authorisation; it does not measure live
Tailscale revocation propagation or show an unsynchronised memory access.

A separate probe combined the real `tsweb.Debugger` with #684's outer gate and a
controlled peer resolver. All five expected cases passed under the race detector:

| Caller reaching the debug route                                                    | Observed HTTP status |
| ---------------------------------------------------------------------------------- | -------------------- |
| Ordinary LAN                                                                       | 403                  |
| Loopback without forwarding headers                                                | 200                  |
| Direct Tailscale administrator                                                     | 200                  |
| Direct Tailscale viewer                                                            | 403                  |
| Tailscale Serve administrator, represented by loopback plus forwarded peer address | 403                  |

The existing API and Tailscale package suites passed under the race detector when
the deliberately failing regression was excluded. Test fixtures and raw output were
kept outside the checkout. These controlled checks explain the overlapping policy and
cache risk; they do not validate a Pi, real Serve headers or physical sensor behaviour.

## Permissions and role presets

Classify the operation, including its side effects and disclosure, rather than just
the HTTP verb or URL prefix. A GET database backup creates files; a source archive
download can reveal more information than an aggregate chart.

| Permission               | Operations                                                                                          | Initial holder                                                                               |
| ------------------------ | --------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Aggregate viewing        | Charts, aggregate statistics, safe display metadata and safe device status                          | Anonymous LAN viewer where enabled; authenticated viewers                                    |
| Ordinary report reading  | List, inspect and download existing ordinary PDFs available to the viewer                           | Same viewing policy                                                                          |
| Report creation          | Generate reports and bounded aggregate exports; consumes resources and creates records/files        | Reporter or administrator                                                                    |
| Configuration inspection | Detailed sensor settings, serial device paths and operational configuration                         | Administrator; separately grantable later                                                    |
| Configuration changes    | Edit sites/settings, reload serial configuration, tune sensors, control acquisition/replay/workers  | Administrator                                                                                |
| Data export              | Detailed observations, raw captures and report source archives pending their content classification | Explicit export permission; may be included in the initial admin bundle; no anonymous access |
| Maintenance              | Database backups, arbitrary SQL, profiling, sensitive diagnostic dumps and destructive operations   | Explicit maintainer grant, with maintenance enabled                                          |
| Access management        | Change authentication policy, enrolment/recovery authority or credentials                           | OS-authorised local operator initially; separate permission with native authentication later |

Use viewer, reporter and administrator as convenient bundles over permissions.
Maintenance and access management remain separate: routine configuration authority
does not automatically confer arbitrary SQL or the ability to change access policy.
Do not hard-code a universal role hierarchy into handlers.

Existing Tailscale `view` and `admin` grants can map to the viewing and routine
administration bundles, with report creation/export included explicitly in the initial
admin mapping. Add separate application grants when finer delegation is needed.
Maintenance remains outside routine admin. Specify each mapping in one place; do not
infer permissions from the presence of any capability name.

### Reports and resource visibility

An ordinary PDF is a viewer-facing aggregate report. The agreed LAN policy includes
downloading those reports; it does not make every stored artefact anonymously
readable. Current report data includes operator-entered surveyor, contact, location
and descriptions. Operators must understand what the chosen viewing audience receives.
The sensor's lack of cameras and licence plates does not establish the disclosure
policy for those fields.

The current source ZIP contains Typst templates, fonts, chart SVGs and `data.json`.
It is not a database backup. Initially require export permission for this separate
artefact, then review its actual contents before deciding whether it can share the
ordinary report-reading permission. Do not label every ZIP as raw sensor data.

The first implementation may use a device-wide viewing scope. Carry the actual
resource into the policy decision so future user/group permissions can restrict sites,
reports or captures without changing every handler's contract. Do not claim that
private report visibility or per-site access already exists. Do not equate permission
to view data with preventing a viewer from copying that same data.

Anonymous visibility is a disclosure floor: anyone able to reach the reading ingress
can omit credentials and retrieve those resources. Future account-specific denials
cannot make the same anonymously available PDF private. Private resources must be
excluded from that audience or anonymous access must be disabled.

## Ingress policy

| Entry point                                               | Hardened behaviour                                                                                       |
| --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Deliberately exposed LAN reading listener                 | Anonymous aggregate and ordinary report reading if enabled; no privileged operations                     |
| Direct Tailscale connection                               | Verified peer identity and application grants; protected actions require successful authentication       |
| Dedicated Tailscale Serve backend                         | Verified Serve capability claims on the configured backend; the same operation policy as direct access   |
| Unexpected proxy, malformed identity or forwarded address | Deny protected operations; never infer local administration                                              |
| Funnel/public internet                                    | Unsupported and refused; a non-Tailscale address must not become a LAN administrator                     |
| Ordinary loopback HTTP                                    | No automatic administrative privilege; Serve also reaches the server through loopback                    |
| OS-authorised local CLI/console or protected Unix socket  | Bootstrap, recovery and local administration; filesystem permissions and OS identity define the boundary |
| Future native session or API credential on the LAN        | The same operation permissions and resource policy, with suitable transport and browser protections      |

Define LAN exposure through a configured listener/interface and network restrictions.
An application cannot prove which physical network a request crossed from a source IP
alone. Source-NAT, port forwarding and a reverse proxy can erase that provenance.
Anonymous reading is an intentional disclosure on that ingress; privileged operations
still need authentication if traffic reaches it through an unexpected path.

The initial hardened profile selects anonymous reading. Future private-resource policy
must let operators narrow or disable that audience independently. Reject invalid credentials
on the request that presents them; do not retry that request as a more privileged
principal. This does not prevent a separate credential-free request for deliberately
anonymous material. Do not expose full configuration, enrolment URLs or errors merely
because their endpoint is named status.

## Shared authorisation boundary

Keep the authentication adapter separate from the operation policy. Principals carry
an authentication mechanism, effective permissions and an opaque provider-specific
subject/device identifier where reliably available. Native users need a stable subject;
capability-only proxy claims must not invent a human identity. Keep ingress attributes
separate from identity. Include the resource and permission in each decision; do not
make a source address a user ID.

```text
LAN reading ingress -------- anonymous principal ------------------+
Direct Tailscale ingress ---- local API identity and grants --------+
Dedicated Serve ingress ----- verified proxy capability claims -----+
Future session/API token ---- native user and group principal ------+--> operation policy
Protected local recovery ---- OS-authorised operator ---------------+    + resource scope
                                                                          |
                                                     HTTP / LiDAR / future gRPC
```

Register required operations beside the routes that implement them. Test the complete
inventory, including aliases, unsupported methods, file downloads, static artefact
paths and stream upgrades. Unknown routes must not obtain privileged access by
omission. Path-level rules alone cannot distinguish a PDF from its source archive or
a status read from a mutating control action.

For browser mutations, enforce permitted origins and appropriate CSRF protections.
Tailscale identity is ambient machine authority: a hostile website can induce requests
from an authorised browser even when the app has no login cookie.

Add a caller-access endpoint, separate from the existing sensor-capabilities endpoint.
It reports effective permissions for UI behaviour. Disable or hide unavailable actions
and explain access failures; the server remains authoritative.

When native authentication arrives, select the request's principal deliberately. An
invalid native credential must not fall back to an anonymous or device administrator.
Do not silently union a native viewer's permissions with the machine owner's Tailscale
admin grant, or link accounts by matching email addresses. Tailscale identifies a peer
and its associated identity; it does not prove which person is using a shared browser.
Keep unnecessary names, emails and profile claims out of measurement storage and logs.

## Tailscale implementation direction

Tailscale Serve supports forwarding selected application capabilities through
`Tailscale-App-Capabilities`, removing client-supplied copies. This applies to user-owned
and tagged devices. The feature is documented for daemon versions 1.92 and later:
[Serve application capabilities](https://tailscale.com/docs/features/tailscale-serve#app-capabilities-header).
The packaged daemon is pinned independently of the Go dependency; verify both the
installed daemon and the Serve configuration before relying on this mechanism.

Prefer those per-request claims on a dedicated, protected Serve backend. Never accept
them on an ordinary LAN-facing listener. Retain a local API adapter for supported direct
Tailscale connections; both adapters produce the same application permissions.

Configure a separate backend address and permitted proxy source, with no anonymous
reading or ordinary localhost-admin bypass on that backend. Reject missing, malformed
or unrecognised claims for protected requests. A loopback backend excludes remote
header spoofing but trusts local processes able to reach it; document that OS trust
assumption, and use stronger process isolation if untrusted local services are in scope.
The LAN reading listener must ignore or reject purported Serve identity headers.

Use a short, bounded successful lookup cache only where the direct adapter needs it.
Do not retain the ten-minute stale-admin allowance. An unavailable resolver refuses
protected operations; anonymous viewing continues where independently permitted.
Updates and authoritative denials need atomic ordering, including a retained denial
generation while older lookups are in flight. Deleting an identity must not erase the
ordering evidence that prevents an older answer from recreating it.
Apply that ordering to the identity returned to the waiting request as well as cache
writes: rejecting an obsolete cache write must not let that same obsolete result
authorise its request after a newer authoritative denial.

A successful local lookup proves what the daemon currently knows, not that its policy
was freshly obtained from the coordination server. Document both daemon propagation
and application cache limits when describing revocation.

## Debug and maintenance

Preserve the original sensitive-route boundary while migrating. Removing `tsweb`
first would expose diagnostics through the `on` profile's LAN bypass. Move these
handlers behind the shared maintenance policy on a protected management ingress, then
remove the conflicting source check once equivalence tests pass.

Maintenance is disabled by default and requires an explicit grant when enabled.
Serial configuration controls belong to routine configuration permissions, even when
historically registered under `/debug/`; SQL, database backup and process diagnostics
remain maintenance. A file-writing GET is not viewer access.

Tailscale enrolment/logout, listener exposure, trusted-proxy configuration, enforcement
profiles, maintenance activation and authentication settings belong to access
management. Routine administrators cannot change their own security boundary. A
maintainer with arbitrary writable SQL or equivalent process access is fully trusted:
permission names cannot isolate future accounts or resource restrictions from that
authority. Restrict the actual maintenance tool if a narrower trust boundary is needed.

## gRPC and separate LiDAR HTTP

Keep gRPC local to the Mac development environment for now. The user has explicitly
deferred LAN exposure of file-writing or privileged control RPCs until user
authentication is available. A future genuinely read-only service can be considered
for LAN use independently; do not expose the entire current service on that basis.
The audited default was `localhost:50051`, but its listen flag permitted other addresses.
Every profile must reject a non-loopback full gRPC service until its authentication is
implemented; the current default alone does not enforce that constraint.

The current implementation has the following effects:

| RPC                               | Implemented effect                                                         | LAN treatment                                                                           |
| --------------------------------- | -------------------------------------------------------------------------- | --------------------------------------------------------------------------------------- |
| `StreamFrames`                    | Subscribes to published frames; request filters apply to that subscription | Eligible for a separately enabled passive stream profile                                |
| `GetCapabilities`                 | Reads sensor/service capabilities                                          | Eligible for that passive profile after correcting its advertised capabilities          |
| `Pause`, `Play`                   | Change shared playback and delivery for all subscribers                    | Local until authenticated control access is implemented                                 |
| `Seek`, `SetRate`                 | Change the shared VRLOG reader's position/rate where supported             | Local until authenticated control access is implemented                                 |
| `SetOverlayModes`                 | Stores global in-memory preferences; no streaming consumer was found       | Treat as control; do not expose because its present effect is limited                   |
| `StartRecording`, `StopRecording` | Return `Unimplemented`; neither starts a recorder nor writes a file        | Exclude from the passive profile so later implementation cannot silently broaden access |

The recording request's output path is unused. At the audited snapshot, `GetCapabilities`
advertised recording support; this branch corrects that advertisement. No implemented RPC directly edits
tuning files, saves labels/annotations or chooses a capture file. Those macOS actions
use HTTP APIs, and actual recorder start/stop callbacks are wired to HTTP replay
orchestration.

Playback still has consequential side effects. Reaching VRLOG EOF invokes the
publisher's replay-ended callback, wired to `ParkFinishedReplay`. That marks the replay
inactive and starts a live listener; a new sensor packet then returns the pipeline to
live input. An attached recorder also records published frames, including replay
frames: an RPC cannot attach it, but changing shared replay is not a guarantee of
isolation from an existing writer. The full service is therefore unsuitable for an
unauthenticated read-only LAN profile even though it has no direct settings-file write.

These findings come from the [RPC handlers](../../internal/lidar/l9endpoints/grpc_server.go),
[production wiring](../../internal/cmd/server/radar.go),
[replay publisher](../../internal/lidar/l9endpoints/publisher_vrlog.go) and
[park/live transition](../../internal/lidar/server/return_to_live.go) at main snapshot
`1b0523d5d`. They establish code paths, not live multi-client or physical-sensor behaviour.

For the immediate hardened profile, keep the alternate LiDAR HTTP listener constrained
to local development or attach the same operation policy before allowing network
exposure. The hardened profile must reject unauthenticated privileged alternate
listeners at startup. A loopback bind is containment for remote clients, not
authentication of every local process.

Future authenticated gRPC needs method-specific permissions, protected transport,
resource checks and stream lifecycle rules. A read-only exposure must register only
read methods and constrain payload disclosure; returning sensor capabilities does not
make playback, recording or global preference changes read operations.

Passive LiDAR streaming reveals individual points, tracks and potentially diagnostics,
rather than only the agreed anonymous aggregate dashboard. Decide its payload and
audience explicitly when enabling LAN streaming; do not inherit that permission from
ordinary chart viewing. Bound concurrent streams and resource use. The standalone
VRLOG replay server consumes a shared reader from `StreamFrames`, so it needs independent
read sessions or an already-published stream before it qualifies as passive viewing.

## Failure and recovery registry

| Failure                                                                      | Required behaviour                                                                        | Recovery                                                                       |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| Tailscale identity lookup unavailable                                        | Refuse protected operations with a retryable error; no stale admin fallback               | Retry after the daemon recovers or use OS-authorised local administration      |
| Unknown peer or absent grant                                                 | Deny the operation; preserve denial ordering against in-flight older answers              | Correct the grant using an authorised operator                                 |
| Spoofed/malformed proxy claims                                               | Refuse; no LAN/local admin fallback                                                       | Correct the explicitly configured ingress                                      |
| Revoked grant during an in-flight lookup                                     | Older success cannot restore a newer denial                                               | Fresh authorised lookup after policy is corrected                              |
| Incorrect grants lock out remote administrators                              | Sensor capture and permitted viewing continue                                             | Local console or SSH identity repairs configuration                            |
| First Tailscale enrolment                                                    | No anonymous LAN enrolment URL or unrestricted mutation flow                              | OS-authorised Tailscale CLI/console; add a protected local channel if required |
| Maintenance disabled                                                         | Refuse maintenance regardless of routine admin authority                                  | Explicit local enablement and maintenance grant                                |
| Protected service exposed on an alternate listener                           | Refuse hardened startup until policy or containment is configured                         | Constrain the listener or enable the shared enforcement boundary               |
| Missing adapter, unsupported daemon/configuration or invalid hardened policy | Refuse startup or protected requests; never silently select compatibility mode            | Repair through the OS-authorised recovery path                                 |
| Native identity added later                                                  | Explicit credential failure cannot become a more privileged Tailscale/anonymous principal | Authenticate again through the selected mechanism                              |

Audit permission decisions with bounded logs: operation, outcome and an opaque actor
identifier where reliably available. Record capability-only principals as such rather
than inventing a user identity. Avoid login URLs, tokens, request bodies and unnecessary
identity claims.

Recovery must work before hardened mode is activated, including when Tailscale is
offline or unenrolled. The current `velocity device tailscale` wrapper installs and
controls the daemon lifecycle; it does not implement enrolment/logout. Use an
OS-authorised Tailscale CLI/console flow or add a protected local channel. Do not rely
on a device command that merely calls HTTP endpoints after localhost administration
has been removed.

## Compatibility with the pre-hardening gate

The gate as audited on PR #684 is a useful implementation base. Its Tailscale
integration and HTTP plumbing are largely reusable; the trust policy needs substantial
revision. A percentage of changed lines would obscure that distinction: a small bypass
can determine the safety of the entire service.

| Existing work                                                                 | Treatment in this plan                                                                                                          |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Local `WhoIs` adapter and application-owned peer types                        | Retain the SDK boundary; translate verified grants into the shared principal and operation permissions                          |
| Main HTTP mux wrapper and explicit unauthenticated asset allowlist            | Retain central enforcement; apply it to every permitted listener and classify operations/resources explicitly                   |
| Unknown-peer versus unavailable-resolver errors; structured 403/503 responses | Retain the distinction and response conventions; refuse unresolved protected operations                                         |
| Bounded cache, short TTL and concurrency tests                                | Retain useful bounds/tests; remove stale-grant fallback and prevent older in-flight answers overriding newer denials            |
| Funnel checks and rejection of untrusted forwarding                           | Retain the protections; make ingress trust explicit rather than treating every other source as local admin                      |
| Status redaction and enrolment-URL protection                                 | Retain the redaction mechanism; apply it to anonymous LAN too and reserve enrolment information for access-management authority |
| Early flag validation, startup integration, docs and test fixtures            | Retain the integration; introduce explicit hardened/compatibility semantics and update assertions/runbooks for the new policy   |
| LAN and ordinary loopback administration                                      | Replace in hardened mode with deliberate anonymous reading and OS-authorised recovery                                           |
| `view`/`admin` path map, including all report GETs as viewer access           | Replace with operation permissions; distinguish PDF/source ZIP, safe status/detailed configuration and reads/side effects       |
| Loopback plus forwarded-IP inference for Serve                                | Replace the implicit inference with a configured backend/proxy contract; prefer verified per-request capability forwarding      |
| Silent enforcement-off selection when the auth client is missing              | Remove in hardened mode; missing authentication cannot select the compatibility policy                                          |
| Existing inner debug gate                                                     | Preserve during migration; keep remote maintenance disabled until the shared maintenance boundary is ready                      |

Existing tests are useful fixtures, not proof of the changed policy. LAN-admin and
loopback-admin assertions must change in hardened mode; retain separate compatibility
tests. Add complete route/operation coverage, alternate-listener checks and the failed
revocation ordering regression to the implementation change.

## Recommended release boundary

### v0.5.1: a usable hardened boundary

Deliver an opt-in hardened profile alongside an explicitly documented compatibility
profile. Do not silently migrate existing installations or downgrade a failed hardened
configuration. The existing backlog description and its `S` estimate cover the older
LAN-admin feature, not this expanded work; split and re-estimate the hardening before
committing the release scope.

The minimum hardened profile consists of:

1. One operation policy and a principal/resource contract that native authentication
   can use later. Keep the present Tailscale viewer/admin presets; routine admin may
   bundle report creation. A reporter preset and custom role editor are unnecessary
   for this release. Raw/source exports require export permission, which can be
   explicitly included in the initial admin bundle; maintenance and access management
   do not inherit routine admin.
2. Deliberate anonymous LAN aggregate/PDF reading, protected settings/generation and
   safe status redaction. Ordinary loopback HTTP does not confer administration.
3. A verified direct-Tailscale adapter and an explicit Serve trust contract. Remove
   the ten-minute stale grant fallback, bound the short cache and fix concurrent
   response ordering. Refuse malformed claims, unsupported hardened configurations,
   Funnel and untrusted forwarding.
4. Coverage or containment of every alternate control path. Constrain the separate
   LiDAR HTTP listener to loopback unless it uses the shared policy. Enforce loopback
   for the full gRPC service. Keep remote debug/maintenance disabled where migration
   is incomplete; a new remotely accessible maintenance UI is not a release requirement.
5. Working OS-authorised bootstrap/recovery, browser-origin protections, a caller
   permission response and minimal UI behaviour for refused actions. Establish
   persistent activation, compatibility and rollback on a Pi/live tailnet, alongside
   the local route, outage and revocation tests.

PR #503 selects the dedicated capability-header backend for this release. It requires
Tailscale 1.92 or later and configures both accepted application capabilities. There
is no forwarded-IP fallback in hardened mode; unsupported or missing claims fail
closed. The legacy `on` adapter remains separate for compatibility.

These are acceptance criteria for claiming the new hardened profile works. Deferring
one leaves that profile incomplete; it does not prevent separately shipping the
original compatibility feature after its correctness defects are fixed and its more
limited security guarantees are stated accurately.

### Later: identities and additional remote surfaces

Native users/groups, credentials, sessions, account linking, group administration and
private per-site/report visibility are a separate feature. Preserve operation and
resource checks now; implementing their user interface and storage is not part of
v0.5.1. Keep intentionally anonymous resources distinct from future private resources.

LAN access to the full gRPC control service follows native authentication, method
permissions and transport/stream validation. A separately enabled passive streaming
profile may precede it, but its method set, shared-reader isolation, payload audience
and resource bounds need their own review; it is not required for v0.5.1.

Remote maintenance tooling, richer permission presets/editors and finer resource
scopes can follow once their enforcement and trust contracts are ready. Disabled
remote maintenance is an acceptable first release; an exposed unprotected control
surface is not.

## Delivery sequence and acceptance

1. Freeze the operation/ingress matrix and report disclosure policy. Update the v0.5.1
   item to distinguish this hardening from the `on` profile's LAN-admin compatibility.
2. Implement the shared principal/policy boundary, route inventory and direct/Serve
   adapters. Remove the stale-admin cache and fix the reproduced deletion race.
3. Establish local bootstrap/recovery, migrate debug routes safely and contain the
   alternate LiDAR listener. Keep gRPC local; native authentication and LAN control RPCs
   are a separate later work unit.
4. Add caller-access UI behaviour and validate browser-origin protections. Document
   persistent activation, compatibility mode and rollback before recommending the
   hardened profile for deployments.
5. Add native users/groups later using the same permissions and resource decisions.
   Expose protected gRPC controls only after that authentication and method policy are
   implemented and validated.

The release gate is a route-and-operation matrix across anonymous LAN, localhost HTTP,
local OS operator, direct Tailscale viewer/reporter/admin/maintainer, Serve callers,
unknown proxies and Funnel. Exercise outage, authoritative revocation, concurrent
lookups, file downloads and aliases. Verify that ordinary PDFs and their source ZIPs
have deliberate permissions, and that view-only methods cannot trigger backup/export
or shared control side effects.

Local and CI tests establish application decisions and ordering. A Pi/live-tailnet
pass must establish actual proxy claims, persistent activation, enrolment, revocation,
recovery and listener containment. Record those separately; current evidence does not
establish physical deployment behaviour.

## Alternatives and remaining choices

Keeping LAN administration is convenient for enrolment and offline use but grants
configuration authority to any reachable unauthenticated host. A blanket localhost
exception also confuses Serve traffic with a local operator. Neither is the hardened
profile proposed here.

Requiring authentication for all viewing would simplify disclosure control but make
the community dashboard and offline read access harder to use. Anonymous aggregate
viewing and ordinary PDF downloads are the agreed compromise; their exposure remains
explicit; future private resources need a stricter audience policy.

Building native users/groups first would provide human identity sooner but delay the
requested Tailscale hardening and broaden this work considerably. A shared operation
policy with a Tailscale adapter provides the boundary now and keeps account work
separate.

PR #503 settles the initial permissions, routine-admin report/export bundle, dedicated
Serve backend and explicit compatibility choice. Remote maintenance remains disabled;
its future activation mechanism and native private-resource policy need separate
implementation. Software checks establish local behaviour, while the Pi/live-tailnet
activation and recovery matrix remains the release acceptance gate.
