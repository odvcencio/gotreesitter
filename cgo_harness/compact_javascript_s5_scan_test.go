//go:build cgo && treesitter_c_parity

package cgoharness

import (
	"fmt"
	"testing"

	gotreesitter "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// TestCompactJavaScriptS5RecoveryMutationDifferential finds S5-only routes in
// a deterministic mutation set. Every expanded route must match C exactly.
func TestCompactJavaScriptS5RecoveryMutationDifferential(t *testing.T) {
	cLanguage, err := ParityCLanguage("javascript")
	if err != nil {
		t.Fatal(err)
	}
	goLanguage := grammars.JavascriptLanguage()
	if !goLanguage.CompactStrategy2ErrorRegionCertified || !goLanguage.CompactMissingTokenInsertionCertified {
		t.Fatal("the exact JavaScript artifact lacks complete compact recovery certification")
	}
	s3OnlyLanguage := *goLanguage
	s3OnlyLanguage.CompactMissingTokenInsertionCertified = false
	fullParser := gotreesitter.NewParser(goLanguage)
	fullParser.SetAdmissionCandidateRoute(true)
	s3OnlyParser := gotreesitter.NewParser(&s3OnlyLanguage)
	s3OnlyParser.SetAdmissionCandidateRoute(true)

	parseRoute := func(parser *gotreesitter.Parser, source []byte) (*gotreesitter.Tree, bool) {
		t.Helper()
		beforeRouted, beforeFallback := gotreesitter.AdmissionCandidateCounters()
		tree, parseErr := parser.Parse(source)
		if parseErr != nil {
			t.Fatalf("parse %q: %v", source, parseErr)
		}
		if tree == nil || tree.RootNode() == nil {
			t.Fatalf("parse %q returned no root", source)
		}
		afterRouted, afterFallback := gotreesitter.AdmissionCandidateCounters()
		switch {
		case afterRouted-beforeRouted == 1 && afterFallback-beforeFallback == 0:
			return tree, true
		case afterRouted-beforeRouted == 0 && afterFallback-beforeFallback == 1:
			return tree, false
		default:
			t.Fatalf("parse %q route counters=%d/%d, want 1/0 or 0/1",
				source, afterRouted-beforeRouted, afterFallback-beforeFallback)
			return nil, false
		}
	}

	seeds := []string{
		"function f(a) { return a; }",
		"function f(a, b) { return a + b; }",
		"const f = (a, b) => a + b;",
		"const f = async (a) => await a;",
		"const x = foo(1, 2, 3);",
		"const x = (1 + 2) * 3;",
		"const x = [1, 2, 3];",
		"const x = {a: 1, b: 2};",
		"if (x) { y(); } else { z(); }",
		"while (x) { y(); }",
		"for (let i = 0; i < 3; i++) { x(i); }",
		"for (const x of xs) { y(x); }",
		"class A { m(a) { return a; } }",
		"try { x(); } catch (e) { y(e); }",
		"switch (x) { case 1: y(); break; default: z(); }",
		"const x = a ? b : c;",
		"const x = `hello ${name}`;",
		"export function f() { return 1; }",
		"import {a, b} from 'm';",
		"const {a, b} = value;",
		"const [a, b] = value;",
		"async function f() { await x(); }",
		"function* f() { yield 1; }",
		"const x = new A(1, 2);",
		"a.b.c(1, 2);",
	}
	replacements := []byte{'?', '#', ';', ')', '}', ']', ':'}
	var cases, expanded, contracted int
	check := func(name string, source []byte) {
		t.Helper()
		cases++
		fullTree, fullRouted := parseRoute(fullParser, source)
		s3Tree, s3Routed := parseRoute(s3OnlyParser, source)
		if !fullRouted && s3Routed {
			contracted++
		}
		if !fullRouted || s3Routed {
			fullTree.Release()
			s3Tree.Release()
			return
		}
		expanded++
		cTree := compactT3ParseC(t, cLanguage, source)
		if divergences := compactT3StructuralDivergences(fullTree.RootNode(), goLanguage, cTree.RootNode()); len(divergences) != 0 {
			t.Fatalf("%s: S5-expanded tree diverges from C at %d node(s); first: %s; source=%q",
				name, len(divergences), compactT3FormatDivergence(divergences[0]), source)
		}
		cTree.Close()
		fullTree.Release()
		s3Tree.Release()
	}
	for seedIndex, seedText := range seeds {
		seed := []byte(seedText)
		check(fmt.Sprintf("seed-%d", seedIndex), seed)
		for offset := range seed {
			deleted := append([]byte(nil), seed[:offset]...)
			deleted = append(deleted, seed[offset+1:]...)
			check(fmt.Sprintf("delete-%d-%d", seedIndex, offset), deleted)
			for _, replacement := range replacements {
				if seed[offset] == replacement {
					continue
				}
				replaced := append([]byte(nil), seed...)
				replaced[offset] = replacement
				check(fmt.Sprintf("replace-%d-%d-%02x", seedIndex, offset, replacement), replaced)
			}
		}
	}
	// Stateless external-scanner recovery adds 13 exact routes to the prior
	// 45-route frontier. The loop compares every expanded tree with C above.
	if expanded != 58 || contracted != 0 {
		t.Fatalf("S5 route delta expanded=%d contracted=%d, want 58/0 across %d cases", expanded, contracted, cases)
	}
	t.Logf("JavaScript S5 mutation differential: cases=%d expanded=%d contracted=%d", cases, expanded, contracted)
}
