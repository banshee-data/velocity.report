package network

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// ProgressStats decorates a PacketStatsInterface to print a wall-clock-paced
// progress line during a long PCAP read, then forwards every call to the inner
// stats. The percentage is processed UDP-payload bytes over the capture file
// size — an exact-input estimate (payload is the bulk of a dense LiDAR capture)
// that needs no up-front packet count, which on a multi-GB file would itself
// take over a minute.
type ProgressStats struct {
	inner      PacketStatsInterface
	fileSize   int64
	interval   time.Duration
	label      string
	out        io.Writer
	packetOnly bool

	mu           sync.Mutex
	payloadBytes int64
	packets      int64
	points       int64
	rpm          uint16    // most recent motor speed (revolutions per minute)
	captureNow   time.Time // capture time of the most recent packet
	start        time.Time
	lastReport   time.Time
	// Snapshots taken at the last printed report, for per-interval rates.
	captureAtReport time.Time
	pointsAtReport  int64
	haveCapture     bool
}

// ProgressObserver is an optional richer hook a PacketStatsInterface may
// implement to receive each packet's capture time and motor speed, so progress
// output can report RPM and points-per-frame. ReadPCAPFile calls it per packet
// when the stats sink implements it.
type ProgressObserver interface {
	ObserveProgress(captureTime time.Time, motorRPM uint16)
}

// NewProgressStats wraps inner so a progress line is emitted to out at most once
// per interval. interval <= 0 disables progress entirely and returns inner
// undecorated. fileSize is the capture size in bytes (from os.Stat); pass 0 to
// suppress the percentage when the size is unknown.
func NewProgressStats(inner PacketStatsInterface, fileSize int64, interval time.Duration, label string, out io.Writer) PacketStatsInterface {
	if interval <= 0 {
		return inner
	}
	now := time.Now()
	return &ProgressStats{
		inner:      inner,
		fileSize:   fileSize,
		interval:   interval,
		label:      label,
		out:        out,
		start:      now,
		lastReport: now,
	}
}

// NewPacketOnlyProgressStats mirrors NewProgressStats but suppresses the
// points/RPM/points-per-frame columns. This is used by pass-2 writers that
// stream packets without decoding per-frame sensor details.
func NewPacketOnlyProgressStats(inner PacketStatsInterface, fileSize int64, interval time.Duration, label string, out io.Writer) PacketStatsInterface {
	wrapped := NewProgressStats(inner, fileSize, interval, label, out)
	p, ok := wrapped.(*ProgressStats)
	if !ok {
		return wrapped
	}
	p.packetOnly = true
	return p
}

// AddPacket records a packet's payload size and forwards to the inner stats.
// The progress line is emitted from ObserveProgress, which also carries the
// capture time and motor speed.
func (p *ProgressStats) AddPacket(bytes int) {
	if p.inner != nil {
		p.inner.AddPacket(bytes)
	}
	p.mu.Lock()
	p.payloadBytes += int64(bytes)
	p.packets++
	p.mu.Unlock()
}

// ObserveProgress records the packet's capture time and motor speed, and emits a
// progress line when the wall-clock interval has elapsed. Points-per-frame is
// computed over the interval from the capture-time span and the motor speed
// (frames = capture-seconds * RPM / 60).
func (p *ProgressStats) ObserveProgress(captureTime time.Time, motorRPM uint16) {
	p.mu.Lock()
	if motorRPM > 0 {
		p.rpm = motorRPM
	}
	p.captureNow = captureTime
	if !p.haveCapture {
		p.captureAtReport = captureTime
		p.haveCapture = true
	}
	var line string
	if time.Since(p.lastReport) >= p.interval {
		p.lastReport = time.Now()
		line = p.formatLocked()
		p.captureAtReport = captureTime
		p.pointsAtReport = p.points
	}
	p.mu.Unlock()
	if line != "" && p.out != nil {
		fmt.Fprintln(p.out, line)
	}
}

// AddPoints records parsed points and forwards to the inner stats.
func (p *ProgressStats) AddPoints(count int) {
	if p.inner != nil {
		p.inner.AddPoints(count)
	}
	p.mu.Lock()
	p.points += int64(count)
	p.mu.Unlock()
}

// AddDropped forwards to the inner stats.
func (p *ProgressStats) AddDropped() {
	if p.inner != nil {
		p.inner.AddDropped()
	}
}

// LogStats forwards to the inner stats.
func (p *ProgressStats) LogStats(parsePackets bool) {
	if p.inner != nil {
		p.inner.LogStats(parsePackets)
	}
}

// formatLocked renders one progress line. The caller must hold p.mu.
func (p *ProgressStats) formatLocked() string {
	elapsed := time.Since(p.start)
	var rate float64
	if s := elapsed.Seconds(); s > 0 {
		rate = float64(p.packets) / s
	}
	pct := ""
	if p.fileSize > 0 {
		frac := 100 * float64(p.payloadBytes) / float64(p.fileSize)
		if frac > 100 {
			frac = 100
		}
		pct = fmt.Sprintf("~%4.1f%% | ", frac)
	}
	if p.packetOnly {
		return fmt.Sprintf("[%s] %s%s packets | %s elapsed | %.0f pkt/s",
			p.label, pct, commaInt(p.packets), mmss(elapsed), rate)
	}
	// RPM and points-per-frame for the interval since the last report. Frames are
	// estimated from the capture-time span and motor speed, so this needs RPM.
	rpmStr, ppfStr := "- rpm", "- pts/frame"
	if p.rpm > 0 {
		rpmStr = fmt.Sprintf("%d rpm", p.rpm)
		if captureSecs := p.captureNow.Sub(p.captureAtReport).Seconds(); captureSecs > 0 {
			if frames := captureSecs * float64(p.rpm) / 60.0; frames >= 1 {
				ppfStr = fmt.Sprintf("%.0f pts/frame", float64(p.points-p.pointsAtReport)/frames)
			}
		}
	}
	return fmt.Sprintf("[%s] %s%s packets | %s points | %s | %s | %s elapsed | %.0f pkt/s",
		p.label, pct, commaInt(p.packets), compactInt(p.points), rpmStr, ppfStr, mmss(elapsed), rate)
}

// commaInt formats an integer with thousands separators (1234567 -> "1,234,567").
func commaInt(n int64) string {
	s := fmt.Sprintf("%d", n)
	if n < 0 {
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}

// compactInt formats large counts compactly (308000000 -> "308M", 12500 -> "12.5K").
func compactInt(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.0fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// mmss formats a duration as MM:SS (or H:MM:SS past an hour).
func mmss(d time.Duration) string {
	total := int(d.Seconds() + 0.5)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
