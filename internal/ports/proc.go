package ports

import "time"

// procInfo is the slow-to-gather, rarely-changing part of a process.
type procInfo struct {
	Source  string
	Started time.Time
	Comm    string
	Exe     string
	Args    []string
	Cwd     string
}

// socket is one TCP listening socket held by a process.
type socket struct {
	pid, port int
	addr      string // "*" for any address, like lsof
}

// procTable is one snapshot of every process. It feeds socket discovery (our
// own processes), parent names, child trees and the container VM lookup.
type procTable struct {
	own      []int
	ppid     map[int]int
	children map[int][]int
	started  map[int]time.Time
	comm     map[int]string
	counters map[int]counters // filled where the table read already has them (Linux)
}

func newProcTable() procTable {
	return procTable{ppid: map[int]int{}, children: map[int][]int{}, started: map[int]time.Time{}, comm: map[int]string{}}
}

// add records one process.
func (t *procTable) add(pid, ppid int, started time.Time, comm string, own bool) {
	t.ppid[pid] = ppid
	if pid != ppid {
		t.children[ppid] = append(t.children[ppid], pid)
	}
	t.started[pid] = started
	t.comm[pid] = comm
	if own && pid > 0 {
		t.own = append(t.own, pid)
	}
}
