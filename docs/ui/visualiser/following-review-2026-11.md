# Lombard–Broderick and Columbus–Broadway review readiness

- **Status:** Partial workflow; accepted lane snapshots and a matching recording/evidence review remain open
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
site-to-capture binding. All of these database runs lack a clip association.

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
archived raw evidence were preserved. Full repository Go/web suites and a Release distribution
build are outside these validation claims.
