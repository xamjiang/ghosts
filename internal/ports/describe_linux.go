package ports

import (
	"path/filepath"
	"strings"
)

// classify tells dev tools from desktop apps and system services by where
// their executable lives. Linux has no app bundles, so this goes by the
// conventional install locations of distro packages, snaps and flatpaks.
func classify(exe string) Kind {
	switch {
	case exe == "":
		return KindDev
	case hasAnyPrefix(exe, "/usr/lib/systemd/", "/lib/systemd/", "/usr/libexec/", "/usr/sbin/", "/sbin/"):
		return KindSystem
	}
	// Runtimes can live anywhere (/snap/node, /usr/lib/jvm); what they run is
	// the user's code.
	if _, ok := interpreters[strings.ToLower(filepath.Base(exe))]; ok || pythonVersioned.MatchString(filepath.Base(exe)) {
		return KindDev
	}
	switch {
	case strings.HasPrefix(exe, "/usr/") && !hasAnyPrefix(exe, "/usr/bin/", "/usr/local/"),
		// /usr/lib/firefox, /usr/share/code, /usr/NX…: packaged apps outside $PATH.
		hasAnyPrefix(exe, "/opt/", "/snap/", "/var/lib/flatpak/", "/app/", "/tmp/.mount_"),
		strings.Contains(exe, "/.local/share/flatpak/"):
		return KindApp
	}
	return KindDev
}
