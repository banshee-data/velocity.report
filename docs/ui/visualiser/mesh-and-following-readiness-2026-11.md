# Road mesh and five-site following review readiness

- **Status:** Five paired reviews verified; mesh repair and physical qualification remain pending
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

Both frozen preparer builds used `ObservationMaxSamplePoints: 256`, a fixed per-cluster
evidence-retention policy separate from the tuning file's `max_sample_points: 0`. The
paired VRLOG retains every foreground point produced by L3 without that cap. L5 box and
solid-body estimation can consume retained points, so the evidence policy is not
universally storage-only. The producer build and this policy are part of provenance;
a matching tuning fingerprint alone does not establish equivalence with older runs.

The frozen parameter file is `config/experiments/weekend-following-baseline.json`, with
exact file SHA-256 `9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`
and canonical parameter fingerprint
`sha256:ab0c4035780dcc678bc21b112890c1db6cdc9c802eb3d1ab5d0f8d442e872d08`.
The calibration is `calibration/v1/30226f66f62ba71b24694c07b3793ab761657aa67d43b929c4cb33ad6d099e28`:
sensor `hesai-pandar40p`, sensor frame to sensor frame, identity transform. The recording
header's renderer frame label, `site/hesai-pandar40p` with reference `ENU`, does not
establish a surveyed transform. Physical qualification must use the explicit calibration
declaration and separately verified site placement.

Kirkham's registered preparation was produced by
`c6a4f98bfecce8d9b7f6d67555df52ed488bf265`. Ashbury and the other three sites were
produced by `0d9e55fbe7eec91c3468b361046e60248022b396`. Both report
`0.5.1-pre52`; their distinct build identities remain in the paired header and evidence.
The visualiser's later main build is a separate identity from these data producers.

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
| Kirkham–9th        | `2864ebde38e736b496d33361e9bcdc9246aa5147459ec48aee0f8f11f1f58b9a` | development     |
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
recorded observation-count states for the run browser, which may include coasting. These
browser summaries are not the following analyser's estimator input. No human labels were copied or
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
fsync dominating execution. An intermediate change selected a bounded 64 MiB page
cache per connection and a 64 MiB WAL checkpoint interval. WAL/NORMAL durability and atomic
frame transactions remain unchanged. This is an I/O scheduling change, not a perception or
following parameter change; tests retain the observation-on/off replay comparison.

That intermediate policy still produced long pauses on the full Ashbury capture as its WAL
reached about 65 MiB. The revised offline policy uses a bounded 512 MiB cache, disables
automatic checkpoints during the finite replay, and requires a successful `TRUNCATE`
checkpoint after all frames and refinement writes finish. Export refuses both query errors
and an incomplete/busy checkpoint. WAL/NORMAL keeps frame transactions consistent; the
unpublished power-loss recovery window is longer until the final sync. The original capture
and recordings are preserved. SQLite documents the [checkpoint sync and truncation contract](https://www.sqlite.org/c3ref/wal_checkpoint_v2.html)
and [NORMAL-mode sync behaviour](https://www.sqlite.org/pragma.html#pragma_synchronous).
Tests cover a pinned reader, a closed database, failure propagation and unchanged Kirkham
replay scores with and without observation storage. Operational timings on this shared host
are not an isolated performance benchmark or evidence of physical measurement accuracy.

## Offline launch and source handover

A final native check on main `45312b2` exposed a separate source transition fault:
choosing Kirkham started its PCAP analysis, and opening a prepared VRLOG left that
packet producer running behind the recorded playback. The status named VRLOG while
recording remained active with no recording run ID. The replacement now serialises
operator source changes, cancels and waits for the PCAP finaliser, stops UDP input,
and drains pending frame callbacks before resetting state and loading the VRLOG.
It keeps the live socket closed throughout the replacement. A parked replay watcher
rechecks its source and cancellation after acquiring the transition lock, so an old
watcher cannot stop a newer recording. Concurrent stop/load, callback lock ordering,
load failure and stale-watcher regressions exercise this boundary.

A second check opened Ashbury from Runs while Kirkham was selected. The new recording
loaded, but the toolbar, placement hint and resident reference still belonged to Kirkham.
Successful run loading now replaces the complete site context using the run's recorded
clip-to-site binding, without starting another PCAP. Missing, conflicting or ambiguous
bindings clear the old site rather than inheriting its geometry. A refused load preserves
the current context. Regressions cover the different-site and unresolved-site cases.
The recorded clip's sensor and eye-read coordinates are retained for placement; missing
or invalid coordinates leave the site unselected. The browser restores its new selection
after the context reset. A request cancelled while draining the previous producer restores
live input without starting the new recording.

The private local city index now includes Kirkham–9th alongside the existing public
entries, using its already registered Gerardus scan and accepted pose. Its five focus
capture bindings name the exact requested current sources. The private index and its
receipt live on the attached evidence volume; this does not publish Kirkham geometry
or accept a new pose. The native folder chooser grants access to that local root.

Make-built main `45312b2` launch binaries were independently copied and hashed at the
normal internal launch paths, retaining the originals on the attached volume. This
avoids cold executable paging competing with the observation database on the rotating
disk. A first catalogue request took 4.8 ms after this change, compared with earlier
4–6 s requests that could exceed the native lookup timeout. These shared-host timings
are operational observations, not an isolated benchmark. Required data remain on
`/Volumes/lidar`, with NAS shares unmounted.

Ashbury's prepared recording independently decoded 11,574 foreground frames and
9,040,486 foreground points, with zero background points and 1,516 recorded track IDs.
Its matching canonical export contains 31,000 rows projected into 20 minute slices:
610 supported closing samples, 723 eligible non-closing samples, 11,781 no-leader rows,
15,018 excluded pairings, 2,587 record gaps, 187 ineligible following rows and 94 rows
with missing inputs. The tracks API returns all 1,516 exact recorded IDs. These counts
establish output completeness and software status, not physical measurement accuracy.
Columbus's preparation subsequently completed and independently decoded 19,629 foreground
frames, 111,885,204 foreground points and 8,631 recorded track IDs, with zero background
frames. Its 459,139 rows project into 33 minute slices, each below 48 MB. The row buckets
contain 323,783 excluded pairings, 30,059 record gaps, 90,866 no-leader rows, 8,267 excluded
following rows and 6,164 eligible following rows. Of the eligible rows, 2,369 have a supported
closing calculation, 3,793 are non-closing and two have invalid DRAC input. A further ten
excluded rows retain a supported calculation; their calculation status does not make the
interval eligible. The API returns all 8,631 exact recorded IDs. These are sample counts,
not exposure durations or physically qualified findings.

Bryant's preparation then independently decoded 12,924 foreground frames, 37,056,415
foreground points and 8,937 recorded track IDs, with zero background frames. Its 213,147
rows project into 22 minute slices: 133,421 excluded pairings, 21,685 record gaps, 47,072
no-leader rows, 4,552 excluded following rows and 6,417 eligible following rows. The eligible
rows contain 1,973 supported closing calculations and 4,444 non-closing cases. Exclusions
retain 3,624 ineligible cases, 914 missing-input cases and 14 unresolved positive gaps.
The API returns all 8,937 exact recorded IDs.

Lombard's final preparation independently decoded 11,318 foreground frames, 48,473,684
foreground points and 6,144 recorded track IDs, with zero background frames. Its 159,265
rows project into 19 minute slices: 103,098 excluded pairings, 12,026 record gaps, 31,176
no-leader rows, 5,189 excluded following rows and 7,776 eligible following rows. Eligible
rows contain 3,417 supported closing calculations, 4,306 non-closing cases and 53 invalid-input
cases. Excluded rows retain 3,369 ineligible cases, 1,342 missing-input cases, 88 supported
calculations, 240 invalid-input cases, 127 non-closing cases and 23 unresolved positive gaps.
The 88 excluded calculations remain excluded. The API returns all 6,144 exact recorded IDs;
2,537 have durable estimator metadata and 3,607 use recorded browser summaries.

All five prepared reviews have passed independent file and API verification, followed
by native replay, reference loading and matching evidence import:

| Site               | Registered review run                                   | Frames | Foreground points | Recorded IDs | Evidence rows | Minute files |
| ------------------ | ------------------------------------------------------- | -----: | ----------------: | -----------: | ------------: | -----------: |
| Ashbury–Downey     | `following-review-a5254a5f-82c5-4c69-a3a3-0c40f8a9fde1` | 11,574 |         9,040,486 |        1,516 |        31,000 |           20 |
| Kirkham–9th        | `following-review-9ea0ede2-e4fc-48b7-a6a0-9f84fddd6202` |    631 |         2,574,168 |          136 |         5,624 |            2 |
| Columbus–Broadway  | `following-review-b71ec25e-18ee-477e-994b-7b4e09cca73d` | 19,629 |       111,885,204 |        8,631 |       459,139 |           33 |
| Embarcadero–Bryant | `following-review-9e777570-8286-4f3a-be91-74e913b4513d` | 12,924 |        37,056,415 |        8,937 |       213,147 |           22 |
| Lombard–Broderick  | `following-review-9cfda4b3-358c-4a8d-a096-637e79342fc7` | 11,318 |        48,473,684 |        6,144 |       159,265 |           19 |

The header and canonical-export identities are frozen independently of the minute files,
whose hashes remain in each complete preparation's receipt:

| Site               | Recording-header SHA-256                                           | Canonical-export SHA-256                                           |
| ------------------ | ------------------------------------------------------------------ | ------------------------------------------------------------------ |
| Ashbury–Downey     | `525cd8bdb96969cd4342d7685e884d6bb11e2a0fc004f23088f94530de72bece` | `6189612a1ef2e36efffef03be89907f967398a3faf65bafa55ebf5b448ba12ac` |
| Kirkham–9th        | `97d4e41070e6ce9ab67fd415c619fa190a98dc8b8c0dd45e176f105ac94c88a3` | `3365f3d5b615a1313493be7c118d97a2094e841d18fb81a6f5093590a219ace9` |
| Columbus–Broadway  | `7052f58cd16bb2fbfb6aeaf3cb6f1d8218bcb9240d340cda209606fa57b5a65c` | `76cf7427c14afad57114d95a8daa50430fcd930fb481327e7b71af39fae50023` |
| Embarcadero–Bryant | `ed2fcef7cd27fcf9f0c51ca02a309814b2b1d8d61d3d9f71655e0c29b1e42e0a` | `b961ff87666c2cddf2f70ad5e2cee44a05d3eb29efd82ca32d02dfa3dd83b427` |
| Lombard–Broderick  | `2f5885d0edacf4fff160a04d0c18f0bdb0c21e9fc79cb75fa6aea6becb28dbf0` | `867c595c0df057c331f7ec57b6f5c4dbb46ecc071d2eb3a3585647b17e58a70f` |

After the four paired native checks, the operator remounted the NAS and asked that its
shares stay mounted. This task did not change those mounts. The final mount inspection
subsequently showed no NAS shares. Lombard's reference and paused evidence review were
rechecked in that observed state, before saving its final receipt. Earlier snapshots remain
alongside the completed checks; the task does not attribute the mount change to itself.

### Merged-main offline check

PRs #1048 and #1049 passed CI and merged, followed by the review, preparation and source
handover fixes through #1053. Make-built Go and native binaries from main
`f9a6ba362e07bd5e97014a07cf42f88a4789b06d` reported that SHA in the API and About dialog.
Their independently hashed archive remains on the attached volume; the archived app passed
deep, strict signature verification. All returned workflow runs for #1053 succeeded, and
there were no outstanding review threads at merge.

With NAS shares unmounted and Gerardus restricted to local data, all five sites loaded
their foreground replay, buildings and accepted scans in that merged app:

| Site               | Replay checked                   | Buildings | Roof planes | Walls | Accepted scans | Scan points |
| ------------------ | -------------------------------- | --------: | ----------: | ----: | -------------: | ----------: |
| Ashbury–Downey     | New paired review, 11,574 frames |       349 |       1,110 | 5,641 |              2 |      87,299 |
| Kirkham–9th        | New paired review, 631 frames    |       301 |         847 | 5,011 |              7 |     330,865 |
| Columbus–Broadway  | New paired review, 19,629 frames |       277 |         961 | 5,077 |              1 |      36,611 |
| Embarcadero–Bryant | New paired review, 12,924 frames |        41 |         156 |   796 |              1 |      50,128 |
| Lombard–Broderick  | New paired review, 11,318 frames |       266 |         669 | 5,144 |              1 |     117,901 |

Kirkham selected its registered accepted scan `sha256:9f547025e08237d02163c19d5a6f7e3caa20e513e7283ae974581573e410bbb3`
and displayed the accepted 235.3° heading. Each Runs selection replaced the site and placement
hint; **Align → Place from hint** loaded that site's reference. These checks establish
offline availability and site selection, not acceptance of the road mesh. Lombard's earlier
parent-replay check and the later paired-recording check both remain in the local evidence.

All five sites imported their matching minute files on merged main,
selected an actual recorded track, sought to an exact sample timestamp, exposed classification
and quality controls, and opened the manual audit form. Each form was cancelled with zero
operator records saved. Both members of each inspected pair were independently decoded
in that exact recorded frame.

Ashbury's illustrative first-minute sample displayed a 13.5062353 m gap, 1.2915821 s net
time gap and 0.33977255 m/s² experimental DRAC at capture timestamp `1788290848418901330` ns.
It was selected to exercise numeric display, not to characterise risk or exposure.
Kirkham's positive-duration sample displayed a 30.8390454 m gap and 29.7311826 s net time
gap at `1764973088326342330` ns, with numeric demand correctly suppressed because the pair
was non-closing. Across Kirkham's three eligible samples, one has zero
duration and the other two sum to 0.199921 s of eligible exposure. Three isolated samples
must not be described as three intervals of exposure.

Columbus's first-minute sample at `1788380529986405330` ns displayed a 47.141 m gap,
5.6954 s net time gap and 2.0795e-05 m/s² experimental DRAC. Bryant's first recorded frame,
`1788471707925341330` ns, displayed 13.565 m, 1.1736 s, and 0.0044617 m/s² respectively.
Both selections used a positive-duration eligible pair, preferring a supported calculation
and the earliest sample time in the first minute. Their exact follower and leader IDs were
decoded in recording offsets 229 and 0 respectively, original frame IDs 931 and 702.
The audit forms retained the original interval ends, and were cancelled without saving.

Lombard's sample at `1788367172526609330` ns, recording offset 3 and original frame 704,
displayed a 37.062 m gap, 3.534 s net time gap and 7.9717e-06 m/s² experimental DRAC.
Both exact track IDs were independently decoded in that frame. The positive-duration
eligible sample exercised numeric display; it does not characterise risk or exposure.
Its audit retained the exclusive end `1788367172626615330` ns and was cancelled with
zero operator records saved.

One UI request timed out during Bryant's reference load. Two native control requests
also timed out while Lombard was streaming. Pausing through the gRPC API and re-reading
the window recovered the check without restarting the app; the reference loaded. These
checks establish loading and paused review, not a playback frame-rate guarantee. A Columbus
paint-pane request for the current recording's lane field also timed out. No passed result
is claimed for that route or for lane inference: the following checks here use the separate
Inspector import and track-derived local paths.

The native view correctly reports that recording source identity is unavailable: imported
files are manual review evidence, without automatic playback binding or overlays. Header,
source, configuration, timestamp and exact track-ID agreement were checked independently.
Online estimates retain `estimate_not_final`; suppressed named metrics are distinct from
the baseline arithmetic shown for inspection. Neither successful import nor a numeric DRAC
value supplies physical identity, common-path, endpoint, speed or uncertainty qualification.

## Validation and remaining work

The native suite passed with 858 XCTest cases and 2,000 Swift Testing tests after the review
entry, stale-selection, coverage and slice-decoding changes. The foreground-recording regression
checks actual point filtering and leaves the default replay payload unchanged. Preparation tests
cover exact recording bounds, unchanged source samples, slice partition boundaries, retained
straddling intervals, source-site mismatch and annotation-only validation-cohort admission.

All five complete recordings have independently verified source/header/export identities,
slice coverage and recorded track sets, exact clip registration, API checks and native review
receipts. The local review index exposes the five complete bundles and matching minute files.
Keep mesh failures visible until the road generator
provides non-overlapping junctions and valid boundary rings. No measured following accuracy,
DRAC threshold, lateral-risk interpretation or physical qualification has been adopted.
