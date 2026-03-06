# v0.5.0 Backward Compatibility Shim Removal Plan

## Status: Draft

## Goal

Audit and remove all backward compatibility shims, legacy field aliases, and
compat hacks across Go, Python, Svelte, and macOS before v0.5.0 ships. These
shims add maintenance burden and obscure the canonical data model. Removing them
now — as a single coordinated breaking change — is cheaper than maintaining
indefinite dual-format support.

**Principle:** rip the bandaid off. One version bump, one migration guide, clean
interfaces going forward.

## Scope

This plan covers **data model and API compat shims only** — not the deployment
deprecation surface (covered in
[platform-simplification-and-deprecation-plan.md](platform-simplification-and-deprecation-plan.md)).

---

## Inventory of Backward Compatibility Shims

### 1. Go Server — `AvgSpeedMps` in visualiser model and REST API

| Item                 | Location                                            | Detail                                                                                                                              |
| -------------------- | --------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| Internal model field | `internal/lidar/visualiser/model.go:245`            | `AvgSpeedMps float32 // running mean for classifier/VRLOG compat` — exists alongside `MedianSpeedMps`, `P85SpeedMps`, `P98SpeedMps` |
| REST API JSON field  | `internal/lidar/monitor/track_api.go:124`           | `AvgSpeedMps float32 json:"avg_speed_mps"` — exposed to web frontend                                                                |
| pcap-analyse tool    | `cmd/tools/pcap-analyse/main.go:107`                | References `avg_speed_mps` in analysis output                                                                                       |
| Proto field rename   | `proto/velocity_visualiser/v1/visualiser.proto:233` | Field 24 already renamed to `median_speed_mps` in proto, but Go model still carries `AvgSpeedMps` for "VRLOG compat"                |

**Action:** Remove `AvgSpeedMps` from the internal model, REST API, and
pcap-analyse. Consumers should use `MedianSpeedMps` (p50), `P85SpeedMps`, or
`P98SpeedMps` as appropriate. Update VRLOG writer to emit `median_speed_mps`.

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

### 15. macOS Visualiser — `medianSpeedMps` field rename incomplete

| Item                          | Location                                                       | Detail                                                                  |
| ----------------------------- | -------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Model comment                 | `tools/visualiser-macos/.../Models/Models.swift:230`           | `var medianSpeedMps: Float = 0  // p50 (was avgSpeedMps before v0.5.0)` |
| Proto still uses old name     | `tools/visualiser-macos/.../Generated/visualiser.pb.swift:786` | Generated code still has `avgSpeedMps` (field 24)                       |
| Client reads mismatched field | `tools/visualiser-macos/.../gRPC/VisualiserClient.swift:594`   | References `t.medianSpeedMps` but proto has `avgSpeedMps`               |

**Action:** Regenerate Swift protobuf from the updated `.proto` that already uses
`median_speed_mps`. The Swift model already expects `medianSpeedMps` — the
generated code just needs to catch up.

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

| Old                                                          | New                                         | Notes                                      |
| ------------------------------------------------------------ | ------------------------------------------- | ------------------------------------------ |
| `avg_speed_mps` in track responses                           | `median_speed_mps`                          | Semantic change: was running mean, now p50 |
| `/api/reports/{id}/download?file_type=pdf`                   | `/api/reports/{id}/download/{filename}.pdf` | Query-param format removed                 |
| Sweep results with `noise`/`closeness`/`neighbour` top-level | `param_values` map                          | All params keyed under `param_values`      |
| Stats response as bare `[...]` array                         | `{ "metrics": [...], "histogram": {...} }`  | Old format removed                         |

### Protobuf (gRPC visualiser stream)

| Old                       | New                                        | Notes                               |
| ------------------------- | ------------------------------------------ | ----------------------------------- |
| Field 24: `avg_speed_mps` | Field 24: `median_speed_mps`               | Wire-compatible (same field number) |
| No field 36/37            | `p85_speed_mps` (36), `p98_speed_mps` (37) | New additions                       |

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
- [ ] Review with maintainer

### Phase 2 — Server-side removals (Go)

- [ ] Remove `AvgSpeedMps` from `model.go`, `track_api.go`, `pcap-analyse`
- [ ] Remove sweep legacy request fields and `computeCombinations()`
- [ ] Remove legacy sweep result fields from `ComboResult`
- [ ] Remove download endpoint query-param path
- [ ] Return 400 on malformed sweep JSON instead of swallowing errors
- [ ] Delete `PacketHeader` struct and `AddPoints` removal comment
- [ ] Evaluate and remove `lidar/aliases.go` if unused

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

- [ ] Regenerate Swift protobuf from updated `.proto`
- [ ] Remove legacy `pointBuffer` if composite renderer is complete
- [ ] Update callers of `setPlaybackMode(.unknown)` legacy branch
- [ ] Verify `medianSpeedMps` field reads correctly from regenerated proto

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
- The proto field 24 rename is wire-compatible (same field number, same type).
  Old binaries will still decode the field — they will just interpret it as
  "avg" when it is actually "median". This is acceptable since no pre-v0.5.0
  consumers exist in production.
- Items gated on external dependencies (deploy retirement, frontend consolidation)
  are excluded and tracked in the
  [platform simplification plan](platform-simplification-and-deprecation-plan.md).
