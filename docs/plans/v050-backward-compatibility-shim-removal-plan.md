# v0.5.0 Backward Compatibility Shim Removal Plan

**Parent plan:** [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md) — Project E
**Related:** [LiDAR Visualiser Proto Contract Plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md) (speed summary fields)

## Status: In Progress

**Completed (PR #336 + proto contract work):**

- Visualiser model carries both `AvgSpeedMps` (running mean) and `P50SpeedMps` (p50)
- Proto field 24 stays `avg_speed_mps` (unchanged); p50/p85/p98 fields added (36/37/38)
- `classifyOrConvert()` uses `AvgSpeedMps` (running mean) for classification input
- Debug overlay serialisation in `frameBundleToProto` (gated by `IncludeDebug`)
- Cluster feature fields (`HeightP95`, `IntensityMean`, `SamplePoints`) serialised
- Positive serialisation tests replace negative debug tests
- Swift proto regenerated with `p50SpeedMps`, `p85SpeedMps`, `p98SpeedMps`

**Decision — avg_speed_mps retained alongside p50_speed_mps:**

Removing `avg_speed_mps` from the data model and algorithms was causing unintended
side effects in classification, ground truth evaluation, and pipeline export.
The running-average speed is a distinct metric from the p50 (median) speed: it
weights outlier speeds differently and some downstream consumers depend on this
behaviour. Both fields are now retained:

- `avg_speed_mps` — running mean, computed incrementally per observation
- `p50_speed_mps` — floor-based 50th percentile, computed from speed history

Deprecation of `avg_speed_mps` is deferred to future work and should be
investigated once all consumers have been audited for sensitivity to the
metric change.

**Remaining:** REST API cleanup (§3 download format), sweep legacy fields (§2),
and remaining shims (§4–17).

**Next steps (PR #336 review):**

1. **P50 summary endpoint** — `handleTrackSummary` currently returns `p50_speed_mps: 0`
   because a true median across tracks is not yet computed. Implementation deferred
   until the summary endpoint audit is complete.
2. **pcap-analyse avg_speed_mps** — INSERT now populates `avg_speed_mps` using p50
   as a fallback (pcap-analyse does not compute a running mean).
3. **speedWindow maxLen=0 guard** — Add() now returns early when maxLen ≤ 0 to
   prevent a panic if `max_speed_history_length` is runtime-tuned to zero.

## Goal

Audit and remove all backward compatibility shims, legacy field aliases, and
compat hacks across Go, Python, Svelte, and macOS before v0.5.0 ships. These
shims add maintenance burden and obscure the canonical data model. Removing them
now — as a single coordinated breaking change — is cheaper than maintaining
indefinite dual-format support.

**Principle:** rip the bandaid off. One version bump, one migration guide, clean
interfaces going forward.

## Scope

This plan covers **data model and API compat shims only**. It is a sub-plan of
the [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md),
which is the parent plan for all simplification work including deployment
deprecation (Project B), frontend consolidation (Project C), and CLI
simplification (Project D). This sub-plan corresponds to **Project E** in the
parent.

Intersections with other parent-plan projects:

- **Project B (deploy retirement):** Deploy executor compat methods (§5 below)
  are deferred to Project B and will be removed when the retirement gate is met.
  No action in this plan beyond documenting them.
- **Project C (#252 sweep migration):** Sweep legacy field removal (§2, §14
  below) removes the old request/result field names. Project C covers the full
  sweep UI migration to the Svelte frontend. The field-name cleanup here is a
  prerequisite for clean Project C work.
- **Proto contract plan:** The `avg_speed_mps` field coexistence (§1, §15
  below) is also tracked in the
  [proto contract plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
  Phase C/D. That plan owns the gRPC serialisation and Swift proto regeneration;
  this plan owns the REST API and internal model cleanup. Proto field 24
  stays `avg_speed_mps` (unchanged); p50/p85/p98 added as fields 36/37/38.
  `avg_speed_mps` is retained in the Go model, REST API, and database.

---

## Inventory of Backward Compatibility Shims

### 1. Go Server — `avg_speed_mps` / `p50_speed_mps` coexistence

| Item                  | Location                                                       | Status | Detail                                                                                  |
| --------------------- | -------------------------------------------------------------- | ------ | --------------------------------------------------------------------------------------- |
| Internal model field  | `internal/lidar/visualiser/model.go:245`                       | ✅     | Both `AvgSpeedMps` (running mean) and `P50SpeedMps` (p50) retained                      |
| Proto field unchanged | `proto/velocity_visualiser/v1/visualiser.proto:233`            | ✅     | Field 24 stays `avg_speed_mps`; p50 (36), p85 (37), p98 (38) added                      |
| Swift proto regen     | `tools/visualiser-macos/.../Generated/visualiser.pb.swift:787` | ✅     | Generated code uses `p50SpeedMps`, `p85SpeedMps`, `p98SpeedMps`                         |
| REST API JSON field   | `internal/lidar/monitor/track_api.go:124`                      | ✅     | Both `avg_speed_mps` and `p50_speed_mps` emitted in track responses                     |
| TrackFeatures field   | `internal/lidar/l6objects/features.go:31`                      | ✅     | `AvgSpeedMps` retained in ML feature-vector struct (classifier depends on running mean) |
| Track store (SQLite)  | `internal/lidar/storage/sqlite/track_store.go`                 | ✅     | Reads/writes `avg_speed_mps` column alongside `p50_speed_mps`                           |
| DB column             | `internal/db/schema.sql:90,190`                                | ✅     | `avg_speed_mps REAL` retained in both tables alongside `p50_speed_mps`                  |
| pcap-analyse tool     | `cmd/tools/pcap-analyse/main.go:107`                           | ✅     | Uses `AvgSpeedMps` from tracked objects                                                 |

**Decision:** Both `avg_speed_mps` (running mean) and `p50_speed_mps` (median)
are retained. They are semantically different metrics — the running mean weights
all observations equally while the median is robust to outlier speeds. Removing
`avg_speed_mps` caused unintended side effects in:

- Classification: the `ClassificationFeatures.AvgSpeed` input uses running mean
- Ground truth evaluation: velocity coverage checks use `AvgSpeedMps`
- Pipeline export: VRLOG track export uses `AvgSpeedMps` for `SpeedMps`
- Feature vectors: ML training data includes `avg_speed_mps` as a feature

**Future work:** Investigate deprecation of `avg_speed_mps` as a separate effort
once all consumers have been audited and migrated to `p50_speed_mps` where
appropriate. This requires:

1. Audit each consumer for sensitivity to the avg → p50 metric change
2. Run classification accuracy comparison with p50 vs running mean
3. Validate ground truth evaluation metrics are not degraded
4. Coordinate with any downstream data consumers

**Future work — speed percentile consistency:**

The p50/p85/p98 fields are computed and populated in several layers, but there
are gaps that should be addressed:

| Layer                             | p50                  | p85          | p98          | Notes                                                     |
| --------------------------------- | -------------------- | ------------ | ------------ | --------------------------------------------------------- |
| L5 tracking (`tracking.go`)       | Computed             | Not computed | Not computed | Only p50 via `p50OfSpeeds()`; p85/p98 remain zero         |
| Visualiser adapter (`adapter.go`) | Computed             | Computed     | Computed     | Recomputes all three from `SpeedHistory()` for gRPC       |
| SQLite storage (`track_store.go`) | Written              | Written      | Written      | Uses `l6objects.ComputeSpeedPercentiles` on insert/update |
| L6 objects (`classification.go`)  | Computed             | Computed     | Computed     | Feature vectors use p50/p85/p98                           |
| REST API individual track         | Exposed              | Not exposed  | Not exposed  | Only `p50_speed_mps` in JSON response                     |
| REST API summary endpoints        | Incorrect (uses avg) | Not exposed  | Not exposed  | `P50SpeedMps` is set to average speed, not actual p50     |
| gRPC proto stream                 | Exposed              | Exposed      | Exposed      | All three populated via adapter                           |

Action items:

1. **REST API**: Add `p85_speed_mps` to individual track response JSON
2. **REST API summary**: Compute actual p50 for summary endpoints instead of
   using average speed as a proxy
3. ~~**p98 vs p95 inconsistency**~~: **Resolved** — all layers now use p98
   (`ComputeSpeedPercentiles` threshold changed from 0.95 to 0.98, DB columns
   renamed via migration 000030)
4. **L5 tracking layer**: Consider computing p85/p98 at the tracking layer
   instead of only at the adapter layer, so all consumers get consistent values

---

### 2. Go Server — Sweep legacy request format

| Item                     | Location                                    | Detail                                                                                                                                                                                |
| ------------------------ | ------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Legacy multi-mode fields | `internal/lidar/sweep/runner.go:57-76`      | `NoiseValues`, `ClosenessValues`, `NeighbourValues`, single-variable range fields (`NoiseStart`/`End`/`Step`, etc.), fixed-value fields (`FixedNoise`, etc.) — all marked `// legacy` |
| Legacy result fields     | `internal/lidar/sweep/runner.go:104-107`    | `Noise`, `Closeness`, `Neighbour` in `ComboResult` — "populated from ParamValues for backward compat"                                                                                 |
| Legacy combination logic | `internal/lidar/sweep/sweep_params.go:9-66` | `computeCombinations()` handles old "multi", "noise", "closeness", "neighbour" modes                                                                                                  |

**Action:** Remove the legacy request fields and `computeCombinations()`. Callers
must use the `ParamValues` map format. Remove legacy `Noise`/`Closeness`/`Neighbour`
from `ComboResult`; consumers use `param_values` only. Update the web frontend
sweep dashboard to stop emitting or consuming legacy field names.

---

### 3. Go Server — Legacy download endpoint format

| Item             | Location                           | Detail                                                                                                                                 |
| ---------------- | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Dual URL support | `internal/api/server.go:1217-1280` | Supports both `/api/reports/123/download?file_type=pdf` (legacy query param) and `/api/reports/123/download/filename.pdf` (path-based) |

**Action:** Remove query-parameter format. Standardise on the path-based
`/api/reports/{id}/download/{filename}` endpoint. Web frontend already uses the
new format.

---

### 4. Go Server — Lenient JSON parsing in sweep handler

| Item                  | Location                                       | Detail                                                                                   |
| --------------------- | ---------------------------------------------- | ---------------------------------------------------------------------------------------- |
| Ignored decode errors | `internal/lidar/monitor/sweep_handlers.go:189` | `_ = json.NewDecoder(r.Body).Decode(&body) // ignore decode errors for backwards compat` |

**Action:** Return `400 Bad Request` on malformed JSON instead of silently
accepting. Callers must send valid JSON.

---

### 5. Go Server — Deploy executor backward-compat methods

| Item              | Location                              | Detail                                           |
| ----------------- | ------------------------------------- | ------------------------------------------------ |
| `buildSSHCommand` | `internal/deploy/executor.go:208-210` | "kept for backward compatibility with WriteFile" |
| `buildSCPArgs`    | `internal/deploy/executor.go:277-280` | "exists for backward compatibility in tests"     |

**Action:** These will be removed along with `cmd/deploy` when the retirement
gate is met (separate plan). No action in v0.5.0 beyond the existing deprecation
warning.

---

### 6. Go Server — Deprecated packet header struct

| Item                  | Location                                            | Detail                                                       |
| --------------------- | --------------------------------------------------- | ------------------------------------------------------------ |
| `PacketHeader` struct | `internal/lidar/l1packets/parse/extract.go:139-147` | All fields marked `UNUSED`, struct kept "for reference only" |

**Action:** Delete the struct. The comment explains the correct offset is 0;
anyone reading the code can refer to git history if needed.

---

### 7. Go Server — Removed method comment

| Item                     | Location                                       | Detail                                                                           |
| ------------------------ | ---------------------------------------------- | -------------------------------------------------------------------------------- |
| `AddPoints` removal note | `internal/lidar/l2frames/frame_builder.go:354` | `// NOTE: Legacy AddPoints removed in polar-first refactor. Use AddPointsPolar.` |

**Action:** Delete the comment. The method no longer exists; the comment adds no
value.

---

### 8. Go Server — Type aliases in `lidar/aliases.go`

| Item                | Location                         | Detail                                                                                                        |
| ------------------- | -------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| Cross-layer aliases | `internal/lidar/aliases.go:1-89` | Re-exports types from l2frames, l3grid, l4perception, l5tracks, l6objects for integration testing convenience |

**Action:** Evaluate whether these are still needed. If integration tests can
import from the correct sub-package directly, remove the aliases. If the aliases
serve a legitimate API-surface purpose (public package boundary), keep them but
document the intent.

---

### 9. Python — Legacy API response format handling

| Item                | Location                                                             | Detail                                                                               |
| ------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------ |
| Dual format parsing | `tools/pdf-generator/pdf_generator/core/api_client.py:111-116`       | Handles both `dict` (new) and plain `list` (legacy) response from `/api/radar/stats` |
| Test coverage       | `tools/pdf-generator/pdf_generator/tests/test_api_client.py:215-238` | `test_get_stats_legacy_format()` explicitly tests the old format                     |

**Action:** Remove the `isinstance(payload, list)` branch. The server has
returned the dict format since v0.3.x. Delete `test_get_stats_legacy_format`.

---

### 10. Python — Config dict-conversion backward compatibility

| Item                    | Location                                                           | Detail                                                                    |
| ----------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------- |
| `geometry` property     | `tools/pdf-generator/pdf_generator/core/config_manager.py:214-222` | Returns dict for "backward compatibility" with code expecting dict access |
| Dict conversion helpers | `tools/pdf-generator/pdf_generator/core/config_manager.py:531-572` | `_colors_to_dict`, `_fonts_to_dict`, `_layout_to_dict`, `_pdf_to_dict`    |

**Action:** Audit callers. If all callers now use the dataclass properties
directly, remove the dict-conversion helpers and the `geometry` property. If
callers remain, migrate them to direct attribute access first.

---

### 11. Python — PyLaTeX fallback stubs

| Item         | Location                                                           | Detail                                                              |
| ------------ | ------------------------------------------------------------------ | ------------------------------------------------------------------- |
| Stub classes | `tools/pdf-generator/pdf_generator/core/document_builder.py:11-46` | `class Document`, `class Package`, etc. when `HAVE_PYLATEX = False` |

**Action:** Make `pylatex` a hard dependency. Remove the fallback stubs. The PDF
generator is non-functional without pylatex — the stubs just defer the error.

---

### 12. Svelte/Web — Legacy `BackgroundCell` fields

| Item                   | Location                             | Detail                                                                                                                      |
| ---------------------- | ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| Legacy optional fields | `web/src/lib/types/lidar.ts:127-130` | `ring?`, `azimuth_deg?`, `average_range_meters?` — explicitly marked "Legacy fields (optional, for backward compatibility)" |

**Action:** Remove these fields from the TypeScript type. The server stopped
sending them; the `?` optionality was the compat shim.

---

### 13. Svelte/Web — API response envelope migration

| Item                       | Location                          | Detail                                                                                                              |
| -------------------------- | --------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Dual-format cache handling | `web/src/routes/+page.svelte:309` | `arr = Array.isArray(cached) ? cached : cached.metrics \|\| []` — handles both old (array) and new (object) formats |
| Stats API response         | `web/src/lib/api.ts:91-105`       | Expects `{ metrics: [...], histogram: {...} }` but code still guards against bare arrays                            |

**Action:** Remove `Array.isArray(cached)` branch. Invalidate any client-side
caches that might contain the old format (bump a cache version key or clear
localStorage on upgrade).

---

### 14. Svelte/Web — Sweep results legacy field names

| Item                     | Location                                                  | Detail                                                                                                    |
| ------------------------ | --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Legacy param keys        | `web/src/lib/__tests__/sweep_dashboard.test.ts:432`       | Test: "falls back to legacy format without param_values" using `noise`, `closeness`, `neighbour` directly |
| Legacy results rendering | `web/src/lib/__tests__/sweep_dashboard.test.ts:1617-1633` | Test: "handles legacy results without param_values"                                                       |
| Legacy CSV download      | `web/src/lib/__tests__/sweep_dashboard.test.ts:2479-2511` | Test: "downloads CSV with legacy param keys"                                                              |

**Action:** Remove the legacy fallback code paths and their tests. The sweep
dashboard should only accept the `param_values` map format. Aligns with Go
server-side removal (item 2).

---

### 15. macOS Visualiser — `p50SpeedMps` field rename

| Item              | Location                                                       | Status | Detail                                                                 |
| ----------------- | -------------------------------------------------------------- | ------ | ---------------------------------------------------------------------- |
| Model field       | `tools/visualiser-macos/.../Models/Models.swift:230`           | ✅     | `var p50SpeedMps: Float = 0  // p50 (was avgSpeedMps before v0.5.0)`   |
| Proto regenerated | `tools/visualiser-macos/.../Generated/visualiser.pb.swift:787` | ✅     | Generated code uses `p50SpeedMps` (field 36); `avgSpeedMps` retained   |
| Client mapping    | `tools/visualiser-macos/.../gRPC/VisualiserClient.swift:594`   | ✅     | `p50SpeedMps: t.p50SpeedMps` — correctly mapped from proto             |
| Inspector rows    | `tools/visualiser-macos/.../UI/ContentView.swift`              |        | p50/p85/p98 inspector rows not yet added (server populates the fields) |

**Action:** Re-add p50/p85/p98 inspector `DetailRow` entries in the Velocity GroupBox
of `ContentView.swift`. The server now populates the fields end-to-end
(adapter → `frameBundleToProto` → proto → Swift client).

---

### 16. macOS Visualiser — Legacy point buffer

| Item               | Location                                                      | Detail                                                                             |
| ------------------ | ------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Old rendering path | `tools/visualiser-macos/.../Rendering/MetalRenderer.swift:66` | `var pointBuffer: MTLBuffer? // Legacy point buffer (for backwards compatibility)` |

**Action:** Remove if the composite renderer fully replaces the legacy rendering
path. Verify no code paths still populate or read from `pointBuffer`.

---

### 17. macOS Visualiser — Legacy playback defaults

| Item                      | Location                                            | Detail                                                                                                                          |
| ------------------------- | --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Legacy field preservation | `tools/visualiser-macos/.../App/AppState.swift:345` | "Preserve legacy defaults for callers/tests that still inspect these fields directly" in `.unknown` case of `setPlaybackMode()` |

**Action:** Update callers/tests to use the structured playback mode enum instead
of inspecting `isLive`/`isSeekable` directly. Then remove the legacy branch.

---

## Items Explicitly NOT Removed

The following are **not** compat shims and should be retained:

| Item                                                                                     | Reason to keep                                            |
| ---------------------------------------------------------------------------------------- | --------------------------------------------------------- |
| Type aliases in `lidar/l3grid/types.go`, `l6objects/types.go`, `storage/sqlite/types.go` | Avoid import cycles — architectural necessity, not compat |
| `ClassPed = ClassPedestrian` short alias                                                 | Convenience alias, not legacy                             |
| gRPC `UnimplementedServer` embedding                                                     | Required by protobuf-go for forward compat                |
| gRPC stream type aliases (generated)                                                     | Auto-generated by protoc, not hand-maintained             |
| `FrameType_FRAME_TYPE_FULL` enum value                                                   | Valid operational mode, not deprecated                    |
| SVG-to-PDF converter fallback chain                                                      | Graceful degradation for different environments           |
| Font fallback logic in PDF generator                                                     | Operational resilience, not compat                        |
| DB legacy detection in `db.go:296-319`                                                   | Needed for upgrades from pre-migration databases          |
| Old migration files (000002-000019)                                                      | Immutable history; never modify applied migrations        |

---

## Migration Guide (for downstream consumers)

### REST API

| Old                                                          | New                                         | Notes                                                |
| ------------------------------------------------------------ | ------------------------------------------- | ---------------------------------------------------- |
| `avg_speed_mps` only in track responses                      | Both `avg_speed_mps` and `p50_speed_mps`    | avg is running mean, p50 is median; both now emitted |
| `/api/reports/{id}/download?file_type=pdf`                   | `/api/reports/{id}/download/{filename}.pdf` | Query-param format removed                           |
| Sweep results with `noise`/`closeness`/`neighbour` top-level | `param_values` map                          | All params keyed under `param_values`                |
| Stats response as bare `[...]` array                         | `{ "metrics": [...], "histogram": {...} }`  | Old format removed                                   |

### Database schema

| Old                  | New                               | Notes                                             |
| -------------------- | --------------------------------- | ------------------------------------------------- |
| `avg_speed_mps` only | `avg_speed_mps` + `p50_speed_mps` | Both retained; avg is running mean, p50 is median |

### Protobuf (gRPC visualiser stream)

| Old                       | New                                                              | Notes                       |
| ------------------------- | ---------------------------------------------------------------- | --------------------------- |
| Field 24: `avg_speed_mps` | Field 24: `avg_speed_mps` (unchanged)                            | Not renamed; retained as-is |
| No field 36/37/38         | `p50_speed_mps` (36), `p85_speed_mps` (37), `p98_speed_mps` (38) | New additions               |

### Sweep request JSON

| Old                                                    | New                            | Notes                          |
| ------------------------------------------------------ | ------------------------------ | ------------------------------ |
| `noise_values`, `closeness_values`, `neighbour_values` | `param_values` map             | Legacy multi-mode removed      |
| `noise_start`/`end`/`step` etc.                        | `param_values` + grid spec     | Single-variable ranges removed |
| `fixed_noise`, `fixed_closeness`, `fixed_neighbour`    | Not needed with `param_values` | Removed                        |

### Web frontend types

| Old                                   | New     | Notes                                      |
| ------------------------------------- | ------- | ------------------------------------------ |
| `BackgroundCell.ring`                 | Removed | Server no longer sends; field was optional |
| `BackgroundCell.azimuth_deg`          | Removed | Same                                       |
| `BackgroundCell.average_range_meters` | Removed | Same                                       |

---

## Delivery Plan

### Phase 1 — Audit and plan (this document)

- [x] Inventory all compat shims across Go, Python, Svelte, macOS
- [x] Classify as "remove in v0.5.0" vs "retain"
- [x] Review with maintainer

### Phase 2 — Server-side removals (Go)

- [x] Retain `AvgSpeedMps` alongside `P50SpeedMps` in visualiser `model.go`
- [x] Retain `avg_speed_mps` and add `p50_speed_mps` in `track_api.go` REST responses
- [x] Retain `AvgSpeedMps` in `l6objects/features.go` (TrackFeatures struct + CSV export)
- [x] Retain `avg_speed_mps` reads/writes in `storage/sqlite/track_store.go` and `analysis_run.go`
- [x] Retain `avg_speed_mps` column in DB schema (no drop migration)
- [x] Remove sweep legacy request fields and `computeCombinations()`
- [x] Remove legacy sweep result fields from `ComboResult`
- [x] Remove download endpoint query-param path
- [x] Return 400 on malformed sweep JSON instead of swallowing errors
- [x] Delete `PacketHeader` struct and `AddPoints` removal comment
- [x] Evaluate and remove `lidar/aliases.go` if unused — **retained**: actively used by 9+ files (cmd/radar, integration tests, network listeners, visualiser)

### Phase 3 — Frontend removals (Svelte)

- [ ] Remove `BackgroundCell` legacy fields from `lidar.ts`
- [ ] Remove `Array.isArray(cached)` dual-format branch
- [ ] Remove sweep legacy field fallback code and tests
- [ ] Bump cache version to invalidate stale client-side data

### Phase 4 — Python removals

- [ ] Remove legacy stats format branch in `api_client.py`
- [ ] Remove `test_get_stats_legacy_format` test
- [ ] Audit and remove config dict-conversion helpers
- [ ] Make pylatex a hard dependency; remove fallback stubs

### Phase 5 — macOS removals (Swift)

- [x] Regenerate Swift protobuf from updated `.proto`
- [x] Verify `p50SpeedMps` field reads correctly from regenerated proto
- [ ] Re-add p50/p85/p98 inspector rows in `ContentView.swift`
- [ ] Remove legacy `pointBuffer` if composite renderer is complete
- [ ] Update callers of `setPlaybackMode(.unknown)` legacy branch

### Phase 6 — Validation

- [ ] `make format && make lint && make test` passes
- [ ] `make build-radar-local` succeeds
- [ ] `make build-web` succeeds
- [ ] macOS visualiser builds and connects to gRPC stream
- [ ] Sweep dashboard works with `param_values` format only
- [ ] PDF generator fetches stats in dict format only

---

## Decision Notes

- This plan is intentionally aggressive: all shims removed in one release.
  Maintaining dual formats across a minor release boundary would require test
  matrices and documentation for both formats, which costs more than a clean break.
- Proto field 24 remains `avg_speed_mps` (unchanged). The new percentile fields
  are p50 (36), p85 (37), p98 (38). Old binaries will ignore the new field numbers.
  This is acceptable since no pre-v0.5.0 consumers exist in production.
- Items gated on external dependencies (deploy retirement, frontend consolidation)
  are excluded from this plan and tracked in the parent
  [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md)
  Projects B and C respectively.
- The breaking changes in this sub-plan are summarised in the parent plan's
  v0.5.0 Breaking Changes section (items 1, 4-7). That section is the
  consumer-facing changelog; this document is the implementation detail.
