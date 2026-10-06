package server

import (
	"fmt"
	"strings"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

// TestEnsureValidLidarNetworkingFlagsFatalsOnBadFlags covers the wrapper that
// turns a validation error into a startup abort. The server must refuse to
// start on an unusable port rather than bind something unexpected.
func TestEnsureValidLidarNetworkingFlagsFatalsOnBadFlags(t *testing.T) {
	tests := []struct {
		name                                                   string
		udpPort, udpRcvBuf, forwardPort, foregroundForwardPort int
		wantFragment                                           string
	}{
		{"udp port", 0, 1 << 20, 0, 0, "--lidar-udp-port"},
		{"receive buffer", 2368, 0, 0, 0, "--lidar-udp-rcv-buf"},
		{"forward port", 2368, 1 << 20, 70000, 0, "--lidar-forward-port"},
		{"foreground forward port", 2368, 1 << 20, 0, -2, "--lidar-foreground-forward-port"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			fatalf := func(format string, args ...any) {
				got = fmt.Sprintf(format, args...)
			}

			ensureValidLidarNetworkingFlags(tc.udpPort, tc.udpRcvBuf, tc.forwardPort, tc.foregroundForwardPort, fatalf)

			if got == "" {
				t.Fatal("expected the invalid flag to abort startup")
			}
			if !strings.Contains(got, tc.wantFragment) {
				t.Errorf("abort message %q does not name %q", got, tc.wantFragment)
			}
		})
	}
}

// TestEnsureValidLidarNetworkingFlagsAcceptsValidFlags checks the wrapper stays
// quiet on a usable set, so a false abort cannot slip in behind the cases above.
func TestEnsureValidLidarNetworkingFlagsAcceptsValidFlags(t *testing.T) {
	called := false
	fatalf := func(string, ...any) { called = true }

	ensureValidLidarNetworkingFlags(2368, 1<<20, 2370, 2371, fatalf)

	if called {
		t.Error("valid networking flags should not abort startup")
	}
}

// TestApplyRecordingMetadataDefaultsLogger covers the nil-logger fallback.
// Callers in the composition root pass their own logger; the fallback keeps a
// zero-value call from panicking on the first warning it tries to emit.
func TestApplyRecordingMetadataDefaultsLogger(t *testing.T) {
	testDB, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()

	sink := &recordingMetadataSinkStub{}

	// A missing run makes the function reach for its logger on the warning
	// path, so passing nil here exercises the fallback rather than skipping it.
	applyRecordingMetadata(sink, testDB.DB, nil, "missing-run", "tuning-hash", nil)

	if sink.provenanceSource != "live" {
		t.Errorf("provenance source = %q, want %q", sink.provenanceSource, "live")
	}
	if sink.provenanceHash != "tuning-hash" {
		t.Errorf("provenance hash = %q, want %q", sink.provenanceHash, "tuning-hash")
	}
}

// TestRecoverOrphanedSweepsOnStartDefaultsLogger covers the same fallback on
// the startup sweep-recovery path.
func TestRecoverOrphanedSweepsOnStartDefaultsLogger(t *testing.T) {
	// Both arms of the report, with no logger supplied.
	recoverOrphanedSweepsOnStart(orphanedSweepRecovererStub{affected: 3}, nil)
	recoverOrphanedSweepsOnStart(orphanedSweepRecovererStub{err: fmt.Errorf("boom")}, nil)
}

// TestVisualiserPlaybackProbeWithoutServer covers the nil-server arm. The probe
// is wired before the visualiser exists, so it has to report a sane resting
// state rather than panic — a rate of 1.0, meaning "playing at normal speed".
func TestVisualiserPlaybackProbeWithoutServer(t *testing.T) {
	probe := visualiserPlaybackProbe{}

	got := probe.PlaybackPosition()

	if got.Rate != 1.0 {
		t.Errorf("Rate = %v, want 1.0 when no visualiser is attached", got.Rate)
	}
	if got.Paused {
		t.Error("expected the resting state to be unpaused")
	}
	if got.Seekable {
		t.Error("expected an unattached probe to report no seekable log")
	}
}

// TestVisualiserPlaybackProbeDelegatesToServer covers the conversion arm. The
// probe exists only to translate between two identically-shaped structs in
// packages that must not import each other, so the test that earns its keep is
// the one checking every field actually crosses.
func TestVisualiserPlaybackProbeDelegatesToServer(t *testing.T) {
	vs := l9endpoints.NewServer(nil)
	vs.SetReplayMode(true)
	vs.SetPCAPProgress(120, 800)
	vs.SetPCAPTimestamps(1_000, 9_000)

	probe := visualiserPlaybackProbe{server: vs}
	got := probe.PlaybackPosition()
	want := vs.PlaybackPosition()

	if got.Paused != want.Paused {
		t.Errorf("Paused = %v, want %v", got.Paused, want.Paused)
	}
	if got.Rate != want.Rate {
		t.Errorf("Rate = %v, want %v", got.Rate, want.Rate)
	}
	if got.Seekable != want.Seekable {
		t.Errorf("Seekable = %v, want %v", got.Seekable, want.Seekable)
	}
	if got.CurrentFrame != want.CurrentFrame {
		t.Errorf("CurrentFrame = %v, want %v", got.CurrentFrame, want.CurrentFrame)
	}
	if got.TotalFrames != want.TotalFrames {
		t.Errorf("TotalFrames = %v, want %v", got.TotalFrames, want.TotalFrames)
	}
	if got.TimestampNs != want.TimestampNs {
		t.Errorf("TimestampNs = %v, want %v", got.TimestampNs, want.TimestampNs)
	}
	if got.LogStartNs != want.LogStartNs {
		t.Errorf("LogStartNs = %v, want %v", got.LogStartNs, want.LogStartNs)
	}
	if got.LogEndNs != want.LogEndNs {
		t.Errorf("LogEndNs = %v, want %v", got.LogEndNs, want.LogEndNs)
	}
	if got.ReplayEpoch != want.ReplayEpoch {
		t.Errorf("ReplayEpoch = %v, want %v", got.ReplayEpoch, want.ReplayEpoch)
	}
}
