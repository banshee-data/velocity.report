//go:build !pcap

package server

import (
	"context"
	"fmt"

	"github.com/banshee-data/velocity.report/internal/lidar/capjobs"
)

func (ws *Server) runLaneProposals(context.Context, capjobs.Job, func(capjobs.Progress)) error {
	return fmt.Errorf("lane proposals jobs require the pcap build")
}
