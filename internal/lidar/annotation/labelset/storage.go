package labelset

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The head as it is kept on disk.
//
// A state's identity is its canonical encoding (EncodeState): packets.bin and
// five canonical-JSON files, digested by StateDigest. That never changes for a
// state, whatever is on disk. The files on disk are laid out for the people
// reading their diffs and for plain Git:
//
//	packets.bin                    the packet table, unchanged
//	objects.json                   records, one per line
//	domains.json                   domains, one per line
//	windows.json                   windows, one per line
//	masks/<first>-<last>.ndjson    masks of windows first..last, one per line
//	domain-returns/<...>.ndjson    foreground returns of the same windows
//
// Masks and foreground returns are split into files of storage.window_chunk
// windows, so a mask edit rewrites one small file and every text file can stay
// under Hugging Face's 10 MiB limit for plain Git. A reader decodes the files,
// digests the canonical encoding of what it read, and refuses files that are
// not exactly the layout of that state, so two writers that follow these rules
// write the same bytes.

// Storage is labels.json's storage block: how the head is laid out on disk.
type Storage struct {
	WindowChunk int `json:"window_chunk"`
}

const (
	// DefaultWindowChunk is the window span of a new label set's chunk files:
	// about 0.85 MB of masks at kirk0's density.
	DefaultWindowChunk = 100
	// MaxWindowChunk bounds the span, so a chunk name stays a number.
	MaxWindowChunk = 1_000_000
	MasksDir       = "masks"
	// DomainReturnsDir holds the foreground returns, split as the masks are.
	DomainReturnsDir = "domain-returns"
	// MaxPlainGitBytes is the largest file Hugging Face keeps in plain Git.
	MaxPlainGitBytes = 10 << 20
)

// wholeFiles are the head files kept whole, in the order a listing gives them.
var wholeFiles = []string{PacketsFile, RecordsFile, DomainsFile, WindowsFile}

// chunkDirs are the head directories split by window.
var chunkDirs = []string{DomainReturnsDir, MasksDir}

var chunkNamePattern = regexp.MustCompile(`^([0-9]{6,})-([0-9]{6,})\.ndjson$`)

// ErrOldLayout marks a label set written before the storage layout, with its
// masks in one file. Relayout converts it; its digest does not change.
var ErrOldLayout = errors.New("label set is in the single-file layout: convert it with 'velocity lidar labels relayout'")

func (st Storage) validate() error {
	if st.WindowChunk < 1 || st.WindowChunk > MaxWindowChunk {
		return fmt.Errorf("storage window_chunk %d, want 1 to %d", st.WindowChunk, MaxWindowChunk)
	}
	return nil
}

// chunkName is the file holding a window's masks or returns.
func (st Storage) chunkName(window int) string {
	first := window / st.WindowChunk * st.WindowChunk
	return fmt.Sprintf("%06d-%06d.ndjson", first, first+st.WindowChunk-1)
}

// encodeStorage renders a state's head as it is kept on disk, keyed by
// slash-separated path within the head directory.
func encodeStorage(in *State, st Storage) (map[string][]byte, error) {
	if err := st.validate(); err != nil {
		return nil, err
	}
	c := in.Clone()
	c.Canonicalise()
	s := &c
	files := map[string][]byte{}
	packets := make([]byte, 8*len(s.Packets))
	for i, k := range s.Packets {
		binary.LittleEndian.PutUint64(packets[8*i:], uint64(k))
	}
	files[PacketsFile] = packets
	domains, windows := s.Domains, s.Windows
	if domains == nil {
		domains = []Domain{}
	}
	if windows == nil {
		windows = []Window{}
	}
	for name, v := range map[string]any{RecordsFile: s.Records, DomainsFile: domains, WindowsFile: windows} {
		b, err := recordLayout(v)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		files[name] = b
	}
	// Both lists are in canonical order, so each chunk's lines are too.
	for _, d := range s.DomainReturns {
		b, err := canonicalLine(d)
		if err != nil {
			return nil, err
		}
		name := path.Join(DomainReturnsDir, st.chunkName(d.WindowID))
		files[name] = append(files[name], b...)
	}
	for _, m := range s.Masks {
		b, err := canonicalLine(m)
		if err != nil {
			return nil, err
		}
		name := path.Join(MasksDir, st.chunkName(m.WindowID))
		files[name] = append(files[name], b...)
	}
	return files, nil
}

// recordLayout writes JSON with one record per line: the canonical encoding,
// except that the elements of the top-level array, or of each array that is a
// top-level field, each sit on their own line. Keys are sorted, indented two
// spaces per level, with ": " after a key, and the file ends in a newline.
func recordLayout(v any) ([]byte, error) {
	raw, err := Canonical(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	switch t := generic.(type) {
	case []any:
		if err := writeRecordArray(&buf, t, ""); err != nil {
			return nil, err
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if len(keys) == 0 {
			buf.WriteString("{}")
			break
		}
		buf.WriteString("{\n")
		for i, k := range keys {
			buf.WriteString("  ")
			writeCanonicalString(&buf, k)
			buf.WriteString(": ")
			var err error
			if a, ok := t[k].([]any); ok {
				err = writeRecordArray(&buf, a, "  ")
			} else {
				err = writeCanonical(&buf, t[k])
			}
			if err != nil {
				return nil, err
			}
			if i < len(keys)-1 {
				buf.WriteByte(',')
			}
			buf.WriteByte('\n')
		}
		buf.WriteByte('}')
	default:
		if err := writeCanonical(&buf, generic); err != nil {
			return nil, err
		}
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

func writeRecordArray(buf *bytes.Buffer, a []any, indent string) error {
	if len(a) == 0 {
		buf.WriteString("[]")
		return nil
	}
	buf.WriteString("[\n")
	for i, e := range a {
		buf.WriteString(indent + "  ")
		if err := writeCanonical(buf, e); err != nil {
			return err
		}
		if i < len(a)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(indent + "]")
	return nil
}

// readStorage reads the head kept under dir. It returns the state and the
// bytes it read, keyed as encodeStorage keys them. Hidden files and
// interrupted writes in the chunk directories are passed over; any other
// stray file is refused.
func readStorage(root *os.Root, dir string, st Storage) (*State, map[string][]byte, error) {
	return readStorageSpans(root, dir, st, false)
}

// readStorageSpans is readStorage; with otherSpans it passes over chunk files
// of another span instead of refusing them, which only Relayout does.
func readStorageSpans(root *os.Root, dir string, st Storage, otherSpans bool) (*State, map[string][]byte, error) {
	if err := st.validate(); err != nil {
		return nil, nil, err
	}
	for _, old := range []string{MasksFile, DomainReturnsFile} {
		if _, err := root.Lstat(path.Join(dir, old)); err == nil {
			return nil, nil, fmt.Errorf("%s holds %s, a file of the single-file layout, beside the chunked head", dir, old)
		}
	}
	files := map[string][]byte{}
	for _, name := range wholeFiles {
		b, err := readFile(root, path.Join(dir, name))
		if err != nil {
			return nil, nil, fmt.Errorf("read %s: %w", name, err)
		}
		files[name] = b
	}
	s := &State{}
	p := files[PacketsFile]
	if len(p)%8 != 0 {
		return nil, nil, fmt.Errorf("%s is %d bytes, not whole packet keys", PacketsFile, len(p))
	}
	s.Packets = make([]PacketKey, len(p)/8)
	for i := range s.Packets {
		s.Packets[i] = PacketKey(binary.LittleEndian.Uint64(p[8*i:]))
	}
	if err := strictUnmarshal(files[RecordsFile], &s.Records); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", RecordsFile, err)
	}
	if err := strictUnmarshal(files[DomainsFile], &s.Domains); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", DomainsFile, err)
	}
	if err := strictUnmarshal(files[WindowsFile], &s.Windows); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", WindowsFile, err)
	}
	for _, chunkDir := range chunkDirs {
		names, err := chunkFiles(root, path.Join(dir, chunkDir))
		if err != nil {
			return nil, nil, err
		}
		for _, name := range names {
			m := chunkNamePattern.FindStringSubmatch(name)
			first, _ := strconv.Atoi(m[1])
			last, _ := strconv.Atoi(m[2])
			if first%st.WindowChunk != 0 || last != first+st.WindowChunk-1 {
				if otherSpans {
					continue
				}
				return nil, nil, fmt.Errorf("%s/%s does not span %d windows from a multiple of %d (an interrupted relayout is finished by running it again)",
					chunkDir, name, st.WindowChunk, st.WindowChunk)
			}
			key := path.Join(chunkDir, name)
			b, err := readFile(root, path.Join(dir, key))
			if err != nil {
				return nil, nil, fmt.Errorf("read %s: %w", key, err)
			}
			files[key] = b
			inChunk := func(window int) error {
				if window < first || window > last {
					return fmt.Errorf("window %d is outside %s", window, name)
				}
				return nil
			}
			err = eachLine(b, func(line []byte) error {
				if chunkDir == MasksDir {
					var mask Mask
					if err := strictUnmarshal(line, &mask); err != nil {
						return err
					}
					s.Masks = append(s.Masks, mask)
					return inChunk(mask.WindowID)
				}
				var d DomainReturns
				if err := strictUnmarshal(line, &d); err != nil {
					return err
				}
				s.DomainReturns = append(s.DomainReturns, d)
				return inChunk(d.WindowID)
			})
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", key, err)
			}
		}
	}
	return s, files, nil
}

// chunkFiles lists a chunk directory's files, sorted; a missing directory is
// an empty one.
func chunkFiles(root *os.Root, dir string) ([]string, error) {
	f, err := root.Open(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	entries, err := f.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		if !e.Type().IsRegular() || !chunkNamePattern.MatchString(name) {
			return nil, fmt.Errorf("%s/%s is not a chunk file", dir, name)
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

// checkLayout refuses head files that are not exactly the layout of the state
// they decode to.
func checkLayout(s *State, st Storage, have map[string][]byte) error {
	want, err := encodeStorage(s, st)
	if err != nil {
		return err
	}
	for name, b := range want {
		if got, ok := have[name]; !ok {
			return fmt.Errorf("%s is missing from the head (rewrite it with 'velocity lidar labels rehead')", name)
		} else if !bytes.Equal(got, b) {
			return fmt.Errorf("%s is not in the label set's layout (rewrite it with 'velocity lidar labels rehead')", name)
		}
	}
	for name := range have {
		if _, ok := want[name]; !ok {
			return fmt.Errorf("%s holds nothing the state does (rewrite the head with 'velocity lidar labels rehead')", name)
		}
	}
	return nil
}

// writeHead replaces the head under dir with files: a file whose bytes equal
// have's is left alone, and a chunk file no longer in files is removed.
func writeHead(root *os.Root, dir string, files, have map[string][]byte) error {
	if err := writeFiles(root, dir, files, have); err != nil {
		return err
	}
	return removeStale(root, dir, files)
}

// writeFiles writes every file whose bytes differ from have's.
func writeFiles(root *os.Root, dir string, files, have map[string][]byte) error {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, chunkDir := range chunkDirs {
		if err := root.MkdirAll(path.Join(dir, chunkDir), 0o755); err != nil {
			return err
		}
	}
	for _, name := range names {
		if old, ok := have[name]; ok && bytes.Equal(old, files[name]) {
			continue
		}
		if len(files[name]) > MaxFileBytes {
			return fmt.Errorf("%s would be %d bytes, above the %d limit", name, len(files[name]), MaxFileBytes)
		}
		if err := writeFileAtomic(root, path.Join(dir, name), files[name]); err != nil {
			return fmt.Errorf("replace %s (reopen to recover): %w", name, err)
		}
	}
	return nil
}

// removeStale removes the chunk files under dir that files does not hold.
func removeStale(root *os.Root, dir string, files map[string][]byte) error {
	for _, chunkDir := range chunkDirs {
		existing, err := chunkFiles(root, path.Join(dir, chunkDir))
		if err != nil {
			return err
		}
		for _, name := range existing {
			if _, keep := files[path.Join(chunkDir, name)]; keep {
				continue
			}
			if err := root.Remove(path.Join(dir, chunkDir, name)); err != nil {
				return fmt.Errorf("remove %s/%s (reopen to recover): %w", chunkDir, name, err)
			}
		}
	}
	return nil
}

// Relayout rewrites a label set's head, and its journal base if it has one,
// in the storage layout with chunks of windowChunk windows. It converts a set
// in the single-file layout, or changes an existing set's chunk span. The
// state, its digest and the journal are unchanged: a layout is not a revision.
func Relayout(dir string, windowChunk int) error {
	st := Storage{WindowChunk: windowChunk}
	if err := st.validate(); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	lock, err := lockSet(root)
	if err != nil {
		return err
	}
	defer lock.Close()
	b, err := readFile(root, ManifestFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", ManifestFile, err)
	}
	var m Manifest
	if err := strictUnmarshal(b, &m); err != nil {
		return fmt.Errorf("parse %s: %w", ManifestFile, err)
	}
	heads := []string{"."}
	if _, err := root.Stat(JournalBaseDir); err == nil {
		heads = append(heads, JournalBaseDir)
	}
	// Each head is read under the span labels.json declares, passing over
	// chunk files of any other span: before labels.json is rewritten they are
	// this run's partial output, and after it the old span's leftovers. The
	// digest check below holds the result either way. New files are written,
	// then labels.json, then the leftovers removed, so an interrupted run is
	// finished by running it again.
	written := map[string]map[string][]byte{}
	for _, dir := range heads {
		s, have, err := readAnyLayout(root, dir, m.Storage)
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		if dir == "." {
			if d, err := DigestOf(s); err != nil {
				return err
			} else if d != m.Head.State {
				return fmt.Errorf("head files digest %s, manifest says %s at revision %d: open the label set to recover it first", d, m.Head.State, m.Head.Revision)
			}
		}
		files, err := encodeStorage(s, st)
		if err != nil {
			return err
		}
		if err := writeFiles(root, dir, files, have); err != nil {
			return err
		}
		written[dir] = files
	}
	m.Storage = st
	if err := writeManifest(root, &m); err != nil {
		return err
	}
	for _, dir := range heads {
		if err := removeStale(root, dir, written[dir]); err != nil {
			return err
		}
		for _, old := range []string{MasksFile, DomainReturnsFile} {
			if err := root.Remove(path.Join(dir, old)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// readAnyLayout reads a head in the storage layout, or in the single-file
// layout when the manifest names no storage. Only Relayout reads the old one.
func readAnyLayout(root *os.Root, dir string, st Storage) (*State, map[string][]byte, error) {
	if st.WindowChunk != 0 {
		return readStorageSpans(root, dir, st, true)
	}
	files, err := readStateFiles(root, dir)
	if err != nil {
		return nil, nil, err
	}
	s, err := DecodeState(files)
	if err != nil {
		return nil, nil, err
	}
	// Whole files at the same names are rewritten in place; the old masks and
	// returns files are removed by the caller.
	have := map[string][]byte{}
	for _, name := range wholeFiles {
		have[name] = files[name]
	}
	return s, have, nil
}
