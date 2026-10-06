//go:build !unix

package runner

func freeBytes(string) uint64 { return 0 }
