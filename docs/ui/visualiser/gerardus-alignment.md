# Gerardus alignment pane

The visualiser's client for the Gerardus alignment service: it draws USGS 3DEP aerial points and OpenStreetMap ways over a capture's settled background, places the sensor from its eye-read position, lets the operator nudge it, and records the operator's decision in Gerardus's alignment log.

- **Plan:** [lidar-visualiser-gerardus-alignment-plan.md](../../plans/lidar-visualiser-gerardus-alignment-plan.md) (the work still to come)
- **Contract:** [proto/gerardus/align/v1alpha1/align.proto](../../../proto/gerardus/align/v1alpha1/align.proto), vendored from banshee-data/gerardus

## Summary

Gerardus is a separate service in its own repository. It prepares reference data for a survey site and registers sensor scans against it. The visualiser talks to it directly on a second gRPC channel, beside its channel to velocity.report; velocity.report's Go server is not in the path. The dependency is optional: when no Gerardus server answers, the app shows no alignment control and behaves as it did before.

What ships is the manual path of Gerardus MVP 0: the reference, a hint pose, local nudges, and an accepted pose recorded as manually placed. Automatic registration (ICP), pose scoring and tie points are not built in Gerardus yet, so the pane does not call `EvaluatePose` or `SolveTiePoints`.

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

| Step             | What the pane does                                                                                                                          | Gerardus RPC                                     |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------ |
| Open the pane    | Lists velocity.report's located replay cases and picks the open capture's                                                                   | `GetServerInfo` (on connect)                     |
| Place from hint  | Prepares the site, places the sensor from the eye-read position and the typed `north_azimuth_deg`, and streams 150 m of reference around it | `PrepareSite`, `PoseFromHint`, `StreamReference` |
| Nudge            | Moves the reference along the sensor's axes, or turns it about the sensor                                                                   | none                                             |
| Register         | Uploads the settled background once per snapshot, then registers it from the pose drawn now                                                 | `UploadScan`, `Register`                         |
| Accept or reject | Records the pose drawn now as manually placed, or a rejection with its reason                                                               | `ReviewRegistration`, `ListRegistrationRecords`  |

Register comes back `FAILED` while ICP is unbuilt; the registration carries the pose it was given, and Accept records the operator's pose against it. The accepted record is not yet written back to velocity.report: that needs `PUT /api/lidar/sites/{l16}/pose` (Gerardus MVP step 2), and the code marks the place with a TODO.

## Which site

Gerardus keys everything by the site's S2 L16 token, and its hint needs the eye-read latitude and longitude. The pane reads both from velocity.report's replay cases (`GET /api/lidar/scene-map`), and finds the open capture's case by, in order:

1. the run's `replay_case_id`, for the run loaded from the run browser or the one the server is replaying (a VRLOG replay names its run only by its directory);
2. the run's source file, matched against each case's capture files;
3. the published capture stem `<start>_<L13 display>_<site id>`, whose L13 cell names the site, with the site id's words choosing between cases in a shared cell.

When nothing matches, the operator picks the site from the list.

The operator types `north_azimuth_deg` from `tools/s2-archive/site-index.json`. No velocity.report API serves it, and serving it needs a stored column, which belongs to the step-2 pose record. The pane remembers the typed value per site.

`north_azimuth_deg` is not the bearing of the sensor's +Y. It is measured with the web scene viewer's "Align from above" tool, against that viewer's camera. The pane sends `(180 − north_azimuth_deg) mod 360` as `PoseHint.heading_deg`, the bearing of +Y, and shows that value beside the field. The rule is measured, not derived. Gerardus's automatic registration searched every heading and ignored the hint. At three sites whose `north_azimuth_deg` is 35, 131 and 51, it found +Y at 143.5°, 49.4° and 129.9°: 180 − N each time, within 1.5°. A mirrored copy of the scan fitted much worse, so the sensor frame itself is right-handed. An empty field sends no heading, and Gerardus then reports a heading uncertainty of 180°.

## Frames and drawing

The visualiser draws in the sensor frame, velocity.report's pipeline frame: +Y is azimuth zero, +X is 90° clockwise from it, and +Z is up. Reference geometry arrives in the site frame, east-north-up at the centre of the L16 cell. A pose maps sensor to site, `p_site = R(q)·p_sensor + t`, with `q` a Hamilton quaternion, `w` first.

The renderer draws the reference with the inverse of the pose as its model matrix, folded into `Uniforms.modelViewProjection` and `Uniforms.modelView` for those draws only. A nudge replaces that matrix and sends nothing. Each nudge moves the drawing by one step in the sensor frame, and the pose becomes `P·N⁻¹`, so the reference lands where the operator put it.

| Layer         | Pipeline                                                              | Colour                                                                                                                         |
| ------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ |
| 3DEP points   | Point pipeline, `[x, y, z, code / 255, 0]` per point, table colouring | The ASPRS class picks a ramp (ground amber, unclassified violet, building red, and so on); intensity picks the shade within it |
| OSM ways      | Debug-line pipeline, `[x, y, z, r, g, b, a]` per vertex               | By `FeatureKind`: roads yellow, paths light blue, kerbs orange, rails magenta, buildings red                                   |
| Intersections | Debug-line pipeline                                                   | A white cross at each node, and a cyan tick down each approach                                                                 |

The point shader's table mode reads a palette from fragment buffer 0. The reference binds its own 256-entry palette there, so one byte carries both class and brightness and the shader needs no new mode. Draped lines sit 0.10 m above the 3DEP ground so they do not fight its points for depth; this offset is display only. Aerial points draw at half the scene's point size.

When OSM ways are visible, "© OpenStreetMap contributors" (from `Site.sources`) shows in the corner of the scene. The 3DEP attribution shows in the pane.

## What the pane shows

- The server's `PoseSummary` for the hint pose: heading with its 1σ, pitch and roll, height above the 3DEP ground, position on NAD83(2011), and NAVD88 height. Ellipsoidal height reads "not given" until Gerardus wires in GEOID18.
- The nudge as east, north and up offsets and a heading change, computed in the client. The server never sees a summary of a nudged pose until it is registered or accepted.
- The background snapshot that Register would upload, and whether the grid has settled. An unsettled snapshot is not uploaded.
- The site's records, newest first.

## Known limits

- OSM building outlines sit about 0.8 m east of the 3DEP points at the measured sites, and named intersections sit 9 to 34 m from the eye-read positions. Both are in the data, not the drawing; align to the 3DEP points.
- The sensor model in a scan header is always `hesai-pandar40p`.
- `lidar-state-estimation-baseline`'s `siteCalibration` rotates the sensor's +X onto `north_azimuth_deg`. By the measurement above, +Y lies at 180° − N, so +X lies at 270° − N: that transform is wrong too. It is not used here.
