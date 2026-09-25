package vrlog

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// RecoveryReport says what Recover found and did.
type RecoveryReport struct {
	// Generation is the generation the pointer names afterwards, and the
	// committed account through it.
	Generation  uint64
	State       CaptureState
	EndSequence uint64
	Records     uint64
	// PointerDetail says why the pointer could not be used, if it could not.
	PointerDetail string
	// Promoted are complete generations beyond the pointer now committed.
	Promoted []uint64
	// Quarantined are the objects moved under quarantine/<generation>/.
	Quarantined []string
	// Wrote reports a recovery generation; PointerRewritten a pointer
	// replaced without one (completing an interrupted recovery).
	Wrote, PointerRewritten bool
}

// recoveredBy names this implementation in recovery records.
const recoveredBy = "storage/vrlog.Recover"

// Recover makes an interrupted container consistent, for offline readers,
// without rewriting committed evidence. It takes the container's lock, so
// it refuses a container a live writer holds. It then:
//
//  1. validates the chain up to the pointer; damage inside that
//     acknowledged prefix is returned, located, and nothing is changed (a
//     salvage that omits damage is a separate, explicit operation);
//  2. continues the chain while later generations validate: these were
//     published but never acknowledged, and are promoted;
//  3. moves every object no committed generation references (the open
//     batch, a sealed but uncommitted chunk, a torn generation or temporary
//     object) under quarantine/, never deleting it and never offering it as
//     evidence;
//  4. publishes a recovery generation recording all of this, and marks an
//     unclosed session incomplete, with its tail extent unknown unless a
//     failure marker survived; then points the pointer at it.
//
// Recover is idempotent: a second run, or a run after one interrupted at any
// step, finds nothing left to do or only the pointer to replace, and never
// writes a second record for the same promotion.
func Recover(dir string, opts Options) (RecoveryReport, error) {
	var report RecoveryReport
	r, err := openManifest(dir, opts)
	if err != nil {
		return report, err
	}
	lock, err := acquireLock(dir)
	if err != nil {
		return report, err
	}
	defer lock.Close()

	ptr, ptrErr := readCurrent(dir, r.tag)
	switch {
	case ptrErr == nil:
		if err := r.extendTo(ptr.GetGeneration()); err != nil {
			return report, r.committedDamage(err, ptr)
		}
		if r.chain.digest != [sha256.Size]byte(ptr.GetGenerationSha256()) {
			return report, &CorruptionError{Kind: CorruptDisagreement, Object: generationObject(ptr.GetGeneration()), Chunk: -1, Length: -1,
				Detail: "the current pointer names a generation with a different digest: committed evidence was replaced"}
		}
	case isUnsupported(ptrErr):
		return report, ptrErr
	default:
		report.PointerDetail = ptrErr.Error()
	}
	pointed := r.chain
	var beyond []*pb.CommitGeneration
	for {
		st, err := r.stage(r.chain, r.lastChunk(), r.chain.next())
		if err != nil {
			break // a missing or invalid generation ends the chain; the latter is quarantined below
		}
		if err := r.apply(st); err != nil {
			return report, err
		}
		beyond = append(beyond, st.g)
	}
	// A recovery generation beyond the pointer is the record of an
	// interrupted recovery, and it already promoted the generations before
	// it: only those after the last one still need a record. This is what
	// keeps a promotion from being recorded twice.
	unrecorded := beyond
	for i, g := range beyond {
		if g.GetKind() == pb.GenerationKind_GENERATION_KIND_RECOVERY {
			unrecorded = beyond[i+1:]
		}
	}
	if ptrErr == nil {
		for _, g := range unrecorded {
			report.Promoted = append(report.Promoted, g.GetGeneration())
		}
	}
	uncommitted, err := r.uncommittedObjects()
	if err != nil {
		return report, err
	}
	endsInRecovery := r.chain.kind == pb.GenerationKind_GENERATION_KIND_RECOVERY
	needRecord := len(uncommitted) > 0 || len(unrecorded) > 0 || !r.chain.terminal() || (ptrErr != nil && !endsInRecovery)
	if !r.chain.valid {
		return report, fmt.Errorf("%s: no committed generation, not even generation 0: nothing to recover", dir)
	}

	w := r.recoveryWriter()
	switch {
	case needRecord:
		n := r.chain.next()
		quarantined, err := r.quarantine(uncommitted, n)
		if err != nil {
			return report, err
		}
		record := &pb.RecoveryRecord{PointerValid: ptrErr == nil, PointerDetail: report.PointerDetail,
			PromotedGenerations: report.Promoted, QuarantinedCount: uint64(len(quarantined)),
			SessionIncomplete: !r.chain.closed, TailExtentUnknown: !r.chain.closed && !r.chain.failed,
			RecoveredUnixNanos: time.Now().UnixNano(), RecoveredBy: recoveredBy}
		if ptrErr == nil {
			record.PointerGeneration = pointed.generation
		}
		record.Quarantined = quarantined[:min(len(quarantined), maxListedQuarantine)]
		res := w.publish(&commitJob{prev: r.chain, delta: batchDelta{kind: pb.GenerationKind_GENERATION_KIND_RECOVERY, recovery: record}})
		if res.err != nil {
			return report, fmt.Errorf("publish recovery generation %d: %w", n, res.err)
		}
		r.chain = res.state
		report.Wrote, report.Quarantined = true, quarantined
	case ptrErr != nil || pointed.generation != r.chain.generation:
		if err := w.replacePointer(r.chain); err != nil {
			return report, err
		}
		report.PointerRewritten = true
	}
	_ = os.Remove(filepath.Join(dir, reserveName)) // not evidence; a stopped writer's reserve is dead weight
	report.Generation, report.EndSequence, report.Records = r.chain.generation, r.chain.endSequence, r.chain.records
	report.State = r.accountStatus().State
	return report, nil
}

// recoveryWriter is the minimum of a writer that publish needs.
func (r *Reader) recoveryWriter() *Writer {
	w := &Writer{dir: r.dir, manifestDigest: r.manifestDigest, tag: r.tag}
	w.hooks.Store(recoveryHooks.Load())
	return w
}

// recoveryHooks lets a test interrupt a recovery's own publication.
var recoveryHooks atomic.Pointer[writerHooks]

// replacePointer atomically points current at c.
func (w *Writer) replacePointer(c chainState) error {
	pointer, err := encodeCurrent(w.tag, c)
	if err != nil {
		return err
	}
	if _, err := w.writeObject(currentName, pointer, bytesCurrent); err != nil {
		return err
	}
	return w.syncDirectory(".")
}

// uncommittedObjects lists, in full, every container object no committed
// generation references. Names that are not this format's objects are left
// alone: recovery quarantines its own debris, not a user's files.
func (r *Reader) uncommittedObjects() ([]string, error) {
	var out []string
	root, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}
	for _, e := range root {
		switch name := e.Name(); {
		case name == summaryName && !r.chain.closed:
			out = append(out, name)
		case name == currentName+openSuffix:
			// A pointer written but not renamed holds no evidence, and the
			// next pointer write replaces it.
		case strings.HasSuffix(name, openSuffix):
			out = append(out, name)
		}
	}
	chunks, err := os.ReadDir(filepath.Join(r.dir, chunksDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	committed := uint64(len(r.chunks))
	for _, e := range chunks {
		name := e.Name()
		base, suffix, ok := strings.Cut(name, ".")
		if !ok {
			continue
		}
		ordinal, known := parseOrdinal(base)
		switch suffix {
		case "chunk", "index":
			if known && ordinal >= committed {
				out = append(out, chunksDir+"/"+name)
			}
		case "chunk.open", "index.open":
			if known {
				out = append(out, chunksDir+"/"+name)
			}
		}
	}
	generations, err := os.ReadDir(filepath.Join(r.dir, generationsDir))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range generations {
		name := e.Name()
		base, suffix, ok := strings.Cut(name, ".")
		n, known := parseOrdinal(base)
		switch {
		case !ok || !known:
		case suffix == "gen" && n > r.chain.generation:
			out = append(out, generationsDir+"/"+name)
		case suffix == "gen.open":
			out = append(out, generationsDir+"/"+name)
		}
	}
	sort.Strings(out)
	return out, nil
}

// quarantine moves objects under quarantine/<generation>/, keeping their
// relative names, synchronises the directories, and returns everything that
// directory now holds, including objects an interrupted recovery moved.
func (r *Reader) quarantine(objects []string, generation uint64) ([]string, error) {
	base := filepath.Join(quarantineDir, chunkBase(generation))
	touched := map[string]bool{}
	for _, rel := range objects {
		dest := filepath.Join(r.dir, base, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return nil, err
		}
		if err := os.Rename(filepath.Join(r.dir, rel), dest); err != nil {
			return nil, fmt.Errorf("quarantine %s: %w", rel, err)
		}
		touched[filepath.Dir(filepath.Join(r.dir, rel))] = true
		touched[filepath.Dir(dest)] = true
	}
	if len(objects) > 0 {
		touched[filepath.Join(r.dir, quarantineDir)] = true
		touched[r.dir] = true
	}
	for dir := range touched {
		if err := syncDir(dir); err != nil {
			return nil, err
		}
	}
	var held []string
	err := filepath.WalkDir(filepath.Join(r.dir, base), func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(filepath.Join(r.dir, base), path)
			held = append(held, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(held)
	return held, err
}
