package segments

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func validRecord() Record {
	params := DefaultParams()
	window := Window{Finder: "following", Version: Version, Source: "run", Role: "tuning", StartNs: testBase, EndNs: testBase + 10_000_000_000}
	window.ID = Identity(window.Finder, window.Source, window.Role, params, window.StartNs)
	return Record{Schema: "velocity.report/annotation-segment", SchemaVersion: 1, PackDigest: "sha256:digest", Role: "tuning", Finder: "following", FinderVersion: Version, Parameters: params, Segment: window}
}

type failingRecordFile struct {
	writeErr, syncErr, closeErr error
}

func (f failingRecordFile) Write(b []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(b), nil
}
func (f failingRecordFile) Sync() error  { return f.syncErr }
func (f failingRecordFile) Close() error { return f.closeErr }

func TestRecordWriteFailuresRemoveIncompleteProvenance(t *testing.T) {
	for _, which := range []string{"write", "sync", "close"} {
		t.Run(which, func(t *testing.T) {
			dir := t.TempDir()
			failure := errors.New(which + " failed")
			stub := failingRecordFile{}
			switch which {
			case "write":
				stub.writeErr = failure
			case "sync":
				stub.syncErr = failure
			case "close":
				stub.closeErr = failure
			}
			err := writeRecordWithOpen(dir, validRecord(), func(path string) (recordOutput, error) {
				if err := os.WriteFile(path, []byte("partial"), 0644); err != nil {
					t.Fatal(err)
				}
				return stub, nil
			})
			if !errors.Is(err, failure) {
				t.Fatalf("failure not reported: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "segment.json")); !os.IsNotExist(err) {
				t.Fatalf("incomplete record remained: %v", err)
			}
		})
	}
	r := validRecord()
	r.Schema = "wrong"
	if err := WriteRecord(t.TempDir(), r); err == nil {
		t.Fatal("invalid record was written")
	}
}

func TestRecordRefusesChangedSelectionProvenance(t *testing.T) {
	base := validRecord()
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Record){
		"schema":  func(r *Record) { r.SchemaVersion++ },
		"digest":  func(r *Record) { r.PackDigest = "" },
		"role":    func(r *Record) { r.Role = "mystery" },
		"finder":  func(r *Record) { r.Finder = "lateral_jump"; r.Role = "held_out" },
		"version": func(r *Record) { r.FinderVersion++ },
		"bounds":  func(r *Record) { r.Segment.EndNs = r.Segment.StartNs },
		"window":  func(r *Record) { r.Segment.Finder = "exposure" },
		"params":  func(r *Record) { r.Parameters.MaxGap = -1 },
		"id":      func(r *Record) { r.Segment.ID = "another" },
		"length":  func(r *Record) { r.Segment.EndNs++ },
	} {
		t.Run(name, func(t *testing.T) {
			r := base
			mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("changed provenance was accepted")
			}
		})
	}
}

func TestWriteRecordIsDigestBoundAndExclusive(t *testing.T) {
	dir := t.TempDir()
	r := validRecord()
	if err := WriteRecord(dir, r); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "segment.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved Record
	if err := json.Unmarshal(b, &saved); err != nil || saved.Segment.ID != r.Segment.ID {
		t.Fatalf("saved provenance: %+v, %v", saved, err)
	}
	if err := WriteRecord(dir, r); err == nil {
		t.Fatal("overwrote immutable segment record")
	}
	r.Segment.Score = math.NaN()
	if err := WriteRecord(t.TempDir(), r); err == nil {
		t.Fatal("wrote a record with a non-finite score")
	}
}
