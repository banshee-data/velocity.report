# SF district time trials and Gerardus handoff

This defines the frontend contract for district-only emoji runs and the separate Gerardus
boundary/overlap work needed to supply it. It leaves the existing city geometry and quest
anchors unchanged.

- **Status:** Frontend consumer implemented; authoritative district data and Gerardus work pending.

## Scope and open choice

The frontend accepts named district sets without assuming their count or boundary vintage.
The outstanding product choice is whether district means the 11 supervisor districts or
named analysis neighbourhoods. Use a source-specific scheme identifier and revision once
that choice is made; do not silently treat those two boundary sets as equivalent.

No district data is bundled with this change. The live game therefore offers Whole city and
marks district runs unavailable for the current map. A valid published district block enables
the selector, boundary overlay, filtered pickups, district start, completion and separate best
times. The frontend does not make a live Gerardus request or invent geographic membership.

## Published metadata contract

Add an optional `districts` object to `public_html/src/sf/quest-route.json` when the map export
has authoritative data. The example below illustrates the schema, not an actual district.
Coordinates are metres in the game's planar frame: EPSG:7131 less the manifest origin.

```json
{
  "districts": {
    "version": 1,
    "scheme": "source-defined-district-scheme",
    "revision": "source-boundary-and-assignment-revision",
    "frame": "EPSG:7131",
    "origin_epsg7131": [48881.186, 25109.973],
    "areas": [
      {
        "id": "stable-district-id",
        "name": "District display name",
        "overlapTiles": ["example-tile-a", "example-tile-b"],
        "targetTiles": ["example-tile-a"],
        "geometry": {
          "type": "Polygon",
          "coordinates": [
            [
              [0, 0],
              [100, 0],
              [100, 100],
              [0, 100],
              [0, 0]
            ]
          ]
        }
      }
    ]
  }
}
```

`geometry` accepts Polygon and MultiPolygon, with closed outer/hole rings. The origin must
match the current tile manifest exactly. District IDs are unique nonempty strings; `city`
and `unavailable` are reserved UI values. An eligible area needs at least one target.

`overlapTiles` names known L13 tiles intersecting the district; overlap is many-to-many.
`targetTiles` names existing quest anchors whose actual pickup coordinates belong to that
district. A tile overlapping two districts does not automatically put its emoji in both.
Assign a boundary pickup to one district using a documented deterministic tie rule. The
frontend rejects unknown/duplicate targets, repeated ownership, invalid geometry, projection
mismatches and pickups outside the supplied boundary. It keeps the city run available if the
optional district block is malformed. Legacy tile ownership and actual pickup coordinates
are distinct: overlap cells are not used as a substitute for point membership.

The first listed target is the district's fixed starting intersection. Publish a stable order.
Choosing a district starts a fresh run there and filters the seeker, world beacons, map dots,
and completion requirement to that target set. Its minimap shows the district boundary and
fits the area/car. The city starting-place selector is disabled for a district run. Switching
back restores the city starting place. Existing city best-time keys are preserved; district
records also include scheme, revision, district ID, start and target membership.

## Separate Gerardus implementation

This work belongs in its own chat and PR in Gerardus:

1. Confirm the district boundary family and authoritative DataSF source, licence, revision and
   acquisition evidence. Inspect the existing streets' district/neighbourhood fields, but do
   not infer polygon boundaries from street names or district-number labels.
2. Add a district overlay/query using Gerardus's existing server conventions. Transform source
   coordinates correctly and support islands, holes and shared boundaries. Expose provenance
   and stable IDs alongside the geometry.
3. Compute L13 overlaps using actual S2 cell geometry, not the game's approximate rendering
   regions or the manifest's square half-widths. Separately compute quest-point membership.
   Check legacy anchors near cell/district edges explicitly rather than silently moving them.
4. Connect the map-export recipe to the published metadata contract above. Record the boundary
   and assignment revision, preserve tile payloads, and avoid a city geometry rebake merely to
   add this index. Keep raw source/analysis outputs outside Git.
5. Verify that every included target lies in its district, ownership is deterministic,
   overlapping cells remain many-to-many, all references exist, and the frontend can finish
   each district without collecting outside pickups. Measure the added compressed payload.

Read both repositories' agent instructions before implementation. The frontend consumer and
tests are `public_html/src/js/sf-district-trials.js` and its adjacent tests in velocity.report.
Use the [driving-game reference](../ui/sf-driving-view.md) for the existing run and recovery rules.

## Validation

Frontend tests cover missing metadata, invalid membership/projection, polygon holes/islands,
separate record scopes, district-only completion and seeker filtering. Browser checks use an
explicit test-only district fixture to exercise selection and the overlay; that fixture is
not published as real SF boundary data. Validate the actual exported source separately in the
Gerardus change. Touch controls and compass clarity are independent of district availability.
