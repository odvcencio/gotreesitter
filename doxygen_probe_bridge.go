//go:build doxygen_probe

package gotreesitter

// DoxygenProbeNormalization applies the live result normalizer to a raw root
// and returns the direct arm counters for the temporary probe.
func DoxygenProbeNormalization(root *Node, source []byte, lang *Language) (before, after string, checked, run, visited, rewritten uint64) {
	if root == nil || lang == nil {
		return "", "", 0, 0, 0, 0
	}
	before = root.SExpr(lang)
	p := &Parser{language: lang}
	_ = normalizeResultCompatibility(root, source, p, nil)
	after = root.SExpr(lang)
	for _, pass := range p.normalizationStats.namedPasses {
		if pass.name == "dispatch.doxygen" {
			return before, after, pass.checked, pass.run, pass.nodesVisited, pass.nodesRewritten
		}
	}
	return before, after, p.normalizationStats.passesChecked, p.normalizationStats.passesRun, p.normalizationStats.nodesVisited, p.normalizationStats.nodesRewritten
}
