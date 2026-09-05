package gotreesitter_test

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/benchfixtures"
)

// The release disables the unsafe whole-root shortcut, not ordinary reuse.
func requireReleaseSameWidthReparse(t *testing.T, profile gotreesitter.IncrementalParseProfile) {
	t.Helper()
	if profile.ReparseNanos <= 0 {
		t.Fatalf("real edit bypassed reparsing: %+v", profile)
	}
	if profile.ReuseUnsupported && (profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0) {
		t.Fatalf("unsupported reuse reported borrowed work: %+v", profile)
	}
}

func TestExternalScannerIncrementalReusePolicy(t *testing.T) {
	cases := []struct {
		name           string
		lang           func() *gotreesitter.Language
		source         func(int) []byte
		marker         string
		wantReuse      bool
		wantReason     string
		wantSubtreeMin uint64
	}{
		{
			name:           "typescript",
			lang:           grammars.TypescriptLanguage,
			source:         makeTypeScriptBenchmarkSource,
			marker:         "const v = ",
			wantReuse:      true,
			wantSubtreeMin: 1,
		},
		{
			name:       "python",
			lang:       grammars.PythonLanguage,
			source:     makePythonBenchmarkSource,
			marker:     "v = ",
			wantReason: "external_scanner_prefix_frontier_unproven",
		},
		{
			name:       "svelte",
			lang:       grammars.SvelteLanguage,
			source:     makeSvelteBenchmarkSource,
			marker:     "let v0 = ",
			wantReuse:  false,
			wantReason: "external_scanner_unsupported",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			src := tc.source(128)
			sites := makeBenchmarkEditSites(src, tc.marker)
			if len(sites) == 0 {
				t.Fatalf("missing edit sites for marker %q", tc.marker)
			}
			site := sites[0]

			oldTree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse failed: %v", err)
			}
			requireCompleteParse(t, oldTree, src, lang, "initial")
			if oldTree.RootNode().HasError() {
				t.Fatal("initial parse produced error root")
			}

			next := append([]byte(nil), src...)
			toggleDigitAt(next, site.offset)
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(site.offset),
				OldEndByte:  uint32(site.offset + 1),
				NewEndByte:  uint32(site.offset + 1),
				StartPoint:  site.start,
				OldEndPoint: site.end,
				NewEndPoint: site.end,
			})

			newTree, prof, err := parser.ParseIncrementalProfiled(next, oldTree)
			if err != nil {
				t.Fatalf("incremental parse failed: %v", err)
			}
			requireCompleteParse(t, newTree, next, lang, "incremental")
			if newTree.RootNode().HasError() {
				t.Fatal("incremental parse produced error root")
			}
			defer oldTree.Release()
			defer newTree.Release()
			fresh, err := parser.Parse(next)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, newTree, fresh, lang)
			requireReleaseSameWidthReparse(t, prof)
			if tc.wantReuse {
				if prof.ReuseUnsupported {
					t.Fatalf("ReuseUnsupported = true, want false (reason=%q)", prof.ReuseUnsupportedReason)
				}
				if prof.ReusedSubtrees < tc.wantSubtreeMin {
					t.Fatalf("ReusedSubtrees = %d, want >= %d", prof.ReusedSubtrees, tc.wantSubtreeMin)
				}
				return
			}
			if !prof.ReuseUnsupported {
				t.Fatal("ReuseUnsupported = false, want true")
			}
			if prof.ReuseUnsupportedReason != tc.wantReason {
				t.Fatalf("ReuseUnsupportedReason = %q, want %q", prof.ReuseUnsupportedReason, tc.wantReason)
			}
		})
	}
}

type pythonDerivedIncrementalCase struct {
	name   string
	lang   func() *gotreesitter.Language
	source []byte
	marker byte
}

func pythonDerivedIncrementalCases() []pythonDerivedIncrementalCase {
	return []pythonDerivedIncrementalCase{
		{name: "python", lang: grammars.PythonLanguage, source: []byte("value = 1\n"), marker: '1'},
		{name: "mojo", lang: grammars.MojoLanguage, source: []byte("fn main():\n    print(1)\n"), marker: '1'},
		{name: "starlark", lang: grammars.StarlarkLanguage, source: []byte("value = 1\n"), marker: '1'},
	}
}

func requireIncrementalDeepTreeMatchesFresh(t *testing.T, incremental, fresh *gotreesitter.Tree, lang *gotreesitter.Language) {
	t.Helper()
	incrementalInspection, err := benchfixtures.InspectGoTree(incremental.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	freshInspection, err := benchfixtures.InspectGoTree(fresh.RootNode(), lang)
	if err != nil {
		t.Fatal(err)
	}
	if incrementalInspection.SHA256 != freshInspection.SHA256 {
		t.Fatalf("incremental/fresh deep tree mismatch: incremental=%s fresh=%s", incrementalInspection.SHA256, freshInspection.SHA256)
	}
}

func TestUncertifiedExternalScannerLengthChangingEditFallsBack(t *testing.T) {
	cases := []struct {
		name        string
		lang        func() *gotreesitter.Language
		source      []byte
		oldText     []byte
		replacement []byte
	}{
		{
			name:        "svelte",
			lang:        grammars.SvelteLanguage,
			source:      []byte("<main><p>hello</p></main>\n"),
			oldText:     []byte("hello"),
			replacement: []byte("hello world"),
		},
		{
			name:        "markdown_inline",
			lang:        grammars.MarkdownInlineLanguage,
			source:      []byte("Hello *world*.\n"),
			oldText:     []byte("world"),
			replacement: []byte("wide world"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)

			oldTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			requireCompleteParse(t, oldTree, tc.source, lang, "old")

			next, edit := replaceExternalScannerWitness(t, tc.source, tc.oldText, tc.replacement)
			oldTree.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(next, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer incremental.Release()
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" {
				t.Fatalf("uncertified scanner did not fail closed: %+v", profile)
			}
			if profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("uncertified scanner fallback reported old-tree reuse: %+v", profile)
			}
			if profile.TokensConsumed == 0 || profile.NewNodesAllocated == 0 {
				t.Fatalf("uncertified scanner fallback did not execute a full parse: %+v", profile)
			}
			requireCompleteParse(t, incremental, next, lang, "incremental fallback")

			freshParser := gotreesitter.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(false)
			fresh, err := freshParser.Parse(next)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			requireCompleteParse(t, fresh, next, lang, "fresh")
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}

func replaceExternalScannerWitness(t *testing.T, source, oldText, replacement []byte) ([]byte, gotreesitter.InputEdit) {
	t.Helper()
	if len(oldText) == len(replacement) {
		t.Fatalf("fallback witness must change length: old=%q replacement=%q", oldText, replacement)
	}
	offset := bytes.Index(source, oldText)
	if offset < 0 {
		t.Fatalf("fixture missing edit text %q", oldText)
	}
	oldEnd := offset + len(oldText)
	next := make([]byte, 0, len(source)-len(oldText)+len(replacement))
	next = append(next, source[:offset]...)
	next = append(next, replacement...)
	next = append(next, source[oldEnd:]...)
	return next, gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(oldEnd),
		NewEndByte:  uint32(offset + len(replacement)),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, oldEnd),
		NewEndPoint: pointForOffset(next, offset+len(replacement)),
	}
}

func TestExternalScannerDerivedSameLengthTokenChangeInvalidatesScannerReuse(t *testing.T) {
	for _, languageCase := range pythonDerivedIncrementalCases() {
		languageCase := languageCase
		t.Run(languageCase.name, func(t *testing.T) {
			for _, route := range []struct {
				name           string
				includedRanges bool
			}{
				{name: "direct"},
				{name: "included_range", includedRanges: true},
			} {
				route := route
				t.Run(route.name, func(t *testing.T) {
					source := languageCase.source
					next := append([]byte(nil), source...)
					offset := bytes.IndexByte(source, languageCase.marker)
					if offset < 0 {
						t.Fatalf("locked %s token-change witness is malformed", languageCase.name)
					}
					next[offset] = 'x'

					lang := languageCase.lang()
					parser := gotreesitter.NewParser(lang)
					parser.SetAdmissionCandidateRoute(false)
					if route.includedRanges {
						parser.SetIncludedRanges([]gotreesitter.Range{{
							StartByte: 0,
							EndByte:   uint32(len(source)),
							EndPoint:  pointForOffset(source, len(source)),
						}})
					}
					oldTree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer oldTree.Release()
					oldTree.Edit(gotreesitter.InputEdit{
						StartByte:   uint32(offset),
						OldEndByte:  uint32(offset + 1),
						NewEndByte:  uint32(offset + 1),
						StartPoint:  pointForOffset(source, offset),
						OldEndPoint: pointForOffset(source, offset+1),
						NewEndPoint: pointForOffset(next, offset+1),
					})

					incremental, profile, err := parser.ParseIncrementalProfiled(next, oldTree)
					if err != nil {
						t.Fatal(err)
					}
					defer incremental.Release()
					if languageCase.name == "python" {
						if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" {
							t.Fatalf("same-length Python token-class change bypassed prefix fallback: %+v", profile)
						}
						if profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
							t.Fatalf("same-length Python token-class change reused old syntax: %+v", profile)
						}
					} else {
						if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_unsupported" {
							t.Fatalf("same-length token change did not preserve %s scanner refusal: %+v", languageCase.name, profile)
						}
						if profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
							t.Fatalf("same-length token change reused old %s syntax: %+v", languageCase.name, profile)
						}
					}
					if profile.TokensConsumed == 0 || profile.NewNodesAllocated == 0 {
						t.Fatalf("same-length token change did not execute a full parse: %+v", profile)
					}

					fresh, err := parser.Parse(next)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
				})
			}
		})
	}
}

// TestPythonSameSymbolScannerStateEditFallsBack proves that token identity and
// width do not authenticate an external-scanner transition. Python scans both
// prefixes as string_start, but only the f prefix enables interpolation.
func TestPythonSameSymbolScannerStateEditFallsBack(t *testing.T) {
	source := []byte("def first(value):\n    return u\"{x}\"\n\ndef second(value):\n    return \"unchanged\"\n")
	marker := []byte("u\"{x}\"")
	offset := bytes.Index(source, marker)
	if offset < 0 {
		t.Fatal("locked Python scanner-state witness is malformed")
	}
	edited := append([]byte(nil), source...)
	edited[offset] = 'f'
	edit := gotreesitter.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(source, offset),
		OldEndPoint: pointForOffset(source, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	for _, route := range []struct {
		name           string
		includedRanges bool
	}{
		{name: "direct"},
		{name: "included_range", includedRanges: true},
	} {
		route := route
		t.Run(route.name, func(t *testing.T) {
			lang := grammars.PythonLanguage()
			parser := gotreesitter.NewParser(lang)
			parser.SetAdmissionCandidateRoute(false)
			if route.includedRanges {
				parser.SetIncludedRanges([]gotreesitter.Range{{
					StartByte: 0,
					EndByte:   uint32(len(source)),
					EndPoint:  pointForOffset(source, len(source)),
				}})
			}
			oldTree, err := parser.Parse(source)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			oldTree.Edit(edit)
			incremental, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			if incremental != oldTree {
				defer incremental.Release()
			}
			if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" ||
				profile.OldTreeReuseRoute || profile.ReusedSubtrees != 0 || profile.ReusedBytes != 0 {
				t.Fatalf("same-symbol scanner-state edit bypassed prefix fallback: %+v", profile)
			}
			if profile.TokensConsumed == 0 || profile.NewNodesAllocated == 0 {
				t.Fatalf("same-symbol scanner-state edit did not execute a fresh parse: %+v", profile)
			}

			freshParser := gotreesitter.NewParser(lang)
			freshParser.SetAdmissionCandidateRoute(false)
			if route.includedRanges {
				freshParser.SetIncludedRanges([]gotreesitter.Range{{
					StartByte: 0,
					EndByte:   uint32(len(edited)),
					EndPoint:  pointForOffset(edited, len(edited)),
				}})
			}
			fresh, err := freshParser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
		})
	}
}

func TestPythonDerivedTokenInvariantLeafReusePrecedesScannerFallback(t *testing.T) {
	for _, languageCase := range pythonDerivedIncrementalCases() {
		languageCase := languageCase
		t.Run(languageCase.name, func(t *testing.T) {
			for _, route := range []struct {
				name           string
				includedRanges bool
			}{
				{name: "direct"},
				{name: "included_range", includedRanges: true},
			} {
				route := route
				t.Run(route.name, func(t *testing.T) {
					source := languageCase.source
					next := append([]byte(nil), source...)
					offset := bytes.IndexByte(source, languageCase.marker)
					if offset < 0 {
						t.Fatalf("locked %s token-invariant witness is malformed", languageCase.name)
					}
					next[offset] = '2'

					lang := languageCase.lang()
					parser := gotreesitter.NewParser(lang)
					// The release reparses edits with uncertified stateful scanners.
					parser.SetAdmissionCandidateRoute(false)
					if route.includedRanges {
						parser.SetIncludedRanges([]gotreesitter.Range{{
							StartByte: 0,
							EndByte:   uint32(len(source)),
							EndPoint:  pointForOffset(source, len(source)),
						}})
					}
					oldTree, err := parser.Parse(source)
					if err != nil {
						t.Fatal(err)
					}
					defer oldTree.Release()
					oldTree.Edit(gotreesitter.InputEdit{
						StartByte:   uint32(offset),
						OldEndByte:  uint32(offset + 1),
						NewEndByte:  uint32(offset + 1),
						StartPoint:  pointForOffset(source, offset),
						OldEndPoint: pointForOffset(source, offset+1),
						NewEndPoint: pointForOffset(next, offset+1),
					})

					incremental, profile, err := parser.ParseIncrementalProfiled(next, oldTree)
					if err != nil {
						t.Fatal(err)
					}
					defer incremental.Release()
					requireReleaseSameWidthReparse(t, profile)
					if !profile.ReuseUnsupported || profile.ReuseUnsupportedReason != "external_scanner_prefix_frontier_unproven" {
						t.Fatalf("expected scanner fallback: %+v", profile)
					}

					fresh, err := parser.Parse(next)
					if err != nil {
						t.Fatal(err)
					}
					defer fresh.Release()
					requireIncrementalDeepTreeMatchesFresh(t, incremental, fresh, lang)
				})
			}
		})
	}
}

func TestExternalScannerTokenInvariantLeafReuse(t *testing.T) {
	cases := []struct {
		name           string
		lang           func() *gotreesitter.Language
		explicitForest bool
		source         []byte
		marker         []byte
		replacement    byte
	}{
		{
			name:        "elixir identifier",
			lang:        grammars.ElixirLanguage,
			source:      []byte("value = 1\nIO.inspect(value)\n"),
			marker:      []byte("value"),
			replacement: 'w',
		},
		{
			name:           "bash comment",
			lang:           grammars.BashLanguage,
			explicitForest: true,
			source:         []byte("# look for old 0.x cruft\n"),
			marker:         []byte("0"),
			replacement:    '1',
		},
		{
			name:           "bash number",
			lang:           grammars.BashLanguage,
			explicitForest: true,
			source:         []byte("echo -9\n"),
			marker:         []byte("9"),
			replacement:    '0',
		},
		{
			name:        "julia line comment",
			lang:        grammars.JuliaLanguage,
			source:      []byte("# This file is part of Julia.\nx = 1\n"),
			marker:      []byte("This"),
			replacement: 'U',
		},
		{
			name:        "sql identifier",
			lang:        grammars.SqlLanguage,
			source:      []byte("select * from table1\n"),
			marker:      []byte("1"),
			replacement: '2',
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			offset := bytes.Index(tc.source, tc.marker)
			if offset < 0 {
				t.Fatalf("fixture missing marker %q", tc.marker)
			}
			next := append([]byte(nil), tc.source...)
			next[offset] = tc.replacement

			lang := tc.lang()
			if tc.explicitForest {
				lang = explicitForestLanguage(t, lang)
			}
			parser := gotreesitter.NewParser(lang)
			// Preserve the original legacy and explicit forest input routes.
			parser.SetAdmissionCandidateRoute(false)
			fresh, err := parser.Parse(next)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer fresh.Release()
			requireCompleteParse(t, fresh, next, lang, "fresh")
			if tc.explicitForest {
				requireAcceptedForestRuntime(t, "fresh "+tc.name+" parse", fresh)
			}
			if fresh.RootNode().HasError() {
				t.Fatalf("fresh parse has error: %s", fresh.RootNode().SExpr(lang))
			}

			oldTree, err := parser.Parse(tc.source)
			if err != nil {
				t.Fatalf("old parse: %v", err)
			}
			defer oldTree.Release()
			requireCompleteParse(t, oldTree, tc.source, lang, "old")
			if tc.explicitForest {
				requireAcceptedForestRuntime(t, "old "+tc.name+" parse", oldTree)
			}
			oldTree.Edit(gotreesitter.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(tc.source, offset),
				OldEndPoint: pointForOffset(tc.source, offset+1),
				NewEndPoint: pointForOffset(next, offset+1),
			})

			newTree, profile, err := parser.ParseIncrementalProfiled(next, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer newTree.Release()
			requireCompleteParse(t, newTree, next, lang, "incremental")
			requireReleaseSameWidthReparse(t, profile)
			requireIncrementalDeepTreeMatchesFresh(t, newTree, fresh, lang)
			if got, want := newTree.RootNode().SExpr(lang), fresh.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func makeSvelteBenchmarkSource(funcCount int) []byte {
	var buf bytes.Buffer
	buf.WriteString("<script>\n")
	for i := 0; i < funcCount; i++ {
		buf.WriteString("  let v")
		buf.WriteString(stringInt(i))
		buf.WriteString(" = ")
		buf.WriteString(stringInt(i))
		buf.WriteString(";\n")
	}
	buf.WriteString("</script>\n\n")
	for i := 0; i < funcCount; i++ {
		buf.WriteString("{#if v")
		buf.WriteString(stringInt(i))
		buf.WriteString(" > 0}\n")
		buf.WriteString("  <section class=\"item\"><button>{v")
		buf.WriteString(stringInt(i))
		buf.WriteString("}</button></section>\n")
		buf.WriteString("{/if}\n")
	}
	return buf.Bytes()
}
