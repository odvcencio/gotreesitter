//go:build gts_workcount

package gotreesitter

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestDiagnosticWorkCountSaturatesAndFlagsOverflow(t *testing.T) {
	BeginDiagnosticWorkCount()
	activeDiagnosticWorkCount.Shifts = math.MaxUint64
	workCountRecordShift()
	counts := EndDiagnosticWorkCount()
	if !counts.Overflow {
		t.Fatal("overflow flag is false")
	}
	if counts.Shifts != math.MaxUint64 {
		t.Fatalf("shifts=%d want saturation at %d", counts.Shifts, uint64(math.MaxUint64))
	}
}

func TestDiagnosticWorkCountLinearReductionPop(t *testing.T) {
	leaf := newLeafNodeInArena(nil, 1, true, 0, 1, Point{}, Point{Column: 1})
	extra := newLeafNodeInArena(nil, 2, false, 1, 2, Point{Column: 1}, Point{Column: 2})
	extra.setExtra(true)
	stack := newGLRStack(1)
	stack.entries = append(stack.entries,
		newStackEntryNode(2, leaf),
		newStackEntryNode(2, extra),
	)

	BeginDiagnosticWorkCount()
	workCountObserveReductionPop(&stack, 1)
	counts := EndDiagnosticWorkCount()
	if counts.EmittedPopPaths != 1 || counts.EmittedPopPayloads != 2 {
		t.Fatalf("paths=%d payloads=%d want 1/2", counts.EmittedPopPaths, counts.EmittedPopPayloads)
	}
}

func TestDiagnosticWorkCountBoardDirectEvents(t *testing.T) {
	BeginDiagnosticWorkCount()
	workCountRecordFrontierLexerElection()
	workCountRecordRawMainLexerInvocation()
	workCountRecordRawMainLexerInvocation()
	workCountRecordResolvedActionCell(4)
	workCountRecordResolvedActionCell(1)
	workCountRecordAlternatePredecessorLinkAppended()
	counts := EndDiagnosticWorkCount().BoardDirect()
	if counts.Schema != "gts-work-count-board-direct/v3" || counts.Overflow || !counts.FrontierLexerElectionsAvailable || counts.FrontierLexerElections != 1 || counts.PerVersionLexRequestsAvailable || counts.PerVersionLexRequests != 0 || counts.RawMainLexerInvocations != 2 || counts.ResolvedActionCellsExamined != 2 || counts.RawActionEntriesBeyondFirst != 3 || counts.AlternatePredecessorLinksAppended != 1 {
		t.Fatalf("board direct counts=%+v", counts)
	}
}

func TestDiagnosticWorkCountSelectedCensusSaturates(t *testing.T) {
	counts := DiagnosticWorkCount{
		DiagnosticWorkCountValues: DiagnosticWorkCountValues{SelectedNodes: math.MaxUint64},
	}
	counts.AddDiagnosticSelectedNode(false)
	if !counts.Overflow || counts.SelectedNodes != math.MaxUint64 || counts.SelectedLeafNodes != 1 {
		t.Fatalf("selected census saturation=%+v", counts)
	}
}

func TestDiagnosticRetryTraceRecordsAttemptsWithoutWorkObserver(t *testing.T) {
	BeginDiagnosticRetryTrace()
	if activeDiagnosticWorkCount != nil {
		t.Fatal("attempt-only trace enabled the work-count ledger")
	}
	if workCountConvergenceActive() {
		t.Fatal("attempt-only trace enabled the convergence observer")
	}
	workCountRecordShift()
	workCountSetNextParseAttempt("initial_full", "fresh_dfa_full_parse")
	token := workCountBeginParseAttempt(8, 100, 6)
	workCountResolveParseAttempt(token, 8, false, 6, 10, 1000, 100)
	workCountBeginFinalizeParseAttempt(token)
	workCountEndFinalizeParseAttempt(token, ParseStopNoStacksAlive, nil)
	trace := EndDiagnosticRetryTrace()

	if len(trace.Attempts) != 1 {
		t.Fatalf("attempts=%d want=1", len(trace.Attempts))
	}
	attempt := trace.Attempts[0]
	if attempt.LogicalRung != "initial_full" || attempt.OperationCause != "fresh_dfa_full_parse" ||
		attempt.StopReason != ParseStopNoStacksAlive || attempt.ResolvedMaxStacks != 8 ||
		attempt.ResolvedRetryPass || attempt.ResolvedMaxMergePerKey != 6 {
		t.Fatalf("attempt=%+v", attempt)
	}
	if attempt.Counters.Shifts != 0 {
		t.Fatalf("attempt shifts=%d want=0", attempt.Counters.Shifts)
	}
}

func TestDiagnosticWorkCountStillRecordsAttemptBoundaries(t *testing.T) {
	BeginDiagnosticWorkCount()
	workCountSetNextParseAttempt("initial_full", "fresh_dfa_full_parse")
	token := workCountBeginParseAttempt(8, 100, 6)
	workCountResolveParseAttempt(token, 8, false, 6, 10, 1000, 100)
	workCountBeginFinalizeParseAttempt(token)
	workCountEndFinalizeParseAttempt(token, ParseStopAccepted, nil)
	counts := EndDiagnosticWorkCount()

	if len(counts.Attempts) != 1 {
		t.Fatalf("attempts=%d want=1", len(counts.Attempts))
	}
	if attempt := counts.Attempts[0]; attempt.StopReason != ParseStopAccepted || attempt.ResolvedMaxStacks != 8 {
		t.Fatalf("attempt=%+v", attempt)
	}
}

func testNestedCounterParse(t *testing.T) DiagnosticWorkCount {
	t.Helper()
	lang := buildArithmeticLanguage()
	outer := NewParser(lang)
	called := false
	outer.SetLogger(func(kind ParserLogType, msg string) {
		if called || kind != ParserLogParse || !strings.HasPrefix(msg, "start len=") {
			return
		}
		called = true
		child := NewParser(lang)
		grandCalled := false
		child.SetLogger(func(kind ParserLogType, msg string) {
			if grandCalled || kind != ParserLogParse || !strings.HasPrefix(msg, "start len=") {
				return
			}
			grandCalled = true
			t1, e := NewParser(lang).Parse([]byte("3"))
			if e != nil || t1 == nil {
				t.Fatalf("grandchild %v", e)
			}
			t1.Release()
		})
		t1, e := child.Parse([]byte("2"))
		if e != nil || t1 == nil {
			t.Fatalf("child %v", e)
		}
		t1.Release()
		t2, e := NewParser(lang).Parse([]byte("4"))
		if e != nil || t2 == nil {
			t.Fatalf("sibling %v", e)
		}
		t2.Release()
	})
	BeginDiagnosticWorkCount()
	workCountRecordShift()
	tree, err := outer.Parse([]byte("1+2"))
	if err != nil || tree == nil {
		t.Fatalf("outer %v", err)
	}
	tree.Release()
	workCountRecordShift()
	return EndDiagnosticWorkCount()
}
func TestDiagnosticNestedCounterConservation(t *testing.T) {
	c := testNestedCounterParse(t)
	if c.Overflow {
		t.Fatal("valid nested accounting marked overflow")
	}
	if len(c.Attempts) != 4 {
		t.Fatalf("attempts=%d", len(c.Attempts))
	}

	sum := c.OutsideAttempt
	for _, a := range c.Attempts {
		phase, ok := workCountNestedVectorAdd(a.EntryToCaps, a.CapsToFinalize)
		if !ok {
			t.Fatal("phase overflow")
		}
		phase, ok = workCountNestedVectorAdd(phase, a.Finalize)
		if !ok || !reflect.DeepEqual(phase, a.Counters) {
			t.Fatal("phase partition mismatch")
		}
		sum, ok = workCountNestedVectorAdd(sum, a.Counters)
		if !ok {
			t.Fatal("sum overflow")
		}
	}
	if !reflect.DeepEqual(sum, c.DiagnosticWorkCountValues) {
		t.Fatal("exclusive + outside differs from aggregate")
	}
	if c.OutsideAttempt.Shifts != 2 {
		t.Fatalf("outside=%d", c.OutsideAttempt.Shifts)
	}
}
func TestDiagnosticNestedCounterPhaseAndFailure(t *testing.T) {
	BeginDiagnosticWorkCount()
	root := workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	child := workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	workCountBeginFinalizeParseAttempt(child)
	workCountEndFinalizeParseAttempt(child, ParseStopAccepted, nil)
	workCountResolveParseAttempt(root, 8, false, 6, 10, 1000, 100)
	workCountRecordShift()
	child = workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	workCountBeginFinalizeParseAttempt(child)
	workCountEndFinalizeParseAttempt(child, ParseStopAccepted, nil)
	workCountBeginFinalizeParseAttempt(root)
	workCountRecordShift()
	child = workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	workCountBeginFinalizeParseAttempt(child)
	workCountEndFinalizeParseAttempt(child, ParseStopAccepted, nil)
	workCountEndFinalizeParseAttempt(root, ParseStopAccepted, nil)
	c := EndDiagnosticWorkCount()
	if c.Overflow || len(c.Attempts) != 4 {
		t.Fatalf("accounting invalid %+v", c)
	}
	a := c.Attempts[0]
	if a.EntryToCaps.Shifts != 1 || a.CapsToFinalize.Shifts != 1 || a.Finalize.Shifts != 1 || a.Counters.Shifts != 3 {
		t.Fatalf("phase accounting %+v", a)
	}
}
func TestDiagnosticNestedCounterFailureClosed(t *testing.T) {
	cases := []struct {
		name string
		run  func(*DiagnosticWorkCount)
	}{
		{"out_of_order", func(c *DiagnosticWorkCount) {
			workCountNestedBegin(c, 1)
			workCountNestedBegin(c, 2)
			workCountNestedEnd(c, 1, &DiagnosticWorkCountAttempt{})
		}},
		{"cap", func(c *DiagnosticWorkCount) {
			for i := 1; i <= workCountNestedDepthLimit+1; i++ {
				workCountNestedBegin(c, uint32(i))
			}
		}},
		{"negative", func(c *DiagnosticWorkCount) {
			workCountNestedBegin(c, 1)
			workCountNestedAttempts[0].child[0] = DiagnosticWorkCountValues{Shifts: 1}
			workCountNestedEnd(c, 1, &DiagnosticWorkCountAttempt{})
		}},
		{"arithmetic", func(c *DiagnosticWorkCount) {
			workCountNestedBegin(c, 1)
			workCountNestedAttempts[0].child[0] = DiagnosticWorkCountValues{Shifts: ^uint64(0)}
			workCountNestedAttempts[0].child[1] = DiagnosticWorkCountValues{Shifts: 1}
			workCountNestedEnd(c, 1, &DiagnosticWorkCountAttempt{})
		}},
		{"unfinished", func(c *DiagnosticWorkCount) { workCountNestedBegin(c, 1); workCountNestedFinishSession(c) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			workCountNestedStart()
			c := &DiagnosticWorkCount{}
			tc.run(c)
			if !c.Overflow {
				t.Fatal("invalid accounting certified")
			}
		})
	}
}

func TestDiagnosticNestedEarlyFinalizeNoCaps(t *testing.T) {
	BeginDiagnosticWorkCount()
	root := workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	child := workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	grand := workCountBeginParseAttempt(8, 100, 6)
	workCountRecordShift()
	workCountBeginFinalizeParseAttempt(grand)
	workCountEndFinalizeParseAttempt(grand, ParseStopAccepted, nil)
	workCountBeginFinalizeParseAttempt(child)
	workCountEndFinalizeParseAttempt(child, ParseStopAccepted, nil)
	workCountBeginFinalizeParseAttempt(root)
	workCountEndFinalizeParseAttempt(root, ParseStopAccepted, nil)
	c := EndDiagnosticWorkCount()
	if c.Overflow || len(c.Attempts) != 3 {
		t.Fatalf("unexpected accounting %+v", c)
	}
	for _, a := range c.Attempts {
		if a.EntryToCaps.Shifts != 0 || a.CapsToFinalize.Shifts != 1 || a.Finalize.Shifts != 0 || a.Counters.Shifts != 1 {
			t.Fatalf("wrong early-finalize partition %+v", a)
		}
	}
}

func TestDiagnosticRejectedNestedBeginPreservesLedger(t *testing.T) {
	BeginDiagnosticWorkCount()
	root := workCountBeginParseAttempt(8, 100, 6)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("nested Begin did not panic")
			}
		}()
		BeginDiagnosticWorkCount()
	}()
	workCountResolveParseAttempt(root, 8, false, 6, 10, 1000, 100)
	workCountRecordShift()
	workCountBeginFinalizeParseAttempt(root)
	workCountEndFinalizeParseAttempt(root, ParseStopAccepted, nil)
	c := EndDiagnosticWorkCount()
	if c.Overflow || len(c.Attempts) != 1 || c.Attempts[0].Counters.Shifts != 1 {
		t.Fatalf("ledger reset on rejected Begin: %+v", c)
	}
}

// The frozen v2 JSON represents 600 sequential, non-nested attempts.
func TestDiagnosticFlat600CounterJSON(t *testing.T) {
	BeginDiagnosticWorkCount()
	for i := 0; i < 600; i++ {
		token := workCountBeginParseAttempt(8, 100, 6)
		workCountResolveParseAttempt(token, 8, false, 6, 10, 1000, 100)
		workCountRecordShift()
		workCountBeginFinalizeParseAttempt(token)
		workCountEndFinalizeParseAttempt(token, ParseStopAccepted, nil)
	}
	counts := EndDiagnosticWorkCount()
	data, err := json.Marshal(counts)
	if err != nil {
		t.Fatal(err)
	}
	const want = "5f811f25072a07692609cbc3971908c55b3688cc4bdd03480b17ee2229f049cc"
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
		t.Fatalf("flat v2 JSON sha256=%s want=%s", got, want)
	}
}
