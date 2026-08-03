package gotreesitter

func normalizeRootEOFNewlineSpan(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || len(source) == 0 || root.endByte >= uint32(len(source)) {
		return
	}
	if lang.Name != "scala" || root.Type(lang) != "compilation_unit" {
		return
	}
	gap := source[root.endByte:]
	if !bytesAreTrivia(gap) || !bytesContainLineBreak(gap) {
		return
	}
	extendNodeEndTo(root, uint32(len(source)), source)
}
