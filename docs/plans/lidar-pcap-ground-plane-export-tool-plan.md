# Ground Plane Export for pcap-analyse Tool

Status: Planned
Purpose/Summary: lidar-pcap-ground-plane-export-tool-plan.

**Status**: Planning
**Target**: `cmd/tools/pcap-analyse`
**Related Design Docs**:

- `docs/lidar/architecture/ground-plane-extraction.md`
- `docs/plans/lidar-architecture-gps-ethernet-parsing-plan.md`
- `docs/maths/ground-plane-maths.md`

## Objective

Extend the existing `pcap-analyse` command-line tool to compute and export ground plane geometry from static PCAP captures, with **optional** GPS geo-referencing. This enables:

1. **Road surface reconstruction** — Export accurate road geometry for civil engineering analysis
2. **GIS integration** — Generate geo-referenced ground plane data compatible with mapping tools (GPS additive)
3. **Offline processing** — Extract ground plane from archived PCAP files without real-time replay
4. **Quality assurance** — Validate sensor placement and ground plane extraction algorithms
5. **Global grid population** — Merge settled tiles into the persistent lat/long global grid (GPS additive)

**Sensor-iterative principle:** Ground plane extraction **must work with LiDAR data alone**. GPS flags are strictly optional and only enable geographic export formats and Tier 2 global grid population. The core extraction pipeline operates in sensor-local coordinates.

The ground plane extraction reuses the existing L1→L2→L3 background grid pipeline, with ground plane fitting within L4 Perception, exporting to multiple formats with optional GPS coordinates.

## Background

The current `pcap-analyse` tool (`cmd/tools/pcap-analyse/main.go`, ~53 KB) processes PCAP files through the full L1→L2→L3→L4→L5→L6 pipeline and exports:

- CSV tracks (vehicle trajectories)
- JSON results (detection summary)
- Training data (foreground blobs for ML)

Existing export infrastructure:

- `ExportBackgroundGridToASC()` in `internal/lidar/l3grid/export_bg_snapshot.go` — ASC format for CloudCompare
- Web API endpoints: `/api/lidar/export/frame-sequence-asc`, `handleExportSnapshotASC`
- VTK export recommended in `docs/lidar/architecture/lidar-background-grid-standards.md`

GPS support exists but is unused:

- L1 parser extracts GPS timestamps from PCAP
- Site config stores lat/long in database
- No coordinate transformation or geo-referencing currently implemented

## New CLI Flags

Add the following flags to `cmd/tools/pcap-analyse/main.go`:

### Ground Plane Extraction

```
--ground-plane          Enable ground plane extraction and export (default: false)
--ground-plane-format   Export format: "geojson", "asc", "vtk", "csv" (default: "geojson")
--ground-tile-size      Tile size in metres (default: 1.0)
--ground-range-max      Maximum range for ground plane tiles in metres (default: 50.0)
--ground-confidence-min Minimum confidence score for exported tiles (0.0-1.0, default: 0.5)
```

### GPS Geo-Referencing (Optional — Additive Only)

```
--gps-lat               Manual GPS latitude for geo-referencing (decimal degrees)
--gps-lon               Manual GPS longitude for geo-referencing (decimal degrees)
--gps-alt               Manual GPS altitude MSL in metres (default: 0.0)
--gps-heading           Sensor heading in degrees clockwise from true north (default: 0.0)
--gps-from-pcap         Extract GPS coordinates from PCAP packets (default: false)
--global-grid-merge     Merge settled tiles into Tier 2 global grid file (default: false)
--global-grid-file      Path to global grid file for load/merge (default: "")
```

### Flag Validation Rules

- If `--ground-plane` is false, all other ground plane flags are ignored
- `--ground-plane-format` accepts multiple comma-separated values: `--ground-plane-format geojson,csv,vtk`
- **GPS flags are strictly optional.** If no GPS source is available, export in local Cartesian coordinates (sensor at origin). All core extraction works without GPS.
- If `--gps-from-pcap` is true and no GPS packets found, fall back to manual coordinates
- If neither GPS source is available, GeoJSON export uses `coordinate_system: "Sensor-XY"` (local metres)
- `--ground-confidence-min` filters tiles below threshold from all exports
- `--global-grid-merge` requires either `--gps-lat/--gps-lon` or `--gps-from-pcap` (GPS is needed for global grid positioning)

## Processing Pipeline Extension

The ground plane extraction integrates into the existing PCAP analysis pipeline as follows:

### Phase 1: Existing Pipeline (Unchanged)

1. **L1**: Parse PCAP packets, decode LiDAR frames, extract GPS timestamps
2. **L2**: Convert spherical coordinates to Cartesian (sensor-local frame), apply sensor corrections
3. **L3**: Accumulate background grid, settle static points, classify foreground/background

### Phase 2: Ground Plane Extraction (New — within L4 Perception)

4. **Ground Classification**: After L3 grid settling (typically 5-10 seconds):
   - Classify ground cells using height-based threshold (Z < -1.8m from sensor)
   - Apply spatial coherence filter (ground cells must be contiguous)
   - Mark ground cells in background grid metadata

5. **Tile Accumulation** (Tier 1 local scene, sensor-local coordinates — no GPS required):
   - Map XYZ point to tile coordinates (tile_x, tile_y) based on `--ground-tile-size`
   - Accumulate incremental covariance for plane fitting (μ, Σ)
   - Track point count, height statistics, first/last observation timestamps

6. **Continue Processing**: Foreground detection (L4 clustering → L5 → L6) proceeds as normal for vehicle tracking

### Phase 3: Plane Fitting and Export (New)

7. **Final Plane Fitting**: After PCAP replay completes:
   - For each tile with sufficient points (≥10), fit plane using SVD on covariance matrix
   - Compute confidence score: `conf = 1 - (λ_min / λ_max)` where λ are eigenvalues
   - Classify curvature: flat (λ_min < 0.01), cambered (0.01 ≤ λ_min < 0.05), rough (≥ 0.05)
   - Filter tiles below `--ground-confidence-min` threshold

8. **GPS Transformation** (optional — only if GPS coordinates available):
   - Construct ENU (East-North-Up) coordinate frame at GPS origin
   - Transform tile corners from sensor Cartesian to ENU to WGS84 (lat/long)
   - Rotate plane normals by sensor heading

9. **Global Grid Merge** (optional — only if `--global-grid-merge` and GPS available):
   - Load existing global grid from `--global-grid-file` (if exists)
   - Diff settled local tiles against global tiles
   - Merge consistent tiles; flag divergent tiles for review
   - Write updated global grid back to file

10. **Export to Formats**: Write files to output directory (see Output Structure)

## Export Formats

### GeoJSON (Default, Priority 1)

**Use Case**: GIS tools (QGIS, ArcGIS), web mapping (Leaflet, Mapbox), geospatial analysis

**Format**:

```json
{
  "type": "FeatureCollection",
  "metadata": {
    "sensor": "Ouster OS1-64",
    "capture_timestamp": "2026-01-15T10:30:00Z",
    "gps_origin": {"lat": 51.5074, "lon": -0.1278, "alt_msl": 10.0, "heading_deg": 45.0},
    "tile_size_m": 1.0,
    "range_max_m": 50.0,
    "confidence_min": 0.5,
    "coordinate_system": "WGS84"
  },
  "features": [
    {
      "type": "Feature",
      "geometry": {
        "type": "Polygon",
        "coordinates": [[[lon1, lat1], [lon2, lat2], [lon3, lat3], [lon4, lat4], [lon1, lat1]]]
      },
      "properties": {
        "tile_x": 10,
        "tile_y": 5,
        "plane_normal": [0.02, -0.01, 0.9998],
        "plane_offset": -1.85,
        "confidence": 0.95,
        "curvature_class": "flat",
        "curvature_deg": 1.2,
        "point_count": 847,
        "mean_height": -1.85,
        "height_std_dev": 0.03,
        "settlement_time_ms": 2340
      }
    }
  ]
}
```

**Implementation Notes**:

- Polygon coordinates must close (first point == last point) per GeoJSON spec (RFC 7946)
- If no GPS coordinates, use local Cartesian (meters) with `coordinate_system: "Sensor-XY"`
- Plane equation: `ax + by + cz + d = 0` where `[a,b,c]` is `plane_normal`, `d` is `plane_offset`

### ASC (CloudCompare Compatible, Priority 2)

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

- Reuse existing `ExportBackgroundGridToASC()` from `internal/lidar/l3grid/export_bg_snapshot.go`
- Z values are fitted plane heights, not raw point heights
- Tiles below confidence threshold written as `NODATA_value`
- If GPS coordinates available, use ENU X/Y for xllcorner/yllcorner (meters from GPS origin)

### CSV (Simple Tabular, Priority 2)

**Use Case**: Spreadsheet analysis, data science, custom processing

**Format**:

```csv
tile_x,tile_y,lat,lon,plane_a,plane_b,plane_c,plane_d,confidence,curvature_class,curvature_deg,point_count,mean_height,height_std_dev,settlement_time_ms
10,5,51.507412,-0.127834,0.02,-0.01,0.9998,-1.85,0.95,flat,1.2,847,-1.85,0.03,2340
11,5,51.507421,-0.127825,0.01,-0.02,0.9997,-1.83,0.92,flat,1.5,791,-1.83,0.04,2510
```

**Implementation Notes**:

- If no GPS: omit `lat,lon` columns or write `0,0`
- Plane equation: `ax + by + cz + d = 0`
- All numeric values rounded to sensible precision (lat/lon: 6 decimals, heights: 3 decimals)

### VTK (ParaView, Priority 3)

**Use Case**: 3D scientific visualisation, advanced analysis in ParaView/LidarView

**Format**: VTK StructuredGrid with scalar fields

```xml
<VTKFile type="StructuredGrid" version="1.0" byte_order="LittleEndian">
  <StructuredGrid WholeExtent="0 100 0 100 0 0">
    <Piece Extent="0 100 0 100 0 0">
      <Points>
        <DataArray type="Float32" NumberOfComponents="3" format="ascii">
          ... x y z coordinates ...
        </DataArray>
      </Points>
      <PointData Scalars="confidence">
        <DataArray type="Float32" Name="confidence" format="ascii">...</DataArray>
        <DataArray type="Float32" Name="curvature_deg" format="ascii">...</DataArray>
        <DataArray type="Int32" Name="point_count" format="ascii">...</DataArray>
      </PointData>
    </Piece>
  </StructuredGrid>
</VTKFile>
```

**Implementation Notes**:

- Defer to Phase 4 — requires VTK library or manual XML generation
- Recommended library: `github.com/lanl/vpic-utils/vtk` or custom XML writer
- Coordinate system: ENU if GPS available, else sensor-relative Cartesian

## GPS Integration

### Coordinate Fallback Chain

1. **PCAP GPS** (if `--gps-from-pcap` enabled):
   - Parse GPS ethernet packets using `docs/plans/lidar-architecture-gps-ethernet-parsing-plan.md` spec
   - Extract first valid GNGGA or GNRMC sentence with 3D fix
   - Use lat/lon/alt from GPS, heading from `--gps-heading` or NMEA course-over-ground

2. **Manual CLI Flags** (if PCAP GPS unavailable or disabled):
   - Use `--gps-lat`, `--gps-lon`, `--gps-alt`, `--gps-heading`
   - Validate lat/lon ranges: -90 ≤ lat ≤ 90, -180 ≤ lon ≤ 180
   - Default altitude 0.0m MSL, default heading 0.0° (north)

3. **No Geo-Referencing** (if neither source available):
   - Export in local sensor-relative Cartesian coordinates (X: forward, Y: left, Z: up)
   - Set `coordinate_system: "Sensor-XY"` in GeoJSON metadata
   - Omit lat/lon from CSV, use tile_x/tile_y only

### Coordinate Transformation

- **Local Cartesian → ENU**: Translate origin to GPS point, rotate by heading
- **ENU → WGS84**: Use `github.com/wroge/wgs84` library for geodetic conversion
- **Heading Convention**: Degrees clockwise from true north (0° = north, 90° = east)

### GPS Metadata in Exports

All formats include GPS origin in metadata/header:

- GeoJSON: `metadata.gps_origin` object
- ASC: Comment lines `# GPS_ORIGIN: lat lon alt heading`
- CSV: Separate `ground-plane-meta.json` sidecar file
- VTK: `<FieldData>` with GPS parameters

## Output Structure

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

**Global grid file** (if `--global-grid-merge`): Written to path specified by `--global-grid-file`, outside the per-run output directory. This file accumulates across runs.

**Naming Convention**: `ground-plane.<format>` for main export files

**Metadata File** (`ground-plane-meta.json`): Always written when `--ground-plane` enabled:

```json
{
  "extraction_timestamp": "2026-01-15T10:45:23Z",
  "pcap_file": "capture-2026-01-15.pcap",
  "sensor_model": "Hesai Pandar40P",
  "coordinate_system": "Sensor-XY",
  "gps_source": "none",
  "gps_origin": null,
  "tile_size_m": 1.0,
  "range_max_m": 50.0,
  "confidence_min": 0.5,
  "total_tiles": 847,
  "exported_tiles": 791,
  "filtered_tiles": 56,
  "processing_time_s": 12.4,
  "formats": ["csv", "asc"],
  "global_grid_merged": false
}
```

When GPS is available, `coordinate_system` becomes `"WGS84"`, `gps_source` becomes `"manual"` or `"pcap"`, and `gps_origin` is populated.

## Implementation Phases

### Phase 1: Core Ground Plane Extraction (No GPS)

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

### Phase 2: CLI Flags and CSV/ASC Export

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

### Phase 3: GPS Geo-Referencing and GeoJSON Export

**Goal**: Add GPS coordinate transformation and primary export format

**Tasks**:

1. Add GPS CLI flags (`--gps-lat`, `--gps-lon`, `--gps-alt`, `--gps-heading`)
2. Implement `--gps-from-pcap` flag and NMEA sentence parsing
3. Implement coordinate transformation: Cartesian → ENU → WGS84
4. Implement GeoJSON export with geo-referenced tile polygons
5. Update CSV/ASC exports to include GPS metadata
6. Integration tests: GPS fallback chain, coordinate transformation accuracy

**Deliverable**: `pcap-analyse --ground-plane --gps-lat 51.5074 --gps-lon -0.1278` produces geo-referenced GeoJSON

**Testing**: Validate GeoJSON schema (RFC 7946), test with QGIS import, verify coordinate transformation

### Phase 4: VTK Export and Advanced Features

**Goal**: Add VTK format for scientific visualisation, polish features

**Tasks**:

1. Implement VTK StructuredGrid export (manual XML or library)
2. Add curvature classification and per-tile statistics
3. Optimise memory usage for large PCAP files (streaming tile export)
4. Add progress reporting for long-running extractions
5. Performance benchmarking and optimisation

**Deliverable**: Full feature set with all four export formats

**Testing**: VTK validation with ParaView import, performance testing with multi-GB PCAP files

## Testing Strategy

### Unit Tests

- **Tile Fitting**: `internal/lidar/l4perception/ground_plane_test.go`
  - Test plane fitting with known point clouds (flat, sloped, noisy)
  - Test confidence scoring with varying eigenvalue ratios
  - Test curvature classification thresholds

- **Coordinate Transformation**: `internal/lidar/l4perception/gps_transform_test.go`
  - Test Cartesian → ENU → WGS84 round-trip accuracy (GPS additive path)
  - Test heading rotation (0°, 90°, 180°, 270°)
  - Test edge cases (poles, antimeridian)

### Integration Tests

- **PCAP Processing**: `cmd/tools/pcap-analyse/ground_plane_test.go`
  - Use existing test PCAP files (e.g., `data/test-captures/*.pcap`)
  - Validate ground plane extraction produces expected tile count
  - Validate export files are created with correct names
  - Validate no regression in existing exports (tracks.csv, results.json)

- **Export Format Validation**:
  - GeoJSON: Parse with `encoding/json`, validate against RFC 7946 schema
  - CSV: Parse with `encoding/csv`, validate column headers and data types
  - ASC: Parse header, validate grid dimensions match tile count

### Manual Testing

- Import GeoJSON into QGIS, verify tile positions and properties
- Open ASC in CloudCompare, verify ground plane visualisation
- Open VTK in ParaView, verify scalar field rendering
- Test GPS fallback chain with real PCAP files (with/without GPS packets)

### Regression Testing

- Ensure `--ground-plane=false` (default) produces identical output to current version
- Benchmark PCAP processing time with/without ground plane extraction
- Validate memory usage does not exceed 2x baseline for large PCAP files

## Acceptance Criteria

### Phase 1: Core Extraction

- [ ] Ground plane tiles extracted from test PCAP files
- [ ] Plane normals within 5° of expected values (for flat ground)
- [ ] Confidence scores > 0.9 for high-quality tiles, < 0.5 for noisy tiles
- [ ] Unit test coverage > 80% for tile fitting code
- [ ] No regression in existing PCAP processing (tracks.csv unchanged)

### Phase 2: CLI and Basic Exports

- [ ] All CLI flags documented in `--help` output
- [ ] CSV export validates against schema (all columns present)
- [ ] ASC export opens in CloudCompare without errors
- [ ] `ground-plane-meta.json` includes all required fields
- [ ] Integration test passes with 3 different PCAP files
- [ ] Export files written to correct output directory structure

### Phase 3: GPS and GeoJSON

- [ ] GPS fallback chain works: PCAP → manual → local coordinates
- [ ] GeoJSON validates against RFC 7946 schema (use `geojsonlint`)
- [ ] GeoJSON imports into QGIS with correct coordinate system (WGS84)
- [ ] Tile positions within 10cm of expected locations (for known GPS origin)
- [ ] Coordinate transformation accuracy: < 1cm error for points within 100m
- [ ] CSV and ASC exports include GPS metadata

### Phase 4: VTK and Polish

- [ ] VTK file opens in ParaView without errors
- [ ] Scalar fields (confidence, curvature) render correctly in ParaView
- [ ] PCAP processing time increases < 20% with ground plane extraction enabled
- [ ] Memory usage < 1GB for 10-minute PCAP files
- [ ] Progress reporting shows % completion during extraction
- [ ] All four export formats tested with real PCAP files

## Non-Goals (Out of Scope)

- **Real-time ground plane export**: This tool is for offline PCAP analysis only
- **Multi-sensor fusion**: Single sensor per PCAP file
- **Ground plane tracking over time**: Static extraction only, no temporal analysis
- **Automatic sensor height calibration**: Use existing site config or manual GPS altitude
- **Point cloud decimation**: Export all fitted tiles above confidence threshold
- **Web UI integration**: Command-line tool only, web API extensions handled separately

## Future Extensions

- **Ground plane change detection**: Compare ground plane exports from multiple captures to detect road damage or surface changes
- **Integration with web API**: Add `/api/lidar/export/ground-plane` endpoint for on-demand extraction
- **Automatic GPS from database**: Query site config for GPS coordinates if not provided via CLI
- **Multi-PCAP batch processing**: Process entire directory of PCAP files with single command
- **Ground plane texture mapping**: Export surface roughness or reflectivity as additional tile properties
- **Global grid visualisation**: Web UI for browsing the persistent Tier 2 global ground grid
- **OSM polyline import** (v2): Anchor ground plane tiles to kerb lines, crosswalks, and road edges from OpenStreetMap
- **OSM write-back** (v2): Propose edits to OSM with more accurate geometry from LiDAR measurements (requires OSM API key)
