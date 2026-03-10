package gotreesitter

// externalScannerOrderAdapter adapts an external scanner from one language to
// another by remapping external token order and scanner result symbols.
//
// It is intended for parity scenarios where two languages share scanner logic
// but use different symbol IDs (or external symbol names/aliases).
type externalScannerOrderAdapter struct {
	inner                ExternalScanner
	targetToSource       []int
	sourceResultToTarget map[Symbol]Symbol
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
	return a.inner.Create()
}

func (a *externalScannerOrderAdapter) Destroy(payload any) {
	a.inner.Destroy(payload)
}

func (a *externalScannerOrderAdapter) Serialize(payload any, buf []byte) int {
	return a.inner.Serialize(payload, buf)
}

func (a *externalScannerOrderAdapter) Deserialize(payload any, buf []byte) {
	a.inner.Deserialize(payload, buf)
}

func (a *externalScannerOrderAdapter) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	if a == nil || a.inner == nil {
		return false
	}

	sourceValid := make([]bool, len(a.targetToSource))
	for targetIdx, isValid := range validSymbols {
		if !isValid || targetIdx < 0 || targetIdx >= len(a.targetToSource) {
			continue
		}
		sourceIdx := a.targetToSource[targetIdx]
		if sourceIdx >= 0 && sourceIdx < len(sourceValid) {
			sourceValid[sourceIdx] = true
		}
	}

	ok := a.inner.Scan(payload, lexer, sourceValid)
	if !ok {
		return false
	}

	// Map scanner result symbol from source language ID to target language ID.
	if lexer != nil && lexer.hasResult {
		if mapped, exists := a.sourceResultToTarget[lexer.resultSymbol]; exists {
			lexer.resultSymbol = mapped
		}
	}
	return true
}

// AdaptExternalScannerByExternalOrder builds an ExternalScanner adapter that
// reuses sourceLang's scanner for targetLang by remapping external symbols.
//
// Mapping strategy:
//  1. If either side has duplicate external names, use index mapping.
//     (Name-based matching is ambiguous and can mis-map variants.)
//  2. Otherwise, prefer exact external-symbol-name matches.
//  3. Fill remaining slots by index order.
//
// Returns (nil, false) when adaptation is not possible.
func AdaptExternalScannerByExternalOrder(sourceLang, targetLang *Language) (ExternalScanner, bool) {
	if sourceLang == nil || targetLang == nil || sourceLang.ExternalScanner == nil {
		return nil, false
	}

	sourceExt := sourceLang.ExternalSymbols
	targetExt := targetLang.ExternalSymbols
	if len(sourceExt) == 0 || len(sourceExt) != len(targetExt) {
		return nil, false
	}

	n := len(sourceExt)
	targetToSource := make([]int, n)
	usedSource := make([]bool, n)
	for i := range targetToSource {
		targetToSource[i] = -1
	}

	useIndexOnly := hasDuplicateExternalNames(sourceLang, sourceExt) ||
		hasDuplicateExternalNames(targetLang, targetExt)

	if useIndexOnly {
		for i := 0; i < n; i++ {
			targetToSource[i] = i
			usedSource[i] = true
		}
	} else {
		// Build source symbol-name buckets (for exact-name alignment).
		sourceByName := make(map[string][]int, n)
		for i, sym := range sourceExt {
			sourceByName[externalSymbolName(sourceLang, sym)] = append(sourceByName[externalSymbolName(sourceLang, sym)], i)
		}

		// Pass 1: name-based matching.
		for targetIdx, targetSym := range targetExt {
			candidates := sourceByName[externalSymbolName(targetLang, targetSym)]
			for _, sourceIdx := range candidates {
				if !usedSource[sourceIdx] {
					targetToSource[targetIdx] = sourceIdx
					usedSource[sourceIdx] = true
					break
				}
			}
		}

		// Pass 2: index fallback.
		for i := 0; i < n; i++ {
			if targetToSource[i] != -1 {
				continue
			}
			if !usedSource[i] {
				targetToSource[i] = i
				usedSource[i] = true
			}
		}
	}

	// Pass 3: assign any remaining unused source indices.
	nextUnused := 0
	for i := 0; i < n; i++ {
		if targetToSource[i] != -1 {
			continue
		}
		for nextUnused < n && usedSource[nextUnused] {
			nextUnused++
		}
		if nextUnused >= n {
			return nil, false
		}
		targetToSource[i] = nextUnused
		usedSource[nextUnused] = true
		nextUnused++
	}

	// Build mapping from source scanner result symbols to target symbols.
	sourceResultToTarget := make(map[Symbol]Symbol, n)
	sourceAssigned := make([]bool, n)
	for targetIdx, sourceIdx := range targetToSource {
		if sourceIdx < 0 || sourceIdx >= n {
			continue
		}
		sourceAssigned[sourceIdx] = true
		sourceResultToTarget[sourceExt[sourceIdx]] = targetExt[targetIdx]
	}
	for sourceIdx, assigned := range sourceAssigned {
		if assigned {
			continue
		}
		sourceResultToTarget[sourceExt[sourceIdx]] = targetExt[sourceIdx]
	}

	return &externalScannerOrderAdapter{
		inner:                sourceLang.ExternalScanner,
		targetToSource:       targetToSource,
		sourceResultToTarget: sourceResultToTarget,
	}, true
}
