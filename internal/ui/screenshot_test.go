package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/xamjiang/ghosts/internal/docker"
	"github.com/xamjiang/ghosts/internal/ports"
)

// TestScreenshots renders the README's screenshots from a made-up workspace,
// so they never show the processes of whoever generates them. It only runs
// when asked to, with an absolute output directory, and should be run on
// macOS, whose labels and power column the images show:
//
//	GHOSTS_SCREENSHOTS=$PWD/docs/images go test -run TestScreenshots ./internal/ui
func TestScreenshots(t *testing.T) {
	dir := os.Getenv("GHOSTS_SCREENSHOTS")
	if dir == "" {
		t.Skip("set GHOSTS_SCREENSHOTS to a directory to render screenshots")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	home = "/Users/you"
	defer func() { home, _ = os.UserHomeDir() }()

	press := func(m *Model, keys ...string) {
		for _, k := range keys {
			msg := tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
			switch k {
			case "down":
				msg = tea.KeyPressMsg{Code: tea.KeyDown}
			case "tab":
				msg = tea.KeyPressMsg{Code: tea.KeyTab}
			}
			m.update(msg)
		}
	}
	demo := func(w, h int) *Model {
		m := New(Options{}, docker.New())
		m.update(tea.WindowSizeMsg{Width: w, Height: h})
		m.update(dockerMsg{cs: demoContainers()})
		m.update(portsMsg{snap: demoSnapshot(false)})
		// A second scan finds the server an agent just started.
		m.update(portsMsg{snap: demoSnapshot(true)})
		m.switchTab(tabPorts)
		return &m
	}
	type recorder struct {
		frames []svgFrame
		named  map[string]int
	}
	shot := func(r *recorder, m *Model, name string, hold time.Duration) {
		if r.named == nil {
			r.named = map[string]int{}
		}
		r.named[name] = len(r.frames)
		r.frames = append(r.frames, svgFrame{grid: parseANSI(m.render()), hold: hold})
	}
	// stop selects the forgotten vite server and stops it.
	stop := func(r *recorder, m *Model) {
		press(m, "down")
		shot(r, m, "detached", 2400*time.Millisecond)
		press(m, "x")
		shot(r, m, "confirm", 1800*time.Millisecond)
		// "y" would really stop the process; show what the user sees instead.
		victim := m.selected().l
		m.confirm = nil
		m.busy[victim.Key()] = "Stopping"
		m.spinning = true
		for range 4 {
			shot(r, m, "stopping", 150*time.Millisecond)
			m.frame += 2
		}
		m.update(actionMsg{keys: []string{victim.Key()}, ok: fmt.Sprintf("Stopped %s · port %d is free", victim.Name, victim.Port)})
		m.update(portsMsg{snap: without(demoSnapshot(true), victim.Port)})
		shot(r, m, "stopped", 2200*time.Millisecond)
	}

	// The tour: the full window, details panel and all.
	const w, h = 160, 28
	var tour recorder
	m := demo(w, h)
	shot(&tour, m, "ports", 2600*time.Millisecond)
	stop(&tour, m)
	m.toast = nil
	press(m, "s")
	shot(&tour, m, "sort", 2000*time.Millisecond)
	m.toast = nil
	press(m, "tab")
	shot(&tour, m, "containers", 2200*time.Millisecond)
	press(m, "x")
	shot(&tour, m, "confirm-compose", 2200*time.Millisecond)

	// Just the stop, in a terminal too narrow for the panel.
	const sw, sh = 108, 17
	var stopping recorder
	stop(&stopping, demo(sw, sh))

	const bg, fgc = "#1C1C1E", "#F5F5F7"
	list := 103 // columns left of the details panel
	frame := func(name string) []svgFrame { return []svgFrame{tour.frames[tour.named[name]]} }
	images := map[string]string{
		"demo.svg":       windowSVG(tour.frames, crop{0, 0, w, h}, "ghosts", bg, fgc),
		"detached.svg":   windowSVG(frame("detached"), crop{list + 2, 4, w, h - 2}, "ghosts", bg, fgc),
		"containers.svg": windowSVG(frame("containers"), crop{0, 4, list, 15}, "ghosts", bg, fgc),
		"power.svg":      windowSVG(frame("sort"), crop{0, 4, list, 13}, "ghosts", bg, fgc),
		"stop.svg":       windowSVG(stopping.frames, crop{0, 0, sw, sh}, "ghosts", bg, fgc),
	}
	for name, svg := range images {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(svg), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func without(s ports.Snapshot, port int) ports.Snapshot {
	var kept []ports.Listener
	for _, l := range s.Listeners {
		if l.Port != port {
			kept = append(kept, l)
		}
	}
	s.Listeners = kept
	return s
}

const mb = 1 << 20

func demoSnapshot(withStorybook bool) ports.Snapshot {
	now := time.Now()
	dev := func(port, pid int, name, rt, project string, age time.Duration, cpu float64, mem uint64, watts float64, args ...string) ports.Listener {
		exe := "/opt/homebrew/bin/" + rt
		if rt == "python" {
			exe = project + "/.venv/bin/python3"
		}
		return ports.Listener{
			Port: port, PID: pid, PPID: pid - 3, Addrs: []string{"127.0.0.1"},
			Comm: rt, Exe: exe, Args: args, Cwd: project, Started: now.Add(-age),
			ParentName: "zsh", Name: name, Runtime: rt, Project: project, Kind: ports.KindDev,
			Usage: ports.Usage{CPU: cpu, Mem: mem, Power: watts, AvgPower: watts * 0.8, Wakeups: cpu*9 + 2, Procs: 1, Ready: true},
			Tree:  []int{pid},
		}
	}
	ls := []ports.Listener{
		dev(3000, 48211, "next dev", "node", "/Users/you/code/storefront", 3*time.Hour+12*time.Minute,
			41.3, 1210*mb, 2.4, "node", "/Users/you/code/storefront/node_modules/.bin/next", "dev", "--turbopack"),
		dev(5173, 61822, "vite", "node", "/Users/you/code/citymap", 2*24*time.Hour+4*time.Hour,
			0.3, 148*mb, 0.012, "node", "/Users/you/code/citymap/node_modules/.bin/vite", "--port", "5173"),
		dev(8000, 70412, "uvicorn", "python", "/Users/you/code/agent-api", 47*time.Minute,
			3.8, 96*mb, 0.21, "python3", "-m", "uvicorn", "app.main:app", "--reload"),
		dev(4321, 33190, "astro dev", "node", "/Users/you/code/blog", 6*24*time.Hour+2*time.Hour,
			0.1, 212*mb, 0.004, "node", "/Users/you/code/blog/node_modules/.bin/astro", "dev"),
		dev(8888, 52004, "jupyter-lab", "python", "/Users/you/notebooks", 26*time.Hour,
			0.6, 318*mb, 0.03, "python3", "-m", "jupyterlab", "--no-browser"),
	}
	// Two of them outlived the session (a closed terminal, a finished agent run).
	for _, i := range []int{1, 3} {
		ls[i].Detached, ls[i].PPID, ls[i].ParentName = true, 1, "launchd"
	}
	ls[0].Usage.Procs, ls[0].Tree = 4, []int{48211, 48230, 48231, 48232}
	ls[2].Addrs = []string{"*"}
	if withStorybook {
		ls = append(ls, dev(6006, 90117, "storybook", "node", "/Users/you/code/design-system", 9*time.Second,
			12.5, 402*mb, 0.62, "node", "/Users/you/code/design-system/node_modules/.bin/storybook", "dev", "-p", "6006"))
	}
	// Docker Desktop's port proxy, shown as the containers it forwards to.
	for _, p := range []int{5432, 6379, 8025} {
		ls = append(ls, ports.Listener{
			Port: p, PID: 1288, PPID: 1201, Addrs: []string{"127.0.0.1"}, Comm: "com.docker.backend",
			Exe: "/Applications/Docker.app/Contents/MacOS/com.docker.backend", Started: now.Add(-50 * time.Hour),
			Name: "Docker", Kind: ports.KindApp, Usage: ports.Usage{Ready: true},
		})
	}
	// Apps and macOS services, hidden until "a".
	for _, a := range []struct {
		port int
		name string
		kind ports.Kind
	}{{5000, "ControlCenter", ports.KindSystem}, {7000, "ControlCenter", ports.KindSystem}, {57621, "Spotify", ports.KindApp}} {
		ls = append(ls, ports.Listener{Port: a.port, PID: 700 + a.port%97, Name: a.name, Kind: a.kind, Started: now.Add(-80 * time.Hour)})
	}

	procs := map[int]ports.Usage{}
	for _, l := range ls {
		for _, pid := range l.Tree {
			procs[pid] = ports.Usage{CPU: l.Usage.CPU / float64(len(l.Tree)), Mem: l.Usage.Mem / uint64(len(l.Tree)),
				Power: l.Usage.Power / float64(len(l.Tree)), Procs: 1, Ready: true}
		}
	}
	return ports.Snapshot{
		Listeners: ls, Procs: procs, HasEnergy: true, HasWakeups: true,
		VM: ports.Usage{CPU: 9.4, Mem: 2300 * mb, Power: 0.71, Procs: 6, Ready: true},
	}
}

func demoContainers() []docker.Container {
	now := time.Now()
	c := func(id, name, image, status, project, service, dir string, cpu float64, mem uint64, ps ...docker.PortMap) docker.Container {
		return docker.Container{
			ID: id, Name: name, Image: image, State: "running", Status: status, Created: now.Add(-50 * time.Hour),
			Ports: ps, Project: project, Service: service, WorkingDir: dir, CPU: cpu, Mem: mem, UsageReady: true,
		}
	}
	local := func(pub, priv int) docker.PortMap {
		return docker.PortMap{IP: "127.0.0.1", Private: priv, Public: pub, Type: "tcp"}
	}
	api := "/Users/you/code/agent-api"
	shop := "/Users/you/code/storefront"
	return []docker.Container{
		c("9f2c41ab77d0e1", "agent-api-db-1", "postgres:17", "Up 2 days (healthy)", "agent-api", "db", api, 0.8, 74*mb, local(5432, 5432)),
		c("4be8d01c93f2aa", "agent-api-redis-1", "redis:7-alpine", "Up 2 days", "agent-api", "redis", api, 0.3, 9*mb, local(6379, 6379)),
		c("c07a7f5e2b1d93", "agent-api-worker-1", "agent-api-worker", "Up 2 days", "agent-api", "worker", api, 6.2, 188*mb),
		c("71d3e9a0c4b8f5", "storefront-mailpit-1", "axllent/mailpit", "Up 3 hours", "storefront", "mailpit", shop, 0.1, 21*mb, local(8025, 8025)),
		c("e5b19c22f0a7d6", "minio", "minio/minio", "Up 6 days", "", "", "", 0.2, 112*mb, local(9000, 9000), local(9001, 9001)),
	}
}
