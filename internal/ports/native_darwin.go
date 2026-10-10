package ports

/*
#include <libproc.h>
#include <sys/proc_info.h>
#include <stdlib.h>
#include <string.h>
#include <arpa/inet.h>

typedef struct {
	int pid;
	int port;
	int v6;
	unsigned char addr[16];
} sm_listen;

// sm_listeners walks the file descriptors of the given processes and reports
// every TCP socket in LISTEN state. This is what lsof does internally, minus
// the fork/exec and text formatting: a few ms instead of ~50ms per scan.
static int sm_listeners(int *pids, int npids, sm_listen *out, int max) {
	int n = 0, cap = 0;
	struct proc_fdinfo *fds = NULL;
	for (int i = 0; i < npids && n < max; i++) {
		int pid = pids[i];
		int sz = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, NULL, 0);
		if (sz <= 0) continue;
		if (sz > cap) {
			free(fds);
			cap = sz * 2;
			fds = malloc(cap);
			if (!fds) return n;
		}
		sz = proc_pidinfo(pid, PROC_PIDLISTFDS, 0, fds, cap);
		if (sz <= 0) continue;
		int count = sz / PROC_PIDLISTFD_SIZE;
		for (int j = 0; j < count && n < max; j++) {
			if (fds[j].proc_fdtype != PROX_FDTYPE_SOCKET) continue;
			struct socket_fdinfo si;
			if (proc_pidfdinfo(pid, fds[j].proc_fd, PROC_PIDFDSOCKETINFO, &si, sizeof si) < (int)sizeof si) continue;
			if (si.psi.soi_kind != SOCKINFO_TCP) continue;
			struct tcp_sockinfo *t = &si.psi.soi_proto.pri_tcp;
			if (t->tcpsi_state != TSI_S_LISTEN) continue;
			sm_listen *l = &out[n++];
			l->pid = pid;
			l->port = ntohs(t->tcpsi_ini.insi_lport);
			l->v6 = (t->tcpsi_ini.insi_vflag & INI_IPV6) != 0;
			if (l->v6) memcpy(l->addr, &t->tcpsi_ini.insi_laddr.ina_6, 16);
			else memcpy(l->addr, &t->tcpsi_ini.insi_laddr.ina_46.i46a_addr4, 4);
		}
	}
	free(fds);
	return n;
}

static int sm_cwd(int pid, char *buf, int size) {
	struct proc_vnodepathinfo vpi;
	if (proc_pidinfo(pid, PROC_PIDVNODEPATHINFO, 0, &vpi, sizeof vpi) < (int)sizeof vpi) return 0;
	strlcpy(buf, vpi.pvi_cdir.vip_path, size);
	return 1;
}
*/
import "C"

import (
	"net"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// listenSockets returns the TCP listening sockets of the given processes.
func listenSockets(pids []int) []socket {
	if len(pids) == 0 {
		return nil
	}
	cpids := make([]C.int, len(pids))
	for i, p := range pids {
		cpids[i] = C.int(p)
	}
	const max = 4096
	out := make([]C.sm_listen, max)
	n := int(C.sm_listeners(&cpids[0], C.int(len(cpids)), &out[0], max))
	res := make([]socket, 0, n)
	for _, l := range out[:n] {
		b := C.GoBytes(unsafe.Pointer(&l.addr[0]), 16)
		ip := net.IP(b[:4])
		if l.v6 != 0 {
			ip = net.IP(b)
		}
		addr := ip.String()
		if ip.IsUnspecified() {
			addr = "*"
		}
		res = append(res, socket{pid: int(l.pid), port: int(l.port), addr: addr})
	}
	return res
}

// cwd returns a process's working directory.
func cwd(pid int) string {
	var buf [C.MAXPATHLEN]C.char
	if C.sm_cwd(C.int(pid), &buf[0], C.int(len(buf))) == 0 {
		return ""
	}
	return C.GoString(&buf[0])
}

// readProcTable reads every process with a single sysctl.
func readProcTable() (procTable, error) {
	t := newProcTable()
	kps, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return t, err
	}
	uid := uint32(os.Getuid())
	for i := range kps {
		kp := &kps[i]
		tv := kp.Proc.P_starttime
		c := kp.Proc.P_comm[:]
		n := 0
		for n < len(c) && c[n] != 0 {
			n++
		}
		t.add(int(kp.Proc.P_pid), int(kp.Eproc.Ppid), time.Unix(tv.Sec, int64(tv.Usec)*1000),
			string(c[:n]), kp.Eproc.Ucred.Uid == uid)
	}
	return t, nil
}

// initName is what the PID 1 parent of a detached process is called.
const initName = "launchd"

// detached reports whether a process lost its session: on macOS orphans are
// always adopted by launchd.
func detached(pid, ppid int, t procTable) bool { return ppid == 1 }
