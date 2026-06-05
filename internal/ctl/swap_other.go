//go:build !linux

package ctl

import "os"

// swapCurrent repoints linkPath at newTarget. This non-atomic fallback (remove
// then symlink) is used on non-Linux dev and test hosts; production runs on
// Linux, where swap_linux.go provides the atomic renameat2 implementation.
func swapCurrent(linkPath, newTarget string) error {
	if err := os.Remove(linkPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(newTarget, linkPath)
}
