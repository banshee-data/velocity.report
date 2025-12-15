# DRY cleanup plan (Go + JS focus)

This document captures the highest-impact duplication hotspots and a concrete plan to converge on single codepaths.

## Findings

- **Aggregation groups drift**  
  - Go server defines `supportedGroups` with long-range buckets (`2d`, `3d`, `7d`, `14d`, `28d`) (`internal/api/server.go`).  
  - Frontend hardcodes a separate `groupOptions` array (`web/src/routes/+page.svelte`).  
  - Python stats client ships a third map (`tools/pdf-generator/.../api_client.py`) that omits day-level groups. This is already diverging.

- **Units + timezone validation duplicated across languages**  
  - Go canonical list and conversion live in `internal/units/velocity.go` and `internal/units/timezone.go`.  
  - Frontend re-declares unit strings and labels in `web/src/lib/units.ts` (plus localStorage helpers). Any new unit or label change requires touching both stacks.

- **Radar stats contract re-specified per consumer**  
  - Go response types: `RadarStatsResult` (`internal/db/db.go`) and the HTTP shape assembled in `internal/api/server.go`.  
  - Frontend redefines `RawRadarStats`/`RadarStats` (`web/src/lib/api.ts`) and reshapes payloads manually.  
  - Python `RadarStatsClient` assumes its own field names. There is no shared schema, so a field rename will silently drift consumers.

- **Query parsing/validation repeated in handlers**  
  - `listEvents`, `showRadarObjectStats`, and `generateReport` each hand-parse `units`, `timezone`, `group`, and defaults inside `internal/api/server.go`.
  - Repetitive error strings and defaulting increase the chance of inconsistent validation paths.

- **Frontend fetch wrappers repeat status/error handling**  
  - `web/src/lib/api.ts` repeats fetch boilerplate and JSON parsing per endpoint. There is no shared helper to normalize errors or inject defaults (units/timezone/source).

## Plan (sequenced, minimal, DRY-first)

1. **Single aggregation-group source of truth**
   - Move the canonical group map to a dedicated Go module (e.g., `internal/config/groups.go`) and expose via a lightweight `/api/metadata/groups` endpoint.
   - Generate TS constants from that source during web builds (simple JSON emit consumed by `groupOptions`) and update the Python client to read the same JSON file or endpoint.

2. **Unify unit/timezone definitions**
   - Export `ValidUnits` (and display labels) plus timezone validation hints via `/api/config/metadata` backed by `internal/units`.
   - Replace hardcoded unions in `web/src/lib/units.ts` with a generated `Unit` type derived from the Go list; keep storage helpers but not the string list.

3. **Shared radar stats contract**
   - Author a small OpenAPI/JSON Schema for `RadarStatsResponse` and the request query params.  
   - Use it to generate TS types (replacing manual `RawRadarStats` mapping) and to drive the Python `RadarStatsClient`.  
   - In Go, keep using the existing structs but serve the schema alongside the endpoint to anchor compatibility tests.

4. **Centralize query parsing in Go**
   - Extract helpers for `(units, timezone, group)` parsing/defaulting into `internal/api/params` and reuse in `listEvents`, `showRadarObjectStats`, and `generateReport`.  
   - Add a single error builder to keep messages consistent and easier to localize.

5. **Normalize frontend API wrapper**
   - Introduce a small `request` helper in `web/src/lib/api.ts` (or `web/src/lib/http.ts`) that handles status checks, JSON parsing, and optional unit/timezone injection.  
   - Refactor existing functions to call the helper to remove duplicated boilerplate and align error handling.

6. **Guardrails**
   - Add a lightweight contract test that compares generated TS constants against the Go `supportedGroups` and `ValidUnits` to catch drift in CI.  
   - Document the single sources (groups + units + stats schema) in `ARCHITECTURE.md` once wired to prevent future duplication.

These steps keep behavior stable while converging on one source per concern, reducing regression risk from future changes.
