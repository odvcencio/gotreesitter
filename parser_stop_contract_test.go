package gotreesitter

import (
	"strings"
	"testing"
)

// buildArithmeticPlusChain builds a deterministic arithmetic source ("1+1+1+...")
// with count terms, for use with buildArithmeticLanguage(). The grammar has no
// GLR ambiguity, so its node count for a given term count is exact and stable.
func buildArithmeticPlusChain(count int) []byte {
	terms := make([]string, count)
	for i := range terms {
		terms[i] = "1"
	}
	return []byte(strings.Join(terms, "+"))
}

// --- Defect 1: rawParseEligibleForFreshRetryLadder gating ---

func TestRawParseEligibleForFreshRetryLadder(t *testing.T) {
	tests := []struct {
		name   string
		reason ParseStopReason
		want   bool
	}{
		{"accepted stays eligible (not stopped early)", ParseStopAccepted, true},
		{"no_stacks_alive stays eligible (not stopped early)", ParseStopNoStacksAlive, true},
		{"node_limit is the one early-stop exception", ParseStopNodeLimit, true},
		{"iteration_limit stays a hard stop", ParseStopIterationLimit, false},
		{"stack_depth_limit stays a hard stop", ParseStopStackDepthLimit, false},
		{"memory_budget stays a hard stop", ParseStopMemoryBudget, false},
		{"reuse_budget stays a hard stop", ParseStopReuseBudget, false},
		{"token_source_eof stays a hard stop", ParseStopTokenSourceEOF, false},
		{"timeout stays a hard stop", ParseStopTimeout, false},
		{"cancelled stays a hard stop", ParseStopCancelled, false},
		{"invariant_violation stays a hard stop", ParseStopInvariantViolation, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tree := &Tree{parseRuntime: &ParseRuntime{StopReason: test.reason}}
			if got := tree.rawParseEligibleForFreshRetryLadder(); got != test.want {
				t.Fatalf("rawParseEligibleForFreshRetryLadder(%s) = %t, want %t", test.reason, got, test.want)
			}
		})
	}
}

// TestFreshParseWidensDefaultNodeBudgetOnNodeLimitStop is the Defect 1
// regression: a fresh top-level Parse whose first pass stops on the
// source-derived default node budget must enter the retry ladder and widen
// (fullParseRetryNodeLimitScale = 2), not return the truncated tree as-is.
//
// parseNodeLimitTestOverride replaces the huge (>=300,000) source-derived
// default with a small, deterministic value so a first pass reliably trips
// ParseStopNodeLimit on a tiny, fast grammar, without touching the caller-
// facing ParseWorkLimits contract this test also exercises separately below.
func TestFreshParseWidensDefaultNodeBudgetOnNodeLimitStop(t *testing.T) {
	lang := buildArithmeticLanguage()
	source := buildArithmeticPlusChain(60)

	// Measure the exact node count a clean accept needs on this input so the
	// override is neither too tight (never makes progress) nor too loose
	// (never trips at all).
	baseline := NewParser(lang)
	baseline.SetAdmissionCandidateRoute(false)
	baselineTree, err := baseline.Parse(source)
	if err != nil {
		t.Fatalf("baseline Parse error: %v", err)
	}
	baselineRuntime := baselineTree.ParseRuntime()
	baselineTree.Release()
	if baselineRuntime.StopReason != ParseStopAccepted {
		t.Fatalf("baseline StopReason = %q, want %q (%s)", baselineRuntime.StopReason, ParseStopAccepted, baselineRuntime.Summary())
	}
	needed := baselineRuntime.NodesAllocated
	if needed < 4 {
		t.Fatalf("baseline NodesAllocated = %d, too small to construct a meaningful override", needed)
	}
	override := (needed + 1) / 2
	widened := override * 2

	prevOverride := parseNodeLimitTestOverride
	parseNodeLimitTestOverride = override
	t.Cleanup(func() { parseNodeLimitTestOverride = prevOverride })

	t.Run("Parse (production DFA route)", func(t *testing.T) {
		parser := NewParser(lang)
		parser.SetAdmissionCandidateRoute(false)
		tree, err := parser.Parse(source)
		if err != nil {
			t.Fatalf("Parse error: %v", err)
		}
		if tree == nil {
			t.Fatal("Parse returned nil tree")
		}
		defer tree.Release()
		runtime := tree.ParseRuntime()
		root := tree.RootNode()
		if runtime.StopReason != ParseStopAccepted {
			t.Fatalf("StopReason = %q, want %q (override=%d widened=%d, runtime=%s)", runtime.StopReason, ParseStopAccepted, override, widened, runtime.Summary())
		}
		if root == nil {
			t.Fatal("accepted tree has nil root")
		}
		if got, want := root.EndByte(), uint32(len(source)); got != want {
			t.Fatalf("root end = %d, want %d (full span); runtime=%s", got, want, runtime.Summary())
		}
		if root.HasError() {
			t.Fatalf("root has error, want clean widened parse; runtime=%s", runtime.Summary())
		}
		if runtime.NodeLimit != widened {
			t.Fatalf("ParseRuntime().NodeLimit = %d, want widened limit %d (override=%d); runtime=%s", runtime.NodeLimit, widened, override, runtime.Summary())
		}
	})

	t.Run("ParseWithTokenSource (caller token-source route)", func(t *testing.T) {
		parser := NewParser(lang)
		ts := parser.acquireParserDFATokenSource(source)
		tree, err := parser.ParseWithTokenSource(source, ts)
		if err != nil {
			t.Fatalf("ParseWithTokenSource error: %v", err)
		}
		if tree == nil {
			t.Fatal("ParseWithTokenSource returned nil tree")
		}
		defer tree.Release()
		runtime := tree.ParseRuntime()
		root := tree.RootNode()
		if runtime.StopReason != ParseStopAccepted {
			t.Fatalf("StopReason = %q, want %q (override=%d widened=%d, runtime=%s)", runtime.StopReason, ParseStopAccepted, override, widened, runtime.Summary())
		}
		if root == nil {
			t.Fatal("accepted tree has nil root")
		}
		if got, want := root.EndByte(), uint32(len(source)); got != want {
			t.Fatalf("root end = %d, want %d (full span); runtime=%s", got, want, runtime.Summary())
		}
		if root.HasError() {
			t.Fatalf("root has error, want clean widened parse; runtime=%s", runtime.Summary())
		}
		if runtime.NodeLimit != widened {
			t.Fatalf("ParseRuntime().NodeLimit = %d, want widened limit %d (override=%d); runtime=%s", runtime.NodeLimit, widened, override, runtime.Summary())
		}
	})
}

// TestFreshParseExplicitNodeLimitNeverWidens is the Defect 1 companion case:
// when the caller sets an explicit ParseWorkLimits.NodeLimit, that is a
// deterministic contract (SetParseWorkLimits' own doc comment). The parse
// still stops with ParseStopNodeLimit, but the retry ladder must never widen
// past the caller's own configured value.
func TestFreshParseExplicitNodeLimitNeverWidens(t *testing.T) {
	lang := buildArithmeticLanguage()
	source := buildArithmeticPlusChain(60)

	baseline := NewParser(lang)
	baseline.SetAdmissionCandidateRoute(false)
	baselineTree, err := baseline.Parse(source)
	if err != nil {
		t.Fatalf("baseline Parse error: %v", err)
	}
	needed := baselineTree.ParseRuntime().NodesAllocated
	baselineTree.Release()

	explicitLimit := (needed + 1) / 2
	parser := NewParser(lang)
	parser.SetParseWorkLimits(ParseWorkLimits{NodeLimit: explicitLimit})
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
	defer tree.Release()
	runtime := tree.ParseRuntime()
	if runtime.StopReason != ParseStopNodeLimit {
		t.Fatalf("StopReason = %q, want %q (explicit limit=%d, runtime=%s)", runtime.StopReason, ParseStopNodeLimit, explicitLimit, runtime.Summary())
	}
	if runtime.NodeLimit != explicitLimit {
		t.Fatalf("ParseRuntime().NodeLimit = %d, want unwidened explicit limit %d; runtime=%s", runtime.NodeLimit, explicitLimit, runtime.Summary())
	}
}

// --- Defect 2: a stopped-early tree must report HasError()==true when its
// root does not cover the whole source. ---

// truncatedIterationLimitTree runs a small deterministic parse with a tiny
// IterationLimit, producing a real (nonzero, non-full-span) prefix root and
// ParseStopIterationLimit.
func truncatedIterationLimitTree(t *testing.T, admissionCandidateRoute bool) (*Tree, []byte) {
	t.Helper()
	lang := buildArithmeticLanguage()
	source := buildArithmeticPlusChain(60)
	parser := NewParser(lang)
	parser.SetAdmissionCandidateRoute(admissionCandidateRoute)
	parser.SetParseWorkLimits(ParseWorkLimits{IterationLimit: 5})
	tree, err := parser.Parse(source)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if tree == nil {
		t.Fatal("Parse returned nil tree")
	}
	return tree, source
}

func TestStoppedEarlyTruncatedTreeReportsHasError(t *testing.T) {
	for _, route := range []struct {
		name    string
		enabled bool
	}{
		{"production-pinned (SetAdmissionCandidateRoute(false))", false},
		{"default route", true},
	} {
		t.Run(route.name, func(t *testing.T) {
			tree, source := truncatedIterationLimitTree(t, route.enabled)
			defer tree.Release()

			if !tree.ParseStoppedEarly() {
				t.Fatalf("ParseStoppedEarly() = false, want true; runtime=%s", tree.ParseRuntime().Summary())
			}
			root := tree.RootNode()
			if root == nil {
				t.Fatal("stopped-early tree has nil root")
			}
			if got, want := root.EndByte(), uint32(len(source)); got >= want {
				t.Fatalf("root end = %d, want < %d (a real dropped tail); runtime=%s", got, want, tree.ParseRuntime().Summary())
			}
			if root.EndByte() == 0 {
				t.Fatalf("root end = 0, want a genuine nonzero clean prefix; runtime=%s", tree.ParseRuntime().Summary())
			}
			if !root.HasError() {
				t.Fatalf("HasError() = false, want true for a truncated stopped-early tree; runtime=%s", tree.ParseRuntime().Summary())
			}
			if !root.HasErrorOrMissing() {
				t.Fatalf("HasErrorOrMissing() = false, want true to agree with HasError(); runtime=%s", tree.ParseRuntime().Summary())
			}
		})
	}
}
