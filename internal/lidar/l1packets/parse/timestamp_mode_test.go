package parse

import (
	"encoding/binary"
	"testing"
	"time"
)

// Time-domain audit of resolvePacketTime (clock plan F1). Each test pins what
// one timestamp mode gives the rest of the pipeline, because the packet time
// becomes every point's timestamp, the frame's StartTimestamp, and so the
// tracker's prediction interval. See docs/lidar/architecture/time-domain-model.md.
//
// createTestMockConfig gives channel 1 a zero firetime, so the first point of
// a parsed packet carries the packet time exactly.

// sensorPacket is a mock packet whose tail says it was sampled at utc: the
// whole second in DateTime and the microseconds in Timestamp, as the
// Pandar40P reports them.
func sensorPacket(utc time.Time) []byte {
	packet := createTestMockPacket()
	tail := testPacketSizeStandard - testTailSize
	binary.LittleEndian.PutUint32(packet[tail+10:tail+14], uint32(utc.Nanosecond()/1000))
	copy(packet[tail+16:tail+22], []byte{
		byte(utc.Year() - 2000), byte(utc.Month()), byte(utc.Day()),
		byte(utc.Hour()), byte(utc.Minute()), byte(utc.Second()),
	})
	return packet
}

func packetTime(t *testing.T, parser *Pandar40PParser, packet []byte) time.Time {
	t.Helper()
	points, err := parser.ParsePacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) == 0 {
		t.Fatal("packet parsed to no points")
	}
	return time.Unix(0, points[0].Timestamp)
}

var auditSensorStart = time.Date(2026, 9, 25, 12, 0, 0, 950_000_000, time.UTC)

// Native LiDAR time is the sensor's own clock: the interval between packets is
// the sensor's interval exactly, and it stays monotonic across the second
// boundary, where the microsecond field wraps and DateTime carries.
func TestLiDARModeFollowsTheSensorClockAcrossTheSecond(t *testing.T) {
	parser := NewPandar40PParser(*createTestMockConfig())
	parser.SetTimestampMode(TimestampModeLiDAR)

	first := packetTime(t, parser, sensorPacket(auditSensorStart))
	second := packetTime(t, parser, sensorPacket(auditSensorStart.Add(100*time.Millisecond)))

	if !first.Equal(auditSensorStart) {
		t.Fatalf("packet time %v, want the sensor's %v", first, auditSensorStart)
	}
	if got := second.Sub(first); got != 100*time.Millisecond {
		t.Fatalf("interval across the second boundary = %v, want the sensor's 100ms", got)
	}
}

// A replay supplies each packet's capture time, and it wins in every mode.
// The interval the tracker sees is then the capture interval, whatever the
// sensor fields say. Here they even run backwards.
func TestCaptureTimeOverridesEveryMode(t *testing.T) {
	captureStart := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for _, mode := range []TimestampMode{
		TimestampModeSystemTime, TimestampModePTP, TimestampModeGPS, TimestampModeInternal, TimestampModeLiDAR,
	} {
		parser := NewPandar40PParser(*createTestMockConfig())
		parser.SetTimestampMode(mode)

		parser.SetPacketTime(captureStart)
		first := packetTime(t, parser, sensorPacket(auditSensorStart))
		parser.SetPacketTime(captureStart.Add(102 * time.Millisecond))
		second := packetTime(t, parser, sensorPacket(auditSensorStart.Add(-time.Second)))

		if !first.Equal(captureStart) || second.Sub(first) != 102*time.Millisecond {
			t.Errorf("mode %d: packet times %v, %v; want the capture times %v and +102ms", mode, first, second, captureStart)
		}
	}
}

// System time is host arrival time. It cannot follow the sensor interval: it
// is whatever the socket, queue and parser delivered, and it moves with any
// step of the host clock. The live default, so live tracker intervals are
// arrival intervals.
func TestSystemTimeModeIsHostArrivalTime(t *testing.T) {
	parser := NewPandar40PParser(*createTestMockConfig())
	parser.SetTimestampMode(TimestampModeSystemTime)

	before := time.Now()
	first := packetTime(t, parser, sensorPacket(auditSensorStart))
	second := packetTime(t, parser, sensorPacket(auditSensorStart.Add(10*time.Second)))
	after := time.Now()

	for _, ts := range []time.Time{first, second} {
		if ts.Before(before) || ts.After(after) {
			t.Fatalf("packet time %v is not host arrival time (parsed between %v and %v)", ts, before, after)
		}
	}
	if second.Sub(first) >= 10*time.Second {
		t.Fatal("system time followed the sensor's 10 s interval")
	}
}

// PTP, GPS and internal modes add the microsecond field to the parser's boot
// time. Inside one second that follows the sensor's interval exactly. The
// Pandar40P's field is the microsecond part of the UTC second, though, not
// elapsed time since boot, so at every second boundary the packet time steps
// back by nearly a second. These modes cannot guarantee a monotonic frame
// clock; the tracker's backward-timestamp guard is what keeps that from
// becoming a negative prediction interval.
func TestBootOffsetModesStepBackAtEverySecond(t *testing.T) {
	for _, mode := range []TimestampMode{TimestampModePTP, TimestampModeGPS, TimestampModeInternal} {
		parser := NewPandar40PParser(*createTestMockConfig())
		parser.SetTimestampMode(mode)

		inSecond := auditSensorStart.Add(-500 * time.Millisecond) // .450
		a := packetTime(t, parser, sensorPacket(inSecond))
		b := packetTime(t, parser, sensorPacket(inSecond.Add(100*time.Millisecond)))
		if b.Sub(a) != 100*time.Millisecond {
			t.Errorf("mode %d: interval inside a second = %v, want the sensor's 100ms", mode, b.Sub(a))
		}

		c := packetTime(t, parser, sensorPacket(auditSensorStart))                           // .950
		d := packetTime(t, parser, sensorPacket(auditSensorStart.Add(100*time.Millisecond))) // next second, .050
		if got := d.Sub(c); got != -900*time.Millisecond {
			t.Errorf("mode %d: interval across the second = %v, want the -900ms wrap this mode produces", mode, got)
		}
	}
}

// PTP and GPS modes fall back to system time once the microsecond field has
// repeated more than STATIC_TIMESTAMP_THRESHOLD times, and the fallback never
// ends: the static count is not reset when the field moves again. A stream
// that stalls once is on host arrival time for the rest of the parser's life,
// switching time domain mid-stream. This characterises current behaviour.
func TestPTPStaticFallbackIsPermanent(t *testing.T) {
	parser := NewPandar40PParser(*createTestMockConfig())
	parser.SetTimestampMode(TimestampModePTP)

	stalled := sensorPacket(auditSensorStart)
	for i := 0; i < STATIC_TIMESTAMP_THRESHOLD+2; i++ {
		packetTime(t, parser, stalled)
	}
	before := time.Now()
	resumed := packetTime(t, parser, sensorPacket(auditSensorStart.Add(-400*time.Millisecond)))
	after := time.Now()
	if resumed.Before(before) || resumed.After(after) {
		t.Fatalf("after the field resumed, packet time %v is not host arrival time; the fallback released", resumed)
	}
}
