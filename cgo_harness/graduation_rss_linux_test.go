//go:build cgo && treesitter_c_parity && linux

package cgoharness

import (
	"fmt"
	"os"
	"strings"
)

// gradProcRSS returns the peak and the current resident set size of this process in
// kB, as /proc/self/status reports them (VmHWM and VmRSS).
func gradProcRSS() (peak, cur uint64, err error) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, 0, err
	}
	found := 0
	for _, line := range strings.Split(string(b), "\n") {
		var v *uint64
		switch {
		case strings.HasPrefix(line, "VmHWM:"):
			v = &peak
		case strings.HasPrefix(line, "VmRSS:"):
			v = &cur
		default:
			continue
		}
		f := strings.Fields(line)
		if len(f) < 2 {
			return 0, 0, fmt.Errorf("unreadable line %q", line)
		}
		if _, err := fmt.Sscan(f[1], v); err != nil {
			return 0, 0, fmt.Errorf("unreadable line %q: %v", line, err)
		}
		found++
	}
	if found != 2 {
		return 0, 0, fmt.Errorf("/proc/self/status has no VmHWM and VmRSS lines")
	}
	return peak, cur, nil
}

// gradClearPeakRSS restarts the peak resident set size from the current size.
func gradClearPeakRSS() error {
	return os.WriteFile("/proc/self/clear_refs", []byte("5"), 0)
}
