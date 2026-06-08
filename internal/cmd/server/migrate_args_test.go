package server

import "testing"

func TestParseMigrateCommandArgsHandlesDBPathAfterAction(t *testing.T) {
	args, dbPath, explicitDBPath, err := parseMigrateCommandArgs([]string{"up", "--db-path", "/tmp/custom.db"}, "sensor_data.db")
	if err != nil {
		t.Fatalf("parseMigrateCommandArgs returned error: %v", err)
	}
	if len(args) != 1 || args[0] != "up" {
		t.Fatalf("unexpected migrate args: %#v", args)
	}
	if dbPath != "/tmp/custom.db" {
		t.Fatalf("unexpected db path: %q", dbPath)
	}
	if !explicitDBPath {
		t.Fatal("expected explicit db-path to be recorded")
	}
}

func TestParseMigrateCommandArgsHandlesDBPathBeforeAction(t *testing.T) {
	args, dbPath, explicitDBPath, err := parseMigrateCommandArgs([]string{"--db-path=/tmp/custom.db", "status"}, "sensor_data.db")
	if err != nil {
		t.Fatalf("parseMigrateCommandArgs returned error: %v", err)
	}
	if len(args) != 1 || args[0] != "status" {
		t.Fatalf("unexpected migrate args: %#v", args)
	}
	if dbPath != "/tmp/custom.db" {
		t.Fatalf("unexpected db path: %q", dbPath)
	}
	if !explicitDBPath {
		t.Fatal("expected explicit db-path to be recorded")
	}
}

func TestParseMigrateCommandArgsRejectsUnknownFlags(t *testing.T) {
	_, _, _, err := parseMigrateCommandArgs([]string{"up", "--bogus"}, "sensor_data.db")
	if err == nil {
		t.Fatal("expected error for unknown migrate flag")
	}
}

func TestResolveDataCommandDBPathUsesApplianceDBForImplicitDefault(t *testing.T) {
	got := resolveDataCommandDBPathWith(defaultRuntimeDBPath, true)
	if got != deployedRuntimeDBPath {
		t.Fatalf("resolveDataCommandDBPathWith(default, appliance=true) = %q, want %q", got, deployedRuntimeDBPath)
	}
}

func TestResolveDataCommandDBPathPreservesLocalDefaultOffAppliance(t *testing.T) {
	got := resolveDataCommandDBPathWith(defaultRuntimeDBPath, false)
	if got != defaultRuntimeDBPath {
		t.Fatalf("resolveDataCommandDBPathWith(default, appliance=false) = %q, want %q", got, defaultRuntimeDBPath)
	}
}

func TestResolveDataCommandDBPathPreservesExplicitPath(t *testing.T) {
	got := resolveDataCommandDBPathWith("/tmp/custom.db", true)
	if got != "/tmp/custom.db" {
		t.Fatalf("resolveDataCommandDBPathWith(explicit, appliance=true) = %q, want explicit path", got)
	}
}
