package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
)

// ForegroundForwarder handles forwarding of foreground-only point clouds as UDP packets.
// Points are encoded as Pandar40P-compatible packets on a separate port (default 2370).
type ForegroundForwarder struct {
	conn         *net.UDPConn
	channel      chan []lidar.PointPolar
	address      string
	port         int
	sensorConfig *SensorConfig
	packetCount  uint64
}

// SensorConfig holds sensor configuration for packet encoding.
type SensorConfig struct {
	MotorSpeedRPM float64
	Channels      int
}

// DefaultSensorConfig returns default configuration for Pandar40P.
func DefaultSensorConfig() *SensorConfig {
	return &SensorConfig{
		MotorSpeedRPM: 600.0,
		Channels:      40,
	}
}

// NewForegroundForwarder creates a new foreground packet forwarder.
func NewForegroundForwarder(addr string, port int, config *SensorConfig) (*ForegroundForwarder, error) {
	if config == nil {
		config = DefaultSensorConfig()
	}

	forwardAddress := fmt.Sprintf("%s:%d", addr, port)
	forwardUDPAddr, err := net.ResolveUDPAddr("udp", forwardAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve foreground forward address: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, forwardUDPAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to create foreground forward connection: %v", err)
	}

	return &ForegroundForwarder{
		conn:         conn,
		channel:      make(chan []lidar.PointPolar, 100), // Buffer 100 point cloud frames
		address:      forwardAddress,
		port:         port,
		sensorConfig: config,
		packetCount:  0,
	}, nil
}

// Start begins the foreground forwarding goroutine.
func (f *ForegroundForwarder) Start(ctx context.Context) {
	go func() {
		log.Printf("Foreground forwarding started to %s (port %d)", f.address, f.port)

		for {
			select {
			case <-ctx.Done():
				log.Printf("Foreground forwarder stopping (sent %d packets)", f.packetCount)
				return
			case points := <-f.channel:
				if len(points) == 0 {
					continue
				}

				// Encode points as Pandar40P packets
				packets, err := f.encodePointsAsPackets(points)
				if err != nil {
					log.Printf("Error encoding foreground points: %v", err)
					continue
				}

				// Send each packet
				for _, packet := range packets {
					_, err := f.conn.Write(packet)
					if err != nil {
						log.Printf("Error forwarding foreground packet: %v", err)
					} else {
						f.packetCount++
					}
				}
			}
		}
	}()
}

// ForwardForeground queues foreground points for forwarding.
func (f *ForegroundForwarder) ForwardForeground(points []lidar.PointPolar) {
	if len(points) == 0 {
		return
	}

	// Make a copy to avoid data races
	pointsCopy := make([]lidar.PointPolar, len(points))
	copy(pointsCopy, points)

	// Non-blocking send
	select {
	case f.channel <- pointsCopy:
		// Successfully queued
	default:
		// Drop if buffer full (prevents blocking)
		log.Printf("Warning: Foreground forwarding buffer full, dropping %d points", len(points))
	}
}

// encodePointsAsPackets encodes foreground points into Pandar40P-compatible UDP packets.
// Multiple packets may be generated if there are more points than fit in one packet.
func (f *ForegroundForwarder) encodePointsAsPackets(points []lidar.PointPolar) ([][]byte, error) {
	const (
		PACKET_SIZE           = 1262
		BLOCKS_PER_PACKET     = 10
		BLOCK_SIZE            = 124
		CHANNELS_PER_BLOCK    = 40
		TAIL_SIZE             = 22
		MAX_POINTS_PER_PACKET = BLOCKS_PER_PACKET * CHANNELS_PER_BLOCK // 400 points max
	)

	// Calculate number of packets needed
	numPackets := (len(points) + MAX_POINTS_PER_PACKET - 1) / MAX_POINTS_PER_PACKET
	packets := make([][]byte, 0, numPackets)

	for i := 0; i < len(points); i += MAX_POINTS_PER_PACKET {
		end := i + MAX_POINTS_PER_PACKET
		if end > len(points) {
			end = len(points)
		}

		packet := make([]byte, PACKET_SIZE)
		packetPoints := points[i:end]

		// Encode data blocks (10 blocks)
		for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
			blockOffset := blockIdx * BLOCK_SIZE

			// Block preamble (0xFFEE)
			binary.LittleEndian.PutUint16(packet[blockOffset:], 0xFFEE)

			// Block azimuth: prefer the first point with matching BlockID; otherwise fall back
			var blockAzimuth float64
			azFound := false
			for _, p := range packetPoints {
				if p.BlockID == blockIdx {
					blockAzimuth = p.Azimuth
					azFound = true
					break
				}
			}
			if !azFound && blockIdx < len(packetPoints) {
				blockAzimuth = packetPoints[blockIdx].Azimuth
			}
			azimuthScaled := uint16(math.Mod(blockAzimuth*100.0+36000.0, 36000.0))
			binary.LittleEndian.PutUint16(packet[blockOffset+2:], azimuthScaled)

			// Channel data (40 channels × 3 bytes)
			channelOffset := blockOffset + 4
			for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
				// Default: no-return marker
				var distance uint16 = 0xFFFF
				var intensity uint8 = 0

				// Find point for this channel in this block
				for _, p := range packetPoints {
					if p.Channel == ch && p.BlockID == blockIdx {
						// Distance in 2mm resolution
						distance = uint16(p.Distance * 500)
						intensity = p.Intensity
						break
					}
				}

				// Encode: 2 bytes distance + 1 byte intensity
				idx := channelOffset + (ch * 3)
				binary.LittleEndian.PutUint16(packet[idx:], distance)
				packet[idx+2] = intensity
			}
		}

		// Encode tail (22 bytes at offset 1240)
		tailOffset := BLOCKS_PER_PACKET * BLOCK_SIZE

		// Reserved fields
		for i := 0; i < 8; i++ {
			if i != 5 {
				packet[tailOffset+i] = 0
			}
		}

		// HighTempFlag (byte 5)
		packet[tailOffset+5] = 0

		// MotorSpeed (bytes 8-9) in 0.01 RPM units
		motorSpeedEncoded := uint16(math.Round(f.sensorConfig.MotorSpeedRPM * 100))
		binary.LittleEndian.PutUint16(packet[tailOffset+8:], motorSpeedEncoded)

		// Timestamp (bytes 10-13)
		now := time.Now()
		timestampMicros := uint32(now.UnixNano() / 1000)
		binary.LittleEndian.PutUint32(packet[tailOffset+10:], timestampMicros)

		// ReturnMode (byte 14) - 0x37 for strongest return
		packet[tailOffset+14] = 0x37

		// FactoryInfo (byte 15)
		packet[tailOffset+15] = 0

		// DateTime (bytes 16-21)
		packet[tailOffset+16] = uint8(now.Year() - 2000)
		packet[tailOffset+17] = uint8(now.Month())
		packet[tailOffset+18] = uint8(now.Day())
		packet[tailOffset+19] = uint8(now.Hour())
		packet[tailOffset+20] = uint8(now.Minute())
		packet[tailOffset+21] = uint8(now.Second())

		packets = append(packets, packet)
	}

	return packets, nil
}

// Close closes the UDP connection and channel.
func (f *ForegroundForwarder) Close() error {
	close(f.channel)
	return f.conn.Close()
}
