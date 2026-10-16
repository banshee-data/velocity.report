package evalfixture

import (
	"fmt"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// RetentionSourceID is the evidence source WriteRetentionDB writes.
const RetentionSourceID = "source/v1/fixture-retention"

// WriteRetentionDB writes a replay's cluster evidence for the retention
// measure over the held-out split, whose only scored object is obj_car_a:
// its four returns are pack indices 0 to 3 in every sample, and obj_noise_n's
// are 12 to 15. Every sample point is a pack point, read from the pack, so the
// match is the exact identity a replay's would be.
//
//	sample 0  car a split two and two over clusters 1 and 2: two fragments
//	sample 1  car a whole in cluster 1, with one noise return: retention 1
//	sample 2  half of car a in cluster 1: retention 0.5
//	sample 3  cluster 1 holds eight points, sampled two: one car a return and
//	          one noise return, so car a's share expands to 1 x 8 / 2 = 4
//	sample 4  only the noise object clustered: car a missed
//	sample 5  car a is fully occluded there, so the policy does not score it
//	sample 6  no cluster at all: a sample with no evidence frame
//	sample 7  only the noise object clustered: car a missed
//	sample 8  only the noise object clustered: car a missed
//	sample 9  past the last evidence frame: outside the window
func WriteRetentionDB(path, packDir string) error {
	pack, err := annotation.OpenPack(packDir)
	if err != nil {
		return err
	}
	database, err := db.NewDB(path)
	if err != nil {
		return err
	}
	defer database.Close()
	store := sqlite.NewObservationStore(database.DB)

	type cluster struct {
		population int
		members    []int // pack indices in the retained sample
	}
	noise := []int{12, 13, 14, 15}
	frames := map[int][]cluster{
		0: {{2, []int{0, 1}}, {2, []int{2, 3}}},
		1: {{5, []int{0, 1, 2, 3, 12}}},
		2: {{2, []int{0, 1}}},
		3: {{8, []int{0, 12}}},
		4: {{4, noise}},
		5: {{4, noise}},
		7: {{4, noise}},
		8: {{4, noise}},
	}
	for sample, clusters := range frames {
		pts, err := pack.PointsAt(sample)
		if err != nil {
			return err
		}
		for c, cl := range clusters {
			wc := l4perception.WorldCluster{ClusterID: int64(c + 1), SensorID: "fixture", FrameID: "site/fixture",
				TSUnixNanos: SampleTime(sample), PointsCount: cl.population}
			for _, i := range cl.members {
				wc.SamplePoints = append(wc.SamplePoints, [3]float32{pts.X[i], pts.Y[i], pts.Z[i]})
				wc.RetainedPoints = append(wc.RetainedPoints, l4perception.WorldPoint{
					X: float64(pts.X[i]), Y: float64(pts.Y[i]), Z: float64(pts.Z[i]), SensorID: "fixture"})
			}
			o, err := l4bobserve.New(l4bobserve.Record{SchemaVersion: 1,
				ObservationID: fmt.Sprintf("observation/v1/fixture/%d/%d", sample, c+1), SourceID: RetentionSourceID,
				CalibrationID: "calibration/v1/fixture", FrameUnixNanos: SampleTime(sample), Cluster: wc})
			if err != nil {
				return err
			}
			if err := store.Insert(o); err != nil {
				return err
			}
		}
	}
	return nil
}
