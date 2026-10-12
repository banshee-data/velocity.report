package annotation

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The macOS annotation client reads version 1 packs, so the two
// implementations have to agree byte for byte about the canonical layout: all
// X, then all Y, then all Z as little-endian float32, then intensity and
// classification as uint8, with the samples digest taken before the trailing
// newline.
//
// This test pins one small pack's exact bytes and digests. The same values are
// embedded in the Swift reader's tests
// (tools/visualiser-macos/VelocityVisualiserTests/AnnotationPackTests.swift,
// PackFixture). Nothing writes version 1 any more, so the files are laid down
// verbatim here, as the Swift tests lay them down, and the test holds this
// package's reader to them: legacy packs must stay readable for the label set
// migration.
const (
	swiftFixturePointsBase64 = "AACAPwAAwD8AAABAAAAgQgAAgD8AAKA/AADAPwAA8MEAAAA/AABAPwAAgD8AAEBAChQeKAEBAQIAAEBAAABgQAAAAEAAACBAAADAPwAA4D8yPAEB"
	swiftFixturePointsSHA    = "sha256:f8bf067228e595dfee12c0cd3d974db11ebe5474f30e6e08fafef9da47211186"
	swiftFixtureSamplesSHA   = "sha256:38e62501b09464832baa1bcdbcc21b3f58f2b77e58327df59cbda66109654410"
	swiftFixturePackDigest   = "sha256:41b9794747f16ebe2722ea9f75a631f38e35f9670e14dccc3636ed480320d84c"

	// samples.json verbatim, with the writer's trailing newline.
	swiftFixtureSamplesJSON = `[
  {
    "sample_id": 0,
    "source_ordinal": 0,
    "source_frame_id": 100,
    "timestamp_ns": 1000000000,
    "sensor_id": "hesai-pandar40p",
    "point_count": 4,
    "byte_offset": 0
  },
  {
    "sample_id": 1,
    "source_ordinal": 1,
    "source_frame_id": 101,
    "timestamp_ns": 1100000000,
    "sensor_id": "hesai-pandar40p",
    "point_count": 2,
    "byte_offset": 56
  }
]
`

	swiftFixtureManifestJSON = `{
  "schema_version": 1,
  "dataset_id": "ds_41b9794747f16ebe",
  "created_ns": 0,
  "source": {
    "vrlog_path": "fixture.vrlog",
    "vrlog_header_sha256": "sha256:aa",
    "vrlog_frames_sha256": "sha256:bb",
    "sensor_id": "hesai-pandar40p",
    "build_version": "fixture"
  },
  "coordinate": {
    "units": "metres",
    "frame_id": "sensor",
    "reference_frame": "site",
    "handedness": "right",
    "origin_note": "sensor origin",
    "transform_version": "v1"
  },
  "coverage": "foreground_only",
  "coverage_note": "fixture",
  "sample_count": 2,
  "point_count": 6,
  "points_sha256": "` + swiftFixturePointsSHA + `",
  "samples_sha256": "` + swiftFixtureSamplesSHA + `",
  "pack_digest": "` + swiftFixturePackDigest + `",
  "has_intensity": true,
  "has_classification": true,
  "completeness": {
    "requested_start_ns": 0,
    "requested_end_ns": 0,
    "actual_start_ns": 0,
    "actual_end_ns": 0,
    "frames_without_points": 0,
    "duplicate_timestamps": 0,
    "max_timestamp_gap_ns": 0
  }
}
`
)

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

// writeSwiftFixturePack lays the shared version 1 fixture pack down in dir.
func writeSwiftFixturePack(t *testing.T, dir string) {
	t.Helper()
	points, err := base64.StdEncoding.DecodeString(swiftFixturePointsBase64)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{
		manifestFile: []byte(swiftFixtureManifestJSON),
		samplesFile:  []byte(swiftFixtureSamplesJSON),
		pointsFile:   points,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSwiftFixturePackBytesAreStable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	writeSwiftFixturePack(t, dir)

	pack, err := OpenPack(dir)
	if err != nil {
		t.Fatalf("the version 1 fixture no longer opens: %v", err)
	}
	if pack.Manifest.SchemaVersion != PackSchemaVersionLegacy || pack.HasReturns() {
		t.Fatalf("fixture read as version %d with returns %v", pack.Manifest.SchemaVersion, pack.HasReturns())
	}
	if _, err := pack.ReturnsAt(0); err == nil {
		t.Error("a version 1 pack answered for returns it does not carry")
	}

	// The points the bytes encode, as the Swift test reads them.
	points, err := os.ReadFile(filepath.Join(dir, pointsFile))
	if err != nil {
		t.Fatal(err)
	}
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
	if string(points) != string(append(block0, block1...)) {
		t.Error("points.bin no longer encodes the fixture's points; update AnnotationPackTests.swift only if the layout changed on purpose")
	}
	if got := sha256Hex(points); got != swiftFixturePointsSHA {
		t.Errorf("points digest = %s, want %s", got, swiftFixturePointsSHA)
	}
	if got := sha256Hex([]byte(strings.TrimSuffix(swiftFixtureSamplesJSON, "\n"))); got != swiftFixtureSamplesSHA {
		t.Errorf("samples digest = %s, want %s: the digest is over the bytes before the trailing newline", got, swiftFixtureSamplesSHA)
	}
	if pack.Manifest.PackDigest != swiftFixturePackDigest || packDigest(pack.Manifest) != swiftFixturePackDigest {
		t.Errorf("pack digest = %s, want %s: the version 1 digest rule changed", packDigest(pack.Manifest), swiftFixturePackDigest)
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

	// Fails closed like any other pack.
	points[0] ^= 0xFF
	if err := os.WriteFile(filepath.Join(dir, pointsFile), points, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPack(dir); err == nil {
		t.Error("a version 1 pack with altered points opened")
	}
}

// TestSwiftWrittenSidecarIsReadable proves a sidecar carrying the fields the
// Swift client writes round-trips through this package's reader. The Swift
// store advances revisions itself, so schema drift there would otherwise only
// surface as an unreadable pack on an operator's machine.
func TestSwiftWrittenSidecarIsReadable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	writeSwiftFixturePack(t, dir)
	pack, err := OpenPack(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Exactly the shape AnnotationSession.save writes: schema 1, an object
	// and a mask with sorted canonical indices, operator provenance, and a
	// change record naming the parent revision.
	swiftWritten := `{
  "change" : {
    "author" : "dd",
    "created_utc" : "2026-09-15T21:00:00Z",
    "operation" : "save_mask",
    "parent_revision" : 0,
    "revision" : 1,
    "session" : "11111111-2222-3333-4444-555555555555"
  },
  "dataset_id" : "` + pack.Manifest.DatasetID + `",
  "masks" : [
    {
      "completeness" : "partial",
      "object_id" : "obj_abcd1234",
      "point_indices" : [ 0, 1, 2 ],
      "provenance" : {
        "author" : "dd",
        "created_utc" : "2026-09-15T21:00:00Z",
        "operation" : "save_mask",
        "revision" : 0
      },
      "sample_id" : 0,
      "status" : "proposed",
      "visibility" : "present"
    }
  ],
  "objects" : [
    {
      "class" : "car",
      "confidence" : 1,
      "object_id" : "obj_abcd1234",
      "provenance" : {
        "author" : "dd",
        "created_utc" : "2026-09-15T21:00:00Z",
        "operation" : "create_object",
        "revision" : 0
      },
      "status" : "proposed"
    }
  ],
  "pack_digest" : "` + pack.Manifest.PackDigest + `",
  "revision" : 1,
  "schema_version" : 1,
  "updated_utc" : "2026-09-15T21:00:00Z"
}
`
	if err := os.WriteFile(filepath.Join(dir, "annotations.json"), []byte(swiftWritten), 0o644); err != nil {
		t.Fatal(err)
	}

	sidecar, err := LoadSidecar(pack)
	if err != nil {
		t.Fatalf("a Swift-written sidecar must load here: %v", err)
	}
	if len(sidecar.Objects) != 1 || sidecar.Objects[0].ObjectID != "obj_abcd1234" {
		t.Fatalf("objects = %+v", sidecar.Objects)
	}
	if len(sidecar.Masks) != 1 {
		t.Fatalf("masks = %+v", sidecar.Masks)
	}
	mask := sidecar.Masks[0]
	if mask.SampleID != 0 || len(mask.PointIndices) != 3 {
		t.Fatalf("mask = %+v", mask)
	}
	if mask.Completeness != MaskPartial || mask.Visibility != VisiblePresent {
		t.Fatalf("mask enums did not decode: completeness=%q visibility=%q", mask.Completeness, mask.Visibility)
	}
	// A saved-but-unreviewed mask must not count as reference truth.
	if got := sidecar.ReviewedMasks(); len(got) != 0 {
		t.Fatalf("a proposed mask under a proposed object is not reviewed truth, got %d", len(got))
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
