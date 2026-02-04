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

	rec, _ := recorder.NewRecorder(*output, "sample")
	defer rec.Close()

	gen := visualiser.NewSyntheticGenerator("sample")
	for i := 0; i < *frames; i++ {
		rec.Record(gen.NextFrame())
		time.Sleep(100 * time.Millisecond)
		if (i+1)%10 == 0 {
			log.Printf("%d/%d frames", i+1, *frames)
		}
	}
	log.Printf("✓ Created: %s", *output)
}
