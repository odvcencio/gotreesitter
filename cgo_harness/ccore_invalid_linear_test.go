//go:build cgo && treesitter_c_parity

package cgoharness

// The step on an ill-formed byte (internal/ccore/lexdfa.go ccInvalidNext) must
// cost time in proportion to the number of such bytes. A start state of the
// python or go grammar has 700 or more ranges; a nested loop over them made
// every ill-formed byte cost about 518,000 steps.

import (
	"bytes"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/grammars"
)

// It parses 64 KiB and 1 MiB of ill-formed bytes three times each and asserts a
// timing ratio, so it runs only when CCORE_HEAVY=1 is set; CI does not run it.
func TestCcoreInvalidLinear(t *testing.T) {
	if os.Getenv("CCORE_HEAVY") == "" {
		t.Skip("heavy: set CCORE_HEAVY=1")
	}
	for _, name := range []string{"python", "go"} {
		gl := grammars.DetectLanguageByName(name).Language()
		var ms [2]float64
		var stops [2]string
		for i, n := range []int{64 << 10, 1 << 20} {
			src := bytes.Repeat([]byte{0xff}, n)
			best := time.Duration(1 << 62)
			for run := 0; run < 3; run++ {
				p := ccoreNew(gl)
				start := time.Now()
				stops[i] = p.Parse(src, nil)
				if d := time.Since(start); d < best {
					best = d
				}
			}
			ms[i] = float64(best) / float64(time.Millisecond)
		}
		ratio := ms[1] / ms[0]
		fmt.Printf("CCINVALID language=%s ms_64KiB=%.1f ms_1MiB=%.1f ratio=%.1f stop64=%q stop1M=%q\n", name, ms[0], ms[1], ratio, stops[0], stops[1])
		// 16 times the bytes: linear is 16; allow 2 times for cache and allocation effects.
		if ratio > 32 {
			t.Errorf("%s: 1 MiB takes %.1f times as long as 64 KiB, want about 16", name, ratio)
		}
	}
}
