package bench_test

import (
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

func BenchmarkCPPConditionClauseAmbiguityDFA(b *testing.B) {
	cases := []struct {
		name string
		src  string
	}{
		{name: "while_assign", src: "int f() { while ((a = b)) {} }\n"},
		{name: "while_equal", src: "int f() { while ((a == b)) {} }\n"},
		{name: "if_assign", src: "int f() { if ((a = b)) {} }\n"},
		{name: "for_assign", src: "int f() { for (; (a = b); ) {} }\n"},
	}

	var entry grammars.LangEntry
	found := false
	for _, candidate := range grammars.AllLanguages() {
		if candidate.Name == "cpp" {
			entry = candidate
			found = true
			break
		}
	}
	if !found {
		b.Fatal("missing cpp language entry")
	}

	lang := entry.Language()
	for _, tc := range cases {
		tc := tc
		b.Run(tc.name, func(b *testing.B) {
			parser := gotreesitter.NewParser(lang)
			src := []byte(tc.src)

			tree, err := parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
			if err != nil {
				b.Fatalf("initial parse error: %v", err)
			}
			root := tree.RootNode()
			if root == nil {
				b.Fatal("initial parse returned nil root")
			}
			if root.HasError() {
				b.Fatalf("initial parse has error: %s", root.SExpr(lang))
			}
			tree.Release()

			b.ReportAllocs()
			b.SetBytes(int64(len(src)))
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				tree, err := parser.ParseWithTokenSource(src, entry.TokenSourceFactory(src, lang))
				if err != nil {
					b.Fatalf("parse error: %v", err)
				}
				if tree.RootNode() == nil {
					b.Fatal("parse returned nil root")
				}
				tree.Release()
			}
		})
	}
}
