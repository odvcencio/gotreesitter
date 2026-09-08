//go:build gts_parsercorephase0

package gotreesitter

import (
	"bytes"
	"fmt"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// TestCompactFusedReplayMatchesTopDownReplay proves that the parse states the
// postorder visit assigns through its replay transition equal the states the
// separate top-down replay (replayCompactDerivation) assigns, id by id, on Go
// sources with plain code, extras (comments), and a syntax error.
func TestCompactFusedReplayMatchesTopDownReplay(t *testing.T) {
	var clean, commented, broken bytes.Buffer
	for _, b := range []*bytes.Buffer{&clean, &commented, &broken} {
		b.WriteString("package main\n\nimport \"fmt\"\n\n")
	}
	for i := 0; clean.Len() < 24<<10; i++ {
		fmt.Fprintf(&clean, "func f%d(a int, b int) int {\n\tx := a + b\n\tfmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i)
		fmt.Fprintf(&commented, "// f%d adds.\nfunc f%d(a int, b int) int {\n\tx := a + b // sum\n\t/* print */ fmt.Println(\"f%d\", x)\n\treturn x\n}\n\n", i, i, i)
		if i == 7 {
			fmt.Fprintf(&broken, "func f%d(a int, b int) int {\n\tx := a +\n\treturn x\n}\n\n", i)
			continue
		}
		fmt.Fprintf(&broken, "func f%d(a int, b int) int {\n\tx := a + b\n\treturn x\n}\n\n", i)
	}
	for _, source := range []struct {
		name string
		src  []byte
	}{{"clean", clean.Bytes()}, {"commented", commented.Bytes()}, {"broken", broken.Bytes()}} {
		t.Run(source.name, func(t *testing.T) {
			runner, err := newParserCoreFreshFullRunner(parserCoreWarmGoScanner, parserCoreFreshFullCanonicalOptions())
			if err != nil {
				t.Fatalf("runner: %v", err)
			}
			scheduler, tokenSource, err := runner.executeSchedulerOpen(source.src, runner.compact, true)
			if err != nil {
				if tokenSource != nil {
					tokenSource.Close()
				}
				t.Skipf("scheduler declined this source: %v", err)
			}
			defer tokenSource.Close()
			derivations, err := runner.compact.Derivations(scheduler.acceptedHead)
			if err != nil || len(derivations) != 1 {
				t.Fatalf("derivations=%d err=%v", len(derivations), err)
			}
			payloads := derivations[0].Payloads
			expected, err := runner.parser.replayCompactDerivation(runner.compact, payloads)
			if err != nil {
				t.Fatalf("top-down replay: %v", err)
			}
			defer expected.release()
			transition := func(pre core.StateID, view core.MaterializationReplayView) (core.StateID, bool, error) {
				state, known, err := runner.parser.replayCompactMaterializationTransition(StateID(pre), view)
				return core.StateID(state), known, err
			}
			var scratch core.MaterializationPostorderScratch
			visited := 0
			err = runner.compact.VisitMaterializationPostorderWithReplay(payloads, nil, &scratch,
				core.StateID(runner.parser.replayRootPreGotoState()), transition,
				func(id core.SubtreeID, view core.MaterializationSubtreeView) error {
					visited++
					pre, ps, preOk, psOk := expected.get(id)
					if psOk != view.ReplayParseStateKnown || preOk != view.ReplayPreGotoKnown ||
						(psOk && ps != StateID(view.ReplayParseState)) || (preOk && pre != StateID(view.ReplayPreGotoState)) {
						return fmt.Errorf("id %d: fused=(pre %d known %t, state %d known %t) replay=(pre %d known %t, state %d known %t)",
							id, view.ReplayPreGotoState, view.ReplayPreGotoKnown, view.ReplayParseState, view.ReplayParseStateKnown, pre, preOk, ps, psOk)
					}
					return nil
				})
			if err != nil {
				t.Fatal(err)
			}
			if visited == 0 {
				t.Fatal("visited no subtrees")
			}
			t.Logf("%s: %d subtrees agree", source.name, visited)
		})
	}
}
