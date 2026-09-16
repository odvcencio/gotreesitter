//go:build !grammar_subset || grammar_subset_lua

package grammarruntime

import (
	"bytes"
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func TestNewLuaTokenSourceReturnsErrorOnMissingSymbols(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	if _, err := NewLuaTokenSource([]byte("local x = 1\n"), lang); err == nil {
		t.Fatal("expected error for language missing lua token symbols")
	}
}

func TestNewLuaTokenSourceOrEOFFallsBack(t *testing.T) {
	lang := &gotreesitter.Language{
		TokenCount:  1,
		SymbolNames: []string{"end"},
	}
	ts := NewLuaTokenSourceOrEOF([]byte("local x = 1\n"), lang)
	tok := ts.Next()
	if tok.Symbol != 0 {
		t.Fatalf("fallback token symbol = %d, want EOF (0)", tok.Symbol)
	}
}

func TestLuaTokenSourceSkipToByte(t *testing.T) {
	lang := LuaLanguage()
	src := []byte("local x = 1\nlocal y = 2\n")
	target := bytes.Index(src, []byte("y"))
	if target < 0 {
		t.Fatal("missing target marker")
	}

	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
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

func TestParseLuaWithTokenSource(t *testing.T) {
	lang := LuaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("local x = 1\n")
	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected lua parse without syntax errors")
	}
}

func TestParseLuaWithTokenSourceAfterLineComment(t *testing.T) {
	lang := LuaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("local function func_one() end\n--             ^ definition.function\nfunc_one()\n")
	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected lua parse without syntax errors")
	}
}

func TestParseLuaWithTokenSourceLeadingCommentThenAssignment(t *testing.T) {
	lang := LuaLanguage()
	parser := gotreesitter.NewParser(lang)
	src := []byte("-- stylua: ignore start\n\nlocal _\n\n_ = \"x\"\n")
	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
	}

	tree, err := parser.ParseWithTokenSource(src, ts)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parse returned nil root")
	}
	if tree.RootNode().HasError() {
		t.Fatal("expected lua parse without syntax errors")
	}
}

func TestParseLuaWithTokenSourceBlockStrings(t *testing.T) {
	lang := LuaLanguage()
	parser := gotreesitter.NewParser(lang)
	cases := []struct {
		name string
		src  []byte
	}{
		{name: "double_bracket", src: []byte("_ = [[x]]\n")},
		{name: "equals_bracket", src: []byte("_ = [=[x]=]\n")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts, err := NewLuaTokenSource(tc.src, lang)
			if err != nil {
				t.Fatalf("NewLuaTokenSource failed: %v", err)
			}

			tree, err := parser.ParseWithTokenSource(tc.src, ts)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("parse returned nil root")
			}
			if tree.RootNode().HasError() {
				t.Fatal("expected lua parse without syntax errors")
			}
		})
	}
}

func TestLuaTokenSourceEscapeZConsumesFollowingWhitespace(t *testing.T) {
	lang := LuaLanguage()
	src := []byte("_ = \"x\\z\n    y\"\n")
	ts, err := NewLuaTokenSource(src, lang)
	if err != nil {
		t.Fatalf("NewLuaTokenSource failed: %v", err)
	}

	found := false
	for {
		tok := ts.Next()
		if tok.Symbol == 0 {
			break
		}
		if tok.Symbol != ts.escapeSymbol {
			continue
		}
		if got, want := tok.Text, "\\z\n    "; got != want {
			t.Fatalf("escape token text = %q, want %q", got, want)
		}
		found = true
	}
	if !found {
		t.Fatal("expected to find escape_sequence token")
	}
}
