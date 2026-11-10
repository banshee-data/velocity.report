package experiments

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/banshee-data/velocity.report/internal/experiments/harness"
)

func harnessCommand(cmd string, args []string) int {
	fs := flag.NewFlagSet("experiments "+cmd, flag.ContinueOnError)
	base := fs.String("base", "", "check-notes: Git base revision")
	branch := fs.Bool("harness-branch", false, "check-notes: allow only added notes")
	root := fs.String("root", ".", "clean local main checkout")
	host := fs.String("host", "", "untracked host profile JSON (required)")
	retry := fs.Bool("retry-failed", false, "allow three more attempts for exhausted failed units")
	dry := fs.Bool("dry-run", false, "check selection without builds or replays")
	noPublish := fs.Bool("no-publish", false, "disable GitHub publication for this invocation")
	offline := fs.Bool("offline", false, "disable fetch and publication for this invocation")
	rest, e := parse(fs, args)
	if e != nil {
		return 2
	}

	if cmd == "check-notes" {
		if len(rest) != 0 {
			return 2
		}
		if e := harness.GuardNotes(context.Background(), *root, *base, *branch); e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		return 0
	}
	if cmd == "check-note" {
		if len(rest) != 1 {
			return 2
		}
		b, e := os.ReadFile(rest[0])
		if e == nil {
			_, e = harness.CheckNote(b)
		}
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		return 0
	}
	if *host == "" {
		fmt.Fprintln(os.Stderr, "-host profile.json is required")
		return 2
	}
	h, e := harness.ReadHost(*host)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if *noPublish || *offline {
		h.Publish = nil
	}
	if *offline {
		h.FetchOnTick = false
	}
	if cmd == "harness-status" {
		if len(rest) != 0 {
			return 2
		}
		p := filepath.Join(h.EvidenceRoot, "status", "heartbeat.json")
		b, e := os.ReadFile(p)
		if e != nil {
			fmt.Fprintln(os.Stderr, e)
			return 1
		}
		snapshot := map[string]json.RawMessage{"tick": b}
		if last, e := os.ReadFile(filepath.Join(h.EvidenceRoot, "status", "last-run.json")); e == nil {
			snapshot["last_run"] = last
		}
		payload, _ := json.MarshalIndent(snapshot, "", "  ")
		fmt.Println(string(payload))
		return 0
	}
	slug := ""
	if cmd == "run" {
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "usage: velocity experiments run <slug> -host profile.json [-root DIR] [-dry-run]")
			return 2
		}
		slug = rest[0]
	} else if len(rest) != 0 {
		return 2
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	status, e := harness.Tick(ctx, harness.Options{Root: *root, Host: h, Slug: slug, DryRun: *dry, RetryFailed: *retry})
	b, _ := json.MarshalIndent(status, "", "  ")
	fmt.Println(string(b))
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		return 1
	}
	if cmd == "run" && !*dry {
		if why, ok := status.Skipped[slug]; ok && why != "complete" {
			fmt.Fprintln(os.Stderr, why)
			return 1
		}
	}
	return 0
}
