// following-review-prepare creates a paired recording and offline following
// export for operator annotation. It never tunes, scores a candidate or changes
// a cohort assignment. Existing recordings and evidence are never overwritten.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	"github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitycli"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/headway/fieldrun"
	"github.com/banshee-data/velocity.report/internal/version"
)

type options struct {
	Capture, Output, Site, Clip, Binding, Tuning, Calibration string
	Warmup                                                    float64
	Port                                                      int
}

func main() {
	var o options
	flag.StringVar(&o.Capture, "pcap", "", "one explicit source capture")
	flag.StringVar(&o.Output, "output", "", "new local review directory; must not exist")
	flag.StringVar(&o.Site, "site", "", "explicit site ID, matching the cohort assignment when known")
	flag.StringVar(&o.Clip, "clip", "", "source clip identity")
	flag.StringVar(&o.Binding, "binding", "", "source-to-site review binding, including parent run ID")
	flag.StringVar(&o.Tuning, "tuning", "", "checked-in tuning JSON; no embedded fallback")
	flag.StringVar(&o.Calibration, "calibration", "", "explicit l4bobserve.Calibration JSON; never inferred from the site")
	flag.Float64Var(&o.Warmup, "warmup", 70, "capture prefix processed before recording; require settled background")
	flag.IntVar(&o.Port, "port", 2369, "capture's LiDAR UDP port")
	guard := eligibilitycli.RegisterFlags(flag.CommandLine)
	flag.Parse()
	if err := prepare(o, guard); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func (o options) validate() error {
	for name, value := range map[string]string{"pcap": o.Capture, "output": o.Output, "site": o.Site, "clip": o.Clip, "binding": o.Binding, "tuning": o.Tuning, "calibration": o.Calibration} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("-%s is required", name)
		}
	}
	if math.IsNaN(o.Warmup) || math.IsInf(o.Warmup, 0) || o.Warmup <= 0 || o.Port < 1 || o.Port > 65535 {
		return fmt.Errorf("require positive finite warm-up and a valid UDP port")
	}
	if version.GitSHA == "" || version.GitSHA == "unknown" {
		return fmt.Errorf("build through make build-following-review so provenance is stamped")
	}
	if _, err := os.Stat(o.Tuning); err != nil {
		return err
	}
	return nil
}

func authorizeReview(guard *eligibilitycli.CLIFlags, id eligibility.Identity, site, tuning string) (eligibility.Decision, error) {
	d, err := guard.Authorize("following-review-prepare", eligibility.Request{
		Purpose: eligibility.PurposeAnnotationReview, ConfigDigest: tuning, Inputs: []eligibility.Input{{Identity: id}},
	})
	if err != nil {
		return d, err
	}
	if len(d.Inputs) != 1 {
		return d, fmt.Errorf("review admission returned no unique source")
	}
	if assigned := d.Inputs[0].Assignment.SiteID; assigned != "" && assigned != site {
		return d, fmt.Errorf("declared site %s differs from admitted source site %s", site, assigned)
	}
	return d, nil
}

func prepare(o options, guard *eligibilitycli.CLIFlags) error {
	return prepareWith(o, guard, replayeval.Run, fieldrun.Analyse)
}

func prepareWith(o options, guard *eligibilitycli.CLIFlags,
	replayRun func(replayeval.Config) (*replayeval.Result, error),
	analyse func(sqlite.DBClient, fieldrun.Spec) (fieldrun.Result, error),
) error {
	if err := o.validate(); err != nil {
		return err
	}
	calibrationBytes, err := os.ReadFile(o.Calibration)
	if err != nil {
		return err
	}
	var calibration l4bobserve.Calibration
	if err = json.Unmarshal(calibrationBytes, &calibration); err != nil {
		return err
	}
	if _, err = l4bobserve.CalibrationID(calibration); err != nil {
		return err
	}
	id, err := eligibility.NewHasher().Capture(o.Capture)
	if err != nil {
		return err
	}
	admission, err := authorizeReview(guard, id, o.Site, o.Tuning)
	if err != nil {
		return err
	}
	// Mkdir, rather than MkdirAll, refuses any previous attempt's artefacts.
	if err = os.Mkdir(o.Output, 0755); err != nil {
		return err
	}
	cfg := replayeval.Config{PCAPFile: o.Capture, OutDir: filepath.Join(o.Output, "vrlog"),
		TuningFile: o.Tuning, SensorID: calibration.SensorID, UDPPort: o.Port,
		StartSeconds: o.Warmup, WarmupSeconds: o.Warmup, RequireSettled: true,
		IncludePoints: true, ForegroundOnly: true, ProgressEvery: 200, ObservationDBPath: filepath.Join(o.Output, "observations.db"),
		ClipID: o.Clip, ObservationCalibration: calibration, ObservationMaxSamplePoints: 256}
	result, err := replayRun(cfg)
	if err != nil {
		return err
	}
	replay, err := recorder.NewReplayer(result.VRLOGPath)
	if err != nil {
		return err
	}
	header := replay.Header()
	replay.Close()
	if header.StartNs <= 0 || header.EndNs <= header.StartNs || result.FramesRecorded == 0 {
		return fmt.Errorf("recording has no usable capture bounds")
	}
	window, err := cfg.ObservationWindow()
	if err != nil {
		return err
	}
	source := l4bobserve.CaptureSource{ClipID: o.Clip, CapturePaths: []string{o.Capture},
		CaptureSHA256s: []string{strings.TrimPrefix(id.SHA256, "sha256:")},
		ExtractorID:    "l4.dbscan_xy/v1/" + header.ParamsHash, Window: &window}
	sourceID, err := l4bobserve.SourceID(source)
	if err != nil {
		return err
	}
	if sourceID != result.ObservationSourceID {
		return fmt.Errorf("recording and observation source identities differ")
	}
	database, err := sqlite.OpenReadOnly(cfg.ObservationDBPath)
	if err != nil {
		return err
	}
	defer database.Close()
	r, err := analyse(database, fieldrun.Spec{SourceID: sourceID, Boxes: true, Stage: l8behaviour.StageOnline})
	if err != nil {
		return err
	}
	// Warm-up evidence remains in the immutable database, but no exported
	// sample may require a frame outside the paired recording's coverage.
	r.Trajectories = recordedTrajectories(r.Trajectories, header.StartNs, header.EndNs)
	r.Analysis = l8behaviour.FollowingAnalysis{}
	r.Interactions = nil
	context := fieldrun.EvidenceContext{SiteNamespace: "velocity.report", SiteID: o.Site, SourceID: sourceID,
		BindingID: o.Binding, BuildGitSHA: version.GitSHA, PolicyID: admission.PolicyDigest,
		SourceManifest: &source, CaptureFiles: []string{o.Capture}}
	doc, err := fieldrun.Evidence(r, context)
	if err != nil {
		return err
	}
	r = fieldrun.Result{}
	runtime.GC()
	export, err := writeCanonical(o.Output, doc)
	if err != nil {
		return err
	}
	calibrationID, _ := l4bobserve.CalibrationID(calibration)
	slices, err := writeSlices(o.Output, export, result.VRLOGPath, calibrationID, doc, header)
	if err != nil {
		return err
	}
	receipt := struct {
		Schema      string                   `json:"schema"`
		Context     fieldrun.EvidenceContext `json:"context"`
		Calibration l4bobserve.Calibration   `json:"calibration"`
		Header      recorder.LogHeader       `json:"recording_header"`
		VRLOG       string                   `json:"vrlog"`
		Evidence    string                   `json:"evidence"`
		Rows        int                      `json:"rows"`
		Purpose     eligibility.Purpose      `json:"purpose"`
		Slices      []sliceFile              `json:"slices"`
	}{"following_review_preparation_v1", context, calibration, header, result.VRLOGPath, export, len(doc.Rows), eligibility.PurposeAnnotationReview, slices}
	b, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(o.Output, "complete.json"), b, 0644); err != nil {
		return err
	}
	fmt.Printf("Review prepared: %s\n", export)
	return nil
}

func recordedTrajectories(in []l8behaviour.Trajectory, start, end int64) []l8behaviour.Trajectory {
	var out []l8behaviour.Trajectory
	for _, t := range in {
		samples := make([]l8behaviour.TrajectorySample, 0, len(t.Samples))
		for _, s := range t.Samples {
			if s.CaptureUnixNanos >= start && s.CaptureUnixNanos <= end {
				samples = append(samples, s)
			}
		}
		if len(samples) > 0 {
			t.Samples = samples
			out = append(out, t)
		}
	}
	return out
}
