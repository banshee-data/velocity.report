package annotation

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"testing"
)

// The macOS window runs these rules in its own port (LabelChecks.swift), and
// LabelChecksTests.swift reads this fixture: the input and the warnings Go
// gives for it. A change to the rules fails here first; rerun with
// -update-label-checks-fixture, and the Swift tests say whether the port
// still agrees.

var updateLabelChecksFixture = flag.Bool("update-label-checks-fixture", false,
	"rewrite testdata/label_checks_fixture.json from the current rules")

const labelChecksFixture = "testdata/label_checks_fixture.json"

type labelChecksFixtureFile struct {
	Input    CheckInput     `json:"input"`
	Warnings []LabelWarning `json:"warnings"`
}

// labelChecksInput trips every rule once, beside an object that trips none:
//
//   - car_a moves 1 m a frame (10 m/s), then has no mask for 12 frames: gap.
//   - ped_b moves 2 m in a frame (20 m/s) and has a 2-return mask: speed, tiny.
//   - car_c is seen in one frame only: single-frame.
//   - car_d and car_e sit on one spot for three frames, sharing returns in
//     the first: twin, shared-return.
//   - car_f moves 1 m a frame for four frames: nothing.
func labelChecksInput() CheckInput {
	in := CheckInput{}
	for _, o := range [][2]string{{"car_a", "car"}, {"ped_b", "pedestrian"}, {"car_c", "car"}, {"car_d", "car"}, {"car_e", "car"}, {"car_f", "car"}} {
		in.Objects = append(in.Objects, Object{ObjectID: o[0], Class: o[1], Confidence: 1, Status: StatusProposed})
	}
	// Each sample holds a block of 10 points per object, at the object's
	// position in that sample: indices 10k..10k+9 belong to object k.
	at := map[string]map[int][2]float32{
		"car_a": {0: {0, 0}, 1: {1, 0}, 2: {2, 0}, 15: {15, 0}},
		"ped_b": {0: {0, 10}, 1: {2, 10}},
		"car_c": {5: {30, 30}},
		"car_d": {3: {50, 50}, 4: {50, 50}, 5: {50, 50}},
		"car_e": {3: {50.1, 50}, 4: {50.1, 50}, 5: {50.1, 50}},
		"car_f": {6: {0, -20}, 7: {1, -20}, 8: {2, -20}, 9: {3, -20}},
	}
	for sample := 0; sample < 16; sample++ {
		s := CheckSample{SampleID: 100 + sample, TimestampNs: 1_000_000_000 + int64(sample)*100_000_000}
		for k, o := range in.Objects {
			pos, ok := at[o.ObjectID][sample]
			if !ok {
				pos = [2]float32{-1000, -1000}
			}
			for j := 0; j < 10; j++ {
				s.X = append(s.X, pos[0]+float32(j)*0.01)
				s.Y = append(s.Y, pos[1])
			}
			if !ok {
				continue
			}
			indices := []int{}
			for j := 0; j < 10; j++ {
				indices = append(indices, 10*k+j)
			}
			switch {
			case o.ObjectID == "ped_b" && sample == 1:
				indices = indices[:2]
			case o.ObjectID == "car_e" && sample == 3:
				indices = append(indices, 30, 31) // two of car_d's returns
			}
			in.Masks = append(in.Masks, FrameMask{ObjectID: o.ObjectID, SampleID: s.SampleID, PointIndices: indices,
				Completeness: MaskPartial, Visibility: VisiblePresent, Status: StatusProposed})
		}
		in.Samples = append(in.Samples, s)
	}
	return in
}

func TestLabelChecksFixture(t *testing.T) {
	in := labelChecksInput()
	got := RunLabelChecks(in)
	rules := map[string]int{}
	for _, w := range got {
		rules[w.Rule]++
		if w.ObjectID == "car_f" || w.OtherID == "car_f" {
			t.Errorf("a clean object was warned about: %+v", w)
		}
	}
	for _, rule := range []string{CheckSingleFrame, CheckGap, CheckImplausibleSpeed, CheckSharedReturn, CheckTinyMask, CheckTwin} {
		if rules[rule] != 1 {
			t.Errorf("rule %s warned %d times, want once: %+v", rule, rules[rule], got)
		}
	}
	file := labelChecksFixtureFile{Input: in, Warnings: got}
	want, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	want = append(want, '\n')
	if *updateLabelChecksFixture {
		if err := os.WriteFile(labelChecksFixture, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	have, err := os.ReadFile(labelChecksFixture)
	if err != nil {
		t.Fatalf("%v (write it with -update-label-checks-fixture)", err)
	}
	if !bytes.Equal(have, want) {
		t.Fatalf("%s is not what the rules give: rerun with -update-label-checks-fixture, then the Swift tests", labelChecksFixture)
	}
}
