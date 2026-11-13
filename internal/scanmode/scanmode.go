// Package scanmode holds the list of scanner ports the core may serve, the
// interface of a port's C-faithful mode, and three questions about the scanner
// lexer that the public ExternalLexer cannot answer.
//
// Some scanner ports serve two engines: the default parser, whose documented
// checkpoint contract (docs/external-scanners.md) asks for reusable, safely
// restorable state and for valid result symbols, and the new core
// (internal/ccore), which must see what the pinned C scanner does, including the
// bytes of its saved state. Where the two differ, a port keeps its default
// behaviour and switches to the C behaviour when the core turns the mode on.
//
// The reference of the mode is one scanner: the pinned C scanner compiled with
// signed char for a little-endian target (the C oracle of CI is linux/amd64). A
// port follows it on every Go target, so a pure-Go parse gives the same tree on
// every platform. C itself gives other results on other targets where a scanner
// depends on the signedness of char or on native byte order; a port does not copy
// that. Where a port depends on either: hcl uses CHAR_MAX 127 and little-endian
// state fields; teal reads opening_quote as a signed char and keeps its state
// little-endian; racket builds the strcmp buffers of its here-string comparison
// little-endian. An unsigned-char target differs from the reference for an hcl
// heredoc identifier of 128 or more letters and for a teal quote byte at or above
// 0x80; a big-endian target differs for racket's here-string comparison and for
// the hcl and teal state layouts.
//
// Nothing is discovered. A port that has been checked against its C scanner
// registers its scanner type and the type of the payload its Create returns, with
// whether it has a behavioural mode. The core serves a grammar only when its
// scanner is the repository's order adapter around such a registered scanner;
// any other scanner value makes binding fail.
package scanmode

import (
	"reflect"
	"sync"
)

// CFaithful is implemented by the payload of a registered scanner that has a
// behavioural mode: the pinned C scanner's behaviour, including the bytes of its
// saved state.
type CFaithful interface{ SetCFaithful(on bool) }

// Registered describes a scanner type that has been checked against its C scanner.
type Registered struct {
	// Payload is the concrete type of the payload the scanner's Create returns
	// (nil when Create returns nil).
	Payload reflect.Type
	// NeedsMode is true when the port has a behavioural mode: the core turns it
	// on for the innermost payload, which must then implement CFaithful.
	NeedsMode bool
}

// Reasons binding fails with. The core's error text for a refused grammar is one of
// these strings.
const (
	ReasonScannerNotRegistered = "ccore: the scanner is not one the core serves: it is neither a registered scanner nor the order adapter around one"
	ReasonScannerNil           = "ccore: the scanner value is nil (a nil interface, a typed nil, or an order adapter without an inner scanner)"
	ReasonPayloadNotRegistered = "ccore: the scanner's payload is not the registered payload type behind order adapter payloads"
	ReasonModeNotDelivered     = "ccore: the registered scanner needs the C-faithful mode but its innermost payload did not take it"
	ReasonModeNotReset         = "ccore: the C-faithful mode could not be switched off in the payload"
)

var (
	mu       sync.RWMutex
	registry = map[reflect.Type]Registered{}
	frozen   bool
)

// RegisterScanner records scanner's concrete type as served by the core. payload
// is a value of the type Create returns: a typed nil pointer such as
// (*State)(nil), or nil when Create returns nil. A scanner package calls it from
// init, one line per checked port. It panics when the type is already registered
// and when binding has started (Freeze): a registration never changes after a
// binding has used it.
func RegisterScanner(scanner any, payload any, needsMode bool) {
	mu.Lock()
	defer mu.Unlock()
	t := reflect.TypeOf(scanner)
	if _, dup := registry[t]; dup {
		panic("scanmode: RegisterScanner(" + t.String() + ") twice")
	}
	if frozen {
		panic("scanmode: RegisterScanner(" + t.String() + ") after binding started")
	}
	registry[t] = Registered{Payload: reflect.TypeOf(payload), NeedsMode: needsMode}
}

// Freeze ends registration. The core calls it when it starts to bind a scanner.
func Freeze() {
	mu.Lock()
	frozen = true
	mu.Unlock()
}

// Lookup returns the registration of scanner's concrete type.
func Lookup(scanner any) (Registered, bool) {
	mu.RLock()
	defer mu.RUnlock()
	r, ok := registry[reflect.TypeOf(scanner)]
	return r, ok
}

// Flag is the payload of a port that has no scanner state but has a mode. A nil
// *Flag reports the mode as off.
type Flag struct{ on bool }

// SetCFaithful implements CFaithful.
func (f *Flag) SetCFaithful(on bool) {
	if f != nil {
		f.on = on
	}
}

// On reports whether the mode is on.
func (f *Flag) On() bool { return f != nil && f.on }

// Of reports whether payload is a *Flag with the mode on.
func Of(payload any) bool {
	f, _ := payload.(*Flag)
	return f.On()
}

// The probes are set by the root package, which owns the lexer type.
var (
	eofProbe        func(lexer any) bool
	lookaheadProbe  func(lexer any) int32
	rangeStartProbe func(lexer any) bool
)

// SetProbes is called once by the root package.
func SetProbes(eof func(lexer any) bool, lookahead func(lexer any) int32, rangeStart func(lexer any) bool) {
	eofProbe, lookaheadProbe, rangeStartProbe = eof, lookahead, rangeStart
}

// AtEOF reports whether the cursor is at the end of the input, as C's
// lexer->eof. The public lexer returns 0 from Lookahead both at the end and at a
// NUL byte; this tells them apart. Without a probe it falls back to the
// lookahead being 0.
func AtEOF(lexer interface{ Lookahead() rune }) bool {
	if eofProbe != nil {
		return eofProbe(lexer)
	}
	return lexer.Lookahead() == 0
}

// CLookahead returns lexer->lookahead as C sees it: the code point, 0 at the end
// or for a NUL byte, and -1 (TS_DECODE_ERROR) for a byte that is not valid
// UTF-8. The public lexer returns U+FFFD for an invalid byte.
func CLookahead(lexer interface{ Lookahead() rune }) int32 {
	if lookaheadProbe != nil {
		return lookaheadProbe(lexer)
	}
	return int32(lexer.Lookahead())
}

// AtRangeStart answers C's lexer->is_at_included_range_start: whether the cursor
// is at the first byte of the current included range. For a parse over the whole
// input there is one range that starts at byte 0, so the answer is true only there
// and only when the input is not empty (C's lexer is at the end of input, with its
// ranges consumed, for an empty source: lexer.c:97, :356). Without a probe it
// reports false.
func AtRangeStart(lexer any) bool {
	return rangeStartProbe != nil && rangeStartProbe(lexer)
}
