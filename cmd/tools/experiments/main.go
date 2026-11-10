// Command experiments exposes the experiment CLI for repository tooling.
package main

import (
	experimentscmd "github.com/banshee-data/velocity.report/internal/cmd/experiments"
	"os"
)

func main() { os.Exit(experimentscmd.Main(os.Args[1:])) }
