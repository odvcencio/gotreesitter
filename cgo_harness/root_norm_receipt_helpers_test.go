//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Helpers that the dispatch-blocker receipts and next-live probes share.

// endPoint returns the point just past source, counting columns in bytes.
func endPoint(source []byte) gotreesitter.Point {
	var point gotreesitter.Point
	for _, value := range source {
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

// pointAtByteOffset returns the point at offset, counting columns in bytes.
func pointAtByteOffset(source []byte, offset int) gotreesitter.Point {
	var point gotreesitter.Point
	for index, value := range source {
		if index >= offset {
			break
		}
		if value == '\n' {
			point.Row++
			point.Column = 0
		} else {
			point.Column++
		}
	}
	return point
}

// receiptCTree parses source with the locked C parser and fails the test if
// the parser returns no root.
func receiptCTree(t *testing.T, language *sitter.Language, source []byte) *sitter.Tree {
	t.Helper()
	parser := sitter.NewParser()
	t.Cleanup(parser.Close)
	if err := parser.SetLanguage(language); err != nil {
		t.Fatal(err)
	}
	tree := parser.Parse(source, nil)
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("C parser returned no root")
	}
	return tree
}

// normalizationPassesSummary lists each normalization pass as
// name:checked/run/visited/rewritten.
func normalizationPassesSummary(tree *gotreesitter.Tree) string {
	runtime := tree.ParseRuntime()
	if runtime.NormalizationPasses == nil {
		return "none"
	}
	parts := make([]string, 0, len(*runtime.NormalizationPasses))
	for _, pass := range *runtime.NormalizationPasses {
		parts = append(parts, fmt.Sprintf("%s:%d/%d/%d/%d", pass.Name, pass.Checked, pass.Run, pass.NodesVisited, pass.NodesRewritten))
	}
	return fmt.Sprintf("%v", parts)
}

// admissionCompactMode names how one parse moved the admission counters.
func admissionCompactMode(routedBefore, fallbackBefore, routedAfter, fallbackAfter uint64) string {
	if routedAfter == routedBefore+1 && fallbackAfter == fallbackBefore {
		return "accepted"
	}
	if routedAfter == routedBefore && fallbackAfter == fallbackBefore+1 {
		return "fallback"
	}
	return fmt.Sprintf("counters=%d/%d->%d/%d", routedBefore, fallbackBefore, routedAfter, fallbackAfter)
}

// requireSameDivergence fails the test unless got and want describe the same divergence.
func requireSameDivergence(t *testing.T, route string, got, want *DumpV1Divergence) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
		}
		return
	}
	if *got != *want {
		t.Fatalf("%s divergence=%+v, want %+v", route, got, want)
	}
}
