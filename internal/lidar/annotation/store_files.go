package annotation

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"golang.org/x/sys/unix"
)

// The pack-bound stores (physical references, feature proposals) share these:
// one writer lock per pack, bounded reads of regular files inside the pack,
// and atomic, synced replacement.

// ErrSidecarConflict means the caller must reload and resolve its edits, not
// retry blindly: the base it edited, or the membership it pinned, changed.
var ErrSidecarConflict = errors.New("annotation revision changed; reload before saving")

// ErrSidecarBusy means another process owns the short-lived save transaction.
var ErrSidecarBusy = errors.New("annotation writer busy")

const (
	annotationLock = ".annotations.lock"
	// MaxSidecarBytes bounds any one annotation file read from a pack.
	MaxSidecarBytes = 64 << 20
)

// lockAnnotations takes the pack's non-blocking annotation writer lock. The
// caller closes the file to release it.
func lockAnnotations(root *os.Root) (*os.File, error) {
	lock, err := root.OpenFile(annotationLock, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if errors.Is(err, os.ErrExist) {
		lock, err = root.OpenFile(annotationLock, os.O_RDWR, 0o600)
	}
	if err != nil {
		return nil, fmt.Errorf("open annotation lock: %w", err)
	}
	if err := unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		lock.Close()
		return nil, fmt.Errorf("%w: %v", ErrSidecarBusy, err)
	}
	return lock, nil
}

// archiveRevision retains a head's exact bytes before it is replaced. An
// archive already present must be those bytes: a retry after a failed commit
// finds its own archive, and anything else is refused rather than
// overwritten.
func archiveRevision(root *os.Root, dir, name string, previous []byte, revision int) error {
	if err := root.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := root.Lstat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("history %s must be a real directory", dir)
	}
	archived, err := readAnnotationFile(root, name)
	switch {
	case err == nil && !bytes.Equal(archived, previous):
		return fmt.Errorf("revision %d archive differs; refusing overwrite", revision)
	case errors.Is(err, os.ErrNotExist):
		if err := writeAnnotationFile(root, name, previous); err != nil {
			return fmt.Errorf("archive revision %d: %w", revision, err)
		}
	case err != nil:
		return err
	}
	return nil
}

func readAnnotationFile(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("annotation %s is not a regular file", name)
	}
	f, err := root.OpenFile(name, os.O_RDONLY|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readAnnotationBytes(f)
}

func readAnnotationBytes(f *os.File) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("annotation descriptor is not a regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxSidecarBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxSidecarBytes {
		return nil, fmt.Errorf("annotation exceeds %d bytes", MaxSidecarBytes)
	}
	return b, nil
}

func writeAnnotationFile(root *os.Root, name string, b []byte) error {
	// Keep temporary data beside its destination so rename remains atomic.
	tmp := name + "." + uuid.NewString() + ".tmp"
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer root.Remove(tmp)
	if err := flushAnnotationFile(f, b); err != nil {
		return err
	}
	if err := root.Rename(tmp, name); err != nil {
		return err
	}
	dir := filepath.Dir(name)
	d, err := root.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return err
	}
	// Persist creation of the history directory before replacing the head.
	if dir != "." {
		return rootSync(root)
	}
	return nil
}

func rootSync(root *os.Root) error {
	d, err := root.Open(".")
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// flushAnnotationFile always closes the descriptor, including write/sync failure.
func flushAnnotationFile(f interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
}, b []byte) error {
	n, writeErr := f.Write(b)
	if n != len(b) && writeErr == nil {
		writeErr = io.ErrShortWrite
	}
	return errors.Join(writeErr, f.Sync(), f.Close())
}
