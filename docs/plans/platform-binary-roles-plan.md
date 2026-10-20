# Binary roles: one binary, each role a composition root with an asserted closure (v0.5.8)

- **Status:** Proposed. Expands option b of C1 in the [architecture review](platform-architecture-review-plan.md#c1-every-role-links-everything-and-cgo-is-always-on)
- **Layers:** Cross-cutting (CLI composition, Go package boundaries, LiDAR layer rule, build)
- **Target:** v0.5.8 for the gate and the graph moves; v0.5.9 for the lifecycle supervisor
- **Decisions:** D-09 (single binary), D-26 (static Linux binaries), ADR-1 of the review
- **Companion plans:** [binary-size-reduction-plan](binary-size-reduction-plan.md), [cmd-server-decomposition-plan](cmd-server-decomposition-plan.md), [go-cmd-extraction-plan](go-cmd-extraction-plan.md), [go-codebase-structural-hygiene-plan](go-codebase-structural-hygiene-plan.md), [lidar-job-queue-implementation-plan](lidar-job-queue-implementation-plan.md)
- **Canonical:** [go-package-structure.md](../platform/architecture/go-package-structure.md)

Keep the one `velocity` binary and make each thing it does (edge, hub, worker, device, jobs) a
composition root whose dependency closure CI checks on every PR. Then move packages until the
checks hold, and measure whether the cgo dependency is needed at all.

## Motivation

The owner likes the one-binary approach and expects more roles: a worker, a storage hub, an
edge sensor host. Today a role cannot be reasoned about, because every role links everything:
`velocity jobs` imports no internal package and still ships libpcap, the Typst engine, gRPC and
tailsql. The LiDAR layer rule ("lower layers never import higher ones; layers never import
`pipeline/`") is written in AGENTS.md and the layer model and enforced by nothing, and six edges
break it. Without a gate, every new role makes the graph worse, and splitting the binary later
would mean untangling a graph nobody can see.

## Current state

Measured with `go list -deps -tags=pcap` on `acdb03e`.

| Entry package         | Internal packages reached | Third-party packages | Heavy dependencies it should not need                                               |
| --------------------- | ------------------------: | -------------------: | ----------------------------------------------------------------------------------- |
| `internal/cmd/jobs`   |                         0 |                    0 | none on its own; it ships inside the full binary                                    |
| `internal/cmd/device` |                         9 |                   93 | tailsql, Tailscale and SQLite migrations, through `internal/db` for one backup call |
| `internal/cmd/worker` |                        28 |                  101 | the web and docs embeds, tailsql                                                    |
| `internal/cmd/scene`  |                        29 |                  208 | gRPC, through the frame model in `l9endpoints`                                      |
| `internal/cmd/tune`   |                        53 |                  280 | the whole server, because its HTTP client lives in `internal/lidar/server`          |
| `internal/cmd/server` |                        54 |                  280 | (the edge; needs most of it)                                                        |
| `cmd/velocity`        |                        69 |                    — | every applet, imported eagerly by `internal/cmd/root`                               |

| Edge that breaks the layer rule                                             | Why it exists                                                                                   |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| `internal/lidar` (root alias package) imports `pipeline`, storage, l2 to l6 | Its only production callers, `l1packets/network` and `l9endpoints`, want four logging functions |
| `l1packets/network` imports `l3grid`                                        | Foreground extraction inside the real-time PCAP reader                                          |
| `l1packets/network` imports `internal/db`                                   | An unused field in `listener.go`                                                                |
| `l1packets/network` and `l1packets/parse` import `l2frames`                 | `PointPolar` is defined one layer up                                                            |
| `l6objects` imports `l8analytics`                                           | Re-exports in `comparison.go`                                                                   |
| `l4bobserve` imports `l4perception`                                         | Sideways: shared world-point and cluster types                                                  |
| `internal/db` imports `l3grid`                                              | The core storage package knows a LiDAR type                                                     |

Two further facts shape the design:

- **cgo is used only to read capture files.** Every production libpcap call is `pcap.OpenOffline` (five call sites in `l1packets/network` and `pcapsplit`) plus a `udp and port N` filter; the server's self-check calls `FindAllDevs` and `OpenLive` only to prove libpcap loads. The repository already reads pcap and pcapng in pure Go with `gopacket/pcapgo` (`annotation/labelset/bind.go`). The Docker, zig, musl and vendored libpcap toolchain of D-26 exists for this one dependency.
- **The frame model lives in the transport package.** `FrameBundle`, `Track`, `TrackState` and `PointCloudFrame` are defined in `l9endpoints/model.go`, so 37 packages link gRPC, `config-validate` among them.

## Findings

| Area               | Current state                                                                                                   | Severity | Release view       |
| ------------------ | --------------------------------------------------------------------------------------------------------------- | -------- | ------------------ |
| Role closures      | Not stated, not checked; every role links every dependency                                                      | High     | v0.5.8             |
| Layer rule         | Documented, unenforced, six violations                                                                          | High     | v0.5.8             |
| Frame model        | In the gRPC package                                                                                             | High     | v0.5.8             |
| Root embed package | Web and docs assets beside tuning defaults; four evaluation tools need `web/build` to compile                   | Medium   | v0.5.8             |
| Hidden defaults    | `MustLoadDefaultConfig` walks `../..` from inside l3, l4, l5, l6 and storage; `LIDAR_TIMESTAMP_MODE` read in L1 | Medium   | v0.5.8             |
| cgo                | Always on, for file reading a pure-Go reader can do                                                             | Medium   | measure (host0004) |
| Lifecycle          | Seven Start, Stop and Run signatures, no supervisor; HTTP handlers start ingest                                 | Medium   | v0.5.9             |

## Design / approach

### Roles and their rules

| Role                   | Composition root                     | Must not reach                                                                              |
| ---------------------- | ------------------------------------ | ------------------------------------------------------------------------------------------- |
| edge                   | `internal/cmd/server`                | worker kinds' evaluation packages (`replayeval`, `sweep`, `annotation` tooling)             |
| hub                    | `internal/api` and `internal/report` | `pipeline`, `l1packets/network`, `gopacket/pcap`, `google.golang.org/grpc`                  |
| worker                 | `internal/cmd/worker`                | `serialmux`, `tailscale`, the Typst engine, the web and docs embeds, `internal/api`         |
| device                 | `internal/cmd/device`                | `internal/db`, `pipeline`, `gopacket/pcap`, gRPC, Typst                                     |
| jobs                   | `internal/cmd/jobs`                  | any internal package except the job contract (`internal/lidar/jobs`) and `internal/version` |
| every LiDAR layer `lN` | `internal/lidar/lN*`                 | any `lM` with M greater than N, `pipeline`, `server`, `internal/db`                         |

The hub row names `internal/api` rather than a hub applet because the hub is `velocity serve`
with sensors off; the rule is that the API and reports never need the sensor stack, so a hub can
be the same binary with ingest disabled and no cost beyond the bytes.

### The gate is a ratchet

The check is a Go test over `go list -deps -json` output, run by `make lint-go`. It holds the
table above as data, and a short allowlist of today's violations, each naming the move below that
removes it. It fails on any violation not on the allowlist, and on any allowlist entry that no
longer occurs, so the list can only shrink. It is gated from the first PR: nothing new can break
a rule, and nothing has to be fixed before it lands.

### The pure-Go capture reader is measured, not assumed

Replacing `pcap.OpenOffline` with `pcapgo` readers and the port filter with a decoded-UDP check
would let every role build with `CGO_ENABLED=0`, cross-compile from any host without Docker, and
retire the vendored libpcap. Two things could make it wrong: a packet stream that differs (link
types, truncated packets, nanosecond timestamps) and a slower read. Experiment host0004 replays
the same captures through both readers and compares packets, frames and wall time; its rule is
written before the run. If it passes, D-26 is restated; if not, the `nopcap` build still serves
the hub and the clients, which never read captures.

## Scope

### Item 1: the closure and layer gate

**Summary:** the ratchet test above, with today's violations allowlisted against the items that remove them.

**Steps:**

1. Encode the roles table and the layer rule as test data; read the graph with `go list -deps -json -tags=pcap`.
2. Allowlist the seven layer edges and the role reaches in Current state, each naming its item.
3. Fail on an unlisted violation and on a stale allowlist entry; run in `make lint-go`.
4. Record the rule in go-package-structure.md as the Kept-by line of D-09.

**Milestone:** v0.5.8

### Item 2: the alias package and the layer edges

**Summary:** delete `internal/lidar/aliases.go`; move the four logging functions to `internal/lidar/logutil`; fix the six edges.

**Steps:**

1. `l1packets/network` and `l9endpoints` call `logutil` directly; delete the root alias package and the unused alias shims in `l4perception` and `l6objects`.
2. Move `PointPolar` down to L1 or into a shared types package both layers import.
3. Move foreground extraction out of the real-time reader: the reader yields packets, the pipeline extracts.
4. Delete the unused `*db.DB` field in `listener.go`; delete the L6 re-exports of L8; give `l4bobserve` the shared types from a package beneath both.
5. Move the LiDAR types `internal/db` uses into `internal/lidar/storage`, removing `db` to `l3grid`.

**Milestone:** v0.5.8

### Item 3: the frame model and the recorder

**Summary:** `FrameBundle` and its companions move to a transport-free package; `l9endpoints` converts to protobuf and nothing else; the VRLOG recorder moves beside `storage/vrlog`.

**Milestone:** v0.5.8

### Item 4: embeds, defaults and the tuning client

**Summary:** split the root embed package into assets and defaults; inject defaults from the composition root and delete the `../..` walk; read `LIDAR_TIMESTAMP_MODE` once at the root; move the tuning HTTP client out of `internal/lidar/server`; give `device` a backup that does not need `internal/db`.

**Milestone:** v0.5.8

### Item 5: a `nopcap` build, and host0004

**Summary:** a Make target that builds the binary without the `pcap` tag for hubs and clients; the pure-Go reader built behind a build tag and measured against libpcap by host0004.

**Steps:**

1. `build-velocity-nopcap` (named under the Make trim's rules) builds and passes the role gate for hub, device and jobs.
2. The `pcapgo` reader, the decoded port filter and a self-check that needs no libpcap, behind a tag.
3. Run host0004; adopt or reject under its rule; if adopted, restate D-26 and retire the vendored libpcap and the static Docker build in one release.

**Milestone:** v0.5.8 for the target; host0004's verdict decides the rest

### Item 6: lifecycle

**Summary:** a `Service` interface with `Run(ctx) error` and an errgroup supervisor in the serve root; ingest started by the supervisor, never by an HTTP handler; the transit worker moved from `internal/db` to `internal/radar`.

**Milestone:** v0.5.9, with the merged cmd-server decomposition

## Failure registry

| Component          | How it fails                                | What happens                                                                       |
| ------------------ | ------------------------------------------- | ---------------------------------------------------------------------------------- |
| The gate           | A legitimate new edge is refused            | The PR adds a reviewed row to the roles table; the change is visible in review     |
| The gate           | `go list` cannot load (missing `web/build`) | The test uses the same stubs as `lint-go`; Item 4 removes the need                 |
| A moved package    | A cycle appears during a move               | Moves land one edge per PR; the gate's allowlist shows what is left                |
| The pure-Go reader | Packets differ on a capture libpcap reads   | host0004 fails, the libpcap path stays the default, and the difference is reported |
| The `nopcap` build | A hub is asked to replay a capture          | The command fails with the existing "rebuild with -tags pcap" message              |

## Dependencies

- Item 1 precedes the others: it is what keeps them from regressing.
- Item 3 precedes the vocabulary cutovers' Go renames, so the renamed types land once, in their new package.
- Item 5's adoption depends on host0004; host0001 measures what the role builds cost.
- Item 6 rides on the merged cmd-server decomposition plan.

## Risks

| Risk                                                 | Likelihood | Impact | Mitigation                                                        |
| ---------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------- |
| Moves collide with the perception branches in flight | High       | Medium | One edge per PR; mechanical moves; the gate shows progress        |
| The gate is too strict for a real need               | Medium     | Low    | Widening a rule is a one-row, reviewed change                     |
| The pure-Go reader is slower on the Pi               | Medium     | Low    | host0004 measures the Mac and the Pi before anything changes      |
| A role split is attempted before the closures hold   | Low        | High   | Splitting binaries stays out of scope until host0001 says it pays |

## Checklist

### Complete

- [x] Closures and layer violations measured on `acdb03e`
- [x] host0001 (role build cost) proposed

### Outstanding

- [ ] Item 1: the closure and layer gate (`S`)
- [ ] Item 2: the alias package and the layer edges (`M`)
- [ ] Item 3: the frame model and the recorder (`M`)
- [ ] Item 4: embeds, defaults and the tuning client (`S`)
- [ ] Item 5: the `nopcap` build and host0004 (`M`)
- [ ] Item 6: lifecycle supervisor (`M`)

### Deferred

- [ ] Separate binaries per role: only if host0001 shows a role build at least a quarter smaller and a sixth lighter at idle on the Pi 5

### Accepted residuals (no action planned)

- [ ] `cmd/tools/*` stay dev-host tools outside the role table, as D-26 already says
