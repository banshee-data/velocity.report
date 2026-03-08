# v0.5.0 Backward Compatibility Shim Removal Plan

**Parent plan:** [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md) — Project E
**Related:** [LiDAR Visualiser Proto Contract Plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md) (speed summary fields), [Speed Percentile Aggregation Alignment Plan](speed-percentile-aggregation-alignment-plan.md)

## Status: In Progress

**Update (March 8, 2026):** The earlier per-track percentile expansion is
superseded. Public track contracts should not ship `p50/p85/p98`; those remain
grouped/report aggregate metrics only. Stable track surfaces remain
`avg_speed_mps` plus the raw maximum for now, with a separate pending rename of
raw `peak_speed_mps` to `max_speed_mps` on unshipped contracts.

## Tracking Snapshot

| Outcome                            | Sections        | Notes                                                                                                                                                             |
| ---------------------------------- | --------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Removed in code                    | §2, §4, §6, §7  | The Go-side sweep request/result cleanup is already landed; malformed sweep JSON now returns `400`; `PacketHeader` and the stale `AddPoints` compat note are gone |
| Pending                            | §3, §9-§14, §17 | Consumer migrations and fallback/test cleanup still need implementation                                                                                           |
| Deferred / retained                | §5, §8, §16     | Either owned by another plan or still an active implementation path rather than a removable shim today                                                            |
| Superseded / back out before merge | §1, §15         | Unmerged per-track percentile surfaces should be backed out; raw `peak` to `max` rename is tracked separately                                                     |

## Shim Work Already Removed

| Shim                                                                 | Section | Notes                                                                                                    |
| -------------------------------------------------------------------- | ------- | -------------------------------------------------------------------------------------------------------- |
| Sweep legacy request fields and result aliases removed from Go types | §2      | `SweepRequest` now uses `Params`; `ComboResult` now uses `param_values`; `computeCombinations()` is gone |
| Lenient sweep JSON parsing removed                                   | §4      | Empty body (`io.EOF`) is still tolerated, but malformed JSON now returns `400 Bad Request`               |
| Deprecated packet header reference struct removed                    | §6      | `PacketHeader` no longer exists in `extract.go`                                                          |
| Stale `AddPoints` removal note deleted                               | §7      | `frame_builder.go` no longer carries the compat comment                                                  |

**Remaining:** finish the report-download migration end-to-end, remove the
remaining Python/web/macOS fallback code, and back out the unmerged per-track
percentile surfaces.

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
- **Proto contract plan:** The track speed contract reset (§1, §15 below) is
  also tracked in the
  [proto contract plan](lidar-visualiser-proto-contract-and-debug-overlay-fixes-plan.md)
  Phase C/D. That plan owns gRPC/proto surface changes and Swift regeneration;
  this plan owns the REST API and internal model cleanup. Proto field 24 stays
  `avg_speed_mps` (unchanged). The branch-local per-track percentile fields are
  not approved to ship as stable public track metrics and should be backed out
  or quarantined as part of the reset.

---

## Inventory of Backward Compatibility Shims

### 1. Go Server — track speed contract reset

| Item                                     | Location                                                                               | Status       | Detail                                                                                                                              |
| ---------------------------------------- | -------------------------------------------------------------------------------------- | ------------ | ----------------------------------------------------------------------------------------------------------------------------------- |
| Stable public track field                | `internal/lidar/monitor/track_api.go`, `proto/velocity_visualiser/v1/visualiser.proto` | Active       | `avg_speed_mps` remains the stable running-mean field for now                                                                       |
| Stable public raw-max field              | `internal/lidar/monitor/track_api.go`, `proto/velocity_visualiser/v1/visualiser.proto` | Active       | The raw maximum remains available today, but should be renamed from `peak_speed_mps` to `max_speed_mps` before merge where possible |
| Branch-local percentile additions        | proto, REST, visualiser model/UI                                                       | Superseded   | Do not merge per-track `p50/p85/p98` surface expansion                                                                              |
| Existing percentile columns/calculations | `lidar_tracks`, analysis runs, classifier features                                     | Transitional | Existing internal/storage use may remain temporarily during migration, but no new public dependency should be added                 |

**Decision:** Keep `avg_speed_mps` and the raw maximum as the only stable public
track speed fields for now. Reserve `p50/p85/p98` for grouped/report aggregates
only. Rename the raw maximum from `peak_speed_mps` to `max_speed_mps` in
unshipped contracts, and reserve the word `peak` for a future filtered measure.
Track-level speed summaries will be redesigned separately with distinct
non-percentile names and formulas.

**Action items:**

1. Back out unmerged per-track `p50/p85/p98` proto/REST/UI work before merge
2. Rename public/raw `peak_speed_mps` references to `max_speed_mps` where the
   contract is still unshipped
3. Define replacement public track metrics in the speed percentile alignment plan
4. Migrate any temporary internal percentile dependencies to the new track metrics
5. Keep aggregate percentile work isolated to grouped/report surfaces

---

### 2. Go Server — Sweep legacy request format

| Item                             | Location                               | Status  | Detail                                                                                                                            |
| -------------------------------- | -------------------------------------- | ------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Legacy multi-mode request fields | `internal/lidar/sweep/runner.go`       | Removed | `SweepRequest` no longer carries `NoiseValues`, `ClosenessValues`, `NeighbourValues`, range triples, or fixed-value compat fields |
| Legacy result fields             | `internal/lidar/sweep/runner.go`       | Removed | `ComboResult` now exposes `param_values` only; top-level `Noise` / `Closeness` / `Neighbour` aliases are gone                     |
| Legacy combination helper        | `internal/lidar/sweep/sweep_params.go` | Removed | `expandSweepParam()` / `cartesianProduct()` replaced the old `computeCombinations()` logic                                        |

**Action:** Go-side request/result compat cleanup is complete. The remaining
sweep fallback work is frontend-only and is tracked in §14.

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

| Item                  | Location                                    | Status  | Detail                                                    |
| --------------------- | ------------------------------------------- | ------- | --------------------------------------------------------- |
| `PacketHeader` struct | `internal/lidar/l1packets/parse/extract.go` | Removed | The unused reference-only struct has already been deleted |

**Action:** None.

---

### 7. Go Server — Removed method comment

| Item                     | Location                                   | Status  | Detail                                            |
| ------------------------ | ------------------------------------------ | ------- | ------------------------------------------------- |
| `AddPoints` removal note | `internal/lidar/l2frames/frame_builder.go` | Removed | The stale compat comment has already been deleted |

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

### 15. macOS Visualiser — Branch-local track percentile surfaces

| Item                                     | Location                                                                  | Status       | Detail                                                                                                            |
| ---------------------------------------- | ------------------------------------------------------------------------- | ------------ | ----------------------------------------------------------------------------------------------------------------- |
| Swift track model percentile fields      | `tools/visualiser-macos/VelocityVisualiser/Models/Models.swift`           | Superseded   | `p50SpeedMps`, `p85SpeedMps`, and `p98SpeedMps` exist locally but should not ship as stable per-track metrics     |
| Client mapping for per-track percentiles | `tools/visualiser-macos/VelocityVisualiser/gRPC/VisualiserClient.swift`   | Superseded   | The client still maps branch-local percentile fields from the proto into the Swift model                          |
| Generated proto bindings                 | `tools/visualiser-macos/VelocityVisualiser/Generated/visualiser.pb.swift` | Transitional | Generated from the current branch-local proto; revisit when the track contract is reset                           |
| Raw-max terminology in helpers and UI    | `tools/visualiser-macos/VelocityVisualiser/UI/ContentView.swift`          | Pending      | Helpers and labels still use `peak` language for the raw maximum; this should move to `max` on unshipped surfaces |

**Action:** Do not add more UI around per-track percentiles. Back out the
unmerged percentile surfaces and apply the separate raw `peak` to `max` rename
on unshipped visualiser contracts.

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

| Area                        | Old / branch-local state                                                                             | Target state                                                                                              | Status               | Notes                                                               |
| --------------------------- | ---------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- | -------------------- | ------------------------------------------------------------------- |
| Track speed public contract | Branch-local work adds per-track `p50/p85/p98`; raw max still uses `peak_speed_mps` in some surfaces | Public track metrics stay non-percentile; raw maximum is renamed to `max_speed_mps` on unshipped surfaces | Superseded / pending | Percentiles remain aggregate-only                                   |
| Report downloads            | `/api/reports/{id}/download?file_type=pdf`                                                           | `/api/reports/{id}/download/{filename}.pdf`                                                               | Partial              | Server route is already strict; callers/tests still need migration  |
| Sweep results               | Top-level `noise` / `closeness` / `neighbour` fallbacks                                              | `param_values` only                                                                                       | Partial              | Go request/result cleanup is done; dashboard fallbacks/tests remain |
| Radar stats payload         | Bare `[...]` arrays may still exist in cached client data                                            | `{ "metrics": [...], "histogram": {...} }` only                                                           | Partial              | Fetch helper is updated; cache fallback remains                     |

### Internal cleanup targets

| Area                    | Target state                                                        | Status  | Notes                          |
| ----------------------- | ------------------------------------------------------------------- | ------- | ------------------------------ |
| Sweep JSON parsing      | Malformed JSON rejected with `400`                                  | Removed | Landed                         |
| Packet parsing          | `PacketHeader` deleted                                              | Removed | Landed                         |
| Frame builder docs      | Stale `AddPoints` compat note removed                               | Removed | Landed                         |
| Background grid TS type | Optional legacy fields removed                                      | Pending | Web-only cleanup               |
| Python PDF generator    | Dict-only stats payload, no dict-conversion shims, no PyLaTeX stubs | Pending | Needs dedicated Python cleanup |

---

## Delivery Plan

### Phase 1 — Audit and plan (this document)

- [x] Inventory all compat shims across Go, Python, Svelte, macOS
- [x] Classify as "remove in v0.5.0" vs "retain"
- [ ] Review with maintainer

### Phase 2 — Server-side removals (Go)

- [x] Remove sweep legacy request fields and `computeCombinations()`
- [x] Remove legacy sweep result fields from `ComboResult`
- [x] Return 400 on malformed sweep JSON instead of swallowing errors
- [x] Delete `PacketHeader` struct and `AddPoints` removal comment
- [x] Evaluate `lidar/aliases.go` outcome — retained and documented as an active package-boundary choice
- [ ] Finish the report download migration end-to-end (`file_type` callers/tests/terminology)
- [ ] Back out unmerged public track percentile surfaces and queue the raw `peak` to `max` rename

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

- [ ] Back out branch-local `p50/p85/p98` track fields from the Swift model/client/UI
- [ ] Rename raw `peak` terminology to `max` on unshipped visualiser surfaces
- [ ] Reclassify or remove `pointBuffer` only if the composite renderer fully replaces it
- [ ] Update callers of `setPlaybackMode(.unknown)` legacy branch
- [ ] Verify `medianSpeedMps` field reads correctly from regenerated proto

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
- Per-track percentile additions are no longer part of the approved v0.5.0
  contract. `avg_speed_mps` remains the stable running-mean field for tracks;
  grouped/report aggregates continue to use percentile terminology; raw track
  `peak` to `max` naming is tracked as a separate unshipped contract cleanup.
- Items gated on external dependencies (deploy retirement, frontend consolidation)
  are excluded from this plan and tracked in the parent
  [Simplification and Deprecation Plan](platform-simplification-and-deprecation-plan.md)
  Projects B and C respectively.
- The breaking changes in this sub-plan are summarised in the parent plan's
  v0.5.0 Breaking Changes section (items 1, 4-7). That section is the
  consumer-facing changelog; this document is the implementation detail.
