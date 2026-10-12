package annotation

import (
	"encoding/base64"
	"encoding/binary"
	"flag"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The macOS annotation client reads packs itself, so the two implementations
// have to agree byte for byte about the layout: all X, then all Y, then all Z
// as little-endian float32, then intensity and classification as uint8; the
// packet table and one return per point as little-endian uint64; the samples
// digest taken before the trailing newline; and the pack digest over the
// points, samples, packets and returns digests.
//
// testdata/swift-pack-v2 is one small pack written by this package's writer.
// The Swift reader's tests read the same directory
// (tools/visualiser-macos/VelocityVisualiserTests/AnnotationPackTests.swift),
// so a change to the layout fails on both sides. Regenerate it with
// -update-swift-pack-fixture only when the format changes on purpose.
var updateSwiftPackFixture = flag.Bool("update-swift-pack-fixture", false, "rewrite testdata/swift-pack-v2")

const swiftPackFixtureDir = "testdata/swift-pack-v2"

func swiftFixtureBlock(xs, ys, zs []float32, intensity, class []uint8) []byte {
	n := len(xs)
	buf := make([]byte, n*12+n*2)
	off := 0
	for _, arr := range [][]float32{xs, ys, zs} {
		for _, v := range arr {
			binary.LittleEndian.PutUint32(buf[off:], math.Float32bits(v))
			off += 4
		}
	}
	for i := 0; i < n; i++ {
		buf[off] = intensity[i]
		off++
	}
	for i := 0; i < n; i++ {
		buf[off] = class[i]
		off++
	}
	return buf
}

// writeSwiftFixturePack writes the shared fixture pack into dir: two samples
// of four and two points, each from its own packet.
func writeSwiftFixturePack(t *testing.T, dir string) {
	t.Helper()
	block0 := swiftFixtureBlock(
		[]float32{1.0, 1.5, 2.0, 40.0},
		[]float32{1.0, 1.25, 1.5, -30.0},
		[]float32{0.5, 0.75, 1.0, 3.0},
		[]uint8{10, 20, 30, 40},
		[]uint8{1, 1, 1, 2},
	)
	block1 := swiftFixtureBlock(
		[]float32{3.0, 3.5},
		[]float32{2.0, 2.5},
		[]float32{1.5, 1.75},
		[]uint8{50, 60},
		[]uint8{1, 1},
	)
	present := true
	samples := []Sample{
		{SourceOrdinal: 0, SourceFrameID: 100, TimestampNs: 1_000_000_000, SensorID: "hesai-pandar40p", PointCount: 4,
			HasIntensity: &present, ReturnStart: 0, ReturnEnd: 400},
		{SourceOrdinal: 1, SourceFrameID: 101, TimestampNs: 1_100_000_000, SensorID: "hesai-pandar40p", PointCount: 2,
			HasIntensity: &present, ReturnStart: 400, ReturnEnd: 800},
	}
	m := Manifest{
		Source: SourceProvenance{VRLOGPath: "fixture.vrlog", VRLOGHeaderSHA: "sha256:aa", VRLOGFramesSHA: "sha256:bb",
			PCAPBasename: "fixture.pcapng", SensorID: "hesai-pandar40p", BuildVersion: "fixture"},
		Coordinate: CoordinateContract{Units: "metres", FrameID: "sensor", ReferenceFrame: "site", Handedness: "right",
			OriginNote: "sensor origin", TransformVersion: "v1"},
		Coverage: CoverageForegroundOnly, CoverageNote: "fixture", HasIntensity: true, HasClassification: true,
	}
	returns := PackReturns{
		Layout:  ReturnLayout{Model: "hesai-pandar40p", Blocks: 10, Channels: 40},
		Packets: []uint64{0x0123456789abcdef, 0xfedcba9876543210},
		Returns: [][]uint64{{0, 41, 82, 399}, {400, 445}},
	}
	if err := WritePack(dir, m, samples, [][]byte{block0, block1}, returns); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestSwiftFixturePackBytesAreStable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	writeSwiftFixturePack(t, dir)
	files := []string{manifestFile, samplesFile, pointsFile, packetsFile, returnsFile}
	if *updateSwiftPackFixture {
		if err := os.MkdirAll(swiftPackFixtureDir, 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range files {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(swiftPackFixtureDir, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range files {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join(swiftPackFixtureDir, name))
		if err != nil {
			t.Fatalf("%v (run with -update-swift-pack-fixture to create it)", err)
		}
		if string(got) != string(want) {
			t.Errorf("%s differs from %s: the pack layout changed; regenerate only on purpose, and update AnnotationPackTests.swift with it", name, swiftPackFixtureDir)
		}
	}

	pack, err := OpenPack(swiftPackFixtureDir)
	if err != nil {
		t.Fatalf("the fixture no longer opens: %v", err)
	}
	// The layout claim the Swift decoder depends on: sample 1's block starts
	// immediately after sample 0's 4 points (4*12 + 4*2 = 56 bytes).
	if pack.Samples[1].ByteOffset != 56 {
		t.Errorf("sample 1 byte offset = %d, want 56", pack.Samples[1].ByteOffset)
	}
	second, err := pack.PointsAt(1)
	if err != nil || len(second.X) != 2 || second.X[1] != 3.5 || second.Intensity[1] != 60 {
		t.Errorf("sample 1 decoded as %+v (%v)", second, err)
	}
	returns, err := pack.ReturnsAt(1)
	if err != nil || len(returns) != 2 || returns[1] != 445 {
		t.Errorf("sample 1 returns %v (%v)", returns, err)
	}

	// Fails closed like any other pack.
	points, _ := os.ReadFile(filepath.Join(swiftPackFixtureDir, pointsFile))
	points[0] ^= 0xFF
	tampered := filepath.Join(t.TempDir(), "tampered")
	os.MkdirAll(tampered, 0o755)
	for _, name := range files {
		b, _ := os.ReadFile(filepath.Join(swiftPackFixtureDir, name))
		if name == pointsFile {
			b = points
		}
		os.WriteFile(filepath.Join(tampered, name), b, 0o644)
	}
	if _, err := OpenPack(tampered); err == nil {
		t.Error("a pack with altered points opened")
	}
}

// The background context is laid out by hand on both sides too: all X, then
// all Y, then all Z as little-endian float32, indexed by a JSON file whose
// digest is taken before the trailing newline. The same bytes and digests are
// embedded in the Swift reader's tests
// (tools/visualiser-macos/VelocityVisualiserTests/AnnotationBackgroundTests.swift).
const (
	swiftFixtureBackgroundBase64    = "AADAPwAAAMAAAEBAAACIQAAAAL8AAMBA"
	swiftFixtureBackgroundPointsSHA = "sha256:9e890764a8e9cc124c1296e1c8fdff379ee970f840df9c6d3d2e886d0eb86239"
	swiftFixtureBackgroundsSHA      = "sha256:691fdcc25726fbf8872126b9252ae21c20d8fffb466cf1c23508a78298355529"
)

func TestSwiftFixtureBackgroundIsByteStable(t *testing.T) {
	block, err := encodeBackground([]float32{1.5, -2}, []float32{3, 4.25}, []float32{-0.5, 6})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if got := base64.StdEncoding.EncodeToString(block); got != swiftFixtureBackgroundBase64 {
		t.Errorf("background bytes changed: %s\nUpdate the Swift fixture to match.", got)
	}

	dir := filepath.Join(t.TempDir(), "pack")
	samples := []Sample{{SourceOrdinal: 1, TimestampNs: 1000, SensorID: "s", PointCount: 1}}
	points := swiftFixtureBlock([]float32{1}, []float32{1}, []float32{1}, []uint8{1}, []uint8{1})
	backgrounds := []Background{{
		SourceOrdinal: 0, TimestampNs: 999, SettlingComplete: true, PointCount: 2,
	}}
	if err := WritePackWithBackground(
		dir, Manifest{Coverage: CoverageForegroundOnly}, samples, [][]byte{points},
		FixtureReturns(samples), backgrounds, [][]byte{block},
	); err != nil {
		t.Fatalf("write: %v", err)
	}
	pack, err := OpenPack(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if got := pack.Manifest.BackgroundPointsSHA256; got != swiftFixtureBackgroundPointsSHA {
		t.Errorf("background points digest = %s\nUpdate the Swift fixture to match.", got)
	}
	if got := pack.Manifest.BackgroundsSHA256; got != swiftFixtureBackgroundsSHA {
		t.Errorf("backgrounds index digest = %s\nUpdate the Swift fixture to match.", got)
	}
	if _, err := os.Stat(filepath.Join(dir, backgroundsFile)); err != nil {
		t.Errorf("backgrounds index not written: %v", err)
	}
}
