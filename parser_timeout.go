package gotreesitter

import (
	"sync/atomic"
	"time"
)

type parseStopCheck func() ParseStopReason

type parseStopPoller struct {
	check parseStopCheck
	count uint64
}

const parseStopPollMask = 1023

func (p *parseStopPoller) poll() ParseStopReason {
	if p == nil || p.check == nil {
		return ParseStopNone
	}
	p.count++
	if p.count&parseStopPollMask != 0 {
		return ParseStopNone
	}
	return p.pollNow()
}

func (p *parseStopPoller) pollNow() ParseStopReason {
	if p == nil || p.check == nil {
		return ParseStopNone
	}
	reason := p.check()
	if reason == "" {
		return ParseStopNone
	}
	return reason
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
	return p.activeParseStopReason
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

func (p *Parser) applyActiveStopReasonToTree(tree *Tree) {
	if p == nil || tree == nil {
		return
	}
	reason := p.activeParseStopReason()
	if !parseStopReasonIsActive(reason) {
		return
	}
	rt := tree.ParseRuntime()
	if parseStopReasonIsActive(rt.StopReason) {
		return
	}
	rt.StopReason = reason
	tree.setParseRuntime(rt)
}
