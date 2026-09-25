package vrlog

// Test-only helpers, exported so the pcap-tagged external test package can
// inject the same damage into a kirk0 container as the unit tests do into
// synthetic ones. Nothing here is compiled into the package itself.

import (
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// ChunkPath and IndexPath name a chunk's objects.
func ChunkPath(dir string, ordinal uint64) string { return filepath.Join(dir, chunkObject(ordinal)) }
func IndexPath(dir string, ordinal uint64) string { return filepath.Join(dir, indexObject(ordinal)) }

// RecordSpan returns the byte offset and length of record i in a chunk, from
// its stored index.
func RecordSpan(t testing.TB, dir string, ordinal uint64, i int) (uint64, uint32) {
	t.Helper()
	ix := readIndexForTest(t, dir, ordinal)
	return ix.Offset[i], ix.Length[i]
}

func readIndexForTest(t testing.TB, dir string, ordinal uint64) *pb.ChunkIndex {
	t.Helper()
	data, err := os.ReadFile(IndexPath(dir, ordinal))
	if err != nil {
		t.Fatal(err)
	}
	var m pb.ChunkIndex
	if err := unmarshalOptions.Unmarshal(data[preambleSize+envelopeSize:], &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

// FlipBit inverts one bit of a file in place.
func FlipBit(t testing.TB, path string, offset uint64, bit uint) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[offset] ^= 1 << bit
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// Truncate cuts a file to size bytes.
func Truncate(t testing.TB, path string, size int64) {
	t.Helper()
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}
}

// SwapFiles exchanges two files' contents.
func SwapFiles(t testing.TB, a, b string) {
	t.Helper()
	tmp := a + ".swap"
	for _, step := range [][2]string{{a, tmp}, {b, a}, {tmp, b}} {
		if err := os.Rename(step[0], step[1]); err != nil {
			t.Fatal(err)
		}
	}
}

// ForgeRecordLength rewrites record i's declared payload length in chunk
// ordinal and then re-seals the chunk and its index consistently, so the
// chunk digest, seal and index all pass and only the length lies. A reader
// must refuse it by bounds, before allocating or reading past the record.
func ForgeRecordLength(t testing.TB, dir string, ordinal uint64, i int, length uint32) {
	t.Helper()
	offset, _ := RecordSpan(t, dir, ordinal, i)
	Reseal(t, dir, ordinal, func(body []byte) {
		binary.LittleEndian.PutUint32(body[offset+32:], length)
		binary.LittleEndian.PutUint32(body[offset+36:], length)
	})
}

// Reseal applies mutate to a chunk's body (the bytes before its seal), then
// recomputes the body digest in the seal and the index, and their CRCs.
func Reseal(t testing.TB, dir string, ordinal uint64, mutate func(body []byte)) {
	t.Helper()
	path := ChunkPath(dir, ordinal)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ix := readIndexForTest(t, dir, ordinal)
	body, sealRecord := data[:ix.SealOffset], data[ix.SealOffset:]
	mutate(body)
	sum := sha256.Sum256(body)

	env, err := parseEnvelope(sealRecord)
	if err != nil {
		t.Fatal(err)
	}
	var seal pb.ChunkSeal
	if err := unmarshalOptions.Unmarshal(sealRecord[envelopeSize:], &seal); err != nil {
		t.Fatal(err)
	}
	seal.BodySha256 = sum[:]
	payload, err := marshalOptions.Marshal(&seal)
	if err != nil {
		t.Fatal(err)
	}
	out := appendRecord(append([]byte(nil), body...), env, payload)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}

	ix.ChunkBodySha256 = sum[:]
	ix.ChunkBytes = uint64(len(out))
	ix.SealLength = uint32(len(out)) - uint32(ix.SealOffset)
	RewriteIndex(t, dir, ordinal, func(*pb.ChunkIndex) {}, ix)
}

// RewriteIndex re-encodes a chunk's index after mutate, with a valid CRC, so
// only the mutated values are wrong. from, when non-nil, replaces the stored
// index before mutate runs.
func RewriteIndex(t testing.TB, dir string, ordinal uint64, mutate func(*pb.ChunkIndex), from *pb.ChunkIndex) {
	t.Helper()
	data, err := os.ReadFile(IndexPath(dir, ordinal))
	if err != nil {
		t.Fatal(err)
	}
	env, err := parseEnvelope(data[preambleSize:])
	if err != nil {
		t.Fatal(err)
	}
	ix := from
	if ix == nil {
		ix = readIndexForTest(t, dir, ordinal)
	}
	mutate(ix)
	payload, err := marshalOptions.Marshal(ix)
	if err != nil {
		t.Fatal(err)
	}
	out := appendRecord(appendPreamble(nil, objectIndex), env, payload)
	if err := os.WriteFile(IndexPath(dir, ordinal), out, 0o644); err != nil {
		t.Fatal(err)
	}
}
