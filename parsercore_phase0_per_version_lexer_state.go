//go:build !gts_no_parsercorephase0

package gotreesitter

import (
	"bytes"
	"errors"
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
	storage        *core.ScannerSnapshotStorage
	language       *Language
	metadata       *core.FrozenValue[diagnosticParserCoreVersionLexerMetadata]
	dfa            dfaRelexSnapshot
	triedMask      core.ScannerTrialMask

	beforeCheckpoint      core.CheckpointID
	afterCheckpoint       core.CheckpointID
	beforeCheckpointBytes []byte
	afterCheckpointBytes  []byte
	beforeCheckpointInfo  DiagnosticParserCoreScannerCheckpoint
	afterCheckpointInfo   DiagnosticParserCoreScannerCheckpoint
}

// diagnosticParserCoreVersionLexerMetadata is shared only as an immutable
// value. Each snapshot keeps its core generation and mutable cursor private.
type diagnosticParserCoreVersionLexerMetadata struct {
	scanner                    diagnosticParserCoreVersionLexerScannerContract
	checkpointIdentity         [32]byte
	checkpointIdentityRequired bool
	checkpointIdentityValid    bool
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
	return core.ScannerFingerprint(scanner)
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

// diagnosticParserCoreVersionLexerSnapshotEqual compares the complete
// immutable identity of two owned lexer snapshots. Identical immutable objects
// can short-circuit; distinct rollback copies still compare their complete values.
func diagnosticParserCoreVersionLexerSnapshotEqual(
	left, right *diagnosticParserCoreVersionLexerSnapshot,
) bool {
	if left == right {
		return true
	}
	if left == nil || right == nil {
		return false
	}
	return left.compact == right.compact &&
		left.coreGeneration == right.coreGeneration &&
		left.language == right.language &&
		(left.metadata == right.metadata || left.metadata.Value() == right.metadata.Value()) &&
		left.dfa.equal(&right.dfa) &&
		(left.dfa.extZeroPos != left.dfa.lexerPos || left.triedMask == right.triedMask) &&
		left.beforeCheckpoint == right.beforeCheckpoint &&
		left.afterCheckpoint == right.afterCheckpoint &&
		bytes.Equal(left.beforeCheckpointBytes, right.beforeCheckpointBytes) &&
		bytes.Equal(left.afterCheckpointBytes, right.afterCheckpointBytes) &&
		left.beforeCheckpointInfo == right.beforeCheckpointInfo &&
		left.afterCheckpointInfo == right.afterCheckpointInfo
}

// diagnosticParserCoreVersionLexerRequestEqual compares lexer request state,
// excluding scheduler lifecycle provenance such as election and creation IDs.
//
// state (the parser state the request was originally validated against) is
// deliberately excluded from this comparison. Two owned headers can each
// reduce their own way to the identical (token, scanner snapshot) pair
// through different, independently-reduced chains of parser states -- an
// ordinary GLR convergence, not a bug -- while their own cached requests
// keep whatever state each header happened to hold when it first issued the
// request, since a pending request survives unchanged across a header's own
// later reduce-only dispatches (a reduction never re-requests the still
// unconsumed token). Comparing that frozen field would report two requests
// as different solely because they were issued at different moments in each
// header's own history, even when the token and every scanner snapshot they
// captured are byte-for-byte identical -- the only two headers this
// equality check exists to identify as mergeable. state cannot mask a real
// difference here regardless: any parser state that would have driven the
// scanner to a genuinely different result already shows up in token or in
// before/after, both still compared below.
func diagnosticParserCoreVersionLexerRequestEqual(
	left, right *diagnosticParserCoreVersionLexerRequest,
) bool {
	if left == nil || right == nil {
		return left == right
	}
	return tokensSameLex(left.token, right.token) &&
		diagnosticParserCoreVersionLexerSnapshotEqual(left.before, right.before) &&
		left.beforeCheckpoint == right.beforeCheckpoint &&
		diagnosticParserCoreVersionLexerSnapshotEqual(left.after, right.after) &&
		left.afterCheckpoint == right.afterCheckpoint &&
		left.beforeID == right.beforeID &&
		left.afterID == right.afterID &&
		left.raggedSpan == right.raggedSpan &&
		left.valid == right.valid
}

// versionLexerStateEqual compares scheduler ownership. Snapshot pointers and
// recovery regions identify graph owners. Resolve request references only
// after the owner identities match.
func (s *diagnosticParserCoreGenericScheduler) versionLexerStateEqual(
	left, right *diagnosticParserCoreVersionState,
) bool {
	if left == nil || right == nil {
		return left == right
	}
	if left.relexSnapshot != right.relexSnapshot ||
		left.recoveryNodeBaseline != right.recoveryNodeBaseline || left.recoveryNodeBaselineSet != right.recoveryNodeBaselineSet {
		return false
	}
	if left.recovery != right.recovery {
		a, b := left.recovery.Value(), right.recovery.Value()
		if a.Region != b.Region || a.RecoveryGroup != b.RecoveryGroup || a.MissingGroup != b.MissingGroup {
			return false
		}
		// Preserve effective closed-group identity and the active-key fallback.
		if a.AcceptanceGroup != b.AcceptanceGroup &&
			recoveredRootGroupOf(diagnosticParserCoreHeader{versionState: left}) != recoveredRootGroupOf(diagnosticParserCoreHeader{versionState: right}) {
			return false
		}
	}

	if left.lexerRequest == 0 || right.lexerRequest == 0 {
		return left.lexerRequest == right.lexerRequest
	}
	if s == nil || int(left.lexerRequest) > len(s.versionLexerRequests) ||
		int(right.lexerRequest) > len(s.versionLexerRequests) {
		return false
	}
	return diagnosticParserCoreVersionLexerRequestEqual(
		&s.versionLexerRequests[left.lexerRequest-1],
		&s.versionLexerRequests[right.lexerRequest-1],
	)
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
	var out *diagnosticParserCoreVersionLexerSnapshot
	var payload, start, end []byte
	var tried []bool
	if s.triedMask.Len() != 0 {
		out, payload, start, end = core.NewPackedScannerSnapshotRecord[diagnosticParserCoreVersionLexerSnapshot](s.dfa.externalPayload, s.dfa.externalTokenStart, s.dfa.externalTokenEnd)
	} else {
		out, payload, start, end, tried = core.NewScannerSnapshotRecord[diagnosticParserCoreVersionLexerSnapshot](s.dfa.externalPayload, s.dfa.externalTokenStart, s.dfa.externalTokenEnd, s.dfa.extZeroTried)
	}
	*out = *s
	out.storage = nil
	out.dfa.externalPayload, out.dfa.externalTokenStart, out.dfa.externalTokenEnd = payload, start, end
	out.dfa.extZeroTried = tried
	out.beforeCheckpointBytes = cloneBytesForDiagnosticParserCoreVersion(s.beforeCheckpointBytes)
	out.afterCheckpointBytes = cloneBytesForDiagnosticParserCoreVersion(s.afterCheckpointBytes)
	return out
}

// copyDiagnosticParserCoreVersionCheckpoint obtains a complete private copy
// of one compact checkpoint and verifies its length and digest metadata.
func copyDiagnosticParserCoreVersionCheckpoint(
	compact *core.Core,
	owner core.SchedulerTransactionToken,
	id core.CheckpointID,
) ([]byte, DiagnosticParserCoreScannerCheckpoint, error) {
	return copyDiagnosticParserCoreVersionCheckpointWithPrivateCopy(compact, owner, id, nil)
}

func copyDiagnosticParserCoreVersionCheckpointWithPrivateCopy(
	compact *core.Core,
	owner core.SchedulerTransactionToken,
	id core.CheckpointID,
	private []byte,
) ([]byte, DiagnosticParserCoreScannerCheckpoint, error) {
	if compact == nil {
		return nil, DiagnosticParserCoreScannerCheckpoint{}, errors.New("parser-core phase zero: version lexer checkpoint requires a core")
	}
	copy, ok := compact.PrivateCheckpointCopyOwned(owner, id, private)
	if !ok {
		return nil, DiagnosticParserCoreScannerCheckpoint{}, errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	return copy.Bytes, DiagnosticParserCoreScannerCheckpoint{Length: int(copy.Length), SHA256: copy.SHA256}, nil
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
	return newDiagnosticParserCoreVersionLexerSnapshotWithMetadata(compact, language, owner, dfa, beforeCheckpoint, afterCheckpoint, nil)
}

func newDiagnosticParserCoreVersionLexerSnapshotWithMetadata(
	compact *core.Core, language *Language, owner core.SchedulerTransactionToken,
	dfa dfaRelexSnapshot, beforeCheckpoint, afterCheckpoint core.CheckpointID,
	previous *core.FrozenValue[diagnosticParserCoreVersionLexerMetadata],
) (*diagnosticParserCoreVersionLexerSnapshot, error) {
	return newDiagnosticParserCoreVersionLexerSnapshotWithMetadataAndPages(compact, language, owner, dfa, beforeCheckpoint, afterCheckpoint, previous, nil)
}

func newDiagnosticParserCoreVersionLexerSnapshotWithMetadataAndPages(
	compact *core.Core, language *Language, owner core.SchedulerTransactionToken,
	dfa dfaRelexSnapshot, beforeCheckpoint, afterCheckpoint core.CheckpointID,
	previous *core.FrozenValue[diagnosticParserCoreVersionLexerMetadata],
	pages *core.ScannerSnapshotPages[diagnosticParserCoreVersionLexerSnapshot],
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
	// Authenticate both handles before allocating the private DFA copy.
	if compact == nil {
		return nil, errors.New("parser-core phase zero: version lexer checkpoint requires a core")
	}
	if _, _, ok := compact.CheckpointReceiptOwned(owner, beforeCheckpoint); !ok {
		return nil, errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if _, _, ok := compact.CheckpointReceiptOwned(owner, afterCheckpoint); !ok {
		return nil, errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	mask, packed := core.PackScannerTrialMask(dfa.extZeroTried)
	var out *diagnosticParserCoreVersionLexerSnapshot
	var payload, start, end []byte
	var tried []bool
	var storage *core.ScannerSnapshotStorage
	if packed {
		out, payload, start, end, storage = pages.NewPacked(dfa.externalPayload, dfa.externalTokenStart, dfa.externalTokenEnd)
	} else {
		out, payload, start, end, tried, storage = pages.New(dfa.externalPayload, dfa.externalTokenStart, dfa.externalTokenEnd, dfa.extZeroTried)
	}
	dfa.externalPayload, dfa.externalTokenStart, dfa.externalTokenEnd = payload, start, end
	dfa.extZeroTried = tried
	afterBytes, afterInfo, err := copyDiagnosticParserCoreVersionCheckpointWithPrivateCopy(compact, owner, afterCheckpoint, dfa.externalPayload)
	if err != nil {
		return nil, err
	}
	beforeBytes, beforeInfo, err := copyDiagnosticParserCoreVersionCheckpointWithPrivateCopy(compact, owner, beforeCheckpoint, afterBytes)
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
	*out = diagnosticParserCoreVersionLexerSnapshot{
		storage:        storage,
		compact:        compact,
		coreGeneration: generation,
		language:       language,
		metadata: core.ReuseFrozenValue(previous, diagnosticParserCoreVersionLexerMetadata{
			scanner: scanner, checkpointIdentity: checkpointIdentity,
			checkpointIdentityRequired: checkpointIdentityRequired, checkpointIdentityValid: checkpointIdentityValid,
		}),
		dfa:                   dfa,
		triedMask:             mask,
		beforeCheckpoint:      beforeCheckpoint,
		afterCheckpoint:       afterCheckpoint,
		beforeCheckpointBytes: beforeBytes,
		afterCheckpointBytes:  afterBytes,
		beforeCheckpointInfo:  beforeInfo,
		afterCheckpointInfo:   afterInfo,
	}
	return out, nil
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
	binding := language.ExternalScanner
	metadata := s.metadata.Value()
	scanner, err := diagnosticParserCoreVersionLexerScannerContractForLanguage(language)
	if err != nil {
		return err
	}
	if !metadata.scanner.equal(scanner) {
		return errors.New("parser-core phase zero: version lexer snapshot scanner contract changed")
	}
	identity, identityRequired, identityValid := diagnosticParserCoreVersionLexerCheckpointIdentity(language)
	// Capability methods are user callbacks. Reject a changed live descriptor
	// before publication or restore; keep every existing contract and identity
	// check authoritative for equal value bindings.
	if !core.ScannerBindingUnchanged(binding, language.ExternalScanner) {
		return errors.New("parser-core phase zero: version lexer snapshot scanner binding changed during capability validation")
	}
	if metadata.checkpointIdentityRequired != identityRequired {
		return errors.New("parser-core phase zero: version lexer snapshot checkpoint capability changed")
	}
	if metadata.checkpointIdentityRequired &&
		(!metadata.checkpointIdentityValid || !identityValid || identity != metadata.checkpointIdentity) {
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
	info *DiagnosticParserCoreScannerCheckpoint,
) error {
	if id == 0 {
		if len(bytes) != 0 || *info != parserCoreEmptyCheckpoint {
			return errors.New("parser-core phase zero: version lexer empty checkpoint metadata changed")
		}
		return nil
	}
	if compact == nil {
		return errors.New("parser-core phase zero: version lexer checkpoint has no core")
	}
	if info.Length != len(bytes) {
		return errors.New("parser-core phase zero: version lexer checkpoint metadata does not match its bytes")
	}
	match := compact.MatchCheckpointPrivateCopy(id, bytes, &info.SHA256)
	if match == core.CheckpointCopyUnknown {
		return errDiagnosticParserCoreUnknownCheckpointIdentity
	}
	if match != core.CheckpointCopyExact {
		return errors.New("parser-core phase zero: version lexer checkpoint metadata does not match its bytes")
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
	if err := validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.beforeCheckpoint, s.beforeCheckpointBytes, &s.beforeCheckpointInfo); err != nil {
		return err
	}
	return validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.afterCheckpoint, s.afterCheckpointBytes, &s.afterCheckpointInfo)
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
	if d.hasExternalScanner && len(s.dfa.externalPayload) == 0 && !s.metadata.Value().scanner.stateless {
		return errors.New("parser-core phase zero: version lexer snapshot target scanner state is unrepresentable")
	}
	if len(s.dfa.externalPayload) > externalScannerSerializationBufferSize {
		return errors.New("parser-core phase zero: version lexer snapshot target scanner state exceeds its bound")
	}
	if err := validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.beforeCheckpoint, s.beforeCheckpointBytes, &s.beforeCheckpointInfo); err != nil {
		return err
	}
	if err := validateDiagnosticParserCoreVersionLexerCheckpoint(s.compact, s.afterCheckpoint, s.afterCheckpointBytes, &s.afterCheckpointInfo); err != nil {
		return err
	}
	s.dfa.restore(d)
	if s.triedMask.Len() != 0 {
		d.extZeroTried = s.triedMask.Restore(d.extZeroTried)
	}
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
	bytes := diagnosticParserCoreVersionLexerSnapshotFootprintWithoutStorage(snapshot)
	if snapshot != nil && snapshot.storage != nil {
		extra := snapshot.storage.FootprintBytes()
		if extra > math.MaxUint64-bytes {
			return math.MaxUint64
		}
		bytes += extra
	}
	return bytes
}

func diagnosticParserCoreVersionLexerSnapshotFootprintWithoutStorage(snapshot *diagnosticParserCoreVersionLexerSnapshot) uint64 {
	if snapshot == nil {
		return 0
	}
	// dfa is embedded in the snapshot record, so sizeof(snapshot) already
	// includes its scalar fields and slice headers.
	total := uint64(0)
	if snapshot.storage == nil {
		total = uint64(unsafe.Sizeof(*snapshot))
	}
	// Conservatively charge the shared holder once per snapshot. Sharing cannot
	// bypass the existing per-version memory budget.
	if snapshot.metadata != nil {
		total += uint64(unsafe.Sizeof(*snapshot.metadata))
	}

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
	if snapshot.storage == nil {
		add(cap(snapshot.dfa.externalPayload), 1)
		add(cap(snapshot.dfa.externalTokenStart), 1)
		add(cap(snapshot.dfa.externalTokenEnd), 1)
		add(cap(snapshot.dfa.extZeroTried), unsafe.Sizeof(bool(false)))
	}
	add(cap(snapshot.beforeCheckpointBytes), 1)
	add(cap(snapshot.afterCheckpointBytes), 1)
	return total
}

func diagnosticParserCoreVersionStateFootprintBytes(state *diagnosticParserCoreVersionState) uint64 {
	if state == nil {
		return 0
	}
	total := uint64(unsafe.Sizeof(*state))
	if state.recovery != nil {
		total += uint64(unsafe.Sizeof(*state.recovery))
	}
	regionBytes := diagnosticParserCoreVersionS3RegionFootprintBytes(state.recovery.Value().Region)
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
