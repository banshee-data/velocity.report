package api

import "testing"

func TestSensorModelsEmbed(t *testing.T) {
	if len(SupportedSensorModels) < 2 {
		t.Fatalf("expected at least 2 sensor models, got %d", len(SupportedSensorModels))
	}

	a, ok := GetSensorModel("ops243-a")
	if !ok {
		t.Fatal("ops243-a missing from registry")
	}
	if a.DisplayName == "" {
		t.Error("ops243-a missing display_name")
	}
	if a.DefaultBaudRate != 19200 {
		t.Errorf("ops243-a default_baud_rate = %d, want 19200", a.DefaultBaudRate)
	}
	if len(a.InitCommands) == 0 {
		t.Error("ops243-a has no init_commands")
	}
	if len(a.SupportedBaudRates) == 0 {
		t.Error("ops243-a has no supported_baud_rates")
	}
	if !a.HasDoppler || a.HasFMCW || a.HasDistance {
		t.Errorf("ops243-a capability flags wrong: doppler=%v fmcw=%v distance=%v",
			a.HasDoppler, a.HasFMCW, a.HasDistance)
	}

	c, ok := GetSensorModel("ops243-c")
	if !ok {
		t.Fatal("ops243-c missing from registry")
	}
	if !c.HasDoppler || !c.HasFMCW || !c.HasDistance {
		t.Errorf("ops243-c should have all capabilities: doppler=%v fmcw=%v distance=%v",
			c.HasDoppler, c.HasFMCW, c.HasDistance)
	}

	all := GetAllSensorModels()
	if len(all) != len(SupportedSensorModels) {
		t.Errorf("GetAllSensorModels returned %d, want %d", len(all), len(SupportedSensorModels))
	}
}

func TestSensorModelsLookupMissing(t *testing.T) {
	if _, ok := GetSensorModel("does-not-exist"); ok {
		t.Error("expected lookup of unknown slug to return ok=false")
	}
}

// TestGetAllSensorModelsSortedBySlug verifies the slice is returned in a stable
// slug-sorted order. SupportedSensorModels is a map, so without the sort the
// order is nondeterministic and /api/serial/models could flake between runs.
func TestGetAllSensorModelsSortedBySlug(t *testing.T) {
	all := GetAllSensorModels()
	if len(all) < 2 {
		t.Fatalf("expected at least 2 sensor models, got %d", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i-1].Slug > all[i].Slug {
			t.Errorf("GetAllSensorModels not sorted by slug: %q precedes %q", all[i-1].Slug, all[i].Slug)
		}
	}
}
