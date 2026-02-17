package l3grid

import (
"bytes"
"compress/gzip"
"encoding/gob"
"encoding/json"
"os"
"strings"
"testing"
)

// TestExportBgSnapshotToASC tests that export functions work correctly.
// Note: Since security fixes now generate export paths internally (not from user input),
// the test verifies that ExportPointsToASC and ExportBgSnapshotToASC return the actual
// path used and that the exported file can be read.
func TestExportBgSnapshotToASC(t *testing.T) {
// Build a small grid: 2 rings x 4 azbins
rings := 2
azimuthBins := 4
cells := make([]BackgroundCell, rings*azimuthBins)
// Set one cell to a non-zero average range on ring 1, azbin 1
cells[azimuthBins+1].AverageRangeMeters = 5.0
// Serialize cells into grid blob
var buf []byte
{ // gob+gzip into bytes.Buffer
var b bytes.Buffer
gw := gzip.NewWriter(&b)
enc := gob.NewEncoder(gw)
if err := enc.Encode(cells); err != nil {
t.Fatalf("encode: %v", err)
}
if err := gw.Close(); err != nil {
t.Fatalf("gzip close: %v", err)
}
buf = b.Bytes()
}

snap := &BgSnapshot{
SensorID:       "test-sensor",
TakenUnixNanos: 12345,
Rings:          rings,
AzimuthBins:    azimuthBins,
GridBlob:       buf,
}

// Register a live BackgroundManager with ring elevations
liveMgr := NewBackgroundManager("test-sensor", rings, azimuthBins, BackgroundParams{}, nil)
// Set ring elevations on live manager
elevs := []float64{-1.0, 1.0}
if err := liveMgr.SetRingElevations(elevs); err != nil {
t.Fatalf("SetRingElevations: %v", err)
}
RegisterBackgroundManager("test-sensor", liveMgr)
defer func() {
	// Unregister by registering nil
	RegisterBackgroundManager("test-sensor", nil)
}()

// Export the snapshot
actualPath, err := ExportBgSnapshotToASC(snap, nil)
if err != nil {
t.Fatalf("ExportBgSnapshotToASC: %v", err)
}
defer func() {
if err := os.Remove(actualPath); err != nil {
t.Logf("warning: failed to remove test export file %s: %v", actualPath, err)
}
}()

// Verify the file exists and contains expected data
content, err := os.ReadFile(actualPath)
if err != nil {
t.Fatalf("ReadFile: %v", err)
}
// Verify it's ASCII format with some points
if len(content) < 10 {
t.Errorf("exported file is too small: %d bytes", len(content))
}
if !strings.Contains(string(content), "#") {
t.Error("exported file should contain comment lines starting with #")
}
t.Logf("Exported %d bytes to %s", len(content), actualPath)
}

func TestExportBgSnapshotToASC_NilSnapshot(t *testing.T) {
_, err := ExportBgSnapshotToASC(nil, nil)
if err == nil {
t.Error("expected error for nil snapshot")
}
if !strings.Contains(err.Error(), "nil snapshot") {
t.Errorf("unexpected error: %v", err)
}
}

func TestExportBgSnapshotToASC_InvalidGridBlob(t *testing.T) {
snap := &BgSnapshot{
SensorID:    "test-sensor",
Rings:       2,
AzimuthBins: 4,
GridBlob:    []byte("not valid gzipped gob data"),
}
_, err := ExportBgSnapshotToASC(snap, nil)
if err == nil {
t.Error("expected error for invalid grid blob")
}
}

func TestExportBgSnapshotToASC_WithCallerElevations(t *testing.T) {
rings := 2
azimuthBins := 4
cells := make([]BackgroundCell, rings*azimuthBins)
cells[1].AverageRangeMeters = 10.0

var buf []byte
{
var b bytes.Buffer
gw := gzip.NewWriter(&b)
enc := gob.NewEncoder(gw)
if err := enc.Encode(cells); err != nil {
t.Fatalf("encode: %v", err)
}
gw.Close()
buf = b.Bytes()
}

snap := &BgSnapshot{
SensorID:    "test-caller-elevs",
Rings:       rings,
AzimuthBins: azimuthBins,
GridBlob:    buf,
}

elevs := []float64{-2.0, 2.0}
actualPath, err := ExportBgSnapshotToASC(snap, elevs)
if err != nil {
t.Fatalf("ExportBgSnapshotToASC: %v", err)
}
defer os.Remove(actualPath)

content, err := os.ReadFile(actualPath)
if err != nil {
t.Fatalf("ReadFile: %v", err)
}
if len(content) < 10 {
t.Errorf("exported file too small")
}
t.Logf("Exported %d bytes with caller elevations", len(content))
}

func TestExportBgSnapshotToASC_WithEmbeddedElevations(t *testing.T) {
rings := 2
azimuthBins := 4
cells := make([]BackgroundCell, rings*azimuthBins)
cells[2].AverageRangeMeters = 7.5

var buf []byte
{
var b bytes.Buffer
gw := gzip.NewWriter(&b)
enc := gob.NewEncoder(gw)
if err := enc.Encode(cells); err != nil {
t.Fatalf("encode: %v", err)
}
gw.Close()
buf = b.Bytes()
}

elevs := []float64{-3.0, 3.0}
elevsJSON, _ := json.Marshal(elevs)

snap := &BgSnapshot{
SensorID:           "test-embedded-elevs",
Rings:              rings,
AzimuthBins:        azimuthBins,
GridBlob:           buf,
RingElevationsJSON: string(elevsJSON),
}

actualPath, err := ExportBgSnapshotToASC(snap, nil)
if err != nil {
t.Fatalf("ExportBgSnapshotToASC: %v", err)
}
defer os.Remove(actualPath)

content, err := os.ReadFile(actualPath)
if err != nil {
t.Fatalf("ReadFile: %v", err)
}
if len(content) < 10 {
t.Errorf("exported file too small")
}
t.Logf("Exported %d bytes with embedded elevations", len(content))
}

func TestExportBgSnapshotToASC_WithInvalidElevationsJSON(t *testing.T) {
rings := 2
azimuthBins := 4
cells := make([]BackgroundCell, rings*azimuthBins)
// Add at least one cell with data so export doesn't fail with "no points"
cells[0].AverageRangeMeters = 5.0

var buf []byte
{
var b bytes.Buffer
gw := gzip.NewWriter(&b)
enc := gob.NewEncoder(gw)
if err := enc.Encode(cells); err != nil {
t.Fatalf("encode: %v", err)
}
gw.Close()
buf = b.Bytes()
}

snap := &BgSnapshot{
SensorID:           "test-invalid-json",
Rings:              rings,
AzimuthBins:        azimuthBins,
GridBlob:           buf,
RingElevationsJSON: "{ invalid json",
}

// Should still export successfully (will use defaults or zero elevations)
actualPath, err := ExportBgSnapshotToASC(snap, nil)
if err != nil {
t.Fatalf("ExportBgSnapshotToASC should not fail with invalid JSON: %v", err)
}
defer os.Remove(actualPath)
}

func TestExportBgSnapshotToASC_WithWrongElevationCount(t *testing.T) {
rings := 2
azimuthBins := 4
cells := make([]BackgroundCell, rings*azimuthBins)
// Add at least one cell with data so export doesn't fail with "no points"
cells[0].AverageRangeMeters = 5.0

var buf []byte
{
var b bytes.Buffer
gw := gzip.NewWriter(&b)
enc := gob.NewEncoder(gw)
if err := enc.Encode(cells); err != nil {
t.Fatalf("encode: %v", err)
}
gw.Close()
buf = b.Bytes()
}

// Provide wrong number of elevations
wrongElevs := []float64{-1.0, 0.0, 1.0} // 3 elevations for 2 rings
elevsJSON, _ := json.Marshal(wrongElevs)

snap := &BgSnapshot{
SensorID:           "test-wrong-count",
Rings:              rings,
AzimuthBins:        azimuthBins,
GridBlob:           buf,
RingElevationsJSON: string(elevsJSON),
}

// Should still export (will try to use defaults or skip invalid elevations)
actualPath, err := ExportBgSnapshotToASC(snap, nil)
if err != nil {
t.Fatalf("ExportBgSnapshotToASC: %v", err)
}
defer os.Remove(actualPath)
}
