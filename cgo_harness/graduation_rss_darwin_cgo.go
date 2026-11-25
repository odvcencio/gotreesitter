//go:build cgo && treesitter_c_parity && darwin

package cgoharness

/*
#include <libproc.h>
#include <sys/resource.h>
#include <unistd.h>

// libproc.h in the SDK does not declare this function, but libSystem exports it.
extern int proc_reset_footprint_interval(pid_t pid);

// gts_rusage fills the footprint of this process in bytes: the current size and
// the largest size since the last gts_reset_interval (or since the start).
static int gts_rusage(unsigned long long *cur, unsigned long long *peak) {
	struct rusage_info_v4 ri;
	if (proc_pid_rusage(getpid(), RUSAGE_INFO_V4, (rusage_info_t *)&ri) != 0) return -1;
	*cur = ri.ri_phys_footprint;
	*peak = ri.ri_interval_max_phys_footprint;
	return 0;
}

static int gts_reset_interval(void) { return proc_reset_footprint_interval(getpid()); }
*/
import "C"

import "fmt"

// gradProcRSS returns the peak and the current memory size of this process in kB.
// macOS has no resettable peak of the resident set size, so the size is the
// process footprint (ri_phys_footprint, which includes compressed pages) and the
// peak is its largest value since the last reset (ri_interval_max_phys_footprint).
// Item 9 compares two arms measured the same way; the numbers are not comparable
// with the VmHWM values of Linux.
func gradProcRSS() (peak, cur uint64, err error) {
	var c, p C.ulonglong
	if C.gts_rusage(&c, &p) != 0 {
		return 0, 0, fmt.Errorf("proc_pid_rusage failed")
	}
	if p < c {
		p = c
	}
	return uint64(p) / 1024, uint64(c) / 1024, nil
}

// gradClearPeakRSS restarts the peak from the current footprint.
func gradClearPeakRSS() error {
	if C.gts_reset_interval() != 0 {
		return fmt.Errorf("proc_reset_footprint_interval failed")
	}
	return nil
}
