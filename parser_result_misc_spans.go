package gotreesitter

func normalizeNimTopLevelCallEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "nim" || root.Type(lang) != "source_file" || len(root.children) != 1 {
		return
	}
	call := root.children[0]
	if call == nil || call.IsExtra() || call.Type(lang) != "call" {
		return
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= call.endByte {
		return
	}
	call.endByte = end
	call.endPoint = advancePointByBytes(Point{}, source[:end])
}

func normalizePascalTopLevelProgramEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "pascal" || root.Type(lang) != "root" || len(root.children) == 0 {
		return
	}
	program := root.children[0]
	if program == nil || program.IsExtra() || program.Type(lang) != "program" {
		return
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= program.endByte {
		return
	}
	program.endByte = end
	program.endPoint = advancePointByBytes(Point{}, source[:end])
}

func normalizeCommentTrailingExtraTrivia(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "comment" || root.Type(lang) != "source" {
		return
	}
	trimTrailingExtraTriviaRoot(root, source)
}

func normalizePascalTrailingExtraTrivia(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "pascal" || root.Type(lang) != "root" {
		return
	}
	trimTrailingExtraTriviaRoot(root, source)
}

func normalizeRSTTopLevelSectionEnd(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "rst" || root.Type(lang) != "document" || len(root.children) == 0 {
		return
	}
	trimTrailingExtraTriviaRoot(root, source)
	section := root.children[0]
	if section == nil || section.IsExtra() || section.Type(lang) != "section" {
		return
	}
	end := lastNonTriviaByteEnd(source)
	if end == 0 || end >= section.endByte {
		return
	}
	section.endByte = end
	section.endPoint = advancePointByBytes(Point{}, source[:end])
}

func bytesAreCooklangStepTail(b []byte) bool {
	sawPunctuation := false
	for _, c := range b {
		switch c {
		case '.', '!', '?':
			if sawPunctuation {
				return false
			}
			sawPunctuation = true
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return sawPunctuation
}
func normalizeCooklangTrailingStepTail(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "cooklang" || len(source) == 0 {
		return
	}
	if root.Type(lang) != "recipe" || len(root.children) != 1 {
		return
	}
	step := root.children[0]
	if step == nil || step.Type(lang) != "step" || step.endByte >= uint32(len(source)) {
		return
	}
	tail := source[step.endByte:]
	if !bytesAreCooklangStepTail(tail) {
		return
	}
	stepEnd := step.endByte
	for i := int(step.endByte); i < len(source); i++ {
		switch source[i] {
		case '.', '!', '?':
			stepEnd = uint32(i + 1)
		}
	}
	if stepEnd > step.endByte {
		extendNodeEndTo(step, stepEnd, source)
	}
	if root.endByte < uint32(len(source)) {
		extendNodeEndTo(root, uint32(len(source)), source)
	}
}

func lineBreakEndAt(source []byte, start, limit uint32) uint32 {
	if start >= limit || start >= uint32(len(source)) {
		return 0
	}
	switch source[start] {
	case '\n':
		return start + 1
	case '\r':
		if start+1 < limit && start+1 < uint32(len(source)) && source[start+1] == '\n' {
			return start + 2
		}
		return start + 1
	default:
		return 0
	}
}

func normalizeFortranStatementLineBreaks(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "fortran" || len(source) == 0 {
		return
	}
	var walk func(*Node, int)
	walk = func(n *Node, depth int) {
		if n == nil || depth > maxTreeWalkDepth {
			return
		}
		if n.Type(lang) == "program" {
			for i := 0; i+1 < len(n.children); i++ {
				cur := n.children[i]
				next := n.children[i+1]
				if cur == nil || next == nil || cur.endByte >= next.startByte {
					continue
				}
				if cur.Type(lang) != "program_statement" {
					continue
				}
				if end := lineBreakEndAt(source, cur.endByte, next.startByte); end > cur.endByte {
					extendNodeEndTo(cur, end, source)
				}
			}
		}
		for _, child := range n.children {
			walk(child, depth+1)
		}
	}
	walk(root, 0)
}

func normalizeNginxAttributeLineBreaks(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "nginx" || len(source) == 0 {
		return
	}
	var walk func(*Node, int)
	walk = func(n *Node, depth int) {
		if n == nil || depth > maxTreeWalkDepth {
			return
		}
		if n.Type(lang) == "attribute" {
			if end := lineBreakEndAt(source, n.endByte, uint32(len(source))); end > n.endByte {
				extendNodeEndTo(n, end, source)
			}
		}
		for _, child := range n.children {
			walk(child, depth+1)
		}
	}
	walk(root, 0)
}

func normalizeTopLevelTrailingLineBreakSpan(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 {
		return
	}
	switch lang.Name {
	case "caddy", "fortran", "pug":
	default:
		return
	}
	if len(root.children) != 1 {
		return
	}
	child := root.children[0]
	if child == nil || child.endByte >= root.endByte || root.endByte > uint32(len(source)) {
		return
	}
	gap := source[child.endByte:root.endByte]
	if !bytesAreTrivia(gap) || !bytesContainLineBreak(gap) {
		return
	}
	extendNodeEndTo(child, root.endByte, source)
}

func normalizeRootEOFNewlineSpan(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 || root.endByte >= uint32(len(source)) {
		return
	}
	switch {
	case lang.Name == "go" && root.Type(lang) == "source_file":
	case lang.Name == "scala" && root.Type(lang) == "compilation_unit":
	default:
		return
	}
	gap := source[root.endByte:]
	if !bytesAreTrivia(gap) || !bytesContainLineBreak(gap) {
		return
	}
	extendNodeEndTo(root, uint32(len(source)), source)
}
