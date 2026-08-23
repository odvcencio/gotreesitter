//go:build !grammar_subset || grammar_subset_swift

package grammars

import (
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// External token indexes for the Swift grammar (order must match grammar.js externals).
const (
	swtTokBlockComment              = iota // 0
	swtTokRawStrPart                       // 1
	swtTokRawStrContinuingIndicator        // 2
	swtTokRawStrEndPart                    // 3
	swtTokImplicitSemi                     // 4
	swtTokExplicitSemi                     // 5
	swtTokArrowOperator                    // 6
	swtTokDotOperator                      // 7
	swtTokConjunctionOperator              // 8
	swtTokDisjunctionOperator              // 9
	swtTokNilCoalescingOperator            // 10
	swtTokEqualSign                        // 11
	swtTokEqEq                             // 12
	swtTokPlusThenWs                       // 13
	swtTokMinusThenWs                      // 14
	swtTokBang                             // 15
	swtTokThrowsKeyword                    // 16
	swtTokRethrowsKeyword                  // 17
	swtTokDefaultKeyword                   // 18
	swtTokWhereKeyword                     // 19
	swtTokElseKeyword                      // 20
	swtTokCatchKeyword                     // 21
	swtTokAsKeyword                        // 22
	swtTokAsQuest                          // 23
	swtTokAsBang                           // 24
	swtTokAsyncKeyword                     // 25
	swtTokCustomOperator                   // 26
	swtTokHashSymbol                       // 27
	swtTokDirectiveIf                      // 28
	swtTokDirectiveElseif                  // 29
	swtTokDirectiveElse                    // 30
	swtTokDirectiveEndif                   // 31
	swtTokFakeTryBang                      // 32
	swtTokenCount                          // 33 — sentinel
)

// Concrete symbol IDs from the generated Swift grammar ExternalSymbols.
const (
	swtSymBlockComment              gotreesitter.Symbol = 190
	swtSymRawStrPart                gotreesitter.Symbol = 191
	swtSymRawStrContinuingIndicator gotreesitter.Symbol = 192
	swtSymRawStrEndPart             gotreesitter.Symbol = 193
	swtSymImplicitSemi              gotreesitter.Symbol = 194
	swtSymExplicitSemi              gotreesitter.Symbol = 195
	swtSymArrowOperator             gotreesitter.Symbol = 196
	swtSymDotOperator               gotreesitter.Symbol = 197
	swtSymConjunctionOperator       gotreesitter.Symbol = 198
	swtSymDisjunctionOperator       gotreesitter.Symbol = 199
	swtSymNilCoalescingOperator     gotreesitter.Symbol = 200
	swtSymEqualSign                 gotreesitter.Symbol = 201
	swtSymEqEq                      gotreesitter.Symbol = 202
	swtSymPlusThenWs                gotreesitter.Symbol = 203
	swtSymMinusThenWs               gotreesitter.Symbol = 204
	swtSymBang                      gotreesitter.Symbol = 205
	swtSymThrowsKeyword             gotreesitter.Symbol = 206
	swtSymRethrowsKeyword           gotreesitter.Symbol = 207
	swtSymDefaultKeyword            gotreesitter.Symbol = 208
	swtSymWhereKeyword              gotreesitter.Symbol = 209
	swtSymElseKeyword               gotreesitter.Symbol = 210
	swtSymCatchKeyword              gotreesitter.Symbol = 211
	swtSymAsKeyword                 gotreesitter.Symbol = 212
	swtSymAsQuest                   gotreesitter.Symbol = 213
	swtSymAsBang                    gotreesitter.Symbol = 214
	swtSymAsyncKeyword              gotreesitter.Symbol = 215
	swtSymCustomOperator            gotreesitter.Symbol = 216
	swtSymHashSymbol                gotreesitter.Symbol = 217
	swtSymDirectiveIf               gotreesitter.Symbol = 218
	swtSymDirectiveElseif           gotreesitter.Symbol = 219
	swtSymDirectiveElse             gotreesitter.Symbol = 220
	swtSymDirectiveEndif            gotreesitter.Symbol = 221
	swtSymFakeTryBang               gotreesitter.Symbol = 222
)

// swtDefaultSymTable maps token indexes to concrete ts2go symbol IDs.
var swtDefaultSymTable = [swtTokenCount]gotreesitter.Symbol{
	swtSymBlockComment,
	swtSymRawStrPart,
	swtSymRawStrContinuingIndicator,
	swtSymRawStrEndPart,
	swtSymImplicitSemi,
	swtSymExplicitSemi,
	swtSymArrowOperator,
	swtSymDotOperator,
	swtSymConjunctionOperator,
	swtSymDisjunctionOperator,
	swtSymNilCoalescingOperator,
	swtSymEqualSign,
	swtSymEqEq,
	swtSymPlusThenWs,
	swtSymMinusThenWs,
	swtSymBang,
	swtSymThrowsKeyword,
	swtSymRethrowsKeyword,
	swtSymDefaultKeyword,
	swtSymWhereKeyword,
	swtSymElseKeyword,
	swtSymCatchKeyword,
	swtSymAsKeyword,
	swtSymAsQuest,
	swtSymAsBang,
	swtSymAsyncKeyword,
	swtSymCustomOperator,
	swtSymHashSymbol,
	swtSymDirectiveIf,
	swtSymDirectiveElseif,
	swtSymDirectiveElse,
	swtSymDirectiveEndif,
	swtSymFakeTryBang,
}

var swiftExternalScannerSpec = ExternalScannerSpec{
	Language:       "swift",
	UpstreamRepo:   "https://github.com/alex-pinkus/tree-sitter-swift",
	UpstreamCommit: "41d6e5fe811ec94229ee71771174a8cce558dfee",
	SourceFiles: []ExternalScannerSourceFile{
		{Path: "src/grammar.json", SHA256: "4632eabe75a68f35641f99e9bf07bdb481b91605d5caa7fef1f0876a3e9aceb2"},
		{Path: "src/scanner.c", SHA256: "f3d6271d64f58c39eed544104a70ca2cf9ecbf80c5d900620f1afd38836542cb"},
	},
	Externals: []string{
		"multiline_comment",
		"raw_str_part",
		"raw_str_continuing_indicator",
		"raw_str_end_part",
		"_implicit_semi",
		"_explicit_semi",
		"_arrow_operator_custom",
		"_dot_custom",
		"_conjunction_operator_custom",
		"_disjunction_operator_custom",
		"_nil_coalescing_operator_custom",
		"_eq_custom",
		"_eq_eq_custom",
		"_plus_then_ws",
		"_minus_then_ws",
		"_bang_custom",
		"_throws_keyword",
		"_rethrows_keyword",
		"default_keyword",
		"where_keyword",
		"else",
		"catch_keyword",
		"_as_custom",
		"_as_quest_custom",
		"_as_bang_custom",
		"_async_keyword_custom",
		"_custom_operator",
		"_hash_symbol_custom",
		"_directive_if",
		"_directive_elseif",
		"_directive_else",
		"_directive_endif",
		"_fake_try_bang",
	},
}

func init() {
	RegisterExternalScannerSpec(swiftExternalScannerSpec)
}

// ---------- illegal terminator groups ----------

const (
	swtIllegalAlphanumeric = iota
	swtIllegalOperatorSyms
	swtIllegalOperatorOrDot
	swtIllegalNonWhitespace
)

// ---------- operators table ----------

const swtOperatorCount = 20

var swtOperators = [swtOperatorCount]string{
	"->",
	".",
	"&&",
	"||",
	"??",
	"=",
	"==",
	"+",
	"-",
	"!",
	"throws",
	"rethrows",
	"default",
	"where",
	"else",
	"catch",
	"as",
	"as?",
	"as!",
	"async",
}

var swtOpIllegalTerminators = [swtOperatorCount]int{
	swtIllegalOperatorSyms,  // ->
	swtIllegalOperatorOrDot, // .
	swtIllegalOperatorSyms,  // &&
	swtIllegalOperatorSyms,  // ||
	swtIllegalOperatorSyms,  // ??
	swtIllegalOperatorSyms,  // =
	swtIllegalOperatorSyms,  // ==
	swtIllegalNonWhitespace, // +
	swtIllegalNonWhitespace, // -
	swtIllegalOperatorSyms,  // !
	swtIllegalAlphanumeric,  // throws
	swtIllegalAlphanumeric,  // rethrows
	swtIllegalAlphanumeric,  // default
	swtIllegalAlphanumeric,  // where
	swtIllegalAlphanumeric,  // else
	swtIllegalAlphanumeric,  // catch
	swtIllegalAlphanumeric,  // as
	swtIllegalOperatorSyms,  // as?
	swtIllegalOperatorSyms,  // as!
	swtIllegalAlphanumeric,  // async
}

var swtOpSymbols = [swtOperatorCount]int{
	swtTokArrowOperator,
	swtTokDotOperator,
	swtTokConjunctionOperator,
	swtTokDisjunctionOperator,
	swtTokNilCoalescingOperator,
	swtTokEqualSign,
	swtTokEqEq,
	swtTokPlusThenWs,
	swtTokMinusThenWs,
	swtTokBang,
	swtTokThrowsKeyword,
	swtTokRethrowsKeyword,
	swtTokDefaultKeyword,
	swtTokWhereKeyword,
	swtTokElseKeyword,
	swtTokCatchKeyword,
	swtTokAsKeyword,
	swtTokAsQuest,
	swtTokAsBang,
	swtTokAsyncKeyword,
}

// swtOpSymbolSuppressor: bitmask of token indexes whose validity suppresses the operator match.
// Only BANG (index 9) is suppressed by FAKE_TRY_BANG (index 32).
var swtOpSymbolSuppressor = [swtOperatorCount]uint64{
	0,                      // ->
	0,                      // .
	0,                      // &&
	0,                      // ||
	0,                      // ??
	0,                      // =
	0,                      // ==
	0,                      // +
	0,                      // -
	1 << swtTokFakeTryBang, // !
	0,                      // throws
	0,                      // rethrows
	0,                      // default
	0,                      // where
	0,                      // else
	0,                      // catch
	0,                      // as
	0,                      // as?
	0,                      // as!
	0,                      // async
}

// ---------- reserved operators ----------

const swtReservedOpCount = 31

var swtReservedOps = [swtReservedOpCount]string{
	"/",
	"=",
	"-",
	"+",
	"!",
	"*",
	"%",
	"<",
	">",
	"&",
	"|",
	"^",
	"?",
	"~",
	".",
	"..",
	"->",
	"/*",
	"*/",
	"+=",
	"-=",
	"*=",
	"/=",
	"%=",
	">>",
	"<<",
	"++",
	"--",
	"===",
	"...",
	"..<",
}

// ---------- non-consuming cross-semi characters ----------

var swtNonConsumingCrossSemiChars = [4]rune{'?', ':', '{', ','}

// ---------- parse directive ----------

const (
	swtContinueParsingNothingFound = iota
	swtContinueParsingTokenFound
	swtContinueParsingSlashConsumed
	swtStopParsingNothingFound
	swtStopParsingTokenFound
	swtStopParsingEndOfFile
)

// ---------- compiler directives ----------

const swtDirectiveCount = 4

var swtDirectives = [swtDirectiveCount]string{
	"if",
	"elseif",
	"else",
	"endif",
}

var swtDirectiveSymbols = [swtDirectiveCount]int{
	swtTokDirectiveIf,
	swtTokDirectiveElseif,
	swtTokDirectiveElse,
	swtTokDirectiveEndif,
}

// ---------- scanner state ----------

type swtScannerState struct {
	ongoingRawStrHashCount uint32
}

// SwiftExternalScanner handles all external tokens for the Swift grammar.
type SwiftExternalScanner struct {
	symbols         [swtTokenCount]gotreesitter.Symbol
	externalToToken []int
}

func (SwiftExternalScanner) ExternalScannerForLanguage(lang *gotreesitter.Language) gotreesitter.ExternalScanner {
	s := SwiftExternalScanner{symbols: swtDefaultSymTable}
	s.externalToToken = bindExternalScannerSpec(lang, swiftExternalScannerSpec, func(tokenIdx int, sym gotreesitter.Symbol) {
		s.symbols[tokenIdx] = sym
	})
	return s
}

func (SwiftExternalScanner) Create() any {
	return &swtScannerState{}
}

func (SwiftExternalScanner) Destroy(payload any) {}

func (SwiftExternalScanner) Serialize(payload any, buf []byte) int {
	s := payload.(*swtScannerState)
	hc := s.ongoingRawStrHashCount
	if len(buf) < 4 {
		return 0
	}
	buf[0] = byte(hc >> 24)
	buf[1] = byte(hc >> 16)
	buf[2] = byte(hc >> 8)
	buf[3] = byte(hc)
	return 4
}

func (SwiftExternalScanner) Deserialize(payload any, buf []byte) {
	s := payload.(*swtScannerState)
	s.ongoingRawStrHashCount = 0
	if len(buf) < 4 {
		return
	}
	s.ongoingRawStrHashCount = uint32(buf[0])<<24 |
		uint32(buf[1])<<16 |
		uint32(buf[2])<<8 |
		uint32(buf[3])
}

func (s SwiftExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	state := payload.(*swtScannerState)
	if len(s.externalToToken) > 0 {
		var semanticValid [swtTokenCount]bool
		for externalIdx, valid := range validSymbols {
			if !valid || externalIdx >= len(s.externalToToken) {
				continue
			}
			tokenIdx := s.externalToToken[externalIdx]
			if tokenIdx >= 0 && tokenIdx < swtTokenCount {
				semanticValid[tokenIdx] = true
			}
		}
		validSymbols = semanticValid[:]
	}
	return swtScan(state, lexer, validSymbols, s.symbolTable())
}

func (s SwiftExternalScanner) symbolTable() *[swtTokenCount]gotreesitter.Symbol {
	if s.symbols == ([swtTokenCount]gotreesitter.Symbol{}) {
		return &swtDefaultSymTable
	}
	return &s.symbols
}

// ---------- helpers ----------

func swtAdvance(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
}

func swtSetResult(lexer *gotreesitter.ExternalLexer, tok int, symbols *[swtTokenCount]gotreesitter.Symbol) {
	lexer.SetResultSymbol(symbols[tok])
}

func swtShouldTreatAsWspace(ch rune) bool {
	return unicode.IsSpace(ch) || ch == ';'
}

func swtIsOperatorSymbol(ch rune) bool {
	switch ch {
	case '/', '=', '-', '+', '!', '*', '%', '<', '>', '&', '|', '^', '?', '~':
		return true
	}
	return false
}

func swtIsAlphanumeric(ch rune) bool {
	return unicode.IsLetter(ch) || unicode.IsDigit(ch)
}

func swtIsCrossSemiToken(op int) bool {
	switch op {
	case swtTokArrowOperator,
		swtTokDotOperator,
		swtTokConjunctionOperator,
		swtTokDisjunctionOperator,
		swtTokNilCoalescingOperator,
		swtTokEqualSign,
		swtTokEqEq,
		swtTokPlusThenWs,
		swtTokMinusThenWs,
		swtTokThrowsKeyword,
		swtTokRethrowsKeyword,
		swtTokDefaultKeyword,
		swtTokWhereKeyword,
		swtTokElseKeyword,
		swtTokCatchKeyword,
		swtTokAsKeyword,
		swtTokAsQuest,
		swtTokAsBang,
		swtTokAsyncKeyword,
		swtTokCustomOperator:
		return true
	}
	return false
}

func swtEncounteredOpCount(encountered *[swtOperatorCount]bool) int {
	count := 0
	for i := 0; i < swtOperatorCount; i++ {
		if encountered[i] {
			count++
		}
	}
	return count
}

func swtAnyReservedOps(encountered *[swtReservedOpCount]uint8) bool {
	for i := 0; i < swtReservedOpCount; i++ {
		if encountered[i] == 2 {
			return true
		}
	}
	return false
}

func swtIsLegalCustomOperator(charIdx int, firstChar rune, curChar rune) bool {
	isFirst := charIdx == 0
	switch curChar {
	case '=', '-', '+', '!', '%', '<', '>', '&', '|', '^', '?', '~':
		return true
	case '.':
		return isFirst || firstChar == '.'
	case '*', '/':
		// /* and // can't start an operator since they start comments
		return charIdx != 1 || firstChar != '/'
	default:
		if (curChar >= 0x00A1 && curChar <= 0x00A7) ||
			curChar == 0x00A9 ||
			curChar == 0x00AB ||
			curChar == 0x00AC ||
			curChar == 0x00AE ||
			(curChar >= 0x00B0 && curChar <= 0x00B1) ||
			curChar == 0x00B6 ||
			curChar == 0x00BB ||
			curChar == 0x00BF ||
			curChar == 0x00D7 ||
			curChar == 0x00F7 ||
			(curChar >= 0x2016 && curChar <= 0x2017) ||
			(curChar >= 0x2020 && curChar <= 0x2027) ||
			(curChar >= 0x2030 && curChar <= 0x203E) ||
			(curChar >= 0x2041 && curChar <= 0x2053) ||
			(curChar >= 0x2055 && curChar <= 0x205E) ||
			(curChar >= 0x2190 && curChar <= 0x23FF) ||
			(curChar >= 0x2500 && curChar <= 0x2775) ||
			(curChar >= 0x2794 && curChar <= 0x2BFF) ||
			(curChar >= 0x2E00 && curChar <= 0x2E7F) ||
			(curChar >= 0x3001 && curChar <= 0x3003) ||
			(curChar >= 0x3008 && curChar <= 0x3020) ||
			curChar == 0x3030 {
			return true
		}
		if (curChar >= 0x0300 && curChar <= 0x036F) ||
			(curChar >= 0x1DC0 && curChar <= 0x1DFF) ||
			(curChar >= 0x20D0 && curChar <= 0x20FF) ||
			(curChar >= 0xFE00 && curChar <= 0xFE0F) ||
			(curChar >= 0xFE20 && curChar <= 0xFE2F) ||
			(curChar >= 0xE0100 && curChar <= 0xE01EF) {
			return !isFirst
		}
		return false
	}
}

// ---------- eat_operators ----------

func swtEatOperators(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
	markEnd bool,
	priorChar rune,
) (found bool, symbolResult int) {
	var possibleOps [swtOperatorCount]bool
	var reservedOps [swtReservedOpCount]uint8

	for i := 0; i < swtOperatorCount; i++ {
		possibleOps[i] = validSymbols[swtOpSymbols[i]] &&
			(priorChar == 0 || rune(swtOperators[i][0]) == priorChar)
	}
	for i := 0; i < swtReservedOpCount; i++ {
		if priorChar == 0 || rune(swtReservedOps[i][0]) == priorChar {
			reservedOps[i] = 1
		}
	}

	possibleCustomOp := validSymbols[swtTokCustomOperator]
	var firstChar rune
	if priorChar != 0 {
		firstChar = priorChar
	} else {
		firstChar = lexer.Lookahead()
	}
	lastExaminedChar := firstChar

	strIdx := 0
	if priorChar != 0 {
		strIdx = 1
	}
	fullMatch := -1

	for {
		la := lexer.Lookahead()

		for i := 0; i < swtOperatorCount; i++ {
			if !possibleOps[i] {
				continue
			}

			op := swtOperators[i]
			if strIdx >= len(op) {
				// Operator fully matched; check illegal terminator.
				illegal := swtOpIllegalTerminators[i]
				terminate := false

				switch {
				case swtIsOperatorSymbol(la):
					if illegal == swtIllegalOperatorSyms {
						terminate = true
					} else if la == '.' && illegal == swtIllegalOperatorOrDot {
						terminate = true
					} else if !unicode.IsSpace(la) && illegal == swtIllegalNonWhitespace {
						terminate = true
					} else {
						// Legal terminator — record match.
						fullMatch = i
						if markEnd {
							lexer.MarkEnd()
						}
					}
				case la == '.':
					if illegal == swtIllegalOperatorOrDot {
						terminate = true
					} else {
						fullMatch = i
						if markEnd {
							lexer.MarkEnd()
						}
					}
				default:
					if swtIsAlphanumeric(la) && illegal == swtIllegalAlphanumeric {
						terminate = true
					} else if !unicode.IsSpace(la) && la != 0 && illegal == swtIllegalNonWhitespace {
						terminate = true
					} else {
						fullMatch = i
						if markEnd {
							lexer.MarkEnd()
						}
					}
				}

				_ = terminate
				possibleOps[i] = false
				continue
			}

			if rune(op[strIdx]) != la {
				possibleOps[i] = false
				continue
			}
		}

		for i := 0; i < swtReservedOpCount; i++ {
			if reservedOps[i] == 0 {
				continue
			}

			rop := swtReservedOps[i]
			if strIdx >= len(rop) {
				reservedOps[i] = 0
				continue
			}

			if rune(rop[strIdx]) != la {
				reservedOps[i] = 0
				continue
			}

			if strIdx+1 >= len(rop) {
				reservedOps[i] = 2
				continue
			}
		}

		possibleCustomOp = possibleCustomOp && swtIsLegalCustomOperator(strIdx, firstChar, la)

		encountered := swtEncounteredOpCount(&possibleOps)
		if encountered == 0 {
			if !possibleCustomOp {
				break
			} else if markEnd && fullMatch == -1 {
				lexer.MarkEnd()
			}
		}

		lastExaminedChar = la
		lexer.Advance(false)
		strIdx++

		if encountered == 0 && !swtIsLegalCustomOperator(strIdx, firstChar, lexer.Lookahead()) {
			break
		}
	}

	if fullMatch != -1 {
		// Check suppressor bitmask.
		suppressing := swtOpSymbolSuppressor[fullMatch]
		if suppressing != 0 {
			for sup := 0; sup < swtTokenCount; sup++ {
				if suppressing&(1<<uint(sup)) == 0 {
					continue
				}
				if validSymbols[sup] {
					return false, 0
				}
			}
		}
		return true, swtOpSymbols[fullMatch]
	}

	if possibleCustomOp && !swtAnyReservedOps(&reservedOps) {
		if (lastExaminedChar != '<' || unicode.IsSpace(lexer.Lookahead())) && markEnd {
			lexer.MarkEnd()
		}
		return true, swtTokCustomOperator
	}

	return false, 0
}

// ---------- eat_comment ----------

func swtEatComment(
	lexer *gotreesitter.ExternalLexer,
	markEnd bool,
) (directive int, symbolResult int) {
	if lexer.Lookahead() != '/' {
		return swtContinueParsingNothingFound, 0
	}

	swtAdvance(lexer)

	if lexer.Lookahead() != '*' {
		return swtContinueParsingSlashConsumed, 0
	}

	swtAdvance(lexer)

	afterStar := false
	nestingDepth := 1

	for {
		la := lexer.Lookahead()
		switch {
		case la == 0:
			return swtStopParsingEndOfFile, 0

		case la == '*':
			swtAdvance(lexer)
			afterStar = true

		case la == '/':
			if afterStar {
				swtAdvance(lexer)
				afterStar = false
				nestingDepth--
				if nestingDepth == 0 {
					if markEnd {
						lexer.MarkEnd()
					}
					return swtStopParsingTokenFound, swtTokBlockComment
				}
			} else {
				swtAdvance(lexer)
				afterStar = false
				if lexer.Lookahead() == '*' {
					nestingDepth++
					swtAdvance(lexer)
				}
			}

		default:
			swtAdvance(lexer)
			afterStar = false
		}
	}
}

// ---------- eat_whitespace ----------

func swtEatWhitespace(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) (directive int, symbolResult int) {
	previous := lexer.Previous()
	wsDirective := swtContinueParsingNothingFound
	semiIsValid := validSymbols[swtTokImplicitSemi] && validSymbols[swtTokExplicitSemi]

	var lookahead rune
	for {
		lookahead = lexer.Lookahead()
		if !swtShouldTreatAsWspace(lookahead) {
			break
		}

		if lookahead == ';' {
			if semiIsValid {
				wsDirective = swtStopParsingTokenFound
				lexer.Advance(false)
			}
			break
		}

		lexer.Advance(true)
		lexer.MarkEnd()

		if wsDirective == swtContinueParsingNothingFound && (lookahead == '\n' || lookahead == '\r') {
			wsDirective = swtContinueParsingTokenFound
		}
	}

	anyComment := swtContinueParsingNothingFound
	if wsDirective == swtContinueParsingTokenFound && lookahead == '/' {
		hasSeenSingleComment := false
		for lexer.Lookahead() == '/' {
			commentDirective, commentResult := swtEatComment(lexer, false)
			anyComment = commentDirective

			if anyComment == swtStopParsingTokenFound {
				if !hasSeenSingleComment {
					lexer.MarkEnd()
					return swtStopParsingTokenFound, commentResult
				}
			} else if anyComment == swtStopParsingEndOfFile {
				return swtStopParsingEndOfFile, 0
			} else if anyComment == swtContinueParsingSlashConsumed {
				if lexer.Lookahead() == '/' {
					// Single-line comment: second slash seen.
					hasSeenSingleComment = true
					for lexer.Lookahead() != '\n' && lexer.Lookahead() != 0 {
						lexer.Advance(true)
					}
				} else if unicode.IsSpace(lexer.Lookahead()) {
					return swtStopParsingNothingFound, 0
				}
			}

			// Skip whitespace after comment.
			for unicode.IsSpace(lexer.Lookahead()) {
				anyComment = swtContinueParsingNothingFound
				lexer.Advance(true)
			}
		}

		sawOp, _ := swtEatOperators(lexer, validSymbols, false, 0)
		if sawOp {
			return swtStopParsingNothingFound, 0
		}
		// Promote to explicit result.
		wsDirective = swtStopParsingTokenFound
		return wsDirective, swtTokImplicitSemi
	}

	// A comma continues a list across a newline. Do not synthesize a
	// semicolon before the next constraint in a multiline where clause.
	if wsDirective == swtContinueParsingTokenFound && previous == ',' {
		return swtContinueParsingNothingFound, 0
	}

	// Check non-consuming cross-semi characters.
	if wsDirective == swtContinueParsingTokenFound {
		for _, ch := range swtNonConsumingCrossSemiChars {
			if ch == lookahead {
				return swtContinueParsingNothingFound, 0
			}
		}
	}

	if semiIsValid && wsDirective != swtContinueParsingNothingFound {
		result := swtTokImplicitSemi
		if lookahead == ';' {
			result = swtTokExplicitSemi
		}
		return wsDirective, result
	}

	return swtContinueParsingNothingFound, 0
}

// ---------- find_possible_compiler_directive ----------

func swtFindPossibleCompilerDirective(lexer *gotreesitter.ExternalLexer) int {
	var possibleDirs [swtDirectiveCount]bool
	for i := 0; i < swtDirectiveCount; i++ {
		possibleDirs[i] = true
	}

	strIdx := 0
	fullMatch := -1

	for {
		la := lexer.Lookahead()

		for i := 0; i < swtDirectiveCount; i++ {
			if !possibleDirs[i] {
				continue
			}

			dir := swtDirectives[i]
			if strIdx >= len(dir) {
				fullMatch = i
				lexer.MarkEnd()
			}

			if strIdx >= len(dir) || rune(dir[strIdx]) != la {
				possibleDirs[i] = false
				continue
			}
		}

		matchCount := 0
		for i := 0; i < swtDirectiveCount; i++ {
			if possibleDirs[i] {
				matchCount++
			}
		}

		if matchCount == 0 {
			break
		}

		lexer.Advance(false)
		strIdx++
	}

	if fullMatch == -1 {
		return swtTokHashSymbol
	}

	return swtDirectiveSymbols[fullMatch]
}

// ---------- eat_raw_str_part ----------

func swtEatRawStrPart(
	state *swtScannerState,
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) (found bool, symbolResult int) {
	hashCount := state.ongoingRawStrHashCount

	if !validSymbols[swtTokRawStrPart] {
		return false, 0
	}

	if hashCount == 0 {
		// First raw_str_part — look for hashes.
		for lexer.Lookahead() == '#' {
			hashCount++
			swtAdvance(lexer)
		}

		if hashCount == 0 {
			return false, 0
		}

		if lexer.Lookahead() == '"' {
			swtAdvance(lexer)
		} else if hashCount == 1 {
			lexer.MarkEnd()
			result := swtFindPossibleCompilerDirective(lexer)
			return true, result
		} else {
			return false, 0
		}
	} else if validSymbols[swtTokRawStrContinuingIndicator] {
		// End of interpolation — continue raw string.
	} else {
		return false, 0
	}

	// Consume characters until we find `hashCount` consecutive '#' symbols.
	for lexer.Lookahead() != 0 {
		var lastChar rune
		lexer.MarkEnd()

		// Advance through non-hash characters.
		for lexer.Lookahead() != '#' && lexer.Lookahead() != 0 {
			lastChar = lexer.Lookahead()
			swtAdvance(lexer)
			if lastChar != '\\' || lexer.Lookahead() == '\\' {
				lexer.MarkEnd()
			}
		}

		// Count consecutive hashes.
		var currentHashCount uint32
		for lexer.Lookahead() == '#' && currentHashCount < hashCount {
			currentHashCount++
			swtAdvance(lexer)
		}

		if currentHashCount == hashCount {
			if lastChar == '\\' && lexer.Lookahead() == '(' {
				// Interpolation.
				state.ongoingRawStrHashCount = hashCount
				return true, swtTokRawStrPart
			} else if lastChar == '"' {
				// End of string.
				lexer.MarkEnd()
				state.ongoingRawStrHashCount = 0
				return true, swtTokRawStrEndPart
			}
			// Nothing special; continue.
		}
	}

	return false, 0
}

// ---------- main scan ----------

func swtScan(state *swtScannerState, lexer *gotreesitter.ExternalLexer, validSymbols []bool, symbols *[swtTokenCount]gotreesitter.Symbol) bool {
	// Consume any whitespace at the start.
	wsDirective, wsResult := swtEatWhitespace(lexer, validSymbols)
	if wsDirective == swtStopParsingTokenFound {
		swtSetResult(lexer, wsResult, symbols)
		return true
	}

	if wsDirective == swtStopParsingNothingFound || wsDirective == swtStopParsingEndOfFile {
		return false
	}

	hasWsResult := wsDirective == swtContinueParsingTokenFound

	// Parse block comments (before custom operators so comments aren't treated as operators).
	var commentDirective int
	if wsDirective == swtContinueParsingSlashConsumed {
		commentDirective = swtContinueParsingSlashConsumed
	} else {
		var commentResult int
		commentDirective, commentResult = swtEatComment(lexer, true)
		if commentDirective == swtStopParsingTokenFound {
			lexer.MarkEnd()
			swtSetResult(lexer, commentResult, symbols)
			return true
		}
	}

	if commentDirective == swtStopParsingEndOfFile {
		return false
	}

	// Parse operators that might suppress the whitespace result.
	var priorChar rune
	if commentDirective == swtContinueParsingSlashConsumed {
		priorChar = '/'
	}

	sawOp, opResult := swtEatOperators(lexer, validSymbols, !hasWsResult, priorChar)
	if sawOp && (!hasWsResult || swtIsCrossSemiToken(opResult)) {
		swtSetResult(lexer, opResult, symbols)
		if hasWsResult {
			lexer.MarkEnd()
		}
		return true
	}

	if hasWsResult {
		// Don't mark_end since we may have advanced through operators.
		swtSetResult(lexer, wsResult, symbols)
		return true
	}

	// Raw string parts — consumes '#' characters, so keep at the end.
	sawRaw, rawResult := swtEatRawStrPart(state, lexer, validSymbols)
	if sawRaw {
		swtSetResult(lexer, rawResult, symbols)
		return true
	}

	return false
}
