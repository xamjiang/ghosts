package ui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/xamjiang/ghosts/internal/docker"
	"github.com/xamjiang/ghosts/internal/ports"
)

type rowKind int

const (
	rowListener rowKind = iota
	rowContainer
	rowGroup
	rowSpacer
)

type row struct {
	kind rowKind
	key  string

	l   *ports.Listener
	ctr *docker.Container // the container itself, or the one owning a listener's port

	group   string // compose project ("" for standalone)
	dir     string
	members []docker.Container
}

func (r row) selectable() bool { return r.kind != rowSpacer }

// containerByPort indexes published host ports, so a listener owned by Docker
// Desktop's proxy can be shown (and stopped) as the container it really is.
func containerByPort(cs []docker.Container) map[int]*docker.Container {
	idx := map[int]*docker.Container{}
	for i := range cs {
		for _, p := range cs[i].Ports {
			idx[p.Public] = &cs[i]
		}
	}
	return idx
}

// visibleListeners applies the "show all" toggle; it returns the rows to show
// and how many were hidden.
func (m *Model) visibleListeners() (out []row, hidden int) {
	byPort := containerByPort(m.containers)
	for i := range m.listeners {
		l := &m.listeners[i]
		r := row{kind: rowListener, key: l.Key(), l: l, ctr: byPort[l.Port]}
		if r.ctr == nil && l.Kind != ports.KindDev && !m.showAll {
			hidden++
			continue
		}
		out = append(out, r)
	}
	return out, hidden
}

func (m *Model) buildPortRows() []row {
	rows, _ := m.visibleListeners()
	if q := strings.ToLower(m.filter); q != "" {
		kept := rows[:0:0]
		for _, r := range rows {
			if matchListener(r, q) {
				kept = append(kept, r)
			}
		}
		rows = kept
	}
	switch m.sort {
	case sortEnergy:
		sort.SliceStable(rows, func(i, j int) bool {
			a, b := usageOf(rows[i]), usageOf(rows[j])
			if a.power != b.power {
				return a.power > b.power
			}
			return a.cpu > b.cpu
		})
	case sortMemory:
		sort.SliceStable(rows, func(i, j int) bool { return usageOf(rows[i]).mem > usageOf(rows[j]).mem })
	case sortNewest:
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].l.Started.After(rows[j].l.Started) })
	case sortName:
		sort.SliceStable(rows, func(i, j int) bool {
			return strings.ToLower(displayName(rows[i])) < strings.ToLower(displayName(rows[j]))
		})
	}
	return rows
}

// rowUsage is what a row costs. Listeners that belong to a container report
// the container's numbers; containers have no per-container energy.
type rowUsage struct {
	cpu, power float64
	mem        uint64
	ready      bool
	hasPower   bool
}

func usageOf(r row) rowUsage {
	if r.ctr != nil {
		return rowUsage{cpu: r.ctr.CPU, mem: r.ctr.Mem, ready: r.ctr.UsageReady}
	}
	u := r.l.Usage
	return rowUsage{cpu: u.CPU, power: u.Power, mem: u.Mem, ready: u.Ready, hasPower: true}
}

// totalUsage sums the visible process rows, counting each PID once (a parent
// listener's tree may contain a worker that is listed too).
func (m *Model) totalUsage() (u ports.Usage) {
	seen := map[int]bool{}
	for _, r := range m.rows {
		if r.kind != rowListener || r.ctr != nil {
			continue
		}
		for _, p := range r.l.Tree {
			if seen[p] {
				continue
			}
			seen[p] = true
			if pu, ok := m.procs[p]; ok {
				u.CPU += pu.CPU
				u.Mem += pu.Mem
				u.Power += pu.Power
				u.Ready = u.Ready || pu.Ready
			}
		}
	}
	return u
}

func displayName(r row) string {
	if r.ctr != nil {
		return r.ctr.Name
	}
	return r.l.Name
}

func matchListener(r row, q string) bool {
	l := r.l
	fields := []string{strconv.Itoa(l.Port), l.Name, l.Runtime, l.Comm, tilde(l.Project), l.CommandLine(), strconv.Itoa(l.PID)}
	if r.ctr != nil {
		fields = append(fields, r.ctr.Name, r.ctr.Image, r.ctr.Project, "docker")
	}
	return matchAny(q, fields...)
}

func matchContainer(c docker.Container, q string) bool {
	fields := []string{c.Name, c.Image, c.Project, c.Service, tilde(c.WorkingDir), c.ID[:min(12, len(c.ID))]}
	for _, p := range c.Ports {
		fields = append(fields, strconv.Itoa(p.Public))
	}
	return matchAny(q, fields...)
}

func matchAny(q string, fields ...string) bool {
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}

// buildContainerRows groups containers by compose project. Groups are
// separated by a blank, non-selectable spacer row.
func (m *Model) buildContainerRows() []row {
	q := strings.ToLower(m.filter)
	type grp struct {
		name, dir string
		cs        []docker.Container
	}
	var groups []*grp
	byName := map[string]*grp{}
	for _, c := range m.containers {
		g := byName[c.Project]
		if g == nil {
			g = &grp{name: c.Project, dir: c.WorkingDir}
			byName[c.Project] = g
			groups = append(groups, g)
		}
		g.cs = append(g.cs, c)
	}
	onlyStandalone := len(groups) == 1 && groups[0].name == ""

	var rows []row
	for _, g := range groups {
		members := g.cs
		if q != "" && !(g.name != "" && strings.Contains(strings.ToLower(g.name), q)) {
			members = nil
			for _, c := range g.cs {
				if matchContainer(c, q) {
					members = append(members, c)
				}
			}
		}
		if len(members) == 0 {
			continue
		}
		if !onlyStandalone {
			if len(rows) > 0 {
				rows = append(rows, row{kind: rowSpacer, key: "spacer:" + g.name})
			}
			rows = append(rows, row{kind: rowGroup, key: "g:" + g.name, group: g.name, dir: g.dir, members: g.cs})
		}
		for i := range members {
			rows = append(rows, row{kind: rowContainer, key: members[i].Key(), ctr: &members[i], group: g.name})
		}
	}
	return rows
}
