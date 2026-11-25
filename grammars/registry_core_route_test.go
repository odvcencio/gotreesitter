package grammars

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/ccore/certified"
)

// registryCoreInputs holds a valid input and an input with a syntax error for
// each grammar whose registry entry has a token-source factory. A certified
// grammar with a factory and no inputs here fails the test, so a new factory
// cannot skip it.
var registryCoreInputs = map[string][2]string{
	"c":       {"#include <stdio.h>\nint main(void) {\n  int x = 1;\n  return x + 2;\n}\n", "void f() { x = ; y = 1 }\n"},
	"cpp":     {"#include <vector>\nint main() {\n  std::vector<int> v{1, 2};\n  return v.size();\n}\n", "class A { int f( { return 1; } };\n"},
	"java":    {"class A {\n  int x = 1;\n  void f() { x++; }\n}\n", "class A { void f() { int x = ; } }\n"},
	"authzed": {"definition user {}\n\ndefinition document {\n\trelation viewer: user\n\tpermission view = viewer\n}\n", "definition document {\n\trelation viewer: \n\tpermission view = viewer +\n}\n"},
}

// registryCoreDigest is a digest of every node of the tree under root: type,
// range, points, flags and field name.
func registryCoreDigest(root *gotreesitter.Node, lang *gotreesitter.Language) string {
	if root == nil {
		return "nil"
	}
	h := sha256.New()
	var visit func(n *gotreesitter.Node, field string)
	visit = func(n *gotreesitter.Node, field string) {
		s, e := n.StartPoint(), n.EndPoint()
		fmt.Fprintf(h, "%s|%d|%d|%d.%d|%d.%d|%v|%v|%v|%v|%v|%s;", n.Type(lang), n.StartByte(), n.EndByte(), s.Row, s.Column, e.Row, e.Column,
			n.IsNamed(), n.IsMissing(), n.IsExtra(), n.IsError(), n.HasError(), field)
		for i := 0; i < n.ChildCount(); i++ {
			visit(n.Child(i), n.FieldNameForChild(i, lang))
		}
	}
	visit(root, "")
	return fmt.Sprintf("%x", h.Sum(nil))
}

// registryCoreServed is the number of requests the core served, fresh and
// incremental, and the number it declined, since the process started.
func registryCoreServed() (served, declined int64) {
	d, f, i := bind.RouteCounters()
	for _, v := range d {
		declined += v
	}
	return f + i, declined
}

// registryCoreCertified reports whether the generated certificate list holds the
// language's grammar blob.
func registryCoreCertified(lang *gotreesitter.Language) bool {
	sha, ok := lang.GrammarBlobSHA256()
	if !ok {
		return false
	}
	_, ok = certified.Blobs[sha]
	return ok
}

// registryCoreEdit inserts one space before the first newline of src.
func registryCoreEdit(src []byte) ([]byte, gotreesitter.InputEdit) {
	at := strings.IndexByte(string(src), '\n')
	if at < 0 {
		at = len(src)
	}
	out := append(append(append([]byte(nil), src[:at]...), ' '), src[at:]...)
	p := gotreesitter.Point{Row: 0, Column: uint32(at)}
	return out, gotreesitter.InputEdit{
		StartByte: uint32(at), OldEndByte: uint32(at), NewEndByte: uint32(at + 1),
		StartPoint: p, OldEndPoint: p, NewEndPoint: gotreesitter.Point{Row: 0, Column: uint32(at + 1)},
	}
}

// registryCoreRoute is one public way to parse through a registry entry's
// token-source factory. A fresh route parses src; an incremental route parses
// src after a one-byte edit of an old tree that the same route made. run
// returns the root of the result (nil for a route that returns no tree) and a
// function that releases what it made.
type registryCoreRoute struct {
	name string
	incr bool
	run  func(t *testing.T, e *LangEntry, lang *gotreesitter.Language, src []byte) (*gotreesitter.Node, func())
}

func registryCoreRoutes() []registryCoreRoute {
	tree := func(tr *gotreesitter.Tree, err error, t *testing.T, what string) (*gotreesitter.Node, func()) {
		t.Helper()
		if err != nil || tr == nil {
			t.Fatalf("%s: tree %v, error %v", what, tr != nil, err)
		}
		return tr.RootNode(), tr.Release
	}
	bound := func(bt *gotreesitter.BoundTree, err error, t *testing.T, what string) (*gotreesitter.Node, func()) {
		t.Helper()
		if err != nil || bt == nil {
			t.Fatalf("%s: tree %v, error %v", what, bt != nil, err)
		}
		return bt.RootNode(), bt.Release
	}
	filename := func(t *testing.T, e *LangEntry) string {
		t.Helper()
		for _, ext := range e.Extensions {
			if d := DetectLanguage("x" + ext); d != nil && d.Name == e.Name {
				return "x" + ext
			}
		}
		t.Fatalf("%s: no extension detects the entry", e.Name)
		return ""
	}
	factory := func(e *LangEntry, lang *gotreesitter.Language) func([]byte) gotreesitter.TokenSource {
		return func(b []byte) gotreesitter.TokenSource { return e.TokenSourceFactory(b, lang) }
	}
	highlighter := func(t *testing.T, e *LangEntry) *gotreesitter.Highlighter {
		t.Helper()
		h, err := e.NewHighlighter()
		if err != nil {
			t.Fatalf("NewHighlighter: %v", err)
		}
		return h
	}
	tagger := func(t *testing.T, e *LangEntry, lang *gotreesitter.Language) *gotreesitter.Tagger {
		t.Helper()
		query := e.TagsQuery
		if query == "" {
			query = "(ERROR) @name" // the route is what is checked; every grammar has ERROR
		}
		tg, err := gotreesitter.NewTagger(lang, query, gotreesitter.WithTaggerTokenSourceFactory(factory(e, lang)))
		if err != nil {
			t.Fatalf("NewTagger: %v", err)
		}
		return tg
	}
	editedOld := func(t *testing.T, src []byte, first func([]byte) *gotreesitter.Tree) (*gotreesitter.Tree, []byte) {
		t.Helper()
		old := first(src)
		if old == nil {
			t.Fatal("no old tree")
		}
		edited, e := registryCoreEdit(src)
		old.Edit(e)
		return old, edited
	}
	return []registryCoreRoute{
		{name: "LangEntry.Parse", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			tr, err := e.Parse(src)
			return tree(tr, err, t, "LangEntry.Parse")
		}},
		{name: "LangEntry.ParseContext", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			tr, err := e.ParseContext(ctx, src)
			return tree(tr, err, t, "LangEntry.ParseContext")
		}},
		{name: "LangEntry.ParseStrict", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			tr, err := e.ParseStrict(context.Background(), src)
			return tree(tr, err, t, "LangEntry.ParseStrict")
		}},
		{name: "ParseFile", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			bt, err := ParseFile(filename(t, e), src)
			return bound(bt, err, t, "ParseFile")
		}},
		{name: "ParseFilePooled", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			bt, err := ParseFilePooled(filename(t, e), src)
			return bound(bt, err, t, "ParseFilePooled")
		}},
		{name: "ParseFilePooledStrict", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			bt, err := ParseFilePooledStrict(filename(t, e), src)
			return bound(bt, err, t, "ParseFilePooledStrict")
		}},
		{name: "Highlighter.Highlight", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			highlighter(t, e).Highlight(src)
			return nil, func() {}
		}},
		{name: "Highlighter.HighlightIncremental/fresh", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			_, tr := highlighter(t, e).HighlightIncremental(src, nil)
			return tree(tr, nil, t, "HighlightIncremental")
		}},
		{name: "Highlighter.HighlightIncrementalUTF16/fresh", run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			_, tr := highlighter(t, e).HighlightIncrementalUTF16(utf16.Encode([]rune(string(src))), nil)
			return tree(tr, nil, t, "HighlightIncrementalUTF16")
		}},
		{name: "Tagger.TagIncremental/fresh", run: func(t *testing.T, e *LangEntry, lang *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			tg := tagger(t, e, lang)
			_, tr := tg.TagIncremental(src, nil)
			return tree(tr, nil, t, "TagIncremental")
		}},
		{name: "Parser.ParseIncrementalWithTokenSource", incr: true, run: func(t *testing.T, e *LangEntry, lang *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			p := gotreesitter.NewParser(lang)
			old, edited := editedOld(t, src, func(b []byte) *gotreesitter.Tree {
				tr, _ := p.ParseWithTokenSource(b, e.TokenSourceFactory(b, lang))
				return tr
			})
			defer old.Release()
			before, _ := registryCoreServed()
			tr, err := p.ParseIncrementalWithTokenSource(edited, old, e.TokenSourceFactory(edited, lang))
			registryCoreMark(t, before)
			return tree(tr, err, t, "ParseIncrementalWithTokenSource")
		}},
		{name: "Highlighter.HighlightIncremental/edit", incr: true, run: func(t *testing.T, e *LangEntry, _ *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			h := highlighter(t, e)
			old, edited := editedOld(t, src, func(b []byte) *gotreesitter.Tree {
				_, tr := h.HighlightIncremental(b, nil)
				return tr
			})
			defer old.Release()
			before, _ := registryCoreServed()
			_, tr := h.HighlightIncremental(edited, old)
			registryCoreMark(t, before)
			return tree(tr, nil, t, "HighlightIncremental")
		}},
		{name: "Tagger.TagIncremental/edit", incr: true, run: func(t *testing.T, e *LangEntry, lang *gotreesitter.Language, src []byte) (*gotreesitter.Node, func()) {
			tg := tagger(t, e, lang)
			old, edited := editedOld(t, src, func(b []byte) *gotreesitter.Tree {
				_, tr := tg.TagIncremental(b, nil)
				return tr
			})
			defer old.Release()
			before, _ := registryCoreServed()
			_, tr := tg.TagIncremental(edited, old)
			registryCoreMark(t, before)
			return tree(tr, nil, t, "TagIncremental")
		}},
	}
}

// registryCoreIncrServed carries, for an incremental route, the serve count of
// the reparse alone (the route also makes its old tree).
var registryCoreIncrServed int64

func registryCoreMark(t *testing.T, before int64) {
	t.Helper()
	after, _ := registryCoreServed()
	registryCoreIncrServed = after - before
}

// Every public entry point that parses through a registry entry's token-source
// factory parses a certified grammar on the core, as Parser.Parse does: the
// core's serve counter rises by one for the request, no decline is counted, and
// the tree equals the one Parser.Parse gives for the same bytes. The grammars
// are those that have a factory and a certificate in the generated list; the
// test finds them, so a grammar that is certified later is covered without a
// change here. (Markdown's fenced-code injection is not one of these routes:
// the highlighter pins every injected parser to the existing engine.)
func TestRegistryEntryPointsParseCertifiedGrammarsOnTheCore(t *testing.T) {
	var covered []string
	for _, entry := range AllLanguages() {
		if entry.TokenSourceFactory == nil || entry.Language == nil {
			continue
		}
		e := entry
		lang := e.Language()
		if !registryCoreCertified(lang) {
			t.Logf("%s: token-source factory, not certified", e.Name)
			continue
		}
		covered = append(covered, e.Name)
		inputs, ok := registryCoreInputs[e.Name]
		if !ok {
			t.Errorf("%s is certified and has a token-source factory: add a valid and an error input to registryCoreInputs", e.Name)
			continue
		}
		for i, text := range inputs {
			src := []byte(text)
			kind := [2]string{"valid", "error"}[i]
			t.Run(e.Name+"/"+kind, func(t *testing.T) {
				for _, route := range registryCoreRoutes() {
					t.Run(route.name, func(t *testing.T) {
						registryCoreCheck(t, &e, lang, src, kind, route)
					})
				}
			})
		}
	}
	t.Logf("certified grammars with a registry token-source factory: %v", covered)
	if len(covered) == 0 {
		t.Skip("no certified grammar has a registry token-source factory in this build")
	}
}

func registryCoreCheck(t *testing.T, e *LangEntry, lang *gotreesitter.Language, src []byte, kind string, route registryCoreRoute) {
	t.Helper()
	wantSrc := src
	if route.incr {
		wantSrc, _ = registryCoreEdit(src)
	}
	s0, _ := registryCoreServed()
	want, err := gotreesitter.NewParser(lang).Parse(wantSrc)
	if err != nil || want == nil {
		t.Fatalf("Parser.Parse: %v", err)
	}
	defer want.Release()
	if s1, _ := registryCoreServed(); s1-s0 != 1 {
		t.Fatalf("Parser.Parse was served %d times by the core, want 1", s1-s0)
	}
	if kind == "error" && !want.RootNode().HasError() {
		t.Fatalf("the error input parses without an error: %s", want.RootNode().SExpr(lang))
	}
	before, declinedBefore := registryCoreServed()
	registryCoreIncrServed = -1
	root, release := route.run(t, e, lang, src)
	defer release()
	after, declinedAfter := registryCoreServed()
	served := after - before
	if route.incr {
		served = registryCoreIncrServed
	}
	if served != 1 {
		t.Errorf("served %d times by the core, want 1", served)
	}
	if d := declinedAfter - declinedBefore; d != 0 {
		d1, _, _ := bind.RouteCounters()
		t.Errorf("the core declined %d requests (counters now %v), want 0", d, d1)
	}
	if root == nil {
		return
	}
	if got, w := registryCoreDigest(root, lang), registryCoreDigest(want.RootNode(), lang); got != w {
		t.Errorf("tree differs from Parser.Parse on the same bytes\n got: %s\nwant: %s", root.SExpr(lang), want.RootNode().SExpr(lang))
	}
}
