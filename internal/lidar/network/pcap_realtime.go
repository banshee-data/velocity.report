//go:build pcap
// +build pcap

package network

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// RealtimeReplayConfig configures real-time PCAP replay behavior.
type RealtimeReplayConfig struct {
	// SpeedMultiplier controls replay speed (1.0 = real-time, 2.0 = 2x speed, 0.5 = half speed)
	SpeedMultiplier float64

	// SensorID is used for caching debug foreground snapshots during replay.
	SensorID string

	// PacketForwarder forwards packets to a UDP destination (optional)
	PacketForwarder *PacketForwarder

	// ForegroundForwarder forwards foreground points to a separate port (optional)
	ForegroundForwarder *ForegroundForwarder

	// BackgroundManager performs foreground/background classification (required for ForegroundForwarder)
	BackgroundManager *lidar.BackgroundManager
}

// ReadPCAPFileRealtime reads and replays a PCAP file in real-time, respecting original packet timing.
// This allows live network forwarding of PCAP data for real-time analysis.
// Packets are forwarded via PacketForwarder if configured.
func ReadPCAPFileRealtime(ctx context.Context, pcapFile string, udpPort int, parser Parser, frameBuilder FrameBuilder, stats PacketStatsInterface, config RealtimeReplayConfig) error {
	// Default speed multiplier
	if config.SpeedMultiplier <= 0 {
		config.SpeedMultiplier = 1.0
	}

	// Open PCAP file
	handle, err := pcap.OpenOffline(pcapFile)
	if err != nil {
		return fmt.Errorf("failed to open PCAP file %s: %w", pcapFile, err)
	}
	defer handle.Close()

	// Set BPF filter to only capture UDP packets on the specified port
	filterStr := fmt.Sprintf("udp port %d", udpPort)
	if err := handle.SetBPFFilter(filterStr); err != nil {
		return fmt.Errorf("failed to set BPF filter '%s': %w", filterStr, err)
	}
	log.Printf("PCAP real-time replay: BPF filter set: %s (speed: %.1fx)", filterStr, config.SpeedMultiplier)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetCount := 0
	totalPoints := 0
	startTime := time.Now()

	var firstPacketTime time.Time
	var lastPacketTime time.Time
	replayStartTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			log.Printf("PCAP real-time replay stopping due to context cancellation (processed %d packets)", packetCount)
			return ctx.Err()
		case packet := <-packetSource.Packets():
			if packet == nil {
				// End of PCAP file
				elapsed := time.Since(startTime)
				log.Printf("PCAP real-time replay complete: %d packets processed in %v (speed: %.1fx)", packetCount, elapsed, config.SpeedMultiplier)
				return nil
			}

			packetCount++

			// Calculate timing for real-time replay
			captureTime := packet.Metadata().Timestamp

			if firstPacketTime.IsZero() {
				firstPacketTime = captureTime
				lastPacketTime = captureTime
			} else {
				// Calculate delay since last packet (scaled by speed multiplier)
				delay := captureTime.Sub(lastPacketTime)
				scaledDelay := time.Duration(float64(delay) / config.SpeedMultiplier)

				// Wait for scaled delay to maintain timing
				if scaledDelay > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(scaledDelay):
						// Continue
					}
				}

				lastPacketTime = captureTime
			}

			// Extract UDP layer
			udpLayer := packet.Layer(layers.LayerTypeUDP)
			if udpLayer == nil {
				continue // Skip non-UDP packets
			}

			udp, ok := udpLayer.(*layers.UDP)
			if !ok {
				continue
			}

			// Extract payload (LiDAR data)
			payload := udp.Payload
			if len(payload) == 0 {
				continue
			}

			// Record packet statistics
			if stats != nil {
				stats.AddPacket(len(payload))
			}

			// Forward packet to UDP destination if configured
			if config.PacketForwarder != nil {
				config.PacketForwarder.ForwardAsync(payload)
			}

			// Parse and process the packet if parser is provided
			if parser != nil {
				// Use capture timestamp for time-aware processing
				if tsParser, ok := parser.(interface{ SetPacketTime(time.Time) }); ok {
					tsParser.SetPacketTime(captureTime)
				}

				points, err := parser.ParsePacket(payload)
				if err != nil {
					log.Printf("Error parsing PCAP packet %d: %v", packetCount, err)
					continue
				}

				if len(points) == 0 {
					log.Printf("PCAP real-time replay: packet %d parsed -> 0 points", packetCount)
				} else {
					totalPoints += len(points)

					// Log progress every 1000 packets
					if packetCount%1000 == 0 {
						elapsed := time.Since(replayStartTime)
						originalDuration := captureTime.Sub(firstPacketTime)
						compressionRatio := float64(originalDuration) / float64(elapsed)

						log.Printf("PCAP real-time replay: packet=%d, points=%d, total_points=%d, elapsed=%v, original_duration=%v, compression=%.1fx",
							packetCount, len(points), totalPoints, elapsed, originalDuration, compressionRatio)
					}
				}

				if stats != nil {
					stats.AddPoints(len(points))
				}

				if frameBuilder != nil {
					frameBuilder.AddPointsPolar(points)
					motorSpeed := parser.GetLastMotorSpeed()
					if motorSpeed > 0 {
						frameBuilder.SetMotorSpeed(motorSpeed)
					}
				}

				// Forward foreground points if forwarder configured
				if config.ForegroundForwarder != nil && config.BackgroundManager != nil {
					// Extract foreground points using background subtraction
					foregroundMask, err := config.BackgroundManager.ProcessFramePolarWithMask(points)
					if err != nil {
						log.Printf("Error extracting foreground points: %v", err)
					} else if len(foregroundMask) > 0 {
						// Extract only foreground points
						foregroundPoints := lidar.ExtractForegroundPoints(points, foregroundMask)

						// Build background subset for debugging/export without overwhelming charts
						backgroundPolar := make([]lidar.PointPolar, 0, len(points)-len(foregroundPoints))
						for i, isForeground := range foregroundMask {
							if !isForeground {
								backgroundPolar = append(backgroundPolar, points[i])
							}
						}
						const maxBackgroundChartPoints = 5000
						if len(backgroundPolar) > maxBackgroundChartPoints {
							stride := len(backgroundPolar) / maxBackgroundChartPoints
							if stride < 1 {
								stride = 1
							}
							downsampled := make([]lidar.PointPolar, 0, maxBackgroundChartPoints)
							for i := 0; i < len(backgroundPolar); i += stride {
								downsampled = append(downsampled, backgroundPolar[i])
								if len(downsampled) >= maxBackgroundChartPoints {
									break
								}
							}
							backgroundPolar = downsampled
						}

						// Cache latest foreground snapshot for export/debug endpoints
						snapshotTS := time.Now()
						if len(points) > 0 && points[0].Timestamp > 0 {
							snapshotTS = time.Unix(0, points[0].Timestamp)
						}
						if config.SensorID != "" {
							lidar.StoreForegroundSnapshot(config.SensorID, snapshotTS, foregroundPoints, backgroundPolar, len(points), len(foregroundPoints))
						}

						// Forward foreground points to port 2370
						if len(foregroundPoints) > 0 {
							config.ForegroundForwarder.ForwardForeground(foregroundPoints)

							// Log foreground extraction stats periodically
							if packetCount%1000 == 0 {
								fgRatio := float64(len(foregroundPoints)) / float64(len(points))
								log.Printf("Foreground extraction: %d/%d points (%.1f%%)",
									len(foregroundPoints), len(points), fgRatio*100)
							}
						}
					}
				}
			}

			// Log progress periodically
			if packetCount%10000 == 0 {
				elapsed := time.Since(startTime)
				log.Printf("PCAP real-time replay progress: %d packets in %v (%.0f pkt/s, speed: %.1fx)",
					packetCount, elapsed, float64(packetCount)/elapsed.Seconds(), config.SpeedMultiplier)
			}
		}
	}
}
