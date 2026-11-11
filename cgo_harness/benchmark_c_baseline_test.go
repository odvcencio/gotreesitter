//go:build cgo && treesitter_c_bench

package cgoharness

import (
	"bytes"
	"testing"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func oracleCTreeSitterPointAtOffset(src []byte, offset int) sitter.Point {
	p := pointAtOffset(src, offset)
	return sitter.Point{Row: uint(p.Row), Column: uint(p.Column)}
}

func requireCompleteOracleCTree(tb testing.TB, tree *sitter.Tree, src []byte, phase string) *sitter.Node {
	tb.Helper()
	if tree == nil {
		tb.Fatalf("%s parse returned nil tree", phase)
		return nil
	}
	root := tree.RootNode()
	if root == nil {
		tb.Fatalf("%s parse returned nil root", phase)
		return nil
	}
	if got, want := root.EndByte(), uint(len(src)); got != want {
		tb.Fatalf("%s parse truncated: root.EndByte=%d want=%d kind=%q hasError=%v", phase, got, want, root.Kind(), root.HasError())
	}
	return root
}

func newOracleCTreeSitterParser(tb testing.TB) *sitter.Parser {
	tb.Helper()
	parser := sitter.NewParser()
	if parser == nil {
		tb.Fatal("sitter.NewParser returned nil")
		return nil
	}
	language, err := COracleLanguage("go")
	if err != nil {
		tb.Fatalf("load locked Go C oracle: %v", err)
	}
	if err := parser.SetLanguage(language); err != nil {
		tb.Fatalf("set locked Go C oracle: %v", err)
	}
	identity, err := COracleIdentity("go")
	if err != nil {
		tb.Fatalf("identify locked Go C oracle: %v", err)
	}
	tb.Logf("C oracle: contract=%s transport=%s runtime=%s@%s grammar=%s@%s grammar_artifact_sha256=%s", identity.Contract, identity.Transport, identity.RuntimeVersion, identity.RuntimeCommit, identity.GrammarRepo, identity.GrammarCommit, identity.GrammarArtifactSHA256)
	return parser
}

func parseOracleCTree(tb testing.TB, parser *sitter.Parser, oldTree *sitter.Tree, src []byte, phase string) *sitter.Tree {
	tb.Helper()
	tree := parser.Parse(src, oldTree)
	if tree == nil {
		tb.Fatalf("%s parse returned nil tree", phase)
	}
	return tree
}

// BenchmarkCTreeSitterGoParseFull benchmarks the C tree-sitter Go parser as a
// baseline for the pure-Go parser benchmarks in benchmark_test.go.
func BenchmarkCTreeSitterGoParseFull(b *testing.B) {
	parser := newOracleCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tree := parseOracleCTree(b, parser, nil, src, "c full")
		requireCompleteOracleCTree(b, tree, src, "c full")
		tree.Close()
	}
}

func BenchmarkCTreeSitterGoParseIncrementalSingleByteEdit(b *testing.B) {
	parser := newOracleCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))

	editAt := bytes.Index(src, []byte("v := 0"))
	if editAt < 0 {
		b.Fatal("could not find edit marker")
	}
	editAt += len("v := ")
	start := oracleCTreeSitterPointAtOffset(src, editAt)
	end := oracleCTreeSitterPointAtOffset(src, editAt+1)

	tree := parseOracleCTree(b, parser, nil, src, "initial")
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	defer func() {
		if tree != nil {
			tree.Close()
		}
	}()

	edit := sitter.InputEdit{
		StartByte:      uint(editAt),
		OldEndByte:     uint(editAt + 1),
		NewEndByte:     uint(editAt + 1),
		StartPosition:  start,
		OldEndPosition: end,
		NewEndPosition: end,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if src[editAt] == '0' {
			src[editAt] = '1'
		} else {
			src[editAt] = '0'
		}

		tree.Edit(&edit)
		newTree := parseOracleCTree(b, parser, tree, src, "incremental")
		if newTree == nil || newTree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		tree.Close()
		tree = newTree
	}
}

// benchmarkCTreeSitterGoLengthChangingEdit is the C twin of the pure-Go
// benchmarks that insert or delete one byte inside an identifier. The edit
// positions and texts are the same as in benchmark_test.go.
func benchmarkCTreeSitterGoLengthChangingEdit(b *testing.B, prefix string, letter byte, deleting bool) {
	parser := newOracleCTreeSitterParser(b)
	defer parser.Close()

	base := makeGoBenchmarkSource(benchmarkFuncCount(b))
	i := bytes.Index(base, []byte(prefix))
	if i < 0 {
		b.Fatalf("could not find edit site %q", prefix)
	}
	at := i + len(prefix)
	var with, without []byte
	if deleting {
		with = append([]byte(nil), base...)
		without = append(append([]byte(nil), base[:at]...), base[at+1:]...)
	} else {
		without = append([]byte(nil), base...)
		with = append(append(append([]byte(nil), base[:at]...), letter), base[at:]...)
	}
	for _, text := range [][]byte{with, without} {
		tree := parseOracleCTree(b, parser, nil, text, "check")
		if requireCompleteOracleCTree(b, tree, text, "check"); tree.RootNode().HasError() {
			b.Fatal("benchmark text does not parse cleanly")
		}
		tree.Close()
	}
	cur, other := without, with
	if deleting {
		cur, other = with, without
	}
	tree := parseOracleCTree(b, parser, nil, cur, "initial")
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	defer func() {
		if tree != nil {
			tree.Close()
		}
	}()
	startPoint := oracleCTreeSitterPointAtOffset(without, at)
	withEnd := oracleCTreeSitterPointAtOffset(with, at+1)
	insertEdit := sitter.InputEdit{
		StartByte: uint(at), OldEndByte: uint(at), NewEndByte: uint(at + 1),
		StartPosition: startPoint, OldEndPosition: startPoint, NewEndPosition: withEnd,
	}
	deleteEdit := sitter.InputEdit{
		StartByte: uint(at), OldEndByte: uint(at + 1), NewEndByte: uint(at),
		StartPosition: startPoint, OldEndPosition: withEnd, NewEndPosition: startPoint,
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(with)))
	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		edit := insertEdit
		if len(cur) > len(other) {
			edit = deleteEdit
		}
		tree.Edit(&edit)
		newTree := parseOracleCTree(b, parser, tree, other, "incremental")
		if newTree == nil || newTree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		tree.Close()
		tree = newTree
		cur, other = other, cur
	}
}

func BenchmarkCTreeSitterGoParseIncrementalTypedLetterEdit(b *testing.B) {
	benchmarkCTreeSitterGoLengthChangingEdit(b, "{ v", 'x', false)
}

func BenchmarkCTreeSitterGoParseIncrementalDeletedByteEdit(b *testing.B) {
	benchmarkCTreeSitterGoLengthChangingEdit(b, "func f", 0, true)
}

func BenchmarkCTreeSitterGoParseIncrementalNoEdit(b *testing.B) {
	parser := newOracleCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	tree := parseOracleCTree(b, parser, nil, src, "initial")
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	defer func() {
		if tree != nil {
			tree.Close()
		}
	}()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		newTree := parseOracleCTree(b, parser, tree, src, "incremental")
		if newTree == nil || newTree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		tree.Close()
		tree = newTree
	}
}

func BenchmarkCTreeSitterGoParseIncrementalRandomSingleByteEdit(b *testing.B) {
	parser := newOracleCTreeSitterParser(b)
	defer parser.Close()

	src := makeGoBenchmarkSource(benchmarkFuncCount(b))
	sites := makeGoBenchmarkEditSites(src)
	if len(sites) == 0 {
		b.Fatal("could not find random edit sites")
	}

	tree := parseOracleCTree(b, parser, nil, src, "initial")
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("initial parse returned nil root")
	}
	defer func() {
		if tree != nil {
			tree.Close()
		}
	}()

	seed := uint32(0x9e3779b9)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		seed = seed*1664525 + 1013904223
		site := sites[int(seed%uint32(len(sites)))]
		toggleDigitAt(src, site.offset)

		edit := sitter.InputEdit{
			StartByte:      uint(site.offset),
			OldEndByte:     uint(site.offset + 1),
			NewEndByte:     uint(site.offset + 1),
			StartPosition:  sitter.Point{Row: uint(site.start.Row), Column: uint(site.start.Column)},
			OldEndPosition: sitter.Point{Row: uint(site.end.Row), Column: uint(site.end.Column)},
			NewEndPosition: sitter.Point{Row: uint(site.end.Row), Column: uint(site.end.Column)},
		}

		tree.Edit(&edit)
		newTree := parseOracleCTree(b, parser, tree, src, "incremental")
		if newTree == nil || newTree.RootNode() == nil {
			b.Fatal("incremental parse returned nil root")
		}
		tree.Close()
		tree = newTree
	}
}
