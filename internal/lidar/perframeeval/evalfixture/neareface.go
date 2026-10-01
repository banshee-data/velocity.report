package evalfixture

import (
	"fmt"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// The near-face scene, for the tests of the near-face scorer and its command:
// a car of length 4.4 m and width 1.8 m drives along +x at 10 m/s, one sample
// (100 ms) apart, 5 m to the sensor's left and 30 m ahead of it at sample 0.
// The sensor is at the origin, behind the car and to its right, so it sees the
// rear face (x = cx - 2.2, y from cy - 0.9 to cy + 0.9) and the right face
// (y = cy - 0.9, x from cx - 2.2 forward 2 m). The reviewed mask holds exactly
// those returns, ten on each face, so a body that puts both faces where they
// are has zero residuals and any other has the offset.
const (
	NearFaceLength  = 4.4
	NearFaceWidth   = 1.8
	NearFaceSamples = 6
	NearFaceObject  = "obj_car"
	NearFaceSplit   = "tune"
	NearFaceSource  = "src"
)

// NearFaceCentre is the car's true centre at a sample.
func NearFaceCentre(i int) (float64, float64) { return 30 + float64(i), 5 }

// NearFaceSampleNs is a sample's capture time.
func NearFaceSampleNs(i int) int64 { return 1_000_000_000 + int64(i)*100_000_000 }

// NearFaceOptions varies the pack.
type NearFaceOptions struct {
	// Proposed leaves the object proposed, so no mask is scored.
	Proposed bool
	// PartialRear keeps only the rear face's right third.
	PartialRear bool
}

// NearFaceFixture is where the pack and its split manifest were written.
type NearFaceFixture struct {
	PackDir, SplitManifestPath string
}

// WriteNearFacePack writes the pack, its reviewed sidecar and a version 1 split
// manifest (one tuning split and episode over every sample) under dir.
func WriteNearFacePack(dir string, o NearFaceOptions) (*NearFaceFixture, error) {
	packDir := filepath.Join(dir, "pack")
	var samples []annotation.Sample
	var blocks [][]byte
	masks := make([][]int, NearFaceSamples)
	for i := 0; i < NearFaceSamples; i++ {
		cx, cy := NearFaceCentre(i)
		var p annotation.Points
		add := func(x, y float64) int {
			p.X, p.Y, p.Z = append(p.X, float32(x)), append(p.Y, float32(y)), append(p.Z, 0.8)
			return len(p.X) - 1
		}
		for k := 0; k < 10; k++ {
			f := float64(k) / 9
			y := cy - NearFaceWidth/2 + f*NearFaceWidth // the rear face, the full width
			if o.PartialRear {
				y = cy - NearFaceWidth/2 + f*NearFaceWidth/3
			}
			masks[i] = append(masks[i], add(cx-NearFaceLength/2, y))
		}
		for k := 0; k < 10; k++ {
			masks[i] = append(masks[i], add(cx-NearFaceLength/2+float64(k)*2/9, cy-NearFaceWidth/2)) // the right face
		}
		block, err := annotation.EncodePoints(p)
		if err != nil {
			return nil, err
		}
		samples = append(samples, annotation.Sample{SourceOrdinal: i, SourceFrameID: uint64(i), TimestampNs: NearFaceSampleNs(i), SensorID: "nf", PointCount: len(p.X)})
		blocks = append(blocks, block)
	}
	m := annotation.Manifest{
		Coverage: annotation.CoverageForegroundOnly,
		Source:   annotation.SourceProvenance{SensorID: "nf", VRLOGHeaderSHA: "sha256:h", VRLOGFramesSHA: "sha256:f"},
		Coordinate: annotation.CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "sensor",
			Handedness: "right", OriginNote: "fixture"},
	}
	if err := annotation.WritePack(packDir, m, samples, blocks); err != nil {
		return nil, err
	}
	pack, err := annotation.OpenPack(packDir)
	if err != nil {
		return nil, err
	}
	status := annotation.StatusReviewed
	if o.Proposed {
		status = annotation.StatusProposed
	}
	s := annotation.NewSidecar(pack)
	s.Change = annotation.Provenance{Author: "fixture", Operation: "fixture"}
	s.Objects = []annotation.Object{{ObjectID: NearFaceObject, Class: "car", Confidence: 1, Status: status}}
	for i := 0; i < NearFaceSamples; i++ {
		s.Masks = append(s.Masks, annotation.FrameMask{ObjectID: NearFaceObject, SampleID: i, PointIndices: masks[i],
			Completeness: annotation.MaskPartial, Visibility: annotation.VisiblePresent, Status: status})
	}
	if err := annotation.SaveSidecar(pack, s); err != nil {
		return nil, err
	}
	split := annotation.SplitManifest{
		Schema: annotation.SplitSchema, SchemaVersion: annotation.SplitSchemaVersion,
		PackDigest: pack.Manifest.PackDigest, DatasetID: pack.Manifest.DatasetID, SidecarRevision: 1,
		Splits: []annotation.Split{{Name: NearFaceSplit, Role: annotation.SplitRoleTuning, ObjectIDs: []string{NearFaceObject}}},
		Episodes: []annotation.Episode{{EpisodeID: "ep", Split: NearFaceSplit, ObjectIDs: []string{NearFaceObject},
			FrameIntervals: []annotation.FrameInterval{{FirstSample: 0, LastSample: NearFaceSamples - 1}}}},
	}
	path := filepath.Join(dir, "split.json")
	if err := WriteSplitManifest(path, split); err != nil {
		return nil, err
	}
	return &NearFaceFixture{PackDir: packDir, SplitManifestPath: path}, nil
}

// NearFaceBody is one arm's belief at each sample, as an offset from the truth.
type NearFaceBody struct {
	// DX and DY move the believed centre.
	DX, DY float64
	// Length and Width believed; zero means the true ones.
	Length, Width float64
	// Prior makes both extents class priors rather than learnt.
	Prior bool
	// Ref is what the position refers to; zero means the body centre.
	Ref l5tracks.ReferencePoint
	// Missing leaves out the row at a sample.
	Missing func(i int) bool
	// NoHeading drops the orientation belief; NoExtent the length and width.
	NoHeading, NoExtent bool
	// Far moves the body out of every gate.
	Far bool
}

// WriteNearFaceDB writes one solid-body version (parameter hash) per entry of
// arms, each a body following the car, to a new evidence database at path.
func WriteNearFaceDB(path string, arms map[string]NearFaceBody) error {
	database, err := db.NewDB(path)
	if err != nil {
		return err
	}
	defer database.Close()
	store := sqlite.NewStateEstimateStore(database.DB)
	for params, b := range arms {
		for i := 0; i < NearFaceSamples; i++ {
			if b.Missing != nil && b.Missing(i) {
				continue
			}
			cx, cy := NearFaceCentre(i)
			cx, cy = cx+b.DX, cy+b.DY
			if b.Far {
				cx += 100
			}
			frame := NearFaceSampleNs(i) + 300_000
			id := fmt.Sprintf("%s/%d", params, i)
			if _, err := database.Exec(`INSERT INTO lidar_observations
				(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
				 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
				VALUES (?, 1, ?, 'cal', 'nf', 'f', ?, ?, 1, '{}', 1)`, "observation/"+id, NearFaceSource, frame, frame); err != nil {
				return err
			}
			prov := l5tracks.ProvenanceAccumulated
			if b.Prior {
				prov = l5tracks.ProvenanceClassPrior
			}
			length, width := b.Length, b.Width
			if length == 0 {
				length = NearFaceLength
			}
			if width == 0 {
				width = NearFaceWidth
			}
			lengthBelief := l5tracks.DimensionBelief{Metres: float32(length), SigmaMetres: 0.1, AdmissibleFrames: 6, Provenance: prov}
			widthBelief := l5tracks.DimensionBelief{Metres: float32(width), SigmaMetres: 0.1, AdmissibleFrames: 6, Provenance: prov}
			if b.NoExtent {
				lengthBelief, widthBelief = l5tracks.DimensionBelief{}, l5tracks.DimensionBelief{}
			}
			ref := b.Ref
			if ref == 0 {
				ref = l5tracks.ReferenceBodyCentre
			}
			orientation := l5tracks.OrientationBelief{PsiRad: 0, VarianceRad2: 0.0025, Provenance: l5tracks.ProvenanceObserved}
			if b.NoHeading {
				orientation = l5tracks.OrientationBelief{}
			}
			cov := [16]float32{0.01, 0, 0, 0, 0, 0.01, 0, 0, 0, 0, 0.1, 0, 0, 0, 0, 0.1}
			if err := store.InsertSolidBody(sqlite.TrackSolidBody{
				EstimateID: "solid_body/" + id, TrackID: "uuid-" + params, ObservationID: "observation/" + id,
				SourceID: NearFaceSource, CalibrationID: "cal", FrameUnixNanos: frame, MeasurementUnixNanos: frame,
				EstimatorID: EstimatorID, ObservationModelID: string(l5tracks.MeasurementNearEdgeCandidateV1), ParamHash: params,
				Stage: "online", CreationSequence: 1,
				Reading: l5tracks.SolidBodyReading{
					Estimate: l5tracks.SolidBodyEstimate{
						StateModel: l5tracks.StateModelCVCartesianV1, Reference: ref, X: float32(cx), Y: float32(cy),
						PositionCovariance:    [4]float32{cov[0], cov[1], cov[4], cov[5]},
						Orientation:           orientation,
						Length:                lengthBelief,
						Width:                 widthBelief,
						Height:                l5tracks.DimensionBelief{Metres: 1.5, SigmaMetres: 0.3, Provenance: l5tracks.ProvenanceClassPrior},
						Motion:                l5tracks.MotionClassBelief{Class: l5tracks.MotionRigidVehicle, Posterior: 0.9},
						Estimation:            l5tracks.EstimationGeometryConverging,
						Stage:                 l5tracks.StageLive,
						LastObservedUnixNanos: frame,
						Support:               l5tracks.SupportState{PointCount: 8},
					},
					VX: 10, Covariance: cov,
					Measurement: l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: 2},
				},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
