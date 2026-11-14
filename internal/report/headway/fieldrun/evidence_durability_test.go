package fieldrun

import (
	"errors"
	"os"
	"strings"
	"testing"
)

type failedEvidenceFile struct {
	evidenceFile
	operation string
}

func (f failedEvidenceFile) Write(b []byte) (int, error) {
	if f.operation == "write" {
		return 0, errors.New("ENOSPC: write failed")
	}
	return f.evidenceFile.Write(b)
}
func (f failedEvidenceFile) Sync() error {
	if f.operation == "sync" {
		return errors.New("EIO: sync failed")
	}
	return f.evidenceFile.Sync()
}
func (f failedEvidenceFile) Close() error {
	err := f.evidenceFile.Close()
	if f.operation == "close" {
		return errors.New("close failed")
	}
	return err
}

func TestEvidencePublicationFailuresLeaveNoFinalFile(t *testing.T) {
	d, err := Evidence(evidenceFixture(t), evidenceContext())
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range []string{"mkdir", "temp", "write", "sync", "close", "link"} {
		t.Run(operation, func(t *testing.T) {
			dir := t.TempDir()
			ops := localEvidenceFiles()
			fail := errors.New(operation + " failed")
			switch operation {
			case "mkdir":
				ops.mkdir = func(string, os.FileMode) error { return fail }
			case "temp":
				ops.temp = func(string, string) (evidenceFile, error) { return nil, fail }
			case "link":
				ops.link = func(string, string) error { return fail }
			default:
				ops.temp = func(dir, pattern string) (evidenceFile, error) {
					f, e := os.CreateTemp(dir, pattern)
					return failedEvidenceFile{evidenceFile: f, operation: operation}, e
				}
			}
			if _, err := writeEvidence(dir, d, ops); err == nil || !strings.Contains(err.Error(), operation) {
				t.Fatalf("failure lost: %v", err)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("partial or final evidence left after failed %s: %v", operation, entries)
			}
		})
	}
}
