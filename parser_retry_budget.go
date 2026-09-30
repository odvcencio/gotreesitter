package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/retrybudget"

func (p *Parser) fullParseOperationRetryBudget() *retrybudget.Budget {
	cold := p.ensureParserColdState()
	if cold.retryBudget == nil {
		cold.retryBudgetStorage.Passes = p.fullParseRetryPassesTaken
		cold.retryBudget = &cold.retryBudgetStorage
	}
	return cold.retryBudget
}

// inheritFullParseRetryBudget keeps snippet parses in their parent's operation.
// Restore the pointer before returning a snippet parser to its pool.
func (p *Parser) inheritFullParseRetryBudget(parent *Parser) func() {
	budget := parent.fullParseOperationRetryBudget()
	cold := p.ensureParserColdState()
	previous := cold.retryBudget
	previousPasses := parent.fullParseRetryPassesTaken
	sharedPasses := budget.Passes
	cold.retryBudget = budget
	p.fullParseRetryPassesTaken = 0
	return func() {
		parent.fullParseRetryPassesTaken = previousPasses + budget.Passes - sharedPasses
		cold.retryBudget = previous
	}
}

func (p *Parser) seedFullParseRetryWorkBudget(tokens uint64, stacks, stackCap int) {
	if p.language == nil || !p.language.FullParseRetryWorkBudgetEnabled ||
		parseMaxGLRStacksEnvConfigured() || parseMaxMergePerKeyEnvConfigured() {
		return
	}
	cold := p.ensureParserColdState()
	if cold.retryBudget != nil && cold.retryBudget != &cold.retryBudgetStorage {
		// A nested parse cannot replace its parent's first-pass allowance.
		return
	}
	p.fullParseOperationRetryBudget().Seed(tokens, stacks, stackCap)
}

func (p *Parser) takeFullParseRetryPass(limit int) bool {
	budget := p.fullParseOperationRetryBudget()
	if !budget.TakePass(limit) {
		return false
	}
	p.fullParseRetryPassesTaken++
	return true
}

func (p *Parser) fullParseRetryBudgetExhausted(limit int) bool {
	if p == nil {
		return false
	}
	if p.fullParseRetryPassesTaken >= limit {
		return true
	}
	if cold := p.forestDeclineMemo; cold != nil && cold.retryBudget != nil {
		return cold.retryBudget.Exhausted(limit)
	}
	return false
}

func (p *Parser) chargeFullParseRetryWork(tree *Tree) {
	if p == nil || tree == nil {
		return
	}
	rt := tree.rawParseRuntime()
	p.fullParseOperationRetryBudget().Charge(rt.TokensConsumed, rt.MaxStacksSeen)
}
