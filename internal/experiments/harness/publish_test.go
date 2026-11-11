package harness

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublisherMergesOnlyCheckedHeadWithClearReviews(t *testing.T) {
	for _, tc := range []struct {
		name, check, review string
		statusCount         int
		status              string
		want                bool
	}{
		{"green", "success", "clear", 0, "", true},
		{"pending", "pending", "clear", 0, "", false},
		{"failed", "failure", "clear", 0, "", false},
		{"unresolved review", "success", "open", 0, "", false},
		{"required approval", "success", "required", 0, "", false},
		{"changes requested", "success", "requested", 0, "", false},
		{"missing review data", "success", "missing", 0, "", false},
		{"commit status pending", "success", "clear", 1, "pending", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o, _ := fixture(t)
			if _, e := Tick(context.Background(), o); e != nil {
				t.Fatal(e)
			}
			r := activeRun(t, o)
			note, e := os.ReadFile(filepath.Join(o.Host.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
			if e != nil {
				t.Fatal(e)
			}
			head := strings.Repeat("a", 40)
			merged := false
			wrote := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
				if q.Header.Get("Authorization") != "Bearer installation-test" {
					t.Error("missing installation auth")
				}
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(q.URL.Path, "/pulls") && q.Method == "GET":
					fmt.Fprint(w, `[{"number":7,"node_id":"PR_7","draft":false,"state":"open","html_url":"https://github.com/owner/repo/pull/7"}]`)
				case strings.HasSuffix(q.URL.Path, "/commits/main"):
					fmt.Fprintf(w, `{"sha":%q}`, r.Commit)
				case strings.Contains(q.URL.Path, "/git/ref/heads/"):
					fmt.Fprintf(w, `{"object":{"sha":%q}}`, head)
				case strings.Contains(q.URL.Path, "/compare/"):
					fmt.Fprintf(w, `{"files":[{"filename":%q,"status":"added"}]}`, "data/experiments/runs/"+r.Slug+"/"+r.ID+".md")
				case strings.Contains(q.URL.Path, "/contents/"):
					if q.Method != "GET" {
						wrote = true
					}
					fmt.Fprintf(w, `{"sha":"blob","content":%q}`, base64.StdEncoding.EncodeToString(note))
				case strings.HasSuffix(q.URL.Path, "/check-runs"):
					status := "completed"
					if tc.check == "pending" {
						status = "in_progress"
					}
					fmt.Fprintf(w, `{"total_count":1,"check_runs":[{"head_sha":%q,"status":%q,"conclusion":%q}]}`, head, status, tc.check)
				case strings.HasSuffix(q.URL.Path, "/status"):
					fmt.Fprintf(w, `{"total_count":%d,"state":%q}`, tc.statusCount, tc.status)
				case q.URL.Path == "/graphql":
					if tc.review == "missing" {
						fmt.Fprint(w, `{"data":{"repository":null}}`)
						break
					}
					decision := ""
					if tc.review == "requested" {
						decision = "CHANGES_REQUESTED"
					}
					if tc.review == "required" {
						decision = "REVIEW_REQUIRED"
					}
					fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{"reviewDecision":%q,"reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[{"isResolved":%t}]}}}}}`, decision, tc.review != "open")
				case strings.HasSuffix(q.URL.Path, "/merge"):
					var body map[string]any
					json.NewDecoder(q.Body).Decode(&body)
					if body["sha"] != head || body["merge_method"] != "squash" {
						t.Error("merge did not pin checked head")
					}
					merged = true
					fmt.Fprint(w, `{"merged":true}`)
				default:
					t.Errorf("unexpected %s %s", q.Method, q.URL)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			g := githubClient{base: server.URL, token: "installation-test", client: server.Client()}
			p := Publication{Repository: "owner/repo", BotName: "Harness", BotEmail: "harness@example.invalid"}
			_, e = publishNote(context.Background(), g, p, &r, note)
			if tc.review == "missing" {
				if e == nil {
					t.Error("missing review response accepted")
				}
			} else if e != nil {
				t.Fatal(e)
			}
			if merged != tc.want {
				t.Errorf("merge=%v want %v", merged, tc.want)
			}
			if wrote {
				t.Fatal("unchanged terminal note was pushed again, resetting CI")
			}
		})
	}
}

func TestPublisherCreatesDraftClaimAndRecoversMergedPR(t *testing.T) {
	o, _ := fixture(t)
	if _, e := Tick(context.Background(), o); e != nil {
		t.Fatal(e)
	}
	r := activeRun(t, o)
	note, _ := os.ReadFile(filepath.Join(o.Host.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
	created := false
	committed := false
	recoverMerged := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		switch {
		case strings.HasSuffix(q.URL.Path, "/pulls") && q.Method == "GET":
			if recoverMerged {
				fmt.Fprint(w, `[{"number":7,"state":"closed","merged_at":"2026-11-10T20:00:00Z"}]`)
			} else {
				fmt.Fprint(w, `[]`)
			}
		case strings.HasSuffix(q.URL.Path, "/commits/main"):
			if recoverMerged {
				t.Error("merged recovery touched branch")
			}
			fmt.Fprintf(w, `{"sha":%q}`, r.Commit)
		case strings.Contains(q.URL.Path, "/git/ref/heads/"):
			w.WriteHeader(404)
		case strings.HasSuffix(q.URL.Path, "/git/refs"):
			var b map[string]any
			json.NewDecoder(q.Body).Decode(&b)
			if b["sha"] != r.Commit {
				t.Error("branch not pinned")
			}
			created = true
			fmt.Fprint(w, `{}`)
		case strings.Contains(q.URL.Path, "/contents/"):
			if q.Method == "GET" {
				w.WriteHeader(404)
				break
			}
			var b map[string]any
			json.NewDecoder(q.Body).Decode(&b)
			decoded, _ := base64.StdEncoding.DecodeString(b["content"].(string))
			if string(decoded) != string(note) || !strings.Contains(b["message"].(string), "Signed-off-by:") {
				t.Error("incorrect note commit")
			}
			committed = true
			fmt.Fprint(w, `{"commit":{"sha":"head"}}`)
		case strings.HasSuffix(q.URL.Path, "/pulls") && q.Method == "POST":
			var b map[string]any
			json.NewDecoder(q.Body).Decode(&b)
			if b["draft"] != true {
				t.Error("claim was not draft")
			}
			fmt.Fprint(w, `{"number":7,"node_id":"PR_7","html_url":"https://github.com/owner/repo/pull/7"}`)
		default:
			t.Errorf("unexpected %s %s", q.Method, q.URL)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	g := githubClient{base: server.URL, token: "test", client: server.Client()}
	p := Publication{Repository: "owner/repo", BotName: "Harness", BotEmail: "test@example.invalid"}
	state, e := publishNote(context.Background(), g, p, &r, note)
	if e != nil || state != "draft" || !created || !committed {
		t.Fatalf("%s %v created=%v committed=%v", state, e, created, committed)
	}
	recoverMerged = true
	state, e = publishNote(context.Background(), g, p, &r, note)
	if e != nil || state != "merged" {
		t.Fatalf("lost merge response did not recover: %s %v", state, e)
	}
}

func TestPublicationValidationAndKeyPermissions(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "app.pem")
	raw := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if e = os.WriteFile(path, raw, 0644); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	p := Publication{Repository: "owner/repo", AppID: 1, InstallationID: 2, PrivateKey: path, BotName: "Harness", BotEmail: "test@example.invalid"}
	if e = validatePublication(&p); e != nil {
		t.Fatal(e)
	}
	if _, e = appClient(context.Background(), p); e == nil || !strings.Contains(e.Error(), "permissions") {
		t.Fatalf("world-readable App key accepted: %v", e)
	}
	p.Repository = "not-a-repository"
	if e = validatePublication(&p); e == nil {
		t.Fatal("invalid repository accepted")
	}
}

func TestConnectedFetchAndOfflineFallback(t *testing.T) {
	o, _ := fixture(t)
	remote := filepath.Join(t.TempDir(), "remote.git")
	gitTest(t, o.Root, "clone", "--bare", o.Root, remote)
	gitTest(t, o.Root, "remote", "add", "origin", remote)
	contributor := filepath.Join(t.TempDir(), "contributor")
	gitTest(t, o.Root, "clone", remote, contributor)
	gitTest(t, contributor, "config", "user.name", "Test")
	gitTest(t, contributor, "config", "user.email", "test@example.invalid")
	put(t, contributor, "new-checkin.md", "new source check-in\n")
	gitTest(t, contributor, "add", ".")
	gitTest(t, contributor, "commit", "-m", "new check-in")
	newSHA := gitTest(t, contributor, "rev-parse", "HEAD")
	gitTest(t, contributor, "push", "origin", "main")
	o.Host.FetchOnTick = true
	st, e := Tick(context.Background(), o)
	if e != nil || st.Commit != newSHA {
		t.Fatalf("fetch did not advance local main: %+v %v", st, e)
	}
	gitTest(t, o.Root, "remote", "set-url", "origin", filepath.Join(t.TempDir(), "unavailable.git"))
	st, e = Tick(context.Background(), o)
	if e != nil || st.Commit != newSHA || st.Skipped["sync"] == "" {
		t.Fatalf("offline fallback lost queue: %+v %v", st, e)
	}
}
