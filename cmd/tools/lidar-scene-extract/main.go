// Command lidar-scene-extract reads a Hesai PCAP and writes a JSON "scene"
// that the homepage hero (public_html/src/js/hero-scene.js) can ingest in
// place of its synthetic point cloud.
//
// Output is two-part:
//
//  1. A static background frame (voxel-downsampled point cloud of points
//     observed during the warmup window). This is the "sensor's view of the
//     street with the road empty".
//  2. A seamless loop of dynamic frames sampled at a low rate (default
//     0.5 fps, i.e. one frame every 2 seconds of pcap wall-clock — 1/20 of
//     a 10 Hz Pandar40P scan rate). Each dynamic frame contains only points
//     that weren't in the warmup background, so the hero can render them as
//     "moving stuff" on top of the static scene.
//
// Capture is intentionally slow so when we eventually wire in the heavier
// pipeline (l3grid → l4perception → l5tracks → l6objects), CPU pressure
// doesn't drop frames at the input. The pcap is read as fast as the parser
// can keep up; we just sample 1 of every N scans for the loop.
//
// The hero's three.js camera is positioned to a configurable pose so the
// extracted scene can be aimed at whatever the sensor saw. Defaults match
// the synthetic hero's camera (slight off-centre, looking down the street).
//
// Usage:
//
//	go run -tags=pcap ./cmd/tools/lidar-scene-extract \
//	    -pcap internal/lidar/perf/pcap/kirk0.pcapng \
//	    -out  public_html/src/data/hero-scene.json \
//	    -warmup 4 -loop 30 -loop-fps 0.5 \
//	    -cam-x -2.6 -cam-y 2.6 -cam-z 6.2 \
//	    -look-x 0 -look-y 1.2 -look-z -22 -fov 54
//
// JSON schema (v2):
//
//	{
//	  "version": 2,
//	  "source": "kirk0.pcapng",
//	  "voxel_metres": 0.15,
//	  "warmup_seconds": 4,
//	  "loop_seconds":   30,
//	  "loop_fps":       0.5,
//	  "camera": {
//	    "position": [x, y, z],
//	    "look_at":  [x, y, z],
//	    "fov_deg":  54
//	  },
//	  "static": [[x,y,z,i], ...],
//	  "frames": [
//	    {"t": 0.0,  "moving": [[x,y,z,i], ...]},
//	    {"t": 2.0,  "moving": [...]},
//	    ...
//	  ]
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

type cameraConfig struct {
	Pos    [3]float64
	LookAt [3]float64
	FovDeg float64
}

type config struct {
	PCAPFile           string
	OutFile            string
	UDPPort            int
	StartSeconds       float64
	WarmupSeconds      float64 // duration spent accumulating the static background
	LoopSeconds        float64 // duration of the dynamic loop window
	LoopFPS            float64 // output frame rate for the dynamic loop
	SensorFPS          float64 // input scan rate, used to compute subsample stride
	TargetStaticPoints int
	VoxelMeters        float64
	MinRange           float64
	MaxRange           float64
	Camera             cameraConfig
}

func parseFlags() config {
	var c config
	flag.StringVar(&c.PCAPFile, "pcap", "internal/lidar/perf/pcap/kirk0.pcapng", "PCAP file to read")
	flag.StringVar(&c.OutFile, "out", "public_html/src/data/hero-scene.json", "output JSON path")
	flag.IntVar(&c.UDPPort, "udp-port", 2369, "Hesai UDP port (matches kirk0.pcapng; production sensors typically use 2368)")
	flag.Float64Var(&c.StartSeconds, "start", 0, "skip this many seconds from the start of the PCAP")
	flag.Float64Var(&c.WarmupSeconds, "warmup", 4, "seconds spent accumulating the static background frame")
	flag.Float64Var(&c.LoopSeconds, "loop", 30, "seconds of pcap captured into the dynamic loop window")
	flag.Float64Var(&c.LoopFPS, "loop-fps", 0.5, "output frame rate for the dynamic loop (1/20 of sensor fps gives 0.5 from a 10 Hz Pandar40P)")
	flag.Float64Var(&c.SensorFPS, "sensor-fps", 10, "input scan rate of the sensor; used to compute the loop subsample stride")
	flag.IntVar(&c.TargetStaticPoints, "target-points", 70000, "approximate static-frame point budget after downsampling")
	flag.Float64Var(&c.VoxelMeters, "voxel", 0.15, "voxel size in metres for downsampling (smaller = more points)")
	flag.Float64Var(&c.MinRange, "min-range", 1.0, "drop points closer than this (metres)")
	flag.Float64Var(&c.MaxRange, "max-range", 90.0, "drop points farther than this (metres)")
	flag.Float64Var(&c.Camera.Pos[0], "cam-x", -2.6, "hero camera position X (metres)")
	flag.Float64Var(&c.Camera.Pos[1], "cam-y", 2.6, "hero camera position Y (metres)")
	flag.Float64Var(&c.Camera.Pos[2], "cam-z", 6.2, "hero camera position Z (metres)")
	flag.Float64Var(&c.Camera.LookAt[0], "look-x", 0, "hero camera look-at X (metres)")
	flag.Float64Var(&c.Camera.LookAt[1], "look-y", 1.2, "hero camera look-at Y (metres)")
	flag.Float64Var(&c.Camera.LookAt[2], "look-z", -22, "hero camera look-at Z (metres)")
	flag.Float64Var(&c.Camera.FovDeg, "fov", 54, "hero camera vertical field of view (degrees)")
	flag.Parse()
	return c
}

// voxelKey is a packed grid cell identifier. Keeping it as an int64 (rather
// than three int32s in a struct) makes voxel-map lookups noticeably faster —
// this is the hot path.
type voxelKey int64

func makeVoxelKey(x, y, z float64, voxel float64) voxelKey {
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

type loopFrame struct {
	T      float64      `json:"t"`
	Moving []scenePoint `json:"moving"`
}

// sceneBuilder is the FrameBuilder we hand to network.ReadPCAPFile. It runs
// in two phases driven by a per-scan time: warmup (build background) then
// loop (emit subsampled foreground per scan).
type sceneBuilder struct {
	mu  sync.Mutex
	cfg config

	// scan-boundary detection. We bin by timestamp rather than azimuth wrap
	// because multi-return / multi-channel sensors interleave azimuths within
	// a packet and over-count wraps by ~10x (cmd/tools/pcap-analyse has the
	// same caveat documented inline).
	scanDurationNS int64
	scanIdx        int
	scanStartNS    int64
	scanPts        []cartPoint

	// pacing
	firstScanNS  int64 // anchor for "scan time" in seconds
	loopBaseScan int   // scan index at which warmup ended
	subsample    int   // 1-of-N stride into the loop window

	// outputs
	bgVoxels   map[voxelKey]scenePoint
	loopFrames []loopFrame

	// counters
	pointsSeen int
	pointsKept int
	pointsDrop int
}

// cartPoint is a Cartesian point with intensity. We compute these once per
// input point so we don't pay the trig cost twice when classifying foreground.
type cartPoint struct {
	X, Y, Z   float64
	Intensity float32
	Voxel     voxelKey
}

func newSceneBuilder(cfg config) *sceneBuilder {
	stride := int(math.Round(cfg.SensorFPS / cfg.LoopFPS))
	if stride < 1 {
		stride = 1
	}
	scanDurNS := int64(1e9 / cfg.SensorFPS)
	return &sceneBuilder{
		cfg:            cfg,
		bgVoxels:       make(map[voxelKey]scenePoint, 200_000),
		scanPts:        make([]cartPoint, 0, 80_000),
		subsample:      stride,
		scanDurationNS: scanDurNS,
	}
}

func (b *sceneBuilder) scanTimeSeconds(tsNS int64) float64 {
	if b.firstScanNS == 0 {
		return 0
	}
	return float64(tsNS-b.firstScanNS) / 1e9
}

// AddPointsPolar receives the parser's per-packet output. We convert each
// point to Cartesian, range-filter, and detect scan boundaries via azimuth
// wrap.
func (b *sceneBuilder) AddPointsPolar(points []l2frames.PointPolar) {
	if len(points) == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, p := range points {
		b.pointsSeen++
		// Scan boundary: bin by timestamp. The point's Timestamp is unix
		// nanoseconds; each scan covers 1000/SensorFPS milliseconds.
		if b.firstScanNS == 0 {
			b.firstScanNS = p.Timestamp
			b.scanStartNS = p.Timestamp
		} else if p.Timestamp-b.scanStartNS >= b.scanDurationNS {
			b.finalizeScanLocked()
			b.scanStartNS = p.Timestamp
		}

		if p.Distance < b.cfg.MinRange || p.Distance > b.cfg.MaxRange {
			b.pointsDrop++
			continue
		}
		x, y, z := l2frames.SphericalToCartesian(p.Distance, p.Azimuth, p.Elevation)
		b.scanPts = append(b.scanPts, cartPoint{
			X: x, Y: y, Z: z,
			Intensity: float32(p.Intensity) / 255.0,
			Voxel:     makeVoxelKey(x, y, z, b.cfg.VoxelMeters),
		})
	}
}

func (b *sceneBuilder) finalizeScanLocked() {
	t := b.scanTimeSeconds(b.scanStartNS)
	b.scanIdx++

	switch {
	case t < b.cfg.WarmupSeconds:
		// Warmup phase: every point goes into the background map (first
		// write wins per voxel). This effectively "paints in" the static
		// scene over multiple revolutions.
		for _, p := range b.scanPts {
			if _, ok := b.bgVoxels[p.Voxel]; !ok {
				b.bgVoxels[p.Voxel] = scenePoint{
					float32(p.X), float32(p.Y), float32(p.Z), p.Intensity,
				}
				b.pointsKept++
			}
		}
		b.loopBaseScan = b.scanIdx

	case t < b.cfg.WarmupSeconds+b.cfg.LoopSeconds:
		// Loop phase: emit one frame every `subsample` scans.
		relScan := b.scanIdx - b.loopBaseScan
		if relScan%b.subsample != 0 {
			break
		}
		// Foreground = points whose voxel isn't in the background map.
		// Cheap, imperfect, but fine for a hero background; will be replaced
		// once we wire in l3grid background subtraction.
		moving := make([]scenePoint, 0, 1024)
		for _, p := range b.scanPts {
			if _, isBg := b.bgVoxels[p.Voxel]; isBg {
				continue
			}
			moving = append(moving, scenePoint{
				float32(p.X), float32(p.Y), float32(p.Z), p.Intensity,
			})
		}
		b.loopFrames = append(b.loopFrames, loopFrame{
			T:      t - b.cfg.WarmupSeconds,
			Moving: moving,
		})
	}

	b.scanPts = b.scanPts[:0]
}

func (b *sceneBuilder) SetMotorSpeed(_ uint16) {}

// snapshotStatic returns the background voxels, subsampled to roughly the
// target count. The voxel grid already gives even spatial coverage so a
// deterministic stride sample stays representative.
func (b *sceneBuilder) snapshotStatic(target int) []scenePoint {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(b.bgVoxels)
	out := make([]scenePoint, 0, n)
	if target <= 0 || target >= n {
		for _, p := range b.bgVoxels {
			out = append(out, p)
		}
		return out
	}
	step := float64(n) / float64(target)
	i := 0
	threshold := 0.0
	for _, p := range b.bgVoxels {
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
	Version   int          `json:"version"`
	Source    string       `json:"source"`
	Voxel     float64      `json:"voxel_metres"`
	WarmupSec float64      `json:"warmup_seconds"`
	LoopSec   float64      `json:"loop_seconds"`
	LoopFPS   float64      `json:"loop_fps"`
	Camera    cameraJSON   `json:"camera"`
	Static    []scenePoint `json:"static"`
	Frames    []loopFrame  `json:"frames"`
	// TODO: wire in l3grid → l4perception → l5tracks → l6objects so the
	// `frames` array carries identified tracks (id, class, OBB, yaw) rather
	// than raw foreground point clouds. The hero JS already has a slot
	// system that can render those richer events.
}

type cameraJSON struct {
	Position [3]float64 `json:"position"`
	LookAt   [3]float64 `json:"look_at"`
	FovDeg   float64    `json:"fov_deg"`
}

func main() {
	cfg := parseFlags()
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	if _, err := os.Stat(cfg.PCAPFile); err != nil {
		log.Fatalf("pcap not found: %v", err)
	}
	if cfg.WarmupSeconds <= 0 || cfg.LoopSeconds <= 0 {
		log.Fatalf("warmup and loop durations must be positive")
	}
	if cfg.LoopFPS <= 0 || cfg.SensorFPS <= 0 {
		log.Fatalf("loop-fps and sensor-fps must be positive")
	}

	parser := parse.NewPandar40PParser(*parse.DefaultPandar40PConfig())
	builder := newSceneBuilder(cfg)

	totalDuration := cfg.WarmupSeconds + cfg.LoopSeconds
	log.Printf("reading %s (start=%.1fs warmup=%.1fs loop=%.1fs @ %.2f fps, stride %d scans)...",
		cfg.PCAPFile, cfg.StartSeconds, cfg.WarmupSeconds, cfg.LoopSeconds, cfg.LoopFPS, builder.subsample)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
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
		totalDuration,
		0, 0, nil,
	); err != nil {
		log.Fatalf("read pcap: %v", err)
	}

	// Flush any final partial scan so we don't drop the last frame.
	builder.mu.Lock()
	if len(builder.scanPts) > 0 {
		builder.finalizeScanLocked()
	}
	bgCount := len(builder.bgVoxels)
	frameCount := len(builder.loopFrames)
	builder.mu.Unlock()

	log.Printf("ingest done in %s — %d points seen, %d in-range, %d voxels bg, %d loop frames",
		time.Since(startedAt).Truncate(time.Millisecond),
		builder.pointsSeen, builder.pointsSeen-builder.pointsDrop, bgCount, frameCount,
	)

	staticPts := builder.snapshotStatic(cfg.TargetStaticPoints)
	movingTotal := 0
	for _, f := range builder.loopFrames {
		movingTotal += len(f.Moving)
	}
	log.Printf("snapshot: %d static points (target %d), %d total moving points across %d frames",
		len(staticPts), cfg.TargetStaticPoints, movingTotal, frameCount)

	out := sceneJSON{
		Version:   2,
		Source:    cfg.PCAPFile,
		Voxel:     cfg.VoxelMeters,
		WarmupSec: cfg.WarmupSeconds,
		LoopSec:   cfg.LoopSeconds,
		LoopFPS:   cfg.LoopFPS,
		Camera: cameraJSON{
			Position: cfg.Camera.Pos,
			LookAt:   cfg.Camera.LookAt,
			FovDeg:   cfg.Camera.FovDeg,
		},
		Static: staticPts,
		Frames: builder.loopFrames,
	}
	if err := os.MkdirAll(dirOf(cfg.OutFile), 0o755); err != nil {
		log.Fatalf("create output dir: %v", err)
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
	size := int64(0)
	if fi != nil {
		size = fi.Size()
	}
	log.Printf("wrote %s (%d bytes)", cfg.OutFile, size)
	fmt.Println("OK")
}

// dirOf is a tiny helper so we can MkdirAll the output directory without
// pulling in path/filepath just for one call.
func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == os.PathSeparator {
			return p[:i]
		}
	}
	return "."
}
