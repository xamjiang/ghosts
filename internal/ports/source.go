package ports

import (
	"path/filepath"
	"runtime"
	"strings"
)

var launchers = map[string]string{
	"terminal": "Terminal", "iterm2": "iTerm2", "ghostty": "Ghostty", "warp": "Warp",
	"visual studio code": "VS Code", "code": "VS Code", "visual studio code - insiders": "VS Code Insiders",
	"cursor": "Cursor", "codex": "Codex", "claude": "Claude", "zed": "Zed",
	"wezterm-gui": "WezTerm", "alacritty": "Alacritty", "kitty": "kitty",
	"windowsterminal": "Windows Terminal", "devenv": "Visual Studio", "code - insiders": "VS Code Insiders",
	"powershell": "PowerShell", "pwsh": "PowerShell", "cmd": "Command Prompt",
	"gnome-terminal-server": "GNOME Terminal", "konsole": "Konsole",
}

func launchSource(pid int, t procTable, executable func(int) string) string {
	shell := ""
	for depth := 0; depth < 64; depth++ {
		parent := t.ppid[pid]
		if parent <= 1 || parent == pid || t.started[parent].After(t.started[pid]) {
			break
		}
		exe := executable(parent)
		for _, name := range []string{filepath.Base(exe), appName(exe, ""), t.comm[parent]} {
			name = strings.ToLower(name)
			if runtime.GOOS == "windows" {
				name = strings.TrimSuffix(name, ".exe")
			}
			if source := launchers[name]; source != "" {
				if runtime.GOOS == "windows" && (name == "powershell" || name == "pwsh" || name == "cmd") {
					if shell == "" {
						shell = source
					}
					continue
				}
				return source
			}
		}
		pid = parent
	}
	return shell
}
