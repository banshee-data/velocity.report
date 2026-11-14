package experiments

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CreateTemp is os.CreateTemp without O_EXCL. NFSv3 implements an exclusive
// create as a create followed by a SETATTR, and the NAS that holds the
// evidence share refuses the SETATTR, so every os.CreateTemp there fails. A
// random name that is checked for an existing file is as safe here: one
// worker writes an evidence root at a time under its kernel lock. The pattern
// follows os.CreateTemp: the last * is replaced by the random string.
func CreateTemp(dir, pattern string) (*os.File, error) {
	prefix, suffix := pattern, ""
	if i := strings.LastIndex(pattern, "*"); i >= 0 {
		prefix, suffix = pattern[:i], pattern[i+1:]
	}
	for try := 0; try < 100; try++ {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, err
		}
		name := filepath.Join(dir, prefix+hex.EncodeToString(b[:])+suffix)
		if _, err := os.Lstat(name); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		return os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o600)
	}
	return nil, fmt.Errorf("no unused temporary name in %s", dir)
}
