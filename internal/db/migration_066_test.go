package db

import "testing"

func TestMigration066ClipExtractsRoundTrip(t *testing.T) {
	db := setupEmptyTestDB(t)
	defer db.Close()
	migrations, err := getMigrationsFS()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateTo(migrations, 65); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) error {
		_, err := db.Exec(query, args...)
		return err
	}
	must := func(query string, args ...any) {
		t.Helper()
		if err := exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	must(`INSERT INTO lidar_clips(clip_id,sensor_id,created_at_ns) VALUES('kept','hesai',1)`)
	if err := db.MigrateTo(migrations, 66); err != nil {
		t.Fatal(err)
	}

	if got := queryStrings(t, db, `SELECT storage FROM lidar_clips WHERE clip_id='kept'`); len(got) != 1 || got[0] != "sources" {
		t.Fatalf("an existing clip defaults to %v, want sources", got)
	}
	must(`UPDATE lidar_clips SET dataset_id='kirkham-9th-k1', north_azimuth_deg=35 WHERE clip_id='kept'`)
	must(`INSERT INTO lidar_clips(clip_id,sensor_id,created_at_ns) VALUES('other','hesai',2)`)
	for name, query := range map[string]string{
		"duplicate dataset id": `UPDATE lidar_clips SET dataset_id='kirkham-9th-k1' WHERE clip_id='other'`,
		"azimuth out of range": `UPDATE lidar_clips SET north_azimuth_deg=360 WHERE clip_id='other'`,
		"unknown storage":      `UPDATE lidar_clips SET storage='copy' WHERE clip_id='other'`,
	} {
		if exec(query) == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	if exec(`INSERT INTO lidar_capture_jobs(job_id,kind,session_id,queued_at_ns) VALUES('s','capture_export','ses',1)`) == nil {
		t.Error("an export job naming a session was accepted")
	}
	must(`INSERT INTO lidar_capture_jobs(job_id,kind,queued_at_ns) VALUES('job','capture_export',1)`)
	must(`INSERT INTO lidar_clip_extracts(job_id,clip_id,rubric_version,created_at_ns) VALUES('job','kept',1,1)`)
	digest := "sha256:" + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	for name, query := range map[string]string{
		"path without digest": `UPDATE lidar_clip_extracts SET rel_path='clips/a.pcapng' WHERE job_id='job'`,
		"escaping path":       `UPDATE lidar_clip_extracts SET rel_path='../a.pcapng', sha256='` + digest + `' WHERE job_id='job'`,
		"absolute path":       `UPDATE lidar_clip_extracts SET rel_path='/a.pcapng', sha256='` + digest + `' WHERE job_id='job'`,
		"bad digest":          `UPDATE lidar_clip_extracts SET rel_path='clips/a.pcapng', sha256='sha256:XYZ' WHERE job_id='job'`,
		"bad sidecar":         `UPDATE lidar_clip_extracts SET sidecar_json='[1]' WHERE job_id='job'`,
		"bad closed_by":       `UPDATE lidar_clip_extracts SET closed_by='grid' WHERE job_id='job'`,
	} {
		if exec(query) == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	must(`UPDATE lidar_clip_extracts SET rel_path='clips/a.pcapng', sha256=?, closed_by='window', sidecar_json='{}' WHERE job_id='job'`, digest)
	must(`DELETE FROM lidar_capture_jobs WHERE job_id='job'`)
	if got := queryStrings(t, db, `SELECT job_id FROM lidar_clip_extracts`); len(got) != 0 {
		t.Fatalf("an extract outlived its job: %v", got)
	}

	must(`INSERT INTO lidar_capture_jobs(job_id,kind,queued_at_ns) VALUES('queued','capture_export',2)`)
	if err := db.MigrateTo(migrations, 65); err != nil {
		t.Fatal(err)
	}
	if got := queryStrings(t, db, `SELECT job_id FROM lidar_capture_jobs WHERE kind='capture_export'`); len(got) != 0 {
		t.Fatalf("rollback kept export jobs the old triggers refuse: %v", got)
	}
	if exec(`INSERT INTO lidar_capture_jobs(job_id,kind,queued_at_ns) VALUES('late','capture_export',3)`) == nil {
		t.Error("the rolled-back triggers admit an export job")
	}
	if got := queryStrings(t, db, `SELECT clip_id FROM lidar_clips ORDER BY clip_id`); len(got) != 2 {
		t.Fatalf("rollback lost clips: %v", got)
	}
	if err := db.MigrateTo(migrations, 66); err != nil {
		t.Fatal(err)
	}
}
