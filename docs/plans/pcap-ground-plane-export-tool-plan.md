# Ground plane export for pcap-analyse tool

- **Status:** Planning
- **Target:** [internal/lidar/lidarbench](../../internal/lidar/lidarbench)
- **Canonical:** [pcap-analysis-mode.md](../lidar/operations/pcap-analysis-mode.md)
- **Related:**

- [docs/lidar/architecture/ground-plane-extraction.md](../lidar/architecture/ground-plane-extraction.md)
- [docs/lidar/architecture/gps-ethernet-parsing.md](../lidar/architecture/gps-ethernet-parsing.md)
- [docs/lidar/architecture/geographic-indexing.md](../lidar/architecture/geographic-indexing.md)
- [data/maths/ground-plane-maths.md](../../data/maths/ground-plane-maths.md)

## Objective

Extend the existing `pcap-analyse` command-line tool to compute and export ground plane geometry from static PCAP captures, with **optional** GPS geo-referencing. This enables:

1. **Road surface reconstruction**: Export accurate road geometry for civil engineering analysis
2. **GIS integration**: Generate geo-referenced ground plane data compatible with mapping tools (GPS additive)
3. **Offline processing**: Extract ground plane from archived PCAP files without real-time replay
4. **Quality assurance**: Validate sensor placement and ground plane extraction algorithms
5. **Global dataset population**: Merge settled tiles into persistent WGS84 ground data partitioned by canonical S2 L13 cells, with L10 for coarse filesystem grouping (GPS additive)

**Sensor-iterative principle:** Ground plane extraction **must work with LiDAR data alone**. GPS flags are strictly optional and only enable geographic export formats and Tier 2 global dataset population. The core extraction pipeline operates in sensor-local coordinates.

The ground plane extraction reuses the existing L1→L2→L3 background grid
pipeline, with ground plane fitting within L4 Perception. Optional GPS supplies
a precise WGS84 geometry origin; every geographic identity and partition is a
canonical S2 cell.

## Background

The current `pcap-analyse` tool ([internal/lidar/lidarbench/lidarbench.go](../../internal/lidar/lidarbench/lidarbench.go), ~53 KB) processes PCAP files through the full L1→L2→L3→L4→L5→L6 pipeline and exports:

- CSV tracks (vehicle trajectories)
- JSON results (detection summary)
- Foreground research blobs for offline benchmarks and experiments

Existing export infrastructure:

- `ExportBackgroundGridToASC()` in [internal/lidar/l3grid/export_bg_snapshot.go](../../internal/lidar/l3grid/export_bg_snapshot.go): ASC format for CloudCompare
- Web API endpoints: `/api/lidar/export/frame-sequence-asc`, `handleExportSnapshotASC`
- VTK export recommended in [docs/lidar/architecture/lidar-background-grid-standards.md](../lidar/architecture/lidar-background-grid-standards.md)

GPS support exists but is unused:

- L1 parser extracts GPS timestamps from PCAP
- Site config has a WGS84 origin but no canonical S2 geographic index
- No coordinate transformation or geo-referencing currently implemented

## New CLI flags

Add the following flags to [internal/lidar/lidarbench/lidarbench.go](../../internal/lidar/lidarbench/lidarbench.go):

### Ground plane extraction

```
--ground-plane          Enable ground plane extraction and export (default: false)
--ground-plane-format   Export format: "geojson", "asc", "vtk", "csv" (default: "geojson")
--ground-tile-size      Tile size in metres (default: 1.0)
--ground-range-max      Maximum range for ground plane tiles in metres (default: 50.0)
--ground-confidence-min Minimum confidence score for exported tiles (0.0-1.0, default: 0.5)
```

### GPS geo-referencing (optional: additive only)

```
--wgs84-origin <east_west_deg,north_south_deg[,elevation_msl_m]>
                        Manual precise WGS84 geometry origin
--gps-heading           Sensor heading in degrees clockwise from true north (required unless derived from NMEA course-over-ground)
--gps-from-pcap         Extract the WGS84 fix from PCAP packets (default: false)
--s2-dataset-merge      Merge settled tiles into the Tier 2 S2 dataset (default: false)
--s2-dataset-root       Root of the canonical L10/L13 partition tree (default: "")
```

### Flag validation rules

- If `--ground-plane` is false, all other ground plane flags are ignored
- `--ground-plane-format` accepts multiple comma-separated values: `--ground-plane-format geojson,csv,vtk`
- **GPS flags are strictly optional.** If no GPS source is available, export in local Cartesian coordinates (sensor at origin). All core extraction works without GPS.
- If `--gps-from-pcap` is true and no GPS packets are found, fall back to the manual WGS84 origin
- `--wgs84-origin` validates the signed east/west component in the range −180° to 180°, the signed north/south component in the range −90° to 90°, and optional MSL elevation in metres
- If neither GPS source is available, GeoJSON export uses `coordinate_system: "Sensor-XY"` (local metres)
- `--ground-confidence-min` filters tiles below threshold from all exports
- `--s2-dataset-merge` requires either `--wgs84-origin` or `--gps-from-pcap`; the tool derives and validates canonical L13 before writing any global record

## Processing pipeline extension

The ground plane extraction integrates into the existing PCAP analysis pipeline as follows:

### Phase 1: existing pipeline (unchanged)

1. **L1**: Parse PCAP packets, decode LiDAR frames, extract GPS timestamps
2. **L2**: Convert spherical coordinates to Cartesian (sensor-local frame), apply sensor corrections
3. **L3**: Accumulate background grid, settle static points, classify foreground/background

### Phase 2: ground plane extraction (new; within L4 perception)

4. **Ground Classification**: After L3 grid settling (typically 5-10 seconds):
   - Classify ground cells using height-based threshold (Z < -1.8m from sensor)
   - Apply spatial coherence filter (ground cells must be contiguous)
   - Mark ground cells in background grid metadata

5. **Tile Accumulation** (Tier 1 local scene, sensor-local coordinates: no GPS required):
   - Map XYZ point to tile coordinates (tile_x, tile_y) based on `--ground-tile-size`
   - Accumulate incremental covariance for plane fitting (μ, Σ)
   - Track point count, height statistics, first/last observation timestamps

6. **Continue Processing**: Foreground detection (L4 clustering → L5 → L6) proceeds as normal for vehicle tracking

### Phase 3: plane fitting and export (new)

7. **Final Plane Fitting**: After PCAP replay completes:
   - For each tile with sufficient points (≥10), fit plane using SVD on covariance matrix
   - Compute confidence score: `conf = 1 - (λ_min / λ_max)` where λ are eigenvalues
   - Classify curvature: flat (λ_min < 0.01), cambered (0.01 ≤ λ_min < 0.05), rough (≥ 0.05)
   - Filter tiles below `--ground-confidence-min` threshold

8. **GPS Transformation** (optional: only if a WGS84 fix is available):
   - Construct ENU (East-North-Up) coordinate frame at GPS origin
   - Transform tile corners from sensor Cartesian to ENU to WGS84
   - Rotate plane normals by sensor heading
   - Calculate canonical S2 L13 from the resulting WGS84 geometry

9. **Global Dataset Merge** (optional: only if `--s2-dataset-merge` and GPS available):
   - Convert the WGS84 position to a canonical S2 L13 CellID/token using the [canonical cell-positioning model](../lidar/architecture/geographic-indexing.md#how-s2-positions-and-numbers-cells)
   - Derive its L10 parent with `CellID.Parent(10)`, never token truncation
   - Load applicable L13 records beneath `--s2-dataset-root/<l10>/`
   - Diff settled local tiles against global ground geometry
   - Merge consistent tiles; flag divergent tiles for review
   - Write canonical-token paths back beneath the L10 directory

10. **Export to Formats**: Write files to output directory (see Output Structure)

## Export formats

### GeoJSON (default, priority 1)

**Use Case**: GIS tools (QGIS, ArcGIS), web mapping (Leaflet, Mapbox), geospatial analysis

**Format**:

A GeoJSON `FeatureCollection` with top-level `metadata` (sensor model, capture timestamp, WGS84 origin, canonical S2 L13 token, tile size, range, confidence threshold, and geometry reference) and one `Feature` per tile. Each feature is a `Polygon` geometry (closed ring of four corners) with properties:

| Property             | Type     | Example               | Notes                               |
| -------------------- | -------- | --------------------- | ----------------------------------- |
| `tile_x`             | int      | 10                    | Grid column index                   |
| `tile_y`             | int      | 5                     | Grid row index                      |
| `plane_normal`       | float[3] | [0.02, −0.01, 0.9998] | Unit normal `[a, b, c]`             |
| `plane_offset`       | float    | −1.85                 | Plane `d` in `ax + by + cz + d = 0` |
| `confidence`         | float    | 0.95                  | Fit confidence 0–1                  |
| `curvature_class`    | string   | "flat"                | Classification label                |
| `curvature_deg`      | float    | 1.2                   | Surface curvature in degrees        |
| `point_count`        | int      | 847                   | Points in tile                      |
| `mean_height`        | float    | −1.85                 | Mean ground height (m)              |
| `height_std_dev`     | float    | 0.03                  | Height standard deviation (m)       |
| `settlement_time_ms` | int      | 2340                  | Time to converge (ms)               |

**Implementation Notes**:

- Polygon coordinates must close (first point == last point) per GeoJSON spec (RFC 7946)
- If no WGS84 fix is available, use local Cartesian metres with `coordinate_system: "Sensor-XY"` and omit S2 identity
- Plane equation: `ax + by + cz + d = 0` where `[a,b,c]` is `plane_normal`, `d` is `plane_offset`

### ASC (cloudCompare compatible, priority 2)

**Use Case**: Existing CloudCompare workflow, 3D point cloud visualisation

**Format**:

```
ncols 100
nrows 100
xllcorner 0.0
yllcorner 0.0
cellsize 1.0
NODATA_value -9999
<z_00> <z_01> ... <z_0n>
<z_10> <z_11> ... <z_1n>
...
```

**Implementation Notes**:

- Reuse existing `ExportBackgroundGridToASC()` from [internal/lidar/l3grid/export_bg_snapshot.go](../../internal/lidar/l3grid/export_bg_snapshot.go)
- Z values are fitted plane heights, not raw point heights
- Tiles below confidence threshold written as `NODATA_value`
- If a WGS84 fix is available, use ENU X/Y for xllcorner/yllcorner (metres from the GPS origin)

### CSV (simple tabular, priority 2)

**Use Case**: Spreadsheet analysis, data science, custom processing

**Format**:

One header row followed by one row per tile. Columns:

| Column               | Example    | Notes                                                                  |
| -------------------- | ---------- | ---------------------------------------------------------------------- |
| `tile_x`             | 10         | Grid column                                                            |
| `tile_y`             | 5          | Grid row                                                               |
| `s2_l13_token`       | "80858004" | Canonical fine geographic partition (hex string; treat as text)        |
| `s2_l10_token`       | "808581"   | Canonical parent derived with `Parent(10)` (hex string; treat as text) |
| `plane_a`            | 0.02       | Normal x                                                               |
| `plane_b`            | −0.01      | Normal y                                                               |
| `plane_c`            | 0.9998     | Normal z                                                               |
| `plane_d`            | −1.85      | Offset (3 decimals)                                                    |
| `confidence`         | 0.95       | Fit confidence 0–1                                                     |
| `curvature_class`    | flat       | Classification label                                                   |
| `curvature_deg`      | 1.2        | Degrees                                                                |
| `point_count`        | 847        | Points in tile                                                         |
| `mean_height`        | −1.85      | Metres (3 decimals)                                                    |
| `height_std_dev`     | 0.03       | Metres                                                                 |
| `settlement_time_ms` | 2340       | Convergence time                                                       |

**Implementation Notes**:

- If no WGS84 fix is available, omit both S2 token columns
- Plane equation: `ax + by + cz + d = 0`
- Numeric geometry values use format-appropriate precision; heights use 3 decimals

### VTK (paraView, priority 3)

**Use Case**: 3D scientific visualisation, advanced analysis in ParaView/LidarView

**Format**: VTK StructuredGrid with scalar fields

A VTK `StructuredGrid` file (XML format, version 1.0, little-endian) with extent matching the tile grid dimensions. Contains a single `Piece` with:

- **Points**: `Float32` array with 3 components (x, y, z coordinates per tile)
- **PointData** scalar fields: `confidence` (Float32), `curvature_deg` (Float32), `point_count` (Int32)

**Implementation Notes**:

- Defer to Phase 4: requires VTK library or manual XML generation
- Recommended library: `github.com/lanl/vpic-utils/vtk` or custom XML writer
- Coordinate system: ENU if GPS available, else sensor-relative Cartesian

## GPS integration

### Coordinate fallback chain

1. **PCAP GPS** (if `--gps-from-pcap` enabled):
   - Parse GPS ethernet packets using [docs/lidar/architecture/gps-ethernet-parsing.md](../lidar/architecture/gps-ethernet-parsing.md) spec
   - Extract first valid GNGGA or GNRMC sentence with 3D fix
   - Use the precise WGS84 fix from GPS, derive canonical L13, and use heading from `--gps-heading` or NMEA course-over-ground

2. **Manual CLI Flags** (if PCAP GPS unavailable or disabled):
   - Use `--wgs84-origin` and `--gps-heading`
   - Validate the WGS84 fix before deriving canonical L13
   - Default elevation 0.0m MSL and heading 0.0° (north) only when the operator explicitly accepts those assumptions

3. **No Geo-Referencing** (if neither source available):
   - Export in local sensor-relative Cartesian coordinates (X: forward, Y: left, Z: up)
   - Set `coordinate_system: "Sensor-XY"` in GeoJSON metadata
   - Omit S2 identity from CSV and use tile_x/tile_y only

### Coordinate transformation

- **Local Cartesian → ENU**: Translate origin to GPS point, rotate by heading
- **ENU → WGS84**: Use `github.com/wroge/wgs84` library for geodetic conversion
- **Heading Convention**: Degrees clockwise from true north (0° = north, 90° = east)

### GPS metadata in exports

All formats include GPS origin in metadata/header:

- GeoJSON: `metadata.wgs84_origin` object
- ASC: sidecar metadata records the WGS84 origin, canonical L13/L10 tokens, elevation, and heading
- CSV: Separate `ground-plane-meta.json` sidecar file
- VTK: `<FieldData>` with GPS parameters

## Output structure

Files are written to the existing output directory structure:

```
output/<run-id>/
├── tracks.csv                   # Existing: Vehicle trajectories
├── results.json                 # Existing: Detection summary
├── ground-plane.geojson         # New: Ground plane tiles (GeoJSON)
├── ground-plane.csv             # New: Ground plane tiles (CSV)
├── ground-plane.asc             # New: Ground plane tiles (ASC)
├── ground-plane.vtk             # New: Ground plane tiles (VTK)
├── ground-plane-meta.json       # New: Extraction metadata (always)
└── training/                    # Existing: Training data (if --training)
    └── ...
```

**Global dataset** (if `--s2-dataset-merge`): Written beneath
`--s2-dataset-root` outside the per-run output directory. Machine-readable
paths use canonical tokens, for example
`<root>/808581/80858004.geojson`; family displays never appear in paths. An L13
boundary-crossing policy (split geometry or multi-index it) must be chosen
before implementation.

**Naming Convention**: `ground-plane.<format>` for main export files

**Metadata File** (`ground-plane-meta.json`): Always written when `--ground-plane` enabled:

The metadata file (`ground-plane-meta.json`) records extraction parameters and results:

| Field                   | Type        | Example                   | Notes                                                 |
| ----------------------- | ----------- | ------------------------- | ----------------------------------------------------- |
| `extraction_timestamp`  | string      | "2026-01-15T10:45:23Z"    | ISO 8601                                              |
| `pcap_file`             | string      | "capture-2026-01-15.pcap" | Source PCAP                                           |
| `sensor_model`          | string      | "Hesai Pandar40P"         | Detected sensor                                       |
| `coordinate_system`     | string      | "Sensor-XY"               | "WGS84" when GPS available                            |
| `gps_source`            | string      | "none"                    | "manual" or "pcap" when available                     |
| `wgs84_origin`          | object/null | null                      | Precise geometry origin when GPS is available         |
| `s2_l13_token`          | string/null | "80858004"                | Canonical fine partition when GPS is available        |
| `s2_l10_token`          | string/null | "808581"                  | Canonical parent derived with `Parent(10)`            |
| `tile_size_m`           | float       | 1.0                       | Grid resolution                                       |
| `range_max_m`           | float       | 50.0                      | Max range filter                                      |
| `confidence_min`        | float       | 0.5                       | Min confidence threshold                              |
| `total_tiles`           | int         | 847                       | Tiles computed                                        |
| `exported_tiles`        | int         | 791                       | Tiles above threshold                                 |
| `filtered_tiles`        | int         | 56                        | Tiles below threshold                                 |
| `processing_time_s`     | float       | 12.4                      | Wall clock time                                       |
| `formats`               | string[]    | ["csv", "asc"]            | Output formats written                                |
| `global_dataset_merged` | bool        | false                     | Whether merged into the global S2-partitioned dataset |

When GPS is available, `coordinate_system` becomes `"WGS84"`, `gps_source` becomes `"manual"` or `"pcap"`, and `wgs84_origin` is populated.

## Implementation phases

### Phase 1: core ground plane extraction (no GPS)

**Goal**: Extract and fit ground plane tiles from PCAP in local coordinates

**Tasks**:

1. Add `--ground-plane` flag to enable extraction
2. Implement ground cell classification in L3 grid (height threshold + spatial filter)
3. Implement tile accumulation with incremental covariance (in-memory hashmap)
4. Implement plane fitting using SVD (Eigen decomposition)
5. Implement confidence scoring and filtering
6. Unit tests: tile fitting, covariance accumulation, confidence calculation

**Deliverable**: Ground plane tiles fitted in local Cartesian coordinates, no export yet

**Testing**: Integration test with existing test PCAP files, validate plane normals and confidence scores

### Phase 2: CLI flags and CSV/ASC export

**Goal**: Add command-line interface and basic export formats

**Tasks**:

1. Add all ground plane CLI flags (format, tile size, range, confidence)
2. Implement CSV export with all tile properties
3. Implement ASC export (reuse `ExportBackgroundGridToASC` with fitted Z values)
4. Implement `ground-plane-meta.json` metadata file generation
5. Update output directory structure
6. Integration tests: validate CSV schema, ASC header format

**Deliverable**: `pcap-analyse --ground-plane --ground-plane-format csv,asc` produces valid exports

**Testing**: Export format validation, regression testing (existing exports unchanged)

### Phase 3: GPS geo-referencing, S2 partitioning, and GeoJSON export

**Goal**: Add GPS coordinate transformation, canonical geographic partitioning, and primary export format

**Tasks**:

1. Add GPS CLI flags (`--wgs84-origin`, `--gps-heading`)
2. Implement `--gps-from-pcap` flag and NMEA sentence parsing
3. Implement coordinate transformation: Cartesian → ENU → WGS84
4. Introduce the shared S2 utility; convert WGS84 → L13 and derive L10 with `CellID.Parent(10)`
5. Decide the canonical SQLite/JSON L13 representation, including signed-`INTEGER`/`uint64` trade-offs and migration/index requirements
6. Implement the canonical L10/L13 filesystem layout and GeoJSON export with geo-referenced tile polygons
7. Add the explicitly L10/L13-aware family-display formatter for human-facing text and a separate UI scan-cue renderer. The cue is layout only: it must add no copyable character and must stay out of accessibility names, storage, and hierarchy operations
8. Update CSV/ASC exports to include GPS and canonical S2 metadata
9. Add known-vector, hierarchy, level-marker, boundary, and coordinate transformation tests, including parents that do not resemble token truncation

**Deliverable**: `pcap-analyse --ground-plane --wgs84-origin <position>` produces geo-referenced GeoJSON with canonical L13 metadata and an L10-grouped filesystem destination

**Testing**: Validate GeoJSON schema (RFC 7946), test with QGIS import, verify coordinate transformation

### Phase 4: VTK export and advanced features

**Goal**: Add VTK format for scientific visualisation, polish features

**Tasks**:

1. Implement VTK StructuredGrid export (manual XML or library)
2. Add curvature classification and per-tile statistics
3. Optimise memory usage for large PCAP files (streaming tile export)
4. Add progress reporting for long-running extractions
5. Performance benchmarking and optimisation

**Deliverable**: Full feature set with all four export formats

**Testing**: VTK validation with ParaView import, performance testing with multi-GB PCAP files

## Testing strategy

### Unit tests

- **Tile Fitting**: `internal/lidar/l4perception/ground_plane_test.go`
  - Test plane fitting with known point clouds (flat, sloped, noisy)
  - Test confidence scoring with varying eigenvalue ratios
  - Test curvature classification thresholds

- **Coordinate Transformation**: `internal/lidar/l4perception/gps_transform_test.go`
  - Test Cartesian → ENU → WGS84 round-trip accuracy (GPS additive path)
  - Test heading rotation (0°, 90°, 180°, 270°)
  - Test edge cases (poles, antimeridian)
- **S2 Geographic Indexing**: future shared geographic utility tests
  - Test known WGS84 → L13 vectors and L13 → L10 `Parent(10)` relationships
  - Test canonical-token and family-display round trips separately
  - Test that the UI scan cue contributes no character to selection, clipboard, accessibility names, search, or input values
  - Test level-marker cases where an L8 parent is not a lexical truncation of its L10 child
  - Test that no hierarchy operation consumes a token prefix or family display

### Integration tests

- **PCAP Processing**: `internal/lidar/lidarbench/ground_plane_test.go`
  - Use existing test PCAP files (e.g., `data/test-captures/*.pcap`)
  - Validate ground plane extraction produces expected tile count
  - Validate export files are created with correct names
  - Validate no regression in existing exports (tracks.csv, results.json)

- **Export Format Validation**:
  - GeoJSON: Parse with `encoding/json`, validate against RFC 7946 schema
  - CSV: Parse with `encoding/csv`, validate column headers and data types
  - ASC: Parse header, validate grid dimensions match tile count

### Manual testing

- Import GeoJSON into QGIS, verify tile positions and properties
- Open ASC in CloudCompare, verify ground plane visualisation
- Open VTK in ParaView, verify scalar field rendering
- Test GPS fallback chain with real PCAP files (with/without GPS packets)

### Regression testing

- Ensure `--ground-plane=false` (default) produces identical output to current version
- Benchmark PCAP processing time with/without ground plane extraction
- Validate memory usage does not exceed 2x baseline for large PCAP files

## Acceptance criteria

### Phase 1: core extraction

- [ ] Ground plane tiles extracted from test PCAP files
- [ ] Plane normals within 5° of expected values (for flat ground)
- [ ] Confidence scores > 0.9 for high-quality tiles, < 0.5 for noisy tiles
- [ ] Unit test coverage > 80% for tile fitting code
- [ ] No regression in existing PCAP processing (tracks.csv unchanged)

### Phase 2: CLI and basic exports

- [ ] All CLI flags documented in `--help` output
- [ ] CSV export validates against schema (all columns present)
- [ ] ASC export opens in CloudCompare without errors
- [ ] `ground-plane-meta.json` includes all required fields
- [ ] Integration test passes with 3 different PCAP files
- [ ] Export files written to correct output directory structure

### Phase 3: GPS, S2, and GeoJSON

- [ ] GPS fallback chain works: PCAP → manual → local coordinates
- [ ] GeoJSON validates against RFC 7946 schema (use `geojsonlint`)
- [ ] GeoJSON imports into QGIS with correct coordinate system (WGS84)
- [ ] Tile positions within 10cm of expected locations (for known GPS origin)
- [ ] Coordinate transformation accuracy: < 1cm error for points within 100m
- [ ] CSV and ASC exports include GPS metadata
- [ ] Geographic records use canonical L13 tokens and filesystem paths use canonical L10 parents derived with `Parent(10)`
- [ ] Known-vector and non-lexical-parent tests pass; family displays are confined to human-facing presentation and the UI scan cue is non-text
- [ ] Site, deployment, and session identities remain independent of S2 cells

### Phase 4: VTK and polish

- [ ] VTK file opens in ParaView without errors
- [ ] Scalar fields (confidence, curvature) render correctly in ParaView
- [ ] PCAP processing time increases < 20% with ground plane extraction enabled
- [ ] Memory usage < 1GB for 10-minute PCAP files
- [ ] Progress reporting shows % completion during extraction
- [ ] All four export formats tested with real PCAP files

## Non-Goals (out of scope)

- **Real-time ground plane export**: This tool is for offline PCAP analysis only
- **Multi-sensor fusion**: Single sensor per PCAP file
- **Ground plane tracking over time**: Static extraction only, no temporal analysis
- **Automatic sensor height calibration**: Use existing site config or manual GPS altitude
- **Point cloud decimation**: Export all fitted tiles above confidence threshold
- **Web UI integration**: Command-line tool only, web API extensions handled separately

## Future extensions

- **Ground plane change detection**: Compare ground plane exports from multiple captures to detect road damage or surface changes
- **Integration with web API**: Add `/api/lidar/export/ground-plane` endpoint for on-demand extraction
- **Automatic GPS from database**: Query the site WGS84 origin, derive canonical L13, and validate it against any supplied partition
- **Multi-PCAP batch processing**: Process entire directory of PCAP files with single command
- **Ground plane texture mapping**: Export surface roughness or reflectivity as additional tile properties
- **Global dataset visualisation**: Web UI for browsing canonical S2 L10/L13 ground partitions
- **OSM polyline import** (v2): Anchor ground plane tiles to kerb lines, crosswalks, and road edges from OpenStreetMap
- **OSM write-back** (v2): Propose edits to OSM with more accurate geometry from LiDAR measurements (requires OSM API key)
