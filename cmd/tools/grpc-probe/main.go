// Command grpc-probe streams frames from the visualiser gRPC server and reports
// gaps between messages.
//
// It exists to settle one question: when the server sits blocked in Send for a
// minute or more, is that the macOS client's doing or the server's? The stalls
// observed on 2026-08-27 and 2026-08-28 left both ends waiting on each other —
// the server blocked sending a 0.1 KB frame, the client idle on a background
// thread with a responsive main thread — which is consistent with the transport
// on either side. A second client with an entirely different HTTP/2 stack
// separates the two: if this stalls as well, the server is at fault; if it
// streams cleanly while the visualiser stalls, the fault is grpc-swift's.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	pb "github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/pb"
)

type probeStats struct {
	frames    int
	bytesSeen int64
	worstGap  time.Duration
	gaps      int
}

func (s *probeStats) observe(size int, gap, threshold time.Duration) bool {
	s.frames++
	s.bytesSeen += int64(size)
	if gap <= threshold {
		return false
	}
	s.gaps++
	if gap > s.worstGap {
		s.worstGap = gap
	}
	return true
}

func (s probeStats) framesPerSecond(elapsed time.Duration) float64 {
	if elapsed <= 0 {
		return 0
	}
	return float64(s.frames) / elapsed.Seconds()
}

func main() {
	addr := flag.String("addr", "localhost:50051", "visualiser gRPC address")
	gapThreshold := flag.Duration("gap", time.Second, "report gaps longer than this")
	duration := flag.Duration("duration", 0, "stop after this long (0 runs until interrupted)")
	windowSize := flag.Int("window", 0, "HTTP/2 initial window size in bytes (0 uses the default)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println("\ninterrupted")
		cancel()
	}()

	if *duration > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, *duration)
		defer stop()
	}

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if *windowSize > 0 {
		// Setting these disables Go's dynamic window sizing, which is the point
		// when testing whether a fixed window is what stalls.
		opts = append(opts,
			grpc.WithInitialWindowSize(int32(*windowSize)),
			grpc.WithInitialConnWindowSize(int32(*windowSize)))
		fmt.Printf("HTTP/2 windows pinned to %d bytes\n", *windowSize)
	}

	conn, err := grpc.NewClient(*addr, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dial %s: %v\n", *addr, err)
		os.Exit(1)
	}
	defer func() { _ = conn.Close() }()

	client := pb.NewVisualiserServiceClient(conn)
	stream, err := client.StreamFrames(ctx, &pb.StreamRequest{
		SensorId:        "all",
		IncludePoints:   true,
		IncludeTracks:   true,
		IncludeClusters: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "StreamFrames: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("streaming from %s, reporting gaps over %v\n", *addr, *gapThreshold)

	var stats probeStats
	last := time.Now()
	started := time.Now()

	for {
		frame, err := stream.Recv()
		now := time.Now()
		gap := now.Sub(last)
		last = now

		if err == io.EOF {
			break
		}
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			fmt.Fprintf(os.Stderr, "recv after %d frames: %v\n", stats.frames, err)
			os.Exit(1)
		}

		size := proto.Size(frame)
		if stats.observe(size, gap, *gapThreshold) {
			// Cumulative bytes locate the gap against the HTTP/2 connection
			// window, which opens at 65535 and grows only by WINDOW_UPDATE.
			fmt.Printf("%s gap %v before frame %d (%.1fKB, %.1fKB received on this stream)\n",
				now.Format("2006/01/02 15:04:05.000"), gap.Round(time.Millisecond),
				stats.frames, float64(size)/1024, float64(stats.bytesSeen)/1024)
		}

		if stats.frames%500 == 0 {
			fmt.Printf("%s %d frames, %.1fMB, %d gaps over %v\n",
				now.Format("2006/01/02 15:04:05.000"), stats.frames,
				float64(stats.bytesSeen)/(1024*1024), stats.gaps, *gapThreshold)
		}
	}

	elapsed := time.Since(started)
	fmt.Printf("\n%d frames in %v (%.1f/s), %.1fMB\n",
		stats.frames, elapsed.Round(time.Millisecond),
		stats.framesPerSecond(elapsed), float64(stats.bytesSeen)/(1024*1024))
	if stats.gaps == 0 {
		fmt.Printf("no gap over %v: this client streamed without stalling\n", *gapThreshold)
	} else {
		fmt.Printf("%d gaps over %v, worst %v\n", stats.gaps, *gapThreshold, stats.worstGap.Round(time.Millisecond))
	}
}
