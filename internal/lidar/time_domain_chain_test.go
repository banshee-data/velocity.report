package lidar_test

import (
	"encoding/binary"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// End-to-end time-domain audit (clock plan F1): synthetic Pandar40P packets,
// through the real parser in each timestamp mode and the real frame builder,
// into the tracker exactly as the pipeline feeds it (LiDARFrame.StartTimestamp).
// The question each case answers is whether the tracker's prediction interval
// is the sensor's rotation interval. The packet-level reasons are pinned in
// l1packets/parse/timestamp_mode_test.go.

const (
	chainPacketsPerRotation = 200                    // 10 Hz: 2,000 blocks of 0.18°
	chainPacketInterval     = 500 * time.Microsecond // sensor time between packets
	chainRotation           = chainPacketsPerRotation * chainPacketInterval
)

func chainParserConfig() parse.Pandar40PConfig {
	var cfg parse.Pandar40PConfig
	for i := 0; i < 40; i++ {
		cfg.AngleCorrections[i] = parse.AngleCorrection{Channel: i + 1, Elevation: float64(i-20) * 0.5}
		cfg.FiretimeCorrections[i] = parse.FiretimeCorrection{Channel: i + 1}
	}
	return cfg
}

// chainPacket builds packet n of the stream: ten blocks continuing the sweep,
// every channel returning, and a tail stamped with sensor time utc.
func chainPacket(n int, utc time.Time) []byte {
	const blockSize, tailStart = 124, 1240
	packet := make([]byte, 1262)
	for b := 0; b < 10; b++ {
		off := b * blockSize
		binary.LittleEndian.PutUint16(packet[off:], 0xEEFF)
		azimuth := uint16(((n%chainPacketsPerRotation)*10 + b) * 18)
		binary.LittleEndian.PutUint16(packet[off+2:], azimuth)
		for c := 0; c < 40; c++ {
			ch := off + 4 + c*3
			binary.LittleEndian.PutUint16(packet[ch:], 2500) // 10 m
			packet[ch+2] = 100
		}
	}
	tail := packet[tailStart:]
	binary.LittleEndian.PutUint16(tail[8:], 600) // RPM
	binary.LittleEndian.PutUint32(tail[10:], uint32(utc.Nanosecond()/1000))
	tail[14], tail[15] = 0x37, 0x42
	copy(tail[16:], []byte{byte(utc.Year() - 2000), byte(utc.Month()), byte(utc.Day()),
		byte(utc.Hour()), byte(utc.Minute()), byte(utc.Second())})
	return packet
}

// chainFrameStarts streams rotations of packets through a parser in mode and
// an offline-configured frame builder, and returns each emitted frame's
// StartTimestamp. captureInterval > 0 supplies replay capture times, as the
// PCAP readers do, spaced differently from the sensor's own.
func chainFrameStarts(t *testing.T, mode parse.TimestampMode, rotations int, sensorStart time.Time, captureInterval time.Duration) []time.Time {
	t.Helper()
	parser := parse.NewPandar40PParser(chainParserConfig())
	parser.SetTimestampMode(mode)

	var (
		mu     sync.Mutex
		starts []time.Time
	)
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{
		SensorID:        "time-domain-chain",
		FrameChCapacity: 32,
		BufferTimeout:   time.Duration(math.MaxInt64),
		CleanupInterval: time.Duration(math.MaxInt64),
		FrameCallback: func(f *l2frames.LiDARFrame) {
			mu.Lock()
			starts = append(starts, f.StartTimestamp)
			mu.Unlock()
		},
	})
	fb.SetBlockOnFrameChannel(true)

	captureStart := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	for n := 0; n < rotations*chainPacketsPerRotation; n++ {
		if captureInterval > 0 {
			parser.SetPacketTime(captureStart.Add(time.Duration(n) * captureInterval))
		}
		points, err := parser.ParsePacket(chainPacket(n, sensorStart.Add(time.Duration(n)*chainPacketInterval)))
		if err != nil {
			t.Fatal(err)
		}
		fb.AddPointsPolar(points)
	}
	fb.Close()

	mu.Lock()
	defer mu.Unlock()
	if len(starts) < rotations-1 {
		t.Fatalf("frame builder emitted %d frames from %d rotations", len(starts), rotations)
	}
	return starts
}

// feedTracker hands each frame's StartTimestamp to a tracker, as the pipeline
// does, and returns the gap before every frame after the first.
func feedTracker(starts []time.Time) ([]float64, l5tracks.TimeDomainStats) {
	tk := l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())
	var gaps []float64
	for i, ts := range starts {
		tk.Update(nil, ts)
		if i > 0 {
			gaps = append(gaps, tk.TimeDomainStats().LastGapSecs)
		}
	}
	return gaps, tk.TimeDomainStats()
}

func requireGaps(t *testing.T, name string, gaps []float64, want time.Duration) {
	t.Helper()
	if len(gaps) == 0 {
		t.Fatalf("%s: no intervals to check", name)
	}
	for i, g := range gaps {
		if math.Abs(g-want.Seconds()) > 1e-9 {
			t.Fatalf("%s: tracker interval %d = %.9fs, want %v", name, i, g, want)
		}
	}
}

// Native LiDAR time: the tracker's interval is the sensor's rotation period,
// exactly, including across a UTC second boundary.
func TestTrackerIntervalFollowsSensorTimeInLiDARMode(t *testing.T) {
	sensorStart := time.Date(2026, 9, 25, 12, 0, 0, 780_000_000, time.UTC) // crosses the second
	gaps, stats := feedTracker(chainFrameStarts(t, parse.TimestampModeLiDAR, 6, sensorStart, 0))
	requireGaps(t, "lidar", gaps, chainRotation)
	if stats.BackwardTimestamps != 0 {
		t.Fatalf("native time stepped backwards: %+v", stats)
	}
}

// Replay: the tracker's interval is the capture interval in every mode, even
// where it differs from the sensor's (here packets were captured 510 µs apart
// against the sensor's 500 µs). Replay therefore reproduces the capture's
// host-reception timing, not the sensor clock.
func TestTrackerIntervalFollowsCaptureTimeOnReplay(t *testing.T) {
	sensorStart := time.Date(2026, 9, 25, 12, 0, 0, 780_000_000, time.UTC)
	for _, mode := range []parse.TimestampMode{
		parse.TimestampModeSystemTime, parse.TimestampModePTP, parse.TimestampModeGPS,
		parse.TimestampModeInternal, parse.TimestampModeLiDAR,
	} {
		gaps, _ := feedTracker(chainFrameStarts(t, mode, 5, sensorStart, 510*time.Microsecond))
		requireGaps(t, "replay", gaps, chainPacketsPerRotation*510*time.Microsecond)
	}
}

// PTP/GPS/Internal modes now use the sensor's combined UTC timestamp, so they
// follow the LiDAR clock cleanly across a UTC second boundary instead of
// stepping backwards as the old boot-offset logic did.
func TestSensorTimestampModesStayMonotonicAcrossTheSecond(t *testing.T) {
	sensorStart := time.Date(2026, 9, 25, 12, 0, 0, 780_000_000, time.UTC)
	for _, mode := range []parse.TimestampMode{parse.TimestampModePTP, parse.TimestampModeGPS, parse.TimestampModeInternal} {
		gaps, stats := feedTracker(chainFrameStarts(t, mode, 6, sensorStart, 0))
		if stats.BackwardTimestamps != 0 || stats.MaxBackwardStepSecs != 0 {
			t.Fatalf("mode %d: sensor time stepped backwards across the second boundary: %+v", mode, stats)
		}
		if len(gaps) != 5 {
			t.Fatalf("mode %d: intervals %v, want 5 gaps", mode, gaps)
		}
		for i, g := range gaps {
			if math.Abs(g-chainRotation.Seconds()) > 1e-9 {
				t.Fatalf("mode %d: tracker interval %d = %.9fs, want %v", mode, i, g, chainRotation)
			}
		}
	}
}

// System time, live: frame timestamps are host arrival times, bounded by the
// wall clock around the stream whatever the sensor fields say. The tracker's
// interval is then how fast frames were assembled, which is why live temporal
// behaviour is not reproducible and a replay uses capture time instead.
func TestTrackerIntervalIsHostArrivalInSystemTimeMode(t *testing.T) {
	sensorStart := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	before := time.Now()
	starts := chainFrameStarts(t, parse.TimestampModeSystemTime, 4, sensorStart, 0)
	after := time.Now()
	for i, ts := range starts {
		if ts.Before(before) || ts.After(after) {
			t.Fatalf("frame %d starts at %v, outside the wall-clock span of the stream [%v, %v]", i, ts, before, after)
		}
	}
}
