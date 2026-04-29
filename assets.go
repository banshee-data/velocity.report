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
