//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// Separate top-level benchmarks let -shuffle randomize Go/C execution order.
// Both runtimes parse fresh trees using the revisions in COracleIdentity.
func BenchmarkIssue1336LessFreshMalformed80Go(b *testing.B) {
	benchmarkIssue1336LessFresh(b, 80, true, false)
}
func BenchmarkIssue1336LessFreshMalformed80C(b *testing.B) {
	benchmarkIssue1336LessFresh(b, 80, true, true)
}
func BenchmarkIssue1336LessFreshClean80Go(b *testing.B) {
	benchmarkIssue1336LessFresh(b, 80, false, false)
}
func BenchmarkIssue1336LessFreshClean80C(b *testing.B) {
	benchmarkIssue1336LessFresh(b, 80, false, true)
}
func BenchmarkIssue1336LessFreshClean16384Go(b *testing.B) {
	benchmarkIssue1336LessFresh(b, 16384, false, false)
}
func BenchmarkIssue1336LessFreshClean16384C(b *testing.B) {
	benchmarkIssue1336LessFresh(b, 16384, false, true)
}

func benchmarkIssue1336LessFresh(b *testing.B, rules int, malformed, useC bool) {
	source := issue1336LessSource(rules, malformed)
	b.ReportAllocs()
	b.SetBytes(int64(len(source)))
	if useC {
		lang, err := ParityCLanguage("less")
		if err != nil {
			b.Fatal(err)
		}
		parser := sitter.NewParser()
		defer parser.Close()
		if err := parser.SetLanguage(lang); err != nil {
			b.Fatal(err)
		}
		probe := parser.Parse(source, nil)
		if probe == nil {
			b.Fatal("nil C fixture tree")
		}
		if probe.RootNode().EndByte() != uint(len(source)) || int(probe.RootNode().ChildCount()) != rules || probe.RootNode().HasError() != malformed {
			b.Fatal("incomplete C fixture")
		}
		probe.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			tree := parser.Parse(source, nil)
			if tree == nil {
				b.Fatal("nil C tree")
			}
			if tree.RootNode().EndByte() != uint(len(source)) {
				b.Fatal("truncated C parse")
			}
			tree.Close()
		}
		return
	}
	parser := gts.NewParser(grammars.LessLanguage())
	parser.SetAdmissionCandidateRoute(false)
	probe, err := parser.Parse(source)
	if err != nil {
		b.Fatal(err)
	}
	// The baseline malformed tree is deliberately wrong; clean fixtures must
	// contain every rule so a budget-produced ERROR cannot count as a parse.
	if probe.ParseStopReason() != gts.ParseStopAccepted || probe.RootNode().EndByte() != uint32(len(source)) || (!malformed && probe.RootNode().ChildCount() != rules) || probe.RootNode().HasError() != malformed {
		b.Fatalf("incomplete Go fixture: children=%d error=%t stop=%s", probe.RootNode().ChildCount(), probe.RootNode().HasError(), probe.ParseStopReason())
	}
	probe.Release()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(source)
		if err != nil {
			b.Fatal(err)
		}
		if tree.RootNode().EndByte() != uint32(len(source)) {
			b.Fatalf("truncated Go parse: %s", tree.ParseStopReason())
		}
		tree.Release()
	}
}
