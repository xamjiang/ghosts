package ui

import (
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

const (
	osName      = "macOS" // in "apps and macOS services"
	systemLabel = "macOS" // tags a system service in the details
	localOnly   = "This Mac only"
)

// openURL opens u in the default browser.
func openURL(u string) (tea.Cmd, string, error) {
	return nil, "Opened", exec.Command("open", u).Start()
}
