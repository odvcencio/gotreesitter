// Package grammargen implements a pure-Go grammar generator for gotreesitter.
// It compiles grammar definitions expressed in a Go DSL into binary blobs
// that the gotreesitter runtime can load and use for parsing.
package grammargen

// RuleKind identifies the type of a grammar rule node.
type RuleKind int

const (
	RuleString      RuleKind = iota // literal string: "{"
	RulePattern                     // regex pattern: /[0-9]+/
	RuleSymbol                      // symbol reference: $.object
	RuleSeq                         // sequence: seq(a, b, c)
	RuleChoice                      // alternation: choice(a, b)
	RuleRepeat                      // zero-or-more: repeat(a)
	RuleRepeat1                     // one-or-more: repeat1(a)
	RuleOptional                    // optional: optional(a)
	RuleToken                       // token boundary: token(a)
	RuleImmToken                    // immediate token: token.immediate(a)
	RuleField                       // field annotation: field("name", a)
	RulePrec                        // precedence: prec(n, a)
	RulePrecLeft                    // left-associative: prec.left(n, a)
	RulePrecRight                   // right-associative: prec.right(n, a)
	RulePrecDynamic                 // dynamic precedence: prec.dynamic(n, a)
	RuleBlank                       // epsilon / empty
	RuleAlias                       // alias: alias(a, "name")
)

// Rule is a node in the grammar rule tree.
type Rule struct {
	Kind     RuleKind
	Value    string  // literal/pattern/symbol/field name
	Children []*Rule // sub-rules
	Prec     int     // precedence value
	Named    bool    // for alias: whether the alias is a named node
}

// TestCase is an embedded grammar test case.
type TestCase struct {
	Name        string // test name
	Input       string // input to parse
	Expected    string // expected S-expression (empty = just check no errors)
	ExpectError bool   // if true, expect ERROR nodes in the tree
}

// PrecEntry is an entry in a precedences level. It is either a named
// precedence (STRING type, Name is the prec name) or a rule reference
// (SYMBOL type, Name is the rule name).
type PrecEntry struct {
	IsSymbol bool   // true for SYMBOL entries, false for STRING entries
	Name     string // prec name or rule name
}

// ReservedWordSet is an ordered named set of reserved word token rules.
// The first set is the global set from grammar.json's top-level `reserved`
// object. Additional sets are preserved for future context-specific support.
type ReservedWordSet struct {
	Name  string
	Rules []*Rule
}

// Grammar is the top-level grammar definition.
type Grammar struct {
	Name                                       string
	Rules                                      map[string]*Rule
	RuleOrder                                  []string // order rules were defined (first = start rule)
	Extras                                     []*Rule
	Conflicts                                  [][]string
	Externals                                  []*Rule
	Inline                                     []string
	Word                                       string
	ReservedWordSets                           []ReservedWordSet
	Supertypes                                 []string
	Tests                                      []TestCase    // embedded test cases
	EnableLRSplitting                          bool          // opt-in: attempt LR(1) state splitting for merge pathology
	BinaryRepeatMode                           bool          // use tree-sitter's binary repeat helper shape (aux→seq(aux,aux)|inner)
	FlattenGeneratedRepeatAux                  bool          // allow generated repeat helpers to participate in hidden-choice flattening
	ReuseRepeatAuxForParents                   []string      // parent rule names whose repeat helpers may be shared by canonical body
	PreserveKeywordIdentifierConflicts         bool          // keep keyword-as-identifier S/R ambiguity for grammars like Fortran
	ExactPrefixStates                          int           // keep this many LR(1) states exact before merge compaction
	Precedences                                [][]PrecEntry // ordered precedence levels (each level: earlier = higher prec)
	ChoiceLiftThreshold                        int           // if >0, lift inline CHOICE nodes with more alternatives than this into auxiliary nonterminals to prevent production explosion
	SuppressEquivalentExternalReduceLookaheads bool          // suppress external scanner validity for duplicate reduce-only lookaheads
	ExternalReduceFollowLookaheads             []string      // external token names that may be valid after reducing in the current state
	PriorityInlinePatterns                     []string      // anonymous pattern terminals that should win same-length ties against named tokens
	WantsForest                                bool          // opt this grammar's assembled Language into the GSS-forest GLR fast path (see gotreesitter.Language.WantsForest)
}

// NewGrammar creates a new grammar with the given name.
func NewGrammar(name string) *Grammar {
	return &Grammar{
		Name:  name,
		Rules: make(map[string]*Rule),
	}
}

// Define adds a rule to the grammar. The first rule defined is the start rule.
func (g *Grammar) Define(name string, rule *Rule) {
	if _, exists := g.Rules[name]; !exists {
		g.RuleOrder = append(g.RuleOrder, name)
	}
	g.Rules[name] = rule
}

// AppendChoice appends an alternative to an existing rule, wrapping the prior
// definition in a Choice if needed.
func AppendChoice(g *Grammar, name string, rule *Rule) {
	if existing, ok := g.Rules[name]; ok && existing != nil {
		if existing.Kind == RuleChoice {
			existing.Children = append(existing.Children, rule)
			return
		}
		g.Rules[name] = Choice(existing, rule)
		return
	}
	g.Define(name, rule)
}

// SetExtras sets the extra rules (e.g. whitespace, comments).
func (g *Grammar) SetExtras(rules ...*Rule) {
	g.Extras = rules
}

// SetConflicts declares grammar conflicts for GLR.
func (g *Grammar) SetConflicts(conflicts ...[]string) {
	g.Conflicts = conflicts
}

// AddConflict appends a GLR conflict declaration to the grammar.
func AddConflict(g *Grammar, names ...string) {
	g.Conflicts = append(g.Conflicts, names)
}

// SetExternals declares external scanner tokens.
func (g *Grammar) SetExternals(rules ...*Rule) {
	g.Externals = rules
}

// SetInline marks rules to be inlined.
func (g *Grammar) SetInline(names ...string) {
	g.Inline = names
}

// SetWord sets the word token for keyword extraction.
func (g *Grammar) SetWord(name string) {
	g.Word = name
}

// SetSupertypes declares supertype rules.
func (g *Grammar) SetSupertypes(names ...string) {
	g.Supertypes = names
}

// --- Builder functions ---

// Str creates a string literal rule.
func Str(s string) *Rule {
	return &Rule{Kind: RuleString, Value: s}
}

// Pat creates a regex pattern rule.
func Pat(pattern string) *Rule {
	return &Rule{Kind: RulePattern, Value: pattern}
}

// Sym creates a symbol reference rule.
func Sym(name string) *Rule {
	return &Rule{Kind: RuleSymbol, Value: name}
}

// Blank creates an epsilon (empty) rule.
func Blank() *Rule {
	return &Rule{Kind: RuleBlank}
}

// Seq creates a sequence of rules.
func Seq(rules ...*Rule) *Rule {
	return &Rule{Kind: RuleSeq, Children: rules}
}

// Choice creates an alternation of rules.
func Choice(rules ...*Rule) *Rule {
	return &Rule{Kind: RuleChoice, Children: rules}
}

// Repeat creates a zero-or-more repetition.
func Repeat(rule *Rule) *Rule {
	return &Rule{Kind: RuleRepeat, Children: []*Rule{rule}}
}

// Repeat1 creates a one-or-more repetition.
func Repeat1(rule *Rule) *Rule {
	return &Rule{Kind: RuleRepeat1, Children: []*Rule{rule}}
}

// Optional creates an optional rule.
func Optional(rule *Rule) *Rule {
	return &Rule{Kind: RuleOptional, Children: []*Rule{rule}}
}

// Token creates a token boundary (content is a single lexer token).
func Token(rule *Rule) *Rule {
	return &Rule{Kind: RuleToken, Children: []*Rule{rule}}
}

// ImmToken creates an immediate token (no preceding whitespace).
func ImmToken(rule *Rule) *Rule {
	return &Rule{Kind: RuleImmToken, Children: []*Rule{rule}}
}

// Field annotates a rule with a field name.
func Field(name string, rule *Rule) *Rule {
	return &Rule{Kind: RuleField, Value: name, Children: []*Rule{rule}}
}

// Prec sets precedence on a rule.
func Prec(n int, rule *Rule) *Rule {
	return &Rule{Kind: RulePrec, Prec: n, Children: []*Rule{rule}}
}

// PrecLeft sets left-associative precedence on a rule.
func PrecLeft(n int, rule *Rule) *Rule {
	return &Rule{Kind: RulePrecLeft, Prec: n, Children: []*Rule{rule}}
}

// PrecRight sets right-associative precedence on a rule.
func PrecRight(n int, rule *Rule) *Rule {
	return &Rule{Kind: RulePrecRight, Prec: n, Children: []*Rule{rule}}
}

// PrecDynamic sets dynamic precedence on a rule.
func PrecDynamic(n int, rule *Rule) *Rule {
	return &Rule{Kind: RulePrecDynamic, Prec: n, Children: []*Rule{rule}}
}

// Alias aliases a rule to a different name.
func Alias(rule *Rule, name string, named bool) *Rule {
	return &Rule{Kind: RuleAlias, Value: name, Named: named, Children: []*Rule{rule}}
}

// Test adds an embedded test case. Input is parsed and the resulting tree
// is compared against the expected S-expression. If expected is empty,
// the test only checks that no ERROR nodes appear.
func (g *Grammar) Test(name, input, expected string) {
	g.Tests = append(g.Tests, TestCase{
		Name:     name,
		Input:    input,
		Expected: expected,
	})
}

// TestError adds an embedded test case that expects parse errors.
func (g *Grammar) TestError(name, input string) {
	g.Tests = append(g.Tests, TestCase{
		Name:        name,
		Input:       input,
		ExpectError: true,
	})
}

// --- Convenience combinators ---

// CommaSep creates an optional comma-separated list.
func CommaSep(rule *Rule) *Rule {
	return Optional(CommaSep1(rule))
}

// CommaSep1 creates a non-empty comma-separated list.
func CommaSep1(rule *Rule) *Rule {
	return Seq(rule, Repeat(Seq(Str(","), rule)))
}

// SepBy creates an optional list separated by the given separator.
func SepBy(sep, rule *Rule) *Rule {
	return Optional(SepBy1(sep, rule))
}

// SepBy1 creates a non-empty list separated by the given separator.
func SepBy1(sep, rule *Rule) *Rule {
	return Seq(rule, Repeat(Seq(sep, rule)))
}

// Surround wraps a rule with open and close delimiters.
func Surround(open, rule, close *Rule) *Rule {
	return Seq(open, rule, close)
}

// Parens wraps a rule in parentheses.
func Parens(rule *Rule) *Rule {
	return Surround(Str("("), rule, Str(")"))
}

// Brackets wraps a rule in square brackets.
func Brackets(rule *Rule) *Rule {
	return Surround(Str("["), rule, Str("]"))
}

// Braces wraps a rule in curly braces.
func Braces(rule *Rule) *Rule {
	return Surround(Str("{"), rule, Str("}"))
}

// --- Grammar composition ---

// ExtendGrammar creates a new grammar that inherits from a base grammar.
// The customize function receives the new grammar with all base rules copied in,
// and can override rules, add new ones, or modify extras/conflicts/etc.
//
// Example:
//
//	cpp := ExtendGrammar("cpp", cGrammar(), func(g *Grammar) {
//	    g.Define("class_declaration", Seq(Str("class"), Sym("identifier"), Sym("class_body")))
//	    // Override an existing rule:
//	    g.Define("declaration", Choice(Sym("class_declaration"), Sym("function_declaration")))
//	})
func ExtendGrammar(name string, base *Grammar, customize func(g *Grammar)) *Grammar {
	g := &Grammar{
		Name:                               name,
		Rules:                              make(map[string]*Rule, len(base.Rules)),
		RuleOrder:                          make([]string, len(base.RuleOrder)),
		Extras:                             make([]*Rule, len(base.Extras)),
		Conflicts:                          make([][]string, len(base.Conflicts)),
		Externals:                          make([]*Rule, len(base.Externals)),
		Inline:                             make([]string, len(base.Inline)),
		Word:                               base.Word,
		ReservedWordSets:                   cloneReservedWordSets(base.ReservedWordSets),
		Supertypes:                         make([]string, len(base.Supertypes)),
		Tests:                              make([]TestCase, len(base.Tests)),
		EnableLRSplitting:                  base.EnableLRSplitting,
		BinaryRepeatMode:                   base.BinaryRepeatMode,
		FlattenGeneratedRepeatAux:          base.FlattenGeneratedRepeatAux,
		ReuseRepeatAuxForParents:           make([]string, len(base.ReuseRepeatAuxForParents)),
		PreserveKeywordIdentifierConflicts: base.PreserveKeywordIdentifierConflicts,
		ExactPrefixStates:                  base.ExactPrefixStates,
		Precedences:                        clonePrecedenceLevels(base.Precedences),
		ChoiceLiftThreshold:                base.ChoiceLiftThreshold,
		SuppressEquivalentExternalReduceLookaheads: base.SuppressEquivalentExternalReduceLookaheads,
		ExternalReduceFollowLookaheads:             append([]string(nil), base.ExternalReduceFollowLookaheads...),
		PriorityInlinePatterns:                     append([]string(nil), base.PriorityInlinePatterns...),
		WantsForest:                                base.WantsForest,
	}

	// Deep copy rules.
	copy(g.RuleOrder, base.RuleOrder)
	for k, v := range base.Rules {
		g.Rules[k] = cloneRule(v)
	}
	for i, extra := range base.Extras {
		g.Extras[i] = cloneRule(extra)
	}
	for i, c := range base.Conflicts {
		g.Conflicts[i] = make([]string, len(c))
		copy(g.Conflicts[i], c)
	}
	for i, ext := range base.Externals {
		g.Externals[i] = cloneRule(ext)
	}
	copy(g.Inline, base.Inline)
	copy(g.Supertypes, base.Supertypes)
	copy(g.Tests, base.Tests)
	copy(g.ReuseRepeatAuxForParents, base.ReuseRepeatAuxForParents)

	// Let the caller customize.
	customize(g)

	return g
}

func clonePrecedenceLevels(src [][]PrecEntry) [][]PrecEntry {
	if len(src) == 0 {
		return nil
	}
	out := make([][]PrecEntry, len(src))
	for i, level := range src {
		if len(level) == 0 {
			continue
		}
		out[i] = make([]PrecEntry, len(level))
		copy(out[i], level)
	}
	return out
}

func cloneReservedWordSets(src []ReservedWordSet) []ReservedWordSet {
	if len(src) == 0 {
		return nil
	}
	out := make([]ReservedWordSet, len(src))
	for i, set := range src {
		out[i].Name = set.Name
		if len(set.Rules) == 0 {
			continue
		}
		out[i].Rules = make([]*Rule, len(set.Rules))
		for j, rule := range set.Rules {
			out[i].Rules[j] = cloneRule(rule)
		}
	}
	return out
}

// cloneRule is defined in regex.go — reused here for grammar composition.
