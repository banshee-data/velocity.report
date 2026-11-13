# Road surface review

The visualiser's view of Gerardus's road surface preview (`REFERENCE_LAYER_ROAD_SURFACE`, 17): filled carriageway patches fitted on 2023 3DEP ground inside hand-authored or City-kerb boundaries, with the evidence beside them and a road-following driving camera. It needs a Gerardus connection and nothing else: no sensor, no recording.

- **Status:** prototype. The contract's support classes and boundary provenance are meant to last; its 0.5 m grid, its mesh payload and its use of `Site.frame` are scaffolding that the parametric road replaces.
- **Plan and contract:** Gerardus `docs/plans/GGP-ROAD-PROTOTYPE.md` and `docs/INTERFACE-VISUALISER.md` §14, vendored as [align.proto](../../../proto/gerardus/align/v1alpha1/align.proto)
- **Code:** [RoadSurface.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadSurface.swift) (model and store), [RoadSurfaceLoader.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadSurfaceLoader.swift) (loading), [RoadSurfaceStyle.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadSurfaceStyle.swift) (what is drawn), [RoadReviewSession.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadReviewSession.swift) and [RoadReviewPane.swift](../../../tools/visualiser-macos/VelocityVisualiser/UI/RoadReviewPane.swift) (the pane), [RoadDrive.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadDrive.swift) and [RoadDriveSession.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadDriveSession.swift) (the driving view), [RoadWorkingSet.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadWorkingSet.swift) and [RoadTraverse.swift](../../../tools/visualiser-macos/VelocityVisualiser/Alignment/RoadTraverse.swift) (bounded loading and its measurement)
- **Tests:** [RoadSurfaceTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadSurfaceTests.swift), [RoadReviewTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadReviewTests.swift), [RoadDriveTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadDriveTests.swift), [RoadBoundedLoadingTests.swift](../../../tools/visualiser-macos/VelocityVisualiserTests/RoadBoundedLoadingTests.swift), on Gerardus's committed fixture in [Resources/road-surface](../../../tools/visualiser-macos/VelocityVisualiserTests/Resources/road-surface/README.md)

## Opening a corridor

Press **Road** in the toolbar. The pane lists the prepared sites the connected Gerardus server holds (`ListSites`). Gerardus has no corridor list: a corridor is found by asking a site's bundle for layer 17, and its patches arrive with the corridor's header. The visualiser therefore opens the site you choose, then reads every prepared site whose origin lies within 350 m of the patches found, up to twice, and merges them into the first site's frame.

- The layer is requested only when `GetServerInfo.road_surface_version` is 1. A version 0 server, a server older than the layer (`INVALID_ARGUMENT unknown layer 17`) and a site whose layer completes empty each say so in the pane.
- A patch id that arrives from two bundles is kept once. The same id with a different `geometry_hash` is shown as a conflict.
- A patch with a triangle on a missing vertex, an index out of range or mismatched array lengths is refused and counted, never drawn.
- A bundle the server does not hold is named. A failed or cancelled load keeps what arrived and says it is partial. A late answer for a corridor that has since been replaced is discarded.
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

**Start driving** builds a route from a patch's profile and takes the camera under the `drive` holder (orbit, flight and the plan view are refused while it is held; **Overhead** gives it back and frames the corridor). Speed, eye height (1.5 m to start), pause and a position slider control it.

- The path is the profile's; the height is the mesh under it. A junction patch has no profile, so the route is bridged between the arms' ends along a chord and every sample must lie on the junction's mesh. At a junction, the arm taken can be chosen; the default is the straightest.
- The camera **stops**, and says why, at a missing height, at geometry that is not loaded (a junction patch absent, a corridor still loading or partly loaded) and at an unresolved choice (two roads continuing straight on at different heights, as stacked decks). A scrub across a gap stops at its start. No height is carried across a missing station.

## Bounded loading

While driving, the view draws patches within 75 m of the eye, asks the server for the 150 m around it (`centre`, `radius_m`) on a worker whenever the eye has moved 37.5 m, and drops meshes beyond 200 m, keeping each dropped patch's skeleton (rings, profile, stats, hashes) so the route can pass it. A route stopped for lack of geometry continues when more arrives, from where it stopped, so the stretch already driven keeps its heights. Overlays are built on a worker and swapped in between frames. **Overhead** reads the whole corridor again.

**Traverse** drives the route in a loop for a set time and measures it: the frame interval (p50, p95, p99, max), the app's tick, the renderer's encode and GPU time, draw calls, resident patches and mesh bytes, and the process footprint. **Copy note** gives a Markdown summary with the Mac, build, settings and input hashes. The provisional target is p95 ≤ 16.7 ms.

A scripted first-pass review (screenshots of each step and a timed traverse) runs with `VV_ROAD_AUTOPILOT=<folder>`, optionally with `VV_GERARDUS_ADDRESS`, `VV_ROAD_SITE` and `VV_ROAD_TRAVERSE`; it does nothing in a normal launch. The app is sandboxed, so a folder outside its container is replaced by the container's temporary folder, which the run prints.

## Reviews

A visual review in the visualiser and independent field validation are separate entries: the visualiser shows how the fit agrees with its own aerial acquisition, not where the road is.
