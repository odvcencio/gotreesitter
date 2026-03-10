package grammargen

import (
	"fmt"
	"strings"
	"unicode"
)

// ParseGrammarFile parses a declarative .grammar file into a Grammar IR.
//
// Syntax:
//
//	grammar <name>
//
//	extras = [ /\s/ ]
//	word = <rule_name>
//	supertypes = [ <rule_name>, ... ]
//	conflicts = [ [<rule>, <rule>], ... ]
//
//	rule <name> = <expr>
//
// Expressions:
//
//	"string"         string literal
//	/pattern/        regex pattern
//	<name>           symbol reference
//	seq(a, b, ...)   sequence
//	choice(a, b, ..) alternation
//	repeat(a)        zero or more
//	repeat1(a)       one or more
//	optional(a)      optional
//	token(a)         token boundary
//	field("name", a) field annotation
//	prec(n, a)       precedence
//	prec.left(n, a)  left-associative precedence
//	prec.right(n, a) right-associative precedence
func ParseGrammarFile(source string) (*Grammar, error) {
	p := &grammarParser{
		source: source,
		lines:  strings.Split(source, "\n"),
	}
	return p.parse()
}

type grammarParser struct {
	source string
	lines  []string
	pos    int // current line index
}

func (p *grammarParser) parse() (*Grammar, error) {
	g := NewGrammar("")

	for p.pos < len(p.lines) {
		line := strings.TrimSpace(p.lines[p.pos])

		// Skip empty lines and comments.
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			p.pos++
			continue
		}

		if strings.HasPrefix(line, "grammar ") {
			g.Name = strings.TrimSpace(strings.TrimPrefix(line, "grammar"))
			p.pos++
			continue
		}

		if strings.HasPrefix(line, "extras") {
			extras, err := p.parseAssignedRuleArray(line, "extras")
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", p.pos+1, err)
			}
			g.Extras = extras
			p.pos++
			continue
		}

		if strings.HasPrefix(line, "word") {
			g.Word = p.parseAssignedString(line, "word")
			p.pos++
			continue
		}

		if strings.HasPrefix(line, "supertypes") {
			g.Supertypes = p.parseAssignedStringArray(line, "supertypes")
			p.pos++
			continue
		}

		if strings.HasPrefix(line, "conflicts") {
			conflicts, err := p.parseConflicts(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", p.pos+1, err)
			}
			g.Conflicts = conflicts
			p.pos++
			continue
		}

		if strings.HasPrefix(line, "rule ") {
			name, rule, err := p.parseRuleDef(line)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", p.pos+1, err)
			}
			g.Define(name, rule)
			p.pos++
			continue
		}

		return nil, fmt.Errorf("line %d: unexpected: %s", p.pos+1, line)
	}

	return g, nil
}

// parseAssignedString parses: key = value
func (p *grammarParser) parseAssignedString(line, key string) string {
	rest := strings.TrimPrefix(line, key)
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, "=")
	return strings.TrimSpace(rest)
}

// parseAssignedStringArray parses: key = [ name1, name2 ]
func (p *grammarParser) parseAssignedStringArray(line, key string) []string {
	rest := strings.TrimPrefix(line, key)
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, "=")
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, "[]")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, ",")
	var result []string
	for _, part := range parts {
		s := strings.TrimSpace(part)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

// parseAssignedRuleArray parses: key = [ rule1, rule2 ]
func (p *grammarParser) parseAssignedRuleArray(line, key string) ([]*Rule, error) {
	rest := strings.TrimPrefix(line, key)
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, "=")
	rest = strings.TrimSpace(rest)
	rest = strings.Trim(rest, "[]")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil, nil
	}

	var rules []*Rule
	exprs := splitTopLevel(rest, ',')
	for _, expr := range exprs {
		expr = strings.TrimSpace(expr)
		if expr == "" {
			continue
		}
		r, err := parseRuleExpr(expr)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// parseConflicts parses: conflicts = [ [a, b], [c, d] ]
func (p *grammarParser) parseConflicts(line string) ([][]string, error) {
	rest := strings.TrimPrefix(line, "conflicts")
	rest = strings.TrimSpace(rest)
	rest = strings.TrimPrefix(rest, "=")
	rest = strings.TrimSpace(rest)

	// Strip outer brackets.
	if len(rest) >= 2 && rest[0] == '[' && rest[len(rest)-1] == ']' {
		rest = rest[1 : len(rest)-1]
	}
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil, nil
	}

	var conflicts [][]string
	groups := splitTopLevel(rest, ',')
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		group = strings.Trim(group, "[]")
		parts := strings.Split(group, ",")
		var names []string
		for _, part := range parts {
			s := strings.TrimSpace(part)
			if s != "" {
				names = append(names, s)
			}
		}
		if len(names) > 0 {
			conflicts = append(conflicts, names)
		}
	}
	return conflicts, nil
}

// parseRuleDef parses: rule <name> = <expr>
func (p *grammarParser) parseRuleDef(line string) (string, *Rule, error) {
	rest := strings.TrimPrefix(line, "rule")
	rest = strings.TrimSpace(rest)

	// Find the = sign.
	eqIdx := strings.Index(rest, "=")
	if eqIdx < 0 {
		return "", nil, fmt.Errorf("expected '=' in rule definition")
	}

	name := strings.TrimSpace(rest[:eqIdx])
	expr := strings.TrimSpace(rest[eqIdx+1:])

	rule, err := parseRuleExpr(expr)
	if err != nil {
		return "", nil, fmt.Errorf("rule %q: %w", name, err)
	}

	return name, rule, nil
}

// parseRuleExpr parses a rule expression string.
func parseRuleExpr(expr string) (*Rule, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return Blank(), nil
	}

	// String literal: "..."
	if len(expr) >= 2 && expr[0] == '"' && expr[len(expr)-1] == '"' {
		return Str(expr[1 : len(expr)-1]), nil
	}

	// Regex pattern: /.../
	if len(expr) >= 2 && expr[0] == '/' && expr[len(expr)-1] == '/' {
		return Pat(expr[1 : len(expr)-1]), nil
	}

	// Function call: name(args...)
	if parenIdx := findTopLevelParen(expr); parenIdx > 0 && expr[len(expr)-1] == ')' {
		fnName := expr[:parenIdx]
		argsStr := expr[parenIdx+1 : len(expr)-1]
		return parseCallExpr(fnName, argsStr)
	}

	// Plain identifier → Sym()
	if isIdentifier(expr) {
		return Sym(expr), nil
	}

	return nil, fmt.Errorf("cannot parse expression: %s", expr)
}

// parseCallExpr parses a function call: fnName(args).
func parseCallExpr(fnName, argsStr string) (*Rule, error) {
	args := splitTopLevel(argsStr, ',')

	switch fnName {
	case "seq":
		children, err := parseRuleArgs(args)
		if err != nil {
			return nil, fmt.Errorf("seq: %w", err)
		}
		return Seq(children...), nil

	case "choice":
		children, err := parseRuleArgs(args)
		if err != nil {
			return nil, fmt.Errorf("choice: %w", err)
		}
		return Choice(children...), nil

	case "repeat":
		if len(args) != 1 {
			return nil, fmt.Errorf("repeat expects 1 arg")
		}
		child, err := parseRuleExpr(args[0])
		if err != nil {
			return nil, err
		}
		return Repeat(child), nil

	case "repeat1":
		if len(args) != 1 {
			return nil, fmt.Errorf("repeat1 expects 1 arg")
		}
		child, err := parseRuleExpr(args[0])
		if err != nil {
			return nil, err
		}
		return Repeat1(child), nil

	case "optional":
		if len(args) != 1 {
			return nil, fmt.Errorf("optional expects 1 arg")
		}
		child, err := parseRuleExpr(args[0])
		if err != nil {
			return nil, err
		}
		return Optional(child), nil

	case "token":
		if len(args) != 1 {
			return nil, fmt.Errorf("token expects 1 arg")
		}
		child, err := parseRuleExpr(args[0])
		if err != nil {
			return nil, err
		}
		return Token(child), nil

	case "field":
		if len(args) != 2 {
			return nil, fmt.Errorf("field expects 2 args")
		}
		name := strings.TrimSpace(args[0])
		name = strings.Trim(name, `"`)
		child, err := parseRuleExpr(args[1])
		if err != nil {
			return nil, err
		}
		return Field(name, child), nil

	case "prec":
		return parsePrecArgs(args, Prec)

	case "prec.left":
		return parsePrecArgs(args, PrecLeft)

	case "prec.right":
		return parsePrecArgs(args, PrecRight)

	case "prec.dynamic":
		return parsePrecArgs(args, PrecDynamic)

	case "alias":
		if len(args) < 2 {
			return nil, fmt.Errorf("alias expects 2 args")
		}
		child, err := parseRuleExpr(args[0])
		if err != nil {
			return nil, err
		}
		aliasName := strings.TrimSpace(args[1])
		named := true
		if len(aliasName) >= 2 && aliasName[0] == '"' && aliasName[len(aliasName)-1] == '"' {
			aliasName = aliasName[1 : len(aliasName)-1]
			named = false
		}
		return Alias(child, aliasName, named), nil

	default:
		return nil, fmt.Errorf("unknown function %q", fnName)
	}
}

func parsePrecArgs(args []string, make_ func(int, *Rule) *Rule) (*Rule, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("prec expects 2 args (precedence, rule)")
	}
	precStr := strings.TrimSpace(args[0])
	prec := 0
	fmt.Sscanf(precStr, "%d", &prec)
	child, err := parseRuleExpr(args[1])
	if err != nil {
		return nil, err
	}
	return make_(prec, child), nil
}

func parseRuleArgs(args []string) ([]*Rule, error) {
	var rules []*Rule
	for _, a := range args {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		r, err := parseRuleExpr(a)
		if err != nil {
			return nil, err
		}
		rules = append(rules, r)
	}
	return rules, nil
}

// splitTopLevel splits a string by a separator, respecting nested parens/brackets/strings.
func splitTopLevel(s string, sep byte) []string {
	var result []string
	depth := 0
	inStr := false
	strChar := byte(0)
	start := 0

	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == strChar {
				inStr = false
			} else if c == '\\' {
				i++ // skip escaped char
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = true
			strChar = c
			continue
		}
		if c == '/' && i+1 < len(s) {
			// Skip regex patterns /.../.
			end := strings.IndexByte(s[i+1:], '/')
			if end >= 0 {
				i += end + 1
				continue
			}
		}
		if c == '(' || c == '[' {
			depth++
		} else if c == ')' || c == ']' {
			depth--
		} else if c == sep && depth == 0 {
			result = append(result, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		result = append(result, s[start:])
	}
	return result
}

// findTopLevelParen finds the first top-level '(' not inside strings/regex.
func findTopLevelParen(s string) int {
	inStr := false
	strChar := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			if c == strChar {
				inStr = false
			} else if c == '\\' {
				i++
			}
			continue
		}
		if c == '"' || c == '\'' {
			inStr = true
			strChar = c
			continue
		}
		if c == '(' {
			return i
		}
	}
	return -1
}

// isIdentifier returns true if s looks like a grammar identifier.
func isIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	for i, c := range s {
		if i == 0 && !unicode.IsLetter(c) && c != '_' {
			return false
		}
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			return false
		}
	}
	return true
}
