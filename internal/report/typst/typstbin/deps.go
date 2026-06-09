package typstbin

import (
	"os"
	"os/exec"
)

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

	cacheDirFunc       = cacheDir
	typstTargetFunc    = typstTarget
	httpDownloadFunc   = httpDownload
	extractTypstFunc   = extractTypst
	copyExecutableFunc = copyExecutable
	embeddedTypstFunc  = embeddedTypst
	cachedDownloadFunc = cachedDownload
)

func isExecutableMode(goos string, mode os.FileMode) bool {
	return goos == "windows" || mode&0o111 != 0
}

func exeSuffixFor(goos string) string {
	if goos == "windows" {
		return ".exe"
	}
	return ""
}
