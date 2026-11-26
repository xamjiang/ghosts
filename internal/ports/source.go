package ports

import (
	"path/filepath"
	"strings"
)

var launchers = map[string]string{
	"terminal": "Terminal", "iterm2": "iTerm2", "ghostty": "Ghostty", "warp": "Warp",
	"visual studio code": "VS Code", "code": "VS Code", "visual studio code - insiders": "VS Code Insiders",
	"cursor": "Cursor", "codex": "Codex", "claude": "Claude", "zed": "Zed",
	"wezterm-gui": "WezTerm", "alacritty": "Alacritty", "kitty": "kitty",
	"gnome-terminal-server": "GNOME Terminal", "konsole": "Konsole",
}

func launchSource(pid int, t procTable, executable func(int) string) string {
	for depth := 0; depth < 64; depth++ {
		parent := t.ppid[pid]
		if parent <= 1 || parent == pid || t.started[parent].After(t.started[pid]) {
			break
		}
		exe := executable(parent)
		for _, name := range []string{filepath.Base(exe), appName(exe, ""), t.comm[parent]} {
			if source := launchers[strings.ToLower(name)]; source != "" {
				return source
			}
		}
		pid = parent
	}
	return ""
}
