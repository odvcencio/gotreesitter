package gotreesitter

import (
	"sync/atomic"
	"time"
)

type parseStopCheck func() ParseStopReason

type parseStopPoller struct {
	check parseStopCheck
	count uint64
	// memoryBudgetParser, when non-nil, makes poll/pollNow also force a
	// runtime memory-budget check (see (*Parser).compatRuntimeMemoryBudgetStopReason)
	// at the same coarse cadence as check, in addition to whatever check
	// reports. check alone only ever reports Timeout/Cancelled (see
	// parseStopReasonIsActive) — some long tree walks (the Go compat walk,
	// see normalizeGoCompatibilityInRangesWithStopAndScratch; and the JS/TS
	// fused compat walk, see rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex)
	// need to also bound their own runtime heap growth even when the parse
	// loop itself never tripped the budget, so they opt in by setting this
	// field. nil by default: every existing caller that only sets check
	// keeps its exact current behavior, byte-for-byte, since the branches
	// below are additive and only ever taken when this field is non-nil.
	memoryBudgetParser *Parser
}

const parseStopPollMask = 1023

func (p *parseStopPoller) poll() ParseStopReason {
	if p == nil {
		return ParseStopNone
	}
	if p.check == nil && p.memoryBudgetParser == nil {
		return ParseStopNone
	}
	p.count++
	if p.count&parseStopPollMask != 0 {
		return ParseStopNone
	}
	return p.pollNow()
}

func (p *parseStopPoller) pollNow() ParseStopReason {
	if p == nil {
		return ParseStopNone
	}
	if p.check != nil {
		// check() (always (*Parser).activeParseStopReason) never returns a Go
		// empty string; its "nothing happened" sentinel is the ParseStopNone
		// constant ("none"). Comparing against "" here would make this branch
		// always return early — including the "none" case — and starve the
		// memoryBudgetParser check below of ever running when check is set,
		// which every Go compat caller does.
		if reason := p.check(); reason != ParseStopNone && reason != "" {
			return reason
		}
	}
	if p.memoryBudgetParser != nil {
		if reason := p.memoryBudgetParser.compatRuntimeMemoryBudgetStopReason(); reason == ParseStopMemoryBudget {
			return reason
		}
	}
	return ParseStopNone
}

func parseStopReasonIsActive(reason ParseStopReason) bool {
	switch reason {
	case ParseStopTimeout, ParseStopCancelled:
		return true
	default:
		return false
	}
}

func (p *Parser) enterParseBudget() func() {
	if !p.needsParseBudget() {
		return func() {}
	}
	return p.enterParseBudgetAt(time.Now())
}

func (p *Parser) enterParseBudgetAt(start time.Time) func() {
	if p == nil {
		return func() {}
	}
	if !p.needsParseBudget() {
		return func() {}
	}
	prevDepth := p.parseBudgetDepth
	prevDeadline := p.parseDeadline
	prevStopped := p.parseStoppedReason

	if prevDepth == 0 {
		p.parseStoppedReason = ParseStopNone
		if p.timeoutMicros > 0 {
			p.parseDeadline = start.Add(time.Duration(p.timeoutMicros) * time.Microsecond)
		} else {
			p.parseDeadline = time.Time{}
		}
	}
	p.parseBudgetDepth = prevDepth + 1

	return func() {
		p.parseBudgetDepth = prevDepth
		p.parseDeadline = prevDeadline
		p.parseStoppedReason = prevStopped
	}
}

func (p *Parser) needsParseBudget() bool {
	return p != nil && (p.parseBudgetDepth > 0 || p.timeoutMicros > 0 || p.cancellationFlag != nil)
}

func (p *Parser) activeParseStopCheck() parseStopCheck {
	if p == nil {
		return nil
	}
	if p.activeParseStopCheckFn == nil {
		p.activeParseStopCheckFn = p.activeParseStopReason
	}
	return p.activeParseStopCheckFn
}

func (p *Parser) activeParseStopReason() ParseStopReason {
	if p == nil {
		return ParseStopNone
	}
	if !p.needsParseBudget() {
		return ParseStopNone
	}
	if parseStopReasonIsActive(p.parseStoppedReason) {
		return p.parseStoppedReason
	}
	if flag := p.cancellationFlag; flag != nil && atomic.LoadUint32(flag) != 0 {
		return p.markActiveParseStopped(ParseStopCancelled)
	}
	if !p.parseDeadline.IsZero() && !time.Now().Before(p.parseDeadline) {
		return p.markActiveParseStopped(ParseStopTimeout)
	}
	return ParseStopNone
}

func (p *Parser) markActiveParseStopped(reason ParseStopReason) ParseStopReason {
	if p == nil || !parseStopReasonIsActive(reason) {
		return ParseStopNone
	}
	if !parseStopReasonIsActive(p.parseStoppedReason) {
		p.parseStoppedReason = reason
	}
	return p.parseStoppedReason
}

func (p *Parser) remainingTimeoutMicros() uint64 {
	if p == nil || p.timeoutMicros == 0 {
		return 0
	}
	if p.parseBudgetDepth == 0 || p.parseDeadline.IsZero() {
		return p.timeoutMicros
	}
	remaining := time.Until(p.parseDeadline)
	if remaining <= 0 {
		p.markActiveParseStopped(ParseStopTimeout)
		return 1
	}
	micros := remaining / time.Microsecond
	if micros <= 0 {
		return 1
	}
	return uint64(micros)
}
