package ports

import (
	"bytes"
	"encoding/binary"
	"errors"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// startTime returns when a process started using sysctl, which is a single
// cheap syscall and needs no subprocess.
func startTime(pid int) (time.Time, error) {
	kp, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return time.Time{}, err
	}
	if kp.Proc.P_pid == 0 && pid != 0 {
		return time.Time{}, errors.New("no such process")
	}
	tv := kp.Proc.P_starttime
	return time.Unix(tv.Sec, int64(tv.Usec)*1000), nil
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
