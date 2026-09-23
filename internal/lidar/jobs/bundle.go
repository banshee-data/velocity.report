package jobs

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

// BundleManifest is the manifest of a result bundle: what the worker
// computed, on what, and the files that hold it. It is the one file the hub
// reads to decide whether to accept a bundle, and its digest is the bundle's
// identity for an idempotent upload.
type BundleManifest struct {
	OutputSchema int `json:"output_schema"`
	// Identity is the run identity the worker computed from the job it was
	// given. The hub compares it with its own: a bundle whose identity
	// differs from the job's is refused, whatever else it says.
	Identity       RunIdentity   `json:"identity"`
	IdentityDigest Digest        `json:"identity_digest"`
	JobID          string        `json:"job_id"`
	AttemptID      string        `json:"attempt_id"`
	Worker         WorkerProfile `json:"worker"`
	// StartedUTC and FinishedUTC are RFC 3339.
	StartedUTC  string `json:"started_utc"`
	FinishedUTC string `json:"finished_utc"`
	// Outcome is "completed" or "partial". A partial bundle is evidence of a
	// failed attempt and is never accepted as a result.
	Outcome string `json:"outcome"`
	// Summary is the kind's own summary, read from Kind.Summary and copied
	// here so the hub can index it without opening the bundle's files.
	Summary json.RawMessage `json:"summary,omitempty"`
	// Files are the bundle's contents, each by relative path and digest.
	// The manifest itself is not listed.
	Files []BundleFile `json:"files"`
}

// WorkerProfile is the host a bundle was made on. Performance figures are
// host-specific and keep this label; correctness figures do not need it.
type WorkerProfile struct {
	WorkerID string `json:"worker_id"`
	Hostname string `json:"hostname"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	CPUs     int    `json:"cpus"`
	// Version is the worker binary's version and commit.
	Version string `json:"version"`
	GitSHA  string `json:"git_sha"`
}

// BundleFile is one file in a bundle.
type BundleFile struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 Digest `json:"sha256"`
}

// BundleManifestFile is the manifest's name inside a bundle directory.
const BundleManifestFile = "bundle.json"

// Validate checks the manifest is complete and its files are inside the
// bundle. Whether the files' bytes match their digests is the hub's ingest
// check, which reads them; this is what can be checked from the manifest.
func (b BundleManifest) Validate() error {
	if b.OutputSchema != OutputSchemaVersion {
		return fmt.Errorf("bundle output schema %d, this build reads %d", b.OutputSchema, OutputSchemaVersion)
	}
	if !b.IdentityDigest.Valid() {
		return fmt.Errorf("bundle has no valid identity digest")
	}
	want, err := DigestCanonical(b.Identity)
	if err != nil {
		return err
	}
	if want != b.IdentityDigest {
		return fmt.Errorf("bundle identity digest %s does not match its identity, which digests to %s", b.IdentityDigest.Short(), want.Short())
	}
	if strings.TrimSpace(b.JobID) == "" || strings.TrimSpace(b.AttemptID) == "" {
		return fmt.Errorf("bundle names no job or attempt")
	}
	if strings.TrimSpace(b.Worker.WorkerID) == "" {
		return fmt.Errorf("bundle names no worker")
	}
	switch b.Outcome {
	case "completed", "partial":
	default:
		return fmt.Errorf("bundle outcome %q is not completed or partial", b.Outcome)
	}
	if b.Outcome == "completed" && len(b.Summary) == 0 {
		return fmt.Errorf("a completed bundle carries its kind's summary")
	}
	if len(b.Summary) > 0 && !json.Valid(b.Summary) {
		return fmt.Errorf("bundle summary is not valid JSON")
	}
	seen := make(map[string]bool, len(b.Files))
	for i, f := range b.Files {
		if err := validRelativePath(f.Path); err != nil {
			return fmt.Errorf("bundle file %d: %w", i, err)
		}
		if path.Base(f.Path) == BundleManifestFile && path.Dir(f.Path) == "." {
			return fmt.Errorf("bundle lists its own manifest")
		}
		if seen[f.Path] {
			return fmt.Errorf("bundle lists %s twice", f.Path)
		}
		seen[f.Path] = true
		if f.Bytes < 0 {
			return fmt.Errorf("bundle file %s declares %d bytes", f.Path, f.Bytes)
		}
		if !f.SHA256.Valid() {
			return fmt.Errorf("bundle file %s has no valid sha256", f.Path)
		}
	}
	return nil
}

// Digest is the bundle's identity: the canonical form of its manifest.
// Uploading the same bundle twice yields the same digest, which is how the
// hub makes the upload idempotent.
func (b BundleManifest) Digest() (Digest, error) {
	if err := b.Validate(); err != nil {
		return "", err
	}
	return DigestCanonical(b)
}
