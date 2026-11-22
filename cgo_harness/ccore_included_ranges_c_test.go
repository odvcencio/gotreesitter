//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	"github.com/odvcencio/gotreesitter/cgo_harness/internal/corecords"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestCcoreIncludedRangesJSONMatchesPinnedC(t *testing.T) {
	language := grammars.JsonLanguage()
	cLanguage, err := COracleLanguage("json")
	if err != nil {
		t.Fatal(err)
	}
	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	config, err := bind.Config(language)
	if err != nil {
		t.Fatal(err)
	}
	parser, err := ccore.NewParser(config)
	if err != nil {
		t.Fatal(err)
	}
	parser.SetStopAfter(512, false)
	for _, test := range []struct {
		name  string
		input string
		spans [][2]int
	}{
		{"number crosses gap", "1x2", [][2]int{{0, 1}, {2, 3}}},
		{"punctuation ends before gap", "[x1]", [][2]int{{0, 1}, {2, 4}}},
		{"trivia ends at gap", "[ x1]", [][2]int{{0, 2}, {3, 5}}},
		{"string crosses gap", "xx{\"abZZcd\":1}yy", [][2]int{{2, 6}, {8, 14}}},
		{"row jumps", "xx[1,\nexcluded\n 2]yy", [][2]int{{2, 5}, {15, 18}}},
		{"adjacent ranges", "[1]", [][2]int{{0, 1}, {1, 3}}},
		{"empty before next start", "[...1]", [][2]int{{0, 1}, {2, 2}, {4, 6}}},
		{"trailing empty range", "[1]xy", [][2]int{{0, 3}, {5, 5}}},
		{"all ranges empty", "content", [][2]int{{1, 1}, {5, 5}}},
		{"range outlives source", "[1]", [][2]int{{0, 5}}},
		{"recovery skips across gap", "[@x1]", [][2]int{{0, 2}, {3, 5}}},
		{"utf8 split at range end", "\"\u00e9\"", [][2]int{{0, 2}, {3, 4}}},
		{"invalid ascii after gap", "[x@1]", [][2]int{{0, 1}, {2, 5}}},
		{"invalid utf8 after gap", "[x\xff1]", [][2]int{{0, 1}, {2, 5}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			src := []byte(test.input)
			var ranges []ccore.Range
			var cRanges []sitter.Range
			for _, span := range test.spans {
				startRow, startCol := includedRangesPointAt(src, span[0])
				endRow, endCol := includedRangesPointAt(src, min(span[1], len(src)))
				ranges = append(ranges, ccore.Range{
					StartByte: uint32(span[0]), EndByte: uint32(span[1]),
					StartRow: uint32(startRow), StartCol: uint32(startCol),
					EndRow: uint32(endRow), EndCol: uint32(endCol),
				})
				cRanges = append(cRanges, sitter.Range{
					StartByte: uint(span[0]), EndByte: uint(span[1]),
					StartPoint: sitter.Point{Row: startRow, Column: startCol},
					EndPoint:   sitter.Point{Row: endRow, Column: endCol},
				})
			}
			if err := cp.SetIncludedRanges(cRanges); err != nil {
				t.Fatal(err)
			}
			ct := cp.Parse(src, nil)
			if ct == nil {
				t.Fatal("pinned C returned no tree")
			}
			defer ct.Close()

			tree, stop := parser.ParseIncluded(src, ranges)
			if tree == nil || stop != "" {
				t.Fatalf("ccore stopped: %q", stop)
			}
			defer tree.Release()
			assertIncludedTreeMatchesC(t, parser, tree, ct)
			assertIncludedTableProviderMatchesC(t, parser, config.Tables, src, ranges, ct)
		})
	}
}

func TestCcoreIncludedRangesJavaKeywordMatchesPinnedC(t *testing.T) {
	language := grammars.JavaLanguage()
	if len(language.KeywordLexStates) == 0 {
		t.Fatal("fixture language must use keyword promotion")
	}
	cp := sitter.NewParser()
	defer cp.Close()
	cLanguage, err := COracleLanguage("java")
	if err != nil {
		t.Fatal(err)
	}
	if err := cp.SetLanguage(cLanguage); err != nil {
		t.Fatal(err)
	}
	const text = "cl__ass Demo {}"
	const end = uint32(len(text))
	ranges := []ccore.Range{
		{EndByte: 2, EndCol: 2},
		{StartByte: 4, StartCol: 4, EndByte: end, EndCol: end},
	}
	cRanges := []sitter.Range{
		{EndByte: 2, EndPoint: sitter.Point{Column: 2}},
		{StartByte: 4, StartPoint: sitter.Point{Column: 4}, EndByte: uint(end), EndPoint: sitter.Point{Column: uint(end)}},
	}
	if err := cp.SetIncludedRanges(cRanges); err != nil {
		t.Fatal(err)
	}
	ct := cp.Parse([]byte(text), nil)
	if ct == nil {
		t.Fatal("pinned C returned no tree")
	}
	defer ct.Close()
	if root := ct.RootNode(); root.HasError() || root.NamedChild(0).Kind() != "class_declaration" {
		t.Fatal("pinned C did not recognize the keyword spanning the gap")
	}

	config, err := bind.Config(language)
	if err != nil {
		t.Fatal(err)
	}
	parser, err := ccore.NewParser(config)
	if err != nil {
		t.Fatal(err)
	}
	parser.SetStopAfter(512, false)
	tree, stop := parser.ParseIncluded([]byte(text), ranges)
	if stop != "" || tree == nil {
		t.Fatalf("ccore stopped: %q", stop)
	}
	defer tree.Release()
	assertIncludedTreeMatchesC(t, parser, tree, ct)
	assertIncludedTableProviderMatchesC(t, parser, config.Tables, []byte(text), ranges, ct)
}

func assertIncludedTableProviderMatchesC(t *testing.T, parser *ccore.Parser, tables *ccore.Tables, src []byte, ranges []ccore.Range, ct *sitter.Tree) {
	t.Helper()
	work := func(stats ccore.Stats) [5]int {
		return [5]int{stats.Tokens, stats.Shifts + stats.Reductions,
			stats.RecLeaf + stats.RecNode + stats.RecCopy, stats.MaxProcessed, stats.MultiTokens}
	}
	before := work(parser.Stats())
	provider := ccore.NewTableScan(tables, src)
	tree, stop := parser.ParseWithIncludedScan(src, nil, ranges, provider)
	if stop != "" || tree == nil {
		t.Fatalf("ccore with range provider stopped: %q", stop)
	}
	defer tree.Release()
	assertIncludedTreeMatchesC(t, parser, tree, ct)
	after := work(parser.Stats())
	if after != before {
		t.Fatalf("ranged provider work differs (tokens, steps, records, width, multi-stack tokens): before=%v after=%v", before, after)
	}
	t.Logf("tokens, steps, records, width, multi-stack tokens: before=%v after=%v; provider calls=%d", before, after, provider.Calls())
	if provider.Calls() == 0 {
		t.Fatal("the core did not ask the range provider to scan")
	}
}

func assertIncludedTreeMatchesC(t *testing.T, parser *ccore.Parser, tree *ccore.Tree, ct *sitter.Tree) {
	t.Helper()
	if want, got := ccoreCDigestChanges(ct, false), parser.Digest(tree); want != got {
		t.Fatalf("ccore digest=%s C digest=%s, C root [%d,%d) error=%t", got, want,
			ct.RootNode().StartByte(), ct.RootNode().EndByte(), ct.RootNode().HasError())
	}
	cHash, cCount, rc := corecords.Hash(ct, "")
	goHash, goCount := parser.Records(tree, nil)
	if rc != 0 || cHash != goHash || cCount != goCount {
		t.Fatalf("ccore records=%016x (%d) C records=%016x (%d), status=%d", goHash, goCount, cHash, cCount, rc)
	}
}
