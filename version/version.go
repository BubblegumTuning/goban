// Package version holds build-time version information injectable via -ldflags.
package version

// These are set at build time via -ldflags.
var (
	Version   = "1.2.0"
	BuildTime = ""
)
