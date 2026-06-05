//go:build linux

package ctl

import (
	"os"

	"golang.org/x/sys/unix"
)

// swapCurrent atomically repoints linkPath at newTarget. It writes a temporary
// sibling symlink and swaps it into place with renameat2(RENAME_EXCHANGE)
// (Linux 3.15+), so there is never a window where linkPath is missing. The
// temporary symlink ends up pointing at the prior target and is removed.
func swapCurrent(linkPath, newTarget string) error {
	tmp := linkPath + ".new"
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(newTarget, tmp); err != nil {
		return err
	}
	if err := unix.Renameat2(unix.AT_FDCWD, tmp, unix.AT_FDCWD, linkPath, unix.RENAME_EXCHANGE); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Remove(tmp)
}
