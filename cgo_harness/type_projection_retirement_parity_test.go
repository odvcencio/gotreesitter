//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

func TestTypeProjectionRetirementCOracleParity(t *testing.T) {
	tests := []parityCase{
		{
			name:   "arduino",
			source: "int f(void) { char c = 0; return c; }\n",
		},
		{
			name:   "objc",
			source: "@interface CallbackClient : NSObject <ClientProtocol>\n@end\n",
		},
		{
			name: "objc",
			source: "@interface T : NSObject\n" +
				"- (NSUInteger)count;\n" +
				"+ (NSArray*)items;\n" +
				"- (void)reset;\n" +
				"@end\n",
		},
		{
			name: "objc",
			source: "@interface T : NSObject\n" +
				"+ (NSArray*)items;\n" +
				"- (NSUInteger)count;\n" +
				"- (void)reset;\n" +
				"@end\n",
		},
		{
			name:   "objc",
			source: "void f(){int a=sizeof(CustomType);int b=sizeof(int);}\n",
		},
		{
			name:   "hyprlang",
			source: "resize_on_border = true\nname = myBezier\n",
		},
		{
			// The word token's own pattern (`token(prec(-1, /[^\n,#]+|.*##.*/))`)
			// absorbs leading whitespace via maximal munch before the DFA even
			// asks whether the captured text is a keyword, so the keyword
			// re-lex must skip a leading run of extras — not just match at
			// byte zero — the same way tree-sitter's own generated keyword
			// lexer SKIPs them.
			name:   "hyprlang",
			source: "a =   true\nb = false\nc = on\nd = off\ne = yes\nf = no\n",
		},
		{
			// Trailing content after the keyword (a space before the
			// linebreak, or extra identifier characters) must NOT promote:
			// the captured span has to be exactly "skippable prefix + one
			// full keyword literal", nothing left over.
			name:   "hyprlang",
			source: "a = true \nb = truex\nc = xtrue\n",
		},
		{
			name:   "hyprlang",
			source: "a = true,b\n",
		},
	}
	for index, test := range tests {
		test := test
		t.Run(fmt.Sprintf("%s_%d", test.name, index), func(t *testing.T) {
			scheduleParityMemoryScavenge(t)
			source := []byte(test.source)
			entry, ok := parityEntriesByName[test.name]
			if !ok {
				t.Fatalf("missing grammar entry for %s", test.name)
			}
			language := entry.Language()
			goTree, err := gotreesitter.NewParser(language).
				ParseNoResultCompatibilityBenchmarkOnly(source)
			if err != nil {
				t.Fatal(err)
			}
			defer goTree.Release()

			cLanguage, err := ParityCLanguage(test.name)
			if err != nil {
				t.Fatal(err)
			}
			cParser := sitter.NewParser()
			defer cParser.Close()
			if err := cParser.SetLanguage(cLanguage); err != nil {
				t.Fatal(err)
			}
			cTree := cParser.Parse(source, nil)
			if cTree == nil || cTree.RootNode() == nil {
				t.Fatal("C oracle returned a nil tree")
			}
			defer cTree.Close()

			var divergences []string
			compareNodes(
				goTree.RootNode(),
				language,
				cTree.RootNode(),
				"root",
				&divergences,
			)
			if len(divergences) != 0 {
				t.Fatalf("C-oracle divergences: %v", divergences)
			}
		})
	}
}
