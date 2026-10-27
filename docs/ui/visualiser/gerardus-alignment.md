# Gerardus alignment pane

The visualiser's client for the Gerardus alignment service: it draws USGS 3DEP aerial points and OpenStreetMap ways over a capture's settled background, places the sensor from its eye-read position, registers the scan automatically, lets the operator judge and correct the result, and records the operator's decision in Gerardus's alignment log.

- **Plan:** [lidar-visualiser-gerardus-alignment-plan.md](../../plans/lidar-visualiser-gerardus-alignment-plan.md) (the work still to come)
- **Contract:** [proto/gerardus/align/v1alpha1/align.proto](../../../proto/gerardus/align/v1alpha1/align.proto), vendored from banshee-data/gerardus

## Summary

Gerardus is a separate service in its own repository. It prepares reference data for a survey site and registers sensor scans against it. The visualiser talks to it directly on a second gRPC channel, beside its channel to velocity.report; velocity.report's Go server is not in the path. The dependency is optional: when no Gerardus server answers, the app shows no alignment control and behaves as it did before.

**What the pane draws.** Measured or derived geometry at each source's acquisition time, in the
site frame, in metres. The reference bundle names each input's source, version, licence and
retrieval time (`Site.sources`) and the exact inputs it was prepared from (`bundle_id`). The 3DEP
points and the accepted scans are measured; footprints, boxes, building parts and roof planes are
derived from public data and refined by the scans; street-use marks are the operator's reading of
the street, not a measurement. The fit gates and residuals below say how well a scan sits against
the reference. None of it is a plan, a land or boundary survey, a structural assessment or a
specification, and a feature that is not drawn is not asserted absent. See
[Geometry priors: data purpose and interpretation](../../geometry-priors-data-purpose.md).

What ships is Gerardus MVP 0's operator loop, without tie points:

- the reference and a pose placed from the hint;
- automatic registration, with its hypotheses and gates;
- scoring of the pose drawn now, with the scan coloured by residual, and Snap;
- local nudges;
- accepting a hypothesis or the operator's own pose;
- paint mode, the street painter: marking the site's surfaces, travel lanes, parked-car spaces and driveways from a top-down view over the placed reference ([Paint mode](#paint-mode-the-street-painter)).

Gerardus does not build tie points yet, so the pane does not call `SolveTiePoints`.

## Running it

1. Start Gerardus with `gerardus serve`. It needs the site prepared under its data directory, or the NAS EPT copy and Overpass to prepare it. To be reached from another machine it must listen on more than loopback, for example `--listen 0.0.0.0:50061`.
2. Start velocity.report on `localhost:8080` and open a capture in the visualiser.
3. The toolbar shows **Align** once Gerardus answers `GetServerInfo`. While the visualiser is connected to velocity.report it asks at once, then every 30 seconds until a server answers, so Gerardus can start before or after the visualiser. It stops asking when the visualiser disconnects.

### Which Gerardus server

Settings (⌘,) ▸ **Servers** has a **Gerardus** section, below velocity.report's ([server-settings.md](server-settings.md)). It holds the server's address: a host name or IP address, with an optional port (50061 if left out). For example: `machine.tailnet.ts.net`, `100.64.0.1:50061`, or `[fd7a:115c:a1e0::1]:50061`.

- **Saved:** the address is kept in the app's preferences as `alignment.serviceAddress`. An unset preference, and **Use this Mac**, mean `127.0.0.1:50061`.
- **Changing it:** **Connect** reconnects at once, closes the pane until the new server answers, and drops whatever was placed against the old one.
- **Status:** the line under the field names the address in use, and says whether Gerardus answered there and, if not, why. The status inlaid in the top left of the 3D view shows the same under velocity.report's.
- **Bad addresses:** text that is not a host and port is refused before anything is sent. An address with no Gerardus behind it leaves the alignment controls hidden, and nothing else in the app changes.
- **Test connection:** asks for `GetServerInfo` on a channel of its own, at the typed address when it parses, with a 3 s timeout, and reports reachable with the round trip in milliseconds, wrong service (`UNIMPLEMENTED`: no Gerardus `SiteService` there), unreachable or no answer. When it answers, **This Gerardus** shows its version and git sha, which Gerardus sends together as `server_version` (`0.1.0+3f2a9c1e0b7d`: the sha is the semver build metadata), and its API version.
- **Serves registered scans:** shown when `GetServerInfo` lists any layer: yes when it lists `REGISTERED_SCANS`, which Gerardus does only under `serve --serve-scans`. It is the server's setting, read only here; a server that lists no layers says nothing either way, and the line is left out.
- **Author:** the street painter's author name, kept as `streets.author`. The painter's own **Author** field is the same setting.

The velocity.report HTTP address that the site lookup shares with the other clients is set in the same tab, as `velocityReport.serverURL` (formerly `annotation.serviceURL`, moved at launch).

Reaching Gerardus on another machine needs nothing more over Tailscale:

- The app is sandboxed with the outgoing-network entitlement (`com.apple.security.network.client`), which covers any host.
- macOS 15's Local Network privacy covers only networks on broadcast-capable interfaces, Wi-Fi and Ethernet, not VPNs (Apple TN3179). A Tailscale address or MagicDNS name therefore needs no permission.
- A server's LAN address does. The first connection makes macOS ask whether VelocityVisualiser may use the local network, giving the app's `NSLocalNetworkUsageDescription`. The system may refuse that first attempt before you answer; the 30-second re-probe then finds the server once access is allowed. If it is denied, re-allow it in System Settings → Privacy & Security → Local Network.

## The operator's path

| Step              | What the pane does                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | Gerardus RPC                                                                      |
| ----------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| Open the pane     | Lists velocity.report's located clips and picks the open capture's                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   | `GetServerInfo` (on connect)                                                      |
| Place from hint   | Prepares the site, places the sensor from the eye-read position and the typed `north_azimuth_deg`, and streams 150 m of reference around it, the street layers on a call of their own when the server lists them, each W4–W7 layer (buildings, lane markings, street furniture, trees) on a call of its own when the server lists it, then reads the site's street use                                                                                                                                                                               | `PrepareSite`, `PoseFromHint`, `StreamReference`, `GetSiteModel`, `ListStreetUse` |
| Accumulate frames | During a pcap replay (with velocity.report's full frames on meanwhile) or a full-frames run's VRLOG replay, collects the background and ground points of the next N frames, thins them to 0.05 m and uploads them as an accumulated scan, which Register then uses                                                                                                                                                                                                                                                                                   | `UploadScan` (`SCAN_KIND_ACCUMULATED`)                                            |
| Register          | Uploads the settled background once per snapshot (or takes the accumulated scan), then registers it from the position drawn now, against 3DEP alone or, with **Against site model** on, against the site model too. **Against DataSF boxes** also fits it against the site's DataSF box walls, corrected by the measured wall planes; it can be on with the other switch, and is disabled, with the reason shown, when Gerardus lists no boxes layer or streamed no box walls. Hypotheses show as they converge; the best is drawn when the run ends | `UploadScan`, `Register`                                                          |
| Pick a hypothesis | Draws it; Accept then records that hypothesis                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | none                                                                              |
| Nudge             | Moves the reference along the sensor's axes, or turns it about the sensor; once the pose has rested for 0.4 s it is scored and the scan coloured by residual                                                                                                                                                                                                                                                                                                                                                                                         | `EvaluatePose` (after the rest)                                                   |
| Snap              | Refines the pose drawn now with local ICP from it, draws and scores the result                                                                                                                                                                                                                                                                                                                                                                                                                                                                       | `EvaluatePose(refine)`                                                            |
| Accept or reject  | Records the hypothesis drawn, or the pose as it stands, or a rejection with its reason, then redraws the site model's layers if the model moved                                                                                                                                                                                                                                                                                                                                                                                                      | `ReviewRegistration`, `ListRegistrationRecords`, `GetSiteModel`                   |
| Refine site       | Opens the refinement column left of the pane. Re-solves the ticked accepted poses at the site together (all of them unless some are unticked), with the N_eff chosen there, then reads each scan's run for its gates. Needs two ticked scans                                                                                                                                                                                                                                                                                                         | `RefineSite`, `GetRegistration`                                                   |
| Accept refinement | Records every scan's refined pose (hypothesis `j1`) in order, then redraws the site model's layers. Stops at the first refusal                                                                                                                                                                                                                                                                                                                                                                                                                       | `ReviewRegistration`, `ListRegistrationRecords`, `GetSiteModel`                   |
| Paint streets     | Locks the camera to a top-down plan view and marks the site's street use; nothing is sent while painting                                                                                                                                                                                                                                                                                                                                                                                                                                             | none                                                                              |
| Save street use   | Sends every change since the marks were read as one batch, then draws the marks Gerardus answers with and its problems                                                                                                                                                                                                                                                                                                                                                                                                                               | `EditStreetUse`                                                                   |

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

The site model is Gerardus's name for the site's accepted scans, from which the registered-scan,
observed-feature and ground layers are built (see below). It holds measured surfaces and positions,
not a plan of the site.

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

Refining has a column of its own, 460 pt wide, to the left of the alignment pane. The pane keeps a compact **Refine site…** button under **Site model**, which opens it. A refinement opens it too, from its start. Its close button hides it. Closing keeps the refinement and each scan's run, and reopening shows them again. A new site closes it and drops the refinement.

**Refine site** in the column asks Gerardus to re-solve the site's accepted poses as one small pose graph: each scan's fit to 3DEP, and the fits between overlapping scans (Gerardus `docs/plans/SITE-MODEL-SPIKE.md` §6.4). It is enabled once the site model holds two accepted scans; until then the column says how many it holds. The **Weighting** picker beside it sets `n_eff`, how many independent residuals each fit counts as (500 by default, or 250 or 1,000); the result names the weighting it ran with, so the spike's q2 can be answered by refining twice.

**Scans to refine.** Each of the site model's scans has a box, ticked by default, that keeps it in the next Refine. Before a refinement the column lists them; once there is one the boxes sit on its summary rows. With every box ticked the request names no scans, which Gerardus reads as all of them; with some unticked it sends the rest as `RefineSiteRequest.scan_ids`. Refine needs two ticked scans, and says how many are ticked when there are fewer.

**The summary.** The result opens with a one-line verdict, for example "7 scans · largest move 0.5 cm, 0.05° · all gates pass", then how many iterations it ran, its N_eff and the model it refined. A row per refined scan follows, largest move first (by distance, then by heading):

- a swatch in the scan's state colour (see below);
- a label: the scan's `capture_ref`, from `GetSiteModel` or else the registered layers, shortened in the middle to 24 characters, then the last 6 characters of its `scan_id`, as the pairs table names scans;
- Δ position in centimetres and Δ heading in degrees;
- what accepting it records: `REGISTERED` when its run passed every gate, else `MANUALLY_PLACED`. A run Gerardus did not return counts as not passed;
- its joint constraint rank.

Scans the site model holds but the refinement did not refine follow, with no move and "not refined". Each row's detail folds under it, closed at first: the full `scan_id`, the move to the millimetre, its gates one row each (the mark, the name, and the value against its threshold), its 3DEP scores after refining, and any weak direction. The pair table folds into **Pairs (used n of m)**: whether each pair was used, its p75 residual before and after, the points matched, and why a pair was left out. Gerardus also gates how far a `REGISTERED` scan may move (0.10 m, 0.5°) and that no 3DEP score gets worse (Gerardus `docs/plans/SITE-MODEL-SPIKE-PORT.md` §7).

**States.** Each scan takes one of four states, coloured the same in the column and the scene (see the colour table):

| State                   | When                                                                                                   |
| ----------------------- | ------------------------------------------------------------------------------------------------------ |
| **unchanged**           | moved under 1 cm and under 0.05°, whatever its gates                                                   |
| **moved, gates pass**   | moved 1 cm or 0.05° or more, and its run passed every gate: accepting records `REGISTERED`             |
| **moved, a gate fails** | moved as far, and a gate failed or was not read: accepting records `MANUALLY_PLACED`                   |
| **left out or stale**   | none of its pairs was used, it was not refined, or it was accepted on its own since the refinement ran |

Left out or stale wins over the move. A refinement that reports no pairs leaves no scan out on that account.

**Picking a scan.** Clicking a row (its label, move, records or rank) selects that scan:

- **Camera:** the scene frames it, centred on its sensor: the refined pose's position, or the accepted one for a scan the refinement did not move. The site point goes through the reference's model matrix, so it lands where the scene draws it. The camera stays 25 m back along the direction it already looks.
- **Bold:** its sensor arrow and correction line are drawn five times, as given and 5 cm to each side, so they read bold. The other registered scans' points dim to 30 % of their colour.
- **Clearing:** clicking it again, or Escape, clears the selection. The camera stays where it is.
- **Kept:** the selection survives closing and reopening the column with the same refinement. A new refinement, or a new site, clears it.

**Accept refinement** says, beside the button: "Accepts all N refined poses together. Each was solved against the others, so they are accepted as one; to leave a scan out, refine without it." It records every scan's `j1` together, in order: accepting some would leave the rest placed against poses that were never accepted, so a refinement cannot be accepted in part. Gerardus refuses a `j1` whose scan's accepted pose has changed since the refinement ran; the pane then stops, says how many were recorded before the refusal, and the way on is to refine again. An Accept recorded since the refinement ran, for one of its scans, makes that refusal certain. Accept refinement is then disabled before it records any scan, and says to refine again; that scan's row turns left out or stale.

### Why Accept is disabled

Accept, Reject and Accept refinement list why they are disabled, in orange text under the button, not only in a tooltip. One line is given per reason:

- **No registration to decide on**, or **no pose drawn** (Accept and Reject);
- **No refinement yet**, **the refinement moved no scan**, **already recorded** (with the count), or **a scan accepted again since the refinement ran** (Accept refinement);
- **Busy**, naming what is running, for example "Busy re-streaming the site model: wait for it to finish.";
- **No reviewer name.** Gerardus refuses any decision, a rejection included, without a reviewer.

The reviewer is one field, pinned under the pane's title above the scroll. It is shown whenever Accept or Accept refinement is, and its label turns orange while it is blank. The name is saved as `alignment.reviewer`. A name saved blank is passed over at launch for the annotation operator's name, then the account's, so a cleared field cannot block every decision from then on. It is trimmed before it is sent, as Gerardus trims it.

Before this, accepting a refinement left the pane busy for good. When the model moved, the re-stream of its layers set "Re-streaming the site model" after Accept refinement had cleared its own busy line. Nothing cleared it, and Accept, Reject, Register and Refine site all stayed disabled until the site was placed again.

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

| Layer             | Pipeline                                                                                                                                                                                                                       | Colour                                                                                                                                                                                                                                                                                  |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 3DEP points       | Point pipeline, `[x, y, z, code / 255, 0]` per point, table colouring                                                                                                                                                          | The ASPRS class picks a ramp (ground amber, unclassified violet, building red, and so on); intensity picks the shade within it                                                                                                                                                          |
| Scan residuals    | Point pipeline in the sensor frame, drawn over the background with a less-or-equal depth test                                                                                                                                  | By the size of each point's `EvaluatePose` residual: green, yellow, red at 0.5 m; grey with no reference in reach                                                                                                                                                                       |
| OSM ways          | Debug-line pipeline, `[x, y, z, r, g, b, a]` per vertex                                                                                                                                                                        | By `FeatureKind`: roads yellow, paths light blue, kerbs orange, rails magenta, buildings red                                                                                                                                                                                            |
| Intersections     | Debug-line pipeline                                                                                                                                                                                                            | A white cross at each node, and a cyan tick down each approach                                                                                                                                                                                                                          |
| DataSF footprints | Debug-line pipeline                                                                                                                                                                                                            | Red, as OSM buildings are, with a toggle of their own                                                                                                                                                                                                                                   |
| DataSF boxes      | Debug-line pipeline, in the site frame; each wall a closed vertical polygon drawn as given, a rectangle or, with `top=roofline`, more vertices along the roof                                                                  | A muted building red, darker and greyer than the flat footprints. Walls on an edge another footprint shares (`party=true`) are the same red, fainter. A toggle of their own, **DataSF boxes (N walls)**                                                                                 |
| Registered scans  | Point pipeline in the site frame, with a buffer and toggle of its own                                                                                                                                                          | One palette code per scan, above the ASPRS ramps (codes 224 to 255): a distinct hue per scan, skipping the foreground's greens. The open capture's own scan is hidden by `scan_id` unless **Include this capture's own scan** is on                                                     |
| Observed features | Debug-line pipeline                                                                                                                                                                                                            | By `FeatureKind`: sensors white, corrections amber, facade traces cyan (with a top edge at `height_m` and an upright at each end), poles blue. While a refinement is shown in its column, each scan's sensor and correction (by their `scan_id` tag) take its refinement state's colour |
| Wall planes       | Debug-line pipeline; each a closed vertical rectangle drawn as given, with no top edge or uprights                                                                                                                             | By the `role` tag: walls cyan-teal, popouts orange, recesses violet, any other role grey. A toggle of their own, **Wall planes (N)**, apart from the observed features                                                                                                                  |
| Ground mesh       | Debug-line pipeline, as a wireframe of the heightfield: no triangles                                                                                                                                                           | Faint green. Cell by cell within 40 m of the sensor and every 2 m beyond, clipped to the 150 m streaming radius; cells with no ground are holes                                                                                                                                         |
| Kerbs and islands | Debug-line pipeline, a buffer and toggle of its own (`REFERENCE_LAYER_KERBS`)                                                                                                                                                  | Orange, as OSM kerbs are; islands the same                                                                                                                                                                                                                                              |
| City streets      | Debug-line pipeline, one buffer and toggle for `CITY_STREETS` and `CITY_INTERSECTIONS`                                                                                                                                         | Centrelines pale slate, apart from OSM's yellow roads; a white cross at each City intersection, without approach ticks                                                                                                                                                                  |
| Buildings         | Debug-line pipeline for each building part's base outline, its wall surfaces' outlines (an older server's wireframe prism) and its roof planes' outlines, and triangles for the roof planes' fills; one buffer pair and toggle | Parts sand. Roof planes by `slope_deg` (flat blue, 22.5° yellow, 45° and steeper red) or, with **Roofs by Shape**, by their part's `roof:shape` (each Simple 3D Buildings shape its own colour, grey for none or `unknown`)                                                             |
| Lane markings     | Debug-line pipeline, 0.10 m above the road, closed area markings closed (`REFERENCE_LAYER_LANE_MARKINGS`)                                                                                                                      | Bright and opaque: white or yellow by the `colour` tag, cream when it has none                                                                                                                                                                                                          |
| Street furniture  | Debug-line pipeline (`REFERENCE_LAYER_STREET_FURNITURE`), drawn where Gerardus places it                                                                                                                                       | Poles blue: a line base to top and a ring of `diameter_m` (0.2 m when absent) at each end. Signs crimson: the face's segment and four copies 4 cm about it, so it reads thick                                                                                                           |
| Trees             | Debug-line pipeline (`REFERENCE_LAYER_TREES`), drawn where Gerardus places them                                                                                                                                                | Trunk brown, base to the crown's base; crown green, a ring of `crown_radius_m` (2 m when absent) at `crown_base_m` and at `height_m` above the trunk's base                                                                                                                             |
| Street use        | Debug-line pipeline, triangles for fills and bands and lines for outlines, in the site frame                                                                                                                                   | By kind, as in the [painter](#what-the-painter-draws)                                                                                                                                                                                                                                   |
| Refinement states | The column's swatches, and the observed layer's sensors and corrections while a refinement is shown                                                                                                                            | Okabe–Ito colours, each at least 0.25 apart in RGB from every layer colour above and from each other: unchanged slate grey `#80808F`, moved with gates passing bluish green `#009E73`, moved with a gate failing vermillion `#D55E00`, left out or stale blue `#0072B2`                 |

The point shader's table mode reads a palette from fragment buffer 0. The reference binds its own 256-entry palette there, so one byte carries both class and brightness and the shader needs no new mode. Draped lines sit 0.10 m above the 3DEP ground so they do not fight its points for depth; this offset is display only. Aerial points draw at half the scene's point size.

When OSM ways are visible, "© OpenStreetMap contributors" (from `Site.sources`) shows in the corner of the scene. The 3DEP attribution shows in the pane.

DataSF boxes (`REFERENCE_LAYER_BOXES`, Gerardus site model spike §6.6) are vertical faces raised along each footprint's edges from the 3DEP ground to a roof height measured in 3DEP, moved by the site's footprint shift: an extent inferred from dated public data, not observed walls. Once Gerardus has derived the site model, a wall of a building with roof planes follows their roofline instead (tag `top=roofline`, else `top=box`), so it has more than four vertices where it rises to a gable; the pane draws every wall as the closed polygon it comes as, and adds nothing for `height_m`. Footprints arrive moved by their box's shift (tags `shift_east_m`, `shift_north_m`) and are drawn where they arrive: the pane does not shift them again. Boxes are public reference data, built with the bundle, so the pane asks for them on the bundle's call, and only when `GetServerInfo` lists the layer: a server from before it would refuse the whole call. A layer served empty, as Gerardus serves it before it builds boxes, shows the toggle with no walls. The DataSF attribution shows while footprints or boxes are drawn.

**Faces.** The toolbar's **W** toggle, after **G**, with the tooltip "Faces: DataSF boxes and wall planes" (and the W key), shows or hides the site's faces in one click: the DataSF box walls and the wall planes. Off hides both, whatever the pane's **DataSF boxes** and **Wall planes** toggles say. On gives them back to those toggles. Like the other letter toggles it is not saved between launches.

### The site model's layers

Registered scans, observed features and the ground mesh are built from the site model: the site's accepted scans (Gerardus `docs/plans/SITE-MODEL-SPIKE.md`). They arrive in the site frame, like every other layer, and draw with the same model matrix. The pane asks for each on a call of its own, after the bundle's layers, and only for the layers `GetServerInfo` lists:

- **Refused:** Gerardus serves registered scans only under `serve --serve-scans`, and to loopback and tailnet peers. A `PERMISSION_DENIED` leaves that layer empty and says why in the pane; the other layers draw as usual.
- **Empty:** a layer served empty, with no accepted scans yet, is simply empty.
- **Wall planes:** `FEATURE_KIND_WALL_PLANE` features arrive in the observed layer, each a wall plane the registered scans measure against a box wall. The pane keeps them apart from the other observed features, with a toggle of their own and their count by role. Until Gerardus builds them the observed layer holds none.
- **After a decision:** once Accept or Reject is recorded, the pane calls `GetSiteModel` and compares its `model_id` with the one the layers carry. When it has moved, or when Gerardus answers `UNIMPLEMENTED`, the three layers are streamed again. The bundle's layers are not.

### Buildings, lane markings, street furniture and trees

Gerardus's W4–W7 layers: building parts with fitted roof planes, and their walls (`REFERENCE_LAYER_BUILDINGS`, `FEATURE_KIND_BUILDING_PART`, `FEATURE_KIND_ROOF_PLANE` and `FEATURE_KIND_WALL_SURFACE`), painted markings (`REFERENCE_LAYER_LANE_MARKINGS`, `FEATURE_KIND_LANE_MARKING`), poles and signs (`REFERENCE_LAYER_STREET_FURNITURE`, `FEATURE_KIND_POLE` and `FEATURE_KIND_SIGN`), and trees (`REFERENCE_LAYER_TREES`, `FEATURE_KIND_TREE`). Gerardus derives them in parallel, so the pane asks for each on a call of its own, after the street layers, and only when `GetServerInfo` lists it. A server that lists no layers at all is not asked for them either. A layer that fails is left empty and named under the toggles; the others draw as usual. Each layer has a toggle with its counts, shown only when the server lists the layer.

- **Building parts** come as their outline at the part's base: the ground, or `min_height` above it. Their tags stay right for OSM export, so `height` is the top of the roof (the highest roof cell), not the eave, and the pane does not raise the outline to it when the server sends wall surfaces.
- **Wall surfaces** (Gerardus `docs/plans/ROOFLINE-WALLS.md`) are vertical faces Gerardus derives along each part's outline: closed vertical polygons from the part's base up to its roofline, with their z in their vertices and `height_m` 0, derived from public data and the scans rather than observed as walls. An eave face (`role=wall`) stops at the eave, a gable face (`gable`) rises to the ridge, a `step` stands between two roof faces inside a part, and a face on an edge shared with a neighbouring part (`party`) shows only what stands above that part's roof. The pane draws each outline as it comes, a little darker than the parts' sand, and counts them in the toggle, **Buildings (N parts, M roof planes, K walls)**.
- **An older server** sends no wall surfaces. The pane then draws a wireframe prism: a copy of each part's outline raised by `height − min_height` (OSM's `height` is measured from the ground), joined to the outline with an upright at every vertex, as before. It is an extent inferred from the part's height tags rather than from its roof planes. A part with no `height` uses `height_m`; with neither, only the outline is drawn. The toggle's help says which the server sent.
- **Roof planes** are closed planar polygons with their z. The pane fills each by ear clipping in the plane it lies in, so a steep face fills as well as a flat one, and outlines it. **Roofs by** switches the colouring between slope and the `roof:shape` of the part named by the plane's `part` tag; it rebuilds the overlay.
- **Model ids.** BUILDINGS names a model of its own. Only the registered layers' `model_id` is the site model's, so streaming buildings does not change the model the pane compares after a decision. The W4–W7 layers are not streamed again after a decision.

## Paint mode: the street painter

No public layer says which stretch of kerb is a driveway, where cars park, or where lanes run, so an operator marks them once per site (Gerardus `docs/plans/VECTOR-MAP.md` §4.1). The marks are the operator's reading of the street, recorded with their author and edit time; they are not measurements. Paint mode is where: it replaces the web Streets page, now the read-only [Site viewer](../../lidar/LIDAR.md#site-viewer) at `/lidar/sites`, and ports that editor's logic (the snap, rows of spaces, a lane's direction, the save diff and Undo).

**Getting there.** Place the sensor from its hint, then press **Paint streets**, in the alignment pane's header or in the main toolbar beside **Align**. It is disabled, with the reason as its tooltip, until a reference is placed, and when the server answers `ListStreetUse` `UNIMPLEMENTED`. The pane's sections give way to the painter's while it paints; pressing it again, closing the pane, choosing another site or losing the server leaves paint mode.

**The plan view.** The camera snaps to an orthographic view looking straight down the site frame, north up and east to the right, centred over the sensor. It pans (drag empty ground, or right-drag) and zooms (scroll or pinch, about the pointer), and does not turn: the renderer refuses orbiting, its own pan and zoom, and the arrow keys while the plan view holds the camera. Leaving paint mode puts back the camera the operator had. The camera is built in the site frame and carried into the sensor frame by the reference's own model matrix, so a pixel maps to one site (east, north) at any height, and a click lands on the ground under it: the ground mesh's height there, else the nearest kerb's or street's height within 60 m, else 0. Gerardus drapes every vertex again on save, so these heights are only for drawing.

**Under the marks.** The pane's layers toggles for kerbs and islands, the City's streets and intersections, the DataSF boxes and footprints, OSM ways, the ground mesh, registered scans and 3DEP points. The marks themselves are the site's street use (`STREET_USE`), read with `ListStreetUse` rather than streamed, because the painter needs each mark's id, author and row. **Show the marks outside paint mode** keeps them drawn in the 3D view.

**Tools.**

| Tool     | How                                                                                                                                                                     | Tags                                |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- |
| Surface  | Click the corners; Return or a double-click finishes. A closed polygon (`FEATURE_KIND_SURFACE`)                                                                         | `surface`                           |
| Lane     | Click along the lane in its direction of travel; Return or a double-click finishes. **Reverse** turns a selected lane round                                             | `lane_type`, `width_m`              |
| Space    | Click the corners; Return or a double-click finishes                                                                                                                    | `space_type`, `count`, `time_limit` |
| Row      | Click two points along the kerb: `count` equal spaces run between them, **Depth** deep, on the side towards the nearest centreline; **Away from the street** flips them | `space_type`, `count`, `time_limit` |
| Driveway | Click the two ends of the kerb cut                                                                                                                                      | none                                |
| Select   | Click a mark to select it and edit its tags. Drag a vertex to move it; drag (or click) an edge to add one; right-click a vertex to remove it. Delete removes the mark   | as above                            |

The values offered are those Gerardus's contract lists: `surface` carriageway, sidewalk, verge, apron, median; `lane_type` general, bus, bike, turn; `space_type` metered, permit, loading, disabled, free. A vertex is never removed below what its kind needs (three for a polygon, two for a line), and a driveway stays one span.

**Snapping.** While drawing, the cursor shows where the next vertex will go: a green ring when it snapped, a white cross when not. A vertex snaps, as a preview, to a vertex within 0.5 m of a kerb, a centreline (the City's, or OSM's roads where the City sent none), a box wall's foot or footprint, or another mark; otherwise to the nearest point along one of their edges within 0.5 m. A vertex is preferred to a nearer edge, so marks meet end to end. A mark being moved does not snap to itself.

**Keys.** Return finishes a mark; Escape cancels it, or clears the selection; Delete or Backspace takes back the last vertex placed, or deletes the selected mark; a right-click while drawing takes back the last vertex. Edit ▸ Undo and Redo (⌘Z, ⇧⌘Z) step through the unsaved changes, and while drawing Undo takes back a vertex. **Revert** drops every unsaved change.

**Saving.** **Save** sends one `EditStreetUse`: the site, the author (required: Gerardus records who marked each feature, and the button stays off without one), the note, and the difference between the marks as read and as they stand: a remove for each saved mark deleted, then an upsert for each new mark (`feature_id` empty) or changed one. A mark moved and moved back sends nothing, and heights are not compared. The batch is applied whole or not at all; a refusal keeps the unsaved marks and shows Gerardus's message. Gerardus answers with every current mark, snapped within 0.5 m and draped, and these replace the local ones. Its problems (`"<feature id>: message"`) are shown under the mark they name, in the marks list and the selected mark, with a red halo round it in the view; a problem naming no mark is shown under Save. Ids hold colons, so they are matched as prefixes, longest first.

### What the painter draws

| Mark          | Drawn as                                                                                                                     |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Surface       | Filled at 30 % opacity and outlined, by `surface`: carriageway grey, sidewalk sand, verge green, apron orange, median violet |
| Travel lane   | Its centreline with an arrowhead every 8 m in the direction of travel, over a faint band `width_m` across, by `lane_type`    |
| Parking space | Outlined, faintly filled, with a divider between spaces when `count` is above 1, by `space_type` (the web editor's colours)  |
| Driveway      | A band 0.8 m across along the kerb, pink                                                                                     |
| Selected      | A white band round the mark, with a handle on each vertex; the vertex being dragged green                                    |
| Problem       | A translucent red band round the mark                                                                                        |

Fills and bands are triangles on the debug-line pipeline, lifted 0.15 m above the vertices. While the plan view holds the camera they draw over everything; outside it they are depth-tested like the reference.

## What the pane shows

- The server's `PoseSummary` for the hint pose: heading with its 1σ, pitch and roll, height above the 3DEP ground, position on NAD83(2011), and NAVD88 height. Ellipsoidal height reads "not given" until Gerardus wires in GEOID18.
- The nudge as east, north and up offsets and a heading change, computed in the client. The server never sees a summary of a nudged pose until it is registered or accepted.
- The background snapshot that Register would upload, and whether the grid has settled. An unsettled snapshot is not uploaded, scored or registered.
- The registration's state, gates and hypotheses, and the fit of the pose drawn now; the site model's residuals beside the gates when the fit used the model.
- Which scan Register uses: the background, or the accumulated frames with their point counts and `capture_ref`.
- A refinement, in its own column: a verdict, then per scan its state, move, what accepting records and rank, with its gates and scores folded under it, then its scan pairs, folded.
- The site's records, newest first. Clicking one snaps the drawn pose to where that decision put the sensor (a rejected record has no pose, and cannot be clicked); the row stays highlighted until the pose is moved off it, and Accept then records the pose as manually placed. On placement the newest record with a pose is drawn by default, this capture's own (by `capture_ref`) before any other at the site; **Reset nudges** goes back to the hint.

## Known limits

- The street painter is tested against a fake Gerardus, not yet against a server with `FEATURE_KIND_SURFACE`, which is being built alongside it.
- The W4–W7 layers are tested against a fake Gerardus with the contract's tags (Gerardus `f3beec1`, which merged the roofline walls of banshee-data/gerardus#35), not yet against a server that derives them. Wall surfaces are drawn as outlines, not filled. A sign's face has no facing in the contract, so it is drawn as a thick vertical segment, not a plate.
- A lane's width band and a driveway's span are drawn segment by segment, so their corners show small gaps or overlaps where a line turns.

- OSM building outlines sit about 0.8 m east of the 3DEP points at the measured sites, and named intersections sit 9 to 34 m from the eye-read positions. Both are in the data, not the drawing; align to the 3DEP points.
- The sensor model in a scan header is always `hesai-pandar40p`.
- **Accumulate frames needs a pcap replay, or a full-frames run's VRLOG** (the spike's q4). velocity.report split-streams foreground points only, so the pane switches its full frames on for the collection. Every VRLOG recorded before `frame_mode` existed, and every run started without **Full frames**, holds foreground points only, and is refused. This has been checked in tests against fake servers, not yet on a real replay.
- velocity.report's full frames classify points 0 (background) and 1 (foreground) only: ground is in class 0, so "background and ground" means class 0 in practice.
- A full-frames replay streams about 70,000 points a frame to the visualiser while it runs, and a full-frames VRLOG is correspondingly larger than a foreground one.
- A pcap replay started with **Full frames** keeps them on until it ends: the pane's switch-off is refused (409) and only logged.

- The visualiser's client passes on at most about 30 frames a second, so above 3× playback of a 10 Hz capture the N frames are not consecutive; `capture_ref` still names the first and last.
- `lidar-state-estimation-baseline`'s `siteCalibration` rotates the sensor's +X onto `north_azimuth_deg`. By the measurement above, +Y lies at 180° − N, so +X lies at 270° − N: that transform is wrong too. It is not used here.

## Safe writes and conflicts (2026-10-25)

The vendored alpha contract includes Gerardus #33's safe-write requirements.
Every decision carries the record the operator read and a request ID. Street
updates and removals carry their original row IDs; creates have no expected row.
An unchanged retry after an unknown outcome retains the entire request and its ID, so a lost response
cannot create a second contribution. Changing an unresolved request is refused
until the operator reconciles it with a deliberate read.

A conflict keeps the pose, note or street draft in place. The client reads the
current server state separately and never replaces an expected revision and
resubmits automatically. Use the records refresh to inspect the current log
before making a new review decision. Reloading street marks discards the street
draft explicitly; copy any wanted changes before doing so.

Joint refinement acceptance uses one `ReviewRegistrations` RPC for every j1.
There is no sequential fallback for an older server. A failed batch accepts no
partial subset; after a connection failure retry the identical batch, and after
a stale-head refusal refine again. Reads in the web site viewer remain read-only.

Session tests exercise the real visualiser state/client contracts against a fake
service, including lost responses, conflicts and atomic batch refusal. They do
not establish production deployment, storage recovery or physical scan quality.

The fake applies the same write rules as Gerardus, so a client that omits a field
fails its tests as it would fail against the server:

| Request                                             | Gerardus answers      |
| --------------------------------------------------- | --------------------- |
| `request_id` empty, over 128 bytes or not printable | `INVALID_ARGUMENT`    |
| Same `request_id` with the same content             | The saved answer      |
| Same `request_id` with different content            | `ABORTED`             |
| Review without `expected_record_id`                 | `FAILED_PRECONDITION` |
| `expected_record_id` not the scan's latest record   | `ABORTED`             |
| Street update or removal without `expected_row_id`  | `FAILED_PRECONDITION` |
| `expected_row_id` not the feature's current row     | `ABORTED`             |
| Street create with a non-empty `expected_row_id`    | `INVALID_ARGUMENT`    |

The fake keeps only successful writes, as Gerardus does, so a refused request
can be corrected and sent again. It can also apply a write and then lose the
answer. The tests use that to check that an unchanged retry adds no second
record or mark.

A definite validation refusal keeps the draft editable and gives a corrected
intent a new request ID. A transport/internal failure retains the pending ID
until its outcome is reconciled. Review heads follow the server's append order,
independently of the timestamps used to display the records.
