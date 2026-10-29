//go:build !grammar_subset

package grammars

import (
	"testing"

	"github.com/odvcencio/gotreesitter"
)

func benchmarkTokenize(b *testing.B, src []byte, factory func([]byte, *gotreesitter.Language) gotreesitter.TokenSource, langFn func() *gotreesitter.Language) {
	lang := langFn()
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := factory(src, lang)
		for {
			tok := ts.Next()
			if tok.Symbol == 0 {
				break
			}
		}
	}
}

func benchmarkParse(b *testing.B, src []byte, factory func([]byte, *gotreesitter.Language) gotreesitter.TokenSource, langFn func() *gotreesitter.Language) {
	lang := langFn()
	parser := gotreesitter.NewParser(lang)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := factory(src, lang)
		tree, err := parser.ParseWithTokenSource(src, ts)
		if err != nil {
			b.Fatalf("parse failed: %v", err)
		}
		if tree != nil {
			tree.Release()
		}
	}
}

func benchmarkSkipToByte(b *testing.B, src []byte, factory func([]byte, *gotreesitter.Language) gotreesitter.TokenSource, langFn func() *gotreesitter.Language) {
	lang := langFn()
	mid := uint32(len(src) / 2)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := factory(src, lang)
		if skipper, ok := ts.(gotreesitter.ByteSkippableTokenSource); ok {
			skipper.SkipToByte(mid)
		} else {
			b.Skip("lexer does not implement ByteSkippableTokenSource")
		}
	}
}

// --- C ---
func BenchmarkTokenize_C(b *testing.B) {
	src := []byte("#include <stdio.h>\n#define MAX 100\nint main(void) { int x = MAX; printf(\"%d\\n\", x); return 0; }\n")
	benchmarkTokenize(b, src, NewCTokenSourceOrEOF, CLanguage)
}

func BenchmarkParse_C(b *testing.B) {
	src := []byte("#include <stdio.h>\n#define MAX 100\nint main(void) { int x = MAX; printf(\"%d\\n\", x); return 0; }\n")
	benchmarkParse(b, src, NewCTokenSourceOrEOF, CLanguage)
}

func BenchmarkSkipToByte_C(b *testing.B) {
	src := []byte("#include <stdio.h>\n#define MAX 100\nint main(void) { int x = MAX; printf(\"%d\\n\", x); return 0; }\n")
	benchmarkSkipToByte(b, src, NewCTokenSourceOrEOF, CLanguage)
}

// --- Go ---
func BenchmarkTokenize_Go(b *testing.B) {
	src := []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
	benchmarkTokenize(b, src, NewGoTokenSourceOrEOF, GoLanguage)
}

func BenchmarkParse_Go(b *testing.B) {
	src := []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
	benchmarkParse(b, src, NewGoTokenSourceOrEOF, GoLanguage)
}

func BenchmarkSkipToByte_Go(b *testing.B) {
	src := []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
	benchmarkSkipToByte(b, src, NewGoTokenSourceOrEOF, GoLanguage)
}

// --- Java ---
func BenchmarkTokenize_Java(b *testing.B) {
	src := []byte("public class Main {\n  public static void main(String[] args) {\n    System.out.println(\"hello\");\n  }\n}\n")
	benchmarkTokenize(b, src, NewJavaTokenSourceOrEOF, JavaLanguage)
}

func BenchmarkParse_Java(b *testing.B) {
	src := []byte("public class Main {\n  public static void main(String[] args) {\n    System.out.println(\"hello\");\n  }\n}\n")
	benchmarkParse(b, src, NewJavaTokenSourceOrEOF, JavaLanguage)
}

// --- HTML ---
func BenchmarkTokenize_HTML(b *testing.B) {
	src := []byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>")
	benchmarkTokenize(b, src, NewHTMLTokenSourceOrEOF, HtmlLanguage)
}

func BenchmarkParse_HTML(b *testing.B) {
	src := []byte("<html><head><title>Test</title></head><body><p>Hello</p></body></html>")
	benchmarkParse(b, src, NewHTMLTokenSourceOrEOF, HtmlLanguage)
}

// --- Lua ---
func BenchmarkTokenize_Lua(b *testing.B) {
	src := []byte("local x = 1\nfunction foo(a, b)\n  return a + b\nend\nprint(foo(1, 2))\n")
	benchmarkTokenize(b, src, NewLuaTokenSourceOrEOF, LuaLanguage)
}

func BenchmarkParse_Lua(b *testing.B) {
	src := []byte("local x = 1\nfunction foo(a, b)\n  return a + b\nend\nprint(foo(1, 2))\n")
	benchmarkParse(b, src, NewLuaTokenSourceOrEOF, LuaLanguage)
}

// --- YAML (uses DFA+ExternalScanner, not TokenSource) ---
func BenchmarkParse_YAML(b *testing.B) {
	src := []byte("key: value\nlist:\n  - one\n  - two\nnested:\n  a: 1\n  b: true\n")
	lang := YamlLanguage()
	parser := gotreesitter.NewParser(lang)
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tree, err := parser.Parse(src)
		if err != nil {
			b.Fatal(err)
		}
		tree.Release()
	}
}

// --- TOML ---
func BenchmarkTokenize_TOML(b *testing.B) {
	src := []byte("[section]\nkey = \"value\"\nnum = 42\narr = [1, 2, 3]\n")
	benchmarkTokenize(b, src, NewTomlTokenSourceOrEOF, TomlLanguage)
}

func BenchmarkParse_TOML(b *testing.B) {
	src := []byte("[section]\nkey = \"value\"\nnum = 42\narr = [1, 2, 3]\n")
	benchmarkParse(b, src, NewTomlTokenSourceOrEOF, TomlLanguage)
}

// --- Authzed ---
func BenchmarkTokenize_Authzed(b *testing.B) {
	src := []byte("definition user {}\n\ndefinition document {\n  relation viewer: user\n  permission view = viewer\n}\n")
	benchmarkTokenize(b, src, NewAuthzedTokenSourceOrEOF, AuthzedLanguage)
}

func BenchmarkParse_Authzed(b *testing.B) {
	src := []byte("definition user {}\n\ndefinition document {\n  relation viewer: user\n  permission view = viewer\n}\n")
	benchmarkParse(b, src, NewAuthzedTokenSourceOrEOF, AuthzedLanguage)
}
