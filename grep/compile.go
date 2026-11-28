package grep

import (
	"fmt"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// CompiledPattern holds a compiled tree-sitter query along with metavariable
// metadata from the original code pattern.
type CompiledPattern struct {
	// Query is the compiled tree-sitter S-expression query ready for
	// matching against syntax trees.
	Query *gotreesitter.Query

	// MetaVars maps placeholder identifier to its MetaVar descriptor.
	// The keys are the same as in the Preprocess output.
	MetaVars map[string]*MetaVar

	// Lang is the language the pattern was compiled for.
	Lang *gotreesitter.Language

	// SExpr is the generated S-expression query string (useful for debugging).
	SExpr string
}

// CompilePattern compiles a code pattern string into a tree-sitter query for
// the given language. The pattern may contain metavariables ($NAME, $$$NAME,
// $_, $NAME:type) that are converted into query captures.
//
// The compilation pipeline:
//  1. Preprocess: replace metavariables with language-valid placeholders
//  2. Parse: parse the preprocessed string using the language's grammar
//  3. Translate: walk the parse tree and emit an S-expression query
func CompilePattern(lang *gotreesitter.Language, pattern string) (*CompiledPattern, error) {
	if lang == nil {
		return nil, fmt.Errorf("compile: nil language")
	}
	if strings.TrimSpace(pattern) == "" {
		return nil, fmt.Errorf("compile: empty pattern")
	}

	// Stage 1: Preprocess metavariables into placeholders.
	preprocessed, mvars, err := Preprocess(pattern)
	if err != nil {
		return nil, fmt.Errorf("compile: preprocess: %w", err)
	}

	// Build reverse lookup: placeholder text → *MetaVar.
	phLookup := make(map[string]*MetaVar, len(mvars))
	for _, mv := range mvars {
		phLookup[mv.Placeholder] = mv
	}

	// Stage 2: Parse the preprocessed pattern using the target language
	// grammar, inside the grammar's snippet context when it has one and the
	// pattern parses there without an error.
	var (
		interesting *gotreesitter.Node
		src         []byte
	)
	if ctx := snippetContextFor(lang); ctx != "" {
		at := strings.Index(ctx, grammars.SnippetPlaceholder)
		full := []byte(ctx[:at] + preprocessed + ctx[at+len(grammars.SnippetPlaceholder):])
		if ctxTree, err := parseSnippet(lang, full); err == nil && ctxTree != nil {
			ctxBound := gotreesitter.Bind(ctxTree)
			defer ctxBound.Release()
			if n := findInRegion(ctxBound.RootNode(), uint32(at), uint32(at+len(preprocessed)), lang); n != nil && !n.HasError() {
				interesting, src = n, full
			}
		}
	}
	if interesting == nil {
		tree, err := parseSnippet(lang, []byte(preprocessed))
		if err != nil {
			return nil, fmt.Errorf("compile: parse: %w", err)
		}
		bt := gotreesitter.Bind(tree)
		defer bt.Release()

		root := bt.RootNode()
		if root == nil {
			return nil, fmt.Errorf("compile: parse produced nil tree")
		}

		// Stage 3: Find the interesting subtree and translate to S-expression.
		interesting = findInterestingNode(root, lang)
		src = []byte(preprocessed)
	}
	if interesting == nil {
		return nil, fmt.Errorf("compile: no meaningful syntax found in pattern")
	}

	tc := &treeCompiler{
		lang:     lang,
		source:   src,
		mvars:    phLookup,
		literals: make(map[string]string),
	}

	sexpr := tc.compile(interesting)
	if sexpr == "" {
		return nil, fmt.Errorf("compile: failed to generate query from parse tree")
	}

	// Append #eq? predicates for literal text matches.
	predicates := tc.predicateString()
	if predicates != "" {
		sexpr = sexpr + "\n" + predicates
	}

	// Compile the S-expression into a tree-sitter Query.
	query, err := gotreesitter.NewQuery(sexpr, lang)
	if err != nil {
		return nil, fmt.Errorf("compile: query compile: %w (sexpr: %s)", err, sexpr)
	}

	return &CompiledPattern{
		Query:    query,
		MetaVars: mvars,
		Lang:     lang,
		SExpr:    sexpr,
	}, nil
}

// CompilePatternForLang is a convenience that looks up a language by name
// before compiling.
func CompilePatternForLang(langName, pattern string) (*CompiledPattern, error) {
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil {
		return nil, fmt.Errorf("compile: unknown language %q", langName)
	}
	return CompilePattern(entry.Language(), pattern)
}

// parseSnippet parses a code snippet using the correct method for the
// language, including token source factories where needed.
func parseSnippet(lang *gotreesitter.Language, source []byte) (*gotreesitter.Tree, error) {
	// Find the LangEntry to check for a TokenSourceFactory.
	var entry *grammars.LangEntry
	for _, e := range grammars.AllLanguages() {
		if e.Language() == lang {
			entry = &e
			break
		}
	}

	parser := gotreesitter.NewParser(lang)

	if entry != nil && entry.TokenSourceFactory != nil {
		ts := entry.TokenSourceFactory(source, lang)
		return parser.ParseWithTokenSource(source, ts)
	}
	return parser.Parse(source)
}

// findInterestingNode skips the top-level source_file (or equivalent) wrapper
// and returns the first meaningful child node. If the root has exactly one
// named child that isn't just wrapping structure, we descend into it.
func findInterestingNode(root *gotreesitter.Node, lang *gotreesitter.Language) *gotreesitter.Node {
	if root == nil {
		return nil
	}
	// If root has exactly one named child, descend (skip source_file wrapper).
	if root.NamedChildCount() == 1 {
		child := root.NamedChild(0)
		// Unwrap trivial statement wrappers like expression_statement.
		return unwrapTrivial(child, lang)
	}
	// Multiple top-level nodes: return root itself.
	return root
}

// unwrapTrivial descends through nodes that are trivial wrappers (containing
// exactly one named child) until we reach a non-trivial node. This handles
// wrapper nodes like expression_statement that tree-sitter inserts for
// statement-level expressions.
func unwrapTrivial(n *gotreesitter.Node, lang *gotreesitter.Language) *gotreesitter.Node {
	for n != nil && isTrivialWrapper(n, lang) {
		n = n.NamedChild(0)
	}
	return n
}

// trivialWrapperTypes are node types that tree-sitter grammars commonly insert
// as statement-level wrappers around expressions. When a user writes a pattern
// like "$X + $Y", tree-sitter wraps it in expression_statement, but the user
// intends to match the binary expression anywhere, not only as a statement.
var trivialWrapperTypes = map[string]bool{
	"expression_statement": true,
}

// isTrivialWrapper returns true if the node is a wrapper that contains
// exactly one named child and is of a known trivial wrapper type.
func isTrivialWrapper(n *gotreesitter.Node, lang *gotreesitter.Language) bool {
	if n == nil || n.NamedChildCount() != 1 {
		return false
	}
	return trivialWrapperTypes[n.Type(lang)]
}

// treeCompiler walks a parse tree and generates an S-expression query string.
type treeCompiler struct {
	lang   *gotreesitter.Language
	source []byte
	mvars  map[string]*MetaVar // placeholder text → MetaVar

	// literals tracks capture names for literal leaf nodes that need
	// #eq? predicates. Maps capture name → literal text.
	literals map[string]string

	litSeq int // counter for generating unique literal capture names
}

// compile generates the S-expression for a node and its subtree.
func (tc *treeCompiler) compile(n *gotreesitter.Node) string {
	if n == nil {
		return ""
	}

	text := n.Text(tc.source)

	// Check if this node's text is a placeholder.
	if mv, ok := tc.mvars[text]; ok {
		return tc.placeholderExpr(n, mv)
	}

	// Not a placeholder. Build a structured pattern.
	if !n.IsNamed() {
		// Anonymous nodes (keywords, punctuation) are structural — skip in query.
		return ""
	}

	nodeType := n.Type(tc.lang)

	// Leaf named node with literal text.
	if n.NamedChildCount() == 0 {
		return tc.literalLeafExpr(n, nodeType, text)
	}

	// Branch node: recurse into children.
	return tc.branchExpr(n, nodeType)
}

// placeholderExpr generates the S-expression for a node that directly IS a
// placeholder.
func (tc *treeCompiler) placeholderExpr(n *gotreesitter.Node, mv *MetaVar) string {
	switch {
	case mv.Wildcard:
		// $_ → (_) — wildcard, no capture
		return "(_)"
	case mv.TypeConstraint != "":
		// $NAME:type → (type) @NAME
		return fmt.Sprintf("(%s) @%s", mv.TypeConstraint, mv.Name)
	case mv.Variadic:
		// $$$NAME → (_) @NAME — capture at this level
		return fmt.Sprintf("(_) @%s", mv.Name)
	default:
		// $NAME → (_) @NAME
		return fmt.Sprintf("(_) @%s", mv.Name)
	}
}

// bubbledPlaceholderExpr generates the S-expression when a node's entire
// subtree contains only a single placeholder. Instead of matching the node's
// structure exactly, we match it as a wildcard/capture at this level.
func (tc *treeCompiler) bubbledPlaceholderExpr(n *gotreesitter.Node, mv *MetaVar) string {
	switch {
	case mv.Wildcard:
		return "(_)"
	case mv.TypeConstraint != "":
		return fmt.Sprintf("(%s) @%s", mv.TypeConstraint, mv.Name)
	case mv.Variadic:
		// For variadic captures that have been "wrapped" by grammar structure,
		// capture the wrapping node as a wildcard. This lets $$$PARAMS match
		// the entire parameter_list when tree-sitter wraps the placeholder
		// inside parameter_declaration nodes.
		return fmt.Sprintf("(_) @%s", mv.Name)
	default:
		return fmt.Sprintf("(_) @%s", mv.Name)
	}
}

// findSolePlaceholder checks if a node's subtree contains exactly one
// placeholder and no other semantic content. Returns the placeholder text
// and MetaVar if found.
func (tc *treeCompiler) findSolePlaceholder(n *gotreesitter.Node) (string, *MetaVar) {
	if n == nil || !n.IsNamed() {
		return "", nil
	}

	// Leaf node: check text.
	if n.ChildCount() == 0 {
		text := n.Text(tc.source)
		if mv, ok := tc.mvars[text]; ok {
			return text, mv
		}
		return "", nil
	}

	// Branch node: check if there's exactly one named child subtree
	// that contains a placeholder, and all other content is anonymous.
	var foundPh string
	var foundMV *MetaVar
	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		if !child.IsNamed() {
			continue
		}
		ph, mv := tc.findSolePlaceholder(child)
		if mv == nil {
			// Named child has no placeholder → not a sole placeholder subtree.
			return "", nil
		}
		if foundMV != nil {
			// Multiple placeholders in different branches → not "sole".
			return "", nil
		}
		foundPh = ph
		foundMV = mv
	}
	return foundPh, foundMV
}

// literalLeafExpr generates the S-expression for a named leaf node with
// literal text. We match the node type and add an #eq? predicate if the
// text is specific enough to warrant it.
func (tc *treeCompiler) literalLeafExpr(n *gotreesitter.Node, nodeType, text string) string {
	// For named leaf nodes (like identifiers, type_identifiers, etc.),
	// we want to match both the type AND the specific text.
	tc.litSeq++
	capName := fmt.Sprintf("_lit_%d", tc.litSeq)
	tc.literals[capName] = text
	return fmt.Sprintf("(%s) @%s", nodeType, capName)
}

// branchExpr generates the S-expression for a branch node by recursing
// into its children with field annotations.
func (tc *treeCompiler) branchExpr(n *gotreesitter.Node, nodeType string) string {
	var parts []string

	for i := 0; i < n.ChildCount(); i++ {
		child := n.Child(i)
		fieldName := n.FieldNameForChild(i, tc.lang)

		if !child.IsNamed() {
			// Anonymous nodes (keywords, operators, punctuation) are included
			// in the query only when they have a field annotation, indicating
			// they carry structural significance (e.g., operator: "+").
			if fieldName != "" {
				text := child.Text(tc.source)
				parts = append(parts, fmt.Sprintf("%s: %q", fieldName, text))
			}
			continue
		}

		// Before recursing into the child normally, check if the child's
		// entire subtree contains only a single placeholder. This handles
		// cases like $$$PARAMS being wrapped in parameter_declaration inside
		// a parameter_list — we bubble the placeholder up to this child
		// position rather than generating intermediate structure.
		var childExpr string
		if _, mv := tc.findSolePlaceholder(child); mv != nil {
			childExpr = tc.bubbledPlaceholderExpr(child, mv)
		} else {
			childExpr = tc.compile(child)
		}
		if childExpr == "" {
			continue
		}

		if fieldName != "" {
			parts = append(parts, fmt.Sprintf("%s: %s", fieldName, childExpr))
		} else {
			parts = append(parts, childExpr)
		}
	}

	if len(parts) == 0 {
		return fmt.Sprintf("(%s)", nodeType)
	}
	return fmt.Sprintf("(%s %s)", nodeType, strings.Join(parts, " "))
}

// predicateString generates the #eq? predicates for literal text matches.
func (tc *treeCompiler) predicateString() string {
	if len(tc.literals) == 0 {
		return ""
	}

	// Sort for deterministic output.
	var preds []string
	for capName, text := range tc.literals {
		// Escape the text for S-expression string literals.
		escaped := strings.ReplaceAll(text, `\`, `\\`)
		escaped = strings.ReplaceAll(escaped, `"`, `\"`)
		preds = append(preds, fmt.Sprintf(`(#eq? @%s "%s")`, capName, escaped))
	}

	// Sort for deterministic output (literal capture names are _lit_1, _lit_2, etc.)
	// They're already generated in order, but the map iteration is random.
	// Use a simple sort.
	sortStrings(preds)
	return strings.Join(preds, "\n")
}

// sortStrings sorts a slice of strings in place.
func sortStrings(ss []string) {
	for i := 1; i < len(ss); i++ {
		for j := i; j > 0 && ss[j] < ss[j-1]; j-- {
			ss[j], ss[j-1] = ss[j-1], ss[j]
		}
	}
}

// snippetContextFor returns the snippet context of the registered language
// that lang is, or "".
func snippetContextFor(lang *gotreesitter.Language) string {
	for _, e := range grammars.AllLanguages() {
		if e.Language() == lang {
			return grammars.SnippetContext(e.Name)
		}
	}
	return ""
}

// findInRegion returns the node that stands for the snippet that sits at bytes
// [start, end) of a parsed snippet context: the smallest node that encloses the
// region, or, when it holds exactly one named child inside the region, that
// child, with trivial statement wrappers removed. It returns nil when no node
// lies inside the region.
func findInRegion(root *gotreesitter.Node, start, end uint32, lang *gotreesitter.Language) *gotreesitter.Node {
	n := root
	for n != nil {
		var next *gotreesitter.Node
		for i := 0; i < n.NamedChildCount(); i++ {
			c := n.NamedChild(i)
			if c.StartByte() <= start && end <= c.EndByte() {
				next = c
				break
			}
		}
		if next == nil {
			break
		}
		n = next
	}
	if n == nil {
		return nil
	}
	if n.StartByte() >= start && n.EndByte() <= end {
		return unwrapTrivial(n, lang)
	}
	var inside []*gotreesitter.Node
	for i := 0; i < n.NamedChildCount(); i++ {
		c := n.NamedChild(i)
		if c.StartByte() >= start && c.EndByte() <= end {
			inside = append(inside, c)
		}
	}
	switch len(inside) {
	case 0:
		return nil
	case 1:
		return unwrapTrivial(inside[0], lang)
	}
	return n
}
