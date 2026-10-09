// Command ghosts shows which ports and Docker containers are running in the
// background, and lets you stop them.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/xamjiang/ghosts/internal/docker"
	"github.com/xamjiang/ghosts/internal/ports"
	"github.com/xamjiang/ghosts/internal/ui"
)

// version is set at release time with -ldflags "-X main.version=…".
var version = "dev"

func main() {
	interval := flag.Duration("interval", 2*time.Second, "how often to rescan ports")
	noMouse := flag.Bool("no-mouse", false, "don't capture the mouse (keeps native text selection)")
	list := flag.Bool("list", false, "print listening ports once and exit")
	all := flag.Bool("all", false, "with -list: include apps and macOS services")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("ghosts", version)
		return
	}
	if *list {
		if err := printList(*all); err != nil {
			fmt.Fprintln(os.Stderr, "ghosts:", err)
			os.Exit(1)
		}
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dc := docker.New()
	// The renderer wakes on every frame even when nothing changed; 30fps keeps
	// keys feeling instant while halving idle wakeups versus the default 60.
	p := tea.NewProgram(ui.New(ui.Options{Interval: *interval, Mouse: !*noMouse}, dc), tea.WithFPS(30))
	go ui.WatchDocker(ctx, dc, p.Send)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "ghosts:", err)
		os.Exit(1)
	}
}

func printList(all bool) error {
	snap, err := ports.NewScanner().Scan()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORT\tNAME\tPID\tMEM\tAVG POWER\tUPTIME\tPROJECT")
	for _, l := range snap.Listeners {
		if l.Kind != ports.KindDev && !all {
			continue
		}
		name := l.Name
		if l.Runtime != "" {
			name += " (" + l.Runtime + ")"
		}
		proj := strings.Replace(l.Project, home, "~", 1)
		fmt.Fprintf(w, "%d\t%s\t%d\t%.0f MB\t%.1f mW\t%s\t%s\n", l.Port, name, l.PID,
			float64(l.Usage.Mem)/1e6, l.Usage.AvgPower*1000, time.Since(l.Started).Round(time.Second), proj)
	}
	return w.Flush()
}
