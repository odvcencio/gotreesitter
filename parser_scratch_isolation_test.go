package gotreesitter_test

import (
	"bytes"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestParseScratchIsolationSmallLargeSmall is the scratch lifetime isolation
// gate from the parser cost envelope. A tiny parse that follows a large parse
// in the same process must not inherit the large parse's transient scratch.
// The pool bills scratch capacity to the operation that holds it, so an
// inherited slab taxes every small operation that follows a large one.
func TestParseScratchIsolationSmallLargeSmall(t *testing.T) {
	tiny := []byte(`{"a": [1, 2, 3], "b": {"c": true, "d": null}}`)
	var large bytes.Buffer
	large.WriteString("package p\n\n")
	for large.Len() < 320<<10 {
		large.WriteString("func f(a, b int) int {\n\tif a > b {\n\t\treturn a - b\n\t}\n\treturn b - a\n}\n\n")
	}
	parse := func(lang *gts.Language, source []byte) int64 {
		t.Helper()
		parser := gts.NewParser(lang)
		parser.SetAdmissionCandidateRoute(false)
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return tree.ParseRuntime().TransientScratchBytesAllocated
	}
	// A tiny source may hold one default parent slab and one default child
	// slab. The ceiling sits above that floor and far below the slabs a
	// 320 KiB parse leaves behind.
	const ceiling = int64(8 << 20)
	first := parse(grammars.JsonLanguage(), tiny)
	largeScratch := parse(grammars.GoLanguage(), large.Bytes())
	second := parse(grammars.JsonLanguage(), tiny)
	if largeScratch <= ceiling {
		t.Skipf("large parse held only %d transient scratch bytes; the sequence cannot witness inheritance", largeScratch)
	}
	if second > ceiling {
		t.Fatalf("tiny parse after a large parse inherited transient scratch: first=%d large=%d second=%d ceiling=%d",
			first, largeScratch, second, ceiling)
	}
}
