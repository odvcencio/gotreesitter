//go:build !grammar_subset || grammar_subset_json

package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewJSONTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewJSONTokenSource([]byte(`{"a":1}`), lang); err == nil {
		t.Fatal("expected error for language missing json token symbols")
	}
}

func TestNewJSONTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewJSONTokenSourceOrEOF([]byte(`{"a":1}`), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestJSONTokenSourceSplitsStringEscapes(t *testing.T) {
	lang := JsonLanguage()
	src := []byte(`{"a":"x\n\u0041"}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	var sawContent, sawEscape bool
	for i := 0; i < 64; i++ {
		tok := ts.Next()
		if tok.Symbol == 0 {
			break
		}
		typ := lang.SymbolNames[tok.Symbol]
		if typ == "string_content" {
			sawContent = true
		}
		if typ == "escape_sequence" {
			sawEscape = true
		}
	}

	if !sawContent {
		t.Fatal("expected at least one string_content token")
	}
	if !sawEscape {
		t.Fatal("expected at least one escape_sequence token")
	}
}

func TestJSONTokenSourceSkipToByte(t *testing.T) {
	lang := JsonLanguage()
	src := []byte(`{"a":1, "target": 2}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	target := uint32(8) // points near "target"
	tok := ts.SkipToByte(target)
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if tok.StartByte < target {
		t.Fatalf("token starts before target: got %d, target %d", tok.StartByte, target)
	}
}

func TestJSONTokenSourceSkipToByteInsideStringContent(t *testing.T) {
	lang := JsonLanguage()
	src := []byte(`{"version": "0.24.8", "target": "x"}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	target := bytes.Index(src, []byte("24.8"))
	if target < 0 {
		t.Fatal("test source missing target string")
	}
	tok := ts.SkipToByte(uint32(target))
	if got := lang.SymbolNames[tok.Symbol]; got != "string_content" {
		t.Fatalf("SkipToByte token = %q, want string_content; token=%+v", got, tok)
	}
	if got, want := tok.StartByte, uint32(target); got != want {
		t.Fatalf("StartByte=%d, want %d", got, want)
	}
	if got := tok.Text; got != "24.8" {
		t.Fatalf("Text=%q, want %q", got, "24.8")
	}
	next := ts.Next()
	if got := lang.SymbolNames[next.Symbol]; got != "\"" {
		t.Fatalf("next token after clipped string content = %q, want quote; token=%+v", got, next)
	}
	if next.StartByte <= tok.StartByte {
		t.Fatalf("next token did not advance after clipped token: current=%+v next=%+v", tok, next)
	}
	if _, ok := any(ts).(gotreesitter.PointSkippableTokenSource); !ok {
		t.Fatal("JSONTokenSource should implement PointSkippableTokenSource")
	}
}

func TestParseJSONWithTokenSource(t *testing.T) {
	lang := JsonLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte(`{"a":[1,true,null,false,"x\n"],"b":{"c":2}}`)
	ts, err := NewJSONTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJSONTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected json parse without syntax errors")
	}
}

func TestParseJSONIncrementalWithTokenSourceStringEdit(t *testing.T) {
	lang := JsonLanguage()
	parser := gotreesitter.NewParser(lang)
	oldSrc := []byte(`{"version": "0.24.8", "target": "x"}`)
	newSrc := append([]byte(nil), oldSrc...)
	editAt := bytes.IndexByte(newSrc, '0')
	if editAt < 0 {
		t.Fatal("test source missing edit byte")
	}
	newSrc[editAt] = '1'

	oldTree, err := parser.ParseWithTokenSource(oldSrc, NewJSONTokenSourceOrEOF(oldSrc, lang))
	if err != nil {
		t.Fatalf("old parse failed: %v", err)
	}
	defer oldTree.Release()
	freshTree, err := parser.ParseWithTokenSource(newSrc, NewJSONTokenSourceOrEOF(newSrc, lang))
	if err != nil {
		t.Fatalf("fresh parse failed: %v", err)
	}
	defer freshTree.Release()

	edit := gotreesitter.InputEdit{
		StartByte:   uint32(editAt),
		OldEndByte:  uint32(editAt + 1),
		NewEndByte:  uint32(editAt + 1),
		StartPoint:  pointAtByte(oldSrc, editAt),
		OldEndPoint: pointAtByte(oldSrc, editAt+1),
		NewEndPoint: pointAtByte(newSrc, editAt+1),
	}
	oldTree.Edit(edit)

	incrTree, err := parser.ParseIncrementalWithTokenSource(newSrc, oldTree, NewJSONTokenSourceOrEOF(newSrc, lang))
	if err != nil {
		t.Fatalf("incremental parse failed: %v", err)
	}
	defer incrTree.Release()

	if incrTree.RootNode().HasError() {
		t.Fatalf("incremental parse has error: %s", incrTree.RootNode().SExpr(lang))
	}
	if got, want := incrTree.RootNode().SExpr(lang), freshTree.RootNode().SExpr(lang); got != want {
		t.Fatalf("incremental SExpr mismatch\n got: %s\nwant: %s", got, want)
	}
}
