//go:build darwin || linux

package ports

import (
	"errors"
	"syscall"
	"time"
)

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
