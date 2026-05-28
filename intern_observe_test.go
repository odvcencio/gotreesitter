package gotreesitter_test

import (
	"os"
	"testing"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestInternLeafObservationMatrix walks several real-corpus files
// across languages and reports the leaf interning hit rate for each.
// The output of this test (`go test -v`) is the measurement artifact
// that informs whether Phase 3 should actually flip from observation
// to canonical-substitution.
func TestInternLeafObservationMatrix(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		langKey string
	}{
		{"javascript", "cgo_harness/corpus_real/javascript/large__text-editor-component.js", "javascript"},
		{"javascript-small", "cgo_harness/corpus_real/javascript/small__functions.js", "javascript"},
		{"c-large", "cgo_harness/corpus_real/c/large__cluster.c", "c"},
		{"python-large", "cgo_harness/corpus_real/python/large__python3.8_grammar.py", "python"},
	}

	gotreesitter.SetInternLeavesObserveEnabled(true)
	t.Cleanup(func() { gotreesitter.SetInternLeavesObserveEnabled(false) })

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(tc.path)
			if err != nil {
				t.Skipf("corpus not present: %v", err)
			}
			lang := languageForName(t, tc.langKey)
			parser := gotreesitter.NewParser(lang)
			tree, err := parser.Parse(src)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			stats := gotreesitter.InternStatsFor(tree.RootNode())
			if stats.LeafLookups == 0 {
				t.Errorf("zero leaf lookups — observation hook not wired")
				return
			}
			blindRate := float64(stats.LeafHits) / float64(stats.LeafLookups) * 100
			t.Logf("%s blind: %.1f%% hit rate (%d hits / %d lookups, %d unique)",
				tc.name, blindRate, stats.LeafHits, stats.LeafLookups, stats.LeafStores)

			// Phase 3: parseState-aware measurement at the shift path.
			// A hit here is a leaf whose entire shape AND per-stack state
			// matches a prior leaf — i.e. truly safe to canonically
			// substitute.
			if stats.FullLookups > 0 {
				fullRate := float64(stats.FullHits) / float64(stats.FullLookups) * 100
				t.Logf("  state-aware: %.1f%% hit rate (%d hits / %d shift lookups)",
					fullRate, stats.FullHits, stats.FullLookups)
			}
		})
	}
}

func languageForName(t *testing.T, name string) *gotreesitter.Language {
	t.Helper()
	switch name {
	case "javascript":
		return grammars.JavascriptLanguage()
	case "c":
		return grammars.CLanguage()
	case "python":
		return grammars.PythonLanguage()
	}
	t.Fatalf("unknown language: %s", name)
	return nil
}

// TestInternLeafSubstitutionParityMatrix asserts that enabling Phase 3
// canonical substitution produces the same parse tree as the baseline
// across multiple languages and corpus files. Parses each file twice
// (substitution off then on), then walks both trees in parallel
// asserting that every (type, span, childCount, named, error) tuple
// matches. If substitution were breaking parser correctness (clobbered
// parseState, wrong child link, divergent flags), this test would
// diverge.
func TestInternLeafSubstitutionParityMatrix(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		langKey string
	}{
		{"javascript-large", "cgo_harness/corpus_real/javascript/large__text-editor-component.js", "javascript"},
		{"javascript-small", "cgo_harness/corpus_real/javascript/small__functions.js", "javascript"},
		{"c-large", "cgo_harness/corpus_real/c/large__cluster.c", "c"},
		{"python-large", "cgo_harness/corpus_real/python/large__python3.8_grammar.py", "python"},
	}
	t.Cleanup(func() {
		gotreesitter.SetInternLeavesSubstituteEnabled(false)
		gotreesitter.SetInternLeavesObserveEnabled(false)
	})

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src, err := os.ReadFile(tc.path)
			if err != nil {
				t.Skipf("corpus not present: %v", err)
			}
			lang := languageForName(t, tc.langKey)

			gotreesitter.SetInternLeavesSubstituteEnabled(false)
			gotreesitter.SetInternLeavesObserveEnabled(false)
			baselineTree, err := gotreesitter.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("baseline parse: %v", err)
			}

			gotreesitter.SetInternLeavesSubstituteEnabled(true)
			internedTree, err := gotreesitter.NewParser(lang).Parse(src)
			if err != nil {
				t.Fatalf("interned parse: %v", err)
			}

			stats := gotreesitter.InternStatsFor(internedTree.RootNode())
			if stats.FullLookups > 0 {
				t.Logf("%s substitution: %.1f%% hit rate (%d hits / %d lookups)",
					tc.name,
					float64(stats.FullHits)/float64(stats.FullLookups)*100,
					stats.FullHits, stats.FullLookups)
			}
			assertTreesEqual(t, lang, baselineTree.RootNode(), internedTree.RootNode())
		})
	}
}

func assertTreesEqual(t *testing.T, lang *gotreesitter.Language, base, interned *gotreesitter.Node) {
	t.Helper()
	var (
		diffs int
		walk  func(a, b *gotreesitter.Node, path string)
	)
	walk = func(a, b *gotreesitter.Node, p string) {
		if diffs >= 5 {
			return
		}
		if (a == nil) != (b == nil) {
			t.Errorf("at %s: nil mismatch baseline=%v interned=%v", p, a == nil, b == nil)
			diffs++
			return
		}
		if a == nil {
			return
		}
		if a.Type(lang) != b.Type(lang) ||
			a.StartByte() != b.StartByte() || a.EndByte() != b.EndByte() ||
			a.ChildCount() != b.ChildCount() ||
			a.IsNamed() != b.IsNamed() || a.HasError() != b.HasError() {
			t.Errorf("at %s: divergent\n  baseline: type=%q span=[%d..%d] childCount=%d named=%v error=%v\n  interned: type=%q span=[%d..%d] childCount=%d named=%v error=%v",
				p,
				a.Type(lang), a.StartByte(), a.EndByte(), a.ChildCount(), a.IsNamed(), a.HasError(),
				b.Type(lang), b.StartByte(), b.EndByte(), b.ChildCount(), b.IsNamed(), b.HasError())
			diffs++
			return
		}
		for i := 0; i < a.ChildCount(); i++ {
			walk(a.Child(i), b.Child(i), p+"/"+a.Type(lang))
		}
	}
	walk(base, interned, "")
}

// TestInternLeafObservationParseJS exercises the Phase 2 observation
// path end-to-end: enable observation, parse a non-trivial JS file,
// and assert that the table populates. Reports the hit rate so
// `go test -v` output is the measurement artifact. Skips if the
// corpus file isn't available locally.
func TestInternLeafObservationParseJS(t *testing.T) {
	const path = "cgo_harness/corpus_real/javascript/large__text-editor-component.js"
	src, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("corpus not present: %v", err)
	}
	gotreesitter.SetInternLeavesObserveEnabled(true)
	t.Cleanup(func() { gotreesitter.SetInternLeavesObserveEnabled(false) })

	lang := grammars.JavascriptLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	root := tree.RootNode()
	stats := gotreesitter.InternStatsFor(root)

	t.Logf("leaf observation: lookups=%d hits=%d misses=%d stores=%d growths=%d",
		stats.LeafLookups, stats.LeafHits, stats.LeafMisses, stats.LeafStores, stats.LeafGrowths)

	if stats.LeafLookups == 0 {
		t.Errorf("expected at least one leaf lookup, got zero — observation hook not wired")
	}
	if stats.LeafLookups != stats.LeafHits+stats.LeafMisses {
		t.Errorf("counter invariant: lookups=%d but hits+misses=%d", stats.LeafLookups, stats.LeafHits+stats.LeafMisses)
	}

	// Hit rate gives us the Phase-2-to-3 go/no-go signal. Sub-5% hit
	// rate suggests leaf interning is not worth wiring; high hit rate
	// (e.g. 30%+) justifies Phase 3 behavior change.
	if stats.LeafLookups > 0 {
		hitRate := float64(stats.LeafHits) / float64(stats.LeafLookups) * 100
		t.Logf("leaf hit rate: %.1f%% (%d/%d)", hitRate, stats.LeafHits, stats.LeafLookups)
	}
}
