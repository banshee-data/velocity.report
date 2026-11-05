// Command lidar-pack-rebuild rebuilds a version 2 annotation pack's binary
// files from the capture it was cut from and the text files of its label set,
// for a host that can read the dataset's text files but not its binaries.
//
// A cloud container reaches the sf-street-speeds dataset's JSON and NDJSON
// files on huggingface.co but not the CDN that serves its .bin files, so the
// reviewed kirk0 labels could not be scored there. Every binary is derived
// data:
//
//   - the label set's packets.bin is the capture's packet keys in file order,
//     and Open checks the state it then reads against the label set's head
//     digest;
//   - the pack's packets.bin is the same table, and returns.bin is each
//     window's domain returns (offsets from the window's start) in order:
//     both are checked against the pack manifest's digests and the run fails
//     on a mismatch;
//   - points.bin is each return decoded by this build's parser. Its digest
//     need not equal the published one, and on kirk0 it does not; the
//     manifest records the new digest and pack digest, and its coverage note
//     says the pack is reconstructed. Backgrounds are not rebuilt and are
//     dropped from the manifest.
//
// The result scores like the published pack wherever the labels, not the
// exact coordinates, decide a figure; a report must say it used a
// reconstructed pack and how it checked that.
//
//	lidar-pack-rebuild -pcap kirk0.pcapng -labels DIR/kirk0-reference -pack DIR/<pack>-v2
//
// -labels is the label set directory holding labels.json, windows.json and
// domain-returns/; -pack holds the pack's manifest.json and samples.json.
// Both are written in place: packets.bin into each, and returns.bin and
// points.bin into the pack, with its manifest rewritten.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation/labelset"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// perPacket is the Pandar40P's return layout: 10 blocks of 40 channels.
const (
	blocks    = 10
	channels  = 40
	perPacket = blocks * channels
	// classForeground is the class an exported foreground-only pack gives
	// every point (annotation's keepForeground).
	classForeground = 1
)

func main() {
	pcap := flag.String("pcap", "", "the capture the pack was cut from (required)")
	labels := flag.String("labels", "", "label set directory with labels.json, windows.json and domain-returns/ (required)")
	pack := flag.String("pack", "", "pack directory with manifest.json and samples.json (required)")
	port := flag.Int("port", 2369, "UDP port of the sensor's packets")
	flag.Parse()
	if *pcap == "" || *labels == "" || *pack == "" {
		flag.Usage()
		os.Exit(2)
	}
	if err := run(*pcap, *labels, *pack, *port, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "lidar-pack-rebuild: %v\n", err)
		os.Exit(1)
	}
}

func run(pcap, labelsDir, packDir string, port int, out io.Writer) error {
	var m map[string]any
	if err := readJSON(filepath.Join(packDir, "manifest.json"), &m); err != nil {
		return err
	}
	digest, _ := m["pack_digest"].(string)
	if !strings.HasPrefix(digest, "sha256:") || len(digest) < 19 {
		return fmt.Errorf("manifest pack_digest %q is not a sha256 digest", digest)
	}
	domain := "fg-" + digest[7:19]

	// The packet table, and every return decoded.
	parser := newParser()
	var table []byte
	points := map[uint64]point{}
	ordinal := 0
	if _, err := labelset.ScanCapture(pcap, port, func(p labelset.Packet) error {
		table = binary.LittleEndian.AppendUint64(table, uint64(p.Key))
		ps, err := parser.ParsePacket(p.Payload)
		if err == nil {
			for _, q := range ps {
				x, y, z := l2frames.SphericalToCartesian(q.Distance, q.Azimuth, q.Elevation)
				r := uint64(ordinal)*perPacket + uint64(q.BlockID)*channels + uint64(q.Channel-1)
				points[r] = point{float32(x), float32(y), float32(z), q.Intensity}
			}
		}
		ordinal++
		return nil
	}); err != nil {
		return err
	}
	if got, want := sha(table), str(m, "packets_sha256"); got != want {
		return fmt.Errorf("packet table from %s is %s, the manifest says %s: not the pack's capture", pcap, got, want)
	}
	for _, dir := range []string{labelsDir, packDir} {
		if err := os.WriteFile(filepath.Join(dir, "packets.bin"), table, 0o644); err != nil {
			return err
		}
	}
	ls, err := labelset.Open(labelsDir)
	if err != nil {
		return fmt.Errorf("open the label set with its rebuilt packet table: %w", err)
	}

	var samples []struct {
		PointCount int `json:"point_count"`
	}
	if err := readJSON(filepath.Join(packDir, "samples.json"), &samples); err != nil {
		return err
	}
	perSample, err := sampleReturns(ls.State.Windows, ls.State.DomainReturns, domain, len(samples))
	if err != nil {
		return err
	}
	var returnsBin, pointsBin []byte
	missing := 0
	for i, rets := range perSample {
		if len(rets) != samples[i].PointCount {
			return fmt.Errorf("sample %d declares %d points, domain %s holds %d returns in its window", i, samples[i].PointCount, domain, len(rets))
		}
		for _, r := range rets {
			returnsBin = binary.LittleEndian.AppendUint64(returnsBin, r)
		}
		block, n := encodeSample(rets, points)
		missing += n
		pointsBin = append(pointsBin, block...)
	}
	if missing > 0 {
		return fmt.Errorf("%d domain returns decode to no point", missing)
	}
	if got, want := sha(returnsBin), str(m, "returns_sha256"); got != want {
		return fmt.Errorf("returns from the label set are %s, the manifest says %s", got, want)
	}
	if err := os.WriteFile(filepath.Join(packDir, "returns.bin"), returnsBin, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(packDir, "points.bin"), pointsBin, 0o644); err != nil {
		return err
	}
	published := str(m, "points_sha256")
	rewriteManifest(m, sha(pointsBin))
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(packDir, "manifest.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(packDir, "backgrounds.json"))
	fmt.Fprintf(out, "packets %d: %s (matches the manifest)\n", ordinal, sha(table))
	fmt.Fprintf(out, "returns: %s (matches the manifest)\n", sha(returnsBin))
	fmt.Fprintf(out, "points: %s (published %s)\n", m["points_sha256"], published)
	fmt.Fprintf(out, "pack digest: %s (domain %s)\n", m["pack_digest"], domain)
	return nil
}

type point struct {
	x, y, z float32
	in      uint8
}

func newParser() *parse.Pandar40PParser {
	cfg, err := parse.LoadEmbeddedPandar40PConfig()
	if err != nil {
		panic(err)
	}
	return parse.NewPandar40PParser(*cfg)
}

// sampleReturns gives each sample, by window id, the return numbers its
// domain kept: each window's offsets added to its start, ascending.
func sampleReturns(windows []labelset.Window, domains []labelset.DomainReturns, domain string, n int) ([][]uint64, error) {
	start := make(map[int]uint64, len(windows))
	for _, w := range windows {
		start[w.WindowID] = w.Start
	}
	out := make([][]uint64, n)
	for _, d := range domains {
		if d.DomainID != domain {
			continue
		}
		if d.WindowID < 0 || d.WindowID >= n {
			return nil, fmt.Errorf("domain %s names window %d, outside the pack's %d samples", domain, d.WindowID, n)
		}
		s, ok := start[d.WindowID]
		if !ok {
			return nil, fmt.Errorf("domain %s names window %d, which the label set does not hold", domain, d.WindowID)
		}
		for _, r := range d.Returns {
			for k := uint64(0); k < r.Len; k++ {
				out[d.WindowID] = append(out[d.WindowID], s+r.Start+k)
			}
		}
		sort.Slice(out[d.WindowID], func(i, j int) bool { return out[d.WindowID][i] < out[d.WindowID][j] })
	}
	return out, nil
}

// encodeSample lays one sample's points out as the pack does: every x, then
// y, then z as float32, then intensity, then class. It reports how many
// returns decoded to no point.
func encodeSample(rets []uint64, points map[uint64]point) ([]byte, int) {
	n := len(rets)
	buf := make([]byte, 0, n*14)
	missing := 0
	ps := make([]point, n)
	for i, r := range rets {
		p, ok := points[r]
		if !ok {
			missing++
		}
		ps[i] = p
	}
	for c := 0; c < 3; c++ {
		for _, p := range ps {
			v := [3]float32{p.x, p.y, p.z}[c]
			buf = binary.LittleEndian.AppendUint32(buf, math.Float32bits(v))
		}
	}
	for _, p := range ps {
		buf = append(buf, p.in)
	}
	for range ps {
		buf = append(buf, classForeground)
	}
	return buf, missing
}

// rewriteManifest records the rebuilt points' digest, recomputes the pack
// digest the way the pack writer does, and drops the backgrounds.
func rewriteManifest(m map[string]any, points string) {
	m["points_sha256"] = points
	m["pack_digest"] = sha([]byte(points + "\n" + str(m, "samples_sha256") + "\n" + str(m, "packets_sha256") + "\n" + str(m, "returns_sha256")))
	delete(m, "backgrounds_sha256")
	delete(m, "background_points_sha256")
	m["background_count"] = 0
	note := str(m, "coverage_note")
	m["coverage_note"] = strings.TrimPrefix(note+"; ", "; ") +
		"RECONSTRUCTED by lidar-pack-rebuild: packet table and returns rebuilt from the capture and the label set and checked against the published digests; points decoded by this build's parser; backgrounds omitted"
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(s[:])
}

func str(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return v
}

func readJSON(path string, v any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(bufio.NewReader(f)).Decode(v)
}
