package annotation

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validSplitManifest(packDigest string) SplitManifest {
	return SplitManifest{
		Schema: SplitSchema, SchemaVersion: SplitSchemaVersion, PackDigest: packDigest,
		Splits: []Split{
			{Name: "tune", Role: SplitRoleTuning, ObjectIDs: []string{"obj_t"}},
			{Name: "hold", Role: SplitRoleHeldOut, ObjectIDs: []string{"obj_a", "obj_b"}},
		},
		Episodes: []Episode{
			{EpisodeID: "ep_t", Split: "tune", ObjectIDs: []string{"obj_t"}, FrameIntervals: []FrameInterval{{0, 1}}},
			{EpisodeID: "ep_a", Split: "hold", ObjectIDs: []string{"obj_a"}, FrameIntervals: []FrameInterval{{0, 0}, {2, 2}}},
			{EpisodeID: "ep_b", Split: "hold", ObjectIDs: []string{"obj_b"}, FrameIntervals: []FrameInterval{{1, 2}}},
		},
	}
}

func marshalManifest(t *testing.T, m SplitManifest) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSplitManifestLoadsAndDigestsItsBytes(t *testing.T) {
	b := marshalManifest(t, validSplitManifest("sha256:abc"))
	path := filepath.Join(t.TempDir(), "split.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadSplitManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if m.Digest != sha256Hex(b) {
		t.Fatalf("digest %s, want the digest of the file bytes %s", m.Digest, sha256Hex(b))
	}
	if !m.Episodes[1].ContainsSample(2) || m.Episodes[1].ContainsSample(1) {
		t.Fatalf("episode ep_a should contain samples 0 and 2 only")
	}
}

// Every structural rule is a refusal, not a warning: a manifest that loads is
// one whose held-out claim can be checked.
func TestSplitManifestStructuralRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*SplitManifest)
		want   string
	}{
		{"wrong schema", func(m *SplitManifest) { m.Schema = "something/else" }, "schema"},
		{"wrong version", func(m *SplitManifest) { m.SchemaVersion = 2 }, "schema version"},
		{"bare digest", func(m *SplitManifest) { m.PackDigest = "abc" }, "pack_digest"},
		{"negative revision", func(m *SplitManifest) { m.SidecarRevision = -1 }, "negative"},
		{"no splits", func(m *SplitManifest) { m.Splits = nil }, "no splits"},
		{"unnamed split", func(m *SplitManifest) { m.Splits[0].Name = "" }, "no name"},
		{"duplicate split", func(m *SplitManifest) { m.Splits[1].Name = "tune" }, "declared twice"},
		{"unknown role", func(m *SplitManifest) { m.Splits[0].Role = "validation" }, "role"},
		{"empty split", func(m *SplitManifest) { m.Splits[0].ObjectIDs = nil }, "no objects"},
		{"empty object id", func(m *SplitManifest) { m.Splits[0].ObjectIDs = []string{""} }, "empty object id"},
		{"object twice in a split", func(m *SplitManifest) { m.Splits[1].ObjectIDs = []string{"obj_a", "obj_a"} }, "twice"},
		{"object in two splits", func(m *SplitManifest) {
			m.Splits[1].ObjectIDs = append(m.Splits[1].ObjectIDs, "obj_t")
		}, "object-disjoint"},
		{"no episodes", func(m *SplitManifest) { m.Episodes = nil }, "no episodes"},
		{"unnamed episode", func(m *SplitManifest) { m.Episodes[0].EpisodeID = "" }, "no id"},
		{"duplicate episode", func(m *SplitManifest) { m.Episodes[2].EpisodeID = "ep_a" }, "declared twice"},
		{"episode of unknown split", func(m *SplitManifest) { m.Episodes[0].Split = "gate" }, "unknown split"},
		{"episode scores nothing", func(m *SplitManifest) { m.Episodes[0].ObjectIDs = nil }, "scores no objects"},
		{"episode lists object twice", func(m *SplitManifest) { m.Episodes[1].ObjectIDs = []string{"obj_a", "obj_a"} }, "twice"},
		{"held-out episode scores a tuning object", func(m *SplitManifest) {
			m.Episodes[1].ObjectIDs = []string{"obj_a", "obj_t"}
		}, `scores object "obj_t" of split "tune"`},
		{"episode scores an object in no split", func(m *SplitManifest) { m.Episodes[1].ObjectIDs = []string{"obj_x"} }, "in no split"},
		{"no intervals", func(m *SplitManifest) { m.Episodes[0].FrameIntervals = nil }, "no frame intervals"},
		{"negative interval", func(m *SplitManifest) { m.Episodes[0].FrameIntervals = []FrameInterval{{-1, 2}} }, "empty or negative"},
		{"reversed interval", func(m *SplitManifest) { m.Episodes[0].FrameIntervals = []FrameInterval{{3, 2}} }, "empty or negative"},
		{"overlapping intervals", func(m *SplitManifest) {
			m.Episodes[0].FrameIntervals = []FrameInterval{{0, 2}, {2, 4}}
		}, "overlaps"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validSplitManifest("sha256:abc")
			tc.mutate(&m)
			_, err := ParseSplitManifest(marshalManifest(t, m))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}

	t.Run("unknown field", func(t *testing.T) {
		b := marshalManifest(t, validSplitManifest("sha256:abc"))
		b = append([]byte(`{"splits_v2": [],`), b[1:]...)
		if _, err := ParseSplitManifest(b); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("error = %v, want an unknown-field refusal", err)
		}
	})
	t.Run("trailing data", func(t *testing.T) {
		b := append(marshalManifest(t, validSplitManifest("sha256:abc")), []byte(" {}")...)
		if _, err := ParseSplitManifest(b); err == nil || !strings.Contains(err.Error(), "trailing") {
			t.Fatalf("error = %v, want a trailing-data refusal", err)
		}
	})
}

func TestSplitManifestBindsToOnePackAndRevision(t *testing.T) {
	p := synthPack(t)
	s := NewSidecar(p)
	for _, id := range []string{"obj_a", "obj_b", "obj_t"} {
		s.Objects = append(s.Objects, reviewedObject(id, "car"))
	}

	good := validSplitManifest(p.Manifest.PackDigest)
	if err := good.ValidateAgainst(p, s); err != nil {
		t.Fatalf("valid manifest refused: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*SplitManifest, *Sidecar)
		want   string
	}{
		{"another pack", func(m *SplitManifest, _ *Sidecar) { m.PackDigest = "sha256:0000" }, "frozen against pack"},
		{"another dataset", func(m *SplitManifest, _ *Sidecar) { m.DatasetID = "ds_other" }, "dataset"},
		{"another revision", func(m *SplitManifest, _ *Sidecar) { m.SidecarRevision = 7 }, "pins annotation revision 7"},
		{"object the annotation lost", func(_ *SplitManifest, sc *Sidecar) { sc.Objects = sc.Objects[:2] }, "does not carry"},
		{"object since rejected", func(_ *SplitManifest, sc *Sidecar) { sc.Objects[0].Status = StatusRejected }, "stale"},
		{"interval past the pack", func(m *SplitManifest, _ *Sidecar) {
			m.Episodes[2].FrameIntervals = []FrameInterval{{1, 3}}
		}, "the pack has 3 samples"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validSplitManifest(p.Manifest.PackDigest)
			sc := *s
			sc.Objects = append([]Object(nil), s.Objects...)
			tc.mutate(&m, &sc)
			if err := m.ValidateAgainst(p, &sc); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestSelectEpisodesRefusesTuningSplitsForHeldOutScoring(t *testing.T) {
	m := validSplitManifest("sha256:abc")

	got, err := m.SelectEpisodes("hold", nil, true)
	if err != nil || len(got) != 2 || got[0].EpisodeID != "ep_a" || got[1].EpisodeID != "ep_b" {
		t.Fatalf("held-out selection = %v, %v; want ep_a and ep_b in manifest order", got, err)
	}

	if _, err := m.SelectEpisodes("tune", nil, true); !errors.Is(err, ErrNotHeldOut) {
		t.Fatalf("tuning split under held-out scoring: error %v, want ErrNotHeldOut", err)
	}
	if got, err := m.SelectEpisodes("tune", nil, false); err != nil || len(got) != 1 {
		t.Fatalf("tuning split scored by request = %v, %v", got, err)
	}

	// A filter selects exactly what it names, however many episodes follow.
	if got, err := m.SelectEpisodes("hold", []string{"ep_a"}, true); err != nil || len(got) != 1 || got[0].EpisodeID != "ep_a" {
		t.Fatalf("filtered selection = %v, %v; want ep_a alone", got, err)
	}
	if _, err := m.SelectEpisodes("hold", []string{"ep_t"}, true); err == nil || !strings.Contains(err.Error(), "belongs to split") {
		t.Fatalf("an episode of another split: error %v", err)
	}
	if _, err := m.SelectEpisodes("hold", []string{"ep_z"}, true); err == nil || !strings.Contains(err.Error(), "no episode") {
		t.Fatalf("unknown episode: error %v", err)
	}
	if _, err := m.SelectEpisodes("gate", nil, false); err == nil {
		t.Fatal("an unknown split was selected")
	}
}
