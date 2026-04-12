# Phase 2 (Replay Case Management) Implementation

- **Status:** ✅ Complete
- **Design Document:** `docs/plans/lidar-track-labelling-auto-aware-tuning-plan.md`
- **Terminology note:** Database schema renamed to `lidar_replay_cases` in v0.5.x migration 031. API paths and Go types (`Scene`, `SceneStore`) still use the old "scene" term pending code rename in v0.5.1+. Dashboard frontend already migrated.

Implementation of replay case management: structured evaluation environments from PCAP captures tied to sensors, ground truth runs, and optimal parameter sets.

## Overview

Implemented Phase 2 of the track labelling system, which introduces **replay cases** — evaluation environments captured in PCAP files that can be tied to sensors, reference ground truth runs, and optimal parameter configurations.

## What is a Replay Case?

A **replay case** represents a specific environment captured in a PCAP file:

- Ties a PCAP file to a specific sensor
- Can have a reference analysis run (labelled ground truth)
- Stores optimal parameters discovered through auto-tuning
- Supports time windowing (start offset and duration)
- Human-readable description for documentation

Different replay cases from the same PCAP (e.g., different time segments) can have different optimal parameters.

## Implementation Details

### Phase 2.1: Database Schema (v0.5.x Migrations)

Replay cases are persisted in the `lidar_replay_cases` table, created by migration 031 (which renamed the earlier `lidar_scenes` table):

```sql
CREATE TABLE IF NOT EXISTS "lidar_replay_cases" (
    replay_case_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    pcap_file TEXT NOT NULL,
    pcap_start_secs REAL,
    pcap_duration_secs REAL,
    description TEXT,
    reference_run_id TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    recommended_param_set_id TEXT REFERENCES lidar_param_sets (param_set_id) ON DELETE SET NULL
);
```

**Indexes:**

- `idx_lidar_replay_cases_sensor` on `sensor_id`
- `idx_lidar_replay_cases_pcap` on `pcap_file`
- `idx_lidar_replay_cases_recommended_param_set` on `recommended_param_set_id`

**Files:**

- `internal/db/migrations/000031_table_naming.up.sql` (renames from `lidar_scenes`)
- `internal/db/migrations/000031_table_naming.down.sql`

### Phase 2.2: ReplayCaseStore

Created `internal/lidar/storage/sqlite/scene_store.go` with comprehensive CRUD operations (file rename pending in v0.5.1+).

#### ReplayCase Struct

```go
type ReplayCase struct {
    ReplayCaseID           string          `json:"replay_case_id"`
    SensorID               string          `json:"sensor_id"`
    PCAPFile               string          `json:"pcap_file"`
    PCAPStartSecs          *float64        `json:"pcap_start_secs,omitempty"`
    PCAPDurationSecs       *float64        `json:"pcap_duration_secs,omitempty"`
    Description            string          `json:"description,omitempty"`
    ReferenceRunID         string          `json:"reference_run_id,omitempty"`
    RecommendedParamSetID  string          `json:"recommended_param_set_id,omitempty"`
    CreatedAtNs            int64           `json:"created_at_ns"`
    UpdatedAtNs            *int64          `json:"updated_at_ns,omitempty"`
}
```

#### Store Methods

Current method names (pending rename to `ReplayCase*` prefix):

- `InsertScene(scene)` → retrieve as `ReplayCase`, method name pending
- `GetScene(replayCaseID)` → retrieve as `ReplayCase`, method name pending
- `ListScenes(sensorID)` → retrieve as `[]*ReplayCase`, method name pending
- `UpdateScene(scene)` → update with `ReplayCase`, method name pending
- `DeleteScene(replayCaseID)` → delete by ID, method name pending
- `SetReferenceRun(replayCaseID, runID)` — Sets reference run (ground truth)
- `SetOptimalParams(replayCaseID, paramsJSON)` — Updates optimal parameters JSON (via `recommended_param_set_id` link)

**Nullable Field Handling:**

- Uses `sql.NullFloat64`, `sql.NullString`, `sql.NullInt64` for database operations
- Pointers in Go structs for optional fields
- Helper functions `nullFloat64()`, `nullInt64()` for conversion
- Reuses `nullString()` from track_store.go

### Phase 2.3: REST API

Created `internal/lidar/server/scene_api.go` with HTTP endpoints (file and handler names pending rename in v0.5.1+):

| Method | Endpoint                                    | Description                              |
|--------|---------------------------------------------|------------------------------------------|
| GET    | `/api/lidar/scenes`                         | List all replay cases (optional filter)  |
| POST   | `/api/lidar/scenes`                         | Create new replay case from JSON body    |
| GET    | `/api/lidar/scenes/{replay_case_id}`        | Get replay case details including params |
| PUT    | `/api/lidar/scenes/{replay_case_id}`        | Update replay case metadata              |
| DELETE | `/api/lidar/scenes/{replay_case_id}`        | Delete replay case                       |
| POST   | `/api/lidar/scenes/{replay_case_id}/replay` | Replay PCAP, creating analysis run (202) |

> **Pending rename (v0.5.1+):** API paths will change from `/api/lidar/scenes` to `/api/lidar/replay-cases`. This is a breaking change planned for the v0.5.1 code rename. See [lidar-replay-case-terminology-alignment-plan.md](../../plans/lidar-replay-case-terminology-alignment-plan.md).

**Request/Response Types:**

- `CreateSceneRequest` — validated required fields (sensor_id, pcap_file) — handler name pending rename
- `UpdateSceneRequest` — uses pointers to distinguish "not set" from "set to empty" — handler name pending rename

**Routes Registration:**

Routes added to `server/routes.go` RegisterRoutes():

```go
// Scene API routes (scene management for track labelling and auto-tuning)
mux.HandleFunc("/api/lidar/scenes", ws.withDB(ws.handleScenes))
mux.HandleFunc("/api/lidar/scenes/", ws.withDB(ws.handleSceneByID))
```

### Phase 2.4 & 2.5: Replay and Sweep Integration

Phase 2.4 (replay) is now implemented. The `/replay` endpoint creates an analysis run and initiates PCAP replay using the replay case's parameters:

- Returns 202 with the created `run_id` on success
- Returns 404 if the replay case is not found
- Returns 500 if the analysis run cannot be created

Phase 2.5 (sweep integration) adds the `AnalysisRunCreator` interface and `RunID` field on `ComboResult` to support creating analysis runs per sweep combination.

## Testing

### ReplayCaseStore Tests (`scene_store_test.go`)

7 test cases covering:

1. **TestSceneStore_InsertAndGet** — Basic CRUD, UUID generation, timestamp handling
2. **TestSceneStore_ListScenes** — Filtering by sensor_id, ordering (newest first)
3. **TestSceneStore_UpdateScene** — Field updates, updated_at_ns tracking
4. **TestSceneStore_DeleteScene** — Deletion, non-existent replay case handling
5. **TestSceneStore_SetReferenceRun** — Reference run setting/clearing
6. **TestSceneStore_SetOptimalParams** — Optimal params JSON storage
7. **TestSceneStore_NullableFields** — Verify nullable fields remain nil when not set

### Replay Case API Tests (`scene_api_test.go`)

8 test cases (15 total sub-tests) covering all REST endpoints with validation.

**Test Results:** ✅ All 15 tests passing

## Code Quality

- ✅ British English in all comments ("labelling", "optimisation")
- ✅ UUID generation using `github.com/google/uuid`
- ✅ Proper nullable field handling throughout
- ✅ Field validation in create handler
- ✅ Error handling with appropriate HTTP status codes
- ✅ Go formatting (gofmt) passes
- ✅ Full test coverage of success and error paths

## API Design Decisions

1. **UUID for replay case ID:** Global uniqueness, generated client-side or server-side
2. **Empty array on no results:** List endpoint returns `[]` not `null` for consistency
3. **Pointer fields in updates:** Distinguishes "not provided" from "set to empty string"
4. **404 for missing resources:** Delete/update operations return 404 for non-existent replay cases
5. **Database check:** All endpoints verify db != nil before proceeding (503 if not)

## Integration Points

### Current

- Database: `lidar_analysis_runs` table (foreign key)
- WebServer: Routes registered in `RegisterRoutes()`
- Existing patterns: Follows `run_track_api.go` URL parsing pattern

### Future

- Auto-tuning: Sweep runner will create replay cases and link optimal params
- Ground truth evaluation: Compare runs against replay case's reference run

## Next Steps

**Phase 3 (Svelte UI):** Add replay case selector and track labelling controls to tracks page
**Phase 4 (Ground Truth Evaluation):** Implement track matching algorithm and scoring
**Phase 5 (Label-Aware Auto-Tuning):** Connect auto-tuner to use reference runs for optimisation

## Files Changed

**Migration Files:**

- `internal/db/migrations/000020_create_lidar_scenes.up.sql` — Original table creation (v0.5.0)
- `internal/db/migrations/000020_create_lidar_scenes.down.sql` — Rollback
- `internal/db/migrations/000031_table_naming.up.sql` — Renames table to `lidar_replay_cases` and columns (v0.5.x)
- `internal/db/migrations/000031_table_naming.down.sql` — Rollback to `scene` names

**Store Layer:**

- `internal/lidar/storage/sqlite/scene_store.go` → File rename pending; type `ReplayCase` fully migrated
- `internal/lidar/storage/sqlite/scene_store_test.go` → File rename pending; tests use `ReplayCase` type
- `internal/lidar/storage/sqlite/scene_store_coverage_test.go` → File rename pending

**API Layer:**

- `internal/lidar/server/scene_api.go` → File rename pending; handlers use `ReplayCase` internally
- `internal/lidar/server/scene_api_test.go` → File rename pending
- `internal/lidar/server/scene_api_coverage_test.go` → File rename pending
- `internal/lidar/server/routes.go` — Routes still use `/api/lidar/scenes` paths (pending rename)

**Lines of Code:**

- Production: ~650 lines
- Tests: ~520 lines
- Total: ~1,170 lines