package lidar

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// Pandar40P packet structure constants
const (
	PACKET_SIZE        = 1262 // Standard Pandar40P packet size
	BLOCKS_PER_PACKET  = 10   // 10 data blocks per packet
	CHANNELS_PER_BLOCK = 40   // 40 channels per block
	BYTES_PER_CHANNEL  = 3    // 2 bytes distance + 1 byte reflectivity
	HEADER_SIZE        = 6    // Packet header size
	TAIL_SIZE          = 32   // Packet tail size
	BLOCK_HEADER_SIZE  = 2    // Block header size
	AZIMUTH_SIZE       = 2    // Azimuth data size

	// Constants for calculations
	DISTANCE_RESOLUTION = 0.004 // 4mm resolution
	AZIMUTH_RESOLUTION  = 0.01  // 0.01 degree resolution
	ROTATION_MAX_UNITS  = 36000 // 360.00 degrees
)

// Pandar40P configuration loaded from CSV files
type Pandar40PConfig struct {
	AngleCorrections    [CHANNELS_PER_BLOCK]AngleCorrection
	FiretimeCorrections [CHANNELS_PER_BLOCK]FiretimeCorrection
}

type AngleCorrection struct {
	Channel   int
	Elevation float64 // degrees
	Azimuth   float64 // degrees
}

type FiretimeCorrection struct {
	Channel  int
	FireTime float64 // microseconds
}

// Point represents a single LiDAR point
type Point struct {
	X         float64   `json:"x"`         // meters
	Y         float64   `json:"y"`         // meters
	Z         float64   `json:"z"`         // meters
	Intensity uint8     `json:"intensity"` // reflectivity value
	Distance  float64   `json:"distance"`  // meters
	Azimuth   float64   `json:"azimuth"`   // degrees
	Elevation float64   `json:"elevation"` // degrees
	Channel   int       `json:"channel"`   // channel number (1-40)
	Timestamp time.Time `json:"timestamp"` // packet timestamp
	BlockID   int       `json:"block_id"`  // block within packet
}

// PacketHeader represents the packet header structure
type PacketHeader struct {
	SOB              uint16 // Start of packet identifier
	ChLaserNum       uint8  // Channel and laser number
	ChBlockNum       uint8  // Channel and block number
	FirstBlockReturn uint8  // First block return mode
	DisUnit          uint8  // Distance unit
}

// DataBlock represents a single data block within a packet
type DataBlock struct {
	BlockID  uint16                          // Block identifier (0xEEFF)
	Azimuth  uint16                          // Azimuth angle * 100
	Channels [CHANNELS_PER_BLOCK]ChannelData // Channel data
}

// ChannelData represents data for a single channel
type ChannelData struct {
	Distance     uint16 // Distance measurement
	Reflectivity uint8  // Reflectivity value
}

// PacketTail represents the packet tail with timestamp and other info
type PacketTail struct {
	Reserved1         [10]uint8 // Reserved bytes
	HighPrecisionFlag uint8     // High precision flag
	Reserved2         [11]uint8 // Reserved bytes
	AzimuthFlag       uint8     // Azimuth flag
	Reserved3         [2]uint8  // Reserved bytes
	MotorSpeed        uint16    // Motor speed
	Timestamp         uint32    // Microsecond timestamp
	FactoryInfo       uint8     // Factory information
}

// Pandar40PParser handles parsing of Pandar40P LiDAR packets
type Pandar40PParser struct {
	config Pandar40PConfig
}

// NewPandar40PParser creates a new parser with the given configuration
func NewPandar40PParser(config Pandar40PConfig) *Pandar40PParser {
	return &Pandar40PParser{
		config: config,
	}
}

// ParsePacket parses a raw UDP packet into LiDAR points
func (p *Pandar40PParser) ParsePacket(data []byte) ([]Point, error) {
	if len(data) != PACKET_SIZE {
		return nil, fmt.Errorf("invalid packet size: expected %d, got %d", PACKET_SIZE, len(data))
	}

	// Parse header
	_, err := p.parseHeader(data[:HEADER_SIZE])
	if err != nil {
		return nil, fmt.Errorf("failed to parse header: %v", err)
	}

	// Parse tail to get timestamp
	tailOffset := PACKET_SIZE - TAIL_SIZE
	tail, err := p.parseTail(data[tailOffset:])
	if err != nil {
		return nil, fmt.Errorf("failed to parse tail: %v", err)
	}

	// Parse data blocks
	var points []Point
	dataOffset := HEADER_SIZE

	for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
		blockSize := BLOCK_HEADER_SIZE + AZIMUTH_SIZE + (CHANNELS_PER_BLOCK * BYTES_PER_CHANNEL)
		if dataOffset+blockSize > tailOffset {
			break // Not enough data for complete block
		}

		block, err := p.parseDataBlock(data[dataOffset : dataOffset+blockSize])
		if err != nil {
			return nil, fmt.Errorf("failed to parse block %d: %v", blockIdx, err)
		}

		// Convert block data to points
		blockPoints := p.blockToPoints(block, blockIdx, tail.Timestamp)
		points = append(points, blockPoints...)

		dataOffset += blockSize
	}

	return points, nil
}

// parseHeader parses the packet header
func (p *Pandar40PParser) parseHeader(data []byte) (*PacketHeader, error) {
	if len(data) < HEADER_SIZE {
		return nil, fmt.Errorf("insufficient data for header")
	}

	header := &PacketHeader{
		SOB:              binary.LittleEndian.Uint16(data[0:2]),
		ChLaserNum:       data[2],
		ChBlockNum:       data[3],
		FirstBlockReturn: data[4],
		DisUnit:          data[5],
	}

	return header, nil
}

// parseDataBlock parses a single data block
func (p *Pandar40PParser) parseDataBlock(data []byte) (*DataBlock, error) {
	if len(data) < BLOCK_HEADER_SIZE+AZIMUTH_SIZE {
		return nil, fmt.Errorf("insufficient data for block header")
	}

	block := &DataBlock{
		BlockID: binary.LittleEndian.Uint16(data[0:2]),
		Azimuth: binary.LittleEndian.Uint16(data[2:4]),
	}

	// Verify block identifier
	if block.BlockID != 0xEEFF {
		return nil, fmt.Errorf("invalid block identifier: 0x%04X", block.BlockID)
	}

	// Parse channel data
	channelOffset := BLOCK_HEADER_SIZE + AZIMUTH_SIZE
	for i := 0; i < CHANNELS_PER_BLOCK; i++ {
		if channelOffset+BYTES_PER_CHANNEL > len(data) {
			return nil, fmt.Errorf("insufficient data for channel %d", i)
		}

		block.Channels[i] = ChannelData{
			Distance:     binary.LittleEndian.Uint16(data[channelOffset : channelOffset+2]),
			Reflectivity: data[channelOffset+2],
		}
		channelOffset += BYTES_PER_CHANNEL
	}

	return block, nil
}

// parseTail parses the packet tail
func (p *Pandar40PParser) parseTail(data []byte) (*PacketTail, error) {
	if len(data) != TAIL_SIZE {
		return nil, fmt.Errorf("invalid tail size: expected %d, got %d", TAIL_SIZE, len(data))
	}

	tail := &PacketTail{
		HighPrecisionFlag: data[10],
		AzimuthFlag:       data[22],
		MotorSpeed:        binary.LittleEndian.Uint16(data[25:27]),
		Timestamp:         binary.LittleEndian.Uint32(data[27:31]),
		FactoryInfo:       data[31],
	}

	// Copy reserved fields
	copy(tail.Reserved1[:], data[0:10])
	copy(tail.Reserved2[:], data[11:22])
	copy(tail.Reserved3[:], data[23:25])

	return tail, nil
}

// blockToPoints converts a data block to individual points
func (p *Pandar40PParser) blockToPoints(block *DataBlock, blockIdx int, timestamp uint32) []Point {
	var points []Point

	// Convert timestamp to time.Time (assuming microseconds since epoch)
	packetTime := time.Unix(0, int64(timestamp)*1000) // Convert to nanoseconds

	// Base azimuth from block (in 0.01 degree units)
	baseAzimuth := float64(block.Azimuth) * AZIMUTH_RESOLUTION

	for channelIdx := 0; channelIdx < CHANNELS_PER_BLOCK; channelIdx++ {
		channelData := block.Channels[channelIdx]

		// Skip invalid measurements (distance 0 usually means no return)
		if channelData.Distance == 0 {
			continue
		}

		// Channel numbers are 1-based in the config
		channelNum := channelIdx + 1

		// Get angle corrections for this channel
		angleCorrection := p.config.AngleCorrections[channelIdx]
		firetimeCorrection := p.config.FiretimeCorrections[channelIdx]

		// Calculate actual azimuth with correction
		azimuth := baseAzimuth + angleCorrection.Azimuth
		if azimuth < 0 {
			azimuth += 360
		} else if azimuth >= 360 {
			azimuth -= 360
		}

		// Calculate distance in meters
		distance := float64(channelData.Distance) * DISTANCE_RESOLUTION

		// Calculate elevation
		elevation := angleCorrection.Elevation

		// Convert spherical coordinates to Cartesian
		azimuthRad := azimuth * math.Pi / 180.0
		elevationRad := elevation * math.Pi / 180.0

		x := distance * math.Cos(elevationRad) * math.Sin(azimuthRad)
		y := distance * math.Cos(elevationRad) * math.Cos(azimuthRad)
		z := distance * math.Sin(elevationRad)

		// Apply firetime correction to timestamp
		firetimeOffset := time.Duration(firetimeCorrection.FireTime * float64(time.Microsecond))
		pointTime := packetTime.Add(firetimeOffset)

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
