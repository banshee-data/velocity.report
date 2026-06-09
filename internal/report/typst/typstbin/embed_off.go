//go:build !typst_embed

package typstbin

// embeddedTypst returns no binary in the default build. The typst executable
// is resolved from PATH or the development downloader instead. Build with
// `-tags typst_embed` (after placing a platform binary at dist/typst via
// `make install-typst-dist`) to embed the binary into the program.
func embeddedTypst() ([]byte, bool) { return nil, false }
