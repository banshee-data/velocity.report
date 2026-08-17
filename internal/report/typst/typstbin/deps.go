package typstbin

import (
	"os"
	"os/exec"
)

type tempExecutable interface {
	Name() string
	Write([]byte) (int, error)
	Chmod(os.FileMode) error
	Close() error
}

var (
	osGetenv       = os.Getenv
	osStat         = os.Stat
	execLookPath   = exec.LookPath
	osUserCacheDir = os.UserCacheDir
	osMkdirAll     = os.MkdirAll
	osMkdirTemp    = os.MkdirTemp
	osCreateTemp   = os.CreateTemp
	osRename       = os.Rename
	osChmod        = os.Chmod
	createTempExec = defaultCreateTempExec

	cacheDirFunc       = cacheDir
	typstTargetFunc    = typstTarget
	httpDownloadFunc   = httpDownload
	extractTypstFunc   = extractTypst
	copyExecutableFunc = copyExecutable
	embeddedTypstFunc  = embeddedTypst
	cachedDownloadFunc = cachedDownload
)

// defaultCreateTempExec is the production implementation behind the
// createTempExec seam. It is a named function rather than an inline closure so
// tests can exercise it directly: every test that touches createTempExec
// replaces it, which would otherwise leave the real one unexercised.
func defaultCreateTempExec(dir, pattern string) (tempExecutable, error) {
	return osCreateTemp(dir, pattern)
}

func isExecutableMode(goos string, mode os.FileMode) bool {
	return goos == "windows" || mode&0o111 != 0
}

func exeSuffixFor(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}
