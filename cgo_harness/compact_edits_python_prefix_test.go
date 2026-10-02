//go:build cgo && treesitter_c_parity && gts_engine_ceiling

package cgoharness

import (
	"bytes"
	"fmt"
	"testing"
)

// This is the exact first-child indentation fixture whose old compact test
// pinned the scanner-prefix fallback. Prove both edit directions against C
// before refreshing that route expectation.
func TestCompactEditsPythonPrefix(t *testing.T) {
	if ceilingLanguage(t) != "python" {
		t.Fatal("this fixture requires python")
	}
	var inputs []ceilingInput
	for _, target := range []int{20 * 1024, 137 * 1024} {
		var b bytes.Buffer
		for index := 0; b.Len() < target; index++ {
			fmt.Fprintf(&b, "def f%d(a):\n    if a:\n        return 0\n    return 0\n\n", index)
		}
		source := b.Bytes()
		at := bytes.Index(source, []byte("    return 0\n"))
		edited := append(append(append([]byte{}, source[:at]...), []byte("    ")...), source[at:]...)
		input := ceilingInput{language: "python", size: fmt.Sprintf("prefix%dk", target/1024), mode: "indent", source: [2][]byte{source, edited}}
		input.edit[0] = ceilingEdit(source, edited, at, at, at+4)
		input.edit[1] = ceilingEdit(edited, source, at, at+4, at)
		inputs = append(inputs, input)
	}
	runCompactEditsReuseInputs(t, "python", inputs)
}
