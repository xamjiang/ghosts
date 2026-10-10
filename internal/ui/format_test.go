package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestAgo(t *testing.T) {
	cases := map[time.Duration]string{
		5 * time.Second:               "5s",
		12 * time.Minute:              "12m",
		2*time.Hour + 13*time.Minute:  "2h 13m",
		3 * time.Hour:                 "3h",
		34 * time.Hour:                "1d 10h",
		15*24*time.Hour + 2*time.Hour: "15d",
	}
	for d, want := range cases {
		if got := ago(d); got != want {
			t.Errorf("ago(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestDockerUptime(t *testing.T) {
	cases := map[string]string{
		"Up 2 hours (healthy)":  "2h",
		"Up 34 hours":           "1d 10h",
		"Up About a minute":     "1m",
		"Up 3 days":             "3d",
		"Up 2 weeks":            "14d",
		"Up Less than a second": "0s",
	}
	for in, want := range cases {
		if got := dockerUptime(in); got != want {
			t.Errorf("dockerUptime(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFitLeft(t *testing.T) {
	got := fitLeft("~/Documents/Project/app", 10)
	if ansi.StringWidth(got) != 10 || !strings.HasPrefix(got, "…") || !strings.HasSuffix(got, "/app") {
		t.Errorf("fitLeft = %q", got)
	}
	if got := fitLeft("short", 10); got != "short" {
		t.Errorf("fitLeft = %q", got)
	}
}

func TestPlural(t *testing.T) {
	cases := []struct {
		n          int
		word, want string
	}{
		{1, "container", "1 container"},
		{3, "container", "3 containers"},
		{1, "child process", "1 child process"},
		{3, "child process", "3 child processes"},
	}
	for _, c := range cases {
		if got := plural(c.n, c.word); got != c.want {
			t.Errorf("plural(%d, %q) = %q, want %q", c.n, c.word, got, c.want)
		}
	}
}

func TestWrapCommand(t *testing.T) {
	cases := []struct {
		s    string
		w    int
		want []string
	}{
		{"node vite --port 5173", 40, []string{"node vite --port 5173"}},
		{"node /a/node_modules/.bin/vite --port 5173", 33, []string{"node /a/node_modules/.bin/vite", "--port 5173"}},
		{"python3 -m uvicorn app.main:app", 12, []string{"python3 -m", "uvicorn", "app.main:app"}},
		{"/usr/local/bin/x", 6, []string{"/usr/", "local/", "bin/x"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		{"", 10, []string{""}},
	}
	for _, c := range cases {
		got := wrapCommand(c.s, c.w)
		if strings.Join(got, "|") != strings.Join(c.want, "|") {
			t.Errorf("wrapCommand(%q, %d) = %q, want %q", c.s, c.w, got, c.want)
		}
	}
}
