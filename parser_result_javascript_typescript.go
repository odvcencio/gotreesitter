package gotreesitter

import "bytes"

// javaScriptTypeScriptCompatMemoryBudgetStopReason forces a real (unmasked)
// memory-budget check for the JS/TS fused compat pipeline's stage boundaries
// in normalizeJavaScriptCompatibility / normalizeTypeScriptTreeCompatibilityWithParser.
// Mirrors (*Parser).goCompatMemoryBudgetStopReason (parser_result_go.go): the
// JS/TS fused walk shared the identical containment gap the Go compat walk
// had before the 2026-07-12 gocompat-walk-containment-gap fix — no stop
// polling of any kind (no timeout, cancellation, or memory-budget check) —
// so a parse that is already over budget (the parse loop stopped, or the
// arena/runtime heap grew past budget during an earlier stage) would
// otherwise normalize a partial tree with no guarantee of a consistent,
// budget-honoring result.
func (p *Parser) javaScriptTypeScriptCompatMemoryBudgetStopReason(arena *nodeArena) ParseStopReason {
	if p == nil {
		return ParseStopNone
	}
	if arena != nil && arena.budgetExhausted() {
		return p.noteMemoryBudgetStop(parseMemoryBudgetStopSourceArena)
	}
	return p.compatRuntimeMemoryBudgetStopReason()
}

// normalizeJavaScriptCompatibility drives JavaScript's post-build
// compatibility pipeline. It mirrors normalizeGoReturnedTreeCompatibilityWithCensus's
// containment discipline (parser_result_go.go): it is skipped entirely when
// the parse already carries an active stop (timeout/cancellation) or an
// already-tripped memory budget, and the fused walk plus the other
// whole-tree passes below are wired with a parseStopPoller (timeout +
// cancellation + runtime memory budget) at the same coarse, ~1024-node
// stride the Go compat walk uses (parseStopPollMask), so a runaway tree can
// no longer balloon heap growth budget-blind during result finalization.
func normalizeJavaScriptCompatibility(root *Node, source []byte, p *Parser, lang *Language) ParseStopReason {
	var arena *nodeArena
	if root != nil {
		arena = root.ownerArena
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.javaScriptTypeScriptCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	normalizeJavaScriptProgramStart(root, lang)

	poller := parseStopPoller{check: p.activeParseStopCheck(), memoryBudgetParser: p}
	// Fused walk handles empty_statement, statement keywords (if/while),
	// optional-chain leaves, call_expression precedence, and builds unary/
	// binary candidate indexes — six separate full-tree walks collapsed into
	// one walk + indexed rewrites.
	if _, reason := normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root, source, lang, &poller); resultMaterializationShouldStop(reason) {
		return reason
	}
	if reason := normalizeJavaScriptTrailingContinueComments(root, source, lang, &poller); resultMaterializationShouldStop(reason) {
		return reason
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := p.javaScriptTypeScriptCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
	normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
	normalizeJavaScriptTopLevelObjectLiterals(root, lang)
	normalizeJavaScriptProgramEnd(root, source, lang)
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	return p.javaScriptTypeScriptCompatMemoryBudgetStopReason(arena)
}

// normalizeTypeScriptTreeCompatibilityWithParser drives TypeScript/TSX's
// post-build compatibility pipeline. It shares normalizeJavaScriptCompatibility's
// containment discipline (see its doc comment above): skipped entirely when
// the parse already carries an active stop or an already-tripped memory
// budget, and the fused walk is wired with a parseStopPoller at the same
// coarse ~1024-node stride the Go compat walk uses. Both the fast (default)
// path and the metrics-recording path below share the same fused walk and
// poller, so containment applies identically regardless of which path is
// taken.
func normalizeTypeScriptTreeCompatibilityWithParser(root *Node, source []byte, parser *Parser, lang *Language) ParseStopReason {
	var arena *nodeArena
	if root != nil {
		arena = root.ownerArena
	}
	if reason := parser.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	if reason := parser.javaScriptTypeScriptCompatMemoryBudgetStopReason(arena); reason == ParseStopMemoryBudget {
		return reason
	}
	poller := parseStopPoller{check: parser.activeParseStopCheck(), memoryBudgetParser: parser}
	recordPasses := parser != nil && parser.currentMaterializationTiming() != nil
	if !recordPasses {
		// Statement keyword and precedence normalizers share one indexed walk.
		syntaxStats, reason := normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root, source, lang, &poller)
		if resultMaterializationShouldStop(reason) {
			return reason
		}
		normalizeTypeScriptRecoveredTernaryGenericCallRoot(root, source, lang)
		normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)
		normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
		if syntaxStats.typeScriptCompatibility.built {
			normalizeTypeScriptCompatibilityCandidates(syntaxStats.typeScriptCompatibility, root, source, lang)
		} else {
			normalizeTypeScriptCompatibility(root, source, lang)
		}
		normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
		if reason := parser.activeParseStopReason(); parseStopReasonIsActive(reason) {
			return reason
		}
		return parser.javaScriptTypeScriptCompatMemoryBudgetStopReason(arena)
	}
	run := func(name string, fn func() normalizationPassCounters) {
		parser.runNamedNormalizationPass(name, func() bool { return true }, fn)
	}
	runVoid := func(name string, fn func()) {
		run(name, func() normalizationPassCounters {
			fn()
			return normalizationPassCounters{}
		})
	}
	recordTypeScriptCompatSourceFlagMetrics(parser, typeScriptCompatSourceFlagsFor(source))

	if reason := recordTypeScriptCompatCandidateMetrics(parser, root, lang, &poller); resultMaterializationShouldStop(reason) {
		return reason
	}
	var syntaxStats javaScriptTypeScriptSyntaxNormalizationStats
	var haveSyntaxStats bool
	var syntaxStopReason ParseStopReason
	// ts_compat_index_walk drives the fused walk: dynamic-import leaf
	// retyping plus building the TypeScript-compatibility candidate index.
	// Renamed from ts_statement_keyword_leaves by the wave11 compat-sunset
	// (juniper), which removed the walk's empty_statement, existential_type,
	// statement-keyword, call-precedence, and unary/binary-precedence-
	// rotation work (confirmed dead: zero rewrites census, see the fused
	// walk's doc comment above) along with their per-pass metric recording.
	run("ts_compat_index_walk", func() normalizationPassCounters {
		syntaxStats, syntaxStopReason = normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root, source, lang, &poller)
		haveSyntaxStats = true
		parser.recordNormalizationMetric("ts_syntax_precedence_index", 1, syntaxStats.indexBuilds, syntaxStats.indexNodesVisited, 0)
		return normalizationPassCounters{}
	})
	// resultMaterializationShouldStop (not parseStopReasonIsActive): the fused
	// walk above may report ParseStopMemoryBudget, which parseStopReasonIsActive
	// deliberately excludes. Skip every remaining metrics pass below once the
	// walk itself has already bailed out — mirroring normalizeGoReturnedTreeCompatibilityWithCensus's
	// per-stage bail-out discipline (parser_result_go.go) — rather than running
	// more passes over a tree the walk stopped partway through.
	if resultMaterializationShouldStop(syntaxStopReason) {
		return syntaxStopReason
	}
	runVoid("ts_recovered_ternary_generic_call_root", func() {
		normalizeTypeScriptRecoveredTernaryGenericCallRoot(root, source, lang)
	})
	runVoid("ts_recovered_namespace_root", func() {
		normalizeTypeScriptRecoveredNamespaceRoot(root, source, lang)
	})
	runVoid("ts_top_level_declaration_bounds", func() {
		normalizeJavaScriptTopLevelDeclarationBounds(root, lang)
	})
	run("ts_type_compatibility", func() normalizationPassCounters {
		var stats typeScriptCompatibilityStats
		if haveSyntaxStats && syntaxStats.typeScriptCompatibility.built {
			stats = normalizeTypeScriptCompatibilityCandidatesWithStats(syntaxStats.typeScriptCompatibility, root, source, lang)
		} else {
			stats = normalizeTypeScriptCompatibilityWithStats(root, source, lang)
		}
		parser.recordNormalizationMetric("ts_type_identifier_alias_candidates", 1, 1, stats.identifierAliases.nodesVisited, stats.identifierAliases.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_import_keyword_candidates", 1, 1, stats.importKeywords.nodesVisited, stats.importKeywords.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_member_modifier_candidates", 1, 1, stats.memberModifiers.nodesVisited, stats.memberModifiers.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_enum_body_candidates", 1, 1, stats.enumBodies.nodesVisited, stats.enumBodies.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_child_candidates", 1, 1, stats.binaryChildren.nodesVisited, stats.binaryChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_generic_candidates", 1, 1, stats.binaryGenericChildren.nodesVisited, stats.binaryGenericChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_as_type_candidates", 1, 1, stats.binaryAsTypeChildren.nodesVisited, stats.binaryAsTypeChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_binary_fast_skip", 1, 1, stats.binaryFastSkipped.nodesVisited, 0)
		parser.recordNormalizationMetric("ts_type_call_child_candidates", 1, 1, stats.callChildren.nodesVisited, stats.callChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_call_instantiated_candidates", 1, 1, stats.callInstantiatedChildren.nodesVisited, stats.callInstantiatedChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_call_fast_skip", 1, 1, stats.callFastSkipped.nodesVisited, 0)
		parser.recordNormalizationMetric("ts_type_as_child_candidates", 1, 1, stats.asChildren.nodesVisited, stats.asChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_assertion_child_candidates", 1, 1, stats.typeAssertionChildren.nodesVisited, stats.typeAssertionChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_expression_statement_child_candidates", 1, 1, stats.expressionStatementChildren.nodesVisited, stats.expressionStatementChildren.nodesRewritten)
		parser.recordNormalizationMetric("ts_type_destructuring_pattern_candidates", 1, 1, stats.destructuringPatterns.nodesVisited, stats.destructuringPatterns.nodesRewritten)
		return stats.total
	})
	runVoid("ts_top_level_expression_bounds", func() {
		normalizeJavaScriptTopLevelExpressionStatementBounds(root, lang)
	})
	if reason := parser.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return reason
	}
	return parser.javaScriptTypeScriptCompatMemoryBudgetStopReason(arena)
}

type typeScriptCompatSourceFlags struct {
	hasSemicolon        bool
	hasKeywordStatement bool
	hasCallAngle        bool
	hasAsKeyword        bool
	hasClassKeyword     bool
	hasEnumKeyword      bool
	hasUnaryCandidate   bool
	hasBinaryCandidate  bool
	hasTypeKeyword      bool
	hasImportKeyword    bool
	hasImportType       bool
	hasMappedTypeSyntax bool
	hasIndexTypeSyntax  bool
	hasMemberModifier   bool
	hasNamespaceModule  bool
	hasOptionalChain    bool
}

func typeScriptCompatSourceFlagsFor(source []byte) typeScriptCompatSourceFlags {
	var flags typeScriptCompatSourceFlags
	for i := 0; i < len(source); {
		switch source[i] {
		case ';':
			flags.hasSemicolon = true
		case '?':
			if i+1 < len(source) && source[i+1] == '.' {
				flags.hasOptionalChain = true
			}
		case '<':
			flags.hasCallAngle = true
			flags.hasBinaryCandidate = true
		case '>', '=', '*', '/', '%', '&', '|', '^':
			flags.hasBinaryCandidate = true
		case '+', '-':
			flags.hasUnaryCandidate = true
			flags.hasBinaryCandidate = true
		case '!', '~':
			flags.hasUnaryCandidate = true
		case '[':
			flags.hasIndexTypeSyntax = true
			if typeScriptSourceRangeHasKeyword(source, i+1, typeScriptSourceBracketEnd(source, i+1), "in") {
				flags.hasMappedTypeSyntax = true
			}
		}
		if !isTypeScriptIdentifierStartByte(source[i]) {
			i++
			continue
		}
		start := i
		i++
		for i < len(source) && isTypeScriptIdentifierByte(source[i]) {
			i++
		}
		switch string(source[start:i]) {
		case "if", "while":
			flags.hasKeywordStatement = true
		case "type", "keyof", "as", "satisfies":
			flags.hasTypeKeyword = true
			if typeScriptBytesEqualString(source[start:i], "as") {
				flags.hasAsKeyword = true
			}
		case "import":
			flags.hasImportKeyword = true
			if typeScriptImportSourceLooksTypeLike(source, i) {
				flags.hasImportType = true
			}
		case "public", "private", "protected", "readonly", "static", "abstract", "async", "get", "set", "declare", "accessor", "override":
			flags.hasMemberModifier = true
		case "class":
			flags.hasClassKeyword = true
		case "enum":
			flags.hasEnumKeyword = true
		case "namespace", "module":
			flags.hasNamespaceModule = true
		case "delete", "typeof", "void", "await":
			flags.hasUnaryCandidate = true
		case "in", "instanceof":
			flags.hasBinaryCandidate = true
		}
		continue
	}
	return flags
}

func isTypeScriptIdentifierByte(ch byte) bool {
	return isTypeScriptIdentifierStartByte(ch) || (ch >= '0' && ch <= '9')
}

func typeScriptSourceBracketEnd(source []byte, pos int) int {
	for pos < len(source) {
		if source[pos] == ']' || source[pos] == '\n' || source[pos] == '\r' {
			return pos
		}
		pos++
	}
	return len(source)
}

func typeScriptSourceRangeHasKeyword(source []byte, start, end int, keyword string) bool {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	for i := start; i+len(keyword) <= end; i++ {
		if string(source[i:i+len(keyword)]) == keyword &&
			(i == 0 || !isTypeScriptIdentifierByte(source[i-1])) &&
			(i+len(keyword) >= len(source) || !isTypeScriptIdentifierByte(source[i+len(keyword)])) {
			return true
		}
	}
	return false
}

func typeScriptImportSourceLooksTypeLike(source []byte, pos int) bool {
	for pos < len(source) && isASCIIWhitespace(source[pos]) {
		pos++
	}
	if pos < len(source) && source[pos] == '(' {
		return true
	}
	return typeScriptSourceRangeHasKeyword(source, pos, pos+4, "type")
}

func recordTypeScriptCompatSourceFlagMetrics(parser *Parser, flags typeScriptCompatSourceFlags) {
	if parser == nil {
		return
	}
	record := func(name string, enabled bool) {
		parser.recordNormalizationMetric(name, 1, boolToUint64(enabled), 0, 0)
	}
	record("ts_source_has_semicolon", flags.hasSemicolon)
	record("ts_source_has_keyword_statement", flags.hasKeywordStatement)
	record("ts_source_has_call_angle", flags.hasCallAngle)
	record("ts_source_has_as_keyword", flags.hasAsKeyword)
	record("ts_source_has_class_keyword", flags.hasClassKeyword)
	record("ts_source_has_enum_keyword", flags.hasEnumKeyword)
	record("ts_source_has_unary_candidate", flags.hasUnaryCandidate)
	record("ts_source_has_binary_candidate", flags.hasBinaryCandidate)
	record("ts_source_has_type_keyword", flags.hasTypeKeyword)
	record("ts_source_has_import_keyword", flags.hasImportKeyword)
	record("ts_source_has_import_type", flags.hasImportType)
	record("ts_source_has_mapped_type_syntax", flags.hasMappedTypeSyntax)
	record("ts_source_has_index_type_syntax", flags.hasIndexTypeSyntax)
	record("ts_source_has_member_modifier", flags.hasMemberModifier)
	record("ts_source_has_namespace_module", flags.hasNamespaceModule)
	record("ts_source_has_optional_chain", flags.hasOptionalChain)
}

func boolToUint64(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

type typeScriptCompatCandidateMetrics struct {
	nodesVisited      uint64
	callExpressions   uint64
	unaryExpressions  uint64
	binaryExpressions uint64
	typeNodes         uint64
	importNodes       uint64
	memberNodes       uint64
	namespaceNodes    uint64
}

// recordTypeScriptCompatCandidateMetrics is diagnostic-only (only reached
// when parser.currentMaterializationTiming() != nil): it does its own
// separate full-tree walk purely to record per-category candidate counts
// before the fused walk runs. Threading poller through it (like every other
// whole-tree pass in this file) means enabling detailed normalization
// metrics can never itself create an unbounded, budget-blind walk.
func recordTypeScriptCompatCandidateMetrics(parser *Parser, root *Node, lang *Language, poller *parseStopPoller) ParseStopReason {
	if parser == nil || root == nil || lang == nil {
		return ParseStopNone
	}
	metrics, reason := typeScriptCompatCandidateMetricsFor(root, lang, poller)
	parser.recordNormalizationMetric("ts_candidates_index", 1, 1, metrics.nodesVisited, 0)
	parser.recordNormalizationMetric("ts_candidates_call_expression", 1, 1, metrics.callExpressions, 0)
	parser.recordNormalizationMetric("ts_candidates_unary_expression", 1, 1, metrics.unaryExpressions, 0)
	parser.recordNormalizationMetric("ts_candidates_binary_expression", 1, 1, metrics.binaryExpressions, 0)
	parser.recordNormalizationMetric("ts_candidates_type_nodes", 1, 1, metrics.typeNodes, 0)
	parser.recordNormalizationMetric("ts_candidates_import_nodes", 1, 1, metrics.importNodes, 0)
	parser.recordNormalizationMetric("ts_candidates_member_nodes", 1, 1, metrics.memberNodes, 0)
	parser.recordNormalizationMetric("ts_candidates_namespace_nodes", 1, 1, metrics.namespaceNodes, 0)
	return reason
}

func typeScriptCompatCandidateMetricsFor(root *Node, lang *Language, poller *parseStopPoller) (typeScriptCompatCandidateMetrics, ParseStopReason) {
	var metrics typeScriptCompatCandidateMetrics
	var stopReason ParseStopReason
	syms := typeScriptCompatCandidateSymbolsFor(lang)
	walkResultTreeUntil(root, func(n *Node) bool {
		if poller != nil {
			if reason := poller.poll(); resultMaterializationShouldStop(reason) {
				stopReason = reason
				return false
			}
		}
		metrics.nodesVisited++
		switch {
		case syms.hasCallExpression && n.symbol == syms.callExpression:
			metrics.callExpressions++
		case syms.hasUnaryExpression && n.symbol == syms.unaryExpression:
			metrics.unaryExpressions++
		case syms.hasBinaryExpression && n.symbol == syms.binaryExpression:
			metrics.binaryExpressions++
		case (syms.hasImportStatement && n.symbol == syms.importStatement) ||
			(syms.hasImportType && n.symbol == syms.importType) ||
			(syms.hasImportKeyword && n.symbol == syms.importKeyword):
			metrics.importNodes++
		case syms.hasInternalModule && n.symbol == syms.internalModule:
			metrics.namespaceNodes++
		case (syms.hasMethodDefinition && n.symbol == syms.methodDefinition) ||
			(syms.hasMethodSignature && n.symbol == syms.methodSignature) ||
			(syms.hasPropertySignature && n.symbol == syms.propertySignature) ||
			(syms.hasPublicFieldDefinition && n.symbol == syms.publicFieldDefinition):
			metrics.memberNodes++
		case (syms.hasTypeAnnotation && n.symbol == syms.typeAnnotation) ||
			(syms.hasTypeIdentifier && n.symbol == syms.typeIdentifier) ||
			(syms.hasTypeQuery && n.symbol == syms.typeQuery) ||
			(syms.hasMappedType && n.symbol == syms.mappedType) ||
			(syms.hasIndexSignature && n.symbol == syms.indexSignature) ||
			(syms.hasIndexedAccessType && n.symbol == syms.indexedAccessType):
			metrics.typeNodes++
		}
		return true
	})
	return metrics, stopReason
}

type typeScriptCompatCandidateSymbols struct {
	callExpression           Symbol
	hasCallExpression        bool
	unaryExpression          Symbol
	hasUnaryExpression       bool
	binaryExpression         Symbol
	hasBinaryExpression      bool
	importStatement          Symbol
	hasImportStatement       bool
	importType               Symbol
	hasImportType            bool
	importKeyword            Symbol
	hasImportKeyword         bool
	internalModule           Symbol
	hasInternalModule        bool
	methodDefinition         Symbol
	hasMethodDefinition      bool
	methodSignature          Symbol
	hasMethodSignature       bool
	propertySignature        Symbol
	hasPropertySignature     bool
	publicFieldDefinition    Symbol
	hasPublicFieldDefinition bool
	typeAnnotation           Symbol
	hasTypeAnnotation        bool
	typeIdentifier           Symbol
	hasTypeIdentifier        bool
	typeQuery                Symbol
	hasTypeQuery             bool
	mappedType               Symbol
	hasMappedType            bool
	indexSignature           Symbol
	hasIndexSignature        bool
	indexedAccessType        Symbol
	hasIndexedAccessType     bool
}

func typeScriptCompatCandidateSymbolsFor(lang *Language) typeScriptCompatCandidateSymbols {
	var syms typeScriptCompatCandidateSymbols
	syms.callExpression, syms.hasCallExpression = symbolByName(lang, "call_expression")
	syms.unaryExpression, syms.hasUnaryExpression = symbolByName(lang, "unary_expression")
	syms.binaryExpression, syms.hasBinaryExpression = symbolByName(lang, "binary_expression")
	syms.importStatement, syms.hasImportStatement = symbolByName(lang, "import_statement")
	syms.importType, syms.hasImportType = symbolByName(lang, "import_type")
	syms.importKeyword, syms.hasImportKeyword = symbolByName(lang, "import")
	syms.internalModule, syms.hasInternalModule = symbolByName(lang, "internal_module")
	syms.methodDefinition, syms.hasMethodDefinition = symbolByName(lang, "method_definition")
	syms.methodSignature, syms.hasMethodSignature = symbolByName(lang, "method_signature")
	syms.propertySignature, syms.hasPropertySignature = symbolByName(lang, "property_signature")
	syms.publicFieldDefinition, syms.hasPublicFieldDefinition = symbolByName(lang, "public_field_definition")
	syms.typeAnnotation, syms.hasTypeAnnotation = symbolByName(lang, "type_annotation")
	syms.typeIdentifier, syms.hasTypeIdentifier = symbolByName(lang, "type_identifier")
	syms.typeQuery, syms.hasTypeQuery = symbolByName(lang, "type_query")
	syms.mappedType, syms.hasMappedType = symbolByName(lang, "mapped_type")
	syms.indexSignature, syms.hasIndexSignature = symbolByName(lang, "index_signature")
	syms.indexedAccessType, syms.hasIndexedAccessType = symbolByName(lang, "indexed_access_type")
	return syms
}

func normalizeJavaScriptTopLevelObjectLiterals(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.Type(lang) != "program" {
		return
	}
	exprSym, exprNamed, ok := symbolMeta(lang, "expression_statement")
	if !ok {
		return
	}
	objectSym, objectNamed, ok := symbolMeta(lang, "object")
	if !ok {
		return
	}
	pairSym, pairNamed, ok := symbolMeta(lang, "pair")
	if !ok {
		return
	}
	propSym, _, ok := symbolMeta(lang, "property_identifier")
	if !ok {
		return
	}
	for i, child := range root.children {
		repl, ok := rewriteJavaScriptTopLevelObjectLiteral(child, lang, root.ownerArena, exprSym, exprNamed, objectSym, objectNamed, pairSym, pairNamed, propSym)
		if ok {
			root.children[i] = repl
		}
	}
}

func normalizeJavaScriptProgramStart(root *Node, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.Type(lang) != "program" {
		return
	}
	first, _ := firstAndLastNonNilChild(root.children)
	if first == nil {
		return
	}
	root.startByte = first.startByte
	root.startPoint = first.startPoint
}

func normalizeJavaScriptProgramEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "javascript" || root.endByte >= uint32(len(source)) {
		return
	}
	switch root.Type(lang) {
	case "program", "ERROR":
	default:
		return
	}
	tail := source[root.endByte:]
	if !bytesAreTrivia(tail) && !bytesAreJavaScriptStatementTerminatorTail(tail) {
		return
	}
	extendNodeEndTo(root, uint32(len(source)), source)
}

func bytesAreJavaScriptStatementTerminatorTail(b []byte) bool {
	seenSemicolon := false
	for _, c := range b {
		switch c {
		case ';':
			seenSemicolon = true
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return seenSemicolon
}

func normalizeJavaScriptTopLevelExpressionStatementBounds(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "program" {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if normalizeJavaScriptTopLevelBoundsFinalRefs(root, lang, func(name string) bool {
		return name == "expression_statement"
	}) {
		return
	}
	for _, child := range root.children {
		if child == nil || child.Type(lang) != "expression_statement" || len(child.children) == 0 {
			continue
		}
		first, last := firstAndLastNonNilChild(child.children)
		if first == nil || last == nil {
			continue
		}
		child.startByte = first.startByte
		child.startPoint = first.startPoint
		child.endByte = last.endByte
		child.endPoint = last.endPoint
	}
}

func normalizeJavaScriptTopLevelDeclarationBounds(root *Node, lang *Language) {
	if root == nil || lang == nil || root.Type(lang) != "program" {
		return
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return
	}
	if normalizeJavaScriptTopLevelBoundsFinalRefs(root, lang, func(name string) bool {
		switch name {
		case "lexical_declaration",
			"variable_declaration",
			"function_declaration",
			"generator_function_declaration",
			"class_declaration",
			"import_statement",
			"export_statement":
			return true
		default:
			return false
		}
	}) {
		return
	}
	for _, child := range root.children {
		if child == nil || len(child.children) == 0 {
			continue
		}
		switch child.Type(lang) {
		case "lexical_declaration",
			"variable_declaration",
			"function_declaration",
			"generator_function_declaration",
			"class_declaration",
			"import_statement",
			"export_statement":
		default:
			continue
		}
		first, last := firstAndLastNonNilChild(child.children)
		if first == nil || last == nil {
			continue
		}
		child.startByte = first.startByte
		child.startPoint = first.startPoint
		child.endByte = last.endByte
		child.endPoint = last.endPoint
	}
}

func normalizeJavaScriptTopLevelBoundsFinalRefs(root *Node, lang *Language, match func(string) bool) bool {
	view := resultMutableChildrenForMutation(root)
	if !view.hasFinalChildRefs() {
		return false
	}
	for i := 0; i < view.Len(); i++ {
		entry, ok := view.Entry(i)
		if !ok || !match(symbolTypeName(lang, stackEntryNodeSymbol(entry))) {
			continue
		}
		first, last, ok := firstAndLastStackEntryChild(root.ownerArena, entry)
		if !ok {
			continue
		}
		setStackEntryStart(entry, stackEntryNodeStartByte(first), stackEntryNodeStartPoint(first))
		setStackEntryEnd(entry, stackEntryNodeEndByte(last), stackEntryNodeEndPoint(last))
	}
	return true
}

func firstAndLastStackEntryChild(arena *nodeArena, entry stackEntry) (stackEntry, stackEntry, bool) {
	childCount := stackEntryNodeChildCount(entry)
	if childCount == 0 {
		return stackEntry{}, stackEntry{}, false
	}
	childAt := func(i int) (stackEntry, bool) {
		if parent := stackEntryPendingParent(entry); parent != nil {
			child := parent.childEntry(arena, i)
			return child, stackEntryHasNode(child)
		}
		if node := stackEntryNode(entry); node != nil {
			return nodeChildEntryAtNoMaterialize(node, i)
		}
		return stackEntry{}, false
	}
	firstIdx := 0
	first, ok := childAt(firstIdx)
	for !ok && firstIdx+1 < childCount {
		firstIdx++
		first, ok = childAt(firstIdx)
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	lastIdx := childCount - 1
	last, ok := childAt(lastIdx)
	for !ok && lastIdx > firstIdx {
		lastIdx--
		last, ok = childAt(lastIdx)
	}
	if !ok {
		return stackEntry{}, stackEntry{}, false
	}
	return first, last, true
}

func normalizeJavaScriptTrailingContinueComments(root *Node, source []byte, lang *Language, poller *parseStopPoller) ParseStopReason {
	if root == nil || lang == nil || lang.Name != "javascript" || len(source) == 0 {
		return ParseStopNone
	}
	// Source-level gate: this pass only acts on continue_statement nodes
	// with trailing comments. If source has neither "continue" nor "//",
	// no candidate exists and the walk is wasted on large files like jquery.
	if !bytes.Contains(source, []byte("continue")) || !bytes.Contains(source, []byte("//")) {
		return ParseStopNone
	}
	var stopReason ParseStopReason
	walkResultTreeUntil(root, func(n *Node) bool {
		if poller != nil {
			if reason := poller.poll(); resultMaterializationShouldStop(reason) {
				stopReason = reason
				return false
			}
		}
		normalizeJavaScriptTrailingContinueCommentSiblings(n, source, lang)
		return true
	})
	return stopReason
}

func normalizeJavaScriptTrailingContinueCommentSiblings(parent *Node, source []byte, lang *Language) {
	if parent == nil || len(parent.children) < 3 || parent.Type(lang) != "statement_block" {
		return
	}
	for i := 1; i+1 < len(parent.children); i++ {
		if comment, ok := extractJavaScriptTrailingContinueComment(parent.children[i], source, lang); ok {
			insertJavaScriptStatementBlockComment(parent, i, comment)
			i++
			continue
		}
		stmt := parent.children[i]
		if stmt == nil || stmt.Type(lang) != "if_statement" || len(stmt.children) < 3 {
			continue
		}
		branch := stmt.children[len(stmt.children)-1]
		comment, ok := extractJavaScriptTrailingContinueComment(branch, source, lang)
		if !ok {
			continue
		}
		stmt.endByte = branch.endByte
		stmt.endPoint = branch.endPoint
		insertJavaScriptStatementBlockComment(parent, i, comment)
		i++
	}
}

func extractJavaScriptTrailingContinueComment(node *Node, source []byte, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "continue_statement" || len(node.children) < 3 {
		return nil, false
	}
	comment := node.children[len(node.children)-1]
	if comment == nil || comment.Type(lang) != "comment" || comment.startByte >= comment.endByte {
		return nil, false
	}
	if int(comment.endByte) > len(source) || !bytes.HasPrefix(source[comment.startByte:comment.endByte], []byte("//")) {
		return nil, false
	}
	prev := node.children[len(node.children)-2]
	if prev == nil || prev.endByte > comment.startByte || bytesContainLineBreak(source[prev.endByte:comment.startByte]) {
		return nil, false
	}
	node.children = node.children[:len(node.children)-1]
	fieldIDs := node.fieldIDs()
	fieldSources := node.fieldSources()
	if len(fieldIDs) > len(node.children) {
		fieldIDs = fieldIDs[:len(node.children)]
		if len(fieldSources) > len(node.children) {
			fieldSources = fieldSources[:len(node.children)]
		}
		node.setFieldMetadata(fieldIDs, fieldSources)
	}
	node.endByte = prev.endByte
	node.endPoint = prev.endPoint
	return comment, true
}

func insertJavaScriptStatementBlockComment(parent *Node, childIdx int, comment *Node) {
	if parent == nil || comment == nil || childIdx < 0 || childIdx >= len(parent.children) {
		return
	}
	parent.children = append(parent.children[:childIdx+1], append([]*Node{comment}, parent.children[childIdx+1:]...)...)
	parentFieldIDs := parent.fieldIDs()
	if len(parentFieldIDs) > 0 {
		fieldIDs := append([]FieldID(nil), parentFieldIDs[:childIdx+1]...)
		fieldIDs = append(fieldIDs, 0)
		fieldIDs = append(fieldIDs, parentFieldIDs[childIdx+1:]...)
		parentFieldSources := parent.fieldSources()
		if len(parentFieldSources) > 0 {
			fieldSources := append([]uint8(nil), parentFieldSources[:childIdx+1]...)
			fieldSources = append(fieldSources, fieldSourceNone)
			fieldSources = append(fieldSources, parentFieldSources[childIdx+1:]...)
			parent.setFieldMetadata(fieldIDs, fieldSources)
		} else {
			parent.setFieldIDs(fieldIDs)
		}
	}
	populateParentNode(parent, parent.children)
}

type javaScriptTypeScriptSyntaxNormalizationStats struct {
	typeScriptCompatibility typeScriptCompatibilityCandidates
	indexBuilds             uint64
	indexNodesVisited       uint64
}

// normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats
// drives the fused walk (rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex).
// poller, when non-nil, is threaded through that walk — a real whole-tree
// pass — so a timeout, cancellation, or runtime memory-budget trip aborts
// the remaining work instead of running unbounded to completion. poller ==
// nil (every caller other than normalizeJavaScriptCompatibility /
// normalizeTypeScriptTreeCompatibilityWithParser) preserves the exact prior
// unbounded behavior.
//
// The wave11 compat-sunset (juniper) removed this function's empty_statement,
// existential_type, statement-keyword (if/while), call-precedence, and
// unary/binary-precedence-rotation handling: a census over ~23MB of real
// JS/TS/TSX (undici.js, TypeScript's own checker.ts/parser.ts/utilities.ts,
// real TSX) plus the 5003ac64 regression snippets and independent adversarial
// precedence chains showed zero rewrites from any of those six passes —
// later grammargen table fixes made the GLR tables produce the already-
// correct shape directly, so the post-hoc rewrite passes had become dead
// weight on every JS/TS/TSX parse. What remains — dynamic-import leaf
// retyping and the TypeScript-compatibility candidate index — still does
// real, live work and keeps the exact same containment discipline.
func normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats(root *Node, source []byte, lang *Language, poller *parseStopPoller) (javaScriptTypeScriptSyntaxNormalizationStats, ParseStopReason) {
	var stats javaScriptTypeScriptSyntaxNormalizationStats
	if root == nil || lang == nil {
		return stats, ParseStopNone
	}
	switch lang.Name {
	case "javascript", "typescript", "tsx":
	default:
		return stats, ParseStopNone
	}

	// Source-level gate: only enable the optional-chain leaf handler when "?."
	// actually appears in the file, mirroring the standalone pass's gate so we
	// don't slow down files that have no chance of matching.
	optionalChainSym, hasOptionalChainSym := symbolByName(lang, "optional_chain")
	optionalChainTokenSym, hasOptionalChainTokenSym := symbolByName(lang, "?.")
	optionalChainTokenNamed := hasOptionalChainTokenSym && symbolIsNamed(lang, optionalChainTokenSym)
	enableOptionalChain := hasOptionalChainSym && bytes.Contains(source, []byte("?."))
	dynamicImportSym, hasDynamicImport := lang.symbolByNameAndNamed("import", true)
	importKeywordSym, hasImportKeyword := lang.symbolByNameAndNamed("import", false)
	enableDynamicImport := hasDynamicImport && hasImportKeyword && bytes.Contains(source, []byte("import"))

	var typeScriptCtx *typeScriptNormalizationContext
	if lang.Name == "typescript" || lang.Name == "tsx" {
		sourceFlags := typeScriptCompatSourceFlagsFor(source)
		if ctx, ok := newTypeScriptNormalizationContextForSourceFlags(source, lang, sourceFlags); ok {
			typeScriptCtx = &ctx
		}
	}

	index, reason := rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex(
		root, source, lang,
		optionalChainSym, optionalChainTokenSym, optionalChainTokenNamed, enableOptionalChain,
		dynamicImportSym, importKeywordSym, enableDynamicImport,
		typeScriptCtx,
		poller,
	)
	stats.typeScriptCompatibility = index.typeScriptCompatibility
	stats.indexBuilds += index.builds
	stats.indexNodesVisited += index.nodesVisited
	return stats, reason
}

// rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex
// walks the whole JS/TS/TSX result tree once, doing dynamic-import leaf
// retyping and building the TypeScript-compatibility candidate index (see
// normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats's
// doc comment for the wave11 census that removed this walk's former
// empty_statement, existential_type, statement-keyword, call-precedence, and
// unary/binary-precedence-rotation work — all confirmed dead: zero rewrites
// across the census corpus).
func rewriteJavaScriptTypeScriptStatementKeywordsCallPrecedenceAndBuildUnaryBinaryIndex(
	root *Node,
	source []byte,
	lang *Language,
	optionalChainSym Symbol,
	optionalChainTokenSym Symbol,
	optionalChainTokenNamed bool,
	enableOptionalChain bool,
	dynamicImportSym Symbol,
	importKeywordSym Symbol,
	enableDynamicImport bool,
	typeScriptCtx *typeScriptNormalizationContext,
	poller *parseStopPoller,
) (javaScriptTypeScriptUnaryBinaryPrecedenceIndex, ParseStopReason) {
	var index javaScriptTypeScriptUnaryBinaryPrecedenceIndex
	if root == nil {
		return index, ParseStopNone
	}
	// If none of the symbols this walk targets exists in the language, there
	// is nothing to do; returning early avoids materializing final-ref
	// children for callers (e.g. mock-lang unit tests) that exercise only
	// other compat passes.
	if !enableDynamicImport && typeScriptCtx == nil {
		return index, ParseStopNone
	}
	index.builds = 1
	if typeScriptCtx != nil {
		index.typeScriptCompatibility.built = true
	}
	// optional_chain intentionally retains its visible "?." anonymous-token
	// child: C tree-sitter emits (optional_chain (?.)) with childCount==1.
	// The parser already builds that child; no normalization is applied here.
	_ = enableOptionalChain
	_ = optionalChainSym
	_ = optionalChainTokenSym
	_ = optionalChainTokenNamed
	enableTypeScriptChildCompatibility := typeScriptCtx != nil &&
		(typeScriptCtx.canRewriteGenericCalls ||
			typeScriptCtx.canRewriteInstantiatedCalls ||
			typeScriptCtx.canRewriteAsExpressions ||
			typeScriptCtx.canRewriteGenericArrows ||
			typeScriptCtx.canRewriteClassDeclarations ||
			typeScriptCtx.canRewriteDestructuring)
	// stopReason/stopped let this walk bail out mid-traversal — checked once
	// per node visited, at the coarse, poller-throttled cadence poll() applies
	// (parseStopPollMask, the same ~1024-node stride walkGoCompatSubtree uses,
	// see parser_result_go_compat.go) — on timeout, cancellation, or a runtime
	// memory-budget trip. Every loop below re-checks the cheap `stopped` flag
	// before doing further per-child work, so a trip anywhere in the
	// recursion unwinds the whole walk immediately instead of continuing to
	// materialize a partial candidate index (see the 2026-07-12
	// gocompat-walk-containment-gap finding, which flagged this fused walk as
	// sharing the Go compat walk's containment gap: before this, this walk
	// polled nothing at all — no timeout, cancellation, or memory-budget
	// check of any kind).
	// stopReason starts at ParseStopNone (not the ParseStopReason zero value,
	// which is "" — distinct from ParseStopNone's "none") so a walk that
	// completes without ever tripping the poller reports ParseStopNone, not
	// "": the pre-wave11 code achieved this by having the caller
	// (normalizeJavaScriptTypeScriptStatementKeywordsAndPrecedenceWithDetailedStats)
	// return a literal ParseStopNone on its success path instead of
	// forwarding this walk's raw return value; that caller now forwards this
	// return value directly, so the normalization has to happen here.
	stopReason := ParseStopNone
	stopped := false
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil || stopped {
			return
		}
		if poller != nil {
			if reason := poller.poll(); resultMaterializationShouldStop(reason) {
				stopReason = reason
				stopped = true
				return
			}
		}
		index.nodesVisited++
		if enableDynamicImport && n.symbol == dynamicImportSym {
			normalizeJavaScriptTypeScriptDynamicImportLeafWithSymbolChanged(n, importKeywordSym)
		}
		collectTypeScriptCompatibilityNodeCandidate(&index.typeScriptCompatibility, n, typeScriptCtx)

		if n.childIndex <= finalChildSidecarIndexBase && n.ownerArena != nil {
			childCount := resultChildCount(n)
			for i := 0; i < childCount; i++ {
				if stopped {
					break
				}
				child := resultChildAt(n, i)
				if child == nil {
					continue
				}
				walk(child)
				if stopped {
					break
				}
			}
			return
		}

		children := n.children
		for i, child := range children {
			if stopped {
				break
			}
			if child == nil {
				continue
			}
			if enableTypeScriptChildCompatibility {
				switch child.symbol {
				case typeScriptCtx.binaryExpressionSym:
					if (typeScriptCtx.canRewriteGenericCalls && typeScriptBinaryOperatorCouldBeGenericCall(child, typeScriptCtx)) ||
						(typeScriptCtx.canRewriteAsExpressions && typeScriptBinaryOperatorCouldBeAsTypeChain(child, typeScriptCtx)) {
						index.typeScriptCompatibility.append(typeScriptCompatibilityCandidate{
							kind: typeScriptCompatibilityCandidateChild,
							child: javaScriptTypeScriptPrecedenceCandidate{
								parent:     n,
								childIndex: i,
							},
						})
					}
				case typeScriptCtx.callSym:
					if typeScriptCtx.canRewriteInstantiatedCalls && typeScriptCallCouldBeInstantiated(child, typeScriptCtx) {
						index.typeScriptCompatibility.append(typeScriptCompatibilityCandidate{
							kind: typeScriptCompatibilityCandidateChild,
							child: javaScriptTypeScriptPrecedenceCandidate{
								parent:     n,
								childIndex: i,
							},
						})
					}
				case typeScriptCtx.asExpressionSym:
					if typeScriptCtx.canRewriteAsExpressions && typeScriptAsAssignmentOrTernaryCandidate(child, typeScriptCtx) {
						index.typeScriptCompatibility.append(typeScriptCompatibilityCandidate{
							kind: typeScriptCompatibilityCandidateChild,
							child: javaScriptTypeScriptPrecedenceCandidate{
								parent:     n,
								childIndex: i,
							},
						})
					}
				case typeScriptCtx.typeAssertionSym:
					if typeScriptCtx.canRewriteGenericArrows && typeScriptGenericArrowTypeAssertionCandidate(child, typeScriptCtx) {
						index.typeScriptCompatibility.append(typeScriptCompatibilityCandidate{
							kind: typeScriptCompatibilityCandidateChild,
							child: javaScriptTypeScriptPrecedenceCandidate{
								parent:     n,
								childIndex: i,
							},
						})
					}
				case typeScriptCtx.expressionStatementSym:
					if typeScriptCtx.canRewriteClassDeclarations && typeScriptClassExpressionStatementCandidate(child, typeScriptCtx) {
						index.typeScriptCompatibility.append(typeScriptCompatibilityCandidate{
							kind: typeScriptCompatibilityCandidateChild,
							child: javaScriptTypeScriptPrecedenceCandidate{
								parent:     n,
								childIndex: i,
							},
						})
					}
				case typeScriptCtx.nonNullExpressionSym:
					if typeScriptCtx.canRewriteDestructuring {
						index.typeScriptCompatibility.append(typeScriptCompatibilityCandidate{
							kind: typeScriptCompatibilityCandidateChild,
							child: javaScriptTypeScriptPrecedenceCandidate{
								parent:     n,
								childIndex: i,
							},
						})
					}
				}
			}
			walk(child)
			if stopped {
				break
			}
		}
	}
	walk(root)
	return index, stopReason
}

func normalizeJavaScriptTypeScriptDynamicImportLeafWithSymbolChanged(node *Node, importKeywordSym Symbol) bool {
	if node == nil || resultChildCount(node) != 0 || node.endByte <= node.startByte {
		return false
	}
	child := newLeafNodeInArena(node.ownerArena, importKeywordSym, false, node.startByte, node.endByte, node.startPoint, node.endPoint)
	replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, []*Node{child}))
	return true
}

type javaScriptTypeScriptPrecedenceCandidate struct {
	parent     *Node
	childIndex int
}

func javaScriptTypeScriptPrecedenceCandidateNode(candidate javaScriptTypeScriptPrecedenceCandidate) *Node {
	if candidate.parent == nil || candidate.childIndex < 0 {
		return nil
	}
	if candidate.childIndex >= resultChildCount(candidate.parent) {
		return nil
	}
	return resultChildAt(candidate.parent, candidate.childIndex)
}

func replaceJavaScriptTypeScriptPrecedenceCandidate(candidate javaScriptTypeScriptPrecedenceCandidate, replacement *Node) bool {
	if candidate.parent == nil || candidate.childIndex < 0 || replacement == nil {
		return false
	}
	view := resultMutableChildrenForMutation(candidate.parent)
	if view.hasFinalChildRefs() {
		if candidate.childIndex >= view.Len() {
			return false
		}
		return view.ReplaceFinalRefRangeWithNode(candidate.childIndex, candidate.childIndex+1, replacement)
	}
	if candidate.childIndex >= len(candidate.parent.children) {
		return false
	}
	candidate.parent.children[candidate.childIndex] = replacement
	setNodeParentLink(replacement, candidate.parent, candidate.childIndex)
	return true
}

// javaScriptTypeScriptUnaryBinaryPrecedenceIndex is the fused walk's return
// type. The wave11 compat-sunset (juniper) removed its emptyStatement,
// existentialType, statementKeyword, call, unaryCandidates, and
// binaryCandidates fields along with the dead rewrite passes that populated
// them (see the fused walk's doc comment above); typeScriptCompatibility,
// builds, and nodesVisited are still real, live bookkeeping.
type javaScriptTypeScriptUnaryBinaryPrecedenceIndex struct {
	typeScriptCompatibility typeScriptCompatibilityCandidates
	builds                  uint64
	nodesVisited            uint64
}

func rewriteJavaScriptTopLevelObjectLiteral(node *Node, lang *Language, arena *nodeArena, exprSym Symbol, exprNamed bool, objectSym Symbol, objectNamed bool, pairSym Symbol, pairNamed bool, propSym Symbol) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "statement_block" || len(node.children) != 3 {
		return nil, false
	}
	if node.children[0] == nil || node.children[0].Type(lang) != "{" || node.children[2] == nil || node.children[2].Type(lang) != "}" {
		return nil, false
	}
	label := node.children[1]
	if label == nil || label.Type(lang) != "labeled_statement" || len(label.children) != 3 {
		return nil, false
	}
	key := label.children[0]
	colon := label.children[1]
	valueStmt := label.children[2]
	if key == nil || key.Type(lang) != "statement_identifier" || colon == nil || colon.Type(lang) != ":" || valueStmt == nil || valueStmt.Type(lang) != "expression_statement" || len(valueStmt.children) != 1 || valueStmt.children[0] == nil {
		return nil, false
	}
	pair := newParentNodeInArena(arena, pairSym, pairNamed, []*Node{
		aliasedNodeInArena(arena, lang, key, propSym),
		colon,
		valueStmt.children[0],
	}, nil, 0)
	for fieldIdx, fieldName := range lang.FieldNames {
		switch fieldName {
		case "key":
			ensureNodeFieldStorage(pair, len(pair.children))
			pair.fieldIDs()[0] = FieldID(fieldIdx)
			pair.fieldSources()[0] = fieldSourceDirect
		case "value":
			ensureNodeFieldStorage(pair, len(pair.children))
			pair.fieldIDs()[2] = FieldID(fieldIdx)
			pair.fieldSources()[2] = fieldSourceDirect
		}
	}
	object := newParentNodeInArena(arena, objectSym, objectNamed, []*Node{
		node.children[0],
		pair,
		node.children[2],
	}, nil, 0)
	return newParentNodeInArena(arena, exprSym, exprNamed, []*Node{object}, nil, 0), true
}
