package grammars

import (
	"fmt"
	"strings"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

func retryBudgetJavaCorpus(classCount int) []byte {
	var b strings.Builder
	b.Grow(classCount * 64)
	for i := 0; i < classCount; i++ {
		fmt.Fprintf(&b, "class C%d { int f%d() { return %d; } }\n", i, i, i)
	}
	return []byte(b.String())
}

func retryBudgetLuaCorpus(lineCount int) []byte {
	var b strings.Builder
	b.Grow(lineCount * 32)
	for i := 0; i < lineCount; i++ {
		fmt.Fprintf(&b, "local x%d = %d\n", i, i)
	}
	return []byte(b.String())
}

func TestParseWithTokenSourceRetriesNodeLimitForJavaAndLua(t *testing.T) {
	t.Setenv("GOT_PARSE_NODE_LIMIT_SCALE", "")
	gotreesitter.ResetParseEnvConfigCacheForTests()

	for _, tc := range []struct {
		name string
		src  []byte
	}{
		{name: "java", src: retryBudgetJavaCorpus(224)},
		{name: "lua", src: retryBudgetLuaCorpus(224)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := lookupByName(tc.name)
			if entry == nil {
				t.Fatalf("missing registry entry for %q", tc.name)
			}
			if entry.TokenSourceFactory == nil {
				t.Fatalf("%q does not use a token source backend", tc.name)
			}

			lang := entry.Language()
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.ParseWithTokenSource(tc.src, entry.TokenSourceFactory(tc.src, lang))
			if err != nil {
				t.Fatalf("ParseWithTokenSource() error = %v", err)
			}
			if tree == nil || tree.RootNode() == nil {
				t.Fatal("ParseWithTokenSource() returned nil tree/root")
			}

			rt := tree.ParseRuntime()
			root := tree.RootNode()
			if rt.StopReason != gotreesitter.ParseStopAccepted {
				t.Fatalf("StopReason = %q, want %q", rt.StopReason, gotreesitter.ParseStopAccepted)
			}
			if rt.Truncated {
				t.Fatal("Truncated = true, want false")
			}
			if rt.TokenSourceEOFEarly {
				t.Fatal("TokenSourceEOFEarly = true, want false")
			}
			if root.HasError() {
				t.Fatalf("root.HasError() = true, summary=%s", rt.Summary())
			}
			if got, want := int(root.EndByte()), len(tc.src); got != want {
				t.Fatalf("root.EndByte() = %d, want %d (summary=%s)", got, want, rt.Summary())
			}
			if rt.NodeLimit <= 0 {
				t.Fatalf("NodeLimit = %d, want > 0", rt.NodeLimit)
			}
		})
	}
}
