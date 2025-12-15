package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"sort"
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
	frameCount   uint64
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
				lidar.Debugf("Foreground forwarder stopping (sent %d packets)", f.packetCount)
				return
			case points := <-f.channel:
				if len(points) == 0 {
					continue
				}

				f.frameCount++

				// Encode points as Pandar40P packets
				packets, err := f.encodePointsAsPackets(points)
				if err != nil {
					lidar.Debugf("Error encoding foreground points: %v", err)
					continue
				}

				// Send each packet
				for _, packet := range packets {
					_, err := f.conn.Write(packet)
					if err != nil {
						lidar.Debugf("Error forwarding foreground packet: %v", err)
					} else {
						f.packetCount++
					}
				}

				if f.frameCount <= 5 || f.frameCount%100 == 0 {
					lidar.Debugf("[ForegroundForwarder] sent frame=%d packets=%d points=%d total_packets=%d dest=%s",
						f.frameCount, len(packets), len(points), f.packetCount, f.address)
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
		MAX_DISTANCE_METERS   = 200.0                                  // Pandar40P max range
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

		// Sort by azimuth to keep buckets contiguous
		sort.Slice(packetPoints, func(a, b int) bool {
			return packetPoints[a].Azimuth < packetPoints[b].Azimuth
		})

		azBucketSize := 360.0 / float64(BLOCKS_PER_PACKET)
		filledBlocks := 0
		invalidChannelCount := 0
		const azimuthEpsilon = 0.01 // Small tolerance for boundary comparisons

		// Diagnostic metrics for this packet
		var minDist, maxDist, sumDist float64 = math.MaxFloat64, 0.0, 0.0
		var minAz, maxAz float64 = 360.0, 0.0
		distCount := 0

		// Encode data blocks (10 blocks) using azimuth buckets rather than BlockID.
		for blockIdx := 0; blockIdx < BLOCKS_PER_PACKET; blockIdx++ {
			blockOffset := blockIdx * BLOCK_SIZE
			binary.LittleEndian.PutUint16(packet[blockOffset:], 0xFFEE)

			// Collect points in this azimuth bucket
			minAz := float64(blockIdx) * azBucketSize
			maxAz := minAz + azBucketSize
			bucket := make([]lidar.PointPolar, 0)
			for _, p := range packetPoints {
				// Normalize azimuth to [0, 360) range
				az := math.Mod(p.Azimuth+360.0, 360.0)

				// Handle wrap-around at 0°/360° boundary for last bucket
				// Last bucket (9) covers [324°, 360°) but should also include [0°, epsilon)
				if blockIdx == BLOCKS_PER_PACKET-1 {
					// Special handling for bucket 9: include [324°, 360°) AND [0°, epsilon)
					if (az >= minAz && az < 360.0) || (az >= 0.0 && az < azimuthEpsilon) {
						bucket = append(bucket, p)
					}
				} else {
					// Normal bucket: [minAz, maxAz) with small epsilon tolerance on upper bound
					if az >= minAz && az < maxAz+azimuthEpsilon {
						bucket = append(bucket, p)
					}
				}
			}

			// Choose block azimuth: median of bucket or center of bucket if empty
			blockAzimuth := minAz + azBucketSize/2
			if len(bucket) > 0 {
				mid := len(bucket) / 2
				blockAzimuth = bucket[mid].Azimuth
			}
			azimuthScaled := uint16(math.Mod(blockAzimuth*100.0+36000.0, 36000.0))
			binary.LittleEndian.PutUint16(packet[blockOffset+2:], azimuthScaled)

			// Channel data (40 channels × 3 bytes)
			channelOffset := blockOffset + 4
			blockHasData := false
			for ch := 0; ch < CHANNELS_PER_BLOCK; ch++ {
				var distance uint16 = 0xFFFF
				var intensity uint8 = 0
				for _, p := range bucket {
					// Validate channel range before matching
					if p.Channel < 0 || p.Channel >= CHANNELS_PER_BLOCK {
						invalidChannelCount++
						continue // Skip invalid channels
					}

					if p.Channel == ch {
						d := p.Distance
						switch {
						case d <= 0:
							distance = 0xFFFF
						case d > MAX_DISTANCE_METERS:
							distance = 0xFFFE // Clamp to max representable distance
						default:
							// Pandar40P distance encoding: 1 unit = 0.5cm = 0.005m
							// So: distance_units = distance_meters / 0.005 = distance_meters * 200
							// (Previous code used *500 which was incorrect - that's 0.2cm resolution)
							distScaled := d * 200.0
							if distScaled > 65534.0 {
								distance = 0xFFFE
							} else {
								distance = uint16(distScaled)
							}

							// Update diagnostic metrics
							if d < minDist {
								minDist = d
							}
							if d > maxDist {
								maxDist = d
							}
							sumDist += d
							distCount++
						}
						intensity = p.Intensity
						blockHasData = true

						// Track azimuth range
						az := math.Mod(p.Azimuth+360.0, 360.0)
						if az < minAz {
							minAz = az
						}
						if az > maxAz {
							maxAz = az
						}

						break
					}
				}

				idx := channelOffset + (ch * 3)
				binary.LittleEndian.PutUint16(packet[idx:], distance)
				packet[idx+2] = intensity
			}

			if blockHasData {
				filledBlocks++
			}
		}

		emptyBlocks := BLOCKS_PER_PACKET - filledBlocks

		// Comprehensive diagnostic logging
		if invalidChannelCount > 0 {
			lidar.Debugf("[ForegroundForwarder] WARNING: %d invalid channel values (< 0 or >= 40) in packet", invalidChannelCount)
		}

		if filledBlocks < 3 {
			lidar.Debugf("[ForegroundForwarder] WARNING: Very sparse packet (%d/%d blocks filled, %d points) - may indicate encoding issue",
				filledBlocks, BLOCKS_PER_PACKET, len(packetPoints))
		} else if emptyBlocks > BLOCKS_PER_PACKET/2 {
			lidar.Debugf("[ForegroundForwarder] Sparse packet (%d/%d blocks filled, %d empty)",
				filledBlocks, BLOCKS_PER_PACKET, emptyBlocks)
		}

		// Log packet statistics periodically (every 50th packet)
		if f.packetCount%50 == 0 && distCount > 0 {
			avgDist := sumDist / float64(distCount)
			azRange := maxAz - minAz
			if azRange < 0 {
				azRange += 360.0 // Handle wrap-around
			}
			lidar.Debugf("[ForegroundForwarder] Packet stats: %d/%d blocks filled, %d points | Distance: %.1fm-%.1fm (avg %.1fm) | Azimuth: [%.1f°-%.1f°] range %.1f° | Invalid ch: %d",
				filledBlocks, BLOCKS_PER_PACKET, len(packetPoints), minDist, maxDist, avgDist, minAz, maxAz, azRange, invalidChannelCount)
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
