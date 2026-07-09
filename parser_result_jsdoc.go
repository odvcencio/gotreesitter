package gotreesitter

// normalizeJsdocCompatibility applies narrow post-build tree rewrites that
// keep gotreesitter's jsdoc output aligned with C tree-sitter.
func normalizeJsdocCompatibility(root *Node, source []byte, lang *Language) {
	normalizeJsdocNestedDocumentRecovery(root, source, lang)
}

// normalizeJsdocNestedDocumentRecovery mirrors C tree-sitter's shape for a
// whole-input jsdoc comment (e.g. multiple `@tag` lines) whose GLR recovery
// pass wraps the recovered tag structure in a *nested* `document` node —
// with interstitial ERROR nodes around the inter-tag whitespace, and a
// trailing zero-width MISSING `/` left dangling on the outer root — instead
// of reducing everything flat into the root `document`, which is what C
// tree-sitter (and gotreesitter itself, for the single-tag case that never
// hits this recovery path) produces.
//
// It hoists the nested document's non-ERROR children in place of the nested
// document (dropping the interstitial ERROR wrappers), then drops the
// trailing zero-width MISSING `/` that is now redundant with the nested
// document's own closing `/`, which was just promoted to the root.
//
// This is narrowly gated on the exact defect shape — a whole-input `document`
// root flagged hasError with a nested `document` child — so ordinary jsdoc
// comments (the common case: at most one `@tag`, never hitting this recovery
// path) are left completely untouched.
func normalizeJsdocNestedDocumentRecovery(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "jsdoc" || len(source) == 0 {
		return
	}
	if root.Type(lang) != "document" || root.startByte != 0 || root.endByte != uint32(len(source)) || !root.HasError() {
		return
	}
	total := resultChildCount(root)
	if total == 0 {
		return
	}
	nestedIndex := -1
	for i := 0; i < total; i++ {
		child := resultChildAt(root, i)
		if child != nil && child.Type(lang) == "document" {
			nestedIndex = i
			break
		}
	}
	if nestedIndex < 0 {
		return
	}
	nested := resultChildAt(root, nestedIndex)
	nestedTotal := resultChildCount(nested)

	flattened := make([]*Node, 0, total-1+nestedTotal)
	for i := 0; i < total; i++ {
		if i == nestedIndex {
			for j := 0; j < nestedTotal; j++ {
				grandchild := resultChildAt(nested, j)
				if grandchild == nil || grandchild.Type(lang) == "ERROR" {
					continue
				}
				flattened = append(flattened, grandchild)
			}
			continue
		}
		if child := resultChildAt(root, i); child != nil {
			flattened = append(flattened, child)
		}
	}

	flattened = jsdocDropTrailingZeroWidthMissing(flattened)
	if len(flattened) == 0 {
		return
	}

	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(root.ownerArena, flattened))
	root.startByte = 0
	root.startPoint = Point{}
	setNodeEndTo(root, uint32(len(source)), source)
	refreshResultRootError(root)
}

// jsdocDropTrailingZeroWidthMissing drops a trailing synthetic MISSING token
// (zero-width, unnamed) that GLR recovery appended after the outer document
// ran out of real input to attach it to. Once the nested document is
// unwrapped in place, its own closing `/` already occupies that structural
// role, so the outer placeholder is redundant relative to C's flat shape.
func jsdocDropTrailingZeroWidthMissing(nodes []*Node) []*Node {
	for len(nodes) > 0 {
		last := nodes[len(nodes)-1]
		if last == nil || !last.IsMissing() || last.IsNamed() || last.startByte != last.endByte {
			break
		}
		nodes = nodes[:len(nodes)-1]
	}
	return nodes
}
