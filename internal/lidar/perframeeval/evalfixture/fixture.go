// Package evalfixture writes a small synthetic pack, review sidecar, split
// manifest and evidence database whose per-frame scores can be counted by
// hand. It exists for tests of the per-frame acceptance harness and of the
// command that fronts it; nothing in production imports it.
//
// The scene, ten samples 100 ms apart, every object moving along +x at one
// metre per sample (x = sample index):
//
//	obj_car_a   car, reviewed, held out, y = 0. Fully occluded at sample 5.
//	obj_car_b   car, reviewed, tuning split, y = 10.
//	obj_ped_p   pedestrian, only proposed, held out, y = 20.
//	obj_noise_n noise, reviewed, held out, y = -10.
//
// Episode hold-ep1 scores obj_car_a and obj_ped_p over every sample; tune-ep1
// scores obj_car_b. Arm A tracks everything exactly. Arm B loses obj_car_a at
// samples 4 and 5 and picks it up under a new identity at 6, and a ghost 40 m
// off the road lives for two samples. Both are written as final estimates;
// arm A also as online estimates; and both again as analysis runs.
package evalfixture

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Timing.
const (
	Samples          = 10
	FirstSampleNs    = int64(1_000_000_000)
	SamplePeriodNs   = int64(100_000_000)
	EstimateOffsetNs = int64(300_000)  // estimates are stamped 0.3 ms late
	RunOffsetNs      = int64(-200_000) // run observations 0.2 ms early
)

// Reference identities.
const (
	CarA  = "obj_car_a"
	CarB  = "obj_car_b"
	Ped   = "obj_ped_p"
	Noise = "obj_noise_n"

	SplitHeldOut   = "hold"
	SplitTuning    = "tune"
	EpisodeHeldOut = "hold-ep1"
	EpisodeTuning  = "tune-ep1"
	OccludedSample = 5
)

// Hypothesis identities.
const (
	SourceID           = "source/v1/fixture"
	EstimatorID        = "cv_kf_v1"
	ObservationModelID = "medoid_v1"
	ParamsA            = "params/a"
	ParamsB            = "params/b"
	RunA               = "run-a"
	RunB               = "run-b"
)

// Fixture is where the files were written.
type Fixture struct {
	PackDir           string
	SplitManifestPath string
	DBPath            string
	PackDigest        string
	DatasetID         string
}

// SampleTime is a sample's capture timestamp.
func SampleTime(i int) int64 { return FirstSampleNs + int64(i)*SamplePeriodNs }

type object struct {
	id, class string
	status    annotation.ReviewStatus
	y         float32
	// half-extent of the square of returns around the centre
	halfX, halfY float32
}

var scene = []object{
	{CarA, "car", annotation.StatusReviewed, 0, 1, 0.5},
	{CarB, "car", annotation.StatusReviewed, 10, 1, 0.5},
	{Ped, "pedestrian", annotation.StatusProposed, 20, 0.2, 0.2},
	{Noise, "noise", annotation.StatusReviewed, -10, 0.3, 0.3},
}

// Write creates the pack, sidecar, manifest and database under dir.
func Write(dir string) (*Fixture, error) {
	f := &Fixture{
		PackDir:           filepath.Join(dir, "pack"),
		SplitManifestPath: filepath.Join(dir, "split.json"),
		DBPath:            filepath.Join(dir, "evidence.db"),
	}
	pack, err := writePack(f.PackDir)
	if err != nil {
		return nil, err
	}
	f.PackDigest, f.DatasetID = pack.Manifest.PackDigest, pack.Manifest.DatasetID
	if err := WriteSplitManifest(f.SplitManifestPath, f.Manifest()); err != nil {
		return nil, err
	}
	if err := WriteDB(f.DBPath, []string{"final", "online"}); err != nil {
		return nil, err
	}
	return f, nil
}

func writePack(dir string) (*annotation.Pack, error) {
	var samples []annotation.Sample
	var blocks [][]byte
	for i := 0; i < Samples; i++ {
		var p annotation.Points
		x := float32(i)
		for _, o := range scene {
			for _, corner := range [][2]float32{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
				p.X = append(p.X, x+corner[0]*o.halfX)
				p.Y = append(p.Y, o.y+corner[1]*o.halfY)
				p.Z = append(p.Z, 0.8)
			}
		}
		block, err := annotation.EncodePoints(p)
		if err != nil {
			return nil, err
		}
		samples = append(samples, annotation.Sample{
			SourceOrdinal: i, SourceFrameID: uint64(i), TimestampNs: SampleTime(i),
			SensorID: "fixture", PointCount: len(p.X),
		})
		blocks = append(blocks, block)
	}
	m := annotation.Manifest{Coverage: annotation.CoverageForegroundOnly, Source: annotation.SourceProvenance{SensorID: "fixture"}}
	if err := annotation.WritePack(dir, m, samples, blocks); err != nil {
		return nil, err
	}
	pack, err := annotation.OpenPack(dir)
	if err != nil {
		return nil, err
	}

	s := annotation.NewSidecar(pack)
	for k, o := range scene {
		s.Objects = append(s.Objects, annotation.Object{ObjectID: o.id, Class: o.class, Confidence: 1, Status: o.status})
		for i := 0; i < Samples; i++ {
			m := annotation.FrameMask{
				ObjectID: o.id, SampleID: i,
				PointIndices: []int{4 * k, 4*k + 1, 4*k + 2, 4*k + 3},
				Completeness: annotation.MaskComplete, Visibility: annotation.VisiblePresent, Status: o.status,
			}
			if o.status == annotation.StatusProposed {
				m.Completeness = annotation.MaskPartial
			}
			if o.id == CarA && i == OccludedSample {
				m.Visibility = annotation.VisibleFullyOccluded
			}
			s.Masks = append(s.Masks, m)
		}
	}
	s.Change = annotation.Provenance{Author: "fixture", Operation: "fixture"}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		return nil, err
	}
	return pack, nil
}

// Manifest is the fixture's split manifest, for tests to vary.
func (f *Fixture) Manifest() annotation.SplitManifest {
	return annotation.SplitManifest{
		Schema: annotation.SplitSchema, SchemaVersion: annotation.SplitSchemaVersion,
		PackDigest: f.PackDigest, DatasetID: f.DatasetID, SidecarRevision: 1,
		Splits: []annotation.Split{
			{Name: SplitTuning, Role: annotation.SplitRoleTuning, ObjectIDs: []string{CarB}},
			{Name: SplitHeldOut, Role: annotation.SplitRoleHeldOut, ObjectIDs: []string{CarA, Noise, Ped}},
		},
		Episodes: []annotation.Episode{
			{EpisodeID: EpisodeHeldOut, Split: SplitHeldOut, ObjectIDs: []string{CarA, Ped},
				FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: Samples - 1}}},
			{EpisodeID: EpisodeTuning, Split: SplitTuning, ObjectIDs: []string{CarB},
				FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: Samples - 1}}},
		},
	}
}

// WriteSplitManifest writes a manifest as JSON.
func WriteSplitManifest(path string, m annotation.SplitManifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(b, '\n'), 0o644)
}

// track is one hypothesis: which object it follows (by y) over which samples.
type track struct {
	seq      int64
	y        float32
	from, to int
}

// ArmA tracks every object exactly.
var ArmA = []track{{1, 0, 0, 9}, {2, 10, 0, 9}, {3, 20, 0, 9}, {4, -10, 0, 9}}

// ArmB splits obj_car_a around samples 4-5 and adds a two-sample ghost.
var ArmB = []track{{1, 0, 0, 3}, {2, 0, 6, 9}, {3, 10, 0, 9}, {4, 20, 0, 9}, {5, 40, 0, 1}}

// WriteDB writes both arms' estimates at each stage (arm B at final only) and
// both arms as analysis runs.
func WriteDB(path string, stages []string) error {
	database, err := db.NewDB(path)
	if err != nil {
		return err
	}
	defer database.Close()
	states := sqlite.NewStateEstimateStore(database.DB)
	for _, stage := range stages {
		arms := map[string][]track{ParamsA: ArmA}
		if stage == "final" {
			arms[ParamsB] = ArmB
		}
		for _, params := range []string{ParamsA, ParamsB} {
			for _, tr := range arms[params] {
				for i := tr.from; i <= tr.to; i++ {
					id := fmt.Sprintf("estimate/%s/%s/%d/%d", params, stage, tr.seq, i)
					frame := SampleTime(i) + EstimateOffsetNs
					e := sqlite.TrackEstimate{
						EstimateID: id, TrackID: fmt.Sprintf("uuid-%s-%d", params, tr.seq),
						ObservationID: "observation/" + id, SourceID: SourceID, CalibrationID: "calibration/v1/fixture",
						FrameUnixNanos: frame, MeasurementUnixNanos: frame,
						EstimatorID: EstimatorID, ObservationModelID: ObservationModelID, ParamHash: params,
						Stage: stage, MeasurementSource: ObservationModelID, CreationSequence: tr.seq,
						X: float32(i), Y: tr.y,
					}
					r := sqlite.TrackResidual{EstimateID: id, ObservationID: e.ObservationID, Disposition: "accepted", Reason: "fixture"}
					if err := states.Insert(e, r); err != nil {
						return err
					}
				}
			}
		}
	}

	runs := sqlite.NewAnalysisRunStore(database.DB)
	for runID, arm := range map[string][]track{RunA: ArmA, RunB: ArmB} {
		if err := runs.InsertRun(&sqlite.AnalysisRun{
			RunID: runID, SourceType: "pcap", SourcePath: "fixture.pcap", SensorID: "fixture", Status: "completed",
		}); err != nil {
			return err
		}
		for _, tr := range arm {
			// IDs that sort against the tracks' order in time, so a test
			// can see the run adapter renames by content.
			trackID := fmt.Sprintf("zz-%s-%d", runID, 10-tr.seq)
			m := sqlite.TrackMeasurement{SensorID: "fixture", TrackState: sqlite.TrackConfirmed}
			if err := runs.InsertRunTrack(&sqlite.RunTrack{RunID: runID, TrackID: trackID, TrackMeasurement: m}); err != nil {
				return err
			}
			if err := sqlite.InsertTrack(database.DB, &sqlite.TrackedObject{TrackID: trackID, TrackMeasurement: m}, "fixture"); err != nil {
				return err
			}
			for i := tr.from; i <= tr.to; i++ {
				obs := sqlite.TrackObservation{
					TrackID: trackID, TSUnixNanos: SampleTime(i) + RunOffsetNs, FrameUnixNanos: SampleTime(i) + RunOffsetNs,
					FrameID: "fixture", X: float32(i), Y: tr.y,
				}
				if err := sqlite.InsertTrackObservation(database.DB, &obs); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
