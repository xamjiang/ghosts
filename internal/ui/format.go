package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"
)

var home, _ = os.UserHomeDir()

// tilde shortens paths under the home directory.
func tilde(p string) string {
	if home != "" && (p == home || strings.HasPrefix(p, home+"/")) {
		return "~" + p[len(home):]
	}
	return p
}

// ago renders a compact duration: 45s, 12m, 2h 13m, 3d 4h.
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(0, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		h := int(d.Hours())
		if m := int(d.Minutes()) % 60; m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	if h := int(d.Hours()) % 24; h > 0 && days < 10 {
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}

// dockerUptime turns Docker's "Up 2 hours (healthy)" into the same compact
// form used for processes, so both tabs read alike.
func dockerUptime(status string) string {
	s := strings.TrimPrefix(status, "Up ")
	if i := strings.Index(s, " ("); i >= 0 {
		s = s[:i]
	}
	switch s {
	case "Less than a second":
		return "0s"
	case "About a minute":
		return "1m"
	case "About an hour":
		return "1h"
	}
	f := strings.Fields(s)
	if len(f) != 2 {
		return s
	}
	n, err := strconv.Atoi(f[0])
	if err != nil {
		return s
	}
	unit := map[string]time.Duration{
		"second": time.Second, "minute": time.Minute, "hour": time.Hour,
		"day": 24 * time.Hour, "week": 7 * 24 * time.Hour,
		"month": 30 * 24 * time.Hour, "year": 365 * 24 * time.Hour,
	}
	if u, ok := unit[strings.TrimSuffix(f[1], "s")]; ok {
		return ago(time.Duration(n) * u)
	}
	return s
}

// fit truncates (with an ellipsis) or pads s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
}

// fitRight is fit with the text aligned to the right edge.
func fitRight(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return strings.Repeat(" ", w-ansi.StringWidth(s)) + s
}

// fitLeft keeps the end of s, which is the informative part of a path.
func fitLeft(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if n := ansi.StringWidth(s); n > w {
		s = "…" + ansi.TruncateLeft(s, n-w+1, "")
	}
	return s
}

// splitPath splits a display path into its parent (with trailing slash) and
// last element, so the last element can be emphasised.
func splitPath(p string) (string, string) {
	if p == "" || p == "~" || p == "/" {
		return "", p
	}
	dir, base := filepath.Split(p)
	return dir, base
}

// bytesText follows Activity Monitor: binary units, labelled MB / GB.
func bytesText(b uint64) string {
	mb := float64(b) / (1 << 20)
	switch {
	case mb < 10:
		return fmt.Sprintf("%.1f MB", mb)
	case mb < 1000:
		return fmt.Sprintf("%.0f MB", mb)
	}
	return fmt.Sprintf("%.1f GB", mb/1024)
}

func cpuText(p float64) string {
	if p < 10 {
		return fmt.Sprintf("%.1f%%", p)
	}
	return fmt.Sprintf("%.0f%%", p)
}

func powerText(w float64) string {
	if w > 0 && w < 0.0005 {
		return "<1 mW"
	}
	if w < 1 {
		return fmt.Sprintf("%.0f mW", w*1000)
	}
	return fmt.Sprintf("%.1f W", w)
}

// wrapCommand wraps a command line to w cells, breaking after spaces and
// slashes. Unlike ansi.Wrap it never breaks at a hyphen, which would split
// flags like "--port" across lines; a word longer than a line is cut.
func wrapCommand(s string, w int) []string {
	if w < 1 {
		return []string{s}
	}
	var lines []string
	var cur []rune
	flush := func() {
		lines = append(lines, strings.TrimRight(string(cur), " "))
		cur = cur[:0]
	}
	rs := []rune(s)
	for len(rs) > 0 {
		// The next piece runs up to and including a breakpoint.
		n := 0
		for n < len(rs) && rs[n] != ' ' && rs[n] != '/' {
			n++
		}
		n = min(n+1, len(rs))
		piece := rs[:n]
		rs = rs[n:]
		for len(piece) > 0 {
			room := w - len(cur)
			fits := len(piece)
			if piece[len(piece)-1] == ' ' {
				fits-- // a trailing space may hang past the edge
			}
			switch {
			case fits <= room:
				cur = append(cur, piece...)
				piece = nil
			case len(cur) > 0:
				flush()
			default:
				cur = append(cur, piece[:w]...)
				piece = piece[w:]
				flush()
			}
		}
	}
	if len(cur) > 0 || len(lines) == 0 {
		flush()
	}
	return lines
}
