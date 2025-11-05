package serialmux

import (
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
)

const radarFixture = `{"classifier" : "object_outbound", "end_time" : "1750719826.467", "start_time" : "1750719826.031", "delta_time_msec" : 736, "max_speed_mps" : 13.39, "min_speed_mps" : 11.33, "max_magnitude" : 55, "avg_magnitude" : 36, "total_frames" : 7, "frames_per_mps" : 0.5228, "length_m" : 9.86, "speed_change" : 2.799}`

func TestClassifyPayload(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`{"classifier":"x","end_time":123}`, EventTypeRadarObject},
		{`{"magnitude":1.2,"speed":3.4}`, EventTypeRawData},
		{`{"foo":"bar"}`, EventTypeConfig},
		{`plain text line`, EventTypeUnknown},
	}

	for _, c := range cases {
		got := ClassifyPayload(c.in)
		if got != c.want {
			t.Fatalf("ClassifyPayload(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestHandleConfigResponse_ValidAndInvalid(t *testing.T) {
	// reset state
	CurrentState = nil

	if err := HandleConfigResponse(`{"alpha":123,"beta":"x"}`); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if CurrentState == nil {
		t.Fatalf("expected CurrentState to be initialized")
	}
	if v, ok := CurrentState["alpha"]; !ok || v == nil {
		t.Fatalf("expected alpha in CurrentState")
	}

	// invalid JSON should return an error and not panic
	if err := HandleConfigResponse("not-json"); err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestHandleEvent_RadarObjectAndRawData(t *testing.T) {
	tmp := t.TempDir()
	d, err := db.NewDB(tmp + "/test.db")
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}
	defer d.Close()

	// Radar object
	if err := HandleEvent(d, radarFixture); err != nil {
		t.Fatalf("HandleEvent radar fixture failed: %v", err)
	}
	objs, err := d.RadarObjects()
	if err != nil {
		t.Fatalf("failed to read radar objects: %v", err)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 radar object, got %d", len(objs))
	}

	// Raw data event
	raw := `{"uptime": 1.23, "magnitude": 5.6, "speed": 7.8}`
	if err := HandleEvent(d, raw); err != nil {
		t.Fatalf("HandleEvent raw failed: %v", err)
	}
	events, err := d.Events()
	if err != nil {
		t.Fatalf("failed to read events: %v", err)
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}

	// quick sanity on timestamps handling in radar object (not strict match)
	if objs[0].StartTime.IsZero() || objs[0].EndTime.IsZero() {
		t.Fatalf("unexpected zero timestamps: start=%v end=%v", objs[0].StartTime, objs[0].EndTime)
	}

	// ensure DB operations persisted quickly
	time.Sleep(5 * time.Millisecond)
}
