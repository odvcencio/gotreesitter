package gotreesitter

import (
	"slices"
	"unicode/utf8"
)

// A proof pays for both source versions. Exhaustion declines before reuse.
type tokenInvariantPrimitiveBudget struct{ bytes, scans uint32 }

func (b *tokenInvariantPrimitiveBudget) charge(span uint32) bool {
	if b.scans == 0 || span > b.bytes {
		return false
	}
	b.scans--
	b.bytes -= span
	return true
}

func (b *tokenInvariantPrimitiveBudget) chargeBytes(span uint32) bool {
	if span > b.bytes {
		return false
	}
	b.bytes -= span
	return true
}

func (d *dfaTokenSource) tokenInvariantPrimitiveEditsEquivalent(oldSource, newSource []byte, edit InputEdit, maxReadSpan uint32) (uint32, bool) {
	return d.tokenInvariantPrimitiveEditsEquivalentWithScannerProof(oldSource, newSource, edit, maxReadSpan, false)
}

func (d *dfaTokenSource) tokenInvariantPrimitiveEditsEquivalentWithScannerProof(oldSource, newSource []byte, edit InputEdit, maxReadSpan uint32, scannerEquivalent bool) (uint32, bool) {
	if d == nil || d.lexer == nil || d.language == nil || len(d.lexer.includedRanges) != 0 ||
		len(oldSource) != len(newSource) || edit.OldEndByte != edit.NewEndByte || edit.StartByte >= edit.OldEndByte ||
		uint64(edit.OldEndByte) > uint64(len(oldSource)) || edit.OldEndPoint != edit.NewEndPoint || maxReadSpan == 0 {
		return 0, false
	}
	budget := tokenInvariantPrimitiveBudget{32768, 2048}
	if !budget.chargeBytes(uint32(2 * len(utf8BOM))) {
		return 0, false
	}
	oldBOM, newBOM := Lexer{source: oldSource}, Lexer{source: newSource}
	oldBOM.skipLeadingBOM()
	newBOM.skipLeadingBOM()
	if oldBOM.pos != newBOM.pos || oldBOM.col != newBOM.col {
		return 0, false
	}
	if !tokenInvariantWhitespaceGatesEquivalent(oldSource, newSource, edit, &budget) {
		return 0, false
	}
	var modeStorage [1024]uint32
	modes := modeStorage[:0]
	// The lexer can use state zero for a fallback scan without a mode row.
	if len(d.lexer.states) != 0 {
		modes = append(modes, 0)
	}
	if len(d.language.LexModes) > 32768 {
		return 0, false
	}
	for _, mode := range d.language.LexModes {
		for index, state := range [...]uint32{mode.LexStateIndex(), mode.AfterWhitespaceLexStateIndex()} {
			if index == 1 && state == 0 {
				continue
			}
			if state == noLookaheadLexState {
				continue
			}
			if uint64(state) >= uint64(len(d.lexer.states)) {
				return 0, false
			}
			if !slices.Contains(modes, state) {
				if len(modes) == len(modeStorage) {
					return 0, false
				}
				modes = append(modes, state)
			}
		}
		if d.language.ExternalScanner != nil && !scannerEquivalent {
			if int(mode.ExternalLexState) >= len(d.language.ExternalLexStates) {
				return 0, false
			}
			if len(d.language.ExternalLexStates[mode.ExternalLexState]) > len(d.language.ExternalSymbols) {
				return 0, false
			}
		}
	}
	if len(modes) == 0 {
		return 0, false
	}
	maskCount := 0
	var externalPayload any
	var externalScratch ExternalLexer
	var maskStorage [8]bool
	if d.language.ExternalScanner != nil && !scannerEquivalent {
		scanner, ok := d.language.ExternalScanner.(StatelessExternalScanner)
		if !ok || !scanner.ExternalScannerIsStateless() || len(d.language.ExternalSymbols) > len(maskStorage) {
			return 0, false
		}
		// Retry masks and GLR unions need not occur in ExternalLexStates.
		// Enumerate the complete bounded mask space, including the empty mask.
		maskCount = 1 << uint(len(d.language.ExternalSymbols))
		externalPayload = scanner.Create()
		defer scanner.Destroy(externalPayload)
	}
	origin := uint32(0)
	if edit.StartByte > maxReadSpan {
		origin = edit.StartByte - maxReadSpan
	}
	// Derive the initial point from the supplied edit point, not a full-file
	// scan. A previous-line search also consumes the bounded byte allowance.
	point := edit.StartPoint
	for i := edit.StartByte; i > origin; i-- {
		if budget.bytes == 0 {
			return 0, false
		}
		budget.bytes--
		if oldSource[i-1] == '\n' {
			if point.Row == 0 {
				return 0, false
			}
			point.Row--
			column := uint32(0)
			for j := i - 1; j > 0 && oldSource[j-1] != '\n'; j-- {
				if budget.bytes == 0 {
					return 0, false
				}
				budget.bytes--
				column++
			}
			point.Column = column
		} else {
			if point.Column == 0 {
				return 0, false
			}
			point.Column--
		}
	}
	oldPoint, newPoint := point, point
	maximum := maxReadSpan
	// Keyword selection depends on its bounded source slice, not the LR
	// mode that returned that slice. Cache only completed equal comparisons.
	var keywordSpans [16][2]uint32
	keywordSpanCount := 0
	for ; origin < edit.OldEndByte; origin++ {
		for _, mode := range modes {
			oldToken, oldLex, oldOK, ok := d.tokenInvariantProbeDFALimited(oldSource, origin, oldPoint, mode, &budget, maxReadSpan)
			if !ok {
				return 0, false
			}
			// Complete old coverage proves that a wider hypothetical scan
			// never executed. Do not let unreachable modes inflate new coverage.
			if oldToken.lexerLookaheadEndByte-origin > maxReadSpan {
				continue
			}
			// The deterministic raw scan cannot change when every byte it
			// examined precedes the edit. Its token and keyword slice also
			// precede the edit, so no duplicate scan or text check is needed.
			if oldToken.lexerLookaheadEndByte <= edit.StartByte {
				continue
			}
			newToken, newLex, newOK, ok := d.tokenInvariantProbeDFA(newSource, origin, newPoint, mode, &budget)
			if !ok || oldOK != newOK || !tokenInvariantPrimitiveTokensEqual(oldToken, newToken) ||
				oldLex.pos != newLex.pos || oldLex.row != newLex.row || oldLex.col != newLex.col ||
				oldLex.failTokenStartPos != newLex.failTokenStartPos || oldLex.failTokenStartRow != newLex.failTokenStartRow || oldLex.failTokenStartCol != newLex.failTokenStartCol {
				return 0, false
			}
			maximum = maxUint32(maximum, newToken.lexerLookaheadEndByte-origin)
			if oldOK && !d.tokenInvariantLiteralCandidatesEquivalent(oldToken, newToken, &budget) {
				return 0, false
			}
			if oldOK && oldToken.Symbol == d.language.KeywordCaptureToken && oldToken.EndByte > oldToken.StartByte && len(d.language.KeywordLexStates) != 0 {
				key := [2]uint32{oldToken.StartByte, oldToken.EndByte}
				if !budget.chargeBytes(uint32(keywordSpanCount)) {
					return 0, false
				}
				if slices.Contains(keywordSpans[:keywordSpanCount], key) {
					continue
				}
				if !budget.chargeBytes(1) || !budget.chargeBytes(1) {
					return 0, false
				}
				oldPrefilter := d.language.keywordLexCouldMatch(oldSource, int(oldToken.StartByte), int(oldToken.EndByte))
				newPrefilter := d.language.keywordLexCouldMatch(newSource, int(newToken.StartByte), int(newToken.EndByte))
				if oldPrefilter != newPrefilter {
					return 0, false
				}
				var tokens [2]Token
				var accepted [2]bool
				for index, source := range [][]byte{oldSource, newSource} {
					width := oldToken.EndByte - oldToken.StartByte
					if budget.scans == 0 || uint64(width)+5 > uint64(budget.bytes) {
						return 0, false
					}
					probe := dfaTokenSource{language: d.language}
					tokens[index], accepted[index] = probe.lexKeywordSource(source[oldToken.StartByte:oldToken.EndByte])
					if !budget.charge(probe.tokenInvariantReadSpan()) {
						return 0, false
					}
					if index == 1 {
						maximum = maxUint32(maximum, probe.tokenInvariantReadSpan())
					}
				}
				if accepted[0] != accepted[1] || !tokenInvariantPrimitiveTokensEqual(tokens[0], tokens[1]) {
					return 0, false
				}
				if keywordSpanCount < len(keywordSpans) {
					keywordSpans[keywordSpanCount] = key
					keywordSpanCount++
				}
			}
		}
		for bits := 0; bits < maskCount; bits++ {
			mask := maskStorage[:len(d.language.ExternalSymbols)]
			for index := range mask {
				mask[index] = bits&(1<<uint(index)) != 0
			}
			oldLex, oldOK, oldSpan, ok := d.tokenInvariantProbeExternal(oldSource, origin, oldPoint, mask, &budget, externalPayload, &externalScratch, maxReadSpan)
			if !ok {
				return 0, false
			}
			if oldSpan > maxReadSpan || uint64(origin)+uint64(oldSpan) <= uint64(edit.StartByte) {
				continue
			}
			newLex, newOK, newSpan, ok := d.tokenInvariantProbeExternal(newSource, origin, newPoint, mask, &budget, externalPayload, &externalScratch, 0)
			if !ok || oldOK != newOK {
				return 0, false
			}
			maximum = maxUint32(maximum, newSpan)
			if oldLex.startPos != newLex.startPos || oldLex.pos != newLex.pos || oldLex.endPos != newLex.endPos ||
				oldLex.startPoint != newLex.startPoint || oldLex.point != newLex.point || oldLex.endPoint != newLex.endPoint ||
				oldLex.endMarked != newLex.endMarked || oldLex.advancedContent != newLex.advancedContent ||
				oldLex.resultSymbol != newLex.resultSymbol || oldLex.hasResult != newLex.hasResult {
				return 0, false
			}
		}
		if oldSource[origin] == '\n' {
			oldPoint.Row++
			oldPoint.Column = 0
		} else {
			oldPoint.Column++
		}
		if newSource[origin] == '\n' {
			newPoint.Row++
			newPoint.Column = 0
		} else {
			newPoint.Column++
		}
	}
	return maximum, true
}

func tokenInvariantWhitespaceGatesEquivalent(oldSource, newSource []byte, edit InputEdit, budget *tokenInvariantPrimitiveBudget) bool {
	start := uint32(0)
	if edit.StartByte > utf8.UTFMax {
		start = edit.StartByte - utf8.UTFMax
	}
	end := min(uint64(len(oldSource)), uint64(edit.OldEndByte)+utf8.UTFMax)
	oldLexer, newLexer := Lexer{source: oldSource}, Lexer{source: newSource}
	oldProbe, newProbe := dfaTokenSource{lexer: &oldLexer}, dfaTokenSource{lexer: &newLexer}
	for position := uint64(start); position <= end; position++ {
		// Each helper decodes at most one UTF-8 rune. Include boundaries
		// after the edit because the prefix decoder can inspect changed bytes.
		if !budget.chargeBytes(2*utf8.UTFMax) || !budget.chargeBytes(2*utf8.UTFMax) {
			return false
		}
		oldLexer.pos, newLexer.pos = int(position), int(position)
		if oldProbe.isAtWhitespacePosition() != newProbe.isAtWhitespacePosition() ||
			oldProbe.isAfterWhitespacePosition() != newProbe.isAfterWhitespacePosition() {
			return false
		}
	}
	return true
}

func (d *dfaTokenSource) tokenInvariantLiteralCandidatesEquivalent(oldToken, newToken Token, budget *tokenInvariantPrimitiveBudget) bool {
	// Compare the whole ordered candidate signature, independent of active
	// parser states. Equal candidates preserve every action-filtered election.
	if uint64(len(oldToken.Text))+uint64(len(newToken.Text)) > uint64(budget.bytes) ||
		!budget.chargeBytes(uint32(len(oldToken.Text))) || !budget.chargeBytes(uint32(len(newToken.Text))) {
		return false
	}
	oldCandidates := d.language.TokenSymbolsByName(oldToken.Text)
	newCandidates := d.language.TokenSymbolsByName(newToken.Text)
	if !budget.chargeBytes(uint32(len(oldCandidates))) || !budget.chargeBytes(uint32(len(newCandidates))) || !slices.Equal(oldCandidates, newCandidates) {
		return false
	}
	if d.language.TokenCount == 0 {
		// The keyword wrapper has a fallback for metadata-light languages.
		for symbol := uint32(1); symbol < d.language.SymbolCount && uint64(symbol) < uint64(len(d.language.SymbolNames)); symbol++ {
			name := d.language.SymbolNames[symbol]
			if uint64(len(name))*2 > uint64(budget.bytes) || !budget.chargeBytes(uint32(len(name))) || !budget.chargeBytes(uint32(len(name))) {
				return false
			}
			if (name == oldToken.Text) != (name == newToken.Text) {
				return false
			}
		}
	}
	if len(oldCandidates) == 0 {
		return true
	}
	if !budget.chargeBytes(uint32(len(oldToken.Text))) || !budget.chargeBytes(uint32(len(newToken.Text))) {
		return false
	}
	return isIdentifierLikeLiteralText(oldToken.Text) == isIdentifierLikeLiteralText(newToken.Text) &&
		d.language.anonymousTokenNameShapePossible(oldToken.Text) == d.language.anonymousTokenNameShapePossible(newToken.Text)
}

func tokenInvariantPrimitiveTokensEqual(a, b Token) bool {
	a.Text, b.Text = "", ""
	a.lexerLookaheadEndByte, b.lexerLookaheadEndByte = 0, 0
	return a == b
}

func (d *dfaTokenSource) tokenInvariantProbeDFA(source []byte, origin uint32, point Point, mode uint32, budget *tokenInvariantPrimitiveBudget) (Token, Lexer, bool, bool) {
	return d.tokenInvariantProbeDFALimited(source, origin, point, mode, budget, 0)
}

func tokenInvariantProbeLimit(source []byte, origin uint32, budget *tokenInvariantPrimitiveBudget, oldMaximum uint32) (uint64, bool) {
	limit := min(uint64(len(source)), uint64(origin)+uint64(budget.bytes))
	// Keep a full UTF-8 decoding margin beyond the exclusion boundary.
	// A cut there can only affect scans already wider than old coverage.
	proofLimit := uint64(origin) + uint64(oldMaximum) + 1 + utf8.UTFMax
	proofCut := oldMaximum != 0 && proofLimit < uint64(len(source)) && proofLimit < limit
	if proofCut {
		limit = proofLimit
	}
	return limit, proofCut
}

func (d *dfaTokenSource) tokenInvariantProbeDFALimited(source []byte, origin uint32, point Point, mode uint32, budget *tokenInvariantPrimitiveBudget, oldMaximum uint32) (Token, Lexer, bool, bool) {
	if budget.scans == 0 || budget.bytes == 0 {
		return Token{}, Lexer{}, false, false
	}
	limit, proofCut := tokenInvariantProbeLimit(source, origin, budget, oldMaximum)
	probe := *d.lexer
	probe.source = source[:int(limit)]
	probe.pos, probe.row, probe.col = int(origin), point.Row, point.Column
	probe.tokenInvariantReadSpanMax = nil
	probe.failTokenStartPos, probe.failTokenStartRow, probe.failTokenStartCol, probe.failTokenStartRangeIdx = 0, 0, 0, 0
	tok, accepted := probe.scan(mode, int(origin), point.Row, point.Column)
	frontier := tokenInvariantExaminedEnd(probe.source, tok.lexerLookaheadEndByte)
	// This private token carries the proof bound, not the public C frontier.
	tok.lexerLookaheadEndByte = frontier
	if frontier < origin || (limit < uint64(len(source)) && uint64(frontier) >= limit && !proofCut) || !budget.charge(frontier-origin) {
		return Token{}, Lexer{}, false, false
	}
	return tok, probe, accepted, true
}

func (d *dfaTokenSource) tokenInvariantProbeExternal(source []byte, origin uint32, point Point, mask []bool, budget *tokenInvariantPrimitiveBudget, payload any, lexer *ExternalLexer, oldMaximum uint32) (ExternalLexer, bool, uint32, bool) {
	if budget.scans == 0 || budget.bytes == 0 {
		return ExternalLexer{}, false, 0, false
	}
	limit, proofCut := tokenInvariantProbeLimit(source, origin, budget, oldMaximum)
	lexer.reset(source[:int(limit)], int(origin), point.Row, point.Column)
	accepted := RunExternalScanner(d.language, payload, lexer, mask)
	frontier := tokenInvariantExaminedEnd(lexer.source, maxUint32(lexer.lookaheadEndByte, lexer.lookaheadEndByteAtCursor()))
	if lexer.readFrontier != nil {
		frontier = maxUint32(frontier, lexer.readFrontier.examined)
	}
	if frontier < origin || (limit < uint64(len(source)) && uint64(frontier) >= limit && !proofCut) || !budget.charge(frontier-origin) {
		return ExternalLexer{}, false, 0, false
	}
	return *lexer, accepted, frontier - origin, true
}
