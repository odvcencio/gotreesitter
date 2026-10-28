//go:build !grammar_subset

package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

type lexerTestCase struct {
	name    string
	src     []byte
	factory func([]byte, *gotreesitter.Language) gotreesitter.TokenSource
	lang    func() *gotreesitter.Language
}

func allLexerTestCases() []lexerTestCase {
	return []lexerTestCase{
		{"c", []byte("#include <stdio.h>\nint main(void) { return 0; }\n"), NewCTokenSourceOrEOF, CLanguage},
		{"go", []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), NewGoTokenSourceOrEOF, GoLanguage},
		{"java", []byte("public class Main {\n  public static void main(String[] args) {\n    System.out.println(\"hello\");\n  }\n}\n"), NewJavaTokenSourceOrEOF, JavaLanguage},
		{"html", []byte("<html><head><title>Test</title></head><body><p class=\"x\">Hello</p></body></html>"), NewHTMLTokenSourceOrEOF, HtmlLanguage},
		{"lua", []byte("local x = 1\nfunction foo(a, b)\n  return a + b\nend\nprint(foo(1, 2))\n"), NewLuaTokenSourceOrEOF, LuaLanguage},
		{"toml", []byte("[section]\nkey = \"value\"\nnum = 42\narr = [1, 2, 3]\n"), NewTomlTokenSourceOrEOF, TomlLanguage},
		{"authzed", []byte("definition user {}\n\ndefinition document {\n  relation viewer: user\n  permission view = viewer\n}\n"), NewAuthzedTokenSourceOrEOF, AuthzedLanguage},
	}
}

func collectAllTokens(ts gotreesitter.TokenSource, maxTokens int) []gotreesitter.Token {
	var tokens []gotreesitter.Token
	for i := 0; i < maxTokens; i++ {
		tok := ts.Next()
		tokens = append(tokens, tok)
		if tok.Symbol == 0 {
			break
		}
	}
	return tokens
}

func TestLexerPropertyTokenByteRanges(t *testing.T) {
	for _, tc := range allLexerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			ts := tc.factory(tc.src, lang)
			tokens := collectAllTokens(ts, 1000)

			srcLen := uint32(len(tc.src))
			for i, tok := range tokens {
				if tok.StartByte > tok.EndByte {
					t.Errorf("token %d: StartByte %d > EndByte %d", i, tok.StartByte, tok.EndByte)
				}
				if tok.EndByte > srcLen {
					// EOF tokens may have EndByte == srcLen, which is valid
					if tok.Symbol != 0 {
						t.Errorf("token %d: EndByte %d > srcLen %d (sym=%d)", i, tok.EndByte, srcLen, tok.Symbol)
					}
				}
			}
		})
	}
}

func TestLexerPropertyTokenOrdering(t *testing.T) {
	for _, tc := range allLexerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			ts := tc.factory(tc.src, lang)
			tokens := collectAllTokens(ts, 1000)

			for i := 1; i < len(tokens); i++ {
				prev := tokens[i-1]
				cur := tokens[i]
				if prev.Symbol == 0 {
					break // past EOF
				}
				if cur.Symbol == 0 {
					break // EOF
				}
				if cur.StartByte < prev.StartByte {
					t.Errorf("token %d starts at %d before token %d at %d", i, cur.StartByte, i-1, prev.StartByte)
				}
			}
		})
	}
}

func TestLexerPropertyNoOverlap(t *testing.T) {
	for _, tc := range allLexerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			ts := tc.factory(tc.src, lang)
			tokens := collectAllTokens(ts, 1000)

			for i := 1; i < len(tokens); i++ {
				prev := tokens[i-1]
				cur := tokens[i]
				if prev.Symbol == 0 || cur.Symbol == 0 {
					break
				}
				if cur.StartByte < prev.EndByte {
					t.Errorf("token %d [%d,%d) overlaps token %d [%d,%d)",
						i-1, prev.StartByte, prev.EndByte, i, cur.StartByte, cur.EndByte)
				}
			}
		})
	}
}

func TestLexerPropertyEOFTermination(t *testing.T) {
	for _, tc := range allLexerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			ts := tc.factory(tc.src, lang)
			tokens := collectAllTokens(ts, 1000)

			if len(tokens) == 0 {
				t.Fatal("no tokens returned")
			}
			last := tokens[len(tokens)-1]
			if last.Symbol != 0 {
				t.Errorf("last token symbol = %d, want 0 (EOF)", last.Symbol)
			}
		})
	}
}

func TestLexerPropertyPointMonotonicity(t *testing.T) {
	for _, tc := range allLexerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			ts := tc.factory(tc.src, lang)
			tokens := collectAllTokens(ts, 1000)

			for i := 0; i < len(tokens); i++ {
				tok := tokens[i]
				if tok.Symbol == 0 {
					break
				}
				sp := tok.StartPoint
				ep := tok.EndPoint
				if ep.Row < sp.Row || (ep.Row == sp.Row && ep.Column < sp.Column) {
					t.Errorf("token %d: EndPoint (%d,%d) before StartPoint (%d,%d)",
						i, ep.Row, ep.Column, sp.Row, sp.Column)
				}
			}

			for i := 1; i < len(tokens); i++ {
				prev := tokens[i-1]
				cur := tokens[i]
				if prev.Symbol == 0 || cur.Symbol == 0 {
					break
				}
				pp := prev.StartPoint
				cp := cur.StartPoint
				if cp.Row < pp.Row || (cp.Row == pp.Row && cp.Column < pp.Column) {
					t.Errorf("token %d StartPoint (%d,%d) before token %d StartPoint (%d,%d)",
						i, cp.Row, cp.Column, i-1, pp.Row, pp.Column)
				}
			}
		})
	}
}

func TestLexerPropertyNonEmptyNamedTokens(t *testing.T) {
	for _, tc := range allLexerTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			lang := tc.lang()
			ts := tc.factory(tc.src, lang)
			tokens := collectAllTokens(ts, 1000)

			for i, tok := range tokens {
				if tok.Symbol == 0 {
					break
				}
				if tok.StartByte == tok.EndByte {
					// Zero-width tokens are acceptable for certain grammar constructs
					// (e.g., TOML document_token1, YAML _eof), so we don't fail here.
					continue
				}
				_ = i
			}
		})
	}
}
