package labelset

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"unicode/utf8"
)

// File names inside a label set directory.
const (
	ManifestFile      = "labels.json"
	PacketsFile       = "packets.bin"
	RecordsFile       = "objects.json"
	DomainsFile       = "domains.json"
	WindowsFile       = "windows.json"
	DomainReturnsFile = "domain-returns.ndjson"
	MasksFile         = "masks.ndjson"
	JournalFile       = "journal.ndjson"
	JournalBaseDir    = "journal-base"
	LockFile          = ".lock"
	// PendingFile names the revision a commit is making while it makes it:
	// written before the journal line, removed after labels.json. A journal
	// ahead of the head is an interrupted save only when it is present.
	PendingFile = ".pending"
)

// stateFiles is the order in which the state digest reads the head files.
var stateFiles = []string{PacketsFile, RecordsFile, DomainsFile, WindowsFile, DomainReturnsFile, MasksFile}

// MaxFileBytes bounds any one head file or journal line on read.
const MaxFileBytes = 256 << 20

// Canonical encodes v as canonical JSON: object keys sorted by code point, no
// insignificant whitespace, numbers as Go's encoding/json writes them (the
// ECMAScript form RFC 8785 uses), and strings escaped only where JSON
// requires. Every head file and journal line is written this way, so one
// state has exactly one encoding and the state digest can be recomputed by
// any writer that follows the same rules.
func Canonical(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
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
	if err := writeCanonical(&buf, generic); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		buf.WriteString(strconv.FormatBool(t))
	case json.Number:
		buf.WriteString(t.String())
	case string:
		writeCanonicalString(buf, t)
	case []any:
		buf.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, e); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeCanonicalString(buf, k)
			buf.WriteByte(':')
			if err := writeCanonical(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("canonical JSON: unexpected %T", v)
	}
	return nil
}

// writeCanonicalString escapes the quote, the backslash and control
// characters only; everything else, including '/', '<', '>' and '&', is
// written as UTF-8.
func writeCanonicalString(buf *bytes.Buffer, s string) {
	buf.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			buf.WriteString(`\"`)
		case '\\':
			buf.WriteString(`\\`)
		case '\b':
			buf.WriteString(`\b`)
		case '\f':
			buf.WriteString(`\f`)
		case '\n':
			buf.WriteString(`\n`)
		case '\r':
			buf.WriteString(`\r`)
		case '\t':
			buf.WriteString(`\t`)
		default:
			if r < 0x20 {
				fmt.Fprintf(buf, `\u%04x`, r)
			} else if r == utf8.RuneError {
				buf.WriteString("�")
			} else {
				buf.WriteRune(r)
			}
		}
	}
	buf.WriteByte('"')
}

// canonicalLine is Canonical plus a newline.
func canonicalLine(v any) ([]byte, error) {
	b, err := Canonical(v)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// EncodeState renders a state as the bytes of its head files, in canonical
// order whatever order s is in.
func EncodeState(in *State) (map[string][]byte, error) {
	c := in.Clone()
	c.Canonicalise()
	s := &c
	files := make(map[string][]byte, len(stateFiles))
	packets := make([]byte, 8*len(s.Packets))
	for i, k := range s.Packets {
		binary.LittleEndian.PutUint64(packets[8*i:], uint64(k))
	}
	files[PacketsFile] = packets
	var err error
	if files[RecordsFile], err = canonicalLine(s.Records); err != nil {
		return nil, err
	}
	domains := s.Domains
	if domains == nil {
		domains = []Domain{}
	}
	if files[DomainsFile], err = canonicalLine(domains); err != nil {
		return nil, err
	}
	windows := s.Windows
	if windows == nil {
		windows = []Window{}
	}
	if files[WindowsFile], err = canonicalLine(windows); err != nil {
		return nil, err
	}
	var lines bytes.Buffer
	for _, d := range s.DomainReturns {
		b, err := canonicalLine(d)
		if err != nil {
			return nil, err
		}
		lines.Write(b)
	}
	files[DomainReturnsFile] = append([]byte(nil), lines.Bytes()...)
	lines.Reset()
	for _, m := range s.Masks {
		b, err := canonicalLine(m)
		if err != nil {
			return nil, err
		}
		lines.Write(b)
	}
	files[MasksFile] = append([]byte(nil), lines.Bytes()...)
	return files, nil
}

// StateDigest is SHA-256 over the head files in a fixed order, each framed by
// its name and length, so no rearrangement of bytes between files can keep
// the digest.
func StateDigest(files map[string][]byte) string {
	h := sha256.New()
	h.Write([]byte("velocity.report label set state v2\n"))
	for _, name := range stateFiles {
		b := files[name]
		fmt.Fprintf(h, "%s\n%d\n", name, len(b))
		h.Write(b)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// DigestOf returns the digest of the state's canonical encoding.
func DigestOf(s *State) (string, error) {
	files, err := EncodeState(s)
	if err != nil {
		return "", err
	}
	return StateDigest(files), nil
}

// DecodeState parses head files. It does not validate; callers do, against
// the manifest's layout.
func DecodeState(files map[string][]byte) (*State, error) {
	s := &State{}
	p := files[PacketsFile]
	if len(p)%8 != 0 {
		return nil, fmt.Errorf("%s is %d bytes, not whole packet keys", PacketsFile, len(p))
	}
	s.Packets = make([]PacketKey, len(p)/8)
	for i := range s.Packets {
		s.Packets[i] = PacketKey(binary.LittleEndian.Uint64(p[8*i:]))
	}
	if err := strictUnmarshal(files[RecordsFile], &s.Records); err != nil {
		return nil, fmt.Errorf("%s: %w", RecordsFile, err)
	}
	if err := strictUnmarshal(files[DomainsFile], &s.Domains); err != nil {
		return nil, fmt.Errorf("%s: %w", DomainsFile, err)
	}
	if err := strictUnmarshal(files[WindowsFile], &s.Windows); err != nil {
		return nil, fmt.Errorf("%s: %w", WindowsFile, err)
	}
	if err := eachLine(files[DomainReturnsFile], func(b []byte) error {
		var d DomainReturns
		if err := strictUnmarshal(b, &d); err != nil {
			return err
		}
		s.DomainReturns = append(s.DomainReturns, d)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("%s: %w", DomainReturnsFile, err)
	}
	if err := eachLine(files[MasksFile], func(b []byte) error {
		var m Mask
		if err := strictUnmarshal(b, &m); err != nil {
			return err
		}
		s.Masks = append(s.Masks, m)
		return nil
	}); err != nil {
		return nil, fmt.Errorf("%s: %w", MasksFile, err)
	}
	return s, nil
}

func strictUnmarshal(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("trailing data")
	}
	return nil
}

func eachLine(b []byte, fn func([]byte) error) error {
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(make([]byte, 0, 1<<20), MaxFileBytes)
	n := 0
	for sc.Scan() {
		n++
		if len(sc.Bytes()) == 0 {
			return fmt.Errorf("line %d is empty", n)
		}
		if err := fn(sc.Bytes()); err != nil {
			return fmt.Errorf("line %d: %w", n, err)
		}
	}
	return sc.Err()
}

// readLimited reads at most MaxFileBytes.
func readLimited(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > MaxFileBytes {
		return nil, fmt.Errorf("file exceeds %d bytes", MaxFileBytes)
	}
	return b, nil
}
