// Command velocity is the single multi-call binary for velocity.report.
package main

import (
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/cmd/root"
)

func main() {
	os.Exit(root.Dispatch(filepath.Base(os.Args[0]), os.Args[1:]))
}
