package db

import (
	"testing"
)

func TestSpeedLimitSchedule(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Create a test site first
	site := &Site{
		Name:             "Test Site",
		Location:         "Test Location",
		CosineErrorAngle: 0.5,
		SpeedLimit:       25,
		Surveyor:         "Test Surveyor",
		Contact:          "test@example.com",
	}
	if err := db.CreateSite(site); err != nil {
		t.Fatalf("failed to create test site: %v", err)
	}

	t.Run("CreateSpeedLimitSchedule", func(t *testing.T) {
		schedule := &SpeedLimitSchedule{
			SiteID:     site.ID,
			DayOfWeek:  1, // Monday
			StartTime:  "06:00",
			EndTime:    "07:05",
			SpeedLimit: 15, // School zone speed
		}

		err := db.CreateSpeedLimitSchedule(schedule)
		if err != nil {
			t.Fatalf("failed to create speed limit schedule: %v", err)
		}

		if schedule.ID == 0 {
			t.Error("expected schedule ID to be set")
		}
	})

	t.Run("GetSpeedLimitSchedule", func(t *testing.T) {
		// Create a schedule
		schedule := &SpeedLimitSchedule{
			SiteID:     site.ID,
			DayOfWeek:  2, // Tuesday
			StartTime:  "14:00",
			EndTime:    "15:00",
			SpeedLimit: 20,
		}
		if err := db.CreateSpeedLimitSchedule(schedule); err != nil {
			t.Fatalf("failed to create schedule: %v", err)
		}

		// Retrieve it
		retrieved, err := db.GetSpeedLimitSchedule(schedule.ID)
		if err != nil {
			t.Fatalf("failed to get schedule: %v", err)
		}

		if retrieved.SiteID != schedule.SiteID {
			t.Errorf("expected site_id %d, got %d", schedule.SiteID, retrieved.SiteID)
		}
		if retrieved.DayOfWeek != schedule.DayOfWeek {
			t.Errorf("expected day_of_week %d, got %d", schedule.DayOfWeek, retrieved.DayOfWeek)
		}
		if retrieved.StartTime != schedule.StartTime {
			t.Errorf("expected start_time %s, got %s", schedule.StartTime, retrieved.StartTime)
		}
		if retrieved.EndTime != schedule.EndTime {
			t.Errorf("expected end_time %s, got %s", schedule.EndTime, retrieved.EndTime)
		}
		if retrieved.SpeedLimit != schedule.SpeedLimit {
			t.Errorf("expected speed_limit %d, got %d", schedule.SpeedLimit, retrieved.SpeedLimit)
		}
	})

	t.Run("GetSpeedLimitSchedulesForSite", func(t *testing.T) {
		// Create multiple schedules for the site
		schedules := []*SpeedLimitSchedule{
			{SiteID: site.ID, DayOfWeek: 0, StartTime: "08:00", EndTime: "09:00", SpeedLimit: 25},
			{SiteID: site.ID, DayOfWeek: 1, StartTime: "08:00", EndTime: "09:00", SpeedLimit: 15},
			{SiteID: site.ID, DayOfWeek: 5, StartTime: "16:00", EndTime: "17:00", SpeedLimit: 20},
		}

		for _, s := range schedules {
			if err := db.CreateSpeedLimitSchedule(s); err != nil {
				t.Fatalf("failed to create schedule: %v", err)
			}
		}

		// Retrieve all schedules for the site
		retrieved, err := db.GetSpeedLimitSchedulesForSite(site.ID)
		if err != nil {
			t.Fatalf("failed to get schedules: %v", err)
		}

		// We should have at least the 3 we just created (plus any from other tests)
		if len(retrieved) < 3 {
			t.Errorf("expected at least 3 schedules, got %d", len(retrieved))
		}

		// Verify they're sorted by day_of_week and start_time
		for i := 1; i < len(retrieved); i++ {
			prev := retrieved[i-1]
			curr := retrieved[i]

			if prev.DayOfWeek > curr.DayOfWeek {
				t.Error("schedules not sorted by day_of_week")
			}
			if prev.DayOfWeek == curr.DayOfWeek && prev.StartTime > curr.StartTime {
				t.Error("schedules not sorted by start_time within same day")
			}
		}
	})

	t.Run("UpdateSpeedLimitSchedule", func(t *testing.T) {
		// Create a schedule
		schedule := &SpeedLimitSchedule{
			SiteID:     site.ID,
			DayOfWeek:  3, // Wednesday
			StartTime:  "10:00",
			EndTime:    "11:00",
			SpeedLimit: 25,
		}
		if err := db.CreateSpeedLimitSchedule(schedule); err != nil {
			t.Fatalf("failed to create schedule: %v", err)
		}

		// Update it
		schedule.SpeedLimit = 30
		schedule.StartTime = "10:30"
		if err := db.UpdateSpeedLimitSchedule(schedule); err != nil {
			t.Fatalf("failed to update schedule: %v", err)
		}

		// Verify the update
		retrieved, err := db.GetSpeedLimitSchedule(schedule.ID)
		if err != nil {
			t.Fatalf("failed to get schedule: %v", err)
		}

		if retrieved.SpeedLimit != 30 {
			t.Errorf("expected speed_limit 30, got %d", retrieved.SpeedLimit)
		}
		if retrieved.StartTime != "10:30" {
			t.Errorf("expected start_time 10:30, got %s", retrieved.StartTime)
		}
	})

	t.Run("DeleteSpeedLimitSchedule", func(t *testing.T) {
		// Create a schedule
		schedule := &SpeedLimitSchedule{
			SiteID:     site.ID,
			DayOfWeek:  4, // Thursday
			StartTime:  "12:00",
			EndTime:    "13:00",
			SpeedLimit: 25,
		}
		if err := db.CreateSpeedLimitSchedule(schedule); err != nil {
			t.Fatalf("failed to create schedule: %v", err)
		}

		// Delete it
		if err := db.DeleteSpeedLimitSchedule(schedule.ID); err != nil {
			t.Fatalf("failed to delete schedule: %v", err)
		}

		// Verify it's gone
		_, err := db.GetSpeedLimitSchedule(schedule.ID)
		if err == nil {
			t.Error("expected error when getting deleted schedule")
		}
	})

	t.Run("DeleteAllSpeedLimitSchedulesForSite", func(t *testing.T) {
		// Create a new site
		newSite := &Site{
			Name:             "Test Site 2",
			Location:         "Test Location 2",
			CosineErrorAngle: 0.5,
			SpeedLimit:       25,
			Surveyor:         "Test Surveyor",
			Contact:          "test@example.com",
		}
		if err := db.CreateSite(newSite); err != nil {
			t.Fatalf("failed to create test site: %v", err)
		}

		// Create schedules for the new site
		for i := 0; i < 5; i++ {
			schedule := &SpeedLimitSchedule{
				SiteID:     newSite.ID,
				DayOfWeek:  i,
				StartTime:  "08:00",
				EndTime:    "09:00",
				SpeedLimit: 25,
			}
			if err := db.CreateSpeedLimitSchedule(schedule); err != nil {
				t.Fatalf("failed to create schedule: %v", err)
			}
		}

		// Delete all schedules for the site
		if err := db.DeleteAllSpeedLimitSchedulesForSite(newSite.ID); err != nil {
			t.Fatalf("failed to delete schedules: %v", err)
		}

		// Verify they're all gone
		schedules, err := db.GetSpeedLimitSchedulesForSite(newSite.ID)
		if err != nil {
			t.Fatalf("failed to get schedules: %v", err)
		}

		if len(schedules) != 0 {
			t.Errorf("expected 0 schedules after deletion, got %d", len(schedules))
		}
	})

	t.Run("GetNonExistentSchedule", func(t *testing.T) {
		_, err := db.GetSpeedLimitSchedule(99999)
		if err == nil {
			t.Error("expected error when getting non-existent schedule")
		}
	})
}
