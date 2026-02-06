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

	// StartSeconds is the offset in seconds from the beginning of the PCAP file to start playback (default 0)
	StartSeconds float64

	// DurationSeconds is the duration in seconds to play from StartSeconds (default -1 means play until end)
	DurationSeconds float64

	// SensorID is used for caching debug foreground snapshots during replay.
	SensorID string

	// PacketForwarder forwards packets to a UDP destination (optional)
	PacketForwarder *PacketForwarder

	// ForegroundForwarder forwards foreground points to a separate port (optional)
	ForegroundForwarder *ForegroundForwarder

	// BackgroundManager performs foreground/background classification (required for ForegroundForwarder)
	BackgroundManager *lidar.BackgroundManager

	// WarmupPackets skips forwarding for the first N packets to seed background.
	WarmupPackets int

	// Debug range parameters for focused diagnostics
	DebugRingMin int     // Min ring index (inclusive, 0 = disabled)
	DebugRingMax int     // Max ring index (inclusive, 0 = disabled)
	DebugAzMin   float32 // Min azimuth degrees (inclusive, 0 = disabled)
	DebugAzMax   float32 // Max azimuth degrees (inclusive, 0 = disabled)

	// OnFrameCallback is called after each frame is processed with foreground extraction.
	// This can be used for sampling grid state for plotting.
	OnFrameCallback func(mgr *lidar.BackgroundManager, points []lidar.PointPolar)

	// PacketOffset is the 0-based packet index to seek to before starting
	// playback. Packets before this offset are skipped without processing.
	PacketOffset uint64

	// TotalPackets is the pre-counted total number of matching packets in
	// the PCAP file. When > 0, enables progress reporting via OnProgress.
	TotalPackets uint64

	// OnProgress is called periodically during replay with the current
	// packet index and total packet count, enabling seek-bar updates.
	OnProgress func(currentPacket, totalPackets uint64)
}

const (
	// Max points to buffer before forcing a flush to foreground forwarder
	maxForegroundBufferPoints = 1200 // Approx 10 blocks (1 full packet)
	// Max distinct source packets to buffer before flushing (to avoid latency)
	maxForegroundBufferPackets = 20
)

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
	var packetIndex uint64 // 0-based index across all matching packets
	packetCount := 0
	totalPoints := 0
	startTime := time.Now()
	warmupRemaining := config.WarmupPackets

	// Offset-based seek: skip packets until we reach PacketOffset
	skippingToOffset := config.PacketOffset > 0

	var firstPacketTime time.Time
	replayStartTime := time.Now()
	var startThreshold time.Time
	var endThreshold time.Time
	skippingToStart := config.StartSeconds > 0

	// Buffer for aggregating foreground points to reduce packet overhead
	var foregroundBuffer []lidar.PointPolar
	var bufferedPackets int

	for {
		select {
		case <-ctx.Done():
			// Flush remaining foreground buffer on exit
			if config.ForegroundForwarder != nil && len(foregroundBuffer) > 0 {
				config.ForegroundForwarder.ForwardForeground(foregroundBuffer)
			}
			log.Printf("PCAP real-time replay stopping due to context cancellation (processed %d packets)", packetCount)
			return ctx.Err()
		case packet := <-packetSource.Packets():
			if packet == nil {
				// End of PCAP file
				elapsed := time.Since(startTime)
				log.Printf("PCAP real-time replay complete: %d packets processed in %v (speed: %.1fx)", packetCount, elapsed, config.SpeedMultiplier)
				// Final progress callback
				if config.OnProgress != nil && config.TotalPackets > 0 {
					config.OnProgress(packetIndex, config.TotalPackets)
				}
				return nil
			}

			packetIndex++
			packetCount++

			// Offset-based seek: skip packets until we reach the requested offset
			if skippingToOffset && packetIndex < config.PacketOffset {
				continue
			}
			if skippingToOffset {
				skippingToOffset = false
				log.Printf("PCAP replay: seeked to packet offset %d", config.PacketOffset)
				replayStartTime = time.Now()
			}

			// Report progress periodically (every 100 packets)
			if config.OnProgress != nil && config.TotalPackets > 0 && packetIndex%100 == 0 {
				config.OnProgress(packetIndex, config.TotalPackets)
			}

			// Calculate timing for real-time replay
			captureTime := packet.Metadata().Timestamp

			if firstPacketTime.IsZero() {
				firstPacketTime = captureTime
				// Set start and end thresholds based on config
				if config.StartSeconds > 0 {
					startThreshold = firstPacketTime.Add(time.Duration(config.StartSeconds * float64(time.Second)))
				} else {
					// When no start offset, use first packet time as the baseline
					startThreshold = firstPacketTime
					// Reset replay start time for accurate pacing when not skipping
					replayStartTime = time.Now()
				}
				if config.DurationSeconds > 0 {
					// Duration is always relative to the effective start threshold
					endThreshold = startThreshold.Add(time.Duration(config.DurationSeconds * float64(time.Second)))
				}
				log.Printf("PCAP start: first_packet=%v, start_threshold=%v, end_threshold=%v, skipping_to_start=%v",
					firstPacketTime, startThreshold, endThreshold, skippingToStart)
			}

			// Skip packets before start threshold
			if skippingToStart && !startThreshold.IsZero() && captureTime.Before(startThreshold) {
				continue
			}
			if skippingToStart {
				skippingToStart = false
				log.Printf("PCAP replay: started at %.2fs offset", config.StartSeconds)
				replayStartTime = time.Now() // Reset start time for accurate speed reporting
			}

			// Stop if we've reached the end threshold
			if !endThreshold.IsZero() && captureTime.After(endThreshold) {
				log.Printf("PCAP replay complete: reached duration limit of %.2fs (packet_ts=%v, end_threshold=%v, packets=%d, delta=%.3fs)",
					config.DurationSeconds, captureTime, endThreshold, packetCount,
					captureTime.Sub(endThreshold).Seconds())
				return nil
			}

			// Log first few packets for debugging timestamp issues
			if packetCount <= 3 {
				log.Printf("PCAP packet #%d: capture_time=%v, first_packet=%v, delta_from_first=%.3fs",
					packetCount, captureTime, firstPacketTime, captureTime.Sub(firstPacketTime).Seconds())
			}

			// Real-time pacing: compare wall clock elapsed with PCAP time elapsed
			// This accounts for processing time and avoids cumulative lag
			if firstPacketTime != captureTime {
				// How much PCAP time has elapsed since the effective start?
				pcapElapsed := captureTime.Sub(startThreshold)
				// How much wall clock time should have elapsed at this speed?
				targetWallElapsed := time.Duration(float64(pcapElapsed) / config.SpeedMultiplier)
				// How much wall clock time has actually elapsed?
				actualWallElapsed := time.Since(replayStartTime)
				// Wait for the difference (if we're ahead of schedule)
				waitTime := targetWallElapsed - actualWallElapsed

				if waitTime > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(waitTime):
						// Continue
					}
				}
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
					lidar.Debugf("PCAP real-time replay: packet %d parsed -> 0 points", packetCount)
				} else {
					totalPoints += len(points)

					// Log progress every 1000 packets
					if packetCount%1000 == 0 {
						elapsed := time.Since(replayStartTime)
						originalDuration := captureTime.Sub(firstPacketTime)
						compressionRatio := float64(originalDuration) / float64(elapsed)

						lidar.Debugf("PCAP real-time replay: packet=%d, points=%d, total_points=%d, elapsed=%v, original_duration=%v, compression=%.1fx",
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

				// Foreground extraction & snapshot caching if background manager is available
				// IMPORTANT: Skip if frameBuilder is set, because the FrameBuilder callback
				// (from TrackingPipelineConfig) already handles background processing,
				// foreground extraction, snapshot caching, and forwarding. Processing here
				// would cause double-processing of points through the grid, corrupting
				// the running averages and causing false positives (trails).
				if config.BackgroundManager != nil && frameBuilder == nil {
					foregroundMask, err := config.BackgroundManager.ProcessFramePolarWithMask(points)
					if err != nil {
						log.Printf("Error extracting foreground points: %v", err)
					} else if len(foregroundMask) > 0 {
						foregroundPoints := lidar.ExtractForegroundPoints(points, foregroundMask)

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

						snapshotTS := time.Now()
						if len(points) > 0 && points[0].Timestamp > 0 {
							snapshotTS = time.Unix(0, points[0].Timestamp)
						}
						if config.SensorID != "" {
							lidar.StoreForegroundSnapshot(config.SensorID, snapshotTS, foregroundPoints, backgroundPolar, len(points), len(foregroundPoints))
						}

						// Warmup: seed background without forwarding until warmupRemaining hits zero
						if config.ForegroundForwarder != nil {
							if warmupRemaining > 0 {
								warmupRemaining--
								if len(foregroundPoints) > 0 && (warmupRemaining%100 == 0 || warmupRemaining < 5) {
									lidar.Debugf("[ForegroundForwarder] warmup skipping frame: remaining_packets=%d fg_points=%d total_points=%d", warmupRemaining, len(foregroundPoints), len(points))
								}
							} else if len(foregroundPoints) > 0 {
								// Filter by debug range if configured
								pointsToForward := foregroundPoints
								if config.BackgroundManager != nil {
									params := config.BackgroundManager.GetParams()
									if params.HasDebugRange() {
										filtered := make([]lidar.PointPolar, 0, len(foregroundPoints))
										for _, p := range foregroundPoints {
											// Channel is 1-based in PointPolar, ring is 0-based in params
											if params.IsInDebugRange(p.Channel-1, p.Azimuth) {
												filtered = append(filtered, p)
											}
										}
										pointsToForward = filtered
									}
								}

								if len(pointsToForward) > 0 {
									// Accumulate points in buffer
									// Stripping UDPSequence to allow better packing is done implicitly
									// if the Forwarder chooses to ignore it, but we can help by
									// aggregating multiple source packets into one Forward call.
									// However, we must ensure we don't hold them too long.

									// Clear UDPSequence to aid packing in Forwarder
									for i := range pointsToForward {
										pointsToForward[i].UDPSequence = 0
									}

									foregroundBuffer = append(foregroundBuffer, pointsToForward...)
									bufferedPackets++

									// Flush if buffer is full or enough distinct packets collected
									if len(foregroundBuffer) >= maxForegroundBufferPoints || bufferedPackets >= maxForegroundBufferPackets {
										config.ForegroundForwarder.ForwardForeground(foregroundBuffer)
										foregroundBuffer = nil // Reallocate or clear? nil lets GC handle old slice
										foregroundBuffer = make([]lidar.PointPolar, 0, maxForegroundBufferPoints)
										bufferedPackets = 0
									}
								}

								if packetCount%1000 == 0 {
									fgRatio := float64(len(foregroundPoints)) / float64(len(points))
									lidar.Debugf("Foreground extraction: %d/%d points (%.1f%%)",
										len(foregroundPoints), len(points), fgRatio*100)
								}
							}
						}

						// Call frame callback for grid sampling (e.g., for plotting)
						if config.OnFrameCallback != nil {
							config.OnFrameCallback(config.BackgroundManager, points)
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
