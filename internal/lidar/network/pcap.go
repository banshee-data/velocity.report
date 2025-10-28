//go:build pcap
// +build pcap

package network

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcap"
)

// ReadPCAPFile reads and processes LiDAR packets from a PCAP file
// This function is only available when building with the 'pcap' build tag
func ReadPCAPFile(ctx context.Context, pcapFile string, udpPort int, parser Parser, frameBuilder FrameBuilder, stats PacketStatsInterface) error {
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
	log.Printf("PCAP BPF filter set: %s", filterStr)

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetCount := 0
	startTime := time.Now()

	for {
		select {
		case <-ctx.Done():
			log.Printf("PCAP reader stopping due to context cancellation (processed %d packets)", packetCount)
			return ctx.Err()
		case packet := <-packetSource.Packets():
			if packet == nil {
				// End of PCAP file
				elapsed := time.Since(startTime)
				log.Printf("PCAP file reading complete: %d packets processed in %v", packetCount, elapsed)
				return nil
			}

			packetCount++

			// Extract UDP layer
			udpLayer := packet.Layer(layers.LayerTypeUDP)
			if udpLayer == nil {
				continue // Skip non-UDP packets (shouldn't happen with BPF filter)
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

			// Parse and process the packet if parser is provided
			if parser != nil {
				points, err := parser.ParsePacket(payload)
				if err != nil {
					log.Printf("Error parsing PCAP packet %d: %v", packetCount, err)
					continue
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
			}

			// Log progress periodically
			if packetCount%10000 == 0 {
				elapsed := time.Since(startTime)
				log.Printf("PCAP progress: %d packets processed in %v (%.0f pkt/s)",
					packetCount, elapsed, float64(packetCount)/elapsed.Seconds())
			}
		}
	}
}
