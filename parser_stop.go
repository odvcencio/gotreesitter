package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/retrybudget"

type parseOperationBudgetState struct {
	endBudget func()
}

func (p *Parser) beginParseOperationBudget() parseOperationBudgetState {
	if p == nil {
		return parseOperationBudgetState{}
	}
	if p.cNodeMemoOperationDepth == 0 {
		if cold := p.forestDeclineMemo; cold != nil && cold.retryBudget != nil && cold.retryBudget != &cold.retryBudgetStorage {
			// The quota is shared; telemetry counts this snippet's own attempts.
			p.fullParseRetryPassesTaken = 0
		} else {
			p.fullParseRetryPassesTaken = 0
			if cold != nil {
				cold.retryBudgetStorage = retrybudget.Budget{}
				cold.retryBudget = nil
			}
		}
		// Keep retries in one operation warm. Start each independent operation
		// with the same logical cache size so parser reuse cannot change shape.
		if len(p.cNodeMemoCache) > cNodeMemoCacheInitialSize {
			p.cNodeMemoCache = p.cNodeMemoCache[:cNodeMemoCacheInitialSize]
		}
		p.cNodeMemoOperationPeakTier = RecoveryNodeMemoTierNone
		if cold := p.forestDeclineMemo; cold != nil {
			cold.cNodeMemoCollisions = 0
		}
	}
	p.cNodeMemoOperationDepth++
	state := parseOperationBudgetState{}
	if p.needsParseBudget() {
		state.endBudget = p.enterParseBudget()
	}
	return state
}

func (p *Parser) endParseOperationBudget(state parseOperationBudgetState) {
	if state.endBudget != nil {
		state.endBudget()
	}
	if p == nil {
		return
	}
	p.cNodeMemoOperationDepth--
	if p.cNodeMemoOperationDepth == 0 {
		p.finishCNodeMemoParse()
	}
}

func (p *Parser) parseStopReasonNow() ParseStopReason {
	// Parse loops arm parseBudgetDepth. Retry work can poll cancellation between budgets.
	if p == nil || (p.parseBudgetDepth == 0 && p.cancellationFlag == nil) {
		return ParseStopNone
	}
	return p.activeParseStopReason()
}

func parseStopReasonIsTerminal(reason ParseStopReason) bool {
	return parseStopReasonIsActive(reason) || reason == ParseStopInvariantViolation
}
