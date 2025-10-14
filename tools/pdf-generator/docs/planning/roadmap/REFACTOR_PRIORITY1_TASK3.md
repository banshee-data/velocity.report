# Priority 1, Task 3: Extract map_utils.py

## Status: ✅ COMPLETE

## Summary
Extracted SVG map handling and radar marker injection into a dedicated `map_utils.py` module. The module is designed to be forward-compatible with future OpenStreetMap (OSM) vector data integration while currently working with static SVG files.

## Changes Made

### 1. Created `map_utils.py` (579 lines)

**Core Classes:**

- `RadarMarker`: Represents a radar sensor with GPS-compatible positioning
  - Currently uses fractional coordinates (0-1 range) for SVG positioning
  - Supports bearing angle (degrees, 0=North, clockwise)
  - Includes optional GPS lat/lon fields for future OSM integration
  - `to_svg_coords()`: Converts fractional position to SVG coordinates

- `SVGMarkerInjector`: Handles SVG manipulation to add radar markers
  - `_extract_viewbox()`: Extracts SVG viewBox from various formats
  - `_compute_triangle_points()`: Calculates triangle geometry based on bearing and coverage angle
  - `inject_marker()`: Injects radar marker overlay into SVG content
  - Supports configurable circle marker at sensor position

- `SVGToPDFConverter`: Converts SVG files to PDF using available tools
  - Tries conversion methods in order: cairosvg → inkscape → rsvg-convert
  - Gracefully falls back through options based on availability
  - `_try_cairosvg()`, `_try_inkscape()`, `_try_rsvg_convert()`: Individual converter attempts

- `MapProcessor`: High-level API for map processing workflow
  - `process_map()`: Orchestrates marker injection and PDF conversion
  - Handles temporary SVG creation when markers are added
  - Manages timestamp-based conversion decisions
  - Returns (success, pdf_path) tuple

**Helper Functions:**

- `create_marker_from_config()`: Creates RadarMarker from config dictionary
  - Convenient integration with `report_config.MAP_CONFIG`

**Future OSM Integration Placeholders:**

- `download_osm_map()`: Reserved for OSM vector data download
- `compute_viewbox_from_gps()`: Reserved for GPS-to-viewBox conversion

**Key Design Features:**

1. **GPS-Compatible Positioning**: Triangle calculations use bearing angles (0=North) instead of arbitrary rotations, making it straightforward to use GPS heading data in the future

2. **Fractional Coordinates**: Current implementation uses 0-1 fractional coordinates that can be easily converted to/from GPS coordinates when OSM integration is added

3. **Separation of Concerns**:
   - `RadarMarker`: Data representation
   - `SVGMarkerInjector`: SVG manipulation logic
   - `SVGToPDFConverter`: Format conversion
   - `MapProcessor`: Workflow orchestration

4. **Extensibility**: Structure supports multiple sensors, custom zoom levels, and bounding boxes in future versions

### 2. Updated `pdf_generator.py`

**Removed** (~180 lines):
- Inline SVG marker injection code
- Triangle geometry calculations
- SVG viewBox parsing
- PDF conversion logic (cairosvg, inkscape, rsvg-convert attempts)
- Temporary SVG file handling

**Added** (~27 lines):
- Import of `map_utils` module
- Instantiation of `MapProcessor` with config
- Creation of `RadarMarker` from `MAP_CONFIG`
- Simple call to `process_map()` for complete workflow
- Conditional map inclusion based on success

**Net Result**: 153 lines removed, much cleaner code

### 3. Created `test_map_utils.py` (432 lines)

**Test Coverage** (23 tests, 100% passing):

- `TestRadarMarker` (3 tests):
  - Initialization with defaults and custom parameters
  - Coordinate conversion to SVG space

- `TestSVGMarkerInjector` (8 tests):
  - ViewBox extraction from various SVG formats
  - Triangle point computation for different bearings
  - Marker injection with content preservation
  - Custom color handling

- `TestSVGToPDFConverter` (4 tests):
  - cairosvg availability and fallback
  - inkscape conversion attempts
  - rsvg-convert conversion attempts

- `TestMapProcessor` (6 tests):
  - Processor initialization and configuration
  - Map processing with and without markers
  - Force conversion behavior
  - Temporary SVG creation

- `TestHelperFunctions` (2 tests):
  - Marker creation from config
  - Default value handling

## Metrics

- **Lines Added**: 579 (map_utils.py) + 432 (test_map_utils.py) = 1,011
- **Lines Removed**: 153 (pdf_generator.py)
- **Net Change**: +858 lines (includes comprehensive tests and documentation)
- **Tests Created**: 23
- **Test Pass Rate**: 100% (23/23)
- **Code Reduction in pdf_generator.py**: 153 lines (13.8% reduction)

## Benefits

1. **Separation of Concerns**: Map handling logic isolated from PDF generation
2. **Testability**: 23 comprehensive unit tests covering all functionality
3. **Future-Ready**: Designed for OSM integration with GPS positioning
4. **Maintainability**: Clear class boundaries and single responsibility
5. **Reusability**: Map utilities can be used outside PDF generation context
6. **Cleaner Code**: pdf_generator.py is significantly simpler

## Future Enhancements Ready

The module is structured to support:

1. **OSM Vector Data Download**:
   - Download map tiles based on GPS coordinates
   - Automatic bounding box calculation
   - Configurable zoom levels

2. **Multi-Sensor Support**:
   - Multiple `RadarMarker` instances on same map
   - Each with independent GPS position and bearing

3. **Dynamic Map Generation**:
   - Generate SVG from OSM data at runtime
   - No need for static map.svg files

4. **Advanced Positioning**:
   - Automatic conversion from GPS lat/lon to SVG coordinates
   - ViewBox calculation from geographic bounds

## Smoke Test Results

```bash
$ python get_stats.py --file-prefix map-test-3 --group 1h --units mph \
    --histogram --hist-bucket-size 5 --source radar_data_transits \
    --timezone US/Pacific --min-speed 5 --debug 2025-06-02 2025-06-04
```

**Output**:
```
DEBUG: API response status=200 elapsed=27.5ms metrics=22 histogram_present=True
Wrote stats PDF: map-test-3-1_stats.pdf
Wrote daily PDF: map-test-3-1_daily.pdf
DEBUG: histogram bins=10 total=3469
Wrote histogram PDF: map-test-3-1_histogram.pdf
Generated PDF: map-test-3-1_report.pdf (engine=xelatex)
Generated PDF report: map-test-3-1_report.pdf
```

✅ **Result**: All PDFs generated successfully with map overlay intact

## Validation Steps

1. ✅ All 23 unit tests pass
2. ✅ CLI smoke test generates PDFs successfully
3. ✅ Map overlay (triangle + circle) appears correctly in PDF
4. ✅ No regression in PDF generation workflow
5. ✅ Code is cleaner and more maintainable

## Notes

- The `RadarMarker.bearing_deg` field uses compass conventions (0=North, 90=East, 180=South, 270=West) to align with future GPS heading data
- Fractional coordinates (0-1 range) provide a normalized coordinate system that can be mapped to any viewBox or geographic bounds
- The module gracefully handles missing conversion tools (cairosvg, inkscape, rsvg-convert) by trying multiple fallback options
- Future OSM integration can be added incrementally without breaking existing functionality

## Priority 1 Progress

- ✅ Task 1: Extract report_config.py (COMPLETE)
- ✅ Task 2: Extract data_transformers.py (COMPLETE)
- ✅ Task 3: Extract map_utils.py (COMPLETE)

**Next**: Await user validation before proceeding to Priority 2 tasks.
