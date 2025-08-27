package radar

import "embed"

//go:embed static/*
var StaticFiles embed.FS

//go:embed web/build/*
var WebBuildFiles embed.FS
