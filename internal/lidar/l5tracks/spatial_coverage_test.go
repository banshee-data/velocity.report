package l5tracks

import "testing"

func TestSpatialCoverage(t *testing.T) {
	cases := []struct {
		name         string
		observations int
		durationSecs float32
		want         float32
		defined      bool
	}{
		{"half of a 10 Hz track", 10, 2, 0.5, true},
		{"clamped at 1", 30, 2, 1, true},
		{"no observations", 0, 2, 0, false},
		{"no elapsed time", 5, 0, 0, false},
		{"negative duration", 5, -1, 0, false},
	}
	for _, c := range cases {
		got, ok := SpatialCoverage(c.observations, c.durationSecs)
		if got != c.want || ok != c.defined {
			t.Errorf("%s: SpatialCoverage(%d, %v) = %v, %v; want %v, %v", c.name, c.observations, c.durationSecs, got, ok, c.want, c.defined)
		}
	}
}
