# Road surface preview fixture

A hand-authored `REFERENCE_LAYER_ROAD_SURFACE` (17) layer, `road_surface_version` 1, for VelocityVisualiser to vendor and load before real corridors exist. The contract is in [`align.proto`](https://github.com/banshee-data/gerardus/blob/a365f0eacf67a18b27a51dacb7ad5cc2fbae8d9e/proto/gerardus/align/v1alpha1/align.proto) (`RoadSurfaceChunk`) and [INTERFACE-VISUALISER §14](https://github.com/banshee-data/gerardus/blob/a365f0eacf67a18b27a51dacb7ad5cc2fbae8d9e/docs/INTERFACE-VISUALISER.md#14-road-surface-preview). The plan is [GGP-ROAD-PROTOTYPE](https://github.com/banshee-data/gerardus/blob/a365f0eacf67a18b27a51dacb7ad5cc2fbae8d9e/docs/plans/GGP-ROAD-PROTOTYPE.md).

## Files

| File                     | What                                                                                                                                                                                  |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `stream-80858771f.binpb` | the layer as `StreamReference` sends it to site `80858771f`: length-delimited `StreamReferenceResponse` messages (varint length, then the message; SwiftProtobuf's `BinaryDelimited`) |
| `stream-808587721.binpb` | the same patches through the neighbouring site `808587721`                                                                                                                            |
| `manifest.json`          | site origins, CS13 → site control points, per-patch counts and hashes, file digests                                                                                                   |

Each stream has two messages. The first carries `RoadSurfaceLayer` and two patches, and the second carries three patches with `layer_complete = true`.

## Patches

Local axes are u east and v north, in metres from the CS13 anchor (45 200, 25 900), on the south side of Golden Gate Park. The ground is analytic, NAVD88: z = 12 + 0.02 u + 0.005 v + 0.05 sin(u/15) cos(v/20).

| Patch                 | Exercises                                                                                                             |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- |
| `segment/straight`    | 40 m straight, 8 m wide, kerbs both sides, all observed                                                               |
| `segment/bend`        | 90° left bend, 30 m centreline radius; inferred-width outer edge; an interpolated disc 3.5 m in radius under "canopy" |
| `junction/9000001`    | T junction with kerb returns; shares its three mouths' vertices with the arms                                         |
| `segment/island`      | arm with a 2 m × 16 m island hole that stays open; the driving line keeps right of it                                 |
| `segment/unsupported` | ground missing west of u = −50: NaN heights, no triangles, the profile stops                                          |

`RoadSurfaceLayer.gaps` lists one selected road with no patch (`car_free`). Every patch carries C1–C18 as `NOT_ASSESSED`. Held-out residuals are against synthetic points with ±2 cm of noise.

CNNs 9000001–9000199 are fixture values, not City ids.

## Verifying and regenerating

From this repository, verify the vendored streams and their manifest against the Go wire bindings:

```sh
go test ./internal/lidar/gerarduspb -run TestRoadSurfaceVendoredContract
```

The generator lives in **banshee-data/gerardus**, not in this repository. To regenerate, check out
Gerardus at `a365f0eacf67a18b27a51dacb7ad5cc2fbae8d9e` and run there:

```sh
GERARDUS_REGENERATE_FIXTURES=1 cargo test --test road_surface_fixture
```

Without the variable, the test checks the committed files against the generator. It compares hashes exactly and site-frame floats to 0.1 mm, because libm can differ in its last bit between macOS and Linux.

Copy the two streams and `manifest.json` from Gerardus's `tests/fixtures/road-surface/` together, then rerun the verification here. Preserve this README's pinned source links when updating the vendor.
