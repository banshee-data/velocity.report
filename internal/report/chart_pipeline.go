package report

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type chartArtifact struct {
	name     string
	svg      []byte
	workDir  string
	zipFiles map[string][]byte
}

func (a chartArtifact) materialise(ctx context.Context) error {
	svgPath := filepath.Join(a.workDir, a.name+".svg")
	if err := os.WriteFile(svgPath, a.svg, 0644); err != nil {
		return fmt.Errorf("write %s.svg: %w", a.name, err)
	}

	pdfPath := filepath.Join(a.workDir, a.name+".pdf")
	if err := convertSVGToPDF(ctx, svgPath, pdfPath); err != nil {
		return fmt.Errorf("convert %s.svg: %w", a.name, err)
	}

	a.zipFiles[a.name+".svg"] = a.svg
	return nil
}
