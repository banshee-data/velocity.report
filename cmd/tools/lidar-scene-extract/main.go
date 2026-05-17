// Command lidar-scene-extract reads a Hesai PCAP and writes a JSON "scene"
// that the homepage hero (public_html/src/js/hero-scene.js) can ingest in
// place of its synthetic point cloud.
//
// This is a DRAFT. The goal here is to get a static, voxel-downsampled point
// cloud out of a real recording so the hero background reads as the actual
// street the sensor saw, not a generated approximation. Track extraction is
// scaffolded but not wired up yet — see TODOs below.
//
// Usage:
//
//	go run ./cmd/tools/lidar-scene-extract \
//	    -pcap internal/lidar/perf/pcap/kirk0.pcapng \
//	    -out  public_html/src/data/hero-scene.json \
//	    -duration 8 -target-points 70000
//
// JSON schema:
//
//	{
//	  "version": 1,
//	  "source":  "kirk0.pcapng",
//	  "static":  [[x,y,z,intensity], ...],   // voxel-downsampled background
//	  "tracks":  [ /* TODO: per-object trajectories */ ]
//	}
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

type config struct {
	PCAPFile     string
	OutFile      string
	UDPPort      int
	StartSeconds float64
	Duration     float64
	TargetPoints int
	VoxelMeters  float64
	MinRange     float64
	MaxRange     float64
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.PCAPFile, "pcap", "internal/lidar/perf/pcap/kirk0.pcapng", "PCAP file to read")
	flag.StringVar(&c.OutFile, "out", "public_html/src/data/hero-scene.json", "output JSON path")
	flag.IntVar(&c.UDPPort, "udp-port", 2369, "Hesai UDP port (matches kirk0.pcapng; production sensors typically use 2368)")
	flag.Float64Var(&c.StartSeconds, "start", 0, "skip this many seconds from the start of the PCAP")
	flag.Float64Var(&c.Duration, "duration", 8, "seconds of PCAP to ingest (-1 = whole file)")
	flag.IntVar(&c.TargetPoints, "target-points", 70000, "approximate point budget after downsampling")
	flag.Float64Var(&c.VoxelMeters, "voxel", 0.15, "voxel size in metres for downsampling (smaller = more points)")
	flag.Float64Var(&c.MinRange, "min-range", 1.0, "drop points closer than this (metres)")
	flag.Float64Var(&c.MaxRange, "max-range", 90.0, "drop points farther than this (metres)")
	flag.Parse()
	return c
}

// voxelKey is a packed grid cell identifier. Keeping it as an int64 (rather
// than three int32s in a struct) lets us hash much faster — voxel maps are
// the hot path.
type voxelKey int64

func makeVoxelKey(x, y, z float64, voxel float64) voxelKey {
	// Quantise to grid. We bias each axis into [-2^20, 2^20] which is fine
	// for any street scene we'd ever feed in.
	ix := int64(math.Floor(x / voxel))
	iy := int64(math.Floor(y / voxel))
	iz := int64(math.Floor(z / voxel))
	const bits = 21
	const mask = (1 << bits) - 1
	return voxelKey((ix&mask)<<(2*bits) | (iy&mask)<<bits | (iz & mask))
}

// scenePoint is the on-disk representation. Compact array form keeps the JSON
// payload small (the hero loads it on every page view).
type scenePoint [4]float32

// sceneBuilder is the FrameBuilder we hand to network.ReadPCAPFile. For now
// it just dumps every point into a voxel map — once we have background
// extraction wired in (TODO) we'll split static vs. foreground here.
type sceneBuilder struct {
	mu       sync.Mutex
	cfg      config
	voxels   map[voxelKey]scenePoint // first-write-wins per cell
	frames   int
	lastRPM  uint16
	dropped  int // points outside the [MinRange, MaxRange] band
	totalPts int
}

func newSceneBuilder(cfg config) *sceneBuilder {
	return &sceneBuilder{
		cfg:    cfg,
		voxels: make(map[voxelKey]scenePoint, 200_000),
	}
}

// AddPointsPolar receives the parser's output. We convert each polar point
// to Cartesian, range-filter, and write into the voxel map.
func (b *sceneBuilder) AddPointsPolar(points []l2frames.PointPolar) {
	if len(points) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.frames++
	for _, p := range points {
		if p.Distance < b.cfg.MinRange || p.Distance > b.cfg.MaxRange {
			b.dropped++
			continue
		}
		x, y, z := l2frames.SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
		k := makeVoxelKey(x, y, z, b.cfg.VoxelMeters)
		if _, ok := b.voxels[k]; !ok {
			b.voxels[k] = scenePoint{
				float32(x), float32(y), float32(z),
				float32(p.Intensity) / 255.0,
			}
		}
		b.totalPts++
	}
}

func (b *sceneBuilder) SetMotorSpeed(rpm uint16) {
	b.mu.Lock()
	b.lastRPM = rpm
	b.mu.Unlock()
}

// snapshot copies out the voxel map, optionally subsampling to roughly the
// target point count (uniform random keep — voxel grid already gives us
// even spatial coverage, so a uniform subsample stays representative).
func (b *sceneBuilder) snapshot(target int) []scenePoint {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(b.voxels)
	out := make([]scenePoint, 0, n)
	if target <= 0 || target >= n {
		for _, p := range b.voxels {
			out = append(out, p)
		}
		return out
	}
	// Deterministic stride sample. We don't need cryptographic uniformity
	// here — the voxel hash order is already pseudo-random.
	step := float64(n) / float64(target)
	i := 0
	threshold := 0.0
	for _, p := range b.voxels {
		if float64(i) >= threshold {
			out = append(out, p)
			threshold += step
		}
		i++
		if len(out) >= target {
			break
		}
	}
	return out
}

// packetStats implements network.PacketStatsInterface. We don't care about
// detailed stats here — just satisfy the interface so ReadPCAPFile is happy.
type packetStats struct{}

func (packetStats) AddPacket(_ int) {}
func (packetStats) AddDropped()     {}
func (packetStats) AddPoints(_ int) {}
func (packetStats) LogStats(_ bool) {}

type sceneJSON struct {
	Version int          `json:"version"`
	Source  string       `json:"source"`
	Voxel   float64      `json:"voxel_metres"`
	Frames  int          `json:"frames_seen"`
	Static  []scenePoint `json:"static"`
	// TODO: Tracks []trackJSON `json:"tracks"`
	//
	// To wire up tracks, plug in the existing pipeline:
	//   1. Inside AddPointsPolar, after the voxel insert, run the same frame
	//      through l3grid.BackgroundManager (see cmd/tools/pcap-analyse/main.go
	//      for the pattern). Foreground points go to a per-frame slice.
	//   2. Feed those foreground points to l4perception.Cluster + an OBB pass.
	//   3. Push the OBBs into l5tracks.Tracker.Update for stable IDs.
	//   4. Classify with l6objects.TrackClassifier.
	//   5. For each tracked object, record (t_seconds, x, y, z, yaw, class)
	//      samples here. The hero JS can then replay them via the same slot
	//      system that drives the synthetic objects.
}

func main() {
	cfg := parseFlags()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if _, err := os.Stat(cfg.PCAPFile); err != nil {
		log.Fatalf("pcap not found: %v", err)
	}

	parser := parse.NewPandar40PParser(*parse.DefaultPandar40PConfig())
	builder := newSceneBuilder(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("reading %s (start=%.1fs duration=%.1fs)...", cfg.PCAPFile, cfg.StartSeconds, cfg.Duration)
	startedAt := time.Now()
	if err := network.ReadPCAPFile(
		ctx,
		cfg.PCAPFile,
		cfg.UDPPort,
		parser,
		builder,
		packetStats{},
		nil, // no PacketForwarder
		cfg.StartSeconds,
		cfg.Duration,
		0, 0, nil,
	); err != nil {
		log.Fatalf("read pcap: %v", err)
	}
	log.Printf("ingest done in %s — %d frames, %d input points, %d voxels, %d outside range band",
		time.Since(startedAt).Truncate(time.Millisecond),
		builder.frames, builder.totalPts, len(builder.voxels), builder.dropped,
	)

	pts := builder.snapshot(cfg.TargetPoints)
	log.Printf("snapshot: %d points after subsample (target=%d)", len(pts), cfg.TargetPoints)

	out := sceneJSON{
		Version: 1,
		Source:  cfg.PCAPFile,
		Voxel:   cfg.VoxelMeters,
		Frames:  builder.frames,
		Static:  pts,
	}
	f, err := os.Create(cfg.OutFile)
	if err != nil {
		log.Fatalf("create output: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	if err := enc.Encode(out); err != nil {
		log.Fatalf("encode JSON: %v", err)
	}
	fi, _ := f.Stat()
	log.Printf("wrote %s (%d bytes)", cfg.OutFile, func() int64 {
		if fi != nil {
			return fi.Size()
		}
		return 0
	}())
	fmt.Println("OK")
}
