package gotreesitter

import "strings"

func normalizeCobolCompatibility(root *Node, source []byte, lang *Language) {
	normalizeCobolLeadingAreaStart(root, source, lang)
	normalizeCobolTopLevelDefinitionEnd(root, source, lang)
	normalizeCobolDivisionSiblingEnds(root, source, lang)
	normalizeCobolPeriodChildren(root, source, lang)
}
func normalizeCobolLeadingAreaStart(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || (lang.Name != "cobol" && lang.Name != "COBOL") || len(source) == 0 {
		return
	}
	start := firstNonWhitespaceByte(source)
	if start == 0 {
		// COBOL fixed format: columns 1-6 are sequence numbers (non-whitespace).
		// Detect this pattern and use column 7 (byte 6) as the adjusted start.
		if len(source) >= 7 && (source[6] == ' ' || source[6] == '*' || source[6] == '-' || source[6] == '/') {
			start = 6
		} else {
			return
		}
	}
	startPoint := advancePointByBytes(Point{}, source[:start])
	setNodeStartTo := func(n *Node) {
		if n == nil || n.startByte == start {
			return
		}
		n.startByte = start
		n.startPoint = startPoint
	}
	setNodeStartTo(root)
	if len(root.children) == 0 {
		return
	}
	def := (*Node)(nil)
	for _, child := range root.children {
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	setNodeStartTo(def)
	if len(def.children) == 0 {
		return
	}
	for _, child := range def.children {
		if child != nil && !child.IsExtra() && child.Type(lang) == "identification_division" {
			setNodeStartTo(child)
			break
		}
	}
}

func normalizeCobolTopLevelDefinitionEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || (lang.Name != "cobol" && lang.Name != "COBOL") || root.Type(lang) != "start" || len(root.children) == 0 {
		return
	}
	def := (*Node)(nil)
	for _, child := range root.children {
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= def.endByte {
		return
	}
	def.endByte = end
	def.endPoint = advancePointByBytes(Point{}, source[:end])
}

func normalizeCobolDivisionSiblingEnds(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || (lang.Name != "cobol" && lang.Name != "COBOL") || root.Type(lang) != "start" || len(root.children) == 0 {
		return
	}
	def := (*Node)(nil)
	for _, child := range root.children {
		if child != nil && !child.IsExtra() && child.Type(lang) == "program_definition" {
			def = child
			break
		}
	}
	if def == nil {
		return
	}
	for i := 0; i+1 < len(def.children); i++ {
		cur := def.children[i]
		next := def.children[i+1]
		if cur == nil || next == nil || cur.IsExtra() || next.IsExtra() {
			continue
		}
		if !strings.HasSuffix(cur.Type(lang), "_division") {
			continue
		}
		end := lastNonTriviaByteEnd(source[:next.startByte])
		if end == 0 || end <= cur.startByte || end >= cur.endByte {
			continue
		}
		cur.endByte = end
		cur.endPoint = advancePointByBytes(Point{}, source[:end])
	}
}

func normalizeCobolPeriodChildren(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || (lang.Name != "cobol" && lang.Name != "COBOL") {
		return
	}
	normalizeCollapsedNamedLeafChildren(root, lang, "period", ".")
}
