//go:build !grammar_subset || grammar_subset_tlaplus

package grammars

import (
	"encoding/binary"
	"unicode"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// ---------------------------------------------------------------------------
// Token indexes (position in the externals array of grammar.js)
// ---------------------------------------------------------------------------

const (
	tlaTokLeadingExtramodularText  = 0  // Freeform text at the start of the file.
	tlaTokTrailingExtramodularText = 1  // Freeform text between or after modules.
	tlaTokIndent                   = 2  // Marks beginning of junction list.
	tlaTokBullet                   = 3  // New item of a junction list.
	tlaTokDedent                   = 4  // Marks end of junction list.
	tlaTokBeginProof               = 5  // Marks the beginning of an entire proof.
	tlaTokBeginProofStep           = 6  // Marks the beginning of a proof step.
	tlaTokProofKeyword             = 7  // The PROOF keyword.
	tlaTokByKeyword                = 8  // The BY keyword.
	tlaTokObviousKeyword           = 9  // The OBVIOUS keyword.
	tlaTokOmittedKeyword           = 10 // The OMITTED keyword.
	tlaTokQedKeyword               = 11 // The QED keyword.
	tlaTokWeakFairness             = 12 // The WF_ keyword.
	tlaTokStrongFairness           = 13 // The SF_ keyword.
	tlaTokPcalStart                = 14 // Notifies scanner of start of PlusCal block.
	tlaTokPcalEnd                  = 15 // Notifies scanner of end of PlusCal block.
	tlaTokDoubleExcl               = 16 // The !! infix op.
	tlaTokErrorSentinel            = 17 // Only valid if in error recovery mode.
)

// ---------------------------------------------------------------------------
// Symbol IDs (mapped from grammar)
// ---------------------------------------------------------------------------

const (
	tlaSymLeadingExtramodularText  gotreesitter.Symbol = 353
	tlaSymTrailingExtramodularText gotreesitter.Symbol = 354
	tlaSymIndent                   gotreesitter.Symbol = 355
	tlaSymBullet                   gotreesitter.Symbol = 356
	tlaSymDedent                   gotreesitter.Symbol = 357
	tlaSymBeginProof               gotreesitter.Symbol = 358
	tlaSymBeginProofStep           gotreesitter.Symbol = 359
	tlaSymProofKeyword             gotreesitter.Symbol = 301
	tlaSymByKeyword                gotreesitter.Symbol = 302
	tlaSymObviousKeyword           gotreesitter.Symbol = 304
	tlaSymOmittedKeyword           gotreesitter.Symbol = 305
	tlaSymQedKeyword               gotreesitter.Symbol = 312
	tlaSymWeakFairness             gotreesitter.Symbol = 103
	tlaSymStrongFairness           gotreesitter.Symbol = 104
	tlaSymPcalStart                gotreesitter.Symbol = 360
	tlaSymPcalEnd                  gotreesitter.Symbol = 361
	tlaSymDoubleExcl               gotreesitter.Symbol = 362
	tlaSymErrorSentinel            gotreesitter.Symbol = 363
)

// ---------------------------------------------------------------------------
// Lexeme / Token enumerations (internal to the scanner)
// ---------------------------------------------------------------------------

// tlaLexeme represents a recognized lexeme from the lookahead lexer.
type tlaLexeme int

const (
	tlaLexForwardSlash tlaLexeme = iota
	tlaLexBackwardSlash
	tlaLexGt
	tlaLexEq
	tlaLexDash
	tlaLexComma
	tlaLexColon
	tlaLexSemicolon
	tlaLexLand
	tlaLexLor
	tlaLexDoubleExcl
	tlaLexLParen
	tlaLexRParen
	tlaLexRSquareBracket
	tlaLexRCurlyBrace
	tlaLexRAngleBracket
	tlaLexRightArrow
	tlaLexRightMapArrow
	tlaLexCommentStart
	tlaLexBlockCommentStart
	tlaLexSingleLine
	tlaLexDoubleLine
	tlaLexAssumeKeyword
	tlaLexAssumptionKeyword
	tlaLexAxiomKeyword
	tlaLexByKeyword
	tlaLexConstantKeyword
	tlaLexConstantsKeyword
	tlaLexCorollaryKeyword
	tlaLexElseKeyword
	tlaLexInKeyword
	tlaLexLemmaKeyword
	tlaLexLocalKeyword
	tlaLexObviousKeyword
	tlaLexOmittedKeyword
	tlaLexProofKeyword
	tlaLexPropositionKeyword
	tlaLexQedKeyword
	tlaLexThenKeyword
	tlaLexTheoremKeyword
	tlaLexVariableKeyword
	tlaLexVariablesKeyword
	tlaLexProofStepID
	tlaLexIdentifier
	tlaLexWeakFairness
	tlaLexStrongFairness
	tlaLexOther
	tlaLexEndOfFile
)

// tlaToken represents a classified token category.
type tlaToken int

const (
	tlaTknLand tlaToken = iota
	tlaTknLor
	tlaTknDoubleExcl
	tlaTknRightDelimiter
	tlaTknCommentStart
	tlaTknTerminator
	tlaTknProofStepID
	tlaTknProofKeyword
	tlaTknByKeyword
	tlaTknObviousKeyword
	tlaTknOmittedKeyword
	tlaTknQedKeyword
	tlaTknWeakFairness
	tlaTknStrongFairness
	tlaTknOther
)

// tlaTokenizeLexeme maps a lexeme to a token category.
func tlaTokenizeLexeme(lex tlaLexeme) tlaToken {
	switch lex {
	case tlaLexForwardSlash:
		return tlaTknOther
	case tlaLexBackwardSlash:
		return tlaTknOther
	case tlaLexGt:
		return tlaTknOther
	case tlaLexEq:
		return tlaTknOther
	case tlaLexDash:
		return tlaTknOther
	case tlaLexComma:
		return tlaTknRightDelimiter
	case tlaLexColon:
		return tlaTknRightDelimiter
	case tlaLexSemicolon:
		return tlaTknTerminator
	case tlaLexLand:
		return tlaTknLand
	case tlaLexLor:
		return tlaTknLor
	case tlaLexDoubleExcl:
		return tlaTknDoubleExcl
	case tlaLexLParen:
		return tlaTknOther
	case tlaLexRParen:
		return tlaTknRightDelimiter
	case tlaLexRSquareBracket:
		return tlaTknRightDelimiter
	case tlaLexRCurlyBrace:
		return tlaTknRightDelimiter
	case tlaLexRAngleBracket:
		return tlaTknRightDelimiter
	case tlaLexRightArrow:
		return tlaTknRightDelimiter
	case tlaLexRightMapArrow:
		return tlaTknRightDelimiter
	case tlaLexCommentStart:
		return tlaTknCommentStart
	case tlaLexBlockCommentStart:
		return tlaTknCommentStart
	case tlaLexSingleLine:
		return tlaTknTerminator
	case tlaLexDoubleLine:
		return tlaTknTerminator
	case tlaLexAssumeKeyword:
		return tlaTknTerminator
	case tlaLexAssumptionKeyword:
		return tlaTknTerminator
	case tlaLexAxiomKeyword:
		return tlaTknTerminator
	case tlaLexByKeyword:
		return tlaTknByKeyword
	case tlaLexConstantKeyword:
		return tlaTknTerminator
	case tlaLexConstantsKeyword:
		return tlaTknTerminator
	case tlaLexCorollaryKeyword:
		return tlaTknTerminator
	case tlaLexElseKeyword:
		return tlaTknRightDelimiter
	case tlaLexInKeyword:
		return tlaTknRightDelimiter
	case tlaLexLemmaKeyword:
		return tlaTknTerminator
	case tlaLexLocalKeyword:
		return tlaTknTerminator
	case tlaLexObviousKeyword:
		return tlaTknObviousKeyword
	case tlaLexOmittedKeyword:
		return tlaTknOmittedKeyword
	case tlaLexProofKeyword:
		return tlaTknProofKeyword
	case tlaLexPropositionKeyword:
		return tlaTknTerminator
	case tlaLexQedKeyword:
		return tlaTknQedKeyword
	case tlaLexThenKeyword:
		return tlaTknRightDelimiter
	case tlaLexTheoremKeyword:
		return tlaTknTerminator
	case tlaLexVariableKeyword:
		return tlaTknTerminator
	case tlaLexVariablesKeyword:
		return tlaTknTerminator
	case tlaLexProofStepID:
		return tlaTknProofStepID
	case tlaLexIdentifier:
		return tlaTknOther
	case tlaLexWeakFairness:
		return tlaTknWeakFairness
	case tlaLexStrongFairness:
		return tlaTknStrongFairness
	case tlaLexOther:
		return tlaTknOther
	case tlaLexEndOfFile:
		return tlaTknTerminator
	default:
		return tlaTknOther
	}
}

// ---------------------------------------------------------------------------
// Junction list types
// ---------------------------------------------------------------------------

type tlaJunctType uint8

const (
	tlaJunctConjunction tlaJunctType = iota
	tlaJunctDisjunction
)

// tlaJunctList tracks the type and alignment column of a junction list.
type tlaJunctList struct {
	jType           tlaJunctType
	alignmentColumn int16
}

// ---------------------------------------------------------------------------
// Proof step ID types
// ---------------------------------------------------------------------------

type tlaProofStepIDType int

const (
	tlaProofStepStar     tlaProofStepIDType = iota // <*>
	tlaProofStepPlus                               // <+>
	tlaProofStepNumbered                           // <1234>
	tlaProofStepNone                               // Invalid or nonexistent
)

type tlaProofStepID struct {
	idType tlaProofStepIDType
	level  int32
}

func tlaParseProofStepID(rawLevel []byte) tlaProofStepID {
	id := tlaProofStepID{level: -1}
	if len(rawLevel) == 0 {
		id.idType = tlaProofStepNone
	} else if rawLevel[0] == '*' {
		id.idType = tlaProofStepStar
	} else if rawLevel[0] == '+' {
		id.idType = tlaProofStepPlus
	} else {
		id.idType = tlaProofStepNumbered
		id.level = 0
		multiplier := int32(1)
		for i := len(rawLevel) - 1; i >= 0; i-- {
			digitValue := int32(rawLevel[i]) - 48
			if digitValue >= 0 && digitValue <= 9 {
				id.level += digitValue * multiplier
				multiplier *= 10
			} else {
				id.idType = tlaProofStepNone
				id.level = -1
				break
			}
		}
	}
	return id
}

// ---------------------------------------------------------------------------
// tlaScanner — the core scanner state (one per context)
// ---------------------------------------------------------------------------

type tlaScanner struct {
	jlists               []tlaJunctList
	proofs               []int32
	lastProofLevel       int32
	haveSeenProofKeyword bool
}

func newTlaScanner() *tlaScanner {
	return &tlaScanner{
		lastProofLevel: -1,
	}
}

func (s *tlaScanner) reset() {
	s.jlists = s.jlists[:0]
	s.proofs = s.proofs[:0]
	s.lastProofLevel = -1
	s.haveSeenProofKeyword = false
}

func (s *tlaScanner) serialize() []byte {
	// Layout:
	//   int16: jlist_depth
	//   for each jlist: uint8(type) + int16(alignment_column)
	//   int16: proof_depth
	//   for each proof: int32(level)
	//   int32: last_proof_level
	//   uint8: have_seen_proof_keyword
	jlistDepth := int16(len(s.jlists))
	proofDepth := int16(len(s.proofs))

	size := 2 + int(jlistDepth)*3 + 2 + int(proofDepth)*4 + 4 + 1
	buf := make([]byte, size)
	offset := 0

	binary.LittleEndian.PutUint16(buf[offset:], uint16(jlistDepth))
	offset += 2
	for i := int16(0); i < jlistDepth; i++ {
		buf[offset] = uint8(s.jlists[i].jType)
		offset++
		binary.LittleEndian.PutUint16(buf[offset:], uint16(s.jlists[i].alignmentColumn))
		offset += 2
	}

	binary.LittleEndian.PutUint16(buf[offset:], uint16(proofDepth))
	offset += 2
	for i := int16(0); i < proofDepth; i++ {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(s.proofs[i]))
		offset += 4
	}

	binary.LittleEndian.PutUint32(buf[offset:], uint32(s.lastProofLevel))
	offset += 4

	if s.haveSeenProofKeyword {
		buf[offset] = 1
	} else {
		buf[offset] = 0
	}

	return buf
}

func (s *tlaScanner) deserialize(data []byte) {
	s.reset()
	if len(data) == 0 {
		return
	}

	offset := 0

	jlistDepth := int16(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	for i := int16(0); i < jlistDepth; i++ {
		jt := tlaJunctType(data[offset])
		offset++
		ac := int16(binary.LittleEndian.Uint16(data[offset:]))
		offset += 2
		s.jlists = append(s.jlists, tlaJunctList{jType: jt, alignmentColumn: ac})
	}

	proofDepth := int16(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2
	for i := int16(0); i < proofDepth; i++ {
		level := int32(binary.LittleEndian.Uint32(data[offset:]))
		offset += 4
		s.proofs = append(s.proofs, level)
	}

	s.lastProofLevel = int32(binary.LittleEndian.Uint32(data[offset:]))
	offset += 4

	s.haveSeenProofKeyword = data[offset]&1 != 0
}

func (s *tlaScanner) isInJlist() bool {
	return len(s.jlists) > 0
}

func (s *tlaScanner) getCurrentJlistColumnIndex() int16 {
	if s.isInJlist() {
		return s.jlists[len(s.jlists)-1].alignmentColumn
	}
	return -1
}

func (s *tlaScanner) currentJlistTypeIs(jt tlaJunctType) bool {
	return s.isInJlist() && s.jlists[len(s.jlists)-1].jType == jt
}

func (s *tlaScanner) isInProof() bool {
	return len(s.proofs) > 0
}

func (s *tlaScanner) getCurrentProofLevel() int32 {
	if s.isInProof() {
		return s.proofs[len(s.proofs)-1]
	}
	return -1
}

// ---------------------------------------------------------------------------
// Helper: check if a codepoint is an identifier character
// ---------------------------------------------------------------------------

func tlaIsIdentifierChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// ---------------------------------------------------------------------------
// Helper: check if the lexer has more input
// ---------------------------------------------------------------------------

func tlaHasNext(lexer *gotreesitter.ExternalLexer) bool {
	return lexer.Lookahead() != 0
}

// ---------------------------------------------------------------------------
// Helper: advance consuming character as non-whitespace
// ---------------------------------------------------------------------------

func tlaAdvance(lexer *gotreesitter.ExternalLexer) {
	lexer.Advance(false)
}

// ---------------------------------------------------------------------------
// Helper: consume all occurrences of a specific codepoint
// ---------------------------------------------------------------------------

func tlaConsume(lexer *gotreesitter.ExternalLexer, cp rune) {
	for tlaHasNext(lexer) && lexer.Lookahead() == cp {
		tlaAdvance(lexer)
	}
}

// ---------------------------------------------------------------------------
// Helper: check if the next sequence matches a string; advances through
// all but the last character (matching the C behavior).
// ---------------------------------------------------------------------------

func tlaIsNextSequence(lexer *gotreesitter.ExternalLexer, seq string) bool {
	for i := 0; i < len(seq); i++ {
		if lexer.Lookahead() != rune(seq[i]) {
			return false
		}
		if i+1 < len(seq) {
			tlaAdvance(lexer)
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Extramodular text scanning
// ---------------------------------------------------------------------------

func tlaScanExtramodularText(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) bool {
	hasConsumedAny := false

	// State machine for scanning extramodular text.
	// We use a loop with an explicit state variable, matching the C macro-based
	// state machine that uses goto.
	type emtState int
	const (
		emtConsume emtState = iota
		emtDash
		emtSingleLine
		emtModule
		emtBlankBeforeModule
		emtEndOfFile
		emtBlankBeforeEndOfFile
	)

	state := emtConsume

	for {
		switch state {
		case emtConsume:
			if !tlaHasNext(lexer) {
				state = emtEndOfFile
				lexer.Advance(false) // consume EOF position (noop, mirrors C ADVANCE)
				continue
			}
			if unicode.IsSpace(lexer.Lookahead()) && !hasConsumedAny {
				lexer.Advance(true) // SKIP
				state = emtConsume
				continue
			}
			if unicode.IsSpace(lexer.Lookahead()) && hasConsumedAny {
				tlaAdvance(lexer) // ADVANCE
				state = emtConsume
				continue
			}
			lexer.MarkEnd()
			if lexer.Lookahead() == '-' {
				tlaAdvance(lexer)
				state = emtDash
				continue
			}
			hasConsumedAny = true
			tlaAdvance(lexer)
			state = emtConsume
			continue

		case emtDash:
			if tlaIsNextSequence(lexer, "---") {
				tlaAdvance(lexer) // consume last char of sequence
				state = emtSingleLine
				continue
			}
			hasConsumedAny = true
			state = emtConsume
			continue

		case emtSingleLine:
			tlaConsume(lexer, '-')
			tlaConsume(lexer, ' ')
			if tlaIsNextSequence(lexer, "MODULE") {
				tlaAdvance(lexer) // consume last char of "MODULE"
				state = emtModule
				continue
			}
			hasConsumedAny = true
			state = emtConsume
			continue

		case emtModule:
			if !hasConsumedAny {
				state = emtBlankBeforeModule
				continue
			}
			var sym gotreesitter.Symbol
			if validSymbols[tlaTokLeadingExtramodularText] {
				sym = tlaSymLeadingExtramodularText
			} else {
				sym = tlaSymTrailingExtramodularText
			}
			lexer.SetResultSymbol(sym)
			return true

		case emtBlankBeforeModule:
			return false

		case emtEndOfFile:
			if !hasConsumedAny {
				state = emtBlankBeforeEndOfFile
				continue
			}
			if validSymbols[tlaTokTrailingExtramodularText] {
				lexer.MarkEnd()
				lexer.SetResultSymbol(tlaSymTrailingExtramodularText)
				return true
			}
			return false

		case emtBlankBeforeEndOfFile:
			return false
		}
	}
}

// ---------------------------------------------------------------------------
// Lookahead lexer — identifies the next lexeme
// ---------------------------------------------------------------------------

func tlaLexLookahead(
	lexer *gotreesitter.ExternalLexer,
) (lex tlaLexeme, col int16, proofStepIDLevel []byte) {

	// resultLexeme tracks the last accepted lexeme (for the macro-like ACCEPT_LEXEME).
	resultLexeme := tlaLexOther
	col = -1

	// State machine
	type ls int
	const (
		lsConsumeLeadingSpace ls = iota
		lsForwardSlash
		lsBackwardSlash
		lsLT
		lsGT
		lsEQ
		lsDash
		lsComma
		lsColon
		lsExcl
		lsDoubleExcl
		lsSemicolon
		lsLand
		lsLor
		lsLParen
		lsRParen
		lsRSquareBracket
		lsRCurlyBrace
		lsRAngleBracket
		lsS
		lsSF_
		lsRightArrow
		lsRightMapArrow
		lsCommentStart
		lsBlockCommentStart
		lsSingleLine
		lsDoubleLine
		lsPipe
		lsRightTurnstile
		lsA
		lsASSUM
		lsASSUME
		lsASSUMPTION
		lsAX
		lsAXIOM
		lsB
		lsBY
		lsC
		lsCO
		lsCON
		lsCOR
		lsCONSTANT
		lsCONSTANTS
		lsCOROLLARY
		lsE
		lsELSE
		lsI
		lsIN
		lsL
		lsLE
		lsLEMMA
		lsLO
		lsLOCAL
		lsO
		lsOB
		lsOBVIOUS
		lsOM
		lsOMITTED
		lsP
		lsPRO
		lsPROO
		lsPROOF
		lsPROP
		lsPROPOSITION
		lsQ
		lsQED
		lsT
		lsTHE
		lsTHEN
		lsTHEOREM
		lsV
		lsVARIABLE
		lsVARIABLES
		lsW
		lsWF_
		lsIDENTIFIER
		lsProofLevelNumber
		lsProofLevelStar
		lsProofLevelPlus
		lsProofName
		lsProofID
		lsOTHER
		lsEndOfFile
	)

	state := lsConsumeLeadingSpace

	for {
		switch state {
		case lsConsumeLeadingSpace:
			if unicode.IsSpace(lexer.Lookahead()) {
				lexer.Advance(true) // SKIP
				state = lsConsumeLeadingSpace
				continue
			}
			col = int16(lexer.Column())
			lexer.MarkEnd()
			if !tlaHasNext(lexer) {
				tlaAdvance(lexer)
				state = lsEndOfFile
				continue
			}
			la := lexer.Lookahead()
			switch {
			case la == '/':
				tlaAdvance(lexer)
				state = lsForwardSlash
			case la == '\\':
				tlaAdvance(lexer)
				state = lsBackwardSlash
			case la == '<':
				tlaAdvance(lexer)
				state = lsLT
			case la == '>':
				tlaAdvance(lexer)
				state = lsGT
			case la == '=':
				tlaAdvance(lexer)
				state = lsEQ
			case la == '-':
				tlaAdvance(lexer)
				state = lsDash
			case la == ',':
				tlaAdvance(lexer)
				state = lsComma
			case la == ':':
				tlaAdvance(lexer)
				state = lsColon
			case la == ';':
				tlaAdvance(lexer)
				state = lsSemicolon
			case la == '(':
				tlaAdvance(lexer)
				state = lsLParen
			case la == ')':
				tlaAdvance(lexer)
				state = lsRParen
			case la == ']':
				tlaAdvance(lexer)
				state = lsRSquareBracket
			case la == '}':
				tlaAdvance(lexer)
				state = lsRCurlyBrace
			case la == '|':
				tlaAdvance(lexer)
				state = lsPipe
			case la == '!':
				tlaAdvance(lexer)
				state = lsExcl
			case la == 'A':
				tlaAdvance(lexer)
				state = lsA
			case la == 'B':
				tlaAdvance(lexer)
				state = lsB
			case la == 'C':
				tlaAdvance(lexer)
				state = lsC
			case la == 'E':
				tlaAdvance(lexer)
				state = lsE
			case la == 'I':
				tlaAdvance(lexer)
				state = lsI
			case la == 'L':
				tlaAdvance(lexer)
				state = lsL
			case la == 'O':
				tlaAdvance(lexer)
				state = lsO
			case la == 'P':
				tlaAdvance(lexer)
				state = lsP
			case la == 'Q':
				tlaAdvance(lexer)
				state = lsQ
			case la == 'S':
				tlaAdvance(lexer)
				state = lsS
			case la == 'T':
				tlaAdvance(lexer)
				state = lsT
			case la == 'V':
				tlaAdvance(lexer)
				state = lsV
			case la == 'W':
				tlaAdvance(lexer)
				state = lsW
			case la == '\u2227': // '∧'
				tlaAdvance(lexer)
				state = lsLand
			case la == '\u2228': // '∨'
				tlaAdvance(lexer)
				state = lsLor
			case la == '\u3009': // '〉'
				tlaAdvance(lexer)
				state = lsRAngleBracket
			case la == '\u27E9': // '⟩'
				tlaAdvance(lexer)
				state = lsRAngleBracket
			case la == '\u27F6': // '⟶'
				tlaAdvance(lexer)
				state = lsRightArrow
			case la == '\u2192': // '→'
				tlaAdvance(lexer)
				state = lsRightArrow
			case la == '\u27FC': // '⟼'
				tlaAdvance(lexer)
				state = lsRightMapArrow
			case la == '\u21A6': // '↦'
				tlaAdvance(lexer)
				state = lsRightMapArrow
			default:
				tlaAdvance(lexer)
				state = lsOTHER
			}
			continue

		case lsForwardSlash:
			resultLexeme = tlaLexForwardSlash
			if lexer.Lookahead() == '\\' {
				tlaAdvance(lexer)
				state = lsLand
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsBackwardSlash:
			resultLexeme = tlaLexBackwardSlash
			la := lexer.Lookahead()
			if la == '/' {
				tlaAdvance(lexer)
				state = lsLor
				continue
			}
			if la == '*' {
				tlaAdvance(lexer)
				state = lsCommentStart
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsLT:
			b := byte(lexer.Lookahead() & 0x7F)
			proofStepIDLevel = append(proofStepIDLevel, b)
			if unicode.IsDigit(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsProofLevelNumber
				continue
			}
			if lexer.Lookahead() == '*' {
				tlaAdvance(lexer)
				state = lsProofLevelStar
				continue
			}
			if lexer.Lookahead() == '+' {
				tlaAdvance(lexer)
				state = lsProofLevelPlus
				continue
			}
			tlaAdvance(lexer)
			state = lsOTHER
			continue

		case lsGT:
			resultLexeme = tlaLexGt
			if lexer.Lookahead() == '>' {
				tlaAdvance(lexer)
				state = lsRAngleBracket
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsEQ:
			resultLexeme = tlaLexEq
			if tlaIsNextSequence(lexer, "===") {
				tlaAdvance(lexer)
				state = lsDoubleLine
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsDash:
			resultLexeme = tlaLexDash
			if lexer.Lookahead() == '>' {
				tlaAdvance(lexer)
				state = lsRightArrow
				continue
			}
			if tlaIsNextSequence(lexer, "---") {
				tlaAdvance(lexer)
				state = lsSingleLine
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsComma:
			resultLexeme = tlaLexComma
			return resultLexeme, col, proofStepIDLevel

		case lsColon:
			resultLexeme = tlaLexColon
			la := lexer.Lookahead()
			if la == ':' {
				tlaAdvance(lexer)
				state = lsOTHER
				continue
			}
			if la == '=' {
				tlaAdvance(lexer)
				state = lsOTHER
				continue
			}
			if la == '>' {
				tlaAdvance(lexer)
				state = lsOTHER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsSemicolon:
			resultLexeme = tlaLexSemicolon
			return resultLexeme, col, proofStepIDLevel

		case lsLand:
			resultLexeme = tlaLexLand
			return resultLexeme, col, proofStepIDLevel

		case lsLor:
			resultLexeme = tlaLexLor
			return resultLexeme, col, proofStepIDLevel

		case lsLParen:
			resultLexeme = tlaLexLParen
			if lexer.Lookahead() == '*' {
				tlaAdvance(lexer)
				state = lsBlockCommentStart
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsRParen:
			resultLexeme = tlaLexRParen
			return resultLexeme, col, proofStepIDLevel

		case lsRSquareBracket:
			resultLexeme = tlaLexRSquareBracket
			return resultLexeme, col, proofStepIDLevel

		case lsRCurlyBrace:
			resultLexeme = tlaLexRCurlyBrace
			return resultLexeme, col, proofStepIDLevel

		case lsRAngleBracket:
			resultLexeme = tlaLexRAngleBracket
			return resultLexeme, col, proofStepIDLevel

		case lsRightArrow:
			resultLexeme = tlaLexRightArrow
			return resultLexeme, col, proofStepIDLevel

		case lsRightMapArrow:
			resultLexeme = tlaLexRightMapArrow
			return resultLexeme, col, proofStepIDLevel

		case lsCommentStart:
			resultLexeme = tlaLexCommentStart
			return resultLexeme, col, proofStepIDLevel

		case lsBlockCommentStart:
			resultLexeme = tlaLexBlockCommentStart
			return resultLexeme, col, proofStepIDLevel

		case lsSingleLine:
			resultLexeme = tlaLexSingleLine
			return resultLexeme, col, proofStepIDLevel

		case lsDoubleLine:
			resultLexeme = tlaLexDoubleLine
			return resultLexeme, col, proofStepIDLevel

		case lsPipe:
			if lexer.Lookahead() == '-' {
				tlaAdvance(lexer)
				state = lsRightTurnstile
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsExcl:
			if lexer.Lookahead() == '!' {
				tlaAdvance(lexer)
				state = lsDoubleExcl
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsDoubleExcl:
			resultLexeme = tlaLexDoubleExcl
			return resultLexeme, col, proofStepIDLevel

		case lsRightTurnstile:
			if lexer.Lookahead() == '>' {
				tlaAdvance(lexer)
				state = lsRightMapArrow
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		// ---- Keyword recognition states ----

		case lsA:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'X' {
				tlaAdvance(lexer)
				state = lsAX
				continue
			}
			if tlaIsNextSequence(lexer, "SSUM") {
				tlaAdvance(lexer)
				state = lsASSUM
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsASSUM:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'E' {
				tlaAdvance(lexer)
				state = lsASSUME
				continue
			}
			if tlaIsNextSequence(lexer, "PTION") {
				tlaAdvance(lexer)
				state = lsASSUMPTION
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsASSUME:
			resultLexeme = tlaLexAssumeKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsASSUMPTION:
			resultLexeme = tlaLexAssumptionKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsAX:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "IOM") {
				tlaAdvance(lexer)
				state = lsAXIOM
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsAXIOM:
			resultLexeme = tlaLexAxiomKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsB:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'Y' {
				tlaAdvance(lexer)
				state = lsBY
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsBY:
			resultLexeme = tlaLexByKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsC:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'O' {
				tlaAdvance(lexer)
				state = lsCO
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsCO:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'N' {
				tlaAdvance(lexer)
				state = lsCON
				continue
			}
			if lexer.Lookahead() == 'R' {
				tlaAdvance(lexer)
				state = lsCOR
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsCON:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "STANT") {
				tlaAdvance(lexer)
				state = lsCONSTANT
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsCONSTANT:
			resultLexeme = tlaLexConstantKeyword
			if lexer.Lookahead() == 'S' {
				tlaAdvance(lexer)
				state = lsCONSTANTS
				continue
			}
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsCONSTANTS:
			resultLexeme = tlaLexConstantsKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsCOR:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "OLLARY") {
				tlaAdvance(lexer)
				state = lsCOROLLARY
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsCOROLLARY:
			resultLexeme = tlaLexCorollaryKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsE:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "LSE") {
				tlaAdvance(lexer)
				state = lsELSE
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsELSE:
			resultLexeme = tlaLexElseKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsI:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'N' {
				tlaAdvance(lexer)
				state = lsIN
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsIN:
			resultLexeme = tlaLexInKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsL:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'E' {
				tlaAdvance(lexer)
				state = lsLE
				continue
			}
			if lexer.Lookahead() == 'O' {
				tlaAdvance(lexer)
				state = lsLO
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsLE:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "MMA") {
				tlaAdvance(lexer)
				state = lsLEMMA
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsLEMMA:
			resultLexeme = tlaLexLemmaKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsLO:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "CAL") {
				tlaAdvance(lexer)
				state = lsLOCAL
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsLOCAL:
			resultLexeme = tlaLexLocalKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsO:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'B' {
				tlaAdvance(lexer)
				state = lsOB
				continue
			}
			if lexer.Lookahead() == 'M' {
				tlaAdvance(lexer)
				state = lsOM
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsOB:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "VIOUS") {
				tlaAdvance(lexer)
				state = lsOBVIOUS
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsOBVIOUS:
			resultLexeme = tlaLexObviousKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsOM:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "ITTED") {
				tlaAdvance(lexer)
				state = lsOMITTED
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsOMITTED:
			resultLexeme = tlaLexOmittedKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsP:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "RO") {
				tlaAdvance(lexer)
				state = lsPRO
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsPRO:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'O' {
				tlaAdvance(lexer)
				state = lsPROO
				continue
			}
			if lexer.Lookahead() == 'P' {
				tlaAdvance(lexer)
				state = lsPROP
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsPROO:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'F' {
				tlaAdvance(lexer)
				state = lsPROOF
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsPROOF:
			resultLexeme = tlaLexProofKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsPROP:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "OSITION") {
				tlaAdvance(lexer)
				state = lsPROPOSITION
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsPROPOSITION:
			resultLexeme = tlaLexPropositionKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsQ:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "ED") {
				tlaAdvance(lexer)
				state = lsQED
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsQED:
			resultLexeme = tlaLexQedKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsS:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "F_") {
				tlaAdvance(lexer)
				state = lsSF_
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsSF_:
			resultLexeme = tlaLexStrongFairness
			return resultLexeme, col, proofStepIDLevel

		case lsT:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "HE") {
				tlaAdvance(lexer)
				state = lsTHE
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsTHE:
			resultLexeme = tlaLexIdentifier
			if lexer.Lookahead() == 'N' {
				tlaAdvance(lexer)
				state = lsTHEN
				continue
			}
			if tlaIsNextSequence(lexer, "OREM") {
				tlaAdvance(lexer)
				state = lsTHEOREM
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsTHEN:
			resultLexeme = tlaLexThenKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsTHEOREM:
			resultLexeme = tlaLexTheoremKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsV:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "ARIABLE") {
				tlaAdvance(lexer)
				state = lsVARIABLE
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsVARIABLE:
			resultLexeme = tlaLexVariableKeyword
			if lexer.Lookahead() == 'S' {
				tlaAdvance(lexer)
				state = lsVARIABLES
				continue
			}
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsVARIABLES:
			resultLexeme = tlaLexVariablesKeyword
			if tlaIsIdentifierChar(lexer.Lookahead()) {
				tlaAdvance(lexer)
				state = lsIDENTIFIER
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsW:
			resultLexeme = tlaLexIdentifier
			if tlaIsNextSequence(lexer, "F_") {
				tlaAdvance(lexer)
				state = lsWF_
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsWF_:
			resultLexeme = tlaLexWeakFairness
			return resultLexeme, col, proofStepIDLevel

		// ---- Proof step ID states ----

		case lsProofLevelNumber:
			if unicode.IsDigit(lexer.Lookahead()) {
				proofStepIDLevel = append(proofStepIDLevel, byte(lexer.Lookahead()&0x7F))
				tlaAdvance(lexer)
				state = lsProofLevelNumber
				continue
			}
			if lexer.Lookahead() == '>' {
				tlaAdvance(lexer)
				state = lsProofName
				continue
			}
			tlaAdvance(lexer)
			state = lsOTHER
			continue

		case lsProofLevelStar:
			if lexer.Lookahead() == '>' {
				tlaAdvance(lexer)
				state = lsProofName
				continue
			}
			tlaAdvance(lexer)
			state = lsOTHER
			continue

		case lsProofLevelPlus:
			if lexer.Lookahead() == '>' {
				tlaAdvance(lexer)
				state = lsProofName
				continue
			}
			tlaAdvance(lexer)
			state = lsOTHER
			continue

		case lsProofName:
			resultLexeme = tlaLexProofStepID
			la := lexer.Lookahead()
			if unicode.IsLetter(la) || unicode.IsDigit(la) {
				tlaAdvance(lexer)
				state = lsProofName
				continue
			}
			if la == '.' {
				tlaAdvance(lexer)
				state = lsProofID
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsProofID:
			resultLexeme = tlaLexProofStepID
			if lexer.Lookahead() == '.' {
				tlaAdvance(lexer)
				state = lsProofID
				continue
			}
			return resultLexeme, col, proofStepIDLevel

		case lsIDENTIFIER:
			resultLexeme = tlaLexIdentifier
			return resultLexeme, col, proofStepIDLevel

		case lsOTHER:
			resultLexeme = tlaLexOther
			return resultLexeme, col, proofStepIDLevel

		case lsEndOfFile:
			resultLexeme = tlaLexEndOfFile
			return resultLexeme, col, proofStepIDLevel
		}
	}
}

// ---------------------------------------------------------------------------
// Junction list / proof emission helpers on tlaScanner
// ---------------------------------------------------------------------------

func (s *tlaScanner) emitIndent(
	lexer *gotreesitter.ExternalLexer,
	jt tlaJunctType,
	col int16,
) bool {
	lexer.SetResultSymbol(tlaSymIndent)
	s.jlists = append(s.jlists, tlaJunctList{jType: jt, alignmentColumn: col})
	return true
}

func (s *tlaScanner) emitBullet(lexer *gotreesitter.ExternalLexer) bool {
	lexer.SetResultSymbol(tlaSymBullet)
	return true
}

func (s *tlaScanner) emitDedent(lexer *gotreesitter.ExternalLexer) bool {
	if s.isInJlist() {
		lexer.SetResultSymbol(tlaSymDedent)
		s.jlists = s.jlists[:len(s.jlists)-1]
		return true
	}
	return false
}

// isJunctTokenHigherLevelOpParameter checks whether the next non-whitespace
// char after a junct token is , or ), indicating it is a higher-level
// parameter rather than a jlist start.
func tlaIsJunctTokenHigherLevelOpParameter(lexer *gotreesitter.ExternalLexer) bool {
	for unicode.IsSpace(lexer.Lookahead()) && tlaHasNext(lexer) {
		lexer.Advance(true)
	}
	return lexer.Lookahead() == ',' || lexer.Lookahead() == ')'
}

func (s *tlaScanner) handleJunctToken(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
	nextType tlaJunctType,
	nextCol int16,
) bool {
	currentCol := s.getCurrentJlistColumnIndex()
	if currentCol < nextCol {
		if validSymbols[tlaTokIndent] {
			if tlaIsJunctTokenHigherLevelOpParameter(lexer) {
				return false
			}
			return s.emitIndent(lexer, nextType, nextCol)
		}
		return false
	} else if currentCol == nextCol {
		if s.currentJlistTypeIs(nextType) {
			return s.emitBullet(lexer)
		}
		return s.emitDedent(lexer)
	} else {
		return s.emitDedent(lexer)
	}
}

func (s *tlaScanner) handleRightDelimiterToken(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) bool {
	return s.isInJlist() && validSymbols[tlaTokDedent] && s.emitDedent(lexer)
}

func (s *tlaScanner) handleTerminatorToken(
	lexer *gotreesitter.ExternalLexer,
) bool {
	return s.isInJlist() && s.emitDedent(lexer)
}

func (s *tlaScanner) handleOtherToken(
	lexer *gotreesitter.ExternalLexer,
	next int16,
) bool {
	return s.isInJlist() &&
		next <= s.getCurrentJlistColumnIndex() &&
		s.emitDedent(lexer)
}

func (s *tlaScanner) handleDoubleExclToken(
	lexer *gotreesitter.ExternalLexer,
	next int16,
) bool {
	if s.handleOtherToken(lexer, next) {
		return true
	}
	if lexer.Lookahead() == '!' {
		return false
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(tlaSymDoubleExcl)
	return true
}

func (s *tlaScanner) emitBeginProof(
	lexer *gotreesitter.ExternalLexer,
	level int32,
) bool {
	lexer.SetResultSymbol(tlaSymBeginProof)
	s.proofs = append(s.proofs, level)
	s.lastProofLevel = level
	s.haveSeenProofKeyword = false
	return true
}

func (s *tlaScanner) emitBeginProofStep(
	lexer *gotreesitter.ExternalLexer,
	level int32,
) bool {
	s.lastProofLevel = level
	lexer.SetResultSymbol(tlaSymBeginProofStep)
	return true
}

func (s *tlaScanner) handleProofStepIDToken(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
	next int16,
	psid tlaProofStepID,
) bool {
	if psid.idType == tlaProofStepNone {
		return false
	}
	if validSymbols[tlaTokBeginProof] || validSymbols[tlaTokBeginProofStep] {
		var nextProofLevel int32 = -1
		currentProofLevel := s.getCurrentProofLevel()
		switch psid.idType {
		case tlaProofStepStar:
			if !s.isInProof() || s.haveSeenProofKeyword {
				nextProofLevel = s.lastProofLevel + 1
			} else {
				nextProofLevel = currentProofLevel
			}
		case tlaProofStepPlus:
			if validSymbols[tlaTokBeginProof] {
				nextProofLevel = s.lastProofLevel + 1
			} else {
				nextProofLevel = currentProofLevel
			}
		case tlaProofStepNumbered:
			nextProofLevel = psid.level
		default:
			return false
		}

		if nextProofLevel > currentProofLevel {
			return s.emitBeginProof(lexer, nextProofLevel)
		} else if nextProofLevel == currentProofLevel {
			if s.haveSeenProofKeyword {
				return false
			}
			return s.emitBeginProofStep(lexer, nextProofLevel)
		} else {
			return false
		}
	} else {
		if validSymbols[tlaTokDedent] {
			return s.handleTerminatorToken(lexer)
		}
		return s.handleOtherToken(lexer, next)
	}
}

func (s *tlaScanner) handleProofKeywordToken(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) bool {
	if validSymbols[tlaTokProofKeyword] {
		s.haveSeenProofKeyword = true
		lexer.SetResultSymbol(tlaSymProofKeyword)
		lexer.MarkEnd()
		return true
	}
	return s.handleTerminatorToken(lexer)
}

func (s *tlaScanner) handleTerminalProofKeywordToken(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
	keywordTokIdx int,
	keywordSym gotreesitter.Symbol,
) bool {
	if validSymbols[keywordTokIdx] {
		s.haveSeenProofKeyword = false
		lexer.SetResultSymbol(keywordSym)
		lexer.MarkEnd()
		return true
	}
	return s.handleTerminatorToken(lexer)
}

func (s *tlaScanner) handleQedKeywordToken(
	lexer *gotreesitter.ExternalLexer,
) bool {
	if s.isInProof() {
		s.lastProofLevel = s.getCurrentProofLevel()
		s.proofs = s.proofs[:len(s.proofs)-1]
	}
	lexer.SetResultSymbol(tlaSymQedKeyword)
	lexer.MarkEnd()
	return true
}

func (s *tlaScanner) handleFairnessKeywordToken(
	lexer *gotreesitter.ExternalLexer,
	next int16,
	keywordSym gotreesitter.Symbol,
) bool {
	if s.handleOtherToken(lexer, next) {
		return true
	}
	lexer.SetResultSymbol(keywordSym)
	lexer.MarkEnd()
	return true
}

// scan is the main scan function for a single context (no PlusCal nesting).
func (s *tlaScanner) scan(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) bool {
	isErrorRecovery := validSymbols[tlaTokErrorSentinel]
	if isErrorRecovery {
		return false
	}

	if validSymbols[tlaTokLeadingExtramodularText] || validSymbols[tlaTokTrailingExtramodularText] {
		return tlaScanExtramodularText(lexer, validSymbols)
	}

	lex, col, proofStepIDLevel := tlaLexLookahead(lexer)
	token := tlaTokenizeLexeme(lex)
	psid := tlaParseProofStepID(proofStepIDLevel)

	switch token {
	case tlaTknLand:
		return s.handleJunctToken(lexer, validSymbols, tlaJunctConjunction, col)
	case tlaTknLor:
		return s.handleJunctToken(lexer, validSymbols, tlaJunctDisjunction, col)
	case tlaTknRightDelimiter:
		return s.handleRightDelimiterToken(lexer, validSymbols)
	case tlaTknCommentStart:
		return false
	case tlaTknTerminator:
		return s.handleTerminatorToken(lexer)
	case tlaTknProofStepID:
		return s.handleProofStepIDToken(lexer, validSymbols, col, psid)
	case tlaTknProofKeyword:
		return s.handleProofKeywordToken(lexer, validSymbols)
	case tlaTknByKeyword:
		return s.handleTerminalProofKeywordToken(lexer, validSymbols, tlaTokByKeyword, tlaSymByKeyword)
	case tlaTknObviousKeyword:
		return s.handleTerminalProofKeywordToken(lexer, validSymbols, tlaTokObviousKeyword, tlaSymObviousKeyword)
	case tlaTknOmittedKeyword:
		return s.handleTerminalProofKeywordToken(lexer, validSymbols, tlaTokOmittedKeyword, tlaSymOmittedKeyword)
	case tlaTknQedKeyword:
		return s.handleQedKeywordToken(lexer)
	case tlaTknWeakFairness:
		return s.handleFairnessKeywordToken(lexer, col, tlaSymWeakFairness)
	case tlaTknStrongFairness:
		return s.handleFairnessKeywordToken(lexer, col, tlaSymStrongFairness)
	case tlaTknDoubleExcl:
		return s.handleDoubleExclToken(lexer, col)
	case tlaTknOther:
		return s.handleOtherToken(lexer, col)
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// tlaNestedScanner — handles PlusCal context nesting
// ---------------------------------------------------------------------------

type tlaNestedScanner struct {
	enclosingContexts [][]byte
	currentContext    *tlaScanner
}

func newTlaNestedScanner() *tlaNestedScanner {
	return &tlaNestedScanner{
		currentContext: newTlaScanner(),
	}
}

func (ns *tlaNestedScanner) serialize() []byte {
	// Layout:
	//   int16: context_depth (= len(enclosingContexts) + 1)
	//   N * uint32: sizes of all contexts (N-1 enclosing + 1 current)
	//   raw bytes of each enclosing context
	//   raw bytes of current context
	contextDepth := int16(len(ns.enclosingContexts) + 1)
	currentBytes := ns.currentContext.serialize()

	// Pre-compute total size
	totalSize := 2                     // context_depth
	totalSize += int(contextDepth) * 4 // context sizes (uint32 each, matching C's unsigned)
	for _, ctx := range ns.enclosingContexts {
		totalSize += len(ctx)
	}
	totalSize += len(currentBytes)

	buf := make([]byte, totalSize)
	offset := 0

	// Write context depth
	binary.LittleEndian.PutUint16(buf[offset:], uint16(contextDepth))
	offset += 2

	// Write sizes of N-1 enclosing contexts
	for _, ctx := range ns.enclosingContexts {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(len(ctx)))
		offset += 4
	}

	// Write size of current context
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(currentBytes)))
	offset += 4

	// Write N-1 enclosing contexts
	for _, ctx := range ns.enclosingContexts {
		copy(buf[offset:], ctx)
		offset += len(ctx)
	}

	// Write current context
	copy(buf[offset:], currentBytes)

	return buf
}

func (ns *tlaNestedScanner) deserialize(data []byte) {
	ns.enclosingContexts = nil
	ns.currentContext.reset()

	if len(data) == 0 {
		return
	}

	offset := 0

	contextDepth := int16(binary.LittleEndian.Uint16(data[offset:]))
	offset += 2

	// Read N context sizes
	contextSizes := make([]uint32, contextDepth)
	for i := int16(0); i < contextDepth; i++ {
		contextSizes[i] = binary.LittleEndian.Uint32(data[offset:])
		offset += 4
	}

	// Deserialize N-1 enclosing contexts (stored as raw bytes)
	for i := int16(0); i < contextDepth-1; i++ {
		sz := contextSizes[i]
		ctx := make([]byte, sz)
		copy(ctx, data[offset:offset+int(sz)])
		offset += int(sz)
		ns.enclosingContexts = append(ns.enclosingContexts, ctx)
	}

	// Deserialize current context
	sz := contextSizes[contextDepth-1]
	ns.currentContext.deserialize(data[offset : offset+int(sz)])
}

func (ns *tlaNestedScanner) scan(
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) bool {
	if validSymbols[tlaTokErrorSentinel] {
		return false
	}

	if validSymbols[tlaTokPcalStart] {
		// Entering PlusCal block: push current context, then clear
		serialized := ns.currentContext.serialize()
		ns.enclosingContexts = append(ns.enclosingContexts, serialized)
		ns.currentContext.reset()
		lexer.SetResultSymbol(tlaSymPcalStart)
		return true
	}

	if validSymbols[tlaTokPcalEnd] && len(ns.enclosingContexts) > 0 {
		// Exiting PlusCal block: rehydrate context, then pop
		last := ns.enclosingContexts[len(ns.enclosingContexts)-1]
		ns.currentContext.deserialize(last)
		ns.enclosingContexts = ns.enclosingContexts[:len(ns.enclosingContexts)-1]
		lexer.SetResultSymbol(tlaSymPcalEnd)
		return true
	}

	return ns.currentContext.scan(lexer, validSymbols)
}

// ---------------------------------------------------------------------------
// TlaplusExternalScanner — implements gotreesitter.ExternalScanner
// ---------------------------------------------------------------------------

// TlaplusExternalScanner handles junction lists, proofs, extramodular text,
// fairness operators, and PlusCal context nesting for the TLA+ grammar.
type TlaplusExternalScanner struct{}

func (TlaplusExternalScanner) Create() any {
	return newTlaNestedScanner()
}

func (TlaplusExternalScanner) Destroy(payload any) {}

func (TlaplusExternalScanner) Serialize(payload any, buf []byte) int {
	ns := payload.(*tlaNestedScanner)
	data := ns.serialize()
	if len(data) > len(buf) {
		return 0
	}
	copy(buf, data)
	return len(data)
}

func (TlaplusExternalScanner) Deserialize(payload any, buf []byte) {
	ns := payload.(*tlaNestedScanner)
	ns.deserialize(buf)
}

func (TlaplusExternalScanner) Scan(
	payload any,
	lexer *gotreesitter.ExternalLexer,
	validSymbols []bool,
) bool {
	ns := payload.(*tlaNestedScanner)
	return ns.scan(lexer, validSymbols)
}
