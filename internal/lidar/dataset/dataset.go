// Package dataset writes a clip's extract into the published survey dataset:
// the names its files take, the sidecar beside an extract, the manifest entry
// and the site-index entry the scene publisher reads.
//
// The shapes follow tools/s2-archive: huggingface-dataset-layout.md for names
// and the manifest, export-static-pcaps.py for the sidecar and
// build-site-index.py for the site index. Field order matches those writers,
// so an entry this package adds reads like the ones beside it.
package dataset

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/geoindex"
)

// Timezone is the dataset's clock for names and timestamps.
const Timezone = "America/Los_Angeles"

// ExportPolicy names how a clip's extract chose its packets: the whole
// rotations between the frame boundaries that bound the clip's window.
const ExportPolicy = "clip_frame_bounds"

// Origin marks a site-index entry that came from a clip, so a rebuild of the
// index from the s2 archive keeps it.
const Origin = "clip"

func location() (*time.Location, error) {
	loc, err := time.LoadLocation(Timezone)
	if err != nil {
		return nil, fmt.Errorf("dataset: load %s: %w", Timezone, err)
	}
	return loc, nil
}

// ISOTime formats a capture time as Python's isoformat does in the dataset's
// timezone, with microseconds.
func ISOTime(t time.Time) (string, error) {
	loc, err := location()
	if err != nil {
		return "", err
	}
	return t.In(loc).Format("2006-01-02T15:04:05.000000-07:00"), nil
}

// MinuteStamp is a capture time as the YYYYMMDDTHHMM that leads every
// dataset file name, in the dataset timezone.
func MinuteStamp(t time.Time) (string, error) {
	loc, err := location()
	if err != nil {
		return "", err
	}
	return t.In(loc).Format("20060102T1504"), nil
}

// Stem is the name every file of a published capture shares:
// <YYYYMMDDTHHMM>_<S2 L13 display>_<id>, the minute in the dataset timezone.
func Stem(start time.Time, l13Token, id string) (string, error) {
	if l13Token == "" || id == "" {
		return "", errors.New("dataset: a stem needs an L13 cell and an id")
	}
	stamp, err := MinuteStamp(start)
	if err != nil {
		return "", err
	}
	return stamp + "_" + geoindex.FamilyDisplay(l13Token) + "_" + id, nil
}

// CaptureID is the stem with the canonical, hyphen-free L13 token, as the
// manifest records it.
func CaptureID(start time.Time, l13Token, id string) (string, error) {
	stem, err := Stem(start, l13Token, id)
	if err != nil {
		return "", err
	}
	return strings.Replace(stem, geoindex.FamilyDisplay(l13Token), l13Token, 1), nil
}

// SourceFile is one source capture as the sidecar and manifest name it.
type SourceFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Sidecar is the JSON written beside an extract. Its first fields are
// export-static-pcaps.py's schema 1, so the dataset scripts read it.
type Sidecar struct {
	SchemaVersion         int          `json:"schema_version"`
	SiteID                string       `json:"site_id"`
	Where                 string       `json:"where"`
	StaticExportPolicy    string       `json:"static_export_policy"`
	Start                 string       `json:"start"`
	End                   string       `json:"end"`
	OperatorOverrides     []any        `json:"operator_overrides"`
	SourceFiles           []SourceFile `json:"source_files"`
	OutputFile            string       `json:"output_file"`
	OutputSHA256          string       `json:"output_sha256"`
	OutputBytes           int64        `json:"output_bytes"`
	Latitude              *float64     `json:"latitude"`
	Longitude             *float64     `json:"longitude"`
	PCAPSplitBuildVersion string       `json:"pcap_split_build_version"`
	ClipID                string       `json:"clip_id"`
	RubricVersion         int          `json:"rubric_version"`
	FrameCount            int64        `json:"frame_count"`
	PacketCount           int64        `json:"packet_count"`
	ClosedBy              string       `json:"closed_by"`
	WindowStart           string       `json:"window_start"`
	WindowEnd             string       `json:"window_end,omitempty"`
}

// ExportProvenance is the manifest's record of how a raw file was made.
type ExportProvenance struct {
	StaticExportPolicy string       `json:"static_export_policy"`
	OperatorOverrides  []any        `json:"operator_overrides"`
	SourceFiles        []SourceFile `json:"source_files"`
	ClipID             string       `json:"clip_id,omitempty"`
	RubricVersion      int          `json:"rubric_version,omitempty"`
}

// ManifestEntry is one capture in manifest.json, in stage-huggingface-dataset's
// field order.
type ManifestEntry struct {
	CaptureID         string           `json:"capture_id"`
	Timestamp         string           `json:"timestamp"`
	FilenameTimestamp string           `json:"filename_timestamp"`
	Timezone          string           `json:"timezone"`
	Latitude          float64          `json:"latitude"`
	Longitude         float64          `json:"longitude"`
	S2L10             string           `json:"s2_l10"`
	S2L13             string           `json:"s2_l13"`
	SiteSlug          string           `json:"site_slug"`
	SensorType        string           `json:"sensor_type"`
	SensorModel       *string          `json:"sensor_model"`
	CaptureType       string           `json:"capture_type"`
	DurationSeconds   float64          `json:"duration_seconds"`
	RawPath           string           `json:"raw_path"`
	ScenePath         *string          `json:"scene_path"`
	TracksPath        *string          `json:"tracks_path"`
	MapPath           *string          `json:"map_path"`
	MetricsPath       *string          `json:"metrics_path"`
	ReportPath        *string          `json:"report_path"`
	PipelineVersion   string           `json:"pipeline_version"`
	PipelineGitSHA    *string          `json:"pipeline_git_sha"`
	RawSHA256         string           `json:"raw_sha256"`
	RawBytes          int64            `json:"raw_bytes"`
	ExportProvenance  ExportProvenance `json:"export_provenance"`
}

// StaticPart is a site-index static stretch.
type StaticPart struct {
	Start    string   `json:"start"`
	End      string   `json:"end"`
	Captures []string `json:"captures"`
}

// SiteIndexEntry is one survey in site-index.json, in build-site-index.py's
// field order, plus the clip it came from.
type SiteIndexEntry struct {
	ID                    string       `json:"id"`
	Site                  string       `json:"site"`
	Day                   string       `json:"day"`
	Start                 string       `json:"start"`
	End                   string       `json:"end"`
	Clock                 string       `json:"clock"`
	Minutes               float64      `json:"minutes"`
	Fragments             int          `json:"fragments"`
	StaticParts           []StaticPart `json:"static_parts"`
	StaticExportPolicy    string       `json:"static_export_policy"`
	StaticExportOverrides []any        `json:"static_export_overrides"`
	Captures              []string     `json:"captures"`
	MapMark               *string      `json:"map_mark"`
	Where                 string       `json:"where"`
	Lat                   float64      `json:"lat"`
	Lon                   float64      `json:"lon"`
	PositionConfidence    string       `json:"position_confidence"`
	PCAPSplitBuildVersion string       `json:"pcap_split_build_version"`
	GridAzimuthDeg        *float64     `json:"grid_azimuth_deg"`
	NorthAzimuthDeg       *float64     `json:"north_azimuth_deg"`
	PublishedAs           string       `json:"published_as"`
	Origin                string       `json:"origin"`
	ClipID                string       `json:"clip_id"`
}

// Survey is what both entries are built from.
type Survey struct {
	ID              string
	ClipID          string
	Where           string
	Lat, Lon        float64
	L10, L13        string
	First, Last     time.Time
	NorthAzimuthDeg *float64
	SourceFiles     []SourceFile
	RawPath         string
	RawSHA256       string
	RawBytes        int64
	BuildVersion    string
	RubricVersion   int
}

// Manifest builds the survey's manifest entry.
func (s Survey) Manifest() (ManifestEntry, error) {
	stem, err := Stem(s.First, s.L13, s.ID)
	if err != nil {
		return ManifestEntry{}, err
	}
	captureID, err := CaptureID(s.First, s.L13, s.ID)
	if err != nil {
		return ManifestEntry{}, err
	}
	ts, err := ISOTime(s.First)
	if err != nil {
		return ManifestEntry{}, err
	}
	sources := s.SourceFiles
	if sources == nil {
		sources = []SourceFile{}
	}
	return ManifestEntry{
		CaptureID: captureID, Timestamp: ts, FilenameTimestamp: stem[:13], Timezone: Timezone,
		Latitude: s.Lat, Longitude: s.Lon, S2L10: s.L10, S2L13: s.L13, SiteSlug: s.ID,
		SensorType: "lidar", CaptureType: "static",
		DurationSeconds: s.Last.Sub(s.First).Seconds(),
		RawPath:         s.RawPath, PipelineVersion: s.BuildVersion,
		RawSHA256: s.RawSHA256, RawBytes: s.RawBytes,
		ExportProvenance: ExportProvenance{
			StaticExportPolicy: ExportPolicy, OperatorOverrides: []any{},
			SourceFiles: sources, ClipID: s.ClipID, RubricVersion: s.RubricVersion,
		},
	}, nil
}

// SiteIndex builds the survey's site-index entry.
func (s Survey) SiteIndex() (SiteIndexEntry, error) {
	start, err := ISOTime(s.First)
	if err != nil {
		return SiteIndexEntry{}, err
	}
	end, err := ISOTime(s.Last)
	if err != nil {
		return SiteIndexEntry{}, err
	}
	loc, _ := location()
	local := s.First.In(loc)
	names := make([]string, 0, len(s.SourceFiles))
	for _, f := range s.SourceFiles {
		names = append(names, f.Name)
	}
	minutes := s.Last.Sub(s.First).Minutes()
	return SiteIndexEntry{
		ID: s.ID, Site: s.ID, Day: local.Format("2006-01-02"), Start: start, End: end,
		Clock:   strings.TrimPrefix(local.Format("03:04"), "0"),
		Minutes: float64(int(minutes*10+0.5)) / 10, Fragments: 1,
		StaticParts:        []StaticPart{{Start: start, End: end, Captures: names}},
		StaticExportPolicy: ExportPolicy, StaticExportOverrides: []any{},
		Captures: names, Where: s.Where, Lat: s.Lat, Lon: s.Lon,
		PositionConfidence: "operator", PCAPSplitBuildVersion: s.BuildVersion,
		NorthAzimuthDeg: s.NorthAzimuthDeg, PublishedAs: s.ID, Origin: Origin, ClipID: s.ClipID,
	}, nil
}

// ErrSlugTaken means the manifest already publishes another capture under the
// survey's id.
var ErrSlugTaken = errors.New("dataset: the manifest already has another capture for this site slug")

// encode writes v as the dataset's scripts do: two-space indent, no HTML
// escaping, a trailing newline.
func encode(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// EncodeJSON is encode for callers outside the package.
func EncodeJSON(v any) ([]byte, error) { return encode(v) }

// UpsertManifest adds entry to the manifest at path, replacing an earlier
// entry made from the same clip. Every other entry is kept byte for byte in
// its original field order. An entry for the same site slug from anywhere
// else is refused, because the scene publisher keys captures by slug.
func UpsertManifest(path string, entry ManifestEntry) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("dataset: read manifest: %w", err)
	}
	var entries []json.RawMessage
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &entries); err != nil {
			return fmt.Errorf("dataset: %s is not a JSON array: %w", path, err)
		}
	}
	kept := make([]json.RawMessage, 0, len(entries)+1)
	for _, raw := range entries {
		var head struct {
			CaptureID        string `json:"capture_id"`
			SiteSlug         string `json:"site_slug"`
			SensorType       string `json:"sensor_type"`
			ExportProvenance struct {
				ClipID string `json:"clip_id"`
			} `json:"export_provenance"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			return fmt.Errorf("dataset: manifest entry: %w", err)
		}
		if head.ExportProvenance.ClipID != "" && head.ExportProvenance.ClipID == entry.ExportProvenance.ClipID {
			continue
		}
		if head.CaptureID == entry.CaptureID ||
			(head.SensorType == "lidar" && head.SiteSlug == entry.SiteSlug) {
			return fmt.Errorf("%w: %s", ErrSlugTaken, entry.SiteSlug)
		}
		kept = append(kept, raw)
	}
	added, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	kept = append(kept, added)
	out, err := encode(kept)
	if err != nil {
		return err
	}
	return writeAtomic(path, out)
}

func writeAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".")
	if err != nil {
		return fmt.Errorf("dataset: %w", err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// WriteJSONFile writes v to path atomically in the dataset's JSON style.
func WriteJSONFile(path string, v any) error {
	data, err := encode(v)
	if err != nil {
		return err
	}
	return writeAtomic(path, data)
}
