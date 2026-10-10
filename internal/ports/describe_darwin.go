package ports

import "strings"

// classify tells dev tools from macOS apps and system services by where
// their executable lives.
func classify(exe string) Kind {
	switch {
	case exe == "":
		return KindDev
	case hasAnyPrefix(exe, "/System/", "/usr/libexec/", "/usr/sbin/", "/sbin/", "/Library/Apple/"):
		return KindSystem
	case strings.Contains(exe, ".framework/"):
		// Python.framework and friends ship an inner .app; they're still dev tools.
		return KindDev
	case hasAnyPrefix(exe, "/Applications/", "/Library/Application Support/"),
		strings.Contains(exe, "/Library/Application Support/"),
		strings.Contains(exe, "/Applications/"),
		strings.Contains(exe, ".app/Contents/"):
		return KindApp
	}
	return KindDev
}
