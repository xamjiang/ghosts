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
