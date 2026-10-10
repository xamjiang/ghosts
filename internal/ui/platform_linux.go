package ui

import (
	"os"
	"os/exec"

	tea "charm.land/bubbletea/v2"
)

const (
	osName      = "system"
	systemLabel = "System"
	localOnly   = "This machine only"
)

// openURL opens u in the default browser. Over SSH, or without a display,
// the browser would open on the remote machine (or nowhere), so the URL is
// copied to the user's clipboard through the terminal instead (OSC 52).
func openURL(u string) (tea.Cmd, string, error) {
	remote := os.Getenv("SSH_CONNECTION") != ""
	if remote || os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return tea.SetClipboard(u), "Copied", nil
	}
	return nil, "Opened", exec.Command("xdg-open", u).Start()
}
