// Command gen-vrlog generates sample .vrlog recordings for testing replay.
package main

import (
	"flag"
	"log"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/visualiser"
	"github.com/banshee-data/velocity.report/internal/lidar/visualiser/recorder"
)

func main() {
	output := flag.String("o", "sample.vrlog", "output path")
	frames := flag.Int("n", 100, "number of frames")
	flag.Parse()

	rec, err := recorder.NewRecorder(*output, "sample")
	if err != nil {
		log.Fatalf("Failed to create recorder: %v", err)
	}
	defer func() {
		if err := rec.Close(); err != nil {
			log.Printf("Failed to close recorder: %v", err)
		}
	}()

	gen := visualiser.NewSyntheticGenerator("sample")
	for i := 0; i < *frames; i++ {
		if err := rec.Record(gen.NextFrame()); err != nil {
			log.Fatalf("Failed to record frame %d: %v", i+1, err)
		}
		time.Sleep(100 * time.Millisecond)
		if (i+1)%10 == 0 {
			log.Printf("%d/%d frames", i+1, *frames)
		}
	}
	log.Printf("✓ Created: %s", *output)
}
