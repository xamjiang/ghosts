package ports

import (
	"encoding/binary"
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"net/netip"
	"strconv"
	"syscall"
	"unsafe"
)

var extendedTCPTable = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetExtendedTcpTable")

func scanSockets(pids []int) ([]socket, error) {
	own := make(map[int]bool, len(pids))
	for _, pid := range pids {
		own[pid] = true
	}
	var out []socket
	for _, family := range []int{windows.AF_INET, windows.AF_INET6} {
		data, err := tcpListenerTable(family)
		if family == windows.AF_INET6 && errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
			continue
		}
		if err != nil {
			return nil, err
		}
		rows, err := parseTCPListeners(data, family, own)
		if err != nil {
			return nil, err
		}
		out = append(out, rows...)
	}
	return out, nil
}

func tcpListenerTable(family int) ([]byte, error) {
	var size uint32 = 4096
	for {
		buf := make([]byte, size)
		// TCP_TABLE_OWNER_PID_LISTENER (3) returns only listening endpoints.
		status, _, _ := extendedTCPTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(family), 3, 0)
		switch syscall.Errno(status) {
		case 0:
			return buf, nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			continue
		default:
			return nil, syscall.Errno(status)
		}
	}
}

func parseTCPListeners(buf []byte, family int, own map[int]bool) ([]socket, error) {
	// MIB_TCPROW_OWNER_PID / MIB_TCP6ROW_OWNER_PID have different field layouts.
	stride, portOffset, pidOffset := 24, 8, 20
	if family == windows.AF_INET6 {
		stride, portOffset, pidOffset = 56, 20, 52
	}
	if len(buf) < 4 || uint64(binary.LittleEndian.Uint32(buf)) > uint64((len(buf)-4)/stride) {
		return nil, fmt.Errorf("truncated TCP listener table")
	}
	count := int(binary.LittleEndian.Uint32(buf))
	var rows []socket
	for i := 0; i < count; i++ {
		r := buf[4+i*stride:]
		pid := int(binary.LittleEndian.Uint32(r[pidOffset:]))
		if !own[pid] {
			continue
		}
		var ip netip.Addr
		if family == windows.AF_INET6 {
			ip = netip.AddrFrom16([16]byte(r[:16]))
			if scope := binary.LittleEndian.Uint32(r[16:]); scope != 0 {
				ip = ip.WithZone(strconv.FormatUint(uint64(scope), 10))
			}
		} else {
			ip = netip.AddrFrom4([4]byte(r[4:8]))
		}
		addr := ip.String()
		if ip.IsUnspecified() {
			addr = "*"
		}
		rows = append(rows, socket{pid: pid, port: int(binary.BigEndian.Uint16(r[portOffset:])), addr: addr})
	}
	return rows, nil
}
