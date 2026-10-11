package gotreesitter

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter/internal/declarationfacts"
	"github.com/odvcencio/gotreesitter/internal/syntaxfacts"
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

	// FactDeclarations selects grammar-owned declaration facts and type shapes.
	// It requires WithDeclarationRules and emits into FactSet.Declarations.
	// Combine it with FactDefinitions to retain ordinary function/method facts.
	FactDeclarations

	// FactSignatures selects grammar-owned function and method signatures.
	// It requires WithSignatureRules and excludes function literals/types.
	FactSignatures
	// FactCallArguments selects direct call arguments, independently of FactCalls.
	// It requires WithCallArgumentRules.
	FactCallArguments

	// FactAll selects the original fact kinds. FactDeclarations is opt-in and
	// intentionally excluded to preserve existing results.
	FactAll = FactDefinitions | FactCalls | FactHeritage | FactImports
)

// FactSet contains the language-neutral facts emitted by a FactProgram.
type FactSet struct {
	Definitions []DefinitionSpan
	Calls       []CallRef
	Heritage    []HeritageRef
	Imports     []ImportRef
	// Declarations is opt-in and leaves Definitions unchanged.
	Declarations  []DeclarationFact  `json:",omitempty"`
	Signatures    []SignatureFact    `json:",omitempty"`
	CallArguments []CallArgumentFact `json:",omitempty"`
}

// FactProgram is a compiled, reusable syntax-fact extractor.
//
// Each grammar symbol indexes one packed instruction. Extraction executes the
// ordinary selected operations during one tree traversal. A program only
// accepts trees built with the Language value supplied to NewFactProgram.
// FactDeclarations is opt-in: attach grammar-owned rows with
// WithDeclarationRules to populate Declarations in a separate internal pass.
// Definitions stays unchanged even when both bits are selected. FactAll
// retains the original selection. Signatures and call arguments likewise require
// their bits and grammar-owned options. Disabled kinds add no traversal.
type FactProgram struct {
	language      *Language
	kinds         FactKind
	code          []factInstruction
	fields        factProgramFields
	importer      factImporter
	hasOperations bool
	declarations  *declarationfacts.Program[*Node]
	signatures    *syntaxfacts.Signatures[*Node]
	arguments     *syntaxfacts.Arguments[*Node]
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

// NewFactProgram compiles a reusable extractor with the original constructor
// signature. FactAll retains the original outputs. Use
// NewFactProgramWithOptions to attach opt-in grammar-owned declaration rules.
func NewFactProgram(lang *Language, kinds FactKind) (*FactProgram, error) {
	return NewFactProgramWithOptions(lang, kinds)
}

// NewFactProgramWithOptions compiles selected facts and applies opts in order.
// FactDeclarations emits grammar-owned facts into FactSet.Declarations without
// changing Definitions. Attach rows with WithDeclarationRules; unsupported
// grammar data is dropped. Without the bit, no declaration program is compiled.
func NewFactProgramWithOptions(lang *Language, kinds FactKind, opts ...FactProgramOption) (*FactProgram, error) {
	if lang == nil {
		return nil, fmt.Errorf("fact program: language is nil")
	}
	if unknown := kinds &^ (FactAll | FactDeclarations | FactSignatures | FactCallArguments); unknown != 0 {
		return nil, fmt.Errorf("fact program: unknown fact-kind bits 0x%x", uint8(unknown))
	}

	program := &FactProgram{
		language: lang,
		kinds:    kinds,
		code:     make([]factInstruction, len(lang.SymbolNames)),
		importer: factImporterForLanguage(lang.Name, kinds),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(program)
		}
	}
	program.compileFields()
	program.compileInstructions()
	return program, nil
}

// DeclarationRule describes one declaration shape using grammar data only.
// Names must be direct children in NameField with exactly NameNodeType.
// Every nonblank name emits a sibling span covering the whole NodeType node.
// A rule never searches descendants for a guessed name. BaseName instead follows
// TypeNames through a direct BaseNameField (or one named child), emitting an
// embedding after stripping type wrappers. ExcludeNames omits constraint terms.
//
// Ancestors lists an exact parent chain, nearest first. ContainerNameField
// and ContainerNameNodeType select a single name on its last ancestor;
// ContainerTypeField must link that ancestor to the preceding ancestor.
// ContainerPath optionally replaces this fixed ownership with exact recursive
// paths, producing dotted names and retaining the direct owner's range.
// Without either container selection the rule has no container.
//
// TypeField/TypeNodeType optionally constrain the declaration's direct type
// child. Shape is set only on Kind "type". For several matching rules on one
// node, the first applicable rule wins. Missing grammar data fails closed.
type DeclarationRule = declarationfacts.Rule

// FactProgramOption configures a FactProgram.
type FactProgramOption func(*FactProgram)

// WithDeclarationRules attaches grammar-owned declaration rules. A later
// option replaces earlier rules. The option has no effect unless
// FactDeclarations is selected; ordinary programs compile no extra operations
// and allocate no rule storage. NewFactProgramWithOptions owns the compiled data.
func WithDeclarationRules(rules []DeclarationRule) FactProgramOption {
	return func(p *FactProgram) {
		if p.kinds&FactDeclarations == 0 {
			return
		}
		p.declarations = declarationfacts.Compile(p.factGrammar(), p.factReader(), rules)
	}
}

// Kinds returns the outputs selected when the program was compiled.
func (p *FactProgram) Kinds() FactKind {
	if p == nil {
		return 0
	}
	return p.kinds
}

// Extract emits ordinary facts in one traversal and, when enabled, declaration
// facts in a separate internal pass. It returns an empty set for a nil tree or
// a tree built with a different Language value.
func (p *FactProgram) Extract(tree *Tree) FactSet {
	if p == nil || tree == nil || tree.Language() != p.language || (!p.hasOperations && p.declarations == nil && p.signatures == nil && p.arguments == nil) {
		return FactSet{}
	}
	root := tree.RootNode()
	if root == nil {
		return FactSet{}
	}

	var facts FactSet
	if p.hasOperations {
		p.extractNode(root, tree.Source(), p.importer != factImporterNone, &facts)
	}
	if p.declarations != nil {
		p.declarations.Extract(root, tree.Source(), &facts.Declarations)
	}
	if p.signatures != nil {
		p.signatures.Extract(root, tree.Source(), &facts.Signatures)
	}
	if p.arguments != nil {
		p.arguments.Extract(root, tree.Source(), p.acceptArgumentCall, &facts.CallArguments)
	}
	return facts
}

// ExtractInto replaces dst with the selected facts and reuses its slice storage.
// It clears previous entries, including facts from kinds that this program excludes.
// A nil program, invalid tree, or language mismatch leaves dst empty with its capacity retained.
// A nil dst has no effect.
//
// Results share storage with dst. Clone the result slices before the next extraction to retain them.
// Use a separate destination for each concurrent extraction.
// Signature parameter slices are freshly allocated; only the outer Signatures
// slice is reused. Clearing a signature releases its nested slices and strings.
// Assign FactSet{} to *dst when its retained storage is no longer needed.
func (p *FactProgram) ExtractInto(tree *Tree, dst *FactSet) {
	if dst == nil {
		return
	}
	clear(dst.Signatures)
	clear(dst.CallArguments)
	clear(dst.Declarations)
	clear(dst.Definitions)
	clear(dst.Calls)
	clear(dst.Heritage)
	clear(dst.Imports)
	dst.Signatures = dst.Signatures[:0]
	dst.CallArguments = dst.CallArguments[:0]
	dst.Declarations = dst.Declarations[:0]
	dst.Definitions = dst.Definitions[:0]
	dst.Calls = dst.Calls[:0]
	dst.Heritage = dst.Heritage[:0]
	dst.Imports = dst.Imports[:0]

	if p == nil || tree == nil || tree.Language() != p.language || (!p.hasOperations && p.declarations == nil && p.signatures == nil && p.arguments == nil) {
		return
	}
	if p.hasOperations {
		p.extractNode(tree.RootNode(), tree.Source(), p.importer != factImporterNone, dst)
	}
	if p.declarations != nil {
		p.declarations.Extract(tree.RootNode(), tree.Source(), &dst.Declarations)
	}
	if p.signatures != nil {
		p.signatures.Extract(tree.RootNode(), tree.Source(), &dst.Signatures)
	}
	if p.arguments != nil {
		p.arguments.Extract(tree.RootNode(), tree.Source(), p.acceptArgumentCall, &dst.CallArguments)
	}
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
	if p.kinds&(FactCalls|FactCallArguments) != 0 {
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
	if p.kinds&(FactCalls|FactCallArguments) != 0 {
		if p.kinds&FactCalls != 0 && isCallNode(p.language.Name, nodeType) {
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

// TypeNameRule selects a base type identifier through grammar-owned paths.
type TypeNameRule = declarationfacts.TypeNameRule

// ContainerRule describes exact, optionally recursive type-body ownership.
type ContainerRule = declarationfacts.ContainerRule

// SignatureRule describes a grammar-owned declared function or method.
type SignatureRule = syntaxfacts.SignatureRule

// ParameterRule describes one grouped or unnamed parameter declaration.
type ParameterRule = syntaxfacts.ParameterRule

// ExpressionKindRule maps a grammar node type to a normalized argument kind.
type ExpressionKindRule = syntaxfacts.ExpressionKindRule

// CallArgumentRule selects a call's direct argument list and expression kinds.
type CallArgumentRule = syntaxfacts.CallArgumentRule

// WithSignatureRules replaces signature rules when FactSignatures is selected.
// With the bit off, it retains no rows and compiles no instructions.
func WithSignatureRules(rules []SignatureRule) FactProgramOption {
	return func(p *FactProgram) {
		if p.kinds&FactSignatures != 0 {
			p.signatures = syntaxfacts.CompileSignatures(p.factGrammar(), p.factReader(), rules)
		}
	}
}

// WithCallArgumentRules replaces argument rules when FactCallArguments is selected.
// It emits the same call ranges as Calls even when FactCalls is off. With the bit
// off, it retains no rows and adds no extraction pass.
func WithCallArgumentRules(rules []CallArgumentRule) FactProgramOption {
	return func(p *FactProgram) {
		if p.kinds&FactCallArguments != 0 {
			p.arguments = syntaxfacts.CompileArguments(p.factGrammar(), p.factReader(), rules)
		}
	}
}

func (p *FactProgram) factGrammar() declarationfacts.Grammar {
	lang := p.language
	return declarationfacts.Grammar{Names: lang.SymbolNames,
		Symbol: func(name string) (uint16, bool) { s, ok := lang.SymbolByName(name); return uint16(s), ok },
		Field:  func(name string) (uint16, bool) { f, ok := lang.FieldByName(name); return uint16(f), ok }}
}

func (p *FactProgram) factReader() declarationfacts.Reader[*Node] {
	lang := p.language
	return declarationfacts.Reader[*Node]{Language: lang.Name,
		Symbol: func(n *Node) uint16 { return uint16(n.Symbol()) }, Parent: (*Node).Parent,
		ChildCount: nodeChildCountNoMaterialize, Child: func(n *Node, i int) *Node { return nodeChildAtForReason(n, i, materializeForParentAPI) },
		Field: func(n *Node, i int) uint16 { return uint16(nodeFieldIDAt(n, i)) }, Missing: (*Node).IsMissing, Named: (*Node).IsNamed,
		Text: (*Node).Text, NodeType: func(n *Node) string { return n.Type(lang) }, StartByte: (*Node).StartByte, EndByte: (*Node).EndByte}
}

func (p *FactProgram) acceptArgumentCall(n *Node, source []byte) bool {
	if !isCallNode(p.language.Name, n.Type(p.language)) {
		return false
	}
	_, ok := p.callRef(n, source)
	return ok
}
