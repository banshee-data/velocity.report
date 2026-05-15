package main

import "testing"

func TestParseMigrateCommandArgsHandlesDBPathAfterAction(t *testing.T) {
	args, dbPath, err := parseMigrateCommandArgs([]string{"up", "--db-path", "/tmp/custom.db"}, "sensor_data.db")
	if err != nil {
		t.Fatalf("parseMigrateCommandArgs returned error: %v", err)
	}
	if len(args) != 1 || args[0] != "up" {
		t.Fatalf("unexpected migrate args: %#v", args)
	}
	if dbPath != "/tmp/custom.db" {
		t.Fatalf("unexpected db path: %q", dbPath)
	}
}

func TestParseMigrateCommandArgsHandlesDBPathBeforeAction(t *testing.T) {
	args, dbPath, err := parseMigrateCommandArgs([]string{"--db-path=/tmp/custom.db", "status"}, "sensor_data.db")
	if err != nil {
		t.Fatalf("parseMigrateCommandArgs returned error: %v", err)
	}
	if len(args) != 1 || args[0] != "status" {
		t.Fatalf("unexpected migrate args: %#v", args)
	}
	if dbPath != "/tmp/custom.db" {
		t.Fatalf("unexpected db path: %q", dbPath)
	}
}

func TestParseMigrateCommandArgsRejectsUnknownFlags(t *testing.T) {
	_, _, err := parseMigrateCommandArgs([]string{"up", "--bogus"}, "sensor_data.db")
	if err == nil {
		t.Fatal("expected error for unknown migrate flag")
	}
}
