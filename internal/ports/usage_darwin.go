package ports

/*
#include <libproc.h>
#include <sys/resource.h>
#include <mach/mach_time.h>

static int sm_rusage(int pid, struct rusage_info_v6 *ri) {
	return proc_pid_rusage(pid, RUSAGE_INFO_V6, (rusage_info_t *)ri);
}
*/
import "C"

// newCounterReader reads proc_pid_rusage, which reports CPU time in mach
// ticks; the timebase converts them to nanoseconds.
func newCounterReader() counterReader {
	var tb C.mach_timebase_info_data_t
	C.mach_timebase_info(&tb)
	timebase := float64(tb.numer) / float64(tb.denom)
	return func(pid int, _ procTable) (counters, bool) {
		var ri C.struct_rusage_info_v6
		if C.sm_rusage(C.int(pid), &ri) != 0 {
			return counters{}, false
		}
		return counters{
			cpuNs:    float64(ri.ri_user_time+ri.ri_system_time) * timebase,
			energyNJ: float64(ri.ri_energy_nj),
			wakeups:  float64(ri.ri_pkg_idle_wkups + ri.ri_interrupt_wkups),
			mem:      uint64(ri.ri_phys_footprint),
		}, true
	}
}

// hasWakeups: macOS counts idle and interrupt wakeups per process.
const hasWakeups = true
