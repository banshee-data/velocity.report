package labelset

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

var updateFixtures = flag.Bool("update", false, "rewrite testdata/parity")

// parityDir holds what another implementation of this format (the macOS
// client) must reproduce byte for byte: the head files of a state, its
// digest, a journal of three revisions with each revision's digest, and the
// canonical encoding of awkward values.
const parityDir = "testdata/parity"

// parityStorageDir holds the fixture head in the storage layout, with
// parityStorage's chunks.
const parityStorageDir = "storage"

var parityStorage = Storage{WindowChunk: 1}

func parityJournal(t *testing.T) ([]State, [][]byte) {
	t.Helper()
	s0 := State{}
	s0.Canonicalise()
	s1 := testState()
	s2 := s1.Clone()
	s2.Masks[0].Members = Union(s2.Masks[0].Members, Ranges{{40, 2}})
	s2.Masks[1].Status = annotation.StatusRejected
	s2.Records.DismissedProposals = []string{"cluster_chain@1-ABC"}
	s2.Canonicalise()
	s3 := s2.Clone()
	s3.Masks = s3.Masks[1:]
	s3.Packets = append(s3.Packets, 0x5555)
	s3.Records.Objects[0].Notes = "deliberately separate from obj_b: \"twins\" / <tab>\t"
	s3.Canonicalise()
	states := []State{s0, s1, s2, s3}
	var lines [][]byte
	for i := 1; i < len(states); i++ {
		d, err := Diff(&states[i-1], &states[i])
		if err != nil {
			t.Fatal(err)
		}
		parent, _ := DigestOf(&states[i-1])
		state, _ := DigestOf(&states[i])
		d.Revision, d.ParentRevision, d.ParentState, d.State = i, i-1, parent, state
		d.Change = Change{Author: "dd", Session: "S1", Operation: "save_mask", CreatedUTC: fmt.Sprintf("2026-10-12T00:00:0%dZ", i)}
		line, err := canonicalLine(d)
		if err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	return states, lines
}

// parityManifest sets every labels.json field, with strings the encoders
// could disagree on.
func parityManifest() Manifest {
	return Manifest{
		SchemaVersion: SchemaVersion, LabelSetID: "dd6579b7-4705-4a95-a2a3-2a4c12bef749", Name: "kirk0 <reference> & \"twins\"",
		Layout: Pandar40P, PacketKey: PacketKeyScheme,
		AppliesTo: []Binding{
			{Name: "kirk0.pcapng", SHA256: "sha256:2864", Bytes: 200657872, UDPPort: 2369, PacketsFound: 149741,
				WindowsComplete: 832, VerifiedUTC: "2026-10-12T11:23:59Z", VerifiedBuild: "0.5.1-pre45+5d5af1fb3"},
			{Name: "é ü 東京.pcapng", SHA256: "sha256:0f68", Bytes: 1, UDPPort: 2368, PacketsMissing: 3, WindowsPartial: 1,
				WindowsMissing: 2, VerifiedUTC: "2026-10-13T00:00:00.123Z"},
		},
		Head:    Head{Revision: 2232, State: "sha256:75a2", UpdatedUTC: "2026-10-12T04:23:00.123456789Z"},
		Storage: Storage{WindowChunk: 250},
		MigratedFrom: &Migration{PackDigest: "sha256:95be", DatasetID: "ds_95be", PackDir: "ad8b9438/pack", SidecarRevision: 2232,
			PCAPSHA256: "sha256:2864", PCAPName: "kirk0.pcapng", DecoderBuild: "0.5.1-pre45", MappingDigest: "sha256:e9d9",
			Note: "tab\tnewline\n/slash"},
		Note: "a note",
	}
}

// parityManifestMinimal leaves every optional field empty.
func parityManifestMinimal() Manifest {
	return Manifest{SchemaVersion: SchemaVersion, LabelSetID: "parity", Layout: Pandar40P, PacketKey: PacketKeyScheme,
		Storage: Storage{WindowChunk: DefaultWindowChunk}}
}

func TestParityFixtures(t *testing.T) {
	states, lines := parityJournal(t)
	want := map[string][]byte{}
	head, err := EncodeState(&states[3])
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range head {
		want[name] = b
	}
	want[JournalFile] = bytes.Join(lines, nil)
	var digests bytes.Buffer
	for i, s := range states {
		d, _ := DigestOf(&s)
		fmt.Fprintf(&digests, "%d %s\n", i, d)
	}
	want["digests.txt"] = digests.Bytes()
	values, err := Canonical(map[string]any{
		"floats":  []float64{0, 1, -0.5, 0.1, 0.75, 1e-7, 1.5e-5, 123456789, 1e21, 4.2, 1.8000000000000003},
		"strings": []string{"plain", "quote\"back\\slash", "slash/ <angle> & amp", "tab\tnewline\nbell\u0007", "é ü 東京"},
		"key":     PacketKey(0x0123456789abcdef),
		"ranges":  Ranges{{0, 3}, {10, 1}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want["canonical-values.json"] = append(values, '\n')
	for name, m := range map[string]Manifest{"labels.json": parityManifest(), "labels-minimal.json": parityManifestMinimal()} {
		b, err := encodeManifest(&m)
		if err != nil {
			t.Fatal(err)
		}
		want[name] = b
	}

	// The fixture head as it is kept on disk, one window per chunk so the
	// chunk files are exercised: storage/<path>.
	stored, err := encodeStorage(&states[3], parityStorage)
	if err != nil {
		t.Fatal(err)
	}
	for name, b := range stored {
		want[path.Join(parityStorageDir, name)] = b
	}

	if *updateFixtures {
		os.RemoveAll(filepath.Join(parityDir, parityStorageDir))
		for name, b := range want {
			os.MkdirAll(filepath.Dir(filepath.Join(parityDir, name)), 0o755)
			if err := os.WriteFile(filepath.Join(parityDir, name), b, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, b := range want {
		got, err := os.ReadFile(filepath.Join(parityDir, name))
		if err != nil {
			t.Fatalf("%v (run with -update to create the fixtures)", err)
		}
		if !bytes.Equal(got, b) {
			t.Errorf("%s differs from what this build writes; run with -update only if the format change is intended", name)
		}
	}

	// The fixture journal replays to the fixture head, kept in the storage
	// layout.
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, JournalFile), want[JournalFile], 0o644)
	for name, b := range stored {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755)
		os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
	m := Manifest{SchemaVersion: SchemaVersion, LabelSetID: "parity", Layout: Pandar40P, PacketKey: PacketKeyScheme,
		Head: Head{Revision: 3}, Storage: parityStorage}
	m.Head.State, _ = DigestOf(&states[3])
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	if err := writeManifest(root, &m); err != nil {
		t.Fatal(err)
	}
	if n, err := Verify(dir); err != nil || n != 3 {
		t.Fatalf("fixture journal replays %d revisions: %v", n, err)
	}
}

// labels.json reads back to the bytes it was written as, and a new head
// changes only the head's three lines: a save is a small diff whichever
// client made the previous one.
func TestManifestReencodesByteIdentical(t *testing.T) {
	for _, m := range []Manifest{parityManifest(), parityManifestMinimal()} {
		b, err := encodeManifest(&m)
		if err != nil {
			t.Fatal(err)
		}
		var back Manifest
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatal(err)
		}
		again, err := encodeManifest(&back)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(b, again) {
			t.Fatalf("%s re-encodes differently:\n%s\n%s", m.LabelSetID, b, again)
		}
		back.Head = Head{Revision: back.Head.Revision + 1, State: "sha256:next", UpdatedUTC: "2026-10-13T00:00:00Z"}
		next, err := encodeManifest(&back)
		if err != nil {
			t.Fatal(err)
		}
		was, now := bytes.Split(b, []byte("\n")), bytes.Split(next, []byte("\n"))
		if len(was) != len(now) {
			t.Fatalf("%s: a new head changed the line count from %d to %d", m.LabelSetID, len(was), len(now))
		}
		changed := 0
		for i := range was {
			if !bytes.Equal(was[i], now[i]) {
				changed++
			}
		}
		if changed != 3 {
			t.Fatalf("%s: a new head changed %d lines, want 3", m.LabelSetID, changed)
		}
	}
}
