# Road surface review

The visualiser's view of Gerardus's road surface preview (`REFERENCE_LAYER_ROAD_SURFACE`, 17): filled carriageway patches fitted on 2023 3DEP ground inside hand-authored or City-kerb boundaries, with the evidence beside them and a road-following driving camera. It needs a Gerardus connection and nothing else: no sensor, no recording.

- **Status:** prototype. The contract's support classes and boundary provenance are meant to last; its 0.5 m grid, its mesh payload and its use of `Site.frame` are scaffolding that the parametric road replaces.
- **Plan and contract:** Gerardus `docs/plans/GGP-ROAD-PROTOTYPE.md` and `docs/INTERFACE-VISUALISER.md` §14, vendored as [align.proto](../../../proto/gerardus/align/v1alpha1/align.proto)
- **Code:** [RoadSurface.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadSurface.swift) (model and store), [RoadSurfaceLoader.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadSurfaceLoader.swift) (loading), [RoadSurfaceStyle.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadSurfaceStyle.swift) (what is drawn), [RoadReviewSession.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadReviewSession.swift) and [RoadReviewPane.swift](../../../tools/visualiser-macos/VelocityVisualiser/UI/RoadReviewPane.swift) (the pane), [RoadDrive.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadDrive.swift) and [RoadDriveSession.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadDriveSession.swift) (the driving view), [RoadWorkingSet.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadWorkingSet.swift) and [RoadTraverse.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadTraverse.swift) (bounded loading and its measurement)
- **Tests:** [RoadSurfaceTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadSurfaceTests.swift), [RoadReviewTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadReviewTests.swift), [RoadDriveTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadDriveTests.swift), [RoadBoundedLoadingTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadBoundedLoadingTests.swift), on Gerardus's committed fixture in [Resources/road-surface](../../../tools/visualiser-macos/VelocityVisualiserTests/Resources/road-surface/README.md)

## Opening a corridor

Press **Road** in the toolbar. The pane lists the corridors the server names (`ListRoadCorridors`, when `road_corridors_listed` is set) and opens one through exactly the sites it says hold it. It also lists the prepared sites (`ListSites`). On a server that cannot list corridors, a corridor is found by asking a site's bundle for layer 17, whose patches arrive with the corridor's header; the visualiser then opens the site you choose and reads every prepared site whose origin lies within 350 m of the patches found, up to twice, and merges them into the first site's frame.

- The layer is requested only when `GetServerInfo.road_surface_version` is 1. A version 0 server, a server older than the layer (`INVALID_ARGUMENT unknown layer 17`) and a site whose layer completes empty each say so in the pane.
- A patch id that arrives from two bundles is kept once. The same id with a different `geometry_hash` is shown as a conflict.
- A patch with a triangle on a missing vertex, an index out of range, mismatched arrays or an invalid horizontal coordinate is refused and counted, never drawn. NaN heights on missing ground remain valid.
- Opening a named corridor filters every bundle and loading window to that corridor, including headerless continuation chunks. Returning to Overhead keeps that selection.
- A bundle the server does not hold is named. A failed or cancelled load keeps what arrived and says it is partial. A stream that ends without `layer_complete` is partial even if the RPC succeeds. Late answers for replaced corridors and cancelled or superseded loading windows are discarded.
- Neighbouring bundles are shifted into one frame by the difference of their geodetic origins. Against the fixture's control points that agrees to within 5 mm; the layer itself states 1 mm between frames.

## What is drawn

| Layer                                             | Meaning                                                                                                                                                                        |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Road surface                                      | Filled triangles. Support shading: green observed, amber interpolated. Missing ground has no triangle, so it is an open gap, and its edge is drawn red. Island holes stay open |
| Wireframe                                         | The triangle edges                                                                                                                                                             |
| Centreline                                        | The patch profile; the line breaks at a missing station                                                                                                                        |
| Boundary rings                                    | Coloured by the edge's source: kerb pair, City kerb, authored cut. An **inferred width** is orange and dashed, so it is never read as a surveyed edge                          |
| City kerbs, City centrelines, class-2 3DEP points | The existing reference layers, drawn from the first site's bundle                                                                                                              |

Click a patch to inspect it: its id, CNN, boundary source and the counts of its edge sources, the date of the ground observation, area by support class, the rider line's support, held-out point count and residual median, RMS and p95, and the safety components (all C1–C18 `NOT_ASSESSED`). The coverage block shows the corridor's inventory, surfaced, inferred and missing lengths and its gaps, with the words "this is a corridor, not the whole park".

## Driving view

Closing or replacing the corridor stops its driving clock and releases its camera. The road overlay, picking ray and driving camera share their own review frame, independently of an open capture's alignment transform.

**Start driving** builds a route from a patch's profile and takes the camera under the `drive` holder (orbit, flight and the plan view are refused while it is held; **Overhead** gives it back and frames the corridor). Speed, eye height (1.5 m to start), pause and a position slider control it.

- The path is the profile's; the height is the mesh under it. A junction patch has no profile, so the route is bridged between the arms' ends along a chord and every sample must lie on the junction's mesh. At a junction, the arm taken can be chosen; the default is the straightest.
- The camera **stops**, and says why, at a missing height, at geometry that is not loaded (a junction patch absent, a corridor still loading or partly loaded) and at an unresolved choice (two roads continuing straight on at different heights, as stacked decks). A scrub across a gap stops at its start. No height is carried across a missing station.

## Bounded loading

While driving, the view draws patches within 75 m of the eye, asks the server for the 150 m around it (`centre`, `radius_m`) on a worker whenever the eye has moved 37.5 m, and drops meshes beyond 200 m, keeping each dropped patch's skeleton (rings, profile, stats, hashes) so the route can pass it. A route stopped for lack of geometry continues when more arrives, from where it stopped, so the stretch already driven keeps its heights. Overlays are built on a worker and swapped in between frames. Duplicate chunks reuse the driving height index, and style or selection changes reuse the mesh audit while the mesh identities stay the same. The overlap check tests triangle intersections, including crossings where neither centroid lies inside the other triangle. **Overhead** reads the whole corridor again.

**Traverse** drives the route in a loop for a set time and measures it: the frame interval (p50, p95, p99, max), the app's tick, the renderer's encode and GPU time, draw calls, resident patches and mesh bytes, and the process footprint. **Copy note** gives a Markdown summary with the Mac, build, settings and input hashes. The provisional target is p95 ≤ 16.7 ms.

A scripted first-pass review (screenshots of each step and a timed traverse) runs with `VV_ROAD_AUTOPILOT=<folder>`, optionally with `VV_GERARDUS_ADDRESS`, `VV_ROAD_SITE` and `VV_ROAD_TRAVERSE`; it does nothing in a normal launch. The app is sandboxed, so a folder outside its container is replaced by the container's temporary folder, which the run prints.

## Reviews

A visual review in the visualiser and independent field validation are separate entries: the visualiser shows how the fit agrees with its own aerial acquisition, not where the road is.

### 2026-11-13: first pass against the fixture

Real corridor data was not available: `arrow` has been offline for 50 days, and the Gerardus server on `arrow-worker:50061` (0.2.0, `road_surface_version` 1, 86 prepared sites) returns an empty layer 17 for every site in Golden Gate Park, so `ggp-mlk-1` has not been built there. This review is **against Gerardus's committed fixture only**, served locally to the app; it does not review the park corridor, and no checklist item that needs real ground (canopy, parked cars, rider-line support on real data) was exercised.

Run: MacBookPro18,3 (Apple M1 Pro), macOS 15.7.7, Release build 0.5.1-pre52 (git SHA not stamped into the app), eye 1.5 m, speed 8 m/s, radii 75/150/200 m. Inputs: corridor `fixture`, manifest `180763fd…eda50`, build `13667110…b506d7`, patch geometry hashes `ec95333d…d57f7`.

| Checklist item                                              | Result                                                                                                                                                                                         |
| ----------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Boundary against evidence, inferred widths easy to identify | Inferred edge on the bend drawn orange and dashed; kerb-pair edges solid blue. No kerb or class-2 context in the fixture, so no comparison against evidence                                    |
| Support under canopy; gaps visible                          | The interpolated disc under "canopy" is amber; the unsupported patch has no triangles and its edge is red                                                                                      |
| Follow bend, junction and island                            | Route straight → junction → bend: no station off the road, no step over 0.35 m (unit tests); island arm stays outside the hole. Inverted faces and overlaps were not looked for on real meshes |
| Cross a bundle edge both ways                               | Loaded through either site first: vertices agree to 5 mm, heights to 1 cm (unit test); cancelled and late requests discarded                                                                   |
| Missing patch and failed load                               | The camera stopped at 45 m on the missing ground with "no road height (missing ground)"; failure, partial and unloaded states are covered by tests                                             |
| Counts                                                      | 5 patches, 5 duplicates dropped, 0 conflicts, 0 refused; inventory 268 m, surfaced 148 m (55 %), inferred 67 m, missing 16 m, 1 gap; held-out RMS 0.011–0.012 m on the patches inspected       |

Traverse, 120 s of looping the 60 m route (7,170 frames), after the redraw was moved onto the display's cadence and the run was kept out of App Nap: frame interval p50 16.67, p95 16.85, p99 18.18 ms, max 67.6 ms; 18 frames over 20 ms (a missed vsync). The p95 is 0.15 ms over the 16.7 ms target, which is vsync jitter, so read the target as met to within a vsync and the p99 and the 18 slow frames as the real figure. The first run of this review (a 60 Hz timer asking for on-demand frames, no App Nap guard) gave p95 23.65 ms and a 411 ms maximum: the 411 ms was the idle gap before the first frame being counted as a frame, and the slow tail was pacing, not load. Work per frame is small: app tick p95 0.11 ms, render encode 0.11 ms, GPU 0.37 ms, 4 draw calls. Footprint 172 → 162 MB (224 MB peak) but +22 MB over the second half, so it has not been shown to settle in two minutes. The slowest frame (67.6 ms, pass 5) is not explained. A ten-minute run of the app's own per-frame work at 60 Hz of virtual time (36,000 frames, working set 25/40/40 m so patches churn) had tick p95 1.07 ms in a Debug build and a flat footprint (62 → 58 MB). Neither is the ten-minute rendered traverse of the plan, and neither used a real corridor.

The mesh audit on the fixture reads 0 inverted, 0 degenerate and 0 overlapping triangles, and 68 seam vertex pairs with a worst height step of 0.0 mm (limit 5 mm). The audit runs in the pane on every loaded corridor, so the inverted-face, overlap and height-discontinuity items of the checklist are checked by machine on real meshes as soon as one exists; whether the camera cuts a corner outside the road is covered by the on-road route test.

![Overhead, support shading](../../images/road-review-overhead-support.png)
![Driving view, mid-route](../../images/road-review-drive.png)
![Driving view, stopped at missing ground](../../images/road-review-drive-stop.png)
![Pane with patch inspector and coverage](../../images/road-review-inspector.png)

Findings for Gerardus: there was no call that listed corridors, so the visualiser found them through site bundles. [banshee-data/gerardus#85](https://github.com/banshee-data/gerardus/pull/85) adds `ListRoadCorridors` (and `road_corridors_listed`); the Road pane lists the corridors it returns and opens one through exactly the sites it names, and falls back to site probing on a server without it. `ggp-mlk-1` must be built on `arrow-worker` before a real review.
