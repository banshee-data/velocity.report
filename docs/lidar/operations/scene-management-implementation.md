# Phase 2 (Scene Management) Implementation

**Status:** ✅ Complete
**Design Document:** `docs/plans/lidar-track-labeling-auto-aware-tuning-plan.md`

## Overview

Implemented Phase 2 of the track labelling system, which introduces the concept of "scenes" — evaluation environments captured in PCAP files that can be tied to sensors, reference ground truth runs, and optimal parameter configurations.

## What is a Scene?

A **scene** represents a specific environment captured in a PCAP file:

- Ties a PCAP file to a specific sensor
- Can have a reference analysis run (labelled ground truth)
- Stores optimal parameters discovered through auto-tuning
- Supports time windowing (start offset and duration)
- Human-readable description for documentation

Different scenes from the same PCAP (e.g., different time segments) can have different optimal parameters.

## Implementation Details

### Phase 2.1: Database Migration (000020)

Created `lidar_scenes` table with the following schema:

```sql
CREATE TABLE IF NOT EXISTS lidar_scenes (
    scene_id TEXT PRIMARY KEY,
    sensor_id TEXT NOT NULL,
    pcap_file TEXT NOT NULL,
    pcap_start_secs REAL,
    pcap_duration_secs REAL,
    description TEXT,
    reference_run_id TEXT,
    optimal_params_json TEXT,
    created_at_ns INTEGER NOT NULL,
    updated_at_ns INTEGER,
    FOREIGN KEY (reference_run_id) REFERENCES lidar_analysis_runs(run_id) ON DELETE SET NULL
);
```

**Indexes:**

- `idx_lidar_scenes_sensor` on `sensor_id`
- `idx_lidar_scenes_pcap` on `pcap_file`

**Files:**

- `internal/db/migrations/000020_create_lidar_scenes.up.sql`
- `internal/db/migrations/000020_create_lidar_scenes.down.sql`

### Phase 2.2: SceneStore

Created `internal/lidar/scene_store.go` with comprehensive CRUD operations.

#### Scene Struct

```go
type Scene struct {
    SceneID           string          `json:"scene_id"`
    SensorID          string          `json:"sensor_id"`
    PCAPFile          string          `json:"pcap_file"`
    PCAPStartSecs     *float64        `json:"pcap_start_secs,omitempty"`
    PCAPDurationSecs  *float64        `json:"pcap_duration_secs,omitempty"`
    Description       string          `json:"description,omitempty"`
    ReferenceRunID    string          `json:"reference_run_id,omitempty"`
    OptimalParamsJSON json.RawMessage `json:"optimal_params_json,omitempty"`
    CreatedAtNs       int64           `json:"created_at_ns"`
    UpdatedAtNs       *int64          `json:"updated_at_ns,omitempty"`
}
```

#### SceneStore Methods

- **InsertScene(scene)** — Creates new scene, auto-generates UUID if scene_id empty
- **GetScene(sceneID)** — Retrieves scene by ID
- **ListScenes(sensorID)** — Lists all scenes or filtered by sensor_id
- **UpdateScene(scene)** — Updates description, reference_run_id, optimal_params_json
- **DeleteScene(sceneID)** — Removes scene
- **SetReferenceRun(sceneID, runID)** — Sets reference run (ground truth)
- **SetOptimalParams(sceneID, paramsJSON)** — Updates optimal parameters JSON

**Nullable Field Handling:**

- Uses `sql.NullFloat64`, `sql.NullString`, `sql.NullInt64` for database operations
- Pointers in Go structs for optional fields
- Helper functions `nullFloat64()`, `nullInt64()` for conversion
- Reuses `nullString()` from track_store.go

### Phase 2.3: REST API

Created `internal/lidar/monitor/scene_api.go` with HTTP endpoints:

| Method | Endpoint                              | Description                                                       |
| ------ | ------------------------------------- | ----------------------------------------------------------------- |
| GET    | `/api/lidar/scenes`                   | List all scenes (optional `?sensor_id=X` filter)                  |
| POST   | `/api/lidar/scenes`                   | Create new scene from JSON body                                   |
| GET    | `/api/lidar/scenes/{scene_id}`        | Get scene details including reference run and params              |
| PUT    | `/api/lidar/scenes/{scene_id}`        | Update scene (description, reference_run_id, optimal_params_json) |
| DELETE | `/api/lidar/scenes/{scene_id}`        | Delete scene                                                      |
| POST   | `/api/lidar/scenes/{scene_id}/replay` | Replay PCAP, creating an analysis run (returns 202 on success)    |

**Request/Response Types:**

- `CreateSceneRequest` — validated required fields (sensor_id, pcap_file)
- `UpdateSceneRequest` — uses pointers to distinguish "not set" from "set to empty"

**Routes Registration:**
Routes added to `webserver.go` RegisterRoutes():

```go
if ws.db != nil {
    mux.HandleFunc("/api/lidar/scenes", ws.handleScenes)
    mux.HandleFunc("/api/lidar/scenes/", ws.handleSceneByID)
}
```

### Phase 2.4 & 2.5: Scene Replay and Sweep Integration

Phase 2.4 (scene replay) is now implemented. The `/replay` endpoint creates an analysis run and initiates PCAP replay using the scene's parameters:

- Returns 202 with the created `run_id` on success
- Returns 404 if the scene is not found
- Returns 500 if the analysis run cannot be created

Phase 2.5 (sweep integration) adds the `AnalysisRunCreator` interface and `RunID` field on `ComboResult` to support creating analysis runs per sweep combination.

## Testing

### SceneStore Tests (`scene_store_test.go`)

7 test cases covering:

1. **TestSceneStore_InsertAndGet** — Basic CRUD, UUID generation, timestamp handling
2. **TestSceneStore_ListScenes** — Filtering by sensor_id, ordering (newest first)
3. **TestSceneStore_UpdateScene** — Field updates, updated_at_ns tracking
4. **TestSceneStore_DeleteScene** — Deletion, non-existent scene handling
5. **TestSceneStore_SetReferenceRun** — Reference run setting/clearing
6. **TestSceneStore_SetOptimalParams** — Optimal params JSON storage
7. **TestSceneStore_NullableFields** — Verify nullable fields remain nil when not set

### Scene API Tests (`scene_api_test.go`)

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

1. **UUID for scene_id:** Global uniqueness, generated client-side or server-side
2. **Empty array on no results:** List endpoint returns `[]` not `null` for consistency
3. **Pointer fields in updates:** Distinguishes "not provided" from "set to empty string"
4. **404 for missing resources:** Delete/update operations return 404 for non-existent scenes
5. **Database check:** All endpoints verify db != nil before proceeding (503 if not)

## Integration Points

### Current

- Database: `lidar_analysis_runs` table (foreign key)
- WebServer: Routes registered in `RegisterRoutes()`
- Existing patterns: Follows `run_track_api.go` URL parsing pattern

### Future (Phase 2.4 & 2.5)

- PCAP replay: `/api/lidar/scenes/{scene_id}/replay` will trigger replay
- Auto-tuning: Sweep runner will create scenes and link optimal params
- Ground truth evaluation: Compare runs against scene's reference run

## Next Steps

**Phase 3 (Svelte UI):** Add scene selector and track labelling controls to tracks page
**Phase 4 (Ground Truth Evaluation):** Implement track matching algorithm and scoring
**Phase 5 (Label-Aware Auto-Tuning):** Connect auto-tuner to use reference runs for optimisation

## Files Changed

**New Files:**

- `internal/db/migrations/000020_create_lidar_scenes.up.sql`
- `internal/db/migrations/000020_create_lidar_scenes.down.sql`
- `internal/lidar/scene_store.go`
- `internal/lidar/scene_store_test.go`
- `internal/lidar/monitor/scene_api.go`
- `internal/lidar/monitor/scene_api_test.go`

**Modified Files:**

- `internal/lidar/monitor/webserver.go` (route registration)

**Lines of Code:**

- Production: ~650 lines
- Tests: ~520 lines
- Total: ~1,170 lines
