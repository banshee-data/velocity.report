# AV Range Image Format Alignment

Changes required to align the velocity.report LiDAR capture format with industry-standard autonomous vehicle range image conventions, covering dual returns, elongation, and structured range image organisation.

## Overview

This design document outlines the changes required to align the velocity.report Hesai capture format with the open AV dataset lidar data specification (dual returns, elongation, range images). The key alignment areas are:

1. **Dual Return Support**: Capture and store both strongest returns (already supported by Hesai Pandar40P)
2. **Elongation Measurement**: Add pulse elongation to point data structure
3. **Range Image Format**: Organize point cloud data as 2D range images
4. **Channel Structure**: Align with standard AV 4-channel format (range, intensity, elongation, is_in_nlz: No-Label Zone flag)

## Current State Analysis

### Hesai Pandar40P Capabilities

The Hesai Pandar40P sensor natively supports the core features needed for AV format alignment:

| Feature      | Hesai Native Support  | Current Implementation      | AV Format Requirement    |
| ------------ | --------------------- | --------------------------- | ------------------------ |
| Dual Returns | ✅ ReturnMode 0x39    | ⚠️ Only first return parsed | ✅ Two strongest returns |
| Range        | ✅ 4mm resolution     | ✅ Fully implemented        | ✅ Required              |
| Intensity    | ✅ 0-255 reflectivity | ✅ Fully implemented        | ✅ Required              |
| Elongation   | ❌ Not in protocol    | ❌ Not available            | ✅ Required              |
| is_in_nlz    | N/A (AV-specific)     | N/A                         | ⚠️ Not applicable        |

### AV Dataset LiDAR Data Format Reference

From open AV dataset documentation:

```
Range Image Structure (per-lidar):
├── 4 base channels:
│   ├── channel 0: range (spherical coordinate distance)
│   ├── channel 1: lidar intensity
│   ├── channel 2: lidar elongation
│   └── channel 3: is_in_nlz (1 = in, -1 = not in)
├── 6 camera projection channels (optional)
└── Two range images per lidar (for dual returns)
```

**Key Characteristics:**

- Range image organises point cloud in spherical coordinates
- Rows = inclination (elevation), Columns = azimuth
- Row 0 = maximum inclination, center column = forward (+X axis)
- Separate range image for each return

### Elongation Definition

From AV dataset specification:

> "Lidar elongation refers to the elongation of the pulse beyond its nominal width. Returns with long pulse elongation indicate that the laser reflection is potentially smeared or refracted, such that the return pulse is elongated in time."

**Physical Meaning:**

- Low elongation → clean, sharp reflection surface
- High elongation → smeared/refracted reflection (edge, transparent, fog)

---

## Gap Analysis

### 1. Dual Return Parsing (Medium Effort)

**Current State:**

- Parser only extracts single return per channel
- ReturnMode field (byte 14 of tail) is parsed but not used for dual-return extraction
- Return modes: 0x37=Strongest, 0x38=Last, 0x39=Last+Strongest

**Gap:**
When sensor is in dual-return mode (0x39), the Pandar40P sends **two blocks per azimuth position**:

- Block N: Strongest return
- Block N+1: Last return

Current parser treats all blocks as independent, missing the paired relationship.

**Hesai Dual Return Block Structure:**

```
Dual Return Packet (10 blocks = 5 azimuth positions):
├── Block 0: Azimuth A, Strongest Return
├── Block 1: Azimuth A, Last Return
├── Block 2: Azimuth B, Strongest Return
├── Block 3: Azimuth B, Last Return
...
```

### 2. Elongation Data (High Effort - Hardware Limitation)

**Critical Finding:** The Hesai Pandar40P does not provide elongation data in its packet format.

**Options:**

1. **Compute elongation estimate** from intensity variance across neighboring points
2. **Use placeholder values** (all zeros) for compatibility
3. **Upgrade to sensor with elongation** (Hesai QT128, Ouster sensors)
4. **Mark field as unavailable** in exported data

**Recommendation:** Implement option 2 (placeholders) initially, with option 1 as future enhancement.

### 3. Range Image Format (Medium Effort)

**Current State:**

- Points stored in polar form (`PointPolar`) with azimuth, elevation, distance
- Frames accumulated as unordered point lists
- Background grid uses polar organisation (rings × azimuth bins)

**Gap:**
AV format expects 2D image with:

- Fixed number of rows (inclinations/rings)
- Fixed number of columns (azimuth positions per rotation)
- Dense grid with null/zero for missing points

**Mapping:**

```
AV Range Image                Hesai Pandar40P
────────────────             ─────────────────
rows = beam count            rows = 40 channels
cols = azimuth samples       cols = ~1800 per rotation (at 10Hz)
channel[0] = range           = point.Distance
channel[1] = intensity       = point.Intensity
channel[2] = elongation      = not available (use 0)
channel[3] = is_in_nlz       = always -1 (not applicable)
```

### 4. No-Label Zone (NLZ) (Not Applicable)

The `is_in_nlz` channel marks points that are inside "no-label zones" in AV labelling pipelines. This is annotation metadata, not sensor data. For velocity.report:

- **Default value:** -1 (not in NLZ)
- **No implementation needed** unless integrating with external annotation systems

---

## Proposed Changes

### Phase 1: Data structure updates

Add three fields to `Point` and `PointPolar` in `internal/lidar/l2frames/arena.go`: `ReturnType` (uint8: 0=strongest, 1=last, 2=second-strongest), `ReturnIndex` (int: 0 or 1), and `Elongation` (float32: 0.0–1.0, 0=unavailable). Add `IsInNLZ` (int8: always −1 for street monitoring) to `Point` only.

Create `internal/lidar/l2frames/range_image.go` with a `RangeImage` struct: sensor ID, timestamp, return index, row/col dimensions (40 × ~1800), four channel slices (range, intensity, elongation, is_in_nlz) in row-major order, plus beam inclination and azimuth metadata. Provide `NewRangeImage()`, `Index(row, col)`, and `SetPoint()` methods.

### Phase 2: Parser enhancements

**Dual return detection** — add return mode constants (Strongest 0x37, Last 0x38, Dual 0x39) and `IsDualReturn()` to `PacketTail` in `internal/lidar/l1packets/extract.go`. In dual mode, even blocks are strongest returns, odd blocks are last returns. Modify `blockToPoints()` to populate `ReturnType` and `ReturnIndex`.

**Elongation estimation** (optional) — since Pandar40P lacks hardware elongation, estimate from local intensity variance across neighbouring points. Normalise to [0, 1] with empirical threshold (σ > 50 = high elongation).

### Phase 3: Frame builder updates

Add `ToRangeImages()` to `LiDARFrame` in `internal/lidar/l2frames/frame_builder.go`. Maps each point's channel to a row and azimuth to a column (0.2° bins, 1800 columns). Returns two `RangeImage` instances — one per return. AV convention: column 0 = rear (−X axis), centre = front (+X). NLZ always −1.

### Phase 4: Web configuration

Add `GET/POST /api/lidar/return_mode` endpoint for return mode selection. Add three `BackgroundParams` fields: `range_image_azimuth_bins` (default 1800), `enable_range_image_export`, `export_second_return`. Web UI: dropdown in status page for return mode.

### Phase 5: Export formats

Create `internal/lidar/l2frames/export_av_format.go` with `ExportRangeImageToAVFormat()`. Binary format: header (rows, cols, 4 channels, timestamp) followed by planar channel data (range → intensity → elongation → is_in_nlz), all little-endian float32.

---

## Implementation Phases Summary

| Phase | Description                      | Effort | Dependencies |
| ----- | -------------------------------- | ------ | ------------ |
| 1.1   | Point structure updates          | 2h     | None         |
| 1.2   | Range image structure            | 4h     | 1.1          |
| 2.1   | Dual return parser               | 4h     | 1.1          |
| 2.2   | Elongation estimation (optional) | 4h     | 1.1          |
| 3.1   | Range image generation           | 6h     | 1.2, 2.1     |
| 4.1   | Web return mode config           | 4h     | 2.1          |
| 4.2   | Range image export config        | 2h     | 1.2          |
| 5.1   | AV format export                 | 4h     | 3.1          |

**Total Estimated Effort:** 26-30 hours (excluding optional elongation estimation)

---

## Web Configuration Settings

### Required Settings

| Setting                     | Location            | Default       | Description                      |
| --------------------------- | ------------------- | ------------- | -------------------------------- |
| `return_mode`               | `/api/lidar/params` | `0x39` (dual) | Expected return mode for parsing |
| `range_image_azimuth_bins`  | `/api/lidar/params` | 1800          | Azimuth resolution               |
| `enable_range_image_export` | `/api/lidar/params` | false         | Enable range image generation    |
| `export_second_return`      | `/api/lidar/params` | true          | Include second return in exports |

### API Endpoints

| Endpoint                             | Method | Description                         |
| ------------------------------------ | ------ | ----------------------------------- |
| `GET /api/lidar/return_mode`         | GET    | Get current return mode             |
| `POST /api/lidar/return_mode`        | POST   | Set expected return mode            |
| `GET /api/lidar/range_image`         | GET    | Export current frame as range image |
| `POST /api/lidar/range_image/config` | POST   | Configure range image parameters    |

---

## Parser Adaptation Details

### Current Parser Flow

```
UDP Packet (1262/1266 bytes)
    ↓
parseTail() → Extract ReturnMode, Timestamp, MotorSpeed
    ↓
parseDataBlock() × 10 → Extract azimuth + 40 channel measurements
    ↓
blockToPoints() → Apply calibration, generate PointPolar[]
    ↓
FrameBuilder.AddPointsPolar() → Accumulate into LiDARFrame
```

### Modified Parser Flow (with AV format alignment)

```
UDP Packet (1262/1266 bytes)
    ↓
parseTail() → Extract ReturnMode, Timestamp, MotorSpeed
    ↓
IsDualReturn() → Determine block pairing strategy
    ↓
For dual return: blocks 0,2,4,6,8 = first return; 1,3,5,7,9 = second return
    ↓
parseDataBlock() × 10 → Extract azimuth + 40 channel measurements
    ↓
blockToPoints() → Apply calibration, add ReturnType, generate PointPolar[]
    ↓
FrameBuilder.AddPointsPolar() → Accumulate into LiDARFrame with return tracking
    ↓
(Optional) ToRangeImages() → Generate AV-compatible range images
```

---

## Limitations and Considerations

### Hardware Limitations

1. **Elongation not available** from Pandar40P - must use placeholders or estimation
2. **Fixed beam count** of 40 channels (vs 64-beam in some AV dataset lidars)
3. **Max range 200m** vs 75m truncation in some datasets (not a problem)

### Privacy Considerations

- Range image format preserves privacy (no PII, just geometric data)
- No camera projection channels needed for velocity.report use case

### Compatibility Notes

- Range image format is compatible with open AV dataset tools but not identical
- `is_in_nlz` always -1 (no annotation zones in street monitoring)
- Elongation placeholder (0.0) until sensor upgrade or estimation implemented

---

## Testing Strategy

### Unit Tests

- Test dual return block pairing logic
- Test range image indexing and population
- Test azimuth remapping for AV coordinate system
- Test export file format correctness

### Integration Tests

- Replay PCAP in dual-return mode, verify both returns captured
- Export range images, verify dimensions match expected
- Compare point counts between frame and range image

### Validation

- Visual inspection of range images in CloudCompare/LidarView
- Compare against sample AV dataset range images for format alignment

---

## References

1. Open AV Dataset LiDAR specification (dual returns, elongation, range images)
2. [Hesai Pandar40P User Manual](https://www.hesaitech.com/wp-content/uploads/2025/04/Pandar40P_User_Manual_402-en-250410.pdf)
3. Internal: `internal/lidar/parse/extract.go` - Current parser implementation
4. Internal: `data/structures/HESAI_PACKET_FORMAT.md` - Packet structure analysis
