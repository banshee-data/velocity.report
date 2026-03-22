# Label Vocabulary

Active plan: [label-vocabulary-consolidation-plan.md](../../plans/label-vocabulary-consolidation-plan.md)

**Status:** Phases 1–3.2 complete; phases 3.5–6 planned.

## Canonical Vocabulary (proto3 enum authoritative)

| Value | Name         | User-Assignable | Notes                        |
| ----- | ------------ | --------------- | ---------------------------- |
| 0     | UNSPECIFIED  | —               | Default/unknown              |
| 1     | NOISE        | ✅              | Environmental noise          |
| 2     | DYNAMIC      | ✅              | Classifier fallback          |
| 3     | PEDESTRIAN   | ✅              | Foot traffic                 |
| 4     | CYCLIST      | ✅              | Pedal cyclists + motorcycles |
| 5     | BIRD         | ✅              | Airborne fauna               |
| 6     | BUS          | ✅              | Public transit               |
| 7     | CAR          | ✅              | Cars, vans, trucks           |
| 8     | TRUCK        | Reserved v0.6+  | Proto value stable           |
| 9     | MOTORCYCLIST | Reserved v0.6+  | Proto value stable           |

v0.5.0 ships **7 user-assignable classes**. Truck and motorcyclist are
disabled in the classifier, hidden in UIs, and rejected by the label API.

## Wire Protocol

- `visualiser.proto`: `ObjectClass` enum (field 26 on Track)
- Go → proto: `objectClassFromString()` in `grpc_server.go`
- Proto → Swift: `objectClassLabel()` in `VisualiserClient.swift`
- Internal domain model uses string labels; proto enums at boundaries only.

## Migration 000029

Converts legacy rows: `ped` → `pedestrian`, `other` → `dynamic`,
`impossible` → `noise`. Idempotent.

## VRLOG Replay Re-Classification (Phase 3.1)

Older recordings store empty `ObjectClass`. The gRPC server
`classifyOrConvert()` bridge re-classifies on-the-fly using
`ClassifyFeatures()` — a refactored classifier that accepts pre-built
features without a full `TrackedObject`.

## Keyboard Shortcuts

Renumbered 1–7 for v0.5.0: car, bus, pedestrian, cyclist, bird, dynamic,
noise.

## Remaining Work

### Phase 3.5 — Display vs Selectable Split (#381)

Split into `DisplayLabel` (9 classes — rendering, colour, inspector) and
`SelectableLabel` (7 classes — labelling UI, shortcuts, API validation).
Truck/motorcyclist visible when present in data but not user-selectable.

### Phase 4 — Taxonomy API

`GET /api/v1/lidar/taxonomy` returns canonical label list with metadata
(name, description, positive/negative, shortcut). Eliminates hardcoded
lists in frontends.

### Phase 5 — Frontend Deduplication

Replace hardcoded label arrays in Go, TypeScript, Swift, and Svelte with
runtime imports from the taxonomy API.

### Phase 6 — Public API Field Alignment

Ensure REST API track responses use canonical string labels consistent
with the taxonomy API.

## Reactivation Path (v0.6+)

When sufficient labelled data exists: uncomment truck/motorcyclist cascade
rules in `classification.go`, add labels back to `validUserLabels`, restore
UI entries. No proto or database migration needed — enum values already
allocated.
