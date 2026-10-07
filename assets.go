package radar

import "embed"

// WebBuildFiles is the SvelteKit build served under /app, and the source of
// /favicon.ico. It is the only web content embedded. The all: prefix
// matters: without it Go skips every file below web/build whose name starts
// with "_" or ".", and SvelteKit can name a chunk after a content hash that
// begins with "_". Such a file would be missing from the binary and 404 in
// production while the --debug server, which reads web/build from disk,
// served it fine.
//
//go:embed all:web/build
var WebBuildFiles embed.FS

//go:embed all:docs_html/_site
var DocsSiteFiles embed.FS

//go:embed docs_html/stub-index.html
var DocsSiteStub []byte

// TuningDefaults is the canonical tuning configuration, embedded so the shipped
// image carries no separate on-disk tuning.defaults.json. The server falls back
// to these bytes when no --config file is present (see
// config.LoadTuningConfigOrEmbedded).
//
//go:embed config/tuning.defaults.json
var TuningDefaults []byte

// SegmentSelectorDefaults is the canonical segment selector file, embedded
// for the same reason: the server and the segments command read these bytes
// when no selector file is found on disk (see
// segments.LoadSelectorsOrEmbedded).
//
//go:embed config/segment-selectors.defaults.json
var SegmentSelectorDefaults []byte
