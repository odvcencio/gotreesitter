package gotreesitter

import "strings"

func normalizeCobolCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCobolRecoveredRootProgramDefinition(root, source, lang)
	normalizeCobolAcceptedRootProgramDefinition(root, source, lang)
	normalizeCobolZLiteralDataRootRecovery(root, source, lang)
	recoveredProcedureRoot := normalizeCobolProcedureRootRecovery(root, source, lang)
	normalizeCobolRootErrorLeadingComments(root, source, lang)
	normalizeCobolRootProcedureEvaluateError(root, source, lang)
	if !recoveredProcedureRoot {
		normalizeCobolRootProcedurePrefixError(root, source, lang)
	}
	normalizeCobolProcedureLooseIfHeaders(root, source, lang)
	normalizeCobolLeadingAreaStart(root, source, lang)
	normalizeCobolTopLevelDefinitionEnd(root, source, lang)
	normalizeCobolIfHeaderExecCICSClassErrors(root, source, lang)
	normalizeCobolRootProgramDefinitionSiblingEnds(root, source, lang)
	normalizeCobolDivisionSiblingEnds(root, source, lang)
	normalizeCobolSectionSiblingEnds(root, source, lang)
	normalizeCobolRecoveredErrorCommentEntries(root, source, lang)
	normalizeCobolRootCommentsCoveredByError(root, lang)
	normalizeCobolTrailingTriviaSpans(root, source, lang)
	normalizeCobolRecoveredParagraphHeader(root, source, lang)
	normalizeCobolProcedureTrailingParagraphCommentEntry(root, source, lang)
	normalizeCobolProcedureTrailingExecCICSSpans(root, source, lang)
	normalizeCobolRootExecCICSErrorMarkers(root, source, lang)
	normalizeCobolIfHeaderExecCICSProgramEnd(root, source, lang)
}
func normalizeCobolLeadingAreaStart(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || len(source) == 0 {
		return
	}
	rootStart, defStart, ok := cobolLeadingStarts(source)
	if !ok {
		return
	}
	setCobolNodeStart(root, source, rootStart)
	if resultChildCount(root) == 0 {
		return
	}
	def := cobolFirstProgramDefinition(root, lang)
	if def == nil {
		return
	}
	setCobolNodeStart(def, source, defStart)
	if resultChildCount(def) == 0 {
		return
	}
	for i := 0; i < resultChildCount(def); i++ {
		child := resultChildAt(def, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "identification_division" {
			setCobolNodeStart(child, source, defStart)
			break
		}
	}
}

func cobolLeadingStarts(source []byte) (uint32, uint32, bool) {
	start := firstNonWhitespaceByte(source)
	if start != 0 {
		if cobolByteColumn(source, start) == 6 && cobolFixedFormatCommentIndicator(source[start]) {
			if defStart, ok := cobolFirstFixedFormatCodeStart(source, int(start)); ok {
				return start, defStart, true
			}
		}
		return start, start, true
	}
	if len(source) < 7 {
		return 0, 0, false
	}
	switch source[6] {
	case ' ':
		contentStart, ok := firstNonWhitespaceByteFrom(source, 7)
		if !ok {
			return 0, 0, false
		}
		return 6, contentStart, true
	case '*', '/':
		if defStart, ok := cobolFirstFixedFormatCodeStart(source, 6); ok {
			return 6, defStart, true
		}
		return 6, 6, true
	case '-':
		return 6, 6, true
	default:
		return 0, 0, false
	}
}

func cobolFirstFixedFormatCodeStart(source []byte, start int) (uint32, bool) {
	if start < 0 {
		start = 0
	}
	for lineStart := cobolLineStart(source, start); lineStart < len(source); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		if cobolLineHasContent(source, lineStart, lineEnd) {
			if indicator := lineStart + 6; indicator < lineEnd {
				switch source[indicator] {
				case '*', '/':
					lineStart = cobolNextLineStart(source, lineEnd)
					continue
				case ' ':
					if contentStart, ok := firstNonWhitespaceByteInRange(source, indicator+1, lineEnd); ok {
						return contentStart, true
					}
					lineStart = cobolNextLineStart(source, lineEnd)
					continue
				}
			}
			if contentStart, ok := firstNonWhitespaceByteInRange(source, lineStart, lineEnd); ok {
				return contentStart, true
			}
		}
		lineStart = cobolNextLineStart(source, lineEnd)
	}
	return 0, false
}

func cobolLineStart(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos > 0 && source[pos-1] != '\n' && source[pos-1] != '\r' {
		pos--
	}
	return pos
}

func cobolNextLineStart(source []byte, lineEnd int) int {
	for lineEnd < len(source) {
		ch := source[lineEnd]
		lineEnd++
		if ch == '\n' {
			break
		}
		if ch == '\r' {
			if lineEnd < len(source) && source[lineEnd] == '\n' {
				lineEnd++
			}
			break
		}
	}
	return lineEnd
}

func cobolLineHasContent(source []byte, start, end int) bool {
	_, ok := firstNonWhitespaceByteInRange(source, start, end)
	return ok
}

func firstNonWhitespaceByteInRange(source []byte, start, end int) (uint32, bool) {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	for i := start; i < end; i++ {
		switch source[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return uint32(i), true
		}
	}
	return 0, false
}

func cobolByteColumn(source []byte, pos uint32) int {
	i := int(pos)
	if i > len(source) {
		i = len(source)
	}
	start := cobolLineStart(source, i)
	return i - start
}

func cobolFixedFormatCommentIndicator(b byte) bool {
	return b == '*' || b == '/'
}

func normalizeCobolRecoveredRootProgramDefinition(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) != 1 {
		return
	}
	recovered := resultChildAt(root, 0)
	if recovered == nil || recovered.symbol != errorSymbol || resultChildCount(recovered) == 0 {
		return
	}
	children := resultChildSliceForMutation(recovered)
	if len(children) == 0 {
		return
	}
	idIdx := cobolFirstChildIndexOfType(children, lang, "identification_division", 0)
	if idIdx < 0 || !cobolOnlyRootLeadingComments(children[:idIdx], lang) {
		return
	}
	dataIdx := cobolFirstChildIndexOfType(children, lang, "data_division", idIdx+1)
	if dataIdx < 0 {
		return
	}
	procStart, ok := cobolProcedureDivisionSourceStart(source, children[dataIdx].startByte)
	if !ok {
		return
	}
	procChildStart := cobolFirstChildIndexStartingAtOrAfter(children, dataIdx+1, procStart)
	if procChildStart < 0 {
		return
	}
	for i := dataIdx + 1; i < procChildStart; i++ {
		child := children[i]
		if child == nil || child.IsExtra() || child.Type(lang) != "comment" {
			return
		}
	}
	trailingStart := cobolRootTrailingCommentStart(children, procChildStart, lang)
	if trailingStart <= procChildStart {
		return
	}
	tailStart := cobolRecoveredTailStart(children, procChildStart, trailingStart, lang)
	if tailStart < procChildStart || tailStart > trailingStart {
		return
	}
	procChildrenEnd := tailStart
	if procChildrenEnd == procChildStart {
		return
	}
	programSym, ok := symbolByName(lang, "program_definition")
	if !ok {
		return
	}
	procedureSym, ok := symbolByName(lang, "procedure_division")
	if !ok {
		return
	}

	rootStart, rootStartPoint := root.startByte, root.startPoint
	rootEnd, rootEndPoint := root.endByte, root.endPoint

	procedureChildren := cloneNodeSliceInArena(root.ownerArena, children[procChildStart:procChildrenEnd])
	procedureNamed := symbolIsNamed(lang, procedureSym)
	procedure := newParentNodeInArena(root.ownerArena, procedureSym, procedureNamed, procedureChildren, nil, 0)
	procedure.startByte = procStart
	procedure.startPoint = advancePointByBytes(Point{}, source[:procStart])
	cobolRefreshHasErrorFromChildren(procedure)

	programChildren := make([]*Node, 0, procChildStart-idIdx+1)
	programChildren = append(programChildren, children[idIdx:procChildStart]...)
	programChildren = append(programChildren, procedure)
	programNamed := symbolIsNamed(lang, programSym)
	program := newParentNodeInArena(root.ownerArena, programSym, programNamed, cloneNodeSliceInArena(root.ownerArena, programChildren), nil, 0)
	cobolRefreshHasErrorFromChildren(program)

	rootChildren := make([]*Node, 0, idIdx+1+1+len(children)-trailingStart)
	rootChildren = append(rootChildren, children[:idIdx]...)
	rootChildren = append(rootChildren, program)
	if tailStart < trailingStart {
		tailChildren := cloneNodeSliceInArena(root.ownerArena, children[tailStart:trailingStart])
		tail := newParentNodeInArena(root.ownerArena, errorSymbol, true, tailChildren, nil, 0)
		tail.setExtra(true)
		tail.setHasError(true)
		normalizeCobolRecoveredTailErrorChildren(tail, source, lang)
		rootChildren = append(rootChildren, tail)
	}
	rootChildren = append(rootChildren, children[trailingStart:]...)
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, rootChildren))
	root.startByte = rootStart
	root.startPoint = rootStartPoint
	root.endByte = rootEnd
	root.endPoint = rootEndPoint
	cobolRefreshHasErrorFromChildren(root)
}

func normalizeCobolAcceptedRootProgramDefinition(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() {
		return
	}
	if cobolFirstProgramDefinition(root, lang) != nil {
		return
	}
	children := resultChildSliceForMutation(root)
	if len(children) == 0 {
		return
	}
	idIdx := cobolFirstChildIndexOfType(children, lang, "identification_division", 0)
	if idIdx < 0 || !cobolOnlyRootLeadingComments(children[:idIdx], lang) {
		return
	}
	dataIdx := cobolFirstChildIndexOfType(children, lang, "data_division", idIdx+1)
	if dataIdx < 0 {
		return
	}
	procStart, ok := cobolProcedureDivisionSourceStart(source, children[dataIdx].startByte)
	if !ok {
		return
	}
	procIdx := cobolFirstChildIndexStartingAtOrAfter(children, dataIdx+1, procStart)
	if procIdx < 0 {
		return
	}
	trailingStart := cobolRootTrailingCommentStart(children, procIdx+1, lang)
	if trailingStart <= procIdx+1 || !cobolChildRangeHasError(children[procIdx+1:trailingStart]) {
		return
	}
	programSym, ok := symbolByName(lang, "program_definition")
	if !ok {
		return
	}
	procedureSym, ok := symbolByName(lang, "procedure_division")
	if !ok {
		return
	}

	procedureNamed := symbolIsNamed(lang, procedureSym)
	var procedure *Node
	procChild := children[procIdx]
	if procChild != nil && !procChild.IsExtra() && procChild.Type(lang) == "procedure_division" {
		procedure = procChild
	} else if procChild != nil && !procChild.IsExtra() && procChild.Type(lang) == "." {
		procedure = newParentNodeInArena(root.ownerArena, procedureSym, procedureNamed, cloneNodeSliceInArena(root.ownerArena, []*Node{procChild}), nil, 0)
		procedure.startByte = procStart
		procedure.startPoint = advancePointByBytes(Point{}, source[:procStart])
		cobolRefreshHasErrorFromChildren(procedure)
	} else {
		return
	}

	programChildren := make([]*Node, 0, procIdx-idIdx+1)
	programChildren = append(programChildren, children[idIdx:procIdx]...)
	programChildren = append(programChildren, procedure)
	programNamed := symbolIsNamed(lang, programSym)
	program := newParentNodeInArena(root.ownerArena, programSym, programNamed, cloneNodeSliceInArena(root.ownerArena, programChildren), nil, 0)
	cobolRefreshHasErrorFromChildren(program)

	tailChildren := cloneNodeSliceInArena(root.ownerArena, children[procIdx+1:trailingStart])
	tail := newParentNodeInArena(root.ownerArena, errorSymbol, true, tailChildren, nil, 0)
	if tailStart, ok := firstNonWhitespaceByteFrom(source, int(procedure.endByte)); ok && tailStart < tail.startByte {
		tail.startByte = tailStart
		tail.startPoint = advancePointByBytes(Point{}, source[:tailStart])
	}
	tailEnd := uint32(len(source))
	if trailingStart < len(children) && children[trailingStart] != nil {
		tailEnd = children[trailingStart].startByte
	}
	if tailEnd > tail.endByte {
		tail.endByte = tailEnd
		tail.endPoint = advancePointByBytes(Point{}, source[:tailEnd])
	}
	tail.setExtra(true)
	tail.setHasError(true)
	normalizeCobolRecoveredTailErrorChildren(tail, source, lang)

	rootChildren := make([]*Node, 0, idIdx+1+1+len(children)-trailingStart)
	rootChildren = append(rootChildren, children[:idIdx]...)
	rootChildren = append(rootChildren, program)
	rootChildren = append(rootChildren, tail)
	rootChildren = append(rootChildren, children[trailingStart:]...)
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, rootChildren))
	cobolRefreshHasErrorFromChildren(root)
}

func cobolChildRangeHasError(children []*Node) bool {
	for _, child := range children {
		if child != nil && (child.IsError() || child.HasError()) {
			return true
		}
	}
	return false
}

func normalizeCobolZLiteralDataRootRecovery(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() {
		return
	}
	commentEntrySym, ok := symbolByName(lang, "comment_entry")
	if !ok {
		return
	}
	commentSym, ok := symbolByName(lang, "comment")
	if !ok {
		return
	}
	children := resultChildSliceForMutation(root)
	for i, def := range children {
		if def == nil || def.Type(lang) != "program_definition" {
			continue
		}
		dataDesc := cobolFirstZLiteralDataDescription(def, source, lang)
		if dataDesc == nil {
			continue
		}
		errIdx := cobolFirstRootErrorIndex(children, i+1)
		if errIdx < 0 {
			continue
		}
		err := children[errIdx]
		prefix, markerStart := cobolZLiteralRecoveryPrefix(dataDesc, lang)
		if len(prefix) == 0 || markerStart == 0 || markerStart > err.endByte {
			continue
		}
		defEnd := lastNonTriviaByteEnd(source[:dataDesc.startByte])
		if defEnd == 0 || defEnd <= def.startByte {
			continue
		}

		tailChildren := make([]*Node, 0, len(prefix)+64)
		tailChildren = append(tailChildren, prefix...)
		tailChildren = cobolAppendRecoveryLineMarkers(tailChildren, root.ownerArena, source, markerStart, err.endByte, lang, commentSym, symbolIsNamed(lang, commentSym), commentEntrySym, symbolIsNamed(lang, commentEntrySym))
		if len(tailChildren) == len(prefix) {
			continue
		}

		cobolTrimNodeEndForRecovery(def, source, defEnd)
		replaceNodeChildrenUnfielded(err, cloneNodeSliceInArena(err.ownerArena, tailChildren))
		err.startByte = dataDesc.startByte
		err.startPoint = dataDesc.startPoint
		err.setExtra(true)
		err.setHasError(true)
		cobolRefreshHasErrorFromChildren(root)
		return
	}
}

func cobolFirstZLiteralDataDescription(root *Node, source []byte, lang *Language) *Node {
	var found *Node
	walkResultTree(root, func(n *Node) {
		if found != nil || n == nil || n.Type(lang) != "data_description" || int(n.endByte) > len(source) {
			return
		}
		if cobolIndexFoldASCII(source, int(n.startByte), int(n.endByte), "value z'") >= 0 ||
			cobolIndexFoldASCII(source, int(n.startByte), int(n.endByte), "value z\"") >= 0 {
			found = n
		}
	})
	return found
}

func cobolFirstRootErrorIndex(children []*Node, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(children); i++ {
		child := children[i]
		if child != nil && child.symbol == errorSymbol {
			return i
		}
	}
	return -1
}

func cobolZLiteralRecoveryPrefix(dataDesc *Node, lang *Language) ([]*Node, uint32) {
	children := resultChildSliceForMutation(dataDesc)
	prefix := make([]*Node, 0, len(children))
	var markerStart uint32
	for _, child := range children {
		if child == nil {
			continue
		}
		if child.Type(lang) == "value_clause" {
			break
		}
		prefix = append(prefix, child)
		markerStart = child.endByte
	}
	return prefix, markerStart
}

func cobolAppendRecoveryLineMarkers(out []*Node, arena *nodeArena, source []byte, start, end uint32, lang *Language, commentSym Symbol, commentNamed bool, commentEntrySym Symbol, commentEntryNamed bool) []*Node {
	if start >= end || int(end) > len(source) {
		return out
	}
	scan := int(start)
	for lineStart := cobolLineStart(source, scan); lineStart < int(end); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		if lineEnd > int(end) {
			lineEnd = int(end)
		}
		contentStart := scan
		if contentStart < lineStart {
			contentStart = lineStart
		}
		if contentStart < lineEnd && cobolLineHasContent(source, contentStart, lineEnd) {
			sym, named := commentEntrySym, commentEntryNamed
			if cobolLineLooksFixedFormatComment(source, lineStart) {
				sym, named = commentSym, commentNamed
			}
			point := advancePointByBytes(Point{}, source[:lineEnd])
			out = append(out, newLeafNodeInArena(arena, sym, named, uint32(lineEnd), uint32(lineEnd), point, point))
		}
		lineStart = cobolNextLineStart(source, lineEnd)
		scan = lineStart
	}
	return cobolDeduplicateAdjacentZeroWidthMarkers(out, lang)
}

func normalizeCobolProcedureRootRecovery(root *Node, source []byte, lang *Language) bool {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" {
		return false
	}
	children := resultChildSliceForMutation(root)
	for i := 0; i+1 < len(children); i++ {
		def := children[i]
		err := children[i+1]
		if def == nil || err == nil || def.Type(lang) != "program_definition" || err.symbol != errorSymbol {
			continue
		}
		proc := cobolLastChildOfType(def, lang, "procedure_division")
		splitIdx, headerEnd, ok := cobolProcedureDivisionHeaderEnd(proc, lang)
		if !ok || splitIdx >= resultChildCount(proc) {
			continue
		}
		moved := resultChildSliceForMutation(proc)[splitIdx:]
		codeStart, ok := cobolFirstRootRecoveryCodeStart(source, int(headerEnd))
		if !ok || codeStart >= err.endByte {
			continue
		}

		hoistedEnd := 0
		for hoistedEnd < len(moved) {
			child := moved[hoistedEnd]
			if child == nil || child.IsExtra() || child.Type(lang) != "comment" {
				break
			}
			hoistedEnd++
		}
		tailMoved := moved[hoistedEnd:]
		if !cobolProcedureRecoveryTailIsHeaderAdjacent(tailMoved, err.startByte, lang) {
			continue
		}
		if len(tailMoved) == 0 && hoistedEnd == 0 {
			continue
		}

		cobolTrimNodeEndForRecovery(def, source, headerEnd)
		err.startByte = codeStart
		err.startPoint = advancePointByBytes(Point{}, source[:codeStart])
		if len(tailMoved) > 0 {
			tailChildren := make([]*Node, 0, len(tailMoved)+resultChildCount(err))
			tailChildren = append(tailChildren, tailMoved...)
			tailChildren = append(tailChildren, resultChildSliceForMutation(err)...)
			replaceNodeChildrenUnfielded(err, cloneNodeSliceInArena(root.ownerArena, tailChildren))
			normalizeCobolRecoveredTailErrorChildren(err, source, lang)
		}
		err.setExtra(true)
		err.setHasError(true)

		rootChildren := make([]*Node, 0, len(children)+hoistedEnd)
		rootChildren = append(rootChildren, children[:i+1]...)
		rootChildren = append(rootChildren, moved[:hoistedEnd]...)
		rootChildren = append(rootChildren, err)
		rootChildren = append(rootChildren, children[i+2:]...)
		replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, rootChildren))
		cobolRefreshHasErrorFromChildren(root)
		return true
	}
	return false
}

func cobolProcedureRecoveryTailIsHeaderAdjacent(children []*Node, errorStart uint32, lang *Language) bool {
	for _, child := range children {
		if child == nil || child.IsExtra() || child.startByte >= errorStart {
			continue
		}
		if child.Type(lang) == "paragraph_header" {
			return false
		}
	}
	return true
}

func normalizeCobolRootErrorLeadingComments(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() {
		return
	}
	commentSym, ok := symbolByName(lang, "comment")
	if !ok {
		return
	}
	commentNamed := symbolIsNamed(lang, commentSym)
	children := resultChildSliceForMutation(root)
	for i := 0; i < len(children); i++ {
		err := children[i]
		if err == nil || err.symbol != errorSymbol {
			continue
		}
		comments, codeStart, ok := cobolLeadingFixedFormatCommentNodes(root.ownerArena, source, err.startByte, err.endByte, commentSym, commentNamed)
		if !ok || len(comments) == 0 || codeStart <= err.startByte {
			continue
		}
		err.startByte = codeStart
		err.startPoint = advancePointByBytes(Point{}, source[:codeStart])
		errChildren := resultChildSliceForMutation(err)
		drop := 0
		for drop < len(errChildren) {
			child := errChildren[drop]
			if child == nil || child.startByte >= codeStart || child.Type(lang) != "comment" {
				break
			}
			drop++
		}
		if drop > 0 {
			replaceNodeChildrenUnfielded(err, cloneNodeSliceInArena(err.ownerArena, errChildren[drop:]))
			err.startByte = codeStart
			err.startPoint = advancePointByBytes(Point{}, source[:codeStart])
		}

		rootChildren := make([]*Node, 0, len(children)+len(comments))
		rootChildren = append(rootChildren, children[:i]...)
		rootChildren = append(rootChildren, comments...)
		rootChildren = append(rootChildren, err)
		rootChildren = append(rootChildren, children[i+1:]...)
		replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, rootChildren))
		cobolRefreshHasErrorFromChildren(root)
		return
	}
}

func normalizeCobolRootProcedureEvaluateError(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || len(source) == 0 {
		return
	}
	children := resultChildSliceForMutation(root)
	for i := 0; i < len(children); i++ {
		def := children[i]
		if def == nil || def.IsExtra() || def.IsError() || def.IsMissing() || def.Type(lang) != "program_definition" {
			continue
		}
		proc := cobolLastChildOfType(def, lang, "procedure_division")
		_, headerEnd, ok := cobolProcedureDivisionHeaderEnd(proc, lang)
		if !ok || proc.endByte > headerEnd {
			continue
		}
		errIdx, ok := cobolRootProcedureErrorIndex(children, i+1, lang)
		if !ok {
			continue
		}
		err := children[errIdx]
		if err == nil || err.symbol != errorSymbol || resultChildCount(err) == 0 || int(err.endByte) > len(source) {
			continue
		}
		errChildren := resultChildSliceForMutation(err)
		firstCode := cobolFirstNonCommentChildType(errChildren, lang)
		if firstCode != "evaluate_header" || cobolIndexFoldASCII(source, int(err.startByte), int(err.endByte), "exec cics") >= 0 {
			continue
		}

		prefix := make([]*Node, 0, errIdx-i-1+len(errChildren))
		prefix = append(prefix, children[i+1:errIdx]...)
		prefix = append(prefix, errChildren...)
		procStart, procStartPoint := proc.startByte, proc.startPoint
		if headerStart, ok := cobolProcedureDivisionHeaderStart(source, headerEnd); ok {
			procStart = headerStart
			procStartPoint = advancePointByBytes(Point{}, source[:headerStart])
		}
		procChildren := append(resultChildSliceForMutation(proc), prefix...)
		replaceNodeChildrenUnfielded(proc, cloneNodeSliceInArena(proc.ownerArena, procChildren))
		proc.startByte = procStart
		proc.startPoint = procStartPoint
		setCobolNodeEnd(proc, source, err.endByte)
		setCobolNodeEnd(def, source, err.endByte)
		cobolRefreshHasErrorFromChildren(proc)
		cobolRefreshHasErrorFromChildren(def)

		rootChildren := make([]*Node, 0, len(children)-(errIdx-i))
		rootChildren = append(rootChildren, children[:i+1]...)
		rootChildren = append(rootChildren, children[errIdx+1:]...)
		replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, rootChildren))
		cobolRefreshHasErrorFromChildren(root)
		return
	}
}

func normalizeCobolRootProcedurePrefixError(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() || len(source) == 0 {
		return
	}
	children := resultChildSliceForMutation(root)
	for i := 0; i < len(children); i++ {
		def := children[i]
		if def == nil || def.IsExtra() || def.IsError() || def.IsMissing() || def.Type(lang) != "program_definition" {
			continue
		}
		proc := cobolLastChildOfType(def, lang, "procedure_division")
		_, headerEnd, ok := cobolProcedureDivisionHeaderEnd(proc, lang)
		if !ok || proc.endByte > headerEnd {
			continue
		}
		errIdx, ok := cobolRootProcedurePrefixErrorIndex(children, i+1, lang)
		if !ok || errIdx <= i+1 {
			continue
		}
		err := children[errIdx]
		if err == nil || err.symbol != errorSymbol || resultChildCount(err) == 0 {
			continue
		}
		splitEnd, tailStart, ok := cobolFirstExecCICSTailSplit(source, err.startByte, err.endByte)
		if !ok || splitEnd <= proc.endByte || tailStart <= splitEnd || tailStart >= err.endByte {
			continue
		}
		errChildren := resultChildSliceForMutation(err)
		prefixEnd := cobolRecoveredProcedurePrefixEnd(errChildren, splitEnd, tailStart)
		if prefixEnd <= 0 {
			continue
		}
		prefix := make([]*Node, 0, errIdx-i-1+prefixEnd)
		prefix = append(prefix, children[i+1:errIdx]...)
		for j := 0; j < prefixEnd; j++ {
			child := errChildren[j]
			if child == nil {
				continue
			}
			if child.endByte > splitEnd {
				cobolTrimNodeEndForRecovery(child, source, splitEnd)
			}
			normalizeCobolRecoveredProcedurePrefixNode(child, source, lang)
			prefix = append(prefix, child)
		}
		if len(prefix) == 0 || !cobolProcedurePrefixCanMove(prefix, lang) {
			continue
		}

		procStart, procStartPoint := proc.startByte, proc.startPoint
		procChildren := append(resultChildSliceForMutation(proc), prefix...)
		replaceNodeChildrenUnfielded(proc, cloneNodeSliceInArena(proc.ownerArena, procChildren))
		proc.startByte = procStart
		proc.startPoint = procStartPoint
		setCobolNodeEnd(proc, source, splitEnd)
		setCobolNodeEnd(def, source, splitEnd)
		cobolRefreshHasErrorFromChildren(proc)
		cobolRefreshHasErrorFromChildren(def)

		replaceNodeChildrenUnfielded(err, cloneNodeSliceInArena(err.ownerArena, errChildren[prefixEnd:]))
		err.startByte = tailStart
		err.startPoint = advancePointByBytes(Point{}, source[:tailStart])
		err.setExtra(true)
		err.setHasError(true)
		normalizeCobolRecoveredTailErrorChildren(err, source, lang)

		rootChildren := make([]*Node, 0, len(children)-(errIdx-i-1))
		rootChildren = append(rootChildren, children[:i+1]...)
		rootChildren = append(rootChildren, err)
		rootChildren = append(rootChildren, children[errIdx+1:]...)
		replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, rootChildren))
		cobolRefreshHasErrorFromChildren(root)
		return
	}
}

func cobolProcedurePrefixCanMove(children []*Node, lang *Language) bool {
	firstCodeType := ""
	for _, child := range children {
		if child == nil || child.IsMissing() {
			continue
		}
		typ := child.Type(lang)
		if typ == "copy_statement" {
			return true
		}
		if firstCodeType == "" && typ != "comment" && typ != "comment_entry" {
			firstCodeType = typ
		}
	}
	return firstCodeType == "if_header"
}

func cobolRootProcedureErrorIndex(children []*Node, start int, lang *Language) (int, bool) {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(children); i++ {
		child := children[i]
		if child == nil || child.IsMissing() {
			continue
		}
		if child.symbol == errorSymbol {
			return i, true
		}
		if child.IsExtra() || child.IsError() || child.Type(lang) != "comment" {
			return -1, false
		}
	}
	return -1, false
}

func cobolFirstNonCommentChildType(children []*Node, lang *Language) string {
	for _, child := range children {
		if child == nil || child.IsMissing() {
			continue
		}
		typ := child.Type(lang)
		if typ != "comment" && typ != "comment_entry" {
			return typ
		}
	}
	return ""
}

func cobolProcedureDivisionHeaderStart(source []byte, headerEnd uint32) (uint32, bool) {
	if int(headerEnd) > len(source) {
		return 0, false
	}
	pos := int(headerEnd)
	if pos > 0 {
		pos--
	}
	lineStart := cobolLineStart(source, pos)
	idx := cobolIndexFoldASCII(source, lineStart, int(headerEnd), "procedure division")
	if idx < 0 {
		return 0, false
	}
	return uint32(idx), true
}

func cobolRootProcedurePrefixErrorIndex(children []*Node, start int, lang *Language) (int, bool) {
	if start < 0 {
		start = 0
	}
	sawComment := false
	for i := start; i < len(children); i++ {
		child := children[i]
		if child == nil || child.IsMissing() {
			continue
		}
		if child.symbol == errorSymbol {
			return i, sawComment
		}
		if child.IsExtra() || child.IsError() || child.Type(lang) != "comment" {
			return -1, false
		}
		sawComment = true
	}
	return -1, false
}

func cobolFirstExecCICSTailSplit(source []byte, start, end uint32) (uint32, uint32, bool) {
	if start >= end || int(end) > len(source) {
		return 0, 0, false
	}
	for lineStart := cobolLineStart(source, int(start)); lineStart < int(end); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		if lineEnd > int(end) {
			lineEnd = int(end)
		}
		scanStart := int(start)
		if scanStart < lineStart {
			scanStart = lineStart
		}
		if scanStart < lineEnd && !cobolLineLooksFixedFormatComment(source, lineStart) {
			idx := cobolIndexFoldASCII(source, scanStart, lineEnd, "exec cics")
			if idx >= 0 {
				cicsStart := idx + len("exec ")
				splitEnd := lastNonTriviaByteEnd(source[:cicsStart])
				tailStart, ok := firstNonWhitespaceByteInRange(source, int(splitEnd), lineEnd)
				if ok && splitEnd > start && tailStart >= uint32(cicsStart) && splitEnd < tailStart {
					return splitEnd, tailStart, true
				}
			}
		}
		next := cobolNextLineStart(source, lineEnd)
		if next <= lineStart {
			break
		}
		lineStart = next
	}
	return 0, 0, false
}

func cobolRecoveredProcedurePrefixEnd(children []*Node, splitEnd, tailStart uint32) int {
	for i, child := range children {
		if child == nil {
			continue
		}
		if child.startByte >= tailStart {
			return i
		}
		if child.endByte > splitEnd {
			if child.startByte < splitEnd {
				return i + 1
			}
			return i
		}
	}
	return len(children)
}

func normalizeCobolRecoveredProcedurePrefixNode(n *Node, source []byte, lang *Language) {
	if n == nil || int(n.endByte) > len(source) {
		return
	}
	for i := 0; i < resultChildCount(n); i++ {
		normalizeCobolRecoveredProcedurePrefixNode(resultChildAt(n, i), source, lang)
	}
	if n.Type(lang) == "expr" && resultChildCount(n) == 1 {
		child := resultChildAt(n, 0)
		if child != nil && !child.IsExtra() && !child.IsMissing() && child.Type(lang) == "expr" &&
			child.startByte == n.startByte && child.endByte == n.endByte {
			replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, resultChildSliceForMutation(child)))
		}
	}
	if !cobolTrailingTriviaLeafCanTrim(n.Type(lang)) {
		cobolRefreshHasErrorFromChildren(n)
		return
	}
	last := (*Node)(nil)
	for i := resultChildCount(n) - 1; i >= 0; i-- {
		child := resultChildAt(n, i)
		if child == nil || child.IsMissing() {
			continue
		}
		last = child
		break
	}
	if last != nil && last.endByte > n.startByte && last.endByte < n.endByte {
		setCobolNodeEnd(n, source, last.endByte)
	}
	cobolRefreshHasErrorFromChildren(n)
}

func normalizeCobolProcedureLooseIfHeaders(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() || len(source) == 0 {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	changed := false
	for i := 0; i < resultChildCount(program); i++ {
		child := resultChildAt(program, i)
		if child == nil || child.IsExtra() || child.IsError() || child.IsMissing() || child.Type(lang) != "procedure_division" {
			continue
		}
		if normalizeCobolProcedureLooseIfHeaderChildren(child, source, lang) {
			changed = true
		}
	}
	if !changed {
		return
	}
	cobolTrimProgramEndToLastProcedure(program, source, lang)
	cobolRefreshHasErrorFromChildren(program)
}

func normalizeCobolProcedureLooseIfHeaderChildren(proc *Node, source []byte, lang *Language) bool {
	children := resultChildSliceForMutation(proc)
	if len(children) < 3 {
		return false
	}
	ifHeaderSym, ok := symbolByName(lang, "if_header")
	if !ok {
		return false
	}
	exprSym, ok := symbolByName(lang, "expr")
	if !ok {
		return false
	}
	changed := false
	for i := 0; i+2 < len(children); i++ {
		left, op, right := children[i], children[i+1], children[i+2]
		if !cobolLooseIfHeaderParts(left, op, right, lang) {
			continue
		}
		ifStart, ok := cobolIfKeywordStartBefore(source, left.startByte)
		if !ok {
			continue
		}
		lineEnd := cobolLineEnd(source, int(left.startByte))
		conditionEnd := lastNonTriviaByteEnd(source[:lineEnd])
		if conditionEnd <= right.startByte || conditionEnd > right.endByte {
			continue
		}
		if right.endByte > conditionEnd {
			cobolTrimNodeEndForRecovery(right, source, conditionEnd)
		}
		wasLast := i+3 == len(children)
		procStart, procStartPoint := proc.startByte, proc.startPoint
		expr := newParentNodeInArena(proc.ownerArena, exprSym, symbolIsNamed(lang, exprSym), cloneNodeSliceInArena(proc.ownerArena, []*Node{left, op, right}), nil, 0)
		expr.startByte = left.startByte
		expr.startPoint = left.startPoint
		expr.endByte = conditionEnd
		expr.endPoint = advancePointByBytes(Point{}, source[:conditionEnd])
		cobolRefreshHasErrorFromChildren(expr)
		header := newParentNodeInArena(proc.ownerArena, ifHeaderSym, symbolIsNamed(lang, ifHeaderSym), []*Node{expr}, nil, 0)
		header.startByte = ifStart
		header.startPoint = advancePointByBytes(Point{}, source[:ifStart])
		header.endByte = conditionEnd
		header.endPoint = expr.endPoint
		cobolRefreshHasErrorFromChildren(header)

		out := make([]*Node, 0, len(children)-2)
		out = append(out, children[:i]...)
		out = append(out, header)
		out = append(out, children[i+3:]...)
		children = cloneNodeSliceInArena(proc.ownerArena, out)
		replaceNodeChildrenUnfielded(proc, children)
		proc.startByte = procStart
		proc.startPoint = procStartPoint
		if wasLast {
			setCobolNodeEnd(proc, source, conditionEnd)
		}
		cobolRefreshHasErrorFromChildren(proc)
		changed = true
	}
	if changed {
		for i := resultChildCount(proc) - 1; i >= 0; i-- {
			last := resultChildAt(proc, i)
			if last == nil || last.IsMissing() {
				continue
			}
			if last.endByte < proc.endByte {
				setCobolNodeEnd(proc, source, last.endByte)
			}
			break
		}
	}
	return changed
}

func cobolLooseIfHeaderParts(left, op, right *Node, lang *Language) bool {
	if left == nil || op == nil || right == nil {
		return false
	}
	if left.IsExtra() || op.IsExtra() || right.IsExtra() || left.IsMissing() || op.IsMissing() || right.IsMissing() {
		return false
	}
	if left.Type(lang) != "qualified_word" || right.Type(lang) != "qualified_word" {
		return false
	}
	switch op.Type(lang) {
	case "eq", "ne":
		return true
	default:
		return false
	}
}

func cobolIfKeywordStartBefore(source []byte, operandStart uint32) (uint32, bool) {
	if int(operandStart) > len(source) {
		return 0, false
	}
	lineStart := cobolLineStart(source, int(operandStart))
	kwStart, ok := firstNonWhitespaceByteInRange(source, lineStart, int(operandStart))
	if !ok || int(kwStart)+2 > int(operandStart) {
		return 0, false
	}
	if !cobolEqualFoldASCII(source[kwStart:kwStart+2], "if") {
		return 0, false
	}
	for i := int(kwStart) + 2; i < int(operandStart); i++ {
		if source[i] != ' ' && source[i] != '\t' {
			return 0, false
		}
	}
	return kwStart, true
}

func normalizeCobolIfHeaderExecCICSClassErrors(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || len(source) == 0 {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	changed := false
	for i := 0; i < resultChildCount(program); i++ {
		child := resultChildAt(program, i)
		if child == nil || child.IsExtra() || child.IsError() || child.IsMissing() || child.Type(lang) != "procedure_division" {
			continue
		}
		if normalizeCobolProcedureIfHeaderExecCICSClassErrors(child, source, lang) {
			changed = true
		}
	}
	if !changed {
		return
	}
	if proc := cobolLastChildOfType(program, lang, "procedure_division"); proc != nil {
		cobolTrimNodeEndToLastRecoveredSpanChild(proc, source)
		if proc.endByte > program.endByte {
			setCobolNodeEnd(program, source, proc.endByte)
		}
		cobolTrimProgramEndToLastProcedure(program, source, lang)
	}
	cobolRefreshHasErrorFromChildren(program)
	cobolRefreshHasErrorFromChildren(root)
}

func normalizeCobolIfHeaderExecCICSProgramEnd(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || len(source) == 0 {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	proc := cobolLastChildOfType(program, lang, "procedure_division")
	if proc == nil || proc.endByte <= program.endByte || !cobolProcedureHasIfHeaderExecCICSClassError(proc, source, lang) {
		return
	}
	cobolTrimNodeEndToLastRecoveredSpanChild(proc, source)
	setCobolNodeEnd(program, source, proc.endByte)
	cobolRefreshHasErrorFromChildren(program)
	cobolRefreshHasErrorFromChildren(root)
}

func cobolProcedureHasIfHeaderExecCICSClassError(proc *Node, source []byte, lang *Language) bool {
	for i := 0; i < resultChildCount(proc); i++ {
		child := resultChildAt(proc, i)
		if child == nil || child.IsExtra() || child.IsMissing() || child.Type(lang) != "if_header" || !child.hasError() {
			continue
		}
		if _, _, _, ok := cobolIfHeaderExecCICSBounds(child, source); ok {
			return true
		}
	}
	return false
}

func normalizeCobolProcedureIfHeaderExecCICSClassErrors(proc *Node, source []byte, lang *Language) bool {
	changed := false
	for i := 0; i < resultChildCount(proc); i++ {
		child := resultChildAt(proc, i)
		if child == nil || child.IsExtra() || child.IsError() || child.IsMissing() || child.Type(lang) != "if_header" {
			continue
		}
		if normalizeCobolIfHeaderExecCICSClassError(child, source, lang) {
			changed = true
		}
	}
	if changed {
		cobolRefreshHasErrorFromChildren(proc)
	}
	return changed
}

func normalizeCobolIfHeaderExecCICSClassError(header *Node, source []byte, lang *Language) bool {
	if header == nil || header.hasError() || int(header.endByte) > len(source) {
		return false
	}
	execStart, cicsStart, cicsEnd, ok := cobolIfHeaderExecCICSBounds(header, source)
	if !ok {
		return false
	}
	condition := cobolFirstDescendantOfTypeBefore(header, lang, "qualified_word", execStart)
	if condition == nil {
		return false
	}
	exprSym, ok := symbolByName(lang, "expr")
	if !ok {
		return false
	}
	isClassSym, ok := symbolByName(lang, "is_class")
	if !ok {
		return false
	}
	wordSym, ok := symbolByName(lang, "WORD")
	if !ok {
		return false
	}

	execEnd := execStart + uint32(len("EXEC"))
	err := newLeafNodeInArena(header.ownerArena, errorSymbol, true, execStart, execEnd, advancePointByBytes(Point{}, source[:execStart]), advancePointByBytes(Point{}, source[:execEnd]))
	err.setHasError(true)
	cicsWord := newLeafNodeInArena(header.ownerArena, wordSym, symbolIsNamed(lang, wordSym), cicsStart, cicsEnd, advancePointByBytes(Point{}, source[:cicsStart]), advancePointByBytes(Point{}, source[:cicsEnd]))
	isClass := newParentNodeInArena(header.ownerArena, isClassSym, symbolIsNamed(lang, isClassSym), []*Node{condition, err, cicsWord}, nil, 0)
	expr := newParentNodeInArena(header.ownerArena, exprSym, symbolIsNamed(lang, exprSym), []*Node{isClass}, nil, 0)

	ifStart := header.startByte
	if int(ifStart) <= len(source) {
		if kwStart, ok := cobolIfKeywordStartBefore(source, condition.startByte); ok {
			ifStart = kwStart
		}
	}
	replaceNodeChildrenUnfielded(header, cloneNodeSliceInArena(header.ownerArena, []*Node{expr}))
	header.startByte = ifStart
	header.startPoint = advancePointByBytes(Point{}, source[:ifStart])
	header.endByte = cicsEnd
	header.endPoint = advancePointByBytes(Point{}, source[:cicsEnd])
	cobolRefreshHasErrorFromChildren(header)
	return true
}

func cobolIfHeaderExecCICSBounds(header *Node, source []byte) (uint32, uint32, uint32, bool) {
	if header == nil || int(header.startByte) >= len(source) || int(header.endByte) > len(source) || header.startByte >= header.endByte {
		return 0, 0, 0, false
	}
	exec := cobolIndexFoldASCII(source, int(header.startByte), int(header.endByte), "exec cics")
	if exec < 0 {
		return 0, 0, 0, false
	}
	lineStart := cobolLineStart(source, exec)
	codeStart, ok := firstNonWhitespaceByteInRange(source, lineStart, exec+len("EXEC"))
	if !ok || int(codeStart) != exec {
		return 0, 0, 0, false
	}
	execStart := uint32(exec)
	cicsStart := execStart + uint32(len("EXEC "))
	cicsEnd := cicsStart + uint32(len("CICS"))
	if int(cicsEnd) > len(source) || cicsEnd > header.endByte {
		return 0, 0, 0, false
	}
	return execStart, cicsStart, cicsEnd, true
}

func cobolFirstDescendantOfTypeBefore(n *Node, lang *Language, typ string, before uint32) *Node {
	if n == nil || n.startByte >= before {
		return nil
	}
	if !n.IsExtra() && !n.IsMissing() && !n.IsError() && n.endByte <= before && n.Type(lang) == typ {
		return n
	}
	for i := 0; i < resultChildCount(n); i++ {
		if found := cobolFirstDescendantOfTypeBefore(resultChildAt(n, i), lang, typ, before); found != nil {
			return found
		}
	}
	return nil
}

func cobolLineEnd(source []byte, pos int) int {
	if pos > len(source) {
		pos = len(source)
	}
	for pos < len(source) && source[pos] != '\n' && source[pos] != '\r' {
		pos++
	}
	return pos
}

func normalizeCobolProcedureTrailingExecCICSSpans(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() || len(source) == 0 {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	changed := false
	for i := 0; i < resultChildCount(program); i++ {
		proc := resultChildAt(program, i)
		if proc == nil || proc.IsExtra() || proc.IsError() || proc.IsMissing() || proc.Type(lang) != "procedure_division" {
			continue
		}
		last := cobolLastNonMissingChild(proc)
		if last == nil || last.Type(lang) != "if_header" || last.endByte >= proc.endByte {
			continue
		}
		if !cobolNextCodeStartsExecCICS(source, last.endByte) {
			continue
		}
		setCobolNodeEnd(proc, source, last.endByte)
		changed = true
	}
	if !changed {
		return
	}
	cobolTrimProgramEndToLastProcedure(program, source, lang)
	cobolRefreshHasErrorFromChildren(program)
}

func cobolLastNonMissingChild(parent *Node) *Node {
	if parent == nil {
		return nil
	}
	for i := resultChildCount(parent) - 1; i >= 0; i-- {
		child := resultChildAt(parent, i)
		if child != nil && !child.IsMissing() {
			return child
		}
	}
	return nil
}

func cobolNextCodeStartsExecCICS(source []byte, start uint32) bool {
	pos, ok := firstNonWhitespaceByteFrom(source, int(start))
	if !ok || int(pos) >= len(source) {
		return false
	}
	lineEnd := cobolLineEnd(source, int(pos))
	return cobolIndexFoldASCII(source, int(pos), lineEnd, "exec cics") == int(pos)
}

func normalizeCobolRootExecCICSErrorMarkers(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || !root.hasError() || len(source) == 0 {
		return
	}
	commentSym, ok := symbolByName(lang, "comment")
	if !ok {
		return
	}
	commentEntrySym, ok := symbolByName(lang, "comment_entry")
	if !ok {
		return
	}
	programIdx, program := cobolFirstProgramDefinitionIndex(root, lang)
	if program == nil {
		return
	}
	proc := cobolLastChildOfType(program, lang, "procedure_division")
	if proc == nil {
		return
	}
	execStart, ok := firstNonWhitespaceByteFrom(source, int(proc.endByte))
	if !ok || !cobolNextCodeStartsExecCICS(source, proc.endByte) {
		return
	}
	for i := programIdx + 1; i < resultChildCount(root); i++ {
		err := resultChildAt(root, i)
		if err == nil || err.symbol != errorSymbol || err.endByte <= execStart || int(err.endByte) > len(source) {
			continue
		}
		if err.startByte <= execStart {
			return
		}
		out := cobolAppendRecoveryLineMarkers(
			nil,
			err.ownerArena,
			source,
			execStart,
			err.endByte,
			lang,
			commentSym,
			symbolIsNamed(lang, commentSym),
			commentEntrySym,
			symbolIsNamed(lang, commentEntrySym),
		)
		if len(out) == 0 {
			return
		}
		replaceNodeChildrenUnfielded(err, cloneNodeSliceInArena(err.ownerArena, out))
		err.startByte = execStart
		err.startPoint = advancePointByBytes(Point{}, source[:execStart])
		err.endByte = out[len(out)-1].endByte
		err.endPoint = out[len(out)-1].endPoint
		err.setExtra(true)
		err.setHasError(true)
		cobolRefreshHasErrorFromChildren(root)
		return
	}
}

func cobolLeadingFixedFormatCommentNodes(arena *nodeArena, source []byte, start, end uint32, sym Symbol, named bool) ([]*Node, uint32, bool) {
	if start >= end || int(end) > len(source) {
		return nil, 0, false
	}
	var comments []*Node
	scan := int(start)
	for lineStart := cobolLineStart(source, scan); lineStart < int(end); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		if lineEnd > int(end) {
			lineEnd = int(end)
		}
		contentStart := scan
		if contentStart < lineStart {
			contentStart = lineStart
		}
		if contentStart < lineEnd && cobolLineHasContent(source, contentStart, lineEnd) {
			if cobolLineLooksFixedFormatComment(source, lineStart) {
				point := advancePointByBytes(Point{}, source[:lineEnd])
				comments = append(comments, newLeafNodeInArena(arena, sym, named, uint32(lineEnd), uint32(lineEnd), point, point))
				lineStart = cobolNextLineStart(source, lineEnd)
				scan = lineStart
				continue
			}
			codeStart, ok := firstNonWhitespaceByteInRange(source, contentStart, lineEnd)
			return comments, codeStart, ok
		}
		lineStart = cobolNextLineStart(source, lineEnd)
		scan = lineStart
	}
	return comments, 0, false
}

func cobolLastChildOfType(parent *Node, lang *Language, typ string) *Node {
	for i := resultChildCount(parent) - 1; i >= 0; i-- {
		child := resultChildAt(parent, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == typ {
			return child
		}
	}
	return nil
}

func cobolTrimProgramEndToLastProcedure(program *Node, source []byte, lang *Language) {
	last := cobolLastChildOfType(program, lang, "procedure_division")
	if last != nil && last.endByte < program.endByte {
		setCobolNodeEnd(program, source, last.endByte)
	}
}

func cobolTrimNodeEndToLastRecoveredSpanChild(n *Node, source []byte) {
	if n == nil || n.endByte <= n.startByte || int(n.endByte) > len(source) {
		return
	}
	last := cobolLastRecoveredSpanChild(n)
	if last != nil && last.endByte > n.startByte && last.endByte < n.endByte {
		setCobolNodeEnd(n, source, last.endByte)
	}
}

func cobolLastRecoveredSpanChild(parent *Node) *Node {
	if parent == nil {
		return nil
	}
	for i := resultChildCount(parent) - 1; i >= 0; i-- {
		child := resultChildAt(parent, i)
		if child == nil || child.IsMissing() {
			continue
		}
		if child.IsExtra() && !child.IsError() && !child.HasError() {
			continue
		}
		return child
	}
	return nil
}

func cobolProcedureDivisionHeaderEnd(proc *Node, lang *Language) (int, uint32, bool) {
	if proc == nil {
		return 0, 0, false
	}
	for i := 0; i < resultChildCount(proc); i++ {
		child := resultChildAt(proc, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "." {
			return i + 1, child.endByte, true
		}
	}
	return 0, 0, false
}

func cobolFirstRootRecoveryCodeStart(source []byte, start int) (uint32, bool) {
	if start < 0 {
		start = 0
	}
	if start > len(source) {
		return 0, false
	}
	for lineStart := cobolLineStart(source, start); lineStart < len(source); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		contentStart := start
		if contentStart < lineStart {
			contentStart = lineStart
		}
		if contentStart < lineEnd && cobolLineHasContent(source, contentStart, lineEnd) {
			if cobolLineLooksFixedFormatComment(source, lineStart) {
				lineStart = cobolNextLineStart(source, lineEnd)
				start = lineStart
				continue
			}
			return firstNonWhitespaceByteInRange(source, contentStart, lineEnd)
		}
		lineStart = cobolNextLineStart(source, lineEnd)
		start = lineStart
	}
	return 0, false
}

func cobolTrimNodeEndForRecovery(n *Node, source []byte, end uint32) {
	if n == nil || end == 0 || int(end) > len(source) || end >= n.endByte {
		return
	}
	startByte, startPoint := n.startByte, n.startPoint
	children := resultChildSliceForMutation(n)
	if len(children) > 0 {
		kept := make([]*Node, 0, len(children))
		for _, child := range children {
			if child == nil {
				continue
			}
			if child.startByte >= end {
				continue
			}
			if child.endByte > end {
				cobolTrimNodeEndForRecovery(child, source, end)
			}
			kept = append(kept, child)
		}
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, kept))
	}
	n.startByte = startByte
	n.startPoint = startPoint
	setCobolNodeEnd(n, source, end)
	cobolRefreshHasErrorFromChildren(n)
}

func cobolFirstChildIndexOfType(children []*Node, lang *Language, typ string, start int) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(children); i++ {
		child := children[i]
		if child != nil && !child.IsExtra() && child.Type(lang) == typ {
			return i
		}
	}
	return -1
}

func cobolOnlyRootLeadingComments(children []*Node, lang *Language) bool {
	for _, child := range children {
		if child == nil || child.IsExtra() || child.Type(lang) != "comment" {
			return false
		}
	}
	return true
}

func cobolProcedureDivisionSourceStart(source []byte, after uint32) (uint32, bool) {
	if int(after) > len(source) {
		return 0, false
	}
	idx := cobolIndexFoldASCII(source, int(after), len(source), "procedure division")
	if idx < 0 {
		return 0, false
	}
	return uint32(idx), true
}

func cobolFirstChildIndexStartingAtOrAfter(children []*Node, start int, byteStart uint32) int {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(children); i++ {
		child := children[i]
		if child != nil && child.startByte >= byteStart {
			return i
		}
	}
	return -1
}

func cobolRootTrailingCommentStart(children []*Node, min int, lang *Language) int {
	i := len(children)
	for i > min {
		child := children[i-1]
		if child == nil || child.IsExtra() || child.Type(lang) != "comment" {
			break
		}
		i--
	}
	return i
}

func cobolRecoveredTailStart(children []*Node, start, end int, lang *Language) int {
	firstErr := -1
	for i := start; i < end; i++ {
		child := children[i]
		if child != nil && (child.IsError() || child.HasError()) {
			firstErr = i
			break
		}
	}
	if firstErr < 0 {
		return end
	}
	tailStart := firstErr
	for i := firstErr - 1; i >= start; i-- {
		child := children[i]
		if child != nil && !child.IsExtra() && child.Type(lang) == "paragraph_header" {
			tailStart = i + 1
			break
		}
	}
	return tailStart
}

func normalizeCobolRecoveredTailErrorChildren(tail *Node, source []byte, lang *Language) {
	if tail == nil || tail.symbol != errorSymbol || int(tail.endByte) > len(source) {
		return
	}
	tailStart, tailStartPoint := tail.startByte, tail.startPoint
	commentEntrySym, ok := symbolByName(lang, "comment_entry")
	if !ok {
		return
	}
	commentEntryNamed := symbolIsNamed(lang, commentEntrySym)
	children := resultChildSliceForMutation(tail)
	if len(children) == 0 {
		return
	}
	out := make([]*Node, 0, len(children))
	cursor := tail.startByte
	for _, child := range children {
		if child == nil {
			continue
		}
		if cursor < child.startByte {
			out = cobolAppendCommentEntryMarkers(out, tail.ownerArena, source, cursor, child.startByte, commentEntrySym, commentEntryNamed)
			cursor = child.startByte
		}
		if child.IsError() || child.HasError() {
			out = cobolAppendCommentEntryMarkers(out, tail.ownerArena, source, cursor, child.endByte, commentEntrySym, commentEntryNamed)
			cursor = child.endByte
			continue
		}
		out = append(out, child)
		cursor = child.endByte
	}
	if cursor < tail.endByte {
		out = cobolAppendCommentEntryMarkers(out, tail.ownerArena, source, cursor, tail.endByte, commentEntrySym, commentEntryNamed)
	}
	if len(out) == 0 {
		return
	}
	out = cobolDeduplicateAdjacentZeroWidthMarkers(out, lang)
	replaceNodeChildrenUnfielded(tail, cloneNodeSliceInArena(tail.ownerArena, out))
	tail.startByte = tailStart
	tail.startPoint = tailStartPoint
	last := out[len(out)-1]
	tail.endByte = last.endByte
	tail.endPoint = last.endPoint
	tail.setExtra(true)
	tail.setHasError(true)
}

func normalizeCobolRecoveredErrorCommentEntries(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || len(source) == 0 {
		return
	}
	walkResultTreePostorder(root, func(n *Node) {
		if n == nil || n.symbol != errorSymbol || resultChildCount(n) == 0 || !cobolErrorHasCommentEntryChild(n, lang) {
			return
		}
		normalizeCobolRecoveredTailErrorChildren(n, source, lang)
	})
}

func normalizeCobolRootCommentsCoveredByError(root *Node, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" {
		return
	}
	children := resultChildSliceForMutation(root)
	out := make([]*Node, 0, len(children))
	changed := false
	for _, child := range children {
		if child != nil && child.Type(lang) == "comment" && cobolRootCommentCoveredByError(child, children) {
			changed = true
			continue
		}
		out = append(out, child)
	}
	if !changed {
		return
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, out))
	cobolRefreshHasErrorFromChildren(root)
}

func cobolRootCommentCoveredByError(comment *Node, siblings []*Node) bool {
	if comment == nil {
		return false
	}
	for _, sibling := range siblings {
		if sibling == nil || sibling == comment || sibling.symbol != errorSymbol {
			continue
		}
		if comment.startByte >= sibling.startByte && comment.endByte <= sibling.endByte {
			return true
		}
	}
	return false
}

func cobolErrorHasCommentEntryChild(n *Node, lang *Language) bool {
	for i := 0; i < resultChildCount(n); i++ {
		child := resultChildAt(n, i)
		if child != nil && child.Type(lang) == "comment_entry" {
			return true
		}
	}
	return false
}

func cobolDeduplicateAdjacentZeroWidthMarkers(nodes []*Node, lang *Language) []*Node {
	if len(nodes) < 2 {
		return nodes
	}
	out := nodes[:0]
	for _, node := range nodes {
		if len(out) > 0 && cobolSameZeroWidthMarker(out[len(out)-1], node, lang) {
			continue
		}
		out = append(out, node)
	}
	return out
}

func cobolSameZeroWidthMarker(a, b *Node, lang *Language) bool {
	if a == nil || b == nil || a.startByte != a.endByte || b.startByte != b.endByte {
		return false
	}
	if a.startByte != b.startByte || a.startPoint != b.startPoint || a.endPoint != b.endPoint {
		return false
	}
	typ := a.Type(lang)
	return typ == b.Type(lang) && (typ == "comment" || typ == "comment_entry")
}

func cobolAppendCommentEntryMarkers(out []*Node, arena *nodeArena, source []byte, start, end uint32, sym Symbol, named bool) []*Node {
	if start >= end || int(end) > len(source) {
		return out
	}
	for lineStart := cobolLineStart(source, int(start)); lineStart < int(end); {
		lineEnd := lineStart
		for lineEnd < len(source) && source[lineEnd] != '\n' && source[lineEnd] != '\r' {
			lineEnd++
		}
		if cobolLineLooksFixedFormatComment(source, lineStart) {
			next := cobolNextLineStart(source, lineEnd)
			if next <= lineStart {
				break
			}
			lineStart = next
			continue
		}
		markerEnd := cobolCommentEntryMarkerEnd(lineStart, lineEnd)
		if uint32(markerEnd) > start && uint32(markerEnd) <= end && cobolLineHasContent(source, lineStart, markerEnd) {
			point := advancePointByBytes(Point{}, source[:markerEnd])
			out = append(out, newLeafNodeInArena(arena, sym, named, uint32(markerEnd), uint32(markerEnd), point, point))
		}
		if lineEnd > markerEnd && uint32(lineEnd) > start && uint32(lineEnd) <= end && cobolLineHasContent(source, markerEnd, lineEnd) {
			point := advancePointByBytes(Point{}, source[:lineEnd])
			out = append(out, newLeafNodeInArena(arena, sym, named, uint32(lineEnd), uint32(lineEnd), point, point))
		}
		next := cobolNextLineStart(source, lineEnd)
		if next <= lineStart {
			break
		}
		lineStart = next
	}
	return out
}

func cobolCommentEntryMarkerEnd(lineStart, lineEnd int) int {
	if lineEnd <= lineStart {
		return lineEnd
	}
	fixedFormatEnd := lineStart + 71
	if lineEnd > fixedFormatEnd {
		return fixedFormatEnd
	}
	return lineEnd
}

func firstNonWhitespaceByteFrom(source []byte, start int) (uint32, bool) {
	if start < 0 {
		start = 0
	}
	for i := start; i < len(source); i++ {
		switch source[i] {
		case ' ', '\t', '\n', '\r':
			continue
		default:
			return uint32(i), true
		}
	}
	return 0, false
}

func normalizeCobolTopLevelDefinitionEnd(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	def := cobolFirstProgramDefinition(root, lang)
	if def == nil {
		return
	}
	if idDiv := cobolSoleIdentificationDivision(def, lang); idDiv != nil {
		end := cobolProgramIDStatementEnd(source, def.startByte, def.endByte)
		if end != 0 && end < def.endByte {
			setCobolNodeEnd(def, source, end)
			if end < idDiv.endByte {
				setCobolNodeEnd(idDiv, source, end)
			}
			return
		}
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= def.endByte {
		return
	}
	setCobolNodeEnd(def, source, end)
}

func cobolSoleIdentificationDivision(def *Node, lang *Language) *Node {
	var idDiv *Node
	for i := 0; i < resultChildCount(def); i++ {
		child := resultChildAt(def, i)
		if child == nil || child.IsExtra() {
			continue
		}
		if child.Type(lang) != "identification_division" {
			return nil
		}
		if idDiv != nil {
			return nil
		}
		idDiv = child
	}
	return idDiv
}

func cobolProgramIDStatementEnd(source []byte, start, end uint32) uint32 {
	if start >= end || int(end) > len(source) {
		return 0
	}
	idx := cobolIndexFoldASCII(source, int(start), int(end), "program-id.")
	if idx < 0 {
		return 0
	}
	for i := idx + len("program-id."); i < int(end); i++ {
		switch source[i] {
		case '.':
			return uint32(i + 1)
		case '\n', '\r':
			return 0
		}
	}
	return 0
}

func cobolIndexFoldASCII(source []byte, start, end int, needle string) int {
	if start < 0 {
		start = 0
	}
	if end > len(source) {
		end = len(source)
	}
	if len(needle) == 0 || start >= end || end-start < len(needle) {
		return -1
	}
	for i := start; i+len(needle) <= end; i++ {
		if cobolEqualFoldASCII(source[i:i+len(needle)], needle) {
			return i
		}
	}
	return -1
}

func cobolEqualFoldASCII(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := range b {
		if asciiLower(b[i]) != asciiLower(s[i]) {
			return false
		}
	}
	return true
}

func setCobolNodeStart(n *Node, source []byte, start uint32) {
	if n == nil || n.startByte == start || int(start) > len(source) {
		return
	}
	n.startByte = start
	n.startPoint = advancePointByBytes(Point{}, source[:start])
}

func setCobolNodeEnd(n *Node, source []byte, end uint32) {
	if n == nil || n.endByte == end || int(end) > len(source) {
		return
	}
	n.endByte = end
	n.endPoint = advancePointByBytes(Point{}, source[:end])
}

func normalizeCobolRootProgramDefinitionSiblingEnds(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	childCount := resultChildCount(root)
	for i := 0; i+1 < childCount; i++ {
		def := resultChildAt(root, i)
		if def == nil || def.IsExtra() || def.IsError() || def.IsMissing() || def.Type(lang) != "program_definition" {
			continue
		}
		next := cobolNextDefinitionRootSibling(root, i+1)
		if next == nil {
			continue
		}
		end := cobolNodeEndBeforeSibling(next, source, lang)
		if end == 0 || end <= def.startByte || end >= def.endByte {
			continue
		}
		setCobolNodeEnd(def, source, end)
		normalizeCobolLastDivisionEndToParent(def, source, lang)
	}
}

func cobolNextDefinitionRootSibling(root *Node, start int) *Node {
	for i := start; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsMissing() {
			return child
		}
	}
	return nil
}

func normalizeCobolLastDivisionEndToParent(parent *Node, source []byte, lang *Language) {
	if parent == nil {
		return
	}
	for i := resultChildCount(parent) - 1; i >= 0; i-- {
		child := resultChildAt(parent, i)
		if child == nil || child.IsExtra() || child.IsError() || child.IsMissing() {
			continue
		}
		if !strings.HasSuffix(child.Type(lang), "_division") {
			return
		}
		if parent.endByte > child.startByte && parent.endByte < child.endByte {
			setCobolNodeEnd(child, source, parent.endByte)
		}
		return
	}
}

func normalizeCobolDivisionSiblingEnds(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || resultChildCount(root) == 0 {
		return
	}
	def := cobolFirstProgramDefinition(root, lang)
	if def == nil {
		return
	}
	childCount := resultChildCount(def)
	for i := 0; i+1 < childCount; i++ {
		cur := resultChildAt(def, i)
		next := resultChildAt(def, i+1)
		if cur == nil || next == nil || cur.IsExtra() {
			continue
		}
		if next.IsExtra() && next.Type(lang) != "copy_statement" {
			continue
		}
		if !strings.HasSuffix(cur.Type(lang), "_division") {
			continue
		}
		end := cobolNodeEndBeforeSibling(next, source, lang)
		if end == 0 || end <= cur.startByte || end >= cur.endByte {
			continue
		}
		cur.endByte = end
		cur.endPoint = advancePointByBytes(Point{}, source[:end])
	}
}

func normalizeCobolSectionSiblingEnds(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || len(source) == 0 {
		return
	}
	walkResultTreePostorder(root, func(parent *Node) {
		childCount := resultChildCount(parent)
		for i := 0; i+1 < childCount; i++ {
			cur := resultChildAt(parent, i)
			next := resultChildAt(parent, i+1)
			if cur == nil || next == nil || cur.IsExtra() || cur.IsError() || cur.IsMissing() {
				continue
			}
			if !strings.HasSuffix(cur.Type(lang), "_section") {
				continue
			}
			end := cobolNodeEndBeforeSibling(next, source, lang)
			if end == 0 || end <= cur.startByte || end >= cur.endByte {
				continue
			}
			setCobolNodeEnd(cur, source, end)
		}
		if childCount == 0 {
			return
		}
		last := resultChildAt(parent, childCount-1)
		if last == nil || last.IsExtra() || last.IsError() || last.IsMissing() || !strings.HasSuffix(last.Type(lang), "_section") {
			return
		}
		if parent.endByte > last.startByte && parent.endByte < last.endByte {
			setCobolNodeEnd(last, source, parent.endByte)
		}
	})
}

func cobolNodeEndBeforeSibling(next *Node, source []byte, lang *Language) uint32 {
	if next == nil || int(next.startByte) > len(source) {
		return 0
	}
	if next != nil && next.Type(lang) == "comment" && int(next.startByte) <= len(source) {
		lineStart := cobolLineStart(source, int(next.startByte))
		if lineStart < int(next.startByte) && cobolLineLooksFixedFormatComment(source, lineStart) {
			return lastNonTriviaByteEnd(source[:lineStart])
		}
	}
	return lastNonTriviaByteEnd(source[:next.startByte])
}

func normalizeCobolTrailingTriviaSpans(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || len(source) == 0 {
		return
	}
	walkResultTreePostorder(root, func(n *Node) {
		if n == nil || n.Type(lang) == "start" || n.IsExtra() || n.IsMissing() || n.IsError() || n.HasError() {
			return
		}
		childCount := resultChildCount(n)
		if childCount == 0 {
			normalizeCobolTrailingTriviaLeafSpan(n, source, lang)
			return
		}
		if int(n.endByte) > len(source) {
			return
		}
		last := (*Node)(nil)
		for i := childCount - 1; i >= 0; i-- {
			child := resultChildAt(n, i)
			if child == nil || child.IsMissing() {
				continue
			}
			last = child
			break
		}
		if last == nil || last.endByte >= n.endByte || last.endByte < n.startByte {
			return
		}
		if !cobolBytesAreTrailingTrivia(source, last.endByte, n.endByte) {
			return
		}
		setCobolNodeEnd(n, source, last.endByte)
	})
}

func normalizeCobolTrailingTriviaLeafSpan(n *Node, source []byte, lang *Language) {
	if n == nil || int(n.endByte) > len(source) || !cobolTrailingTriviaLeafCanTrim(n.Type(lang)) {
		return
	}
	end := lastNonTriviaByteEnd(source[n.startByte:n.endByte])
	if end == 0 {
		return
	}
	absoluteEnd := n.startByte + end
	if absoluteEnd <= n.startByte || absoluteEnd >= n.endByte {
		return
	}
	setCobolNodeEnd(n, source, absoluteEnd)
}

func cobolTrailingTriviaLeafCanTrim(typ string) bool {
	return strings.HasSuffix(typ, "_statement") || strings.HasSuffix(typ, "_statement_loop")
}

func cobolBytesAreTrailingTrivia(source []byte, start, end uint32) bool {
	if start >= end || int(end) > len(source) {
		return false
	}
	lineStart := int(start)
	for lineStart > 0 && source[lineStart-1] != '\n' && source[lineStart-1] != '\r' {
		lineStart--
	}
	column := int(start) - lineStart
	for i := int(start); i < int(end); i++ {
		switch source[i] {
		case ' ', '\t', '\f':
			column++
		case '\n':
			lineStart = i + 1
			column = 0
		case '\r':
			lineStart = i + 1
			column = 0
		default:
			if cobolLineLooksFixedFormatComment(source, lineStart) {
				column++
				continue
			}
			if column < 6 && cobolLineLooksFixedFormat(source, lineStart) {
				column++
				continue
			}
			if column >= 72 && cobolLineLooksFixedFormat(source, lineStart) {
				column++
				continue
			}
			return false
		}
	}
	return true
}

func cobolLineLooksFixedFormat(source []byte, lineStart int) bool {
	indicator := lineStart + 6
	if lineStart < 0 || indicator >= len(source) {
		return false
	}
	switch source[indicator] {
	case ' ', '*', '-', '/':
		return true
	default:
		return false
	}
}

func cobolLineLooksFixedFormatComment(source []byte, lineStart int) bool {
	indicator := lineStart + 6
	return lineStart >= 0 && indicator < len(source) && cobolFixedFormatCommentIndicator(source[indicator])
}

func normalizeCobolRecoveredParagraphHeader(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || !lang.GeneratedByGrammargen || root.Type(lang) != "start" || !root.hasError() {
		return
	}
	rootChildren := resultChildSliceForMutation(root)
	if len(rootChildren) < 2 {
		return
	}
	errNode := rootChildren[len(rootChildren)-1]
	if errNode == nil || errNode.symbol != errorSymbol || !errNode.hasError() || errNode.startByte >= errNode.endByte || int(errNode.endByte) > len(source) {
		return
	}
	dot := cobolTrailingParagraphErrorDot(errNode, lang)
	if dot == nil || dot.endByte != errNode.endByte || dot.startByte <= errNode.startByte || int(dot.endByte) > len(source) {
		return
	}
	if !cobolBytesAreParagraphLabel(source[errNode.startByte:dot.startByte]) {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	procedure := cobolFirstChildOfType(program, lang, "procedure_division")
	if procedure == nil {
		return
	}
	paragraphSym, ok := symbolByName(lang, "paragraph_header")
	if !ok {
		return
	}

	rootStart, rootStartPoint := root.startByte, root.startPoint
	rootEnd, rootEndPoint := root.endByte, root.endPoint
	procStart, procStartPoint := procedure.startByte, procedure.startPoint

	cobolClearErrorFlags(dot)
	header := newParentNodeInArena(root.ownerArena, paragraphSym, symbolIsNamed(lang, paragraphSym), []*Node{dot}, nil, 0)
	header.startByte = errNode.startByte
	header.startPoint = errNode.startPoint
	header.endByte = errNode.endByte
	header.endPoint = errNode.endPoint
	header.setHasError(false)

	procChildren := append(resultChildSliceForMutation(procedure), header)
	replaceNodeChildrenUnfielded(procedure, cloneNodeSliceInArena(procedure.ownerArena, procChildren))
	procedure.startByte = procStart
	procedure.startPoint = procStartPoint
	procedure.endByte = rootEnd
	procedure.endPoint = rootEndPoint
	cobolRefreshHasErrorFromChildren(procedure)

	keptRootChildren := cloneNodeSliceInArena(root.ownerArena, rootChildren[:len(rootChildren)-1])
	replaceNodeChildrenUnfielded(root, keptRootChildren)
	root.startByte = rootStart
	root.startPoint = rootStartPoint
	root.endByte = rootEnd
	root.endPoint = rootEndPoint
	cobolRefreshHasErrorFromChildren(root)
}

func normalizeCobolProcedureTrailingParagraphCommentEntry(root *Node, source []byte, lang *Language) {
	if root == nil || !isCobolLanguage(lang) || root.Type(lang) != "start" || len(source) == 0 {
		return
	}
	paragraphSym, ok := symbolByName(lang, "paragraph_header")
	if !ok {
		return
	}
	dotSym, ok := symbolByName(lang, ".")
	if !ok {
		return
	}
	program := cobolFirstProgramDefinition(root, lang)
	if program == nil {
		return
	}
	changed := false
	walkResultTreePostorder(program, func(n *Node) {
		if n == nil || n.Type(lang) != "procedure_division" || resultChildCount(n) == 0 {
			return
		}
		last := resultChildAt(n, resultChildCount(n)-1)
		if last == nil || last.Type(lang) != "comment_entry" || last.startByte != last.endByte || int(last.startByte) > len(source) {
			return
		}
		lineStart := cobolLineStart(source, int(last.startByte))
		labelStart, ok := firstNonWhitespaceByteInRange(source, lineStart, int(last.startByte))
		if !ok {
			return
		}
		labelEnd := lastNonTriviaByteEnd(source[:last.startByte])
		if labelEnd == 0 || labelEnd <= labelStart || source[labelEnd-1] != '.' {
			return
		}
		dotStart := labelEnd - 1
		if !cobolBytesAreParagraphLabel(source[labelStart:dotStart]) {
			return
		}
		dot := newLeafNodeInArena(n.ownerArena, dotSym, symbolIsNamed(lang, dotSym), dotStart, labelEnd, advancePointByBytes(Point{}, source[:dotStart]), advancePointByBytes(Point{}, source[:labelEnd]))
		header := newParentNodeInArena(n.ownerArena, paragraphSym, symbolIsNamed(lang, paragraphSym), []*Node{dot}, nil, 0)
		header.startByte = labelStart
		header.startPoint = advancePointByBytes(Point{}, source[:labelStart])
		header.endByte = labelEnd
		header.endPoint = advancePointByBytes(Point{}, source[:labelEnd])
		children := resultChildSliceForMutation(n)
		children[len(children)-1] = header
		procStart, procStartPoint := n.startByte, n.startPoint
		replaceNodeChildrenUnfielded(n, cloneNodeSliceInArena(n.ownerArena, children))
		n.startByte = procStart
		n.startPoint = procStartPoint
		setCobolNodeEnd(n, source, labelEnd)
		changed = true
	})
	if changed {
		cobolRefreshHasErrorFromChildren(program)
		cobolRefreshHasErrorFromChildren(root)
	}
}

func cobolFirstProgramDefinition(root *Node, lang *Language) *Node {
	_, child := cobolFirstProgramDefinitionIndex(root, lang)
	return child
}

func cobolFirstProgramDefinitionIndex(root *Node, lang *Language) (int, *Node) {
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			return i, child
		}
	}
	return -1, nil
}

func cobolFirstChildOfType(parent *Node, lang *Language, typ string) *Node {
	for i := 0; i < resultChildCount(parent); i++ {
		child := resultChildAt(parent, i)
		if child != nil && !child.IsExtra() && child.Type(lang) == typ {
			return child
		}
	}
	return nil
}

func cobolTrailingParagraphErrorDot(errNode *Node, lang *Language) *Node {
	for i := resultChildCount(errNode) - 1; i >= 0; i-- {
		child := resultChildAt(errNode, i)
		if child != nil && child.Type(lang) == "." {
			return child
		}
	}
	return nil
}

func cobolBytesAreParagraphLabel(b []byte) bool {
	seen := false
	for _, c := range b {
		switch {
		case c == ' ' || c == '\t':
			if seen {
				return false
			}
		case c >= 'a' && c <= 'z':
			seen = true
		case c >= 'A' && c <= 'Z':
			seen = true
		case c >= '0' && c <= '9':
			seen = true
		case c == '-' || c == '_':
			seen = true
		default:
			return false
		}
	}
	return seen
}

func cobolClearErrorFlags(n *Node) {
	if n == nil {
		return
	}
	n.setHasError(false)
	for i := 0; i < resultChildCount(n); i++ {
		cobolClearErrorFlags(resultChildAt(n, i))
	}
}

func cobolRefreshHasErrorFromChildren(n *Node) {
	if n == nil {
		return
	}
	n.setHasError(false)
	for i := 0; i < resultChildCount(n); i++ {
		child := resultChildAt(n, i)
		if child != nil && (child.IsError() || child.HasError()) {
			n.setHasError(true)
			return
		}
	}
}
