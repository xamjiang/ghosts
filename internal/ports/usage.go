package ports

import (
	"strings"
	"sync"
	"time"
)

// Usage is the resource cost of a process (or a process tree). Rates are
// measured over the last few scans, the same way Activity Monitor does.
type Usage struct {
	CPU      float64 // percent of one core
	Mem      uint64  // physical footprint (macOS, like Activity Monitor) or resident set (Linux), in bytes
	Power    float64 // watts, recent
	AvgPower float64 // watts, averaged over each process's lifetime
	Energy   float64 // joules used since start
	Wakeups  float64 // idle wakeups per second (macOS only)
	Procs    int     // processes counted
	Ready    bool    // rates need two samples; false on the first scan
}

func (u *Usage) add(o Usage) {
	u.CPU += o.CPU
	u.Mem += o.Mem
	u.Power += o.Power
	u.AvgPower += o.AvgPower
	u.Energy += o.Energy
	u.Wakeups += o.Wakeups
	u.Procs += o.Procs
	u.Ready = u.Ready || o.Ready
}

// rateWindow smooths rates: values are computed against the oldest sample
// that is at most this old, so a 2s scan shows a ~6s average.
const rateWindow = 6 * time.Second

// counters are a process's cumulative counters plus its current memory.
type counters struct {
	cpuNs    float64
	energyNJ float64 // 0 where the OS doesn't attribute energy per process
	wakeups  float64
	mem      uint64
}

// counterReader reads one process's counters, given the current table.
type counterReader func(pid int, t procTable) (counters, bool)

type sample struct {
	at time.Time
	counters
}

type procHist struct {
	started time.Time
	samples []sample
}

// sampler turns cumulative per-process counters into rates. Reading them
// costs microseconds and needs no subprocess, so sampling every listener and
// its children on each scan is effectively free.
type sampler struct {
	mu        sync.Mutex
	hist      map[int]*procHist
	read      counterReader
	hasEnergy bool // energy is only attributed per process on Apple silicon
}

func newSampler() *sampler {
	return &sampler{hist: map[int]*procHist{}, read: newCounterReader()}
}

// tree returns pid and all its descendants (bounded, in case of fork bombs).
func (t procTable) tree(pid int) []int {
	out := []int{pid}
	for i := 0; i < len(out) && len(out) < 256; i++ {
		out = append(out, t.children[out[i]]...)
	}
	return out
}

// measure samples every PID once and returns per-process usage.
func (s *sampler) measure(pids map[int]bool, t procTable) map[int]Usage {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := make(map[int]Usage, len(pids))
	for pid := range pids {
		c, ok := s.read(pid, t)
		if !ok {
			continue // gone, or not ours
		}
		cur := sample{at: now, counters: c}
		if cur.energyNJ > 0 {
			s.hasEnergy = true
		}
		u := Usage{Mem: c.mem, Energy: cur.energyNJ / 1e9, Procs: 1}
		started := t.started[pid]
		if life := now.Sub(started).Seconds(); !started.IsZero() && life > 1 {
			u.AvgPower = u.Energy / life
		}

		h := s.hist[pid]
		if h == nil || !h.started.Equal(started) {
			h = &procHist{started: started}
			s.hist[pid] = h
		}
		// Drop samples older than the window, but always keep one to diff against.
		i := 0
		for i < len(h.samples)-1 && now.Sub(h.samples[i+1].at) >= rateWindow {
			i++
		}
		h.samples = append(h.samples[i:], cur)
		if old := h.samples[0]; len(h.samples) > 1 {
			if dt := now.Sub(old.at).Seconds(); dt > 0 {
				u.CPU = (cur.cpuNs - old.cpuNs) / 1e9 / dt * 100
				u.Power = (cur.energyNJ - old.energyNJ) / 1e9 / dt
				u.Wakeups = (cur.wakeups - old.wakeups) / dt
				u.Ready = true
			}
		}
		out[pid] = u
	}
	for pid := range s.hist {
		if !pids[pid] {
			delete(s.hist, pid)
		}
	}
	return out
}

// vmComms are kernel short names (16 chars max) of processes that host
// container VMs. Their energy is what containers really cost.
var vmComms = []string{"com.docker.", "com.apple.Virtu", "OrbStack", "limactl", "qemu-system"}

func (t procTable) vmPIDs() []int {
	var out []int
	for pid, c := range t.comm {
		for _, p := range vmComms {
			if strings.HasPrefix(c, p) {
				out = append(out, pid)
				break
			}
		}
	}
	return out
}
