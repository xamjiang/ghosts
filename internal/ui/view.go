package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/xamjiang/ghosts/internal/docker"
	"github.com/xamjiang/ghosts/internal/ports"
)

// Screen geometry. Content is indented to column 3 everywhere so the eye
// finds one clean left edge; column 1 holds the selection bar.
const (
	tabsRow  = 2
	listTop  = 5 // header, blank, tabs, blank, column header
	chrome   = listTop + 2
	indent   = 3
	wideMin  = 110
	gap      = "  "
	ellipsis = "…"
)

var spinner = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m Model) listHeight() int { return max(1, m.height-chrome) }

// rowsShown is how many rows fit. When the list overflows, the last line is
// kept for a "↓ 3 more" hint so the selection is never hidden behind it.
func (m Model) rowsShown() int {
	h := m.listHeight()
	if len(m.rows) > h {
		return max(1, h-1)
	}
	return h
}

func (m Model) wide() bool { return m.width >= wideMin }

// split returns the widths of the list and the side panel.
func (m Model) split() (list, panel int) {
	if !m.wide() || !m.panel {
		return m.width, 0
	}
	panel = max(40, min(60, m.width*36/100))
	return m.width - panel, panel
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.ReportFocus = true
	v.WindowTitle = "Ghosts"
	if m.opts.Mouse {
		v.MouseMode = tea.MouseModeCellMotion
	}
	return v
}

func (m Model) render() string {
	if m.width == 0 || m.height == 0 {
		return ""
	}
	lines := []string{m.viewHeader(), "", m.viewTabs(), ""}
	lines = append(lines, m.viewMain()...)
	lines = append(lines, m.viewFooter()...)
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- primitives

type seg struct {
	s  string
	st lipgloss.Style
}

func (m Model) s(text string, c color.Color) seg { return seg{text, fg(c)} }

// line renders segments into exactly w cells. With a background, every
// segment (and the padding) carries it so the highlight has no gaps.
func line(segs []seg, w int, bg color.Color) string {
	var b strings.Builder
	used := 0
	for _, sg := range segs {
		if used >= w {
			break
		}
		t := sg.s
		if tw := ansi.StringWidth(t); tw > w-used {
			t = ansi.Truncate(t, w-used, ellipsis)
		}
		st := sg.st
		if bg != nil {
			st = st.Background(bg)
		}
		b.WriteString(st.Render(t))
		used += ansi.StringWidth(t)
	}
	if used < w {
		pad := lipgloss.NewStyle()
		if bg != nil {
			pad = pad.Background(bg)
		}
		b.WriteString(pad.Render(strings.Repeat(" ", w-used)))
	}
	return b.String()
}

func spaces(n int) seg { return seg{strings.Repeat(" ", max(0, n)), lipgloss.NewStyle()} }

// keyHint renders "x Stop" with the key emphasised.
func (m Model) keyHint(key, label string, keyColor color.Color) []seg {
	return []seg{{key, fg(keyColor).Bold(true)}, m.s(" "+label, m.th.sub)}
}

// ------------------------------------------------------------------- header

func (m Model) viewHeader() string {
	left := []seg{spaces(indent), {"Ghosts", fg(m.th.fg).Bold(true)}}
	var right []seg
	switch {
	case m.portsErr != nil:
		right = []seg{m.s("● ", m.th.red), m.s(firstLine(m.portsErr.Error()), m.th.sub)}
	default:
		right = append(right, m.summary()...)
		if m.dockerReady && !m.dockerUp {
			right = append(right, m.s("Docker offline", m.th.faint), m.s("   ", m.th.faint))
		}
		dot := m.th.green
		if !m.portsReady {
			dot = m.th.faint
		}
		right = append(right, m.s("● ", dot), m.s("Live", m.th.sub))
	}
	return m.justify(left, right, m.width, 2)
}

// summary is the total cost of what the current tab shows: the listed
// processes, or the VM that runs every container. Where containers run
// natively (Docker Engine on Linux) there is no VM, so their own stats add up.
func (m Model) summary() []seg {
	label, u := "Total", m.totalUsage()
	if m.tab == tabContainers {
		if !m.dockerUp {
			return nil
		}
		label, u = "Docker VM", m.vm
		if !m.vm.Ready {
			label, u = "Containers", m.containerUsage()
		}
	}
	if !u.Ready {
		return nil
	}
	parts := bytesText(u.Mem)
	if m.hasEnergy {
		parts += " · " + powerText(u.Power)
	} else {
		parts += " · " + cpuText(u.CPU) + " CPU"
	}
	return []seg{m.s(label+"  ", m.th.faint), m.s(parts, m.th.sub), m.s("   ", m.th.faint)}
}

// justify places right-aligned segments after left ones, with a margin.
func (m Model) justify(left, right []seg, w, margin int) string {
	lw, rw := segsWidth(left), segsWidth(right)
	if lw+rw+2+margin > w {
		return line(left, w, nil)
	}
	all := append(append(append([]seg{}, left...), spaces(w-lw-rw-margin)), right...)
	return line(all, w, nil)
}

func segsWidth(ss []seg) int {
	n := 0
	for _, s := range ss {
		n += ansi.StringWidth(s.s)
	}
	return n
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// --------------------------------------------------------------------- tabs

func (m Model) tabPills() []string {
	names := [2]string{"Ports", "Containers"}
	out := make([]string, 2)
	for i, n := range names {
		count := fmt.Sprint(m.counts[i])
		if i == int(tabContainers) && m.dockerReady && !m.dockerUp {
			count = "–"
		}
		if tab(i) == m.tab {
			bg := lipgloss.NewStyle().Background(m.th.pillBG)
			out[i] = bg.Render(" ") + bg.Foreground(m.th.fg).Bold(true).Render(n) +
				bg.Render(" ") + bg.Foreground(m.th.sub).Render(count) + bg.Render(" ")
		} else {
			out[i] = " " + fg(m.th.sub).Render(n) + " " + fg(m.th.faint).Render(count) + " "
		}
	}
	return out
}

// tabRanges returns the [start, end) columns of each tab, for mouse clicks.
func (m Model) tabRanges() [][2]int {
	x := indent - 1
	var r [][2]int
	for _, p := range m.tabPills() {
		w := lipgloss.Width(p)
		r = append(r, [2]int{x, x + w})
		x += w + 1
	}
	return r
}

func (m Model) viewTabs() string {
	p := m.tabPills()
	left := []seg{spaces(indent - 1), {p[0], lipgloss.NewStyle()}, spaces(1), {p[1], lipgloss.NewStyle()}}
	var right []seg
	switch {
	case m.filtering:
		right = []seg{m.s("/ ", m.th.accent), m.s(m.filter, m.th.fg), m.s("▏", m.th.accent)}
	case m.filter != "":
		right = []seg{m.s("/ ", m.th.faint), m.s(m.filter, m.th.fg)}
	case m.tab == tabPorts && m.sort != sortPort:
		right = []seg{m.s("Sorted by "+m.sortLabel(), m.th.faint)}
	}
	return m.justifyRaw(left, right)
}

// justifyRaw is justify for segments that are already styled.
func (m Model) justifyRaw(left, right []seg) string {
	var b strings.Builder
	lw := 0
	for _, s := range left {
		b.WriteString(s.st.Render(s.s))
		lw += lipgloss.Width(s.s)
	}
	rw := segsWidth(right)
	if rw > 0 && lw+rw+4 <= m.width {
		b.WriteString(strings.Repeat(" ", m.width-lw-rw-2))
		for _, s := range right {
			b.WriteString(s.st.Render(s.s))
		}
	}
	return b.String()
}

// --------------------------------------------------------------------- main

func (m Model) viewMain() []string {
	h := m.listHeight() + 1 // column header + rows
	if m.help {
		return m.viewHelp(h)
	}
	listW, panelW := m.split()
	if m.zoom && !m.wide() {
		return m.viewDetail(m.width, h, false)
	}
	list := m.viewList(listW, h)
	if panelW == 0 {
		return list
	}
	panel := m.viewDetail(panelW, h, true)
	for i := range list {
		list[i] += panel[i]
	}
	return list
}

func (m Model) viewList(w, h int) []string {
	lines := make([]string, 0, h)
	if title, sub, hint := m.emptyState(); title != "" {
		top := max(0, (h-4)*2/5)
		for range top {
			lines = append(lines, line(nil, w, nil))
		}
		lines = append(lines, centered(fg(m.th.fg).Bold(true).Render(title), w))
		if sub != "" {
			lines = append(lines, centered(fg(m.th.sub).Render(sub), w))
		}
		if hint != "" {
			lines = append(lines, line(nil, w, nil), centered(hint, w))
		}
	} else {
		var cols columns
		if m.tab == tabPorts {
			cols = m.portColumns(w)
		} else {
			cols = m.containerColumns(w)
		}
		lines = append(lines, cols.header(m, w))
		end := min(len(m.rows), m.offset[m.tab]+m.rowsShown())
		for i := m.offset[m.tab]; i < end; i++ {
			lines = append(lines, m.viewRow(m.rows[i], i == m.cursor[m.tab], cols, w))
		}
	}
	for len(lines) < h {
		lines = append(lines, line(nil, w, nil))
	}
	// Scroll hint on the reserved last line when the list overflows.
	if len(m.rows) > m.listHeight() {
		below := len(m.rows) - (m.offset[m.tab] + m.rowsShown())
		hint := fmt.Sprintf("↓ %d more", below)
		if below <= 0 {
			hint = fmt.Sprintf("↑ %d more", m.offset[m.tab])
		}
		lines[h-1] = line([]seg{spaces(indent), m.s(hint, m.th.faint)}, w, nil)
	}
	return lines[:h]
}

func centered(s string, w int) string {
	return lipgloss.PlaceHorizontal(w, lipgloss.Center, s)
}

func (m Model) emptyState() (title, sub, hint string) {
	if len(m.rows) > 0 {
		return
	}
	if m.filter != "" {
		return fmt.Sprintf("No matches for “%s”", m.filter), "", m.hintLine("esc", "Clear filter")
	}
	if m.tab == tabPorts {
		switch {
		case !m.portsReady && m.portsErr == nil:
			return "Scanning ports…", "", ""
		case m.portsErr != nil:
			return "Couldn't read listening ports", firstLine(m.portsErr.Error()), m.hintLine("r", "Try again")
		}
		if m.hidden > 0 {
			return "No dev servers running", "Everything is quiet.",
				m.hintLine("a", fmt.Sprintf("Show %d app & system %s", m.hidden, pluralWord(m.hidden, "service")))
		}
		return "Nothing is listening", "No process is accepting connections right now.", ""
	}
	switch {
	case !m.dockerReady:
		return "Connecting to Docker…", "", ""
	case !m.dockerUp:
		return "Docker isn't running", "Containers will appear here as soon as it starts.", ""
	}
	return "No running containers", "Start one and it will show up here instantly.", ""
}

func pluralWord(n int, w string) string {
	if n == 1 {
		return w
	}
	return w + "s"
}

func (m Model) hintLine(key, label string) string {
	return fg(m.th.fg).Bold(true).Render(key) + fg(m.th.sub).Render("  "+label)
}

// ------------------------------------------------------------------ columns

type column struct {
	title string
	w     int
	right bool // numbers are right-aligned so their digits line up
}

type columns struct {
	cols  []column
	badge int
	dot   bool // containers: status dot before the first column
}

func (c columns) header(m Model, w int) string {
	segs := []seg{spaces(indent)}
	if c.dot {
		segs = append(segs, spaces(2))
	}
	for i, col := range c.cols {
		if col.w == 0 {
			continue
		}
		if i > 0 {
			segs = append(segs, spaces(len(gap)))
		}
		segs = append(segs, m.s(col.fit(col.title), m.th.faint))
	}
	return line(segs, w, nil)
}

func (c column) fit(s string) string {
	if c.right {
		return fitRight(s, c.w)
	}
	return fit(s, c.w)
}

// shrink hides columns, in the given order, until the flexible column at
// index flex has at least min cells; it then gives that column the rest.
func shrink(cols []column, w, reserved, flex, min int, order ...int) {
	used := func() int {
		n := reserved
		for i, c := range cols {
			if c.w > 0 && i != flex {
				n += c.w + len(gap)
			}
		}
		return n + len(gap)
	}
	for _, i := range order {
		if w-used() >= min {
			break
		}
		cols[i].w = 0
	}
	cols[flex].w = max(0, w-used())
}

func (m Model) badgeWidth() int {
	bw := 0
	for _, r := range m.rows {
		if _, busy := m.busy[r.key]; busy || (r.ctr != nil && m.busy[r.ctr.Key()] != "") {
			bw = max(bw, 13)
		} else if m.isNew(r.key) {
			bw = max(bw, 3)
		}
	}
	if bw > 0 {
		bw += len(gap)
	}
	return bw
}

// Port list columns; indexes are used by listenerCells.
const (
	pcPort = iota
	pcName
	pcCPU
	pcMem
	pcPower
	pcUp
	pcProject
)

func (m Model) portColumns(w int) columns {
	nameW := 10
	for _, r := range m.rows {
		nameW = max(nameW, ansi.StringWidth(listenerName(r))+1)
	}
	nameW = min(nameW, 26, max(14, w/4))
	cols := []column{
		{"PORT", 6, false}, {"NAME", nameW, false},
		{"CPU", 6, true}, {"MEM", 8, true}, {"POWER", 7, true},
		{"  UPTIME", 9, false}, {"PROJECT", 0, false}, // leading spaces: room after the numbers
	}
	if !m.hasEnergy {
		cols[pcPower].w = 0
	}
	badge := m.badgeWidth()
	shrink(cols, w, indent+badge+2, pcProject, 16, pcUp, pcCPU, pcMem, pcPower)
	return columns{cols: cols, badge: badge}
}

func listenerName(r row) string {
	if r.ctr != nil {
		return r.ctr.Name + " docker"
	}
	if r.l.Runtime != "" {
		return r.l.Name + " " + r.l.Runtime
	}
	return r.l.Name
}

// Container list columns; indexes are used by containerCells.
const (
	ccName = iota
	ccImage
	ccPorts
	ccCPU
	ccMem
	ccUp
)

func (m Model) containerColumns(w int) columns {
	nameW, imgW, portW := 10, 8, 5
	for _, r := range m.rows {
		if r.kind != rowContainer {
			continue
		}
		nameW = max(nameW, ansi.StringWidth(r.ctr.Name))
		imgW = max(imgW, ansi.StringWidth(r.ctr.Image))
		portW = max(portW, ansi.StringWidth(portsText(r.ctr.Ports)))
	}
	cols := []column{
		{"NAME", min(nameW, 30), false}, {"IMAGE", 0, false}, {"PORTS", min(portW, 22), false},
		{"CPU", 6, true}, {"MEM", 8, true}, {"  UP", 8, false},
	}
	badge := m.badgeWidth()
	shrink(cols, w, indent+2+badge+2, ccImage, 8, ccUp, ccCPU)
	cols[ccImage].w = min(cols[ccImage].w, min(imgW, 28))
	if cols[ccImage].w < 8 {
		cols[ccImage].w = 0
	}
	return columns{cols: cols, badge: badge, dot: true}
}

func portsText(ps []docker.PortMap) string {
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		if p.Public == p.Private {
			parts = append(parts, fmt.Sprint(p.Public))
		} else {
			parts = append(parts, fmt.Sprintf("%d→%d", p.Public, p.Private))
		}
	}
	return strings.Join(parts, ", ")
}

// --------------------------------------------------------------------- rows

func (m Model) viewRow(r row, sel bool, cols columns, w int) string {
	var bg color.Color
	bar := spaces(indent)
	if sel {
		bg = m.th.selBG
		bar = seg{" ▍ ", fg(m.th.accent)}
	}
	var segs []seg
	switch r.kind {
	case rowSpacer:
		return line(nil, w, nil)
	case rowListener:
		segs = m.listenerCells(r, cols)
	case rowContainer:
		segs = m.containerCells(r, cols)
	case rowGroup:
		segs = m.groupCells(r, w-indent-2)
	}
	segs = append([]seg{bar}, segs...)
	if b := m.badge(r); b != nil && cols.badge > 0 {
		used := segsWidth(segs)
		segs = append(segs, spaces(w-used-segsWidth(b)-2))
		segs = append(segs, b...)
	}
	return line(segs, w, bg)
}

func (m Model) badge(r row) []seg {
	label := m.busy[r.key]
	if label == "" && r.ctr != nil {
		label = m.busy[r.ctr.Key()]
	}
	if label != "" {
		return []seg{m.s(spinner[m.frame%len(spinner)]+" "+label+"…", m.th.orange)}
	}
	if m.isNew(r.key) {
		return []seg{{"new", fg(m.th.accent).Bold(true)}}
	}
	return nil
}

func (m Model) listenerCells(r row, c columns) []seg {
	l := r.l
	col := func(i int) column { return c.cols[i] }
	segs := []seg{{fit(fmt.Sprint(l.Port), col(pcPort).w), fg(m.th.fg).Bold(true)}, spaces(len(gap))}

	// Name, with the runtime as a quieter suffix.
	name, rt, rtColor := l.Name, l.Runtime, m.th.sub
	if r.ctr != nil {
		name, rt, rtColor = r.ctr.Name, "docker", m.th.accent
	}
	nameW := col(pcName).w
	if ansi.StringWidth(name) >= nameW || rt == "" {
		segs = append(segs, m.s(fit(name, nameW), m.th.fg))
	} else {
		segs = append(segs, m.s(name, m.th.fg), m.s(fit(" "+rt, nameW-ansi.StringWidth(name)), rtColor))
	}

	u := usageOf(r)
	cell := func(i int, text string, c color.Color) {
		if col(i).w > 0 {
			segs = append(segs, spaces(len(gap)), m.s(col(i).fit(text), c))
		}
	}
	if u.ready {
		cpuColor := m.th.sub
		if u.cpu < 0.1 {
			cpuColor = m.th.faint
		}
		cell(pcCPU, cpuText(u.cpu), cpuColor)
		cell(pcMem, bytesText(u.mem), m.th.sub)
		if u.hasPower {
			cell(pcPower, powerText(u.power), m.powerColor(u.power))
		} else {
			cell(pcPower, "–", m.th.faint) // containers: counted for the VM as a whole
		}
	} else {
		cell(pcCPU, "–", m.th.faint)
		cell(pcMem, "–", m.th.faint)
		cell(pcPower, "–", m.th.faint)
	}
	up := ago(time.Since(l.Started))
	if r.ctr != nil {
		up = dockerUptime(r.ctr.Status) // not the uptime of Docker's port proxy
	}
	cell(pcUp, "  "+up, m.th.sub)

	if w := col(pcProject).w; w > 0 {
		proj := l.Project
		if r.ctr != nil {
			proj = r.ctr.WorkingDir
			if proj == "" {
				proj = r.ctr.Project
			}
		}
		segs = append(segs, spaces(len(gap)))
		segs = append(segs, m.pathSegs(tilde(proj), w)...)
	}
	return segs
}

// powerColor stays quiet for idle processes and only speaks up when a
// process draws enough to visibly shorten battery life.
func (m Model) powerColor(w float64) color.Color {
	switch {
	case w >= 2:
		return m.th.red
	case w >= 0.5:
		return m.th.orange
	case w < 0.001:
		return m.th.faint
	}
	return m.th.fg
}

// pathSegs shows a path with its last element emphasised, trimmed from the left.
func (m Model) pathSegs(p string, w int) []seg {
	if p == "" {
		return []seg{m.s("–", m.th.faint)}
	}
	dir, base := splitPath(p)
	if ansi.StringWidth(base) >= w {
		return []seg{m.s(fitLeft(base, w), m.th.fg)}
	}
	dir = fitLeft(dir, w-ansi.StringWidth(base))
	return []seg{m.s(dir, m.th.faint), m.s(base, m.th.fg)}
}

func (m Model) statusDot(c *docker.Container) seg {
	switch {
	case c.State == "paused":
		return m.s("●", m.th.faint)
	case c.State == "restarting", c.Health() == "starting":
		return m.s("●", m.th.orange)
	case c.Health() == "unhealthy":
		return m.s("●", m.th.red)
	}
	return m.s("●", m.th.green)
}

func (m Model) containerCells(r row, c columns) []seg {
	ct := r.ctr
	segs := []seg{m.statusDot(ct), spaces(1)}
	segs = append(segs, m.s(fit(ct.Name, c.cols[ccName].w), m.th.fg))
	cell := func(i int, text string, col color.Color) {
		if c.cols[i].w > 0 {
			segs = append(segs, spaces(len(gap)), m.s(c.cols[i].fit(text), col))
		}
	}
	cell(ccImage, ct.Image, m.th.sub)
	pt := portsText(ct.Ports)
	if pt == "" {
		pt = "–"
	}
	cell(ccPorts, pt, m.th.fg)
	if ct.UsageReady {
		cell(ccCPU, cpuText(ct.CPU), m.th.sub)
	} else {
		cell(ccCPU, "–", m.th.faint)
	}
	if ct.Mem > 0 {
		cell(ccMem, bytesText(ct.Mem), m.th.sub)
	} else {
		cell(ccMem, "–", m.th.faint)
	}
	up := dockerUptime(ct.Status)
	if ct.State == "paused" {
		up = "paused"
	}
	cell(ccUp, "  "+up, m.th.sub)
	return segs
}

func (m Model) groupCells(r row, w int) []seg {
	name := r.group
	if name == "" {
		name = "Standalone"
	}
	count := plural(len(r.members), "container")
	var mem uint64
	for _, c := range r.members {
		mem += c.Mem
	}
	if mem > 0 {
		count += " · " + bytesText(mem)
	}
	segs := []seg{{name, fg(m.th.fg).Bold(true)}}
	rest := w - ansi.StringWidth(name) - ansi.StringWidth(count) - 4
	if r.dir != "" && rest > 6 {
		segs = append(segs, spaces(2), m.s(fitLeft(tilde(r.dir), rest), m.th.faint))
	}
	segs = append(segs, spaces(w-segsWidth(segs)-ansi.StringWidth(count)), m.s(count, m.th.sub))
	return segs
}

// ------------------------------------------------------------------- detail

type kv struct {
	k, v string
	c    color.Color
}

func (m Model) viewDetail(w, h int, bordered bool) []string {
	inner := w - 4
	pad := spaces(2)
	if bordered {
		pad = seg{"│ ", fg(m.th.line)}
	} else {
		inner = w - indent - 2
		pad = spaces(indent)
	}

	var title, subtitle string
	var subColor color.Color = m.th.sub
	var rows []kv
	var extraLabel string
	var block string

	r := m.selected()
	switch {
	case r == nil:
	case r.kind == rowGroup:
		title = r.group
		if title == "" {
			title = "Standalone containers"
			subtitle = plural(len(r.members), "container")
		} else {
			subtitle = "Compose project · " + plural(len(r.members), "container")
		}
		if r.dir != "" {
			rows = append(rows, kv{"Directory", tilde(r.dir), nil})
		}
		var cpu float64
		var mem uint64
		for _, c := range r.members {
			cpu, mem = cpu+c.CPU, mem+c.Mem
		}
		rows = append(rows, kv{"§", "USAGE", nil}, kv{"CPU", cpuText(cpu), nil}, kv{"Memory", bytesText(mem), nil})
		rows = append(rows, m.vmKV()...)
	case r.ctr != nil:
		ct := r.ctr
		title = ct.Name
		subtitle = ct.Image
		state := ct.State
		if h := ct.Health(); h != "" {
			state += " · " + h
		}
		var stateColor color.Color
		switch ct.Health() {
		case "unhealthy":
			stateColor = m.th.red
		case "starting":
			stateColor = m.th.orange
		}
		rows = append(rows, kv{"Status", state, stateColor})
		for _, p := range ct.Ports {
			where := "localhost only"
			if p.IP == "0.0.0.0" || p.IP == "::" || p.IP == "" {
				where = "all interfaces"
			}
			rows = append(rows, kv{"Port", fmt.Sprintf("%d → %d  %s", p.Public, p.Private, where), nil})
		}
		if ct.Project != "" {
			proj := ct.Project
			if ct.Service != "" {
				proj += " · " + ct.Service
			}
			rows = append(rows, kv{"Compose", proj, nil})
		}
		if ct.WorkingDir != "" {
			rows = append(rows, kv{"Directory", tilde(ct.WorkingDir), nil})
		}
		rows = append(rows,
			kv{"Running", dockerUptime(ct.Status), nil},
			kv{"Created", ct.Created.Format("Jan 2, 15:04"), nil},
			kv{"ID", ct.ID[:min(12, len(ct.ID))], nil},
		)
		if u := urlFor(r); u != "" && r.kind == rowContainer {
			rows = append(rows, kv{"Open", u, m.th.accent})
		}
		rows = append(rows, kv{"§", "USAGE", nil})
		if ct.UsageReady {
			rows = append(rows, kv{"CPU", cpuText(ct.CPU), nil})
		}
		if ct.Mem > 0 {
			rows = append(rows, kv{"Memory", bytesText(ct.Mem), nil})
		}
		rows = append(rows, m.vmKV()...)
	case r.kind == rowListener:
		l := r.l
		title = l.Name
		subtitle = fmt.Sprintf("PID %d", l.PID)
		if l.Runtime != "" {
			subtitle = l.Runtime + " · " + subtitle
		}
		switch l.Kind {
		case ports.KindApp:
			subtitle += " · App"
		case ports.KindSystem:
			subtitle += " · " + systemLabel
		}
		where, whereColor := localOnly, color.Color(nil)
		if l.Public() {
			where, whereColor = "Your network (all interfaces)", m.th.orange
		}
		rows = append(rows, kv{"Port", fmt.Sprint(l.Port), nil}, kv{"Listening", where, whereColor})
		if l.Project != "" {
			rows = append(rows, kv{"Project", tilde(l.Project), nil})
		}
		rows = append(rows, kv{"Running", ago(time.Since(l.Started)) + " · since " + l.Started.Format("Jan 2, 15:04"), nil})
		switch {
		case l.Detached && l.Kind == ports.KindDev:
			// Its terminal or agent session is gone; this is the classic
			// forgotten background server.
			rows = append(rows, kv{"Parent", "None · detached from its session", m.th.orange})
		case l.ParentName != "" && l.PPID > 1:
			rows = append(rows, kv{"Parent", fmt.Sprintf("%s (%d)", l.ParentName, l.PPID), nil})
		}
		rows = append(rows, kv{"Open", urlFor(r), m.th.accent})
		rows = append(rows, m.processUsageKV(l.Usage)...)
		extraLabel = "COMMAND"
		block = l.CommandLine()
	}

	lines := []string{line([]seg{pad}, w, nil)}
	add := func(segs ...seg) {
		lines = append(lines, line(append([]seg{pad}, segs...), w, nil))
	}
	if title == "" {
		add(m.s("Nothing selected", m.th.faint))
	} else {
		add(seg{title, fg(m.th.fg).Bold(true)})
		add(m.s(subtitle, subColor))
		add()
		labelW := 11
		for _, kv := range rows {
			c := kv.c
			if c == nil {
				c = m.th.fg
			}
			v := kv.v
			switch kv.k {
			case "§": // section break
				add()
				add(m.s(v, m.th.faint))
				continue
			case "":
				add(m.s(fit("", labelW), m.th.faint), m.s(v, m.th.faint))
				continue
			}
			if kv.k == "Project" || kv.k == "Directory" {
				dir, base := splitPath(fitLeft(v, inner-labelW))
				add(m.s(fit(kv.k, labelW), m.th.faint), m.s(dir, m.th.sub), m.s(base, c))
				continue
			}
			add(m.s(fit(kv.k, labelW), m.th.faint), m.s(v, c))
		}
		if r != nil && r.kind == rowGroup {
			add()
			add(m.s("CONTAINERS", m.th.faint))
			for i := range r.members {
				ct := &r.members[i]
				add(m.statusDot(ct), spaces(1), m.s(ct.Name, m.th.fg), m.s("  "+portsText(ct.Ports), m.th.sub))
			}
		}
		if block != "" {
			add()
			add(m.s(extraLabel, m.th.faint))
			for _, ln := range wrapCommand(block, inner) {
				add(m.s(ln, m.th.sub))
			}
		}
	}
	if len(lines) > h {
		lines = lines[:h]
		lines[h-1] = line([]seg{pad, m.s(ellipsis, m.th.faint)}, w, nil)
	}
	for len(lines) < h {
		lines = append(lines, line([]seg{pad}, w, nil))
	}
	return lines
}

// --------------------------------------------------------------------- help

func (m Model) viewHelp(h int) []string {
	type item struct{ k, d string }
	sections := []struct {
		name  string
		items []item
	}{
		{"Navigate", []item{
			{"↑ ↓  j k", "Move selection"},
			{"⇥  1 2", "Switch between Ports and Containers"},
			{"g G", "Jump to top / bottom"},
			{"⏎", "Show or hide details"},
		}},
		{"Act", []item{
			{"x", "Stop process, container or compose project"},
			{"X", "Force stop (SIGKILL / docker kill)"},
			{"o", "Open in browser"},
		}},
		{"View", []item{
			{"/", "Filter by port, name, project or image"},
			{"a", "Include apps and " + osName + " services"},
			{"s", "Sort ports by port, energy, memory, newest or name"},
			{"r", "Refresh now"},
			{"q", "Quit Ghosts"},
		}},
	}
	render := func(compact bool) string {
		sep, padY := "\n\n", 1
		if compact {
			sep, padY = "\n", 0
		}
		var b strings.Builder
		b.WriteString(fg(m.th.fg).Bold(true).Render("Keyboard Shortcuts"))
		for _, s := range sections {
			b.WriteString(sep + fg(m.th.faint).Render(strings.ToUpper(s.name)))
			for _, it := range s.items {
				b.WriteString("\n" + fg(m.th.fg).Bold(true).Render(fit(it.k, 10)) + fg(m.th.sub).Render(it.d))
			}
		}
		if !compact {
			b.WriteString(sep + fg(m.th.faint).Render("Press any key to close"))
		}
		return lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(m.th.line).
			Padding(padY, 3).
			Render(b.String())
	}
	card := render(false)
	if lipgloss.Height(card) > h {
		card = render(true)
	}
	out := lipgloss.Place(m.width, h, lipgloss.Center, lipgloss.Center, card)
	lines := strings.Split(out, "\n")
	for len(lines) < h {
		lines = append(lines, "")
	}
	return lines[:h]
}

// ------------------------------------------------------------------- footer

func (m Model) viewFooter() []string {
	blank := line(nil, m.width, nil)
	if c := m.confirm; c != nil {
		accent := m.th.accent
		if c.danger {
			accent = m.th.red
		}
		bar := seg{" ▍ ", fg(accent)}
		top := []seg{bar, {c.title, fg(m.th.fg).Bold(true)}, spaces(2), m.s(c.detail, m.th.sub)}
		// The keys come first so they can never be pushed off-screen; a
		// long warning is what gets truncated instead.
		bottom := []seg{bar}
		bottom = append(bottom, m.keyHint("y", c.verb, accent)...)
		bottom = append(bottom, spaces(3))
		bottom = append(bottom, m.keyHint("esc", "Cancel", m.th.fg)...)
		if c.warn != "" {
			bottom = append(bottom, m.s("   ·   ", m.th.faint), m.s(c.warn, m.th.orange))
		}
		return []string{line(top, m.width, nil), line(bottom, m.width, nil)}
	}
	if t := m.toast; t != nil {
		icon, c := "✓ ", m.th.green
		switch t.kind {
		case toastErr:
			icon, c = "✕ ", m.th.red
		case toastInfo:
			icon, c = "", m.th.sub
		}
		return []string{blank, line([]seg{spaces(indent), m.s(icon, c), m.s(t.text, m.th.fg)}, m.width, nil)}
	}
	return []string{blank, m.hints()}
}

func (m Model) hints() string {
	type h struct{ k, d string }
	var hs []h
	switch {
	case m.filtering:
		hs = []h{{"⏎", "Done"}, {"esc", "Clear"}, {"↑↓", "Move"}}
	case m.help:
		hs = []h{{"any key", "Close"}}
	default:
		r := m.selected()
		if r != nil {
			switch {
			case r.kind == rowGroup:
				hs = append(hs, h{"x", "Stop all"})
			case r.ctr != nil:
				hs = append(hs, h{"x", "Stop"})
			default:
				hs = append(hs, h{"x", "Stop"})
			}
			if urlFor(r) != "" {
				hs = append(hs, h{"o", "Open"})
			}
		}
		switch {
		case m.zoom:
			hs = append(hs, h{"esc", "Back"})
		case !m.wide() && r != nil:
			hs = append(hs, h{"⏎", "Details"})
		}
		hs = append(hs, h{"/", "Filter"})
		if m.tab == tabPorts {
			if m.showAll {
				hs = append(hs, h{"a", "Hide apps"})
			} else if m.hidden > 0 {
				hs = append(hs, h{"a", fmt.Sprintf("Show all +%d", m.hidden)})
			}
		}
		other := "Containers"
		if m.tab == tabContainers {
			other = "Ports"
		}
		hs = append(hs, h{"⇥", other}, h{"?", "Help"}, h{"q", "Quit"})
	}
	segs := []seg{spaces(indent)}
	for i, x := range hs {
		part := m.keyHint(x.k, x.d, m.th.fg)
		if i > 0 {
			part = append([]seg{spaces(3)}, part...)
		}
		if segsWidth(segs)+segsWidth(part) > m.width-2 {
			break // drop lower-priority hints rather than wrap
		}
		segs = append(segs, part...)
	}
	return line(segs, m.width, nil)
}

func (m Model) processUsageKV(u ports.Usage) []kv {
	out := []kv{{"§", "USAGE", nil}}
	if !u.Ready {
		return append(out, kv{"", "Measuring…", nil})
	}
	out = append(out, kv{"CPU", cpuText(u.CPU), nil}, kv{"Memory", bytesText(u.Mem), nil})
	if m.hasEnergy {
		out = append(out, kv{"Power", powerText(u.Power) + " now · " + powerText(u.AvgPower) + " average", m.powerColor(u.Power)})
	}
	if m.hasWakeups {
		out = append(out, kv{"Wakeups", fmt.Sprintf("%.0f per second", u.Wakeups), nil})
	}
	if u.Procs > 1 {
		out = append(out, kv{"", fmt.Sprintf("Includes %s", plural(u.Procs-1, "child process")), nil})
	}
	return out
}

// vmKV explains where container energy goes: Docker can't attribute it per
// container, but the VM that runs them all is an ordinary process.
func (m Model) vmKV() []kv {
	if !m.vm.Ready {
		return nil
	}
	if !m.hasEnergy {
		return []kv{{"Docker VM", bytesText(m.vm.Mem) + " · " + cpuText(m.vm.CPU) + " CPU", nil}}
	}
	v := bytesText(m.vm.Mem) + " · " + powerText(m.vm.Power)
	return []kv{{"Docker VM", v, nil}, {"", "Power is measured for the whole VM", nil}}
}
