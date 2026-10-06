package db

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
)

func speedLimitSite(t *testing.T, db *DB) *Site {
	t.Helper()
	site := &Site{
		Name:     "Speed Limit Site",
		Location: "Main and First",
		Surveyor: "Surveyor",
		Contact:  "contact@example.com",
	}
	if err := db.CreateSite(context.Background(), site); err != nil {
		t.Fatalf("CreateSite failed: %v", err)
	}
	return site
}

// A posted limit, its unit and its jurisdiction are stored and read back by
// every reader, with the unit lower-cased and the jurisdiction trimmed.
func TestSiteConfigPeriodSpeedLimitRoundTrip(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	site := speedLimitSite(t, db)

	period := &SiteConfigPeriod{
		SiteID:             site.ID,
		EffectiveStartUnix: 1000,
		IsActive:           true,
		CosineErrorAngle:   5,
		SpeedLimitKph:      floatPtr(40.2336), // 25 mph
		SpeedLimitUnit:     strPtr(" MPH "),
		Jurisdiction:       strPtr("  US-CA "),
	}
	if err := db.CreateSiteConfigPeriod(period); err != nil {
		t.Fatalf("CreateSiteConfigPeriod failed: %v", err)
	}

	check := func(where string, got *SiteConfigPeriod) {
		t.Helper()
		if got.SpeedLimitKph == nil || *got.SpeedLimitKph != 40.2336 {
			t.Errorf("%s: speed_limit_kph = %v, want 40.2336", where, got.SpeedLimitKph)
		}
		if got.SpeedLimitUnit == nil || *got.SpeedLimitUnit != SpeedLimitUnitMph {
			t.Errorf("%s: speed_limit_unit = %v, want mph", where, got.SpeedLimitUnit)
		}
		if got.Jurisdiction == nil || *got.Jurisdiction != "US-CA" {
			t.Errorf("%s: jurisdiction = %v, want US-CA", where, got.Jurisdiction)
		}
	}
	got, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}
	check("get", got)
	active, err := db.GetActiveSiteConfigPeriod(site.ID)
	if err != nil {
		t.Fatalf("GetActiveSiteConfigPeriod failed: %v", err)
	}
	check("active", active)
	listed, err := db.ListSiteConfigPeriods(&site.ID)
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListSiteConfigPeriods = %v, %v; want one period", listed, err)
	}
	check("list", &listed[0])

	// A new sign: 30 km/h, and a blank jurisdiction reads as none.
	got.SpeedLimitKph = floatPtr(30)
	got.SpeedLimitUnit = strPtr("kph")
	got.Jurisdiction = strPtr("   ")
	if err := db.UpdateSiteConfigPeriod(got); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}
	updated, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}
	if updated.SpeedLimitKph == nil || *updated.SpeedLimitKph != 30 || updated.SpeedLimitUnit == nil || *updated.SpeedLimitUnit != SpeedLimitUnitKph {
		t.Errorf("after update: limit %v %v, want 30 kph", updated.SpeedLimitKph, updated.SpeedLimitUnit)
	}
	if updated.Jurisdiction != nil {
		t.Errorf("after update: jurisdiction = %q, want none", *updated.Jurisdiction)
	}

	// And the limit removed: a period may record none.
	updated.SpeedLimitKph, updated.SpeedLimitUnit = nil, nil
	if err := db.UpdateSiteConfigPeriod(updated); err != nil {
		t.Fatalf("UpdateSiteConfigPeriod failed: %v", err)
	}
	cleared, err := db.GetSiteConfigPeriod(period.ID)
	if err != nil {
		t.Fatalf("GetSiteConfigPeriod failed: %v", err)
	}
	if cleared.SpeedLimitKph != nil || cleared.SpeedLimitUnit != nil {
		t.Errorf("after clearing: limit %v %v, want none", cleared.SpeedLimitKph, cleared.SpeedLimitUnit)
	}
}

func TestSiteConfigPeriodSpeedLimitValidation(t *testing.T) {
	db := setupTestDB(t)
	defer cleanupTestDB(t, db)
	site := speedLimitSite(t, db)

	for _, c := range []struct {
		name      string
		limit     *float64
		unit      *string
		jurisdict *string
		want      string
	}{
		{"limit without unit", floatPtr(30), nil, nil, "needs speed_limit_unit"},
		{"unit without limit", nil, strPtr("kph"), nil, "set without speed_limit_kph"},
		{"unknown unit", floatPtr(30), strPtr("km/h"), nil, `must be "kph" or "mph"`},
		{"zero limit", floatPtr(0), strPtr("kph"), nil, "greater than 0"},
		{"negative limit", floatPtr(-30), strPtr("kph"), nil, "greater than 0"},
		{"limit above 200", floatPtr(250), strPtr("kph"), nil, "at most 200"},
		{"NaN limit", floatPtr(math.NaN()), strPtr("kph"), nil, "greater than 0"},
		{"infinite limit", floatPtr(math.Inf(1)), strPtr("kph"), nil, "greater than 0"},
		{"jurisdiction too long", nil, nil, strPtr(strings.Repeat("x", 101)), "at most 100 characters"},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := db.CreateSiteConfigPeriod(&SiteConfigPeriod{
				SiteID:             site.ID,
				EffectiveStartUnix: 1000,
				CosineErrorAngle:   5,
				SpeedLimitKph:      c.limit,
				SpeedLimitUnit:     c.unit,
				Jurisdiction:       c.jurisdict,
			})
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error containing %q", err, c.want)
			}
			if !IsSiteConfigPeriodValidationError(err) {
				t.Errorf("%v is not a validation error", err)
			}
		})
	}
}

// Every validation failure, old or new, is a validation error, so the API can
// answer 400 for it; a storage failure is not.
func TestSiteConfigPeriodValidationErrorsAreTyped(t *testing.T) {
	err := validateSiteConfigPeriod(&SiteConfigPeriod{SiteID: 1, EffectiveStartUnix: 1, CosineErrorAngle: 90})
	if !IsSiteConfigPeriodValidationError(err) || err.Error() != "cosine error angle must be between 0 and 80 degrees" {
		t.Errorf("got %v, want the cosine angle validation error with its message unchanged", err)
	}
	if IsSiteConfigPeriodValidationError(fmt.Errorf("failed to update site config period: %w", errors.New("database is locked"))) {
		t.Error("a storage error was taken for a validation error")
	}
}
