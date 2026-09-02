package gotreesitter

import "time"

func (p *Parser) tryTokenInvariantLeafEdit(source []byte, oldTree *Tree, ts TokenSource, timing *incrementalParseTiming) (*Tree, bool) {
	if p == nil || oldTree == nil || oldTree.RootNode() == nil || oldTree.language != p.language {
		return nil, false
	}
	if compactRecoverEOFTreeMarked(oldTree) {
		return nil, false
	}
	if len(oldTree.edits) != 1 {
		return nil, false
	}
	edit := oldTree.edits[0]
	if edit.NewEndByte-edit.StartByte != edit.OldEndByte-edit.StartByte {
		return nil, false
	}
	if edit.NewEndPoint != edit.OldEndPoint || edit.OldEndByte <= edit.StartByte {
		return nil, false
	}
	if len(source) != len(oldTree.source) {
		return nil, false
	}
	root := oldTree.RootNode()
	node := oldTree.lastEditedLeaf
	if node == nil || !node.containsByteRange(edit.StartByte, edit.OldEndByte) {
		node = root.DescendantForByteRange(edit.StartByte, edit.OldEndByte)
	}
	start := time.Time{}
	if timing != nil {
		start = time.Now()
	}
	if p.canReuseLanguageTextInvariantNode(source, oldTree, node, edit) {
		tree := reuseTreeWithNewSource(oldTree, source, node, true)
		if tree == nil || tree.root == nil {
			return nil, false
		}
		tree.setParseRuntime(ParseRuntime{
			StopReason:       ParseStopAccepted,
			SourceLen:        uint32(len(source)),
			TokensConsumed:   0,
			LastTokenEndByte: node.endByte,
			LastTokenSymbol:  node.symbol,
			ExpectedEOFByte:  uint32(len(source)),
			RootEndByte:      tree.root.EndByte(),
			MaxStacksSeen:    1,
		})
		if timing != nil {
			timing.reuseNanos += time.Since(start).Nanoseconds()
			timing.reusedSubtrees++
			timing.reusedBytes += uint64(len(source))
			timing.maxStacksSeen = 1
			timing.stopReason = ParseStopAccepted
			timing.tokensConsumed = 0
			timing.lastTokenEndByte = node.endByte
			timing.expectedEOFByte = uint32(len(source))
			timing.singleStackIterations = 1
			timing.singleStackTokens = 0
		}
		return tree, true
	}
	leaf := node
	if leaf == nil || leaf.ChildCount() != 0 || leaf.hasError() || leaf.isMissing() {
		return nil, false
	}
	requireScannerCheckpoint := false
	if oldTree.compactMaterialized && oldTree.incrementalReuseDisabled {
		// Keep the scanner-proof closure local to the primitive as well as at
		// admission. Direct callers must not bypass the compact-tree gate and
		// reauthenticate a leaf with a newly-created stateful scanner payload.
		if leaf.preGotoState == 0 || !compactLeafScannerReauthenticationProven(p.language, leaf) {
			return nil, false
		}
		requireScannerCheckpoint = compactLeafScannerCheckpointRequired(p.language)
	}
	var tok Token
	var ok bool
	if requireScannerCheckpoint {
		// This is intentionally a terminal path: a compact tree whose scanner
		// proof depends on a checkpoint must authenticate that checkpoint during
		// the scan. Failure may not fall through to a fresh scanner payload or a
		// generic token-source skip.
		tok, ok = scanCompactLeafWithRequiredScannerCheckpoint(ts, leaf)
	} else {
		tok, ok = p.scanTokenInvariantEditedLeaf(source, ts, leaf)
	}
	if !ok || tok.Symbol != leaf.symbol || tok.StartByte != leaf.startByte || tok.EndByte != leaf.endByte {
		return nil, false
	}
	tree := reuseTreeWithNewSource(oldTree, source, leaf, false)
	if tree == nil || tree.root == nil {
		return nil, false
	}
	tree.setParseRuntime(ParseRuntime{
		StopReason:       ParseStopAccepted,
		SourceLen:        uint32(len(source)),
		TokensConsumed:   1,
		LastTokenEndByte: tok.EndByte,
		LastTokenSymbol:  tok.Symbol,
		ExpectedEOFByte:  uint32(len(source)),
		RootEndByte:      tree.root.EndByte(),
		MaxStacksSeen:    1,
	})
	if timing != nil {
		timing.reuseNanos += time.Since(start).Nanoseconds()
		timing.reusedSubtrees++
		timing.reusedBytes += uint64(len(source))
		timing.maxStacksSeen = 1
		timing.stopReason = ParseStopAccepted
		timing.tokensConsumed = 1
		timing.lastTokenEndByte = tok.EndByte
		timing.expectedEOFByte = uint32(len(source))
		timing.singleStackIterations = 1
		timing.singleStackTokens = 1
	}
	return tree, true
}

func (p *Parser) canReuseLanguageTextInvariantNode(source []byte, oldTree *Tree, node *Node, edit InputEdit) bool {
	if p == nil || p.language == nil || oldTree == nil || node == nil {
		return false
	}
	if !sameLengthEditWithinNode(source, oldTree.source, node, edit) {
		return false
	}
	switch p.language.Name {
	case "awk":
		return oldTree.forestFastPath && node.Type(p.language) == "number" &&
			asciiDigitTextInvariantEdit(source, oldTree.source, edit)
	case "bash":
		return oldTree.forestFastPath && bashTextInvariantNodeEdit(source, oldTree.source, node, edit, p.language)
	case "clojure":
		return clojureTextInvariantNodeEdit(source, oldTree.source, node, edit, p.language)
	case "cmake":
		return oldTree.forestFastPath && node.Type(p.language) == "unquoted_argument" &&
			cmakeTextInvariantEdit(source, oldTree.source, edit)
	case "css", "scss":
		return oldTree.forestFastPath && node.Type(p.language) == "integer_value" &&
			cssTextInvariantIntegerValueEdit(source, oldTree.source, edit)
	case "c_sharp":
		return oldTree.forestFastPath && node.Type(p.language) == "identifier" &&
			csharpTokenInvariantIdentifierText(oldTree.source, node) &&
			csharpTokenInvariantIdentifierText(source, node)
	case "elixir":
		return elixirTextInvariantNodeEdit(source, oldTree.source, node, edit, p.language)
	case "hcl":
		return hclTextInvariantNodeEdit(source, oldTree.source, node, edit, p.language)
	case "julia":
		return node.Type(p.language) == "line_comment" && hashLineCommentTextInvariantEdit(source, oldTree.source, node, edit)
	case "powershell":
		return powershellTextInvariantNodeEdit(source, oldTree.source, node, edit, p.language)
	case "rust":
		return node.Type(p.language) == "line_comment" && rustLineCommentTextInvariantEdit(source, oldTree.source, node, edit)
	case "sql":
		return sqlTextInvariantNodeEdit(source, oldTree.source, node, edit, p.language)
	case "yaml":
		return yamlTextInvariantScalarEdit(source, oldTree.source, node, edit, node.Type(p.language))
	default:
		return false
	}
}

func sameLengthEditWithinNode(source, oldSource []byte, node *Node, edit InputEdit) bool {
	return node != nil &&
		edit.NewEndByte-edit.StartByte == edit.OldEndByte-edit.StartByte &&
		edit.NewEndPoint == edit.OldEndPoint &&
		edit.StartByte >= node.startByte &&
		edit.OldEndByte <= node.endByte &&
		edit.NewEndByte <= uint32(len(source)) &&
		edit.OldEndByte <= uint32(len(oldSource))
}

func cmakeTextInvariantEdit(source, oldSource []byte, edit InputEdit) bool {
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if !cmakeTextInvariantUnquotedByte(oldSource[i]) || !cmakeTextInvariantUnquotedByte(source[i]) {
			return false
		}
	}
	return true
}

func rustLineCommentTextInvariantEdit(source, oldSource []byte, node *Node, edit InputEdit) bool {
	start := int(node.startByte)
	if start+2 > len(oldSource) || oldSource[start] != '/' || oldSource[start+1] != '/' {
		return false
	}
	textStart := start + 2
	if textStart < len(oldSource) && (oldSource[textStart] == '/' || oldSource[textStart] == '!') {
		textStart++
	}
	if edit.StartByte < uint32(textStart) {
		return false
	}
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if rustLineCommentBreakByte(oldSource[i]) || rustLineCommentBreakByte(source[i]) {
			return false
		}
	}
	return true
}

func rustLineCommentBreakByte(b byte) bool {
	return b == '\n' || b == '\r'
}

func hashLineCommentTextInvariantEdit(source, oldSource []byte, node *Node, edit InputEdit) bool {
	if node == nil || edit.StartByte <= node.startByte {
		return false
	}
	start := int(node.startByte)
	if start >= len(source) || start >= len(oldSource) || source[start] != '#' || oldSource[start] != '#' {
		return false
	}
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if rustLineCommentBreakByte(oldSource[i]) || rustLineCommentBreakByte(source[i]) {
			return false
		}
	}
	return true
}

func cmakeTextInvariantUnquotedByte(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		b == '_'
}

func cssTextInvariantIntegerValueEdit(source, oldSource []byte, edit InputEdit) bool {
	return asciiDigitTextInvariantEdit(source, oldSource, edit)
}

func asciiDigitTextInvariantEdit(source, oldSource []byte, edit InputEdit) bool {
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if !asciiDigit(oldSource[i]) || !asciiDigit(source[i]) {
			return false
		}
	}
	return true
}

func asciiDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

func bashTextInvariantNodeEdit(source, oldSource []byte, node *Node, edit InputEdit, lang *Language) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) {
		return false
	}
	switch node.Type(lang) {
	case "comment":
		return hashLineCommentTextInvariantEdit(source, oldSource, node, edit)
	case "number":
		return asciiDigitTextInvariantEdit(source, oldSource, edit) &&
			bashStableNumberText(oldSource, node) &&
			bashStableNumberText(source, node)
	default:
		return false
	}
}

func bashStableNumberText(source []byte, node *Node) bool {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return false
	}
	text := source[node.startByte:node.endByte]
	i := 0
	if text[i] == '-' || text[i] == '+' {
		i++
		if i == len(text) {
			return false
		}
	}
	for ; i < len(text); i++ {
		if !asciiDigit(text[i]) {
			return false
		}
	}
	return true
}

func hclTextInvariantNodeEdit(source, oldSource []byte, node *Node, edit InputEdit, lang *Language) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) {
		return false
	}
	switch node.Type(lang) {
	case "numeric_lit", "template_literal":
		for i := edit.StartByte; i < edit.OldEndByte; i++ {
			if !asciiDigit(oldSource[i]) || !asciiDigit(source[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func powershellTextInvariantNodeEdit(source, oldSource []byte, node *Node, edit InputEdit, lang *Language) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) {
		return false
	}
	switch node.Type(lang) {
	case "comment":
		return powershellCommentTextInvariantEdit(source, oldSource, node, edit)
	case "expandable_string_literal", "expandable_here_string_literal", "verbatim_string_characters":
		return powershellStringTextInvariantEdit(source, oldSource, node, edit)
	default:
		return false
	}
}

func powershellCommentTextInvariantEdit(source, oldSource []byte, node *Node, edit InputEdit) bool {
	if edit.StartByte <= node.startByte {
		return false
	}
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if !powershellCommentStableEditByte(oldSource[i]) || !powershellCommentStableEditByte(source[i]) {
			return false
		}
	}
	return true
}

func powershellCommentStableEditByte(b byte) bool {
	switch b {
	case '\n', '\r', '#', '<', '>':
		return false
	default:
		return true
	}
}

func powershellStringTextInvariantEdit(source, oldSource []byte, node *Node, edit InputEdit) bool {
	if powershellEditOverlapsChild(node, edit) {
		return false
	}
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if !powershellStringStableEditByte(oldSource[i]) || !powershellStringStableEditByte(source[i]) {
			return false
		}
	}
	return true
}

func powershellEditOverlapsChild(node *Node, edit InputEdit) bool {
	if node == nil {
		return true
	}
	childCount := nodeChildCountNoMaterialize(node)
	for i := 0; i < childCount; i++ {
		entry, ok := nodeChildEntryAtNoMaterialize(node, i)
		if !ok {
			return true
		}
		if rangesOverlapHalfOpen(stackEntryNodeStartByte(entry), stackEntryNodeEndByte(entry), edit.StartByte, edit.OldEndByte) {
			return true
		}
	}
	return false
}

func rangesOverlapHalfOpen(aStart, aEnd, bStart, bEnd uint32) bool {
	return aStart < bEnd && bStart < aEnd
}

func powershellStringStableEditByte(b byte) bool {
	switch b {
	case '\n', '\r', '"', '\'', '`', '$', '@':
		return false
	default:
		return true
	}
}

type yamlScalarKind uint8

const (
	yamlScalarString yamlScalarKind = iota
	yamlScalarInteger
	yamlScalarFloat
	yamlScalarBoolean
	yamlScalarNull
)

func yamlTextInvariantScalarEdit(source, oldSource []byte, node *Node, edit InputEdit, nodeType string) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) {
		return false
	}
	if !yamlSameLengthEditBytesStayInPlainScalar(source, oldSource, edit) {
		return false
	}
	oldText := oldSource[node.startByte:node.endByte]
	newText := source[node.startByte:node.endByte]
	oldKind := yamlPlainScalarKind(oldText)
	newKind := yamlPlainScalarKind(newText)
	switch nodeType {
	case "string_scalar":
		return oldKind == yamlScalarString && newKind == yamlScalarString
	case "integer_scalar":
		return oldKind == yamlScalarInteger && newKind == yamlScalarInteger
	case "float_scalar":
		return oldKind == yamlScalarFloat && newKind == yamlScalarFloat
	default:
		return false
	}
}

func yamlSameLengthEditBytesStayInPlainScalar(source, oldSource []byte, edit InputEdit) bool {
	for i := edit.StartByte; i < edit.OldEndByte; i++ {
		if int(i) >= len(oldSource) || int(i) >= len(source) {
			return false
		}
		if !yamlPlainScalarStableEditByte(oldSource[i]) || !yamlPlainScalarStableEditByte(source[i]) {
			return false
		}
	}
	return true
}

func yamlPlainScalarStableEditByte(b byte) bool {
	return (b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		b == '_' ||
		b == '-' ||
		b == '.' ||
		b == '/' ||
		b == '@'
}

func yamlPlainScalarKind(text []byte) yamlScalarKind {
	if len(text) == 0 {
		return yamlScalarNull
	}
	if yamlPlainScalarIsNull(text) {
		return yamlScalarNull
	}
	if yamlPlainScalarIsBoolean(text) {
		return yamlScalarBoolean
	}
	if yamlPlainScalarIsInteger(text) {
		return yamlScalarInteger
	}
	if yamlPlainScalarIsFloat(text) {
		return yamlScalarFloat
	}
	return yamlScalarString
}

func yamlPlainScalarIsNull(text []byte) bool {
	return bytesEqualFoldASCIIString(text, "null") || (len(text) == 1 && text[0] == '~')
}

func yamlPlainScalarIsBoolean(text []byte) bool {
	return bytesEqualFoldASCIIString(text, "true") ||
		bytesEqualFoldASCIIString(text, "false")
}

func yamlPlainScalarIsInteger(text []byte) bool {
	if len(text) == 0 {
		return false
	}
	i := 0
	if text[i] == '+' || text[i] == '-' {
		i++
		if i == len(text) {
			return false
		}
	}
	if i+2 < len(text) && text[i] == '0' && (text[i+1] == 'x' || text[i+1] == 'X') {
		return yamlAllBytes(text[i+2:], yamlHexDigitOrUnderscore)
	}
	if i+2 < len(text) && text[i] == '0' && (text[i+1] == 'o' || text[i+1] == 'O') {
		return yamlAllBytes(text[i+2:], yamlOctalDigitOrUnderscore)
	}
	return yamlAllBytes(text[i:], yamlDecimalDigitOrUnderscore)
}

func yamlPlainScalarIsFloat(text []byte) bool {
	if len(text) == 0 {
		return false
	}
	i := 0
	if text[i] == '+' || text[i] == '-' {
		i++
		if i == len(text) {
			return false
		}
	}
	if len(text)-i == 4 && text[i] == '.' && bytesEqualFoldASCIIString(text[i+1:], "inf") {
		return true
	}
	if len(text)-i == 4 && text[i] == '.' && bytesEqualFoldASCIIString(text[i+1:], "nan") {
		return true
	}
	hasDot := false
	hasExp := false
	digits := 0
	expDigits := 0
	inExp := false
	for ; i < len(text); i++ {
		b := text[i]
		switch {
		case b >= '0' && b <= '9':
			if inExp {
				expDigits++
			} else {
				digits++
			}
		case b == '_':
			continue
		case b == '.' && !hasDot && !inExp:
			hasDot = true
		case (b == 'e' || b == 'E') && !hasExp && digits > 0:
			hasExp = true
			inExp = true
			if i+1 < len(text) && (text[i+1] == '+' || text[i+1] == '-') {
				i++
			}
		default:
			return false
		}
	}
	if hasExp {
		return expDigits > 0
	}
	return hasDot && digits > 0
}

func yamlAllBytes(text []byte, pred func(byte) bool) bool {
	if len(text) == 0 {
		return false
	}
	seenDigit := false
	for _, b := range text {
		if !pred(b) {
			return false
		}
		if b != '_' {
			seenDigit = true
		}
	}
	return seenDigit
}

func yamlDecimalDigitOrUnderscore(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9')
}

func yamlOctalDigitOrUnderscore(b byte) bool {
	return b == '_' || (b >= '0' && b <= '7')
}

func yamlHexDigitOrUnderscore(b byte) bool {
	return b == '_' ||
		(b >= '0' && b <= '9') ||
		(b >= 'a' && b <= 'f') ||
		(b >= 'A' && b <= 'F')
}

func bytesEqualFoldASCIIString(a []byte, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if asciiLower(a[i]) != asciiLower(b[i]) {
			return false
		}
	}
	return true
}

func clojureTextInvariantNodeEdit(source, oldSource []byte, node *Node, edit InputEdit, lang *Language) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) {
		return false
	}
	switch node.Type(lang) {
	case "sym_name":
		start := int(node.startByte)
		end := int(node.endByte)
		return start >= 0 && end <= len(source) && clojureStableSymbolName(source[start:end])
	case "str_lit":
		return clojureStableStringLiteralEdit(source, node, edit)
	default:
		return false
	}
}

func elixirTextInvariantNodeEdit(source, oldSource []byte, node *Node, edit InputEdit, lang *Language) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) {
		return false
	}
	switch node.Type(lang) {
	case "identifier":
		return elixirStableIdentifierText(oldSource, node) && elixirStableIdentifierText(source, node)
	default:
		return false
	}
}

func elixirStableIdentifierText(source []byte, node *Node) bool {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return false
	}
	text := source[node.startByte:node.endByte]
	if len(text) == 0 || !elixirIdentifierStartByte(text[0]) {
		return false
	}
	end := len(text)
	if text[end-1] == '!' || text[end-1] == '?' {
		end--
		if end == 0 {
			return false
		}
	}
	for _, b := range text[1:end] {
		if !elixirIdentifierContinueByte(b) {
			return false
		}
	}
	return !elixirTokenInvariantIdentifierKeyword(string(text))
}

func elixirIdentifierStartByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || b == '_'
}

func elixirIdentifierContinueByte(b byte) bool {
	return elixirIdentifierStartByte(b) ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9')
}

func elixirTokenInvariantIdentifierKeyword(text string) bool {
	switch text {
	case "after", "and", "catch", "do", "else", "end", "false", "fn", "in", "nil", "not", "or", "rescue", "true", "when":
		return true
	default:
		return false
	}
}

func sqlTextInvariantNodeEdit(source, oldSource []byte, node *Node, edit InputEdit, lang *Language) bool {
	if !sameLengthEditWithinNode(source, oldSource, node, edit) || node.Type(lang) != "identifier" {
		return false
	}
	return sqlStableIdentifierText(oldSource, node) && sqlStableIdentifierText(source, node)
}

func sqlStableIdentifierText(source []byte, node *Node) bool {
	if node == nil || node.startByte >= node.endByte || int(node.endByte) > len(source) {
		return false
	}
	text := source[node.startByte:node.endByte]
	if len(text) == 0 || !sqlIdentifierStartByte(text[0]) {
		return false
	}
	for _, b := range text[1:] {
		if !sqlIdentifierContinueByte(b) {
			return false
		}
	}
	return !sqlTokenInvariantIdentifierKeyword(text)
}

func sqlIdentifierStartByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		b == '_'
}

func sqlIdentifierContinueByte(b byte) bool {
	return sqlIdentifierStartByte(b) || (b >= '0' && b <= '9')
}

func sqlTokenInvariantIdentifierKeyword(text []byte) bool {
	switch {
	case bytesEqualFoldASCIIString(text, "all"),
		bytesEqualFoldASCIIString(text, "alter"),
		bytesEqualFoldASCIIString(text, "and"),
		bytesEqualFoldASCIIString(text, "as"),
		bytesEqualFoldASCIIString(text, "between"),
		bytesEqualFoldASCIIString(text, "by"),
		bytesEqualFoldASCIIString(text, "case"),
		bytesEqualFoldASCIIString(text, "cast"),
		bytesEqualFoldASCIIString(text, "create"),
		bytesEqualFoldASCIIString(text, "delete"),
		bytesEqualFoldASCIIString(text, "distinct"),
		bytesEqualFoldASCIIString(text, "drop"),
		bytesEqualFoldASCIIString(text, "else"),
		bytesEqualFoldASCIIString(text, "end"),
		bytesEqualFoldASCIIString(text, "false"),
		bytesEqualFoldASCIIString(text, "from"),
		bytesEqualFoldASCIIString(text, "group"),
		bytesEqualFoldASCIIString(text, "having"),
		bytesEqualFoldASCIIString(text, "in"),
		bytesEqualFoldASCIIString(text, "insert"),
		bytesEqualFoldASCIIString(text, "is"),
		bytesEqualFoldASCIIString(text, "join"),
		bytesEqualFoldASCIIString(text, "like"),
		bytesEqualFoldASCIIString(text, "limit"),
		bytesEqualFoldASCIIString(text, "not"),
		bytesEqualFoldASCIIString(text, "null"),
		bytesEqualFoldASCIIString(text, "offset"),
		bytesEqualFoldASCIIString(text, "on"),
		bytesEqualFoldASCIIString(text, "or"),
		bytesEqualFoldASCIIString(text, "order"),
		bytesEqualFoldASCIIString(text, "select"),
		bytesEqualFoldASCIIString(text, "set"),
		bytesEqualFoldASCIIString(text, "table"),
		bytesEqualFoldASCIIString(text, "then"),
		bytesEqualFoldASCIIString(text, "true"),
		bytesEqualFoldASCIIString(text, "union"),
		bytesEqualFoldASCIIString(text, "update"),
		bytesEqualFoldASCIIString(text, "values"),
		bytesEqualFoldASCIIString(text, "when"),
		bytesEqualFoldASCIIString(text, "where"):
		return true
	default:
		return false
	}
}

func clojureStableStringLiteralEdit(source []byte, node *Node, edit InputEdit) bool {
	if node == nil || node.endByte <= node.startByte+1 {
		return false
	}
	start := node.startByte
	end := node.endByte
	if edit.StartByte <= start || edit.OldEndByte >= end || edit.NewEndByte >= end {
		return false
	}
	if int(end) > len(source) || source[start] != '"' || source[end-1] != '"' {
		return false
	}
	for i := edit.StartByte; i < edit.NewEndByte; i++ {
		if int(i) >= len(source) || clojureStringDelimiterByte(source[i]) {
			return false
		}
	}
	return true
}

func clojureStringDelimiterByte(b byte) bool {
	return b == '"' || b == '\\' || b == '\n' || b == '\r'
}

func clojureStableSymbolName(text []byte) bool {
	if len(text) == 0 || !clojureSymbolStartByte(text[0]) {
		return false
	}
	for _, b := range text[1:] {
		if !clojureSymbolContinueByte(b) {
			return false
		}
	}
	switch string(text) {
	case "nil", "true", "false":
		return false
	default:
		return true
	}
}

func clojureSymbolStartByte(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		b == '_' ||
		b == '*' ||
		b == '+' ||
		b == '-' ||
		b == '!' ||
		b == '?' ||
		b == '<' ||
		b == '>' ||
		b == '=' ||
		b == '$' ||
		b == '%' ||
		b == '&'
}

func clojureSymbolContinueByte(b byte) bool {
	return clojureSymbolStartByte(b) ||
		(b >= '0' && b <= '9') ||
		b == '.' ||
		b == '/' ||
		b == '\''
}

func asciiLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}

func (p *Parser) tokenInvariantLeafEditCandidate(source []byte, oldTree *Tree) (*Node, InputEdit, bool) {
	if p == nil || oldTree == nil || oldTree.RootNode() == nil || oldTree.language != p.language {
		return nil, InputEdit{}, false
	}
	if compactRecoverEOFTreeMarked(oldTree) {
		return nil, InputEdit{}, false
	}
	if len(oldTree.edits) != 1 {
		return nil, InputEdit{}, false
	}
	edit := oldTree.edits[0]
	if !inputEditPreservesTokenExtent(edit) {
		return nil, InputEdit{}, false
	}
	if len(source) != len(oldTree.source) {
		return nil, InputEdit{}, false
	}
	return oldTree.RootNode(), edit, true
}

func inputEditPreservesTokenExtent(edit InputEdit) bool {
	if edit.NewEndByte-edit.StartByte != edit.OldEndByte-edit.StartByte {
		return false
	}
	return edit.NewEndPoint == edit.OldEndPoint && edit.OldEndByte > edit.StartByte
}

func tokenInvariantLeafReusable(leaf *Node) bool {
	return leaf != nil && leaf.ChildCount() == 0 && !leaf.hasError() && !leaf.isMissing()
}

func (p *Parser) scanTokenInvariantEditedLeaf(source []byte, ts TokenSource, leaf *Node) (Token, bool) {
	tok, ok := scanLeafTokenWithoutMutatingSource(ts, leaf)
	if ok {
		return tok, true
	}
	if p != nil && languageUsesExternalScannerCheckpoints(p.language) {
		// The checkpoint scan authenticates both scanner boundary states. A
		// fresh scanner can reproduce the token symbol and span while ending in
		// different state. Do not replace a failed checkpoint proof with it.
		return Token{}, false
	}
	if tok, ok = p.scanLeafTokenWithFreshSource(source, leaf, tokenSourceHasDFABase(ts)); ok {
		return tok, true
	}

	restoreState, ok := snapshotTokenSourceState(ts)
	if !ok {
		return Token{}, false
	}
	defer restoreState()

	stateful, ok := ts.(parserStateTokenSource)
	if !ok {
		return Token{}, false
	}
	stateful.SetParserState(leaf.preGotoState)
	stateful.SetGLRStates(nil)
	return skipTokenSourceToLeaf(ts, leaf)
}

// scanCompactLeafWithRequiredScannerCheckpoint performs the only permitted
// scan for a compact leaf whose scanner proof depends on a recorded checkpoint.
// It calls scanDFALeafTokenWithExternalCheckpoint directly; that routine
// restores the start snapshot and verifies both the token identity/span and end
// snapshot. Any unsupported token source or failed authentication returns
// false; callers must not retry through a fresh scanner or generic skip path.
func scanCompactLeafWithRequiredScannerCheckpoint(ts TokenSource, leaf *Node) (Token, bool) {
	dts, ok := ts.(*dfaTokenSource)
	if !ok || dts == nil || !languageUsesExternalScannerCheckpoints(dts.language) {
		return Token{}, false
	}
	return scanDFALeafTokenWithExternalCheckpoint(dts, leaf)
}

func (p *Parser) scanLeafTokenWithFreshSource(source []byte, leaf *Node, allowDFA bool) (Token, bool) {
	if p == nil || leaf == nil {
		return Token{}, false
	}
	var fresh TokenSource
	if p.reparseFactory != nil {
		var err error
		fresh, err = p.reparseFactory(source)
		if err != nil || fresh == nil {
			return Token{}, false
		}
	} else {
		if !allowDFA {
			return Token{}, false
		}
		// Pooled + lexer-reusing: this scan runs on the clean incremental
		// fast path (token-invariant single-leaf edits), so it must not
		// allocate a fresh token source + lexer per parse. The source's
		// lifetime is strictly local (released via manageTokenSourceLifetime
		// below, which returns it to the pool).
		dfaFresh := p.acquireParserDFATokenSource(source)
		if dfaFresh == nil {
			return Token{}, false
		}
		fresh = dfaFresh
	}
	release := manageTokenSourceLifetime(fresh)
	defer release()

	ts := p.wrapIncludedRanges(fresh)
	if stateful, ok := ts.(parserStateTokenSource); ok {
		stateful.SetParserState(leaf.preGotoState)
		stateful.SetGLRStates(nil)
	}
	return skipTokenSourceToLeaf(ts, leaf)
}

func tokenSourceHasDFABase(ts TokenSource) bool {
	switch typed := ts.(type) {
	case *dfaTokenSource:
		return typed != nil
	case *includedRangeTokenSource:
		if typed == nil {
			return false
		}
		_, ok := typed.base.(*dfaTokenSource)
		return ok
	default:
		return false
	}
}

func skipTokenSourceToLeaf(ts TokenSource, leaf *Node) (Token, bool) {
	if skipper, ok := ts.(PointSkippableTokenSource); ok {
		return skipper.SkipToByteWithPoint(leaf.startByte, leaf.startPoint), true
	}
	if skipper, ok := ts.(ByteSkippableTokenSource); ok {
		return skipper.SkipToByte(leaf.startByte), true
	}
	return Token{}, false
}

func scanLeafTokenWithoutMutatingSource(ts TokenSource, leaf *Node) (Token, bool) {
	if leaf == nil {
		return Token{}, false
	}
	switch typed := ts.(type) {
	case *dfaTokenSource:
		return scanDFALeafTokenWithoutMutatingSource(typed, leaf)
	case *includedRangeTokenSource:
		if typed == nil {
			return Token{}, false
		}
		base, ok := typed.base.(*dfaTokenSource)
		if !ok {
			return Token{}, false
		}
		if languageUsesExternalScannerCheckpoints(base.language) {
			return scanIncludedRangeLeafTokenWithExternalCheckpoint(typed, base, leaf)
		}
		snapshot, ok := prepareDFALeafScan(base, leaf)
		if !ok {
			return Token{}, false
		}
		idx := typed.idx
		tok := typed.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
		restoreDFALeafScan(base, snapshot)
		typed.idx = idx
		return tok, true
	default:
		return Token{}, false
	}
}

func scanIncludedRangeLeafTokenWithExternalCheckpoint(ts *includedRangeTokenSource, dts *dfaTokenSource, leaf *Node) (Token, bool) {
	if ts == nil || dts == nil || dts.lexer == nil || leaf == nil {
		return Token{}, false
	}
	cp, ok := externalScannerCheckpointForNode(leaf)
	if !ok {
		return Token{}, false
	}
	snapshot, ok := snapshotDFATokenSourceState(dts)
	if !ok {
		return Token{}, false
	}
	idx := ts.idx
	defer func() {
		restoreDFATokenSourceState(dts, snapshot)
		ts.idx = idx
	}()

	dts.state = leaf.preGotoState
	dts.glrStates = nil
	dts.restoreExternalScannerState(cp.start)
	tok := ts.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
	if tok.Symbol != leaf.symbol || tok.StartByte != leaf.startByte || tok.EndByte != leaf.endByte {
		return Token{}, false
	}
	if !dts.externalScannerStateMatches(cp.end) {
		return Token{}, false
	}
	return tok, true
}

func scanDFALeafTokenWithoutMutatingSource(dts *dfaTokenSource, leaf *Node) (Token, bool) {
	if dts != nil && languageUsesExternalScannerCheckpoints(dts.language) {
		return scanDFALeafTokenWithExternalCheckpoint(dts, leaf)
	}
	snapshot, ok := prepareDFALeafScan(dts, leaf)
	if !ok {
		return Token{}, false
	}
	tok := dts.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
	restoreDFALeafScan(dts, snapshot)
	return tok, true
}

func scanDFALeafTokenWithExternalCheckpoint(dts *dfaTokenSource, leaf *Node) (Token, bool) {
	if dts == nil || dts.lexer == nil || leaf == nil {
		return Token{}, false
	}
	cp, ok := externalScannerCheckpointForNode(leaf)
	if !ok {
		return Token{}, false
	}
	snapshot, ok := snapshotDFATokenSourceState(dts)
	if !ok {
		return Token{}, false
	}
	defer restoreDFATokenSourceState(dts, snapshot)

	dts.state = leaf.preGotoState
	dts.glrStates = nil
	dts.restoreExternalScannerState(cp.start)
	tok := dts.SkipToByteWithPoint(leaf.startByte, leaf.startPoint)
	if tok.Symbol != leaf.symbol || tok.StartByte != leaf.startByte || tok.EndByte != leaf.endByte {
		return Token{}, false
	}
	if !dts.externalScannerStateMatches(cp.end) {
		return Token{}, false
	}
	return tok, true
}

type dfaLeafScanSnapshot struct {
	state                  StateID
	glrStates              []StateID
	lexer                  Lexer
	lastExternalTokenStart uint32
	lastExternalTokenEnd   uint32
	lastExternalTokenValid bool
	extZeroPos             int
	extZeroState           StateID
	zeroWidthPos           int
	zeroWidthCount         int
}

func prepareDFALeafScan(dts *dfaTokenSource, leaf *Node) (dfaLeafScanSnapshot, bool) {
	if dts == nil || dts.lexer == nil || dts.language == nil || leaf == nil {
		return dfaLeafScanSnapshot{}, false
	}
	if dts.language.ExternalScanner != nil || len(dts.language.ExternalSymbols) != 0 {
		return dfaLeafScanSnapshot{}, false
	}
	snapshot := dfaLeafScanSnapshot{
		state:                  dts.state,
		glrStates:              dts.glrStates,
		lexer:                  *dts.lexer,
		lastExternalTokenStart: dts.lastExternalTokenStartByte,
		lastExternalTokenEnd:   dts.lastExternalTokenEndByte,
		lastExternalTokenValid: dts.lastExternalTokenValid,
		extZeroPos:             dts.extZeroPos,
		extZeroState:           dts.extZeroState,
		zeroWidthPos:           dts.zeroWidthPos,
		zeroWidthCount:         dts.zeroWidthCount,
	}
	dts.state = leaf.preGotoState
	dts.glrStates = nil
	return snapshot, true
}

func restoreDFALeafScan(dts *dfaTokenSource, snapshot dfaLeafScanSnapshot) {
	dts.state = snapshot.state
	dts.glrStates = snapshot.glrStates
	*dts.lexer = snapshot.lexer
	dts.lastExternalTokenStartByte = snapshot.lastExternalTokenStart
	dts.lastExternalTokenEndByte = snapshot.lastExternalTokenEnd
	dts.lastExternalTokenValid = snapshot.lastExternalTokenValid
	dts.extZeroPos = snapshot.extZeroPos
	dts.extZeroState = snapshot.extZeroState
	dts.zeroWidthPos = snapshot.zeroWidthPos
	dts.zeroWidthCount = snapshot.zeroWidthCount
}

type dfaTokenSourceStateSnapshot struct {
	state                  StateID
	glrStates              []StateID
	lexer                  Lexer
	hasLexer               bool
	externalValid          []bool
	extZeroTried           []bool
	externalTokenStart     []byte
	externalTokenEnd       []byte
	externalSnapshot       []byte
	externalRetrySnap      []byte
	externalCompare        []byte
	externalScannerState   []byte
	externalLexer          ExternalLexer
	externalRetryLexer     ExternalLexer
	lastExternalTokenStart uint32
	lastExternalTokenEnd   uint32
	lastExternalTokenValid bool
	extZeroPos             int
	extZeroState           StateID
	zeroWidthPos           int
	zeroWidthCount         int
}

func snapshotTokenSourceState(ts TokenSource) (func(), bool) {
	switch typed := ts.(type) {
	case *dfaTokenSource:
		snapshot, ok := snapshotDFATokenSourceState(typed)
		if !ok {
			return nil, false
		}
		return func() {
			restoreDFATokenSourceState(typed, snapshot)
		}, true
	case *includedRangeTokenSource:
		restoreBase, ok := snapshotTokenSourceState(typed.base)
		if !ok {
			return nil, false
		}
		idx := typed.idx
		return func() {
			restoreBase()
			typed.idx = idx
		}, true
	default:
		return nil, false
	}
}

func snapshotDFATokenSourceState(dts *dfaTokenSource) (dfaTokenSourceStateSnapshot, bool) {
	if dts == nil {
		return dfaTokenSourceStateSnapshot{}, false
	}
	state := dfaTokenSourceStateSnapshot{
		state:                  dts.state,
		glrStates:              append([]StateID(nil), dts.glrStates...),
		externalValid:          append([]bool(nil), dts.externalValid...),
		extZeroTried:           append([]bool(nil), dts.extZeroTried...),
		externalTokenStart:     append([]byte(nil), dts.externalTokenStart...),
		externalTokenEnd:       append([]byte(nil), dts.externalTokenEnd...),
		externalSnapshot:       append([]byte(nil), dts.externalSnapshot...),
		externalRetrySnap:      append([]byte(nil), dts.externalRetrySnap...),
		externalCompare:        append([]byte(nil), dts.externalCompare...),
		externalLexer:          dts.externalLexer,
		externalRetryLexer:     dts.externalRetryLexer,
		lastExternalTokenStart: dts.lastExternalTokenStartByte,
		lastExternalTokenEnd:   dts.lastExternalTokenEndByte,
		lastExternalTokenValid: dts.lastExternalTokenValid,
		extZeroPos:             dts.extZeroPos,
		extZeroState:           dts.extZeroState,
		zeroWidthPos:           dts.zeroWidthPos,
		zeroWidthCount:         dts.zeroWidthCount,
	}
	if dts.language != nil && dts.language.ExternalScanner != nil {
		buf := make([]byte, 0, externalScannerSerializationBufferSize)
		state.externalScannerState = append([]byte(nil), dts.captureExternalScannerStateInto(&buf)...)
	}
	if dts.lexer != nil {
		state.lexer = *dts.lexer
		state.hasLexer = true
	}
	return state, true
}

func restoreDFATokenSourceState(dts *dfaTokenSource, state dfaTokenSourceStateSnapshot) {
	if dts == nil {
		return
	}
	dts.state = state.state
	dts.glrStates = append(dts.glrStates[:0], state.glrStates...)
	if dts.lexer == nil {
		dts.lexer = &Lexer{}
	}
	if state.hasLexer {
		*dts.lexer = state.lexer
	} else {
		dts.lexer = nil
	}
	dts.externalValid = append(dts.externalValid[:0], state.externalValid...)
	dts.extZeroTried = append(dts.extZeroTried[:0], state.extZeroTried...)
	dts.externalTokenStart = append(dts.externalTokenStart[:0], state.externalTokenStart...)
	dts.externalTokenEnd = append(dts.externalTokenEnd[:0], state.externalTokenEnd...)
	dts.externalSnapshot = append(dts.externalSnapshot[:0], state.externalSnapshot...)
	dts.externalRetrySnap = append(dts.externalRetrySnap[:0], state.externalRetrySnap...)
	dts.externalCompare = append(dts.externalCompare[:0], state.externalCompare...)
	dts.externalLexer = state.externalLexer
	dts.externalRetryLexer = state.externalRetryLexer
	dts.lastExternalTokenStartByte = state.lastExternalTokenStart
	dts.lastExternalTokenEndByte = state.lastExternalTokenEnd
	dts.lastExternalTokenValid = state.lastExternalTokenValid
	dts.extZeroPos = state.extZeroPos
	dts.extZeroState = state.extZeroState
	dts.zeroWidthPos = state.zeroWidthPos
	dts.zeroWidthCount = state.zeroWidthCount
	if dts.language != nil && dts.language.ExternalScanner != nil {
		dts.language.ExternalScanner.Deserialize(dts.externalPayload, state.externalScannerState)
	}
}

func reuseTreeWithNewSource(oldTree *Tree, source []byte, dirtyNode *Node, clearSubtree bool) *Tree {
	if oldTree == nil || oldTree.root == nil {
		return nil
	}
	arena := oldTree.arena
	if arena != nil {
		arena.Retain()
	}
	borrowed := retainBorrowedArenasForReusedTree(oldTree, arena)
	if clearSubtree {
		clearDirtySubtreeAndPath(dirtyNode)
	} else {
		clearDirtyPathToRoot(dirtyNode)
	}
	tree := newTreeWithUniqueArenas(oldTree.root, source, oldTree.language, arena, borrowed)
	tree.forestFastPath = oldTree.forestFastPath
	tree.incrementalReuseDisabled = oldTree.incrementalReuseDisabled
	tree.compactMaterialized = oldTree.compactMaterialized
	tree.resultErrorSummary = oldTree.resultErrorSummary
	tree.resultCompatibilityApplied = oldTree.resultCompatibilityApplied
	// This tree shares the old root, so shouldNormalizeIncrementalReturnedTree
	// skips result normalization for it (same root). Even if a range-limited
	// pass ever ran here, incrementalReparsedTopLevelRanges would read this
	// shared tree's arenas rather than any freshly reparsed span, but the walk
	// stays sound: the shared tree is already normalized and the range-limited Go
	// normalizers are idempotent, so re-running them changes nothing.
	return tree
}

func clearDirtySubtreeAndPath(n *Node) {
	clearDirtySubtree(n)
	if n != nil {
		clearDirtyPathToRoot(n.parent)
	}
}

func clearDirtySubtree(n *Node) {
	if n == nil {
		return
	}
	n.setDirty(false)
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		clearDirtySubtree(nodeChildAtForReason(n, i, materializeForEdit))
	}
}

func clearDirtyPathToRoot(n *Node) {
	for n != nil {
		n.setDirty(false)
		n = n.parent
	}
}

func retainBorrowedArenasForReusedTree(oldTree *Tree, primary *nodeArena) []*nodeArena {
	if oldTree == nil || len(oldTree.borrowedArena) == 0 {
		return nil
	}
	var borrowed []*nodeArena
	for _, arena := range oldTree.borrowedArena {
		if arena == nil || arena == primary {
			continue
		}
		duplicate := false
		for _, existing := range borrowed {
			if existing == arena {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		arena.Retain()
		borrowed = append(borrowed, arena)
	}
	return borrowed
}
