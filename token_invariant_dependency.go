package gotreesitter

import "slices"

// The range wrapper only filters token tuples. Equal primitives and unchanged
// ranges preserve its decisions, including discarded boundary tokens.
func tokenInvariantDFASource(ts TokenSource, ranges []Range) *dfaTokenSource {
	if wrapped, ok := ts.(*includedRangeTokenSource); ok {
		if wrapped == nil || !slices.Equal(wrapped.ranges, ranges) {
			return nil
		}
		ts = wrapped.base
	} else if len(ranges) != 0 {
		return nil
	}
	d, ok := ts.(*dfaTokenSource)
	if !ok || d == nil || d.lexer == nil || len(d.lexer.includedRanges) != 0 {
		return nil
	}
	return d
}

// Capture the exact attempt before its token source closes. A later retry can
// return a different tree, so the outer Parse call cannot supply this evidence.
func (t *Tree) captureTokenInvariantReadSpan(ts TokenSource) {
	if t == nil {
		return
	}
	span := uint32(0)
	d := tokenInvariantDFASource(ts, t.includedRanges)
	if d != nil && d.language == t.language {
		span = d.tokenInvariantReadSpan()
	}
	t.captureTokenInvariantReadSpanValue(span)
}

// Publish a bound from the exact accepted attempt. The caller must authenticate
// its token source and preserve the bound before that source closes.
func (t *Tree) captureTokenInvariantReadSpanValue(span uint32) {
	if t == nil {
		return
	}
	t.tokenInvariantReadSpan = 0
	if t.resultCompatibilityFinalizer != nil {
		t.resultCompatibilityFinalizer.tokenInvariantReadSpan = 0
	}
	if !t.tokenInvariantReadSpanResultEligible() {
		return
	}
	if t.hasDeferredResultCompatibility() && !t.resultCompatibilityApplied {
		t.resultCompatibilityFinalizer.tokenInvariantReadSpan = span
		return
	}
	t.tokenInvariantReadSpan = span
}

func (t *Tree) tokenInvariantReadSpanResultEligible() bool {
	return t != nil && t.root != nil && !t.root.hasError() &&
		t.parseRuntime.StopReason == ParseStopAccepted && !t.parseRuntime.Truncated && !t.parseRuntime.CRecoveryEnteredErrorState
}

func (p *Parser) tokenInvariantEditDependencies(source []byte, oldTree *Tree, node *Node, edit InputEdit, ts TokenSource, timing *incrementalParseTiming) (uint32, bool) {
	if oldTree.tokenInvariantReadSpan == 0 || oldTree.root.hasError() ||
		oldTree.sourceEncoding != InputEncodingUTF8 || uint64(edit.OldEndByte) > uint64(len(source)) {
		return 0, false
	}
	d := tokenInvariantDFASource(ts, oldTree.includedRanges)
	if d == nil || d.language != p.language || !d.tokenInvariantInternalPrimitivesSupported() {
		return 0, false
	}
	digitEdit := asciiDigitTextInvariantEdit(source, oldTree.source, edit)
	scannerEquivalent := tokenInvariantScannerASCIIEditEquivalent(p.language.ExternalScanner, oldTree.source, source, edit)
	// TypeScript contextual wrappers inspect punctuation, fixed keywords, and
	// character classes. Digit identity cannot change those decisions within
	// a clean numeric leaf. Raw close-angle probes still require comparison.
	typeScriptNumber := p.language.Name == "typescript" && scannerEquivalent && digitEdit &&
		node != nil && node.isNamed() && node.ChildCount() == 0 && !node.hasError() && !node.isMissing() &&
		node.Type(p.language) == "number"
	if !d.tokenInvariantInternalDependenciesForwardOnly() && !typeScriptNumber {
		return 0, false
	}
	if !scannerEquivalent && !d.compactReuseForwardDependenciesOnly() {
		return 0, false
	}
	// Primitive equality excludes token text. Authenticate source-sensitive
	// parser and result rules separately before using that equality.
	if p.language.Name == "go" || p.language.Name == "python" {
		if !digitEdit {
			return 0, false
		}
		kind := node.Type(p.language)
		if p.language.Name == "go" && kind != "int_literal" && kind != "float_literal" {
			return 0, false
		}
		// Python repairs inspect syntax, delimiters, and identifier membership.
		// They do not distinguish digits within a clean numeric leaf.
		if p.language.Name == "python" && kind != "integer" && kind != "float" {
			return 0, false
		}
	} else if p.language.Name == "typescript" {
		// TypeScript normalization uses syntax and character classes, not the
		// value of a numeric leaf. Do not extend this proof to token text.
		if !typeScriptNumber {
			return 0, false
		}
	} else if p.language.Name == "julia" {
		// Julia normalization does not inspect top-level line-comment text.
		// Keep nested comments outside this source-semantic proof.
		if !scannerEquivalent || node == nil || node.parent != oldTree.root || !node.isNamed() ||
			node.ChildCount() != 0 || node.hasError() || node.isMissing() || node.Type(p.language) != "line_comment" ||
			!hashLineCommentTextInvariantEdit(source, oldTree.source, node, edit) {
			return 0, false
		}
	} else if !resultCompatibilityElisionEligible(p.language) {
		return 0, false
	}
	if timing != nil {
		timing.tokenInvariantDependencyChecks++
	}
	return d.tokenInvariantPrimitiveEditsEquivalentWithScannerProof(oldTree.source, source, edit, oldTree.tokenInvariantReadSpan, scannerEquivalent)
}

func tokenInvariantScannerASCIIEditEquivalent(scanner ExternalScanner, oldSource, source []byte, edit InputEdit) bool {
	classes, ok := scanner.(ASCIIEquivalenceExternalScanner)
	if !ok || edit.StartByte >= edit.OldEndByte || edit.NewEndByte != edit.OldEndByte ||
		uint64(edit.OldEndByte) > uint64(len(oldSource)) || uint64(edit.NewEndByte) > uint64(len(source)) {
		return false
	}
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if oldSource[i] >= 128 || source[i] >= 128 {
			return false
		}
		class := classes.ExternalScannerASCIIEquivalenceClass(oldSource[i])
		if class == 0 || class != classes.ExternalScannerASCIIEquivalenceClass(source[i]) {
			return false
		}
	}
	return true
}

// Forward frontiers cannot authenticate prefix reads or contextual repairs.
// StatelessExternalScanner requires local lookahead and valid symbols.
// Both reuse paths use this contract, including builds without compact parsing.
func (d *dfaTokenSource) compactReuseForwardDependenciesOnly() bool {
	if !d.tokenInvariantInternalDependenciesForwardOnly() {
		return false
	}
	if d.hasExternalSymbols || d.hasExternalScanner || d.language.ExternalScanner != nil {
		scanner, ok := d.language.ExternalScanner.(StatelessExternalScanner)
		return d.hasExternalScanner && ok && scanner.ExternalScannerIsStateless()
	}
	return true
}

func (d *dfaTokenSource) tokenInvariantInternalDependenciesForwardOnly() bool {
	return d.tokenInvariantInternalPrimitivesSupported() && !supportsCompactCloseAngleSplit(d.language.Name)
}

// Primitive equality alone does not authenticate contextual source wrappers.
// Callers must either exclude those wrappers or prove the edit preserves them.
func (d *dfaTokenSource) tokenInvariantInternalPrimitivesSupported() bool {
	return d != nil && d.language != nil && !d.isBash && !d.isBashGenerated && !d.isComment &&
		!d.isFortran && !d.isScheme && !d.isSwift && !d.hasZeroWidthSentinelSymbol
}
