package gotreesitter

import "testing"

func TestTokenInvariantDeferredBudgetDoesNotPublishHistory(t *testing.T) {
	root, lang, source := buildTypeScriptCompatWideDynamicImportTree(8)
	tree := newTreeWithArenas(root, source, lang, root.ownerArena, nil)
	defer tree.Release()
	tree.setParseRuntime(ParseRuntime{StopReason: ParseStopAccepted})
	tree.deferResultCompatibility()
	tree.captureTokenInvariantReadSpanValue(17)
	if tree.tokenInvariantReadSpan != 0 || tree.resultCompatibilityFinalizer.tokenInvariantReadSpan != 17 {
		t.Fatal("deferred history was not kept private")
	}
	root.ownerArena.setBudget(1)
	// Force growth instead of consuming capacity allocated before the budget.
	width := 1
	for _, slab := range root.ownerArena.childSlabs {
		width = max(width, len(slab.data)+1)
	}
	root.ownerArena.allocNodeSlice(width)
	if !root.ownerArena.budgetExhausted() {
		t.Fatal("fixture did not exceed its normalization budget")
	}
	tree.ensureResultCompatibility()
	if tree.resultCompatibilityApplied {
		t.Fatal("budget-stopped normalization was marked complete")
	}
	if tree.tokenInvariantReadSpan != 0 || tree.resultCompatibilityFinalizer.tokenInvariantReadSpan != 0 {
		t.Fatal("budget-stopped normalization published lexical history")
	}
	tree.ensureResultCompatibility()
	if tree.tokenInvariantReadSpan != 0 {
		t.Fatal("repeated access revived rejected history")
	}
}
