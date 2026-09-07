package annotation

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"golang.org/x/sys/unix"
)

func saveReference(t *testing.T, p *Pack) *Sidecar {
	t.Helper()
	s := NewSidecar(p)
	s.Change = Provenance{Author: "test-reviewer", Session: "session-a", Operation: "label"}
	s.Objects = []Object{reviewedObject("car-a", "car")}
	s.Masks = []FrameMask{mask("car-a", 0, 0, 1)}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	return s
}

func readSaved(t *testing.T, p *Pack) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.Dir, sidecarFile))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestRevisionConflictHistoryAndRestore(t *testing.T) {
	p := synthPack(t)
	s := saveReference(t, p)
	firstBytes := readSaved(t, p)
	stale, err := LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	s.Masks[0].PointIndices = []int{2, 3}
	s.Masks[0].Status = StatusProposed
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	if s.Revision != 2 || s.Change.ParentRev != 1 || s.Change.Author != "test-reviewer" {
		t.Fatalf("wrong commit metadata: %+v", s)
	}
	if !strings.HasSuffix(s.UpdatedUTC, "Z") {
		t.Fatal("non-UTC timestamp")
	}
	secondBytes := readSaved(t, p)
	stale.Masks[0].PointIndices = []int{3}
	if err := SaveSidecar(p, stale); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("stale save: %v", err)
	}
	if stale.Revision != 1 || !bytes.Equal(secondBytes, readSaved(t, p)) {
		t.Fatal("failed save mutated state")
	}
	archived, err := os.ReadFile(filepath.Join(p.Dir, revisionName(1)))
	if err != nil || !bytes.Equal(firstBytes, archived) {
		t.Fatalf("history did not preserve exact bytes: %v", err)
	}
	old, err := LoadSidecarRevision(p, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveSidecar(p, old); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("historical snapshot overwrote head: %v", err)
	}
	if err := RestoreSidecarRevision(p, stale, 1); !errors.Is(err, ErrSidecarConflict) {
		t.Fatalf("stale restore: %v", err)
	}
	if err := RestoreSidecarRevision(p, s, 1); err != nil {
		t.Fatal(err)
	}
	if s.Revision != 3 || s.RestoredFrom != 1 || s.Change.ParentRev != 2 || s.Change.Operation != "restore" {
		t.Fatalf("restore was not a new revision: %+v", s)
	}
	if !reflect.DeepEqual(s.Masks[0].PointIndices, []int{0, 1}) {
		t.Fatal("wrong restored membership")
	}
	// Redo across reload retains proposed status rather than promoting it to truth.
	s, err = LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := RestoreSidecarRevision(p, s, 2); err != nil {
		t.Fatal(err)
	}
	if s.Revision != 4 || len(s.ReviewedMasks()) != 0 {
		t.Fatal("redo laundered a proposal")
	}
	latest, err := LoadSidecarRevision(p, 4)
	if err != nil || latest.Revision != 4 {
		t.Fatalf("read current revision: %v", err)
	}
	latest.Change.Operation = "label"
	if err := SaveSidecar(p, latest); err != nil {
		t.Fatal(err)
	}
	if latest.RestoredFrom != 0 {
		t.Fatal("ordinary edit retained a restore marker")
	}
}

func TestLegacySidecarMigratesWithoutLosingBytes(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Revision = 8
	s.Objects = []Object{reviewedObject("legacy", "car")}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.Dir, sidecarFile), b, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = LoadSidecar(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	archived, err := os.ReadFile(filepath.Join(p.Dir, revisionName(8)))
	if err != nil || !bytes.Equal(b, archived) || s.Revision != 9 {
		t.Fatalf("legacy migration: %v", err)
	}
}

func TestConcurrentSessionsCannotLoseAnEdit(t *testing.T) {
	p := synthPack(t)
	sessions := []*Sidecar{NewSidecar(p), NewSidecar(p)}
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make([]error, 2)
	for i := range sessions {
		wg.Add(1)
		go func(i int) { defer wg.Done(); <-start; errs[i] = SaveSidecar(p, sessions[i]) }(i)
	}
	close(start)
	wg.Wait()
	wins := 0
	for i, err := range errs {
		if err == nil {
			wins++
			continue
		}
		if !errors.Is(err, ErrSidecarConflict) && !errors.Is(err, ErrSidecarBusy) {
			t.Fatal(err)
		}
		if err := SaveSidecar(p, sessions[i]); !errors.Is(err, ErrSidecarConflict) {
			t.Fatalf("loser retry: %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("successful writers = %d", wins)
	}
}

func TestSidecarCrossProcessLock(t *testing.T) {
	const envKey = "VELOCITY_ANNOTATION_LOCK_TEST_PACK"
	if dir := os.Getenv(envKey); dir != "" {
		p, err := OpenPack(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := SaveSidecar(p, NewSidecar(p)); !errors.Is(err, ErrSidecarBusy) {
			t.Fatalf("cross-process lock: %v", err)
		}
		return
	}
	p := synthPack(t)
	f, err := os.OpenFile(filepath.Join(p.Dir, annotationLock), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	if err := SaveSidecar(p, NewSidecar(p)); !errors.Is(err, ErrSidecarBusy) {
		t.Fatalf("same-process competing writer: %v", err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSidecarCrossProcessLock$")
	cmd.Env = append(os.Environ(), envKey+"="+p.Dir)
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, b)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	// No lock-file deletion is necessary after a process closes/exits.
	if err := SaveSidecar(p, NewSidecar(p)); err != nil {
		t.Fatal(err)
	}
}

func TestRevisionSafetyRejectsTamperingAndMissingHead(t *testing.T) {
	t.Run("external edit with unchanged revision", func(t *testing.T) {
		p := synthPack(t)
		s := saveReference(t, p)
		b := append(readSaved(t, p), '\n')
		if err := os.WriteFile(filepath.Join(p.Dir, sidecarFile), b, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SaveSidecar(p, s); !errors.Is(err, ErrSidecarConflict) {
			t.Fatalf("external edit: %v", err)
		}
	})
	t.Run("missing head", func(t *testing.T) {
		p := synthPack(t)
		s := saveReference(t, p)
		if err := SaveSidecar(p, s); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(filepath.Join(p.Dir, sidecarFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSidecar(p); err == nil {
			t.Fatal("missing head became an empty dataset")
		}
		if err := SaveSidecar(p, s); !errors.Is(err, ErrSidecarConflict) {
			t.Fatalf("deleted current: %v", err)
		}
		if err := SaveSidecar(p, NewSidecar(p)); err == nil {
			t.Fatal("fresh session bypassed history")
		}
		if _, err := LoadSidecarRevision(p, 1); err != nil {
			t.Fatalf("history recovery failed: %v", err)
		}
	})
	t.Run("damaged current", func(t *testing.T) {
		p := synthPack(t)
		s := saveReference(t, p)
		if err := os.WriteFile(filepath.Join(p.Dir, sidecarFile), []byte("bad"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := SaveSidecar(p, s); err == nil {
			t.Fatal("overwrote corrupt current")
		}
	})
}

func TestRevisionArchiveFailurePreservesCurrent(t *testing.T) {
	for _, kind := range []string{"blocked directory", "mismatching archive", "unreadable archive", "matching retry"} {
		t.Run(kind, func(t *testing.T) {
			p := synthPack(t)
			s := saveReference(t, p)
			before := readSaved(t, p)
			dir := filepath.Join(p.Dir, revisionDir)
			if kind == "blocked directory" {
				if err := os.WriteFile(dir, []byte("block"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.Mkdir(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(p.Dir, revisionName(1))
				if kind == "unreadable archive" {
					if err := os.Mkdir(path, 0o700); err != nil {
						t.Fatal(err)
					}
				} else {
					data := before
					if kind == "mismatching archive" {
						data = []byte("wrong")
					}
					if err := os.WriteFile(path, data, 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			err := SaveSidecar(p, s)
			if kind == "matching retry" {
				if err != nil || s.Revision != 2 {
					t.Fatalf("retry: %v", err)
				}
				return
			}
			if err == nil || !bytes.Equal(before, readSaved(t, p)) || s.Revision != 1 {
				t.Fatalf("unsafe failed archive: %v", err)
			}
		})
	}
}

func TestInvalidEditDoesNotMutateSession(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("z", "car"), reviewedObject("a", "bus")}
	s.Masks = []FrameMask{mask("z", 0, 3, 1, 1)}
	before, _ := json.Marshal(s)
	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("duplicate indices accepted")
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("invalid edit changed the caller")
	}
	s.Masks = []FrameMask{mask("z", 0, 1), mask("z", 0, 2)}
	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("duplicate object/sample masks accepted")
	}
	s.Masks = nil
	s.Objects[0].Confidence = math.NaN()
	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("non-finite edit accepted")
	}
	for _, revision := range []int{0, -1, math.MaxInt} {
		s := NewSidecar(p)
		s.Revision = revision
		if err := s.Validate(p); err == nil {
			t.Fatalf("bad revision %d", revision)
		}
	}
}

func TestRevisionReadFailures(t *testing.T) {
	p := synthPack(t)
	for _, revision := range []int{0, -1, 99} {
		if _, err := LoadSidecarRevision(p, revision); err == nil {
			t.Fatal("missing/invalid revision loaded")
		}
		if err := RestoreSidecarRevision(p, NewSidecar(p), revision); err == nil {
			t.Fatal("invalid restore succeeded")
		}
	}
	s := saveReference(t, p)
	if err := SaveSidecar(p, s); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p.Dir, revisionName(1))
	for _, data := range [][]byte{[]byte("bad"), []byte(`{"schema_version":99}`), readSaved(t, p)} {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSidecarRevision(p, 1); err == nil {
			t.Fatal("invalid archive loaded")
		}
	}
	p.Dir = filepath.Join(p.Dir, "missing")
	if _, err := LoadSidecarRevision(p, 1); err == nil {
		t.Fatal("absent pack opened")
	}
}

func TestSidecarReadSizeLimit(t *testing.T) {
	p := synthPack(t)
	f, err := os.Create(filepath.Join(p.Dir, sidecarFile))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxSidecarBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := LoadSidecar(p); err == nil {
		t.Fatal("oversized sidecar loaded")
	}
}

func TestSidecarWriteSizeLimit(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	s.Objects = []Object{reviewedObject("car", "car")}
	s.Objects[0].Notes = strings.Repeat("x", MaxSidecarBytes)
	if err := SaveSidecar(p, s); err == nil {
		t.Fatal("oversized edit saved")
	}
	if _, err := os.Stat(filepath.Join(p.Dir, sidecarFile)); !os.IsNotExist(err) {
		t.Fatal("failed edit reached disk")
	}
}

func TestRevisionFilesystemFailures(t *testing.T) {
	t.Run("lock path blocked", func(t *testing.T) {
		p := synthPack(t)
		if err := os.Mkdir(filepath.Join(p.Dir, annotationLock), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := SaveSidecar(p, NewSidecar(p)); err == nil {
			t.Fatal("directory used as writer lock")
		}
	})
	t.Run("archive cannot be written", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory write permissions")
		}
		p := synthPack(t)
		s := saveReference(t, p)
		dir := filepath.Join(p.Dir, revisionDir)
		if err := os.Mkdir(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(dir, 0o700)
		before := readSaved(t, p)
		if err := SaveSidecar(p, s); err == nil {
			t.Fatal("unwritable archive accepted")
		}
		if !bytes.Equal(before, readSaved(t, p)) {
			t.Fatal("archive failure changed head")
		}
	})
	t.Run("head cannot be written", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root bypasses directory write permissions")
		}
		p := synthPack(t)
		s := saveReference(t, p)
		if err := os.Mkdir(filepath.Join(p.Dir, revisionDir), 0o700); err != nil {
			t.Fatal(err)
		}
		before := readSaved(t, p)
		if err := os.Chmod(p.Dir, 0o500); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(p.Dir, 0o700)
		if err := SaveSidecar(p, s); err == nil {
			t.Fatal("unwritable head accepted")
		}
		if !bytes.Equal(before, readSaved(t, p)) {
			t.Fatal("failed commit changed head")
		}
	})
	t.Run("symlink escape", func(t *testing.T) {
		p := synthPack(t)
		outside := filepath.Join(t.TempDir(), "outside.json")
		if err := os.WriteFile(outside, []byte("do not touch"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(p.Dir, sidecarFile)); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSidecar(p); err == nil {
			t.Fatal("read outside the pack")
		}
		if err := SaveSidecar(p, NewSidecar(p)); err == nil {
			t.Fatal("wrote through escaping link")
		}
		b, err := os.ReadFile(outside)
		if err != nil || string(b) != "do not touch" {
			t.Fatal("outside file changed")
		}
	})
}

type failingAnnotationFile struct {
	writeErr, syncErr, closeErr error
	short                       bool
	closed                      bool
}

func (f *failingAnnotationFile) Write(b []byte) (int, error) {
	if f.short {
		return 0, f.writeErr
	}
	return len(b), f.writeErr
}
func (f *failingAnnotationFile) Sync() error  { return f.syncErr }
func (f *failingAnnotationFile) Close() error { f.closed = true; return f.closeErr }

func TestAnnotationFlushFailuresAlwaysClose(t *testing.T) {
	for _, f := range []*failingAnnotationFile{
		{writeErr: io.ErrClosedPipe}, {syncErr: io.ErrClosedPipe},
		{closeErr: io.ErrClosedPipe}, {short: true},
	} {
		if err := flushAnnotationFile(f, []byte("payload")); err == nil || !f.closed {
			t.Fatalf("flush failure lost or descriptor leaked: %v", err)
		}
	}
}

func TestAtomicAnnotationWriterErrors(t *testing.T) {
	p := synthPack(t)
	root, err := os.OpenRoot(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := os.Mkdir(filepath.Join(p.Dir, sidecarFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeAnnotationFile(root, sidecarFile, []byte("{}")); err == nil {
		t.Fatal("rename over directory succeeded")
	}
	if err := root.Mkdir("other", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeAnnotationFile(root, "other/file", []byte("{}")); err == nil {
		t.Fatal("missing durability directory accepted")
	}
	if err := root.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rootSync(root); err == nil {
		t.Fatal("closed root synced")
	}
	if err := writeAnnotationFile(root, sidecarFile, nil); err == nil {
		t.Fatal("closed root written")
	}
}

func TestAnnotationReadFailure(t *testing.T) {
	p := synthPack(t)
	path := filepath.Join(p.Dir, sidecarFile)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := readAnnotationBytes(f); err == nil {
		t.Fatal("write-only descriptor read")
	}
	f.Close()
	if _, err := readAnnotationBytes(f); err == nil {
		t.Fatal("closed descriptor read")
	}
	dir, err := os.Open(p.Dir)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	if _, err := readAnnotationBytes(dir); err == nil {
		t.Fatal("directory read as annotation")
	}
	if err := os.WriteFile(path, []byte("{}"), 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0o600)
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	if os.Geteuid() != 0 {
		if _, err := LoadSidecar(p); err == nil {
			t.Fatal("unreadable annotation loaded")
		}
	}
}

func TestAnnotationHistoryCannotAliasCurrent(t *testing.T) {
	for _, directoryLink := range []bool{false, true} {
		p := synthPack(t)
		s := saveReference(t, p)
		before := readSaved(t, p)
		dir := filepath.Join(p.Dir, revisionDir)
		if directoryLink {
			if err := os.Mkdir(filepath.Join(p.Dir, "elsewhere"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("elsewhere", dir); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("../"+sidecarFile, filepath.Join(p.Dir, revisionName(1))); err != nil {
				t.Fatal(err)
			}
		}
		if err := SaveSidecar(p, s); err == nil {
			t.Fatal("linked history accepted")
		}
		if !bytes.Equal(before, readSaved(t, p)) {
			t.Fatal("linked history changed current")
		}
	}
}
