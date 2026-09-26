package gotreesitter

import "os"

type resultCompatibilityContext struct {
	root      *Node
	source    []byte
	parser    *Parser
	lang      *Language
	stopCheck parseStopCheck
	// incrementalRanges confines range-capable result normalizers to the
	// reparsed spans of an incremental parse (campaign O(edit),
	// spec.campaign.oedit). It is nil on every fresh parse and on any
	// incremental parse without reuse, which restores the full-tree walk. Only
	// languages proven node-local for range-limiting consume it; every other
	// language ignores it and keeps its full walk (fail-closed).
	incrementalRanges []Range
}

type resultCompatibilityResult struct {
	stopReason   ParseStopReason
	errorSummary resultErrorSummary
}

// normalizeResultCompatibility applies narrow post-build tree rewrites that
// keep gotreesitter output aligned with C tree-sitter and existing recovery
// expectations for grammars with known normalization gaps.
func normalizeResultCompatibility(root *Node, source []byte, p *Parser, incrementalRanges []Range) resultCompatibilityResult {
	return applyResultCompatibility(root, source, p, incrementalRanges, true)
}

func applyResultCompatibilityForParentLinkSummary(
	root *Node,
	source []byte,
	p *Parser,
	incrementalRanges []Range,
) resultCompatibilityResult {
	return applyResultCompatibility(root, source, p, incrementalRanges, false)
}

func applyResultCompatibility(
	root *Node,
	source []byte,
	p *Parser,
	incrementalRanges []Range,
	summarizeErrors bool,
) resultCompatibilityResult {
	var lang *Language
	if p != nil {
		lang = p.language
	}
	if root == nil || lang == nil {
		return resultCompatibilityResult{}
	}
	ctx := resultCompatibilityContext{
		root:              root,
		source:            source,
		parser:            p,
		lang:              lang,
		stopCheck:         p.activeParseStopCheck(),
		incrementalRanges: incrementalRanges,
	}
	if reason := ctx.stopReason(); parseStopReasonIsActive(reason) {
		return resultCompatibilityResult{stopReason: reason}
	}
	result := runLanguageResultCompatibility(ctx)
	// resultMaterializationShouldStop (not parseStopReasonIsActive) here: Go's
	// normalizer (the only one that can produce it — see
	// normalizeGoReturnedTreeCompatibilityWithCensus) may now report ParseStopMemoryBudget,
	// which parseStopReasonIsActive deliberately excludes (many callers rely on
	// its narrower Timeout/Cancelled-only semantics). Without this, a
	// budget-stopped Go result would still fall through into the read-only
	// error-summary walk below.
	if resultMaterializationShouldStop(result.stopReason) {
		return result
	}
	if reason := ctx.stopReason(); parseStopReasonIsActive(reason) {
		result.stopReason = reason
		return result
	}
	if !summarizeErrors {
		return result
	}
	result.stopReason, result.errorSummary = summarizeResultErrorsWithStop(root, ctx.stopCheck)
	if parseStopReasonIsActive(result.stopReason) {
		return result
	}
	result.stopReason = ctx.stopReason()
	return result
}

func (ctx resultCompatibilityContext) stopReason() ParseStopReason {
	if ctx.stopCheck == nil {
		return ParseStopNone
	}
	reason := ctx.stopCheck()
	if reason == "" {
		return ParseStopNone
	}
	return reason
}

func runLanguageResultCompatibility(ctx resultCompatibilityContext) resultCompatibilityResult {
	if isCobolLanguage(ctx.lang) {
		dispatcherArmCensus(ctx, "predicate.cobol-exact", func() {
			normalizeCobolCompatibility(ctx.root, ctx.source, ctx.lang)
		})
		return resultCompatibilityResult{stopReason: ctx.stopReason()}
	}

	switch ctx.lang.Name {
	case "ada":
		dispatcherArmSubpassCensus(ctx, "dispatch.ada", func(census materializationSubpassCensus) {
			normalizeAdaCompatibilityWithCensus(ctx.root, ctx.source, ctx.lang, census)
		})
	case "apex":
		dispatcherArmSubpassCensus(ctx, "dispatch.apex", func(census materializationSubpassCensus) {
			census.run("dispatch.apex.class-literal-alias", func() {
				normalizeApexClassLiteralAccess(ctx.root, ctx.source, ctx.lang)
			})
		})
	case "authzed":
		dispatcherArmCensus(ctx, "dispatch.authzed", func() { normalizeAuthzedCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "awk":
		dispatcherArmCensus(ctx, "dispatch.awk", func() { normalizeAwkCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "bitbake":
		dispatcherArmCensus(ctx, "dispatch.bitbake", func() { normalizeBitbakeCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "c", "cpp":
		dispatcherArmCensus(ctx, "dispatch.c_cpp", func() { normalizeCCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang) })
	case "c_sharp":
		dispatcherArmCensus(ctx, "dispatch.c_sharp", func() { normalizeCSharpCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang) })
	case "cooklang":
		dispatcherArmSubpassCensus(ctx, "dispatch.cooklang", func(census materializationSubpassCensus) {
			normalizeCooklangCompatibilityWithCensus(ctx.root, ctx.source, ctx.lang, census)
		})
	case "corn":
		dispatcherArmCensus(ctx, "dispatch.corn", func() { normalizeCornCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "dart":
		dispatcherArmCensus(ctx, "dispatch.dart", func() { normalizeDartCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "doxygen":
		dispatcherArmCensus(ctx, "dispatch.doxygen", func() { normalizeDoxygenCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "dtd":
		dispatcherArmCensus(ctx, "dispatch.dtd", func() { normalizeDTDCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "go":
		var stopReason ParseStopReason
		dispatcherArmSubpassCensus(ctx, "dispatch.go", func(census materializationSubpassCensus) {
			stopReason = normalizeGoReturnedTreeCompatibilityWithCensus(ctx.root, ctx.source, ctx.parser, ctx.lang, ctx.incrementalRanges, census)
		})
		return resultCompatibilityResult{stopReason: stopReason}
	case "hlsl":
		dispatcherArmCensus(ctx, "dispatch.hlsl", func() { normalizeHLSLCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "javascript":
		var stopReason ParseStopReason
		dispatcherArmCensus(ctx, "dispatch.javascript", func() {
			stopReason = normalizeJavaScriptCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang)
		})
		return resultCompatibilityResult{stopReason: stopReason}
	case "julia":
		dispatcherArmCensus(ctx, "dispatch.julia", func() { normalizeJuliaCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "kotlin":
		dispatcherArmSubpassCensus(ctx, "dispatch.kotlin", func(census materializationSubpassCensus) {
			normalizeKotlinCompatibilityWithCensus(ctx.root, ctx.source, ctx.lang, census)
		})
	case "perl":
		dispatcherArmCensus(ctx, "dispatch.perl", func() { normalizePerlCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "php":
		dispatcherArmCensus(ctx, "dispatch.php", func() { normalizePHPCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang) })
	case "powershell":
		dispatcherArmCensus(ctx, "dispatch.powershell", func() {
			normalizePowerShellProgramShape(ctx.root, ctx.source, ctx.lang)
			normalizePowerShellErrorProgramRoot(ctx.root, ctx.lang)
			normalizePowerShellPathCommandNameVariables(ctx.root, ctx.source, ctx.lang)
			normalizePowerShellEnumStatementKeywordSpans(ctx.root, ctx.source, ctx.lang)
		})
	case "python":
		dispatcherArmCensus(ctx, "dispatch.python", func() { normalizePythonCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang) })
	case "rust":
		dispatcherArmCensus(ctx, "dispatch.rust", func() { normalizeRustCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang) })
	case "scala":
		dispatcherArmCensus(ctx, "dispatch.scala", func() { normalizeScalaCompatibility(ctx.root, ctx.source, ctx.parser, ctx.lang) })
	case "solidity":
		dispatcherArmCensus(ctx, "dispatch.solidity", func() {
			normalizeSolidityMemberObjectWrappers(ctx.root, ctx.lang)
			normalizeSolidityCallExpressionAliases(ctx.root, ctx.lang)
		})
	case "sql":
		dispatcherArmCensus(ctx, "dispatch.sql", func() {
			normalizeSQLRecoveredSelectRoot(ctx.root, ctx.lang)
			normalizeSQLTrailingSelectListError(ctx.root, ctx.lang)
			if ctx.parser != nil && !ctx.parser.skipRecoveryReparse {
				normalizeSQLRecoveredTopLevelSelectStatements(ctx.root, ctx.source, ctx.parser, ctx.lang)
			}
		})
	case "swift":
		dispatcherArmSubpassCensus(ctx, "dispatch.swift", func(census materializationSubpassCensus) {
			normalizeSwiftCompatibilityWithCensus(ctx.root, ctx.source, ctx.parser, ctx.lang, census)
		})
	case "templ":
		dispatcherArmCensus(ctx, "dispatch.templ", func() { normalizeTemplCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "wgsl":
		dispatcherArmCensus(ctx, "dispatch.wgsl", func() { normalizeWGSLCompatibility(ctx.root, ctx.lang) })
	case "wolfram":
		dispatcherArmCensus(ctx, "dispatch.wolfram", func() { normalizeWolframCompatibility(ctx.root, ctx.source, ctx.lang) })
	case "tsx", "typescript":
		var stopReason ParseStopReason
		dispatcherArmCensus(ctx, "dispatch.typescript", func() {
			stopReason = normalizeTypeScriptTreeCompatibilityWithParser(ctx.root, ctx.source, ctx.parser, ctx.lang)
		})
		return resultCompatibilityResult{stopReason: stopReason}
	case "yaml":
		dispatcherArmCensus(ctx, "dispatch.yaml", func() { normalizeYAMLRecoveredRoot(ctx.root, ctx.source, ctx.lang) })
	}
	return resultCompatibilityResult{stopReason: ctx.stopReason()}
}

// nativeRecoveredStructureIsAuthoritative returns true when the selected
// parser reduction owns a complete recovered root. Exact runtime profiles
// certify the native structure. Use conservative repair for other roots.
func nativeRecoveredStructureIsAuthoritative(root *Node, source []byte, p *Parser, lang *Language) bool {
	if root == nil || p == nil || lang == nil || p.language != lang || len(source) == 0 {
		return false
	}
	if lang.NativeResultCompatibility&ResultCompatibilityNativeRecoveredStructure == 0 {
		return false
	}
	if len(p.included) > 0 || !p.hasRootSymbol || root.symbol != p.rootSymbol {
		return false
	}
	if root.rawShape == 0 || !root.hasError() {
		return false
	}
	if root.startByte > firstNonTriviaByteStart(source) ||
		root.endByte < lastNonTriviaByteEnd(source) {
		return false
	}
	return nativeRecoveredStructureHasIsolatedErrorReceipt(root)
}

// nativeRecoveredStructureHasIsolatedErrorReceipt verifies a narrow
// recovered-root class. Raw top-level spans must agree.
// The tree must contain one one-byte error and no missing nodes.
func nativeRecoveredStructureHasIsolatedErrorReceipt(root *Node) bool {
	if root == nil || root.ownerArena == nil || root.rawShape == 0 {
		return false
	}
	shape, ok := root.ownerArena.rawShapeForRef(root.rawShape)
	if !ok || shape.symbol != root.symbol || shape.childCount() != resultChildCount(root) {
		return false
	}
	rawChildren := root.ownerArena.rawShapeChildren(shape)
	if len(rawChildren) != resultChildCount(root) {
		return false
	}
	for i := range rawChildren {
		rawChild := stackEntryNode(rawChildren[i].entry())
		child := resultChildAt(root, i)
		if rawChild == nil || child == nil ||
			rawChild.startByte != child.startByte ||
			rawChild.endByte != child.endByte {
			return false
		}
	}

	explicitErrors := 0
	valid := true
	var walk func(*Node, int)
	walk = func(node *Node, depth int) {
		if node == nil || !valid || depth >= maxTreeWalkDepth {
			valid = false
			return
		}
		if node.isMissing() {
			valid = false
			return
		}
		if node.symbol == errorSymbol {
			explicitErrors++
			if explicitErrors > 1 ||
				node.endByte <= node.startByte ||
				node.endByte != node.startByte+1 {
				valid = false
				return
			}
		}
		for i := 0; i < resultChildCount(node); i++ {
			walk(resultChildAt(node, i), depth+1)
		}
	}
	walk(root, 0)
	return valid && explicitErrors == 1
}

// --- R2 dispatcher-arm census (docs/root-normalization-retirement.md) ---
//
// runLanguageResultCompatibility's switch is the per-language dispatcher arm
// registry described in testdata/result_compat_ownership_v1.json
// ("dispatcher_arms"). Most arms return nothing: whether a given call ever
// actually rewrites the tree has never been measured in production, only
// asserted by unit fixtures that construct the triggering shape by hand. R2
// requires a census before any retirement.
//
// dispatcherArmCensus is the uniform, zero-per-function-surgery probe: it
// snapshots a full structural fingerprint of ctx.root immediately before and
// after the arm's closure runs and diffs the two snapshots. This needs no
// changes to any of the ~80 normalizeXCompatibility functions themselves and
// covers every arm identically, including the multi-function arms (bash,
// pascal, powershell, solidity, sql) where "the arm" is the whole case body,
// matching the registry's one-entry-per-language granularity.
//
// It is gated behind GTS_DISPATCHER_CENSUS=1 so the fingerprint walk (which
// is a full O(n) tree traversal, run twice) costs nothing on an ordinary
// parse: dispatcherCensusEnabled's os.Getenv check is the only overhead paid
// when the flag is unset, and every dispatcher arm already performs at least
// one full-tree walk of its own, so even the closure indirection paid when
// the flag is set is in the noise relative to the arm's own cost.
//
// parser_result_dispatcher_transcript.go extends this same probe under its
// own flag, GTS_DISPATCHER_TRANSCRIPT=1: where this census answers "did the
// arm change the tree?", the transcript answers "what did it change, and
// where?" for every invocation that did. dispatcherArmCensus,
// dispatcherArmSubpassCensus, and materializationSubpassCensus.run below each
// check both flags, but the disabled and census-only branches are exactly
// the code that shipped before the transcript existed.
func dispatcherCensusEnabled() bool {
	return os.Getenv("GTS_DISPATCHER_CENSUS") == "1"
}

// dispatcherArmCensus runs fn (the body of one dispatcher-arm switch case)
// and, only when census or transcript instrumentation is enabled, snapshots
// ctx.root's structural fingerprint before and after fn runs. When the
// census is enabled it records whether fn changed the fingerprint under
// armID via (*Parser).recordNormalizationMetric. When the transcript is
// enabled and fn changed the fingerprint, it also records a transcript
// effect record (see recordDispatcherArmTranscript). armID matches the
// corresponding "dispatch.<name>" / "predicate.<name>" id in
// testdata/result_compat_ownership_v1.json so both receipts trace directly
// back to the registry entry they measure.
func dispatcherArmCensus(ctx resultCompatibilityContext, armID string, fn func()) {
	census := dispatcherCensusEnabled()
	transcript := dispatcherTranscriptEnabled()
	if !census && !transcript {
		fn()
		return
	}
	if !transcript {
		before := captureDispatcherFingerprint(ctx.root)
		fn()
		after := captureDispatcherFingerprint(ctx.root)
		visited, rewritten := diffDispatcherFingerprint(before, after)
		if ctx.parser != nil {
			ctx.parser.recordNormalizationMetric(armID, 1, 1, visited, rewritten)
		}
		return
	}
	before := captureDispatcherTranscriptSnapshot(ctx.root)
	fn()
	after := captureDispatcherTranscriptSnapshot(ctx.root)
	visited, rewritten := diffDispatcherFingerprint(before.signatures, after.signatures)
	if census && ctx.parser != nil {
		ctx.parser.recordNormalizationMetric(armID, 1, 1, visited, rewritten)
	}
	if rewritten > 0 {
		recordDispatcherArmTranscript(ctx.lang, armID, before, after)
	}
}

// materializationSubpassCensus records one named subpass within a dispatcher
// arm. The parent arm supplies its enabled state, so each subpass avoids
// another environment lookup.
type materializationSubpassCensus struct {
	ctx        resultCompatibilityContext
	enabled    bool // census: GTS_DISPATCHER_CENSUS=1
	transcript bool // transcript: GTS_DISPATCHER_TRANSCRIPT=1
}

// run invokes fn and, when the parent dispatcher census or transcript is
// enabled, records its exact tree mutation receipt (census) and/or a
// transcript effect record (transcript; see recordDispatcherArmTranscript).
func (c materializationSubpassCensus) run(subpassID string, fn func()) {
	if !c.enabled && !c.transcript {
		fn()
		return
	}
	if !c.transcript {
		before := captureDispatcherFingerprint(c.ctx.root)
		fn()
		after := captureDispatcherFingerprint(c.ctx.root)
		visited, rewritten := diffDispatcherFingerprint(before, after)
		if c.ctx.parser != nil {
			c.ctx.parser.recordNormalizationMetric(subpassID, 1, 1, visited, rewritten)
		}
		return
	}
	before := captureDispatcherTranscriptSnapshot(c.ctx.root)
	fn()
	after := captureDispatcherTranscriptSnapshot(c.ctx.root)
	visited, rewritten := diffDispatcherFingerprint(before.signatures, after.signatures)
	if c.enabled && c.ctx.parser != nil {
		c.ctx.parser.recordNormalizationMetric(subpassID, 1, 1, visited, rewritten)
	}
	if rewritten > 0 {
		recordDispatcherArmTranscript(c.ctx.lang, subpassID, before, after)
	}
}

// dispatcherArmSubpassCensus preserves the aggregate dispatcher receipt and
// supplies one recorder for its materialization subpasses.
func dispatcherArmSubpassCensus(
	ctx resultCompatibilityContext,
	armID string,
	fn func(materializationSubpassCensus),
) {
	enabled := dispatcherCensusEnabled()
	transcript := dispatcherTranscriptEnabled()
	census := materializationSubpassCensus{ctx: ctx, enabled: enabled, transcript: transcript}
	if !enabled && !transcript {
		fn(census)
		return
	}
	if !transcript {
		before := captureDispatcherFingerprint(ctx.root)
		fn(census)
		after := captureDispatcherFingerprint(ctx.root)
		visited, rewritten := diffDispatcherFingerprint(before, after)
		if ctx.parser != nil {
			ctx.parser.recordNormalizationMetric(armID, 1, 1, visited, rewritten)
		}
		return
	}
	before := captureDispatcherTranscriptSnapshot(ctx.root)
	fn(census)
	after := captureDispatcherTranscriptSnapshot(ctx.root)
	visited, rewritten := diffDispatcherFingerprint(before.signatures, after.signatures)
	if enabled && ctx.parser != nil {
		ctx.parser.recordNormalizationMetric(armID, 1, 1, visited, rewritten)
	}
	if rewritten > 0 {
		recordDispatcherArmTranscript(ctx.lang, armID, before, after)
	}
}

// dispatcherNodeSignature captures the observable surface of one Node. It
// includes the parser states because callers can query them and incremental
// reuse consumes them. It excludes internal caches and GLR bookkeeping.
//
// This is the full set of fields Node exposes that can differ between two
// trees built from the same source: if every signature in a preorder walk is
// identical before and after an arm runs, nothing the arm could have done is
// observable through the public Node API, so "no rewrite" is a sound
// conclusion, not merely a hash coincidence (there is no hashing here at
// all -- fields are compared for exact equality).
type dispatcherNodeSignature struct {
	symbol       Symbol
	startByte    uint32
	endByte      uint32
	startPoint   Point
	endPoint     Point
	parseState   StateID
	preGotoState StateID
	childCount   int32
	fieldID      FieldID
	flags        nodeFlags
}

// dispatcherCensusContentFlagMask selects the nodeFlags bits that are part of
// a node's observable content (see dispatcherNodeSignature) and excludes
// internal-only bookkeeping bits.
const dispatcherCensusContentFlagMask = nodeFlagNamed | nodeFlagExtra | nodeFlagMissing | nodeFlagHasError | nodeFlagDirty | nodeFlagExternalScannerToken

func dispatcherFingerprintEntryFlags(entry stackEntry) nodeFlags {
	if node := stackEntryNode(entry); node != nil {
		return node.flags & dispatcherCensusContentFlagMask
	}
	if node := stackEntryNoTreeNode(entry); node != nil {
		return node.flags & dispatcherCensusContentFlagMask
	}
	if node := stackEntryCompactFullLeaf(entry); node != nil {
		return node.flags & dispatcherCensusContentFlagMask
	}
	if node := stackEntryPendingParent(entry); node != nil {
		return node.flags & dispatcherCensusContentFlagMask
	}
	return 0
}

func dispatcherFingerprintChild(entry stackEntry, arena *nodeArena, i int) (stackEntry, *nodeArena, FieldID, bool) {
	child, ok := stackEntryPendingChildAt(arena, entry, i)
	if !ok || !stackEntryHasNode(child) {
		return stackEntry{}, nil, 0, false
	}
	fieldID, _, fieldOK := stackEntryPendingFieldMetadataAt(arena, entry, i)
	if !fieldOK {
		fieldID = 0
	}
	childArena := arena
	if node := stackEntryNode(child); node != nil {
		childArena = node.ownerArena
	}
	return child, childArena, fieldID, true
}

// captureDispatcherFingerprint walks root in preorder (iteratively, to avoid
// recursion-depth limits on deep or adversarial trees) and returns one
// signature per node. The walk order itself is part of the fingerprint: a
// normalizer that reorders, inserts, removes, or reparents nodes shifts later
// positions even when the individual node contents are byte-identical, so a
// structural edit is still detected even when no single node's own fields
// changed.
func captureDispatcherFingerprint(root *Node) []dispatcherNodeSignature {
	if root == nil {
		return nil
	}
	type pending struct {
		entry   stackEntry
		arena   *nodeArena
		fieldID FieldID
	}
	out := make([]dispatcherNodeSignature, 0, 64)
	stack := make([]pending, 0, 64)
	stack = append(stack, pending{
		entry: newStackEntryNode(root.parseState, root),
		arena: root.ownerArena,
	})
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !stackEntryHasNode(top.entry) {
			continue
		}
		childCount := stackEntryNodeChildCount(top.entry)
		out = append(out, dispatcherNodeSignature{
			symbol:       stackEntryNodeSymbol(top.entry),
			startByte:    stackEntryNodeStartByte(top.entry),
			endByte:      stackEntryNodeEndByte(top.entry),
			startPoint:   stackEntryNodeStartPoint(top.entry),
			endPoint:     stackEntryNodeEndPoint(top.entry),
			parseState:   stackEntryNodeParseState(top.entry),
			preGotoState: stackEntryNodePreGotoState(top.entry),
			childCount:   int32(childCount),
			fieldID:      top.fieldID,
			flags:        dispatcherFingerprintEntryFlags(top.entry),
		})
		for i := childCount - 1; i >= 0; i-- {
			child, childArena, fieldID, ok := dispatcherFingerprintChild(top.entry, top.arena, i)
			if !ok {
				continue
			}
			stack = append(stack, pending{entry: child, arena: childArena, fieldID: fieldID})
		}
	}
	return out
}

// diffDispatcherFingerprint compares two preorder signature snapshots of the
// same root captured immediately before and after a dispatcher arm ran.
// visited is the node count of the "before" snapshot (what the probe had to
// look at). rewritten is a position-wise difference count: it is exact when
// nothing structural changed (same length, same content at every position
// yields rewritten == 0, an exact, hash-free equality check), but it is only
// an approximation of "how many nodes changed" when a node was inserted or
// removed near the root, because every following node's preorder position
// shifts by one and each shifted position then also compares unequal even
// though its own content did not change. Callers should treat rewritten == 0
// as an exact "no rewrite" receipt and rewritten > 0 as an exact "the arm
// changed something" receipt, but should not treat the magnitude of a
// nonzero rewritten count as a precise edit distance.
func diffDispatcherFingerprint(before, after []dispatcherNodeSignature) (visited, rewritten uint64) {
	visited = uint64(len(before))
	shorter := len(before)
	if len(after) < shorter {
		shorter = len(after)
	}
	var changed uint64
	for i := 0; i < shorter; i++ {
		if before[i] != after[i] {
			changed++
		}
	}
	lengthDelta := len(before) - len(after)
	if lengthDelta < 0 {
		lengthDelta = -lengthDelta
	}
	changed += uint64(lengthDelta)
	return visited, changed
}
