# v0.5.0 Backward Compatibility Shim Removal Plan

- **Parent plan:** [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md) — Project E
- **Layers:** Cross-cutting (API, protobuf, database)
- **Related:** [LiDAR Visualiser Proto Contract Plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md) (speed summary fields), [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md)

- **Status:** In Progress

- **Update:** Status review for v0.5.0 release readiness. The
  speed contract reset (§1, §15) is complete — `peak_speed_mps` → `max_speed_mps`
  rename landed in #352 (proto, Go, Swift, TS); SQL column rename deferred to
  migration 000030. Aggregate percentile labels remain reserved for grouped/report
  metrics only. Remaining work is concentrated in server-side sweep/download
  cleanup, Python/web/macOS consumer migration, and the Phase 6 validation gate.

## Tracking Snapshot

| Outcome             | Sections                | Notes                                                                                                                                           |
| ------------------- | ----------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Removed in code     | §1, §4, §7, §15         | Speed contract reset landed (#352); malformed sweep JSON returns `400`; stale `AddPoints` note gone; proto peak→max rename complete             |
| Pending             | §2, §3, §6, §9–§14, §17 | Server-side sweep legacy fields, report-download follow-through, `PacketHeader`, Python/web/macOS consumer migrations still need implementation |
| Deferred / retained | §5, §8, §16             | Either owned by another plan or still an active implementation path rather than a removable shim today                                          |

## Shim Work Already Removed

| Shim                                     | Section | PR   | Notes                                                                                              |
| ---------------------------------------- | ------- | ---- | -------------------------------------------------------------------------------------------------- |
| Proto `peak_speed_mps` → `max_speed_mps` | §1, §15 | #352 | Proto field 25, Go/Swift/TS model renamed; SQL column deferred to migration 000030                 |
| Lenient sweep JSON parsing removed       | §4      |      | Empty body and malformed JSON now both return `400 Bad Request`; the previous lenient path is gone |
| Stale `AddPoints` removal note deleted   | §7      |      | `frame_builder.go` no longer carries the compat comment                                            |
| Type aliases evaluated and retained      | §8      |      | Documented as an active package-boundary choice, not a removable shim                              |

**Remaining: 10 pending sections (§2, §3, §6, §9–§14, §17) plus the Phase 6 validation gate:** finish the server-side sweep cleanup (§2), finish the report-download migration end-to-end (§3), decide/delete `PacketHeader` (§6), remove the remaining Python (§9–§11), web (§12–§14), and macOS (§17) fallback code, then run the Phase 6 validation gate.

## Goal

Audit and remove all backward compatibility shims, legacy field aliases, and
compat hacks across Go, Python, Svelte, and macOS before v0.5.0 ships. These
shims add maintenance burden and obscure the canonical data model. Removing them
now — as a single coordinated breaking change — is cheaper than maintaining
indefinite dual-format support.

**Principle:** rip the bandaid off. One version bump, one migration guide, clean
interfaces going forward.

Decision recorded in [DECISIONS.md](../DECISIONS.md): `v0.5.0` ships one
coordinated breaking-change sweep. No temporary dual-format shims are retained
after the cut except DB upgrade detection and architecturally necessary aliases
listed in "Items Explicitly NOT Removed" below.

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
- **Proto contract plan:** The track speed contract reset (§1, §15 below) is
  also tracked in the
  [proto contract plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
  Phase C/D. That plan owns gRPC/proto surface changes and Swift regeneration;
  this plan owns the REST API and internal model cleanup. Proto field 24 stays
  `avg_speed_mps` (unchanged). The branch-local single-track speed-label fields are
  not approved to ship as stable public track metrics and should be backed out
  or quarantined as part of the reset.

---

## Inventory of Backward Compatibility Shims

### 1. Go Server — track speed contract reset

| Item                                     | Location                                                                               | Status       | Detail                                                                                                                          |
| ---------------------------------------- | -------------------------------------------------------------------------------------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| Stable public track field                | `internal/lidar/monitor/track_api.go`, `proto/velocity_visualiser/v1/visualiser.proto` | Active       | `avg_speed_mps` remains the stable running-mean field for now                                                                   |
| Stable public raw-max field              | `internal/lidar/monitor/track_api.go`, `proto/velocity_visualiser/v1/visualiser.proto` | ✅ Renamed   | `peak_speed_mps` renamed to `max_speed_mps` on proto (field 25), Go, Swift, TS in #352; SQL column deferred to migration 000030 |
| Branch-local percentile additions        | proto, REST, visualiser model/UI                                                       | ✅ Resolved  | Single-track aggregate-percentile label expansion was not merged; percentiles remain aggregate-only                             |
| Existing percentile columns/calculations | `lidar_tracks`, analysis runs, classifier features                                     | Transitional | SQL columns remain until migration 000030; no new public dependency should be added                                             |

**Decision:** Keep `avg_speed_mps` and the raw maximum as the only stable public
track speed fields for now. Reserve aggregate percentile labels for grouped/report aggregates
only. Rename the raw maximum from `peak_speed_mps` to `max_speed_mps` in
unshipped contracts, and reserve the word `peak` for a future filtered measure.
Track-level speed summaries will be redesigned separately with distinct
non-percentile names and formulas.

**Action items:**

1. ~~Back out unmerged single-track aggregate-percentile proto/REST/UI work before merge~~ — ✅ resolved; fields not merged
2. ~~Rename public/raw `peak_speed_mps` references to `max_speed_mps` where the
   contract is still unshipped~~ — ✅ complete (#352); SQL column deferred to migration 000030
3. Define replacement public track metrics in the speed percentile alignment plan
4. Migrate any temporary internal percentile dependencies to the new track metrics
5. Keep aggregate percentile work isolated to grouped/report surfaces

---

### 2. Go Server — Sweep legacy request format

| Item                             | Location                               | Status  | Detail                                                                                                               |
| -------------------------------- | -------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------------------- |
| Legacy multi-mode request fields | `internal/lidar/sweep/runner.go`       | Pending | `SweepRequest` still exposes `Mode`, `noise_values`, per-variable range fields, and fixed-value compatibility fields |
| Legacy result fields             | `internal/lidar/sweep/runner.go`       | Pending | `ComboResult` still exposes top-level `Noise` / `Closeness` / `Neighbour` aliases alongside `param_values`           |
| Legacy combination helper        | `internal/lidar/sweep/sweep_params.go` | Pending | `computeCombinations()` and the mode-specific expansion path are still present on `main`                             |

**Action:** Server-side request/result compat cleanup is still pending. Remove
the legacy request/result fields, remove `computeCombinations()` and the legacy
mode-specific expansion path from `sweep_params.go` (only deleting or splitting
the file once the non-legacy helpers have been moved), and then finish the
dashboard/test fallback cleanup in §14.

---

### 3. Go Server + Web — Legacy download endpoint format

| Item                                    | Location                                                         | Status  | Detail                                                                                                 |
| --------------------------------------- | ---------------------------------------------------------------- | ------- | ------------------------------------------------------------------------------------------------------ |
| Path-based route enforcement            | `internal/api/server.go`                                         | Removed | `/api/reports/{id}/download/{filename}` is now the only accepted route; missing filenames are rejected |
| Legacy query-param callers              | `web/src/lib/api.ts`                                             | Pending | The web helper still requests `/download?file_type=...` instead of the filename-based route            |
| Legacy `file_type` wording and coverage | `internal/api/server.go`, `internal/api/server_coverage_test.go` | Pending | Helper argument names and error-path coverage still talk about the removed `file_type` parameter       |

**Action:** Finish the migration end-to-end: move callers to the path-based
download URL, then rename the remaining `file_type` terminology inside the
server/tests so the implementation no longer carries the removed parameter name.

---

### 4. Go Server — Lenient JSON parsing in sweep handler

| Item                  | Location                                   | Status  | Detail                                                                                     |
| --------------------- | ------------------------------------------ | ------- | ------------------------------------------------------------------------------------------ |
| Ignored decode errors | `internal/lidar/monitor/sweep_handlers.go` | Removed | The handler now allows empty request bodies only; malformed JSON returns `400 Bad Request` |

**Action:** No further shim removal needed here; keep coverage that malformed
JSON is rejected.

---

### 5. Go Server — Deploy executor backward-compat methods

| Item              | Location                      | Status   | Detail                                                                                        |
| ----------------- | ----------------------------- | -------- | --------------------------------------------------------------------------------------------- |
| `buildSSHCommand` | `internal/deploy/executor.go` | Deferred | Still kept for backward compatibility with `WriteFile`; removal is owned by deploy retirement |
| `buildSCPArgs`    | `internal/deploy/executor.go` | Deferred | Still present for tests until deploy tooling is retired                                       |

**Action:** These will be removed along with `cmd/deploy` when the retirement
gate is met (separate plan). No action in v0.5.0 beyond the existing deprecation
warning.

---

### 6. Go Server — Deprecated packet header struct

| Item                  | Location                                    | Status  | Detail                                                      |
| --------------------- | ------------------------------------------- | ------- | ----------------------------------------------------------- |
| `PacketHeader` struct | `internal/lidar/l1packets/parse/extract.go` | Pending | The deprecated reference-only struct still exists on `main` |

**Action:** Either delete it before `v0.5.0` or explicitly defer/retain it;
until then this plan should treat the removal as pending.

---

### 7. Go Server — Removed method comment

| Item                     | Location                                   | Status  | Detail                                                   |
| ------------------------ | ------------------------------------------ | ------- | -------------------------------------------------------- |
| `AddPoints` removal note | `internal/lidar/l2frames/frame_builder.go` | Removed | The stale compat comment has been deleted on this branch |

**Action:** None.

---

### 8. Go Server — Type aliases in `lidar/aliases.go`

| Item                | Location                    | Status   | Detail                                                                                                                   |
| ------------------- | --------------------------- | -------- | ------------------------------------------------------------------------------------------------------------------------ |
| Cross-layer aliases | `internal/lidar/aliases.go` | Retained | Still actively used by integration tests and adapter-style callers; this is a documented package-boundary choice for now |

**Action:** Evaluate whether these are still needed. If integration tests can
import from the correct sub-package directly, remove the aliases. If the aliases
serve a legitimate API-surface purpose (public package boundary), keep them but
document the intent.

---

### 9. Python — Legacy API response format handling

| Item                        | Location                                                     | Status  | Detail                                                                            |
| --------------------------- | ------------------------------------------------------------ | ------- | --------------------------------------------------------------------------------- |
| Dual-format parsing         | `tools/pdf-generator/pdf_generator/core/api_client.py`       | Pending | The client still accepts both dict and bare-list payloads from `/api/radar/stats` |
| Legacy-format test coverage | `tools/pdf-generator/pdf_generator/tests/test_api_client.py` | Pending | `test_get_stats_legacy_format()` still preserves the removed response shape       |

**Action:** Remove the `isinstance(payload, list)` branch. The server has
returned the dict format since v0.3.x. Delete `test_get_stats_legacy_format`.

---

### 10. Python — Config dict-conversion backward compatibility

| Item                    | Location                                                   | Status  | Detail                                                                                                                |
| ----------------------- | ---------------------------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------- |
| `geometry` property     | `tools/pdf-generator/pdf_generator/core/config_manager.py` | Pending | Still returns a dict explicitly for backward compatibility                                                            |
| Dict conversion helpers | `tools/pdf-generator/pdf_generator/core/config_manager.py` | Pending | `_colors_to_dict`, `_fonts_to_dict`, `_layout_to_dict`, `_pdf_to_dict`, and friends still support legacy dict callers |

**Action:** Audit callers. If all callers now use the dataclass properties
directly, remove the dict-conversion helpers and the `geometry` property. If
callers remain, migrate them to direct attribute access first.

---

### 11. Python — PyLaTeX fallback stubs

| Item         | Location                                                     | Status  | Detail                                                                                                    |
| ------------ | ------------------------------------------------------------ | ------- | --------------------------------------------------------------------------------------------------------- |
| Stub classes | `tools/pdf-generator/pdf_generator/core/document_builder.py` | Pending | The module still defines fallback `Document` / `Package` / `NoEscape` stubs when `pylatex` is unavailable |

**Action:** Make `pylatex` a hard dependency. Remove the fallback stubs. The PDF
generator is non-functional without pylatex — the stubs just defer the error.

---

### 12. Svelte/Web — Legacy `BackgroundCell` fields

| Item                   | Location                     | Status  | Detail                                                                                                         |
| ---------------------- | ---------------------------- | ------- | -------------------------------------------------------------------------------------------------------------- |
| Legacy optional fields | `web/src/lib/types/lidar.ts` | Pending | `ring?`, `azimuth_deg?`, and `average_range_meters?` still exist solely as backward-compatible optional fields |

**Action:** Remove these fields from the TypeScript type. The server stopped
sending them; the `?` optionality was the compat shim.

---

### 13. Svelte/Web — API response envelope migration

| Item                             | Location                      | Status  | Detail                                                                        |
| -------------------------------- | ----------------------------- | ------- | ----------------------------------------------------------------------------- |
| Fetch helper root-object parsing | `web/src/lib/api.ts`          | Removed | Runtime fetch code now expects the `{ metrics, histogram }` response envelope |
| Dual-format cache handling       | `web/src/routes/+page.svelte` | Pending | Cached data still accepts bare-array payloads via `Array.isArray(cached)`     |

**Action:** Remove the cached bare-array fallback and invalidate any stored
client data that still uses the old root-array shape.

---

### 14. Web / sweep dashboard — Sweep results legacy field names

| Item                           | Location                                           | Status  | Detail                                                                                              |
| ------------------------------ | -------------------------------------------------- | ------- | --------------------------------------------------------------------------------------------------- |
| Legacy asset fallback          | `internal/lidar/monitor/assets/sweep_dashboard.js` | Pending | The dashboard still falls back to `noise` / `closeness` / `neighbour` when `param_values` is absent |
| Legacy Svelte tests            | `web/src/lib/__tests__/sweep_dashboard.test.ts`    | Pending | Test cases still encode and assert the removed legacy field layout                                  |
| Legacy CSV export expectations | `web/src/lib/__tests__/sweep_dashboard.test.ts`    | Pending | CSV coverage still assumes legacy parameter keys can appear outside `param_values`                  |

**Action:** Remove the legacy fallback code paths and their tests. The sweep
dashboard should only accept the `param_values` map format. Aligns with Go
server-side removal (item 2).

---

### 15. macOS Visualiser — Branch-local track speed-label surfaces

| Item                                        | Location                                                                  | Status      | Detail                                                             |
| ------------------------------------------- | ------------------------------------------------------------------------- | ----------- | ------------------------------------------------------------------ |
| Swift track model legacy speed-label fields | `tools/visualiser-macos/VelocityVisualiser/Models/Models.swift`           | ✅ Resolved | Branch-local aggregate-percentile-labelled fields were not merged  |
| Client mapping for legacy speed labels      | `tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift`   | ✅ Resolved | No superseded speed-label field mappings in the shipped proto      |
| Generated proto bindings                    | `tools/visualiser-macos/VelocityVisualiser/Generated/visualiser.pb.swift` | ✅ Updated  | Regenerated after `peak_speed_mps` → `max_speed_mps` rename (#352) |
| Raw-max terminology in helpers and UI       | `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`          | ✅ Renamed  | `peak` → `max` terminology updated on unshipped surfaces (#352)    |

**Action:** ✅ Complete. Branch-local speed-label surfaces were not merged.
The `peak` → `max` rename landed in #352 across proto, Go, Swift, and TS.
SQL column rename is deferred to migration 000030.

---

### 16. macOS Visualiser — Legacy point buffer

| Item                         | Location                                                                  | Status   | Detail                                                                                                  |
| ---------------------------- | ------------------------------------------------------------------------- | -------- | ------------------------------------------------------------------------------------------------------- |
| `pointBuffer` rendering path | `tools/visualiser-macos/VelocityVisualiser/Rendering/MetalRenderer.swift` | Retained | Despite the comment, this buffer is still actively allocated and rendered when point drawing is enabled |

**Action:** Reclassify this as renderer-retirement work, not a v0.5 shim
removal, unless the composite renderer fully replaces it.

---

### 17. macOS Visualiser — Legacy playback defaults

| Item                      | Location                                                       | Status  | Detail                                                                                       |
| ------------------------- | -------------------------------------------------------------- | ------- | -------------------------------------------------------------------------------------------- |
| Legacy field preservation | `tools/visualiser-macos/VelocityVisualiser/App/AppState.swift` | Pending | `.unknown` still preserves legacy `isLive` / `isSeekable` defaults for old callers and tests |

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

## Migration Guide (target state for remaining removals)

### External contract changes

| Area                        | Old / branch-local state                                  | Target state                                                                  | Status             | Notes                                                               |
| --------------------------- | --------------------------------------------------------- | ----------------------------------------------------------------------------- | ------------------ | ------------------------------------------------------------------- |
| Track speed public contract | `peak_speed_mps` on proto and model surfaces              | `max_speed_mps` on proto/Go/Swift/TS; SQL column deferred to migration 000030 | ✅ Complete (#352) | Percentiles remain aggregate-only                                   |
| Report downloads            | `/api/reports/{id}/download?file_type=pdf`                | `/api/reports/{id}/download/{filename}.pdf`                                   | Partial            | Server route is already strict; callers/tests still need migration  |
| Sweep results               | Top-level `noise` / `closeness` / `neighbour` fallbacks   | `param_values` only                                                           | Partial            | Go request/result cleanup is done; dashboard fallbacks/tests remain |
| Radar stats payload         | Bare `[...]` arrays may still exist in cached client data | `{ "metrics": [...], "histogram": {...} }` only                               | Partial            | Fetch helper is updated; cache fallback remains                     |

### Internal cleanup targets

| Area                    | Target state                                                        | Status  | Notes                                                   |
| ----------------------- | ------------------------------------------------------------------- | ------- | ------------------------------------------------------- |
| Sweep JSON parsing      | Malformed JSON rejected with `400`                                  | Removed | Landed                                                  |
| Packet parsing          | `PacketHeader` deleted                                              | Pending | Deprecated reference-only struct still exists on `main` |
| Frame builder docs      | Stale `AddPoints` compat note removed                               | Removed | Landed on this branch                                   |
| Background grid TS type | Optional legacy fields removed                                      | Pending | Web-only cleanup                                        |
| Python PDF generator    | Dict-only stats payload, no dict-conversion shims, no PyLaTeX stubs | Pending | Needs dedicated Python cleanup                          |

---

## Delivery Plan

### Phase 1 — Audit and plan (this document)

- [x] Inventory all compat shims across Go, Python, Svelte, macOS
- [x] Classify as "remove in v0.5.0" vs "retain"
- [ ] Review with maintainer

### Phase 2 — Server-side removals (Go)

- [ ] Remove sweep legacy request fields and `computeCombinations()`
- [ ] Remove legacy sweep result fields from `ComboResult`
- [x] Return 400 on malformed sweep JSON instead of swallowing errors
- [ ] Delete `PacketHeader` struct
- [x] Delete stale `AddPoints` removal comment
- [x] Evaluate `lidar/aliases.go` outcome — retained and documented as an active package-boundary choice
- [ ] Finish the report download migration end-to-end (`file_type` callers/tests/terminology)
- [x] Proto `peak_speed_mps` → `max_speed_mps` rename on unshipped contracts (#352)

### Phase 3 — Frontend removals (Svelte)

- [ ] Remove `BackgroundCell` legacy fields from `lidar.ts`
- [ ] Remove `Array.isArray(cached)` dual-format branch
- [ ] Remove sweep legacy field fallback code and tests
- [ ] Move report downloads to filename-based URLs
- [ ] Bump cache version to invalidate stale client-side data

### Phase 4 — Python removals

- [ ] Remove legacy stats format branch in `api_client.py`
- [ ] Remove `test_get_stats_legacy_format` test
- [ ] Audit and remove config dict-conversion helpers
- [ ] Make pylatex a hard dependency; remove fallback stubs

### Phase 5 — macOS removals (Swift)

- [x] Back out branch-local aggregate-percentile label fields from the Swift model/client/UI — resolved; fields not merged
- [x] Rename raw `peak` terminology to `max` on unshipped visualiser surfaces (#352)
- [ ] Reclassify or remove `pointBuffer` only if the composite renderer fully replaces it
- [ ] Update callers of `setPlaybackMode(.unknown)` legacy branch
- [x] Verify `avgSpeedMps` field reads correctly from regenerated proto

### Phase 6 — Validation

- [ ] `make format && make lint && make test` passes
- [ ] `make build-radar-local` succeeds
- [ ] `make build-web` succeeds
- [ ] macOS visualiser builds and connects to gRPC stream after the track contract reset
- [ ] Report downloads work through filename-based routes only
- [ ] Sweep dashboard works with `param_values` format only
- [ ] PDF generator fetches stats in dict format only

---

## Decision Notes

- This plan is intentionally aggressive: all shims removed in one release.
  Maintaining dual formats across a minor release boundary would require test
  matrices and documentation for both formats, which costs more than a clean break.
- The speed contract reset is complete: `peak_speed_mps` → `max_speed_mps` landed in
  #352. `avg_speed_mps` remains the stable running-mean field for tracks;
  grouped/report aggregates continue to use percentile terminology. The SQL
  column rename is deferred to migration 000030.
- Items gated on external dependencies (deploy retirement, frontend consolidation)
  are excluded from this plan and tracked in the parent
  [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md)
  Projects B and C respectively.
- The breaking changes in this sub-plan are summarised in the parent plan's
  v0.5.0 Breaking Changes section (items 1, 4-7). That section is the
  consumer-facing changelog; this document is the implementation detail.
