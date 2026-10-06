// Command scene-export writes a static, browser-servable JSON view of a
// recorded VRLOG. It is a thin wrapper over the shared engine and is
// equivalent to `velocity scene export`.
package main

import (
	"os"

	scenecmd "github.com/banshee-data/velocity.report/internal/cmd/scene"
)

func main() {
	os.Exit(scenecmd.Main(append([]string{"export"}, os.Args[1:]...)))
}
