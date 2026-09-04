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
// The production parser consumes this capability at checkpoint-aware
// incremental reuse and per-version lexer ownership boundaries. It does not
// alter GLR scheduling or recovery election.
type ExternalScannerCheckpointIdentityProvider interface {
	CheckpointedExternalScanner
	CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool)
}

// externalScannerCheckpointIdentitySourceProviderForScanner unwraps order
// adapters to find the scanner implementation identity they preserve.
func externalScannerCheckpointIdentitySourceProviderForScanner(
	scanner ExternalScanner,
) (ExternalScannerCheckpointIdentityProvider, bool) {
	if scanner == nil {
		return nil, false
	}
	if adapter, ok := scanner.(*externalScannerOrderAdapter); ok {
		if adapter == nil {
			return nil, false
		}
		return externalScannerCheckpointIdentitySourceProviderForScanner(adapter.optionalInner())
	}
	provider, ok := scanner.(ExternalScannerCheckpointIdentityProvider)
	if !ok || !provider.UsesExternalScannerCheckpoints() {
		return nil, false
	}
	return provider, true
}

// externalScannerCheckpointIdentityProviderForScanner finds an identity
// provider without promoting a legacy order adapter to the identity contract.
// An identity-bearing order adapter remains required even when its target
// grammar identity is unavailable, so callers fail closed.
func externalScannerCheckpointIdentityProviderForScanner(
	scanner ExternalScanner,
) (ExternalScannerCheckpointIdentityProvider, bool) {
	if scanner == nil {
		return nil, false
	}
	if adapter, ok := scanner.(*externalScannerOrderAdapter); ok {
		if adapter == nil {
			return nil, false
		}
		_, required := externalScannerCheckpointIdentitySourceProviderForScanner(adapter.optionalInner())
		if !required {
			return nil, false
		}
		return adapter, true
	}
	return externalScannerCheckpointIdentitySourceProviderForScanner(scanner)
}

// externalScannerCheckpointIdentityState stores one immutable identity for an
// arena. Fixed storage keeps checkpoint identity outside parser hot structures.
type externalScannerCheckpointIdentityState struct {
	scanner    [externalScannerCheckpointIdentityMaxBytes]byte
	grammar    [externalScannerCheckpointIdentityMaxBytes]byte
	scannerLen uint16
	grammarLen uint16
	valid      bool
	conflict   bool
}

func (s *externalScannerCheckpointIdentityState) set(id ExternalScannerCheckpointIdentity) bool {
	if s == nil || s.conflict || !id.complete() {
		return false
	}
	if s.valid {
		if s.matches(id) {
			return true
		}
		s.conflict = true
		return false
	}
	copy(s.scanner[:], id.Scanner)
	copy(s.grammar[:], id.Grammar)
	s.scannerLen = uint16(len(id.Scanner))
	s.grammarLen = uint16(len(id.Grammar))
	s.valid = true
	return true
}

func (s *externalScannerCheckpointIdentityState) inherit(source *externalScannerCheckpointIdentityState) bool {
	if s == nil {
		return false
	}
	if source == nil {
		return true
	}
	if s.conflict {
		return false
	}
	if source.conflict {
		s.conflict = true
		return false
	}
	if !source.valid {
		return true
	}
	if !s.valid {
		copy(s.scanner[:], source.scanner[:source.scannerLen])
		copy(s.grammar[:], source.grammar[:source.grammarLen])
		s.scannerLen = source.scannerLen
		s.grammarLen = source.grammarLen
		s.valid = true
		return true
	}
	if s.scannerLen == source.scannerLen && s.grammarLen == source.grammarLen &&
		bytes.Equal(s.scanner[:s.scannerLen], source.scanner[:source.scannerLen]) &&
		bytes.Equal(s.grammar[:s.grammarLen], source.grammar[:source.grammarLen]) {
		return true
	}
	s.conflict = true
	return false
}

func (s *externalScannerCheckpointIdentityState) matches(id ExternalScannerCheckpointIdentity) bool {
	return s != nil && s.valid && !s.conflict && id.complete() &&
		s.scannerLen == uint16(len(id.Scanner)) && s.grammarLen == uint16(len(id.Grammar)) &&
		bytes.Equal(s.scanner[:s.scannerLen], id.Scanner) &&
		bytes.Equal(s.grammar[:s.grammarLen], id.Grammar)
}

func (s *externalScannerCheckpointIdentityState) clear() {
	if s == nil {
		return
	}
	*s = externalScannerCheckpointIdentityState{}
}

func (s *externalScannerCheckpointIdentityState) bytesAllocated() int64 {
	if s == nil || !s.valid || s.conflict {
		return 0
	}
	return int64(s.scannerLen) + int64(s.grammarLen)
}

// externalScannerCheckpointIdentityStatus reports whether a language requires
// identity-aware checkpoint reuse and whether its current identity is valid.
// A scanner without this provider keeps the legacy reuse behavior.
func externalScannerCheckpointIdentityStatus(lang *Language) (ExternalScannerCheckpointIdentity, bool, bool) {
	if lang == nil || lang.ExternalScanner == nil {
		return ExternalScannerCheckpointIdentity{}, false, true
	}
	provider, required := externalScannerCheckpointIdentityProviderForScanner(lang.ExternalScanner)
	if !required {
		return ExternalScannerCheckpointIdentity{}, false, true
	}
	identity, ok := provider.CheckpointIdentity()
	return identity, true, ok && identity.complete()
}

func (a *nodeArena) setExternalScannerCheckpointIdentityForLanguage(lang *Language) bool {
	if a == nil {
		return false
	}
	identity, required, valid := externalScannerCheckpointIdentityStatus(lang)
	if !required || !valid {
		return false
	}
	before := a.externalScannerCheckpointIdentity.bytesAllocated()
	if !a.externalScannerCheckpointIdentity.set(identity) {
		a.allocatedBytes += a.externalScannerCheckpointIdentity.bytesAllocated() - before
		return false
	}
	a.allocatedBytes += a.externalScannerCheckpointIdentity.bytesAllocated() - before
	return true
}

func (a *nodeArena) inheritExternalScannerCheckpointIdentity(source *nodeArena) bool {
	if a == nil || source == nil {
		return false
	}
	before := a.externalScannerCheckpointIdentity.bytesAllocated()
	ok := a.externalScannerCheckpointIdentity.inherit(&source.externalScannerCheckpointIdentity)
	a.allocatedBytes += a.externalScannerCheckpointIdentity.bytesAllocated() - before
	return ok
}

func (a *nodeArena) externalScannerCheckpointIdentityMatches(lang *Language) bool {
	identity, required, valid := externalScannerCheckpointIdentityStatus(lang)
	if !required {
		return true
	}
	return valid && a != nil && a.externalScannerCheckpointIdentity.matches(identity)
}

// externalScannerLeafCheckpointIdentityMatches also rejects capability loss.
// An identity-bearing old arena cannot become a legacy checkpoint source only
// because the current language no longer exposes its identity provider.
func (a *nodeArena) externalScannerLeafCheckpointIdentityMatches(lang *Language) bool {
	if a == nil {
		return false
	}
	identity, required, valid := externalScannerCheckpointIdentityStatus(lang)
	if a.externalScannerCheckpointIdentity.valid || a.externalScannerCheckpointIdentity.conflict {
		return required && valid && a.externalScannerCheckpointIdentity.matches(identity)
	}
	if !required {
		return true
	}
	return valid && a.externalScannerCheckpointIdentity.matches(identity)
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
	provider, ok := externalScannerCheckpointIdentityProviderForScanner(scanner)
	if !ok {
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
	provider, ok := externalScannerCheckpointIdentityProviderForScanner(scanner)
	if !ok {
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
