package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/pipeline"
)

func TestJoinRecordWriterWritesOneLinePerRecord(t *testing.T) {
	var buf bytes.Buffer
	w := newJoinRecordWriter(&buf)
	w.write([]pipeline.FragmentJoinRecord{{FrameID: "a", ClusterID: 3, OwnerTrackID: "t1", Tracks: 1}, {FrameID: "a", ClusterID: 7, Tracks: 2}})
	w.write(nil)
	w.write([]pipeline.FragmentJoinRecord{{FrameID: "b", ClusterID: 1}})
	if err := w.close(); err != nil {
		t.Fatal(err)
	}
	var got []pipeline.FragmentJoinRecord
	sc := bufio.NewScanner(&buf)
	for sc.Scan() {
		var r pipeline.FragmentJoinRecord
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatalf("line %q: %v", sc.Text(), err)
		}
		got = append(got, r)
	}
	if len(got) != 3 || got[1].Tracks != 2 || got[2].FrameID != "b" || w.count != 3 {
		t.Fatalf("records: %+v", got)
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// A record that cannot be encoded, or a write that fails, is reported at
// close, so a partial file is not read as complete.
func TestJoinRecordWriterReportsTheFirstError(t *testing.T) {
	w := newJoinRecordWriter(&bytes.Buffer{})
	w.write([]pipeline.FragmentJoinRecord{{ClusterID: 1}, {LengthMetres: math.NaN()}, {ClusterID: 3}})
	if err := w.close(); err == nil || !strings.Contains(err.Error(), "record 2") || w.count != 1 {
		t.Fatalf("encode error: %v, %d written", err, w.count)
	}
	f := newJoinRecordWriter(failingWriter{})
	f.write([]pipeline.FragmentJoinRecord{{ClusterID: 1}})
	if err := f.close(); err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("flush error: %v", err)
	}
}
