package grammars

import gts "github.com/odvcencio/gotreesitter"

// HeaderRange returns the source span of a declaration's header. Pass the
// declaration node and its language. Declarations with a body field end at
// the preceding token. Recognized Go, Python and ECMAScript declaration forms
// also support initializers, bodyless signatures and declaration wrappers.
// An unrecognized boundary yields false.
func HeaderRange(declaration *gts.Node, lang *gts.Language) (gts.Range, bool) {
	if declaration == nil || lang == nil {
		return gts.Range{}, false
	}
	end := headerEnd(declaration, lang)
	if end == nil || end.IsMissing() {
		return gts.Range{}, false
	}
	start := declaration
	for parent := start.Parent(); parent != nil && wrapsHeader(parent, start, lang); parent = start.Parent() {
		start = parent
	}
	if end.EndByte() <= start.StartByte() {
		return gts.Range{}, false
	}
	return gts.Range{StartByte: start.StartByte(), StartPoint: start.StartPoint(), EndByte: end.EndByte(), EndPoint: end.EndPoint()}, true
}

func wrapsHeader(parent, child *gts.Node, lang *gts.Language) bool {
	switch parent.Type(lang) {
	case "export_statement":
		return parent.ChildByFieldName("declaration", lang) == child
	case "decorated_definition":
		return parent.ChildByFieldName("definition", lang) == child
	case "ambient_declaration", "lexical_declaration", "variable_declaration", "type_declaration", "var_declaration", "const_declaration":
		// Only the first declarator can include its shared keyword. The
		// opening parenthesis in a Go group keeps each spec independent.
		prev := child.PrevSibling()
		for prev != nil && prev.Type(lang) == "comment" {
			prev = prev.PrevSibling()
		}
		if prev != nil {
			switch prev.Type(lang) {
			case "declare", "let", "var", "const", "type":
				return true
			}
		}
	}
	return false
}

func headerEnd(n *gts.Node, lang *gts.Language) *gts.Node {
	if body := n.ChildByFieldName("body", lang); body != nil {
		return body.PrevSibling()
	}
	switch lang.Name {
	case "go", "javascript", "typescript", "tsx", "python":
	default:
		return nil
	}
	switch n.Type(lang) {
	case "type_spec", "type_alias":
		typ := n.ChildByFieldName("type", lang)
		if typ != nil && (typ.Type(lang) == "struct_type" || typ.Type(lang) == "interface_type") {
			for i := 0; i < typ.ChildCount(); i++ {
				child := typ.Child(i)
				switch child.Type(lang) {
				case "{", "field_declaration_list":
					return child.PrevSibling()
				}
			}
			return nil
		}
	case "assignment", "var_spec", "const_spec", "variable_declarator", "public_field_definition", "field_definition", "enum_assignment":
		for i := 0; i < n.ChildCount(); i++ {
			if child := n.Child(i); child.Type(lang) == "=" {
				return child.PrevSibling()
			}
		}
	case "field_declaration", "method_elem", "function_declaration", "method_declaration", "function_signature", "method_signature", "abstract_method_signature", "property_signature", "type_alias_declaration":
	default:
		return nil
	}
	return n
}
