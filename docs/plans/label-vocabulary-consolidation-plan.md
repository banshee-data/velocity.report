# Label Vocabulary Consolidation Plan

- **Status:** Phase 1–3.1 completed · Phase 4–5 planned
- **Layers:** Cross-cutting (L6 Objects, API, Web, macOS Visualiser)
- **Related:** [Classification Maths](../maths/classification-maths.md), `proto/velocity_visualiser/v1/visualiser.proto`

## Problem (resolved)

The codebase had **two independent enum-like vocabularies** for track
classification, with overlapping but inconsistent values. The classifier
wrote `"pedestrian"` while user labels used `"ped"`, breaking string
comparison in `isPositiveLabel()` and ground-truth evaluation.

## Current State (after v1.2 + Phase 3)

### Unified vocabulary (proto3 enum authoritative)

String labels are internal; wire protocol uses `ObjectClass` proto enum:

- **UNSPECIFIED** (0): default/unknown
- **NOISE** (1): environmental noise (user-assignable)
- **DYNAMIC** (2): classifier fallback for ambiguous detections (user-assignable)
- **PEDESTRIAN** (3): foot traffic (user-assignable)
- **CYCLIST** (4): pedal cyclists (user-assignable)
- **BIRD** (5): airborne fauna (user-assignable)
- **BUS** (6): public transit (user-assignable)
- **CAR** (7): private automobiles (user-assignable)
- **TRUCK** (8): commercial heavy vehicles (user-assignable)
- **MOTORCYCLIST** (9): motorised two-wheelers (user-assignable)

Classifier outputs 6 positive classes: car, truck, bus, pedestrian, cyclist, motorcyclist + bird (non-positive).
User can assign all 9 classes: all 6 positive + motorcyclist + bird + dynamic + noise. Internal domain model uses string representation; conversion to/from proto enum happens at gRPC boundary.

### Files updated across phases

**Phase 1–2** (unified vocabulary to full-word strings):

- `internal/lidar/l6objects/classification.go`: `ClassPedestrian = "pedestrian"`, `ClassDynamic = "dynamic"`, added truck/motorcyclist
- `internal/api/lidar_labels.go`: Updated `validUserLabels` to exclude `"impossible"`, added new classes
- `internal/lidar/adapters/ground_truth.go`: `isPositiveLabel()` updated for all 6 positive classes
- `web/src/lib/types/lidar.ts`: `DetectionLabel` type and `TRACK_COLORS` aligned (9 classes)
- `web/src/lib/components/lidar/TrackList.svelte`: 7-label dropdown, removed `"impossible"`
- `web/src/lib/components/lidar/MapPane.svelte`: Legend shows 9 object classes
- `tools/visualiser-macos/.../ContentView.swift`: `classificationLabels` array with 7 entries
- `internal/db/migrations/000029_...up.sql`: Converts `"ped"→"pedestrian"`, `"other"→"dynamic"`, `"impossible"→"noise"`

**Phase 3** (proto3 enum wire protocol):

- `proto/velocity_visualiser/v1/visualiser.proto`: Added `ObjectClass` enum (10 values, 0–9), Track field 26: `ObjectClass object_class`
- `internal/lidar/visualiser/grpc_server.go`: Added `objectClassFromString()` converter (string → proto enum)
- `tools/visualiser-macos/.../VisualiserClient.swift`: Added `objectClassLabel()` converter (proto enum → string)
- Tests updated to reflect new enum values and string mappings

**Phase 3.1** (VRLOG replay re-classification):

- `internal/lidar/l6objects/classification.go`: Refactored `Classify` → `ClassifyFeatures` (exported, takes pre-built features without full `TrackedObject`)
- `internal/lidar/visualiser/grpc_server.go`: Added `classifyOrConvert()` bridge function and package-level `replayClassifier`
- `tools/visualiser-macos/.../ContentView.swift`: Track Inspector always shows Class field with "Not classified" fallback

## Completed Phases

### Phase 1: Unify vocabulary (string-based)

✅ **Status: DONE**

Consolidated independent vocabularies. All code uses canonical full-word labels: `"car"`, `"truck"`, `"bus"`, `"pedestrian"`, `"cyclist"`, `"motorcyclist"`, `"bird"`, `"noise"`, `"dynamic"`.

- Removed `"ped"` shorthand and `"impossible"` user label
- Database migration 000029 converts legacy rows
- Go, Swift, TypeScript, Svelte aligned

### Phase 2: Add truck and motorcyclist classes

✅ **Status: DONE**

Classifier v1.2 adds `ClassTruck` and `ClassMotorcyclist` with thresholds and cascade rules. All frontends display 7 classification options + noise.

### Phase 3: Proto3 enum wire protocol

✅ **Status: DONE (v0.5.0 development)**

- `visualiser.proto` declares `ObjectClass` enum (9 semantic classes + unspecified)
- Track field 26: `ObjectClass object_class` replaces string `class_label`
- Go gRPC server converts internal string → enum via `objectClassFromString()`
- Swift visualiser converts enum → string via `objectClassLabel()`
- Enables type-safe gRPC and forward compatibility pre-0.5.0

### Phase 3.1: VRLOG replay re-classification

✅ **Status: DONE**

VRLOG recordings made before classification store empty `ObjectClass`
strings. Without intervention, all replayed tracks show "Not classified".

- Refactored classifier: `ClassifyFeatures()` accepts pre-built features
  without requiring a full `TrackedObject` with observation history
- Added `classifyOrConvert()` bridge in gRPC server: when `ObjectClass`
  is empty, builds features from the Track's per-frame metrics and
  re-classifies on-the-fly using the same rule-based classifier
- Swift Track Inspector always displays Class field ("Not classified" fallback
  when enum is UNSPECIFIED)

## Remaining Phases

### Phase 4: Expose taxonomy via API

Add a `GET /api/v1/lidar/taxonomy` endpoint that returns the canonical label list with metadata (name, description, positive/negative, keyboard shortcut). This eliminates hardcoded lists in frontends.

### Phase 5: Remove duplicates from frontends

| Location                                       | Remove               | Replace with                                    |
| ---------------------------------------------- | -------------------- | ----------------------------------------------- |
| `api/lidar_labels.go` `validUserLabels`        | Hardcoded map        | Import from centralised taxonomy                |
| `web/src/lib/types/lidar.ts` `DetectionLabel`  | Hardcoded union type | Runtime import from taxonomy API                |
| `ContentView.swift` `classificationLabels`     | Hardcoded array      | Fetch from taxonomy API on launch               |
| `TrackList.svelte` `DETECTION_LABELS`          | Hardcoded array      | Fetch from taxonomy API                         |
| `MapPane.svelte` legend classes                | Hardcoded array      | Derive from `TRACK_COLORS` keys or taxonomy API |
| `adapters/ground_truth.go` `isPositiveLabel()` | Inline function      | Centralised `IsPositive()` method               |

### Phase 6: Public API field alignment

Public REST API responses (`/api/v1/lidar/tracks` etc) should continue using canonical
string labels. Coordinate with Phase 4 taxonomy API to ensure wire format matches.
Proto enum conversion is internal; public API uses human-readable strings.

## Risks

- **Proto enum migration** (Phase 3 complete): Complete — wire protocol now uses enum, internal code uses strings with converters at boundaries
- **Database migration** (000029 complete): Implemented idempotently with full reversibility note
- **Frontend cache**: Taxonomy API (Phase 4) should include versioning to bust stale label lists

## Implementation Notes

- Migration 000029 handles all legacy data conversions (ped→pedestrian, other→dynamic, impossible→noise)
- Proto enum values are immutable once committed (must maintain backward compatibility in future versions)
- String-based domain model is intentional: allows internal flexibility while wire protocol is type-safe
- Converters `objectClassFromString()` (Go) and `objectClassLabel()` (Swift) are the only points of string↔enum translation
- `ClassifyFeatures()` enables classification without a full `TrackedObject`, used by VRLOG replay and potentially other offline pipelines
- `classifyOrConvert()` is the single decision point: existing labels pass through, empty labels trigger re-classification
