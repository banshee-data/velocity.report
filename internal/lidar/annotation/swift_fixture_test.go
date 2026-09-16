package annotation

import (
	"encoding/base64"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// The macOS annotation client reads packs written here, so the two
// implementations have to agree byte for byte about the canonical layout: all
// X, then all Y, then all Z as little-endian float32, then intensity and
// classification as uint8, with the samples digest taken before the trailing
// newline.
//
// This test pins one small pack's exact bytes and digests. The same values are
// embedded in the Swift reader's tests
// (tools/visualiser-macos/VelocityVisualiserTests/AnnotationPackTests.swift),
// so a change to the writer fails here first and names what to update rather
// than silently invalidating every point index the client has recorded.
const (
	swiftFixturePointsBase64 = "AACAPwAAwD8AAABAAAAgQgAAgD8AAKA/AADAPwAA8MEAAAA/AABAPwAAgD8AAEBAChQeKAEBAQIAAEBAAABgQAAAAEAAACBAAADAPwAA4D8yPAEB"
	swiftFixturePointsSHA    = "sha256:f8bf067228e595dfee12c0cd3d974db11ebe5474f30e6e08fafef9da47211186"
	swiftFixtureSamplesSHA   = "sha256:38e62501b09464832baa1bcdbcc21b3f58f2b77e58327df59cbda66109654410"
	swiftFixturePackDigest   = "sha256:41b9794747f16ebe2722ea9f75a631f38e35f9670e14dccc3636ed480320d84c"
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

// writeSwiftFixturePack builds the shared fixture pack in dir.
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
	samples := []Sample{
		{SampleID: 0, SourceOrdinal: 0, SourceFrameID: 100, TimestampNs: 1_000_000_000, SensorID: "hesai-pandar40p", PointCount: 4},
		{SampleID: 1, SourceOrdinal: 1, SourceFrameID: 101, TimestampNs: 1_100_000_000, SensorID: "hesai-pandar40p", PointCount: 2},
	}
	m := Manifest{
		Source: SourceProvenance{
			VRLOGPath: "fixture.vrlog", VRLOGHeaderSHA: "sha256:aa", VRLOGFramesSHA: "sha256:bb",
			SensorID: "hesai-pandar40p", BuildVersion: "fixture",
		},
		Coordinate: CoordinateContract{
			Units: "metres", FrameID: "sensor", ReferenceFrame: "site",
			Handedness: "right", OriginNote: "sensor origin", TransformVersion: "v1",
		},
		Coverage: CoverageForegroundOnly, CoverageNote: "fixture",
		HasIntensity: true, HasClassification: true,
	}
	if err := WritePack(dir, m, samples, [][]byte{block0, block1}); err != nil {
		t.Fatalf("write fixture pack: %v", err)
	}
}

func TestSwiftFixturePackBytesAreStable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pack")
	writeSwiftFixturePack(t, dir)

	pack, err := OpenPack(dir)
	if err != nil {
		t.Fatalf("the writer produced a pack its own reader rejects: %v", err)
	}

	points, err := os.ReadFile(filepath.Join(dir, pointsFile))
	if err != nil {
		t.Fatal(err)
	}
	if got := base64.StdEncoding.EncodeToString(points); got != swiftFixturePointsBase64 {
		t.Errorf("points.bin changed.\n got: %s\nwant: %s\nUpdate AnnotationPackTests.swift's fixture to match.", got, swiftFixturePointsBase64)
	}
	if pack.Manifest.PointsSHA256 != swiftFixturePointsSHA {
		t.Errorf("points digest = %s, want %s; update AnnotationPackTests.swift", pack.Manifest.PointsSHA256, swiftFixturePointsSHA)
	}
	if pack.Manifest.SamplesSHA256 != swiftFixtureSamplesSHA {
		t.Errorf("samples digest = %s, want %s; update AnnotationPackTests.swift", pack.Manifest.SamplesSHA256, swiftFixtureSamplesSHA)
	}
	if pack.Manifest.PackDigest != swiftFixturePackDigest {
		t.Errorf("pack digest = %s, want %s; update AnnotationPackTests.swift", pack.Manifest.PackDigest, swiftFixturePackDigest)
	}

	// The layout claim the Swift decoder depends on: sample 1's block starts
	// immediately after sample 0's 4 points (4*12 + 4*2 = 56 bytes).
	if pack.Samples[1].ByteOffset != 56 {
		t.Errorf("sample 1 byte offset = %d, want 56", pack.Samples[1].ByteOffset)
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
