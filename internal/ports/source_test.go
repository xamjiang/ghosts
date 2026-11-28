package ports

import (
	"testing"
	"time"
)

func TestLaunchSource(t *testing.T) {
	table := newProcTable()
	start := time.Unix(100, 0)
	table.add(2, 1, start, "Electron", true)
	table.add(3, 2, start, "zsh", true)
	table.add(4, 3, start, "node", true)
	exes := map[int]string{2: "/Applications/Visual Studio Code.app/Contents/MacOS/Electron"}
	path := func(pid int) string { return exes[pid] }
	if got := launchSource(4, table, path); got != "VS Code" {
		t.Fatalf("editor: %q", got)
	}
	exes[3] = "/usr/local/bin/codex"
	if got := launchSource(4, table, path); got != "Codex" {
		t.Fatalf("nearest: %q", got)
	}
	table.ppid[4] = 1
	if got := launchSource(4, table, path); got != "" {
		t.Fatalf("detached: %q", got)
	}
	table.ppid[4] = 3
	table.started[3] = start.Add(time.Minute)
	if got := launchSource(4, table, path); got != "" {
		t.Fatalf("reused parent PID: %q", got)
	}
}
