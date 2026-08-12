//go:build !typst_embed

package server

// Non-release developer builds intentionally resolve Typst externally. The
// static release path always uses typst_embed and therefore compiles the real
// check in selfcheck_typst_embed.go.
func selfCheckTypst(_ *selfCheckReport) {}
