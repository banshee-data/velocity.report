# Gerardus alignment pane

The visualiser's client for the Gerardus alignment service: it draws USGS 3DEP aerial points and OpenStreetMap ways over a capture's settled background, places the sensor from its eye-read position, registers the scan automatically, lets the operator judge and correct the result, and records the operator's decision in Gerardus's alignment log.

- **Plan:** [lidar-visualiser-gerardus-alignment-plan.md](../../plans/lidar-visualiser-gerardus-alignment-plan.md) (the work still to come)
- **Contract:** [proto/gerardus/align/v1alpha1/align.proto](../../../proto/gerardus/align/v1alpha1/align.proto), vendored from banshee-data/gerardus

## Summary

Gerardus is a separate service in its own repository. It prepares reference data for a survey site and registers sensor scans against it. The visualiser talks to it directly on a second gRPC channel, beside its channel to velocity.report; velocity.report's Go server is not in the path. The dependency is optional: when no Gerardus server answers, the app shows no alignment control and behaves as it did before.

What ships is Gerardus MVP 0's operator loop, without tie points:

- the reference and a pose placed from the hint;
- automatic registration, with its hypotheses and gates;
- scoring of the pose drawn now, with the scan coloured by residual, and Snap;
- local nudges;
- accepting a hypothesis or the operator's own pose.

Gerardus does not build tie points yet, so the pane does not call `SolveTiePoints`.

## Running it

1. Start Gerardus with `gerardus serve`. It needs the site prepared under its data directory, or the NAS EPT copy and Overpass to prepare it. To be reached from another machine it must listen on more than loopback, for example `--listen 0.0.0.0:50061`.
2. Start velocity.report on `localhost:8080` and open a capture in the visualiser.
3. The toolbar shows **Align** once Gerardus answers `GetServerInfo`. While the visualiser is connected to velocity.report it asks at once, then every 30 seconds until a server answers, so Gerardus can start before or after the visualiser. It stops asking when the visualiser disconnects.

### Which Gerardus server

Settings (⌘,) holds the server's address: a host name or IP address, with an optional port (50061 if left out). For example: `machine.tailnet.ts.net`, `100.64.0.1:50061`, or `[fd7a:115c:a1e0::1]:50061`.

- **Saved:** the address is kept in the app's preferences as `alignment.serviceAddress`. An unset preference, and **Use this Mac**, mean `127.0.0.1:50061`.
- **Changing it:** **Connect** reconnects at once, closes the pane until the new server answers, and drops whatever was placed against the old one.
- **Status:** the line under the field names the address in use, and says whether Gerardus answered there and, if not, why.
- **Bad addresses:** text that is not a host and port is refused before anything is sent. An address with no Gerardus behind it leaves the alignment controls hidden, and nothing else in the app changes.

`annotation.serviceURL` sets the velocity.report HTTP address that the site lookup shares with the other clients. It has no Settings field.

Reaching Gerardus on another machine needs nothing more over Tailscale:

- The app is sandboxed with the outgoing-network entitlement (`com.apple.security.network.client`), which covers any host.
- macOS 15's Local Network privacy covers only networks on broadcast-capable interfaces, Wi-Fi and Ethernet, not VPNs (Apple TN3179). A Tailscale address or MagicDNS name therefore needs no permission.
- A server's LAN address does. The first connection makes macOS ask whether VelocityVisualiser may use the local network, giving the app's `NSLocalNetworkUsageDescription`. The system may refuse that first attempt before you answer; the 30-second re-probe then finds the server once access is allowed. If it is denied, re-allow it in System Settings → Privacy & Security → Local Network.

## The operator's path

| Step              | What the pane does                                                                                                                                                                                                                                                                        | Gerardus RPC                                                     |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| Open the pane     | Lists velocity.report's located clips and picks the open capture's                                                                                                                                                                                                                        | `GetServerInfo` (on connect)                                     |
| Place from hint   | Prepares the site, places the sensor from the eye-read position and the typed `north_azimuth_deg`, and streams 150 m of reference around it                                                                                                                                               | `PrepareSite`, `PoseFromHint`, `StreamReference`, `GetSiteModel` |
| Accumulate frames | During a pcap replay (with velocity.report's full frames on meanwhile) or a full-frames run's VRLOG replay, collects the background and ground points of the next N frames, thins them to 0.05 m and uploads them as an accumulated scan, which Register then uses                        | `UploadScan` (`SCAN_KIND_ACCUMULATED`)                           |
| Register          | Uploads the settled background once per snapshot (or takes the accumulated scan), then registers it from the position drawn now, against 3DEP alone or, with **Against site model** on, against the site model too. Hypotheses show as they converge; the best is drawn when the run ends | `UploadScan`, `Register`                                         |
| Pick a hypothesis | Draws it; Accept then records that hypothesis                                                                                                                                                                                                                                             | none                                                             |
| Nudge             | Moves the reference along the sensor's axes, or turns it about the sensor; once the pose has rested for 0.4 s it is scored and the scan coloured by residual                                                                                                                              | `EvaluatePose` (after the rest)                                  |
| Snap              | Refines the pose drawn now with local ICP from it, draws and scores the result                                                                                                                                                                                                            | `EvaluatePose(refine)`                                           |
| Accept or reject  | Records the hypothesis drawn, or the pose as it stands, or a rejection with its reason, then redraws the site model's layers if the model moved                                                                                                                                           | `ReviewRegistration`, `ListRegistrationRecords`, `GetSiteModel`  |
| Refine site       | Re-solves every accepted pose at the site together, with the N_eff chosen beside the button, then reads each scan's run for its gates. Needs two accepted scans                                                                                                                           | `RefineSite`, `GetRegistration`                                  |
| Accept refinement | Records every scan's refined pose (hypothesis `j1`) in order, then redraws the site model's layers. Stops at the first refusal                                                                                                                                                            | `ReviewRegistration`, `ListRegistrationRecords`, `GetSiteModel`  |

### Registration

**Register** searches 30 m around the position drawn now, at every heading (the start's heading is not used). It refines the best distinct poses with point-to-plane ICP and gates the best. That takes 10 to 25 s on a real scan, and **Cancel** stops it.

- **Hypotheses:** each one appears as it converges. When the run ends the list is ranked, best first, and the best is drawn. Each row gives:
  - its heading and how far it moved from the eye-read position;
  - ground p95, facade p75, the facade range slope and the inlier share;
  - how many of the 20 seeds converged on it, and any weak directions.
- **Gates:** the result is `CANDIDATE` when all six gates pass on the best hypothesis, else `FAILED`, and the pane lists each gate with its value and threshold. A `FAILED` run with no hypotheses (no aerial data, or no ground plane in the scan) leaves the operator's pose to place by hand.
- **Accept** says what it will record:
  - **Accept hypothesis N (registered)** for a hypothesis of a run that passed every gate;
  - **(manually placed)** for one that did not;
  - **Accept this pose (manually placed)** once the pose has been nudged or snapped off the hypothesis. A snapped pose is recorded with source `ICP`.

### Against the site model

The **Against site model** switch beside Register sets `RegisterRequest.against_site_model`. It is off by default. On, Gerardus fits the scan against 3DEP plus the site's accepted scans, leaving out the scan being registered (Gerardus `docs/plans/SITE-MODEL-SPIKE.md` §6.3).

- **Gates:** the ground and facade gates are still measured against 3DEP alone, so a pose that passes still sits on public data.
- **Model residuals:** when a hypothesis's fit used the model (`FitQuality.model_points > 0`), the pane shows `model_p50_m`, `model_p75_m` and `model_points` under the gates, and on that hypothesis's row. They are reported as Gerardus gives them, and are not gates: they never change the verdict or what Accept records.
- **Disabled, with the reason under it:**
  - when `GetServerInfo` lists no `REGISTERED_SCANS` layer;
  - when the site model holds no accepted scan, or none besides the one being registered;
  - when the pane cannot tell which scans the model holds.
- **The model's scans:** the pane asks `GetSiteModel` once the reference is placed, and again after each decision. When Gerardus answers `UNIMPLEMENTED` it uses the scans the registered-scans layer streamed.
- **When it is decided:** the switch is read once the scan is uploaded, when its `scan_id` is known. A model holding only this scan therefore sends `against_site_model = false`, whatever the switch shows.

A Gerardus that does not yet build this ignores the field and leaves the `model_*` fields at 0, so nothing extra is shown.

### Accumulated frames

**Accumulate frames** builds a second kind of scan from the replay's own frames, for the null test in the spike's §7: the background and the frames accumulated over the same replay should register to the same pose.

1. **Collect.** Press it during a replay; it collects the next N frames that reach the visualiser. N is chosen beside it and is 50 by default, about 5 s.
   - **A pcap replay** is split-streamed, foreground points only, so the pane first asks velocity.report for full frames (`POST /api/lidar/replay/full_frames {"enabled": true}`), waits out the split frames already on their way, collects, and switches full frames off again however the collection ends: done, refused partway, **Cancel**, another capture, the source turning live, or the pane closing. The off request waits for the on request's answer. velocity.report ends full frames with the replay in any case, and the switch never changes what that replay records.
   - **A VRLOG replay** accumulates only when its run was recorded with full frames (the run's `frame_mode` is `full`, shown as **Full** in the Analysis Runs sheet's Frames column): its frames carry background as recorded, and nothing is switched. A foreground run's VRLOG (**FG**) is refused up front.
   - **Kept:** only points classed background (0) or ground (2). Foreground (1) points are never collected (ADR-0011: an accumulated scan is a settled background, with no foreground and no per-point time).
   - **Full rate only:** frames must have `decimation_ratio` 0 or 1, no voxel decimation, and nothing stripped.
   - **Repeats:** a frame seen twice counts once.
2. **Thin.** The points stay in the sensor frame and are thinned in the client to one per 0.05 m voxel, at the centroid of the points in it.
3. **Upload.** `UploadScan` sends them with header `kind = SCAN_KIND_ACCUMULATED`, in blocks of at most 65,536 points, with no confidence (each point weighs the same).
   - **`capture_ref`:** `<run id>#f<first>-<last>`. The run id is the replay's run, or the site's clip id when the replay has none. The frame numbers are the replay's position in its log for a VRLOG, else the frames' own ids.
4. **Use.** The accumulated scan becomes the scan Register and scoring use, with the switch on or off, and the pose drawn now is scored against it. **Use background** goes back to the settled background. Opening another capture or site drops it.

The button refuses, and says why, when:

- the frames come from a live sensor;
- the replay is a foreground run's VRLOG: "This run's VRLOG holds foreground points only; replay its pcap with Full frames, or load a full-frames run.";
- velocity.report refuses full frames (409 when no pcap replay is running, 501 without its gRPC stream), with the server's reason;
- velocity.report is older than the switch (it answers 404 or 405, or a 200 that does not state `full_frames`), which the pane says rather than collecting nothing;
- the request to switch fails (it is still followed by a switch-off, in case it did switch);
- the stream is decimated;
- the stream carries foreground points only and no source says it is a pcap replay;
- the frames carry no point classes;
- the replay jumps back during the collection;
- the frames hold no background or ground points.

A decimated frame arriving partway through stops the collection, and nothing is uploaded.

### Refining the site

**Refine site** asks Gerardus to re-solve the site's accepted poses as one small pose graph: each scan's fit to 3DEP, and the fits between overlapping scans (Gerardus `docs/plans/SITE-MODEL-SPIKE.md` §6.4). It is enabled once the site model holds two accepted scans. The picker beside it sets `n_eff`, how many independent residuals each fit counts as (500 by default, or 250 or 1,000); the result names the weighting it ran with, so the spike's q2 can be answered by refining twice.

The result lists, per scan, how far the pose moved, its joint constraint rank, its gates as badges, its scores against 3DEP after refining, and any weak direction; then each pair of scans, with its p75 residual before and after and why a pair was left out. A scan whose run passed every gate is recorded `REGISTERED`, any other `MANUALLY_PLACED`. Gerardus also gates how far a `REGISTERED` scan may move (0.10 m, 0.5°) and that no 3DEP score gets worse (Gerardus `docs/plans/SITE-MODEL-SPIKE-PORT.md` §7).

**Accept refinement** records every scan's `j1` together, in order: each pose was solved against the others, so accepting some would leave the rest placed against poses that were never accepted. Gerardus refuses a `j1` whose scan's accepted pose has changed since the refinement ran; the pane then stops, says how many were recorded before the refusal, and the way on is to refine again.

### Scoring and residuals

- **Scoring:** moving the pose sends nothing while it moves. Once it has rested for 0.4 s, `EvaluatePose` scores it against the reference and returns a residual for every uploaded scan point. That takes about 35 ms with the area cached, and a second or two the first time.
- **Fit line:** the pane shows the fit of the pose drawn now: ground p95, facade p75, the range slope, the constraint rank and any weak direction.
- **Residual colours:** the scan is drawn over its own background, green on the 3DEP surface, through yellow to red at 0.5 m off, and grey where no reference lies within 1 m. A pose that is a few centimetres out shows as a band of colour. A heading error shows as colour that grows with range.
- **Stale scores:** a score that arrives after the pose has moved again is dropped.

The accepted record is not yet written back to velocity.report: that needs `PUT /api/lidar/sites/{l16}/pose` (Gerardus MVP step 2), and the code marks the place with a TODO.

## Which site

Gerardus keys everything by the site's S2 L16 token, and its hint needs the eye-read latitude and longitude. The pane reads both from velocity.report's clips (`GET /api/lidar/clip-locations`), and finds the open capture’s clip by, in order:

1. the run's `clip_id`, for the run loaded from the run browser or the one the server is replaying (a VRLOG replay names its run only by its directory);
2. the run's source file, matched against each case's capture files;
3. the published capture stem `<start>_<L13 display>_<site id>`, whose L13 cell names the site, with the site id's words choosing between cases in a shared cell.

When nothing matches, the operator picks the site from the list.

The operator types `north_azimuth_deg` from `tools/s2-archive/site-index.json`. No velocity.report API serves it, and serving it needs a stored column, which belongs to the step-2 pose record. The pane remembers the typed value per site.

`north_azimuth_deg` is not the bearing of the sensor's +Y. It is measured with the web scene viewer's "Align from above" tool, against that viewer's camera. The pane sends `(180 − north_azimuth_deg) mod 360` as `PoseHint.heading_deg`, the bearing of +Y, and shows that value beside the field. The rule is measured, not derived. Gerardus's automatic registration searched every heading and ignored the hint. At three sites whose `north_azimuth_deg` is 35, 131 and 51, it found +Y at 143.5°, 49.4° and 129.9°: 180 − N each time, within 1.5°. A mirrored copy of the scan fitted much worse, so the sensor frame itself is right-handed. An empty field sends no heading, and Gerardus then reports a heading uncertainty of 180°.

## Frames and drawing

The visualiser draws in the sensor frame, velocity.report's pipeline frame: +Y is azimuth zero, +X is 90° clockwise from it, and +Z is up. Reference geometry arrives in the site frame, east-north-up at the centre of the L16 cell. A pose maps sensor to site, `p_site = R(q)·p_sensor + t`, with `q` a Hamilton quaternion, `w` first.

The renderer draws the reference with the inverse of the pose as its model matrix, folded into `Uniforms.modelViewProjection` and `Uniforms.modelView` for those draws only. A nudge replaces that matrix and sends nothing. Each nudge moves the drawing by one step in the sensor frame, and the pose becomes `P·N⁻¹`, so the reference lands where the operator put it.

| Layer             | Pipeline                                                                                      | Colour                                                                                                                                                                                                                              |
| ----------------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 3DEP points       | Point pipeline, `[x, y, z, code / 255, 0]` per point, table colouring                         | The ASPRS class picks a ramp (ground amber, unclassified violet, building red, and so on); intensity picks the shade within it                                                                                                      |
| Scan residuals    | Point pipeline in the sensor frame, drawn over the background with a less-or-equal depth test | By the size of each point's `EvaluatePose` residual: green, yellow, red at 0.5 m; grey with no reference in reach                                                                                                                   |
| OSM ways          | Debug-line pipeline, `[x, y, z, r, g, b, a]` per vertex                                       | By `FeatureKind`: roads yellow, paths light blue, kerbs orange, rails magenta, buildings red                                                                                                                                        |
| Intersections     | Debug-line pipeline                                                                           | A white cross at each node, and a cyan tick down each approach                                                                                                                                                                      |
| DataSF footprints | Debug-line pipeline                                                                           | Red, as OSM buildings are, with a toggle of their own                                                                                                                                                                               |
| Registered scans  | Point pipeline in the site frame, with a buffer and toggle of its own                         | One palette code per scan, above the ASPRS ramps (codes 224 to 255): a distinct hue per scan, skipping the foreground's greens. The open capture's own scan is hidden by `scan_id` unless **Include this capture's own scan** is on |
| Observed features | Debug-line pipeline                                                                           | By `FeatureKind`: sensors white, corrections amber, facade traces cyan (with a top edge at `height_m` and an upright at each end), poles blue                                                                                       |
| Ground mesh       | Debug-line pipeline, as a wireframe of the heightfield: no triangles                          | Faint green. Cell by cell within 40 m of the sensor and every 2 m beyond, clipped to the 150 m streaming radius; cells with no ground are holes                                                                                     |

The point shader's table mode reads a palette from fragment buffer 0. The reference binds its own 256-entry palette there, so one byte carries both class and brightness and the shader needs no new mode. Draped lines sit 0.10 m above the 3DEP ground so they do not fight its points for depth; this offset is display only. Aerial points draw at half the scene's point size.

When OSM ways are visible, "© OpenStreetMap contributors" (from `Site.sources`) shows in the corner of the scene. The 3DEP attribution shows in the pane.

### The site model's layers

Registered scans, observed features and the ground mesh are built from the site model: the site's accepted scans (Gerardus `docs/plans/SITE-MODEL-SPIKE.md`). They arrive in the site frame, like every other layer, and draw with the same model matrix. The pane asks for each on a call of its own, after the bundle's layers, and only for the layers `GetServerInfo` lists:

- **Refused:** Gerardus serves registered scans only under `serve --serve-scans`, and to loopback and tailnet peers. A `PERMISSION_DENIED` leaves that layer empty and says why in the pane; the other layers draw as usual.
- **Empty:** a layer served empty, with no accepted scans yet, is simply empty.
- **After a decision:** once Accept or Reject is recorded, the pane calls `GetSiteModel` and compares its `model_id` with the one the layers carry. When it has moved, or when Gerardus answers `UNIMPLEMENTED`, the three layers are streamed again. The bundle's layers are not.

## What the pane shows

- The server's `PoseSummary` for the hint pose: heading with its 1σ, pitch and roll, height above the 3DEP ground, position on NAD83(2011), and NAVD88 height. Ellipsoidal height reads "not given" until Gerardus wires in GEOID18.
- The nudge as east, north and up offsets and a heading change, computed in the client. The server never sees a summary of a nudged pose until it is registered or accepted.
- The background snapshot that Register would upload, and whether the grid has settled. An unsettled snapshot is not uploaded, scored or registered.
- The registration's state, gates and hypotheses, and the fit of the pose drawn now; the site model's residuals beside the gates when the fit used the model.
- Which scan Register uses: the background, or the accumulated frames with their point counts and `capture_ref`.
- A refinement: per scan its move, rank, gates and scores, then its scan pairs.
- The site's records, newest first.

## Known limits

- OSM building outlines sit about 0.8 m east of the 3DEP points at the measured sites, and named intersections sit 9 to 34 m from the eye-read positions. Both are in the data, not the drawing; align to the 3DEP points.
- The sensor model in a scan header is always `hesai-pandar40p`.
- **Accumulate frames needs a pcap replay, or a full-frames run's VRLOG** (the spike's q4). velocity.report split-streams foreground points only, so the pane switches its full frames on for the collection. Every VRLOG recorded before `frame_mode` existed, and every run started without **Full frames**, holds foreground points only, and is refused. This has been checked in tests against fake servers, not yet on a real replay.
- velocity.report's full frames classify points 0 (background) and 1 (foreground) only: ground is in class 0, so "background and ground" means class 0 in practice.
- A full-frames replay streams about 70,000 points a frame to the visualiser while it runs, and a full-frames VRLOG is correspondingly larger than a foreground one.
- A pcap replay started with **Full frames** keeps them on until it ends: the pane's switch-off is refused (409) and only logged.

- The visualiser's client passes on at most about 30 frames a second, so above 3× playback of a 10 Hz capture the N frames are not consecutive; `capture_ref` still names the first and last.
- `lidar-state-estimation-baseline`'s `siteCalibration` rotates the sensor's +X onto `north_azimuth_deg`. By the measurement above, +Y lies at 180° − N, so +X lies at 270° − N: that transform is wrong too. It is not used here.
