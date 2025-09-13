package monitor

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// StatsSnapshot represents a snapshot of current statistics
type StatsSnapshot struct {
	PacketsPerSec float64
	MBPerSec      float64
	PointsPerSec  float64
	DroppedCount  int64
	Timestamp     time.Time
	ParseEnabled  bool
}

// PacketStats tracks packet statistics with thread-safe operations
type PacketStats struct {
	mu             sync.Mutex
	packetCount    int64
	byteCount      int64
	droppedCount   int64
	pointCount     int64
	lastReset      time.Time
	startTime      time.Time
	latestSnapshot *StatsSnapshot
}

// NewPacketStats creates a new PacketStats instance
func NewPacketStats() *PacketStats {
	now := time.Now()
	return &PacketStats{
		lastReset: now,
		startTime: now,
	}
}

// AddPacket increments packet count and byte count
func (ps *PacketStats) AddPacket(bytes int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.packetCount++
	ps.byteCount += int64(bytes)
}

// AddDropped increments dropped packet count
func (ps *PacketStats) AddDropped() {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.droppedCount++
}

// AddPoints increments parsed point count
func (ps *PacketStats) AddPoints(count int) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.pointCount += int64(count)
}

// GetAndReset returns current stats and resets counters
func (ps *PacketStats) GetAndReset() (packets int64, bytes int64, dropped int64, points int64, duration time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	duration = now.Sub(ps.lastReset)
	packets = ps.packetCount
	bytes = ps.byteCount
	dropped = ps.droppedCount
	points = ps.pointCount

	ps.packetCount = 0
	ps.byteCount = 0
	ps.droppedCount = 0
	ps.pointCount = 0
	ps.lastReset = now

	return
}

// LogStats logs formatted statistics and stores snapshot for web interface
func (ps *PacketStats) LogStats(parsePackets bool) {
	packets, bytes, dropped, points, duration := ps.GetAndReset()
	if packets > 0 || dropped > 0 {
		packetsPerSec := float64(packets) / duration.Seconds()
		mbPerSec := float64(bytes) / duration.Seconds() / (1024 * 1024)
		pointsPerSec := float64(points) / duration.Seconds()

		// Store snapshot for web interface
		ps.mu.Lock()
		ps.latestSnapshot = &StatsSnapshot{
			PacketsPerSec: packetsPerSec,
			MBPerSec:      mbPerSec,
			PointsPerSec:  pointsPerSec,
			DroppedCount:  dropped,
			Timestamp:     time.Now(),
			ParseEnabled:  parsePackets,
		}
		ps.mu.Unlock()

		var logMsg string
		if parsePackets && points > 0 {
			logMsg = fmt.Sprintf("Lidar stats (/sec): %.2f MB, %.1f packets, %s points",
				mbPerSec, packetsPerSec, FormatWithCommas(int64(pointsPerSec)))
		} else {
			logMsg = fmt.Sprintf("Lidar stats (/sec): %.2f MB, %.1f packets",
				mbPerSec, packetsPerSec)
		}

		if dropped > 0 {
			logMsg += fmt.Sprintf(", %d dropped on forward", dropped)
		}

		log.Print(logMsg)
	}
}

// GetUptime returns the time since the stats were created
func (ps *PacketStats) GetUptime() time.Duration {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	return time.Since(ps.startTime)
}

// GetLatestSnapshot returns the most recent stats snapshot for web interface
func (ps *PacketStats) GetLatestSnapshot() *StatsSnapshot {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	if ps.latestSnapshot == nil {
		return nil
	}
	// Return a copy to avoid race conditions
	snapshot := *ps.latestSnapshot
	return &snapshot
}

// FormatWithCommas formats a number with thousands separators
func FormatWithCommas(n int64) string {
	str := fmt.Sprintf("%d", n)
	if len(str) <= 3 {
		return str
	}

	result := ""
	for i, char := range str {
		if i > 0 && (len(str)-i)%3 == 0 {
			result += ","
		}
		result += string(char)
	}
	return result
}
