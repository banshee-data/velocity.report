# Lombard–Broderick and Columbus–Broadway review readiness

- **Status:** Partial workflow; complete recordings and native track checks passed; accepted lanes and native evidence import remain blocked
- **Scope:** Native visualiser discovery, recording provenance, and the existing offline following baseline
- **Related:** [Lane painter plan](../../plans/lidar-visualiser-lane-painter-plan.md), [lane snapshots](../../plans/lidar-lane-snapshot-and-attribution-plan.md), [following exporter](../../lidar/operations/following-evidence-export.md)

The requested first site was clarified from Broderick–Laguna to **Lombard at Broderick**.
Columbus at Broadway is the second site: both it and Embarcadero at Folsom have mounted captures
and short recordings, but Columbus also has the archived following export and a
[measured lane-field report](../../lidar/operations/lane-field-columbus-broadway-2026-11.md).
This is a readiness and software review, not physical qualification of a following metric.

## Ownership and starting state

The main checkout was at `0359442e6`, three commits behind the refreshed `origin/main`
`16fd8287a`. Its uncommitted Analysis Runs pagination and site-label changes were preserved.
The owning Codex task, **Fix visualiser analysis runs**, was contacted. **Build visualiser from
main** confirmed its audit was complete and released UI control. Its open
[PR #1042](https://github.com/banshee-data/velocity.report/pull/1042), head `1c5f2d78b`, already
contains those changes and the road/building frame correction. This review uses that head in an
isolated checkout on `codex/visualiser-following-review`; it does not duplicate the earlier edits.

The other open PRs at inspection were #1043 (tenet review) and #503 (Tailscale grants).
Concurrent interaction and offline SRAC scope had already merged as #1044. The Codex task
**SRAC and concurrent interaction candidates** was contacted about the exporter boundary.
No process named Hydra and no local Hydra registry were found. The app task inventory and process
inspection do not establish the state of every external worker or Claude conversation.

The root owner subsequently consolidated this review's three commits through `b9109a19f`
into #1042 and built its Release app. Stacked #1045 consequently merged into that branch;
it is not evidence that #1042 has landed on main. Further replay metadata work is isolated
on `codex/clip-replay-metadata-review`, based on that consolidated head.

## What exists

The native painter already bridges the traffic field and vector street-use features over the
placed reference. It can load run/site traffic, approve or reject proposals, edit authored
features, save street use to Gerardus, and then send a versioned snapshot to velocity.report.
`LaneSnapshotSender.swift`, `StreetPainterSession.swift`, `site_lanes_api.go`, and
`lane_snapshot_store.go` implement that write path. The two writes are separate: Gerardus first,
then `PUT /api/lidar/sites/{l16}/lanes`. A visible line or saved Gerardus feature is not proof that
the second write succeeded.

`lanes.History`, attribution, `lanegeom.Project`, and `RouteDistance` already provide versioned
lane selection, projection, tangents, and directed distance. They are not wired into the offline
following exporter. Its paths are fitted from tracks and claim no lane or route semantics.
The existing physical Compare pane explicitly uses a projected chord: it is not an along-path gap.

The live database inspected contained two lane snapshots, both at `808580aab`, with 18 lane rows
across those versions. Neither requested site had a velocity.report snapshot. No accepted lane
snapshot was manufactured or adopted for this review. Counts and turning movements were not
invented.

A later read of Gerardus's `street_use` GeoJSON layer in the site frame returned zero features
at both `808580d49` and `808580f47`. This does not mean the street/building reference is absent.
It means there is no existing authored street-use set to adopt at these sites. A credible lane
set still requires review against the placed reference and recorded points before acceptance.

## Available sources and recordings

The site-corpus manifest is mounted under `sf-street-speeds`; the original captures are also
available under `pcaps/s2`. Source names are discovery aids. Match capture hashes, calibration,
window, producer configuration, and track identities before joining an analysis to a recording.

| Site              | Recording run                          | Coverage and recorded provenance                                                                                                                                                |
| ----------------- | -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Lombard–Broderick | `bcab4cd1-cfb5-4821-9cea-3a39840bb8b9` | Complete 1,201.807364 s foreground recording, 12,060 frames; `1788367102219220330`–`1788368304026584330` ns; producer `0.5.1-pre52`, `091e21ed8dca3105a902b052cf9d35c8edac345b` |
| Lombard–Broderick | `d6f820e9-6f9f-43e9-8b7d-8fac67fc2b18` | Short full-frame recording, 89.907051 s; producer `0.5.1-pre47`, `ab787541c3cd9bfb2ac08422f88867caea3d1fed`                                                                     |
| Columbus–Broadway | `9711b678-3e3e-42c8-9380-33533cac334f` | Short full-frame recording, 89.90030303 s; useful for point/reference inspection, not interchangeable with the following export                                                 |
| Columbus–Broadway | `aadfe835-dc49-48a3-979b-cb81e8264973` | Older 299.900693 s foreground recording; `1788380436986210330`–`1788380736886903330` ns; producer `0.5.1-pre33`, `339548fbe88926851b93178f09c3155d39e7957e`                     |

The full Lombard recording names original source `s2_sf_2_20260902093822_00003.pcap`.
Its configuration hash is `sha256:5b0d3593fb36bc8206ce56af9f4c5299e992373d76795c111d8c0d68a5203dfb`;
parameters are `sha256:c927ea3afe40a1be0fb1812083763d05a37269c7bfe6fe1098f91866fdec570d`.
A newer complete run, `40be090f-d38c-46cb-a2eb-f3d871d8373a`, was also present with producer
`0359442e6d33d4de50df01fcfa0a8277736aecad`; it is another version, not extra exposure.
The older run's header calls the frame `site/hesai-pandar40p`, reference `ENU`. That label alone is not a
site-to-capture binding. At first inspection these database runs lacked a clip association.
The Analysis Runs owner subsequently began provenance-based relinking and separate four-site
weekend recordings in the original database. This review's recordings use the dedicated database;
different versions of the same capture are not additional independent exposure.

The matching Columbus campaign evidence is archived in
`l8fol0008-20261114-1752/evidence/s2-prefixes/columbus-broadway/export-a`, file
`48378d018519e227bf6f1a996b3ae28deda264d04fe07fdb4da37d4b76891b13.json`.
It identifies source
`source/v2/e1e00f306ab90f361cd360b2d3cb87829675497964f44f071046a26d7232a158`,
with capture-relative offset 70 s, warm-up 70 s, and duration 230 s. Its corresponding campaign
recording is under `s2-prefixes/replay/columbus-broadway/first`, not one of the older runs above.
The recorded scoring bounds are `1788380507086402330`–`1788380736886903330` ns and its producer is
`0.5.1-pre52`, `a46b25547aa5cba81efe0e79b239fa8c9f204180`. Its frames are archived separately;
header presence alone does not establish playback readiness on the running server.

The real export decodes to 66,198 rows. Older nominations in the development-prefix report use
another source/track version; they must not be silently substituted into this export.

### Exact Columbus nominations in this source version

These are post-hoc software review nominations from the retained export, not verified physical
encounters. The supported row is the earliest supported row within the campaign recording bounds,
ordered by timestamp then follower ID; the ambiguous row is the earliest `ambiguous_leader` row
within those bounds under the same ordering.

| Case               | Capture time (Unix ns) | Follower                                   | Candidate(s) and exported result                                                                                                                                                                   |
| ------------------ | ---------------------- | ------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Supported baseline | `1788380529986405330`  | `trk_90a23905-74b9-45bb-8be0-009499d514c4` | Selected `trk_ada24c0c-c564-438a-9094-46725c773675`; gap 47.1412436633866 m, projected follower/leader speeds 8.277048516901223 / 8.232769465754702 m/s, nominal DRAC 0.000020795318685652486 m/s² |
| Ambiguous pairing  | `1788380535986217330`  | `trk_90bc68c8-c492-4251-97a6-0dfd8644ac24` | Competing `trk_04ea913e-3c35-4b56-a28a-0d37e1fe6742` and `trk_90a23905-74b9-45bb-8be0-009499d514c4`; no selected leader or DRAC value (`no_candidate`)                                             |

Inspect at least one second either side with the same producer and track identities. A row's
nominal arithmetic does not verify endpoint orientation, occlusion, continuity, or its path.
The ambiguous row is not automatically a cut-in: lateral overlap and a transition interpretation
still require evidence. No separate hypothesis is presented as an additional physical actor.

### Matching-recording audit

A verified copy of the campaign recording is now under the dedicated review recording root
as `columbus-following-campaign-a46b25547`. Every copied file was SHA-256 checked, including
the two frame chunks reached through the archive's original `frames` symlink. The repository's
VRLOG decoder read all 2,299 frames, matching the header and its start/end bounds. This recording
has no background snapshots. It is a short campaign recording, separate from the regenerated
full-site source-bound runs.

The exact supported timestamp contains both nominated IDs. The leader has `Hits=1`,
`Misses=0`, and cumulative `OcclusionCount=37`; the follower has `Hits=10`, `Misses=0`.
Recorded speed magnitudes are 8.465510 and 8.304206 m/s respectively, whereas the export's
path-projected speeds are 8.232769 and 8.277049 m/s. Different projections explain why those
numbers must not be silently substituted. These records alone do not verify the path tangent
or endpoint orientation. Reacquisition makes this earliest row a poor steady-following example.

At the ambiguous timestamp, both competing IDs and the follower are present. Candidate
`trk_04ea913e-3c35-4b56-a28a-0d37e1fe6742` has zero hits, 19 consecutive misses, and a
0.849 × 0.432 m box. Its trail row lacks support, stage, revision, and source metadata.
The other competing track has 70 hits and zero misses. Preserved distinct IDs establish neither
distinct physical actors nor two eligible longitudinal leaders. The export's absent selected
leader and suppressed DRAC must remain visible. The cumulative occlusion count is historical;
the frame's `OcclusionNone` enum does not undo those misses or prove observation support.

The surrounding two-second windows were decoded for continuity review. This is an identity and
record-content audit, not a visual qualification of occlusion, track breaks, lateral encroachment,
or endpoint/path geometry. The source-level omissions above remain limitations even when a box
is drawn. No cut-in/cut-out label or precise concurrent-candidate metric was adopted.

A subsequent sustained-opportunity search nominated `1788380537686276330`–`1788380540886542330`
ns (3.200266 s), follower `trk_90bc68c8-c492-4251-97a6-0dfd8644ac24`, leader
`trk_90a23905-74b9-45bb-8be0-009499d514c4`. Selection required supported opportunity,
observed front/rear endpoints, follower speed above 2 m/s, projected speed difference below
2 m/s, one retained pair, no sample interval above 150 ms, and duration at least 3 s. These are
post-hoc navigation criteria, not metric acceptance thresholds. Positive closing speed was not
required: steady following need not have a positive DRAC.

The matching recording contains both IDs at every one of the 33 samples. Each has zero misses;
the follower's hits increase from 1 to 33 and the leader's from 87 to 119. At the first sample,
the authoritative export gives gap 14.069651682427434 m, projected speeds
8.091371639925727 / 8.040786156119488 m/s, and experimental DRAC
0.00009093655016020351 m/s². This is the preferred steady-following review nomination, not a
physically verified encounter. It follows a follower reacquisition; wider continuity remains
part of the review rather than being erased by selecting a clean interval.

At that sample the exported net time gap is 1.7388463054895082 s with marginal sigma
0.11320187917772653 s; gap sigma is 0.2581913288901879 m. Leader rear and follower front
arcs are 75.57693449926266 and 61.50728281683523 m. Both endpoints are temporally inferred
from observed support, with converged accumulated extents. Their positions are estimates,
not directly observed physical bumper points. Closing speed is 0.050585483806239395 m/s.
The point retains `estimate_not_final`; DRAC has no exported uncertainty bounds. Marginal
sigmas and converged extents do not establish joint uncertainty, finality, or physical accuracy.

## Native workflow added

**Runs** now searches run IDs, full source paths, source names, recorded clips/sites, sensor,
frame mode, and configuration hashes. Search runs before folding repeats, so an exact older ID
is not hidden beneath another run. Whitespace-separated terms must all match. A filename match
does not create a site association.

**Inspector → Recording review context** shows the chosen site separately from the loaded run,
source, recording path, exact capture time, recording window, coordinate frame, and configuration
hashes. A missing clip binding is explicit.

**Inspector → Following evidence (offline baseline)** opens the existing `following_evidence_v1`
JSON, names its file SHA-256, source, site, binding, policy, analyser, estimator/model/stage,
parameters, and replay-window provenance. It displays exact timestamp rows with an optional
follower ID filter and previous/next exported-sample navigation. **Use frame time** copies the
current time and selected track; it does not seek or bind evidence. An absent exact sample stays
absent, including at track breaks. File loading is off the UI thread, bounded at 1 GiB.

For each row it shows the selected baseline leader or refusal, all distinct candidate track IDs
and statuses, synchronisation, path membership and lateral offsets. The selected point retains
front/rear endpoint arcs, marginal sigmas, support, extent provenance, projected velocities,
net time gap when defined, suppression reasons, and authoritative exported experimental DRAC.
There is no second implementation of the equations in Swift. A lateral offset is not footprint
overlap; a track ID is not proof of a unique physical actor.

The view does not overlay file results on playback. Even matching trail source metadata does not
prove recording and estimator/version agreement. Its source note keeps that distinction visible.
The evidence contract lacks a coordinate-frame ID, recording producer build, accepted lane
snapshot, lateral overlap, and evaluated values for non-selected candidates. Those are stated
as missing rather than filled from the open site's appearance.

## Walkthrough observations and reproduction

1. Start the matching server and native build together. Open **Runs**, search the exact run ID
   from the table, and load it. Select the intended site separately; inspect the recording context
   before interpreting building alignment. Source path and time overlap alone are insufficient.
2. At Lombard–Broderick, **Align → Place from hint** loaded the real reference: 266 building parts,
   669 roof planes, 21 city streets, 35 kerbs/islands, and 202 lane markings. Once the asynchronous
   load finished, Gerardus returned one accepted scan and the existing manually placed record
   from October 29, 2026. The initial empty record state was not the final inventory.
3. The complete foreground run loaded and played with tracks and the reference visible. Pause
   and seek to around 1:59 elapsed for a repeatable inspection position. The pause/seek state was
   observed at approximately 0.0998424413 progress. This is a navigation check, not a verified
   steady-following encounter. Full-frame accumulation was correctly unavailable for that run.
4. For point/reference comparison use the short **Full** recording. Inspect ground/building
   alignment before drawing. Open **Paint streets** to inspect the existing traffic/feature
   controls; save authored features in Gerardus, then send lanes to velocity.report and verify the
   returned snapshot ID. Do not accept a pose just to enable a button.
5. At Columbus, load the matching campaign recording only after making its complete archive
   available through the server's configured recording root. Open the named export in the
   following inspector. Keep the older local recording separate: its configuration and identities
   differ. Inspect a supported point and a suppressed multi-candidate period in the same source
   version, then check both actors against points, boxes, endpoints, and the fitted path.

No steady-following or cut-in/cut-out period has yet been physically or visually qualified across
the two sites. Identity continuity, occlusion, track breaks, front/rear orientation, path fit,
projected speeds, and suppression correctness remain review tasks. UI navigation and deterministic
regeneration cannot supply those findings.

## Smallest remaining engineering connections

- **Lane baseline:** add an explicit lane-analysis adapter using the accepted snapshot's sensor
  geometry and the existing `PathFrame` contract. Reuse lane projection, route distance, attribution,
  `EvaluateFollowing`, and `EvaluateDRAC`. Bind snapshot/feature/row IDs, validity time, accepted pose
  record, coordinate frame, source/calibration/window, estimator/model/parameters, and recording
  producer. Emit a separate lane-derived schema/method so the current track-derived baseline stays
  unchanged. Missing, ambiguous, expired, or incompatible geometry must suppress evaluation.
- **Concurrent candidates:** retain a result per distinct candidate track and relationship
  hypothesis at each instant, not only the selected leader. Preserve synchronisation, observed
  support, candidate status, endpoints, lateral evidence, and explicit refusal reasons. Current
  `encounter.go` evaluates a point only for the selected leader; changing only the UI or copying
  the selected leader's DRAC to alternatives would be wrong. Candidate-specific evaluation and
  accounting need server/export tests before adoption.
- **Counts and movements:** use the same source-bound accepted snapshot and attribution history
  to record entry/exit lane transitions, with track-break and ambiguous-transition exclusions.
  Existing lane attribution is the reusable connection; current traffic-field totals are not
  lane-aligned counts or turning movements. No counts were added during this review.
- **Binding and delivery:** locate or produce matching source/version recordings for both sites,
  preserve their full producer provenance, and expose them as located clips/runs. A reviewed lane
  snapshot for each site is still required. The current painter write path itself is present.

These are the limits of the current result. Longitudinal DRAC alone does not capture lateral
collision risk; neither nominal precision nor repeatability is physical validation.

## Validation

The native test build and test execution passed: 849 XCTest cases and 1,990 Swift Testing tests.
New cases cover searching an older folded run, combined search terms, unknown site names,
exact 64-bit nanosecond lookup, distinct competing identities, absent samples, source mismatch,
unknown schema, conflicting identities, and a suppressed DRAC carrying a value. A standalone
build of the same decoder read the actual 536 MiB Columbus export, including source-manifest
window fields, and extracted the two nominations above. This validates parsing, not the
exporter's mathematics or the physical scene.

The first isolated native build was refused cache writes by the sandbox. A permitted retry
failed when the disk filled during dependency compilation. Only that attempt's generated cache
was removed; the successful retry reused the existing visualiser build cache. Recordings and
archived raw evidence were preserved. The root owner subsequently completed the Release app
build at `b9109a19f`. The isolated server build at `9ee5ea36d` passed through
`make build-radar-local`, with producer stamps verified. The complete server package tests
passed with the `pcap` tag in 93.378 s after the replay notification fix; its focused success,
refusal, and failed-start callback tests also passed. Full repository Go/web suites, signing,
notarisation, and a physical field qualification are outside these claims.

## Recording regeneration and runtime findings

Both complete corpus captures were read and SHA-256 checked before regeneration:

| Site              | Bytes         | Capture SHA-256                                                    |
| ----------------- | ------------- | ------------------------------------------------------------------ |
| Lombard–Broderick | 2,898,786,944 | `1eb08d9e6ced4e20bf07db2140434fbff314b779beb909d602929d44b4c34643` |
| Columbus–Broadway | 4,903,944,004 | `aa69875be4dd45094d79c371ca25e434b3a4d8bec0966c116931720cc1653b66` |

The dedicated review service uses HTTP `127.0.0.1:18080`, gRPC `127.0.0.1:50052`, and
`/Volumes/lidar/reviews/visualiser-20261114/review.db`. The original database and server were
preserved. Replay starts use the located clip endpoint, the complete trimmed capture, and
`config/tuning.defaults.json` applied at server startup. Clip associations retain the operator's
site placement; those coordinates are not surveyed ground truth.

The initial Lombard full-background-per-frame preview was stopped deliberately. Its run is
`replay-373c6369-2aeb-472f-a2f4-b10db190e499-aeeecd2c`, producer `b9109a19f`. Its final header
contains 2,759 rotation frames over only 275.807388 s, despite a `completed` run status. Its
clip description explicitly says **INCOMPLETE full-frame preview**. It must not be used as a
complete site recording or as additional independent exposure.

Its run summary reports 14,777 frames, including the unrecorded settling pass; the VRLOG has
2,769 total frames, including ten background frames. This discrepancy demonstrates why run
summary totals are not recorded-frame counts or unique vehicle counts. Completeness checks
must wait for recording to close, then inspect header/index bounds against the source window.

The isolated Debug client stalled under fast full-frame generation. The Release client also
stalled while consuming fast foreground generation; the server reported slow-client drops.
These drops describe the UI stream, not independently established recording loss. Playback
review must run after generation, at a rate the client can consume. The Release app did show
the new located Lombard run beneath **Lombard at Broderick** with the **FG** badge, separately
from the clearly named incomplete **Full** preview. An active row's zero duration did not prove
completion. Replay timing later arrived in the Inspector, while an unselected run remained
explicitly unavailable in its provenance context.

Clip replay now invokes the existing stream-start callback after a successful launch, matching
file replay and sweep behaviour. It sends no notification when validation or replay start fails.
This repairs the missing metadata notification; it does not resolve client rendering pressure.

The old Release client eventually recovered after Lombard reached EOF, then stalled again
while Columbus streamed. Its following-evidence chooser opened the real export and showed
**Reading evidence…**; that observation does not establish a completed interactive import.
After explicit user approval, Activity Monitor force-quit that client. The recorder continued
independently. No geometry edit or acceptance was made in the interrupted session.

The scrolling track list now uses a lazy stack instead of laying out every accumulated row.
A separate correction makes the low-frame-rate refresh guard accept a backward index immediately.
Previously it treated a rewind as a zero forward gap and could leave cached previous-source rows
until the new source reached the old index. Source reset, large-index and forward-throttle
boundary tests cover the extracted refresh decision. This correction preserves track IDs;
it does not join them into physical actors. Absent evidence coast age is also displayed as
unavailable rather than zero. Performance and interactive review still require the rebuilt
client to be exercised; a build alone is not evidence that the stall is resolved.

The regenerated Lombard clip is `052fa966-5cdf-4792-9186-7fefc4682e29`, located at site
`808580d49`; its run is `replay-052fa966-5cdf-4792-9186-7fefc4682e29-1c36dd9e`. The final header
contains 12,018 rotation frames and 41 background snapshots, 12,059 records in total, spanning
`1788367102219220330`–`1788368303926512330` ns (1,201.707292 s). Every record decoded, with
counts and bounds matching the final header. The end is approximately one rotation before the
manifest's capture endpoint. This is full-duration foreground coverage with periodic background
snapshots, rather than background points repeated in every frame.

Its producer is `9ee5ea36d5550b171e4ed773f3e23fd575b839b8`, version `0.5.1-pre52`, frame
`site/hesai-pandar40p`, reference `ENU`. Configuration hash is
`sha256:bd476c8a43ba61c2a85ea721ba267c1a17d786db01fec36d22cecaab6fcf7101`; effective parameters
are `sha256:c927ea3afe40a1be0fb1812083763d05a37269c7bfe6fe1098f91866fdec570d`.
The checked-in tuning file's bytes hash to
`9ad7377fa5d356ba5f9d8756d48721708071da3295702350c835110b38b41675`; that is distinct from
the effective-parameter hash. The sequential launcher records both, as well as each source hash,
clip/site association, request, final run and recording header, outside Git.

The regenerated Columbus clip is `59a0bb94-41b5-4b1c-b5d4-e1fbca8a1ea4`, located at site
`808580f47`; its run is `replay-59a0bb94-41b5-4b1c-b5d4-e1fbca8a1ea4-5fd7e16b`. It contains
20,330 rotation frames and 68 background snapshots, 20,398 records in total, spanning
`1788380436986210330`–`1788382469811884330` ns (2,032.825674 s). Every record decoded and the
counts and bounds match the final header. Producer, frame, configuration and effective parameters
match the regenerated Lombard run above. Its end is also approximately one rotation before the
manifest endpoint. Both runs used `full_frames=false`, `settle_before_recording=true`,
`speed_mode=analysis`, and zero clip duration meaning the complete capture through EOF.

## Final native walkthrough and handoff

The isolated Release client was built at `023e16f4c435c0166ccfa49d094d8bcb140bea9b` through
`make build-mac MAC_CONFIG=Release`. The running copy is
`/private/tmp/velocity-review-quiet/VelocityReview.app`; its different bundle identity preserves
the original application's preferences. A durable copy is retained beneath
`/Volumes/lidar/reviews/visualiser-20261114/VelocityReview.app`. The recording service remains
producer `9ee5ea36d`.

1. Set **Settings → Servers** to HTTP `http://127.0.0.1:18080`, stream `50052`. Open **Runs**
   and wait for loading to finish. Both complete recordings were observed beneath their correct
   **Lombard at Broderick** and **Columbus at Broadway** headings, with **FG** badges and
   durations **20:01** and **33:52**. The incomplete preview is separately named and has a
   **Full** badge. Search `052fa966` or `59a0bb94` to select the exact new recording.
2. Load the complete Lombard row, pause and seek near **1:59**. The observed exact frame was
   `1788367221226642330` ns. Inspector showed its new run, source, VRLOG path, clip, recording
   bounds, ENU frame, and both hashes. The chosen rendering site remained explicitly **none**;
   no viewing-context choice was silently promoted to a source binding or accepted pose.
3. Load the complete Columbus row and seek near **1:39**. The observed frame was
   `1788380535986217330` ns, with the correct new Columbus run, source, clip, recording bounds,
   frame and hashes visible. These regenerated tracks were kept separate from the older campaign
   export even at an overlapping capture time.
4. For the matching baseline, stop the selected run first so its context clears. Load
   `columbus-following-campaign-a46b25547` through `POST /api/lidar/vrlog/load`, using its full
   path beneath the dedicated VRLOG root. Its archive is not registered as a database run;
   Inspector correctly showed unavailable run/source/configuration fields instead of borrowing
   those of the previously selected full-site recording. This remains a native provenance gap.
   The verified copy, campaign manifest and retained audit identify it independently.
5. Seek the campaign to `1788380535986217330` ns, about **0:28.9** elapsed. Scroll the track
   list to its end and select `ac24`, then `6742`. Inspector exposed the exact follower and
   competing UUIDs named above. The follower was a car with 30 hits; the competing track was
   classified as a bird with zero hits and a displayed 0.8 × 0.4 × 0.3 m box. Its suppressed
   pairing is not a verified cut-in or an appropriate precise longitudinal vehicle result.
6. Seek to `1788380537686276330` ns, about **0:30.6**, and select `ac24`. Two native forward
   steps preserved its full UUID and advanced hits **1 → 2 → 3**. Displayed speed magnitude
   changed **8.1 → 8.0 → 7.7 m/s**; this is not a claim of constant physical speed. Selecting
   `14c4` exposed the retained leader UUID, 89 hits and 8.5 m/s after those steps. Boxes and
   foreground points were visible. The exported fitted path and endpoint arcs were not drawn,
   and their physical alignment was not qualified.
7. The handoff returned to the complete source-bound Lombard run near **1:59**, with Inspector
   provenance visible. Exact seeks used the supported gRPC `Pause` and `Seek` operations. The
   original server's HTTP pause route returned 501; the gRPC controls worked. A native pause
   icon can retain optimistic state after external RPC control, so it is not independent proof
   of the server's pause state.

The refreshed native client remained responsive during these normal-speed playback and stepping
checks. This does not qualify fast batch streaming. Runs refresh took 58.457 s in one observed
request; run load and track requests also took tens of seconds. Old rows can remain visible while
loading. Wait for the exact requested source and timestamp before interpreting the view, and
close/reopen Inspector after a run switch if its track list is still loading. A retained row is
not proof of membership in the newly loaded source. The Runs owner was informed; no endpoint-cache
bug was inferred after server logs showed requests reaching the correct service.

## Shared storage blocker and remaining review limits

During the session `/Volumes/banshee-results` disappeared. Gerardus's blob and reference paths
are symlinks into it, and the authoritative following JSON also resolves there. Alignment showed
`creating /private/tmp/velocity-gerardus-data/blobs/sha256/04: File exists (os error 17)`.
Read-only checks found a dangling blob-store symlink, rather than a shard to delete. The existing
Gerardus owner is investigating recovery and improving the unavailable-storage error in a separate
worktree. This review did not change that service, its storage, a pose, or a street-use feature.

The full-site captures, both regenerated recordings and the copied matching campaign remain
available on `/Volumes/lidar`. Native import reached **Reading evidence…** before the interrupted
session; it did not complete a demonstrated JSON walkthrough. The same decoder had already read
the real file and retained its authoritative values before the mount disappeared. Synthetic test
success does not replace that missing interactive check. Recover the complete missing reference,
blob and authoritative-export dependency set to `/Volumes/lidar`, point the service and review
there, and verify them with the NAS disconnected before completing reference alignment, lane
authoring/acceptance, snapshot save/reload, and native experimental-DRAC inspection. Reconnecting
the NAS can recover an archival copy; it is not a final working-storage solution. No accepted
lane snapshot, lane-derived gap, concurrent-candidate
DRAC, lane-aligned count, or turning movement was produced.

The final Release build and native lint passed. Focused final tests passed **11 XCTest cases**
(following evidence and track-list display) and **3 Swift Testing cases** (rewind/throttle).
An initial Release test attempt failed to import the app module; enabling
`SWIFT_ENABLE_TESTABILITY=YES` allowed the focused run to execute successfully. The earlier full
native suite remains the 849 XCTest / 1,990 Swift Testing baseline reported above; it was not
rerun in full after the layout and rewind changes. Runtime observations and repeatable decoding
are software evidence, not physical qualification of paths, endpoints, actors or collision risk.
