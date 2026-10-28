package parse_test

import (
	"os"
	"strings"
	"testing"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestForestGapRejection locks the fix for tree-sitter's binary repeat
// (`X_repeat1 -> X_repeat1 X_repeat1`). The forest forks on every grouping of a
// statement list; some binary merges combine two halves with a DROPPED statement
// in the hole between them, and because the (symbol, start, end) dedup ignores
// internal coverage, a gapped grouping could win and silently drop a statement
// (lua small__functions.lua dropped `local func_two = function() end` and
// `tbl.func_one()`). The reduce now rejects any reduction whose children leave a
// non-trivia hole. These corpora dispatch with NO non-trivia inter-child gap;
// bash/python regression-guard that line-continuation trivia is still accepted.
func TestForestGapRejection(t *testing.T) {
	cases := []struct {
		name string
		path string
		lang func() *gts.Language
	}{
		{"lua_functions", "../../cgo_harness/corpus_real/lua/small__functions.lua", grammars.LuaLanguage},
		{"bash_clean", "../../cgo_harness/corpus_real/bash/medium__clean-old.sh", grammars.BashLanguage},
		{"python_grammar", "../../cgo_harness/corpus_real/python/large__python3.8_grammar.py", grammars.PythonLanguage},
	}
	for _, c := range cases {
		src, err := os.ReadFile(c.path)
		if err != nil {
			t.Logf("%s: corpus absent, skipping", c.name)
			continue
		}
		lang := c.lang()
		tr, ok := gts.NewParser(lang).ParseForestExperimental(src)
		if !ok {
			t.Errorf("%s: forest DECLINED — the binary-repeat statement drop should now resolve to a gap-free grouping", c.name)
			continue
		}
		r := tr.RootNode()
		prev := r.StartByte()
		for i := 0; i < int(r.ChildCount()); i++ {
			ch := r.Child(i)
			if ch.StartByte() > prev && strings.TrimSpace(string(src[prev:ch.StartByte()])) != "" {
				t.Errorf("%s: dispatched a tree with a non-trivia gap before child[%d] %s (bytes %d-%d) — a statement was dropped",
					c.name, i, ch.Type(lang), prev, ch.StartByte())
				break
			}
			if ch.EndByte() > prev {
				prev = ch.EndByte()
			}
		}
		tr.Release()
	}
}
