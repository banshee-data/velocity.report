package lidar

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

/*
Pandar40P LiDAR Packet Parser Architecture

This parser is optimized for high-throughput processing of Hesai Pandar40P LiDAR UDP packets.
The Pandar40P sends 1262-byte UDP packets containing measurements from 40 laser channels
organized into 10 data blocks per packet, potentially generating up to 400 3D points per packet.

PACKET STRUCTURE (1262 bytes total):
├── Header (6 bytes) - packet format metadata [SKIPPED for performance]
├── Data Blocks (1224 bytes) - 10 blocks × 122 bytes each
│   └── Each block: 2-byte azimuth + 40 channels × 3 bytes (distance + reflectivity)
└── Tail (32 bytes) - timing and status data [PARTIALLY PARSED]

PARSER ARCHITECTURE:
1. Packet validation (size check)
2. Timestamp extraction (only 4 bytes from tail)
3. Data block parsing (10 iterations)
4. Point generation with calibration corrections
5. Coordinate transformation (spherical → Cartesian)

PERFORMANCE OPTIMIZATIONS IMPLEMENTED:
- Header parsing skipped (only packet format metadata, not needed for point generation)
- Tail parsing minimized (parseTimestamp extracts only required 4-byte timestamp)
- Point slices pre-allocated with capacity to avoid reallocations during append operations
- Trigonometric calculations optimized (pre-compute cos/sin values to avoid repeated calculations)
- BlockID field uses efficient block index instead of parsing packet data

CURRENT PERFORMANCE METRICS:
- Processing time: ~36.5μs per packet
- Memory allocation: ~170KB per packet
- Allocations per operation: 25

PACKET FIELDS NOT CURRENTLY PARSED (available for future features):
Header fields (6 bytes):
- SOB (Start of Block identifier)
- ChLaserNum (channel/laser configuration)
- ChBlockNum (channel/block configuration)
- FirstBlockReturn (return mode)
- DisUnit (distance unit identifier)

Tail fields (28 of 32 bytes unused):
- Reserved bytes (bytes 0-9, 11-21, 23-24)
- HighPrecisionFlag (byte 10)
- AzimuthFlag (byte 22)
- MotorSpeed (bytes 25-26)
- Timestamp (bytes 27-30) is parsed and used for point generation
- FactoryInfo (byte 31)

CALIBRATION DATA:
Uses embedded calibration configuration containing per-channel angle corrections
and firetime corrections for accurate 3D point generation. Calibration accounts
for manufacturing tolerances and ensures precise coordinate transformation.
*/

// Pandar40P LiDAR packet structure constants
// These define the fixed format of UDP packets sent by Hesai Pandar40P sensors
const (
	PACKET_SIZE        = 1262 // Total UDP packet size in bytes (fixed for Pandar40P)
	BLOCKS_PER_PACKET  = 10   // Number of data blocks per packet (each contains measurements from all 40 channels)
	CHANNELS_PER_BLOCK = 40   // Number of laser channels per data block (Pandar40P has 40 channels total)
	BYTES_PER_CHANNEL  = 3    // Channel data size: 2 bytes distance + 1 byte reflectivity
	HEADER_SIZE        = 6    // Packet header size in bytes
	TAIL_SIZE          = 32   // Packet tail size in bytes (contains timestamp and metadata)
	AZIMUTH_SIZE       = 2    // Azimuth field size in each data block (2 bytes, little-endian)

	// Physical measurement conversion constants
	DISTANCE_RESOLUTION = 0.004 // Distance unit: 4mm per LSB (converts raw values to meters)
	AZIMUTH_RESOLUTION  = 0.01  // Azimuth unit: 0.01 degrees per LSB (converts raw values to degrees)
	ROTATION_MAX_UNITS  = 36000 // Maximum azimuth value representing 360.00 degrees
)

// Pandar40P configuration containing calibration data embedded in the binary
// This configuration is essential for accurate point cloud generation as it contains
// sensor-specific calibration parameters that correct for manufacturing tolerances
type Pandar40PConfig struct {
	AngleCorrections    [CHANNELS_PER_BLOCK]AngleCorrection    // Per-channel angle calibration data
	FiretimeCorrections [CHANNELS_PER_BLOCK]FiretimeCorrection // Per-channel timing calibration data
}

// AngleCorrection contains the angular calibration parameters for each laser channel
// These corrections account for mechanical tolerances in the sensor assembly
type AngleCorrection struct {
	Channel   int     // Laser channel number (1-40)
	Elevation float64 // Vertical angle correction in degrees (relative to horizontal plane)
	Azimuth   float64 // Horizontal angle correction in degrees (relative to sensor front)
}

// FiretimeCorrection contains timing calibration for each laser channel
// Different channels fire at slightly different times to avoid interference
type FiretimeCorrection struct {
	Channel  int     // Laser channel number (1-40)
	FireTime float64 // Time offset in microseconds when this channel fires relative to block start
}

// PacketHeader represents the 6-byte header at the start of each UDP packet
// Contains metadata about the packet format and sensor configuration
type PacketHeader struct {
	SOB              uint16 // Start of Block identifier (packet format marker)
	ChLaserNum       uint8  // Channel and laser configuration number
	ChBlockNum       uint8  // Channel and block configuration number
	FirstBlockReturn uint8  // Return mode for first block (single/dual return)
	DisUnit          uint8  // Distance measurement unit identifier
}

// DataBlock represents one of 10 data blocks within a packet
// Each block contains measurements from all 40 channels at a specific azimuth angle
// Pandar40P blocks contain only azimuth + channel data (no block ID header)
type DataBlock struct {
	Azimuth  uint16                          // Raw azimuth angle in 0.01-degree units
	Channels [CHANNELS_PER_BLOCK]ChannelData // Measurement data for all 40 channels
}

// ChannelData represents the raw measurement from a single laser channel
// Contains the fundamental distance and intensity measurements
type ChannelData struct {
	Distance     uint16 // Raw distance measurement in 4mm units (0 = no return)
	Reflectivity uint8  // Laser return intensity/reflectivity value (0-255)
}

// PacketTail represents the 32-byte tail at the end of each UDP packet
// Contains timing information and sensor status data
// Note: Only the timestamp field (bytes 27-30) is currently used by the parser for performance
type PacketTail struct {
	Reserved1         [10]uint8 // Reserved bytes (bytes 0-9)
	HighPrecisionFlag uint8     // High precision measurement mode flag (byte 10)
	Reserved2         [11]uint8 // Reserved bytes (bytes 11-21)
	AzimuthFlag       uint8     // Azimuth measurement mode flag (byte 22)
	Reserved3         [2]uint8  // Reserved bytes (bytes 23-24)
	MotorSpeed        uint16    // Sensor rotation speed in RPM (bytes 25-26)
	Timestamp         uint32    // Microsecond timestamp for packet acquisition (bytes 27-30)
	FactoryInfo       uint8     // Factory/firmware information byte (byte 31)
}

// Pandar40PParser handles parsing of Pandar40P LiDAR UDP packets into 3D point clouds
// The parser uses calibration data to convert raw measurements into accurate 3D coordinates
type Pandar40PParser struct {
	config Pandar40PConfig // Sensor-specific calibration parameters
}

// NewPandar40PParser creates a new parser instance with the provided calibration configuration
// The configuration must contain valid angle and firetime corrections for all 40 channels
func NewPandar40PParser(config Pandar40PConfig) *Pandar40PParser {
	return &Pandar40PParser{
		config: config,
	}
}

// ParsePacket parses a complete UDP packet from Pandar40P sensor into a slice of 3D points
// The packet must be exactly 1262 bytes and contain valid data blocks and timestamp
// Returns up to 400 points (10 blocks × 40 channels, excluding invalid measurements)
func (p *Pandar40PParser) ParsePacket(data []byte) ([]Point, error) {
	// Validate packet size - Pandar40P packets are always exactly 1262 bytes
	if len(data) != PACKET_SIZE {
		return nil, fmt.Errorf("invalid packet size: expected %d, got %d", PACKET_SIZE, len(data))
	}

	// Extract timestamp from the packet tail (optimized to read only needed data)
	tailOffset := PACKET_SIZE - TAIL_SIZE
	timestamp, err := p.parseTimestamp(data[tailOffset:])
	if err != nil {
		return nil, fmt.Errorf("failed to parse timestamp: %v", err)
	}

	// Process all 10 data blocks in the packet (after 6-byte header, before 32-byte tail)
	var points []Point
	dataOffset := HEADER_SIZE

	for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
		// Calculate block size: 2 bytes azimuth + (40 channels × 3 bytes each)
		blockSize := AZIMUTH_SIZE + (CHANNELS_PER_BLOCK * BYTES_PER_CHANNEL)
		if dataOffset+blockSize > tailOffset {
			break // Safety check: ensure we don't read into tail section
		}

		// Parse individual data block
		block, err := p.parseDataBlock(data[dataOffset : dataOffset+blockSize])
		if err != nil {
			return nil, fmt.Errorf("failed to parse block %d: %v", blockIdx, err)
		}

		// Convert raw measurements to calibrated 3D points
		blockPoints := p.blockToPoints(block, blockIdx, timestamp)
		points = append(points, blockPoints...)

		dataOffset += blockSize
	}

	return points, nil
}

// parseDataBlock parses a single 122-byte data block containing measurements from all 40 channels
// Block format: 2 bytes azimuth + (40 × 3 bytes channel data)
func (p *Pandar40PParser) parseDataBlock(data []byte) (*DataBlock, error) {
	if len(data) < AZIMUTH_SIZE {
		return nil, fmt.Errorf("insufficient data for block header")
	}

	block := &DataBlock{
		Azimuth: binary.LittleEndian.Uint16(data[0:2]), // Raw azimuth in 0.01-degree units
	}

	// Parse measurement data for all 40 channels
	channelOffset := AZIMUTH_SIZE // Start parsing after the 2-byte azimuth field
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		if channelOffset+BYTES_PER_CHANNEL > len(data) {
			return nil, fmt.Errorf("insufficient data for channel %d", i)
		}

		// Extract 3-byte channel data: 2 bytes distance + 1 byte reflectivity
		block.Channels[i] = ChannelData{
			Distance:     binary.LittleEndian.Uint16(data[channelOffset : channelOffset+2]),
			Reflectivity: data[channelOffset+2],
		}
		channelOffset += BYTES_PER_CHANNEL
	}

	return block, nil
}

// parseTimestamp extracts only the timestamp from the 32-byte packet tail
// This is optimized to avoid parsing unused tail fields
func (p *Pandar40PParser) parseTimestamp(data []byte) (uint32, error) {
	if len(data) != TAIL_SIZE {
		return 0, fmt.Errorf("invalid tail size: expected %d, got %d", TAIL_SIZE, len(data))
	}

	// Extract only the timestamp (bytes 27-30)
	timestamp := binary.LittleEndian.Uint32(data[27:31])
	return timestamp, nil
}

// blockToPoints converts raw measurements from a data block into calibrated 3D points
// Applies sensor-specific calibrations and converts from spherical to Cartesian coordinates
// Each block can produce up to 40 points (one per channel), excluding invalid measurements
func (p *Pandar40PParser) blockToPoints(block *DataBlock, blockIdx int, timestamp uint32) []Point {
	// Pre-allocate slice with capacity for maximum possible points to avoid reallocations
	points := make([]Point, 0, CHANNELS_PER_BLOCK)

	// Convert microsecond timestamp to Go time.Time once per block
	// Note: Assumes timestamp is microseconds since Unix epoch
	packetTime := time.Unix(0, int64(timestamp)*1000) // Convert microseconds to nanoseconds

	// Extract base azimuth angle from block data (in 0.01-degree units)
	baseAzimuth := float64(block.Azimuth) * AZIMUTH_RESOLUTION

	// Process each of the 40 channels in this block
	for channelIdx := 0; channelIdx < CHANNELS_PER_BLOCK; channelIdx++ {
		channelData := block.Channels[channelIdx]

		// Skip invalid measurements - distance of 0 typically means no laser return
		if channelData.Distance == 0 {
			continue
		}

		// Convert zero-based array index to one-based channel number for calibration lookup
		channelNum := channelIdx + 1

		// Get calibration parameters for this specific channel
		angleCorrection := p.config.AngleCorrections[channelIdx]
		firetimeCorrection := p.config.FiretimeCorrections[channelIdx]

		// Apply azimuth correction and normalize to 0-360 degree range
		azimuth := baseAzimuth + angleCorrection.Azimuth
		if azimuth < 0 {
			azimuth += 360
		} else if azimuth >= 360 {
			azimuth -= 360
		}

		// Convert raw distance measurement to meters using 4mm resolution
		distance := float64(channelData.Distance) * DISTANCE_RESOLUTION

		// Get corrected elevation angle for this channel
		elevation := angleCorrection.Elevation

		// Convert spherical coordinates (distance, azimuth, elevation) to Cartesian (x, y, z)
		// Coordinate system: X=forward, Y=right, Z=up relative to sensor
		azimuthRad := azimuth * math.Pi / 180.0
		elevationRad := elevation * math.Pi / 180.0

		// Optimize trigonometric calculations
		cosElevation := math.Cos(elevationRad)
		sinElevation := math.Sin(elevationRad)
		cosAzimuth := math.Cos(azimuthRad)
		sinAzimuth := math.Sin(azimuthRad)

		x := distance * cosElevation * sinAzimuth // Forward/back
		y := distance * cosElevation * cosAzimuth // Left/right
		z := distance * sinElevation              // Up/down

		// Apply per-channel firetime correction to get accurate point timestamp
		firetimeOffset := time.Duration(firetimeCorrection.FireTime * float64(time.Microsecond))
		pointTime := packetTime.Add(firetimeOffset)

		// Create final calibrated point with all computed values
		point := Point{
			X:         x,
			Y:         y,
			Z:         z,
			Intensity: channelData.Reflectivity,
			Distance:  distance,
			Azimuth:   azimuth,
			Elevation: elevation,
			Channel:   channelNum,
			Timestamp: pointTime,
			BlockID:   blockIdx,
		}

		points = append(points, point)
	}

	return points
}
