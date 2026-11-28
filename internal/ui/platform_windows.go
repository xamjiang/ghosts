package ui

import (
	tea "charm.land/bubbletea/v2"
	"golang.org/x/sys/windows"
	"os"
)

const (
	osName      = "Windows"
	systemLabel = "Windows"
	localOnly   = "This PC only"
)

func openURL(u string) (tea.Cmd, string, error) {
	if os.Getenv("SSH_CONNECTION") != "" {
		return tea.SetClipboard(u), "Copied", nil
	}
	target, err := windows.UTF16PtrFromString(u)
	if err != nil {
		return nil, "", err
	}
	return nil, "Opened", windows.ShellExecute(0, nil, target, nil, nil, windows.SW_SHOWNORMAL)
}
