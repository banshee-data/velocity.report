# GPS Ethernet Parsing Architecture

Implementation plan for adding optional GPS-over-Ethernet parsing to LiDAR ingestion while preserving sensor-only operation, including parsing boundaries, data contracts, rollout safeguards, and integration validation requirements.

**Status:** Proposed
**Author:** Architecture Team
**Related:** [`hesai_packet_structure.md`](./hesai_packet_structure.md), [`ground-plane-extraction.md`](./ground-plane-extraction.md)

## Overview & Motivation

### Architecture Principle: GPS is Additive

**All local PCAP observations are sensor-iterative.** The velocity.report system **must function with the LiDAR sensor alone, with no GPS**. GPS is strictly additive — it enriches the system with geographic coordinates but is never required for core perception, ground plane extraction, or height-above-ground measurements. Every algorithm in the pipeline operates in sensor-local coordinates by default.

This document specifies how GPS data **may** be ingested over ethernet to enable optional geographic features when a GPS receiver is available.

### Current State

The velocity.report system currently stores site-level GPS coordinates in the database (`internal/db/site.go`) for map markers and report generation. LiDAR data operates in a sensor-local coordinate frame (X=right, Y=forward, Z=up) with no automatic geo-referencing capability.

The L1 packet parsing layer (`internal/lidar/l1packets/parse/`) already handles Hesai Pandar40P UDP packets with multiple timestamp modes including `TimestampModePTP` and `TimestampModeGPS`. The `resolvePacketTime()` function supports PTP/GPS timestamps with static-detection fallback, but does not ingest GPS position data.

### What GPS Enables (When Available)

Geographic referencing of LiDAR data is **optional but valuable** for:

1. **Tier 2 Global Ground Grid**: Populate the persistent lat/long-aligned ground plane grid (see `ground-plane-extraction.md`) that accumulates across observation sessions
2. **Multi-location PCAP Analysis**: Process captures from different deployment sites with absolute geographic context
3. **GeoJSON Exports**: Export ground plane tiles and other data with lat/long coordinates for GIS tools
4. **OSM Integration** (future): Anchor LiDAR observations to OpenStreetMap features for validation

### What Works Without GPS (Primary Operating Mode)

Without GPS, the system operates normally in sensor-local coordinates:

- **L3 Background Grid**: Foreground/background separation — no GPS needed
- **L4 Ground Plane (Tier 1)**: Local scene tiles settle from LiDAR returns in sensor frame — no GPS needed
- **L4 Clustering**: Height-above-ground queries via `GroundSurface` interface — no GPS needed
- **L5 Tracking / L6 Classification**: Multi-frame identity and object classes — no GPS needed
- **All PCAP analysis**: Full pipeline replay with CSV/JSON/training exports — no GPS needed

### Design Goals

- **Strictly additive**: System must work identically without GPS; GPS only adds geographic context
- **Privacy-preserving**: GPS coordinates are site-level metadata, not per-vehicle tracking
- **PCAP-compatible**: GPS packets captured alongside LiDAR packets in mixed network captures
- **Fallback graceful**: Use database site config when no GPS hardware available
- **Timestamp-correlated**: Associate GPS fixes with LiDAR frames by timestamp
- **Local-only**: No external transmission (consistent with privacy-first architecture)

## GPS Data Sources over Ethernet

### NMEA-over-UDP (Recommended)

Most GPS receivers with ethernet capability support NMEA-0183 sentences broadcast over UDP:

**Common Hardware:**

- u-blox NEO-M8/ZED-F9P with ethernet bridge modules
- Trimble timing receivers (SMT 360, RES SMT GG)
- Garmin GPS 18x LVC with serial-to-ethernet converter

**Network Configuration:**

- Standard UDP broadcast to subnet (e.g., 192.168.1.255:10110)
- Unicast to specific host (e.g., 192.168.100.50:10110)
- Configurable ports (typical: 10110, 10001, 2947 for gpsd)

**Advantages:**

- Industry-standard NMEA sentences
- Hardware-agnostic protocol
- Mixed capture in PCAP alongside LiDAR packets
- Simple parsing libraries available

### Hesai Built-in GPS

Some Hesai LiDAR sensors include GPS receiver inputs:

**Connection:**

- Physical: PPS (pulse-per-second) + NMEA serial (RS-232)
- Ethernet API: HTTP endpoint for GPS status (`/api/lidar/gps_info`)
- Packet embedding: GPS metadata in extended packet formats (model-dependent)

**Current Support:**

- PTP/GPS timestamp modes already parsed in `resolvePacketTime()`
- Position data not extracted (only time sync information)

**Limitations:**

- Requires physical GPS antenna connected to LiDAR sensor
- HTTP polling adds latency vs. UDP broadcast
- Packet embedding format varies by firmware version

### PTP with GPS Grandmaster

IEEE 1588 Precision Time Protocol (PTP) synchronised to GPS-disciplined grandmaster clock:

**Architecture:**

- Grandmaster clock receives GPS time (PPS + NMEA)
- PTP distributes nanosecond-precision time to LiDAR sensor
- GPS position obtained separately from grandmaster (not in PTP packets)

**Current Support:**

- `TimestampModePTP` already supported for time sync
- Position data requires separate GPS receiver or manual config

**Advantages:**

- Sub-microsecond time accuracy
- Standard for multi-sensor deployments

### Standalone GPS Receiver (Recommended)

External GPS receiver on same network segment as LiDAR sensor:

**Configuration:**

```
Network Segment: 192.168.100.x
LiDAR Sensor:    192.168.100.201 (Hesai Pandar40P)
GPS Receiver:    192.168.100.50 (u-blox with ethernet)
Data Collector:  192.168.100.1 (Raspberry Pi)
```

**PCAP Capture:**

- `tcpdump -i eth0 -w capture.pcap '(udp dst port 2368) or (udp dst port 10110)'`
- Mixed capture: LiDAR packets (port 2368) + GPS packets (port 10110)
- Single PCAP contains all geo-referenced data

**Advantages:**

- Independent of LiDAR hardware
- GPS quality not limited by sensor integration
- Easy to upgrade GPS receiver (e.g., RTK-capable units)
- Direct UDP broadcast requires no polling

## NMEA Sentence Parsing

### Standard Sentences

**$GPGGA - Global Positioning System Fix Data**

Essential sentence for geo-referencing:

```
$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47
       |      |          |           |  |  |   |      |      |
       |      |          |           |  |  |   |      |      +- Checksum
       |      |          |           |  |  |   |      +- Geoid separation (M)
       |      |          |           |  |  |   +- Altitude MSL (M)
       |      |          |           |  |  +- HDOP
       |      |          |           |  +- Satellites used
       |      |          |           +- Fix quality (0=invalid, 1=GPS, 2=DGPS, 4=RTK)
       |      |          +- Longitude (dddmm.mmmm, E/W)
       |      +- Latitude (ddmm.mmmm, N/S)
       +- UTC time (hhmmss.sss)
```

**Fields Required:**

- Latitude, longitude (decimal degrees after conversion)
- Fix quality (must be ≥1 for valid position)
- HDOP (horizontal dilution of precision)
- Satellite count (quality indicator)
- Altitude MSL (mean sea level)

**$GPRMC - Recommended Minimum**

Provides velocity and date:

```
$GPRMC,123519,A,4807.038,N,01131.000,E,022.4,084.4,230394,003.1,W*6A
       |      | |          |           |     |     |      |
       |      | |          |           |     |     |      +- Magnetic variation
       |      | |          |           |     |     +- Date (DDMMYY)
       |      | |          |           |     +- Course (degrees)
       |      | |          |           +- Speed (knots)
       |      | |          +- Longitude
       |      | +- Latitude
       |      +- Status (A=active, V=void)
       +- UTC time
```

**Fields Required:**

- Date (for absolute timestamp construction)
- Status (must be 'A' for valid fix)
- Latitude, longitude (redundant with GGA, used for validation)

**$GPGSA - DOP and Active Satellites**

Provides precision metrics:

```
$GPGSA,A,3,04,05,09,12,24,,,,,,,,,2.5,1.3,2.1*39
       | | |                       |   |   |
       | | |                       |   |   +- VDOP
       | | |                       |   +- HDOP
       | | |                       +- PDOP
       | | +- Satellite PRNs used
       | +- Fix type (1=none, 2=2D, 3=3D)
       +- Mode (A=auto, M=manual)
```

**Fields Required:**

- HDOP (horizontal dilution of precision, <2.0 preferred)
- PDOP (position dilution of precision, quality metric)
- Fix type (require 3D fix for altitude)

### Coordinate Conversion

NMEA uses `DDmm.mmmm` format; must convert to decimal degrees:

```
Input:  4807.038,N  (48° 07.038' North)
Output: 48.1173°

Calculation: degrees + (minutes / 60)
            = 48 + (7.038 / 60)
            = 48.1173
```

**Implementation:**

```go
func parseNMEACoordinate(coord string, hemisphere string) (float64, error) {
    // coord format: "DDmm.mmmm" or "DDDmm.mmmm"
    // hemisphere: "N"/"S" or "E"/"W"

    var degrees, minutes float64
    if len(coord) > 0 && strings.Contains(coord, ".") {
        dotIdx := strings.Index(coord, ".")
        minutesStart := dotIdx - 2

        degrees, _ = strconv.ParseFloat(coord[:minutesStart], 64)
        minutes, _ = strconv.ParseFloat(coord[minutesStart:], 64)
    }

    decimal := degrees + (minutes / 60.0)

    if hemisphere == "S" || hemisphere == "W" {
        decimal = -decimal
    }

    return decimal, nil
}
```

### Checksum Validation

NMEA sentences include XOR checksum for integrity:

```
$GPGGA,123519,4807.038,N,01131.000,E,1,08,0.9,545.4,M,46.9,M,,*47
                                                                ^^
                                                                Checksum
```

**Algorithm:**

1. XOR all characters between '$' and '\*'
2. Compare with two-digit hex checksum
3. Reject sentence if mismatch

**Implementation:**

```go
func validateNMEAChecksum(sentence string) bool {
    if !strings.HasPrefix(sentence, "$") || !strings.Contains(sentence, "*") {
        return false
    }

    starIdx := strings.Index(sentence, "*")
    payload := sentence[1:starIdx]
    checksumStr := sentence[starIdx+1:]

    var checksum byte
    for i := 0; i < len(payload); i++ {
        checksum ^= payload[i]
    }

    expectedChecksum, _ := strconv.ParseUint(checksumStr, 16, 8)
    return checksum == byte(expectedChecksum)
}
```

### Time Synchronisation

NMEA time must correlate with LiDAR timestamps:

**NMEA Time Format:**

- Time: `hhmmss.sss` (UTC, no date in GGA)
- Date: `DDMMYY` (only in RMC)
- Combined: `2026-03-23T12:35:19.000Z`

**Correlation Strategy:**

1. Construct absolute UTC timestamp from NMEA date+time
2. Match to LiDAR packet timestamps via `resolvePacketTime()`
3. Interpolate GPS position between fixes for high-frequency LiDAR frames
4. Detect GPS time jumps (reconnection, leap seconds)

**Challenges:**

- NMEA update rate (1-10 Hz) vs. LiDAR rate (10-20 Hz)
- Network jitter in UDP packet arrival
- GPS receiver startup delay (no fix for 30-60 seconds)

## GPS Data Model

### Go Structs

**GPSFix - Single Position Fix**

```go
// GPSFix represents a single GPS position measurement
type GPSFix struct {
    Timestamp      time.Time // UTC timestamp from NMEA
    Latitude       float64   // Decimal degrees, WGS84
    Longitude      float64   // Decimal degrees, WGS84
    AltitudeMSL    float64   // Metres above mean sea level
    AltitudeHAE    float64   // Metres above WGS84 ellipsoid (MSL + geoid separation)
    FixQuality     int       // 0=invalid, 1=GPS, 2=DGPS, 4=RTK, 5=Float RTK
    SatelliteCount int       // Number of satellites used
    HDOP           float64   // Horizontal dilution of precision
    PDOP           float64   // Position dilution of precision (from GPGSA)
    Speed          float64   // Ground speed, m/s (from GPRMC)
    Course         float64   // True course, degrees (from GPRMC)
    GeoidSep       float64   // Geoid separation, metres (from GPGGA)
}

// IsValid returns true if fix is suitable for geo-referencing
func (f *GPSFix) IsValid() bool {
    return f.FixQuality >= 1 && f.SatelliteCount >= 4 && f.HDOP < 5.0
}

// IsPrecise returns true if fix is suitable for sub-metre accuracy
func (f *GPSFix) IsPrecise() bool {
    return f.FixQuality >= 2 && f.HDOP < 2.0
}
```

**GPSConfig - Data Source Configuration**

```go
// GPSConfig specifies GPS data source parameters
type GPSConfig struct {
    Enabled       bool          // Enable GPS ingestion
    SourceType    GPSSourceType // UDP, HTTP, PCAP
    UDPPort       int           // UDP port for NMEA sentences (e.g., 10110)
    UDPAddress    string        // Multicast or specific address
    HTTPEndpoint  string        // HTTP API endpoint (for Hesai built-in GPS)
    PollInterval  time.Duration // HTTP polling interval
    SentenceTypes []string      // NMEA sentences to parse (e.g., ["GPGGA", "GPRMC"])
    MinSatellites int           // Minimum satellites for valid fix (default: 4)
    MaxHDOP       float64       // Maximum HDOP for valid fix (default: 5.0)
    TimeoutSec    int           // Seconds without fix before fallback to site config
}

type GPSSourceType int

const (
    GPSSourceUDP GPSSourceType = iota  // NMEA over UDP (standalone receiver)
    GPSSourceHTTP                      // HTTP API (Hesai built-in)
    GPSSourcePCAP                      // Replay from PCAP capture
    GPSSourceSiteConfig                // Manual coordinates from database
)
```

**GPSReceiver - Data Source Manager**

```go
// GPSReceiver manages GPS data ingestion and fix distribution
type GPSReceiver struct {
    config      GPSConfig
    latestFix   *GPSFix
    fixHistory  []GPSFix          // Recent fixes for interpolation
    fixChan     chan GPSFix       // Distribution channel for consumers
    mu          sync.RWMutex
    ctx         context.Context
    cancel      context.CancelFunc
}

// Start begins GPS data ingestion
func (r *GPSReceiver) Start(ctx context.Context) error

// GetLatestFix returns most recent valid GPS fix
func (r *GPSReceiver) GetLatestFix() (*GPSFix, error)

// GetFixAtTime returns interpolated GPS position at specific timestamp
func (r *GPSReceiver) GetFixAtTime(t time.Time) (*GPSFix, error)

// Subscribe returns channel for real-time fix updates
func (r *GPSReceiver) Subscribe() <-chan GPSFix
```

### WGS84 Datum

All GPS coordinates use **WGS84 (World Geodetic System 1984)** datum:

- Standard for GPS/GNSS systems worldwide
- Ellipsoid: semi-major axis 6378137 m, flattening 1/298.257223563
- Compatible with GIS tools (QGIS, ArcGIS), web mapping (Leaflet, OpenLayers)

**Altitude References:**

- **MSL (Mean Sea Level)**: Altitude relative to geoid (Earth's gravity equipotential surface)
- **HAE (Height Above Ellipsoid)**: Altitude relative to WGS84 ellipsoid
- **Relationship**: `HAE = MSL + GeoidSeparation` (from GPGGA sentence)

### Coordinate Precision Requirements

**Ground Plane Tile Alignment:**

- Requirement: <1 metre horizontal accuracy for tile georeferencing
- Achievable with: GPS fix quality ≥1 (standalone GPS), HDOP <2.0
- Preferred: DGPS (fix quality 2) or RTK (fix quality 4) for <10 cm accuracy

**Precision by Fix Type:**

- Standalone GPS (quality 1): 2-10 metres (sufficient for site-level georef)
- DGPS (quality 2): 0.5-3 metres (good for ground plane tiles)
- RTK Fixed (quality 4): 1-5 cm (high-precision surveying, future)
- RTK Float (quality 5): 10-50 cm (good for mobile deployments)

## Integration with L1 Packet Pipeline

### Parallel Data Sources

GPS operates alongside LiDAR packet ingestion:

```
Network Interface (eth0)
    |
    +-- UDP Port 2368 → LiDAR Packet Parser → L1 Frames
    |                                            |
    +-- UDP Port 10110 → GPS NMEA Parser --------+
                                                 |
                                                 v
                                    Time-Correlated Data Stream
                                      (LiDAR + GPS Position)
```

### Shared Network Listener

Option 1: **Single pcap listener** (PCAP replay and live capture)

```go
// Packet multiplexer routes by port
func (m *PacketMultiplexer) Dispatch(packet gopacket.Packet) {
    udpLayer := packet.Layer(layers.LayerTypeUDP)
    if udpLayer != nil {
        udp, _ := udpLayer.(*layers.UDP)

        switch udp.DstPort {
        case 2368:
            m.lidarChan <- packet.Data()
        case 10110:
            m.gpsChan <- packet.Data()
        }
    }
}
```

Option 2: **Separate goroutines** (live capture only)

```go
// Independent UDP listeners
go lidarReceiver.Listen(":2368")   // LiDAR packets
go gpsReceiver.Listen(":10110")    // GPS packets
```

**Recommendation:** Unified pcap-based listener for PCAP replay compatibility.

### Time Correlation

Associate GPS fixes with LiDAR frames by timestamp:

**Strategy 1: Nearest-Neighbour**

- Find GPS fix closest in time to LiDAR frame timestamp
- Suitable for stationary deployments (constant position)
- Max time delta: 5 seconds (typical GPS update rate)

**Strategy 2: Linear Interpolation**

- Interpolate position between two GPS fixes
- Suitable for mobile deployments (moving vehicle)
- Requires GPS update rate ≥1 Hz

**Implementation:**

```go
func (r *GPSReceiver) GetFixAtTime(t time.Time) (*GPSFix, error) {
    r.mu.RLock()
    defer r.mu.RUnlock()

    // Find bracketing fixes
    var before, after *GPSFix
    for i := range r.fixHistory {
        fix := &r.fixHistory[i]
        if fix.Timestamp.Before(t) {
            before = fix
        } else if fix.Timestamp.After(t) && after == nil {
            after = fix
            break
        }
    }

    // Stationary: return nearest fix
    if before != nil && after == nil {
        return before, nil
    }

    // Mobile: interpolate position
    if before != nil && after != nil {
        return interpolateFix(before, after, t), nil
    }

    return nil, ErrNoGPSFix
}
```

### Sensor-to-World Transform

Compute WGS84-referenced coordinate transform from GPS position:

**Transform Chain:**

```
Sensor Frame → Site Frame → Local ENU → ECEF → WGS84 Lat/Long
```

**Components:**

1. **Sensor mounting transform**: Rotation/translation from sensor to site anchor point
2. **ENU (East-North-Up) frame**: Local tangent plane at GPS position
3. **ECEF (Earth-Centred Earth-Fixed)**: 3D Cartesian coordinates
4. **WGS84**: Geodetic latitude/longitude/altitude

**For Ground Plane Export:**

- Ground plane tiles aligned to local ENU frame
- Tile origin specified in WGS84 coordinates
- Normal vector: Z-axis in ENU frame (true vertical)

## PCAP Considerations

### Mixed Capture Format

Single PCAP contains both LiDAR and GPS packets:

```bash
# Capture both data sources
sudo tcpdump -i eth0 -w /data/capture_site_A.pcap \
  'udp and (dst port 2368 or dst port 10110)'

# Verify packet types
tcpdump -r capture_site_A.pcap -n 'udp dst port 2368' | wc -l  # LiDAR count
tcpdump -r capture_site_A.pcap -n 'udp dst port 10110' | wc -l # GPS count
```

**PCAP Structure:**

- Frame 1: LiDAR packet (1262 bytes, port 2368)
- Frame 2: LiDAR packet (1262 bytes, port 2368)
- Frame 3: GPS packet (82 bytes, port 10110) - NMEA sentence
- Frame 4: LiDAR packet (1262 bytes, port 2368)
- ...

### Filtering by Port/Protocol

Extract GPS sentences from PCAP:

```bash
# Extract GPS packets only
tcpdump -r capture.pcap -n 'udp dst port 10110' -w gps_only.pcap

# Convert to text (NMEA sentences)
tcpdump -r gps_only.pcap -A | grep '^\$GP'
```

**In-code filtering:**

```go
// BPF filter for packet capture
filter := "udp and (dst port 2368 or dst port 10110)"
err := handle.SetBPFFilter(filter)
```

### Replay Considerations

**GPS Position Extraction:**

- Parse GPS packets during PCAP replay
- Build GPS fix timeline before LiDAR processing
- Associate fixes with LiDAR frames by packet timestamp

**Static Captures:**

- Stationary deployment: expect constant GPS position
- Extract single representative fix (median of all fixes)
- Apply to entire PCAP session

**Mobile Captures:**

- Moving vehicle: GPS position changes over time
- Interpolate position for each LiDAR frame
- Detect stationary periods (speed <0.5 m/s)

**Handling Missing GPS:**

- PCAP contains LiDAR packets but no GPS packets
- Fallback to site config (manual lat/long from database)
- Warn user about missing geo-reference

## Configuration

### Environment Variables

```bash
# GPS data source configuration
export GPS_ENABLED=true
export GPS_SOURCE=udp                # udp, http, pcap, site_config
export GPS_UDP_PORT=10110
export GPS_UDP_ADDRESS=0.0.0.0       # Listen on all interfaces
export GPS_MIN_SATELLITES=4
export GPS_MAX_HDOP=5.0
export GPS_TIMEOUT_SEC=60            # Fallback to site config after timeout

# Hesai HTTP API (alternative)
export GPS_SOURCE=http
export GPS_HTTP_ENDPOINT=http://192.168.100.201/api/lidar/gps_info
export GPS_POLL_INTERVAL=1s
```

### Configuration File

JSON configuration for site-specific deployments:

```json
{
  "site": {
    "name": "Oak Street Residential",
    "latitude": 47.6062,
    "longitude": -122.3321,
    "altitude_msl": 50.0,
    "timezone": "America/Los_Angeles"
  },
  "gps": {
    "enabled": true,
    "source": "udp",
    "udp_port": 10110,
    "sentence_types": ["GPGGA", "GPRMC", "GPGSA"],
    "min_satellites": 4,
    "max_hdop": 5.0,
    "timeout_sec": 60
  },
  "lidar": {
    "model": "Hesai Pandar40P",
    "ip_address": "192.168.100.201",
    "udp_port": 2368
  }
}
```

### Fallback Strategy

Graceful degradation when GPS unavailable:

1. **Primary**: Real-time GPS from UDP stream
2. **Secondary**: GPS from PCAP replay (if present)
3. **Tertiary**: Manual site config from database (`internal/db/site.go`)
4. **Quaternary**: No geo-reference (sensor-local coordinates only)

**Decision Logic:**

```go
func (g *GeoReference) GetPosition(t time.Time) (*GPSFix, error) {
    // Attempt real-time GPS
    if g.gpsReceiver != nil {
        if fix, err := g.gpsReceiver.GetFixAtTime(t); err == nil {
            return fix, nil
        }
    }

    // Fallback to site config
    if g.siteConfig != nil {
        return &GPSFix{
            Latitude:  g.siteConfig.Latitude,
            Longitude: g.siteConfig.Longitude,
            Altitude:  g.siteConfig.Altitude,
            FixQuality: 0, // Indicate manual config
        }, nil
    }

    return nil, ErrNoGeoReference
}
```

## Storage Schema

### GPS Fix History Table

For mobile deployments or multi-session analysis:

```sql
CREATE TABLE gps_fix_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp DATETIME NOT NULL,              -- UTC timestamp
    latitude REAL NOT NULL,                   -- Decimal degrees, WGS84
    longitude REAL NOT NULL,                  -- Decimal degrees, WGS84
    altitude_msl REAL,                        -- Metres above mean sea level
    altitude_hae REAL,                        -- Metres above WGS84 ellipsoid
    fix_quality INTEGER NOT NULL,             -- 0=invalid, 1=GPS, 2=DGPS, 4=RTK
    satellite_count INTEGER,                  -- Satellites used
    hdop REAL,                                -- Horizontal DOP
    pdop REAL,                                -- Position DOP
    speed REAL,                               -- Ground speed, m/s
    course REAL,                              -- True course, degrees
    session_id TEXT,                          -- Link to capture session
    FOREIGN KEY (session_id) REFERENCES capture_session(id)
);

CREATE INDEX idx_gps_fix_timestamp ON gps_fix_history(timestamp);
CREATE INDEX idx_gps_fix_session ON gps_fix_history(session_id);
```

### Session-Level GPS Metadata

Link GPS position to PCAP capture sessions:

```sql
CREATE TABLE capture_session (
    id TEXT PRIMARY KEY,                      -- UUID
    pcap_file TEXT NOT NULL,                  -- PCAP filename
    start_time DATETIME NOT NULL,             -- Session start (UTC)
    end_time DATETIME NOT NULL,               -- Session end (UTC)
    gps_latitude REAL,                        -- Representative position
    gps_longitude REAL,
    gps_altitude_msl REAL,
    gps_fix_count INTEGER DEFAULT 0,          -- Number of GPS fixes
    gps_fix_quality_median INTEGER,           -- Median fix quality
    gps_hdop_median REAL,                     -- Median HDOP
    site_id INTEGER,                          -- Link to site config
    FOREIGN KEY (site_id) REFERENCES site(id)
);
```

### Ground Plane Geo-Referencing

Link ground plane tiles to GPS coordinates:

```sql
CREATE TABLE ground_plane_tile (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL,                 -- Link to capture session
    tile_x INTEGER NOT NULL,                  -- Tile grid coordinates
    tile_y INTEGER NOT NULL,
    center_latitude REAL NOT NULL,            -- WGS84 centre point
    center_longitude REAL NOT NULL,
    center_altitude_msl REAL,
    bbox_north REAL NOT NULL,                 -- Bounding box (WGS84)
    bbox_south REAL NOT NULL,
    bbox_east REAL NOT NULL,
    bbox_west REAL NOT NULL,
    point_count INTEGER NOT NULL,             -- Points in tile
    geometry_blob BLOB NOT NULL,              -- Compressed geometry
    FOREIGN KEY (session_id) REFERENCES capture_session(id)
);

CREATE INDEX idx_gp_tile_session ON ground_plane_tile(session_id);
CREATE INDEX idx_gp_tile_coords ON ground_plane_tile(tile_x, tile_y);
CREATE SPATIAL INDEX idx_gp_tile_bbox ON ground_plane_tile(bbox_north, bbox_south, bbox_east, bbox_west);
```

## Security & Privacy

### GPS as Site-Level Metadata

**Not Personally Identifiable Information:**

- GPS coordinates identify sensor location, not individuals
- No tracking of vehicle positions or routes
- Site-level granularity (building/street level)

**Privacy Alignment:**

- Consistent with privacy-first design (no cameras, no license plates)
- Local-only storage (SQLite database)
- No external transmission of coordinates

### Local-Only Storage

**Data Retention:**

- GPS coordinates stored in local SQLite database
- PCAP files remain on Raspberry Pi (`/var/lib/velocity-report/`)
- No cloud synchronisation or external transmission

**User Control:**

- Users own all GPS data (same as LiDAR data)
- Manual export for GIS integration (user-initiated)
- Can disable GPS ingestion entirely (fallback to site config)

### Attack Surface

**Network Exposure:**

- UDP port 10110 open for GPS receiver (local network only)
- No authentication required (read-only data source)
- Vulnerable to spoofed GPS packets (mitigation: checksum validation)

**Spoofing Mitigation:**

- NMEA checksum validation (reject invalid sentences)
- Fix quality checks (require ≥4 satellites, HDOP <5.0)
- Consistency checks (detect position jumps >100 metres)
- Optional: DGPS or RTK for authenticated position

## Open Questions & Future Work

### IMU Integration for Sensor Orientation

**Current State:**

- GPS provides sensor position (latitude/longitude/altitude)
- Sensor orientation unknown (roll, pitch, yaw)
- Ground plane assumes level mounting

**Future Enhancement:**

- Integrate IMU (Inertial Measurement Unit) for 6DOF pose
- MEMS IMU via I2C (e.g., Bosch BNO055, Adafruit LSM6DS33)
- Sensor fusion: GPS + IMU for complete geo-referenced pose

**Use Cases:**

- Non-level sensor mounting (tilted pole, vehicle dashboard)
- Sensor motion detection (vibration, rotation)
- Accurate ground plane extraction on slopes

### RTK Corrections for Centimetre-Level Accuracy

**Current State:**

- Standalone GPS: 2-10 metre accuracy
- Sufficient for site-level geo-referencing

**RTK Enhancement:**

- Real-Time Kinematic corrections via NTRIP (Networked Transport of RTCM via Internet Protocol)
- Base station or CORS (Continuously Operating Reference Station)
- Achieves 1-5 cm accuracy (high-precision surveying)

**Challenges:**

- Requires internet connection (conflicts with local-only design)
- NTRIP client implementation (RTCM 3.x protocol)
- Post-processing alternative: store raw observations, compute corrections offline

### Mobile Deployment (Vehicle-Mounted Sensor)

**Current State:**

- Stationary deployment assumed (fixed pole/building mount)
- GPS position constant over capture session

**Mobile Enhancement:**

- Vehicle-mounted LiDAR + GPS
- Continuous position updates (5-10 Hz GPS)
- Speed and heading from GPRMC sentence

**Use Cases:**

- Street-level scanning (similar to Google Street View)
- Mobile traffic monitoring (multiple neighbourhoods per day)
- Before/after analysis (drive same route at different times)

**Challenges:**

- Vibration and motion blur
- GPS accuracy in urban canyons (poor satellite visibility)
- Data volume (TB-scale point clouds per day)

### Multi-Sensor Coordination

**Future Vision:**

- Multiple sensors at different locations
- Aggregated ground plane (neighbourhood-scale coverage)
- Privacy-preserving: no raw data sharing, only aggregate tiles

**GPS Requirements:**

- Common WGS84 reference frame
- Time synchronisation (PTP or GPS time)
- Coordinate transform validation

### GeoJSON Export for GIS Integration

**Planned Feature:**

- Export ground plane tiles as GeoJSON
- Compatible with QGIS, ArcGIS, Leaflet
- Include elevation (2.5D polygons)

**Format Example:**

```json
{
  "type": "FeatureCollection",
  "features": [
    {
      "type": "Feature",
      "geometry": {
        "type": "Polygon",
        "coordinates": [[
          [-122.3321, 47.6062, 50.0],
          [-122.3320, 47.6062, 50.1],
          ...
        ]]
      },
      "properties": {
        "tile_id": "tile_0_0",
        "point_count": 15243,
        "session_id": "a4f2c8b9-..."
      }
    }
  ]
}
```

---

**Next Steps:**

All GPS work is **additive** and should not modify any existing LiDAR-only code paths:

1. Implement NMEA parser with checksum validation (`internal/gps/nmea/`)
2. Create `GPSReceiver` for UDP ingestion (`internal/gps/receiver.go`)
3. Extend L1 packet pipeline with **optional** GPS correlation (`internal/lidar/l1packets/`)
4. Add GPS schema to SQLite database (`internal/db/migrations/`)
5. Implement PCAP replay with GPS extraction (`cmd/tools/pcap-analyse/`)
6. Document user-facing GPS configuration (`docs/src/guides/gps-setup.md`)

**Design invariant:** Every feature must have a clean no-GPS fallback. If GPS is absent, disabled, or failing, the system operates identically to today's LiDAR-only pipeline.
