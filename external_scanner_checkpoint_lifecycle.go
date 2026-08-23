package gotreesitter

// externalScannerCheckpointVersionKey identifies one parser version inside a
// single checkpoint lifecycle. It is not a parser stack field.
type externalScannerCheckpointVersionKey struct {
	parseEpoch uint64
	versionID  uint64
}

// externalScannerCheckpointVersion keeps scanner ownership beside, rather
// than inside, a parser stack. The lifecycle is opt-in and has no production
// parser caller yet.
type externalScannerCheckpointVersion struct {
	key        externalScannerCheckpointVersionKey
	payload    any
	checkpoint externalScannerCheckpointRecord
	alive      bool
}

// externalScannerCheckpointLifecycle is a bounded ownership ledger for the
// synthetic parser lifecycle proof. It does not wire scanner state into GLR.
type externalScannerCheckpointLifecycle struct {
	scanner  ExternalScanner
	epoch    uint64
	nextID   uint64
	versions map[externalScannerCheckpointVersionKey]*externalScannerCheckpointVersion
}

// newExternalScannerCheckpointLifecycle requires an explicit opt-in. A
// missing or disabled capability returns no lifecycle and no ledger.
func newExternalScannerCheckpointLifecycle(scanner ExternalScanner, enabled bool) (*externalScannerCheckpointLifecycle, bool) {
	if !enabled || scanner == nil {
		return nil, false
	}
	provider, ok := scanner.(ExternalScannerCheckpointIdentityProvider)
	if !ok || !provider.UsesExternalScannerCheckpoints() {
		return nil, false
	}
	return &externalScannerCheckpointLifecycle{
		scanner:  scanner,
		epoch:    1,
		nextID:   1,
		versions: make(map[externalScannerCheckpointVersionKey]*externalScannerCheckpointVersion),
	}, true
}

func (l *externalScannerCheckpointLifecycle) nextVersionKey() (externalScannerCheckpointVersionKey, bool) {
	if l == nil || l.epoch == 0 || l.nextID == 0 {
		return externalScannerCheckpointVersionKey{}, false
	}
	key := externalScannerCheckpointVersionKey{
		parseEpoch: l.epoch,
		versionID:  l.nextID,
	}
	if _, exists := l.versions[key]; exists {
		return externalScannerCheckpointVersionKey{}, false
	}
	if l.nextID == ^uint64(0) {
		l.nextID = 0
	} else {
		l.nextID++
	}
	return key, true
}

func (l *externalScannerCheckpointLifecycle) close() {
	if l == nil {
		return
	}
	for _, version := range l.versions {
		if version != nil && version.payload != nil {
			l.scanner.Destroy(version.payload)
			version.payload = nil
		}
		if version != nil {
			version.alive = false
		}
	}
	clear(l.versions)
}

func (l *externalScannerCheckpointLifecycle) version(key externalScannerCheckpointVersionKey) (*externalScannerCheckpointVersion, bool) {
	if l == nil || l.versions == nil {
		return nil, false
	}
	version, ok := l.versions[key]
	if !ok || version == nil || !version.alive || !version.checkpoint.complete() {
		return nil, false
	}
	return version, true
}

func (l *externalScannerCheckpointLifecycle) addRoot(
	payload any,
	sourceByte uint32,
	sourcePoint Point,
	externalLexState uint16,
	tokenStartByte uint32,
	tokenEndByte uint32,
) (externalScannerCheckpointVersionKey, bool) {
	if l == nil {
		return externalScannerCheckpointVersionKey{}, false
	}
	record, ok := captureExternalScannerCheckpointRecord(
		l.scanner,
		payload,
		sourceByte,
		sourcePoint,
		externalLexState,
		tokenStartByte,
		tokenEndByte,
	)
	if !ok {
		return externalScannerCheckpointVersionKey{}, false
	}
	key, ok := l.nextVersionKey()
	if !ok {
		if payload != nil {
			l.scanner.Destroy(payload)
		}
		return externalScannerCheckpointVersionKey{}, false
	}
	l.versions[key] = &externalScannerCheckpointVersion{
		key:        key,
		payload:    payload,
		checkpoint: record,
		alive:      true,
	}
	return key, true
}

// restoreAndVerify restores the owned bytes, then captures them again. The
// second capture proves that Deserialize accepted the complete checkpoint.
func (l *externalScannerCheckpointLifecycle) restoreAndVerify(version *externalScannerCheckpointVersion) bool {
	if l == nil || version == nil || !version.alive || !version.checkpoint.complete() {
		return false
	}
	if !version.checkpoint.restore(l.scanner, version.payload) {
		return false
	}
	restored, ok := captureExternalScannerCheckpointRecord(
		l.scanner,
		version.payload,
		version.checkpoint.sourceByte,
		version.checkpoint.sourcePoint,
		version.checkpoint.externalLexState,
		version.checkpoint.tokenStartByte,
		version.checkpoint.tokenEndByte,
	)
	return ok && restored.equal(version.checkpoint)
}

// discardVersion destroys a payload after failed restore verification. It
// also removes an owned version from the lifecycle ledger.
func (l *externalScannerCheckpointLifecycle) discardVersion(version *externalScannerCheckpointVersion) {
	if version == nil {
		return
	}
	if l != nil && l.versions != nil {
		if current, ok := l.versions[version.key]; ok && current == version {
			version.alive = false
			_ = l.deleteDead(version.key)
			return
		}
	}
	if version.payload != nil {
		if l != nil && l.scanner != nil {
			l.scanner.Destroy(version.payload)
		}
		version.payload = nil
	}
	version.alive = false
}

// elect runs one synthetic token election. A failed scan restores the prior
// checkpoint and keeps the version only after verified restoration.
func (l *externalScannerCheckpointLifecycle) elect(
	key externalScannerCheckpointVersionKey,
	sourceByte uint32,
	sourcePoint Point,
	externalLexState uint16,
	scan func(any) (Token, bool),
) (Token, bool) {
	version, ok := l.version(key)
	if !ok || scan == nil {
		return Token{}, false
	}
	before, ok := captureExternalScannerCheckpointRecord(
		l.scanner,
		version.payload,
		sourceByte,
		sourcePoint,
		externalLexState,
		sourceByte,
		sourceByte,
	)
	if !ok {
		return Token{}, false
	}
	tok, ok := scan(version.payload)
	if !ok {
		if !before.restore(l.scanner, version.payload) {
			l.discardVersion(version)
			return Token{}, false
		}
		if restored, restoreOK := captureExternalScannerCheckpointRecord(
			l.scanner,
			version.payload,
			before.sourceByte,
			before.sourcePoint,
			before.externalLexState,
			before.tokenStartByte,
			before.tokenEndByte,
		); !restoreOK || !restored.equal(before) {
			l.discardVersion(version)
			return Token{}, false
		}
		return Token{}, false
	}
	after, ok := captureExternalScannerCheckpointRecord(
		l.scanner,
		version.payload,
		sourceByte,
		sourcePoint,
		externalLexState,
		tok.StartByte,
		tok.EndByte,
	)
	if !ok {
		if !before.restore(l.scanner, version.payload) {
			l.discardVersion(version)
			return Token{}, false
		}
		restored, restoreOK := captureExternalScannerCheckpointRecord(
			l.scanner,
			version.payload,
			before.sourceByte,
			before.sourcePoint,
			before.externalLexState,
			before.tokenStartByte,
			before.tokenEndByte,
		)
		if !restoreOK || !restored.equal(before) {
			l.discardVersion(version)
		}
		return Token{}, false
	}
	version.checkpoint = after
	return tok, true
}

func (l *externalScannerCheckpointLifecycle) fork(parentKey externalScannerCheckpointVersionKey) (externalScannerCheckpointVersionKey, bool) {
	parent, ok := l.version(parentKey)
	if !ok {
		return externalScannerCheckpointVersionKey{}, false
	}
	childPayload := l.scanner.Create()
	child := &externalScannerCheckpointVersion{
		payload:    childPayload,
		checkpoint: parent.checkpoint.clone(),
		alive:      true,
	}
	if !l.restoreAndVerify(child) {
		l.discardVersion(child)
		return externalScannerCheckpointVersionKey{}, false
	}
	key, ok := l.nextVersionKey()
	if !ok {
		l.discardVersion(child)
		return externalScannerCheckpointVersionKey{}, false
	}
	child.key = key
	l.versions[key] = child
	return key, true
}

func (l *externalScannerCheckpointLifecycle) canShare(aKey, bKey externalScannerCheckpointVersionKey) bool {
	a, aOK := l.version(aKey)
	b, bOK := l.version(bKey)
	if !aOK || !bOK {
		return false
	}
	if !l.currentStateMatches(a) {
		l.discardVersion(a)
		return false
	}
	if !l.currentStateMatches(b) {
		l.discardVersion(b)
		return false
	}
	return canShareExternalScannerCheckpoint(a.checkpoint, b.checkpoint)
}

func (l *externalScannerCheckpointLifecycle) currentStateMatches(version *externalScannerCheckpointVersion) bool {
	if l == nil || version == nil || !version.alive || !version.checkpoint.complete() {
		return false
	}
	record, ok := captureExternalScannerCheckpointRecord(
		l.scanner,
		version.payload,
		version.checkpoint.sourceByte,
		version.checkpoint.sourcePoint,
		version.checkpoint.externalLexState,
		version.checkpoint.tokenStartByte,
		version.checkpoint.tokenEndByte,
	)
	return ok && record.equal(version.checkpoint)
}

// merge removes the candidate only when its complete record equals the keep
// record. A mismatch leaves both versions alive for a safe fallback.
func (l *externalScannerCheckpointLifecycle) merge(keepKey, candidateKey externalScannerCheckpointVersionKey) bool {
	if keepKey == candidateKey || !l.canShare(keepKey, candidateKey) {
		return false
	}
	return l.markDeadAndDelete(candidateKey)
}

// condense selects one version and deletes only dead or exact-record siblings.
// It rejects a live sibling with a different scanner state.
func (l *externalScannerCheckpointLifecycle) condense(selectedKey externalScannerCheckpointVersionKey, siblingKeys []externalScannerCheckpointVersionKey) (any, bool) {
	selected, ok := l.version(selectedKey)
	if !ok || !l.restoreAndVerify(selected) {
		if ok {
			l.discardVersion(selected)
		}
		return nil, false
	}
	for _, siblingKey := range siblingKeys {
		if siblingKey == selectedKey {
			continue
		}
		sibling, siblingOK := l.version(siblingKey)
		if !siblingOK {
			continue
		}
		if sibling.alive && !l.canShare(selectedKey, siblingKey) {
			return nil, false
		}
	}
	for _, siblingKey := range siblingKeys {
		if siblingKey == selectedKey {
			continue
		}
		if sibling, siblingOK := l.version(siblingKey); siblingOK {
			sibling.alive = false
			_ = l.deleteDead(siblingKey)
		}
	}
	return selected.payload, true
}

// resume restores the selected version before the recovery callback. A failed
// callback keeps the version only after verified restoration.
func (l *externalScannerCheckpointLifecycle) resume(
	key externalScannerCheckpointVersionKey,
	sourceByte uint32,
	sourcePoint Point,
	externalLexState uint16,
	recover func(any) (Token, bool),
) (Token, bool) {
	version, ok := l.version(key)
	if !ok || recover == nil {
		return Token{}, false
	}
	if !l.restoreAndVerify(version) {
		l.discardVersion(version)
		return Token{}, false
	}
	before := version.checkpoint.clone()
	tok, ok := recover(version.payload)
	if !ok {
		if !before.restore(l.scanner, version.payload) {
			l.discardVersion(version)
			return Token{}, false
		}
		if !l.restoreAndVerify(version) {
			l.discardVersion(version)
		}
		return Token{}, false
	}
	after, ok := captureExternalScannerCheckpointRecord(
		l.scanner,
		version.payload,
		sourceByte,
		sourcePoint,
		externalLexState,
		tok.StartByte,
		tok.EndByte,
	)
	if !ok {
		if !before.restore(l.scanner, version.payload) {
			l.discardVersion(version)
			return Token{}, false
		}
		restored, restoreOK := captureExternalScannerCheckpointRecord(
			l.scanner,
			version.payload,
			before.sourceByte,
			before.sourcePoint,
			before.externalLexState,
			before.tokenStartByte,
			before.tokenEndByte,
		)
		if !restoreOK || !restored.equal(before) {
			l.discardVersion(version)
		}
		return Token{}, false
	}
	version.checkpoint = after
	return tok, true
}

func (l *externalScannerCheckpointLifecycle) markDead(key externalScannerCheckpointVersionKey) bool {
	version, ok := l.version(key)
	if !ok {
		return false
	}
	version.alive = false
	return true
}

func (l *externalScannerCheckpointLifecycle) deleteDead(key externalScannerCheckpointVersionKey) bool {
	if l == nil {
		return false
	}
	version, ok := l.versions[key]
	if !ok || version == nil || version.alive {
		return false
	}
	if version.payload != nil {
		payload := version.payload
		version.payload = nil
		l.scanner.Destroy(payload)
	}
	delete(l.versions, key)
	return true
}

func (l *externalScannerCheckpointLifecycle) markDeadAndDelete(key externalScannerCheckpointVersionKey) bool {
	version, ok := l.versions[key]
	if !ok || version == nil || !version.alive {
		return false
	}
	version.alive = false
	return l.deleteDead(key)
}
