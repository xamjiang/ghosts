package ports

import (
	"golang.org/x/sys/windows"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestWindowsListener(t *testing.T) {
	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			addr := "127.0.0.1:0"
			if network == "tcp6" {
				addr = "[::1]:0"
			}
			ln, err := net.Listen(network, addr)
			if err != nil && network == "tcp6" {
				t.Skip(err)
			}
			if err != nil {
				t.Fatal(err)
			}
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("<title>Ghosts Windows test</title>")) }))
			srv.Listener.Close()
			srv.Listener = ln
			srv.Start()
			defer srv.Close()
			scan := NewScanner()
			snap, err := scan.Scan()
			if err != nil {
				t.Fatal(err)
			}
			for _, l := range snap.Listeners {
				if l.Port != ln.Addr().(*net.TCPAddr).Port || l.PID != os.Getpid() {
					continue
				}
				if l.NewPort || l.Address() != ln.Addr().String() || l.Started.IsZero() || l.Exe == "" || l.Usage.Mem == 0 || l.WebScheme != "http" || l.Title != "Ghosts Windows test" || len(l.Args) == 0 || l.Cwd == "" {
					t.Fatalf("unexpected listener: %+v", l)
				}
				return
			}
			t.Error("test listener missing")
		})
	}
}

func TestWindowsStopIdentity(t *testing.T) {
	if os.Getenv("GHOSTS_STOP_TEST_CHILD") == "1" {
		time.Sleep(time.Minute)
		return
	}
	child := exec.Command(os.Args[0], "-test.run=^TestWindowsStopIdentity$")
	child.Env = append(os.Environ(), "GHOSTS_STOP_TEST_CHILD=1")
	child.SysProcAttr = &windows.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	table, err := readProcTable()
	if err != nil {
		t.Fatal(err)
	}
	pid := child.Process.Pid
	started := table.started[pid]
	if started.IsZero() {
		t.Fatal("missing child identity")
	}
	if _, err := Kill(pid, started.Add(time.Second)); err == nil {
		t.Fatal("accepted stale process identity")
	}
	if gone, err := Terminate(pid, started, 100*time.Millisecond); err != nil || gone {
		t.Fatalf("windowless graceful close: gone=%v error=%v", gone, err)
	}
	if gone, err := Kill(pid, started); err != nil || !gone {
		t.Fatalf("force stop: gone=%v error=%v", gone, err)
	}
}

func TestWindowsDevCommand(t *testing.T) {
	l := Listener{Exe: `C:\Program Files\nodejs\node.exe`, Args: []string{"node.exe", `C:\dev\sample\node_modules\vite\bin\vite.js`}}
	describe(&l)
	if l.Runtime != "node" || l.Name != "vite" || l.Project != `C:\dev\sample` || l.Kind != KindDev {
		t.Fatalf("%+v", l)
	}
	table := newProcTable()
	table.add(20, 1, time.Now(), "Code.exe", true)
	table.add(21, 20, time.Now(), "node.exe", true)
	if got := launchSource(21, table, func(int) string { return "" }); got != "VS Code" {
		t.Fatalf("source %q", got)
	}
}
