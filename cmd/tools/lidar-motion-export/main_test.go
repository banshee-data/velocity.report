package main

import (
	"strings"
	"testing"
)

func TestParseRequiresInputsAndKeepsCaptureOrder(t *testing.T) {
	if _, err := parseFlags([]string{"-pcap", "a.pcap"}); err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing flags accepted: %v", err)
	}
	o, err := parseFlags([]string{"-pcap", "b.pcap", "-pcap", "a.pcap", "-out-dir", "o"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(o.pcaps, ",") != "b.pcap,a.pcap" {
		t.Errorf("capture order %v", o.pcaps)
	}
	if o.timeSource != "azimuth_clock_v1" || o.splitS != 60 || o.stride != 1 {
		t.Errorf("defaults %+v", o)
	}
}

func TestOptionalValues(t *testing.T) {
	if v, err := optionalFloat("h", ""); v != nil || err != nil {
		t.Errorf("empty float: %v %v", v, err)
	}
	if v, err := optionalFloat("h", "1.85"); err != nil || *v != 1.85 {
		t.Errorf("float: %v %v", v, err)
	}
	if _, err := optionalFloat("h", "tall"); err == nil {
		t.Error("bad float accepted")
	}
	if _, err := optionalTime("from", "2026-09-02 11:33"); err == nil {
		t.Error("non-RFC 3339 time accepted")
	}
	if tm, err := optionalTime("from", "2026-09-02T11:33:00-07:00"); err != nil || tm.UTC().Hour() != 18 {
		t.Errorf("time: %v %v", tm, err)
	}
}
