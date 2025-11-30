# Development Log

## November 30, 2025 - REST API Endpoints (Phase 3.5)

### Phase 3.5: Track/Cluster REST API
- Implemented `TrackAPI` struct in `internal/lidar/monitor/track_api.go`:
  - `GET /api/lidar/tracks` - List tracks with optional state filter
  - `GET /api/lidar/tracks/active` - Active tracks (real-time from memory or DB)
  - `GET /api/lidar/tracks/{track_id}` - Get specific track details
  - `PUT /api/lidar/tracks/{track_id}` - Update track metadata (class, confidence)
  - `GET /api/lidar/tracks/{track_id}/observations` - Get track trajectory
  - `GET /api/lidar/tracks/summary` - Aggregated statistics by class/state
  - `GET /api/lidar/clusters` - Recent clusters by time range
- JSON response structures for API consistency:
  - `TrackResponse` with position, velocity, classification, bounding box
  - `ClusterResponse` with centroid, bounding box, point metrics
  - `TracksListResponse` and `ClustersListResponse` for list endpoints
  - `TrackSummaryResponse` with by-class and by-state aggregation
- Support for both in-memory tracker (real-time) and database queries
- Comprehensive unit tests in `internal/lidar/monitor/track_api_test.go`

### Documentation Updates
- Updated `foreground_tracking_plan.md` to v6.0 (Phase 3.5 complete)
- Updated `lidar_sidecar_overview.md` with REST API endpoint status
- Updated implementation files table with track_api.go
- Updated milestones and production readiness assessment

## November 30, 2025 - SQL Schema & Track Classification (Phases 3.3-3.4)

### Phase 3.3: SQL Schema & Database Persistence
- Created migration `000009_create_lidar_tracks.up.sql` with:
  - `lidar_clusters` table for DBSCAN cluster persistence
  - `lidar_tracks` table for track lifecycle, kinematics, and classification
  - `lidar_track_obs` table for per-observation tracking data
  - Appropriate indexes for sensor_id, time range, and state queries
- Implemented persistence functions in `internal/lidar/track_store.go`:
  - `InsertCluster()` - Insert cluster with world-frame features
  - `InsertTrack()` - Create new track with speed percentiles
  - `UpdateTrack()` - Update track state, features, and classification
  - `InsertTrackObservation()` - Record per-observation data
  - `GetActiveTracks()` - Query tracks by sensor and state
  - `GetTrackObservations()` - Get trajectory data for track
  - `GetRecentClusters()` - Query clusters by time range
- Updated `internal/db/schema.sql` to include all track tables
- Comprehensive unit tests in `internal/lidar/track_store_test.go`

### Phase 3.4: Track Classification
- Implemented rule-based classification in `internal/lidar/classification.go`:
  - `TrackClassifier` with model version tracking
  - Object classes: `pedestrian`, `car`, `bird`, `other`
  - Classification features: height, length, width, speed, duration, observation count
  - Configurable thresholds for each class with reasonable defaults
  - Confidence scoring based on feature match quality
- Added speed percentile computation:
  - `ComputeSpeedPercentiles()` for P50/P85/P95 from speed history
- Classification integration:
  - `ClassifyAndUpdate()` for updating track classification fields
  - Added `ObjectClass`, `ObjectConfidence`, `ClassificationModel` fields to `TrackedObject`
- Comprehensive unit tests in `internal/lidar/classification_test.go`

### Documentation Updates
- Updated `foreground_tracking_plan.md` to reflect completion through Phase 3.4
- Updated `lidar_sidecar_overview.md` with new module structure and completed phases
- Updated Implementation Files table with Phase 3.3 and 3.4 files
- Updated milestones and roadmap status

## November 30, 2025 - Foreground Tracking Pipeline (Phases 2.9-3.2)

### Phase 2.9: Foreground Mask Generation (Polar Frame)
- Implemented `ProcessFramePolarWithMask()` in `internal/lidar/foreground.go` for per-point foreground/background classification
- Added `ExtractForegroundPoints()` helper to filter foreground points using mask
- Added `ComputeFrameMetrics()` for frame-level statistics (total, foreground, background counts)
- Comprehensive unit tests in `internal/lidar/foreground_test.go`

### Phase 3.0: Polar → World Transform
- Implemented `WorldPoint` struct for world-frame Cartesian coordinates
- Added `TransformToWorld()` function with pose support in `internal/lidar/clustering.go`
- Added `TransformPointsToWorld()` convenience function for pre-computed Cartesian points
- Identity transform fallback when pose is nil
- Unit tests for transform accuracy with identity and custom poses

### Phase 3.1: DBSCAN Clustering (World Frame)
- Implemented `SpatialIndex` struct with grid-based indexing using Szudzik pairing and zigzag encoding
- Added `DBSCAN()` algorithm with configurable eps (0.6m default) and minPts (12 default)
- Implemented `computeClusterMetrics()` for centroid, bounding box, height P95, intensity mean
- Added `WorldCluster` struct with all required features
- Comprehensive unit tests in `internal/lidar/clustering_test.go`

### Phase 3.2: Kalman Tracking (World Frame)
- Implemented `TrackState` lifecycle: Tentative → Confirmed → Deleted
- Added `TrackedObject` struct with Kalman state (x, y, vx, vy) and covariance matrix
- Implemented `Tracker` with configurable parameters via `TrackerConfig`
- Added Mahalanobis distance gating for cluster-to-track association
- Implemented Kalman predict/update with constant velocity model
- Track lifecycle management with hits/misses counting, promotion, and deletion
- Speed statistics: average, peak, and history for percentile computation
- Comprehensive unit tests in `internal/lidar/tracking_test.go`

### ML Training Data Support
- Added `ForegroundFrame` struct for exporting foreground points with metadata
- Implemented `EncodeForegroundBlob()`/`DecodeForegroundBlob()` for compact binary encoding (8 bytes/point)
- Added `ValidatePose()` for pose quality assessment based on RMSE thresholds
- Implemented `TransformToWorldWithValidation()` with quality gating
- Added `TrainingDataFilter` for filtering by pose quality
- Unit tests in `internal/lidar/training_data_test.go` and `internal/lidar/pose_test.go`

### Documentation Updates
- Updated `foreground_tracking_plan.md` with implementation status and file locations
- Updated `lidar_sidecar_overview.md` with completed phases and module structure
- Added implementation files table to roadmap
- Updated milestones to reflect completed phases

## November 1, 2025 - PCAP Security & Grid Visualization (dd/lidar/read-pcap)

- Implemented path traversal protection with `--lidar-pcap-dir` flag (default: `../sensor-data/lidar`) using `filepath.Join()` + `filepath.Abs()` + prefix checking to prevent `../../` attacks.
- File validation: regular files only, `.pcap`/`.pcapng` extensions required, returns 403 Forbidden for path escape attempts.
- Systemd integration: service auto-creates PCAP directory on startup via `ExecStartPre` directive.
- Enhanced 4K-optimized dashboard (25.6×14.4" @ 150 DPI): 3 polar/spatial charts (top 50%) + 4 stacked metric panels (bottom 50%).
- Chart layout improvements: settle rate left, selected metric middle, optimized spacing (hspace=0.15), title repositioned to top right.
- PCAP snapshot mode: periodic captures with configurable interval/duration, auto-numbered output directories, metadata tracking (JSON).
- Live snapshot mode for continuous monitoring of grid state during operation.
- API helper scripts: grid reset, PCAP replay, background status fetching with sensor_id support.
- Makefile targets for noise sweep/multisweep plotting and visualization workflows.
- Python plotting tools: polar/cartesian heatmaps with live and PCAP replay modes, JSON body support for requests.
- Fixed Python import errors to only display when running as `__main__` (prevents noise during imports).
- Removed duplicate colorbar labels and unused variables in visualization code.
- Moved grid-heatmap files to project structure for better organization.
- Consolidated DEBUG-LOGGING-PLAN, GRID-ANALYSIS-PLAN, GRID-HEATMAP-API, LIDAR-PCAP-Debug docs into sidecar overview.
- Added Grid Analysis, Debugging & Diagnostics, and Security sections to overview.
- Updated Phase 2 status to COMPLETED with visualization and security features.

## October 31, 2025 - Grid Analysis API & Debug Logging

- Added `GET /api/lidar/grid_heatmap` endpoint for spatial bucket aggregation (40 rings × 120 azimuth buckets).
- Implemented `GetGridHeatmap()` method with configurable bucket size and settled threshold.
- Response includes summary stats and per-bucket metrics: fill/settle rates, mean range/times seen, frozen cells.
- Unit tests for GetGridHeatmap method validating grid metrics aggregation.
- Comprehensive API documentation for grid heatmap endpoint design and usage.
- Python plotting tools: polar (ring vs azimuth) and cartesian (X-Y) heatmaps with multiple metrics.
- Noise analysis scripts: `plot_noise_sweep.py`, `plot_noise_buckets.py` for acceptance rate visualization.
- Data analysis scripts for convergence patterns: noise vs distance, neighbor/closeness parameters.
- Comprehensive logging plan: grid reset timing, API call logs, rate-limited population tracking.
- FrameBuilder diagnostics: eviction logging, frame callback (debug mode only), enhanced azimuth wrap detection for large negative jumps (>180°).
- BackgroundManager diagnostics: acceptance decision logging, nonzero cell tracking in snapshots, per-frame acceptance summary with active parameters.
- Re-enabled `SeedFromFirstObservation` with `--lidar-seed-from-first` flag for PCAP mode.
- Added settle time flag for grid stabilization after parameter changes.
- Configurable background flush interval and frame buffer timeout.
- Sweep tools: fetch live nonzero counts from grid_status API (avoids DB timing races), multisweep tracking.
- Makefile improvements: dev-go, log-go-tail, log-go-cat targets with process management.
- Added dev-go-pcap target and streamlined lidar options.
- PCAP replay mutex for state management to prevent concurrent replays.
- Fixed frame eviction callback delivery bug (frames were discarded without invoking callback).
- Improved azimuth detection for large negative jumps to catch more wrap cases.
- Debug verbosity: frame-completion logs only when --debug flag set.

## October 30, 2025 - PCAP Debugging & Development Tools

- Enhanced frame eviction logging and finalized frame callback delivery path.
- Added diagnostics for non-zero channel counts in ParsePacket function.
- Improved azimuth wrap detection to handle edge cases (large negative jumps).
- Enable debug logging for frame completion and PCAP parsing via --debug flag.
- Updated lidar PCAP debug plan with findings and recommendations.
- Added local API helper scripts for PCAP replay and lidar background status fetching.
- Enhanced scripts to include sensor_id in grid status and snapshots requests.
- Consolidated dev-go logic into reusable run_dev_go function in Makefile.
- Added dev-go-pcap target for PCAP mode development workflow.
- Added log-go-cat and log-go-tail targets for log management.
- Enhanced dev-go to stop previously running app-radar-local processes before starting.
- Corrected log directory name in .gitignore (logs/ instead of log/).
- Moved lidar debug documentation to proper location in docs structure.

## October 29, 2025 - Configuration & Documentation Cleanup

- Updated lidar configuration flags for improved clarity and consistency.
- Enhanced documentation for database path and command flags.
- Added `SeedFromFirstObservation` parameter for PCAP mode background initialization.
- Updated README and documentation for improved clarity on database path usage.
- Removed outdated Frontend Units Override Feature documentation.
- Improved consistency across configuration documentation.

## October 28, 2025 - PCAP Support Foundation

- Added PCAP file replay support with BPF filtering for multi-sensor PCAP files.
- Integration with existing parser and frame builder for seamless replay.
- Background persistence during PCAP replay with configurable flush intervals.
- Added `--lidar-pcap-mode` flag to disable UDP listening for replay-only mode.
- POST `/api/lidar/pcap/start` endpoint for triggering PCAP replay via API.
- Enhanced LiDAR sidecar overview with classification, filtering, and metrics implementation details.
- Updated overview to reflect current status and PCAP parameter tuning phase (Phase 2.5).
- Removed outdated architecture diagram from LiDAR overview.
- Documentation nits and minor fixes throughout.

## September 23, 2025 - Background diagnostics, monitor APIs & bg-sweep

- Centralized runtime diagnostics: added `internal/monitoring` logger and per-manager `EnableDiagnostics` flag.
- BackgroundManager helpers: `SetNoiseRelativeFraction`, `SetEnableDiagnostics`, `GetAcceptanceMetrics`, `ResetAcceptanceMetrics`, `GridStatus`, `ResetGrid` for safe runtime control.
- Monitor API additions: `GET/POST /api/lidar/params`, `GET /api/lidar/acceptance`, `POST /api/lidar/acceptance/reset`, `GET /api/lidar/grid_status`, `POST /api/lidar/grid_reset`.
- New CLI `cmd/bg-sweep`: incremental & settle modes, per-noise grid reset, live bucket discovery, per-bucket CSV expansion, fixed-point numeric formatting (no scientific notation), and `--output` CSV file support.
- Experimental note: acceptance rates are affected by in-memory grid state (TimesSeenCount, frozen cells); use `grid_reset` between steps for reproducible comparisons.

## September 22, 2025 - Background model fixes, snapshot export & backfill

- Wired the `BackgroundManager` into the LiDAR pipeline and made snapshots self-contained by
  persisting per-ring elevation angles (`ring_elevations_json`) with each `lidar_bg_snapshot`.
- Centralized snapshot-to-ASC export so exports prefer snapshot-embedded elevations (fallbacks: caller-supplied, then live manager) and added frame-export fallbacks that recompute Z from polar values when needed.
- Added a small CLI backfill tool to populate `ring_elevations_json` for existing snapshots (used embedded Pandar40P config to backfill many rows).
- Small algorithmic improvements to `ProcessFramePolar` to reduce outward drift:
  - restrict neighbor confirmation to same-ring neighbors (avoid cross-ring elevation leakage),
  - update spread EMA relative to the previous mean (reduces alpha-related bias).
- Added unit tests: export behavior (ensures exported Z is correct when elevations are available) and backfill DB tests; fixed concurrent SQLite update pattern (read candidates first, then write) to avoid SQLITE_BUSY.
- Cleaned up debugging prints and standardized CLI logging in the backfill tool; left data-export writes unchanged.

## September 21, 2025 - Server & SerialMux consolidation, DB unification, tests

- Centralized HTTP server and UI paths into `internal/api` (moved server code out of `cmd/radar`).
- Standardized on a single SQLite DB (`sensor_data.db`) and consolidated DB helpers in `internal/db`.
- LiDAR background snapshots persisted to the DB (insert/get snapshot API) and a manual HTTP persist trigger added for on-demand snapshots.
- Added `--disable-radar` flag and a robust `DisabledSerialMux` that no-ops serial I/O but deterministically closes subscriber channels on Unsubscribe/Close (fixes tight-loop log spam when running without hardware).
- Merged duplicate LiDAR webservers; canonical monitor now accepts an injected `*db.DB` and `SensorID` (wired from CLI) so the same DB is used everywhere.
- Moved radar event handlers into `internal/serialmux/handlers.go` and separated classification logic into `internal/serialmux/parse.go` (small, testable `ClassifyPayload`).
- Added unit tests for `serialmux` (DisabledSerialMux behavior, classification, config parsing, event handlers) and ensured the test suite passes.
- Removed several unnecessary import aliases and normalized imports across packages.

## September 20, 2025 - Snapshot & persistence improvements

- Hardened BackgroundGrid persistence: added RW-mutexes and copy-under-read so serial/frame processing isn't blocked during snapshot serialization; metadata updates now occur under write lock.
- Added DB access for snapshots and monitor inspection: implemented a GetLatestBgSnapshot helper and a monitor endpoint to fetch, gunzip/gob-decode and summarize stored background snapshots (includes sample cells and blob hex prefix).
- Moved manual persist endpoint into the lidar monitor webserver: the handler constructs a minimal BgSnapshot and invokes the BackgroundManager PersistCallback to persist on-demand.

## September 19, 2025 - BackgroundManager, snapshot plumbing, and polar processing

- Introduced a BackgroundManager registry and constructor (`NewBackgroundManager`) to own a sensor's `BackgroundGrid`, timers, and a persistence callback. Managers are discoverable via `GetBackgroundManager`/`RegisterBackgroundManager` so APIs can trigger on-demand snapshots.
- Added serialization for background snapshots (gob + gzip) and a `Persist` method that creates `BgSnapshot` records and writes them via a `BgStore` interface (implemented by the lidar DB). This wires up snapshot persistence into the codebase.
- Implemented `InsertBgSnapshot` in the lidar DB layer to persist `lidar_bg_snapshot` rows and return snapshot IDs.
- Expanded background test coverage (`more bg tests`) to exercise snapshotting behaviors and grid processing invariants.
- Processed polar frames into the BackgroundGrid (`ProcessFramePolar`): bin points by ring/azimuth, update EMA-based averages and spreads, apply neighbor-confirmation and freezing heuristics, and count changed cells for snapshot heuristics.

## September 18, 2025 - Polar-first refactor: parser & frame builder

- Centralized spherical→Cartesian math into a small `transform.go` helper and introduced `PointPolar`.
- Parser now emits `PointPolar` (polar-first) and the UDP listener forwards polar points directly when possible.
- Added `FrameBuilder.AddPointsPolar([]PointPolar)` and removed the legacy `AddPoints([]Point)` API; tests and integration updated.
- Result: `internal/lidar` tests (unit + PCAP integration) pass after the migration.

## September 17, 2025 - Background model & transform refactor

- Sensor-frame background model (ring × azimuth) for foreground masking.
- Two-level settling per cell (fast noise settling, slow parked-object settling).
- Persist BackgroundGrid snapshots and warm-start on load.
- Refactor spherical→Cartesian into a small transform helper and split polar/cartesian point types.
- World-grid (height-map / ground estimate) on masked Cartesian points for semantic ops.
- Next: add transform helpers, update point types, implement sensor-frame ProcessFrame and snapshot tests.

## September 13, 2025 - Test Code Maintainability & Optimization

### Parse Test Improvements

- **Eliminated implementation dependencies**: Replaced external constants (CHANNELS*PER_BLOCK, PACKET_SIZE*\*) with local test constants
- **Enhanced maintainability**: Tests now self-contained and independent of implementation changes
- **Fixed boundary conditions**: Corrected loop bounds in PCAP extraction to include valid edge cases
- **Removed redundant checks**: Eliminated unnecessary bounds checking in packet extraction logic
- **Performance optimization**: Streamlined extractUDPPayloads function by removing redundant conditional checks

### Technical Changes

- **Local test constants**: Added testChannelsPerBlock, testPacketSizeStandard, etc. for test isolation
- **Boundary fix**: Changed `i < len(data)-testPacketSizeStandard` to `i <= len(data)-testPacketSizeStandard`
- **Logic optimization**: Removed redundant `if i+testPacketSizeStandard <= len(data)` check
- **Code clarity**: Added explanatory comments for optimization decisions

## September 12, 2025 - Frame Builder Test Suite Fixes & Validation

### Test Suite Completion

- **All frame builder tests passing**: Fixed 3 previously failing tests using realistic production data patterns
- **Integration test relocation**: Moved PCAP integration test from `cmd/pcap-test/` to `internal/lidar/integration_test.go`
- **Test data organization**: Created `internal/lidar/testdata/` directory following Go conventions
- **Data volume upgrade**: Increased test point counts from ~10,680 to 60,000 points (matching successful PCAP integration test)
- **Production-level validation**: Tests now use MinFramePointsForCompletion = 10,000 threshold with realistic coverage
- **Time-based detection validation**: Confirmed hybrid detection with motor speed adaptation and azimuth wrap fallback
- **Configuration completeness**: Added BufferTimeout and CleanupInterval settings for proper async frame processing

### Fixed Test Cases

- **TestFrameBuilder_TraditionalAzimuthOnly**: Traditional azimuth-only detection (350° → 10°) with 60,000 points
- **TestFrameBuilder_HybridDetection**: Time-based detection with azimuth validation and realistic timing
- **TestFrameBuilder_AzimuthWrapWithTimeBased**: Azimuth wrap in time-based mode with proper configuration

### Test Pattern Analysis

- **Successful data patterns**: 0°-356° azimuth coverage with wrap at 356°→5° triggers completion
- **Timing validation**: ~60ms frame duration matches production expectations (600 RPM motor speed)
- **Point distribution**: Even azimuth distribution across 60,000 points provides adequate coverage
- **Configuration requirements**: BufferTimeout=100ms, CleanupInterval=50ms essential for async processing

## September 12, 2025 - Time-Based Frame Detection & Documentation

### Time-Based Frame Detection Implementation

- **Hybrid frame detection**: Time-based primary trigger with azimuth validation for anomaly prevention
- **Motor speed integration**: Real-time motor speed extraction from packet tail (bytes 8-9)
- **Frame timing adaptation**: Dynamic frame duration based on actual RPM (50ms at 1200 RPM, 100ms at 600 RPM)
- **Azimuth safety checks**: Requires 270° coverage before time-based completion to prevent timing glitches
- **Azimuth wrap secondary**: Respects azimuth wraps (340° → 20°) with minimum half-duration timing constraint
- **Traditional fallback**: Pure azimuth-based detection (350° → 10°) when time-based disabled
- **Motor speed caching**: Parser stores last motor speed for frame builder integration
- **Testing validation**: Confirmed proper frame duration changes during RPM transitions (600→1200→600)

### Code Documentation Enhancement

- **Comment verbosity upgrade**: Comprehensive documentation updates in extract.go
- **Packet structure details**: Complete 22-byte tail parsing documentation with all fields
- **Timestamp mode documentation**: Added detailed explanations for all 5 supported modes
- **Calibration explanations**: Enhanced comments for coordinate transformations and corrections
- **Performance optimization notes**: Documented trigonometric optimizations and memory allocations

### Technical Improvements

- **CLI configurability**: Added --sensor-name flag for flexible deployment scenarios
- **Real-time adaptation**: Frame builder now responds immediately to motor speed changes
- **Accurate timing**: Eliminated hardcoded 600 RPM assumption, uses actual motor speed throughout
- **UDP sequence validation**: Confirmed proper handling of optional 4-byte UDP sequence suffix

## September 11, 2025 - Memory Optimization & Frame Rate Fixes

### Packet Structure Analysis

- **Wireshark investigation**: Analyzed Hesai Pandar40P UDP packet structure
- **Discovered Ethernet tail issue**: Extra 4 bytes appended to UDP packets
- **Tail composition**: 2-byte sequence + 2-byte unknown data (0x00 0x00)
- **Parser fix**: Updated tail offset from last 6 bytes to last 10 bytes
- **Validation**: Confirmed correct UDP sequence extraction and point parsing

### Performance Validation

- **Proper frame characteristics**: ~69,000 points per frame, ~100ms duration
- **Correct LiDAR operation**: Full 360° rotations with expected Hesai Pandar40P output
- **Debug logging**: Added temporary logging to diagnose, then removed for production

### Technical Discoveries

- **Ethernet vs UDP parsing**: Raw UDP data includes Ethernet layer artifacts
- **Tail offset critical**: Incorrect offset leads to malformed sequence numbers
- Frame builder processes points individually, not in packets
- Individual UDP packets contain only 2-3 points with small azimuth ranges
- Azimuth wrap detection must account for accumulated vs instantaneous coverage
- Point-level frame detection requires stricter criteria than packet-level detection
