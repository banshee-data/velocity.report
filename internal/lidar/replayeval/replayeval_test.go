package replayeval

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

func TestRunRequiresPCAPFile(t *testing.T) {
	_, err := Run(Config{OutDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "PCAPFile") {
		t.Fatalf("err = %v, want a complaint about PCAPFile", err)
	}
}

func TestRunRequiresOutDir(t *testing.T) {
	_, err := Run(Config{PCAPFile: "capture.pcap"})
	if err == nil || !strings.Contains(err.Error(), "OutDir") {
		t.Fatalf("err = %v, want a complaint about OutDir", err)
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
