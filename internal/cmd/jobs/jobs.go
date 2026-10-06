// Package jobs is `velocity jobs`: a client for a worker's API, so that a
// job can be submitted, watched and fetched from a script or by hand.
package jobs

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const usage = `Usage: velocity jobs <command> [flags]

Commands:
  status                       what the worker is, holds and is doing
  kinds                        the job kinds this worker can run
  submit FILE                  submit the job request in FILE (JSON)
  campaign FILE                submit the campaign in FILE (JSON)
  list [--state S]             attempts, newest first
  show ID                      one attempt
  log ID [--lines N]           the attempt's log
  cancel ID                    cancel an attempt or campaign
  fetch ID [--out DIR]         download the attempt's bundle as a tar
  wait ID                      poll until the attempt is terminal

Flags (before the command):
  --worker URL     the worker's API (default $VELOCITY_WORKER_URL or http://127.0.0.1:8084)
  --token TOKEN    bearer token (default $VELOCITY_WORKER_TOKEN)
`

// Main runs one client command.
func Main(args []string) int {
	fs := flag.NewFlagSet("velocity-jobs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	worker := fs.String("worker", envOr("VELOCITY_WORKER_URL", "http://127.0.0.1:8084"), "worker API URL")
	token := fs.String("token", os.Getenv("VELOCITY_WORKER_TOKEN"), "bearer token")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) == 0 {
		fs.Usage()
		return 2
	}
	c := client{base: strings.TrimRight(*worker, "/"), token: *token}
	sub := flag.NewFlagSet("velocity-jobs-"+rest[0], flag.ContinueOnError)
	sub.SetOutput(os.Stderr)
	var err error
	switch rest[0] {
	case "status":
		err = c.printJSON("GET", "/api/worker/status", nil)
	case "kinds":
		err = c.printJSON("GET", "/api/worker/kinds", nil)
	case "submit", "campaign":
		if len(rest) < 2 {
			return usageError(rest[0] + " FILE")
		}
		body, rerr := os.ReadFile(rest[1])
		if rerr != nil {
			err = rerr
			break
		}
		path := "/api/worker/jobs"
		if rest[0] == "campaign" {
			path = "/api/worker/campaigns"
		}
		err = c.printJSON("POST", path, body)
	case "list":
		state := sub.String("state", "", "only attempts in this state")
		if err = sub.Parse(rest[1:]); err != nil {
			return 2
		}
		q := ""
		if *state != "" {
			q = "?state=" + *state
		}
		err = c.printJSON("GET", "/api/worker/jobs"+q, nil)
	case "show":
		if len(rest) < 2 {
			return usageError("show ID")
		}
		err = c.printJSON("GET", "/api/worker/jobs/"+rest[1], nil)
	case "log":
		if len(rest) < 2 {
			return usageError("log ID")
		}
		lines := sub.Int("lines", 200, "how many lines from the end")
		if err = sub.Parse(rest[2:]); err != nil {
			return 2
		}
		err = c.printLog(rest[1], *lines)
	case "cancel":
		if len(rest) < 2 {
			return usageError("cancel ID")
		}
		err = c.printJSON("POST", "/api/worker/jobs/"+rest[1]+"/cancel", nil)
	case "fetch":
		if len(rest) < 2 {
			return usageError("fetch ID")
		}
		out := sub.String("out", ".", "directory to write the tar into")
		if err = sub.Parse(rest[2:]); err != nil {
			return 2
		}
		err = c.fetch(rest[1], *out)
	case "wait":
		if len(rest) < 2 {
			return usageError("wait ID")
		}
		err = c.wait(rest[1])
	default:
		fs.Usage()
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "velocity jobs: %v\n", err)
		return 1
	}
	return 0
}

func usageError(want string) int {
	fmt.Fprintf(os.Stderr, "velocity jobs: usage: velocity jobs %s\n", want)
	return 2
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

type client struct {
	base, token string
}

func (c client) do(method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = strings.NewReader(string(body))
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := (&http.Client{Timeout: 5 * time.Minute}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(msg)))
	}
	return resp, nil
}

func (c client) printJSON(method, path string, body []byte) error {
	resp, err := c.do(method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	var v any
	if json.Unmarshal(data, &v) == nil {
		data, _ = json.MarshalIndent(v, "", "  ")
	}
	fmt.Println(string(data))
	return nil
}

func (c client) printLog(id string, lines int) error {
	resp, err := c.do("GET", fmt.Sprintf("/api/worker/jobs/%s/log?lines=%d", id, lines), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var out struct {
		Lines []string `json:"lines"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return err
	}
	for _, l := range out.Lines {
		fmt.Println(l)
	}
	return nil
}

func (c client) fetch(id, outDir string) error {
	resp, err := c.do("GET", "/api/worker/jobs/"+id+"/bundle.tar", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(outDir, id+".tar")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d bytes)\n", path, n)
	return nil
}

func (c client) wait(id string) error {
	for {
		resp, err := c.do("GET", "/api/worker/jobs/"+id, nil)
		if err != nil {
			return err
		}
		var rec struct {
			Attempt struct {
				State    string `json:"state"`
				Progress struct {
					Current int64  `json:"current"`
					Total   int64  `json:"total"`
					Detail  string `json:"detail"`
				} `json:"progress"`
				Failure *struct {
					Reason string `json:"reason"`
				} `json:"failure"`
			} `json:"attempt"`
		}
		err = json.NewDecoder(resp.Body).Decode(&rec)
		resp.Body.Close()
		if err != nil {
			return err
		}
		s := rec.Attempt.State
		fmt.Fprintf(os.Stderr, "\r%s %s %s", time.Now().Format("15:04:05"), s, rec.Attempt.Progress.Detail)
		switch s {
		case "accepted":
			fmt.Fprintln(os.Stderr)
			return nil
		case "failed", "cancelled", "lost":
			fmt.Fprintln(os.Stderr)
			reason := ""
			if rec.Attempt.Failure != nil {
				reason = ": " + rec.Attempt.Failure.Reason
			}
			return fmt.Errorf("attempt %s %s%s", id, s, reason)
		}
		time.Sleep(3 * time.Second)
	}
}
