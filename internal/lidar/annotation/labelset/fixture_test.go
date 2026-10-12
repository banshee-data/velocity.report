package labelset

import (
	"bytes"
	"flag"
	"fmt"
	"os"
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

	if *updateFixtures {
		os.MkdirAll(parityDir, 0o755)
		for name, b := range want {
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

	// The fixture journal replays to the fixture head.
	dir := t.TempDir()
	for name, b := range want {
		os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
	m := Manifest{SchemaVersion: SchemaVersion, LabelSetID: "parity", Layout: Pandar40P, PacketKey: PacketKeyScheme, Head: Head{Revision: 3}}
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
