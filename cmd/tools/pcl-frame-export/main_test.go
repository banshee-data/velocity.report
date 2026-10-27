package main

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

func TestFramePreservesNearestIdentityAndMissingCells(t *testing.T) {
	points := []l2frames.Point{{X: 9, Distance: 9, Channel: 1, Azimuth: 0}, {X: 2, Z: 1, Distance: 3, Channel: 1, Azimuth: 360}, {X: 4, Distance: 4, Channel: 40, Azimuth: -1}, {Distance: 1, Channel: 41}}
	var out bytes.Buffer
	if err := writeFrame(&out, points, 2); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.Split(out.String(), "DATA ascii\n")[1], "\n")
	if len(lines) != 81 {
		t.Fatalf("rows: %d", len(lines)-1)
	}
	if lines[0] != "2 0 1 3 1 360 0 1" {
		t.Fatalf("nearest return identity: %s", lines[0])
	}
	if lines[1] != "nan nan nan nan 1 nan nan -1" {
		t.Fatalf("missing cell: %s", lines[1])
	}
	if lines[79] != "4 0 0 4 40 -1 0 2" {
		t.Fatalf("wrap/channel: %s", lines[79])
	}
	if err := writeFrame(&out, points, 0); err == nil {
		t.Fatal("accepted zero bins")
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestFramePropagatesOutputFailure(t *testing.T) {
	if err := writeFrame(failedWriter{}, nil, 1); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write error = %v", err)
	}
}
