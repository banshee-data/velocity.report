# SF tunnels, overpasses and bridges

This is the handoff between the existing SF game tiles and Gerardus's future layered
road export. It separates improvements possible with the published data from the World
and Physics work needed to drive above and below the same horizontal position.

- **Status:** current-tile marking and HUD improvements implemented; layered roads planned.
- **Anchors:** World and Physics for road levels; UI for visibility and markings; Traffic,
  Goals and Data where connectivity, pickups and provenance depend on the selected level.
- **Consumer:** [SF driving view](../ui/sf-driving-view.md).

## What the existing tiles actually contain

The published version-2 city has 157 L13 tiles and 15,671 road records. Every road row
has five fields: name index, class, one-way flag, XY centreline and paired left/right
kerb widths. Road vertices have no road-surface height, stable road/structure ID,
deck level, tunnel flag, portal, clearance, lane count or paint inventory. Each tile
has a single 8 m ground-height grid, plus building prisms, trees, kerbs and signposts.
Tile and quest files are unchanged by the current frontend work.

The renderer paints asphalt on that ground grid; `groundAtWorld` also puts the car
on it. Two roads crossing in XY therefore share the same surface. A ground grid
cannot simultaneously represent a bridge deck, the street beneath it and a tunnel
below that. Roads and kerbs draped over a hill cannot establish a tunnel opening.
Building prisms can also obstruct an entrance or underpass when their semantics are
wrong; deleting a whole building in XY would remove unrelated observed geometry.

## What can be done without another bake

| Improvement                 | Existing evidence                                                     | Limit                                                                                                                                                     |
| --------------------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Sharper centre dashes       | Two-way flag, ordered centreline, existing road width                 | Implemented as stylised game guidance, not an inventory of observed paint or a passing rule                                                               |
| Continuous dash phase       | Distance along each existing road polyline                            | Continuous across its vertices and terrain ownership boundaries; no stable ID to join separate road rows                                                  |
| Clear road/ground rendering | Original ground triangles and tile ownership regions                  | Paint is clipped onto those same planes and missing-ground holes, with depth-buffer offset; no new road elevation is inferred                             |
| Clearer camera/HUD          | Existing camera poses, compass target and speed                       | Portrait compass and edge-docked mirror keep the central half of the view clear of panels below the status strip; trees/buildings still occlude naturally |
| At-grade collision/recovery | Existing building bases/outlines, trunks and signposted intersections | Remains the current single-surface game; it cannot select the right deck by height/connectivity                                                           |

Vector markings use 0.30 m-wide, 3 m dashes with 3 m gaps, carry phase through
polyline bends and use an unlit paint material for contrast. They reuse the current
1024 px mobile terrain texture instead of increasing its GPU allocation. The width
is a display choice. One-way roads and paths receive no new centre dashes. Lane
dividers, stop bars, crossings, turn arrows and restrictions need an appropriate
inventory; road width and nearby signposts alone cannot establish them, especially
at an apparent crossing that may be grade-separated.

## Gerardus work to hand off

Build on the existing
[Road Surface plan and Bay Bridge approach pilot](https://github.com/banshee-data/gerardus/blob/main/docs/plans/ROAD-SURFACE.md#14-bay-bridge-approach-pilot),
[road-surface preview implementation](https://github.com/banshee-data/gerardus/blob/main/src/road_surface.rs)
and [RoadSurfacePatch/Chunk contract](https://github.com/banshee-data/gerardus/blob/main/proto/gerardus/align/v1alpha1/align.proto).
That plan already identifies separate elevation groups and tunnel gaps; its current
ground-preview version must not be described as implemented multilevel driving.

1. **Identify surfaces and connectivity.** Assign stable road/surface and structure
   IDs, structure kind, direction and connected endpoints. A crossing in XY becomes
   a junction only when the connected road level agrees. Represent ramps and tunnel
   portals as explicit transitions, including transitions between different L13 tiles.
2. **Export supported contact geometry.** Retain separate height profiles or indexed
   contact meshes for the ground street, each deck and connecting ramps. Use reviewed
   class-17 deck support and compatible class-1 continuation as the existing pilot
   proposes; do not use class-2 ground as a deck or label every high return a bridge.
   Missing ground over water must not erase a supported deck. Preserve unsupported gaps.
3. **Handle tunnels explicitly.** Aerial ground cannot observe the tunnel interior.
   Export known portals and a separately labelled interpolated road profile where
   evidence permits; retain missing/uncertain support. Interior scans or another
   supported source are needed for observed walls, ceiling and clearance. A straight
   join between portals is an inference, not a measured tunnel.
4. **Keep contact surfaces separate from obstacles.** Deck tops belong to Road
   Surface. Undersides, piers, walls, barriers and abutments need supported obstacle
   geometry and shared structure IDs, following the pilot's separate-layer gate.
   Do not extrude a deck to terrain as a building, remove everything under its XY
   footprint, or infer safe headroom from the height of the deck top.
5. **Version a bounded game export.** Reuse the Road Surface products in an optional
   per-L13 sidecar or versioned tile extension, with consistent EPSG:7131 origin,
   NAVD88 heights, units, ownership/halo rules, product hashes and source epochs.
   Carry observed/interpolated/missing support and compatibility version. A targeted
   pilot overlay can ship alongside the existing city tiles; a complete city rebake
   is not a prerequisite. This is a proposed consumer contract, not a shipped endpoint.
6. **Add markings as data when available.** Export lane counts, directions, widths,
   crossings, stop lines and arrows keyed to the same surface ID, with source and
   observation dates. Preserve unknowns rather than deriving legal rules from width.

## Game engine work once the export exists

- [ ] Load/validate the optional surface products and include their compressed bytes
      and hashes in asset accounting. Keep old city tiles readable. Show unsupported
      areas honestly rather than claiming the flattened fallback is structure-aware.
- [ ] Select wheel contacts using current surface ID, 3D height, heading and explicit
      connected transitions. Retain that surface through a crossing; nearest XY or
      highest Z alone must not switch the car between a bridge and the road below.
      Brake/wait when the required next surface has not loaded.
- [ ] Render decks and ramps on their own supported surfaces, and openings at known
      tunnel portals without flattening the terrain globally. Project markings onto
      the selected surface. Camera cutaways, if added, affect presentation only.
- [ ] Extend collision/headroom checks for ceilings, piers and barriers. Existing 2D
      building/trunk collision and one ground height are insufficient for an underpass.
- [ ] Make street lookup, intersection connections, pickups and recovery checkpoints
      surface-aware. Do not award an emoji on the deck while driving beneath it or
      reset a car onto a nearby disconnected level. Scope records to the revised route
      and surface products; retain the documented crash interval and timer semantics.
- [ ] Keep future Traffic policies on connected lanes at the same level. An apparent
      crossing does not imply a following/yielding interaction. Add vertical seeker
      cues only when a supported target and route establish the right surface.

## Acceptance before calling structures drivable

Use a deterministic fixture containing two crossing decks, the ground road, a ramp,
a tunnel portal/interior, an unsupported gap and neighbouring building/tree geometry.
Test both driving directions and headings, with fixed above/below/in-car views.
Verify no level switching at crossings; continuous ramp/portal contact; correct street,
pickup and recovery ownership; true obstacles and retained under-deck space; no
floating paint; missing-surface braking; and equivalent results when neighbouring
tiles stream in a different order. Include a cross-L13 transition and interrupted load.

Freeze input/product hashes, frame, evidence/support labels and source dates. Validate
geometry against independent held-out evidence as in the Gerardus pilot, and distinguish
display tolerances from physical clearance or safety claims. Measure frame time, contact
query cost and CPU/GPU memory on a stated mobile workload before expanding the pilot.
