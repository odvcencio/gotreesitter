//go:build !grammar_subset

package grammarruntime

import (
	"testing"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// TestCoreModeReachesScannerBehindAnyNumberOfOrderAdapters: the core turns the
// C-faithful mode on for the scanner's payload however many order adapters wrap
// it. Python scanned through zero, one and two identity adapters, input "\n    x",
// only INDENT valid, empty previous state: C saves {0,0,4,0} (string flag,
// delimiter count, the indent 4 as one 16-bit word; scanner.c:364-390); the
// legacy layout adds an indent-count word and gives {0,0,1,0,4,0}.
func TestCoreModeReachesScannerBehindAnyNumberOfOrderAdapters(t *testing.T) {
	for layers := 0; layers <= 3; layers++ {
		sc, lang := wrappedCoreScanner(t, "python", layers)
		valid := make([]bool, len(lang.ExternalSymbols))
		valid[externalIndex(t, lang, "_indent")] = true
		var out ccore.ExtScan
		buf := make([]byte, 1024)
		if !sc.Scan([]byte("\n    x"), 0, 0, 0, nil, valid, buf, &out) {
			t.Fatalf("%d adapter layers: scan returned false", layers)
		}
		got := buf[:out.StateLen]
		want := []byte{0, 0, 4, 0}
		if string(got) != string(want) {
			t.Errorf("%d adapter layers: state %v, want %v", layers, got, want)
		}
	}
}
