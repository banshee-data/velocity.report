package sqlite

import (
	"reflect"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// testExtentSolidBody is testSolidBody with the extent state: its reading
// carries the 6x6 joint covariance over [x, y, vx, vy, hL, hW].
func testExtentSolidBody(t *testing.T, observationID string, frameUnixNanos int64) TrackSolidBody {
	t.Helper()
	sb := testSolidBody(observationID, frameUnixNanos)
	c := sb.Reading.Covariance
	joint := [l5tracks.JointCovarianceSize]float32{
		c[0], c[1], c[2], c[3], -0.012, 0.001,
		c[4], c[5], c[6], c[7], 0.002, -0.004,
		c[8], c[9], c[10], c[11], 0.003, 0,
		c[12], c[13], c[14], c[15], 0, 0.0005,
		-0.012, 0.002, 0.003, 0, 0.0625, 0.001,
		0.001, -0.004, 0, 0.0005, 0.001, 0.0625,
	}
	reading, err := sb.Reading.WithJointCovariance(joint)
	if err != nil {
		t.Fatal(err)
	}
	sb.Reading = reading
	return sb
}

// A reading with the extent state persists its 6x6 joint covariance under
// cv_cartesian_extent_v1 and reads back as it was written, extent blocks and
// all, with the estimate's in-memory state model unchanged; one without it is
// the 4x4 under cv_cartesian_v1, exactly as before.
func TestSolidBodyJointCovarianceRoundTripsWithItsStateModel(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewStateEstimateStore(database)
	extent := testExtentSolidBody(t, "observation/v1/extent", 100)
	plain := testSolidBody("observation/v1/plain", 200)
	for _, sb := range []TrackSolidBody{extent, plain} {
		if err := store.InsertSolidBody(sb); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.ListSolidBodiesBySource("source/v1/frame")
	if err != nil {
		t.Fatal(err)
	}
	if want := []TrackSolidBody{extent, plain}; !reflect.DeepEqual(got, want) {
		t.Fatalf("solid bodies did not round-trip:\n got %+v\nwant %+v", got, want)
	}
	if got[0].Reading.Estimate.StateModel != l5tracks.StateModelCVCartesianV1 || !got[0].Reading.ExtentState {
		t.Fatalf("read back state model %q, extent state %v", got[0].Reading.Estimate.StateModel, got[0].Reading.ExtentState)
	}
	for id, want := range map[string]struct {
		model string
		n     int
	}{extent.EstimateID: {l5tracks.StateModelCVCartesianExtentV1, 36}, plain.EstimateID: {l5tracks.StateModelCVCartesianV1, 16}} {
		var stateModel, blob string
		if err := database.QueryRow(`SELECT state_model, covariance_json FROM lidar_track_solid_bodies WHERE estimate_id = ?`, id).Scan(&stateModel, &blob); err != nil {
			t.Fatal(err)
		}
		if n := strings.Count(blob, ",") + 1; stateModel != want.model || n != want.n {
			t.Errorf("%s: persisted %q with %d elements, want %q with %d", id, stateModel, n, want.model, want.n)
		}
	}
}

// The joint layout's blob is read by its state model, never by its length:
// a 4x4 blob under the joint model, and a joint blob that is not one
// symmetric matrix, are refused on read; an estimate whose extent blocks or
// flag disagree with its reading's is refused on write.
func TestSolidBodyJointCovarianceRefusesWhatItCannotRead(t *testing.T) {
	writes := map[string]func(*TrackSolidBody){
		"extent blocks disagree": func(sb *TrackSolidBody) { sb.Reading.Estimate.HalfExtentCovariance[0] = 1 },
		"extent state disagrees": func(sb *TrackSolidBody) { sb.Reading.Estimate.ExtentState = false },
		"stray extent blocks": func(sb *TrackSolidBody) {
			sb.Reading.ExtentState, sb.Reading.Estimate.ExtentState = false, false
		},
	}
	for name, mutate := range writes {
		database, cleanup := setupTestDB(t)
		sb := testExtentSolidBody(t, "observation/v1/refused", 100)
		mutate(&sb)
		err := NewStateEstimateStore(database).InsertSolidBody(sb)
		cleanup()
		if err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	reads := map[string]string{
		"4x4 under the joint model": `UPDATE lidar_track_solid_bodies SET covariance_json = '[1,0,0,0,0,1,0,0,0,0,1,0,0,0,0,1]'`,
		"asymmetric joint": `UPDATE lidar_track_solid_bodies SET covariance_json =
			'[1,0,0,0,0.5,0, 0,1,0,0,0,0, 0,0,1,0,0,0, 0,0,0,1,0,0, 0,0,0,0,1,0, 0,0,0,0,0,1]'`,
	}
	for name, corrupt := range reads {
		database, cleanup := setupTestDB(t)
		store := NewStateEstimateStore(database)
		if err := store.InsertSolidBody(testExtentSolidBody(t, "observation/v1/corrupt", 100)); err != nil {
			cleanup()
			t.Fatal(err)
		}
		if _, err := database.Exec(corrupt); err != nil {
			cleanup()
			t.Fatalf("%s: %v", name, err)
		}
		_, err := store.ListSolidBodiesBySource("source/v1/frame")
		cleanup()
		if err == nil {
			t.Errorf("%s: a row the reader cannot interpret was returned", name)
		}
	}
}
