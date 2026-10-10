package ports

import (
	"maps"
	"net"
	"os"
	"testing"
)

func TestClassifyLinux(t *testing.T) {
	cases := map[string]Kind{
		"/usr/libexec/gnome-remote-desktop-daemon":            KindSystem,
		"/usr/lib/systemd/systemd-resolved":                   KindSystem,
		"/usr/share/code/code":                                KindApp,
		"/opt/google/chrome/chrome":                           KindApp,
		"/usr/NX/bin/nxnode.bin":                              KindApp,
		"/snap/spotify/80/usr/share/spotify/spotify":          KindApp,
		"/snap/node/9000/bin/node":                            KindDev,
		"/usr/lib/jvm/java-21-openjdk-amd64/bin/java":         KindDev,
		"/usr/bin/python3.12":                                 KindDev,
		"/usr/local/bin/redis-server":                         KindDev,
		"/home/me/.cache/go-build/ab/cd/exe/main":             KindDev,
		"/home/me/.local/share/flatpak/app/x/current/bin/app": KindApp,
	}
	for exe, want := range cases {
		if got := classify(exe); got != want {
			t.Errorf("classify(%q) = %v, want %v", exe, got, want)
		}
	}
}

func TestParseStat(t *testing.T) {
	// A comm with spaces and parentheses must not shift the fields.
	line := "4223 (Web Content (x)) S 4100 4223 4223 0 -1 4194304 1000 0 0 0 150 50 0 0 20 0 30 0 123456 1000000 2500 18446744073709551615"
	s, err := parseStat([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	if s.comm != "Web Content (x)" || s.state != 'S' || s.ppid != 4100 || s.cpuTicks != 200 || s.startTick != 123456 || s.rssPages != 2500 {
		t.Errorf("got %+v", s)
	}
}

func TestParseProcIP(t *testing.T) {
	cases := map[string]string{
		"0100007F":                         "127.0.0.1",
		"00000000":                         "0.0.0.0",
		"00000000000000000000000001000000": "::1",
		"0000000000000000FFFF00000100007F": "127.0.0.1", // v4-mapped
	}
	for in, want := range cases {
		if got := parseProcIP(in); !got.Equal(net.ParseIP(want)) {
			t.Errorf("parseProcIP(%q) = %v, want %s", in, got, want)
		}
	}
}

// The netlink and /proc readers must agree.
func TestDiagMatchesProc(t *testing.T) {
	uid := os.Getuid()
	diag, proc := map[uint64]tcpListen{}, map[uint64]tcpListen{}
	if err := diagListeners(uid, diag); err != nil {
		t.Skip("sock_diag unavailable:", err)
	}
	readTCP("/proc/net/tcp", uid, proc)
	readTCP("/proc/net/tcp6", uid, proc)
	if !maps.Equal(diag, proc) {
		t.Errorf("sock_diag %v\n/proc     %v", diag, proc)
	}
}
