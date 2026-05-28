package gotreesitter

import "strings"

func normalizeKotlinCompatibility(root *Node, source []byte, lang *Language) {
	normalizeKotlinRecoveredSourceFileRoot(root, source, lang)
	normalizeKotlinBindingPatternKindTokens(root, source, lang)
}

func normalizeKotlinRecoveredSourceFileRoot(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" || root.Type(lang) != "ERROR" {
		return
	}
	if !kotlinRootLooksRecoverableSourceFile(root, lang) {
		return
	}
	normalizeKotlinTopLevelFunctionFragments(root, source, lang)
	sym, ok := symbolByName(lang, "source_file")
	if !ok {
		return
	}
	retagResultRootAndRefreshError(root, sym, symbolIsNamed(lang, sym))
}

func kotlinRootLooksRecoverableSourceFile(root *Node, lang *Language) bool {
	if root == nil || lang == nil || resultChildCount(root) == 0 {
		return false
	}
	for i := 0; i < resultChildCount(root); i++ {
		child := resultChildAt(root, i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "package_header",
			"import_list",
			"class_declaration",
			"function_declaration",
			"object_declaration",
			"property_declaration",
			"typealias_declaration",
			"multiline_comment",
			"line_comment":
			return true
		}
	}
	return false
}

func normalizeKotlinTopLevelFunctionFragments(root *Node, source []byte, lang *Language) {
	fnSym, ok := symbolByName(lang, "function_declaration")
	if !ok {
		return
	}
	funSym, ok := symbolByName(lang, "fun")
	if !ok {
		return
	}
	children := resultChildSliceForMutation(root)
	if len(children) < 3 {
		return
	}
	arena := root.ownerArena
	if arena == nil {
		return
	}
	rebuilt := make([]*Node, 0, len(children))
	changed := false
	for i := 0; i < len(children); i++ {
		if fn, ok := kotlinRecoveredTopLevelFunction(arena, children, i, source, lang, fnSym, funSym); ok {
			rebuilt = append(rebuilt, fn)
			i += 2
			changed = true
			continue
		}
		rebuilt = append(rebuilt, children[i])
	}
	if !changed {
		return
	}
	replaceNodeChildrenUnfielded(root, cloneNodeSliceInArena(arena, rebuilt))
}

func kotlinRecoveredTopLevelFunction(arena *nodeArena, children []*Node, idx int, source []byte, lang *Language, fnSym, funSym Symbol) (*Node, bool) {
	if idx+2 >= len(children) {
		return nil, false
	}
	funKeyword := children[idx]
	name := children[idx+1]
	params := children[idx+2]
	if funKeyword == nil || name == nil || params == nil {
		return nil, false
	}
	if funKeyword.Type(lang) != "ERROR" || strings.TrimSpace(funKeyword.Text(source)) != "fun" {
		return nil, false
	}
	if name.Type(lang) != "simple_identifier" || params.Type(lang) != "function_value_parameters" {
		return nil, false
	}
	retagResultRoot(funKeyword, funSym, symbolIsNamed(lang, funSym))
	funKeyword.setHasError(false)
	fnChildren := cloneNodeSliceInArena(funKeyword.ownerArena, []*Node{funKeyword, name, params})
	fn := newParentNodeInArena(funKeyword.ownerArena, fnSym, symbolIsNamed(lang, fnSym), fnChildren, nil, 0)
	fn.setHasError(true)
	return fn, true
}

func normalizeKotlinBindingPatternKindTokens(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil || lang.Name != "kotlin" {
		return
	}

	var walk func(*Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.Type(lang) == "binding_pattern_kind" && len(n.children) == 0 {
			normalizeKotlinBindingPatternKindToken(n, source, lang)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
}

func normalizeKotlinBindingPatternKindToken(n *Node, source []byte, lang *Language) {
	if n == nil || n.startByte > n.endByte || n.endByte > uint32(len(source)) {
		return
	}
	text := string(source[n.startByte:n.endByte])
	if text != "val" && text != "var" {
		return
	}
	normalizeCollapsedTextToken(n, source, lang, func(text string) bool {
		return text == "val" || text == "var"
	})
}
