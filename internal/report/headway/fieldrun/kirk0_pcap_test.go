//go:build pcap && !race

package fieldrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/chart"
	"github.com/banshee-data/velocity.report/internal/report/headway"
	"github.com/banshee-data/velocity.report/internal/report/typst"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

const kirk0 = "../../../lidar/perf/pcap/kirk0.pcapng"

// replayKirk0 replays kirk0 into a new evidence database: a settled
// background from the first 20 s, then the remaining 63 s with its
// immutable observations and online estimates, and the fixed_lag_rts
// experiment's refined fixed_lag and final rows beside them, with any further
// experiments named. No points are recorded and each observation keeps 16
// sample points, which keeps the replay's memory small.
func replayKirk0(t *testing.T, mode l5tracks.MeasurementSource, experiments ...string) (sqlite.DBClient, string) {
	t.Helper()
	pcap, err := filepath.Abs(kirk0)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(pcap); err != nil || info.Size() < 1<<20 {
		t.Skipf("kirk0 not available (run git lfs pull): %v", err)
	}
	dir := t.TempDir()
	cfg := replayeval.Config{
		PCAPFile: pcap, OutDir: filepath.Join(dir, "vrlog"), SensorID: "test-replay", UDPPort: 2369,
		StartSeconds: 20, WarmupSeconds: 20, RequireSettled: true,
		ObservationDBPath: filepath.Join(dir, "evidence.db"), ReplayCaseID: "kirk0-headway-test",
		ObservationCalibration: l4bobserve.Calibration{SensorID: "test-replay", FromFrame: "sensor", ToFrame: "site",
			Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}},
		ObservationMaxSamplePoints: 16,
		MeasurementSourceMode:      mode,
		Experiments:                append([]string{replayeval.ExperimentFixedLagRTS}, experiments...),
	}
	result, err := replayeval.Run(cfg)
	if err != nil {
		t.Fatalf("replay kirk0: %v", err)
	}
	database, err := db.NewDB(cfg.ObservationDBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database, result.ObservationSourceID
}

// fieldReport runs one stage and checks what every field report must be,
// whatever the data yields: provisional, reproducible, audited and free of
// verdict language, with every accounted second of every encounter either
// valid or under its reason.
func fieldReport(t *testing.T, database sqlite.DBClient, spec Spec) Result {
	t.Helper()
	res := run(t, database, spec)
	for _, line := range res.Summary().Lines() {
		t.Log(line)
	}
	t.Log(res.Capture().Description)
	s := res.Summary()
	var suppressed int64
	for _, c := range s.Suppressed {
		suppressed += c.Nanos
	}
	if s.ValidNanos+suppressed != s.AccountedNanos {
		t.Errorf("valid %d ns and suppressed %d ns do not add up to %d ns accounted", s.ValidNanos, suppressed, s.AccountedNanos)
	}

	r, err := Report(res)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != headway.StatusProvisional {
		t.Errorf("status %s", r.Status)
	}
	rendered, charts, err := headway.Assemble(r, chart.PaperA4)
	if err != nil {
		t.Fatal(err)
	}
	data, err := typst.MarshalData(rendered)
	if err != nil {
		t.Fatal(err)
	}
	if err := l8behaviour.AuditSurfaceJSON(data); err != nil {
		t.Errorf("data.json: %v", err)
	}
	if m := headway.VerdictPattern.FindAll(data, -1); len(m) > 0 {
		t.Errorf("data.json prints %q", m)
	}
	for _, c := range charts {
		if !bytes.Contains(c.Data, []byte(">"+r.StatusLabel+"<")) || headway.VerdictPattern.Match(c.Data) {
			t.Errorf("%s does not carry its status, or prints verdict language", c.Name)
		}
	}
	for _, fi := range res.Interactions {
		for _, v := range []any{fi.Event, fi.Instants, fi.Windows} {
			if err := l8behaviour.AuditSurfaceJSON(mustJSON(t, v)); err != nil {
				t.Errorf("persisted %s: %v", fi.Event.EventID, err)
			}
		}
	}

	again := run(t, database, spec)
	if again.AlreadyStored != len(again.Interactions) || !bytes.Equal(reportData(t, again), data) {
		t.Errorf("a second run stored %d of %d anew or built a different data.json",
			len(again.Interactions)-again.AlreadyStored, len(again.Interactions))
	}

	t.Setenv(typstbin.EnvNoDownload, "1")
	if typstAvailable() {
		out, err := headway.Generate(r, headway.Options{Paper: chart.PaperA4, OutputDir: t.TempDir()})
		if err != nil {
			t.Fatalf("Generate: %v", err)
		}
		t.Logf("PDF %s", out.PDFPath)
	} else {
		t.Log("typst not embedded or on PATH: data.json and charts checked, PDF compile skipped")
	}
	return res
}

// TestKirk0ProvisionalHeadway takes kirk0 through the whole slice three
// times. A persisted point estimate refers to the cluster medoid under the
// production measurement model and to the centre of the visible box under
// the OBB-centre candidate, at every stage; neither is a place on the body,
// so no follower's path can be fitted and there is no encounter at all
// (review R1). The solid bodies filed beside the point estimates carry a
// body-centre reference after a near-edge fix, with heading, extents and
// class, and are the input that can place an endpoint. Every outcome is a
// result; the test pins what must hold of any result and logs the figures.
func TestKirk0ProvisionalHeadway(t *testing.T) {
	t.Run("medoid_v0", func(t *testing.T) {
		database, source := replayKirk0(t, l5tracks.MeasurementMedoidV0)
		res := fieldReport(t, database, Spec{SourceID: source, Stage: l8behaviour.StageOnline})
		if s := res.Summary(); s.PathsFitted != 0 || s.Encounters != 0 || s.Tracks == 0 {
			t.Errorf("medoid-referenced rows: %d of %d paths fitted, %d encounters; want none", s.PathsFitted, s.Tracks, s.Encounters)
		}
	})
	t.Run("obb_centre_v1", func(t *testing.T) {
		database, source := replayKirk0(t, l5tracks.MeasurementOBBCentreV1)
		for _, spec := range []Spec{
			{SourceID: source, Stage: l8behaviour.StageOnline},
			{SourceID: source, Stage: l8behaviour.StageFinal},
		} {
			t.Run(spec.Stage.String(), func(t *testing.T) {
				res := fieldReport(t, database, spec)
				if s := res.Summary(); s.PathsFitted != 0 || s.Encounters != 0 || s.Tracks == 0 {
					t.Errorf("visible-box-centre rows at %s: %d of %d paths fitted, %d encounters; want none",
						spec.Stage, s.PathsFitted, s.Tracks, s.Encounters)
				}
			})
		}
		// Several fixed_lag horizons are stored; the run names one, and each
		// named horizon is its own interaction version.
		if _, err := Run(database, Spec{SourceID: source, Stage: l8behaviour.StageFixedLag}); err == nil ||
			!strings.Contains(err.Error(), "fixed_lag versions of estimates") {
			t.Errorf("fixed_lag with several horizons: error %v", err)
		}
		versions, err := sqlite.NewStateEstimateStore(database).ListEstimateVersions()
		if err != nil {
			t.Fatal(err)
		}
		horizons := 0
		for _, v := range versions {
			if v.SourceID != source || v.Stage != l8behaviour.StageFixedLag.String() {
				continue
			}
			horizons++
			t.Run("fixed_lag "+v.ParamHash[:15], func(t *testing.T) {
				fieldReport(t, database, Spec{SourceID: source, Stage: l8behaviour.StageFixedLag, ParamHash: v.ParamHash})
			})
		}
		if horizons != len(replayeval.RefinementHorizons)-1 {
			t.Errorf("%d fixed_lag versions stored, want every horizon but the track end", horizons)
		}
	})
	t.Run("solid_body", func(t *testing.T) {
		database, source := replayKirk0(t, "", replayeval.ExperimentSolidBody)
		res := fieldReport(t, database, Spec{SourceID: source, Stage: l8behaviour.StageOnline, SolidBodies: true})
		s := res.Summary()
		if s.Tracks == 0 {
			t.Fatal("no solid-body trajectories")
		}
		var samples, centred, acquired int
		for _, tr := range res.Trajectories {
			for _, sample := range tr.Samples {
				samples++
				if sample.Reference == l8behaviour.ReferenceBodyCentre {
					centred++
				}
				if sample.AcquisitionUnixNanos > 0 {
					acquired++
				}
			}
		}
		if centred == 0 || acquired != samples {
			t.Errorf("%d of %d samples on the body centre, %d with an acquisition time", centred, samples, acquired)
		}
		if s.PathsFitted == 0 {
			t.Error("no follower path was fitted from the body-centre solid bodies")
		}
	})
}
