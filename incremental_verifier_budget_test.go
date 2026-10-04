package gotreesitter

import (
	"testing"
	"time"
)

func TestIncrementalFreshFallbackPreservesOperationStop(t *testing.T) {
	for _, sticky := range []bool{false, true} {
		t.Run(map[bool]string{false: "expired", true: "sticky"}[sticky], func(t *testing.T) {
			p := NewParser(buildArithmeticLanguage())
			p.SetAdmissionCandidateRoute(false)
			source := []byte("1+2")
			attempt := mustParse(t, p, source)
			attempt.ensureParseRuntime().StopReason = ParseStopNodeLimit
			p.SetTimeoutMicros(1_000_000)
			budget := p.beginParseOperationBudget()
			defer p.endParseOperationBudget(budget)
			if sticky {
				p.parseStoppedReason = ParseStopTimeout
			} else {
				p.parseDeadline = time.Now().Add(-time.Second)
			}
			got := p.retryIncrementalParseAsFullWithDFA(source, 1, attempt, nil)
			defer got.Release()
			if got != attempt || got.ParseStopReason() != ParseStopNodeLimit {
				t.Fatalf("stopped operation scheduled a fresh fallback: %s", got.ParseStopReason())
			}
		})
	}
}

func TestIncrementalFreshVerifierInheritsActiveDeadline(t *testing.T) {
	p := NewParser(buildArithmeticLanguage())
	p.SetTimeoutMicros(1_000_000)
	budget := p.beginParseOperationBudget()
	defer p.endParseOperationBudget(budget)
	v := p.newIncrementalFreshVerifier()
	if v.parseDeadline != p.parseDeadline || v.parseBudgetDepth == 0 {
		t.Fatal("verifier did not inherit the active timeout scope")
	}
	v.parseDeadline = time.Now().Add(-time.Second)
	tree, err := v.Parse([]byte("1+2"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if tree.ParseStopReason() != ParseStopTimeout {
		t.Fatalf("verifier restarted its deadline: %s", tree.ParseStopReason())
	}
}
