package network

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
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
			case points, ok := <-f.channel:
				if !ok {
					log.Printf("CRITICAL BUG: ForegroundForwarder is spinning on closed channel! Context Err: %v", ctx.Err())
					time.Sleep(1 * time.Second)
					continue
				}
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
// encodePointsAsPackets encodes foreground points into Pandar40P-compatible UDP packets.
// It reconstructs the original packet structure using UDPSequence and BlockID to ensure
// azimuths are preserved exactly as they were received.
func (f *ForegroundForwarder) encodePointsAsPackets(points []lidar.PointPolar) ([][]byte, error) {
	if len(points) == 0 {
		return nil, nil
	}

	// Sort points to ensure we process them in packet order
	// Primary: UDPSequence (if present), Secondary: Timestamp, Tertiary: BlockID
	sort.Slice(points, func(i, j int) bool {
		if points[i].UDPSequence != points[j].UDPSequence {
			return points[i].UDPSequence < points[j].UDPSequence
		}
		if points[i].Timestamp != points[j].Timestamp {
			return points[i].Timestamp < points[j].Timestamp
		}
		return points[i].BlockID < points[j].BlockID
	})

	var packets [][]byte

	// Group points into packets
	type PacketBatch struct {
		Points      []lidar.PointPolar
		HasSequence bool
		Sequence    uint32
	}

	var currentBatch *PacketBatch
	var lastBlockID int = -1
	var lastTimestamp int64 = -1

	for _, p := range points {
		isNewPacket := false

		if currentBatch == nil {
			isNewPacket = true
		} else {
			// Detection logic
			if p.UDPSequence != 0 {
				// If sequence numbers are used, they are the authority
				if p.UDPSequence != currentBatch.Sequence {
					isNewPacket = true
				}
			} else {
				// Heuristic for non-sequenced packets
				// 1. BlockID reset (e.g. 9 -> 0)
				if p.BlockID < lastBlockID {
					isNewPacket = true
				}
				// 2. Timestamp gap > 200us (approx packet duration is 166us)
				if lastTimestamp != -1 && (p.Timestamp-lastTimestamp) > 200000 {
					isNewPacket = true
				}
			}
		}

		if isNewPacket {
			// Finalize current batch
			if currentBatch != nil {
				pkt, err := f.buildPacket(currentBatch.Points, currentBatch.HasSequence, currentBatch.Sequence)
				if err == nil {
					packets = append(packets, pkt)
				}
			}

			// Start new batch
			currentBatch = &PacketBatch{
				Points:      []lidar.PointPolar{p},
				HasSequence: p.UDPSequence != 0,
				Sequence:    p.UDPSequence,
			}
		} else {
			currentBatch.Points = append(currentBatch.Points, p)
		}

		lastBlockID = p.BlockID
		lastTimestamp = p.Timestamp
	}

	// Finalize last batch
	if currentBatch != nil {
		pkt, err := f.buildPacket(currentBatch.Points, currentBatch.HasSequence, currentBatch.Sequence)
		if err == nil {
			packets = append(packets, pkt)
		}
	}

	return packets, nil
}

func (f *ForegroundForwarder) buildPacket(points []lidar.PointPolar, hasSequence bool, sequence uint32) ([]byte, error) {
	size := 1262
	if hasSequence {
		size = 1266
	}
	packet := make([]byte, size)

	// Group points by BlockID
	blocks := make(map[int][]lidar.PointPolar)
	for _, p := range points {
		blocks[p.BlockID] = append(blocks[p.BlockID], p)
	}

	for blockIdx := 0; blockIdx < 10; blockIdx++ {
		blockOffset := blockIdx * 124

		// Preamble
		binary.LittleEndian.PutUint16(packet[blockOffset:], 0xFFEE)

		blockPoints, exists := blocks[blockIdx]
		if !exists {
			// Empty block - leave Azimuth as 0, Channels as 0
			continue
		}

		// Azimuth from first point (RawBlockAzimuth)
		// We assume all points in block have same RawBlockAzimuth
		azimuth := blockPoints[0].RawBlockAzimuth
		binary.LittleEndian.PutUint16(packet[blockOffset+2:], azimuth)

		// Channels
		for _, p := range blockPoints {
			if p.Channel < 1 || p.Channel > 40 {
				continue
			}

			// Channel offset: 4 header + (channel-1)*3
			chOffset := blockOffset + 4 + (p.Channel-1)*3

			// Distance
			d := p.Distance
			var distVal uint16
			if d <= 0 {
				distVal = 0
			} else if d > 200.0 {
				distVal = 0xFFFE
			} else {
				distVal = uint16(d * 250.0) // 4mm resolution
			}

			binary.LittleEndian.PutUint16(packet[chOffset:], distVal)
			packet[chOffset+2] = p.Intensity
		}
	}

	// Reconstruct Tail
	if len(points) > 0 {
		firstPt := points[0]
		ts := time.Unix(0, firstPt.Timestamp)

		// Tail starts at 1240
		tailOffset := 1240

		// Motor Speed (bytes 8-9)
		motorSpeed := uint16(f.sensorConfig.MotorSpeedRPM)
		binary.LittleEndian.PutUint16(packet[tailOffset+8:], motorSpeed)

		// Date (bytes 16-21)
		packet[tailOffset+16] = uint8(ts.Year() - 2000)
		packet[tailOffset+17] = uint8(ts.Month())
		packet[tailOffset+18] = uint8(ts.Day())
		packet[tailOffset+19] = uint8(ts.Hour())
		packet[tailOffset+20] = uint8(ts.Minute())
		packet[tailOffset+21] = uint8(ts.Second())

		// Timestamp (us)
		us := uint32(ts.Nanosecond() / 1000)
		binary.LittleEndian.PutUint32(packet[tailOffset+10:], us)

		// ReturnMode (byte 14) - 0x37 for strongest return
		packet[tailOffset+14] = 0x37
	}

	if hasSequence {
		binary.LittleEndian.PutUint32(packet[1262:], sequence)
	}

	return packet, nil
}

// Close closes the UDP connection and channel.
func (f *ForegroundForwarder) Close() error {
	close(f.channel)
	return f.conn.Close()
}
