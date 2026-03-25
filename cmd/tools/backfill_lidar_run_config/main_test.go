package main

import (
	"bytes"
	"database/sql"
	"errors"
	"log"
	"os"
	"testing"

	_ "modernc.org/sqlite"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
	sqlitepkg "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

func TestRun_Success(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer sqlDB.Close()

	var logs bytes.Buffer
	var gotPath string
	var gotDryRun bool
	err = run(
		[]string{"-db", "custom.db", "-dry-run"},
		func(path string) (os.FileInfo, error) {
			gotPath = path
			return nil, nil
		},
		func(path string) (*dbpkg.DB, error) {
			return &dbpkg.DB{DB: sqlDB}, nil
		},
		func(_ sqlitepkg.DBClient, dryRun bool) (*sqlitepkg.ImmutableRunConfigBackfillResult, error) {
			gotDryRun = dryRun
			return &sqlitepkg.ImmutableRunConfigBackfillResult{RunsSeen: 1, RunsUpdated: 2, ReplayCasesSeen: 3, ReplayCasesUpdated: 4}, nil
		},
		log.New(&logs, "", 0),
	)
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}
	if gotPath != "custom.db" {
		t.Fatalf("stat path = %q, want custom.db", gotPath)
	}
	if !gotDryRun {
		t.Fatal("expected dry-run flag to be passed through")
	}
	if !bytes.Contains(logs.Bytes(), []byte("runs seen=1 updated=2")) {
		t.Fatalf("unexpected log output: %s", logs.String())
	}
}

func TestRun_StatError(t *testing.T) {
	err := run(nil, func(string) (os.FileInfo, error) {
		return nil, errors.New("missing")
	}, nil, nil, log.New(&bytes.Buffer{}, "", 0))
	if err == nil || err.Error() != "DB path sensor_data.db not accessible: missing" {
		t.Fatalf("expected stat error, got %v", err)
	}
}

func TestRun_OpenError(t *testing.T) {
	err := run(nil, func(string) (os.FileInfo, error) {
		return nil, nil
	}, func(string) (*dbpkg.DB, error) {
		return nil, errors.New("open failed")
	}, nil, log.New(&bytes.Buffer{}, "", 0))
	if err == nil || err.Error() != "open DB: open failed" {
		t.Fatalf("expected open error, got %v", err)
	}
}

func TestRun_BackfillError(t *testing.T) {
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	defer sqlDB.Close()

	err = run(nil, func(string) (os.FileInfo, error) {
		return nil, nil
	}, func(string) (*dbpkg.DB, error) {
		return &dbpkg.DB{DB: sqlDB}, nil
	}, func(sqlitepkg.DBClient, bool) (*sqlitepkg.ImmutableRunConfigBackfillResult, error) {
		return nil, errors.New("backfill failed")
	}, log.New(&bytes.Buffer{}, "", 0))
	if err == nil || err.Error() != "immutable run-config backfill failed: backfill failed" {
		t.Fatalf("expected backfill error, got %v", err)
	}
}
