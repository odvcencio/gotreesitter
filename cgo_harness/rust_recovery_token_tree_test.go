//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func TestRustRecoveryPreservesTokenTreeErrors(t *testing.T) {
	// The locked grammar rejects metavariable expressions inside these token
	// trees. Compatibility must preserve its recovery, including error spans.
	for _, tc := range []struct {
		name   string
		source string
	}{
		{"count", "macro_rules! a { () => { (${count($foo, 0)},) }; }"},
		{"ignore_and_index", "macro_rules! a { () => { (${ignore($foo)} ${index(10)},) }; }"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.source)
			runParityCase(t, parityCase{name: "rust", source: tc.source}, "fresh", source)
			parser := gotreesitter.NewParser(grammars.RustLanguage())
			tree, err := parser.Parse(source)
			if err != nil {
				t.Fatal(err)
			}
			defer tree.Release()
			if allocations := testing.AllocsPerRun(5, func() {
				next, err := parser.ParseIncremental(source, tree)
				if err != nil {
					panic(err)
				}
				next.Release()
			}); allocations != 0 {
				t.Fatalf("no-edit recovery reparse allocated %g times", allocations)
			}
		})
	}
}
