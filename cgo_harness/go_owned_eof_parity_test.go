//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"testing"

	g "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammargen"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind/bindtest"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestGoOwnedLexerEOFAndConflictsMatchPinnedC(t *testing.T) {
	grammar := grammargen.GoGrammar()
	grammar.ExplicitLexerEOF = true
	grammar.ShiftActionsLast = true
	grammar.Externals = nil
	var inline func(*grammargen.Rule) *grammargen.Rule
	inline = func(rule *grammargen.Rule) *grammargen.Rule {
		if rule == nil {
			return nil
		}
		if rule.Kind == grammargen.RuleSymbol && rule.Value == "_automatic_semicolon" {
			return grammargen.Choice(grammargen.Pat(`\n`), grammargen.Str("\x00"))
		}
		out := *rule
		out.Children = make([]*grammargen.Rule, len(rule.Children))
		for i, child := range rule.Children {
			out.Children[i] = inline(child)
		}
		return &out
	}
	for name, rule := range grammar.Rules {
		grammar.Rules[name] = inline(rule)
	}
	_, blob, err := grammargen.GenerateLanguageAndBlob(grammar)
	if err != nil {
		t.Fatal(err)
	}
	lang, err := g.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	cLang, err := ParityCLanguage("go")
	if err != nil {
		t.Fatal(err)
	}

	for _, source := range []string{
		"",
		"package p",
		"package p\n",
		"package p;",
		"package p // end\n",
		"package p\x00",
		"package p\nfunc f(){}\n",
		"package p\nfunc f(){return}\n",
		"package p\nfunc f() (bool, bool) { return }\n",
		"package p\nfunc f(){ _ = a[b]() }\n",
		"package p\nfunc f(){ _ = a[b] }\n",
		"package p\nfunc f(){ _ = a[0] }\n",
		"package p\nfunc f(){ _ = a[b, c] }\n",
		"package p\nfunc f() (a, b T) { return }\n",
		"package p\nfunc f(){ _ = a[0].b }\n",
		"package p\nfunc f() (A, B) { return }\n",
	} {
		parser, err := bindtest.NewParser(lang)
		if err != nil {
			t.Fatal(err)
		}
		parser.SetStopAfter(360, false)
		data := []byte(source)
		ct := compactT3ParseC(t, cLang, data)
		want := ccoreCDigestChanges(ct, false)
		ct.Close()
		stop := parser.Parse(data, nil)
		got := parser.Digest()
		parser.ReleaseLast()
		if stop != "" || got != want {
			t.Errorf("source %q stop=%q, digest=%s want %s", source, stop, got, want)
		}
	}

	cp := sitter.NewParser()
	defer cp.Close()
	if err := cp.SetLanguage(cLang); err != nil {
		t.Fatal(err)
	}
	const source = "package p"
	const end = uint32(len(source))
	for _, ending := range []byte{'\n', 0} {
		parser, err := bindtest.NewParser(lang)
		if err != nil {
			t.Fatal(err)
		}
		parser.SetStopAfter(360, false)
		if stop := parser.Parse([]byte(source), nil); stop != "" {
			t.Fatalf("initial parse stopped: %s", stop)
		}
		old := parser.Tree()
		ct := cp.Parse([]byte(source), nil)
		if ct == nil {
			parser.Release(old)
			t.Fatal("initial C parse returned no tree")
		}
		col, row := uint32(len(source)+1), uint32(0)
		if ending == '\n' {
			col, row = 0, 1
		}
		edit := bindtest.Edit{
			StartByte: end, OldEndByte: end, NewEndByte: end + 1,
			StartCol: end, OldEndCol: end, NewEndRow: row, NewEndCol: col,
		}
		parser.EditInPlace(old, edit)
		ct.Edit(ccoreSitterEdit(&edit))
		data := append([]byte(source), ending)
		cnext := cp.Parse(data, ct)
		ct.Close()
		if cnext == nil {
			parser.Release(old)
			t.Fatal("incremental C parse returned no tree")
		}
		want := ccoreCDigestChanges(cnext, false)
		cnext.Close()
		_, stop := parser.ParseIncremental(data, old, nil)
		got := parser.Digest()
		parser.ReleaseLast()
		parser.Release(old)
		if stop != "" || got != want {
			t.Errorf("append %q stop=%q, digest=%s want %s", ending, stop, got, want)
		}
	}
}
