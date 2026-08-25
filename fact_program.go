package gotreesitter

import (
	"fmt"
	"strings"
)

// FactKind selects the outputs that a FactProgram emits.
type FactKind uint8

const (
	// FactDefinitions selects declaration spans.
	FactDefinitions FactKind = 1 << iota
	// FactCalls selects call-site references.
	FactCalls
	// FactHeritage selects inheritance and base-class references.
	FactHeritage
	// FactImports selects package and dependency declarations.
	FactImports

	// FactAll selects every supported fact kind.
	FactAll = FactDefinitions | FactCalls | FactHeritage | FactImports
)

// FactSet contains the language-neutral facts emitted by a FactProgram.
type FactSet struct {
	Definitions []DefinitionSpan
	Calls       []CallRef
	Heritage    []HeritageRef
	Imports     []ImportRef
}

// FactProgram is a compiled, reusable syntax-fact extractor.
//
// Each grammar symbol indexes one packed instruction. Extraction executes the
// selected operations during one tree traversal. A program only accepts trees
// built with the Language value supplied to NewFactProgram.
type FactProgram struct {
	language      *Language
	kinds         FactKind
	code          []factInstruction
	fields        factProgramFields
	importer      factImporter
	hasOperations bool
}

type factInstruction uint16

const factDefinitionKindMask factInstruction = 0x000f

const (
	factOpCall factInstruction = 1 << (4 + iota)
	factOpHeritage
	factOpImport
	factRoleDefinitionName
	factRoleCallName
	factRoleCallTargetSkip
)

const factOperationMask = factDefinitionKindMask | factOpCall | factOpHeritage | factOpImport

type factDefinitionKind uint16

const (
	factDefinitionNone factDefinitionKind = iota
	factDefinitionFunction
	factDefinitionMethod
	factDefinitionType
	factDefinitionClass
	factDefinitionInterface
	factDefinitionEnum
	factDefinitionRecord
	factDefinitionConstructor
)

type factImporter uint8

const (
	factImporterNone factImporter = iota
	factImporterGo
	factImporterJava
	factImporterPython
	factImporterStarlark
)

type factProgramFields struct {
	definitionName FieldID
	callTarget     [3]FieldID
	expressionName [4]FieldID
}

// NewFactProgram compiles a reusable extractor for lang and the selected
// kinds. Compilation resolves grammar symbols and field names once.
func NewFactProgram(lang *Language, kinds FactKind) (*FactProgram, error) {
	if lang == nil {
		return nil, fmt.Errorf("fact program: language is nil")
	}
	if unknown := kinds &^ FactAll; unknown != 0 {
		return nil, fmt.Errorf("fact program: unknown fact-kind bits 0x%x", uint8(unknown))
	}

	program := &FactProgram{
		language: lang,
		kinds:    kinds,
		code:     make([]factInstruction, len(lang.SymbolNames)),
		importer: factImporterForLanguage(lang.Name, kinds),
	}
	program.compileFields()
	program.compileInstructions()
	return program, nil
}

// Kinds returns the outputs selected when the program was compiled.
func (p *FactProgram) Kinds() FactKind {
	if p == nil {
		return 0
	}
	return p.kinds
}

// Extract emits the selected facts during one tree traversal. It returns an
// empty set for a nil tree or a tree built with a different Language value.
func (p *FactProgram) Extract(tree *Tree) FactSet {
	if p == nil || tree == nil || tree.Language() != p.language || !p.hasOperations {
		return FactSet{}
	}
	root := tree.RootNode()
	if root == nil {
		return FactSet{}
	}

	var facts FactSet
	p.extractNode(root, tree.Source(), p.importer != factImporterNone, &facts)
	return facts
}

// ExtractBound emits selected facts from a BoundTree.
// It uses the same guards and traversal as Extract.
func (p *FactProgram) ExtractBound(tree *BoundTree) FactSet {
	if tree == nil {
		return p.Extract(nil)
	}
	return p.Extract(tree.tree)
}

func (p *FactProgram) compileFields() {
	if p == nil || p.language == nil {
		return
	}
	if p.kinds&(FactDefinitions|FactHeritage) != 0 {
		p.fields.definitionName = factFieldByName(p.language, "name")
	}
	if p.kinds&FactCalls != 0 {
		p.fields.callTarget = [3]FieldID{
			factFieldByName(p.language, "function"),
			factFieldByName(p.language, "name"),
			factFieldByName(p.language, "constructor"),
		}
		p.fields.expressionName = [4]FieldID{
			factFieldByName(p.language, "name"),
			factFieldByName(p.language, "field"),
			factFieldByName(p.language, "attribute"),
			factFieldByName(p.language, "property"),
		}
	}
}

func factFieldByName(lang *Language, name string) FieldID {
	field, _ := lang.FieldByName(name)
	return field
}

func (p *FactProgram) compileInstructions() {
	for symbol, rawName := range p.language.SymbolNames {
		nodeType := unescapePunctuationSymbolName(rawName)
		instruction := p.compileInstruction(nodeType)
		p.code[symbol] = instruction
		if instruction&factOperationMask != 0 {
			p.hasOperations = true
		}
	}
}

func (p *FactProgram) compileInstruction(nodeType string) factInstruction {
	var instruction factInstruction
	if p.kinds&(FactDefinitions|FactHeritage) != 0 {
		kind := factDefinitionKindForName(definitionKind(p.language.Name, nodeType))
		instruction |= factInstruction(kind)
		if p.kinds&FactHeritage != 0 && factDefinitionHasHeritage(p.language.Name, kind) {
			instruction |= factOpHeritage
		}
		if factDefinitionNameNodeType(nodeType) {
			instruction |= factRoleDefinitionName
		}
	}
	if p.kinds&FactCalls != 0 {
		if isCallNode(p.language.Name, nodeType) {
			instruction |= factOpCall
		}
		if factCallNameNodeType(nodeType) {
			instruction |= factRoleCallName
		}
		if factCallTargetSkipNodeType(nodeType) {
			instruction |= factRoleCallTargetSkip
		}
	}
	if p.kinds&FactImports != 0 && factImportNodeType(p.importer, nodeType) {
		instruction |= factOpImport
	}
	return instruction
}

func factDefinitionKindForName(kind string) factDefinitionKind {
	switch kind {
	case "function":
		return factDefinitionFunction
	case "method":
		return factDefinitionMethod
	case "type":
		return factDefinitionType
	case "class":
		return factDefinitionClass
	case "interface":
		return factDefinitionInterface
	case "enum":
		return factDefinitionEnum
	case "record":
		return factDefinitionRecord
	case "constructor":
		return factDefinitionConstructor
	default:
		return factDefinitionNone
	}
}

func (k factDefinitionKind) String() string {
	switch k {
	case factDefinitionFunction:
		return "function"
	case factDefinitionMethod:
		return "method"
	case factDefinitionType:
		return "type"
	case factDefinitionClass:
		return "class"
	case factDefinitionInterface:
		return "interface"
	case factDefinitionEnum:
		return "enum"
	case factDefinitionRecord:
		return "record"
	case factDefinitionConstructor:
		return "constructor"
	default:
		return ""
	}
}

func factDefinitionHasHeritage(langName string, kind factDefinitionKind) bool {
	switch langName {
	case "java":
		return kind == factDefinitionClass || kind == factDefinitionInterface || kind == factDefinitionRecord
	case "python", "javascript", "typescript", "tsx":
		return kind == factDefinitionClass
	default:
		return false
	}
}

func factDefinitionNameNodeType(nodeType string) bool {
	switch nodeType {
	case "type_identifier", "identifier", "field_identifier", "property_identifier":
		return true
	default:
		return false
	}
}

func factCallNameNodeType(nodeType string) bool {
	switch nodeType {
	case "field_identifier", "property_identifier", "identifier", "type_identifier":
		return true
	default:
		return false
	}
}

func factCallTargetSkipNodeType(nodeType string) bool {
	switch nodeType {
	case "argument_list", "arguments", "type_arguments", "(", ")":
		return true
	default:
		return false
	}
}

func factImporterForLanguage(langName string, kinds FactKind) factImporter {
	if kinds&FactImports == 0 {
		return factImporterNone
	}
	switch langName {
	case "go":
		return factImporterGo
	case "java":
		return factImporterJava
	case "python":
		return factImporterPython
	case "starlark":
		return factImporterStarlark
	default:
		return factImporterNone
	}
}

func factImportNodeType(importer factImporter, nodeType string) bool {
	switch importer {
	case factImporterGo:
		return nodeType == "package_clause" || nodeType == "import_declaration"
	case factImporterJava:
		return nodeType == "package_declaration" || nodeType == "import_declaration"
	case factImporterPython:
		switch nodeType {
		case "import_statement", "import_from_statement", "future_import_statement":
			return true
		}
	case factImporterStarlark:
		return nodeType == "call"
	}
	return false
}

func (p *FactProgram) instruction(n *Node) factInstruction {
	if n == nil {
		return 0
	}
	symbol := int(n.Symbol())
	if symbol >= len(p.code) {
		return 0
	}
	return p.code[symbol]
}

func (p *FactProgram) extractNode(n *Node, source []byte, importsActive bool, facts *FactSet) {
	if n == nil {
		return
	}
	instruction := p.instruction(n)
	definitionKind := factDefinitionKind(instruction & factDefinitionKindMask)
	if definitionKind != factDefinitionNone {
		if span, ok := p.definitionSpan(n, definitionKind, source); ok {
			if p.kinds&FactDefinitions != 0 {
				facts.Definitions = append(facts.Definitions, span)
			}
			if instruction&factOpHeritage != 0 {
				appendHeritageForNodeWithSpan(span, n, p.language, source, &facts.Heritage)
			}
		}
	}
	if instruction&factOpCall != 0 {
		if ref, ok := p.callRef(n, source); ok {
			facts.Calls = append(facts.Calls, ref)
		}
	}

	descendImports := importsActive
	if importsActive && instruction&factOpImport != 0 {
		descendImports = p.extractImportNode(n, source, &facts.Imports)
	}
	if !descendImports && p.kinds&^FactImports == 0 {
		return
	}

	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		p.extractNode(nodeChildAtForReason(n, i, materializeForParentAPI), source, descendImports, facts)
	}
}

func (p *FactProgram) definitionSpan(n *Node, kind factDefinitionKind, source []byte) (DefinitionSpan, bool) {
	nameNode := factChildByField(n, p.fields.definitionName)
	if nameNode == nil {
		nameNode = p.firstDescendantWithRole(n, factRoleDefinitionName)
	}
	if nameNode == nil {
		return DefinitionSpan{}, false
	}
	name := strings.TrimSpace(nameNode.Text(source))
	if name == "" {
		return DefinitionSpan{}, false
	}
	return DefinitionSpan{
		Lang:          p.language.Name,
		Kind:          kind.String(),
		Name:          name,
		NodeType:      n.Type(p.language),
		StartByte:     n.StartByte(),
		EndByte:       n.EndByte(),
		NameStartByte: nameNode.StartByte(),
		NameEndByte:   nameNode.EndByte(),
	}, true
}

func (p *FactProgram) callRef(n *Node, source []byte) (CallRef, bool) {
	target := factChildByAnyField(n, p.fields.callTarget[:])
	if target == nil {
		childCount := nodeChildCountNoMaterialize(n)
		for i := 0; i < childCount; i++ {
			child := nodeChildAtForReason(n, i, materializeForParentAPI)
			if child == nil || p.instruction(child)&factRoleCallTargetSkip != 0 {
				continue
			}
			target = child
			break
		}
	}
	name, receiver, nameStart, nameEnd := p.expressionName(target, source)
	if name == "" {
		return CallRef{}, false
	}
	return CallRef{
		Lang:          p.language.Name,
		Kind:          "call",
		Name:          name,
		Receiver:      receiver,
		NodeType:      n.Type(p.language),
		StartByte:     n.StartByte(),
		EndByte:       n.EndByte(),
		NameStartByte: nameStart,
		NameEndByte:   nameEnd,
	}, true
}

func (p *FactProgram) expressionName(n *Node, source []byte) (name, receiver string, nameStart, nameEnd uint32) {
	if n == nil {
		return "", "", 0, 0
	}
	nameNode := factChildByAnyField(n, p.fields.expressionName[:])
	if nameNode == nil {
		nameNode = p.lastDescendantWithRole(n, factRoleCallName)
	}
	if nameNode != nil {
		name = strings.TrimSpace(nameNode.Text(source))
		nameStart = nameNode.StartByte()
		nameEnd = nameNode.EndByte()
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
	if index := strings.LastIndex(text, name); index >= 0 {
		nameStart = n.StartByte() + uint32(index)
		nameEnd = nameStart + uint32(len(name))
		receiver = strings.TrimRight(strings.TrimSpace(text[:index]), ".")
	}
	return name, receiver, nameStart, nameEnd
}

func factChildByAnyField(n *Node, fields []FieldID) *Node {
	for _, field := range fields {
		if child := factChildByField(n, field); child != nil {
			return child
		}
	}
	return nil
}

func factChildByField(n *Node, field FieldID) *Node {
	if n == nil || field == 0 {
		return nil
	}
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		if nodeFieldIDAt(n, i) == field {
			return nodeChildAtForReason(n, i, materializeForParentAPI)
		}
	}
	return nil
}

func (p *FactProgram) firstDescendantWithRole(n *Node, role factInstruction) *Node {
	if n == nil {
		return nil
	}
	if p.instruction(n)&role != 0 {
		return n
	}
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		if found := p.firstDescendantWithRole(nodeChildAtForReason(n, i, materializeForParentAPI), role); found != nil {
			return found
		}
	}
	return nil
}

func (p *FactProgram) lastDescendantWithRole(n *Node, role factInstruction) *Node {
	if n == nil {
		return nil
	}
	var found *Node
	if p.instruction(n)&role != 0 {
		found = n
	}
	childCount := nodeChildCountNoMaterialize(n)
	for i := 0; i < childCount; i++ {
		if childFound := p.lastDescendantWithRole(nodeChildAtForReason(n, i, materializeForParentAPI), role); childFound != nil {
			found = childFound
		}
	}
	return found
}

func (p *FactProgram) extractImportNode(n *Node, source []byte, refs *[]ImportRef) bool {
	switch p.importer {
	case factImporterGo:
		return extractGoImportNode(n, p.language, source, refs)
	case factImporterJava:
		return extractJavaImportNode(n, p.language, source, refs)
	case factImporterPython:
		return extractPythonImportNode(n, p.language, source, refs)
	case factImporterStarlark:
		return extractStarlarkImportNode(n, p.language, source, refs)
	default:
		return true
	}
}
