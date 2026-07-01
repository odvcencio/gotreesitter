package gotreesitter_test

import (
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	grm "github.com/odvcencio/gotreesitter/grammars"
)

// TestForestDispatchParity verifies the (default-on) forest fast path is
// invisible: for a dispatched language (css ∈ builtinForestDefaults) the forest
// tree must be byte-identical to production — same s-expr AND same root byte
// span — and anything the forest declines (malformed input, non-dispatched
// languages) must match production because we fall back to it.
// SetGLRForestEnabled(false) yields the production baseline; (true) is the
// default-on dispatch.
func TestForestDispatchParity(t *testing.T) {
	css := grm.CssLanguage()

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
	// Bash is dispatched only after root compatibility normalization.
	check("bash-dispatched", grm.BashLanguage(), "f() { echo a; }\n")
	// Non-dispatched languages must be untouched even with the switch on.
	check("go-untouched", grm.GoLanguage(), "package p\nfunc f() { return }\n")
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
	if got, want := root.NamedChildCount(), 2; got != want {
		t.Fatalf("forest Bash root named child count = %d, want %d; root=%s", got, want, root.SExpr(lang))
	}
}

func TestForestDispatchReportsAcceptedRuntime(t *testing.T) {
	gts.SetGLRForestEnabled(true)
	defer gts.SetGLRForestEnabled(true)

	src := []byte("f() { echo a; }\n")
	tree, err := gts.NewParser(grm.BashLanguage()).Parse(src)
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
	lang := grm.CSharpLanguage()
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

	lang := grm.CSharpLanguage()
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
	if profile.ReuseUnsupported {
		t.Fatalf("C# numeric literal edit fell back to fresh parse: %s", profile.ReuseUnsupportedReason)
	}
	if profile.ReparseNanos != 0 {
		t.Fatalf("ReparseNanos = %d, want 0 for token-invariant C# numeric literal edit", profile.ReparseNanos)
	}
	if profile.ReusedSubtrees != 1 || profile.ReusedBytes != uint64(len(edited)) {
		t.Fatalf("reuse profile = subtrees %d bytes %d, want 1/%d", profile.ReusedSubtrees, profile.ReusedBytes, len(edited))
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
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
	if profile.ReuseUnsupported {
		t.Fatalf("AWK number edit fell back to fresh parse: %s", profile.ReuseUnsupportedReason)
	}
	if profile.ReparseNanos != 0 {
		t.Fatalf("ReparseNanos = %d, want 0 for token-invariant AWK number edit", profile.ReparseNanos)
	}
	if profile.ReusedSubtrees == 0 {
		t.Fatalf("AWK number edit reused no subtrees: %+v", profile)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
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

	lang := grm.CSharpLanguage()
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
		if profile.ReuseUnsupported {
			leaf := tree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
			t.Fatalf("C# identifier edit %d fell back to fresh parse: %s leaf=%s text=%q", i, profile.ReuseUnsupportedReason, leaf.Type(lang), leaf.Text(current))
		}
		if profile.ReparseNanos != 0 {
			t.Fatalf("ReparseNanos = %d, want 0 for token-invariant C# identifier edit %d", profile.ReparseNanos, i)
		}
		freshTree, err := parser.Parse(next)
		if err != nil {
			newTree.Release()
			t.Fatalf("fresh parse %d: %v", i, err)
		}
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

	lang := grm.CSharpLanguage()
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

	lang := grm.CSharpLanguage()
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

	parser := gts.NewParser(grm.CssLanguage())
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
	if profile.ReuseUnsupported {
		leaf := oldTree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
		leafType := "<nil>"
		leafText := ""
		leafChildren := 0
		if leaf != nil {
			leafType = leaf.Type(grm.CssLanguage())
			leafText = leaf.Text(src)
			leafChildren = leaf.ChildCount()
		}
		t.Fatalf("css token-invariant leaf edit fell back to fresh parse: %s leaf=%s children=%d text=%q", profile.ReuseUnsupportedReason, leafType, leafChildren, leafText)
	}
	if profile.ReusedSubtrees == 0 {
		t.Fatalf("css token-invariant leaf edit reused no subtrees: %+v", profile)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(grm.CssLanguage()), freshTree.RootNode().SExpr(grm.CssLanguage()); got != want {
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

	parser := gts.NewParser(grm.ScssLanguage())
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
	if profile.ReuseUnsupported {
		leaf := oldTree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
		leafType := "<nil>"
		leafText := ""
		leafChildren := 0
		if leaf != nil {
			leafType = leaf.Type(grm.ScssLanguage())
			leafText = leaf.Text(src)
			leafChildren = leaf.ChildCount()
		}
		t.Fatalf("scss token-invariant leaf edit fell back to fresh parse: %s leaf=%s children=%d text=%q", profile.ReuseUnsupportedReason, leafType, leafChildren, leafText)
	}
	if profile.ReusedSubtrees == 0 {
		t.Fatalf("scss token-invariant leaf edit reused no subtrees: %+v", profile)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(grm.ScssLanguage()), freshTree.RootNode().SExpr(grm.ScssLanguage()); got != want {
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
			if profile.ReuseUnsupported {
				leaf := oldTree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
				leafType := "<nil>"
				leafText := ""
				if leaf != nil {
					leafType = leaf.Type(lang)
					leafText = leaf.Text(src)
				}
				t.Fatalf("yaml token-invariant leaf edit fell back to fresh parse: %s leaf=%s text=%q", profile.ReuseUnsupportedReason, leafType, leafText)
			}
			if profile.ReusedSubtrees == 0 {
				t.Fatalf("yaml token-invariant leaf edit reused no subtrees: %+v", profile)
			}
			if got, want := newTree.RootNode().EndByte(), uint32(len(edited)); got != want {
				t.Fatalf("incremental root end = %d, want %d", got, want)
			}
			freshTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
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
			if profile.ReuseUnsupported {
				leaf := oldTree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
				leafType := "<nil>"
				leafText := ""
				leafChildren := 0
				if leaf != nil {
					leafType = leaf.Type(lang)
					leafText = leaf.Text(src)
					leafChildren = leaf.ChildCount()
				}
				t.Fatalf("powershell text-invariant edit fell back to fresh parse: %s leaf=%s children=%d text=%q", profile.ReuseUnsupportedReason, leafType, leafChildren, leafText)
			}
			if profile.ReusedSubtrees == 0 {
				t.Fatalf("powershell text-invariant edit reused no subtrees: %+v", profile)
			}
			requireCompleteParse(t, newTree, edited, lang, "incremental")
			freshTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
			requireCompleteParse(t, freshTree, edited, lang, "fresh")
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
			if profile.ReuseUnsupported {
				leaf := oldTree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
				leafType := "<nil>"
				leafText := ""
				if leaf != nil {
					leafType = leaf.Type(lang)
					leafText = leaf.Text(src)
				}
				t.Fatalf("hcl digit leaf edit fell back to fresh parse: %s leaf=%s text=%q", profile.ReuseUnsupportedReason, leafType, leafText)
			}
			if profile.ReparseNanos != 0 {
				t.Fatalf("ReparseNanos = %d, want 0 for HCL digit leaf edit", profile.ReparseNanos)
			}
			if profile.ReusedSubtrees == 0 {
				t.Fatalf("hcl digit leaf edit reused no subtrees: %+v", profile)
			}
			freshTree, err := parser.Parse(edited)
			if err != nil {
				t.Fatalf("fresh parse: %v", err)
			}
			defer freshTree.Release()
			if got, want := newTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
				t.Fatalf("incremental HCL tree diverged from fresh parse\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// TestForestTreeIncrementalEditCMakeTextInvariantLeafReuseIsCorrect: cmake
// remains demoted from languageAllowsForestIncrementalPath, but same-length
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

	parser := gts.NewParser(grm.CmakeLanguage())
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
	if profile.ReuseUnsupported {
		leaf := oldTree.RootNode().DescendantForByteRange(uint32(offset), uint32(offset+1))
		leafType := "<nil>"
		leafText := ""
		if leaf != nil {
			leafType = leaf.Type(grm.CmakeLanguage())
			leafText = leaf.Text(src)
		}
		t.Fatalf("cmake text-invariant leaf edit fell back to fresh parse: %s leaf=%s text=%q", profile.ReuseUnsupportedReason, leafType, leafText)
	}
	if profile.ReparseNanos != 0 {
		t.Fatalf("ReparseNanos = %d, want 0 for CMake text-invariant leaf edit", profile.ReparseNanos)
	}
	if profile.ReusedSubtrees == 0 {
		t.Fatalf("cmake text-invariant leaf edit reused no subtrees: %+v", profile)
	}
	freshTree, err := parser.Parse(edited)
	if err != nil {
		t.Fatalf("fresh parse: %v", err)
	}
	defer freshTree.Release()
	if got, want := newTree.RootNode().SExpr(grm.CmakeLanguage()), freshTree.RootNode().SExpr(grm.CmakeLanguage()); got != want {
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
