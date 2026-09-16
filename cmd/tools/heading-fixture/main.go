// heading-fixture exports a small recorded-output regression case, not pose truth.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type sample struct {
	Track        string  `json:"track"`
	Source       int     `json:"source"`
	State        uint8   `json:"state"`
	Observations int     `json:"observations"`
	Speed        float32 `json:"speed"`
	Course       float32 `json:"course"`
	Heading      float32 `json:"heading"`
	Length       float32 `json:"length"`
	Width        float32 `json:"width"`
	X            float32 `json:"x"`
	Y            float32 `json:"y"`
}
type frame struct {
	ID        uint64   `json:"id"`
	Timestamp int64    `json:"timestamp_ns"`
	Samples   []sample `json:"samples"`
}
type fixture struct {
	Schema         int               `json:"schema"`
	Run            string            `json:"run"`
	HeaderHash     string            `json:"header_sha256"`
	ConfigHash     string            `json:"execution_config_sha256"`
	PCAP           string            `json:"pcap_basename"`
	PCAPHash       string            `json:"pcap_sha256"`
	IdentityStatus string            `json:"identity_status"`
	PoseTruth      bool              `json:"pose_truth"`
	FirstFrame     uint64            `json:"first_frame"`
	FrameCount     uint64            `json:"frame_count"`
	Tracks         map[string]string `json:"tracks"`
	Frames         []frame           `json:"frames"`
}

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func run() error {
	vrlog := flag.String("vrlog", "", "Source VRLOG directory")
	pcap := flag.String("pcap", "", "Source PCAP (read and hashed, never copied)")
	start := flag.Uint64("first-frame", 1000, "First recorded frame ID")
	count := flag.Uint64("frames", 200, "Number of consecutive recorded frames")
	prefixes := flag.String("tracks", "trk_18952226,trk_04e4ebd5", "Unique track prefixes, comma separated")
	flag.Parse()
	if *vrlog == "" || *pcap == "" || *count == 0 || *count > 10000 {
		return fmt.Errorf("require vrlog, pcap, and 1..10000 frames")
	}
	f := fixture{Schema: 1, Run: filepath.Base(*vrlog), PCAP: filepath.Base(*pcap), IdentityStatus: "same-object reported by user; not independently adjudicated", FirstFrame: *start, FrameCount: *count, Tracks: map[string]string{}}
	var err error
	if f.HeaderHash, err = hashFile(filepath.Join(*vrlog, "header.json")); err != nil {
		return err
	}
	if f.ConfigHash, err = hashFile(filepath.Join(*vrlog, "execution_config.json")); err != nil {
		return err
	}
	if f.PCAPHash, err = hashFile(*pcap); err != nil {
		return err
	}
	r, err := recorder.NewReplayer(*vrlog)
	if err != nil {
		return err
	}
	defer r.Close()
	for {
		b, err := r.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if b.FrameID < *start || b.FrameID-*start >= *count {
			continue
		}
		row := frame{ID: b.FrameID, Timestamp: b.TimestampNanos, Samples: []sample{}}
		if b.Tracks != nil {
			for _, tr := range b.Tracks.Tracks {
				for _, prefix := range strings.Split(*prefixes, ",") {
					if !strings.HasPrefix(tr.TrackID, prefix) {
						continue
					}
					if previous := f.Tracks[prefix]; previous != "" && previous != tr.TrackID {
						return fmt.Errorf("ambiguous prefix %s", prefix)
					}
					f.Tracks[prefix] = tr.TrackID
					row.Samples = append(row.Samples, sample{prefix, tr.HeadingSource, uint8(tr.State), tr.ObservationCount, tr.SpeedMps, tr.HeadingRad, tr.BBoxHeadingRad, tr.BBoxLength, tr.BBoxWidth, tr.X, tr.Y})
				}
			}
		}
		sort.Slice(row.Samples, func(i, j int) bool { return row.Samples[i].Track < row.Samples[j].Track })
		f.Frames = append(f.Frames, row)
	}
	if uint64(len(f.Frames)) != *count || len(f.Tracks) != len(strings.Split(*prefixes, ",")) {
		return fmt.Errorf("incomplete case: %d frames, %d tracks", len(f.Frames), len(f.Tracks))
	}
	return json.NewEncoder(os.Stdout).Encode(f)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
