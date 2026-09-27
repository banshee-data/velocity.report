package l6objects

import (
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// l5tracks.ClassIdentity refuses a labelled track a cluster outside the
// label's extent envelope. The envelopes restate this package's thresholds,
// because this package imports l5tracks and not the reverse; this pins each
// one to the threshold it mirrors so the two cannot drift.
func TestClassIdentityEnvelopesMirrorTheClassificationThresholds(t *testing.T) {
	want := map[ObjectClass]l5tracks.ClassExtent{
		ClassPedestrian:   {Long: VehicleLengthMin, Short: VehicleWidthMin},
		ClassCyclist:      {Long: CyclistLengthMax, Short: CyclistWidthMax},
		ClassMotorcyclist: {Long: MotorcyclistLengthMax, Short: MotorcyclistWidthMax},
	}
	for class, w := range want {
		got, ok := l5tracks.ClassExtentEnvelope(string(class))
		if !ok || got != w {
			t.Errorf("%s: envelope %+v (present %v), want %+v", class, got, ok, w)
		}
	}
	// A partial view of a vehicle can be any size below it, and the other
	// labels are not road users: none carries an envelope.
	for _, class := range []ObjectClass{ClassCar, ClassTruck, ClassBus, ClassBird, ClassDynamic} {
		if e, ok := l5tracks.ClassExtentEnvelope(string(class)); ok {
			t.Errorf("%s has an envelope %+v; only the small road-user labels should", class, e)
		}
	}
}
