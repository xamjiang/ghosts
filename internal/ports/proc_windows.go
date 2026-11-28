package ports

import (
	"errors"
	"path/filepath"
	"time"
	"unsafe"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/windows"
)

const initName = "Windows"
const hasWakeups = false

func processTimes(h windows.Handle) (time.Time, float64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return time.Time{}, 0, err
	}
	ticks := func(t windows.Filetime) uint64 { return uint64(t.HighDateTime)<<32 | uint64(t.LowDateTime) }
	return time.Unix(0, created.Nanoseconds()), float64(ticks(kernel)+ticks(user)) * 100, nil
}

func sameUser(h windows.Handle, sid *windows.SID) bool {
	var token windows.Token
	if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token) != nil {
		return false
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	return err == nil && user.User.Sid.Equals(sid)
}

func readProcTable() (procTable, error) {
	t := newProcTable()
	t.counters = make(map[int]counters)
	me, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return t, err
	}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return t, err
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		pid := int(e.ProcessID)
		var started time.Time
		var own bool
		h, openErr := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, e.ProcessID)
		if openErr == nil {
			var cpu float64
			started, cpu, openErr = processTimes(h)
			own = openErr == nil && sameUser(h, me.User.Sid)
			if own {
				t.counters[pid] = counters{cpuNs: cpu}
			}
			windows.CloseHandle(h)
		}
		t.add(pid, int(e.ParentProcessID), started, windows.UTF16ToString(e.ExeFile[:]), own)
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return t, err
	}
	// A Windows parent PID survives its process. Drop stale tree edges before aggregating usage.
	for parent, kids := range t.children {
		valid := kids[:0]
		for _, child := range kids {
			if _, ok := t.ppid[parent]; ok && !t.started[parent].After(t.started[child]) {
				valid = append(valid, child)
			}
		}
		t.children[parent] = valid
	}
	return t, nil
}

func detached(pid, parent int, t procTable) bool {
	_, exists := t.ppid[parent]
	return parent > 0 && (!exists || t.started[parent].After(t.started[pid]))
}

func executablePath(pid int) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var path [32768]uint16
	size := uint32(len(path))
	if windows.QueryFullProcessImageName(h, 0, &path[0], &size) != nil {
		return ""
	}
	return windows.UTF16ToString(path[:size])
}

func procArgs(pid int) (string, []string, error) {
	p := process.Process{Pid: int32(pid)}
	line, err := p.Cmdline()
	exe := executablePath(pid)
	if err != nil || line == "" {
		return exe, nil, err
	}
	args, err := windows.DecomposeCommandLine(line)
	return exe, args, err
}

func cwd(pid int) string {
	p := process.Process{Pid: int32(pid)}
	dir, _ := p.Cwd()
	if dir == "" {
		return ""
	}
	return filepath.Clean(dir)
}

func newCounterReader() counterReader {
	return func(pid int, t procTable) (counters, bool) {
		c, ok := t.counters[pid]
		if !ok {
			return counters{}, false
		}
		p := process.Process{Pid: int32(pid)}
		if m, err := p.MemoryInfo(); err == nil {
			c.mem = m.RSS
		}
		return c, true
	}
}
