package gotreesitter

import "testing"

func TestNestedParsePreservesRetryPasses(t *testing.T) {
	p := NewParser(buildArithmeticLanguage())
	operation := p.beginParseOperationBudget()
	defer p.endParseOperationBudget(operation)
	p.fullParseRetryPassesTaken = 7
	tree, err := p.Parse([]byte("1+2"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if p.fullParseRetryPassesTaken != 7 {
		t.Fatalf("nested Parse reset retry passes to %d", p.fullParseRetryPassesTaken)
	}
}

func TestRecoverySnippetSharesRetryBudget(t *testing.T) {
	p := NewParser(buildArithmeticLanguage())
	operation := p.beginParseOperationBudget()
	defer p.endParseOperationBudget(operation)
	p.fullParseRetryPassesTaken = fullParseRetryMaxTotalPasses
	budget := p.fullParseOperationRetryBudget()
	inherited := false
	p.reparseFactory = func(source []byte) (TokenSource, error) {
		child := p.recoveryParser
		inherited = child != nil && child.fullParseRetryBudgetExhausted(fullParseRetryMaxTotalPasses)
		if child != nil && child.takeFullParseRetryPass(fullParseRetryMaxTotalPasses) {
			t.Fatal("snippet admitted a retry beyond the parent ceiling")
		}
		return p.acquireParserDFATokenSource(source), nil
	}
	tree, err := p.parseForRecovery([]byte("1+2"))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Release()
	if child := p.recoveryParser; !inherited || child == nil || child.fullParseRetryPassesTaken != 0 {
		t.Fatal("recovery parser lost parent quota or claimed a retry")
	}
	if budget.Passes != fullParseRetryMaxTotalPasses || p.takeFullParseRetryPass(fullParseRetryMaxTotalPasses) {
		t.Fatalf("recovery parse renewed budget: %+v", budget)
	}
}

func TestInheritedRetryBudgetChargesParentAndRestoresPool(t *testing.T) {
	parent := NewParser(buildArithmeticLanguage())
	operation := parent.beginParseOperationBudget()
	parent.seedFullParseRetryWorkBudget(100, 3, 8) // no policy: no work limit
	budget := parent.fullParseOperationRetryBudget()
	budget.Seed(100, 3, 8)
	child := acquireSnippetParser(parent.language)
	end := child.inheritFullParseRetryBudget(parent)
	childOperation := child.beginParseOperationBudget()
	if !child.takeFullParseRetryPass(24) {
		t.Fatal("child retry denied")
	}
	child.fullParseOperationRetryBudget().Charge(100, 4)
	child.seedFullParseRetryWorkBudget(1000, 7, 8)
	child.endParseOperationBudget(childOperation)
	end()
	if budget.Passes != 1 || budget.Remaining != 200 || parent.fullParseRetryPassesTaken != 1 {
		t.Fatalf("child failed to debit parent: %+v passes=%d", budget, parent.fullParseRetryPassesTaken)
	}
	releaseSnippetParser(child)
	if child.forestDeclineMemo.retryBudget != nil {
		t.Fatal("pooled parser retained parent budget")
	}
	parent.endParseOperationBudget(operation)
	next := parent.beginParseOperationBudget()
	defer parent.endParseOperationBudget(next)
	if parent.fullParseRetryPassesTaken != 0 || parent.fullParseRetryBudgetExhausted(24) {
		t.Fatal("independent parse did not renew retry allowance")
	}
}

func TestStackCapSkipsEveryRetryRung(t *testing.T) {
	p := &Parser{language: &Language{FullParseRetryWorkBudgetEnabled: true}}
	p.seedFullParseRetryWorkBudget(100, 8, 8)
	first := duplicateWideRetryTestTree(4096, 100, 8, false)
	var calls int
	got := p.retryFullParse(make([]byte, 4096), 8, first, func(int, int, int) *Tree { calls++; return nil })
	if got != first || calls != 0 || p.fullParseRetryPassesTaken != 0 {
		t.Fatalf("stack-cap result retried: calls=%d passes=%d", calls, p.fullParseRetryPassesTaken)
	}
}
