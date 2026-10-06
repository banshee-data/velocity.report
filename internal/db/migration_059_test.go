package db

import (
	"slices"
	"strings"
	"testing"
)

// periodsDatabaseAt058 holds one site with two configuration periods as they
// stood before 000059, which records no speed limit.
func periodsDatabaseAt058(t *testing.T) (*DB, func(uint)) {
	t.Helper()
	db, migrateTo := databaseAt055(t)
	migrateTo(58)
	for _, stmt := range []string{
		`INSERT INTO site(id,name,location,surveyor,contact) VALUES(42,'Speed Street','Main and First','Surveyor','contact@example.com')`,
		`INSERT INTO site_config_periods(id,site_id,effective_start_unix,effective_end_unix,is_active,notes,cosine_error_angle) VALUES(420,42,1000,2000,0,'before the new sign',4.5)`,
		`INSERT INTO site_config_periods(id,site_id,effective_start_unix,effective_end_unix,is_active,notes,cosine_error_angle) VALUES(421,42,2000,NULL,1,NULL,5)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	return db, migrateTo
}

func configPeriodRows(t *testing.T, db *DB) []string {
	t.Helper()
	return queryStrings(t, db, `SELECT id || '|' || site_id || '|' || effective_start_unix || '|' ||
		COALESCE(effective_end_unix, '') || '|' || is_active || '|' || COALESCE(notes, '') || '|' || cosine_error_angle
		FROM site_config_periods WHERE site_id = 42 ORDER BY id`)
}

func configPeriodColumns(t *testing.T, db *DB) string {
	t.Helper()
	return strings.Join(queryStrings(t, db, `SELECT name FROM pragma_table_info('site_config_periods')
		WHERE name IN ('speed_limit_kph', 'speed_limit_unit', 'jurisdiction') ORDER BY cid`), ",")
}

func TestMigration059KeepsPeriodsAndRecordsNoLimitForThem(t *testing.T) {
	db, migrateTo := periodsDatabaseAt058(t)
	before := configPeriodRows(t, db)

	migrateTo(59)
	if got := configPeriodColumns(t, db); got != "speed_limit_kph,speed_limit_unit,jurisdiction" {
		t.Fatalf("columns at 59: %q", got)
	}
	if got := configPeriodRows(t, db); !slices.Equal(got, before) {
		t.Fatalf("periods after 000059:\n got %v\nwant %v", got, before)
	}
	// Nothing is inferred for a period that recorded no limit.
	if got := queryStrings(t, db, `SELECT id FROM site_config_periods
		WHERE site_id = 42 AND (speed_limit_kph IS NOT NULL OR speed_limit_unit IS NOT NULL OR jurisdiction IS NOT NULL)`); len(got) != 0 {
		t.Fatalf("periods given a limit by the migration: %v", got)
	}
}

func TestMigration059AcceptsALimitWithItsUnit(t *testing.T) {
	db, migrateTo := periodsDatabaseAt058(t)
	migrateTo(59)
	accepts(t, db, "a 25 mph limit with its jurisdiction",
		`UPDATE site_config_periods SET speed_limit_kph = 40.2336, speed_limit_unit = 'mph', jurisdiction = 'US-CA' WHERE id = 421`)
	accepts(t, db, "a 30 km/h limit with no jurisdiction",
		`UPDATE site_config_periods SET speed_limit_kph = 30, speed_limit_unit = 'kph' WHERE id = 420`)
	accepts(t, db, "the limit removed again",
		`UPDATE site_config_periods SET speed_limit_kph = NULL, speed_limit_unit = NULL WHERE id = 420`)
	accepts(t, db, "a jurisdiction with no limit",
		`UPDATE site_config_periods SET jurisdiction = 'GB' WHERE id = 420`)
}

func TestMigration059RefusesALimitWithoutItsUnit(t *testing.T) {
	db, migrateTo := periodsDatabaseAt058(t)
	migrateTo(59)
	for _, c := range []struct{ what, set string }{
		{"a limit with no unit", `speed_limit_kph = 30`},
		{"a unit with no limit", `speed_limit_unit = 'kph'`},
		{"a unit the schema does not know", `speed_limit_kph = 30, speed_limit_unit = 'km/h'`},
		{"a zero limit", `speed_limit_kph = 0, speed_limit_unit = 'kph'`},
		{"a negative limit", `speed_limit_kph = -30, speed_limit_unit = 'kph'`},
		{"a limit above 200 km/h", `speed_limit_kph = 250, speed_limit_unit = 'kph'`},
		{"a blank jurisdiction", `jurisdiction = '   '`},
		{"a jurisdiction over 100 characters", `jurisdiction = '` + strings.Repeat("x", 101) + `'`},
	} {
		refuses(t, db, c.what, "CHECK constraint failed",
			`UPDATE site_config_periods SET `+c.set+` WHERE id = 421`)
	}
}

func TestMigration059RollsBackToThePeriodsItFound(t *testing.T) {
	db, migrateTo := periodsDatabaseAt058(t)
	before := configPeriodRows(t, db)
	migrateTo(59)
	accepts(t, db, "a limit set at 59",
		`UPDATE site_config_periods SET speed_limit_kph = 40.2336, speed_limit_unit = 'mph', jurisdiction = 'US-CA' WHERE id = 421`)

	migrateTo(58)
	if got := configPeriodColumns(t, db); got != "" {
		t.Fatalf("the rollback left %s", got)
	}
	if got := configPeriodRows(t, db); !slices.Equal(got, before) {
		t.Fatalf("periods after the rollback:\n got %v\nwant %v", got, before)
	}

	migrateTo(59)
	if got := configPeriodColumns(t, db); got != "speed_limit_kph,speed_limit_unit,jurisdiction" {
		t.Fatalf("columns at 59 a second time: %q", got)
	}
}
