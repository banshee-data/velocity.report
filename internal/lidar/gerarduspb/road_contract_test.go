package gerarduspb_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/banshee-data/velocity.report/internal/lidar/gerarduspb"
	"google.golang.org/protobuf/encoding/protodelim"
)

// Decode the upstream wire fixture with the vendored bindings independently of
// the visualiser feature branch. A stub or fixture update must preserve the
// layer's payload, completion marker and explicit missing heights.
func TestRoadSurfaceVendoredContract(t *testing.T) {
	root := filepath.Join("..", "..", "..", "tools", "visualiser-macos", "VelocityVisualiserTests", "Resources", "road-surface")
	manifestBytes, err := os.ReadFile(filepath.Join(root, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Files map[string]struct {
			Bytes int    `json:"bytes"`
			SHA   string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("want both site streams, got %d", len(manifest.Files))
	}
	var firstHashes map[string]string
	for name, want := range manifest.Files {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, name))
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)
			if len(data) != want.Bytes || hex.EncodeToString(digest[:]) != want.SHA {
				t.Fatal("stream differs from the frozen manifest")
			}
			r := bytes.NewReader(data)
			hashes := map[string]string{}
			messages, missing := 0, 0
			for r.Len() > 0 {
				var message pb.StreamReferenceResponse
				if err := protodelim.UnmarshalFrom(r, &message); err != nil {
					t.Fatal(err)
				}
				messages++
				chunk := message.GetRoadSurface()
				if message.Layer != pb.ReferenceLayer_REFERENCE_LAYER_ROAD_SURFACE || chunk == nil {
					t.Fatal("layer 17 did not decode as RoadSurfaceChunk")
				}
				if messages == 1 && (chunk.GetLayer().GetVersion() != 1 || chunk.GetLayer().GetCorridorId() != "fixture") {
					t.Fatal("missing or incompatible first corridor header")
				}
				if message.LayerComplete != (r.Len() == 0) {
					t.Fatal("layer_complete must mark only the final message")
				}
				for _, patch := range chunk.Patches {
					hashes[patch.PatchId] = patch.GeometryHash + "/" + patch.SupportHash
					if len(patch.Vertices) != len(patch.Normals) || len(patch.Vertices) != 3*len(patch.Support) {
						t.Fatal("inconsistent mesh arrays")
					}
					for i, support := range patch.Support {
						if support == pb.RoadSupport_ROAD_SUPPORT_MISSING {
							missing++
							if !math.IsNaN(float64(patch.Vertices[3*i+2])) {
								t.Fatal("missing height must remain NaN on the wire")
							}
						}
					}
					for _, index := range patch.Indices {
						if int(index) >= len(patch.Support) || patch.Support[index] == pb.RoadSupport_ROAD_SUPPORT_MISSING {
							t.Fatal("triangle references an absent height")
						}
					}
				}
			}
			if messages != 2 || len(hashes) != 5 || missing == 0 {
				t.Fatalf("incomplete fixture: messages=%d patches=%d missing=%d", messages, len(hashes), missing)
			}
			if firstHashes == nil {
				firstHashes = hashes
			} else {
				for id, hash := range firstHashes {
					if hashes[id] != hash {
						t.Fatalf("patch %s changed identity between site frames", id)
					}
				}
			}
		})
	}
}
