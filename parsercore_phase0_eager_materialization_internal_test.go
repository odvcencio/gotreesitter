//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"fmt"
	"testing"
)

// compactEagerCompareTrees walks two trees in parallel and reports the first
// node whose materialized fields differ. The compared fields are every field
// the materializer writes: symbol, span, points, flags, production,
// dynamic precedence, the replay stamps, and the field metadata.
func compactEagerCompareTrees(lang *Language, want, got *Tree) error {
	type pair struct {
		want, got *Node
		path      string
	}
	stack := []pair{{want.root, got.root, "root"}}
	for len(stack) > 0 {
		last := len(stack) - 1
		p := stack[last]
		stack = stack[:last]
		w, g := p.want, p.got
		if (w == nil) != (g == nil) {
			return fmt.Errorf("%s: nil mismatch", p.path)
		}
		if w == nil {
			continue
		}
		if w.symbol != g.symbol || w.startByte != g.startByte || w.endByte != g.endByte ||
			w.startPoint != g.startPoint || w.endPoint != g.endPoint || w.flags != g.flags ||
			w.productionID != g.productionID || w.dynamicPrecedence != g.dynamicPrecedence ||
			w.parseState != g.parseState || w.preGotoState != g.preGotoState {
			return fmt.Errorf("%s: symbol=%d/%d span=%d..%d/%d..%d flags=%#x/%#x production=%d/%d precedence=%d/%d parseState=%d/%d preGoto=%d/%d",
				p.path, w.symbol, g.symbol, w.startByte, w.endByte, g.startByte, g.endByte, w.flags, g.flags,
				w.productionID, g.productionID, w.dynamicPrecedence, g.dynamicPrecedence,
				w.parseState, g.parseState, w.preGotoState, g.preGotoState)
		}
		wc, gc := nodeChildCountNoMaterialize(w), nodeChildCountNoMaterialize(g)
		if wc != gc {
			return fmt.Errorf("%s: child count %d/%d", p.path, wc, gc)
		}
		for i := 0; i < wc; i++ {
			if w.FieldNameForChild(i, lang) != g.FieldNameForChild(i, lang) {
				return fmt.Errorf("%s: child %d field %q/%q", p.path, i, w.FieldNameForChild(i, lang), g.FieldNameForChild(i, lang))
			}
			stack = append(stack, pair{
				nodeChildAtForReason(w, i, materializeForEdit),
				nodeChildAtForReason(g, i, materializeForEdit),
				fmt.Sprintf("%s/%d", p.path, i),
			})
		}
	}
	return nil
}

func compactEagerCountNodes(root *Node) int {
	count := 0
	stack := []*Node{root}
	for len(stack) > 0 {
		last := len(stack) - 1
		n := stack[last]
		stack = stack[:last]
		if n == nil {
			continue
		}
		count++
		for i := nodeChildCountNoMaterialize(n) - 1; i >= 0; i-- {
			stack = append(stack, nodeChildAtForReason(n, i, materializeForEdit))
		}
	}
	return count
}

// TestCompactEagerMaterializationMatchesPostorder proves the eager driver
// publishes the same tree, the same replay stamps, and the same work as the
// postorder-only pass on Go sources with trailing extras, unary collapses,
// and grammar conflicts.
func TestCompactEagerMaterializationMatchesPostorder(t *testing.T) {
	if parserCoreWarmGoScanner == nil {
		t.Skip("authenticated Go scanner unavailable")
	}
	var clean, trailing, conflicts bytes.Buffer
	for _, b := range []*bytes.Buffer{&clean, &trailing, &conflicts} {
		b.WriteString("package main\n\nimport \"fmt\"\n\n")
	}
	for i := 0; clean.Len() < 48<<10; i++ {
		fmt.Fprintf(&clean, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
		fmt.Fprintf(&trailing, "// f%d adds.\nfunc f%d(a int, b int) int {\n\tx := a + b // sum\n\t/* print */ fmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i, i)
		fmt.Fprintf(&conflicts, "func g%d[T any](a, b T) []T {\n\tvar s []T\n\tif a == b || a != b {\n\t\ts = append(s, a, b)\n\t}\n\tm := map[string]int{\"k\": %d}\n\tfor k, v := range m {\n\t\tfmt.Println(k, v, s[0:1], x < y, y > z)\n\t}\n\treturn s\n}\n\n", i, i)
	}
	trailing.WriteString("// trailing comment\n")
	sources := []struct {
		name string
		src  []byte
	}{{"clean", clean.Bytes()}, {"trailing", trailing.Bytes()}, {"conflicts", conflicts.Bytes()}}
	for _, row := range diagnosticParserCoreCanonicalAdmissions {
		fixture := loadDiagnosticParserCoreCanonicalFixture(t, row.id)
		sources = append(sources, struct {
			name string
			src  []byte
		}{"fixture-" + row.id, fixture.Source})
	}
	parse := func(t *testing.T, eager bool, source []byte) (*Tree, *parserCoreFreshFullRunner) {
		restore := SetParserCoreEagerMaterializationEnabledForTest(eager)
		defer restore()
		runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
		if err != nil {
			t.Fatal(err)
		}
		tree, err := runner.parse(source)
		if err != nil {
			t.Skipf("scheduler declined this source: %v", err)
		}
		return tree, runner
	}
	for _, source := range sources {
		t.Run(source.name, func(t *testing.T) {
			want, wantRunner := parse(t, false, source.src)
			defer want.Release()
			if wantRunner.scratch.materializer.lastAdopted || wantRunner.scratch.materializer.lastEagerNodes != 0 {
				t.Fatalf("postorder-only run adopted eager state: adopted=%t nodes=%d",
					wantRunner.scratch.materializer.lastAdopted, wantRunner.scratch.materializer.lastEagerNodes)
			}
			got, gotRunner := parse(t, true, source.src)
			defer got.Release()
			m := &gotRunner.scratch.materializer
			if !m.lastAdopted || m.lastEagerNodes == 0 {
				t.Fatalf("eager run did not adopt eager nodes: adopted=%t nodes=%d skipped=%d", m.lastAdopted, m.lastEagerNodes, m.lastEagerSkipped)
			}
			if err := compactEagerCompareTrees(gotRunner.lang, want, got); err != nil {
				t.Fatal(err)
			}
			wantDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, want, wantRunner.lang)
			gotDigest := requireDiagnosticParserCoreCanonicalTreeDigest(t, got, gotRunner.lang)
			if wantDigest != gotDigest {
				t.Fatalf("deep-tree digest %s != %s", gotDigest, wantDigest)
			}
			wantRuntime, gotRuntime := want.rawParseRuntime(), got.rawParseRuntime()
			if wantRuntime.NodesAllocated != gotRuntime.NodesAllocated ||
				wantRuntime.TokensConsumed != gotRuntime.TokensConsumed ||
				wantRuntime.CompactReductions != gotRuntime.CompactReductions {
				t.Fatalf("work differs: nodes=%d/%d tokens=%d/%d reductions=%d/%d",
					wantRuntime.NodesAllocated, gotRuntime.NodesAllocated,
					wantRuntime.TokensConsumed, gotRuntime.TokensConsumed,
					wantRuntime.CompactReductions, gotRuntime.CompactReductions)
			}
			if wantRunner.compact.Work() != gotRunner.compact.Work() {
				t.Fatalf("compact core work differs:\n%+v\n%+v", wantRunner.compact.Work(), gotRunner.compact.Work())
			}
			if want.incrementalReuseDisabled != got.incrementalReuseDisabled {
				t.Fatalf("reuse eligibility differs: %t/%t", want.incrementalReuseDisabled, got.incrementalReuseDisabled)
			}
			total := compactEagerCountNodes(got.root)
			t.Logf("%s: %d public nodes; eager built %d subtrees, skipped %d pushes", source.name, total, m.lastEagerNodes, m.lastEagerSkipped)
		})
	}
}
