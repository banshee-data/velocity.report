package harness

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Publication configures a repository-scoped GitHub App. The private key stays
// on the worker; a token is minted only for a connected publishing attempt.
type Publication struct {
	Repository     string `json:"repository"`
	AppID          int64  `json:"app_id"`
	InstallationID int64  `json:"installation_id"`
	PrivateKey     string `json:"private_key"`
	BotName        string `json:"bot_name"`
	BotEmail       string `json:"bot_email"`
}

type githubClient struct {
	base, token string
	client      *http.Client
}

type apiError struct{ Status int }

func (e apiError) Error() string { return fmt.Sprintf("GitHub API HTTP %d", e.Status) }
func (g githubClient) call(ctx context.Context, method, path string, input, output any) error {
	var body io.Reader
	if input != nil {
		b, e := json.Marshal(input)
		if e != nil {
			return e
		}
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, g.base+path, body)
	if e != nil {
		return e
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Content-Type", "application/json")
	resp, e := g.client.Do(req)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return apiError{resp.StatusCode}
	}
	if output != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(output)
	}
	return nil
}

func appClient(ctx context.Context, p Publication) (githubClient, error) {
	g := githubClient{base: "https://api.github.com", client: &http.Client{Timeout: 20 * time.Second}}
	info, e := os.Stat(p.PrivateKey)
	if e != nil {
		return g, e
	}
	if info.Mode().Perm()&0077 != 0 {
		return g, fmt.Errorf("App key permissions must be 0600 or stricter")
	}
	raw, e := os.ReadFile(p.PrivateKey)
	if e != nil {
		return g, e
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return g, fmt.Errorf("App key is not PEM")
	}
	key, e := x509.ParsePKCS1PrivateKey(block.Bytes)
	if e != nil {
		parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
		if e != nil {
			return g, fmt.Errorf("App key is not an RSA private key")
		}
		var ok bool
		key, ok = parsed.(*rsa.PrivateKey)
		if !ok {
			return g, fmt.Errorf("App key must be RSA")
		}
	}
	enc := base64.RawURLEncoding.EncodeToString
	header := enc([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iat": time.Now().Add(-time.Minute).Unix(), "exp": time.Now().Add(9 * time.Minute).Unix(), "iss": fmt.Sprint(p.AppID)})
	unsigned := header + "." + enc(claims)
	sum := sha256.Sum256([]byte(unsigned))
	sig, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if e != nil {
		return g, e
	}
	g.token = unsigned + "." + enc(sig)
	var result struct {
		Token string `json:"token"`
	}
	if e = g.call(ctx, http.MethodPost, fmt.Sprintf("/app/installations/%d/access_tokens", p.InstallationID), map[string]any{"repositories": []string{strings.Split(p.Repository, "/")[1]}, "permissions": map[string]string{"contents": "write", "pull_requests": "write", "checks": "read", "statuses": "read"}}, &result); e != nil {
		return g, e
	}
	if result.Token == "" {
		return g, fmt.Errorf("App installation returned no token")
	}
	g.token = result.Token
	return g, nil
}

// publishNote changes only this run's note on its own draft branch. Complete
// runs merge on a later tick once every check is green and no review is open.
// Failed runs retain their draft claim so explicit recovery can update it.
func publishNote(ctx context.Context, g githubClient, p Publication, r *Run, note []byte) (string, error) {
	if _, e := CheckNote(note); e != nil {
		return "", e
	}
	repo := "/repos/" + p.Repository
	branch := "harness/" + r.Slug + "/" + r.ID
	path := "data/experiments/runs/" + r.Slug + "/" + r.ID + ".md"
	owner := strings.Split(p.Repository, "/")[0]
	var prs []struct {
		Number   int     `json:"number"`
		NodeID   string  `json:"node_id"`
		Draft    bool    `json:"draft"`
		URL      string  `json:"html_url"`
		MergedAt *string `json:"merged_at"`
		State    string  `json:"state"`
	}
	if e := g.call(ctx, "GET", repo+"/pulls?state=all&head="+url.QueryEscape(owner+":"+branch), nil, &prs); e != nil {
		return "", e
	}
	if len(prs) > 1 {
		return "", fmt.Errorf("more than one run-note PR for the branch")
	}
	if len(prs) == 1 {
		pr := prs[0]
		r.PR = pr.URL
		r.PRNumber = pr.Number
		r.PRNode = pr.NodeID
		if pr.MergedAt != nil {
			r.PublishState = "merged"
			return "merged", nil
		}
		if pr.State == "closed" {
			return "", fmt.Errorf("run-note PR was closed without merging; operator review is required")
		}
	}
	var main struct {
		SHA string `json:"sha"`
	}
	if e := g.call(ctx, "GET", repo+"/commits/main", nil, &main); e != nil {
		return "", e
	}
	if main.SHA != r.Commit {
		var cmp struct {
			MergeBase struct {
				SHA string `json:"sha"`
			} `json:"merge_base_commit"`
		}
		if e := g.call(ctx, "GET", repo+"/compare/"+r.Commit+"..."+main.SHA, nil, &cmp); e != nil {
			return "", e
		}
		if cmp.MergeBase.SHA != r.Commit {
			return "", fmt.Errorf("pinned commit is not an ancestor of remote main")
		}
	}
	var ref struct {
		Object struct {
			SHA string `json:"sha"`
		} `json:"object"`
	}
	e := g.call(ctx, "GET", repo+"/git/ref/heads/"+branch, nil, &ref)
	if e != nil {
		if ae, ok := e.(apiError); !ok || ae.Status != 404 {
			return "", e
		}
		if e = g.call(ctx, "POST", repo+"/git/refs", map[string]string{"ref": "refs/heads/" + branch, "sha": r.Commit}, nil); e != nil {
			return "", e
		}
		ref.Object.SHA = r.Commit
	}
	// A branch changed by another actor must still contain only this note.
	if ref.Object.SHA != r.Commit {
		var cmp struct {
			Files []struct {
				Filename string `json:"filename"`
				Status   string `json:"status"`
			} `json:"files"`
		}
		if e = g.call(ctx, "GET", repo+"/compare/"+r.Commit+"..."+ref.Object.SHA, nil, &cmp); e != nil {
			return "", e
		}
		if len(cmp.Files) != 1 || cmp.Files[0].Filename != path || cmp.Files[0].Status != "added" {
			return "", fmt.Errorf("harness branch contains changes beyond its new run note")
		}
	}
	var existing struct {
		SHA     string `json:"sha"`
		Content string `json:"content"`
	}
	contents := repo + "/contents/" + path
	e = g.call(ctx, "GET", contents+"?ref="+url.QueryEscape(branch), nil, &existing)
	if e != nil {
		if ae, ok := e.(apiError); !ok || ae.Status != 404 {
			return "", e
		}
	}
	current, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(existing.Content, "\n", ""))
	if !bytes.Equal(current, note) {
		message := "[exe][docs] " + r.Slug + " run " + r.ID + "\n\nHarness-Run: " + r.ID + "\nSigned-off-by: " + p.BotName + " <" + p.BotEmail + ">"
		input := map[string]any{"message": message, "content": base64.StdEncoding.EncodeToString(note), "branch": branch, "committer": map[string]string{"name": p.BotName, "email": p.BotEmail}}
		if existing.SHA != "" {
			input["sha"] = existing.SHA
		}
		var changed struct {
			Commit struct {
				SHA string `json:"sha"`
			} `json:"commit"`
		}
		if e = g.call(ctx, "PUT", contents, input, &changed); e != nil {
			return "", e
		}
		ref.Object.SHA = changed.Commit.SHA
	}

	if len(prs) == 0 {
		var pr struct {
			Number int    `json:"number"`
			NodeID string `json:"node_id"`
			URL    string `json:"html_url"`
		}
		if e = g.call(ctx, "POST", repo+"/pulls", map[string]any{"title": "[exe][docs] " + r.Slug + " run " + r.ID, "head": branch, "base": "main", "draft": true, "body": "Measurement record for `" + r.Slug + "`, pinned to `" + r.Commit + "`. Raw evidence stays on the worker. This note makes no adoption or physical validation decision."}, &pr); e != nil {
			return "", e
		}
		r.PR = pr.URL
		r.PRNumber = pr.Number
		r.PRNode = pr.NodeID
		r.PublishState = "draft"
		return "draft", nil
	}
	if len(prs) != 1 {
		return "", fmt.Errorf("more than one open run-note PR")
	}
	pr := prs[0]
	r.PR = pr.URL
	r.PRNumber = pr.Number
	r.PRNode = pr.NodeID
	if r.State != "complete" {
		r.PublishState = "draft"
		return "draft", nil
	}
	if pr.Draft {
		input := map[string]any{"query": "mutation($id:ID!){markPullRequestReadyForReview(input:{pullRequestId:$id}){pullRequest{id}}}", "variables": map[string]any{"id": pr.NodeID}}
		if e = g.graphql(ctx, input, nil); e != nil {
			return "", e
		}
	}
	ready, e := g.checksGreen(ctx, repo, ref.Object.SHA)
	if e != nil {
		return "", e
	}
	if !ready {
		r.PublishState = "checks pending"
		return r.PublishState, nil
	}
	clear, e := g.reviewsClear(ctx, p, pr.Number)
	if e != nil {
		return "", e
	}
	if !clear {
		r.PublishState = "needs review"
		return r.PublishState, nil
	}
	var merged struct {
		Merged bool `json:"merged"`
	}
	if e = g.call(ctx, "PUT", fmt.Sprintf("%s/pulls/%d/merge", repo, pr.Number), map[string]any{"merge_method": "squash", "sha": ref.Object.SHA, "commit_title": "[exe][docs] " + r.Slug + " run " + r.ID, "commit_message": "Harness-Run: " + r.ID + "\nSigned-off-by: " + p.BotName + " <" + p.BotEmail + ">"}, &merged); e != nil {
		return "", e
	}
	if !merged.Merged {
		return "", fmt.Errorf("GitHub did not merge the run note")
	}
	r.PublishState = "merged"
	return "merged", nil
}

func (g githubClient) graphql(ctx context.Context, input any, data any) error {
	var response struct {
		Data   json.RawMessage   `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if e := g.call(ctx, "POST", "/graphql", input, &response); e != nil {
		return e
	}
	if len(response.Errors) > 0 || len(response.Data) == 0 || string(response.Data) == "null" {
		return fmt.Errorf("GitHub GraphQL did not confirm the requested operation")
	}
	if data != nil {
		return json.Unmarshal(response.Data, data)
	}
	return nil
}
func (g githubClient) checksGreen(ctx context.Context, repo, sha string) (bool, error) {
	var checks struct {
		Count int `json:"total_count"`
		Runs  []struct {
			Status     string `json:"status"`
			Conclusion string `json:"conclusion"`
			HeadSHA    string `json:"head_sha"`
		} `json:"check_runs"`
	}
	if e := g.call(ctx, "GET", repo+"/commits/"+sha+"/check-runs?per_page=100", nil, &checks); e != nil {
		return false, e
	}
	if checks.Count == 0 || checks.Count > 100 || len(checks.Runs) != checks.Count {
		return false, nil
	}
	for _, c := range checks.Runs {
		if c.HeadSHA != sha || c.Status != "completed" || c.Conclusion != "success" && c.Conclusion != "neutral" && c.Conclusion != "skipped" {
			return false, nil
		}
	}
	var statuses struct {
		Count int    `json:"total_count"`
		State string `json:"state"`
	}
	if e := g.call(ctx, "GET", repo+"/commits/"+sha+"/status", nil, &statuses); e != nil {
		return false, e
	}
	return statuses.Count == 0 || statuses.State == "success", nil
}
func (g githubClient) reviewsClear(ctx context.Context, p Publication, n int) (bool, error) {
	var result struct {
		Repository *struct {
			PullRequest *struct {
				Threads struct {
					Page struct {
						More bool `json:"hasNextPage"`
					} `json:"pageInfo"`
					Nodes []struct {
						Resolved bool `json:"isResolved"`
					} `json:"nodes"`
				} `json:"reviewThreads"`
				Decision string `json:"reviewDecision"`
			} `json:"pullRequest"`
		} `json:"repository"`
	}
	parts := strings.Split(p.Repository, "/")
	input := map[string]any{"query": "query($owner:String!,$name:String!,$n:Int!){repository(owner:$owner,name:$name){pullRequest(number:$n){reviewDecision reviewThreads(first:100){pageInfo{hasNextPage} nodes{isResolved}}}}}", "variables": map[string]any{"owner": parts[0], "name": parts[1], "n": n}}
	if e := g.graphql(ctx, input, &result); e != nil {
		return false, e
	}
	if result.Repository == nil || result.Repository.PullRequest == nil {
		return false, fmt.Errorf("GitHub did not return pull-request review state")
	}
	pr := result.Repository.PullRequest
	if pr.Decision != "" && pr.Decision != "APPROVED" || pr.Threads.Page.More {
		return false, nil
	}
	for _, t := range pr.Threads.Nodes {
		if !t.Resolved {
			return false, nil
		}
	}
	return true, nil
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func validatePublication(p *Publication) error {
	if p == nil {
		return nil
	}
	if !repositoryPattern.MatchString(p.Repository) || p.AppID <= 0 || p.InstallationID <= 0 || !filepath.IsAbs(p.PrivateKey) || trim(p.BotName) == "" || trim(p.BotEmail) == "" || strings.ContainsAny(p.BotName+p.BotEmail, "\r\n<>") {
		return fmt.Errorf("publication requires owner/repo, App and installation IDs, absolute private_key, bot_name and bot_email")
	}
	return nil
}

func publish(ctx context.Context, h Host, r *Run) error {
	if h.Publish == nil || r.PublishState == "merged" {
		return nil
	}
	raw, e := os.ReadFile(filepath.Join(h.EvidenceRoot, "notes", r.Slug, r.ID+".md"))
	if e != nil {
		return e
	}
	g, e := appClient(ctx, *h.Publish)
	if e != nil {
		return e
	}
	_, e = publishNote(ctx, g, *h.Publish, r, raw)
	return e
}
