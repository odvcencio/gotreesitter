package gotreesitter

// NormalizeCSharpRecoveredTopLevelChunksForTest re-applies the c_sharp
// top-level chunk recovery pass to an already-parsed tree's raw root. It
// exists so the external gotreesitter_test package can prove the pass keeps
// the parent/child HasError invariant on a second application (task #97):
// the pass replaces the root's children with re-parsed chunks, and the
// root's error flag must follow those children instead of being cleared.
func NormalizeCSharpRecoveredTopLevelChunksForTest(p *Parser, tree *Tree, source []byte) {
	normalizeCSharpRecoveredTopLevelChunks(rawRootOrNil(tree), source, p)
}
