package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

// sourceManifest is the small, immutable control record for a Phase 0/1
// corpus. It deliberately names files relative to the LiDAR root while the
// raw PCAPs and their large derived SQLite artefacts remain outside Git.
type sourceManifest struct {
	SchemaVersion    int                  `json:"schema_version"`
	CorpusSHA256     string               `json:"corpus_sha256"`
	IndexSHA256      string               `json:"index_sha256"`
	SensorID         string               `json:"sensor_id"`
	ParametersSHA256 string               `json:"parameters_sha256"`
	CalibrationID    string               `json:"calibration_id"`
	Cases            []sourceManifestCase `json:"cases"`
}

type sourceManifestCase struct {
	ID       string                  `json:"id"`
	SourceID string                  `json:"source_id"`
	Captures []sourceManifestCapture `json:"captures"`
}

type sourceManifestCapture struct {
	Ordinal      int    `json:"ordinal"`
	RelativePath string `json:"relative_path"`
	ByteSize     int64  `json:"byte_size"`
	SHA256       string `json:"sha256"`
}

type resolvedCorpusCase struct {
	corpusCase corpusCase
	paths      []string
}

func resolveCorpusCases(selected corpus, index map[string]indexEntry, pcapRoot, pcapSubdir string) ([]resolvedCorpusCase, error) {
	resolved := make([]resolvedCorpusCase, 0, len(selected.Cases))
	for _, selectedCase := range selected.Cases {
		entry, ok := index[selectedCase.ID]
		if !ok {
			return nil, fmt.Errorf("corpus case %q is absent from index", selectedCase.ID)
		}
		if len(entry.Captures) != selectedCase.ExpectedCaptureCount {
			return nil, fmt.Errorf("corpus case %q declares %d captures, index has %d", selectedCase.ID, selectedCase.ExpectedCaptureCount, len(entry.Captures))
		}
		paths := make([]string, len(entry.Captures))
		for i, capture := range entry.Captures {
			paths[i] = filepath.Join(pcapRoot, pcapSubdir, capture)
			info, err := os.Stat(paths[i])
			if err != nil {
				return nil, fmt.Errorf("case %s capture %s: %w", selectedCase.ID, paths[i], err)
			}
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("case %s capture %s is not a regular file", selectedCase.ID, paths[i])
			}
		}
		resolved = append(resolved, resolvedCorpusCase{corpusCase: selectedCase, paths: paths})
	}
	return resolved, nil
}

func buildSourceManifest(corpusPath, indexPath, pcapRoot, tuningPath, sensorID string, cases []resolvedCorpusCase) (sourceManifest, error) {
	corpusDigest, err := fileSHA256(corpusPath)
	if err != nil {
		return sourceManifest{}, fmt.Errorf("hash corpus declaration: %w", err)
	}
	indexDigest, err := fileSHA256(indexPath)
	if err != nil {
		return sourceManifest{}, fmt.Errorf("hash source index: %w", err)
	}
	paramsHash, err := replayParametersSHA256(tuningPath)
	if err != nil {
		return sourceManifest{}, err
	}
	calibrationID, err := l4bobserve.CalibrationID(identityCalibration(sensorID))
	if err != nil {
		return sourceManifest{}, fmt.Errorf("derive observation calibration identity: %w", err)
	}

	manifest := sourceManifest{
		SchemaVersion:    1,
		CorpusSHA256:     corpusDigest,
		IndexSHA256:      indexDigest,
		SensorID:         sensorID,
		ParametersSHA256: paramsHash,
		CalibrationID:    calibrationID,
		Cases:            make([]sourceManifestCase, 0, len(cases)),
	}
	for _, resolved := range cases {
		captures := make([]sourceManifestCapture, 0, len(resolved.paths))
		rawHashes := make([]string, 0, len(resolved.paths))
		for ordinal, path := range resolved.paths {
			info, err := os.Stat(path)
			if err != nil {
				return sourceManifest{}, fmt.Errorf("stat %s: %w", path, err)
			}
			digest, err := fileSHA256(path)
			if err != nil {
				return sourceManifest{}, fmt.Errorf("hash %s: %w", path, err)
			}
			relative, err := filepath.Rel(pcapRoot, path)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return sourceManifest{}, fmt.Errorf("capture %s is outside LiDAR root %s", path, pcapRoot)
			}
			raw, err := sourceManifestRawSHA256(digest)
			if err != nil {
				return sourceManifest{}, err
			}
			rawHashes = append(rawHashes, raw)
			captures = append(captures, sourceManifestCapture{
				Ordinal: ordinal, RelativePath: filepath.ToSlash(relative), ByteSize: info.Size(), SHA256: digest,
			})
		}
		sourceID, err := l4bobserve.SourceID(l4bobserve.CaptureSource{
			ReplayCaseID: resolved.corpusCase.ID, CapturePaths: resolved.paths, CaptureSHA256s: rawHashes,
			ExtractorID: "l4.dbscan_xy/v1/" + paramsHash,
		})
		if err != nil {
			return sourceManifest{}, fmt.Errorf("derive observation source identity for %s: %w", resolved.corpusCase.ID, err)
		}
		manifest.Cases = append(manifest.Cases, sourceManifestCase{ID: resolved.corpusCase.ID, SourceID: sourceID, Captures: captures})
	}
	return manifest, nil
}

func replayParametersSHA256(tuningPath string) (string, error) {
	if tuningPath == "" {
		tuningPath = config.DefaultConfigPath
	}
	tuning, err := config.LoadTuningConfigOrEmbedded(tuningPath, radarassets.TuningDefaults)
	if err != nil {
		return "", fmt.Errorf("load tuning config %s: %w", tuningPath, err)
	}
	payload, err := json.Marshal(tuning)
	if err != nil {
		return "", fmt.Errorf("marshal tuning config for source manifest: %w", err)
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func writeSourceManifest(path string, manifest sourceManifest) (string, error) {
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal source manifest: %w", err)
	}
	payload = append(payload, '\n')
	sum := sha256.Sum256(payload)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("create source manifest %s: %w", path, err)
	}
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return "", fmt.Errorf("write source manifest %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("close source manifest %s: %w", path, err)
	}
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}

func sourceManifestRawSHA256(digest string) (string, error) {
	const prefix = "sha256:"
	if !strings.HasPrefix(digest, prefix) || len(digest) != len(prefix)+sha256.Size*2 {
		return "", fmt.Errorf("invalid SHA-256 digest %q", digest)
	}
	return strings.TrimPrefix(digest, prefix), nil
}
