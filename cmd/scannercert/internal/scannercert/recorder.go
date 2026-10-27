package scannercert

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"slices"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/incr"
)

// stateBufferSize is the buffer the parser hands to Serialize
// (externalScannerSerializationBufferSize in the root package).
const stateBufferSize = 4096

// outcome is everything one run of Scan produced that a replay must match.
type outcome struct {
	accepted          bool
	hasSymbol         bool
	symbol            gts.Symbol
	start, end        uint32
	endRow, endColumn uint32
	state             []byte // Serialize after the scan
}

// call is one distinct Scan call: where it began, what the parser allowed,
// the scanner state before it, and what it did.
type call struct {
	corpus      int // corpus source being parsed when the call first happened
	source      int // index into recorder.sources
	origin      uint32
	row, column uint32
	mask        []bool
	state       []byte // Serialize before the scan
	live        outcome
	unstable    bool // two Serialize calls on one state returned different bytes
	overflow    bool // Serialize reported more bytes than the buffer holds
}

// recorder stands in for a grammar's scanner while the parser runs. It
// forwards every call and records each Scan.
//
// It also implements the optional scanner interfaces that a full parse
// consults. Where the real scanner lacks one, the recorder answers with the
// value the parser assumes for a missing interface, so the parse is the same
// with the recorder as without it. Certify compares trees, and probes_test.go
// compares the root's questions with what the recorder answers; the package
// comment says what neither can see. The interfaces that only incremental
// reuse consults are not mirrored. Every parse, a fresh one too, asks the read
// certificates, and the non-leaf reuse policy of a scanner that certifies
// checkpoint reads, so those are mirrored.
type recorder struct {
	inner gts.ExternalScanner

	corpus   int
	calls    []call
	index    map[string]int
	recorded int
	key      []byte

	sources  [][]byte
	bySource map[[sha256.Size]byte]int
	last     struct {
		first *byte
		n     int
		index int
	}

	buf, check []byte
}

func newRecorder(inner gts.ExternalScanner) *recorder {
	return &recorder{
		inner:    inner,
		index:    map[string]int{},
		bySource: map[[sha256.Size]byte]int{},
		buf:      make([]byte, stateBufferSize),
		check:    make([]byte, stateBufferSize),
	}
}

// begin names the corpus source the next calls belong to.
func (r *recorder) begin(corpus int) { r.corpus = corpus }

func (r *recorder) Create() any                     { return r.inner.Create() }
func (r *recorder) Destroy(payload any)             { r.inner.Destroy(payload) }
func (r *recorder) Serialize(p any, buf []byte) int { return r.inner.Serialize(p, buf) }
func (r *recorder) Deserialize(p any, buf []byte)   { r.inner.Deserialize(p, buf) }

func (r *recorder) SupportsIncrementalReuse() bool {
	s, ok := r.inner.(gts.IncrementalReuseExternalScanner)
	return ok && s.SupportsIncrementalReuse()
}

func (r *recorder) UsesExternalScannerCheckpoints() bool {
	s, ok := r.inner.(gts.CheckpointedExternalScanner)
	return ok && s.UsesExternalScannerCheckpoints()
}

func (r *recorder) ExternalScannerIsStateless() bool {
	s, ok := r.inner.(gts.StatelessExternalScanner)
	return ok && s.ExternalScannerIsStateless()
}

func (r *recorder) PreservesStateOnScanFailure() bool {
	s, ok := r.inner.(gts.FailurePreservingExternalScanner)
	return ok && s.PreservesStateOnScanFailure()
}

func (r *recorder) RetainsStateOnScanFailure() bool {
	s, ok := r.inner.(gts.FailureStateRetainingExternalScanner)
	return ok && s.RetainsStateOnScanFailure()
}

// SupportsErrorModeExternalRetry reads true when the scanner has no opinion,
// as the parser does.
func (r *recorder) SupportsErrorModeExternalRetry() bool {
	s, ok := r.inner.(interface{ SupportsErrorModeExternalRetry() bool })
	return !ok || s.SupportsErrorModeExternalRetry()
}

// SupportsPackedGSSVersionOrder reads true when the scanner has no opinion, as
// the parser does.
func (r *recorder) SupportsPackedGSSVersionOrder(n int) bool {
	s, ok := r.inner.(interface{ SupportsPackedGSSVersionOrder(int) bool })
	return !ok || s.SupportsPackedGSSVersionOrder(n)
}

// The next four methods answer what every parse asks when it starts
// (legacyReuseReadsEligible, through beginLegacyReuseReads). The answers decide
// whether the parse records read history and charges it to the arena budget.
// The root reads a missing interface and a false answer alike, so the recorder
// reads false where the scanner has no opinion.

func (r *recorder) SupportsStatelessReadDependencies() bool {
	s, ok := r.inner.(incr.StatelessReadScanner)
	return ok && s.SupportsStatelessReadDependencies()
}

func (r *recorder) SupportsLargeStatelessReadDependencies() bool {
	s, ok := r.inner.(incr.LargeStatelessReadScanner)
	return ok && s.SupportsLargeStatelessReadDependencies()
}

func (r *recorder) SupportsCheckpointReadDependencies() bool {
	s, ok := r.inner.(incr.CheckpointReadScanner)
	return ok && s.SupportsCheckpointReadDependencies()
}

func (r *recorder) SupportsLargeCheckpointReadDependencies() bool {
	s, ok := r.inner.(incr.LargeCheckpointReadScanner)
	return ok && s.SupportsLargeCheckpointReadDependencies()
}

// SupportsCheckpointedNonLeafReuse reads true when the scanner has no opinion,
// as the parser does. The check that asks the certificates above asks it next
// for a scanner that certifies checkpoint reads.
func (r *recorder) SupportsCheckpointedNonLeafReuse() bool {
	s, ok := r.inner.(interface{ SupportsCheckpointedNonLeafReuse() bool })
	return !ok || s.SupportsCheckpointedNonLeafReuse()
}

// identityRecorder adds the checkpoint identity method for a scanner that has
// one. The parser treats the method's presence, not only its value, as a
// requirement, so a scanner without it must not gain it.
type identityRecorder struct{ *recorder }

func (r *identityRecorder) CheckpointIdentity() (gts.ExternalScannerCheckpointIdentity, bool) {
	return r.inner.(gts.ExternalScannerCheckpointIdentityProvider).CheckpointIdentity()
}

// install returns the scanner to put in the language in place of rec.inner.
func (r *recorder) install() gts.ExternalScanner {
	if _, ok := r.inner.(gts.ExternalScannerCheckpointIdentityProvider); ok {
		return &identityRecorder{r}
	}
	return r
}

// Scan runs the real scanner and records the call. It reads the lexer's
// origin by reflection because every lexer method would update the parser's
// read-frontier bookkeeping.
func (r *recorder) Scan(payload any, lexer *gts.ExternalLexer, valid []bool) bool {
	if lexer == nil {
		return r.inner.Scan(payload, lexer, valid)
	}
	source, pos, row, column := lexerOrigin(lexer)
	c := call{
		corpus: r.corpus, source: r.intern(source),
		origin: uint32(pos), row: row, column: column,
		mask: slices.Clone(valid),
	}
	c.state, c.unstable, c.overflow = snapshot(r.inner, payload, r.buf, r.check)
	accepted := r.inner.Scan(payload, lexer, valid)
	var unstable, overflow bool
	c.live, unstable, overflow = observe(r.inner, payload, lexer, accepted, r.buf, r.check)
	c.unstable = c.unstable || unstable
	c.overflow = c.overflow || overflow
	r.add(c)
	return accepted
}

// snapshot serializes payload twice and returns the first result. It reports
// whether the two results differ and whether the scanner claimed more bytes
// than the buffer holds.
func snapshot(scanner gts.ExternalScanner, payload any, buf, check []byte) (state []byte, unstable, overflow bool) {
	n, over := clampLength(scanner.Serialize(payload, buf), len(buf))
	m, overCheck := clampLength(scanner.Serialize(payload, check), len(check))
	return bytes.Clone(buf[:n]), !bytes.Equal(buf[:n], check[:m]), over || overCheck
}

func clampLength(n, limit int) (int, bool) {
	switch {
	case n < 0:
		return 0, true
	case n > limit:
		return limit, true
	}
	return n, false
}

// observe reads what a scan left behind: its result, its token and the
// scanner state.
func observe(scanner gts.ExternalScanner, payload any, lexer *gts.ExternalLexer, accepted bool, buf, check []byte) (o outcome, unstable, overflow bool) {
	tok, has := lexerToken(lexer)
	o = outcome{accepted: accepted, hasSymbol: has}
	if has {
		o.symbol = tok.Symbol
	}
	if accepted && has {
		o.start, o.end = tok.StartByte, tok.EndByte
		o.endRow, o.endColumn = tok.EndPoint.Row, tok.EndPoint.Column
	}
	o.state, unstable, overflow = snapshot(scanner, payload, buf, check)
	return o, unstable, overflow
}

// intern keeps one private copy of each distinct source, so a replay sees the
// bytes the scanner saw even if the parser reuses its buffer. The parser
// scans the same slice many times in a row, so the last slice is compared
// directly before any hashing.
func (r *recorder) intern(src []byte) int {
	if len(src) > 0 && &src[0] == r.last.first && len(src) == r.last.n && bytes.Equal(r.sources[r.last.index], src) {
		return r.last.index
	}
	sum := sha256.Sum256(src)
	index, ok := r.bySource[sum]
	if !ok {
		index = len(r.sources)
		r.sources = append(r.sources, bytes.Clone(src))
		r.bySource[sum] = index
	}
	if len(src) > 0 {
		r.last.first, r.last.n, r.last.index = &src[0], len(src), index
	}
	return index
}

// add records c unless an identical call is already there. The key includes
// the live outcome, so two calls that differ only in what the scanner did stay
// separate, and the replay (which is deterministic) must disagree with at
// least one of them.
func (r *recorder) add(c call) {
	r.recorded++
	r.key = c.appendKey(r.key[:0])
	if _, seen := r.index[string(r.key)]; seen {
		return
	}
	r.index[string(r.key)] = len(r.calls)
	r.calls = append(r.calls, c)
}

func (c *call) appendKey(b []byte) []byte {
	u32 := binary.LittleEndian.AppendUint32
	b = u32(b, uint32(c.source))
	b = u32(b, c.origin)
	b = u32(b, c.row)
	b = u32(b, c.column)
	b = appendMask(b, c.mask)
	b = appendBytes(b, c.state)
	var flags byte
	for i, set := range []bool{c.live.accepted, c.live.hasSymbol, c.unstable, c.overflow} {
		if set {
			flags |= 1 << i
		}
	}
	b = append(b, flags)
	b = binary.LittleEndian.AppendUint16(b, uint16(c.live.symbol))
	b = u32(b, c.live.start)
	b = u32(b, c.live.end)
	b = u32(b, c.live.endRow)
	b = u32(b, c.live.endColumn)
	return appendBytes(b, c.live.state)
}

func appendBytes(b, data []byte) []byte {
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}

// appendMask appends the mask's length and its bits, eight to a byte.
func appendMask(b []byte, mask []bool) []byte {
	b = binary.LittleEndian.AppendUint32(b, uint32(len(mask)))
	var bits byte
	for i, set := range mask {
		if set {
			bits |= 1 << (i % 8)
		}
		if i%8 == 7 {
			b = append(b, bits)
			bits = 0
		}
	}
	if len(mask)%8 != 0 {
		b = append(b, bits)
	}
	return b
}
