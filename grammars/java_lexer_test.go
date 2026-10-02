//go:build !grammar_subset || grammar_subset_java

package grammars

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewJavaTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewJavaTokenSource([]byte("class Main { int x; }\n"), lang); err == nil {
		t.Fatal("expected error for language missing java token symbols")
	}
}

func TestNewJavaTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewJavaTokenSourceOrEOF([]byte("class Main { int x; }\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestJavaTokenSourceSkipToByte(t *testing.T) {
	lang := JavaLanguage()
	src := []byte("class Main {\n  int x = 1;\n  int y = 2;\n}\n")
	target := bytes.Index(src, []byte("y"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}

	tok := ts.SkipToByte(uint32(target))
	if tok.Symbol == 0 {
		t.Fatal("SkipToByte unexpectedly returned EOF")
	}
	if int(tok.StartByte) < target {
		t.Fatalf("token starts before target offset: got %d, target %d", tok.StartByte, target)
	}
	if tok.Text != "y" {
		t.Fatalf("expected token text %q, got %q", "y", tok.Text)
	}
}

func TestJavaTokenSourceRebuildPreservesPendingStream(t *testing.T) {
	lang := JavaLanguage()
	ts, err := NewJavaTokenSource([]byte(`"old"`), lang)
	if err != nil {
		t.Fatal(err)
	}
	ts.Next()
	rebuilt, err := ts.RebuildTokenSource([]byte("class Fresh {}"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if token := rebuilt.Next(); token.Text != "class" || token.StartByte != 0 {
		t.Fatalf("rebuilt stream did not start fresh: %+v", token)
	}
	if token := ts.Next(); token.Text != "old" {
		t.Fatalf("rebuilding changed the pending original stream: %+v", token)
	}
	if _, err := ts.RebuildTokenSource(nil, &gotreesitter.Language{}); err == nil {
		t.Fatal("rebuilding ignored the supplied language")
	}
}

func TestJavaTokenSourceIncrementalReuseHasFreshVerifier(t *testing.T) {
	lang := JavaLanguage()
	source := append([]byte("class Main { int target = 1; "), bytes.Repeat([]byte("int x = 2; "), 160)...)
	source = append(source, '}')
	offset := bytes.Index(source, []byte("= 1")) + 2
	for _, candidate := range []bool{false, true} {
		parser := gotreesitter.NewParser(lang)
		parser.SetAdmissionCandidateRoute(candidate)
		old, err := parser.ParseWithTokenSource(source, NewJavaTokenSourceOrEOF(source, lang))
		if err != nil {
			t.Fatal(err)
		}
		for _, digit := range []byte{'3', ';', '1'} {
			edited := bytes.Clone(source)
			edited[offset] = digit
			old.Edit(gotreesitter.InputEdit{StartByte: uint32(offset), OldEndByte: uint32(offset + 1), NewEndByte: uint32(offset + 1),
				StartPoint: gotreesitter.Point{Column: uint32(offset)}, OldEndPoint: gotreesitter.Point{Column: uint32(offset + 1)}, NewEndPoint: gotreesitter.Point{Column: uint32(offset + 1)}})
			next, profile, err := parser.ParseIncrementalWithTokenSourceProfiled(edited, old, NewJavaTokenSourceOrEOF(edited, lang))
			old.Release()
			if err != nil {
				t.Fatal(err)
			}
			old = next
			fresh, err := gotreesitter.NewParser(lang).ParseWithTokenSource(edited, NewJavaTokenSourceOrEOF(edited, lang))
			if err != nil {
				t.Fatal(err)
			}
			if next.RootNode().HasError() != fresh.RootNode().HasError() || next.RootNode().EndByte() != uint32(len(edited)) || next.RootNode().SExpr(lang) != fresh.RootNode().SExpr(lang) {
				t.Fatal("incremental Java tree differs from fresh parsing")
			}
			fresh.Release()
			if digit == '3' && (profile.ReuseUnsupported || profile.ReusedSubtrees == 0) {
				t.Fatalf("candidate=%t declined Java reuse: %+v", candidate, profile)
			}
		}
		old.Release()
	}
}

func TestJavaTokenSourceZeroLongLiteralIsDecimal(t *testing.T) {
	lang := JavaLanguage()
	src := []byte("0L")
	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}
	want, ok := lang.SymbolByName("decimal_integer_literal")
	if !ok {
		t.Fatal("missing decimal_integer_literal symbol")
	}

	tok := ts.Next()
	if tok.Symbol != want {
		gotName := "<unknown>"
		if int(tok.Symbol) < len(lang.SymbolNames) {
			gotName = lang.SymbolNames[tok.Symbol]
		}
		t.Fatalf("token symbol = %d (%s), want %d (decimal_integer_literal)", tok.Symbol, gotName, want)
	}
	if tok.Text != "0L" {
		t.Fatalf("token text = %q, want %q", tok.Text, "0L")
	}
}

func TestParseJavaWithTokenSource(t *testing.T) {
	lang := JavaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("class Main { int x; }\n")
	ts, err := NewJavaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewJavaTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected java parse without syntax errors")
	}
}
