package db

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestTransitDirectionFromSigns(t *testing.T) {
	cases := []struct {
		signs []float64
		want  string
	}{
		{[]float64{10, 11, 12}, TransitInbound},
		{[]float64{-10, -11}, TransitOutbound},
		{[]float64{10, 10, 10, 10, 10, 10, 10, 10, 10, -10}, TransitInbound}, // one in ten
		{[]float64{10, 10, 10, 10, 10, 10, 10, 10, -10, -10}, TransitMixed},  // two in ten
		{[]float64{0, 0}, TransitUnknown},
		{nil, TransitUnknown},
	}
	for _, c := range cases {
		if got := TransitDirection(c.signs); got != c.want {
			t.Errorf("%v: %s, want %s", c.signs, got, c.want)
		}
	}
}

func TestRadarDirectionsValidateAndFlip(t *testing.T) {
	ok := RadarDirections{InboundSign: 1, Directions: []DirectionConfig{
		{Key: "inbound", Name: "Kirkham St eastbound", Limits: []DirectionLimit{{Value: 25, Unit: "mph", Source: "sign", Jurisdiction: "US-CA", EffectiveFrom: "2026-01-01"}}},
		{Key: "outbound", Name: "Kirkham St westbound"},
	}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []RadarDirections{
		{InboundSign: 0},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "north", Name: "x"}}},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "inbound", Name: "a"}, {Key: "inbound", Name: "b"}}},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "inbound"}}},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "inbound", Name: "a", Limits: []DirectionLimit{{Value: 91, Unit: "mph", Source: "sign"}}}}},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "inbound", Name: "a", Limits: []DirectionLimit{{Value: 25, Unit: "kph", Source: "sign"}}}}},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "inbound", Name: "a", Limits: []DirectionLimit{{Value: 25, Unit: "mph", Source: "guess"}}}}},
		{InboundSign: 1, Directions: []DirectionConfig{{Key: "inbound", Name: "a", Limits: []DirectionLimit{{Value: 25, Unit: "mph", Source: "sign", EffectiveFrom: "June"}}}}},
	}
	for i, r := range bad {
		if err := r.Validate(); err == nil {
			t.Errorf("set %d accepted", i)
		}
	}
	flipped := &RadarDirections{InboundSign: -1}
	if flipped.Direction(TransitInbound) != TransitOutbound || flipped.Direction(TransitMixed) != TransitMixed || ok.Direction(TransitInbound) != TransitInbound {
		t.Fatal("sign mapping")
	}
}

func TestTheWorkerKeepsTheSignAndReportsReadThroughTheDeployment(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	copyTemplateDB(t, dbPath)
	d, err := NewDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	const t0 = 1_788_000_000.0
	insert := func(ts, speed float64) {
		t.Helper()
		if _, err := d.Exec(`INSERT INTO radar_data (write_timestamp, raw_event) VALUES (?, ?)`, ts, fmt.Sprintf(`{"uptime": 1, "magnitude": 40, "speed": %v}`, speed)); err != nil {
			t.Fatal(err)
		}
	}
	// An approaching car, a receding one a minute later, and two at the
	// same speed in opposite directions that the clustering merges.
	for i := 0; i < 8; i++ {
		insert(t0+float64(i)*0.2, 11+0.1*float64(i))
		insert(t0+60+float64(i)*0.2, -(13 + 0.1*float64(i)))
	}
	for i := 0; i < 4; i++ {
		insert(t0+120+float64(i)*0.2, 9)
		insert(t0+120.1+float64(i)*0.2, -9)
	}
	if err := NewTransitWorker(d, 5, "test-v1").RunRange(context.Background(), t0-10, t0+200); err != nil {
		t.Fatal(err)
	}
	rows, err := d.Query(`SELECT direction FROM radar_data_transits ORDER BY transit_start_unix`)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for rows.Next() {
		var s string
		_ = rows.Scan(&s)
		got = append(got, s)
	}
	rows.Close()
	if fmt.Sprint(got) != "[inbound outbound mixed]" {
		t.Fatalf("directions %v", got)
	}

	// The deployment names its directions with the sensor wired the other
	// way round: inbound readings are its outbound.
	res, err := d.Exec(`INSERT INTO site (name, location, surveyor, contact) VALUES ('directions test', 'l', 'x', 'y')`)
	if err != nil {
		t.Fatal(err)
	}
	siteID64, _ := res.LastInsertId()
	siteID := int(siteID64)
	period := &SiteConfigPeriod{SiteID: siteID, EffectiveStartUnix: t0 - 1000, IsActive: true, Directions: &RadarDirections{InboundSign: -1, Directions: []DirectionConfig{
		{Key: "inbound", Name: "Kirkham St eastbound", Limits: []DirectionLimit{{Value: 25, Unit: "mph", Source: "sign"}}},
		{Key: "outbound", Name: "Kirkham St westbound"},
	}}}
	if err := d.CreateSiteConfigPeriod(period); err != nil {
		t.Fatal(err)
	}
	back, err := d.GetSiteConfigPeriod(period.ID)
	if err != nil || back.Directions == nil || back.Directions.InboundSign != -1 || back.Directions.Config("inbound").Name != "Kirkham St eastbound" {
		t.Fatalf("round trip: %v %+v", err, back)
	}
	ts, err := d.TransitDirections(int64(t0-10), int64(t0+200), siteID, "test-v1", 1)
	if err != nil || len(ts) != 3 {
		t.Fatalf("transits: %v %d", err, len(ts))
	}
	if ts[0].Direction != TransitOutbound || ts[1].Direction != TransitInbound || ts[2].Direction != TransitMixed || ts[0].Directions == nil {
		t.Fatalf("mapped %+v", ts)
	}
	// A malformed set is refused.
	period.Directions = &RadarDirections{InboundSign: 2}
	if err := d.UpdateSiteConfigPeriod(period); err == nil {
		t.Fatal("bad directions saved")
	}
}
