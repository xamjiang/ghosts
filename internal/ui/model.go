// Package ui is the Bubble Tea front end.
package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/xamjiang/ghosts/internal/docker"
	"github.com/xamjiang/ghosts/internal/ports"
)

type tab int

const (
	tabPorts tab = iota
	tabContainers
)

type sortMode int

const (
	sortPort sortMode = iota
	sortEnergy
	sortMemory
	sortNewest
	sortName
	sortModes
)

// sortLabel names the sort as shown; without energy counters (Intel Macs)
// the energy sort falls back to CPU.
func (m Model) sortLabel() string {
	if m.sort == sortEnergy && !m.hasEnergy {
		return "CPU"
	}
	return m.sort.String()
}

func (s sortMode) String() string {
	return [...]string{"Port", "Energy", "Memory", "Newest", "Name"}[s]
}

// How long a freshly appeared service carries the "new" badge.
const newFor = 60 * time.Second

type Options struct {
	Interval time.Duration
	Mouse    bool
}

type toastKind int

const (
	toastOK toastKind = iota
	toastErr
	toastInfo
)

type toast struct {
	text string
	kind toastKind
}

// confirm is a pending destructive action awaiting y/n.
type confirm struct {
	title  string // "Stop vite?"
	detail string // "Port 5173 · PID 81283 · CityMap"
	warn   string // optional caution line
	verb   string // label of the confirm key
	danger bool
	keys   []string // rows to mark busy while running
	busy   string   // "Stopping"
	run    tea.Cmd
}

type Model struct {
	opts    Options
	th      theme
	scanner *ports.Scanner
	dc      *docker.Client

	width, height int

	listeners  []ports.Listener
	procs      map[int]ports.Usage
	vm         ports.Usage
	hasEnergy  bool
	hasWakeups bool
	portsErr   error
	portsReady bool
	scanning   bool
	scanGen    int

	containers    []docker.Container
	dockerUp      bool
	dockerReady   bool
	dockerLoading bool
	dockerDirty   bool

	firstSeen map[string]time.Time

	tab    tab
	rows   []row
	hidden int
	counts [2]int
	cursor [2]int
	offset [2]int
	selKey [2]string

	showAll   bool
	sort      sortMode
	filter    string
	filtering bool

	panel bool // side panel in wide layouts
	zoom  bool // full-screen details in narrow layouts
	help  bool

	confirm  *confirm
	busy     map[string]string
	spinning bool
	frame    int
	toast    *toast
	toastSeq int
	blurred  bool
}

func New(opts Options, dc *docker.Client) Model {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	return Model{
		opts:      opts,
		th:        newTheme(true),
		scanner:   ports.NewScanner(),
		dc:        dc,
		firstSeen: map[string]time.Time{},
		busy:      map[string]string{},
		panel:     true,
	}
}

// Messages.
type (
	scanTickMsg struct{ gen int }
	portsMsg    struct {
		snap ports.Snapshot
		err  error
	}
	DockerChangedMsg struct{}
	dockerMsg        struct {
		cs  []docker.Container
		err error
	}
	actionMsg struct {
		keys  []string
		ok    string // success toast
		err   error
		retry *confirm // offered when a graceful stop timed out
	}
	spinMsg        struct{}
	toastExpireMsg struct{ seq int }
)

// WatchDocker keeps a Docker event stream open and turns every container event
// into a DockerChangedMsg, reconnecting whenever Docker restarts.
func WatchDocker(ctx context.Context, dc *docker.Client, send func(tea.Msg)) {
	notify := func() { send(DockerChangedMsg{}) }
	backoff := time.Second
	for ctx.Err() == nil {
		err := dc.Watch(ctx, func() { backoff = time.Second; notify() }, notify)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			notify() // let the UI notice Docker went away
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

func (m Model) Init() tea.Cmd {
	m.scanning, m.dockerLoading = true, true
	return tea.Batch(tea.RequestBackgroundColor, scanCmd(m.scanner), listCmd(m.dc))
}

func scanCmd(sc *ports.Scanner) tea.Cmd {
	return func() tea.Msg {
		snap, err := sc.Scan()
		return portsMsg{snap, err}
	}
}

func listCmd(dc *docker.Client) tea.Cmd {
	return func() tea.Msg {
		cs, err := dc.List(context.Background())
		if err == nil {
			dc.FillStats(context.Background(), cs)
		}
		return dockerMsg{cs, err}
	}
}

func (m *Model) startScan() tea.Cmd {
	if m.scanning {
		return nil
	}
	m.scanning = true
	return scanCmd(m.scanner)
}

func (m *Model) startList() tea.Cmd {
	if m.dockerLoading {
		m.dockerDirty = true
		return nil
	}
	m.dockerLoading = true
	return listCmd(m.dc)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	cmd := m.update(msg)
	return m, cmd
}

func (m *Model) update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.ensureVisible()

	case tea.BackgroundColorMsg:
		m.th = newTheme(msg.IsDark())

	case tea.FocusMsg:
		m.blurred = false
		return tea.Batch(m.startScan(), m.startList())

	case tea.BlurMsg:
		m.blurred = true

	case portsMsg:
		// Init runs on a copy of the model, so the first result arrives with
		// scanning still false here; either way the scan is now finished.
		m.scanning = false
		m.portsErr = msg.err
		if msg.err == nil {
			m.markNew(msg.snap.Listeners, m.portsReady)
			m.listeners = msg.snap.Listeners
			m.procs, m.vm = msg.snap.Procs, msg.snap.VM
			m.hasEnergy, m.hasWakeups = msg.snap.HasEnergy, msg.snap.HasWakeups
			m.portsReady = true
		}
		m.rebuild()
		m.scanGen++
		interval := m.opts.Interval
		if m.blurred {
			interval *= 3 // nobody is looking; save some cycles
		}
		gen := m.scanGen
		return tea.Tick(interval, func(time.Time) tea.Msg { return scanTickMsg{gen} })

	case scanTickMsg:
		if msg.gen != m.scanGen {
			return nil
		}
		return tea.Batch(m.startScan(), m.startList())

	case DockerChangedMsg:
		return m.startList()

	case dockerMsg:
		m.dockerLoading = false
		m.dockerUp = msg.err == nil
		if msg.err == nil {
			m.markNewContainers(msg.cs, m.dockerReady)
			m.containers = msg.cs
		} else {
			m.containers = nil
		}
		m.dockerReady = true
		m.rebuild()
		if m.dockerDirty {
			m.dockerDirty = false
			return m.startList()
		}

	case actionMsg:
		for _, k := range msg.keys {
			delete(m.busy, k)
		}
		var cmd tea.Cmd
		switch {
		case msg.err != nil:
			cmd = m.showToast(msg.err.Error(), toastErr)
		case msg.retry != nil:
			m.confirm = msg.retry
		case msg.ok != "":
			cmd = m.showToast(msg.ok, toastOK)
		}
		return tea.Batch(cmd, m.startScan(), m.startList())

	case spinMsg:
		if len(m.busy) == 0 {
			m.spinning = false
			return nil
		}
		m.frame++
		return spin()

	case toastExpireMsg:
		if msg.seq == m.toastSeq {
			m.toast = nil
		}

	case tea.MouseWheelMsg:
		if m.confirm != nil || m.help {
			return nil
		}
		switch msg.Button {
		case tea.MouseWheelUp:
			m.move(-1)
		case tea.MouseWheelDown:
			m.move(1)
		}

	case tea.MouseClickMsg:
		return m.click(msg.Mouse())

	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return nil
}

func spin() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinMsg{} })
}

func (m *Model) showToast(text string, kind toastKind) tea.Cmd {
	m.toastSeq++
	m.toast = &toast{text, kind}
	seq := m.toastSeq
	d := 3 * time.Second
	if kind == toastErr {
		d = 6 * time.Second
	}
	return tea.Tick(d, func(time.Time) tea.Msg { return toastExpireMsg{seq} })
}

// markNew records first-seen times. The very first scan is the baseline:
// whatever was already running when the app started isn't "new".
func (m *Model) markNew(ls []ports.Listener, ready bool) {
	now := time.Now()
	for _, l := range ls {
		if _, ok := m.firstSeen[l.Key()]; !ok {
			if ready {
				m.firstSeen[l.Key()] = now
			} else {
				m.firstSeen[l.Key()] = time.Time{}
			}
		}
	}
}

func (m *Model) markNewContainers(cs []docker.Container, ready bool) {
	now := time.Now()
	for _, c := range cs {
		if _, ok := m.firstSeen[c.Key()]; !ok {
			if ready {
				m.firstSeen[c.Key()] = now
			} else {
				m.firstSeen[c.Key()] = time.Time{}
			}
		}
	}
}

func (m Model) isNew(key string) bool {
	t, ok := m.firstSeen[key]
	return ok && !t.IsZero() && time.Since(t) < newFor
}

// rebuild recomputes the rows of the current tab and keeps the selection on
// the same item, so refreshes never make the cursor jump.
func (m *Model) rebuild() {
	vis, hidden := m.visibleListeners()
	m.counts = [2]int{len(vis), len(m.containers)}
	m.hidden = hidden
	if m.tab == tabPorts {
		m.rows = m.buildPortRows()
	} else {
		m.rows = m.buildContainerRows()
	}
	t := m.tab
	idx := -1
	if k := m.selKey[t]; k != "" {
		for i, r := range m.rows {
			if r.key == k {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		idx = m.nearestSelectable(min(m.cursor[t], len(m.rows)-1), 1)
	}
	m.setCursor(idx)
}

func (m *Model) setCursor(i int) {
	t := m.tab
	m.cursor[t] = max(i, 0)
	if i >= 0 && i < len(m.rows) {
		m.selKey[t] = m.rows[i].key
	}
	m.ensureVisible()
}

func (m *Model) nearestSelectable(i, dir int) int {
	if len(m.rows) == 0 {
		return -1
	}
	i = max(0, min(i, len(m.rows)-1))
	for j := i; j >= 0 && j < len(m.rows); j += dir {
		if m.rows[j].selectable() {
			return j
		}
	}
	for j := i; j >= 0 && j < len(m.rows); j -= dir {
		if m.rows[j].selectable() {
			return j
		}
	}
	return -1
}

func (m *Model) move(delta int) {
	if len(m.rows) == 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	i := m.cursor[m.tab] + delta
	i = max(0, min(i, len(m.rows)-1))
	m.setCursor(m.nearestSelectable(i, dir))
}

func (m *Model) ensureVisible() {
	t := m.tab
	h := m.rowsShown()
	c := m.cursor[t]
	off := m.offset[t]
	if c < off {
		off = c
		// Keep a group's spacer/header in view when scrolling up onto it.
		if off > 0 && m.rows[off-1].kind == rowSpacer {
			off--
		}
	}
	if c >= off+h {
		off = c - h + 1
	}
	off = max(0, min(off, len(m.rows)-h))
	m.offset[t] = off
}

func (m Model) selected() *row {
	i := m.cursor[m.tab]
	if i < 0 || i >= len(m.rows) || !m.rows[i].selectable() {
		return nil
	}
	return &m.rows[i]
}

func (m *Model) switchTab(t tab) {
	if m.tab == t {
		return
	}
	m.tab = t
	m.zoom = false
	m.rebuild()
}

func (m *Model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}

	if m.confirm != nil {
		switch k {
		case "y", "Y", "enter":
			c := m.confirm
			m.confirm = nil
			for _, key := range c.keys {
				m.busy[key] = c.busy
			}
			cmds := []tea.Cmd{c.run}
			if !m.spinning {
				m.spinning = true
				cmds = append(cmds, spin())
			}
			return tea.Batch(cmds...)
		case "n", "N", "esc", "q":
			m.confirm = nil
		}
		return nil
	}

	if m.help {
		m.help = false
		return nil
	}

	if m.filtering {
		switch k {
		case "esc":
			m.filter, m.filtering = "", false
		case "enter":
			m.filtering = false
		case "backspace":
			if r := []rune(m.filter); len(r) > 0 {
				m.filter = string(r[:len(r)-1])
			}
		case "ctrl+u":
			m.filter = ""
		case "up", "ctrl+p":
			m.move(-1)
			return nil
		case "down", "ctrl+n":
			m.move(1)
			return nil
		default:
			if t := msg.Key().Text; t != "" {
				m.filter += t
			}
		}
		m.rebuild()
		// Like a search field: as the query changes, the top match is selected.
		m.setCursor(m.nearestSelectable(0, 1))
		return nil
	}

	switch k {
	case "q":
		return tea.Quit
	case "esc":
		switch {
		case m.zoom:
			m.zoom = false
		case m.filter != "":
			m.filter = ""
			m.rebuild()
		}
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "pgup", "ctrl+b":
		m.move(-m.rowsShown())
	case "pgdown", "ctrl+f", "space", " ":
		m.move(m.rowsShown())
	case "home", "g":
		m.setCursor(m.nearestSelectable(0, 1))
	case "end", "G":
		m.setCursor(m.nearestSelectable(len(m.rows)-1, -1))
	case "tab", "right", "l", "shift+tab", "left", "h":
		m.switchTab(1 - m.tab)
	case "1":
		m.switchTab(tabPorts)
	case "2":
		m.switchTab(tabContainers)
	case "enter":
		if m.wide() {
			m.panel = !m.panel
		} else if m.selected() != nil {
			m.zoom = !m.zoom
		}
	case "/":
		m.filtering = true
	case "a":
		m.showAll = !m.showAll
		m.rebuild()
	case "s":
		if m.tab == tabPorts {
			m.sort = (m.sort + 1) % sortModes
			m.rebuild()
			m.setCursor(m.nearestSelectable(0, 1))
			return m.showToast("Sorted by "+strings.ToLower(m.sortLabel()), toastInfo)
		}
	case "r":
		return tea.Batch(m.startScan(), m.startList())
	case "?":
		m.help = true
	case "o":
		return m.open()
	case "x":
		m.askStop(false)
	case "X":
		m.askStop(true)
	}
	return nil
}

func (m *Model) click(ms tea.Mouse) tea.Cmd {
	if m.confirm != nil || m.help || ms.Button != tea.MouseLeft {
		return nil
	}
	if ms.Y == tabsRow {
		for i, r := range m.tabRanges() {
			if ms.X >= r[0] && ms.X < r[1] {
				m.switchTab(tab(i))
				return nil
			}
		}
		return nil
	}
	listW, _ := m.split()
	y := ms.Y - listTop
	if m.zoom || ms.X >= listW || y < 0 || y >= m.rowsShown() {
		return nil
	}
	i := m.offset[m.tab] + y
	if i < len(m.rows) && m.rows[i].selectable() {
		m.setCursor(i)
	}
	return nil
}

// urlFor returns the local URL a row can be opened at, if any.
func urlFor(r *row) string {
	switch {
	case r == nil:
		return ""
	case r.kind == rowListener:
		return fmt.Sprintf("http://localhost:%d", r.l.Port)
	case r.kind == rowContainer && len(r.ctr.Ports) > 0:
		return fmt.Sprintf("http://localhost:%d", r.ctr.Ports[0].Public)
	}
	return ""
}

func (m *Model) open() tea.Cmd {
	u := urlFor(m.selected())
	if u == "" {
		return m.showToast("Nothing to open here", toastInfo)
	}
	cmd, verb, err := openURL(u)
	if err != nil {
		return m.showToast(err.Error(), toastErr)
	}
	return tea.Batch(cmd, m.showToast(verb+" "+strings.TrimPrefix(u, "http://"), toastInfo))
}

func (m *Model) askStop(force bool) {
	r := m.selected()
	if r == nil {
		return
	}
	switch {
	case r.kind == rowGroup:
		m.confirm = m.confirmContainers(r.members, r.group, force)
	case r.ctr != nil:
		m.confirm = m.confirmContainers([]docker.Container{*r.ctr}, "", force)
	case r.kind == rowListener:
		if _, busy := m.busy[r.key]; busy {
			return
		}
		m.confirm = confirmStop(*r.l, force)
	}
}

func confirmStop(l ports.Listener, force bool) *confirm {
	c := &confirm{
		title:  fmt.Sprintf("Stop %s?", l.Name),
		detail: fmt.Sprintf("Port %d · PID %d", l.Port, l.PID),
		verb:   "Stop",
		keys:   []string{l.Key()},
		busy:   "Stopping",
		run:    stopCmd(l, force),
	}
	if l.Project != "" {
		_, base := splitPath(tilde(l.Project))
		c.detail += " · " + base
	}
	if force {
		c.title = fmt.Sprintf("Force stop %s?", l.Name)
		c.verb, c.danger = "Force Stop", true
		c.warn = "Unsaved work in this process will be lost."
	}
	switch l.Kind {
	case ports.KindSystem:
		c.warn, c.danger = "This is a system service and may restart on its own.", true
	case ports.KindApp:
		c.warn = "This belongs to an app; stopping it may break that app."
	}
	return c
}

func stopCmd(l ports.Listener, force bool) tea.Cmd {
	return func() tea.Msg {
		msg := actionMsg{keys: []string{l.Key()}}
		var gone bool
		var err error
		if force {
			gone, err = ports.Kill(l.PID, l.Started)
		} else {
			gone, err = ports.Terminate(l.PID, l.Started, 3*time.Second)
		}
		switch {
		case err != nil:
			msg.err = fmt.Errorf("Couldn't stop %s: %v", l.Name, err)
		case gone && force:
			msg.ok = fmt.Sprintf("Force stopped %s · port %d is free", l.Name, l.Port)
		case gone:
			msg.ok = fmt.Sprintf("Stopped %s · port %d is free", l.Name, l.Port)
		case force:
			msg.err = fmt.Errorf("%s is still running after SIGKILL", l.Name)
		default:
			retry := confirmStop(l, true)
			retry.title = fmt.Sprintf("%s is still running. Force stop?", l.Name)
			retry.warn = "It ignored the request to stop for 3 seconds."
			msg.retry = retry
		}
		return msg
	}
}

func (m *Model) confirmContainers(cs []docker.Container, group string, force bool) *confirm {
	verb, busy := "Stop", "Stopping"
	if force {
		verb, busy = "Kill", "Killing"
	}
	c := &confirm{verb: verb, busy: busy, danger: force}
	var ids, names []string
	for _, ct := range cs {
		ids = append(ids, ct.ID)
		names = append(names, ct.Name)
		c.keys = append(c.keys, ct.Key())
	}
	if group != "" {
		c.keys = append(c.keys, "g:"+group)
		c.title = fmt.Sprintf("%s %s?", verb, group)
		c.detail = plural(len(cs), "container") + " · " + strings.Join(names, ", ")
	} else {
		c.title = fmt.Sprintf("%s %s?", verb, cs[0].Name)
		c.detail = "Container · " + cs[0].Image
	}
	if force {
		c.warn = "Containers are killed immediately, without a graceful shutdown."
	}
	dc := m.dc
	what := names[0]
	if group != "" {
		what = group
	}
	c.run = func() tea.Msg {
		errs := make(chan error, len(ids))
		for _, id := range ids {
			go func() {
				if force {
					errs <- dc.Kill(context.Background(), id)
				} else {
					errs <- dc.Stop(context.Background(), id)
				}
			}()
		}
		var first error
		for range ids {
			if err := <-errs; err != nil && first == nil {
				first = err
			}
		}
		msg := actionMsg{keys: c.keys}
		if first != nil {
			msg.err = fmt.Errorf("Couldn't %s %s: %v", strings.ToLower(verb), what, first)
		} else {
			past := map[string]string{"Stop": "Stopped", "Kill": "Killed"}[verb]
			msg.ok = past + " " + what
		}
		return msg
	}
	return c
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}
