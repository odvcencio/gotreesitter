//go:build cgo && treesitter_c_parity && gts_ccore_test_certify

package cgoharness

import (
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreIncludedRangesPublicRoutes(t *testing.T) {
	testCcoreIncludedRangesPublicRoutes(t, "json", grammars.JsonLanguage(), []byte("xx[1, 2, 3]yy"),
		[][2]int{{2, 11}}, [][2]int{{2, 7}, {10, 11}}, nil)
}

func TestCcoreExternalScannerIncludedRangesPublicRoutes(t *testing.T) {
	for _, grammar := range []struct {
		name   string
		load   func() *g.Language
		chunks []string
	}{
		{"javascript", grammars.JavascriptLanguage, []string{"const t = \x60ab", "cd\x60; let n = 1;"}},
		{"python", grammars.PythonLanguage, []string{"x = \x27\x27\x27ab", "cd\x27\x27\x27\ny = 2\n"}},
	} {
		t.Run(grammar.name, func(t *testing.T) {
			source, spans := selectedScannerChunks("OMIT", grammar.chunks)
			testCcoreIncludedRangesPublicRoutes(t, grammar.name, grammar.load(), source,
				[][2]int{{0, len(source)}}, spans, nil)
		})
	}
}

func TestCcoreTableProviderIncludedRangesPublicRoutes(t *testing.T) {
	for _, test := range []struct {
		language string
		chunks   []string
	}{
		{"c", []string{"int kept = 1;\nint a", "bc = 2;\nint last = 3;\n"}},
		{"cpp", []string{"auto s = R\"tag(ab", "cd)tag\"; int tail = 2;\n"}},
		{"java", []string{"class Demo { int kept = 1; int a", "bc = 2; int tail = 3; }"}},
	} {
		t.Run(test.language, func(t *testing.T) {
			entry := grammars.DetectLanguageByName(test.language)
			if entry == nil || entry.TokenSourceFactory == nil {
				t.Fatal("missing registered token source factory")
			}
			lang := entry.Language()
			makeSource := func(src []byte) (g.TokenSource, error) {
				return entry.TokenSourceFactory(src, lang), nil
			}
			source, after := selectedScannerChunks("OMIT", test.chunks)
			before := append([][2]int(nil), after...)
			before[0][1]++ // include just the first omitted byte to keep both parses valid
			testCcoreIncludedRangesPublicRoutes(t, test.language, lang, source, before, after, makeSource)
		})
	}
}

func testCcoreIncludedRangesPublicRoutes(t *testing.T, name string, lang *g.Language, src []byte, before, after [][2]int, factory g.TokenSourceFactory) {
	t.Helper()
	goRanges := func(spans [][2]int) []g.Range {
		core, _ := coreAndCIncludedRanges(src, spans)
		ranges := make([]g.Range, len(core))
		for i, r := range core {
			ranges[i] = g.Range{StartByte: r.StartByte, EndByte: r.EndByte,
				StartPoint: g.Point{Row: r.StartRow, Column: r.StartCol}, EndPoint: g.Point{Row: r.EndRow, Column: r.EndCol}}
		}
		return ranges
	}

	parser := g.NewParser(lang)
	parser.SetAdmissionCandidateRoute(true)
	parse := func() (*g.Tree, error) {
		if factory != nil {
			return parser.ParseWithTokenSourceFactory(src, factory)
		}
		return parser.Parse(src)
	}
	reparse := func(old *g.Tree) (*g.Tree, error) {
		if factory != nil {
			return parser.ParseIncrementalWithTokenSourceFactory(src, old, factory)
		}
		return parser.ParseIncremental(src, old)
	}

	parser.SetIncludedRanges(goRanges(before))
	_, fresh0, incr0 := bind.RouteCounters()
	ungranted, err := parse()
	if err != nil || ungranted == nil {
		t.Fatalf("ungranted parse: %v", err)
	}
	ungranted.Release()
	_, fresh1, incr1 := bind.RouteCounters()
	if fresh1 != fresh0 || incr1 != incr0 {
		t.Fatal("the core served ranges without a range certificate")
	}

	modes := uint8(certified.Fresh | certified.Recover | certified.Incr | certified.Ranges)
	if lang.ExternalScanner != nil {
		modes |= certified.Scan
	}
	var providers []string
	if factory != nil {
		providers = []string{ccore.TableScanID}
	}
	checked := []string{"ImmediateTokens", "ZeroWidthTokens", "LayoutFallbackLexState", "AfterWhitespaceLexState"}
	restore, err := bindtest.CertifyForTestsProviders(modes, checked, nil, providers)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()
	cp := sitter.NewParser()
	defer cp.Close()
	cl, err := COracleLanguage(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := cp.SetLanguage(cl); err != nil {
		t.Fatal(err)
	}
	setC := func(spans [][2]int) {
		t.Helper()
		_, ranges := coreAndCIncludedRanges(src, spans)
		if err := cp.SetIncludedRanges(ranges); err != nil {
			t.Fatal(err)
		}
	}
	setC(before)
	cOld := cp.Parse(src, nil)
	if cOld == nil {
		t.Fatal("pinned C returned no initial tree")
	}
	defer cOld.Close()
	old, err := parse()
	if err != nil || old == nil {
		t.Fatalf("certified fresh parse: %v", err)
	}
	defer old.Release()
	assertLockedCTreeExact(t, "ranged fresh", old, lang, cOld)
	expectedEOF := uint32(after[len(after)-1][1])
	if rt := old.ParseRuntime(); rt.ExpectedEOFByte != expectedEOF || rt.RootEndByte != expectedEOF {
		t.Fatalf("fresh range extent: %+v", rt)
	}

	setC(after)
	cNext := cp.Parse(src, cOld)
	if cNext == nil {
		t.Fatal("pinned C returned no incremental tree")
	}
	defer cNext.Close()
	parser.SetIncludedRanges(goRanges(after))
	next, err := reparse(old)
	if err != nil || next == nil {
		t.Fatalf("certified incremental parse: %v", err)
	}
	defer next.Release()
	assertLockedCTreeExact(t, "range-only reparse", next, lang, cNext)
	_, fresh2, incr2 := bind.RouteCounters()
	if fresh2-fresh1 != 1 || incr2-incr1 != 1 {
		t.Fatalf("ccore served fresh=%d incremental=%d, want 1 each", fresh2-fresh1, incr2-incr1)
	}
	if rt := next.ParseRuntime(); !rt.IncrementalOldTreeReuseRoute || rt.ExpectedEOFByte != expectedEOF {
		t.Fatalf("incremental range attribution: %+v", rt)
	}
}
