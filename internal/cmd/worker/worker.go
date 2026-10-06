// Package worker is `velocity worker`: the job runner, serving its API on a
// LAN port and running one job at a time from its work directory.
package worker

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/jobs/runner"
	"github.com/banshee-data/velocity.report/internal/version"
)

// Main runs the worker until interrupted.
func Main(args []string) int {
	fs := flag.NewFlagSet("velocity-worker", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	workerID := fs.String("id", "", "This worker's name, as the hub and the dashboard will show it (default: the hostname)")
	listen := fs.String("listen", "127.0.0.1:8084", "Address to serve the worker API on. Use a LAN address to reach it from another host; never a public one")
	token := fs.String("token", os.Getenv("VELOCITY_WORKER_TOKEN"), "Bearer token every request but /health must carry (default: $VELOCITY_WORKER_TOKEN)")
	pcapRoot := fs.String("pcap-root", "", "Directory the captures live under; a job's captures are found and verified inside it (required)")
	workDir := fs.String("work-dir", "", "Directory for attempts, bundles, logs and the capture digest cache (required)")
	sourceRepo := fs.String("source-repo", "", "A clone of velocity.report on this host, for building a job's tool at its commit (optional; without it only in-process kinds run)")
	toolCache := fs.String("tool-cache", "", "Where staged trees and built tools are kept (default: <work-dir>/tools)")
	poll := fs.Duration("poll", 2*time.Second, "How long to wait when the queue is empty")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: velocity worker --pcap-root DIR --work-dir DIR --token TOKEN [--listen ADDR] [--source-repo DIR]\n\n")
		fmt.Fprintf(os.Stderr, "Runs jobs submitted to its API, one at a time, and keeps every attempt's bundle under the work directory.\n\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *pcapRoot == "" || *workDir == "" {
		fs.Usage()
		return 2
	}
	if strings.TrimSpace(*token) == "" {
		fmt.Fprintln(os.Stderr, "velocity worker: a token is required (--token or $VELOCITY_WORKER_TOKEN); the API serves nothing without one")
		return 2
	}
	if info, err := os.Stat(*pcapRoot); err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "velocity worker: --pcap-root %s is not a directory\n", *pcapRoot)
		return 2
	}
	if host, _, err := net.SplitHostPort(*listen); err == nil && !isPrivate(host) {
		fmt.Fprintf(os.Stderr, "velocity worker: --listen %s is not a loopback or private address; this API is LAN only\n", *listen)
		return 2
	}

	store, err := runner.Open(*workDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "velocity worker: %v\n", err)
		return 1
	}
	id := *workerID
	if id == "" {
		id, _ = os.Hostname()
	}
	r := &runner.Runner{
		Store: store, Captures: runner.NewCaptures(*pcapRoot, store.Root()),
		Profile:      runner.DefaultProfile(id, version.Version, version.GitSHA),
		PollInterval: *poll,
		OnEvent:      func(format string, args ...any) { log.Printf("[worker] "+format, args...) },
	}
	if *sourceRepo != "" {
		cache := *toolCache
		if cache == "" {
			cache = filepath.Join(store.Root(), "tools")
		}
		r.Tools = runner.GitStager{Repo: *sourceRepo, CacheDir: cache}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	api := &runner.API{Runner: r, Token: *token}
	srv := &http.Server{Addr: *listen, Handler: api.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("[worker] %s serving on http://%s, captures under %s, work in %s", id, *listen, *pcapRoot, store.Root())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[worker] API stopped: %v", err)
			stop()
		}
	}()

	err = r.Run(ctx)
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
	if err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("[worker] stopped: %v", err)
		return 1
	}
	log.Printf("[worker] stopped")
	return 0
}

// isPrivate admits loopback, RFC 1918, link-local and the unspecified
// address (which binds every interface, and is what a container uses).
func isPrivate(host string) bool {
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// A name: resolved at bind time. Trust it; the policy is documented.
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}
