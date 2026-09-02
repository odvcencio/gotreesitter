package gotreesitter

// externalScannerOrderAdapter adapts an external scanner from one language to
// another by remapping external token order and scanner result symbols.
//
// It is intended for parity scenarios where two languages share scanner logic
// but use different symbol IDs (or external symbol names/aliases).
type externalScannerOrderAdapter struct {
	inner                ExternalScanner
	targetToSource       []int // len == len(targetExt)
	sourceCount          int   // len(sourceExt), for sizing sourceValid in Scan
	sourceSymbolToIndex  map[Symbol]int
	sourceResultToTarget map[Symbol]Symbol
}

type externalScannerOrderAdapterPayload struct {
	inner       any
	sourceValid []bool
}

func externalSymbolName(lang *Language, sym Symbol) string {
	if lang == nil {
		return ""
	}
	if int(sym) >= 0 && int(sym) < len(lang.SymbolNames) {
		return lang.SymbolNames[sym]
	}
	return ""
}

func hasDuplicateExternalNames(lang *Language, externals []Symbol) bool {
	seen := make(map[string]bool, len(externals))
	for _, sym := range externals {
		name := externalSymbolName(lang, sym)
		if seen[name] {
			return true
		}
		seen[name] = true
	}
	return false
}

func (a *externalScannerOrderAdapter) Create() any {
	return &externalScannerOrderAdapterPayload{
		inner:       a.inner.Create(),
		sourceValid: make([]bool, a.sourceCount),
	}
}

func (a *externalScannerOrderAdapter) Destroy(payload any) {
	a.inner.Destroy(a.innerPayload(payload))
}

func (a *externalScannerOrderAdapter) Serialize(payload any, buf []byte) int {
	return a.inner.Serialize(a.innerPayload(payload), buf)
}

func (a *externalScannerOrderAdapter) Deserialize(payload any, buf []byte) {
	a.inner.Deserialize(a.innerPayload(payload), buf)
}

// Optional scanner capabilities describe the inner scanner's state model, not
// its external-symbol numbering. Preserve them across order adaptation so a
// generated language does not lose a valid reuse/checkpoint proof merely
// because its external token order differs from the embedded reference.
func (a *externalScannerOrderAdapter) SupportsIncrementalReuse() bool {
	reusable, ok := a.optionalInner().(IncrementalReuseExternalScanner)
	return ok && reusable.SupportsIncrementalReuse()
}

func (a *externalScannerOrderAdapter) UsesExternalScannerCheckpoints() bool {
	checkpointed, ok := a.optionalInner().(CheckpointedExternalScanner)
	return ok && checkpointed.UsesExternalScannerCheckpoints()
}

// CheckpointIdentity preserves the inner scanner identity across external
// symbol-order adaptation. The target language remains bound by the snapshot
// language pointer and parser table identity.
func (a *externalScannerOrderAdapter) CheckpointIdentity() (ExternalScannerCheckpointIdentity, bool) {
	provider, ok := a.optionalInner().(ExternalScannerCheckpointIdentityProvider)
	if !ok || !provider.UsesExternalScannerCheckpoints() {
		return ExternalScannerCheckpointIdentity{}, false
	}
	return provider.CheckpointIdentity()
}

func (a *externalScannerOrderAdapter) RequiresIncrementalPrefixFrontierProof() bool {
	prefixSensitive, ok := a.optionalInner().(IncrementalPrefixFrontierExternalScanner)
	return ok && prefixSensitive.RequiresIncrementalPrefixFrontierProof()
}

func (a *externalScannerOrderAdapter) AllowsIncrementalReuseWithoutCheckpoint() bool {
	checkpointless, ok := a.optionalInner().(CheckpointlessExternalScannerReuse)
	return ok && checkpointless.AllowsIncrementalReuseWithoutCheckpoint()
}

func (a *externalScannerOrderAdapter) ExternalScannerIsStateless() bool {
	stateless, ok := a.optionalInner().(StatelessExternalScanner)
	return ok && stateless.ExternalScannerIsStateless()
}

func (a *externalScannerOrderAdapter) PreservesStateOnScanFailure() bool {
	preserving, ok := a.optionalInner().(FailurePreservingExternalScanner)
	return ok && preserving.PreservesStateOnScanFailure()
}

func (a *externalScannerOrderAdapter) RetainsStateOnScanFailure() bool {
	retaining, ok := a.optionalInner().(FailureStateRetainingExternalScanner)
	return ok && retaining.RetainsStateOnScanFailure()
}

func (a *externalScannerOrderAdapter) optionalInner() ExternalScanner {
	if a == nil {
		return nil
	}
	return a.inner
}

func (a *externalScannerOrderAdapter) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if a == nil || a.inner == nil {
		return false
	}

	_, innerPayload, sourceValid := a.prepareScanPayload(payload)
	a.mapValidSymbols(validSymbols, sourceValid)
	ok := a.inner.Scan(innerPayload, lexer, sourceValid)
	if !ok {
		return false
	}

	if !a.mapResultSymbol(lexer, sourceValid) {
		if lexer != nil {
			lexer.resultSymbol = 0
			lexer.hasResult = false
		}
		return false
	}
	return true
}

func (a *externalScannerOrderAdapter) prepareScanPayload(payload any) (*externalScannerOrderAdapterPayload, any, []bool) {
	adapterPayload, _ := payload.(*externalScannerOrderAdapterPayload)
	if adapterPayload == nil {
		return nil, payload, make([]bool, a.sourceCount)
	}
	if cap(adapterPayload.sourceValid) < a.sourceCount {
		adapterPayload.sourceValid = make([]bool, a.sourceCount)
	}
	sourceValid := adapterPayload.sourceValid[:a.sourceCount]
	clear(sourceValid)
	return adapterPayload, adapterPayload.inner, sourceValid
}

func (a *externalScannerOrderAdapter) mapValidSymbols(validSymbols []bool, sourceValid []bool) {
	for targetIdx, isValid := range validSymbols {
		if !isValid || targetIdx < 0 || targetIdx >= len(a.targetToSource) {
			continue
		}
		sourceIdx := a.targetToSource[targetIdx]
		if sourceIdx >= 0 && sourceIdx < len(sourceValid) {
			sourceValid[sourceIdx] = true
		}
	}
}

func (a *externalScannerOrderAdapter) mapResultSymbol(lexer *ExternalLexer, sourceValid []bool) bool {
	if lexer == nil || !lexer.hasResult {
		return true
	}
	if mapped, exists := a.sourceResultToTarget[lexer.resultSymbol]; exists && a.sourceResultWasValid(lexer.resultSymbol, sourceValid) {
		lexer.resultSymbol = mapped
		return true
	}
	return false
}

func (a *externalScannerOrderAdapter) sourceResultWasValid(sym Symbol, sourceValid []bool) bool {
	if a == nil {
		return false
	}
	idx, ok := a.sourceSymbolToIndex[sym]
	return ok && idx >= 0 && idx < len(sourceValid) && sourceValid[idx]
}

func (a *externalScannerOrderAdapter) innerPayload(payload any) any {
	if adapterPayload, ok := payload.(*externalScannerOrderAdapterPayload); ok {
		return adapterPayload.inner
	}
	return payload
}

// AdaptExternalScannerByExternalOrder builds an ExternalScanner adapter that
// reuses sourceLang's scanner for targetLang by remapping external symbols.
//
// Mapping strategy:
//  1. If either side has duplicate external names, use index mapping
//     (capped to the shorter list length).
//  2. Otherwise, prefer exact external-symbol-name matches.
//  3. Fill remaining slots by index order (within the shorter dimension).
//
// When source and target have different external symbol counts, name-based
// matching pairs tokens that exist in both grammars. Target externals with no
// source match get -1 (the scanner will never produce them). Source externals
// with no target match are silently ignored.
//
// Returns (nil, false) when adaptation is not possible.
func AdaptExternalScannerByExternalOrder(sourceLang, targetLang *Language) (ExternalScanner, bool) {
	if !canAdaptExternalScanner(sourceLang, targetLang) {
		return nil, false
	}

	sourceExt := sourceLang.ExternalSymbols
	nSource := len(sourceExt)
	mapping := buildExternalScannerOrderMapping(sourceLang, targetLang)
	adaptExternalLexStatesByExternalOrder(sourceLang, targetLang, mapping.targetToSource)

	return &externalScannerOrderAdapter{
		inner:                sourceLang.ExternalScanner,
		targetToSource:       mapping.targetToSource,
		sourceCount:          nSource,
		sourceSymbolToIndex:  buildExternalSymbolIndex(sourceExt),
		sourceResultToTarget: mapping.sourceResultToTarget,
	}, true
}

func canAdaptExternalScanner(sourceLang, targetLang *Language) bool {
	if sourceLang == nil || targetLang == nil || sourceLang.ExternalScanner == nil {
		return false
	}
	return len(sourceLang.ExternalSymbols) > 0 && len(targetLang.ExternalSymbols) > 0
}

type externalScannerOrderMapping struct {
	targetToSource       []int
	sourceResultToTarget map[Symbol]Symbol
}

func buildExternalScannerOrderMapping(sourceLang, targetLang *Language) externalScannerOrderMapping {
	sourceExt := sourceLang.ExternalSymbols
	targetExt := targetLang.ExternalSymbols
	targetToSource := newExternalTargetMap(len(targetExt))
	usedSource := make([]bool, len(sourceExt))

	if externalScannerUsesIndexOnlyMapping(sourceLang, targetLang) {
		mapExternalSymbolsByIndex(targetToSource, usedSource)
	} else {
		mapExternalSymbolsByName(sourceLang, targetLang, targetToSource, usedSource)
		fillExternalSymbolIndexFallback(sourceLang, targetLang, targetToSource, usedSource)
	}

	return externalScannerOrderMapping{
		targetToSource:       targetToSource,
		sourceResultToTarget: buildExternalResultSymbolMap(sourceExt, targetExt, targetToSource),
	}
}

func adaptExternalLexStatesByExternalOrder(sourceLang, targetLang *Language, targetToSource []int) {
	if sourceLang == nil || targetLang == nil || len(sourceLang.ExternalLexStates) == 0 ||
		len(targetLang.ExternalSymbols) == 0 || len(targetToSource) != len(targetLang.ExternalSymbols) {
		return
	}
	if len(targetLang.ExternalLexStates) > 0 {
		CertifyCRecoveryCostCompetition(targetLang)
		return
	}
	rows := make([][]bool, len(sourceLang.ExternalLexStates))
	for rowIdx, sourceRow := range sourceLang.ExternalLexStates {
		targetRow := make([]bool, len(targetLang.ExternalSymbols))
		for targetIdx, sourceIdx := range targetToSource {
			if sourceIdx >= 0 && sourceIdx < len(sourceRow) && sourceRow[sourceIdx] {
				targetRow[targetIdx] = true
			}
		}
		rows[rowIdx] = targetRow
	}
	targetLang.ExternalLexStates = rows
	CertifyCRecoveryCostCompetition(targetLang)
}

func newExternalTargetMap(n int) []int {
	targetToSource := make([]int, n)
	for i := range targetToSource {
		targetToSource[i] = -1
	}
	return targetToSource
}

func externalScannerUsesIndexOnlyMapping(sourceLang, targetLang *Language) bool {
	sourceExt := sourceLang.ExternalSymbols
	targetExt := targetLang.ExternalSymbols
	if targetLang.GeneratedByGrammargen {
		return true
	}
	// Equal-length duplicate-name tables are ambiguous; preserve the old
	// positional contract instead of trying to infer by name.
	return len(sourceExt) == len(targetExt) &&
		(hasDuplicateExternalNames(sourceLang, sourceExt) ||
			hasDuplicateExternalNames(targetLang, targetExt))
}

func mapExternalSymbolsByIndex(targetToSource []int, usedSource []bool) {
	for i := 0; i < len(targetToSource) && i < len(usedSource); i++ {
		targetToSource[i] = i
		usedSource[i] = true
	}
}

func mapExternalSymbolsByName(sourceLang, targetLang *Language, targetToSource []int, usedSource []bool) {
	sourceByName := externalSymbolBuckets(sourceLang)
	for targetIdx, targetSym := range targetLang.ExternalSymbols {
		candidates := sourceByName[externalSymbolName(targetLang, targetSym)]
		assignFirstUnusedExternalSource(targetIdx, candidates, targetToSource, usedSource)
	}
}

func externalSymbolBuckets(lang *Language) map[string][]int {
	sourceByName := make(map[string][]int, len(lang.ExternalSymbols))
	for i, sym := range lang.ExternalSymbols {
		name := externalSymbolName(lang, sym)
		sourceByName[name] = append(sourceByName[name], i)
	}
	return sourceByName
}

func assignFirstUnusedExternalSource(targetIdx int, candidates []int, targetToSource []int, usedSource []bool) {
	for _, sourceIdx := range candidates {
		if !usedSource[sourceIdx] {
			targetToSource[targetIdx] = sourceIdx
			usedSource[sourceIdx] = true
			return
		}
	}
}

func fillExternalSymbolIndexFallback(sourceLang, targetLang *Language, targetToSource []int, usedSource []bool) {
	if targetLang == nil || !targetLang.GeneratedByGrammargen {
		return
	}
	for i := 0; i < len(targetToSource) && i < len(usedSource); i++ {
		if targetToSource[i] != -1 || usedSource[i] {
			continue
		}
		targetToSource[i] = i
		usedSource[i] = true
	}
}

func buildExternalResultSymbolMap(sourceExt, targetExt []Symbol, targetToSource []int) map[Symbol]Symbol {
	sourceResultToTarget := make(map[Symbol]Symbol, len(sourceExt))
	sourceAssigned := make([]bool, len(sourceExt))
	for targetIdx, sourceIdx := range targetToSource {
		if sourceIdx < 0 || sourceIdx >= len(sourceExt) {
			continue
		}
		sourceAssigned[sourceIdx] = true
		sourceResultToTarget[sourceExt[sourceIdx]] = targetExt[targetIdx]
	}
	addUnassignedExternalResultSymbols(sourceExt, sourceAssigned, sourceResultToTarget)
	return sourceResultToTarget
}

func buildExternalSymbolIndex(sourceExt []Symbol) map[Symbol]int {
	out := make(map[Symbol]int, len(sourceExt))
	for idx, sym := range sourceExt {
		if _, exists := out[sym]; !exists {
			out[sym] = idx
		}
	}
	return out
}

func addUnassignedExternalResultSymbols(sourceExt []Symbol, sourceAssigned []bool, sourceResultToTarget map[Symbol]Symbol) {
	for sourceIdx, assigned := range sourceAssigned {
		if !assigned {
			sourceResultToTarget[sourceExt[sourceIdx]] = sourceExt[sourceIdx]
		}
	}
}
