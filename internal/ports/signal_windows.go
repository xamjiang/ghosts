package ports

import (
	"errors"
	"golang.org/x/sys/windows"
	"time"
	"unsafe"
)

var postMessage = windows.NewLazySystemDLL("user32.dll").NewProc("PostMessageW")

type closeRequest struct {
	pid  uint32
	sent bool
}

var closeWindow = windows.NewCallback(func(hwnd windows.HWND, request *closeRequest) uintptr {
	var pid uint32
	if _, err := windows.GetWindowThreadProcessId(hwnd, &pid); err == nil && pid == request.pid {
		const wmClose = 0x0010
		ok, _, _ := postMessage.Call(uintptr(hwnd), wmClose, 0, 0)
		if ok != 0 {
			request.sent = true
		}
	}
	return 1
})

// Keep the verified process handle open through the request and the wait.
// A reused PID must never turn a stale confirmation into a stop of a new process.
func stopProcess(pid int, started time.Time, timeout time.Duration, force bool) (bool, error) {
	if pid <= 0 || started.IsZero() {
		return false, errors.New("missing process identity")
	}
	access := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE)
	if force {
		access |= windows.PROCESS_TERMINATE
	}
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(h)
	when, _, err := processTimes(h)
	if err != nil {
		return false, err
	}
	if !when.Equal(started) {
		return false, errors.New("process has already exited")
	}
	if force {
		if err := windows.TerminateProcess(h, 1); err != nil {
			return false, err
		}
	} else {
		req := closeRequest{pid: uint32(pid)}
		if err := windows.EnumWindows(closeWindow, unsafe.Pointer(&req)); err != nil {
			return false, err
		}
		// Console servers often have no top-level window. The UI offers a separate force confirmation.
		if !req.sent {
			timeout = 0
		}
	}
	state, err := windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
	return state == windows.WAIT_OBJECT_0, err
}

func Terminate(pid int, started time.Time, timeout time.Duration) (bool, error) {
	return stopProcess(pid, started, timeout, false)
}
func Kill(pid int, started time.Time) (bool, error) {
	return stopProcess(pid, started, 2*time.Second, true)
}
