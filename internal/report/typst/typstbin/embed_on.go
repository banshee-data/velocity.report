//go:build typst_embed

package typstbin

import _ "embed"

// typstBinary is the platform-specific typst executable, embedded into the
// program. The binary is downloaded to dist/typst by `make install-typst-dist`
// for the target GOOS/GOARCH before building with `-tags typst_embed`; it is
// gitignored and never committed.
//
//go:embed dist/typst
var typstBinary []byte

func embeddedTypst() ([]byte, bool) { return typstBinary, len(typstBinary) > 0 }
