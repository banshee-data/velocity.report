package vrlog

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
)

// Container version. Major 1 is the first typed container; the legacy
// FrameBundle layout is 0.5 and shares nothing with it on disk. A reader
// refuses any other major. A minor revision is additive: anything an older
// reader must not ignore is declared as a manifest required feature instead.
// Minor 1 adds commit generations and the current-generation pointer, and
// every 1.1 manifest requires featureCommitGenerations, so a 1.0 reader,
// which would list sealed chunks without knowing whether they were ever
// committed, refuses it.
const (
	FormatMajor uint16 = 1
	FormatMinor uint16 = 1
)

// featureCommitGenerations is the required feature naming the commit chain:
// evidence is exactly the chunks the validated generation chain commits,
// never whatever sealed objects a directory happens to hold.
const featureCommitGenerations = "commit-generations"

// rootMagic opens every object. The PNG-style bytes catch the usual ways a
// binary file is damaged in transit: the high-bit byte a 7-bit channel
// strips, CR-LF and LF a newline conversion rewrites, and ^Z.
var rootMagic = [8]byte{0x89, 'V', 'R', 'L', '\r', '\n', 0x1a, '\n'}

// objectKind is the preamble's four-character object type. It stops a chunk
// being read as an index, or a summary as a manifest.
type objectKind [4]byte

var (
	objectManifest   = objectKind{'M', 'A', 'N', 'I'}
	objectChunk      = objectKind{'C', 'H', 'N', 'K'}
	objectIndex      = objectKind{'I', 'N', 'D', 'X'}
	objectSummary    = objectKind{'S', 'U', 'M', 'M'}
	objectGeneration = objectKind{'G', 'E', 'N', 'R'}
	objectCurrent    = objectKind{'C', 'U', 'R', 'R'}
)

func (k objectKind) String() string { return string(k[:]) }

// preambleSize is root magic, object kind, major and minor.
const preambleSize = 16

func appendPreamble(dst []byte, kind objectKind) []byte {
	dst = append(dst, rootMagic[:]...)
	dst = append(dst, kind[:]...)
	dst = binary.LittleEndian.AppendUint16(dst, FormatMajor)
	return binary.LittleEndian.AppendUint16(dst, FormatMinor)
}

// checkPreamble reports whether b opens with the root magic, and refuses a
// wrong object kind or an unknown major version.
func checkPreamble(b []byte, want objectKind) error {
	if len(b) < preambleSize || [8]byte(b[:8]) != rootMagic {
		return errNotContainer
	}
	if got := objectKind(b[8:12]); got != want {
		return fmt.Errorf("object is a %s, not a %s", got, want)
	}
	if major := binary.LittleEndian.Uint16(b[12:14]); major != FormatMajor {
		return &UnsupportedError{What: fmt.Sprintf("container major version %d.%d (this reader knows %d.x)",
			major, binary.LittleEndian.Uint16(b[14:16]), FormatMajor)}
	}
	return nil
}

// RecordKind names what a record's payload is. Kinds with the ancillary bit
// set may be skipped by a reader that does not know them; every other kind
// is critical, and an unknown critical kind stops the read. Values are
// durable identifiers.
type RecordKind uint16

const (
	RecordManifest    RecordKind = 1
	RecordChunkHeader RecordKind = 2
	RecordChunkSeal   RecordKind = 3
	RecordChunkIndex  RecordKind = 4
	RecordSummary     RecordKind = 5
	RecordGeneration  RecordKind = 6
	RecordCurrent     RecordKind = 7
	RecordFrame       RecordKind = 16
	RecordGap         RecordKind = 17

	// ancillaryBit marks a kind an older reader may skip.
	ancillaryBit RecordKind = 0x8000
)

// Ancillary reports whether a reader may skip the kind when it does not know it.
func (k RecordKind) Ancillary() bool { return k&ancillaryBit != 0 }

func (k RecordKind) String() string {
	switch k {
	case RecordManifest:
		return "manifest"
	case RecordChunkHeader:
		return "chunk-header"
	case RecordChunkSeal:
		return "chunk-seal"
	case RecordChunkIndex:
		return "chunk-index"
	case RecordSummary:
		return "summary"
	case RecordGeneration:
		return "generation"
	case RecordCurrent:
		return "current"
	case RecordFrame:
		return "frame"
	case RecordGap:
		return "gap"
	}
	return fmt.Sprintf("kind(%#04x)", uint16(k))
}

// codecProtobuf is the only payload codec: an uncompressed protobuf message.
// Compression is a later, measured decision and would be a new codec value.
const codecProtobuf uint16 = 1

// recordMagic opens every envelope, so a reader resynchronising after
// damage, or a person with a hex dump, can find record boundaries.
var recordMagic = [4]byte{'V', 'R', 'E', 'C'}

// Envelope layout, little-endian:
//
//	 0  4  magic "VREC"
//	 4  2  record kind
//	 6  2  codec
//	 8  8  container tag: first 8 bytes of the manifest object's SHA-256
//	       (zero in the manifest record itself)
//	16  8  sequence: frame sequence, or a gap's first/next sequence
//	24  8  capture time: frame start or bounded gap start, else zero
//	32  4  stored payload length
//	36  4  uncompressed payload length (equal for codec 1)
//	40  4  flags, zero in 1.0
//	44  4  CRC32C (Castagnoli) of bytes 0..44 then the payload
const envelopeSize = 48

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

type envelope struct {
	kind         RecordKind
	codec        uint16
	tag          [8]byte
	sequence     uint64
	time         int64
	stored       uint32
	uncompressed uint32
	flags        uint32
	crc          uint32
}

// appendRecord appends one enveloped record with its CRC to dst.
func appendRecord(dst []byte, e envelope, payload []byte) []byte {
	start := len(dst)
	dst = append(dst, make([]byte, envelopeSize)...)
	dst = append(dst, payload...)
	putEnvelope(dst[start:], e)
	return dst
}

// putEnvelope fills record[:envelopeSize] for the payload that follows it,
// computing the lengths and the CRC. The codec is always codecProtobuf and
// the flags zero; e's other fields are written as given.
func putEnvelope(record []byte, e envelope) {
	h, payload := record[:envelopeSize], record[envelopeSize:]
	copy(h[0:4], recordMagic[:])
	binary.LittleEndian.PutUint16(h[4:6], uint16(e.kind))
	binary.LittleEndian.PutUint16(h[6:8], codecProtobuf)
	copy(h[8:16], e.tag[:])
	binary.LittleEndian.PutUint64(h[16:24], e.sequence)
	binary.LittleEndian.PutUint64(h[24:32], uint64(e.time))
	binary.LittleEndian.PutUint32(h[32:36], uint32(len(payload)))
	binary.LittleEndian.PutUint32(h[36:40], uint32(len(payload)))
	binary.LittleEndian.PutUint32(h[40:44], 0)
	crc := crc32.Update(0, castagnoli, h[:44])
	binary.LittleEndian.PutUint32(h[44:48], crc32.Update(crc, castagnoli, payload))
}

// parseEnvelope decodes the fixed header. It checks only the magic; the
// caller bounds the lengths before reading or allocating the payload.
func parseEnvelope(b []byte) (envelope, error) {
	if len(b) < envelopeSize {
		return envelope{}, fmt.Errorf("envelope truncated at %d of %d bytes", len(b), envelopeSize)
	}
	if [4]byte(b[:4]) != recordMagic {
		return envelope{}, fmt.Errorf("record magic %q is not %q", b[:4], recordMagic[:])
	}
	return envelope{
		kind:         RecordKind(binary.LittleEndian.Uint16(b[4:6])),
		codec:        binary.LittleEndian.Uint16(b[6:8]),
		tag:          [8]byte(b[8:16]),
		sequence:     binary.LittleEndian.Uint64(b[16:24]),
		time:         int64(binary.LittleEndian.Uint64(b[24:32])),
		stored:       binary.LittleEndian.Uint32(b[32:36]),
		uncompressed: binary.LittleEndian.Uint32(b[36:40]),
		flags:        binary.LittleEndian.Uint32(b[40:44]),
		crc:          binary.LittleEndian.Uint32(b[44:48]),
	}, nil
}

// checkBody verifies codec, flags and CRC once the payload is in hand.
func (e envelope) checkBody(header, payload []byte) error {
	if e.codec != codecProtobuf {
		return &UnsupportedError{What: fmt.Sprintf("payload codec %d", e.codec)}
	}
	if e.flags != 0 {
		return &UnsupportedError{What: fmt.Sprintf("envelope flags %#x", e.flags)}
	}
	if e.uncompressed != e.stored {
		return fmt.Errorf("uncompressed length %d differs from stored length %d for an uncompressed codec", e.uncompressed, e.stored)
	}
	crc := crc32.Update(0, castagnoli, header[:44])
	if crc = crc32.Update(crc, castagnoli, payload); crc != e.crc {
		return fmt.Errorf("CRC32C %#08x does not match the recorded %#08x", crc, e.crc)
	}
	return nil
}

// Object names inside a container directory. The legacy layout's names
// (header.json, index.bin, frames/) are deliberately absent.
const (
	manifestName   = "manifest"
	summaryName    = "summary"
	chunksDir      = "chunks"
	chunkSuffix    = ".chunk"
	indexSuffix    = ".index"
	generationsDir = "generations"
	genSuffix      = ".gen"
	// currentName is the pointer to the latest published generation.
	currentName = "current"
	// lockName is held (flock) by the one writer or recovery allowed at a
	// time. It is never unlinked: a second inode would admit a second writer.
	lockName = "lock"
	// reserveName is preallocated space released to write a failure marker.
	reserveName = "reserve"
	// quarantineDir receives uncommitted objects that recovery moves aside.
	quarantineDir = "quarantine"
	// openSuffix marks an object still being written. It is never read as
	// evidence; the reader reports it as an uncommitted tail.
	openSuffix = ".open"
)

func chunkBase(ordinal uint64) string { return fmt.Sprintf("%08d", ordinal) }

func generationObject(n uint64) string { return generationsDir + "/" + chunkBase(n) + genSuffix }

// IsContainer reports whether dir holds a VRLOG 1.x root: a manifest object
// opening with the root magic. It reads sixteen bytes and trusts nothing
// else; Open validates. Other readers call it to refuse a container by its
// root rather than by guessing at the protobuf inside.
func IsContainer(dir string) bool {
	f, err := os.Open(filepath.Join(dir, manifestName))
	if err != nil {
		return false
	}
	defer f.Close()
	var preamble [preambleSize]byte
	if _, err := io.ReadFull(f, preamble[:]); err != nil {
		return false
	}
	return [8]byte(preamble[:8]) == rootMagic && objectKind(preamble[8:12]) == objectManifest
}

// Bounds on the small objects, checked before they are read into memory.
// They are generous for their contents (a manifest embeds a tuning file of
// tens of kilobytes) and small enough that a hostile file cannot make a
// reader allocate much.
const (
	maxManifestBytes    = 1 << 20
	maxChunkHeaderBytes = 256
	maxSealBytes        = 1024
	maxIndexBytes       = 4 << 20
	maxSummaryBytes     = 4 << 10
	maxGenerationBytes  = 64 << 10
	maxCurrentBytes     = 256
	// maxListedQuarantine bounds the names a recovery record lists; the
	// record also carries the full count.
	maxListedQuarantine = 256
	// chunkOverhead reserves room for the preamble, header and seal, so the
	// writer can promise the chunk bound before it knows the seal's size.
	chunkOverhead = preambleSize + 2*envelopeSize + maxChunkHeaderBytes + maxSealBytes
)
