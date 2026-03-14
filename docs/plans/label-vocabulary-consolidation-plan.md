# Label Vocabulary Consolidation Plan

**Status:** Phase 1–3.2 completed · Phase 4–5 planned
**Layers:** Cross-cutting (L6 Objects, API, Web, macOS Visualiser)
**Related:** [Classification Maths](../maths/classification-maths.md), `proto/velocity_visualiser/v1/visualiser.proto`

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
- **CYCLIST** (4): pedal cyclists and motorcyclists (user-assignable)
- **BIRD** (5): airborne fauna (user-assignable)
- **BUS** (6): public transit (user-assignable)
- **CAR** (7): private automobiles, vans, and trucks (user-assignable)
- **TRUCK** (8): reserved for v0.6+ (proto value stable, not user-assignable)
- **MOTORCYCLIST** (9): reserved for v0.6+ (proto value stable, not user-assignable)

v0.5.0 ships **7 user-assignable classes** (noise, dynamic, pedestrian, cyclist, bird, bus, car).
Truck and motorcyclist are reserved in the proto enum for future use but disabled
in the classifier, hidden in all UIs, and rejected by the label validation API.

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

✅ **Status: DONE (subsequently trimmed in Phase 3.2)**

Classifier v1.2 added `ClassTruck` and `ClassMotorcyclist` with thresholds and cascade rules. All frontends displayed 9 classification options.

> Phase 3.2 subsequently disabled these two classes from the classifier
> cascade and UI, retaining the proto enum values for future reactivation.

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

### Phase 3.2: Trim to 7 user-assignable classes for v0.5.0

✅ **Status: DONE**

Disabled truck and motorcyclist from the active classifier, all UIs, and the
label validation API for v0.5.0. Proto enum values 8 (TRUCK) and 9
(MOTORCYCLIST) are retained for forward compatibility.

- Classifier: truck/motorcyclist cascade rules commented out (trucks→CAR, motorcycles→CYCLIST)
- Label API: `validUserLabels` reduced to 7 entries; truck/motorcyclist rejected with 400
- macOS: removed from `classificationLabels` array; help text updated
- Web: removed from `DetectionLabel` type and dropdowns; colours retained for backward compat
- Proto: enum values stable; reactivation path documented (uncomment rules + restore UI entries)

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

### Phase 3.2: Trim to 7 classes for v0.5.0

✅ **Status: DONE**

Truck and motorcyclist classifications were added in Phase 2 but lacked
sufficient labelled training data to distinguish reliably. For v0.5.0,
they are **disabled** across the stack:

- **Proto**: Enum values 8 (TRUCK) and 9 (MOTORCYCLIST) retained with
  "reserved (v0.6+)" comments for wire-format stability.
- **Classifier**: Truck and motorcyclist rules commented out in the
  cascade. Trucks fall through to CAR; motorcyclists fall through to
  CYCLIST.
- **Label API**: `validUserLabels` reduced to 7 entries. Attempting to
  assign `"truck"` or `"motorcyclist"` via the API returns 400.
- **macOS app**: Removed from `classificationLabels` array; car help
  text updated to include trucks, cyclist updated to include motorcycles.
- **Web app**: Removed from `DetectionLabel` type, `DETECTION_LABELS`
  dropdown, and `object_class` union. `TRACK_COLORS` entries retained
  for backward-compatible rendering of existing labelled data.
- **Keyboard shortcuts**: Renumbered 1–7 (car, bus, pedestrian,
  cyclist, bird, dynamic, noise).

**Reactivation path (v0.6+):** When sufficient labelled data exists to
train a reliable truck/motorcyclist classifier, uncomment the cascade
rules in `classification.go`, add the labels back to `validUserLabels`,
and restore the UI entries. No proto or database migration is needed
— the enum values are already allocated.
