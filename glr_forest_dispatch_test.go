package gotreesitter_test

import (
	"fmt"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

func explicitForestLanguage(t *testing.T, lang *gts.Language) *gts.Language {
	t.Helper()
	previous := lang.WantsForest
	lang.WantsForest = true
	t.Cleanup(func() { lang.WantsForest = previous })
	return lang
}

func requireAcceptedForestRuntime(t *testing.T, label string, tree *gts.Tree) {
	t.Helper()
	runtime := tree.ParseRuntime()
	if runtime.StopReason != gts.ParseStopAccepted || !runtime.ForestFastPath ||
		!runtime.LastTokenWasEOF || runtime.TokensConsumed != 0 {
		t.Fatalf("%s did not use accepted forest runtime: %s", label, runtime.Summary())
	}
}

func TestCertifiedAutomaticForestRoutingRequiresExactArtifact(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	tests := []struct {
		name string
		load func() *gts.Language
		src  string
	}{
		{
			name: "awk",
			load: grm.AwkLanguage,
			src: `BEGIN {
	str="\342\200\257"
	print length(str)
	match(str,/^/)
	print RSTART, RLENGTH
}
`,
		},
		{
			name: "kdl",
			load: grm.KdlLanguage,
			src: `package {
    name kdl
    version "0.0.0"
    description "The kdl document language"
}
`,
		},
		{name: "uxntal", load: grm.UxntalLanguage, src: "( comment )\n|0100 BRK\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builtin := tt.load()
			if !builtin.AutomaticForestEnabledByDefault {
				t.Fatal("built-in artifact is missing its automatic forest profile")
			}
			parser := gts.NewParser(builtin)
			tree, err := parser.Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("built-in parse: %v", err)
			}
			defer tree.Release()
			_, _, decline, _ := parser.ForestDeclineInfo()
			if !tree.ParseRuntime().ForestFastPath && decline == "" {
				t.Fatal("built-in artifact did not attempt the automatic forest route")
			}

			custom, err := gts.LoadLanguage(grm.BlobByName(tt.name))
			if err != nil {
				t.Fatalf("load caller-owned same-name grammar: %v", err)
			}
			grm.AttachLanguageSupport(tt.name, custom)
			if custom.AutomaticForestEnabledByDefault {
				t.Fatal("caller-owned same-name grammar inherited built-in certification")
			}
			customParser := gts.NewParser(custom)
			customTree, err := customParser.Parse([]byte(tt.src))
			if err != nil {
				t.Fatalf("caller-owned same-name parse: %v", err)
			}
			defer customTree.Release()
			if rt := customTree.ParseRuntime(); rt.ForestFastPath {
				t.Fatalf("caller-owned same-name grammar used forest: %s", rt.Summary())
			}
			if offset, sym, reason, states := customParser.ForestDeclineInfo(); reason != "" {
				t.Fatalf("caller-owned same-name grammar attempted forest: offset=%d symbol=%d reason=%q states=%v", offset, sym, reason, states)
			}
		})
	}
}

func TestCertifiedKDLForestDeclineFallsBackExactly(t *testing.T) {
	lang := grm.KdlLanguage()
	src := []byte(`package {
    name kdl
    version "0.0.0"
    description "The kdl document language"
    authors "Kat Marchan"
    license-file LICENSE.md
    edition "2018"
}

dependencies {
    nom "6.0.1"
    thiserror "1.0.22"
}
`)

	gts.SetGLRForestEnabled(false)
	production, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer production.Release()

	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)
	parser := gts.NewParser(lang)
	routed, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("automatic parse: %v", err)
	}
	defer routed.Release()
	if rt := routed.ParseRuntime(); rt.ForestFastPath {
		t.Fatalf("declined witness unexpectedly returned a forest tree: %s", rt.Summary())
	}
	if offset, sym, reason, states := parser.ForestDeclineInfo(); reason == "" {
		t.Fatalf("declined witness did not record a forest attempt: offset=%d symbol=%d states=%v", offset, sym, states)
	}
	requireForestFallbackNodeIdentity(t, "root", routed.RootNode(), production.RootNode(), lang)
}

func requireForestFallbackNodeIdentity(t *testing.T, path string, got, want *gts.Node, lang *gts.Language) {
	t.Helper()
	if got == nil || want == nil {
		if got != want {
			t.Fatalf("%s presence got=%t want=%t", path, got != nil, want != nil)
		}
		return
	}
	if got.Type(lang) != want.Type(lang) || got.Symbol() != want.Symbol() {
		t.Fatalf("%s identity got=%q/%d want=%q/%d", path, got.Type(lang), got.Symbol(), want.Type(lang), want.Symbol())
	}
	if got.StartByte() != want.StartByte() || got.EndByte() != want.EndByte() ||
		got.StartPoint() != want.StartPoint() || got.EndPoint() != want.EndPoint() {
		t.Fatalf("%s range got=%d:%d %v:%v want=%d:%d %v:%v", path,
			got.StartByte(), got.EndByte(), got.StartPoint(), got.EndPoint(),
			want.StartByte(), want.EndByte(), want.StartPoint(), want.EndPoint())
	}
	if got.IsNamed() != want.IsNamed() || got.IsExtra() != want.IsExtra() ||
		got.IsMissing() != want.IsMissing() || got.HasError() != want.HasError() {
		t.Fatalf("%s flags differ", path)
	}
	if got.ChildCount() != want.ChildCount() {
		t.Fatalf("%s child count got=%d want=%d", path, got.ChildCount(), want.ChildCount())
	}
	for i := 0; i < got.ChildCount(); i++ {
		childPath := fmt.Sprintf("%s/%d", path, i)
		if got.FieldNameForChild(i, lang) != want.FieldNameForChild(i, lang) {
			t.Fatalf("%s field got=%q want=%q", childPath, got.FieldNameForChild(i, lang), want.FieldNameForChild(i, lang))
		}
		requireForestFallbackNodeIdentity(t, childPath, got.Child(i), want.Child(i), lang)
	}
}

// TestForestDispatchParity verifies the forest fast path is invisible: for an
// explicitly dispatched language, the forest tree must be byte-identical to
// production — same s-expr AND same root byte
// span — and anything the forest declines (malformed input, non-dispatched
// languages) must match production because we fall back to it.
// SetGLRForestEnabled(false) yields the production baseline; true enables the
// dispatch gate.
func TestForestDispatchParity(t *testing.T) {
	css := explicitForestLanguage(t, grm.CssLanguage())

	var big strings.Builder
	for i := 0; i < 60; i++ {
		big.WriteString(".cls-" + cssN(i) + " { color: red; margin: 0 1px 2px 3px; padding: 1em; }\n")
		big.WriteString("#id-" + cssN(i) + " > a:hover, .x .y { background: url(/img.png) no-repeat; }\n")
	}
	clean := []string{
		"a { color: red; }\n",
		".cls { margin: 0; padding: 1px 2px; z-index: 5; }\n",
		"@media (max-width: 600px) { .x { display: none; } }\n",
		"div > p + span ~ a:not(.z)::before { content: \"x\"; }\n",
		":root { --c: #fff; } body { color: var(--c); transform: matrix(1,2,3,4,5,6); }\n",
		big.String(),
	}
	malformed := []string{
		"a { color: red;\n",
		".x { ; } @media\n",
	}

	check := func(label string, lang *gts.Language, src string) {
		gts.SetGLRForestEnabled(false)
		prod, err := gts.NewParser(lang).Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: prod parse failed: %v", label, err)
			return
		}
		defer prod.Release()
		want := prod.RootNode().SExpr(lang)
		wantEnd := prod.RootNode().EndByte()
		gts.SetGLRForestEnabled(true)
		got, err := gts.NewParser(lang).Parse([]byte(src))
		if err != nil {
			t.Errorf("%s: forest parse failed: %v", label, err)
			return
		}
		defer got.Release()
		if got.RootNode().SExpr(lang) != want {
			t.Errorf("%s: forest dispatch s-expr diverged for %q", label, src)
		}
		if got.RootNode().EndByte() != wantEnd {
			t.Errorf("%s: forest dispatch root endByte %d != production %d for %q",
				label, got.RootNode().EndByte(), wantEnd, src)
		}
	}

	for _, s := range clean {
		check("css-clean", css, s)
	}
	for _, s := range malformed {
		check("css-malformed-fallback", css, s)
	}
	// Bash compatibility remains testable through explicit forest dispatch.
	bash := explicitForestLanguage(t, grm.BashLanguage())
	check("bash-dispatched", bash, "f() { echo a; }\n")
	// Non-dispatched languages must be untouched even with the switch on.
	check("go-untouched", grm.GoLanguage(), "package p\nfunc f() { return }\n")
	goTree, err := gts.NewParser(grm.GoLanguage()).Parse([]byte("package p\nfunc f() { return }\n"))
	if err != nil {
		t.Fatalf("go default parse: %v", err)
	}
	defer goTree.Release()
	if rt := goTree.ParseRuntime(); rt.ForestFastPath {
		t.Fatalf("go default parse used forest fast path with global forest enabled: %s", rt.Summary())
	}
	check("rust-untouched", grm.RustLanguage(), "fn main() {}\n")
	gts.SetGLRForestEnabled(true)
}

func TestForestExperimentalAppliesBashCompatibility(t *testing.T) {
	gts.SetGLRForestEnabled(false)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("url=`(curl -SsL https://registry.npmjs.org/npm/$t; echo \"\") \\\n     | sed -e 's/^.*tarball\":\"//' \\\n     | sed -e 's/\".*$//'`\n\nret=$?\n")
	lang := grm.BashLanguage()
	prod, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prod.Release()

	forest, ok := gts.NewParser(lang).ParseForestExperimental(src)
	if !ok || forest == nil || forest.RootNode() == nil {
		t.Fatalf("forest experimental ok=%v tree nil=%v", ok, forest == nil)
	}
	defer forest.Release()
	root := forest.RootNode()
	if got, want := root.SExpr(lang), prod.RootNode().SExpr(lang); got != want {
		t.Fatalf("forest experimental Bash compatibility mismatch\n got: %s\nwant: %s", got, want)
	}
	if got, want := root.NamedChildCount(), prod.RootNode().NamedChildCount(); got != want {
		t.Fatalf("forest Bash root named child count = %d, want production count %d; root=%s", got, want, root.SExpr(lang))
	}
	runtime := forest.ParseRuntime()
	if runtime.StopReason != gts.ParseStopAccepted || !runtime.ForestFastPath || runtime.SourceLen != uint32(len(src)) || runtime.RootEndByte != root.EndByte() {
		t.Fatalf("forest experimental runtime = %s, want accepted forest provenance", runtime.Summary())
	}
}

func TestBeancountDefaultSkipsForestAndExplicitReportsDecline(t *testing.T) {
	// This external-package test follows the file's existing non-parallel
	// convention: no public getter exists for the test/benchmark-only global
	// switch, so restore its process default rather than expanding the API.
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	// The leading comment is the smallest corpus-shaped witness that reaches
	// the explicit forest's conservative EOF recovery-conflict decline.
	src := []byte(";;; comment\n2024-01-01 open Assets:Bank\n")
	lang := grm.BeancountLanguage()
	parser := gts.NewParser(lang)
	automatic, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("automatic Beancount parse: %v", err)
	}
	defer automatic.Release()
	if rt := automatic.ParseRuntime(); rt.ForestFastPath {
		t.Fatalf("automatic Beancount parse used forest fast path: %s", rt.Summary())
	}
	if offset, sym, reason, states := parser.ForestDeclineInfo(); reason != "" {
		t.Fatalf("automatic Beancount parse attempted forest: offset=%d symbol=%d reason=%q states=%v", offset, sym, reason, states)
	}

	explicitParser := gts.NewParser(lang)
	explicit, ok := explicitParser.ParseForestExperimental(src)
	if ok || explicit != nil {
		if explicit != nil {
			explicit.Release()
		}
		t.Fatalf("explicit Beancount forest outcome ok=%v tree nil=%v, want strict decline", ok, explicit == nil)
	}
	if offset, sym, reason, states := explicitParser.ForestDeclineInfo(); reason != "eof-recovery-conflict" {
		t.Fatalf("explicit Beancount forest decline: offset=%d symbol=%d reason=%q states=%v, want reason=%q", offset, sym, reason, states, "eof-recovery-conflict")
	}
}

func TestExplicitOnlyLanguagesHaveStrictForestOutcomes(t *testing.T) {
	// Follow this file's existing non-parallel convention around the global
	// test/benchmark forest switch.
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	tests := []struct {
		name string
		lang func() *gts.Language
		src  string
	}{
		{name: "org", lang: grm.OrgLanguage, src: "# The GNU Free Documentation License.\n#+begin_center\nVersion 1.3, 3 November 2008\n#+end_center\n\n#+begin_verse\nCopyright 2008 Free Software Foundation, Inc.\n#+end_verse\n\n* ADDENDUM: How to use this License for your documents\n:PROPERTIES:\n:UNNUMBERED: notoc\n:END:\n"},
		{name: "vimdoc", lang: grm.VimdocLanguage, src: "*lua-guide.txt*                        Nvim\n\n                            NVIM REFERENCE MANUAL\n\n==============================================================================\nIntroduction                                                         *lua-guide*\n\nThis guide introduces Lua usage in Nvim.\n\nvim:tw=78:ts=8:sw=4:sts=4:et:ft=help:norl:\n"},
		{name: "fish", lang: grm.FishLanguage, src: "function greet\n    echo hello\nend\n"},
		{name: "racket", lang: grm.RacketLanguage, src: "#lang racket\n(define (square x) (* x x))\n(displayln (square 4))\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lang := tt.lang()
			src := []byte(tt.src)
			automaticParser := gts.NewParser(lang)
			automatic, err := automaticParser.Parse(src)
			if err != nil {
				t.Fatalf("automatic parse: %v", err)
			}
			defer automatic.Release()
			if rt := automatic.ParseRuntime(); rt.ForestFastPath {
				t.Fatalf("automatic parse used forest fast path: %s", rt.Summary())
			}
			if offset, sym, reason, states := automaticParser.ForestDeclineInfo(); reason != "" {
				t.Fatalf("automatic parse attempted forest: offset=%d symbol=%d reason=%q states=%v", offset, sym, reason, states)
			}

			explicitParser := gts.NewParser(lang)
			explicit, ok := explicitParser.ParseForestExperimental(src)
			if ok != (explicit != nil) {
				if explicit != nil {
					explicit.Release()
				}
				t.Fatalf("explicit forest outcome is incoherent: ok=%v tree nil=%v", ok, explicit == nil)
			}
			if !ok {
				if offset, sym, reason, states := explicitParser.ForestDeclineInfo(); reason == "" {
					t.Fatalf("explicit forest decline has no diagnostic: offset=%d symbol=%d states=%v", offset, sym, states)
				}
				return
			}
			defer explicit.Release()
			if got, want := explicit.RootNode().SExpr(lang), automatic.RootNode().SExpr(lang); got != want {
				t.Fatalf("explicit forest result mismatch\n got: %s\nwant: %s", got, want)
			}
			if got, want := explicit.RootNode().EndByte(), automatic.RootNode().EndByte(); got != want {
				t.Fatalf("explicit forest root endByte = %d, want %d", got, want)
			}
		})
	}
}

func TestForestDispatchReportsAcceptedRuntime(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("f() { echo a; }\n")
	lang := explicitForestLanguage(t, grm.BashLanguage())
	// Pin to production: this test asserts a production-engine internal (the
	// forest fast-path dispatch runtime) the compact candidate route bypasses.
	parser := gts.NewParser(lang)
	parser.SetAdmissionCandidateRoute(false)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("forest dispatch parse: %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if rt.StopReason != gts.ParseStopAccepted {
		t.Fatalf("forest dispatch stop reason = %q, want %q (%s)", rt.StopReason, gts.ParseStopAccepted, rt.Summary())
	}
	if !rt.ForestFastPath {
		t.Fatalf("forest dispatch ForestFastPath = false, want true (%s)", rt.Summary())
	}
	if rt.SourceLen != uint32(len(src)) || rt.ExpectedEOFByte != uint32(len(src)) || rt.LastTokenEndByte != uint32(len(src)) || !rt.LastTokenWasEOF {
		t.Fatalf("forest dispatch runtime mismatch: %s", rt.Summary())
	}
}

func TestForestDispatchDeclinesIncludedRanges(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("a { color: red; }\n")
	parser := gts.NewParser(explicitForestLanguage(t, grm.CssLanguage()))
	parser.SetIncludedRanges([]gts.Range{{StartByte: 0, EndByte: uint32(len(src))}})
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse with included range: %v", err)
	}
	defer tree.Release()
	rt := tree.ParseRuntime()
	if rt.TokensConsumed == 0 {
		t.Fatalf("forest fast path was used despite included ranges: %s", rt.Summary())
	}
}

func TestForestDispatchPromotesJavaScript(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("function foo() {}\nfoo()\nlet plus1 = x => x + 1\nasync function* bar() { yield 1; }\n")
	lang := grm.JavascriptLanguage()
	gts.SetGLRForestEnabled(false)
	prod, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prod.Release()

	gts.SetGLRForestEnabled(true)
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("forest dispatch parse: %v", err)
	}
	defer tree.Release()
	if got, want := tree.RootNode().SExpr(lang), prod.RootNode().SExpr(lang); got != want {
		t.Fatalf("JavaScript forest dispatch diverged\n got: %s\nwant: %s", got, want)
	}
	if got, want := prod.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("JavaScript production root end byte = %d, want %d", got, want)
	}
	if got, want := tree.RootNode().EndByte(), uint32(len(src)); got != want {
		t.Fatalf("JavaScript forest root end byte = %d, want %d", got, want)
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("JavaScript did not use forest accepted runtime: %s", rt.Summary())
	}
}

func TestForestDispatchPromotesCSharp(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(`using System;
class C {
  string Format(int x) => $"value={x}";
  void M() {
    foreach (var item in new[] {1, 2, 3}) {
      Console.WriteLine(Format(item));
    }
  }
}
`)
	lang := explicitForestLanguage(t, grm.CSharpLanguage())
	gts.SetGLRForestEnabled(false)
	prod, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("production parse: %v", err)
	}
	defer prod.Release()

	gts.SetGLRForestEnabled(true)
	tree, err := gts.NewParser(lang).Parse(src)
	if err != nil {
		t.Fatalf("forest dispatch parse: %v", err)
	}
	defer tree.Release()
	if got, want := tree.RootNode().SExpr(lang), prod.RootNode().SExpr(lang); got != want {
		t.Fatalf("C# forest dispatch diverged\n got: %s\nwant: %s", got, want)
	}
	rt := tree.ParseRuntime()
	if rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("C# did not use forest accepted runtime: %s", rt.Summary())
	}
}

func TestForestTreeIncrementalEditCSharpNumericLiteralFastRescue(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := makeCSharpBenchmarkSource(16)
	sites := makeBenchmarkEditSites(src, "var v = ")
	if len(sites) == 0 {
		t.Fatal("missing C# numeric edit site")
	}
	site := sites[0]
	edited := append([]byte(nil), src...)
	toggleDigitAt(edited, site.offset)

	lang := explicitForestLanguage(t, grm.CSharpLanguage())
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	if rt := oldTree.ParseRuntime(); rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("initial parse did not use forest fast path: %s", rt.Summary())
	}
	oldTree.Edit(gts.InputEdit{
		StartByte:   uint32(site.offset),
		OldEndByte:  uint32(site.offset + 1),
		NewEndByte:  uint32(site.offset + 1),
		StartPoint:  site.start,
		OldEndPoint: site.end,
		NewEndPoint: site.end,
	})

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	requireCompleteParse(t, newTree, edited, lang, "incremental")
	requireReleaseSameWidthReparse(t, profile)
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
	if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental C# tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func TestForestTreeIncrementalEditAwkNumberFastRescue(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("BEGIN { print 1 }\n")
	offset := strings.Index(string(src), "1")
	if offset < 0 || src[offset] != '1' {
		t.Fatalf("AWK fixture drifted: byte %d", offset)
	}
	edited := append([]byte(nil), src...)
	edited[offset] = '2'

	lang := grm.AwkLanguage()
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	if rt := oldTree.ParseRuntime(); rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("initial parse did not use forest fast path: %s", rt.Summary())
	}
	oldTree.Edit(gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	})

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	requireCompleteParse(t, newTree, edited, lang, "incremental")
	requireReleaseSameWidthReparse(t, profile)
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
	if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental AWK tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func TestForestTreeIncrementalEditCSharpIdentifierFastRescue(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(`interface I1 {}
interface I2 {}
record F<T1, T2> where T1 : I1, I2, new() where T2 : I2 { }
`)
	oldNeedle := []byte("T1, T2")
	offset := strings.Index(string(src), string(oldNeedle)) + len("T")
	if offset < len("T") || src[offset] != '1' {
		t.Fatalf("C# identifier fixture drifted: byte %d = %q, want '1'", offset, src[offset])
	}
	edited := append([]byte(nil), src...)
	edited[offset] = '3'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	lang := explicitForestLanguage(t, grm.CSharpLanguage())
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	if rt := oldTree.ParseRuntime(); rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("initial parse did not use forest fast path: %s", rt.Summary())
	}

	tree := oldTree
	current := src
	for i := 0; i < 4; i++ {
		next := append([]byte(nil), current...)
		if next[offset] == '1' {
			next[offset] = '3'
		} else {
			next[offset] = '1'
		}
		tree.Edit(edit)

		newTree, profile, err := parser.ParseIncrementalProfiled(next, tree)
		if err != nil {
			t.Fatalf("incremental parse %d: %v", i, err)
		}
		requireCompleteParse(t, newTree, next, lang, "incremental")
		requireReleaseSameWidthReparse(t, profile)
		freshTree, err := parser.Parse(next)
		if err != nil {
			newTree.Release()
			t.Fatalf("fresh parse %d: %v", i, err)
		}
		requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
		if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
			freshTree.Release()
			newTree.Release()
			t.Fatalf("incremental C# identifier tree %d diverged from fresh parse\n got: %s\nwant: %s", i, got, want)
		}
		freshTree.Release()
		if tree != oldTree {
			tree.Release()
		}
		tree = newTree
		current = next
	}
	if tree != oldTree {
		tree.Release()
	}
}

func TestForestTreeIncrementalEditCSharpContextualIdentifierStillFallsBack(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(`class C {
  void M() {
    var l = scoped => null;
  }
}
`)
	const oldNeedle = "scoped"
	offset := strings.Index(string(src), oldNeedle)
	if offset < 0 || src[offset] != 's' {
		t.Fatalf("C# contextual identifier fixture drifted: offset=%d", offset)
	}
	edited := append([]byte(nil), src...)
	edited[offset] = 't'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	lang := explicitForestLanguage(t, grm.CSharpLanguage())
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	if rt := oldTree.ParseRuntime(); rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("initial parse did not use forest fast path: %s", rt.Summary())
	}
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	requireCompleteParse(t, newTree, edited, lang, "incremental")
	if !profile.ReuseUnsupported {
		t.Fatal("C# contextual identifier edit used disabled-tree token-invariant rescue, want fresh fallback")
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental C# contextual identifier tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func TestForestTreeIncrementalEditCSharpStringLiteralStillFallsBack(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(`class C {
  string M(int x) => $"value={x}";
}
`)
	const oldNeedle = "value="
	offset := strings.Index(string(src), oldNeedle)
	if offset < 0 || src[offset] != 'v' {
		t.Fatalf("C# string fixture drifted: offset=%d", offset)
	}
	edited := append([]byte(nil), src...)
	edited[offset] = 'V'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	lang := explicitForestLanguage(t, grm.CSharpLanguage())
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	if rt := oldTree.ParseRuntime(); rt.StopReason != gts.ParseStopAccepted || !rt.ForestFastPath || !rt.LastTokenWasEOF || rt.TokensConsumed != 0 {
		t.Fatalf("initial parse did not use forest fast path: %s", rt.Summary())
	}
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	requireCompleteParse(t, newTree, edited, lang, "incremental")
	if !profile.ReuseUnsupported {
		t.Fatal("C# string literal edit used disabled-tree token-invariant rescue, want fresh fallback")
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental C# string tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

// TestForestTreeIncrementalEditCSSTokenInvariantLeafReuseIsCorrect verifies the
// safe reuse path for css forest trees that are otherwise demoted from general
// forest-incremental reuse. Same-length edits inside a leaf can reuse the old
// tree when rescanning the edited leaf preserves token kind and span.
func TestForestTreeIncrementalEditCSSTokenInvariantLeafReuseIsCorrect(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte(".a { color: red; margin: 1px; padding: 4px; }\n.b { color: blue; transform: translateX(1px); }\n")
	const oldNeedle = "margin: 1px"
	offset := strings.Index(string(src), oldNeedle) + len("margin: ")
	if offset < len("margin: ") || len(src) <= offset || src[offset] != '1' {
		t.Fatalf("css fixture drifted: byte %d = %q, want '1'", offset, src[offset])
	}

	edited := append([]byte(nil), src...)
	edited[offset] = '2'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	lang := explicitForestLanguage(t, grm.CssLanguage())
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	requireAcceptedForestRuntime(t, "initial CSS parse", oldTree)
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
		t.Fatalf("incremental root end = %d, want %d", got, want)
	}
	requireReleaseSameWidthReparse(t, profile)
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	requireAcceptedForestRuntime(t, "fresh CSS parse", freshTree)
	requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
	if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental CSS tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func TestForestTreeIncrementalEditSCSSTokenInvariantLeafReuseIsCorrect(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("$gap: 1px;\n.a { color: red; margin: $gap; padding: 4px; .child { width: 1px; } }\n")
	const oldNeedle = "padding: 4px"
	offset := strings.Index(string(src), oldNeedle) + len("padding: ")
	if offset < len("padding: ") || len(src) <= offset || src[offset] != '4' {
		t.Fatalf("scss fixture drifted: byte %d = %q, want '4'", offset, src[offset])
	}

	edited := append([]byte(nil), src...)
	edited[offset] = '5'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	lang := explicitForestLanguage(t, grm.ScssLanguage())
	parser := gts.NewParser(lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	requireAcceptedForestRuntime(t, "initial SCSS parse", oldTree)
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
		t.Fatalf("incremental root end = %d, want %d", got, want)
	}
	requireReleaseSameWidthReparse(t, profile)
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	requireAcceptedForestRuntime(t, "fresh SCSS parse", freshTree)
	requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
	if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental SCSS tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func TestYAMLIncrementalEditScalarTokenInvariantLeafReuseIsCorrect(t *testing.T) {
	lang := grm.YamlLanguage()
	src := []byte("uses: actions/checkout@v4\ncount: [0]\ntime: 2001-11-23 15:01:42 -5\n")
	for _, tc := range []struct {
		name        string
		needle      string
		oldByte     byte
		replacement byte
	}{
		{name: "string version", needle: "v4", oldByte: '4', replacement: '5'},
		{name: "integer scalar", needle: "[0]", oldByte: '0', replacement: '1'},
		{name: "timestamp string", needle: "2001", oldByte: '2', replacement: '3'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offset := strings.Index(string(src), tc.needle)
			if offset < 0 {
				t.Fatalf("fixture missing %q", tc.needle)
			}
			for offset < len(src) && src[offset] != tc.oldByte {
				offset++
			}
			if offset >= len(src) {
				t.Fatalf("fixture missing byte %q in %q", tc.oldByte, tc.needle)
			}
			edited := append([]byte(nil), src...)
			edited[offset] = tc.replacement
			edit := gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(src, offset),
				OldEndPoint: pointForOffset(src, offset+1),
				NewEndPoint: pointForOffset(edited, offset+1),
			}

			parser := gts.NewParser(lang)
			// This test isolates production incremental leaf reuse. Compact full-
			// parse trees deliberately carry the decision-0008 reuse bar; as compact
			// coverage expands, relying on an incidental admission fallback would
			// silently change what this test exercises.
			parser.SetAdmissionCandidateRoute(false)
			oldTree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer oldTree.Release()
			oldTree.Edit(edit)

			newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer newTree.Release()
			requireReleaseSameWidthReparse(t, profile)
			if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
				t.Fatalf("incremental root end = %d, want %d", got, want)
			}
			freshTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
			requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
			if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental YAML tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestPowerShellIncrementalEditTextInvariantLeafReuseIsCorrect(t *testing.T) {
	lang := grm.PowershellLanguage()
	src := []byte(`# note 1
. "$PSScriptRoot\..\buildCommon\startNativeExecution.ps1"
@{ GUID = "56D66100-99A0-4FFC-A12D-EEE9A6718AEF" }
`)
	for _, tc := range []struct {
		name        string
		needle      string
		oldByte     byte
		replacement byte
	}{
		{name: "line comment", needle: "# note 1", oldByte: '1', replacement: '2'},
		{name: "interpolated string text", needle: "startNativeExecution.ps1", oldByte: '1', replacement: '2'},
		{name: "guid string", needle: "56D66100", oldByte: '5', replacement: '6'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offset := strings.Index(string(src), tc.needle)
			if offset < 0 {
				t.Fatalf("fixture missing %q", tc.needle)
			}
			for offset < len(src) && src[offset] != tc.oldByte {
				offset++
			}
			if offset >= len(src) {
				t.Fatalf("fixture missing byte %q in %q", tc.oldByte, tc.needle)
			}
			edited := append([]byte(nil), src...)
			edited[offset] = tc.replacement
			edit := gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(src, offset),
				OldEndPoint: pointForOffset(src, offset+1),
				NewEndPoint: pointForOffset(edited, offset+1),
			}

			parser := gts.NewParser(lang)
			oldTree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer oldTree.Release()
			requireCompleteParse(t, oldTree, src, lang, "initial")
			oldTree.Edit(edit)

			newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer newTree.Release()
			requireReleaseSameWidthReparse(t, profile)
			requireCompleteParse(t, newTree, edited, lang, "incremental")
			freshTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
			requireCompleteParse(t, freshTree, edited, lang, "fresh")
			requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
			if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental PowerShell tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestHCLIncrementalEditDigitLeafReuseIsCorrect(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	lang := grm.HclLanguage()
	src := []byte(`resource "aws_instance" "foo" {
  count = "2"
  cidr = "10.0.0.0/16"
  priority = 1
}
`)
	for _, tc := range []struct {
		name        string
		needle      string
		oldByte     byte
		replacement byte
	}{
		{name: "template literal", needle: "10.0.0.0/16", oldByte: '1', replacement: '2'},
		{name: "numeric literal", needle: "priority = 1", oldByte: '1', replacement: '2'},
	} {
		t.Run(tc.name, func(t *testing.T) {
			offset := strings.Index(string(src), tc.needle)
			if offset < 0 {
				t.Fatalf("fixture missing %q", tc.needle)
			}
			for offset < len(src) && src[offset] != tc.oldByte {
				offset++
			}
			if offset >= len(src) {
				t.Fatalf("fixture missing byte %q in %q", tc.oldByte, tc.needle)
			}
			edited := append([]byte(nil), src...)
			edited[offset] = tc.replacement
			edit := gts.InputEdit{
				StartByte:   uint32(offset),
				OldEndByte:  uint32(offset + 1),
				NewEndByte:  uint32(offset + 1),
				StartPoint:  pointForOffset(src, offset),
				OldEndPoint: pointForOffset(src, offset+1),
				NewEndPoint: pointForOffset(edited, offset+1),
			}

			parser := gts.NewParser(lang)
			oldTree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("initial parse: %v", err)
			}
			defer oldTree.Release()
			if oldTree.RootNode().HasError() {
				t.Fatalf("initial HCL parse has errors: %s", oldTree.RootNode().SExpr(lang))
			}
			oldTree.Edit(edit)

			newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
			if err != nil {
				t.Fatalf("incremental parse: %v", err)
			}
			defer newTree.Release()
			requireReleaseSameWidthReparse(t, profile)
			freshTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
			requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, lang)
			if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental HCL tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// TestForestTreeIncrementalEditCMakeTextInvariantLeafReuseIsCorrect: cmake
// remains outside forestIncrementalReuseProven, but same-length
// alphanumeric edits inside unquoted_argument leaves can use the token-invariant
// rescue path safely.
func TestForestTreeIncrementalEditCMakeTextInvariantLeafReuseIsCorrect(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("cmake_minimum_required(VERSION 3.20)\nproject(demo)\nadd_library(demo STATIC demo.cc)\ntarget_compile_definitions(demo PRIVATE VALUE=1)\n")
	oldNeedle := []byte("VALUE=1")
	offset := strings.Index(string(src), string(oldNeedle)) + len("VALUE=")
	if offset < len("VALUE=") || src[offset] != '1' {
		t.Fatalf("cmake fixture drifted: byte %d = %q, want '1'", offset, src[offset])
	}

	edited := append([]byte(nil), src...)
	edited[offset] = '2'
	edit := gts.InputEdit{
		StartByte:   uint32(offset),
		OldEndByte:  uint32(offset + 1),
		NewEndByte:  uint32(offset + 1),
		StartPoint:  pointForOffset(src, offset),
		OldEndPoint: pointForOffset(src, offset+1),
		NewEndPoint: pointForOffset(edited, offset+1),
	}

	lang := *grm.CmakeLanguage()
	lang.WantsForest = true
	parser := gts.NewParser(&lang)
	oldTree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("initial parse: %v", err)
	}
	defer oldTree.Release()
	oldTree.Edit(edit)

	newTree, profile, err := parser.ParseIncrementalProfiled(edited, oldTree)
	if err != nil {
		t.Fatalf("incremental parse: %v", err)
	}
	defer newTree.Release()
	if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
		t.Fatalf("incremental root end = %d, want %d", got, want)
	}
	requireReleaseSameWidthReparse(t, profile)
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	requireIncrementalDeepTreeMatchesFresh(t, newTree, freshTree, &lang)
	if got, want := newTree.RootNode().SExpr(&lang), freshTree.RootNode().SExpr(&lang); got != want {
		t.Fatalf("incremental CMake tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
	}
}

func cssN(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func pointForOffset(src []byte, offset int) gts.Point {
	var pt gts.Point
	for _, b := range src[:offset] {
		if b == '\n' {
			pt.Row++
			pt.Column = 0
		} else {
			pt.Column++
		}
	}
	return pt
}
