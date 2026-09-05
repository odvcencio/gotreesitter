package gotreesitter

import "testing"

func TestForestTokenInvariantHistoryLifecycle(t *testing.T) {
	previousForest := glrForestEnabled
	glrForestEnabled = true
	t.Cleanup(func() { glrForestEnabled = previousForest })
	for _, experimental := range []bool{true, false} {
		name := "dispatch"
		if experimental {
			name = "experimental"
		}
		t.Run(name, func(t *testing.T) {
			// The forest requires the generated grammar's start state 1.
			lang := loadBlobForDecode(t, "json")
			lang.WantsForest = true
			parser := NewParser(lang)
			parse := func(source string) *Tree {
				if experimental {
					tree, _ := parser.ParseForestExperimental([]byte(source))
					return tree
				}
				return parser.tryForestFastPath([]byte(source))
			}
			long := parse("123456789")
			if long == nil {
				t.Fatalf("long forest parse declined: %s", parser.forestDeclineReason)
			}
			longSpan := long.tokenInvariantReadSpan
			long.Release()
			if longSpan == 0 {
				t.Fatal("accepted forest lost lexical history before publication")
			}
			if failed := parse("["); failed != nil {
				failed.Release()
				t.Fatal("incomplete expression unexpectedly accepted")
			}
			short := parse("1")
			if short == nil {
				t.Fatalf("short forest parse declined: %s", parser.forestDeclineReason)
			}
			defer short.Release()
			if short.tokenInvariantReadSpan == 0 || short.tokenInvariantReadSpan >= longSpan {
				t.Fatalf("source reuse retained stale history: short=%d long=%d", short.tokenInvariantReadSpan, longSpan)
			}
			if !short.forestFastPath || short.rawParseRuntime().StopReason != ParseStopAccepted {
				t.Fatal("fixture did not return an accepted forest tree")
			}
		})
	}
}

func TestForestTokenInvariantHistoryDeclineClearsReceipt(t *testing.T) {
	parser := NewParser(loadBlobForDecode(t, "json"))
	for _, source := range []string{"12345", "[", "1"} {
		arena := acquireNodeArena(arenaClassFull)
		receipt := uint32(999)
		root, ok := parser.parseForest(arena, []byte(source), false, parseMemoryBudgetForParser(parser, len(source)), &receipt)
		if source == "[" {
			if ok || root != nil || receipt != 0 {
				t.Errorf("decline published history: ok=%v root=%v receipt=%d", ok, root != nil, receipt)
			}
		} else if !ok || root == nil || receipt == 0 || receipt == 999 {
			t.Errorf("clean attempt failed to replace receipt: ok=%v receipt=%d", ok, receipt)
		}
		arena.Release()
	}
}
