package grammargen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// ImportGrammarJS parses a tree-sitter grammar.js file and returns a Grammar IR.
// This uses gotreesitter's own JavaScript grammar to parse the file, demonstrating
// the full-circle capability: gotreesitter parsing its own input format.
func ImportGrammarJS(source []byte) (*Grammar, error) {
	lang := grammars.JavascriptLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("parse grammar.js: %w", err)
	}

	root := tree.RootNode()
	imp := &jsImporter{
		source: source,
		lang:   lang,
	}

	return imp.extract(root)
}

type jsImporter struct {
	source         []byte
	lang           *gotreesitter.Language
	helperFuncs    map[string]*gotreesitter.Node // top-level function declarations (commaSep, etc.)
	paramSubst     map[string]*Rule              // active parameter substitutions for helper inlining
	localConsts    map[string]*gotreesitter.Node // local const declarations in current rule body
	topLevelConsts map[string]map[string]int     // top-level const objects: PREC.control → int
	namedPrecs     map[string]int                // grammar precedences: "end" → numeric value
}

// nodeText returns the source text of a node.
func (imp *jsImporter) nodeText(n *gotreesitter.Node) string {
	return string(imp.source[n.StartByte():n.EndByte()])
}

// nodeType returns the type name of a node.
func (imp *jsImporter) nodeType(n *gotreesitter.Node) string {
	return n.Type(imp.lang)
}

// extract walks the AST to find module.exports = grammar({...}) and extracts
// all grammar components.
func (imp *jsImporter) extract(root *gotreesitter.Node) (*Grammar, error) {
	// Collect top-level helper functions (commaSep, sep, etc.) before processing grammar.
	imp.collectHelperFunctions(root)
	// Collect top-level const objects (PREC = {...}) for member expression resolution.
	imp.collectTopLevelConsts(root)

	grammarObj, err := imp.findGrammarCall(root)
	if err != nil {
		return nil, err
	}
	imp.namedPrecs = imp.extractNamedPrecs(grammarObj)

	g := NewGrammar("")

	for i := 0; i < int(grammarObj.NamedChildCount()); i++ {
		child := grammarObj.NamedChild(i)
		if imp.nodeType(child) != "pair" {
			continue
		}

		key := imp.getPairKey(child)
		value := imp.getPairValue(child)

		switch key {
		case "name":
			g.Name = imp.extractStringValue(value)

		case "rules":
			if err := imp.extractRules(value, g); err != nil {
				return nil, fmt.Errorf("extract rules: %w", err)
			}

		case "extras":
			extras, err := imp.extractRuleArray(value)
			if err != nil {
				return nil, fmt.Errorf("extract extras: %w", err)
			}
			g.Extras = extras

		case "conflicts":
			conflicts, err := imp.extractConflicts(value)
			if err != nil {
				return nil, fmt.Errorf("extract conflicts: %w", err)
			}
			g.Conflicts = conflicts

		case "externals":
			externals, err := imp.extractExternals(value)
			if err != nil {
				return nil, fmt.Errorf("extract externals: %w", err)
			}
			g.Externals = externals

		case "inline":
			g.Inline = imp.extractStringArray(value)

		case "word":
			g.Word = imp.extractWordRef(value)

		case "supertypes":
			g.Supertypes = imp.extractStringArray(value)
		}
	}

	return g, nil
}

// extractNamedPrecs extracts grammar-level named precedence groups from
// precedences: $ => [["name1", "name2"], ...].
func (imp *jsImporter) extractNamedPrecs(grammarObj *gotreesitter.Node) map[string]int {
	if grammarObj == nil || imp.nodeType(grammarObj) != "object" {
		return nil
	}

	var levels [][]string
	for i := 0; i < int(grammarObj.NamedChildCount()); i++ {
		child := grammarObj.NamedChild(i)
		if imp.nodeType(child) != "pair" || imp.getPairKey(child) != "precedences" {
			continue
		}
		value := imp.getPairValue(child)
		body := imp.extractArrowBody(value)
		if body == nil {
			body = value
		}
		if body == nil || imp.nodeType(body) != "array" {
			return nil
		}
		for j := 0; j < int(body.NamedChildCount()); j++ {
			group := body.NamedChild(j)
			if imp.nodeType(group) != "array" {
				continue
			}
			var level []string
			for k := 0; k < int(group.NamedChildCount()); k++ {
				entry := group.NamedChild(k)
				switch imp.nodeType(entry) {
				case "string":
					level = append(level, imp.extractStringValue(entry))
				case "member_expression":
					level = append(level, imp.extractMemberProp(entry))
				}
			}
			if len(level) > 0 {
				levels = append(levels, level)
			}
		}
		break
	}
	if len(levels) == 0 {
		return nil
	}

	var ordered []string
	for _, level := range levels {
		ordered = append(ordered, level...)
	}

	m := make(map[string]int, len(ordered))
	total := len(ordered)
	for idx, name := range ordered {
		val := total - 1 - idx
		if existing, ok := m[name]; !ok || val > existing {
			m[name] = val
		}
	}
	return m
}

// findGrammarCall locates the grammar({...}) call expression and returns
// the object argument.
func (imp *jsImporter) findGrammarCall(root *gotreesitter.Node) (*gotreesitter.Node, error) {
	var result *gotreesitter.Node

	var walk func(n *gotreesitter.Node)
	walk = func(n *gotreesitter.Node) {
		if result != nil {
			return
		}

		if imp.nodeType(n) == "call_expression" {
			fn := n.ChildByFieldName("function", imp.lang)
			if fn != nil && imp.nodeText(fn) == "grammar" {
				args := n.ChildByFieldName("arguments", imp.lang)
				if args != nil && int(args.NamedChildCount()) > 0 {
					firstArg := args.NamedChild(0)
					if imp.nodeType(firstArg) == "object" {
						result = firstArg
						return
					}
				}
			}
		}

		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)

	if result == nil {
		return nil, fmt.Errorf("could not find grammar({...}) call in source")
	}
	return result, nil
}

// getPairKey extracts the key string from an object pair node.
func (imp *jsImporter) getPairKey(pair *gotreesitter.Node) string {
	key := pair.ChildByFieldName("key", imp.lang)
	if key == nil {
		return ""
	}
	text := imp.nodeText(key)
	text = strings.Trim(text, `"'`)
	return text
}

// getPairValue returns the value node of an object pair.
func (imp *jsImporter) getPairValue(pair *gotreesitter.Node) *gotreesitter.Node {
	return pair.ChildByFieldName("value", imp.lang)
}

// extractStringValue extracts a string value from a string literal node.
func (imp *jsImporter) extractStringValue(n *gotreesitter.Node) string {
	text := imp.nodeText(n)
	if len(text) >= 2 {
		if (text[0] == '"' && text[len(text)-1] == '"') ||
			(text[0] == '\'' && text[len(text)-1] == '\'') {
			return text[1 : len(text)-1]
		}
	}
	return text
}

// extractRules extracts the rules object: { rule_name: $ => rule_expr, ... }
func (imp *jsImporter) extractRules(rulesObj *gotreesitter.Node, g *Grammar) error {
	if imp.nodeType(rulesObj) != "object" {
		return fmt.Errorf("expected object for rules, got %s", imp.nodeType(rulesObj))
	}

	for i := 0; i < int(rulesObj.NamedChildCount()); i++ {
		child := rulesObj.NamedChild(i)
		if imp.nodeType(child) == "method_definition" {
			name := imp.getMethodName(child)
			body := imp.getMethodBody(child)
			if body == nil {
				continue
			}
			rule, err := imp.convertRuleExpr(body)
			if err != nil {
				return fmt.Errorf("rule %q: %w", name, err)
			}
			g.Define(name, rule)
			continue
		}
		if imp.nodeType(child) != "pair" {
			continue
		}

		name := imp.getPairKey(child)
		value := imp.getPairValue(child)

		ruleExpr := imp.extractArrowBody(value)
		if ruleExpr == nil {
			ruleExpr = value
		}

		rule, err := imp.convertRuleExpr(ruleExpr)
		imp.localConsts = nil // clear per-rule local consts
		if err != nil {
			return fmt.Errorf("rule %q: %w", name, err)
		}
		g.Define(name, rule)
	}

	return nil
}

// getMethodName extracts the name from a method definition.
func (imp *jsImporter) getMethodName(n *gotreesitter.Node) string {
	name := n.ChildByFieldName("name", imp.lang)
	if name == nil {
		return ""
	}
	return imp.nodeText(name)
}

// getMethodBody extracts the body expression from a method definition.
func (imp *jsImporter) getMethodBody(n *gotreesitter.Node) *gotreesitter.Node {
	body := n.ChildByFieldName("body", imp.lang)
	if body == nil {
		return nil
	}
	if imp.nodeType(body) == "statement_block" {
		for i := 0; i < int(body.NamedChildCount()); i++ {
			child := body.NamedChild(i)
			if imp.nodeType(child) == "return_statement" {
				if int(child.NamedChildCount()) > 0 {
					return child.NamedChild(0)
				}
			}
		}
	}
	return body
}

// extractArrowBody extracts the body expression from an arrow function.
// For block bodies (e.g. _ => { const x = ...; return expr; }), it collects
// local const declarations and returns the return expression.
func (imp *jsImporter) extractArrowBody(n *gotreesitter.Node) *gotreesitter.Node {
	if n == nil {
		return nil
	}
	if imp.nodeType(n) == "arrow_function" {
		body := n.ChildByFieldName("body", imp.lang)
		if body != nil && imp.nodeType(body) == "statement_block" {
			imp.collectLocalConsts(body)
			ret := imp.findReturnExpr(body)
			if ret != nil {
				return ret
			}
		}
		return body
	}
	return n
}

// convertRuleExpr converts a JavaScript AST expression into a Grammar Rule.
func (imp *jsImporter) convertRuleExpr(n *gotreesitter.Node) (*Rule, error) {
	if n == nil {
		return nil, fmt.Errorf("nil node")
	}

	typ := imp.nodeType(n)
	text := imp.nodeText(n)

	switch typ {
	case "call_expression":
		return imp.convertCallExpr(n)

	case "string":
		val := imp.extractStringValue(n)
		return Str(val), nil

	case "regex":
		pattern := imp.extractRegexPattern(n)
		return Pat(pattern), nil

	case "member_expression":
		return imp.convertMemberExpr(n)

	case "identifier":
		if text == "blank" {
			return Blank(), nil
		}
		// Check parameter substitution (helper function inlining).
		if imp.paramSubst != nil {
			if r, ok := imp.paramSubst[text]; ok {
				return r, nil
			}
		}
		// Check local const declarations (block arrow bodies).
		if imp.localConsts != nil {
			if initNode, ok := imp.localConsts[text]; ok {
				return imp.convertRuleExpr(initNode)
			}
		}
		return Sym(text), nil

	case "template_string":
		inner := text
		if len(inner) >= 2 {
			inner = inner[1 : len(inner)-1]
		}
		return Pat(inner), nil

	case "parenthesized_expression":
		if int(n.NamedChildCount()) > 0 {
			return imp.convertRuleExpr(n.NamedChild(0))
		}
		return nil, fmt.Errorf("empty parenthesized expression")

	default:
		return nil, fmt.Errorf("unsupported rule expression type %q: %s", typ, truncate(text, 80))
	}
}

// convertCallExpr converts a function call like seq(...), choice(...) etc.
func (imp *jsImporter) convertCallExpr(n *gotreesitter.Node) (*Rule, error) {
	fn := n.ChildByFieldName("function", imp.lang)
	args := n.ChildByFieldName("arguments", imp.lang)

	if fn == nil || args == nil {
		return nil, fmt.Errorf("malformed call expression")
	}

	fnText := imp.nodeText(fn)

	// Handle member calls like prec.left(...), token.immediate(...)
	if imp.nodeType(fn) == "member_expression" {
		obj := fn.ChildByFieldName("object", imp.lang)
		prop := fn.ChildByFieldName("property", imp.lang)
		if obj != nil && prop != nil {
			fnText = imp.nodeText(obj) + "." + imp.nodeText(prop)
		}
	}

	// Collect arguments, skipping comments.
	var argNodes []*gotreesitter.Node
	for i := 0; i < int(args.NamedChildCount()); i++ {
		child := args.NamedChild(i)
		if imp.nodeType(child) == "comment" {
			continue
		}
		argNodes = append(argNodes, child)
	}

	switch fnText {
	case "seq":
		children, err := imp.convertRuleArgs(argNodes)
		if err != nil {
			return nil, fmt.Errorf("seq: %w", err)
		}
		return Seq(children...), nil

	case "choice":
		children, err := imp.convertRuleArgs(argNodes)
		if err != nil {
			return nil, fmt.Errorf("choice: %w", err)
		}
		return Choice(children...), nil

	case "repeat":
		if len(argNodes) != 1 {
			return nil, fmt.Errorf("repeat expects 1 arg, got %d", len(argNodes))
		}
		child, err := imp.convertRuleExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return Repeat(child), nil

	case "repeat1":
		if len(argNodes) != 1 {
			return nil, fmt.Errorf("repeat1 expects 1 arg, got %d", len(argNodes))
		}
		child, err := imp.convertRuleExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return Repeat1(child), nil

	case "optional":
		if len(argNodes) != 1 {
			return nil, fmt.Errorf("optional expects 1 arg, got %d", len(argNodes))
		}
		child, err := imp.convertRuleExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return Optional(child), nil

	case "token":
		if len(argNodes) != 1 {
			return nil, fmt.Errorf("token expects 1 arg, got %d", len(argNodes))
		}
		child, err := imp.convertRuleExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return Token(child), nil

	case "token.immediate":
		if len(argNodes) != 1 {
			return nil, fmt.Errorf("token.immediate expects 1 arg, got %d", len(argNodes))
		}
		child, err := imp.convertRuleExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return ImmToken(child), nil

	case "field":
		if len(argNodes) != 2 {
			return nil, fmt.Errorf("field expects 2 args, got %d", len(argNodes))
		}
		name := imp.extractStringValue(argNodes[0])
		child, err := imp.convertRuleExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		return Field(name, child), nil

	case "prec":
		return imp.convertPrecCall(argNodes, func(n int, r *Rule) *Rule {
			return Prec(n, r)
		})

	case "prec.left":
		return imp.convertPrecCall(argNodes, func(n int, r *Rule) *Rule {
			return PrecLeft(n, r)
		})

	case "prec.right":
		return imp.convertPrecCall(argNodes, func(n int, r *Rule) *Rule {
			return PrecRight(n, r)
		})

	case "prec.dynamic":
		return imp.convertPrecCall(argNodes, func(n int, r *Rule) *Rule {
			return PrecDynamic(n, r)
		})

	case "alias":
		if len(argNodes) < 2 {
			return nil, fmt.Errorf("alias expects 2-3 args, got %d", len(argNodes))
		}
		child, err := imp.convertRuleExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		aliasTarget := argNodes[1]
		var aliasName string
		named := false
		if imp.nodeType(aliasTarget) == "string" {
			aliasName = imp.extractStringValue(aliasTarget)
		} else if imp.nodeType(aliasTarget) == "member_expression" {
			aliasName = imp.extractMemberProp(aliasTarget)
			named = true
		} else {
			aliasName = imp.nodeText(aliasTarget)
			named = true
		}
		return Alias(child, aliasName, named), nil

	default:
		// Try inlining a locally-defined helper function.
		if _, ok := imp.helperFuncs[fnText]; ok {
			return imp.inlineHelperCall(fnText, argNodes)
		}
		if rule, ok, err := imp.convertBuiltinHelperCall(fnText, argNodes); ok {
			return rule, err
		}
		return nil, fmt.Errorf("unsupported function call %q", fnText)
	}
}

func (imp *jsImporter) convertBuiltinHelperCall(fnText string, argNodes []*gotreesitter.Node) (*Rule, bool, error) {
	switch fnText {
	case "sep1":
		sep, rule, err := imp.extractBuiltinSeparatorAndRule(argNodes)
		if err != nil {
			return nil, true, err
		}
		return Seq(rule, Repeat(Seq(sep, rule))), true, nil
	case "sep":
		sep, rule, err := imp.extractBuiltinSeparatorAndRule(argNodes)
		if err != nil {
			return nil, true, err
		}
		return Optional(Seq(rule, Repeat(Seq(sep, rule)))), true, nil
	case "commaSep1":
		rule, err := imp.convertBuiltinSingleRuleArg(fnText, argNodes)
		if err != nil {
			return nil, true, err
		}
		return Seq(rule, Repeat(Seq(Str(","), rule))), true, nil
	case "commaSep":
		rule, err := imp.convertBuiltinSingleRuleArg(fnText, argNodes)
		if err != nil {
			return nil, true, err
		}
		return Optional(Seq(rule, Repeat(Seq(Str(","), rule)))), true, nil
	case "trailingSep1":
		sep, rule, err := imp.extractBuiltinSeparatorAndRule(argNodes)
		if err != nil {
			return nil, true, err
		}
		return Seq(Seq(rule, Repeat(Seq(sep, rule))), Optional(sep)), true, nil
	case "trailingCommaSep1":
		rule, err := imp.convertBuiltinSingleRuleArg(fnText, argNodes)
		if err != nil {
			return nil, true, err
		}
		return Seq(Seq(rule, Repeat(Seq(Str(","), rule))), Optional(Str(","))), true, nil
	case "trailingCommaSep":
		rule, err := imp.convertBuiltinSingleRuleArg(fnText, argNodes)
		if err != nil {
			return nil, true, err
		}
		return Optional(Seq(Seq(rule, Repeat(Seq(Str(","), rule))), Optional(Str(",")))), true, nil
	default:
		return nil, false, nil
	}
}

func (imp *jsImporter) convertBuiltinSingleRuleArg(fnText string, argNodes []*gotreesitter.Node) (*Rule, error) {
	if len(argNodes) != 1 {
		return nil, fmt.Errorf("%s expects 1 arg, got %d", fnText, len(argNodes))
	}
	rule, err := imp.convertRuleExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return rule, nil
}

func (imp *jsImporter) extractBuiltinSeparatorAndRule(argNodes []*gotreesitter.Node) (*Rule, *Rule, error) {
	if len(argNodes) != 2 {
		return nil, nil, fmt.Errorf("expected 2 args, got %d", len(argNodes))
	}
	first, err := imp.convertRuleExpr(argNodes[0])
	if err != nil {
		return nil, nil, err
	}
	second, err := imp.convertRuleExpr(argNodes[1])
	if err != nil {
		return nil, nil, err
	}
	if imp.helperArgLooksLikeSeparator(argNodes[0]) && !imp.helperArgLooksLikeSeparator(argNodes[1]) {
		return first, second, nil
	}
	if !imp.helperArgLooksLikeSeparator(argNodes[0]) && imp.helperArgLooksLikeSeparator(argNodes[1]) {
		return second, first, nil
	}
	// Default to the delimiter-first convention used by Scala and many
	// tree-sitter grammars when the argument shapes are ambiguous.
	return first, second, nil
}

func (imp *jsImporter) helperArgLooksLikeSeparator(n *gotreesitter.Node) bool {
	if n == nil {
		return false
	}
	switch imp.nodeType(n) {
	case "string", "regex":
		return true
	case "identifier":
		text := imp.nodeText(n)
		if text == strings.ToUpper(text) {
			return true
		}
		return false
	case "member_expression":
		prop := imp.extractMemberProp(n)
		switch {
		case prop == "":
			return false
		case prop == "_semicolon" || prop == "semicolon" || prop == "comma" || prop == "dot" || prop == "newline":
			return true
		case strings.HasSuffix(prop, "_separator") || strings.HasSuffix(prop, "_delimiter"):
			return true
		}
	}
	return false
}

// convertPrecCall converts prec/prec.left/prec.right/prec.dynamic calls.
func (imp *jsImporter) convertPrecCall(args []*gotreesitter.Node, make_ func(int, *Rule) *Rule) (*Rule, error) {
	switch len(args) {
	case 1:
		child, err := imp.convertRuleExpr(args[0])
		if err != nil {
			return nil, err
		}
		return make_(0, child), nil
	case 2:
		prec, err := imp.extractIntValue(args[0])
		if err != nil {
			return nil, fmt.Errorf("precedence: %w", err)
		}
		child, err := imp.convertRuleExpr(args[1])
		if err != nil {
			return nil, err
		}
		return make_(prec, child), nil
	default:
		return nil, fmt.Errorf("prec expects 1-2 args, got %d", len(args))
	}
}

// convertMemberExpr converts $.rule_name → Sym("rule_name").
func (imp *jsImporter) convertMemberExpr(n *gotreesitter.Node) (*Rule, error) {
	prop := imp.extractMemberProp(n)
	if prop == "" {
		return nil, fmt.Errorf("could not extract property from member expression: %s", imp.nodeText(n))
	}
	return Sym(prop), nil
}

// extractMemberProp extracts the property name from a member expression.
func (imp *jsImporter) extractMemberProp(n *gotreesitter.Node) string {
	prop := n.ChildByFieldName("property", imp.lang)
	if prop != nil {
		return imp.nodeText(prop)
	}
	return ""
}

// convertRuleArgs converts a slice of AST nodes to Rules.
func (imp *jsImporter) convertRuleArgs(nodes []*gotreesitter.Node) ([]*Rule, error) {
	var rules []*Rule
	for _, n := range nodes {
		r, err := imp.convertRuleExpr(n)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// extractRuleArray extracts an array of rule expressions (e.g. extras: $ => [...]).
func (imp *jsImporter) extractRuleArray(n *gotreesitter.Node) ([]*Rule, error) {
	body := imp.extractArrowBody(n)
	if body == nil {
		body = n
	}

	if imp.nodeType(body) != "array" {
		r, err := imp.convertRuleExpr(body)
		if err != nil {
			return nil, err
		}
		return []*Rule{r}, nil
	}

	var rules []*Rule
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		r, err := imp.convertRuleExpr(child)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// extractConflicts extracts conflict declarations: $ => [[$.a, $.b], ...]
func (imp *jsImporter) extractConflicts(n *gotreesitter.Node) ([][]string, error) {
	body := imp.extractArrowBody(n)
	if body == nil {
		body = n
	}

	if imp.nodeType(body) != "array" {
		return nil, nil
	}

	var conflicts [][]string
	for i := 0; i < int(body.NamedChildCount()); i++ {
		group := body.NamedChild(i)
		if imp.nodeType(group) != "array" {
			continue
		}
		var names []string
		for j := 0; j < int(group.NamedChildCount()); j++ {
			elem := group.NamedChild(j)
			if imp.nodeType(elem) == "member_expression" {
				names = append(names, imp.extractMemberProp(elem))
			}
		}
		if len(names) > 0 {
			conflicts = append(conflicts, names)
		}
	}
	return conflicts, nil
}

// extractExternals extracts external token declarations.
func (imp *jsImporter) extractExternals(n *gotreesitter.Node) ([]*Rule, error) {
	return imp.extractRuleArray(n)
}

// extractStringArray extracts an array of strings (for inline, supertypes).
func (imp *jsImporter) extractStringArray(n *gotreesitter.Node) []string {
	body := imp.extractArrowBody(n)
	if body == nil {
		body = n
	}

	if imp.nodeType(body) != "array" {
		return nil
	}

	var result []string
	for i := 0; i < int(body.NamedChildCount()); i++ {
		child := body.NamedChild(i)
		if imp.nodeType(child) == "string" {
			result = append(result, imp.extractStringValue(child))
		} else if imp.nodeType(child) == "member_expression" {
			result = append(result, imp.extractMemberProp(child))
		}
	}
	return result
}

// extractWordRef extracts the word token reference: $ => $.identifier
func (imp *jsImporter) extractWordRef(n *gotreesitter.Node) string {
	body := imp.extractArrowBody(n)
	if body == nil {
		body = n
	}

	if imp.nodeType(body) == "member_expression" {
		return imp.extractMemberProp(body)
	}
	if imp.nodeType(body) == "string" {
		return imp.extractStringValue(body)
	}
	return imp.nodeText(body)
}

// extractIntValue extracts an integer from a number, unary expression, or
// member expression like PREC.control. String precedence values (used in some
// grammars like Scala) are treated as 0.
func (imp *jsImporter) extractIntValue(n *gotreesitter.Node) (int, error) {
	// Try direct integer parse first.
	text := imp.nodeText(n)
	if v, err := strconv.Atoi(text); err == nil {
		return v, nil
	}

	// Check for member expression: OBJ.KEY (e.g., PREC.control)
	if imp.nodeType(n) == "member_expression" {
		objNode := n.ChildByFieldName("object", imp.lang)
		propNode := n.ChildByFieldName("property", imp.lang)
		if objNode != nil && propNode != nil {
			objName := imp.nodeText(objNode)
			propName := imp.nodeText(propNode)
			if vals, ok := imp.topLevelConsts[objName]; ok {
				if v, ok := vals[propName]; ok {
					return v, nil
				}
				return 0, fmt.Errorf("const %s has no key %q", objName, propName)
			}
		}
	}

	// String precedence values (e.g., prec.left("end", ...)) — resolve via the
	// grammar's precedences array when available, otherwise treat as 0.
	if imp.nodeType(n) == "string" || imp.nodeType(n) == "template_string" {
		name := imp.nodeText(n)
		if imp.nodeType(n) == "string" {
			name = imp.extractStringValue(n)
		} else if len(name) >= 2 {
			name = name[1 : len(name)-1]
		}
		if imp.namedPrecs != nil {
			if v, ok := imp.namedPrecs[name]; ok {
				return v, nil
			}
		}
		return 0, nil
	}

	return 0, fmt.Errorf("expected integer, got %q", text)
}

// extractRegexPattern extracts the pattern from a regex literal /pattern/flags.
func (imp *jsImporter) extractRegexPattern(n *gotreesitter.Node) string {
	text := imp.nodeText(n)
	if len(text) >= 2 && text[0] == '/' {
		end := strings.LastIndex(text, "/")
		if end > 0 {
			return text[1:end]
		}
	}
	return text
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// collectTopLevelConsts scans the root for top-level const declarations
// that are objects mapping string keys to integers (like PREC = {control: 1, ...}).
func (imp *jsImporter) collectTopLevelConsts(root *gotreesitter.Node) {
	imp.topLevelConsts = make(map[string]map[string]int)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if imp.nodeType(child) != "lexical_declaration" {
			continue
		}
		// Look for: const NAME = { key: val, ... }
		for j := 0; j < int(child.NamedChildCount()); j++ {
			decl := child.NamedChild(j)
			if imp.nodeType(decl) != "variable_declarator" {
				continue
			}
			nc := int(decl.NamedChildCount())
			if nc < 2 {
				continue
			}
			nameNode := decl.NamedChild(0)
			valueNode := decl.NamedChild(nc - 1)
			if imp.nodeType(valueNode) != "object" {
				continue
			}
			constName := imp.nodeText(nameNode)
			vals := make(map[string]int)
			for k := 0; k < int(valueNode.NamedChildCount()); k++ {
				pair := valueNode.NamedChild(k)
				if imp.nodeType(pair) != "pair" {
					continue
				}
				key := imp.getPairKey(pair)
				val := imp.getPairValue(pair)
				if val != nil {
					text := imp.nodeText(val)
					// Handle negative numbers
					if imp.nodeType(val) == "unary_expression" {
						text = imp.nodeText(val)
					}
					if n, err := strconv.Atoi(text); err == nil {
						vals[key] = n
					}
				}
			}
			if len(vals) > 0 {
				imp.topLevelConsts[constName] = vals
			}
		}
	}
}

// collectHelperFunctions scans the root for top-level function declarations
// (like commaSep, commaSep1, sep, sep1) and stores them for inlining.
func (imp *jsImporter) collectHelperFunctions(root *gotreesitter.Node) {
	imp.helperFuncs = make(map[string]*gotreesitter.Node)
	for i := 0; i < int(root.NamedChildCount()); i++ {
		child := root.NamedChild(i)
		if imp.nodeType(child) == "function_declaration" {
			name := child.ChildByFieldName("name", imp.lang)
			if name != nil {
				imp.helperFuncs[imp.nodeText(name)] = child
			}
		}
	}
}

// inlineHelperCall inlines a helper function call by substituting parameters.
func (imp *jsImporter) inlineHelperCall(funcName string, argNodes []*gotreesitter.Node) (*Rule, error) {
	funcNode := imp.helperFuncs[funcName]

	// Get parameter names.
	params := imp.getHelperParams(funcNode)

	// Convert arguments using the current context.
	args, err := imp.convertRuleArgs(argNodes)
	if err != nil {
		return nil, fmt.Errorf("helper %s args: %w", funcName, err)
	}

	// Save and set parameter substitutions.
	oldSubst := imp.paramSubst
	imp.paramSubst = make(map[string]*Rule)
	// Carry forward existing substitutions for nested helpers.
	for k, v := range oldSubst {
		imp.paramSubst[k] = v
	}
	for i, p := range params {
		if i < len(args) {
			imp.paramSubst[p] = args[i]
		}
	}

	// Get the function body's return expression.
	body := imp.getHelperBody(funcNode)
	if body == nil {
		imp.paramSubst = oldSubst
		return nil, fmt.Errorf("helper %s: could not find return expression", funcName)
	}

	result, err := imp.convertRuleExpr(body)

	// Restore.
	imp.paramSubst = oldSubst
	return result, err
}

// getHelperParams extracts parameter names from a function_declaration.
func (imp *jsImporter) getHelperParams(funcNode *gotreesitter.Node) []string {
	params := funcNode.ChildByFieldName("parameters", imp.lang)
	if params == nil {
		return nil
	}
	var names []string
	for i := 0; i < int(params.NamedChildCount()); i++ {
		child := params.NamedChild(i)
		names = append(names, imp.nodeText(child))
	}
	return names
}

// getHelperBody extracts the return expression from a function body.
func (imp *jsImporter) getHelperBody(funcNode *gotreesitter.Node) *gotreesitter.Node {
	body := funcNode.ChildByFieldName("body", imp.lang)
	if body == nil {
		return nil
	}
	return imp.findReturnExpr(body)
}

// findReturnExpr searches a statement_block for a return statement and returns
// the returned expression node.
func (imp *jsImporter) findReturnExpr(block *gotreesitter.Node) *gotreesitter.Node {
	for i := 0; i < int(block.NamedChildCount()); i++ {
		child := block.NamedChild(i)
		if imp.nodeType(child) == "return_statement" {
			if int(child.NamedChildCount()) > 0 {
				return child.NamedChild(0)
			}
		}
	}
	return nil
}

// collectLocalConsts scans a statement_block for const/let/var declarations
// and stores them in imp.localConsts for variable resolution during conversion.
func (imp *jsImporter) collectLocalConsts(block *gotreesitter.Node) {
	imp.localConsts = make(map[string]*gotreesitter.Node)
	for i := 0; i < int(block.NamedChildCount()); i++ {
		child := block.NamedChild(i)
		typ := imp.nodeType(child)
		if typ == "lexical_declaration" || typ == "variable_declaration" {
			for j := 0; j < int(child.NamedChildCount()); j++ {
				decl := child.NamedChild(j)
				if imp.nodeType(decl) == "variable_declarator" {
					// Use positional access: first named child is the name,
					// second named child is the init value.
					// (ChildByFieldName doesn't work for all grammar blobs.)
					nc := int(decl.NamedChildCount())
					if nc >= 2 {
						name := decl.NamedChild(0)
						value := decl.NamedChild(nc - 1)
						imp.localConsts[imp.nodeText(name)] = value
					}
				}
			}
		}
	}
}
