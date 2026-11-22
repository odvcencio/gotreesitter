package gotreesitter

import (
	"encoding/binary"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sort"

	"github.com/odvcencio/gotreesitter/internal/ccore"
	"github.com/odvcencio/gotreesitter/internal/ccore/bind"
	"github.com/odvcencio/gotreesitter/internal/scanmode"
)

// External scanner hook for internal/ccore: the language's ExternalScanner
// over an ExternalLexer, one call per scan. The lex automata are not here:
// internal/ccore walks them itself (lexdfa.go) over the lex tables that
// ccore_bind.go converts. Nothing in this file parses; ccore_bind.go hands the
// hook to internal/ccore/bind, which the equality tests in cgo_harness use.

// ccoreExtScanner serves ccore.ExtScanner (through bind.NewFuncScanner, so that no
// exported method lands on a root type and in the API snapshots) over a language's
// ExternalScanner. The payload is created on the first Scan and destroyed by
// Close.
type ccoreExtScanner struct {
	sc            ExternalScanner
	payload       any
	live          bool
	el            ExternalLexer
	rangeCursor   ccore.ScannerCursor
	rangeFrontier externalReadFrontier
	// reg is the registration of the innermost scanner (scanmode.RegisterScanner);
	// layers is the number of order adapters around it.
	reg    scanmode.Registered
	layers int
	// failed is the reason a mode reset failed in close. Once set, every later scan
	// panics with it before Create, Deserialize or Scan of the scanner is called.
	failed string
}

// ccoreScannerMatches compares the caller's configuration with the private
// snapshot. A scanner without an exact comparison must be snapshotted again.
func ccoreScannerMatches(st *ccoreLanguageTables, sc ExternalScanner) bool {
	if !st.scannerPresent || st.scannerError != "" {
		return !st.scannerPresent && sc == nil
	}
	private := st.scanner
	for {
		original, ok := sc.(*externalScannerOrderAdapter)
		if !ok {
			break
		}
		copy, ok := private.(*externalScannerOrderAdapter)
		if !ok || original == nil || copy == nil || original.sourceCount != copy.sourceCount ||
			!slices.Equal(original.targetToSource, copy.targetToSource) ||
			!maps.Equal(original.sourceSymbolToIndex, copy.sourceSymbolToIndex) ||
			!maps.Equal(original.sourceResultToTarget, copy.sourceResultToTarget) ||
			original.targetGrammar != copy.targetGrammar || original.targetGrammarValid != copy.targetGrammarValid {
			return false
		}
		sc, private = original.inner, copy.inner
	}
	if sc == nil || reflect.TypeOf(sc) != st.scannerType || reflect.TypeOf(private) != st.scannerType {
		return false
	}
	reg, ok := scanmode.Lookup(sc)
	return ok && reg.SameConfig != nil && reg.SameConfig(sc, private)
}

// Reasons a grammar is not served by the core. Binding fails with one of them and
// the present engine parses.
const (
	errScannerNotRegistered = scanmode.ReasonScannerNotRegistered
	errScannerNil           = scanmode.ReasonScannerNil
	errPayloadNotRegistered = scanmode.ReasonPayloadNotRegistered
	errScanModeNotDelivered = scanmode.ReasonModeNotDelivered
	errModeNotReset         = scanmode.ReasonModeNotReset
)

// isNilValue reports whether v is a nil interface or holds a nil pointer, map,
// slice, channel or function.
func isNilValue(v any) bool {
	if v == nil {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Ptr, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// ccoreScannerChain walks sc: zero or more order adapters, then an innermost
// scanner whose concrete type is registered. Nothing else is accepted: no wrapper
// of another type, no struct that embeds a scanner, no unknown scanner. A nil
// scanner value, directly or under an adapter, is refused with its own reason
// before any method of it is called. It returns the registration, the number of
// adapters and the reason when the chain is refused.
func ccoreScannerChain(sc ExternalScanner) (reg scanmode.Registered, layers int, reason string) {
	scanmode.Freeze()
	for {
		if isNilValue(sc) {
			return scanmode.Registered{}, 0, errScannerNil
		}
		a, isAdapter := sc.(*externalScannerOrderAdapter)
		if !isAdapter {
			break
		}
		layers++
		sc = a.inner
	}
	reg, ok := scanmode.Lookup(sc)
	if !ok {
		return scanmode.Registered{}, 0, errScannerNotRegistered
	}
	return reg, layers, ""
}

// ccoreScannerSnapshot copies the chain and encodes every adapter setting.
func ccoreScannerSnapshot(sc ExternalScanner) (ExternalScanner, reflect.Type, []byte, string) {
	reg, layers, reason := ccoreScannerChain(sc)
	if reason != "" {
		return nil, nil, nil, reason
	}
	adapters := make([]*externalScannerOrderAdapter, 0, layers)
	inner := sc
	for i := 0; i < layers; i++ {
		a := inner.(*externalScannerOrderAdapter)
		adapters = append(adapters, a)
		inner = a.inner
	}
	private, key, ok := reg.Snapshot(inner)
	cloned, valid := private.(ExternalScanner)
	if !ok || !valid || isNilValue(cloned) || reflect.TypeOf(cloned) != reflect.TypeOf(inner) {
		return nil, nil, nil, errScannerNotRegistered
	}
	scannerType := reflect.TypeOf(inner)
	b := binary.AppendUvarint(nil, uint64(layers))
	putInt := func(v int) { b = binary.AppendVarint(b, int64(v)) }
	for _, a := range adapters {
		putInt(a.sourceCount)
		b = binary.AppendUvarint(b, uint64(len(a.targetToSource)))
		for _, v := range a.targetToSource {
			putInt(v)
		}
		keys := make([]int, 0, len(a.sourceSymbolToIndex))
		for k := range a.sourceSymbolToIndex {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		b = binary.AppendUvarint(b, uint64(len(keys)))
		for _, k := range keys {
			putInt(k)
			putInt(a.sourceSymbolToIndex[Symbol(k)])
		}
		keys = keys[:0]
		for k := range a.sourceResultToTarget {
			keys = append(keys, int(k))
		}
		sort.Ints(keys)
		b = binary.AppendUvarint(b, uint64(len(keys)))
		for _, k := range keys {
			putInt(k)
			putInt(int(a.sourceResultToTarget[Symbol(k)]))
		}
		b = append(b, a.targetGrammar[:]...)
		if a.targetGrammarValid {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
	}
	b = binary.AppendUvarint(b, uint64(len(key)))
	b = append(b, key...)
	for i := len(adapters) - 1; i >= 0; i-- {
		original := adapters[i]
		dup := *original
		dup.inner = cloned
		if original.targetToSource != nil {
			dup.targetToSource = append([]int{}, original.targetToSource...)
		}
		if original.sourceSymbolToIndex != nil {
			dup.sourceSymbolToIndex = make(map[Symbol]int, len(original.sourceSymbolToIndex))
			for k, v := range original.sourceSymbolToIndex {
				dup.sourceSymbolToIndex[k] = v
			}
		}
		if original.sourceResultToTarget != nil {
			dup.sourceResultToTarget = make(map[Symbol]Symbol, len(original.sourceResultToTarget))
			for k, v := range original.sourceResultToTarget {
				dup.sourceResultToTarget[k] = v
			}
		}
		cloned = &dup
	}
	return cloned, scannerType, b, ""
}

// ccoreEnter walks a payload created by the chain: order adapter payloads (each
// marked as driven by the core), then the innermost payload, whose concrete type
// must be the registered one. For a scanner with a behavioural mode it turns the
// mode on in the innermost payload. It returns the named reason on failure.
func ccoreEnter(payload any, reg scanmode.Registered, layers int) (err error) {
	for i := 0; i < layers; i++ {
		w, ok := payload.(*externalScannerOrderAdapterPayload)
		if !ok || w == nil {
			return errors.New(errPayloadNotRegistered)
		}
		w.coreMode = true
		payload = w.inner
	}
	if _, isAdapter := payload.(*externalScannerOrderAdapterPayload); isAdapter || reflect.TypeOf(payload) != reg.Payload {
		return errors.New(errPayloadNotRegistered)
	}
	if reg.NeedsMode {
		m, ok := payload.(scanmode.CFaithful)
		if !ok || isNilValue(payload) {
			return errors.New(errScanModeNotDelivered)
		}
		m.SetCFaithful(true)
	}
	return nil
}

// ccoreLeave switches off what ccoreEnter switched on. It clears the flag of every
// adapter payload (the walk is bounded by layers and ends at the first payload that
// is not an adapter payload), and then, only for a registration that declares the
// mode and only for a payload of exactly the registered type, calls SetCFaithful(false)
// once. It never calls a method on a payload that validation rejected. The cursor is
// advanced before any call into the port, so a panic in the port ends the walk: it
// returns errModeNotReset in that case and never calls the setter again.
func ccoreLeave(payload any, reg scanmode.Registered, layers int) (err error) {
	for i := 0; i < layers; i++ {
		w, ok := payload.(*externalScannerOrderAdapterPayload)
		if !ok || w == nil {
			return nil
		}
		next := w.inner
		w.coreMode = false
		payload = next
	}
	if !reg.NeedsMode || reflect.TypeOf(payload) != reg.Payload || isNilValue(payload) {
		return nil
	}
	m, ok := payload.(scanmode.CFaithful)
	if !ok {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = errors.New(errModeNotReset)
		}
	}()
	m.SetCFaithful(false)
	return nil
}

// ccoreRelease ends the core's use of a payload: the mode goes off, then the scanner's
// Destroy runs exactly once, so a scanner that pools payloads never publishes one
// with a mode on while the setter works. If the setter panicked, the payload is still
// passed to Destroy once and the error is returned: the caller must not use the
// scanner again.
func ccoreRelease(sc ExternalScanner, payload any, reg scanmode.Registered, layers int) error {
	err := ccoreLeave(payload, reg, layers)
	sc.Destroy(payload)
	return err
}

// newCcoreExtScanner builds the core's scanner hook over sc, or fails with the
// named reason when the chain is not fully known: it creates a probe payload,
// enters it, and releases it, with the release deferred from the moment the payload
// exists so that a panic anywhere leaves every flag off and the payload destroyed.
func newCcoreExtScanner(sc ExternalScanner) (a *ccoreExtScanner, err error) {
	reg, layers, reason := ccoreScannerChain(sc)
	if reason != "" {
		return nil, errors.New(reason)
	}
	p := sc.Create()
	defer func() {
		if rerr := ccoreRelease(sc, p, reg, layers); rerr != nil && err == nil {
			a, err = nil, rerr
		}
	}()
	if err := ccoreEnter(p, reg, layers); err != nil {
		return nil, err
	}
	return &ccoreExtScanner{sc: sc, reg: reg, layers: layers}, nil
}

// scan runs C's deserialize, scan and serialize in that order
// (ts_parser__external_scanner_deserialize, _scan, _serialize).
func (a *ccoreExtScanner) scan(src []byte, pos int, row, col uint32, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	return a.scanWithCursor(src, pos, row, col, nil, prev, valid, buf, out)
}

func (a *ccoreExtScanner) scanIncluded(src []byte, cursor ccore.ScannerCursor, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	a.rangeCursor = cursor
	at := cursor.Position()
	return a.scanWithCursor(src, int(at.Bytes()), at.Row(), at.Col(), &a.rangeCursor, prev, valid, buf, out)
}

func (a *ccoreExtScanner) scanWithCursor(src []byte, pos int, row, col uint32, cursor *ccore.ScannerCursor, prev []byte, valid []bool, buf []byte, out *ccore.ExtScan) bool {
	defer func() {
		if cursor != nil {
			a.el.readFrontier = nil
			a.rangeFrontier = externalReadFrontier{}
			a.rangeCursor = ccore.ScannerCursor{}
		}
		if r := recover(); r != nil { // a panic ends the parse: release the payload with the mode off
			a.close()
			panic(r)
		}
	}()
	if a.failed != "" { // a failed mode reset ended this hook: the scanner is not called again
		panic(a.failed)
	}
	if !a.live { // ts_parser__external_scanner_create
		a.payload, a.live = a.sc.Create(), true
		if err := ccoreEnter(a.payload, a.reg, a.layers); err != nil {
			panic(err.Error())
		}
	}
	el := &a.el
	el.reset(src, pos, row, col) // ts_lexer_start
	if cursor != nil {
		a.rangeFrontier.included = cursor
		el.readFrontier = &a.rangeFrontier
		if pos == 0 && !el.atEOF() {
			if el.Lookahead() == byteOrderMarkRune {
				el.Advance(true)
			}
			el.columnData, el.columnDataValid = 0, true
		}
	}
	a.sc.Deserialize(a.payload, prev)
	found := a.sc.Scan(a.payload, el, valid)
	out.DidGetColumn = el.didGetColumn
	out.LookaheadEnd = el.lookaheadEndByteAtCursor() // ts_lexer_finish
	if !found {
		return false
	}
	tok, ok := el.token()
	if !ok {
		return false
	}
	out.StateLen = a.sc.Serialize(a.payload, buf)
	out.Token = ccore.Token{
		Symbol:       ccore.Symbol(tok.Symbol),
		StartByte:    tok.StartByte,
		EndByte:      tok.EndByte,
		StartRow:     tok.StartPoint.Row,
		StartCol:     tok.StartPoint.Column,
		EndRow:       tok.EndPoint.Row,
		EndCol:       tok.EndPoint.Column,
		LookaheadEnd: tok.lexerLookaheadEndByte,
	}
	return true
}

// close destroys the scanner payload if one exists (ts_parser__external_scanner_destroy).
// The payload is detached and live is cleared before anything of the scanner runs,
// so a panic in the mode switch or in Destroy leaves one destruction attempt only:
// a second close does nothing.
func (a *ccoreExtScanner) close() {
	if !a.live {
		return
	}
	p := a.payload
	a.payload, a.live = nil, false
	if err := ccoreRelease(a.sc, p, a.reg, a.layers); err != nil {
		a.failed = err.Error()
		panic(a.failed)
	}
}

// hook returns the ccore.ExtScanner for this scanner.
func (a *ccoreExtScanner) hook() ccore.ExtScanner {
	return bind.NewRangedFuncScanner(a.scan, a.scanIncluded, a.close)
}
