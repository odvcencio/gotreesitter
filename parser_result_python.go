package gotreesitter

import (
	"bytes"
	"time"
)

func normalizePythonCompatibilityWithParser(root *Node, source []byte, parser *Parser, lang *Language) {
	if len(source) == 0 {
		return
	}
	var start time.Time
	if parser != nil {
		start = time.Now()
		defer func() {
			parser.normalizationStats.nanos += time.Since(start).Nanoseconds()
		}()
	}
	sourceFlags := pythonCompatibilitySourceFlagsFor(source)
	// Postorder passes: trailingSelfCalls + printChevron. Run separately
	// because they require a different walk order.
	parser.runNormalizationPass(func() bool {
		return sourceFlags.trailingSelfCalls
	}, func() normalizationPassCounters {
		return normalizePythonTrailingSelfCalls(root, source, lang)
	})
	parser.runNormalizationPass(func() bool {
		return sourceFlags.printChevron
	}, func() normalizationPassCounters {
		return normalizePythonPrintStatements(root, source, lang)
	})
	parser.runNormalizationPass(func() bool {
		return sourceFlags.fStringPattern
	}, func() normalizationPassCounters {
		return normalizePythonInterpolationPatterns(root, lang)
	})
	// Fused preorder block: collapsed-keyword (pass/continue/break),
	// inline-return/raise/yield blocks, inline-tuple-expression blocks, and
	// assignment-right expression lists all share a preorder walk. Doing them
	// together in ONE walkResultTree call eliminates ~7× walk overhead on
	// large Python files. Each logical pass still increments passesChecked/
	// passesRun so observability is preserved.
	normalizePythonFusedPreorder(root, source, parser, lang, sourceFlags)
	parser.runNormalizationPass(func() bool {
		return sourceFlags.wildcardImport
	}, func() normalizationPassCounters {
		return normalizePythonStarCollapsedChildrenWithStats(root, source, lang)
	})
	parser.runNormalizationPass(func() bool {
		return sourceFlags.asPattern
	}, func() normalizationPassCounters {
		return normalizePythonAsPatternTargetIdentifiers(root, source, lang)
	})
	parser.runNormalizationPass(func() bool {
		return sourceFlags.continuationEscape
	}, func() normalizationPassCounters {
		return normalizePythonStringContinuationEscapes(root, source, lang)
	})
}

// pythonCollapsedKeywordSetup holds the resolved symbols for one collapsed-
// keyword preorder pass (e.g. pass_statement/"pass"). Used inside the fused
// dispatcher to avoid re-resolving symbols per node.
type pythonCollapsedKeywordSetup struct {
	active       bool
	statementSym Symbol
	keywordSym   Symbol
	keywordNamed bool
	keyword      string
}

func newPythonCollapsedKeywordSetup(lang *Language, statementName, keywordName string) pythonCollapsedKeywordSetup {
	statementSym, statementOK := lang.symbolByNameAndNamed(statementName, true)
	if !statementOK {
		statementSym, statementOK = symbolByName(lang, statementName)
	}
	keywordSym, keywordOK := lang.symbolByNameAndNamed(keywordName, false)
	if !keywordOK {
		keywordSym, keywordOK = symbolByName(lang, keywordName)
	}
	if !statementOK || !keywordOK {
		return pythonCollapsedKeywordSetup{}
	}
	return pythonCollapsedKeywordSetup{
		active:       true,
		statementSym: statementSym,
		keywordSym:   keywordSym,
		keywordNamed: symbolIsNamed(lang, keywordSym),
		keyword:      keywordName,
	}
}

// normalizePythonFusedPreorder runs eight Python preorder normalization passes
// in a SINGLE walkResultTree. The original per-pass functions remain available
// (and individually tested) — this is a perf-only fusion. Per-pass counters
// (passesChecked/passesRun) are emulated to preserve observability.
//
// Fused passes: collapsed-keyword × 3 (pass/continue/break), inline-return-
// blocks, inline-raise-blocks, assignment-right-expression-lists, inline-yield-
// blocks, inline-tuple-expression-blocks. All operate on the same preorder
// traversal and target mutually-distinguishable node shapes, so per-node
// dispatch is cheap.
func normalizePythonFusedPreorder(root *Node, source []byte, parser *Parser, lang *Language, flags pythonCompatibilitySourceFlags) {
	if root == nil || lang == nil {
		// Still account for the 8 pass checks so passesChecked stays stable.
		if parser != nil {
			parser.normalizationStats.passesChecked += 8
		}
		return
	}

	// Resolve symbols and setup per active flag. Inactive flags leave their
	// setups zero-valued (active=false), so the per-node dispatch skips them.
	passCK := pythonCollapsedKeywordSetup{}
	continueCK := pythonCollapsedKeywordSetup{}
	breakCK := pythonCollapsedKeywordSetup{}
	if flags.passWord {
		passCK = newPythonCollapsedKeywordSetup(lang, "pass_statement", "pass")
	}
	if flags.continueWord {
		continueCK = newPythonCollapsedKeywordSetup(lang, "continue_statement", "continue")
	}
	if flags.breakWord {
		breakCK = newPythonCollapsedKeywordSetup(lang, "break_statement", "break")
	}

	var returnStmtSym Symbol
	hasReturn := false
	if flags.returnWord {
		var ok bool
		returnStmtSym, ok = lang.symbolByNameAndNamed("return_statement", true)
		if !ok {
			returnStmtSym, ok = symbolByName(lang, "return_statement")
		}
		hasReturn = ok
	}

	var raiseStmtSym Symbol
	hasRaise := false
	if flags.raiseWord {
		var ok bool
		raiseStmtSym, ok = lang.symbolByNameAndNamed("raise_statement", true)
		if !ok {
			raiseStmtSym, ok = symbolByName(lang, "raise_statement")
		}
		hasRaise = ok
	}

	var patternListSym, expressionListSym Symbol
	var expressionListNamed bool
	hasAssignment := false
	if flags.assignmentList {
		var ok1, ok2 bool
		patternListSym, ok1 = symbolByName(lang, "pattern_list")
		expressionListSym, ok2 = symbolByName(lang, "expression_list")
		if ok1 && ok2 {
			hasAssignment = true
			expressionListNamed = symbolIsNamed(lang, expressionListSym)
		}
	}

	var yieldSym Symbol
	hasYield := false
	if flags.yieldWord {
		var ok bool
		yieldSym, ok = lang.symbolByNameAndNamed("yield", true)
		hasYield = ok
	}

	var tupleExprSym Symbol
	var tupleExprNamed bool
	hasTuple := false
	if flags.comma {
		var ok bool
		tupleExprSym, ok = symbolByName(lang, "tuple_expression")
		if ok {
			hasTuple = true
			tupleExprNamed = symbolIsNamed(lang, tupleExprSym)
		}
	}

	// Always count 8 passes as checked. Count each as "run" only when its
	// flag is set and its symbol resolution succeeded — mirroring the gating
	// in the original runNormalizationPass(flag, fn) call sites.
	if parser != nil {
		parser.normalizationStats.passesChecked += 8
		ranCount := uint64(0)
		if passCK.active {
			ranCount++
		}
		if continueCK.active {
			ranCount++
		}
		if breakCK.active {
			ranCount++
		}
		if hasReturn && flags.returnWord {
			ranCount++
		}
		if hasRaise && flags.raiseWord {
			ranCount++
		}
		if hasAssignment {
			ranCount++
		}
		if hasYield && flags.yieldWord {
			ranCount++
		}
		if hasTuple && flags.comma {
			ranCount++
		}
		parser.normalizationStats.passesRun += ranCount
	}

	// Short-circuit if no pass is actually active.
	if !(passCK.active || continueCK.active || breakCK.active || hasReturn || hasRaise || hasAssignment || hasYield || hasTuple) {
		return
	}

	var visited, rewritten uint64
	walkResultTree(root, func(n *Node) {
		visited++
		childCount := resultChildCount(n)

		// Collapsed-keyword passes fire on leaf nodes whose source bytes
		// exactly match a keyword. Check this before reading n.Type to avoid
		// the lookup for the (common) leaf case.
		if childCount == 0 && len(source) > 0 && n.startByte < n.endByte && int(n.endByte) <= len(source) {
			srcRange := source[n.startByte:n.endByte]
			if passCK.active && string(srcRange) == "pass" {
				if applyCollapsedKeywordRewrite(n, lang, passCK) {
					rewritten++
				}
				return
			}
			if continueCK.active && string(srcRange) == "continue" {
				if applyCollapsedKeywordRewrite(n, lang, continueCK) {
					rewritten++
				}
				return
			}
			if breakCK.active && string(srcRange) == "break" {
				if applyCollapsedKeywordRewrite(n, lang, breakCK) {
					rewritten++
				}
				return
			}
			return
		}

		nodeType := n.Type(lang)

		// Assignment rewrite — exclusive of block rewrites (different shape).
		if hasAssignment && nodeType == "assignment" {
			sawEquals := false
			for i := 0; i < childCount; i++ {
				child := resultChildAt(n, i)
				if child == nil {
					continue
				}
				if child.Type(lang) == "=" {
					sawEquals = true
					continue
				}
				if sawEquals && child.symbol == patternListSym {
					child.symbol = expressionListSym
					child.setNamed(expressionListNamed)
					rewritten++
					return
				}
			}
			return
		}

		// Block-targeted single-line rewrites: raise, yield, return, tuple.
		// They are mutually exclusive on first-child Type, so a single switch
		// covers all four cases.
		if nodeType != "block" || n.startPoint.Row != n.endPoint.Row || childCount < 1 {
			return
		}
		first := resultChildAt(n, 0)
		if first == nil {
			return
		}
		last := resultChildAt(n, childCount-1)
		if last == nil {
			return
		}
		firstType := first.Type(lang)

		switch {
		case hasRaise && firstType == "raise":
			children := resultChildSliceForMutation(n)
			stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
			stmt := newParentNodeInArena(n.ownerArena, raiseStmtSym, true, stmtChildren, nil, 0)
			stmt.startByte = first.startByte
			stmt.endByte = last.endByte
			stmt.startPoint = first.startPoint
			stmt.endPoint = last.endPoint
			stmt.parent = n
			stmt.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
			rewritten++
		case hasYield && firstType == "yield" && !first.IsNamed():
			children := resultChildSliceForMutation(n)
			stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
			stmt := newParentNodeInArena(n.ownerArena, yieldSym, true, stmtChildren, nil, 0)
			stmt.startByte = first.startByte
			stmt.endByte = last.endByte
			stmt.startPoint = first.startPoint
			stmt.endPoint = last.endPoint
			stmt.parent = n
			stmt.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
			rewritten++
		case hasReturn && firstType == "return":
			children := resultChildSliceForMutation(n)
			stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
			stmt := newParentNodeInArena(n.ownerArena, returnStmtSym, true, stmtChildren, nil, 0)
			stmt.startByte = first.startByte
			stmt.endByte = last.endByte
			stmt.startPoint = first.startPoint
			stmt.endPoint = last.endPoint
			stmt.parent = n
			stmt.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
			rewritten++
		case hasTuple && childCount >= 3:
			hasComma := false
			for i := 0; i < childCount; i++ {
				child := resultChildAt(n, i)
				if child != nil && child.Type(lang) == "," {
					hasComma = true
					break
				}
			}
			if !hasComma {
				return
			}
			children := resultChildSliceForMutation(n)
			stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
			stmt := newParentNodeInArena(n.ownerArena, tupleExprSym, tupleExprNamed, stmtChildren, nil, 0)
			stmt.startByte = children[0].startByte
			stmt.endByte = children[len(children)-1].endByte
			stmt.startPoint = children[0].startPoint
			stmt.endPoint = children[len(children)-1].endPoint
			stmt.parent = n
			stmt.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
			rewritten++
		}
	})

	if parser != nil {
		parser.normalizationStats.nodesVisited += visited
		parser.normalizationStats.nodesRewritten += rewritten
	}
}

// applyCollapsedKeywordRewrite mirrors the inner switch of
// normalizePythonCollapsedKeywordStatementWithStats for use inside the fused
// dispatcher. Returns true on rewrite.
func applyCollapsedKeywordRewrite(n *Node, lang *Language, ck pythonCollapsedKeywordSetup) bool {
	switch {
	case n.symbol == ck.statementSym:
		child := newLeafNodeInArena(n.ownerArena, ck.keywordSym, ck.keywordNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
		child.parent = n
		child.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
		return true
	case n.Type(lang) == "block":
		child := newLeafNodeInArena(n.ownerArena, ck.keywordSym, ck.keywordNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
		stmt := newParentNodeInArena(n.ownerArena, ck.statementSym, true, []*Node{child}, nil, 0)
		stmt.startByte = n.startByte
		stmt.endByte = n.endByte
		stmt.startPoint = n.startPoint
		stmt.endPoint = n.endPoint
		stmt.parent = n
		stmt.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
		return true
	}
	return false
}

type pythonCompatibilitySourceFlags struct {
	trailingSelfCalls  bool
	printChevron       bool
	fStringPattern     bool
	passWord           bool
	continueWord       bool
	breakWord          bool
	returnWord         bool
	raiseWord          bool
	assignmentList     bool
	yieldWord          bool
	comma              bool
	wildcardImport     bool
	asPattern          bool
	continuationEscape bool
}

func pythonCompatibilitySourceFlagsFor(source []byte) pythonCompatibilitySourceFlags {
	var flags pythonCompatibilitySourceFlags
	for i := 0; i < len(source); {
		if !flags.continuationEscape && pythonSourceContinuationEscapeAt(source, i) {
			flags.continuationEscape = true
		}
		switch source[i] {
		case '*':
			flags.wildcardImport = true
			i++
			continue
		case '#':
			next := pythonSkipLineComment(source, i+1)
			if !flags.continuationEscape && pythonSourceRangeContainsContinuationEscape(source, i, next) {
				flags.continuationEscape = true
			}
			i = next
			continue
		case '\'', '"':
			start := pythonStringPrefixStart(source, i)
			validPrefix := start < i && (start == 0 || !pythonIdentifierByte(source[start-1]))
			hasFPrefix := false
			if validPrefix {
				for _, p := range source[start:i] {
					if p == 'f' || p == 'F' {
						hasFPrefix = true
						break
					}
				}
			}
			end, ok := pythonSkipQuotedLiteral(source, i)
			if !flags.continuationEscape && pythonSourceRangeContainsContinuationEscape(source, i, end) {
				flags.continuationEscape = true
			}
			if !ok {
				flags.trailingSelfCalls = true
				flags.printChevron = true
				flags.fStringPattern = true
				flags.passWord = true
				i = end
				continue
			}
			if hasFPrefix {
				contentStart, contentEnd := pythonQuotedLiteralContentRange(source, i, end)
				if contentStart < contentEnd && pythonFStringContentMayNeedPatternNormalization(source[contentStart:contentEnd]) {
					flags.fStringPattern = true
				}
			}
			i = end
			continue
		}
		if source[i] == ';' {
			flags.trailingSelfCalls = true
		}
		if source[i] == '=' && bytes.IndexByte(source[i+1:], ',') >= 0 {
			flags.assignmentList = true
		}
		if source[i] == ',' {
			flags.comma = true
		}
		if !flags.printChevron && pythonSourceWordAt(source, i, "print") {
			j := i + len("print")
			for j < len(source) && (source[j] == ' ' || source[j] == '\t' || source[j] == '\f') {
				j++
			}
			if j+1 < len(source) && source[j] == '>' && source[j+1] == '>' {
				flags.printChevron = true
			}
		}
		if !flags.passWord && pythonSourceWordAt(source, i, "pass") {
			flags.passWord = true
		}
		if !flags.continueWord && pythonSourceWordAt(source, i, "continue") {
			flags.continueWord = true
		}
		if !flags.breakWord && pythonSourceWordAt(source, i, "break") {
			flags.breakWord = true
		}
		if !flags.returnWord && pythonSourceWordAt(source, i, "return") {
			flags.returnWord = true
		}
		if !flags.raiseWord && pythonSourceWordAt(source, i, "raise") {
			flags.raiseWord = true
		}
		if !flags.yieldWord && pythonSourceWordAt(source, i, "yield") {
			flags.yieldWord = true
		}
		if !flags.asPattern && pythonSourceWordAt(source, i, "as") {
			flags.asPattern = true
		}
		i++
	}
	return flags
}

func normalizePythonStarCollapsedChildrenWithStats(root *Node, source []byte, lang *Language) normalizationPassCounters {
	wildcard := normalizeCollapsedNamedLeafChildrenBySourceWithStats(root, source, lang, "wildcard_import", "*")
	keywordSeparator := normalizeCollapsedNamedLeafChildrenBySourceWithStats(root, source, lang, "keyword_separator", "*")
	return normalizationPassCounters{
		nodesVisited:   wildcard.nodesVisited + keywordSeparator.nodesVisited,
		nodesRewritten: wildcard.nodesRewritten + keywordSeparator.nodesRewritten,
	}
}

func normalizePythonInlineRaiseBlocksWithStats(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	raiseStmtSym, ok := lang.symbolByNameAndNamed("raise_statement", true)
	if !ok {
		raiseStmtSym, ok = symbolByName(lang, "raise_statement")
	}
	if !ok {
		return counters
	}
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if childCount < 1 || n.Type(lang) != "block" || n.startPoint.Row != n.endPoint.Row {
			return
		}
		first := resultChildAt(n, 0)
		last := resultChildAt(n, childCount-1)
		if first == nil || last == nil || first.Type(lang) != "raise" {
			return
		}
		children := resultChildSliceForMutation(n)
		stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
		stmt := newParentNodeInArena(n.ownerArena, raiseStmtSym, true, stmtChildren, nil, 0)
		stmt.startByte = first.startByte
		stmt.endByte = last.endByte
		stmt.startPoint = first.startPoint
		stmt.endPoint = last.endPoint
		stmt.parent = n
		stmt.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
		counters.nodesRewritten++
	})
	return counters
}

func normalizePythonAssignmentRightExpressionListsWithStats(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	patternListSym, patternOK := symbolByName(lang, "pattern_list")
	expressionListSym, expressionOK := symbolByName(lang, "expression_list")
	if !patternOK || !expressionOK {
		return counters
	}
	expressionListNamed := symbolIsNamed(lang, expressionListSym)
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		if n.Type(lang) != "assignment" {
			return
		}
		sawEquals := false
		childCount := resultChildCount(n)
		for i := 0; i < childCount; i++ {
			child := resultChildAt(n, i)
			if child == nil {
				continue
			}
			if child.Type(lang) == "=" {
				sawEquals = true
				continue
			}
			if sawEquals && child.symbol == patternListSym {
				child.symbol = expressionListSym
				child.setNamed(expressionListNamed)
				counters.nodesRewritten++
				return
			}
		}
	})
	return counters
}

func normalizePythonInlineTupleExpressionBlocksWithStats(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	tupleSym, ok := symbolByName(lang, "tuple_expression")
	if !ok {
		return counters
	}
	tupleNamed := symbolIsNamed(lang, tupleSym)
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if childCount < 3 || n.Type(lang) != "block" || n.startPoint.Row != n.endPoint.Row {
			return
		}
		hasComma := false
		for i := 0; i < childCount; i++ {
			child := resultChildAt(n, i)
			if child != nil && child.Type(lang) == "," {
				hasComma = true
				break
			}
		}
		if !hasComma {
			return
		}
		children := resultChildSliceForMutation(n)
		stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
		stmt := newParentNodeInArena(n.ownerArena, tupleSym, tupleNamed, stmtChildren, nil, 0)
		stmt.startByte = children[0].startByte
		stmt.endByte = children[len(children)-1].endByte
		stmt.startPoint = children[0].startPoint
		stmt.endPoint = children[len(children)-1].endPoint
		stmt.parent = n
		stmt.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
		counters.nodesRewritten++
	})
	return counters
}

func normalizePythonInlineYieldBlocksWithStats(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil {
		return counters
	}
	yieldSym, ok := lang.symbolByNameAndNamed("yield", true)
	if !ok {
		return counters
	}
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if childCount < 1 || n.Type(lang) != "block" || n.startPoint.Row != n.endPoint.Row {
			return
		}
		first := resultChildAt(n, 0)
		last := resultChildAt(n, childCount-1)
		if first == nil || last == nil || first.Type(lang) != "yield" || first.IsNamed() {
			return
		}
		children := resultChildSliceForMutation(n)
		stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
		stmt := newParentNodeInArena(n.ownerArena, yieldSym, true, stmtChildren, nil, 0)
		stmt.startByte = first.startByte
		stmt.endByte = last.endByte
		stmt.startPoint = first.startPoint
		stmt.endPoint = last.endPoint
		stmt.parent = n
		stmt.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
		counters.nodesRewritten++
	})
	return counters
}

func normalizePythonInlineReturnBlocksWithStats(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || len(source) == 0 {
		return counters
	}
	returnStmtSym, ok := lang.symbolByNameAndNamed("return_statement", true)
	if !ok {
		returnStmtSym, ok = symbolByName(lang, "return_statement")
	}
	if !ok {
		return counters
	}
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if childCount < 1 || n.Type(lang) != "block" || n.startPoint.Row != n.endPoint.Row {
			return
		}
		first := resultChildAt(n, 0)
		last := resultChildAt(n, childCount-1)
		if first == nil || last == nil || first.Type(lang) != "return" {
			return
		}
		children := resultChildSliceForMutation(n)
		stmtChildren := cloneNodeSliceInArena(n.ownerArena, children)
		stmt := newParentNodeInArena(n.ownerArena, returnStmtSym, true, stmtChildren, nil, 0)
		stmt.startByte = first.startByte
		stmt.endByte = last.endByte
		stmt.startPoint = first.startPoint
		stmt.endPoint = last.endPoint
		stmt.parent = n
		stmt.childIndex = 0
		n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
		counters.nodesRewritten++
	})
	return counters
}

func normalizePythonCollapsedKeywordStatementWithStats(root *Node, source []byte, lang *Language, statementName, keywordName string) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || len(source) == 0 {
		return counters
	}
	statementSym, statementOK := lang.symbolByNameAndNamed(statementName, true)
	if !statementOK {
		statementSym, statementOK = symbolByName(lang, statementName)
	}
	keywordSym, keywordOK := lang.symbolByNameAndNamed(keywordName, false)
	if !keywordOK {
		keywordSym, keywordOK = symbolByName(lang, keywordName)
	}
	if !statementOK || !keywordOK {
		return counters
	}
	keywordNamed := symbolIsNamed(lang, keywordSym)
	walkResultTree(root, func(n *Node) {
		counters.nodesVisited++
		if resultChildCount(n) != 0 || int(n.endByte) > len(source) || n.startByte >= n.endByte || string(source[n.startByte:n.endByte]) != keywordName {
			return
		}
		switch {
		case n.symbol == statementSym:
			child := newLeafNodeInArena(n.ownerArena, keywordSym, keywordNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			child.parent = n
			child.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
			counters.nodesRewritten++
		case n.Type(lang) == "block":
			child := newLeafNodeInArena(n.ownerArena, keywordSym, keywordNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
			stmt := newParentNodeInArena(n.ownerArena, statementSym, true, []*Node{child}, nil, 0)
			stmt.startByte = n.startByte
			stmt.endByte = n.endByte
			stmt.startPoint = n.startPoint
			stmt.endPoint = n.endPoint
			stmt.parent = n
			stmt.childIndex = 0
			n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{stmt})
			counters.nodesRewritten++
		}
	})
	return counters
}

func normalizePythonAsPatternTargetIdentifiers(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || len(source) == 0 {
		return counters
	}
	parentSym, ok := symbolByName(lang, "as_pattern_target")
	if !ok {
		return counters
	}
	identifierSym, ok := symbolByName(lang, "identifier")
	if !ok {
		return counters
	}
	tupleSym, hasTuple := symbolByName(lang, "tuple")
	tupleNamed := hasTuple && symbolIsNamed(lang, tupleSym)
	identifierNamed := false
	if int(identifierSym) < len(lang.SymbolMetadata) {
		identifierNamed = lang.SymbolMetadata[identifierSym].Named
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		counters.nodesVisited++
		childCount := resultChildCount(n)
		if n.symbol == parentSym {
			switch {
			case childCount == 0 && pythonSourceRangeIsIdentifier(source, n.startByte, n.endByte):
				child := newLeafNodeInArena(n.ownerArena, identifierSym, identifierNamed, n.startByte, n.endByte, n.startPoint, n.endPoint)
				n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{child})
				counters.nodesRewritten++
				childCount = 1
			case hasTuple && pythonAsPatternTargetLooksLikeTuple(n, lang, childCount):
				children := resultChildSliceForMutation(n)
				tupleChildren := cloneNodeSliceInArena(n.ownerArena, children)
				tuple := newParentNodeInArena(n.ownerArena, tupleSym, tupleNamed, tupleChildren, nil, 0)
				tuple.startByte = children[0].startByte
				tuple.endByte = children[len(children)-1].endByte
				tuple.startPoint = children[0].startPoint
				tuple.endPoint = children[len(children)-1].endPoint
				tuple.parent = n
				tuple.childIndex = 0
				n.children = cloneNodeSliceInArena(n.ownerArena, []*Node{tuple})
				counters.nodesRewritten++
				childCount = 1
			}
		}
		for i := 0; i < childCount; i++ {
			walk(resultChildAt(n, i))
		}
	}
	walk(root)
	return counters
}

func pythonAsPatternTargetLooksLikeTuple(n *Node, lang *Language, childCount int) bool {
	if n == nil || childCount < 3 {
		return false
	}
	first := resultChildAt(n, 0)
	last := resultChildAt(n, childCount-1)
	if first == nil || last == nil || first.Type(lang) != "(" || last.Type(lang) != ")" {
		return false
	}
	for i := 1; i+1 < childCount; i++ {
		child := resultChildAt(n, i)
		if child != nil && child.Type(lang) == "," {
			return true
		}
	}
	return false
}

func pythonSourceRangeIsIdentifier(source []byte, start, end uint32) bool {
	if start >= end || int(end) > len(source) {
		return false
	}
	for i := start; i < end; i++ {
		if !pythonIdentifierByte(source[i]) {
			return false
		}
	}
	return true
}

func pythonSourceRangeContainsContinuationEscape(source []byte, start, end int) bool {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	for i := start; i < end; i++ {
		if pythonSourceContinuationEscapeAt(source, i) {
			return true
		}
	}
	return false
}

func pythonSourceContinuationEscapeAt(source []byte, i int) bool {
	return i+1 < len(source) && source[i] == '\\' &&
		(source[i+1] == '\n' || (i+2 < len(source) && source[i+1] == '\r' && source[i+2] == '\n'))
}

func pythonSourceMayContainFString(source []byte) bool {
	for i, c := range source {
		if c != '"' && c != '\'' {
			continue
		}
		start := i
		for start > 0 && i-start < 3 && pythonStringPrefixByte(source[start-1]) {
			start--
		}
		if start == i {
			continue
		}
		if start > 0 && pythonIdentifierByte(source[start-1]) {
			continue
		}
		for _, p := range source[start:i] {
			if p == 'f' || p == 'F' {
				return true
			}
		}
	}
	return false
}

func pythonSourceMayContainFStringPatternNormalization(source []byte) bool {
	for i := 0; i < len(source); {
		switch source[i] {
		case '#':
			i = pythonSkipLineComment(source, i+1)
			continue
		case '\'', '"':
		default:
			i++
			continue
		}
		start := pythonStringPrefixStart(source, i)
		validPrefix := start < i && (start == 0 || !pythonIdentifierByte(source[start-1]))
		hasFPrefix := false
		if validPrefix {
			for _, p := range source[start:i] {
				if p == 'f' || p == 'F' {
					hasFPrefix = true
					break
				}
			}
		}
		end, ok := pythonSkipQuotedLiteral(source, i)
		if !ok {
			return true
		}
		if hasFPrefix {
			contentStart, contentEnd := pythonQuotedLiteralContentRange(source, i, end)
			if contentStart < contentEnd && pythonFStringContentMayNeedPatternNormalization(source[contentStart:contentEnd]) {
				return true
			}
		}
		i = end
	}
	return false
}

func pythonStringPrefixStart(source []byte, quoteIndex int) int {
	start := quoteIndex
	for start > 0 && quoteIndex-start < 3 && pythonStringPrefixByte(source[start-1]) {
		start--
	}
	return start
}

func pythonQuotedLiteralContentRange(source []byte, quoteIndex, end int) (int, int) {
	if quoteIndex < 0 || quoteIndex >= len(source) || end <= quoteIndex {
		return quoteIndex, quoteIndex
	}
	if quoteIndex+2 < end && source[quoteIndex+1] == source[quoteIndex] && source[quoteIndex+2] == source[quoteIndex] {
		return quoteIndex + 3, end - 3
	}
	return quoteIndex + 1, end - 1
}

func pythonFStringContentMayNeedPatternNormalization(content []byte) bool {
	for i := 0; i < len(content); i++ {
		if content[i] != '{' {
			continue
		}
		if i+1 < len(content) && content[i+1] == '{' {
			i++
			continue
		}
		depth := 1
		for j := i + 1; j < len(content); j++ {
			switch content[j] {
			case '\'', '"':
				next, ok := pythonSkipQuotedLiteral(content, j)
				if !ok {
					return true
				}
				j = next - 1
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					i = j
					goto nextInterpolation
				}
			case ',', '*':
				return true
			}
		}
		return true
	nextInterpolation:
	}
	return false
}

func pythonSourceMayContainCodeByte(source []byte, want byte) bool {
	for i := 0; i < len(source); {
		switch source[i] {
		case '#':
			i = pythonSkipLineComment(source, i+1)
			continue
		case '\'', '"':
			next, ok := pythonSkipQuotedLiteral(source, i)
			if !ok {
				return true
			}
			i = next
			continue
		}
		if source[i] == want {
			return true
		}
		i++
	}
	return false
}

func pythonSourceMayContainCodeWord(source []byte, word string) bool {
	if word == "" {
		return false
	}
	for i := 0; i < len(source); {
		switch source[i] {
		case '#':
			i = pythonSkipLineComment(source, i+1)
			continue
		case '\'', '"':
			next, ok := pythonSkipQuotedLiteral(source, i)
			if !ok {
				return true
			}
			i = next
			continue
		}
		if pythonSourceWordAt(source, i, word) {
			return true
		}
		i++
	}
	return false
}

func pythonSourceMayContainPrintChevron(source []byte) bool {
	for i := 0; i < len(source); {
		switch source[i] {
		case '#':
			i = pythonSkipLineComment(source, i+1)
			continue
		case '\'', '"':
			next, ok := pythonSkipQuotedLiteral(source, i)
			if !ok {
				return true
			}
			i = next
			continue
		}
		if !pythonSourceWordAt(source, i, "print") {
			i++
			continue
		}
		j := i + len("print")
		for j < len(source) && (source[j] == ' ' || source[j] == '\t' || source[j] == '\f') {
			j++
		}
		if j+1 < len(source) && source[j] == '>' && source[j+1] == '>' {
			return true
		}
		i += len("print")
	}
	return false
}

func pythonSourceWordAt(source []byte, i int, word string) bool {
	if i < 0 || i+len(word) > len(source) {
		return false
	}
	for j := 0; j < len(word); j++ {
		if source[i+j] != word[j] {
			return false
		}
	}
	if i > 0 && pythonIdentifierByte(source[i-1]) {
		return false
	}
	end := i + len(word)
	return end >= len(source) || !pythonIdentifierByte(source[end])
}

func pythonSkipLineComment(source []byte, i int) int {
	for i < len(source) && source[i] != '\n' && source[i] != '\r' {
		i++
	}
	return i
}

func pythonSkipQuotedLiteral(source []byte, i int) (int, bool) {
	if i < 0 || i >= len(source) || (source[i] != '\'' && source[i] != '"') {
		return i, false
	}
	quote := source[i]
	if i+2 < len(source) && source[i+1] == quote && source[i+2] == quote {
		for j := i + 3; j+2 < len(source); j++ {
			if source[j] == quote && source[j+1] == quote && source[j+2] == quote {
				return j + 3, true
			}
		}
		return len(source), false
	}
	escaped := false
	for j := i + 1; j < len(source); j++ {
		c := source[j]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' {
			escaped = true
			continue
		}
		if c == quote {
			return j + 1, true
		}
		if c == '\n' || c == '\r' {
			return j, false
		}
	}
	return len(source), false
}

func pythonStringPrefixByte(c byte) bool {
	switch c {
	case 'b', 'B', 'f', 'F', 'r', 'R', 'u', 'U':
		return true
	default:
		return false
	}
}

func pythonIdentifierByte(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}

func normalizePythonInterpolationPatterns(root *Node, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || lang.Name != "python" {
		return counters
	}
	patternListSym, ok := symbolByName(lang, "pattern_list")
	if !ok {
		return counters
	}
	listSplatPatternSym, hasListSplatPattern := symbolByName(lang, "list_splat_pattern")
	expressionListSym, hasExpressionList := symbolByName(lang, "expression_list")
	listSplatSym, hasListSplat := symbolByName(lang, "list_splat")

	patternListNamed := symbolIsNamed(lang, patternListSym)
	listSplatPatternNamed := hasListSplatPattern && symbolIsNamed(lang, listSplatPatternSym)

	var rewrite func(*Node, bool)
	rewrite = func(n *Node, inInterpolation bool) {
		if n == nil {
			return
		}
		counters.nodesVisited++
		here := inInterpolation || n.Type(lang) == "interpolation"
		if here {
			if hasExpressionList && n.symbol == expressionListSym {
				n.symbol = patternListSym
				n.setNamed(patternListNamed)
				counters.nodesRewritten++
			}
			if hasListSplatPattern && hasListSplat && n.symbol == listSplatSym {
				n.symbol = listSplatPatternSym
				n.setNamed(listSplatPatternNamed)
				counters.nodesRewritten++
			}
		}
		childCount := resultChildCount(n)
		for i := 0; i < childCount; i++ {
			child := resultChildAt(n, i)
			rewrite(child, here)
		}
	}
	rewrite(root, false)
	return counters
}

func normalizePythonPrintStatements(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return counters
	}
	walkResultTreePostorder(root, func(node *Node) {
		counters.nodesVisited++
		switch node.Type(lang) {
		case "module", "block":
			children := resultChildSliceForMutation(node)
			rewritten, changed := rewritePythonStatementList(children, source, lang)
			if !changed {
				return
			}
			replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, rewritten))
			counters.nodesRewritten++
		}
	})
	return counters
}

func normalizePythonTrailingSelfCalls(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return counters
	}
	walkResultTreePostorder(root, func(node *Node) {
		counters.nodesVisited++
		if node.Type(lang) != "block" {
			return
		}
		children := resultChildSliceForMutation(node)
		rewritten, changed := foldPythonTrailingSelfCallsInBlock(children, source, lang)
		if !changed {
			return
		}
		replaceNodeChildrenUnfielded(node, cloneNodeSliceInArena(node.ownerArena, rewritten))
		counters.nodesRewritten++
	})
	return counters
}

func foldPythonTrailingSelfCallsInBlock(children []*Node, source []byte, lang *Language) ([]*Node, bool) {
	if len(children) < 2 || lang == nil || lang.Name != "python" || len(source) == 0 {
		return children, false
	}
	var out []*Node
	for i := 0; i < len(children); i++ {
		cur := children[i]
		if i+1 >= len(children) {
			if out != nil {
				out = append(out, cur)
			}
			continue
		}
		next := children[i+1]
		rewritten, ok := foldPythonTrailingSelfCallIntoNestedFunction(cur, next, source, lang)
		if !ok {
			if out != nil {
				out = append(out, cur)
			}
			continue
		}
		if out == nil {
			out = make([]*Node, 0, len(children))
			out = append(out, children[:i]...)
		}
		out = append(out, rewritten)
		i++
	}
	if out == nil {
		return children, false
	}
	return out, true
}

func foldPythonTrailingSelfCallIntoNestedFunction(fnNode, trailingCall *Node, source []byte, lang *Language) (*Node, bool) {
	if fnNode == nil || trailingCall == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return nil, false
	}
	if fnNode.Type(lang) != "function_definition" || trailingCall.Type(lang) != "call" {
		return nil, false
	}
	if trailingCall.startPoint.Column != fnNode.startPoint.Column {
		return nil, false
	}
	fnName, ok := pythonFunctionDefinitionNameNode(fnNode, lang)
	if !ok || fnName == nil {
		return nil, false
	}
	callName, ok := pythonCallIdentifierNode(trailingCall, lang)
	if !ok || callName == nil {
		return nil, false
	}
	if !pythonNodeTextEqual(fnName, callName, source) {
		return nil, false
	}
	bodyIndex := pythonChildIndexByTypeNoMaterialize(fnNode, lang, "block")
	body := resultChildAt(fnNode, bodyIndex)
	if bodyIndex < 0 || body == nil || !pythonBlockEndsWithSemicolon(body, lang) {
		return nil, false
	}

	bodyClone := cloneNodeInArenaAppendingChildForMutation(body.ownerArena, body, trailingCall)
	bodyClone.fieldIDs = nil
	bodyClone.fieldSources = nil

	fnClone := cloneNodeInArenaReplacingChildForMutation(fnNode.ownerArena, fnNode, bodyIndex, bodyClone)
	return fnClone, true
}

func rewritePythonStatementList(children []*Node, source []byte, lang *Language) ([]*Node, bool) {
	if len(children) == 0 || lang == nil || lang.Name != "python" {
		return children, false
	}
	var out []*Node
	for i, child := range children {
		if child == nil {
			if out != nil {
				out = append(out, nil)
			}
			continue
		}
		if rewritten, ok := rewriteMalformedPythonPrintStatement(child, source, lang); ok {
			if out == nil {
				out = make([]*Node, 0, len(children))
				out = append(out, children[:i]...)
			}
			out = append(out, rewritten)
			continue
		}
		if out != nil {
			out = append(out, child)
		}
	}
	if out == nil {
		return children, false
	}
	return out, true
}

func rewriteMalformedPythonPrintStatement(node *Node, source []byte, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || lang.Name != "python" {
		return nil, false
	}
	bin, extras, ok := pythonMalformedPrintStatementParts(node, source, lang)
	if !ok || bin == nil || resultChildCount(bin) < 3 {
		return nil, false
	}
	printStmtSym, ok := symbolByName(lang, "print_statement")
	if !ok {
		return nil, false
	}
	chevronSym, ok := symbolByName(lang, "chevron")
	if !ok {
		return nil, false
	}
	printSym, ok := symbolByName(lang, "print")
	if !ok {
		return nil, false
	}

	printNamed := symbolIsNamed(lang, printSym)
	printStmtNamed := symbolIsNamed(lang, printStmtSym)
	chevronNamed := symbolIsNamed(lang, chevronSym)

	left := resultChildAt(bin, 0)
	op := resultChildAt(bin, 1)
	dest := resultChildAt(bin, 2)
	printLeaf := cloneNodeInArena(node.ownerArena, left)
	printLeaf.symbol = printSym
	printLeaf.setNamed(printNamed)
	printLeaf.children = nil
	printLeaf.fieldIDs = nil
	printLeaf.fieldSources = nil

	chevron := cloneNodeInArena(node.ownerArena, bin)
	chevron.symbol = chevronSym
	chevron.setNamed(chevronNamed)
	chevron.children = cloneNodeSliceInArena(chevron.ownerArena, []*Node{op, dest})
	chevron.fieldIDs = nil
	chevron.fieldSources = nil
	chevron.productionID = 0
	populateParentNode(chevron, chevron.children)

	rewritten := cloneNodeInArena(node.ownerArena, node)
	children := make([]*Node, 0, 2+len(extras))
	children = append(children, printLeaf, chevron)
	children = append(children, extras...)
	rewritten.symbol = printStmtSym
	rewritten.setNamed(printStmtNamed)
	rewritten.children = cloneNodeSliceInArena(rewritten.ownerArena, children)
	rewritten.fieldIDs = nil
	rewritten.fieldSources = nil
	rewritten.productionID = 0
	populateParentNode(rewritten, rewritten.children)
	return rewritten, true
}

func pythonMalformedPrintStatementParts(node *Node, source []byte, lang *Language) (*Node, []*Node, bool) {
	if node == nil || lang == nil || lang.Name != "python" {
		return nil, nil, false
	}
	switch node.Type(lang) {
	case "binary_operator":
		if pythonIsPrintChevronBinary(node, source, lang) {
			return node, nil, true
		}
	case "tuple_expression":
		childCount := resultChildCount(node)
		if childCount == 0 {
			return nil, nil, false
		}
		bin := resultChildAt(node, 0)
		if pythonIsPrintChevronBinary(bin, source, lang) {
			extras := make([]*Node, 0, childCount-1)
			for i := 1; i < childCount; i++ {
				extras = append(extras, resultChildAt(node, i))
			}
			return bin, extras, true
		}
	}
	return nil, nil, false
}

func pythonIsPrintChevronBinary(node *Node, source []byte, lang *Language) bool {
	if node == nil || lang == nil || lang.Name != "python" || resultChildCount(node) != 3 {
		return false
	}
	if node.Type(lang) != "binary_operator" {
		return false
	}
	left := resultChildAt(node, 0)
	op := resultChildAt(node, 1)
	if left == nil || op == nil {
		return false
	}
	if left.Type(lang) != "identifier" || op.Type(lang) != ">>" {
		return false
	}
	if left.startByte >= left.endByte || int(left.endByte) > len(source) {
		return false
	}
	return string(source[left.startByte:left.endByte]) == "print"
}

func normalizePythonModuleChildren(nodes []*Node, arena *nodeArena, lang *Language) []*Node {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" {
		return nodes
	}
	out := make([]*Node, 0, len(nodes))
	changed := false
	for _, node := range nodes {
		if node == nil {
			continue
		}
		normalized, nodeChanged := normalizePythonModuleNode(node, lang)
		if nodeChanged {
			out = append(out, normalized)
			changed = true
			continue
		}
		out = append(out, node)
	}
	if !changed {
		return nodes
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		return buf
	}
	return out
}

func normalizePythonModuleNode(node *Node, lang *Language) (*Node, bool) {
	changed := false
	for node != nil {
		if node.Type(lang) == "_simple_statements" && resultChildCount(node) == 1 {
			child := resultChildAt(node, 0)
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		if node.Type(lang) == "expression_statement" && resultChildCount(node) == 1 {
			child := resultChildAt(node, 0)
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		if (node.Type(lang) == "expression" || node.Type(lang) == "primary_expression") && resultChildCount(node) == 1 {
			child := resultChildAt(node, 0)
			if child != nil && child.IsNamed() {
				node = child
				changed = true
				continue
			}
		}
		break
	}
	return node, changed
}

func repairPythonRootNode(root *Node, arena *nodeArena, lang *Language) *Node {
	if root == nil || lang == nil || lang.Name != "python" || root.Type(lang) != "module" {
		return root
	}
	if !pythonRootRepairNeedsMaterializedChildren(root, lang) {
		if root.hasError() && pythonModuleChildrenLookCompleteNoMaterialize(root, lang) {
			cloned := cloneNodeInArenaPreservingFinalRefsForMutation(arena, root)
			cloned.setHasError(false)
			return cloned
		}
		return root
	}
	rootChildren := resultChildSliceForMutation(root)
	children := collapsePythonRootFragments(rootChildren, arena, lang)
	changed := len(children) != len(rootChildren)
	if !changed {
		for i := range children {
			if children[i] != rootChildren[i] {
				changed = true
				break
			}
		}
	}

	var repaired []*Node
	for i, child := range children {
		fixed := repairPythonTopLevelNode(child, arena, lang)
		if fixed != child {
			changed = true
			if repaired == nil {
				repaired = make([]*Node, 0, len(children))
				repaired = append(repaired, children[:i]...)
			}
		}
		if repaired != nil {
			repaired = append(repaired, fixed)
		}
	}
	if repaired == nil {
		repaired = children
	}

	if !changed {
		if root.hasError() && pythonModuleChildrenLookComplete(repaired, lang) {
			cloned := cloneNodeInArenaPreservingFinalRefsForMutation(arena, root)
			cloned.setHasError(false)
			return cloned
		}
		return root
	}

	cloned := cloneNodeInArena(arena, root)
	if arena != nil {
		buf := arena.allocNodeSlice(len(repaired))
		copy(buf, repaired)
		repaired = buf
	}
	cloned.children = repaired
	cloned.fieldIDs = nil
	cloned.fieldSources = nil
	if pythonModuleChildrenLookComplete(repaired, lang) {
		cloned.setHasError(false)
	}
	return cloned
}

type pythonRepairSymbols struct {
	classToken          Symbol
	defToken            Symbol
	ifToken             Symbol
	indent              Symbol
	dedent              Symbol
	simpleStatements    Symbol
	expressionStatement Symbol
	expression          Symbol
	primaryExpression   Symbol
	classDefinition     Symbol
	functionDefinition  Symbol
	ifStatement         Symbol
	block               Symbol
	semicolon           Symbol
}

func pythonRepairSymbolSet(lang *Language) pythonRepairSymbols {
	sym := func(name string) Symbol {
		if s, ok := symbolByName(lang, name); ok {
			return s
		}
		return Symbol(^uint16(0))
	}
	return pythonRepairSymbols{
		classToken:          sym("class"),
		defToken:            sym("def"),
		ifToken:             sym("if"),
		indent:              sym("_indent"),
		dedent:              sym("_dedent"),
		simpleStatements:    sym("_simple_statements"),
		expressionStatement: sym("expression_statement"),
		expression:          sym("expression"),
		primaryExpression:   sym("primary_expression"),
		classDefinition:     sym("class_definition"),
		functionDefinition:  sym("function_definition"),
		ifStatement:         sym("if_statement"),
		block:               sym("block"),
		semicolon:           sym(";"),
	}
}

func pythonRootRepairNeedsMaterializedChildren(root *Node, lang *Language) bool {
	if root == nil || lang == nil || !nodeHasFinalChildRefs(root) {
		return true
	}
	syms := pythonRepairSymbolSet(lang)
	childCount := resultChildCount(root)
	if childCount > 0 {
		entry, ok := nodeChildEntryAtNoMaterialize(root, childCount-1)
		if !ok || !stackEntryHasNode(entry) {
			return true
		}
		if !stackEntryNodeIsNamed(entry) && stackEntryNodeStartByte(entry) == stackEntryNodeEndByte(entry) {
			return true
		}
	}
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(root, i)
		if !ok || !stackEntryHasNode(entry) {
			return true
		}
		sym := stackEntryNodeSymbol(entry)
		switch sym {
		case syms.classToken, syms.defToken, syms.ifToken:
			return true
		}
		if pythonEntryMayNormalizeModuleNode(entry, syms) {
			return true
		}
		if pythonEntryMayNeedRepair(root.ownerArena, entry, lang, syms, false, 0) {
			return true
		}
	}
	return false
}

func pythonEntryMayNormalizeModuleNode(entry stackEntry, syms pythonRepairSymbols) bool {
	switch stackEntryNodeSymbol(entry) {
	case syms.simpleStatements, syms.expressionStatement, syms.expression, syms.primaryExpression:
		return stackEntryNodeChildCount(entry) == 1
	default:
		return false
	}
}

func pythonEntryMayNeedRepair(arena *nodeArena, entry stackEntry, lang *Language, syms pythonRepairSymbols, allowHoist bool, depth int) bool {
	if depth > 32 || !stackEntryHasNode(entry) {
		return true
	}
	if pythonEntryMayNormalizeModuleNode(entry, syms) {
		return true
	}
	switch stackEntryNodeSymbol(entry) {
	case syms.block:
		return pythonBlockEntryMayNeedRepair(arena, entry, lang, syms, allowHoist, depth+1)
	case syms.classDefinition:
		return pythonDefinitionBlockMayNeedRepair(arena, entry, lang, syms, true, depth+1)
	case syms.functionDefinition:
		return pythonDefinitionBlockMayNeedRepair(arena, entry, lang, syms, false, depth+1)
	case syms.ifStatement:
		childCount := stackEntryNodeChildCount(entry)
		for i := 0; i < childCount; i++ {
			child, ok := pythonStackEntryChildEntryAtNoMaterialize(arena, entry, i)
			if !ok || !stackEntryHasNode(child) {
				return true
			}
			if pythonEntryMayNeedRepair(arena, child, lang, syms, allowHoist, depth+1) {
				return true
			}
		}
		return false
	default:
		return stackEntryNodeHasError(entry)
	}
}

func pythonDefinitionBlockMayNeedRepair(arena *nodeArena, entry stackEntry, lang *Language, syms pythonRepairSymbols, allowHoist bool, depth int) bool {
	if stackEntryNodeHasError(entry) {
		return true
	}
	childCount := stackEntryNodeChildCount(entry)
	for i := 0; i < childCount; i++ {
		child, ok := pythonStackEntryChildEntryAtNoMaterialize(arena, entry, i)
		if !ok || !stackEntryHasNode(child) {
			return true
		}
		if stackEntryNodeSymbol(child) == syms.block {
			return pythonBlockEntryMayNeedRepair(arena, child, lang, syms, allowHoist, depth+1)
		}
	}
	return false
}

func pythonBlockEntryMayNeedRepair(arena *nodeArena, entry stackEntry, lang *Language, syms pythonRepairSymbols, allowHoist bool, depth int) bool {
	if stackEntryNodeHasError(entry) {
		return true
	}
	childCount := stackEntryNodeChildCount(entry)
	var firstAnchor stackEntry
	var haveFirstAnchor bool
	var lastSpan stackEntry
	var haveLastSpan bool
	var lastChild stackEntry
	var haveLastChild bool
	for i := 0; i < childCount; i++ {
		child, ok := pythonStackEntryChildEntryAtNoMaterialize(arena, entry, i)
		if !ok || !stackEntryHasNode(child) {
			return true
		}
		haveLastChild = true
		lastChild = child
		sym := stackEntryNodeSymbol(child)
		switch sym {
		case syms.indent, syms.dedent, syms.simpleStatements:
			return true
		case syms.functionDefinition:
			if allowHoist {
				return true
			}
		}
		if pythonEntryMayNormalizeModuleNode(child, syms) {
			return true
		}
		if pythonEntryMayNeedRepair(arena, child, lang, syms, allowHoist, depth+1) {
			return true
		}
		if !haveFirstAnchor && sym != syms.indent && sym != syms.dedent &&
			(stackEntryNodeEndByte(child) > stackEntryNodeStartByte(child) || stackEntryNodeIsNamed(child)) {
			firstAnchor = child
			haveFirstAnchor = true
		}
		if stackEntryNodeEndByte(child) > stackEntryNodeStartByte(child) {
			lastSpan = child
			haveLastSpan = true
		}
	}
	if !haveFirstAnchor || !haveLastSpan {
		return false
	}
	wantEndByte := stackEntryNodeEndByte(lastSpan)
	wantEndPoint := stackEntryNodeEndPoint(lastSpan)
	if haveLastChild && stackEntryNodeEndByte(entry) > wantEndByte && stackEntryNodeSymbol(lastChild) == syms.semicolon {
		wantEndByte = stackEntryNodeEndByte(entry)
		wantEndPoint = stackEntryNodeEndPoint(entry)
	}
	return stackEntryNodeStartByte(entry) != stackEntryNodeStartByte(firstAnchor) ||
		stackEntryNodeStartPoint(entry) != stackEntryNodeStartPoint(firstAnchor) ||
		stackEntryNodeEndByte(entry) != wantEndByte ||
		stackEntryNodeEndPoint(entry) != wantEndPoint
}

func pythonStackEntryChildEntryAtNoMaterialize(arena *nodeArena, entry stackEntry, i int) (stackEntry, bool) {
	if i < 0 {
		return stackEntry{}, false
	}
	if node := stackEntryNode(entry); node != nil {
		return nodeChildEntryAtNoMaterialize(node, i)
	}
	if parent := stackEntryPendingParent(entry); parent != nil {
		if i >= parent.childEntryCount() {
			return stackEntry{}, false
		}
		return parent.childEntry(arena, i), true
	}
	return stackEntry{}, false
}

func pythonModuleChildrenLookCompleteNoMaterialize(root *Node, lang *Language) bool {
	childCount := resultChildCount(root)
	if childCount == 0 {
		return false
	}
	simpleStatements, hasSimpleStatements := symbolByName(lang, "_simple_statements")
	seen := 0
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(root, i)
		if !ok || !stackEntryHasNode(entry) {
			return false
		}
		if stackEntryNodeIsExtra(entry) {
			continue
		}
		if stackEntryNodeIsNamed(entry) {
			seen++
			continue
		}
		if hasSimpleStatements && stackEntryNodeSymbol(entry) == simpleStatements {
			seen++
			continue
		}
		return false
	}
	return seen > 0
}

func repairPythonKeywordErrorNodes(nodes []*Node, source []byte, arena *nodeArena, lang *Language) ([]*Node, bool) {
	if len(nodes) == 0 || lang == nil || lang.Name != "python" || len(source) == 0 {
		return nodes, false
	}
	var out []*Node
	for i, node := range nodes {
		repaired := repairPythonKeywordErrorNode(node, source, arena, lang)
		if repaired != node {
			if out == nil {
				out = make([]*Node, 0, len(nodes))
				out = append(out, nodes[:i]...)
			}
		}
		if out != nil {
			out = append(out, repaired)
		}
	}
	if out == nil {
		return nodes, false
	}
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	return out, true
}

func repairPythonKeywordErrorNode(node *Node, source []byte, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return node
	}
	if !node.hasError() && node.symbol != errorSymbol {
		return node
	}
	childCount := resultChildCount(node)
	if node.Type(lang) == "ERROR" && childCount == 0 {
		if keyword, ok := pythonKeywordLeafSymbol(node, source, lang); ok {
			named := symbolIsNamed(lang, keyword)
			repl := newLeafNodeInArena(arena, keyword, named, node.startByte, node.endByte, node.startPoint, node.endPoint)
			repl.setExtra(node.isExtra())
			return repl
		}
	}
	if childCount == 0 {
		return node
	}
	var children []*Node
	for i := 0; i < childCount; i++ {
		child := resultChildAt(node, i)
		repaired := repairPythonKeywordErrorNode(child, source, arena, lang)
		if repaired != child {
			if children == nil {
				children = make([]*Node, 0, childCount)
				for j := 0; j < i; j++ {
					children = append(children, resultChildAt(node, j))
				}
			}
		}
		if children != nil {
			children = append(children, repaired)
		}
	}
	if children == nil {
		if node.Type(lang) == "ERROR" && childCount == 1 {
			child := resultChildAt(node, 0)
			if child != nil &&
				!child.IsError() &&
				!child.HasError() &&
				child.startByte == node.startByte &&
				child.endByte == node.endByte {
				return child
			}
		}
		return node
	}
	finalChildren := children
	if node.Type(lang) == "ERROR" && len(finalChildren) == 1 {
		child := finalChildren[0]
		if child != nil &&
			!child.IsError() &&
			!child.HasError() &&
			child.startByte == node.startByte &&
			child.endByte == node.endByte {
			return child
		}
	}
	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(finalChildren))
		copy(buf, finalChildren)
		finalChildren = buf
	}
	cloned.children = finalChildren
	return cloned
}

func pythonKeywordLeafSymbol(node *Node, source []byte, lang *Language) (Symbol, bool) {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return 0, false
	}
	text := string(source[node.startByte:node.endByte])
	if text == "" {
		return 0, false
	}
	sym, ok := symbolByName(lang, text)
	if !ok {
		return 0, false
	}
	if !symbolHasMetadata(lang, sym) || symbolIsNamed(lang, sym) {
		return 0, false
	}
	return sym, true
}

func repairPythonTopLevelNode(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" {
		return node
	}
	return repairPythonNode(node, arena, lang)
}

func repairPythonNode(node *Node, arena *nodeArena, lang *Language) *Node {
	if node == nil || lang == nil || lang.Name != "python" {
		return node
	}
	normalized, changed := normalizePythonModuleNode(node, lang)
	if changed {
		node = normalized
	}
	switch node.Type(lang) {
	case "class_definition":
		return repairPythonClassDefinition(node, arena, lang)
	case "function_definition":
		return repairPythonFunctionDefinition(node, arena, lang)
	case "if_statement":
		return repairPythonIfStatement(node, arena, lang)
	case "block":
		repaired, _ := repairPythonBlock(node, arena, lang, false)
		return repaired
	default:
		return node
	}
}

func repairPythonClassDefinition(node *Node, arena *nodeArena, lang *Language) *Node {
	childCount := resultChildCount(node)
	if node == nil || node.Type(lang) != "class_definition" || childCount == 0 {
		return node
	}
	bodyIndex := pythonChildIndexByTypeNoMaterialize(node, lang, "block")
	if bodyIndex < 0 {
		return node
	}
	body := resultChildAt(node, bodyIndex)
	repairedBody, changed := repairPythonBlock(body, arena, lang, true)
	if !changed {
		return node
	}

	cloned := cloneNodeInArenaReplacingChildForMutation(arena, node, bodyIndex, repairedBody)
	if repairedBody != nil {
		cloned.endByte = repairedBody.endByte
		cloned.endPoint = repairedBody.endPoint
	}
	return cloned
}

func repairPythonFunctionDefinition(node *Node, arena *nodeArena, lang *Language) *Node {
	childCount := resultChildCount(node)
	if node == nil || node.Type(lang) != "function_definition" || childCount == 0 {
		return node
	}
	bodyIndex := pythonChildIndexByTypeNoMaterialize(node, lang, "block")
	if bodyIndex < 0 {
		return node
	}
	body := resultChildAt(node, bodyIndex)
	repairedBody, changed := repairPythonBlock(body, arena, lang, false)
	if !changed {
		return node
	}

	cloned := cloneNodeInArenaReplacingChildForMutation(arena, node, bodyIndex, repairedBody)
	if repairedBody != nil {
		cloned.endByte = repairedBody.endByte
		cloned.endPoint = repairedBody.endPoint
	}
	return cloned
}

func repairPythonIfStatement(node *Node, arena *nodeArena, lang *Language) *Node {
	childCount := resultChildCount(node)
	if node == nil || node.Type(lang) != "if_statement" || childCount == 0 {
		return node
	}
	var children []*Node
	for i := 0; i < childCount; i++ {
		child := resultChildAt(node, i)
		repaired := repairPythonNode(child, arena, lang)
		if repaired != child {
			if children == nil {
				children = make([]*Node, 0, childCount)
				for j := 0; j < i; j++ {
					children = append(children, resultChildAt(node, j))
				}
			}
		}
		if children != nil {
			children = append(children, repaired)
		}
	}
	if children == nil {
		return node
	}

	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(children))
		copy(buf, children)
		children = buf
	}
	cloned.children = children
	last := children[len(children)-1]
	if last != nil {
		cloned.endByte = last.endByte
		cloned.endPoint = last.endPoint
	}
	return cloned
}

func repairPythonBlock(node *Node, arena *nodeArena, lang *Language, allowHoist bool) (*Node, bool) {
	if node == nil || node.Type(lang) != "block" {
		return node, false
	}
	childCount := resultChildCount(node)
	var out []*Node
	changed := false
	processedPending := false

	for i := 0; i < childCount; i++ {
		cur := resultChildAt(node, i)
		if cur == nil {
			continue
		}
		norm, normChanged := normalizePythonModuleNode(cur, lang)
		if normChanged {
			changed = true
			if out == nil {
				out = pythonBlockOutputPrefixFromNode(node, i)
			}
		}
		cur = norm
		if cur != nil {
			switch cur.Type(lang) {
			case "_indent", "_dedent":
				changed = true
				if out == nil {
					out = pythonBlockOutputPrefixFromNode(node, i)
				}
				continue
			case "_simple_statements":
				flat := flattenPythonSimpleStatements(cur, nil, lang)
				if len(flat) > 0 {
					changed = true
					if out == nil {
						out = pythonBlockOutputPrefixFromNode(node, i)
					}
					pending := prependPythonBlockPending(flat, resultChildSliceRangeForMutation(node, i+1, childCount))
					out = repairPythonBlockPending(pending, out, arena, lang, allowHoist)
					processedPending = true
					break
				}
			}
		}
		if processedPending {
			break
		}

		if allowHoist && cur != nil && cur.Type(lang) == "function_definition" {
			repairedFn, hoisted, split := splitPythonOvernestedFunction(cur, arena, lang)
			if split {
				changed = true
				if out == nil {
					out = pythonBlockOutputPrefixFromNode(node, i)
				}
				repairedFn = repairPythonNode(repairedFn, arena, lang)
				out = append(out, repairedFn)
				if len(hoisted) > 0 {
					pending := prependPythonBlockPending(hoisted, resultChildSliceRangeForMutation(node, i+1, childCount))
					out = repairPythonBlockPending(pending, out, arena, lang, allowHoist)
					processedPending = true
					break
				}
				continue
			}
		}

		repaired := repairPythonNode(cur, arena, lang)
		if repaired != cur {
			changed = true
			if out == nil {
				out = pythonBlockOutputPrefixFromNode(node, i)
			}
		}
		if out != nil {
			out = append(out, repaired)
		}
	}

	if !changed {
		firstNamed := pythonBlockStartAnchorNode(node, lang)
		lastSpan := pythonBlockEndAnchorNode(node)
		if firstNamed == nil || lastSpan == nil {
			return node, false
		}
		wantEndByte, wantEndPoint := lastSpan.endByte, lastSpan.endPoint
		if pythonBlockShouldPreserveOriginalEndNode(node, lang) {
			wantEndByte, wantEndPoint = node.endByte, node.endPoint
		}
		if node.startByte == firstNamed.startByte &&
			node.startPoint == firstNamed.startPoint &&
			node.endByte == wantEndByte &&
			node.endPoint == wantEndPoint {
			return node, false
		}
		changed = true
		out = pythonBlockOutputPrefixFromNode(node, childCount)
	}

	cloned := cloneNodeInArena(arena, node)
	if arena != nil {
		buf := arena.allocNodeSlice(len(out))
		copy(buf, out)
		out = buf
	}
	cloned.children = out
	cloned.fieldIDs = nil
	cloned.fieldSources = nil
	firstNamed := pythonBlockStartAnchor(out, lang)
	lastSpan := pythonBlockEndAnchor(out)
	if firstNamed != nil {
		cloned.startByte = firstNamed.startByte
		cloned.startPoint = firstNamed.startPoint
	}
	if lastSpan != nil {
		cloned.endByte = lastSpan.endByte
		cloned.endPoint = lastSpan.endPoint
		if pythonBlockShouldPreserveOriginalEnd(node, out, lang) {
			cloned.endByte = node.endByte
			cloned.endPoint = node.endPoint
		}
	}
	return cloned, true
}

func repairPythonBlockPending(pending []*Node, out []*Node, arena *nodeArena, lang *Language, allowHoist bool) []*Node {
	for len(pending) > 0 {
		cur := pending[0]
		pending = pending[1:]
		if cur == nil {
			continue
		}
		norm, normChanged := normalizePythonModuleNode(cur, lang)
		if normChanged {
			cur = norm
		}
		if cur != nil {
			switch cur.Type(lang) {
			case "_indent", "_dedent":
				continue
			case "_simple_statements":
				flat := flattenPythonSimpleStatements(cur, nil, lang)
				if len(flat) > 0 {
					pending = prependPythonBlockPending(flat, pending)
					continue
				}
			}
		}

		if allowHoist && cur != nil && cur.Type(lang) == "function_definition" {
			repairedFn, hoisted, split := splitPythonOvernestedFunction(cur, arena, lang)
			if split {
				repairedFn = repairPythonNode(repairedFn, arena, lang)
				out = append(out, repairedFn)
				if len(hoisted) > 0 {
					pending = prependPythonBlockPending(hoisted, pending)
				}
				continue
			}
		}

		out = append(out, repairPythonNode(cur, arena, lang))
	}
	return out
}

func pythonBlockOutputPrefixFromNode(node *Node, end int) []*Node {
	childCount := resultChildCount(node)
	if end > childCount {
		end = childCount
	}
	out := make([]*Node, 0, childCount)
	for i := 0; i < end; i++ {
		child := resultChildAt(node, i)
		if child != nil {
			out = append(out, child)
		}
	}
	return out
}

func prependPythonBlockPending(prefix, pending []*Node) []*Node {
	next := make([]*Node, 0, len(prefix)+len(pending))
	next = append(next, prefix...)
	next = append(next, pending...)
	return next
}

func pythonBlockStartAnchor(children []*Node, lang *Language) *Node {
	for _, child := range children {
		if child == nil {
			continue
		}
		typ := child.Type(lang)
		if typ == "_indent" || typ == "_dedent" {
			continue
		}
		if child.endByte > child.startByte || child.IsNamed() {
			return child
		}
	}
	return nil
}

func pythonBlockStartAnchorNode(node *Node, lang *Language) *Node {
	childCount := resultChildCount(node)
	for i := 0; i < childCount; i++ {
		child := resultChildAt(node, i)
		if child == nil {
			continue
		}
		typ := child.Type(lang)
		if typ == "_indent" || typ == "_dedent" {
			continue
		}
		if child.endByte > child.startByte || child.IsNamed() {
			return child
		}
	}
	return nil
}

func pythonBlockEndAnchor(children []*Node) *Node {
	for i := len(children) - 1; i >= 0; i-- {
		child := children[i]
		if child != nil && child.endByte > child.startByte {
			return child
		}
	}
	return nil
}

func pythonBlockEndAnchorNode(node *Node) *Node {
	for i := resultChildCount(node) - 1; i >= 0; i-- {
		child := resultChildAt(node, i)
		if child != nil && child.endByte > child.startByte {
			return child
		}
	}
	return nil
}

func pythonBlockShouldPreserveOriginalEnd(node *Node, children []*Node, lang *Language) bool {
	if node == nil || lang == nil || len(children) == 0 {
		return false
	}
	lastSpan := pythonBlockEndAnchor(children)
	if lastSpan == nil || node.endByte <= lastSpan.endByte {
		return false
	}
	lastChild := pythonBlockLastChild(children)
	return lastChild != nil && lastChild.Type(lang) == ";"
}

func pythonBlockShouldPreserveOriginalEndNode(node *Node, lang *Language) bool {
	if node == nil || lang == nil || resultChildCount(node) == 0 {
		return false
	}
	lastSpan := pythonBlockEndAnchorNode(node)
	if lastSpan == nil || node.endByte <= lastSpan.endByte {
		return false
	}
	lastChild := pythonBlockLastChildNode(node)
	return lastChild != nil && lastChild.Type(lang) == ";"
}

func pythonBlockLastChild(children []*Node) *Node {
	for i := len(children) - 1; i >= 0; i-- {
		if children[i] != nil {
			return children[i]
		}
	}
	return nil
}

func pythonBlockLastChildNode(node *Node) *Node {
	for i := resultChildCount(node) - 1; i >= 0; i-- {
		child := resultChildAt(node, i)
		if child != nil {
			return child
		}
	}
	return nil
}

func pythonBlockEndsWithSemicolon(node *Node, lang *Language) bool {
	childCount := resultChildCount(node)
	if node == nil || lang == nil || childCount == 0 {
		return false
	}
	if semiSym, ok := symbolByName(lang, ";"); ok {
		if entry, ok := nodeChildEntryAtNoMaterialize(node, childCount-1); ok && stackEntryHasNode(entry) {
			return stackEntryNodeSymbol(entry) == semiSym
		}
	}
	lastChild := resultChildAt(node, childCount-1)
	return lastChild != nil && lastChild.Type(lang) == ";"
}

func pythonFunctionDefinitionNameNode(node *Node, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "function_definition" {
		return nil, false
	}
	if index := pythonChildIndexByTypeNoMaterialize(node, lang, "identifier"); index >= 0 {
		child := resultChildAt(node, index)
		return child, child != nil
	}
	return nil, false
}

func pythonChildIndexByTypeNoMaterialize(node *Node, lang *Language, typ string) int {
	if node == nil || lang == nil || typ == "" {
		return -1
	}
	childCount := resultChildCount(node)
	if sym, ok := symbolByName(lang, typ); ok {
		for i := 0; i < childCount; i++ {
			entry, entryOK := nodeChildEntryAtNoMaterialize(node, i)
			if entryOK && stackEntryHasNode(entry) {
				if stackEntryNodeSymbol(entry) == sym {
					return i
				}
				continue
			}
			child := resultChildAt(node, i)
			if child != nil && child.symbol == sym {
				return i
			}
		}
		return -1
	}
	for i := 0; i < childCount; i++ {
		child := resultChildAt(node, i)
		if child != nil && child.Type(lang) == typ {
			return i
		}
	}
	return -1
}

func pythonCallIdentifierNode(node *Node, lang *Language) (*Node, bool) {
	if node == nil || lang == nil || node.Type(lang) != "call" || resultChildCount(node) == 0 {
		return nil, false
	}
	fn := resultChildAt(node, 0)
	if fn != nil && fn.Type(lang) == "identifier" {
		return fn, true
	}
	return nil, false
}

func pythonNodeTextEqual(a, b *Node, source []byte) bool {
	if a == nil || b == nil || len(source) == 0 {
		return false
	}
	if a.startByte >= a.endByte || b.startByte >= b.endByte {
		return false
	}
	if int(a.endByte) > len(source) || int(b.endByte) > len(source) {
		return false
	}
	if a.endByte-a.startByte != b.endByte-b.startByte {
		return false
	}
	return bytes.Equal(source[a.startByte:a.endByte], source[b.startByte:b.endByte])
}

func splitPythonOvernestedFunction(node *Node, arena *nodeArena, lang *Language) (*Node, []*Node, bool) {
	if node == nil || node.Type(lang) != "function_definition" {
		return node, nil, false
	}
	bodyIndex := pythonChildIndexByTypeNoMaterialize(node, lang, "block")
	if bodyIndex < 0 {
		return node, nil, false
	}
	body := resultChildAt(node, bodyIndex)
	bodyChildren := resultChildSliceForMutation(body)
	if body == nil || len(bodyChildren) == 0 {
		return node, nil, false
	}
	fnColumn := node.startPoint.Column
	hoistStart := -1
	for i, child := range bodyChildren {
		if child == nil || !child.IsNamed() {
			continue
		}
		if child.startPoint.Column <= fnColumn {
			hoistStart = i
			break
		}
	}
	if hoistStart <= 0 {
		return node, nil, false
	}

	kept := append([]*Node(nil), bodyChildren[:hoistStart]...)
	hoisted := append([]*Node(nil), bodyChildren[hoistStart:]...)
	if len(kept) == 0 {
		return node, nil, false
	}

	newBody := cloneNodeInArena(arena, body)
	if arena != nil {
		buf := arena.allocNodeSlice(len(kept))
		copy(buf, kept)
		kept = buf
	}
	newBody.children = kept
	newBody.fieldIDs = nil
	newBody.fieldSources = nil
	lastKept := kept[len(kept)-1]
	newBody.endByte = lastKept.endByte
	newBody.endPoint = lastKept.endPoint

	newFn := cloneNodeInArenaReplacingChildForMutation(arena, node, bodyIndex, newBody)
	newFn.endByte = newBody.endByte
	newFn.endPoint = newBody.endPoint
	return newFn, hoisted, true
}

func flattenPythonSimpleStatements(node *Node, out []*Node, lang *Language) []*Node {
	if node == nil {
		return out
	}
	switch node.Type(lang) {
	case "_simple_statements", "_simple_statements_repeat1":
		childCount := resultChildCount(node)
		for i := 0; i < childCount; i++ {
			child := resultChildAt(node, i)
			out = flattenPythonSimpleStatements(child, out, lang)
		}
		return out
	case "expression_statement":
		if resultChildCount(node) == 1 {
			child := resultChildAt(node, 0)
			if child != nil && child.IsNamed() {
				return append(out, child)
			}
		}
	}
	if node.IsNamed() || (lang != nil && node.Type(lang) == ";") {
		return append(out, node)
	}
	return out
}

func normalizePythonStringContinuationEscapes(root *Node, source []byte, lang *Language) normalizationPassCounters {
	var counters normalizationPassCounters
	if root == nil || lang == nil || lang.Name != "python" || len(source) == 0 {
		return counters
	}
	escapeSym, ok := symbolByName(lang, "escape_sequence")
	if !ok {
		return counters
	}
	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		counters.nodesVisited++
		if n.Type(lang) == "string_content" && n.startByte < n.endByte && int(n.endByte) <= len(source) {
			children, changed := addPythonContinuationEscapes(n, source, escapeSym)
			if changed {
				n.children = children
				counters.nodesRewritten++
			}
			return
		}
		for i := 0; i < resultChildCount(n); i++ {
			walk(resultChildAt(n, i))
		}
	}
	walk(root)
	return counters
}

func addPythonContinuationEscapes(node *Node, source []byte, escapeSym Symbol) ([]*Node, bool) {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return nil, false
	}
	var children []*Node
	changed := false
	for i := int(node.startByte); i+1 < int(node.endByte); i++ {
		if source[i] != '\\' {
			continue
		}
		end := i + 2
		if source[i+1] == '\r' && end < int(node.endByte) && source[end] == '\n' {
			end++
		} else if source[i+1] != '\n' {
			continue
		}
		found := pythonChildSpanSymbolNoMaterialize(node, uint32(i), uint32(end), escapeSym)
		if changed {
			found = false
			for _, child := range children {
				if child != nil && child.startByte == uint32(i) && child.endByte == uint32(end) && child.symbol == escapeSym {
					found = true
					break
				}
			}
		}
		if found {
			i = end - 1
			continue
		}
		if !changed {
			children = resultChildSliceForMutation(node)
		}
		startPoint := advancePointByBytes(Point{}, source[:i])
		esc := newLeafNodeInArena(node.ownerArena, escapeSym, true, uint32(i), uint32(end), startPoint, advancePointByBytes(startPoint, source[i:end]))
		insertAt := len(children)
		for idx, child := range children {
			if child == nil || child.startByte > uint32(i) {
				insertAt = idx
				break
			}
		}
		next := make([]*Node, 0, len(children)+1)
		next = append(next, children[:insertAt]...)
		next = append(next, esc)
		next = append(next, children[insertAt:]...)
		if node.ownerArena != nil {
			buf := node.ownerArena.allocNodeSlice(len(next))
			copy(buf, next)
			next = buf
		}
		children = next
		changed = true
		i = end - 1
	}
	if !changed {
		return nil, false
	}
	return children, true
}

func pythonChildSpanSymbolNoMaterialize(node *Node, start, end uint32, symbol Symbol) bool {
	childCount := resultChildCount(node)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		if !ok || !stackEntryHasNode(entry) {
			continue
		}
		if stackEntryNodeStartByte(entry) == start &&
			stackEntryNodeEndByte(entry) == end &&
			stackEntryNodeSymbol(entry) == symbol {
			return true
		}
	}
	return false
}

func pythonSyntheticClassFieldIDs(arena *nodeArena, childCount int, hasArgList bool, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("name"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if hasArgList {
		if fid, ok := lang.FieldByName("superclasses"); ok && childCount > 2 {
			fieldIDs[2] = fid
		}
		if fid, ok := lang.FieldByName("body"); ok && childCount > 4 {
			fieldIDs[4] = fid
		}
		return fieldIDs
	}
	if fid, ok := lang.FieldByName("body"); ok && childCount > 3 {
		fieldIDs[3] = fid
	}
	return fieldIDs
}

func pythonSyntheticFunctionFieldIDs(arena *nodeArena, childCount int, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("name"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if fid, ok := lang.FieldByName("parameters"); ok && childCount > 2 {
		fieldIDs[2] = fid
	}
	if fid, ok := lang.FieldByName("body"); ok && childCount > 4 {
		fieldIDs[4] = fid
	}
	return fieldIDs
}
func pythonSyntheticIfFieldIDs(arena *nodeArena, childCount int, lang *Language) []FieldID {
	fieldIDs := make([]FieldID, childCount)
	if arena != nil {
		fieldIDs = arena.allocFieldIDSlice(childCount)
	}
	if fid, ok := lang.FieldByName("condition"); ok && childCount > 1 {
		fieldIDs[1] = fid
	}
	if fid, ok := lang.FieldByName("consequence"); ok && childCount > 3 {
		fieldIDs[3] = fid
	}
	return fieldIDs
}

func pythonModuleChildrenLookComplete(nodes []*Node, lang *Language) bool {
	if len(nodes) == 0 {
		return false
	}
	seen := 0
	for _, n := range nodes {
		if n == nil || n.isExtra() {
			continue
		}
		if n.IsNamed() {
			seen++
			continue
		}
		switch n.Type(lang) {
		case "_simple_statements":
			seen++
		default:
			return false
		}
	}
	return seen > 0
}
