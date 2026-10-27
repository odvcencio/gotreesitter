package scannercert

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"unicode/utf8"

	gts "github.com/odvcencio/gotreesitter"
)

// contextBytes is how much source a witness quotes, starting at the origin.
const contextBytes = 48

// symbolNames turns symbols and masks into the names a reader recognizes.
// The zero value names nothing.
type symbolNames struct{ lang *gts.Language }

func (n symbolNames) symbol(sym gts.Symbol) string {
	if n.lang == nil || int(sym) >= len(n.lang.SymbolNames) {
		return ""
	}
	return n.lang.SymbolNames[sym]
}

// valid lists the external tokens a mask allows as "index:name", or just the
// index when the name is unknown.
func (n symbolNames) valid(mask []bool) []string {
	out := []string{}
	for i, allowed := range mask {
		if !allowed {
			continue
		}
		label := strconv.Itoa(i)
		if n.lang != nil && i < len(n.lang.ExternalSymbols) {
			if name := n.symbol(n.lang.ExternalSymbols[i]); name != "" {
				label += ":" + name
			}
		}
		out = append(out, label)
	}
	return out
}

// analysis is the replay result for one recording.
type analysis struct {
	calls     CallInfo
	checks    Checks
	witnesses []Witness
}

func (a analysis) verdict() string {
	switch {
	case a.checks.failures() > 0:
		return VerdictFindings
	case a.calls.Distinct == 0:
		return VerdictNoEvidence
	}
	return VerdictNoFindings
}

// replayed is what a replay saw.
type replayed struct {
	out      outcome
	restored []byte // Serialize(Deserialize(state)), before the scan
	panicked string
}

// replay runs one recorded call again from the state's serialized bytes, on a
// fresh payload. A panic in the scanner is a result, not a crash: a state the
// scanner cannot restore is the kind of defect this tool exists to find.
func replay(scanner gts.ExternalScanner, c *call, source []byte) (res replayed) {
	defer func() {
		if p := recover(); p != nil {
			res = replayed{panicked: fmt.Sprint(p)}
		}
	}()
	buf, check := make([]byte, stateBufferSize), make([]byte, stateBufferSize)
	payload := scanner.Create()
	defer scanner.Destroy(payload)
	// Deserialize may keep or change its argument, so hand it a copy. The
	// parser passes nil for an empty snapshot.
	var wire []byte
	if len(c.state) > 0 {
		wire = bytes.Clone(c.state)
	}
	scanner.Deserialize(payload, wire)
	res.restored, _, _ = snapshot(scanner, payload, buf, check)
	lexer := newLexer(source, int(c.origin), c.row, c.column)
	accepted := scanner.Scan(payload, lexer, slices.Clone(c.mask))
	res.out, _, _ = observe(scanner, payload, lexer, accepted, buf, check)
	return res
}

// failure is one check that a call failed.
type failure struct{ kind, detail string }

// compare names the checks that call c failed, given what its replay did.
// Failures come in a fixed order, so reports do not vary.
func compare(c *call, res replayed) []failure {
	var failures []failure
	if res.panicked == "" {
		live, again := c.live, res.out
		// When the results differ, the other differences follow from that one,
		// so the report names only the result.
		if live.accepted != again.accepted {
			failures = append(failures, failure{KindResult, "Scan returned a different result"})
		} else {
			if live.hasSymbol != again.hasSymbol || live.symbol != again.symbol {
				failures = append(failures, failure{KindSymbol, "Scan set a different result symbol"})
			}
			if live.accepted && (live.start != again.start || live.end != again.end ||
				live.endRow != again.endRow || live.endColumn != again.endColumn) {
				failures = append(failures, failure{KindSpan, "Scan produced a token with a different span"})
			}
			if !bytes.Equal(live.state, again.state) {
				failures = append(failures, failure{KindState, "Serialize after Scan returned different bytes"})
			}
		}
		if !bytes.Equal(c.state, res.restored) {
			failures = append(failures, failure{KindRoundTrip, "Deserialize then Serialize returned " + hexOrEmpty(res.restored)})
		}
	}
	if !c.live.accepted && !bytes.Equal(c.state, c.live.state) {
		failures = append(failures, failure{KindFailedScanState, "a rejected Scan changed the serialized state"})
	}
	if c.unstable {
		failures = append(failures, failure{KindSerializeUnstable, "two Serialize calls on one state returned different bytes"})
	}
	if c.overflow {
		failures = append(failures, failure{KindSerializeOverflow, fmt.Sprintf("Serialize returned a length outside the %d-byte buffer", stateBufferSize)})
	}
	if res.panicked != "" {
		failures = append(failures, failure{KindReplayPanic, res.panicked})
	}
	return failures
}

// analyze replays every distinct recorded call on inner, which must be the
// scanner the recorder wrapped, and totals the checks.
func (r *recorder) analyze(inner gts.ExternalScanner, names symbolNames) analysis {
	a := analysis{witnesses: []Witness{}}
	a.calls.Recorded = r.recorded
	a.calls.Distinct = len(r.calls)
	states := map[string]struct{}{}
	masks := map[string]struct{}{}
	witnessed := map[string]int{}
	for i := range r.calls {
		c := &r.calls[i]
		source := r.sources[c.source]
		states[string(c.state)] = struct{}{}
		masks[string(appendMask(nil, c.mask))] = struct{}{}
		if c.live.accepted {
			a.calls.Accepted++
		} else {
			a.calls.Rejected++
		}
		if len(c.state) == 0 {
			a.calls.EmptyStates++
		}
		a.calls.MaxStateBytes = max(a.calls.MaxStateBytes, len(c.state))

		res := replay(inner, c, source)
		a.checks.Replayed++
		for _, f := range compare(c, res) {
			a.checks.add(f.kind)
			if witnessed[f.kind] >= MaxWitnessesPerKind {
				continue
			}
			witnessed[f.kind]++
			w := Witness{
				Kind:    f.kind,
				Source:  c.corpus,
				Origin:  Origin{Byte: c.origin, Row: c.row, Column: c.column},
				Context: excerpt(source, c.origin),
				Valid:   names.valid(c.mask),
				State:   hex.EncodeToString(c.state),
				Live:    outcomeReport(c.live, names),
				Detail:  f.detail,
			}
			if res.panicked == "" {
				w.Replay = outcomeReport(res.out, names)
			}
			a.witnesses = append(a.witnesses, w)
		}
	}
	a.calls.States = len(states)
	a.calls.Masks = len(masks)
	return a
}

func hexOrEmpty(b []byte) string {
	if len(b) == 0 {
		return "no bytes"
	}
	return hex.EncodeToString(b)
}

func outcomeReport(o outcome, names symbolNames) *Outcome {
	out := &Outcome{
		Accepted:  o.accepted,
		HasSymbol: o.hasSymbol,
		Symbol:    int(o.symbol),
		StartByte: o.start, EndByte: o.end, EndRow: o.endRow, EndColumn: o.endColumn,
		State: hex.EncodeToString(o.state),
	}
	if o.hasSymbol {
		out.SymbolName = names.symbol(o.symbol)
	}
	return out
}

// excerpt quotes the source from the origin, without a cut-off final rune.
func excerpt(source []byte, origin uint32) string {
	if int(origin) >= len(source) {
		return ""
	}
	end := min(len(source), int(origin)+contextBytes)
	text := source[origin:end]
	if end < len(source) {
		for i := 0; i < utf8.UTFMax-1 && len(text) > 0; i++ {
			if r, size := utf8.DecodeLastRune(text); r != utf8.RuneError || size != 1 {
				break
			}
			text = text[:len(text)-1]
		}
	}
	return string(text)
}
