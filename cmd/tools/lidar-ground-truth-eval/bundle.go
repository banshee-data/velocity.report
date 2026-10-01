package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/perframeeval"
	"github.com/banshee-data/velocity.report/internal/version"
)

// The supervised pilot verification bundle (docs/plans/lidar-swift-physical-pose-plan.md
// section 9.2). A perframe run with -physical-bundle-dir retains the exact
// bytes of the physical-reference revision it scored, the split and pack
// manifest it bound, the options and command line it ran under, and its
// outputs, with a manifest that lists every file's SHA-256. verify-bundle
// re-reads the pinned inputs from where they were, refuses if any pin no
// longer holds or any bundle file was altered, and otherwise re-runs the
// scoring at the pinned physical revision and requires identical output.
//
// The comparison output carries no timestamps, so identical means byte for
// byte. The bundle manifest's created_utc and build are informational: the
// build that verifies may differ, and is reported, since the output check is
// what says the scoring still gives the same answer.
//
// The estimate databases are not byte-pinned: they are large, and a live
// database changes under WAL without its estimates changing. Their arm
// identities are recorded, and the re-run's identical output is the check.

const (
	bundleSchema        = "velocity.report/physical-verification-bundle"
	bundleSchemaVersion = 1
	bundleManifestFile  = "manifest.json"
	bundleJSONFile      = "comparison.json"
	bundleMarkdownFile  = "comparison.md"
	bundleSplitFile     = "split.json"
	bundlePackFile      = "pack-manifest.json"

	// The physical store's file layout, from annotation/physical_store.go,
	// which does not export it. A change there fails writeBundle loudly: no
	// candidate file will have the digest the scoring recorded.
	physicalHeadFile     = "physical-references.json"
	physicalRevisionFile = "physical-reference-revisions/%010d.json"
	packManifestFile     = "manifest.json"
)

type bundleManifest struct {
	Schema        string `json:"schema"`
	SchemaVersion int    `json:"schema_version"`
	// CreatedUTC and Build are informational, not pins.
	CreatedUTC string      `json:"created_utc"`
	Build      bundleBuild `json:"build"`
	// WorkingDir resolves the relative paths in Args.
	WorkingDir string   `json:"working_dir"`
	Args       []string `json:"args"`
	// Config is the comparison the arguments parsed to, with absolute paths.
	Config   json.RawMessage            `json:"config"`
	Pack     bundlePack                 `json:"pack"`
	Split    bundleSplit                `json:"split"`
	Physical bundlePhysical             `json:"physical"`
	Arms     []perframeeval.ArmIdentity `json:"arms"`
	Outputs  bundleOutputs              `json:"outputs"`
	Files    []bundleFile               `json:"files"`
}

type bundleBuild struct {
	Version   string `json:"version"`
	GitSHA    string `json:"git_sha"`
	BuildTime string `json:"build_time"`
	GoVersion string `json:"go_version"`
}

type bundlePack struct {
	Dir            string                      `json:"dir"`
	DatasetID      string                      `json:"dataset_id"`
	PackDigest     string                      `json:"pack_digest"`
	ManifestSHA256 string                      `json:"manifest_sha256"`
	ManifestFile   string                      `json:"manifest_file"`
	Source         annotation.SourceProvenance `json:"source"`
}

type bundleSplit struct {
	Path   string `json:"path"`
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	// SplitDigest and SplitRevision are set for a frozen split.
	SplitDigest   string `json:"split_digest,omitempty"`
	SplitRevision int    `json:"split_revision,omitempty"`
	// SidecarRevision and ReferenceDigest are what the split bound to.
	SidecarRevision int    `json:"sidecar_revision"`
	ReferenceDigest string `json:"reference_digest"`
}

type bundlePhysical struct {
	Revision      int    `json:"revision"`
	File          string `json:"file"`
	SHA256        string `json:"sha256"`
	ContentDigest string `json:"content_digest"`
	// ReferenceDigest is the physical reference identity's digest.
	ReferenceDigest string `json:"reference_digest"`
	// WasHead says the revision was the store's current file when bundled,
	// not yet in its history.
	WasHead bool `json:"was_head"`
}

type bundleOutputs struct {
	JSON     string `json:"json"`
	Markdown string `json:"markdown,omitempty"`
}

type bundleFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// sha256Hex is annotation's digest form: "sha256:" and lower hex.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// prepareBundleDir creates the bundle directory, or accepts an empty one. A
// bundle is never written over another's files.
func prepareBundleDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create bundle directory: %w", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read bundle directory: %w", err)
	}
	if len(entries) > 0 {
		return fmt.Errorf("bundle directory %s is not empty: name a new one", dir)
	}
	return nil
}

// absConfig resolves the comparison's relative paths against base.
func absConfig(cfg perframeeval.Config, base string) perframeeval.Config {
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	cfg.Reference.PackDir = abs(cfg.Reference.PackDir)
	cfg.Reference.SplitManifestPath = abs(cfg.Reference.SplitManifestPath)
	cfg.A.DBPath, cfg.B.DBPath = abs(cfg.A.DBPath), abs(cfg.B.DBPath)
	return cfg
}

// physicalRevisionBytes returns the bytes of the physical revision the
// scoring read: the archived revision or the current head, whichever has
// the digest it recorded. Neither means the store moved during the run.
func physicalRevisionBytes(packDir string, revision int, digest string) ([]byte, bool, error) {
	for _, c := range []struct {
		name string
		head bool
	}{{fmt.Sprintf(physicalRevisionFile, revision), false}, {physicalHeadFile, true}} {
		b, err := os.ReadFile(filepath.Join(packDir, c.name))
		if err == nil && sha256Hex(b) == digest {
			return b, c.head, nil
		}
	}
	return nil, false, fmt.Errorf("no file in %s holds physical reference revision %d as scored (%s): the store changed during the run; run again",
		packDir, revision, digest)
}

// writeBundle writes the bundle for a completed run. The manifest is written
// last, so a bundle interrupted part-way has none and never verifies.
func writeBundle(req *perFrameRequest, args []string, c *perframeeval.Comparison, payload, markdown []byte) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg := absConfig(req.cfg, wd)
	config, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode options: %w", err)
	}
	phys := c.Physical.Reference
	m := bundleManifest{
		Schema: bundleSchema, SchemaVersion: bundleSchemaVersion,
		CreatedUTC: time.Now().UTC().Format(time.RFC3339),
		Build:      currentBuild(),
		WorkingDir: wd, Args: append([]string{}, args...), Config: config,
		Arms:    []perframeeval.ArmIdentity{c.A.Arm, c.B.Arm},
		Outputs: bundleOutputs{JSON: bundleJSONFile},
	}

	files := map[string][]byte{bundleJSONFile: payload}
	if markdown != nil {
		files[bundleMarkdownFile], m.Outputs.Markdown = markdown, bundleMarkdownFile
	}

	refBytes, wasHead, err := physicalRevisionBytes(cfg.Reference.PackDir, phys.PhysicalRevision, phys.PhysicalRevisionDigest)
	if err != nil {
		return err
	}
	physFile := fmt.Sprintf("physical-reference-%010d.json", phys.PhysicalRevision)
	files[physFile] = refBytes
	m.Physical = bundlePhysical{Revision: phys.PhysicalRevision, File: physFile, SHA256: phys.PhysicalRevisionDigest,
		ContentDigest: phys.PhysicalContentDigest, ReferenceDigest: phys.Digest, WasHead: wasHead}

	splitBytes, err := os.ReadFile(cfg.Reference.SplitManifestPath)
	if err != nil {
		return fmt.Errorf("read split: %w", err)
	}
	if got := sha256Hex(splitBytes); got != c.Reference.SplitManifestDigest {
		return fmt.Errorf("split %s changed during the run (scored %s, now %s): run again", cfg.Reference.SplitManifestPath,
			c.Reference.SplitManifestDigest, got)
	}
	files[bundleSplitFile] = splitBytes
	m.Split = bundleSplit{Path: cfg.Reference.SplitManifestPath, File: bundleSplitFile, SHA256: c.Reference.SplitManifestDigest,
		SplitDigest: c.Reference.SplitDigest, SplitRevision: c.Reference.SplitRevision,
		SidecarRevision: c.Reference.SidecarRevision, ReferenceDigest: c.Reference.Digest}

	pack, err := annotation.OpenPack(cfg.Reference.PackDir)
	if err != nil {
		return fmt.Errorf("open pack: %w", err)
	}
	packBytes, err := os.ReadFile(filepath.Join(cfg.Reference.PackDir, packManifestFile))
	if err != nil {
		return fmt.Errorf("read pack manifest: %w", err)
	}
	if pack.Manifest.PackDigest != c.Reference.PackDigest {
		return fmt.Errorf("pack %s changed during the run (scored %s, now %s): run again", cfg.Reference.PackDir,
			c.Reference.PackDigest, pack.Manifest.PackDigest)
	}
	files[bundlePackFile] = packBytes
	m.Pack = bundlePack{Dir: cfg.Reference.PackDir, DatasetID: pack.Manifest.DatasetID, PackDigest: pack.Manifest.PackDigest,
		ManifestSHA256: sha256Hex(packBytes), ManifestFile: bundlePackFile, Source: pack.Manifest.Source}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(req.bundleDir, name), files[name], 0o644); err != nil {
			return err
		}
		m.Files = append(m.Files, bundleFile{Name: name, SHA256: sha256Hex(files[name]), Bytes: len(files[name])})
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("encode bundle manifest: %w", err)
	}
	return os.WriteFile(filepath.Join(req.bundleDir, bundleManifestFile), append(b, '\n'), 0o644)
}

func currentBuild() bundleBuild {
	return bundleBuild{Version: version.Version, GitSHA: version.GitSHA, BuildTime: version.BuildTime, GoVersion: runtime.Version()}
}

// errRefused marks a pin that no longer holds, as against a bundle that
// cannot be read at all.
var errRefused = errors.New("refused")

func refuse(format string, a ...any) error {
	return fmt.Errorf("%w: %s", errRefused, fmt.Sprintf(format, a...))
}

// runVerifyBundle is the verify-bundle subcommand.
func runVerifyBundle(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("lidar-ground-truth-eval verify-bundle", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("bundle", "", "verification bundle directory written by perframe -physical-bundle-dir (required)")
	fs.Usage = func() {
		fmt.Fprintf(stderr, "Usage: lidar-ground-truth-eval verify-bundle -bundle DIR\n\n")
		fmt.Fprintf(stderr, "Checks every pin of a physical verification bundle against the inputs where they were\n")
		fmt.Fprintf(stderr, "(physical-reference bytes and content, split, pack manifest and source, options), refuses\n")
		fmt.Fprintf(stderr, "any change or altered bundle file, then re-runs the scoring at the pinned physical\n")
		fmt.Fprintf(stderr, "revision and requires identical output.\n\nOptions:\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		return usageError(fs, stderr, fmt.Errorf("unexpected arguments: %v", fs.Args()))
	}
	if *dir == "" {
		return usageError(fs, stderr, fmt.Errorf("-bundle is required"))
	}
	m, err := verifyBundle(*dir, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "error: bundle %s: %v\n", *dir, err)
		return 1
	}
	fmt.Fprintf(stderr, "verified: bundle %s reproduces: physical revision %d (%s), split %s, pack %s\n",
		*dir, m.Physical.Revision, m.Physical.SHA256, m.Split.SHA256, m.Pack.PackDigest)
	return 0
}

// verifyBundle checks a bundle and re-runs it. Any error is a refusal.
func verifyBundle(dir string, stderr io.Writer) (*bundleManifest, error) {
	m, copies, err := readBundle(dir)
	if err != nil {
		return nil, err
	}
	if b := currentBuild(); b != m.Build {
		fmt.Fprintf(stderr, "note: bundle was written by build %+v; verifying with %+v\n", m.Build, b)
	}

	// The options: the recorded arguments must still parse to the recorded
	// comparison, so a changed default cannot move the scoring unseen.
	var parseErr bytes.Buffer
	req, _ := parsePerFrame(m.Args, &parseErr)
	if req == nil {
		return nil, refuse("the recorded arguments no longer parse: %s", parseErr.String())
	}
	cfg := absConfig(req.cfg, m.WorkingDir)
	got, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var want perframeeval.Config
	if err := json.Unmarshal(m.Config, &want); err != nil {
		return nil, fmt.Errorf("bundle options: %w", err)
	}
	if wantJSON, _ := json.Marshal(want); !bytes.Equal(got, wantJSON) {
		return nil, refuse("scoring options: the recorded arguments now parse to %s, not the pinned %s", got, wantJSON)
	}
	if cfg.Physical == nil {
		return nil, refuse("the recorded command did not score physical references")
	}
	if cfg.Reference.PackDir != m.Pack.Dir || cfg.Reference.SplitManifestPath != m.Split.Path {
		return nil, refuse("the recorded arguments name pack %s and split %s, not the pinned %s and %s",
			cfg.Reference.PackDir, cfg.Reference.SplitManifestPath, m.Pack.Dir, m.Split.Path)
	}

	// The inputs, where they were.
	split, err := os.ReadFile(m.Split.Path)
	if err != nil {
		return nil, refuse("split %s cannot be read: %v", m.Split.Path, err)
	}
	if d := sha256Hex(split); d != m.Split.SHA256 {
		return nil, refuse("split %s changed (pinned %s, now %s)", m.Split.Path, m.Split.SHA256, d)
	}
	packManifest, err := os.ReadFile(filepath.Join(m.Pack.Dir, packManifestFile))
	if err != nil {
		return nil, refuse("pack manifest in %s cannot be read: %v", m.Pack.Dir, err)
	}
	if d := sha256Hex(packManifest); d != m.Pack.ManifestSHA256 {
		return nil, refuse("pack manifest %s changed (pinned %s, now %s)", filepath.Join(m.Pack.Dir, packManifestFile), m.Pack.ManifestSHA256, d)
	}
	pack, err := annotation.OpenPack(m.Pack.Dir)
	if err != nil {
		return nil, refuse("pack %s cannot be opened: %v", m.Pack.Dir, err)
	}
	if pack.Manifest.PackDigest != m.Pack.PackDigest || pack.Manifest.DatasetID != m.Pack.DatasetID {
		return nil, refuse("pack digest %s / dataset %s, pinned %s / %s", pack.Manifest.PackDigest, pack.Manifest.DatasetID,
			m.Pack.PackDigest, m.Pack.DatasetID)
	}
	if !reflect.DeepEqual(pack.Manifest.Source, m.Pack.Source) {
		return nil, refuse("pack source identity %+v, pinned %+v", pack.Manifest.Source, m.Pack.Source)
	}
	doc, err := annotation.LoadPhysicalReferenceRevision(pack, m.Physical.Revision)
	if err != nil {
		return nil, refuse("physical reference revision %d cannot be read from %s: %v", m.Physical.Revision, m.Pack.Dir, err)
	}
	if doc.Digest() != m.Physical.SHA256 {
		return nil, refuse("physical reference revision %d bytes changed (pinned %s, now %s)", m.Physical.Revision, m.Physical.SHA256, doc.Digest())
	}
	if content, err := doc.ContentDigest(); err != nil || content != m.Physical.ContentDigest {
		return nil, refuse("physical reference revision %d content digest %s, pinned %s (%v)", m.Physical.Revision, content, m.Physical.ContentDigest, err)
	}

	// The re-run, at the pinned revision whatever the head is now.
	cfg.Physical.Revision = m.Physical.Revision
	c, err := perframeeval.Run(cfg)
	if err != nil {
		return nil, refuse("re-run failed: %v", err)
	}
	for _, pin := range []struct {
		what      string
		got, want any
	}{
		{"annotation (sidecar) revision", c.Reference.SidecarRevision, m.Split.SidecarRevision},
		{"membership reference digest", c.Reference.Digest, m.Split.ReferenceDigest},
		{"physical reference digest", c.Physical.Reference.Digest, m.Physical.ReferenceDigest},
		{"arm identities", []perframeeval.ArmIdentity{c.A.Arm, c.B.Arm}, m.Arms},
	} {
		if !reflect.DeepEqual(pin.got, pin.want) {
			return nil, refuse("re-run %s is %v, pinned %v", pin.what, pin.got, pin.want)
		}
	}
	payload, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(append(payload, '\n'), copies[m.Outputs.JSON]) {
		return nil, refuse("re-run comparison JSON differs from %s", m.Outputs.JSON)
	}
	if m.Outputs.Markdown != "" && !bytes.Equal([]byte(perframeeval.RenderMarkdown(*c)), copies[m.Outputs.Markdown]) {
		return nil, refuse("re-run comparison Markdown differs from %s", m.Outputs.Markdown)
	}
	return m, nil
}

// readBundle reads the manifest and every file it lists, and refuses an
// altered, missing or unlisted file, or a manifest whose pins do not name
// its own copies.
func readBundle(dir string) (*bundleManifest, map[string][]byte, error) {
	raw, err := os.ReadFile(filepath.Join(dir, bundleManifestFile))
	if err != nil {
		return nil, nil, fmt.Errorf("read bundle manifest: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var m bundleManifest
	if err := dec.Decode(&m); err != nil {
		return nil, nil, fmt.Errorf("parse bundle manifest: %w", err)
	}
	if m.Schema != bundleSchema || m.SchemaVersion != bundleSchemaVersion {
		return nil, nil, fmt.Errorf("bundle manifest is %s version %d, want %s version %d", m.Schema, m.SchemaVersion,
			bundleSchema, bundleSchemaVersion)
	}

	listed := map[string]string{}
	copies := map[string][]byte{}
	for _, f := range m.Files {
		if f.Name != filepath.Base(f.Name) || f.Name == bundleManifestFile || listed[f.Name] != "" {
			return nil, nil, refuse("bundle manifest lists file %q, which is not a plain, unique bundle file name", f.Name)
		}
		b, err := os.ReadFile(filepath.Join(dir, f.Name))
		if err != nil {
			return nil, nil, refuse("bundle file %s is missing: %v", f.Name, err)
		}
		if d := sha256Hex(b); d != f.SHA256 || len(b) != f.Bytes {
			return nil, nil, refuse("bundle file %s was altered (listed %s, %d bytes; now %s, %d bytes)", f.Name, f.SHA256, f.Bytes, d, len(b))
		}
		listed[f.Name], copies[f.Name] = f.SHA256, b
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, err
	}
	for _, e := range entries {
		if name := e.Name(); name != bundleManifestFile && listed[name] == "" {
			return nil, nil, refuse("bundle holds %s, which its manifest does not list", name)
		}
	}
	for _, pin := range []struct{ what, file, digest string }{
		{"physical reference", m.Physical.File, m.Physical.SHA256},
		{"split", m.Split.File, m.Split.SHA256},
		{"pack manifest", m.Pack.ManifestFile, m.Pack.ManifestSHA256},
		{"comparison JSON", m.Outputs.JSON, listed[m.Outputs.JSON]},
	} {
		if pin.digest == "" || listed[pin.file] != pin.digest {
			return nil, nil, refuse("the %s pin %s does not name the bundle's copy %s (%s)", pin.what, pin.digest, pin.file, listed[pin.file])
		}
	}
	if m.Outputs.Markdown != "" && listed[m.Outputs.Markdown] == "" {
		return nil, nil, refuse("the bundle's Markdown output %s is not listed", m.Outputs.Markdown)
	}
	return &m, copies, nil
}
