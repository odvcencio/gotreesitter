package gotreesitter_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func requireRecurringBenchmarkTree(b *testing.B, tree *gotreesitter.Tree, err error, wantError bool) {
	b.Helper()
	if err != nil {
		b.Fatalf("parse error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		b.Fatal("parse returned nil tree or root")
	}
	if got := tree.RootNode().HasError(); got != wantError {
		b.Fatalf("root.HasError() = %v, want %v", got, wantError)
	}
}

// BenchmarkKDLParseRecurringTinyClean isolates the fixed per-Parse floor on a
// reused parser. The input is intentionally much smaller than the parser's
// retained scratch caches, making recurring reset work visible.
func BenchmarkKDLParseRecurringTinyClean(b *testing.B) {
	benchmarkRecurringTinyClean(b, grammars.KdlLanguage(), []byte("node\n"))
}

// BenchmarkKDLParseRecurringTinyCleanAfterRecovery isolates the retained
// recovery-memo invalidation floor. Keep it opt-in because the priming parse is
// intentionally much larger than the timed tiny-clean workload.
func BenchmarkKDLParseRecurringTinyCleanAfterRecovery(b *testing.B) {
	if os.Getenv("GOT_BENCH_CNODE_MEMO_EPOCH") != "1" {
		b.Skip("set GOT_BENCH_CNODE_MEMO_EPOCH=1 to run the recovery-primed probe")
	}
	lang := grammars.KdlLanguage()
	parser := gotreesitter.NewParser(lang)
	primeSrc := makeKDLRecoveryGarbageSource(120, 300)
	cleanSrc := []byte("node\n")

	prime, err := parser.Parse(primeSrc)
	requireRecurringBenchmarkTree(b, prime, err, true)
	if rt := prime.ParseRuntime(); !rt.CRecoveryEnteredErrorState {
		b.Fatalf("priming parse did not enter C recovery: %s", rt.Summary())
	}
	if got, want := prime.RootNode().EndByte(), uint32(len(primeSrc)); got != want {
		b.Fatalf("priming parse truncated: root.EndByte=%d want=%d", got, want)
	}
	prime.Release()

	warm, err := parser.Parse(cleanSrc)
	requireRecurringBenchmarkTree(b, warm, err, false)
	warm.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(cleanSrc)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(cleanSrc)
		if err != nil {
			b.Fatalf("parse: %v", err)
		}
		tree.Release()
	}
}

// BenchmarkJavaParseRecurringTinyCleanDFA isolates the built-in DFA route. The
// fleet scanner uses Java's registered TokenSourceFactory instead; keep this
// variant explicit so it cannot be mistaken for the fleet witness below.
func BenchmarkJavaParseRecurringTinyCleanDFA(b *testing.B) {
	benchmarkRecurringTinyClean(b, grammars.JavaLanguage(), []byte("class A {}\n"))
}

// BenchmarkJavaParseRecurringTinyTokenSourceFactory matches perf_scan's Java
// route: construct the registered custom token source inside the timed
// operation, then parse it on a reused Parser.
func BenchmarkJavaParseRecurringTinyTokenSourceFactory(b *testing.B) {
	lang := grammars.JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("class A {}\n")

	warm := grammars.NewJavaTokenSourceOrEOF(src, lang)
	warmTree, err := parser.ParseWithTokenSource(src, warm)
	requireRecurringBenchmarkTree(b, warmTree, err, false)
	warmTree.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := grammars.NewJavaTokenSourceOrEOF(src, lang)
		tree, err := parser.ParseWithTokenSource(src, ts)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		tree.Release()
	}
}

// BenchmarkJavaTokenSourceConstructRecurring isolates the per-call factory
// floor, including immutable token-symbol binding and cached lexer-table
// attachment, without parser work.
func BenchmarkJavaTokenSourceConstructRecurring(b *testing.B) {
	lang := grammars.JavaLanguage()
	src := []byte("class A {}\n")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts, err := grammars.NewJavaTokenSource(src, lang)
		if err != nil {
			b.Fatalf("token source: %v", err)
		}
		_ = ts
	}
}

// BenchmarkCSharpParseRecurringTinyClean is a second clean fleet-tail witness
// with substantially different grammar and conflict behavior from Java.
func BenchmarkCSharpParseRecurringTinyClean(b *testing.B) {
	benchmarkRecurringTinyClean(b, grammars.CSharpLanguage(), []byte("class A {}\n"))
}

// BenchmarkCSSParseRecurringTinyClean covers another clean/parity-passing row
// from the post-refresh fleet tail at negligible additional benchmark cost.
func BenchmarkCSSParseRecurringTinyClean(b *testing.B) {
	benchmarkRecurringTinyClean(b, grammars.CssLanguage(), []byte("a{color:red}\n"))
}

func benchmarkRecurringTinyClean(b *testing.B, lang *gotreesitter.Language, src []byte) {
	b.Helper()
	parser := gotreesitter.NewParser(lang)

	warm, err := parser.Parse(src)
	requireRecurringBenchmarkTree(b, warm, err, false)
	warm.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		tree.Release()
	}
}

// BenchmarkKDLParseRecurringErrorCleanAlternate exercises an error parse that
// grows recovery scratch followed by a tiny clean parse on the same Parser.
// Each operation is one error+clean pair so reset costs cannot hide behind a
// sub-benchmark boundary or a fresh parser.
func BenchmarkKDLParseRecurringErrorCleanAlternate(b *testing.B) {
	lang := grammars.KdlLanguage()
	parser := gotreesitter.NewParser(lang)
	errorSrc := makeKDLRecoveryGarbageSource(4, 8)
	cleanSrc := []byte("node\n")

	errorTree, err := parser.Parse(errorSrc)
	requireRecurringBenchmarkTree(b, errorTree, err, true)
	errorTree.Release()
	cleanTree, err := parser.Parse(cleanSrc)
	requireRecurringBenchmarkTree(b, cleanTree, err, false)
	cleanTree.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(errorSrc) + len(cleanSrc)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		errorTree, err := parser.Parse(errorSrc)
		if err != nil {
			b.Fatalf("error parse: %v", err)
		}
		errorTree.Release()

		cleanTree, err := parser.Parse(cleanSrc)
		if err != nil {
			b.Fatalf("clean parse: %v", err)
		}
		cleanTree.Release()
	}
}

func parseKDLSExpr(t *testing.T, parser *gotreesitter.Parser, lang *gotreesitter.Language, src []byte, wantError bool) string {
	t.Helper()
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer tree.Release()
	root := tree.RootNode()
	if root == nil {
		t.Fatal("parse returned nil root")
	}
	if got := root.HasError(); got != wantError {
		t.Fatalf("root.HasError() = %v, want %v: %s", got, wantError, root.SExpr(lang))
	}
	return root.SExpr(lang)
}

func TestKDLRecoveryMemoEpochPreservesExactTreesAcrossReusedParses(t *testing.T) {
	lang := grammars.KdlLanguage()
	errorSrc := makeKDLRecoveryGarbageSource(4, 8)
	cleanSrc := []byte("node\n")
	reused := gotreesitter.NewParser(lang)

	gotError := parseKDLSExpr(t, reused, lang, errorSrc, true)
	wantError := parseKDLSExpr(t, gotreesitter.NewParser(lang), lang, errorSrc, true)
	if gotError != wantError {
		t.Fatalf("reused-parser error tree drift:\n got:  %s\n want: %s", gotError, wantError)
	}

	gotClean := parseKDLSExpr(t, reused, lang, cleanSrc, false)
	wantClean := parseKDLSExpr(t, gotreesitter.NewParser(lang), lang, cleanSrc, false)
	if gotClean != wantClean {
		t.Fatalf("post-recovery clean tree drift:\n got:  %s\n want: %s", gotClean, wantClean)
	}

	gotErrorAgain := parseKDLSExpr(t, reused, lang, errorSrc, true)
	if gotErrorAgain != wantError {
		t.Fatalf("post-clean error tree drift:\n got:  %s\n want: %s", gotErrorAgain, wantError)
	}
}

// BenchmarkJSONParseRecurringTinyTokenSourceFactory measures the recurring
// floor when callers use the public factory route instead of Parser.Parse.
func BenchmarkJSONParseRecurringTinyTokenSourceFactory(b *testing.B) {
	lang := grammars.JsonLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("{}")
	factory := func(source []byte) (gotreesitter.TokenSource, error) {
		return grammars.NewJSONTokenSource(source, lang)
	}

	warm, err := parser.ParseWithTokenSourceFactory(src, factory)
	requireRecurringBenchmarkTree(b, warm, err, false)
	warm.Release()

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.ParseWithTokenSourceFactory(src, factory)
		if err != nil {
			b.Fatalf("parse error: %v", err)
		}
		tree.Release()
	}
}
