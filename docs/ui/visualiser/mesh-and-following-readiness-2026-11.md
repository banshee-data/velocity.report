# Road mesh and five-site following review readiness

- **Status:** Implementation verified; full five-site preparation and native recheck in progress
- **Scope:** Ashbury–Downey, Kirkham–9th, Columbus–Broadway, Embarcadero–Bryant and Lombard–Broderick
- **Related:** [Operator workflow](following-audit-workflow.md), [weekend plan](../../plans/lidar-weekend-following-drac-plan.md), [workspace audit](workspace-ux-audit.md)

The operator reported mesh-audit failures and no accessible way to label tracks or review
following distance, headway and experimental DRAC. These are separate readiness questions:
loading geometry does not establish usable annotation or physically qualified measurements.

## Mesh findings

The inspection used main `82487ea234f29a20846cfed13bb25790da3b740d` and the local
Gerardus `0.2.0+683b09dac4bc`. Its Ashbury corridor has build
`18f2f592c4b1894b1864e3097862b31e1a3af202f3e8fd93bffc645a4801931e`.
The exact streamed patches were audited before and after replacing Float separating-axis
projections with nearby-origin Double arithmetic:

| Check                                               | Both implementations |
| --------------------------------------------------- | -------------------: |
| Resident patches                                    |                   21 |
| Triangles overlapping within a patch                |                    0 |
| Triangles overlapping another patch                 |                4,474 |
| Near-coincident vertex pairs in the retained stream |                1,036 |
| Largest seam-height step                            |         0.4348755 mm |

The screenshot's 0.4 mm height step is below the 5 mm limit. All 4,474 overlap warnings
remain with the more precise arithmetic. They are real intersections between the preview's
separate whole-street strips, which the generator describes as having unreviewed junction
overlaps. The Ashbury Terrace ring also fails meshing because boundary edges cross. No
replacement road geometry or accepted junction was manufactured during this investigation.

A separate translated-geometry regression reproduced 414 false overlap reports in 1,000
adjacent triangle pairs around CS13-scale coordinates. The arithmetic repair fixes that
defect; it does **not** repair the Ashbury intersections. The audit must continue reporting them.

The server supplies no inventory denominator for this site corridor, alongside 1,958.3362 m
surfaced, 1,991.5801 m with inferred widths and 33.24394 m missing. The viewer now reports
unknown coverage rather than presenting that missing denominator as 0% coverage. Inferred-width
extent is stated independently of surfaced extent. Pending mesh checks no longer say there are
no resident patches. Neither wording change qualifies the geometry.

## Annotation and review connections

Inspector now leaves Paint streets, preserving unsaved marks and restoring track picking with
one review column. The painter also offers **Review tracks / following…**. Classification and
quality flags precede following evidence in the Inspector. A client joining an existing replay
can identify its unique run without restarting or seeking; changing playback during that lookup
refuses the binding. A successful run switch clears the previous track selection; a refused
load preserves it. This prevents an old track appearing under the next run's label context.

The merged app exposed labels and flags after selecting an Ashbury track. Those controls alone
do not establish a saved human classification, and no classification was invented for testing.
Ashbury and Kirkham loaded their foreground recordings, buildings and accepted scans with the
NAS unmounted. A later Columbus replay made both clients unresponsive while streaming; the
process sample showed SwiftUI graph updates, including Alignment's site selector. That sample
does not establish a root cause or a successful five-site native recheck. The recording header
retains valid capture bounds although the HTTP playback-status response showed zero bounds.
The status accessor read the PCAP clock even for VRLOG replay; zero HTTP bounds
therefore did not establish lost recording metadata. It now reads a coherent recording
snapshot, including bounds, position, pause and rate without advancing playback.
An obsolete live-toggle test also sent
real replay-stop requests to the local service. It was removed in favour of the existing
stubbed regression, and native checks are kept separate from test execution.

Foreground recordings with a complete header declaring no background frames now skip the
startup scan that previously decoded the entire file looking for a nonexistent snapshot.
Legacy headers retain the scan. Startup seek failure also releases its wait-group count
before stopping, preventing a load-request deadlock. Tests cover both paths and skipped
point-buffer release.

## Preparing source-matched evidence

The main replay database contained no persisted state estimates. Its existing recordings
cannot supply those missing estimator rows retroactively. The fixed-purpose
[review preparer](../../../cmd/tools/following-review-prepare/README.md) therefore creates a new
paired recording, immutable observation database and following export from each explicit source.
It retains the original run and names it in the new binding. New track IDs belong to the new
producer; labels and old following exports are not copied across versions.

The preparer uses the checked-in `config/experiments/weekend-following-baseline.json`, with no
experiment flags or parameter adoption. It requires settled background before recording and
records through EOF after an explicit warm-up prefix: 20 s for Kirkham, 70 s for the other sites.
That prefix remains in the evidence database but is excluded from the export's trajectories so
every exported sample is covered by the paired recording. These review recordings are explicitly
bounded; the original complete foreground recordings remain available. Data from overlapping
windows is not additional independent exposure.

| Site               | Parent run                                             |  Source bytes | Unrecorded warm-up |
| ------------------ | ------------------------------------------------------ | ------------: | -----------------: |
| Ashbury–Downey     | `f718874e-5c25-4e68-af4b-d128a292b385`                 | 2,960,366,452 |               70 s |
| Kirkham–9th        | `cac4f6e4-441b-4c22-9f06-080428cd1261`                 |   200,657,872 |               20 s |
| Columbus–Broadway  | `8d884a74-6c0a-4666-a873-5c2181c28207`                 | 4,903,944,004 |               70 s |
| Embarcadero–Bryant | `9f4a5f41-ec66-4a24-9b50-7a432146209f`                 | 3,286,337,080 |               70 s |
| Lombard–Broderick  | `replay-052fa966-5cdf-4792-9186-7fefc4682e29-1c36dd9e` | 2,898,786,944 |               70 s |

All paths resolved to the attached LiDAR volume at preflight. The calibration declaration is
the identity within the sensor frame, where the offline pipeline actually leaves the points;
it is not a surveyed site placement. The separate accepted Gerardus pose places the recording
for viewing. Source, calibration, analyser and recording-header identities remain separate.

Preparation is admitted as `annotation_review`, with a durable use-ledger record. It is not a
development evaluation or tuning run. Lombard remains in the validation cohort; a regression
verifies that reviewing it does not allow a subsequent developer-diagnostic read. The first
attempts retained explicit refusals for missing or conflicting full-capture byte identities.
Revision 3 subsequently pins the four requested full captures to their existing site
assignments; no site's cohort changes. Kirkham's bytes were already pinned.

### Full-capture identity reconciliation

The user's request covers the five current recordings, whose parent-run, clip and catalogue
site bindings are listed above. These full sources differ from the older screening inputs
already in the cohort manifest. Their exact bytes now have separate capture entries:

| Site               | Full-capture SHA-256                                               | Existing cohort |
| ------------------ | ------------------------------------------------------------------ | --------------- |
| Ashbury–Downey     | `2eec35d381fa4c4ba5a6bf1218982c76e53749287d53691115261f14d8aefbed` | development     |
| Columbus–Broadway  | `aa69875be4dd45094d79c371ca25e434b3a4d8bec0966c116931720cc1653b66` | development     |
| Embarcadero–Bryant | `03e10c86b718f068c8733559b1020b56a4e7ae1ec0361529f4fb19e75090fdf3` | development     |
| Lombard–Broderick  | `1eb08d9e6ced4e20bf07db2140434fbff314b779beb909d602929d44b4c34643` | validation      |

The bindings use the existing parent run's exact source path and clip, plus the catalogue's
S2 L16 site token. Names alone do not establish the assignment. The earlier Ashbury pin
`072e9801…` is retained. Its current `s2-sf-0.pcapng` contains 2,209,822 timestamped packets;
the local named street archive contains 2,209,786 and hashes to `0f6811cd…`. Their ordered
packet hashes differ, and the current source ends 19.959 ms later. They remain distinct
sources; the archive was not substituted for the requested current recording.

The preparer's use ledger now records the exact tuning-file SHA-256 and producer build,
alongside the policy digest. The first Kirkham preparation's older ledger entry retained a
config path and omitted its build fields; its recording header and canonical export retain
the producer and configuration identities. Those historical ledger rows are preserved.

Kirkham's first prepared review independently decoded 631 foreground frames, 2,574,168
foreground points and 136 recorded track IDs over 62.998697 s after its warm-up. Its 5,624
rows include three eligible following samples, all non-closing, so none supplies a closing
DRAC calculation. Excluded samples retain their reasons. Seventy-three recorded tracks have
durable estimator metadata; the other 63 use recorded kinematics and box means over distinct
observed updates within the recording for the run browser. No human labels were copied or
invented. The tracks API returns all 136 rows after registration.

The full canonical export is retained locally. One-minute `following_review_slice_v1` files
project its exact rows, candidate decisions, path IDs and analysis version for the native
viewer's 1 GiB import limit. They carry the full export's SHA-256, recording-header hash,
recording producer, calibration ID and exact sample-anchor bounds. Original intervals may
straddle a slice boundary; they are not clipped, interpolated or re-evaluated. An audit binds
the exact slice bytes. Software consistency checks do not supply physical endpoint, speed,
identity, common-path or uncertainty validation.

The older Columbus baseline export was found as a local symlink into the unmounted NAS archive.
Its paired recording is local, but that export cannot be called an offline dependency. The new
preparation keeps all required outputs on `/Volumes/lidar` and preserves the older archive.
Raw streams, logs, process samples, frozen job plans and output receipts stay outside Git.

The first Kirkham preparation used the default SQLite cache and checkpoint spacing on the
same attached rotating volume as its capture. Progress slowed from 200 frames in 23 s to
200 frames in almost six minutes as the database grew. A process sample showed reads and
fsync dominating execution. Subsequent offline evidence writers use a bounded 64 MiB page
cache per connection and a 64 MiB WAL checkpoint interval. WAL/NORMAL durability and atomic
frame transactions remain unchanged. This is an I/O scheduling change, not a perception or
following parameter change; tests retain the observation-on/off replay comparison.

## Validation and remaining work

The native suite passed with 858 XCTest cases and 2,000 Swift Testing tests after the review
entry, stale-selection, coverage and slice-decoding changes. The foreground-recording regression
checks actual point filtering and leaves the default replay payload unchanged. Preparation tests
cover exact recording bounds, unchanged source samples, slice partition boundaries, retained
straddling intervals, source-site mismatch and annotation-only validation-cohort admission.

Complete and independently decode all five prepared recordings, verify source/header/export
hashes and slice coverage, register their actual tracks and clips, then repeat native playback
and evidence import with the NAS unmounted. Keep mesh failures visible until the road generator
provides non-overlapping junctions and valid boundary rings. No measured following accuracy,
DRAC threshold, lateral-risk interpretation or physical qualification has been adopted.
