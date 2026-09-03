//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"math"
	"reflect"
	"unsafe"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

// diagnosticParserCoreVersionLexerSnapshot is an owned, immutable copy of
// the DFA token-source state needed to retry the next lexer election for one
// parser version. The parser's shared token source remains mutable; this
// value gives one version an independent rollback point.
//
// The checkpoint IDs identify the compact-core records. The byte copies are
// owned here as well, so a reset, scratch reuse, or interner growth cannot
// change a published version's scanner state.
type diagnosticParserCoreVersionLexerSnapshot struct {
	// compact and coreGeneration bind this snapshot to the compact core arena
	// lifetime that authenticated its checkpoint identities. A reset invalidates
	// the snapshot, while frontier and checkpoint phases may advance safely.
	compact        *core.Core
	coreGeneration uint64
	language       *Language
	scanner        diagnosticParserCoreVersionLexerScannerContract
	// checkpointIdentity authenticates the scanner and grammar that produced
	// this snapshot when the scanner exposes the identity-bearing checkpoint
	// capability. Legacy checkpointed scanners keep this optional because
	// internal DFA ownership does not require a grammar identity.
	checkpointIdentity         [32]byte
	checkpointIdentityRequired bool
	checkpointIdentityValid    bool // Published snapshots set this exactly when required.
	dfa                        dfaRelexSnapshot

	beforeCheckpoint      core.CheckpointID
	afterCheckpoint       core.CheckpointID
	beforeCheckpointBytes []byte
	afterCheckpointBytes  []byte
	beforeCheckpointInfo  DiagnosticParserCoreScannerCheckpoint
	afterCheckpointInfo   DiagnosticParserCoreScannerCheckpoint
}

// diagnosticParserCoreVersionLexerScannerContract is comparable and remains
// valid while a token source is returned to and acquired from its pool. The
// language pointer binds the grammar, while the scanner type and fingerprint
// reject a scanner replacement on the same Language value.
type diagnosticParserCoreVersionLexerScannerContract struct {
	present         bool
	typeID          reflect.Type
	fingerprint     [32]byte
	usesCheckpoints bool
	stateless       bool
}

func diagnosticParserCoreVersionLexerScannerFingerprint(scanner ExternalScanner) [32]byte {
	if scanner == nil {
		return [32]byte{}
	}
	typeID := reflect.TypeOf(scanner)
	// Pointer scanners carry their identity in the pointer. Avoid hashing their
	// mutable pointee, because scanner state belongs in Create's payload.
	if value := reflect.ValueOf(scanner); value.IsValid() && value.Kind() == reflect.Pointer {
		return sha256.Sum256([]byte(fmt.Sprintf("%s@%x", typeID, value.Pointer())))
	}
	return sha256.Sum256([]byte(fmt.Sprintf("%T:%#v", scanner, scanner)))
}

func diagnosticParserCoreVersionLexerScannerContractForLanguage(language *Language) (diagnosticParserCoreVersionLexerScannerContract, error) {
	if language == nil {
		return diagnosticParserCoreVersionLexerScannerContract{}, errors.New("parser-core phase zero: version lexer snapshot requires a language")
	}
	contract := diagnosticParserCoreVersionLexerScannerContract{}
	if language.ExternalScanner == nil {
		return contract, nil
	}
	scanner := language.ExternalScanner
	value := reflect.ValueOf(scanner)
	if value.IsValid() && value.Kind() == reflect.Pointer && value.IsNil() {
		return diagnosticParserCoreVersionLexerScannerContract{}, errors.New("parser-core phase zero: version lexer snapshot rejects a nil external scanner")
	}
	contract.present = true
	contract.typeID = reflect.TypeOf(scanner)
	contract.fingerprint = diagnosticParserCoreVersionLexerScannerFingerprint(scanner)
	contract.usesCheckpoints = languageUsesExternalScannerCheckpoints(language)
	if stateless, ok := scanner.(StatelessExternalScanner); ok {
		contract.stateless = stateless.ExternalScannerIsStateless()
	}
	return contract, nil
}

// diagnosticParserCoreVersionLexerCheckpointIdentity reports the stable
// scanner-and-grammar identity for a checkpoint-capable language. The second
// result reports whether the language requires the identity capability. The
// third result reports whether the current identity is complete.
func diagnosticParserCoreVersionLexerCheckpointIdentity(language *Language) ([32]byte, bool, bool) {
	identity, required, valid := externalScannerCheckpointIdentityStatus(language)
	if !required {
		return [32]byte{}, false, false
	}
	if !valid {
		return [32]byte{}, true, false
	}
	return parserCoreExternalScannerIdentityFingerprint(identity), true, true
}

func (s diagnosticParserCoreVersionLexerScannerContract) equal(other diagnosticParserCoreVersionLexerScannerContract) bool {
	return s == other
}

// cloneBytesForDiagnosticParserCoreVersion copies a byte slice while
// retaining nil as nil. Published version state never aliases lexer scratch.
func cloneBytesForDiagnosticParserCoreVersion(src []byte) []byte {
	if src == nil {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func cloneBoolsForDiagnosticParserCoreVersion(src []bool) []bool {
	if src == nil {
		return nil
	}
	out := make([]bool, len(src))
	copy(out, src)
	return out
}

// cloneDiagnosticParserCoreDFARelexSnapshot makes every mutable slice in a
// DFA snapshot independent. Cursor, failure-origin, and zero-width scalars
// copy with the value assignment.
func cloneDiagnosticParserCoreDFARelexSnapshot(src dfaRelexSnapshot) dfaRelexSnapshot {
	src.externalPayload = cloneBytesForDiagnosticParserCoreVersion(src.externalPayload)
	src.externalTokenStart = cloneBytesForDiagnosticParserCoreVersion(src.externalTokenStart)
	src.externalTokenEnd = cloneBytesForDiagnosticParserCoreVersion(src.externalTokenEnd)
	src.extZeroTried = cloneBoolsForDiagnosticParserCoreVersion(src.extZeroTried)
	return src
}

func (s *diagnosticParserCoreVersionLexerSnapshot) clone() *diagnosticParserCoreVersionLexerSnapshot {
	if s == nil {
		return nil
	}
	out := *s
	out.dfa = cloneDiagnosticParserCoreDFARelexSnapshot(s.dfa)
	out.beforeCheckpointBytes = cloneBytesForDiagnosticParserCoreVersion(s.beforeCheckpointBytes)
	out.afterCheckpointBytes = cloneBytesForDiagnosticParserCoreVersion(s.afterCheckpointBytes)
	return &out
}

// copyDiagnosticParserCoreVersionCheckpoint obtains a complete private copy
// of one compact checkpoint and verifies its length and digest metadata.
func copyDiagnosticParserCoreVersionCheckpoint(
	compact *core.Core,
	owner core.SchedulerTransactionToken,
	id core.CheckpointID,
) ([]byte, DiagnosticParserCoreScannerCheckpoint, error) {
	if compact == nil {
		return nil, DiagnosticParserCoreScannerCheckpoint{}, errors.New("parser-core phase zero: version lexer checkpoint requires a core")
	}
	if id == 0 {
		// Checkpoint ID zero means that the scanner has no serialized state.
		// Authenticate the owner even when no bytes need copying. Keep its
		// metadata explicit so callers can distinguish it from a missing
		// non-zero checkpoint identity.
		if _, _, ok := compact.CheckpointReceiptOwned(owner, id); !ok {
			return nil, DiagnosticParserCoreScannerCheckpoint{}, errDiagnosticParserCoreUnknownCheckpointIdentity
		}
		return nil, parserCoreEmptyCheckpoint, nil
	}
	length, digest, ok := compact.CheckpointReceiptOwned(owner, id)
	if !ok {
		return nil, DiagnosticParserCoreScannerCheckpoint{}, errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	bytes, ok := compact.CopyCheckpointBytesOwned(owner, id, nil)
	if !ok || uint64(len(bytes)) != uint64(length) {
		return nil, DiagnosticParserCoreScannerCheckpoint{}, errors.New("parser-core phase zero: version lexer checkpoint bytes are unavailable")
	}
	if sha256.Sum256(bytes) != digest {
		return nil, DiagnosticParserCoreScannerCheckpoint{}, errors.New("parser-core phase zero: version lexer checkpoint digest does not match its bytes")
	}
	return bytes, DiagnosticParserCoreScannerCheckpoint{Length: int(length), SHA256: digest}, nil
}

// newDiagnosticParserCoreVersionLexerSnapshot creates an owned snapshot from
// a live DFA snapshot and two compact checkpoint identities. It is safe to
// call before or during a compact transaction because CopyCheckpointBytes is
// read-only and explicitly supports transactions.
func newDiagnosticParserCoreVersionLexerSnapshot(
	compact *core.Core,
	language *Language,
	owner core.SchedulerTransactionToken,
	dfa dfaRelexSnapshot,
	beforeCheckpoint core.CheckpointID,
	afterCheckpoint core.CheckpointID,
) (*diagnosticParserCoreVersionLexerSnapshot, error) {
	scanner, err := diagnosticParserCoreVersionLexerScannerContractForLanguage(language)
	if err != nil {
		return nil, err
	}
	if dfa.externalScannerPresent != scanner.present {
		return nil, errors.New("parser-core phase zero: version lexer snapshot external scanner presence changed")
	}
	if scanner.present && len(dfa.externalPayload) == 0 && !scanner.stateless {
		return nil, errors.New("parser-core phase zero: version lexer snapshot cannot represent external scanner state")
	}
	beforeBytes, beforeInfo, err := copyDiagnosticParserCoreVersionCheckpoint(compact, owner, beforeCheckpoint)
	if err != nil {
		return nil, err
	}
	afterBytes, afterInfo, err := copyDiagnosticParserCoreVersionCheckpoint(compact, owner, afterCheckpoint)
	if err != nil {
		return nil, err
	}
	generation := compact.ResetGeneration()
	if generation == 0 {
		return nil, errors.New("parser-core phase zero: version lexer snapshot requires an authenticated reset generation")
	}
	checkpointIdentity, checkpointIdentityRequired, checkpointIdentityValid := diagnosticParserCoreVersionLexerCheckpointIdentity(language)
	if checkpointIdentityRequired && !checkpointIdentityValid {
		return nil, errors.New("parser-core phase zero: version lexer snapshot requires a complete checkpoint identity")
	}
	return &diagnosticParserCoreVersionLexerSnapshot{
		compact:                    compact,
		coreGeneration:             generation,
		language:                   language,
		scanner:                    scanner,
		checkpointIdentity:         checkpointIdentity,
		checkpointIdentityRequired: checkpointIdentityRequired,
		checkpointIdentityValid:    checkpointIdentityValid,
		dfa:                        cloneDiagnosticParserCoreDFARelexSnapshot(dfa),
		beforeCheckpoint:           beforeCheckpoint,
		afterCheckpoint:            afterCheckpoint,
		beforeCheckpointBytes:      beforeBytes,
		afterCheckpointBytes:       afterBytes,
		beforeCheckpointInfo:       beforeInfo,
		afterCheckpointInfo:        afterInfo,
	}, nil
}

func (s *diagnosticParserCoreVersionLexerSnapshot) validateGeneration() error {
	if s == nil || s.compact == nil || s.coreGeneration == 0 {
		return errors.New("parser-core phase zero: version lexer snapshot has no authenticated reset generation")
	}
	if s.compact.ResetGeneration() != s.coreGeneration {
		return errors.New("parser-core phase zero: version lexer snapshot Core reset generation changed")
	}
	return nil
}

func (s *diagnosticParserCoreVersionLexerSnapshot) validateDestination(compact *core.Core, language *Language) error {
	if s == nil || compact == nil || language == nil {
		return errors.New("parser-core phase zero: version lexer snapshot destination identity is unavailable")
	}
	if s.compact != compact {
		return errors.New("parser-core phase zero: version lexer snapshot belongs to a different Core")
	}
	if s.language != language {
		return errors.New("parser-core phase zero: version lexer snapshot belongs to a different Language")
	}
	scanner, err := diagnosticParserCoreVersionLexerScannerContractForLanguage(language)
	if err != nil {
		return err
	}
	if !s.scanner.equal(scanner) {
		return errors.New("parser-core phase zero: version lexer snapshot scanner contract changed")
	}
	identity, identityRequired, identityValid := diagnosticParserCoreVersionLexerCheckpointIdentity(language)
	if s.checkpointIdentityRequired != identityRequired {
		return errors.New("parser-core phase zero: version lexer snapshot checkpoint capability changed")
	}
	if s.checkpointIdentityRequired &&
		(!s.checkpointIdentityValid || !identityValid || identity != s.checkpointIdentity) {
		return errors.New("parser-core phase zero: version lexer snapshot checkpoint identity changed")
	}
	if err := s.validateGeneration(); err != nil {
		return err
	}
	if s.dfa.externalScannerPresent != scanner.present {
		return errors.New("parser-core phase zero: version lexer snapshot external scanner presence changed")
	}
	if scanner.present && len(s.dfa.externalPayload) == 0 && !scanner.stateless {
		return errors.New("parser-core phase zero: version lexer snapshot cannot represent external scanner state")
	}
	return nil
}

func validateDiagnosticParserCoreVersionLexerCheckpoint(
	compact *core.Core,
	id core.CheckpointID,
	bytes []byte,
	info DiagnosticParserCoreScannerCheckpoint,
) error {
	if id == 0 {
		if len(bytes) != 0 || info != parserCoreEmptyCheckpoint {
			return errors.New("parser-core phase zero: version lexer empty checkpoint metadata changed")
		}
		return nil
	}
	if compact == nil {
		return errors.New("parser-core phase zero: version lexer checkpoint has no core")
	}
	if info != parserCoreCheckpoint(bytes) {
		return errors.New("parser-core phase zero: version lexer checkpoint metadata does not match its bytes")
	}
	length, digest, ok := compact.CheckpointReceipt(id)
	if !ok || uint64(len(bytes)) != uint64(length) || digest != info.SHA256 {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	return nil
}

// validate confirms the compact-core generation and the immutable checkpoint
// copies before a version can publish or restore its lexer state.
func (s *diagnosticParserCoreVersionLexerSnapshot) validate() error {
	if s == nil {
		return errors.New("parser-core phase zero: version lexer snapshot is nil")
	}
	if err := s.validateDestination(s.compact, s.language); err != nil {
		return err
	}
	if err := validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.beforeCheckpoint, s.beforeCheckpointBytes, s.beforeCheckpointInfo); err != nil {
		return err
	}
	return validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.afterCheckpoint, s.afterCheckpointBytes, s.afterCheckpointInfo)
}

func (h *diagnosticParserCoreHeader) installVersionLexerSnapshot(snapshot *diagnosticParserCoreVersionLexerSnapshot) {
	if h == nil {
		return
	}
	baseline, baselineSet := h.recoveryNodeBaseline()
	h.publishVersionState(
		h.recoveryRegion(), snapshot, 0,
		h.recoveryGroupIdentity(), h.recoveryMissingGroupIdentity(),
		baseline, baselineSet,
	)
}

// publishVersionLexerSnapshot installs a private copy of an owned snapshot
// while preserving any open recovery region on the same parser version.
func (h *diagnosticParserCoreHeader) publishVersionLexerSnapshot(
	compact *core.Core,
	language *Language,
	snapshot *diagnosticParserCoreVersionLexerSnapshot,
) error {
	if snapshot != nil {
		if err := snapshot.validateDestination(compact, language); err != nil {
			return err
		}
		snapshot = snapshot.clone()
	}
	h.installVersionLexerSnapshot(snapshot)
	return nil
}

// publishOwnedVersionLexerSnapshot transfers an already-owned snapshot into
// the header. The constructor creates all of its copies, so no second clone is
// needed on this publication path.
func (h *diagnosticParserCoreHeader) publishOwnedVersionLexerSnapshot(
	compact *core.Core,
	language *Language,
	snapshot *diagnosticParserCoreVersionLexerSnapshot,
) error {
	if snapshot != nil {
		if err := snapshot.validateDestination(compact, language); err != nil {
			return err
		}
	}
	h.installVersionLexerSnapshot(snapshot)
	return nil
}

// publishDFARelexSnapshot copies the live token-source state and checkpoint
// identities into this header's immutable version state.
func (h *diagnosticParserCoreHeader) publishDFARelexSnapshot(
	compact *core.Core,
	language *Language,
	owner core.SchedulerTransactionToken,
	dfa dfaRelexSnapshot,
	beforeCheckpoint core.CheckpointID,
	afterCheckpoint core.CheckpointID,
) error {
	snapshot, err := newDiagnosticParserCoreVersionLexerSnapshot(compact, language, owner, dfa, beforeCheckpoint, afterCheckpoint)
	if err != nil {
		return err
	}
	return h.publishOwnedVersionLexerSnapshot(compact, language, snapshot)
}

// clearVersionLexerSnapshot removes only the owned lexer state. An open
// recovery region remains attached to the header until its own close path.
func (h *diagnosticParserCoreHeader) clearVersionLexerSnapshot() {
	if h == nil {
		return
	}
	baseline, baselineSet := h.recoveryNodeBaseline()
	recoveryGroup := h.recoveryGroupIdentity()
	missingGroup := h.recoveryMissingGroupIdentity()
	if !h.isRecoveryLineage() && h.recoveryRegion() == nil {
		recoveryGroup, missingGroup = 0, 0
		baseline, baselineSet = 0, false
	}
	h.publishVersionState(
		h.recoveryRegion(), nil, 0,
		recoveryGroup, missingGroup,
		baseline, baselineSet,
	)
}

// restore applies the owned DFA state to a token source. The caller restores
// parser-state and frontier checkpoint fields separately.
func (s *diagnosticParserCoreVersionLexerSnapshot) restore(compact *core.Core, d *dfaTokenSource) error {
	if s == nil || d == nil || d.lexer == nil {
		return errors.New("parser-core phase zero: cannot restore a nil version lexer snapshot")
	}
	if err := s.validateDestination(compact, d.language); err != nil {
		return err
	}
	if d.hasExternalScanner != s.dfa.externalScannerPresent {
		return errors.New("parser-core phase zero: version lexer snapshot target scanner presence changed")
	}
	if d.hasExternalScanner && len(s.dfa.externalPayload) == 0 && !s.scanner.stateless {
		return errors.New("parser-core phase zero: version lexer snapshot target scanner state is unrepresentable")
	}
	if len(s.dfa.externalPayload) > externalScannerSerializationBufferSize {
		return errors.New("parser-core phase zero: version lexer snapshot target scanner state exceeds its bound")
	}
	if err := validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.beforeCheckpoint, s.beforeCheckpointBytes, s.beforeCheckpointInfo); err != nil {
		return err
	}
	if err := validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.afterCheckpoint, s.afterCheckpointBytes, s.afterCheckpointInfo); err != nil {
		return err
	}
	s.dfa.restore(d)
	return nil
}

func diagnosticParserCoreVersionS3RegionFootprintBytes(region *diagnosticParserCoreS3Region) uint64 {
	if region == nil {
		return 0
	}
	total := uint64(unsafe.Sizeof(*region))
	size := uint64(unsafe.Sizeof(core.SubtreeID(0)))
	if uint64(cap(region.children)) > math.MaxUint64/size {
		return math.MaxUint64
	}
	bytes := uint64(cap(region.children)) * size
	if math.MaxUint64-total < bytes {
		return math.MaxUint64
	}
	total += bytes
	return total
}

func diagnosticParserCoreVersionLexerSnapshotFootprintBytes(snapshot *diagnosticParserCoreVersionLexerSnapshot) uint64 {
	if snapshot == nil {
		return 0
	}
	// dfa is embedded in the snapshot record, so sizeof(snapshot) already
	// includes its scalar fields and slice headers.
	total := uint64(unsafe.Sizeof(*snapshot))
	add := func(count int, size uintptr) {
		if count <= 0 || size == 0 || total == math.MaxUint64 {
			return
		}
		if uint64(count) > math.MaxUint64/uint64(size) {
			total = math.MaxUint64
			return
		}
		bytes := uint64(count) * uint64(size)
		if math.MaxUint64-total < bytes {
			total = math.MaxUint64
			return
		}
		total += bytes
	}
	add(cap(snapshot.dfa.externalPayload), 1)
	add(cap(snapshot.dfa.externalTokenStart), 1)
	add(cap(snapshot.dfa.externalTokenEnd), 1)
	add(cap(snapshot.dfa.extZeroTried), unsafe.Sizeof(bool(false)))
	add(cap(snapshot.beforeCheckpointBytes), 1)
	add(cap(snapshot.afterCheckpointBytes), 1)
	return total
}

func diagnosticParserCoreVersionStateFootprintBytes(state *diagnosticParserCoreVersionState) uint64 {
	if state == nil {
		return 0
	}
	total := uint64(unsafe.Sizeof(*state))
	regionBytes := diagnosticParserCoreVersionS3RegionFootprintBytes(state.s3Region)
	snapshotBytes := diagnosticParserCoreVersionLexerSnapshotFootprintBytes(state.relexSnapshot)
	if math.MaxUint64-total < regionBytes {
		return math.MaxUint64
	}
	total += regionBytes
	if math.MaxUint64-total < snapshotBytes {
		return math.MaxUint64
	}
	return total + snapshotBytes
}
