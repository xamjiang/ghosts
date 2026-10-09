package ports

import (
	"bytes"
	"encoding/binary"
	"errors"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// procInfo is the slow-to-gather, rarely-changing part of a process.
type procInfo struct {
	Started time.Time
	Comm    string
	Exe     string
	Args    []string
	Cwd     string
}

// kinfo returns the parent PID, start time and short name of a process using
// sysctl, which is a single cheap syscall and needs no subprocess.
func kinfo(pid int) (ppid int, started time.Time, comm string, err error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return 0, time.Time{}, "", err
	}
	if kp.Proc.P_pid == 0 && pid != 0 {
		return 0, time.Time{}, "", errors.New("no such process")
	}
	tv := kp.Proc.P_starttime
	started = time.Unix(tv.Sec, int64(tv.Usec)*1000)
	c := kp.Proc.P_comm[:]
	if i := bytes.IndexByte(c, 0); i >= 0 {
		c = c[:i]
	}
	return int(kp.Eproc.Ppid), started, string(c), nil
}

// procArgs reads the executable path and argv of a process via
// kern.procargs2. Layout: int32 argc, exec path, NUL padding, argv[0..argc).
func procArgs(pid int) (exe string, args []string, err error) {
	buf, err := unix.SysctlRaw("kern.procargs2", pid)
	if err != nil {
		return "", nil, err
	}
	if len(buf) < 4 {
		return "", nil, errors.New("short procargs2")
	}
	argc := int(binary.LittleEndian.Uint32(buf[:4]))
	rest := buf[4:]
	i := bytes.IndexByte(rest, 0)
	if i < 0 {
		return string(rest), nil, nil
	}
	exe = string(rest[:i])
	rest = rest[i:]
	for len(rest) > 0 && rest[0] == 0 {
		rest = rest[1:]
	}
	for n := 0; n < argc && len(rest) > 0; n++ {
		j := bytes.IndexByte(rest, 0)
		if j < 0 {
			args = append(args, string(rest))
			break
		}
		args = append(args, string(rest[:j]))
		rest = rest[j+1:]
	}
	return exe, args, nil
}

// Alive reports whether a process with the given PID still exists. Zombies
// (exited but not yet reaped by their parent) count as gone.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || kp.Proc.P_pid == 0 {
		return false
	}
	const szomb = 5 // SZOMB from <sys/proc.h>
	return kp.Proc.P_stat != szomb
}

// Signal sends sig to pid, but only if the process is still the one that was
// started at the given time. This guards against PID reuse between the moment
// the user saw the row and the moment they confirmed.
func Signal(pid int, started time.Time, sig syscall.Signal) error {
	_, s, _, err := kinfo(pid)
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
