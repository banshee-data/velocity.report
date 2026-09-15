package replayeval

import (
	"bytes"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/capseq"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

func TestTrackingBaselineWriteErrors(t *testing.T) {
	if err := writeTrackingBaseline(t.TempDir(), l5tracks.TrackingMetrics{Residuals: []l5tracks.ResidualBandSummary{{MeanNIS: math.NaN()}}}); err == nil || !strings.Contains(err.Error(), "marshal tracking baseline") {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "tracking_baseline.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeTrackingBaseline(dir, l5tracks.TrackingMetrics{}); err == nil || !strings.Contains(err.Error(), "write tracking baseline") {
		t.Fatal(err)
	}
}

// A repeat gate must be strict, but it must compare the published baseline
// rather than float64 accumulator tails below the report's stated precision.
func TestTrackingBaselineCanonicalisesAccumulatorNoise(t *testing.T) {
	dir := t.TempDir()
	first := l5tracks.TrackingMetrics{Residuals: []l5tracks.ResidualBandSummary{{
		SpeedFloorMps: 0, Count: 28683,
		LongitudinalRMSMetres: 0.4277576140295845,
		LongitudinalBias:      0.21098171973351434,
		MeanNIS:               0.7828233412481577,
	}}}
	repeat := first
	repeat.Residuals = append([]l5tracks.ResidualBandSummary(nil), first.Residuals...)
	repeat.Residuals[0].LongitudinalRMSMetres = 0.42775761987267874
	repeat.Residuals[0].LongitudinalBias = 0.21098172933487122
	repeat.Residuals[0].MeanNIS = 0.7828233305235078

	firstDir, repeatDir := filepath.Join(dir, "first"), filepath.Join(dir, "repeat")
	if err := os.Mkdir(firstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(repeatDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTrackingBaseline(firstDir, first); err != nil {
		t.Fatal(err)
	}
	if err := writeTrackingBaseline(repeatDir, repeat); err != nil {
		t.Fatal(err)
	}
	firstBytes, err := os.ReadFile(filepath.Join(firstDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	repeatBytes, err := os.ReadFile(filepath.Join(repeatDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, repeatBytes) {
		t.Fatalf("canonical baseline changed:\n%s\n%s", firstBytes, repeatBytes)
	}
}

func TestRunRequiresPCAPFile(t *testing.T) {
	_, err := Run(Config{OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "PCAPFile") {
		t.Fatalf("err = %v, want a complaint about PCAPFile", err)
	}
}

func TestCaptureFilesPreserveSpecifiedSequence(t *testing.T) {
	got, err := captureFiles(Config{PCAPFiles: []string{"first.pcap", "second.pcap"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "first.pcap,second.pcap" {
		t.Fatalf("capture order = %v", got)
	}
	got[0] = "changed.pcap"
	again, err := captureFiles(Config{PCAPFiles: []string{"first.pcap", "second.pcap"}})
	if err != nil || again[0] != "first.pcap" {
		t.Fatalf("capture files aliased config: %v, %v", again, err)
	}
}

func TestCaptureFilesRejectAmbiguousInput(t *testing.T) {
	_, err := captureFiles(Config{PCAPFile: "one.pcap", PCAPFiles: []string{"two.pcap"}})
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("err = %v, want mutually exclusive complaint", err)
	}
}

func TestRawSHA256RequiresPrefixedDigest(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	if got, err := rawSHA256(digest); err != nil || got != strings.Repeat("a", 64) {
		t.Fatalf("rawSHA256 = %q, %v", got, err)
	}
	if _, err := rawSHA256(strings.Repeat("a", 64)); err == nil {
		t.Fatal("unprefixed digest accepted")
	}
}

func TestCaptureSHA256sReusesVerifiedSourceManifestDigests(t *testing.T) {
	files := []string{"first.pcap", "second.pcap"}
	first := "sha256:" + strings.Repeat("a", 64)
	second := "sha256:" + strings.Repeat("b", 64)
	called := false
	digests, raw, err := captureSHA256s(Config{PCAPSHA256s: []string{first, second}}, replayRuntime{
		hashFile: func(string) (string, error) {
			called = true
			return "", errors.New("unexpected rehash")
		},
	}, files)
	if err != nil {
		t.Fatal(err)
	}
	if called || strings.Join(digests, ",") != first+","+second || strings.Join(raw, ",") != strings.Repeat("a", 64)+","+strings.Repeat("b", 64) {
		t.Fatalf("digests=%q raw=%q hashFile called=%v", digests, raw, called)
	}
	if _, _, err := captureSHA256s(Config{PCAPSHA256s: []string{first}}, replayRuntime{}, files); err == nil {
		t.Fatal("accepted a digest list with the wrong length")
	}
}

func TestConfiguredCaptureSequenceRequiresExactOrderedPaths(t *testing.T) {
	sequence, err := capseq.Build([]capseq.Segment{{Path: "first.pcap", FirstPacket: time.Unix(1, 0), LastPacket: time.Unix(2, 0), PacketCount: 1}, {Path: "second.pcap", FirstPacket: time.Unix(2, 5e6), LastPacket: time.Unix(3, 0), PacketCount: 1}}, capseq.DefaultTolerances())
	if err != nil {
		t.Fatal(err)
	}
	if got, err := configuredCaptureSequence(Config{CaptureSequence: sequence}, []string{"first.pcap", "second.pcap"}); err != nil || got != sequence {
		t.Fatalf("configuredCaptureSequence = %#v, %v", got, err)
	}
	if _, err := configuredCaptureSequence(Config{CaptureSequence: sequence}, []string{"second.pcap", "first.pcap"}); err == nil {
		t.Fatal("accepted a sequence with mismatched path order")
	}
}

func TestRunRequiresOutDir(t *testing.T) {
	_, err := Run(Config{PCAPFile: "capture.pcap"})
	if err == nil || !strings.Contains(err.Error(), "OutDir") {
		t.Fatalf("err = %v, want a complaint about OutDir", err)
	}
}

func TestOfflineFrameBuilderDisablesWallClockCleanup(t *testing.T) {
	callback := func(*l2frames.LiDARFrame) {}
	cfg := offlineFrameBuilderConfig("offline-test", callback)
	if cfg.SensorID != "offline-test" || cfg.FrameCallback == nil || cfg.FrameChCapacity != 32 {
		t.Fatalf("offline frame builder wiring = %+v", cfg)
	}
	want := time.Duration(math.MaxInt64)
	if cfg.BufferTimeout != want || cfg.CleanupInterval != want {
		t.Fatalf("wall-clock cleanup remains armed: timeout=%v interval=%v", cfg.BufferTimeout, cfg.CleanupInterval)
	}
}

// A missing capture must fail rather than silently producing an empty run that
// then reads as a clean result.
func TestRunFailsOnMissingCapture(t *testing.T) {
	dir := t.TempDir()
	_, err := Run(Config{
		PCAPFile: filepath.Join(dir, "does-not-exist.pcap"),
		OutDir:   filepath.Join(dir, "out"),
		UDPPort:  2369,
	})
	if err == nil {
		t.Fatal("replaying a missing capture returned no error")
	}
}

// --- recordingPublisher ---

// stubRecorder captures what the publisher hands to the recorder.
type stubRecorder struct {
	bundles []*l9endpoints.FrameBundle
	err     error
}

func (s *stubRecorder) record(b *l9endpoints.FrameBundle) error {
	if s.err != nil {
		return s.err
	}
	s.bundles = append(s.bundles, b)
	return nil
}

// publishVia exercises the publisher's logic against a stub, avoiding a real
// recorder and its filesystem layout.
func publishVia(p *recordingPublisher, s *stubRecorder, frames ...interface{}) {
	for _, f := range frames {
		bundle, ok := f.(*l9endpoints.FrameBundle)
		if !ok || bundle == nil {
			continue
		}
		if p.writeErr != nil {
			continue
		}
		if p.dropPoints {
			bundle.PointCloud = nil
		}
		if bundle.Tracks == nil || len(bundle.Tracks.Tracks) == 0 {
			p.emptyFrames++
		}
		if err := s.record(bundle); err != nil {
			p.writeErr = err
		}
		p.recorded++
	}
}

func TestPublisherDropsPointCloudWhenNotRequested(t *testing.T) {
	p := &recordingPublisher{dropPoints: true}
	s := &stubRecorder{}
	b := &l9endpoints.FrameBundle{PointCloud: &l9endpoints.PointCloudFrame{}}

	publishVia(p, s, b)

	if len(s.bundles) != 1 {
		t.Fatalf("recorded %d bundles, want 1", len(s.bundles))
	}
	if s.bundles[0].PointCloud != nil {
		t.Fatal("point cloud was kept despite dropPoints")
	}
}

func TestPublisherKeepsPointCloudWhenRequested(t *testing.T) {
	p := &recordingPublisher{dropPoints: false}
	s := &stubRecorder{}
	b := &l9endpoints.FrameBundle{PointCloud: &l9endpoints.PointCloudFrame{}}

	publishVia(p, s, b)

	if s.bundles[0].PointCloud == nil {
		t.Fatal("point cloud was dropped despite IncludePoints")
	}
}

func TestPublisherCountsTracklessFrames(t *testing.T) {
	p := &recordingPublisher{}
	s := &stubRecorder{}

	withTracks := &l9endpoints.FrameBundle{
		Tracks: &l9endpoints.TrackSet{Tracks: []l9endpoints.Track{{TrackID: "a"}}},
	}
	publishVia(p, s,
		&l9endpoints.FrameBundle{},                                // no track set
		&l9endpoints.FrameBundle{Tracks: &l9endpoints.TrackSet{}}, // empty track set
		withTracks,
	)

	if p.emptyFrames != 2 {
		t.Fatalf("emptyFrames = %d, want 2", p.emptyFrames)
	}
	if p.recorded != 3 {
		t.Fatalf("recorded = %d, want 3", p.recorded)
	}
}

// The first write error must be latched and stop further writes, so a full disk
// surfaces as a failed run rather than a truncated recording reported as good.
func TestPublisherLatchesWriteError(t *testing.T) {
	p := &recordingPublisher{}
	s := &stubRecorder{err: errors.New("disk full")}

	publishVia(p, s,
		&l9endpoints.FrameBundle{},
		&l9endpoints.FrameBundle{},
	)

	if p.writeErr == nil {
		t.Fatal("write error was not latched")
	}
	if p.recorded != 1 {
		t.Fatalf("recorded = %d, want 1: writes should stop after the first error", p.recorded)
	}
}

func TestPublisherIgnoresNonBundle(t *testing.T) {
	p := &recordingPublisher{}
	p.Publish("not a bundle")
	p.Publish(nil)
	p.Publish((*l9endpoints.FrameBundle)(nil))

	if p.recorded != 0 {
		t.Fatalf("recorded = %d, want 0", p.recorded)
	}
}

func TestSchemaVersionOrUnknown(t *testing.T) {
	if got := schemaVersionOrUnknown(nil); got != "unknown" {
		t.Fatalf("nil config gave %q, want \"unknown\"", got)
	}
}

func TestSha256SumIsStable(t *testing.T) {
	a := sha256Sum([]byte("velocity"))
	b := sha256Sum([]byte("velocity"))
	if len(a) != 32 {
		t.Fatalf("digest length %d, want 32", len(a))
	}
	if string(a) != string(b) {
		t.Fatal("digest is not stable across calls")
	}
	if string(a) == string(sha256Sum([]byte("velocity "))) {
		t.Fatal("digest does not distinguish different input")
	}
}
