# Pandar40P Parser & FrameBuilder — Patch Plan

**Goal:** Fix geometry artifacts in single-frame exports (spikes at ~0° and ~220–255°, concentric rings) and align output with LiDARView.

---

## TL;DR Mapping (Symptom → Cause → Fix)

- **Spikes near fixed bearings** → Not consuming **block preamble (0xFFEE)**, misaligned reads.  
  ✅ Parse preamble; set correct block size (**124 bytes**).
- **Concentric rings** → All channels in a block share the same azimuth.  
  ✅ Add **per-channel azimuth** via **fire-time** interpolation (or fallback to next-block interpolation).
- **Scene rotated/weird** → `sin/cos` swapped in XY mapping.  
  ✅ Use `x=cos(el)cos(az)`, `y=cos(el)sin(az)`, `z=sin(el)`.
- **“Ghosts outside room”** → Misparse plus no range validity clamps.  
  ✅ Add simple **distance/intensity** filters.
- **Time weirdness** → Treating sensor μs as Unix epoch.  
  ✅ Keep as **sensor time** for now; map to host later.

---

## 1) Packet Layout: Sizes & Parsing

**Constants (derive from first principles; avoid magic numbers):**
```go
const (
    PACKET_SIZE        = 1262 // verify against your pcap; keep runtime assert
    HEADER_SIZE        = 6
    BLOCKS_PER_PACKET  = 10
    CHANNELS_PER_BLOCK = 40
    BYTES_PER_CHANNEL  = 3    // 2 bytes dist + 1 byte reflectivity

    BLOCK_PREAMBLE_SIZE = 2   // 0xFFEE
    AZIMUTH_SIZE        = 2   // uint16, 0.01°
    BLOCK_SIZE          = BLOCK_PREAMBLE_SIZE + AZIMUTH_SIZE + CHANNELS_PER_BLOCK*BYTES_PER_CHANNEL // 124
    RANGING_DATA_SIZE   = BLOCKS_PER_PACKET * BLOCK_SIZE // 1240
)
```

**Tail offset (compute dynamically):**
```go
tailOffset := HEADER_SIZE + RANGING_DATA_SIZE
if tailOffset > len(data) {
    return nil, fmt.Errorf("payload too short for %d blocks", BLOCKS_PER_PACKET)
}
tail := data[tailOffset:]
TAIL_SIZE := len(tail) // for validation/logging
```

**Parse each block (consume + validate preamble):**
```go
func (p *Pandar40PParser) parseDataBlock(b []byte) (*DataBlock, error) {
    if len(b) < BLOCK_SIZE { return nil, fmt.Errorf("short block") }
    pre := binary.LittleEndian.Uint16(b[0:2])
    if pre != 0xEEFF { // wire 0xFFEE shows as 0xEEFF LE
        return nil, fmt.Errorf("bad block preamble: 0x%04x", pre)
    }
    az := binary.LittleEndian.Uint16(b[2:4])
    off := 4
    var ch [CHANNELS_PER_BLOCK]ChannelData
    for i := 0; i < CHANNELS_PER_BLOCK; i++ {
        ch[i] = ChannelData{
            Distance:     binary.LittleEndian.Uint16(b[off : off+2]),
            Reflectivity: b[off+2],
        }
        off += BYTES_PER_CHANNEL
    }
    return &DataBlock{ Azimuth: az, Channels: ch }, nil
}
```

> **Note:** Keep `parseHeader` and `parseTail` but validate `tail` length you computed above.

---

## 2) Polar → Cartesian Mapping (fix XY)

Adopt a consistent sensor-frame convention: **X forward, Y right, Z up** (or Y left—pick & stick; below assumes Y right).

```go
azRad := azimuthDeg * math.Pi / 180
elRad := elevationDeg * math.Pi / 180

x := distance * math.Cos(elRad) * math.Cos(azRad) // forward
y := distance * math.Cos(elRad) * math.Sin(azRad) // right
z := distance * math.Sin(elRad)                   // up
```

---

## 3) Per-Channel Azimuth (eliminate rings)

**Use motor RPM + per-channel fire time**:
```go
// degrees per microsecond
degPerUs := (360.0 * float64(tail.MotorSpeed) / 60.0) / 1e6

for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
    baseAz := float64(block.Azimuth) * 0.01 // 0.01° units
    dtUs   := p.config.FiretimeCorrections[ch].FireTime // μs
    azCh   := baseAz + degPerUs*dtUs
    // normalize 0..360
    if azCh < 0 { azCh += 360 } else if azCh >= 360 { azCh -= 360 }
    // then compute x,y,z using azCh
}
```

**Fallback (if motor speed or fire-time not trusted):**  
Interpolate to the **next block’s** azimuth across channels (handle wrap: if Δaz > +180°, subtract 360; if < -180°, add 360).

---

## 4) Distance & Validity Filters

```go
if rawDist == 0 { continue }                  // no return
dist := float64(rawDist) * 0.004              // 4mm resolution
if dist < 0.3 || dist > 120.0 { continue }    // clamp (tune per environment)
// optional: drop intensity==0 if you see obvious junk
```

---

## 5) Timestamps (keep sensor time)

`tail.Timestamp` is device μs (since boot/PPS), **not** Unix epoch.  
- Keep `Point.Timestamp` as `sensor_us` (int64) or `time.Duration` since start-of-session.
- Map to host time later via a measured offset if needed.

---

## 6) FrameBuilder Improvements

**Problem:** Pure time-bucketing (100 ms) can cut frames mid-spin if RPM jitters.  
**Add:**

1) **Wrap detection**: if current azimuth < last azimuth by > 180°, **finalize** current frame now.
2) **Coverage guard**: only emit a frame if `(maxAz - minAz)` **≥ 330°**; else keep accumulating.

**Sketch:**
```go
type FrameBuilder struct {
    lastAz         float64
    minAz, maxAz   float64
    // existing buffers...
}

func (fb *FrameBuilder) AddPoint(p Point) {
    az := p.Azimuth
    if fb.lastAz != 0 && (fb.lastAz - az) > 180 {
        fb.Finalize() // spin wrapped
    }
    fb.lastAz = az
    if az < fb.minAz { fb.minAz = az }
    if az > fb.maxAz { fb.maxAz = az }
    // append point...
}

func (fb *FrameBuilder) ShouldEmit() bool {
    cover := fb.maxAz - fb.minAz
    if cover < 0 { cover += 360 }
    return cover >= 330 // tune
}
```

---

## 7) Sanity Checks

- **Azimuth histogram**: ensure no tall spike near ~252° (that would mean preamble read as azimuth).
- **Single-frame export**: expect crisp walls, 40 vertical bands in Z, no sprays at ~0°/220–255°, no concentric rings.

---

## 8) Checklist

- [ ] Set `BLOCK_SIZE = 124` and compute `RANGING_DATA_SIZE`, `TAIL_SIZE` dynamically.  
- [ ] Parse & validate block **preamble (0xFFEE)**.  
- [ ] Fix XY mapping to `cos(el)cos(az)`, `cos(el)sin(az)`.  
- [ ] Implement **per-channel azimuth** (RPM + fire-time) or next-block interpolation.  
- [ ] Add **distance clamps** (and optionally intensity check).  
- [ ] Add **wrap detection** + **coverage guard** to `FrameBuilder`.  
- [ ] Re-export single frame; verify in CloudCompare.
