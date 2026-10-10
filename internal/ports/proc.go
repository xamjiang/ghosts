package ports

import (
	"errors"
	"syscall"
	"time"
)

// procInfo is the slow-to-gather, rarely-changing part of a process.
type procInfo struct {
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

// Signal sends sig to pid, but only if the process is still the one that was
// started at the given time. This guards against PID reuse between the moment
// the user saw the row and the moment they confirmed.
func Signal(pid int, started time.Time, sig syscall.Signal) error {
	s, err := startTime(pid)
	if err != nil {
		return errors.New("process has already exited")
	}
	if !started.IsZero() && !s.Equal(started) {
		return errors.New("process has already exited")
	}
	return syscall.Kill(pid, sig)
}

// Terminate sends SIGTERM and waits up to timeout for the process to exit.
// It returns (true, nil) if the process is gone.
func Terminate(pid int, started time.Time, timeout time.Duration) (bool, error) {
	if err := Signal(pid, started, syscall.SIGTERM); err != nil {
		return false, err
	}
	return waitExit(pid, timeout), nil
}

// Kill sends SIGKILL and waits briefly for the process to disappear.
func Kill(pid int, started time.Time) (bool, error) {
	if err := Signal(pid, started, syscall.SIGKILL); err != nil {
		return false, err
	}
	return waitExit(pid, 2*time.Second), nil
}

func waitExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !Alive(pid) {
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return !Alive(pid)
}
