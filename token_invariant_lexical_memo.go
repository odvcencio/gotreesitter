package gotreesitter

import "github.com/odvcencio/gotreesitter/internal/incr/lexproof"

type tokenInvariantPrimitiveMemoContext struct {
	language                       *Language
	states                         *LexState
	stateCount                     int
	ascii                          *[128]int32
	asciiCount                     int
	immediate, zeroWidth           *bool
	immediateCount, zeroWidthCount int
	scanner                        lexproof.Identity
	scannerEquivalent              bool
}

type tokenInvariantPrimitiveMemo struct {
	context tokenInvariantPrimitiveMemoContext
	proofs  lexproof.Memo
}

// A loaded Language's tables are immutable, like the existing ASCII, keyword,
// and compact-table caches. Custom table producers continue to run the proof.
// Stateless scanners additionally supply an exact snapshot of their receiver
// parameters; opaque or wide parameters cannot use the memo.
func tokenInvariantPrimitiveMemoContextFor(d *dfaTokenSource, scannerEquivalent bool) (tokenInvariantPrimitiveMemoContext, bool) {
	if d == nil || d.language == nil || d.lexer == nil || !d.language.grammarBlobSHA256Valid || len(d.lexer.includedRanges) != 0 {
		return tokenInvariantPrimitiveMemoContext{}, false
	}
	if !lexproof.SameStorage(d.lexer.states, d.language.LexStates) ||
		!lexproof.SameStorage(d.lexer.asciiTable, d.language.lexAsciiTable) ||
		!lexproof.SameStorage(d.lexer.immediateTokens, d.language.ImmediateTokens) ||
		!lexproof.SameStorage(d.lexer.zeroWidthTokens, d.language.ZeroWidthTokens) {
		return tokenInvariantPrimitiveMemoContext{}, false
	}
	c := tokenInvariantPrimitiveMemoContext{
		language: d.language, stateCount: len(d.lexer.states), asciiCount: len(d.lexer.asciiTable),
		immediateCount: len(d.lexer.immediateTokens), zeroWidthCount: len(d.lexer.zeroWidthTokens),
		scannerEquivalent: scannerEquivalent,
	}
	if len(d.lexer.states) != 0 {
		c.states = &d.lexer.states[0]
	}
	if len(d.lexer.asciiTable) != 0 {
		c.ascii = &d.lexer.asciiTable[0]
	}
	if len(d.lexer.immediateTokens) != 0 {
		c.immediate = &d.lexer.immediateTokens[0]
	}
	if len(d.lexer.zeroWidthTokens) != 0 {
		c.zeroWidth = &d.lexer.zeroWidthTokens[0]
	}
	if d.language.ExternalScanner != nil && !scannerEquivalent {
		scanner, ok := d.language.ExternalScanner.(StatelessExternalScanner)
		if !ok || !scanner.ExternalScannerIsStateless() {
			return tokenInvariantPrimitiveMemoContext{}, false
		}
		var supported bool
		c.scanner, supported = lexproof.Snapshot(d.language.ExternalScanner)
		if !supported {
			return tokenInvariantPrimitiveMemoContext{}, false
		}
	}
	return c, true
}

func (p *Parser) tokenInvariantPrimitiveProofCached(d *dfaTokenSource, oldSource, newSource []byte, edit InputEdit, inputSpan uint32, scannerEquivalent bool) (uint32, bool, bool) {
	context, cacheable := tokenInvariantPrimitiveMemoContextFor(d, scannerEquivalent)
	// Explicit memory budgets keep their existing accounting. The optional,
	// fixed-size cache is never allocated or consulted under such a budget.
	cacheable = cacheable && p != nil && p.MemoryBudgetBytes() == 0
	key := lexproof.Edit{
		Start: edit.StartByte, End: edit.OldEndByte,
		Row: edit.StartPoint.Row, Column: edit.StartPoint.Column,
		EndRow: edit.OldEndPoint.Row, EndColumn: edit.OldEndPoint.Column,
	}
	cacheable = cacheable && edit.NewEndByte == edit.OldEndByte && edit.NewEndPoint == edit.OldEndPoint
	if cacheable && p.forestDeclineMemo != nil && p.forestDeclineMemo.tokenInvariantPrimitiveMemo != nil {
		memo := p.forestDeclineMemo.tokenInvariantPrimitiveMemo
		if memo.context == context {
			if span, ok := memo.proofs.Lookup(oldSource, newSource, key, inputSpan); ok {
				return span, true, true
			}
		}
	}
	span, proven := d.tokenInvariantPrimitiveEditsEquivalentWithScannerProof(oldSource, newSource, edit, inputSpan, scannerEquivalent)
	if !cacheable || !proven {
		return span, proven, false
	}
	// Do not allocate a sidecar for a proof whose window cannot be retained.
	var first lexproof.Memo
	if !first.Remember(oldSource, newSource, key, inputSpan, span) {
		return span, true, false
	}
	cold := p.ensureParserColdState()
	if cold.tokenInvariantPrimitiveMemo == nil {
		cold.tokenInvariantPrimitiveMemo = &tokenInvariantPrimitiveMemo{context: context, proofs: first}
	} else if cold.tokenInvariantPrimitiveMemo.context != context {
		*cold.tokenInvariantPrimitiveMemo = tokenInvariantPrimitiveMemo{context: context, proofs: first}
	} else {
		cold.tokenInvariantPrimitiveMemo.proofs.Remember(oldSource, newSource, key, inputSpan, span)
	}
	return span, true, false
}
