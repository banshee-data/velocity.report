# AV Range Image Format Alignment

## Executive Summary

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

- Range image organizes point cloud in spherical coordinates
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
- Background grid uses polar organization (rings × azimuth bins)

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

The `is_in_nlz` channel marks points that are inside "no-label zones" in AV labeling pipelines. This is annotation metadata, not sensor data. For velocity.report:

- **Default value:** -1 (not in NLZ)
- **No implementation needed** unless integrating with external annotation systems

---

## Proposed Changes

### Phase 1: Data Structure Updates

#### 1.1 Enhanced Point Structures

**File: `internal/lidar/arena.go`**

Add new field to `Point`:

```go
type Point struct {
    // ... existing fields ...

    // Dual return support
    ReturnType     uint8   `json:"return_type"`     // 0=strongest, 1=last, 2=second-strongest
    ReturnIndex    int     `json:"return_index"`    // Which dual return this is (0 or 1)

    // Elongation (placeholder for AV format compatibility)
    Elongation     float32 `json:"elongation"`      // Pulse elongation [0.0-1.0], 0=unavailable

    // No-Label Zone (NLZ) flag (for AV format export compatibility)
    IsInNLZ        int8    `json:"is_in_nlz"`       // 1=in NLZ, -1=not in No-Label Zone (NLZ)
}
```

Add to `PointPolar`:

```go
type PointPolar struct {
    // ... existing fields ...

    ReturnType   uint8   `json:"return_type"`
    ReturnIndex  int     `json:"return_index"`
    Elongation   float32 `json:"elongation"`
}
```

**Estimated Effort:** 2 hours

#### 1.2 Range Image Structure

**File: `internal/lidar/range_image.go` (new file)**

```go
package lidar

// RangeImage represents a lidar frame as a 2D image in spherical coordinates
// following the open AV dataset format specification.
type RangeImage struct {
    SensorID      string    `json:"sensor_id"`
    Timestamp     int64     `json:"timestamp_ns"`
    ReturnIndex   int       `json:"return_index"`    // 0=first return, 1=second return

    // Dimensions
    Rows          int       `json:"rows"`            // Number of beams (40 for Pandar40P)
    Cols          int       `json:"cols"`            // Number of azimuth samples per rotation

    // Channel data (row-major order: [row * cols + col])
    Range         []float32 `json:"range"`           // Distance in meters (0 = no return)
    Intensity     []uint8   `json:"intensity"`       // Reflectivity [0-255]
    Elongation    []float32 `json:"elongation"`      // Pulse elongation [0-1] (0 = unavailable)
    IsInNLZ       []int8    `json:"is_in_nlz"`       // 1 = in NLZ, -1 = not in NLZ

    // Metadata
    BeamInclinations []float64 `json:"beam_inclinations"` // Per-row elevation angles (degrees)
    AzimuthStart     float64   `json:"azimuth_start"`     // Column 0 azimuth (degrees)
    AzimuthStep      float64   `json:"azimuth_step"`      // Degrees per column
}

// NewRangeImage creates an empty range image with the specified dimensions.
func NewRangeImage(sensorID string, rows, cols int, returnIdx int) *RangeImage {
    n := rows * cols
    return &RangeImage{
        SensorID:    sensorID,
        ReturnIndex: returnIdx,
        Rows:        rows,
        Cols:        cols,
        Range:       make([]float32, totalPixels),
        Intensity:   make([]uint8, totalPixels),
        Elongation:  make([]float32, totalPixels),
        IsInNLZ:     make([]int8, totalPixels),
    }
}

// Index returns the linear index for a given (row, col) position.
func (ri *RangeImage) Index(row, col int) int {
    return row*ri.Cols + col
}

// SetPoint populates a single pixel in the range image.
func (ri *RangeImage) SetPoint(row, col int, rangeValue, intensity float32, elongation float32, isInNLZ int8) {
    idx := ri.Index(row, col)
    if idx >= 0 && idx < len(ri.Range) {
        ri.Range[idx] = rangeValue
        ri.Intensity[idx] = uint8(intensity)
        ri.Elongation[idx] = elongation
        ri.IsInNLZ[idx] = isInNLZ
    }
}
```

**Estimated Effort:** 4 hours

### Phase 2: Parser Enhancements

#### 2.1 Dual Return Detection

**File: `internal/lidar/parse/extract.go`**

Add return mode awareness to packet parsing:

```go
// ReturnMode constants for dual-return handling
const (
    ReturnModeStrongest     = 0x37
    ReturnModeLast          = 0x38
    ReturnModeDual          = 0x39 // Last + Strongest
)

// IsDualReturn returns true if the packet is from dual-return mode
func (t *PacketTail) IsDualReturn() bool {
    return t.ReturnMode == ReturnModeDual
}

// GetReturnType determines return type for a block index in dual-return mode
// Even blocks = strongest return, odd blocks = last return
func GetReturnType(blockIdx int, isDualReturn bool) uint8 {
    if !isDualReturn {
        return 0 // Single return mode
    }
    if blockIdx%2 == 0 {
        return 0 // Strongest return
    }
    return 1 // Last return
}
```

Modify `blockToPoints()`:

```go
func (p *Pandar40PParser) blockToPoints(block *DataBlock, blockIdx int, tail *PacketTail) []lidar.PointPolar {
    isDual := tail.IsDualReturn()
    returnType := GetReturnType(blockIdx, isDual)
    returnIndex := 0
    if isDual && blockIdx%2 == 1 {
        returnIndex = 1
    }

    // ... existing code ...

    point := lidar.PointPolar{
        // ... existing fields ...
        ReturnType:  returnType,
        ReturnIndex: returnIndex,
        Elongation:  0.0, // Placeholder - not available from Pandar40P
    }
}
```

**Estimated Effort:** 4 hours

#### 2.2 Elongation Estimation (Optional Enhancement)

Since Pandar40P doesn't provide elongation, we can estimate it from local intensity variance:

```go
// EstimateElongation computes a pseudo-elongation value from intensity patterns.
// High variance in neighboring point intensities suggests edge effects or smearing.
// Returns a value in [0.0, 1.0] where 0 = sharp return, 1 = smeared return.
func EstimateElongation(points []PointPolar, idx int, neighbors int) float32 {
    if len(points) <= 1 || neighbors <= 0 {
        return 0.0
    }

    centerIntensity := float64(points[idx].Intensity)
    variance := 0.0
    count := 0

    for i := max(0, idx-neighbors); i <= min(len(points)-1, idx+neighbors); i++ {
        if i != idx {
            diff := float64(points[i].Intensity) - centerIntensity
            variance += diff * diff
            count++
        }
    }

    if count == 0 {
        return 0.0
    }

    // Normalize variance to [0, 1] range
    // Empirical: variance > 2500 (50² intensity difference) = high elongation
    normalizedVar := math.Sqrt(variance / float64(count)) / 50.0
    return float32(math.Min(normalizedVar, 1.0))
}
```

**Estimated Effort:** 4 hours (optional)

### Phase 3: Frame Builder Updates

#### 3.1 Range Image Generation

**File: `internal/lidar/frame_builder.go`**

Add method to convert LiDARFrame to RangeImage:

```go
// ToRangeImages converts a LiDARFrame to AV-style range images.
// Returns two range images: [0] = first/strongest return, [1] = second/last return
func (f *LiDARFrame) ToRangeImages(elevations []float64) [2]*RangeImage {
    const azimuthBins = 1800 // 0.2° resolution for 360°
    rows := len(elevations)
    if rows == 0 {
        rows = 40 // Default for Pandar40P
    }

    ri0 := NewRangeImage(f.SensorID, rows, azimuthBins, 0)
    ri1 := NewRangeImage(f.SensorID, rows, azimuthBins, 1)

    ri0.Timestamp = f.StartTimestamp.UnixNano()
    ri1.Timestamp = f.StartTimestamp.UnixNano()
    ri0.BeamInclinations = elevations
    ri1.BeamInclinations = elevations
    ri0.AzimuthStart = -180.0 // Column 0 = rear (-X axis)
    ri1.AzimuthStart = -180.0
    ri0.AzimuthStep = 360.0 / float64(azimuthBins)
    ri1.AzimuthStep = 360.0 / float64(azimuthBins)

    // Initialize NLZ to -1 (not in NLZ)
    for i := range ri0.IsInNLZ {
        ri0.IsInNLZ[i] = -1
        ri1.IsInNLZ[i] = -1
    }

    for _, pt := range f.Points {
        // Map channel to row (subtract 1 for 0-indexed)
        row := pt.Channel - 1
        if row < 0 || row >= rows {
            continue
        }

        // Map azimuth to column
        // AV format: column 0 = -X axis (rear), center = +X axis (front)
        // Our azimuth: 0° = front (+X), increasing clockwise
        azimuthOffset := pt.Azimuth + 180.0
        if azimuthOffset >= 360.0 {
            azimuthOffset -= 360.0
        }
        col := int(azimuthOffset / ri0.AzimuthStep)
        if col < 0 || col >= azimuthBins {
            continue
        }

        // Select target range image based on return index
        var ri *RangeImage
        if pt.ReturnIndex == 0 {
            ri = ri0
        } else {
            ri = ri1
        }

        ri.SetPoint(row, col, float32(pt.Distance), float32(pt.Intensity),
                    pt.Elongation, -1) // NLZ always -1 for street monitoring
    }

    return [2]*RangeImage{ri0, ri1}
}
```

**Estimated Effort:** 6 hours

### Phase 4: Web Configuration

#### 4.1 Return Mode Configuration

**File: `internal/lidar/monitor/webserver.go`**

Add endpoint for return mode configuration:

```go
// handleReturnModeConfig allows setting/getting the expected return mode for parsing
// GET: returns current return mode setting
// POST: sets expected return mode (affects how dual-return packets are parsed)
func (ws *WebServer) handleReturnModeConfig(w http.ResponseWriter, r *http.Request) {
    // ... implementation
}
```

Add to route registration:

```go
mux.HandleFunc("/api/lidar/return_mode", ws.handleReturnModeConfig)
```

**Web UI Changes:**

- Add dropdown in status.html for return mode selection
- Options: "Strongest (0x37)", "Last (0x38)", "Dual (0x39)"
- Display current return mode from packet tail

**Estimated Effort:** 4 hours

#### 4.2 Range Image Export Configuration

Add background param for range image settings:

```go
type BackgroundParams struct {
    // ... existing fields ...

    // Range image export settings
    RangeImageAzimuthBins int  `json:"range_image_azimuth_bins"` // Default 1800
    EnableRangeImageExport bool `json:"enable_range_image_export"`
    ExportSecondReturn bool     `json:"export_second_return"`
}
```

**Estimated Effort:** 2 hours

### Phase 5: Export Formats

#### 5.1 AV-Compatible Export

**File: `internal/lidar/export_av_format.go` (new file)**

```go
package lidar

import (
    "encoding/binary"
    "os"
)

// ExportRangeImageToAVFormat exports range images in a format compatible with
// open AV dataset tools.
func ExportRangeImageToAVFormat(ri *RangeImage, path string) error {
    f, err := os.Create(path)
    if err != nil {
        return err
    }
    defer f.Close()

    // Write header
    binary.Write(f, binary.LittleEndian, int32(ri.Rows))
    binary.Write(f, binary.LittleEndian, int32(ri.Cols))
    binary.Write(f, binary.LittleEndian, int32(4)) // 4 channels
    binary.Write(f, binary.LittleEndian, ri.Timestamp)

    // Write channel data (interleaved or planar based on AV spec)
    // Channel 0: Range
    for _, v := range ri.Range {
        binary.Write(f, binary.LittleEndian, v)
    }
    // Channel 1: Intensity
    for _, v := range ri.Intensity {
        binary.Write(f, binary.LittleEndian, float32(v))
    }
    // Channel 2: Elongation
    for _, v := range ri.Elongation {
        binary.Write(f, binary.LittleEndian, v)
    }
    // Channel 3: IsInNLZ
    for _, v := range ri.IsInNLZ {
        binary.Write(f, binary.LittleEndian, float32(v))
    }

    return nil
}
```

**Estimated Effort:** 4 hours

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
4. Internal: `internal/lidar/docs/reference/packet_analysis_results.md` - Packet structure analysis
