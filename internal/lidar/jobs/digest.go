package jobs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Digest is a "sha256:" + lower hex string, the one form a content hash takes
// anywhere in this contract, so a bare hex string can never be mistaken for
// another algorithm's output.
type Digest string

const digestPrefix = "sha256:"

// DigestBytes hashes bytes.
func DigestBytes(b []byte) Digest {
	sum := sha256.Sum256(b)
	return Digest(digestPrefix + hex.EncodeToString(sum[:]))
}

// DigestFile hashes a file's contents and reports its size. A multi-gigabyte
// PCAP takes a real while; callers cache the answer against size, mtime and
// device and re-hash when any of them move.
func DigestFile(path string) (Digest, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, fmt.Errorf("hash %s: %w", path, err)
	}
	return Digest(digestPrefix + hex.EncodeToString(h.Sum(nil))), n, nil
}

// Valid reports whether d is a well-formed digest.
func (d Digest) Valid() bool {
	s := string(d)
	if !strings.HasPrefix(s, digestPrefix) || len(s) != len(digestPrefix)+64 {
		return false
	}
	_, err := hex.DecodeString(s[len(digestPrefix):])
	return err == nil
}

// Short is the first twelve hex characters, for logs and file names.
func (d Digest) Short() string {
	s := strings.TrimPrefix(string(d), digestPrefix)
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// canonicalJSON encodes v so that the same value always gives the same bytes:
// map keys sorted, no insignificant whitespace, no trailing newline. Struct
// fields encode in declaration order, which Go fixes; maps do not, which is
// why this exists. The document goes through a generic decode and re-encode
// rather than a custom encoder so that an int and a float that mean the same
// number, or a key order a caller happened to use, cannot change a digest.
func canonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	if err := dec.Decode(&generic); err != nil {
		return nil, err
	}
	var out strings.Builder
	if err := writeCanonical(&out, generic); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

func writeCanonical(w *strings.Builder, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		w.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				w.WriteByte(',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return err
			}
			w.Write(kb)
			w.WriteByte(':')
			if err := writeCanonical(w, t[k]); err != nil {
				return err
			}
		}
		w.WriteByte('}')
	case []any:
		w.WriteByte('[')
		for i, e := range t {
			if i > 0 {
				w.WriteByte(',')
			}
			if err := writeCanonical(w, e); err != nil {
				return err
			}
		}
		w.WriteByte(']')
	case json.Number:
		w.WriteString(t.String())
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		w.Write(b)
	}
	return nil
}

// DigestCanonical hashes the canonical JSON form of v.
func DigestCanonical(v any) (Digest, error) {
	b, err := canonicalJSON(v)
	if err != nil {
		return "", err
	}
	return DigestBytes(b), nil
}
