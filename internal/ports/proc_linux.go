package ports

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Everything here comes from /proc: one read of /proc/<pid>/stat per process
// gives the table, start times, CPU time and memory, so a scan forks nothing.

// clockTicks is USER_HZ, the unit of times in /proc/<pid>/stat. The kernel
// fixes it at 100 on every architecture Go supports.
const clockTicks = 100

var pageSize = uint64(os.Getpagesize())

// bootTime is read once: /proc/stat recomputes it from the wall clock on
// every read, so it can drift by a second after NTP adjustments, and start
// times must compare equal across scans.
var bootTime = sync.OnceValue(func() time.Time {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return time.Time{}
	}
	for line := range strings.Lines(string(b)) {
		if v, ok := strings.CutPrefix(line, "btime "); ok {
			sec, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			return time.Unix(sec, 0)
		}
	}
	return time.Time{}
})

// procStat is the part of /proc/<pid>/stat we use.
type procStat struct {
	comm      string
	state     byte
	ppid      int
	cpuTicks  uint64 // utime + stime, whole process
	startTick uint64 // since boot
	rssPages  uint64
	uid       uint32 // owner of the stat file: the process's effective uid
}

func (s procStat) started() time.Time {
	return bootTime().Add(time.Duration(s.startTick) * time.Second / clockTicks)
}

func (s procStat) counters() counters {
	return counters{cpuNs: float64(s.cpuTicks) * 1e9 / clockTicks, mem: s.rssPages * pageSize}
}

// readStat reads /proc/<pid>/stat with raw syscalls into buf, avoiding the
// per-file allocations of os.Open for the hundreds of processes on a system.
func readStat(pid int, buf []byte) (procStat, error) {
	fd, err := unix.Open("/proc/"+strconv.Itoa(pid)+"/stat", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return procStat{}, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return procStat{}, err
	}
	n, err := unix.Read(fd, buf)
	if err != nil {
		return procStat{}, err
	}
	s, err := parseStat(buf[:n])
	s.uid = st.Uid
	return s, err
}

// parseStat parses "pid (comm) state ppid …". The command name may itself
// contain spaces and parentheses, so fields are counted from the last ')'.
func parseStat(b []byte) (procStat, error) {
	open, end := bytes.IndexByte(b, '('), bytes.LastIndexByte(b, ')')
	if open < 0 || end < open || end+2 > len(b) {
		return procStat{}, errors.New("malformed stat")
	}
	s := procStat{comm: string(b[open+1 : end])}
	// Fields after the comm, numbered from 0 = state (field 3 in proc(5)).
	rest := b[end+2:]
	for i := 0; i <= 21 && len(rest) > 0; i++ {
		j := bytes.IndexByte(rest, ' ')
		if j < 0 {
			j = len(rest)
		}
		f := rest[:j]
		switch i {
		case 0:
			s.state = f[0]
		case 1:
			s.ppid = atoi(f)
		case 11, 12: // utime, stime
			s.cpuTicks += uint64(atoi(f))
		case 19:
			s.startTick = uint64(atoi(f))
		case 21:
			s.rssPages = uint64(atoi(f))
		}
		rest = rest[min(j+1, len(rest)):]
	}
	return s, nil
}

func atoi(b []byte) int {
	n := 0
	for _, c := range b {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// readProcTable reads every process in /proc.
func readProcTable() (procTable, error) {
	t := newProcTable()
	t.counters = map[int]counters{}
	d, err := os.Open("/proc")
	if err != nil {
		return t, err
	}
	names, err := d.Readdirnames(-1)
	d.Close()
	if err != nil {
		return t, err
	}
	uid := uint32(os.Getuid())
	buf := make([]byte, 2048)
	for _, name := range names {
		pid, err := strconv.Atoi(name)
		if err != nil {
			continue
		}
		s, err := readStat(pid, buf)
		if err != nil {
			continue // exited while listing
		}
		t.add(pid, s.ppid, s.started(), s.comm, s.uid == uid)
		t.counters[pid] = s.counters()
	}
	return t, nil
}

// startTime returns when a process started.
func startTime(pid int) (time.Time, error) {
	s, err := readStat(pid, make([]byte, 2048))
	if err != nil {
		return time.Time{}, err
	}
	return s.started(), nil
}

// procArgs reads the executable path and argv of a process.
func procArgs(pid int) (exe string, args []string, err error) {
	dir := "/proc/" + strconv.Itoa(pid)
	exe, _ = os.Readlink(dir + "/exe")
	exe = strings.TrimSuffix(exe, " (deleted)") // rebuilt binaries, e.g. go run
	b, err := os.ReadFile(dir + "/cmdline")
	if err != nil {
		return exe, nil, err
	}
	b = bytes.TrimRight(b, "\x00")
	if len(b) > 0 {
		args = strings.Split(string(b), "\x00")
	}
	return exe, args, nil
}

// cwd returns a process's working directory.
func cwd(pid int) string {
	d, _ := os.Readlink("/proc/" + strconv.Itoa(pid) + "/cwd")
	return d
}

// Alive reports whether a process with the given PID still exists. Zombies
// (exited but not yet reaped by their parent) count as gone.
func Alive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	s, err := readStat(pid, make([]byte, 2048))
	return err == nil && s.state != 'Z' && s.state != 'X'
}

// initName is what the PID 1 parent of a detached process is called.
const initName = "systemd"

// detached reports whether a process lost its session. Orphans are adopted
// by PID 1, or by the user's systemd instance (a subreaper) on desktops.
// Services that systemd started on purpose have the same parents, but run in
// their own "*.service" cgroup, while a server left behind by a closed
// terminal or SSH login stays in that session's "*.scope".
func detached(pid, ppid int, t procTable) bool {
	if ppid != 1 && t.comm[ppid] != "systemd" {
		return false
	}
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cgroup")
	if err != nil {
		return ppid == 1
	}
	// cgroup v2: "0::/user.slice/user-1000.slice/session-3.scope"
	for line := range strings.Lines(string(b)) {
		if strings.HasPrefix(line, "0::") {
			return !strings.HasSuffix(strings.TrimSpace(line), ".service")
		}
	}
	return ppid == 1
}

// newCounterReader reuses the counters read along with the process table.
func newCounterReader() counterReader {
	return func(pid int, t procTable) (counters, bool) {
		c, ok := t.counters[pid]
		return c, ok
	}
}

// hasWakeups: Linux only counts context switches, and only per thread.
const hasWakeups = false

// Linux has no per-process socket query. /proc/net/tcp lists every socket
// with its owner and inode, and mapping an inode back to processes means
// reading the links in /proc/<pid>/fd — every open file of every process we
// own, thousands on a desktop. So holders are cached, and the walk is redone
// only when an unknown listening socket appears, or every holdersTTL to pick
// up workers that inherited a socket that was already known.
const holdersTTL = 30 * time.Second

var sockCache struct {
	mu      sync.Mutex
	holders map[uint64][]int // inode → PIDs holding it
	at      time.Time
}

type tcpListen struct {
	port int
	addr string
}

// listenSockets returns the TCP listening sockets of the given processes.
func listenSockets(pids []int) []socket {
	uid := os.Getuid()
	lns := map[uint64]tcpListen{}
	if err := diagListeners(uid, lns); err != nil {
		readTCP("/proc/net/tcp", uid, lns)
		readTCP("/proc/net/tcp6", uid, lns)
	}
	if len(lns) == 0 {
		return nil
	}
	own := make(map[int]bool, len(pids))
	for _, p := range pids {
		own[p] = true
	}

	sockCache.mu.Lock()
	defer sockCache.mu.Unlock()
	stale := time.Since(sockCache.at) > holdersTTL
	for ino := range lns {
		if _, ok := sockCache.holders[ino]; !ok {
			stale = true
		}
	}
	if stale {
		sockCache.holders = socketHolders(pids, lns)
		sockCache.at = time.Now()
	}

	var res []socket
	for ino, ln := range lns {
		for _, pid := range sockCache.holders[ino] {
			if own[pid] {
				res = append(res, socket{pid: pid, port: ln.port, addr: ln.addr})
			}
		}
	}
	return res
}

// socketHolders finds which of pids hold each wanted socket inode. Inodes no
// process holds (owned by root, or in another namespace) map to nil, so they
// don't trigger a new walk on every scan.
func socketHolders(pids []int, want map[uint64]tcpListen) map[uint64][]int {
	out := make(map[uint64][]int, len(want))
	for ino := range want {
		out[ino] = nil
	}
	for _, pid := range pids {
		dir := "/proc/" + strconv.Itoa(pid) + "/fd/"
		d, err := os.Open(dir)
		if err != nil {
			continue
		}
		fds, _ := d.Readdirnames(-1)
		d.Close()
		seen := map[uint64]bool{}
		for _, fd := range fds {
			link, err := os.Readlink(dir + fd)
			if err != nil || !strings.HasPrefix(link, "socket:[") {
				continue
			}
			ino, err := strconv.ParseUint(link[len("socket:["):len(link)-1], 10, 64)
			if err != nil || seen[ino] {
				continue
			}
			if _, ok := want[ino]; ok {
				seen[ino] = true
				out[ino] = append(out[ino], pid)
			}
		}
	}
	return out
}

// diagListeners asks the kernel for listening TCP sockets over netlink
// (sock_diag, what ss uses). Unlike /proc/net/tcp, which walks every bucket
// of the connection hash table (milliseconds on a machine with lots of RAM),
// a dump filtered to LISTEN only visits the listening sockets.
func diagListeners(uid int, out map[uint64]tcpListen) error {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	for _, family := range []byte{unix.AF_INET, unix.AF_INET6} {
		if err := diagDump(fd, family, uid, out); err != nil {
			return err
		}
	}
	return nil
}

// diagDump sends one inet_diag_req_v2 and reads the multipart reply.
func diagDump(fd int, family byte, uid int, out map[uint64]tcpListen) error {
	const (
		hdrLen = 16 // nlmsghdr
		reqLen = 56 // inet_diag_req_v2
		msgLen = 72 // inet_diag_msg
	)
	req := make([]byte, hdrLen+reqLen)
	ne := binary.NativeEndian
	ne.PutUint32(req[0:], uint32(len(req)))
	ne.PutUint16(req[4:], unix.SOCK_DIAG_BY_FAMILY)
	ne.PutUint16(req[6:], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	req[hdrLen] = family
	req[hdrLen+1] = unix.IPPROTO_TCP
	ne.PutUint32(req[hdrLen+4:], 1<<10) // idiag_states: TCP_LISTEN (10)
	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return err
	}
	buf := make([]byte, 32<<10)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return err
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return err
		}
		for _, m := range msgs {
			switch m.Header.Type {
			case unix.NLMSG_DONE:
				return nil
			case unix.NLMSG_ERROR:
				return errors.New("sock_diag: request refused")
			}
			d := m.Data
			if len(d) < msgLen || int(ne.Uint32(d[64:])) != uid {
				continue
			}
			// inet_diag_msg: family, state, timer, retrans, then the
			// socket id (big-endian port, raw address), …, uid, inode.
			ip := net.IP(append([]byte(nil), d[8:8+4]...))
			if d[0] == unix.AF_INET6 {
				ip = net.IP(append([]byte(nil), d[8:8+16]...))
			}
			addr := ip.String()
			if ip.IsUnspecified() {
				addr = "*"
			}
			out[uint64(ne.Uint32(d[68:]))] = tcpListen{port: int(binary.BigEndian.Uint16(d[4:])), addr: addr}
		}
	}
}

// readTCP adds the listening sockets owned by uid from /proc/net/tcp{,6}:
//
//	sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
//	0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 123456 …
func readTCP(path string, uid int, out map[uint64]tcpListen) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Scan() // header
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 10 || fs[3] != "0A" { // TCP_LISTEN
			continue
		}
		if u, _ := strconv.Atoi(fs[7]); u != uid {
			continue
		}
		ino, err := strconv.ParseUint(fs[9], 10, 64)
		if err != nil || ino == 0 {
			continue
		}
		host, port, ok := strings.Cut(fs[1], ":")
		if !ok {
			continue
		}
		p, err := strconv.ParseUint(port, 16, 16)
		if err != nil {
			continue
		}
		ip := parseProcIP(host)
		if ip == nil {
			continue
		}
		addr := ip.String()
		if ip.IsUnspecified() {
			addr = "*"
		}
		out[ino] = tcpListen{port: int(p), addr: addr}
	}
}

// parseProcIP decodes the kernel's hex address: the raw bytes, printed as
// 32-bit words in host byte order.
func parseProcIP(h string) net.IP {
	b, err := hex.DecodeString(h)
	if err != nil || (len(b) != 4 && len(b) != 16) {
		return nil
	}
	ip := make(net.IP, len(b))
	for i := 0; i < len(b); i += 4 {
		binary.NativeEndian.PutUint32(ip[i:], binary.BigEndian.Uint32(b[i:]))
	}
	return ip
}
