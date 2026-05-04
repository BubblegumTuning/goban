// Package version holds build-time version information injectable via -ldflags.
package version

// These are set at build time via -ldflags.
var (
	Version   = "dev"
	BuildTime = ""
)
