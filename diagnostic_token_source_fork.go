//go:build gts_workcount

package gotreesitter

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// DiagnosticTokenSourceForkReceipt records the state at a diagnostic fork.
// Production builds do not include this type.
type DiagnosticTokenSourceForkReceipt struct {
	TokenSourceKind                  string
	CursorByte                       uint32
	CursorPoint                      Point
	ParserState                      StateID
	ActiveGLRStates                  []StateID
	PendingTokenCount                int
	PendingTokenDigest               [sha256.Size]byte
	ZeroWidthOffset                  int64
	IncludedRangeIndex               int
	IncludedRangeCount               int
	IncludedRangeDigest              [sha256.Size]byte
	ExternalScannerCheckpointPresent bool
	ExternalScannerCheckpointDigest  [sha256.Size]byte
}

type diagnosticTokenSourceForker interface {
	ForkTokenSourceForDiagnostic() (TokenSource, DiagnosticTokenSourceForkReceipt, error)
}

func forkTokenSourceForDiagnostic(source TokenSource) (TokenSource, DiagnosticTokenSourceForkReceipt, error) {
	if source == nil {
		return nil, DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic token fork: source is nil")
	}
	forker, ok := source.(diagnosticTokenSourceForker)
	if !ok {
		return nil, DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic token fork: source %T cannot fork", source)
	}
	return forker.ForkTokenSourceForDiagnostic()
}

func (s *includedRangeTokenSource) ForkTokenSourceForDiagnostic() (TokenSource, DiagnosticTokenSourceForkReceipt, error) {
	if s == nil || s.base == nil {
		return nil, DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic token fork: included-range source is nil")
	}
	if s.idx < 0 || s.idx > len(s.ranges) {
		return nil, DiagnosticTokenSourceForkReceipt{}, fmt.Errorf("diagnostic token fork: included-range index %d is outside 0..%d", s.idx, len(s.ranges))
	}

	base, receipt, err := forkTokenSourceForDiagnostic(s.base)
	if err != nil {
		return nil, DiagnosticTokenSourceForkReceipt{}, err
	}
	ranges := append([]Range(nil), s.ranges...)
	fork := &includedRangeTokenSource{
		base:   base,
		ranges: ranges,
		idx:    s.idx,
	}
	receipt.TokenSourceKind = "included-range/" + receipt.TokenSourceKind
	receipt.IncludedRangeIndex = s.idx
	receipt.IncludedRangeCount = len(s.ranges)
	receipt.IncludedRangeDigest = diagnosticIncludedRangeDigest(s.ranges)
	return fork, receipt, nil
}

func diagnosticIncludedRangeDigest(ranges []Range) [sha256.Size]byte {
	hash := sha256.New()
	var value [8]byte
	write := func(v uint64) {
		binary.LittleEndian.PutUint64(value[:], v)
		_, _ = hash.Write(value[:])
	}
	write(uint64(len(ranges)))
	for _, item := range ranges {
		write(uint64(item.StartByte))
		write(uint64(item.EndByte))
		write(uint64(item.StartPoint.Row))
		write(uint64(item.StartPoint.Column))
		write(uint64(item.EndPoint.Row))
		write(uint64(item.EndPoint.Column))
	}
	var digest [sha256.Size]byte
	copy(digest[:], hash.Sum(nil))
	return digest
}
