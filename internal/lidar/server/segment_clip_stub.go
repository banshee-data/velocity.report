//go:build !pcap

package server

import (
	"context"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
)

func (ws *Server) runSegmentClipJob(context.Context, capjobs.Job, func(capjobs.Progress)) error {
	return fmt.Errorf("clip jobs require the pcap build")
}
