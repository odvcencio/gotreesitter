//go:build !grammar_subset || grammar_subset_yaml

package grammars

import (
	"encoding/binary"

	gotreesitter "github.com/odvcencio/gotreesitter"
)

// YamlExternalScanner is a direct port of tree-sitter-yaml's C external
// scanner (scanner.c + schema.core.c). It tracks indentation state and
// uses the parser's validSymbols context to emit the correct token variant.

// ---------------------------------------------------------------------------
// Section 1: Constants
// ---------------------------------------------------------------------------

// External token indices — must match the enum order in scanner.c exactly.
const (
	yTokEndOfFile = iota

	yTokSDirYmlBgn
	yTokRDirYmlVer
	yTokSDirTagBgn
	yTokRDirTagHdl
	yTokRDirTagPfx
	yTokSDirRsvBgn
	yTokRDirRsvPrm
	yTokSDrsEnd
	yTokSDocEnd
	yTokRBlkSeqBgn
	yTokBRBlkSeqBgn
	yTokBBlkSeqBgn
	yTokRBlkKeyBgn
	yTokBRBlkKeyBgn
	yTokBBlkKeyBgn
	yTokRBlkValBgn
	yTokBRBlkValBgn
	yTokBBlkValBgn
	yTokRBlkImpBgn
	yTokRBlkLitBgn
	yTokBRBlkLitBgn
	yTokRBlkFldBgn
	yTokBRBlkFldBgn
	yTokBRBlkStrCtn
	yTokRFlwSeqBgn
	yTokBRFlwSeqBgn
	yTokBFlwSeqBgn
	yTokRFlwSeqEnd
	yTokBRFlwSeqEnd
	yTokBFlwSeqEnd
	yTokRFlwMapBgn
	yTokBRFlwMapBgn
	yTokBFlwMapBgn
	yTokRFlwMapEnd
	yTokBRFlwMapEnd
	yTokBFlwMapEnd
	yTokRFlwSepBgn
	yTokBRFlwSepBgn
	yTokRFlwKeyBgn
	yTokBRFlwKeyBgn
	yTokRFlwJsvBgn
	yTokBRFlwJsvBgn
	yTokRFlwNjvBgn
	yTokBRFlwNjvBgn
	yTokRDqtStrBgn
	yTokBRDqtStrBgn
	yTokBDqtStrBgn
	yTokRDqtStrCtn
	yTokBRDqtStrCtn
	yTokRDqtEscNwl
	yTokBRDqtEscNwl
	yTokRDqtEscSeq
	yTokBRDqtEscSeq
	yTokRDqtStrEnd
	yTokBRDqtStrEnd
	yTokRSqtStrBgn
	yTokBRSqtStrBgn
	yTokBSqtStrBgn
	yTokRSqtStrCtn
	yTokBRSqtStrCtn
	yTokRSqtEscSqt
	yTokBRSqtEscSqt
	yTokRSqtStrEnd
	yTokBRSqtStrEnd

	yTokRSglPlnNulBlk
	yTokBRSglPlnNulBlk
	yTokBSglPlnNulBlk
	yTokRSglPlnNulFlw
	yTokBRSglPlnNulFlw
	yTokRSglPlnBolBlk
	yTokBRSglPlnBolBlk
	yTokBSglPlnBolBlk
	yTokRSglPlnBolFlw
	yTokBRSglPlnBolFlw
	yTokRSglPlnIntBlk
	yTokBRSglPlnIntBlk
	yTokBSglPlnIntBlk
	yTokRSglPlnIntFlw
	yTokBRSglPlnIntFlw
	yTokRSglPlnFltBlk
	yTokBRSglPlnFltBlk
	yTokBSglPlnFltBlk
	yTokRSglPlnFltFlw
	yTokBRSglPlnFltFlw
	yTokRSglPlnTmsBlk
	yTokBRSglPlnTmsBlk
	yTokBSglPlnTmsBlk
	yTokRSglPlnTmsFlw
	yTokBRSglPlnTmsFlw
	yTokRSglPlnStrBlk
	yTokBRSglPlnStrBlk
	yTokBSglPlnStrBlk
	yTokRSglPlnStrFlw
	yTokBRSglPlnStrFlw

	yTokRMtlPlnStrBlk
	yTokBRMtlPlnStrBlk
	yTokRMtlPlnStrFlw
	yTokBRMtlPlnStrFlw

	yTokRTag
	yTokBRTag
	yTokBTag
	yTokRAcrBgn
	yTokBRAcrBgn
	yTokBAcrBgn
	yTokRAcrCtn
	yTokRAlsBgn
	yTokBRAlsBgn
	yTokBAlsBgn
	yTokRAlsCtn

	yTokBL
	yTokComment

	yTokErrRec

	yTokCount // 113
)

// Indentation types.
const (
	yIndRot int16 = 'r'
	yIndMap int16 = 'm'
	yIndSeq int16 = 'q'
	yIndStr int16 = 's'
)

// Sub-scanner return codes.
const (
	scnSucc int8 = 1
	scnStop int8 = 0
	scnFail int8 = -1
)

// Schema result types — matches schema.core.c ResultSchema.
const (
	rsStr  int8 = 0
	rsInt  int8 = 1
	rsNull int8 = 2
	rsBool int8 = 3
	rsFlt  int8 = 4
)

const schSttFrz int8 = -1

// ---------------------------------------------------------------------------
// Section 2: Types
// ---------------------------------------------------------------------------

// yamlState holds the persistent scanner state (serialized between parses).
type yamlState struct {
	row       int16
	col       int16
	blkImpRow int16
	blkImpCol int16
	blkImpTab int16
	indTypStk []int16
	indLenStk []int16
}

// yamlEnv holds transient per-Scan state.
type yamlEnv struct {
	lexer        *gotreesitter.ExternalLexer
	validSymbols []bool
	st           *yamlState
	symMap       []gotreesitter.Symbol

	endRow int16
	endCol int16
	curRow int16
	curCol int16
	curChr int32
	schStt int8
	rltSch int8
}

// yamlPayload is stored as the scanner payload.
type yamlPayload struct {
	state  yamlState
	symMap []gotreesitter.Symbol // external token index → grammar Symbol
}

// ---------------------------------------------------------------------------
// Section 3: ExternalScanner interface
// ---------------------------------------------------------------------------

type YamlExternalScanner struct{}

func (YamlExternalScanner) Create() any {
	p := &yamlPayload{}
	p.state.blkImpRow = -1
	p.state.blkImpCol = -1
	p.state.indTypStk = []int16{yIndRot}
	p.state.indLenStk = []int16{-1}
	return p
}

func (YamlExternalScanner) Destroy(payload any) {}

func (YamlExternalScanner) Serialize(payload any, buf []byte) int {
	p := payload.(*yamlPayload)
	s := &p.state
	// Header: 5 × int16 = 10 bytes
	if len(buf) < 10 {
		return 0
	}
	size := 0
	binary.LittleEndian.PutUint16(buf[size:], uint16(s.row))
	size += 2
	binary.LittleEndian.PutUint16(buf[size:], uint16(s.col))
	size += 2
	binary.LittleEndian.PutUint16(buf[size:], uint16(s.blkImpRow))
	size += 2
	binary.LittleEndian.PutUint16(buf[size:], uint16(s.blkImpCol))
	size += 2
	binary.LittleEndian.PutUint16(buf[size:], uint16(s.blkImpTab))
	size += 2
	// Stack entries (skip index 0 = root sentinel)
	for i := 1; i < len(s.indTypStk) && size+4 <= len(buf); i++ {
		binary.LittleEndian.PutUint16(buf[size:], uint16(s.indTypStk[i]))
		size += 2
		binary.LittleEndian.PutUint16(buf[size:], uint16(s.indLenStk[i]))
		size += 2
	}
	return size
}

func (YamlExternalScanner) Deserialize(payload any, buf []byte) {
	p := payload.(*yamlPayload)
	s := &p.state
	s.row = 0
	s.col = 0
	s.blkImpRow = -1
	s.blkImpCol = -1
	s.blkImpTab = 0
	s.indTypStk = []int16{yIndRot}
	s.indLenStk = []int16{-1}
	if len(buf) >= 10 {
		off := 0
		s.row = int16(binary.LittleEndian.Uint16(buf[off:]))
		off += 2
		s.col = int16(binary.LittleEndian.Uint16(buf[off:]))
		off += 2
		s.blkImpRow = int16(binary.LittleEndian.Uint16(buf[off:]))
		off += 2
		s.blkImpCol = int16(binary.LittleEndian.Uint16(buf[off:]))
		off += 2
		s.blkImpTab = int16(binary.LittleEndian.Uint16(buf[off:]))
		off += 2
		for off+4 <= len(buf) {
			typ := int16(binary.LittleEndian.Uint16(buf[off:]))
			off += 2
			ln := int16(binary.LittleEndian.Uint16(buf[off:]))
			off += 2
			s.indTypStk = append(s.indTypStk, typ)
			s.indLenStk = append(s.indLenStk, ln)
		}
	}
}

func (YamlExternalScanner) Scan(payload any, lexer *gotreesitter.ExternalLexer, validSymbols []bool) bool {
	p := payload.(*yamlPayload)
	// Lazy-init symbol map on first scan
	if p.symMap == nil {
		lang := YamlLanguage()
		if lang != nil && len(lang.ExternalSymbols) >= yTokCount {
			p.symMap = lang.ExternalSymbols
		} else {
			return false
		}
	}
	env := yamlEnv{
		lexer:        lexer,
		validSymbols: validSymbols,
		st:           &p.state,
		symMap:       p.symMap,
	}
	return env.scan()
}

// ---------------------------------------------------------------------------
// Section 4: Helpers
// ---------------------------------------------------------------------------

func (e *yamlEnv) adv() {
	e.curCol++
	e.curChr = int32(e.lexer.Lookahead())
	e.lexer.Advance(false)
}

func (e *yamlEnv) advNwl() {
	e.curRow++
	e.curCol = 0
	e.curChr = int32(e.lexer.Lookahead())
	e.lexer.Advance(false)
}

func (e *yamlEnv) skp() {
	e.curCol++
	e.curChr = int32(e.lexer.Lookahead())
	e.lexer.Advance(true)
}

func (e *yamlEnv) skpNwl() {
	e.curRow++
	e.curCol = 0
	e.curChr = int32(e.lexer.Lookahead())
	e.lexer.Advance(true)
}

func (e *yamlEnv) mrkEnd() {
	e.endRow = e.curRow
	e.endCol = e.curCol
	e.lexer.MarkEnd()
}

func (e *yamlEnv) init() {
	e.curRow = e.st.row
	e.curCol = e.st.col
	e.curChr = 0
	e.schStt = 0
	e.rltSch = rsStr
}

func (e *yamlEnv) flush() {
	e.st.row = e.endRow
	e.st.col = e.endCol
}

func (e *yamlEnv) retSym(tok int) bool {
	e.flush()
	e.lexer.SetResultSymbol(e.symMap[tok])
	return true
}

func (e *yamlEnv) popInd() bool {
	if len(e.st.indTypStk) == 1 {
		return false // incorrect status caused by error recovering
	}
	e.st.indTypStk = e.st.indTypStk[:len(e.st.indTypStk)-1]
	e.st.indLenStk = e.st.indLenStk[:len(e.st.indLenStk)-1]
	return true
}

func (e *yamlEnv) pushInd(typ, length int16) {
	e.st.indTypStk = append(e.st.indTypStk, typ)
	e.st.indLenStk = append(e.st.indLenStk, length)
}

func (e *yamlEnv) curInd() int16 {
	return e.st.indLenStk[len(e.st.indLenStk)-1]
}

func (e *yamlEnv) curIndTyp() int16 {
	return e.st.indTypStk[len(e.st.indTypStk)-1]
}

func (e *yamlEnv) prtInd() int16 {
	if len(e.st.indLenStk) < 2 {
		return -1
	}
	return e.st.indLenStk[len(e.st.indLenStk)-2]
}

func (e *yamlEnv) lka() int32 { return int32(e.lexer.Lookahead()) }

// Character classification helpers.
func yIsWsp(c int32) bool { return c == ' ' || c == '\t' }
func yIsNwl(c int32) bool { return c == '\r' || c == '\n' }
func yIsWht(c int32) bool { return yIsWsp(c) || yIsNwl(c) || c == 0 }

func yIsNsDecDigit(c int32) bool { return c >= '0' && c <= '9' }

func yIsNsHexDigit(c int32) bool {
	return yIsNsDecDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func yIsNsWordChar(c int32) bool {
	return c == '-' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func yIsNbJson(c int32) bool { return c == 0x09 || (c >= 0x20 && c <= 0x10ffff) }

func yIsNbDoubleChar(c int32) bool { return yIsNbJson(c) && c != '\\' && c != '"' }

func yIsNbSingleChar(c int32) bool { return yIsNbJson(c) && c != '\'' }

func yIsNsChar(c int32) bool {
	return (c >= 0x21 && c <= 0x7e) || c == 0x85 || (c >= 0xa0 && c <= 0xd7ff) ||
		(c >= 0xe000 && c <= 0xfefe) || (c >= 0xff00 && c <= 0xfffd) ||
		(c >= 0x10000 && c <= 0x10ffff)
}

func yIsCIndicator(c int32) bool {
	return c == '-' || c == '?' || c == ':' || c == ',' || c == '[' || c == ']' ||
		c == '{' || c == '}' || c == '#' || c == '&' || c == '*' || c == '!' ||
		c == '|' || c == '>' || c == '\'' || c == '"' || c == '%' || c == '@' || c == '`'
}

func yIsCFlowIndicator(c int32) bool {
	return c == ',' || c == '[' || c == ']' || c == '{' || c == '}'
}

func yIsPlainSafeInBlock(c int32) bool { return yIsNsChar(c) }

func yIsPlainSafeInFlow(c int32) bool { return yIsNsChar(c) && !yIsCFlowIndicator(c) }

func yIsNsUriChar(c int32) bool {
	return yIsNsWordChar(c) || c == '#' || c == ';' || c == '/' || c == '?' || c == ':' ||
		c == '@' || c == '&' || c == '=' || c == '+' || c == '$' || c == ',' || c == '_' ||
		c == '.' || c == '!' || c == '~' || c == '*' || c == '\'' || c == '(' || c == ')' ||
		c == '[' || c == ']'
}

func yIsNsTagChar(c int32) bool {
	return yIsNsWordChar(c) || c == '#' || c == ';' || c == '/' || c == '?' || c == ':' ||
		c == '@' || c == '&' || c == '=' || c == '+' || c == '$' || c == '_' || c == '.' ||
		c == '~' || c == '*' || c == '\'' || c == '(' || c == ')'
}

func yIsNsAnchorChar(c int32) bool { return yIsNsChar(c) && !yIsCFlowIndicator(c) }

// ---------------------------------------------------------------------------
// Section 5: Schema FSM (schema.core.c port)
// ---------------------------------------------------------------------------

func (e *yamlEnv) advSchStt(curChr int32) {
	rlt := &e.rltSch
	switch e.schStt {
	case schSttFrz:
		// Keep frozen state, but still run the post-switch non-whitespace
		// coercion to string (matches upstream schema.core.c behavior).
		break
	case 0:
		if curChr == '.' {
			*rlt = rsStr
			e.schStt = 6
			return
		}
		if curChr == '0' {
			*rlt = rsInt
			e.schStt = 37
			return
		}
		if curChr == 'F' {
			*rlt = rsStr
			e.schStt = 2
			return
		}
		if curChr == 'N' {
			*rlt = rsStr
			e.schStt = 16
			return
		}
		if curChr == 'T' {
			*rlt = rsStr
			e.schStt = 13
			return
		}
		if curChr == 'f' {
			*rlt = rsStr
			e.schStt = 17
			return
		}
		if curChr == 'n' {
			*rlt = rsStr
			e.schStt = 29
			return
		}
		if curChr == 't' {
			*rlt = rsStr
			e.schStt = 26
			return
		}
		if curChr == '~' {
			*rlt = rsNull
			e.schStt = 35
			return
		}
		if curChr == '+' || curChr == '-' {
			*rlt = rsStr
			e.schStt = 1
			return
		}
		if curChr >= '1' && curChr <= '9' {
			*rlt = rsInt
			e.schStt = 38
			return
		}
	case 1:
		if curChr == '.' {
			*rlt = rsStr
			e.schStt = 7
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsInt
			e.schStt = 38
			return
		}
	case 2:
		if curChr == 'A' {
			*rlt = rsStr
			e.schStt = 9
			return
		}
		if curChr == 'a' {
			*rlt = rsStr
			e.schStt = 22
			return
		}
	case 3:
		if curChr == 'A' {
			*rlt = rsStr
			e.schStt = 12
			return
		}
		if curChr == 'a' {
			*rlt = rsStr
			e.schStt = 12
			return
		}
	case 4:
		if curChr == 'E' {
			*rlt = rsBool
			e.schStt = 36
			return
		}
	case 5:
		if curChr == 'F' {
			*rlt = rsFlt
			e.schStt = 41
			return
		}
	case 6:
		if curChr == 'I' {
			*rlt = rsStr
			e.schStt = 11
			return
		}
		if curChr == 'N' {
			*rlt = rsStr
			e.schStt = 3
			return
		}
		if curChr == 'i' {
			*rlt = rsStr
			e.schStt = 24
			return
		}
		if curChr == 'n' {
			*rlt = rsStr
			e.schStt = 18
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsFlt
			e.schStt = 42
			return
		}
	case 7:
		if curChr == 'I' {
			*rlt = rsStr
			e.schStt = 11
			return
		}
		if curChr == 'i' {
			*rlt = rsStr
			e.schStt = 24
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsFlt
			e.schStt = 42
			return
		}
	case 8:
		if curChr == 'L' {
			*rlt = rsNull
			e.schStt = 35
			return
		}
	case 9:
		if curChr == 'L' {
			*rlt = rsStr
			e.schStt = 14
			return
		}
	case 10:
		if curChr == 'L' {
			*rlt = rsStr
			e.schStt = 8
			return
		}
	case 11:
		if curChr == 'N' {
			*rlt = rsStr
			e.schStt = 5
			return
		}
		if curChr == 'n' {
			*rlt = rsStr
			e.schStt = 20
			return
		}
	case 12:
		if curChr == 'N' {
			*rlt = rsFlt
			e.schStt = 41
			return
		}
	case 13:
		if curChr == 'R' {
			*rlt = rsStr
			e.schStt = 15
			return
		}
		if curChr == 'r' {
			*rlt = rsStr
			e.schStt = 28
			return
		}
	case 14:
		if curChr == 'S' {
			*rlt = rsStr
			e.schStt = 4
			return
		}
	case 15:
		if curChr == 'U' {
			*rlt = rsStr
			e.schStt = 4
			return
		}
	case 16:
		if curChr == 'U' {
			*rlt = rsStr
			e.schStt = 10
			return
		}
		if curChr == 'u' {
			*rlt = rsStr
			e.schStt = 23
			return
		}
	case 17:
		if curChr == 'a' {
			*rlt = rsStr
			e.schStt = 22
			return
		}
	case 18:
		if curChr == 'a' {
			*rlt = rsStr
			e.schStt = 25
			return
		}
	case 19:
		if curChr == 'e' {
			*rlt = rsBool
			e.schStt = 36
			return
		}
	case 20:
		if curChr == 'f' {
			*rlt = rsFlt
			e.schStt = 41
			return
		}
	case 21:
		if curChr == 'l' {
			*rlt = rsNull
			e.schStt = 35
			return
		}
	case 22:
		if curChr == 'l' {
			*rlt = rsStr
			e.schStt = 27
			return
		}
	case 23:
		if curChr == 'l' {
			*rlt = rsStr
			e.schStt = 21
			return
		}
	case 24:
		if curChr == 'n' {
			*rlt = rsStr
			e.schStt = 20
			return
		}
	case 25:
		if curChr == 'n' {
			*rlt = rsFlt
			e.schStt = 41
			return
		}
	case 26:
		if curChr == 'r' {
			*rlt = rsStr
			e.schStt = 28
			return
		}
	case 27:
		if curChr == 's' {
			*rlt = rsStr
			e.schStt = 19
			return
		}
	case 28:
		if curChr == 'u' {
			*rlt = rsStr
			e.schStt = 19
			return
		}
	case 29:
		if curChr == 'u' {
			*rlt = rsStr
			e.schStt = 23
			return
		}
	case 30:
		if curChr == '+' || curChr == '-' {
			*rlt = rsStr
			e.schStt = 32
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsFlt
			e.schStt = 43
			return
		}
	case 31:
		if curChr >= '0' && curChr <= '7' {
			*rlt = rsInt
			e.schStt = 39
			return
		}
	case 32:
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsFlt
			e.schStt = 43
			return
		}
	case 33:
		if yIsNsHexDigit(curChr) {
			*rlt = rsInt
			e.schStt = 40
			return
		}
	case 34:
		// C does abort() here; we just freeze.
	case 35:
		*rlt = rsNull
	case 36:
		*rlt = rsBool
	case 37:
		*rlt = rsInt
		if curChr == '.' {
			*rlt = rsFlt
			e.schStt = 42
			return
		}
		if curChr == 'o' {
			*rlt = rsStr
			e.schStt = 31
			return
		}
		if curChr == 'x' {
			*rlt = rsStr
			e.schStt = 33
			return
		}
		if curChr == 'E' || curChr == 'e' {
			*rlt = rsStr
			e.schStt = 30
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsInt
			e.schStt = 38
			return
		}
	case 38:
		*rlt = rsInt
		if curChr == '.' {
			*rlt = rsFlt
			e.schStt = 42
			return
		}
		if curChr == 'E' || curChr == 'e' {
			*rlt = rsStr
			e.schStt = 30
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsInt
			e.schStt = 38
			return
		}
	case 39:
		*rlt = rsInt
		if curChr >= '0' && curChr <= '7' {
			*rlt = rsInt
			e.schStt = 39
			return
		}
	case 40:
		*rlt = rsInt
		if yIsNsHexDigit(curChr) {
			*rlt = rsInt
			e.schStt = 40
			return
		}
	case 41:
		*rlt = rsFlt
	case 42:
		*rlt = rsFlt
		if curChr == 'E' || curChr == 'e' {
			*rlt = rsStr
			e.schStt = 30
			return
		}
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsFlt
			e.schStt = 42
			return
		}
	case 43:
		*rlt = rsFlt
		if curChr >= '0' && curChr <= '9' {
			*rlt = rsFlt
			e.schStt = 43
			return
		}
	default:
		*rlt = rsStr
		e.schStt = schSttFrz
		return
	}
	// Fallthrough: if char is not whitespace/NUL/newline, freeze as string.
	if curChr != '\r' && curChr != '\n' && curChr != ' ' && curChr != 0 {
		*rlt = rsStr
	}
	e.schStt = schSttFrz
}

// sglPlnSym picks the right single-line plain scalar token based on schema result + context.
func (e *yamlEnv) sglPlnSymBlk(isR, isBR bool) int {
	switch e.rltSch {
	case rsNull:
		if isR {
			return yTokRSglPlnNulBlk
		}
		if isBR {
			return yTokBRSglPlnNulBlk
		}
		return yTokBSglPlnNulBlk
	case rsBool:
		if isR {
			return yTokRSglPlnBolBlk
		}
		if isBR {
			return yTokBRSglPlnBolBlk
		}
		return yTokBSglPlnBolBlk
	case rsInt:
		if isR {
			return yTokRSglPlnIntBlk
		}
		if isBR {
			return yTokBRSglPlnIntBlk
		}
		return yTokBSglPlnIntBlk
	case rsFlt:
		if isR {
			return yTokRSglPlnFltBlk
		}
		if isBR {
			return yTokBRSglPlnFltBlk
		}
		return yTokBSglPlnFltBlk
	default:
		if isR {
			return yTokRSglPlnStrBlk
		}
		if isBR {
			return yTokBRSglPlnStrBlk
		}
		return yTokBSglPlnStrBlk
	}
}

func (e *yamlEnv) sglPlnSymFlw(isR bool) int {
	switch e.rltSch {
	case rsNull:
		if isR {
			return yTokRSglPlnNulFlw
		}
		return yTokBRSglPlnNulFlw
	case rsBool:
		if isR {
			return yTokRSglPlnBolFlw
		}
		return yTokBRSglPlnBolFlw
	case rsInt:
		if isR {
			return yTokRSglPlnIntFlw
		}
		return yTokBRSglPlnIntFlw
	case rsFlt:
		if isR {
			return yTokRSglPlnFltFlw
		}
		return yTokBRSglPlnFltFlw
	default:
		if isR {
			return yTokRSglPlnStrFlw
		}
		return yTokBRSglPlnStrFlw
	}
}

// ---------------------------------------------------------------------------
// Section 6: Sub-scanners
// ---------------------------------------------------------------------------

func (e *yamlEnv) scnUriEsc() int8 {
	if e.lka() != '%' {
		return scnStop
	}
	e.mrkEnd()
	e.adv()
	if !yIsNsHexDigit(e.lka()) {
		return scnFail
	}
	e.adv()
	if !yIsNsHexDigit(e.lka()) {
		return scnFail
	}
	e.adv()
	return scnSucc
}

func (e *yamlEnv) scnNsUriChar() int8 {
	if yIsNsUriChar(e.lka()) {
		e.adv()
		return scnSucc
	}
	return e.scnUriEsc()
}

func (e *yamlEnv) scnNsTagChar() int8 {
	if yIsNsTagChar(e.lka()) {
		e.adv()
		return scnSucc
	}
	return e.scnUriEsc()
}

func (e *yamlEnv) scnDirBgn() bool {
	e.adv()
	if e.lka() == 'Y' {
		e.adv()
		if e.lka() == 'A' {
			e.adv()
			if e.lka() == 'M' {
				e.adv()
				if e.lka() == 'L' {
					e.adv()
					if yIsWht(e.lka()) {
						e.mrkEnd()
						return e.retSym(yTokSDirYmlBgn)
					}
				}
			}
		}
	} else if e.lka() == 'T' {
		e.adv()
		if e.lka() == 'A' {
			e.adv()
			if e.lka() == 'G' {
				e.adv()
				if yIsWht(e.lka()) {
					e.mrkEnd()
					return e.retSym(yTokSDirTagBgn)
				}
			}
		}
	}
	for yIsNsChar(e.lka()) {
		e.adv()
	}
	if e.curCol > 1 && yIsWht(e.lka()) {
		e.mrkEnd()
		return e.retSym(yTokSDirRsvBgn)
	}
	return false
}

func (e *yamlEnv) scnDirYmlVer(resultSymbol int) bool {
	var n1, n2 uint16
	for yIsNsDecDigit(e.lka()) {
		e.adv()
		n1++
	}
	if e.lka() != '.' {
		return false
	}
	e.adv()
	for yIsNsDecDigit(e.lka()) {
		e.adv()
		n2++
	}
	if n1 == 0 || n2 == 0 {
		return false
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnTagHdlTal() bool {
	if e.lka() == '!' {
		e.adv()
		return true
	}
	var n uint16
	for yIsNsWordChar(e.lka()) {
		e.adv()
		n++
	}
	if n == 0 {
		return true
	}
	if e.lka() == '!' {
		e.adv()
		return true
	}
	return false
}

func (e *yamlEnv) scnDirTagHdl(resultSymbol int) bool {
	if e.lka() == '!' {
		e.adv()
		if e.scnTagHdlTal() {
			e.mrkEnd()
			return e.retSym(resultSymbol)
		}
	}
	return false
}

func (e *yamlEnv) scnDirTagPfx(resultSymbol int) bool {
	if e.lka() == '!' {
		e.adv()
	} else if e.scnNsTagChar() == scnSucc {
		// consumed
	} else {
		return false
	}
	for {
		switch e.scnNsUriChar() {
		case scnStop:
			e.mrkEnd()
			return e.retSym(resultSymbol)
		case scnFail:
			return e.retSym(resultSymbol)
		default:
			continue
		}
	}
}

func (e *yamlEnv) scnDirRsvPrm(resultSymbol int) bool {
	if !yIsNsChar(e.lka()) {
		return false
	}
	e.adv()
	for yIsNsChar(e.lka()) {
		e.adv()
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnTag(resultSymbol int) bool {
	if e.lka() != '!' {
		return false
	}
	e.adv()
	if yIsWht(e.lka()) {
		e.mrkEnd()
		return e.retSym(resultSymbol)
	}
	if e.lka() == '<' {
		e.adv()
		if e.scnNsUriChar() != scnSucc {
			return false
		}
		for {
			switch e.scnNsUriChar() {
			case scnStop:
				if e.lka() == '>' {
					e.adv()
					e.mrkEnd()
					return e.retSym(resultSymbol)
				}
				return false
			case scnFail:
				return false
			default:
				continue
			}
		}
	} else {
		if e.scnTagHdlTal() && e.scnNsTagChar() != scnSucc {
			return false
		}
		for {
			switch e.scnNsTagChar() {
			case scnStop:
				e.mrkEnd()
				return e.retSym(resultSymbol)
			case scnFail:
				return e.retSym(resultSymbol)
			default:
				continue
			}
		}
	}
}

func (e *yamlEnv) scnAcrBgn(resultSymbol int) bool {
	if e.lka() != '&' {
		return false
	}
	e.adv()
	if !yIsNsAnchorChar(e.lka()) {
		return false
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnAcrCtn(resultSymbol int) bool {
	for yIsNsAnchorChar(e.lka()) {
		e.adv()
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnAlsBgn(resultSymbol int) bool {
	if e.lka() != '*' {
		return false
	}
	e.adv()
	if !yIsNsAnchorChar(e.lka()) {
		return false
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnAlsCtn(resultSymbol int) bool {
	for yIsNsAnchorChar(e.lka()) {
		e.adv()
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnDqtEscSeq(resultSymbol int) bool {
	lka := e.lka()
	switch lka {
	case '0', 'a', 'b', 't', '\t', 'n', 'v', 'r', 'e', 'f', ' ', '"', '/', '\\', 'N', '_', 'L', 'P':
		e.adv()
	case 'U':
		e.adv()
		for i := 0; i < 8; i++ {
			if yIsNsHexDigit(e.lka()) {
				e.adv()
			} else {
				return false
			}
		}
	case 'u':
		e.adv()
		for i := 0; i < 4; i++ {
			if yIsNsHexDigit(e.lka()) {
				e.adv()
			} else {
				return false
			}
		}
	case 'x':
		e.adv()
		for i := 0; i < 2; i++ {
			if yIsNsHexDigit(e.lka()) {
				e.adv()
			} else {
				return false
			}
		}
	default:
		return false
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnDrsDocEnd() bool {
	lka := e.lka()
	if lka != '-' && lka != '.' {
		return false
	}
	delimiter := lka
	e.adv()
	if e.lka() == delimiter {
		e.adv()
		if e.lka() == delimiter {
			e.adv()
			if yIsWht(e.lka()) {
				return true
			}
		}
	}
	e.mrkEnd()
	return false
}

func (e *yamlEnv) scnDqtStrCnt(resultSymbol int) bool {
	if !yIsNbDoubleChar(e.lka()) {
		return false
	}
	if e.curCol == 0 && e.scnDrsDocEnd() {
		e.mrkEnd()
		if e.curChr == '-' {
			return e.retSym(yTokSDrsEnd)
		}
		return e.retSym(yTokSDocEnd)
	}
	e.adv()
	for yIsNbDoubleChar(e.lka()) {
		e.adv()
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnSqtStrCnt(resultSymbol int) bool {
	if !yIsNbSingleChar(e.lka()) {
		return false
	}
	if e.curCol == 0 && e.scnDrsDocEnd() {
		e.mrkEnd()
		if e.curChr == '-' {
			return e.retSym(yTokSDrsEnd)
		}
		return e.retSym(yTokSDocEnd)
	}
	e.adv()
	for yIsNbSingleChar(e.lka()) {
		e.adv()
	}
	e.mrkEnd()
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnBlkStrBgn(resultSymbol int) bool {
	lka := e.lka()
	if lka != '|' && lka != '>' {
		return false
	}
	e.adv()
	curInd := e.curInd()
	ind := int16(-1)
	if e.lka() >= '1' && e.lka() <= '9' {
		ind = int16(e.lka() - '1')
		e.adv()
		if e.lka() == '+' || e.lka() == '-' {
			e.adv()
		}
	} else if e.lka() == '+' || e.lka() == '-' {
		e.adv()
		if e.lka() >= '1' && e.lka() <= '9' {
			ind = int16(e.lka() - '1')
			e.adv()
		}
	}
	if !yIsWht(e.lka()) {
		return false
	}
	e.mrkEnd()
	if ind != -1 {
		ind += curInd
	} else {
		ind = curInd
		for yIsWsp(e.lka()) {
			e.adv()
		}
		if e.lka() == '#' {
			e.adv()
			for !yIsNwl(e.lka()) && e.lka() != 0 {
				e.adv()
			}
		}
		if yIsNwl(e.lka()) {
			e.advNwl()
		}
		for e.lka() != 0 {
			if e.lka() == ' ' {
				e.adv()
			} else if yIsNwl(e.lka()) {
				if e.curCol-1 < ind {
					break
				}
				ind = e.curCol - 1
				e.advNwl()
			} else {
				if e.curCol-1 > ind {
					ind = e.curCol - 1
				}
				break
			}
		}
	}
	e.pushInd(yIndStr, ind)
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnBlkStrCnt(resultSymbol int) bool {
	if !yIsNsChar(e.lka()) {
		return false
	}
	if e.curCol == 0 && e.scnDrsDocEnd() {
		if !e.popInd() {
			return false
		}
		return e.retSym(yTokBL)
	}
	e.adv()
	e.mrkEnd()
	for {
		if yIsNsChar(e.lka()) {
			e.adv()
			for yIsNsChar(e.lka()) {
				e.adv()
			}
			e.mrkEnd()
		}
		if yIsWsp(e.lka()) {
			e.adv()
			for yIsWsp(e.lka()) {
				e.adv()
			}
		} else {
			break
		}
	}
	return e.retSym(resultSymbol)
}

func (e *yamlEnv) scnPlnCnt(isPlainSafe func(int32) bool) int8 {
	isCurSaf := isPlainSafe(e.curChr)
	isLkaWsp := yIsWsp(e.lka())
	isLkaSaf := isPlainSafe(e.lka())
	if isLkaSaf || isLkaWsp {
		for {
			if isLkaSaf && e.lka() != '#' && e.lka() != ':' {
				e.adv()
				e.mrkEnd()
				e.advSchStt(e.curChr)
			} else if isCurSaf && e.lka() == '#' {
				e.adv()
				e.mrkEnd()
				e.advSchStt(e.curChr)
			} else if isLkaWsp {
				e.adv()
				e.advSchStt(e.curChr)
			} else if e.lka() == ':' {
				e.adv() // check later
			} else {
				break
			}

			isCurSaf = isLkaSaf
			isLkaWsp = yIsWsp(e.lka())
			isLkaSaf = isPlainSafe(e.lka())

			if e.curChr == ':' {
				if isLkaSaf {
					e.mrkEnd()
					e.advSchStt(e.curChr)
				} else {
					return scnFail
				}
			}
		}
	} else {
		return scnStop
	}
	return scnSucc
}

// ---------------------------------------------------------------------------
// Section 7: Main scan
// ---------------------------------------------------------------------------

func (e *yamlEnv) scan() bool {
	e.init()
	e.mrkEnd()

	vs := e.validSymbols

	allowComment := !(vs[yTokRDqtStrCtn] || vs[yTokBRDqtStrCtn] ||
		vs[yTokRSqtStrCtn] || vs[yTokBRSqtStrCtn])

	curInd := e.curInd()
	prtInd := e.prtInd()
	curIndTyp := e.curIndTyp()

	hasTabInd := false
	leadingSpaces := int16(0)

	// Skip whitespace/newlines/comments
	for {
		lka := e.lka()
		if lka == ' ' {
			if !hasTabInd {
				leadingSpaces++
			}
			e.skp()
		} else if lka == '\t' {
			hasTabInd = true
			e.skp()
		} else if yIsNwl(lka) {
			hasTabInd = false
			leadingSpaces = 0
			e.skpNwl()
		} else if allowComment && lka == '#' {
			if vs[yTokBRBlkStrCtn] && vs[yTokBL] && e.curCol <= curInd {
				if !e.popInd() {
					return false
				}
				return e.retSym(yTokBL)
			}
			var shouldEmitComment bool
			if vs[yTokBRBlkStrCtn] {
				shouldEmitComment = e.curRow == e.st.row
			} else {
				shouldEmitComment = e.curCol == 0 || e.curRow != e.st.row || e.curCol > e.st.col
			}
			if shouldEmitComment {
				e.adv()
				for !yIsNwl(e.lka()) && e.lka() != 0 {
					e.adv()
				}
				e.mrkEnd()
				return e.retSym(yTokComment)
			}
			break
		} else {
			break
		}
	}

	// EOF
	if e.lka() == 0 {
		if vs[yTokBL] {
			e.mrkEnd()
			if !e.popInd() {
				return false
			}
			return e.retSym(yTokBL)
		}
		if vs[yTokEndOfFile] {
			e.mrkEnd()
			return e.retSym(yTokEndOfFile)
		}
		return false
	}

	bgnRow := e.curRow
	bgnCol := e.curCol
	bgnChr := e.lka()

	// BL emission (dedent detection)
	if vs[yTokBL] && bgnCol <= curInd && !hasTabInd {
		var shouldPop bool
		if curInd == prtInd && curIndTyp == yIndSeq {
			shouldPop = bgnCol < curInd || e.lka() != '-'
		} else {
			shouldPop = bgnCol <= prtInd || curIndTyp == yIndStr
		}
		if shouldPop {
			if !e.popInd() {
				return false
			}
			return e.retSym(yTokBL)
		}
	}

	// Context flags
	hasNwl := e.curRow > e.st.row
	isR := !hasNwl
	isBR := hasNwl && leadingSpaces > curInd
	isB := hasNwl && leadingSpaces == curInd && !hasTabInd
	isS := bgnCol == 0

	// Directive version/tag continuations
	if vs[yTokRDirYmlVer] && isR {
		return e.scnDirYmlVer(yTokRDirYmlVer)
	}
	if vs[yTokRDirTagHdl] && isR {
		return e.scnDirTagHdl(yTokRDirTagHdl)
	}
	if vs[yTokRDirTagPfx] && isR {
		return e.scnDirTagPfx(yTokRDirTagPfx)
	}
	if vs[yTokRDirRsvPrm] && isR {
		return e.scnDirRsvPrm(yTokRDirRsvPrm)
	}

	// Block string continuation
	if vs[yTokBRBlkStrCtn] && isBR && e.scnBlkStrCnt(yTokBRBlkStrCtn) {
		return true
	}

	// Quoted string content
	if (vs[yTokRDqtStrCtn] && isR && e.scnDqtStrCnt(yTokRDqtStrCtn)) ||
		(vs[yTokBRDqtStrCtn] && (isBR || hasNwl) && e.scnDqtStrCnt(yTokBRDqtStrCtn)) {
		return true
	}
	if (vs[yTokRSqtStrCtn] && isR && e.scnSqtStrCnt(yTokRSqtStrCtn)) ||
		(vs[yTokBRSqtStrCtn] && isBR && e.scnSqtStrCnt(yTokBRSqtStrCtn)) {
		return true
	}

	// Anchor/alias continuations
	if vs[yTokRAcrCtn] && isR {
		return e.scnAcrCtn(yTokRAcrCtn)
	}
	if vs[yTokRAlsCtn] && isR {
		return e.scnAlsCtn(yTokRAlsCtn)
	}

	// Character dispatch
	lka := e.lka()
	switch {
	case lka == '%':
		if vs[yTokSDirYmlBgn] && isS {
			return e.scnDirBgn()
		}

	case lka == '*':
		if vs[yTokRAlsBgn] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnAlsBgn(yTokRAlsBgn)
		}
		if vs[yTokBRAlsBgn] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnAlsBgn(yTokBRAlsBgn)
		}
		if vs[yTokBAlsBgn] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnAlsBgn(yTokBAlsBgn)
		}

	case lka == '&':
		if vs[yTokRAcrBgn] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnAcrBgn(yTokRAcrBgn)
		}
		if vs[yTokBRAcrBgn] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnAcrBgn(yTokBRAcrBgn)
		}
		if vs[yTokBAcrBgn] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnAcrBgn(yTokBAcrBgn)
		}

	case lka == '!':
		if vs[yTokRTag] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnTag(yTokRTag)
		}
		if vs[yTokBRTag] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnTag(yTokBRTag)
		}
		if vs[yTokBTag] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			return e.scnTag(yTokBTag)
		}

	case lka == '[':
		if vs[yTokRFlwSeqBgn] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRFlwSeqBgn)
		}
		if vs[yTokBRFlwSeqBgn] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwSeqBgn)
		}
		if vs[yTokBFlwSeqBgn] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBFlwSeqBgn)
		}

	case lka == ']':
		if vs[yTokRFlwSeqEnd] && isR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRFlwSeqEnd)
		}
		if vs[yTokBRFlwSeqEnd] && isBR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwSeqEnd)
		}
		if vs[yTokBFlwSeqEnd] && isB {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwSeqEnd) // matches C: B_FLW_SEQ_END maps to BR_FLW_SEQ_END
		}

	case lka == '{':
		if vs[yTokRFlwMapBgn] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRFlwMapBgn)
		}
		if vs[yTokBRFlwMapBgn] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwMapBgn)
		}
		if vs[yTokBFlwMapBgn] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBFlwMapBgn)
		}

	case lka == '}':
		if vs[yTokRFlwMapEnd] && isR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRFlwMapEnd)
		}
		if vs[yTokBRFlwMapEnd] && isBR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwMapEnd)
		}
		if vs[yTokBFlwMapEnd] && isB {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwMapEnd) // matches C
		}

	case lka == ',':
		if vs[yTokRFlwSepBgn] && isR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRFlwSepBgn)
		}
		if vs[yTokBRFlwSepBgn] && isBR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwSepBgn)
		}

	case lka == '"':
		if vs[yTokRDqtStrBgn] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRDqtStrBgn)
		}
		if vs[yTokBRDqtStrBgn] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRDqtStrBgn)
		}
		if vs[yTokBDqtStrBgn] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBDqtStrBgn)
		}
		if vs[yTokRDqtStrEnd] && isR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRDqtStrEnd)
		}
		if vs[yTokBRDqtStrEnd] && isBR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRDqtStrEnd)
		}

	case lka == '\'':
		if vs[yTokRSqtStrBgn] && isR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRSqtStrBgn)
		}
		if vs[yTokBRSqtStrBgn] && isBR {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRSqtStrBgn)
		}
		if vs[yTokBSqtStrBgn] && isB {
			e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBSqtStrBgn)
		}
		if vs[yTokRSqtStrEnd] && isR {
			e.adv()
			if e.lka() == '\'' {
				e.adv()
				e.mrkEnd()
				return e.retSym(yTokRSqtEscSqt)
			}
			e.mrkEnd()
			return e.retSym(yTokRSqtStrEnd)
		}
		if vs[yTokBRSqtStrEnd] && isBR {
			e.adv()
			if e.lka() == '\'' {
				e.adv()
				e.mrkEnd()
				return e.retSym(yTokBRSqtEscSqt)
			}
			e.mrkEnd()
			return e.retSym(yTokBRSqtStrEnd)
		}

	case lka == '?':
		isRBlkKeyBgn := vs[yTokRBlkKeyBgn] && isR
		isBRBlkKeyBgn := vs[yTokBRBlkKeyBgn] && isBR
		isBBlkKeyBgn := vs[yTokBBlkKeyBgn] && isB
		isRFlwKeyBgn := vs[yTokRFlwKeyBgn] && isR
		isBRFlwKeyBgn := vs[yTokBRFlwKeyBgn] && isBR
		if isRBlkKeyBgn || isBRBlkKeyBgn || isBBlkKeyBgn || isRFlwKeyBgn || isBRFlwKeyBgn {
			e.adv()
			if yIsWht(e.lka()) {
				e.mrkEnd()
				if isRBlkKeyBgn {
					if hasTabInd {
						return false
					}
					e.pushInd(yIndMap, bgnCol)
					return e.retSym(yTokRBlkKeyBgn)
				}
				if isBRBlkKeyBgn {
					if hasTabInd {
						return false
					}
					e.pushInd(yIndMap, bgnCol)
					return e.retSym(yTokBRBlkKeyBgn)
				}
				if isBBlkKeyBgn {
					return e.retSym(yTokBBlkKeyBgn)
				}
				if isRFlwKeyBgn {
					return e.retSym(yTokRFlwKeyBgn)
				}
				if isBRFlwKeyBgn {
					return e.retSym(yTokBRFlwKeyBgn)
				}
			}
		}

	case lka == ':':
		if vs[yTokRFlwJsvBgn] && isR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokRFlwJsvBgn)
		}
		if vs[yTokBRFlwJsvBgn] && isBR {
			e.adv()
			e.mrkEnd()
			return e.retSym(yTokBRFlwJsvBgn)
		}
		isRBlkValBgn := vs[yTokRBlkValBgn] && isR
		isBRBlkValBgn := vs[yTokBRBlkValBgn] && isBR
		isBBlkValBgn := vs[yTokBBlkValBgn] && isB
		isRBlkImpBgn := vs[yTokRBlkImpBgn] && isR
		isRFlwNjvBgn := vs[yTokRFlwNjvBgn] && isR
		isBRFlwNjvBgn := vs[yTokBRFlwNjvBgn] && isBR
		if isRBlkValBgn || isBRBlkValBgn || isBBlkValBgn || isRBlkImpBgn || isRFlwNjvBgn || isBRFlwNjvBgn {
			e.adv()
			isLkaWht := yIsWht(e.lka())
			if isLkaWht {
				if isRBlkValBgn {
					if hasTabInd {
						return false
					}
					e.pushInd(yIndMap, bgnCol)
					e.mrkEnd()
					return e.retSym(yTokRBlkValBgn)
				}
				if isBRBlkValBgn {
					if hasTabInd {
						return false
					}
					e.pushInd(yIndMap, bgnCol)
					e.mrkEnd()
					return e.retSym(yTokBRBlkValBgn)
				}
				if isBBlkValBgn {
					e.mrkEnd()
					return e.retSym(yTokBBlkValBgn)
				}
				if isRBlkImpBgn {
					// MAY_PUSH_IMP_IND
					if curInd != e.st.blkImpCol {
						if e.st.blkImpTab != 0 {
							return false
						}
						e.pushInd(yIndMap, e.st.blkImpCol)
					}
					e.mrkEnd()
					return e.retSym(yTokRBlkImpBgn)
				}
			}
			if isLkaWht || e.lka() == ',' || e.lka() == ']' || e.lka() == '}' {
				if isRFlwNjvBgn {
					e.mrkEnd()
					return e.retSym(yTokRFlwNjvBgn)
				}
				if isBRFlwNjvBgn {
					e.mrkEnd()
					return e.retSym(yTokBRFlwNjvBgn)
				}
			}
		}

	case lka == '-':
		isRBlkSeqBgn := vs[yTokRBlkSeqBgn] && isR
		isBRBlkSeqBgn := vs[yTokBRBlkSeqBgn] && isBR
		isBBlkSeqBgn := vs[yTokBBlkSeqBgn] && isB
		isSDrsEnd := isS
		if isRBlkSeqBgn || isBRBlkSeqBgn || isBBlkSeqBgn || isSDrsEnd {
			e.adv()
			if yIsWht(e.lka()) {
				if isRBlkSeqBgn {
					if hasTabInd {
						return false
					}
					e.pushInd(yIndSeq, bgnCol)
					e.mrkEnd()
					return e.retSym(yTokRBlkSeqBgn)
				}
				if isBRBlkSeqBgn {
					if hasTabInd {
						return false
					}
					e.pushInd(yIndSeq, bgnCol)
					e.mrkEnd()
					return e.retSym(yTokBRBlkSeqBgn)
				}
				if isBBlkSeqBgn {
					// MAY_PUSH_SPC_SEQ_IND
					if curIndTyp == yIndMap {
						e.pushInd(yIndSeq, bgnCol)
					}
					e.mrkEnd()
					return e.retSym(yTokBBlkSeqBgn)
				}
			} else if e.lka() == '-' && isSDrsEnd {
				e.adv()
				if e.lka() == '-' {
					e.adv()
					if yIsWht(e.lka()) {
						if vs[yTokBL] {
							if !e.popInd() {
								return false
							}
							return e.retSym(yTokBL)
						}
						e.mrkEnd()
						return e.retSym(yTokSDrsEnd)
					}
				}
			}
		}

	case lka == '.':
		if isS {
			e.adv()
			if e.lka() == '.' {
				e.adv()
				if e.lka() == '.' {
					e.adv()
					if yIsWht(e.lka()) {
						if vs[yTokBL] {
							if !e.popInd() {
								return false
							}
							return e.retSym(yTokBL)
						}
						e.mrkEnd()
						return e.retSym(yTokSDocEnd)
					}
				}
			}
		}

	case lka == '\\':
		isRDqtEscNwl := vs[yTokRDqtEscNwl] && isR
		isBRDqtEscNwl := vs[yTokBRDqtEscNwl] && isBR
		isRDqtEscSeq := vs[yTokRDqtEscSeq] && isR
		isBRDqtEscSeq := vs[yTokBRDqtEscSeq] && isBR
		if isRDqtEscNwl || isBRDqtEscNwl || isRDqtEscSeq || isBRDqtEscSeq {
			e.adv()
			if yIsNwl(e.lka()) {
				if isRDqtEscNwl {
					e.mrkEnd()
					return e.retSym(yTokRDqtEscNwl)
				}
				if isBRDqtEscNwl {
					e.mrkEnd()
					return e.retSym(yTokBRDqtEscNwl)
				}
			}
			if isRDqtEscSeq {
				return e.scnDqtEscSeq(yTokRDqtEscSeq)
			}
			if isBRDqtEscSeq {
				return e.scnDqtEscSeq(yTokBRDqtEscSeq)
			}
			return false
		}

	case lka == '|':
		if vs[yTokRBlkLitBgn] && isR {
			return e.scnBlkStrBgn(yTokRBlkLitBgn)
		}
		if vs[yTokBRBlkLitBgn] && isBR {
			return e.scnBlkStrBgn(yTokBRBlkLitBgn)
		}

	case lka == '>':
		if vs[yTokRBlkFldBgn] && isR {
			return e.scnBlkStrBgn(yTokRBlkFldBgn)
		}
		if vs[yTokBRBlkFldBgn] && isBR {
			return e.scnBlkStrBgn(yTokBRBlkFldBgn)
		}
	}

	// Plain scalar scanning
	maybeSglPlnBlk := (vs[yTokRSglPlnStrBlk] && isR) || (vs[yTokBRSglPlnStrBlk] && isBR) || (vs[yTokBSglPlnStrBlk] && isB)
	maybeSglPlnFlw := (vs[yTokRSglPlnStrFlw] && isR) || (vs[yTokBRSglPlnStrFlw] && isBR)
	maybeMtlPlnBlk := (vs[yTokRMtlPlnStrBlk] && isR) || (vs[yTokBRMtlPlnStrBlk] && isBR)
	maybeMtlPlnFlw := (vs[yTokRMtlPlnStrFlw] && isR) || (vs[yTokBRMtlPlnStrFlw] && isBR)

	if maybeSglPlnBlk || maybeSglPlnFlw || maybeMtlPlnBlk || maybeMtlPlnFlw {
		isInBlk := maybeSglPlnBlk || maybeMtlPlnBlk
		var isPlainSafe func(int32) bool
		if isInBlk {
			isPlainSafe = yIsPlainSafeInBlock
		} else {
			isPlainSafe = yIsPlainSafeInFlow
		}

		if e.curCol-bgnCol == 0 {
			e.adv()
		}
		if e.curCol-bgnCol == 1 {
			isPlainFirst := (yIsNsChar(bgnChr) && !yIsCIndicator(bgnChr)) ||
				((bgnChr == '-' || bgnChr == '?' || bgnChr == ':') && isPlainSafe(e.lka()))
			if !isPlainFirst {
				return false
			}
			e.advSchStt(e.curChr)
		} else {
			e.schStt = schSttFrz // must be RS_STR
		}

		e.mrkEnd()

		for {
			if !yIsNwl(e.lka()) {
				if e.scnPlnCnt(isPlainSafe) != scnSucc {
					break
				}
			}
			if e.lka() == 0 || !yIsNwl(e.lka()) {
				break
			}
			for {
				if yIsNwl(e.lka()) {
					e.advNwl()
				} else if yIsWsp(e.lka()) {
					e.adv()
				} else {
					break
				}
			}
			if e.lka() == 0 || e.curCol <= curInd {
				break
			}
			if e.curCol == 0 && e.scnDrsDocEnd() {
				break
			}
		}

		if e.endRow == bgnRow {
			if maybeSglPlnBlk {
				e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
				return e.retSym(e.sglPlnSymBlk(isR, isBR))
			}
			if maybeSglPlnFlw {
				return e.retSym(e.sglPlnSymFlw(isR))
			}
		} else {
			if maybeMtlPlnBlk {
				e.mayUpdImpCol(bgnRow, bgnCol, hasTabInd)
				if isR {
					return e.retSym(yTokRMtlPlnStrBlk)
				}
				return e.retSym(yTokBRMtlPlnStrBlk)
			}
			if maybeMtlPlnFlw {
				if isR {
					return e.retSym(yTokRMtlPlnStrFlw)
				}
				return e.retSym(yTokBRMtlPlnStrFlw)
			}
		}

		return false
	}

	return !vs[yTokErrRec]
}

// mayUpdImpCol is the MAY_UPD_IMP_COL macro.
func (e *yamlEnv) mayUpdImpCol(bgnRow, bgnCol int16, hasTabInd bool) {
	if e.st.blkImpRow != bgnRow {
		e.st.blkImpRow = bgnRow
		e.st.blkImpCol = bgnCol
		if hasTabInd {
			e.st.blkImpTab = 1
		} else {
			e.st.blkImpTab = 0
		}
	}
}
