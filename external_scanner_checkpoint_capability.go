package gotreesitter

import "bytes"

const externalScannerCheckpointIdentityMaxBytes = 256

// ExternalScannerCheckpointIdentity identifies the scanner and grammar that
// produced an external-scanner checkpoint. Both identifiers must be stable,
// non-empty, and at most 256 bytes for the scanner capability to be accepted.
type ExternalScannerCheckpointIdentity struct {
	Scanner []byte
	Grammar []byte
}

// ExternalScannerCheckpointIdentityProvider is an opt-in extension for a
// checkpointed scanner. CheckpointIdentity must return stable identifiers for
// the scanner implementation and the exact grammar blob.
//
// This capability is not consumed by parser production, GLR scheduling,
// recovery, merge, or incremental-reuse paths yet.
type ExternalScannerCheckpointIdentityProvider interface {
	CheckpointedExternalScanner
	CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool)
}

type externalScannerCheckpointRecord struct {
	identity         ExternalScannerCheckpointIdentity
	sourceByte       uint32
	sourcePoint      Point
	externalLexState uint16
	tokenStartByte   uint32
	tokenEndByte     uint32
	serialized       []byte
}

func (id ExternalScannerCheckpointIdentity) complete() bool {
	return len(id.Scanner) > 0 && len(id.Scanner) <= externalScannerCheckpointIdentityMaxBytes &&
		len(id.Grammar) > 0 && len(id.Grammar) <= externalScannerCheckpointIdentityMaxBytes
}

func (id ExternalScannerCheckpointIdentity) clone() ExternalScannerCheckpointIdentity {
	return ExternalScannerCheckpointIdentity{
		Scanner: append([]byte(nil), id.Scanner...),
		Grammar: append([]byte(nil), id.Grammar...),
	}
}

func (id ExternalScannerCheckpointIdentity) equal(other ExternalScannerCheckpointIdentity) bool {
	return bytes.Equal(id.Scanner, other.Scanner) && bytes.Equal(id.Grammar, other.Grammar)
}

func (r externalScannerCheckpointRecord) complete() bool {
	return r.identity.complete() && len(r.serialized) != 0 &&
		len(r.serialized) <= externalScannerSerializationBufferSize &&
		r.tokenStartByte <= r.tokenEndByte
}

func (r externalScannerCheckpointRecord) clone() externalScannerCheckpointRecord {
	r.identity = r.identity.clone()
	r.serialized = append([]byte(nil), r.serialized...)
	return r
}

func (r externalScannerCheckpointRecord) equal(other externalScannerCheckpointRecord) bool {
	return r.complete() && other.complete() &&
		r.identity.equal(other.identity) &&
		r.sourceByte == other.sourceByte &&
		r.sourcePoint == other.sourcePoint &&
		r.externalLexState == other.externalLexState &&
		r.tokenStartByte == other.tokenStartByte &&
		r.tokenEndByte == other.tokenEndByte &&
		bytes.Equal(r.serialized, other.serialized)
}

// captureExternalScannerCheckpointRecord captures a complete checkpoint
// without changing any parser or token-source state.
func captureExternalScannerCheckpointRecord(
	scanner ExternalScanner,
	payload any,
	sourceByte uint32,
	sourcePoint Point,
	externalLexState uint16,
	tokenStartByte uint32,
	tokenEndByte uint32,
) (externalScannerCheckpointRecord, bool) {
	if scanner == nil || tokenStartByte > tokenEndByte {
		return externalScannerCheckpointRecord{}, false
	}
	provider, ok := scanner.(ExternalScannerCheckpointIdentityProvider)
	if !ok || !provider.UsesExternalScannerCheckpoints() {
		return externalScannerCheckpointRecord{}, false
	}
	identity, ok := provider.CheckpointIdentity()
	if !ok || !identity.complete() {
		return externalScannerCheckpointRecord{}, false
	}

	buf := make([]byte, externalScannerSerializationBufferSize)
	n := scanner.Serialize(payload, buf)
	if n <= 0 || n > len(buf) {
		return externalScannerCheckpointRecord{}, false
	}
	return externalScannerCheckpointRecord{
		identity:         identity.clone(),
		sourceByte:       sourceByte,
		sourcePoint:      sourcePoint,
		externalLexState: externalLexState,
		tokenStartByte:   tokenStartByte,
		tokenEndByte:     tokenEndByte,
		serialized:       append([]byte(nil), buf[:n]...),
	}, true
}

func canShareExternalScannerCheckpoint(a, b externalScannerCheckpointRecord) bool {
	return a.equal(b)
}

func (r externalScannerCheckpointRecord) restore(scanner ExternalScanner, payload any) bool {
	// Callers must discard payload when restore fails because Deserialize may mutate it.
	if scanner == nil || !r.complete() {
		return false
	}
	provider, ok := scanner.(ExternalScannerCheckpointIdentityProvider)
	if !ok || !provider.UsesExternalScannerCheckpoints() {
		return false
	}
	identity, ok := provider.CheckpointIdentity()
	if !ok || !identity.complete() || !r.identity.equal(identity) {
		return false
	}
	scanner.Deserialize(payload, append([]byte(nil), r.serialized...))
	buf := make([]byte, externalScannerSerializationBufferSize)
	n := scanner.Serialize(payload, buf)
	return n == len(r.serialized) && n <= len(buf) && bytes.Equal(buf[:n], r.serialized)
}
