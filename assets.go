package radar

import "embed"

//go:embed static/*
var StaticFiles embed.FS

//go:embed web/build/*
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
