// Package ports discovers TCP listeners on macOS and Linux and describes the
// processes behind them in human terms (project directory, friendly name, uptime).
package ports

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind tells whether a listener is something the user is likely developing
// (a dev server, a script) or part of an app / the OS.
type Kind int

const (
	KindDev Kind = iota
	KindApp
	KindSystem
)

// Listener is one process listening on one TCP port.
type Listener struct {
	Port       int
	PID        int
	PPID       int
	Addrs      []string // "*", "127.0.0.1", "::1"…
	Comm       string   // kernel short name
	Exe        string
	Args       []string
	Cwd        string
	Started    time.Time
	ParentName string
	Detached   bool // orphaned: its terminal or agent session is gone

	Name    string // friendly display name, e.g. "vite"
	Runtime string // e.g. "node", "python"; empty for native binaries
	Project string // best-guess project directory
	Kind    Kind

	Usage Usage // the process plus its descendants
	Tree  []int // PIDs counted in Usage
}

// Snapshot is the result of one scan.
type Snapshot struct {
	Listeners  []Listener
	Procs      map[int]Usage // per PID, for totals that don't double count
	VM         Usage         // the VM that runs containers (Docker Desktop, OrbStack…)
	HasEnergy  bool          // energy counters exist (Apple silicon)
	HasWakeups bool          // wakeup counters exist (macOS)
}

// Key identifies a listener across scans.
func (l Listener) Key() string { return fmt.Sprintf("p:%d:%d", l.PID, l.Port) }

// Public reports whether the socket accepts connections from other machines.
func (l Listener) Public() bool {
	for _, a := range l.Addrs {
		switch a {
		case "*", "0.0.0.0", "::", "[::]":
			return true
		}
		if !strings.HasPrefix(a, "127.") && a != "::1" && a != "[::1]" {
			return true
		}
	}
	return false
}

// CommandLine returns the full command as a single string.
func (l Listener) CommandLine() string {
	if len(l.Args) == 0 {
		return l.Exe
	}
	return strings.Join(l.Args, " ")
}

// Scanner finds listening sockets and enriches them. It reads the kernel
// directly (sysctl + libproc on macOS, /proc on Linux) instead of running lsof, so a scan takes a few
// milliseconds and spawns no processes. Slow-changing details (argv, working
// directory) are cached by PID and start time.
type Scanner struct {
	mu    sync.Mutex
	cache map[int]procInfo
	usage *sampler
}

func NewScanner() *Scanner {
	return &Scanner{cache: map[int]procInfo{}, usage: newSampler()}
}

// Scan returns all TCP listeners owned by the current user, sorted by port,
// with the resource usage of each one's process tree.
func (s *Scanner) Scan() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	table, err := readProcTable()
	if err != nil {
		return Snapshot{}, fmt.Errorf("reading process table: %w", err)
	}

	// Merge IPv4/IPv6 sockets of the same process and port into one listener.
	type key struct{ pid, port int }
	byKey := map[key]*Listener{}
	var order []key
	for _, so := range listenSockets(table.own) {
		k := key{so.pid, so.port}
		l := byKey[k]
		if l == nil {
			l = &Listener{PID: so.pid, Port: so.port}
			byKey[k] = l
			order = append(order, k)
		}
		if !contains(l.Addrs, so.addr) {
			l.Addrs = append(l.Addrs, so.addr)
		}
	}

	live := map[int]bool{}
	res := make([]Listener, 0, len(order))
	for _, k := range order {
		l := byKey[k]
		info, ok := s.info(l.PID, table)
		if !ok {
			continue // exited mid-scan
		}
		live[l.PID] = true
		l.PPID = table.ppid[l.PID]
		l.Started, l.Comm, l.Exe, l.Args, l.Cwd = info.Started, info.Comm, info.Exe, info.Args, info.Cwd
		l.Detached = detached(l.PID, l.PPID, table)
		switch {
		case l.PPID == 1:
			l.ParentName = initName
		case l.PPID > 1:
			l.ParentName = table.comm[l.PPID]
		}
		describe(l)
		res = append(res, *l)
	}
	for pid := range s.cache {
		if !live[pid] {
			delete(s.cache, pid)
		}
	}
	nameWorkers(res)
	sort.SliceStable(res, func(i, j int) bool {
		if res[i].Port != res[j].Port {
			return res[i].Port < res[j].Port
		}
		return res[i].PID < res[j].PID
	})
	return s.measure(res, table), nil
}

// info returns cached details for pid, refreshing them if the PID now
// belongs to a different process.
func (s *Scanner) info(pid int, table procTable) (procInfo, bool) {
	started, ok := table.started[pid]
	if !ok {
		return procInfo{}, false
	}
	if c, ok := s.cache[pid]; ok && c.Started.Equal(started) {
		return c, true
	}
	c := procInfo{Started: started, Comm: table.comm[pid], Cwd: cwd(pid)}
	c.Exe, c.Args, _ = procArgs(pid)
	s.cache[pid] = c
	return c, true
}

// measure attaches usage to each listener's process tree and to the
// container VM, sampling every involved PID exactly once.
func (s *Scanner) measure(ls []Listener, table procTable) Snapshot {
	pids := map[int]bool{}
	for i := range ls {
		ls[i].Tree = table.tree(ls[i].PID)
		for _, p := range ls[i].Tree {
			pids[p] = true
		}
	}
	vm := table.vmPIDs()
	for _, p := range vm {
		pids[p] = true
	}
	per := s.usage.measure(pids, table)
	snap := Snapshot{Listeners: ls, Procs: per, HasEnergy: s.usage.hasEnergy, HasWakeups: hasWakeups}
	for i := range ls {
		for _, p := range ls[i].Tree {
			if u, ok := per[p]; ok {
				ls[i].Usage.add(u)
			}
		}
	}
	for _, p := range vm {
		if u, ok := per[p]; ok {
			snap.VM.add(u)
		}
	}
	return snap
}

// nameWorkers renames anonymous children that share their parent's socket
// (uvicorn/gunicorn workers, cluster mode) to "<parent> worker".
func nameWorkers(ls []Listener) {
	byPID := map[[2]int]*Listener{}
	for i := range ls {
		byPID[[2]int{ls[i].PID, ls[i].Port}] = &ls[i]
	}
	for i := range ls {
		l := &ls[i]
		parent := byPID[[2]int{l.PPID, l.Port}]
		if parent == nil || l.Runtime != "" {
			continue
		}
		if _, generic := interpreters[strings.ToLower(l.Name)]; generic {
			l.Name, l.Runtime = parent.Name+" worker", parent.Runtime
		}
	}
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
