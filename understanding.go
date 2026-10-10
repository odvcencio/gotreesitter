package gotreesitter

import (
	"strings"

	"github.com/odvcencio/gotreesitter/internal/declarationfacts"
)

// DefinitionSpan is a compact language-neutral declaration span extracted from
// a syntax tree. Each declared name has its own NameStartByte/NameEndByte.
// Names from one construct can share StartByte/EndByte: those equal ranges
// mean siblings, not a conflict or a parent/child relationship.
type DefinitionSpan struct {
	Lang          string
	Kind          string
	Name          string
	NodeType      string
	StartByte     uint32
	EndByte       uint32
	NameStartByte uint32
	NameEndByte   uint32
}

// DeclarationFact is an opt-in grammar-owned declaration, emitted only into
// FactSet.Declarations. It includes the definition/name byte ranges plus Shape
// ("struct", "interface", "alias", or empty for other defined types) and Container,
// ContainerStartByte, and ContainerEndByte for named members. Kind remains "type"
// on type facts. Container ranges cover the containing type_spec.
//
// Equal StartByte/EndByte ranges from one construct mean sibling declarations,
// not conflicts or a parent/child relationship. Each name keeps its own range.
// Zero metadata means not applicable and is omitted from JSON.
type DeclarationFact = declarationfacts.Fact

// CallRef is a compact language-neutral call-site reference extracted from a
// syntax tree.
type CallRef struct {
	Lang          string
	Kind          string
	Name          string
	Receiver      string
	NodeType      string
	StartByte     uint32
	EndByte       uint32
	NameStartByte uint32
	NameEndByte   uint32
}

// HeritageRef is a compact language-neutral inheritance/base-class reference.
type HeritageRef struct {
	Lang            string
	Kind            string
	Name            string
	Parent          string
	NodeType        string
	StartByte       uint32
	EndByte         uint32
	ParentStartByte uint32
	ParentEndByte   uint32
}

// ExtractDefinitionSpans returns language-neutral declaration spans for common
// code-understanding workflows. The extractor is intentionally conservative:
// unsupported languages or declaration shapes are skipped rather than guessed.
func ExtractDefinitionSpans(tree *Tree) []DefinitionSpan {
	if tree == nil || tree.RootNode() == nil || tree.Language() == nil {
		return nil
	}
	lang := tree.Language()
	source := tree.Source()
	var spans []DefinitionSpan
	walkTree(tree.RootNode(), lang, func(n *Node) bool {
		if span, ok := definitionSpanForNode(n, lang, source); ok {
			spans = append(spans, span)
		}
		return true
	})
	return spans
}

// ExtractCalls returns language-neutral call-site references for common
// code-understanding workflows.
func ExtractCalls(tree *Tree) []CallRef {
	if tree == nil || tree.RootNode() == nil || tree.Language() == nil {
		return nil
	}
	lang := tree.Language()
	source := tree.Source()
	var refs []CallRef
	walkTree(tree.RootNode(), lang, func(n *Node) bool {
		if ref, ok := callRefForNode(n, lang, source); ok {
			refs = append(refs, ref)
		}
		return true
	})
	return refs
}

// ExtractHeritage returns language-neutral inheritance/base-class references
// for common code-understanding workflows.
func ExtractHeritage(tree *Tree) []HeritageRef {
	if tree == nil || tree.RootNode() == nil || tree.Language() == nil {
		return nil
	}
	lang := tree.Language()
	source := tree.Source()
	var refs []HeritageRef
	walkTree(tree.RootNode(), lang, func(n *Node) bool {
		appendHeritageForNode(n, lang, source, &refs)
		return true
	})
	return refs
}

// EnclosingDefinition returns the nearest definition node that contains
// byteOffset. It uses NodeAtByte and parent links instead of scanning every
// definition span.
func EnclosingDefinition(tree *Tree, byteOffset uint32) (DefinitionSpan, bool) {
	if tree == nil || tree.Language() == nil {
		return DefinitionSpan{}, false
	}
	lang := tree.Language()
	source := tree.Source()
	for n := tree.NamedNodeAtByte(byteOffset); n != nil; n = n.Parent() {
		if span, ok := definitionSpanForNode(n, lang, source); ok {
			return span, true
		}
	}
	return DefinitionSpan{}, false
}

// EnclosingDefinition returns the nearest definition node that contains
// byteOffset.
func (t *Tree) EnclosingDefinition(byteOffset uint32) (DefinitionSpan, bool) {
	return EnclosingDefinition(t, byteOffset)
}

func definitionSpanForNode(n *Node, lang *Language, source []byte) (DefinitionSpan, bool) {
	if n == nil || lang == nil {
		return DefinitionSpan{}, false
	}
	nodeType := n.Type(lang)
	kind := definitionKind(lang.Name, nodeType)
	if kind == "" {
		return DefinitionSpan{}, false
	}
	nameNode := definitionNameNode(n, lang)
	if nameNode == nil {
		return DefinitionSpan{}, false
	}
	name := strings.TrimSpace(nameNode.Text(source))
	if name == "" {
		return DefinitionSpan{}, false
	}
	return DefinitionSpan{
		Lang:          lang.Name,
		Kind:          kind,
		Name:          name,
		NodeType:      nodeType,
		StartByte:     n.StartByte(),
		EndByte:       n.EndByte(),
		NameStartByte: nameNode.StartByte(),
		NameEndByte:   nameNode.EndByte(),
	}, true
}

func definitionKind(langName, nodeType string) string {
	switch langName {
	case "go":
		switch nodeType {
		case "function_declaration":
			return "function"
		case "method_declaration":
			return "method"
		case "type_spec":
			return "type"
		}
	case "java":
		switch nodeType {
		case "class_declaration":
			return "class"
		case "interface_declaration":
			return "interface"
		case "enum_declaration":
			return "enum"
		case "record_declaration":
			return "record"
		case "method_declaration":
			return "method"
		case "constructor_declaration":
			return "constructor"
		}
	case "python", "starlark":
		switch nodeType {
		case "function_definition":
			return "function"
		case "class_definition":
			return "class"
		}
	case "javascript", "typescript", "tsx":
		switch nodeType {
		case "function_declaration":
			return "function"
		case "method_definition":
			return "method"
		case "class_declaration":
			return "class"
		}
	}
	return ""
}

func definitionNameNode(n *Node, lang *Language) *Node {
	if child := n.ChildByFieldName("name", lang); child != nil {
		return child
	}
	return firstDescendantByType(n, lang,
		"type_identifier",
		"identifier",
		"field_identifier",
		"property_identifier",
	)
}

func callRefForNode(n *Node, lang *Language, source []byte) (CallRef, bool) {
	if n == nil || lang == nil {
		return CallRef{}, false
	}
	nodeType := n.Type(lang)
	if !isCallNode(lang.Name, nodeType) {
		return CallRef{}, false
	}
	target := callTargetNode(n, lang)
	name, receiver, nameStart, nameEnd := expressionName(target, lang, source)
	if name == "" {
		return CallRef{}, false
	}
	return CallRef{
		Lang:          lang.Name,
		Kind:          "call",
		Name:          name,
		Receiver:      receiver,
		NodeType:      nodeType,
		StartByte:     n.StartByte(),
		EndByte:       n.EndByte(),
		NameStartByte: nameStart,
		NameEndByte:   nameEnd,
	}, true
}

func isCallNode(langName, nodeType string) bool {
	switch langName {
	case "go", "javascript", "typescript", "tsx":
		return nodeType == "call_expression"
	case "java":
		switch nodeType {
		case "method_invocation", "constructor_invocation", "super_constructor_invocation", "explicit_constructor_invocation":
			return true
		}
	case "python", "starlark":
		return nodeType == "call"
	}
	return false
}

func callTargetNode(n *Node, lang *Language) *Node {
	for _, field := range []string{"function", "name", "constructor"} {
		if child := n.ChildByFieldName(field, lang); child != nil {
			return child
		}
	}
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if child == nil {
			continue
		}
		switch child.Type(lang) {
		case "argument_list", "arguments", "type_arguments":
			continue
		case "(", ")":
			continue
		}
		return child
	}
	return nil
}

func expressionName(n *Node, lang *Language, source []byte) (name, receiver string, nameStart, nameEnd uint32) {
	if n == nil {
		return "", "", 0, 0
	}
	if child := expressionNameNode(n, lang); child != nil {
		name = strings.TrimSpace(child.Text(source))
		nameStart = child.StartByte()
		nameEnd = child.EndByte()
		if nameStart > n.StartByte() && int(nameStart) <= len(source) {
			receiver = strings.TrimSpace(string(source[n.StartByte():nameStart]))
			receiver = strings.TrimRight(receiver, ".")
		}
		return name, receiver, nameStart, nameEnd
	}
	text := strings.TrimSpace(n.Text(source))
	if text == "" {
		return "", "", 0, 0
	}
	name = lastDottedName(text)
	if name == "" {
		return "", "", 0, 0
	}
	if idx := strings.LastIndex(text, name); idx >= 0 {
		nameStart = n.StartByte() + uint32(idx)
		nameEnd = nameStart + uint32(len(name))
		receiver = strings.TrimRight(strings.TrimSpace(text[:idx]), ".")
	}
	return name, receiver, nameStart, nameEnd
}

func expressionNameNode(n *Node, lang *Language) *Node {
	for _, field := range []string{"name", "field", "attribute", "property"} {
		if child := n.ChildByFieldName(field, lang); child != nil {
			return child
		}
	}
	return lastDescendantByType(n, lang,
		"field_identifier",
		"property_identifier",
		"identifier",
		"type_identifier",
	)
}

func appendHeritageForNode(n *Node, lang *Language, source []byte, refs *[]HeritageRef) {
	span, ok := definitionSpanForNode(n, lang, source)
	if !ok {
		return
	}
	appendHeritageForNodeWithSpan(span, n, lang, source, refs)
}

func appendHeritageForNodeWithSpan(span DefinitionSpan, n *Node, lang *Language, source []byte, refs *[]HeritageRef) {
	switch lang.Name {
	case "java":
		appendJavaHeritage(span, n, source, refs)
	case "python":
		appendPythonHeritage(span, n, source, refs)
	case "javascript", "typescript", "tsx":
		appendJavaScriptHeritage(span, n, source, refs)
	}
}

func appendJavaHeritage(span DefinitionSpan, n *Node, source []byte, refs *[]HeritageRef) {
	if span.Kind != "class" && span.Kind != "interface" && span.Kind != "record" {
		return
	}
	header := declarationHeader(n.Text(source))
	appendDelimitedHeritage(span, header, "extends", "extends", []string{"implements", "permits"}, n.StartByte(), refs)
	appendDelimitedHeritage(span, header, "implements", "implements", []string{"permits"}, n.StartByte(), refs)
}

func appendPythonHeritage(span DefinitionSpan, n *Node, source []byte, refs *[]HeritageRef) {
	if span.Kind != "class" {
		return
	}
	header := declarationHeader(n.Text(source))
	open := strings.IndexByte(header, '(')
	if open < 0 {
		return
	}
	close := findClosingParen(header, open)
	if close < 0 {
		return
	}
	for _, parent := range splitImportList(header[open+1 : close]) {
		parent = strings.TrimSpace(parent)
		if parent == "" || strings.Contains(parent, "=") {
			continue
		}
		appendHeritageRef(span, "base", parent, header, n.StartByte(), refs)
	}
}

func appendJavaScriptHeritage(span DefinitionSpan, n *Node, source []byte, refs *[]HeritageRef) {
	if span.Kind != "class" {
		return
	}
	header := declarationHeader(n.Text(source))
	appendDelimitedHeritage(span, header, "extends", "extends", nil, n.StartByte(), refs)
}

func appendDelimitedHeritage(span DefinitionSpan, header, keyword, kind string, stopKeywords []string, baseByte uint32, refs *[]HeritageRef) {
	keywordAt := keywordIndex(header, keyword)
	if keywordAt < 0 {
		return
	}
	start := keywordAt + len(keyword)
	end := len(header)
	for _, stop := range stopKeywords {
		if stopAt := keywordIndex(header[start:], stop); stopAt >= 0 {
			end = start + stopAt
			break
		}
	}
	for _, parent := range splitImportList(header[start:end]) {
		parent = strings.TrimSpace(parent)
		if parent == "" {
			continue
		}
		appendHeritageRef(span, kind, parent, header, baseByte, refs)
	}
}

func appendHeritageRef(span DefinitionSpan, kind, parent, header string, baseByte uint32, refs *[]HeritageRef) {
	parent = cleanHeritageParent(parent)
	if parent == "" {
		return
	}
	parentStart := baseByte
	if idx := strings.Index(header, parent); idx >= 0 {
		parentStart = baseByte + uint32(idx)
	}
	*refs = append(*refs, HeritageRef{
		Lang:            span.Lang,
		Kind:            kind,
		Name:            span.Name,
		Parent:          parent,
		NodeType:        span.NodeType,
		StartByte:       span.StartByte,
		EndByte:         span.EndByte,
		ParentStartByte: parentStart,
		ParentEndByte:   parentStart + uint32(len(parent)),
	})
}

func declarationHeader(text string) string {
	if idx := strings.IndexByte(text, '{'); idx >= 0 {
		text = text[:idx]
	}
	if idx := strings.IndexByte(text, ':'); idx >= 0 {
		text = text[:idx+1]
	}
	return strings.TrimSpace(text)
}

func cleanHeritageParent(parent string) string {
	parent = strings.TrimSpace(parent)
	parent = strings.TrimSuffix(parent, ":")
	parent = strings.TrimSpace(parent)
	if parent == "" {
		return ""
	}
	if idx := strings.Index(parent, " "); idx >= 0 {
		parent = parent[:idx]
	}
	return strings.TrimSpace(parent)
}

func keywordIndex(text, keyword string) int {
	for searchFrom := 0; searchFrom < len(text); {
		idx := strings.Index(text[searchFrom:], keyword)
		if idx < 0 {
			return -1
		}
		idx += searchFrom
		beforeOK := idx == 0 || !isIdentByte(text[idx-1])
		after := idx + len(keyword)
		afterOK := after == len(text) || !isIdentByte(text[after])
		if beforeOK && afterOK {
			return idx
		}
		searchFrom = after
	}
	return -1
}

func lastDescendantByType(n *Node, lang *Language, types ...string) *Node {
	var found *Node
	var walk func(*Node)
	walk = func(cur *Node) {
		if cur == nil {
			return
		}
		typ := cur.Type(lang)
		for _, want := range types {
			if typ == want {
				found = cur
				break
			}
		}
		for i := 0; i < cur.ChildCount(); i++ {
			walk(cur.Child(i))
		}
	}
	walk(n)
	return found
}
