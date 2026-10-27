package scannercert

import (
	gts "github.com/odvcencio/gotreesitter"
)

// counterScanner is a small stateful scanner with switches that break one part
// of the round-trip contract at a time. With no switch set it is a correct
// scanner: its whole state is one counter and Serialize writes it.
//
// External token 0 accepts "a" and reports a symbol that depends on the
// counter, so a wrong restored counter changes the visible result. External
// token 1 accepts "b" and leaves the counter alone. Any other input is
// rejected.
type counterScanner struct {
	omitState     bool  // Serialize writes nothing, so a restored counter is always 0
	dirtyOnReject bool  // a rejected scan advances the counter
	flipOnSave    bool  // Serialize changes a hidden bit, so two saves differ
	clampAbove    uint8 // Deserialize clamps a larger counter down to this value
	panicOn       uint8 // Deserialize panics when it reads this counter
	oversize      bool  // Serialize reports more bytes than the buffer holds
}

type counterState struct {
	n    uint8
	flip uint8
}

const (
	counterMarkIndex  = 0
	counterOtherIndex = 1
)

func (counterScanner) Create() any { return &counterState{} }
func (counterScanner) Destroy(any) {}
func (s counterScanner) Serialize(payload any, buf []byte) int {
	st := payload.(*counterState)
	switch {
	case s.oversize:
		return len(buf) + 1
	case s.omitState:
		return 0
	case s.flipOnSave:
		st.flip ^= 0x80
		buf[0] = st.n | st.flip
		return 1
	}
	buf[0] = st.n
	return 1
}

func (s counterScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*counterState)
	if len(buf) == 0 {
		*st = counterState{}
		return
	}
	st.n = buf[0] &^ 0x80
	st.flip = buf[0] & 0x80
	if s.clampAbove != 0 && st.n > s.clampAbove {
		st.n = s.clampAbove
	}
	if s.panicOn != 0 && st.n == s.panicOn {
		panic("counterScanner: cannot restore this state")
	}
}

func (s counterScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*counterState)
	switch {
	case valid[counterMarkIndex] && lexer.Lookahead() == 'a':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(gts.Symbol(1 + st.n%3))
		st.n++
		return true
	case valid[counterOtherIndex] && lexer.Lookahead() == 'b':
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(5)
		return true
	}
	if s.dirtyOnReject {
		st.n++
	}
	return false
}

// preservingCounter declares that a rejected scan leaves its state alone, so
// the token source restores nothing after one.
type preservingCounter struct{ counterScanner }

func (preservingCounter) PreservesStateOnScanFailure() bool { return true }

// retainingCounter declares that a rejected scan may change its state on
// purpose, and that the change stays live.
type retainingCounter struct{ counterScanner }

func (retainingCounter) RetainsStateOnScanFailure() bool { return true }

// bothCounter declares both. The token source lets retention win.
type bothCounter struct{ counterScanner }

func (bothCounter) PreservesStateOnScanFailure() bool { return true }
func (bothCounter) RetainsStateOnScanFailure() bool   { return true }

// echoScanner carries no state: it accepts "a" under token 0.
type echoScanner struct{}

func (echoScanner) Create() any                       { return nil }
func (echoScanner) Destroy(any)                       {}
func (echoScanner) Serialize(any, []byte) int         { return 0 }
func (echoScanner) Deserialize(any, []byte)           {}
func (echoScanner) PreservesStateOnScanFailure() bool { return true }
func (echoScanner) ExternalScannerIsStateless() bool  { return true }
func (echoScanner) Scan(_ any, lexer *gts.ExternalLexer, valid []bool) bool {
	if valid[0] && lexer.Lookahead() == 'a' {
		lexer.Advance(false)
		lexer.MarkEnd()
		lexer.SetResultSymbol(9)
		return true
	}
	return false
}

// widthScanner emits one symbol whose token grows from one byte to two once
// its count reaches one. With omitState set, a restored scanner starts the
// count over, so its tokens come out narrower than the live ones.
type widthScanner struct{ omitState bool }

func (widthScanner) Create() any { return &counterState{} }
func (widthScanner) Destroy(any) {}
func (s widthScanner) Serialize(payload any, buf []byte) int {
	if s.omitState {
		return 0
	}
	buf[0] = payload.(*counterState).n
	return 1
}

func (widthScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*counterState)
	st.n = 0
	if len(buf) > 0 {
		st.n = buf[0]
	}
}

func (widthScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*counterState)
	if !valid[0] || lexer.Lookahead() != 'a' {
		return false
	}
	lexer.Advance(false)
	if st.n >= 1 {
		lexer.Advance(false)
	}
	lexer.MarkEnd()
	lexer.SetResultSymbol(7)
	st.n++
	return true
}

// forgetfulScanner accepts "a" under token 0 and always leaves its counter at
// zero, so its outcome does not depend on the state it began with. Calls that
// differ only in that state, in the mask or in the source have one outcome.
type forgetfulScanner struct{}

func (forgetfulScanner) Create() any { return &counterState{} }
func (forgetfulScanner) Destroy(any) {}
func (forgetfulScanner) Serialize(payload any, buf []byte) int {
	buf[0] = payload.(*counterState).n
	return 1
}

func (forgetfulScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*counterState)
	st.n = 0
	if len(buf) > 0 {
		st.n = buf[0]
	}
}

func (forgetfulScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*counterState)
	if !valid[0] || lexer.Lookahead() != 'a' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(1)
	st.n = 0
	return true
}

// shiftScanner accepts "a" and ends its token two bytes in. After its first
// scan it skips the first byte, so the token starts one byte later and ends at
// the same place. With omitState set, a restored scanner is back at its first
// scan.
type shiftScanner struct{ omitState bool }

func (shiftScanner) Create() any { return &counterState{} }
func (shiftScanner) Destroy(any) {}
func (s shiftScanner) Serialize(payload any, buf []byte) int {
	if s.omitState {
		return 0
	}
	buf[0] = payload.(*counterState).n
	return 1
}

func (shiftScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*counterState)
	st.n = 0
	if len(buf) > 0 {
		st.n = buf[0]
	}
}

func (shiftScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*counterState)
	if !valid[0] || lexer.Lookahead() != 'a' {
		return false
	}
	lexer.Advance(st.n > 0)
	lexer.Advance(false)
	lexer.MarkEnd()
	lexer.SetResultSymbol(7)
	st.n++
	return true
}

// unnamedScanner accepts "a". After its first scan it names the token with
// symbol zero; before that it names nothing. A missing symbol and symbol zero
// read the same except for the flag that says a scanner set one. With
// omitState set, a restored scanner is back at its first scan.
type unnamedScanner struct{ omitState bool }

func (unnamedScanner) Create() any { return &counterState{} }
func (unnamedScanner) Destroy(any) {}
func (s unnamedScanner) Serialize(payload any, buf []byte) int {
	if s.omitState {
		return 0
	}
	buf[0] = payload.(*counterState).n
	return 1
}

func (unnamedScanner) Deserialize(payload any, buf []byte) {
	st := payload.(*counterState)
	st.n = 0
	if len(buf) > 0 {
		st.n = buf[0]
	}
}

func (unnamedScanner) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	st := payload.(*counterState)
	if !valid[0] || lexer.Lookahead() != 'a' {
		return false
	}
	lexer.Advance(false)
	lexer.MarkEnd()
	if st.n > 0 {
		lexer.SetResultSymbol(0)
	}
	st.n++
	return true
}

var (
	bothValid = []bool{true, true}
	noneValid = []bool{false, false}
)

// scanEvery scans each origin of src once per mask, resuming the same live
// payload throughout, the way a parser resumes one scanner across tokens.
func scanEvery(rec *recorder, src string, masks ...[]bool) {
	payload := rec.Create()
	defer rec.Destroy(payload)
	row, column := uint32(0), uint32(0)
	for origin := 0; origin < len(src); origin++ {
		for _, mask := range masks {
			rec.Scan(payload, newLexer([]byte(src), origin, row, column), mask)
		}
		if src[origin] == '\n' {
			row, column = row+1, 0
		} else {
			column++
		}
	}
}

// certified records src and returns the analysis of its replay.
func certified(inner gts.ExternalScanner, src string, masks ...[]bool) analysis {
	rec := newRecorder(inner)
	rec.begin(0)
	scanEvery(rec, src, masks...)
	return rec.analyze(inner, symbolNames{})
}

// capableScanner answers every optional scanner interface the recorder mirrors,
// each with a value the test sets.
type capableScanner struct {
	echoScanner
	reuse, checkpointed, stateless, preserves, retains, errorRetry bool
	packedUpTo                                                     int
}

func (s capableScanner) SupportsIncrementalReuse() bool       { return s.reuse }
func (s capableScanner) UsesExternalScannerCheckpoints() bool { return s.checkpointed }
func (s capableScanner) ExternalScannerIsStateless() bool     { return s.stateless }
func (s capableScanner) PreservesStateOnScanFailure() bool    { return s.preserves }
func (s capableScanner) RetainsStateOnScanFailure() bool      { return s.retains }
func (s capableScanner) SupportsErrorModeExternalRetry() bool { return s.errorRetry }
func (s capableScanner) SupportsPackedGSSVersionOrder(n int) bool {
	return n <= s.packedUpTo
}

// identityScanner is a checkpointed scanner that names its implementation.
type identityScanner struct{ capableScanner }

func (identityScanner) CheckpointIdentity() (gts.ExternalScannerCheckpointIdentity, bool) {
	return gts.ExternalScannerCheckpointIdentity{Scanner: []byte("scanner"), Grammar: []byte("grammar")}, true
}
