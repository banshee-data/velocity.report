//go:build typst_embed

package server

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

// selfCheckTypst proves that the architecture-specific embedded Typst payload
// can be extracted and executed. This catches release builds that accidentally
// embed a host-architecture Typst binary into a cross-compiled velocity binary.
func selfCheckTypst(r *selfCheckReport) {
	r.run("typst-embedded", true, func(ctx context.Context) error {
		if !typstbin.Embedded() {
			return fmt.Errorf("typst_embed build contains no embedded payload")
		}
		path, cleanup, err := typstbin.Resolve()
		if err != nil {
			return err
		}
		defer cleanup()

		output, err := exec.CommandContext(ctx, path, "--version").CombinedOutput()
		if err != nil {
			return fmt.Errorf("executing embedded typst: %w (%s)", err, strings.TrimSpace(string(output)))
		}
		fmt.Fprintf(r.out, "       %s\n", strings.TrimSpace(string(output)))
		return nil
	})
}
