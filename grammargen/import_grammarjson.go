package grammargen

import (
	"encoding/json"
	"fmt"
	"strings"
)

func applyImportGrammarShapeHints(g *Grammar) {
	if g == nil {
		return
	}
	switch g.Name {
	case "elixir":
		// Elixir operators are also callable operator identifiers. In expression
		// position, tree-sitter keeps the atom-to-expression reduce live when an
		// operator literal can also start operator_identifier; otherwise `a ** b`
		// shifts `**` into operator_identifier and truncates before the RHS.
		g.PreferExpressionOperatorIdentifierReduces = true
		// Elixir's call rules use left precedence so a same-line `do` sticks to
		// the outermost call. Preserve the completed parenthesized-call reduce
		// in that narrow conflict; otherwise `def f(x) when guard(x) do` lets
		// the guard call consume the block.
		g.PreferParenthesizedCallDoBlockReduces = true
		// Remote calls without explicit arguments (`String.upcase`) must complete
		// before a following binary operator such as `|>` can attach to the
		// enclosing expression. Otherwise the operator shifts from the raw
		// _remote_dot state and the RHS is no longer lexed in expression mode.
		g.PreferRemoteCallOperatorReduces = true
		// A completed stab-clause left operand followed by `->` must reduce
		// before shifting the arrow into stab_clause. Otherwise `acc -> value`
		// treats `->` as an operator identifier and never builds the clause.
		g.PreferStabClauseLeftArrowReduces = true
		// Elixir's quoted-content scanner intentionally declines when both the
		// interpolating single- and double-quote content tokens are valid. After
		// `#{...}`, merged external lex rows can expose both variants, making the
		// normal lexer skip to the closing quote and truncate the quoted node.
		g.PreferPreciseExternalLexStates = true
		// The capture grammar uses a hidden pass-through from
		// _capture_expression to _expression alongside parenthesized capture
		// operands. C keeps that cc=1 reduce available in operator/dot conflicts;
		// flattening it away can truncate quoted pipeline expressions before the
		// following remote call/operator chain.
		g.PreserveHiddenChoicePassthrough = []string{"_capture_expression"}
	case "bash":
		// Bash's external extglob token is intentionally broad. In merged LALR
		// states, reduce-only lookaheads can otherwise ask the scanner for
		// extglob_pattern at command-substitution delimiters and overconsume.
		g.SuppressEquivalentExternalReduceLookaheads = true
		// Bash's scanner also classifies whitespace-sensitive command
		// arguments. Some of those tokens only become valid after reducing the
		// preceding word, so expose this narrow set to the scanner through the
		// external lex-state rows without broadening extglob_pattern.
		g.ExternalReduceFollowLookaheads = []string{"file_descriptor", "test_operator", "$", "{", "(", "<<", "<<-"}
		// Bash's number literals are anonymous regex leaves inside the visible
		// number rule. They overlap the broad word token on strings like "-9";
		// tree-sitter's lexer prefers the number pattern on equal length.
		g.PriorityInlinePatterns = []string{
			"-?(0x)?[0-9]+(#[0-9A-Za-z@_]+)?",
			"-?(0x)?[0-9]+#",
		}
	case "fortran":
		// Fortran has no reserved words. Its grammar routes keyword-shaped
		// tokens back through identifier and declares conflicts to let parser
		// context decide whether a word like "data" is a statement keyword or
		// a callable identifier.
		g.BinaryRepeatMode = true
		g.PreserveKeywordIdentifierConflicts = true
	case "python":
		// Python benefits from tree-sitter's binary repeat helper shape for
		// broad corpus parity, but its soft `type` alias syntax needs the early
		// LR(1) conflict context preserved. Otherwise large-grammar merge
		// compaction can deterministically choose the primary-expression path.
		g.BinaryRepeatMode = true
		g.ExactPrefixStates = 999999
	case "gomod":
		// Go module's bracketed retract intervals need the `retract_spec`
		// reduce lookahead kept distinct through the closing `)` of grouped
		// retract directives. LALR merge compaction otherwise gives the DFA a
		// state that skips the close delimiter and truncates the parse.
		g.ExactPrefixStates = 999999
	case "javascript", "typescript", "tsx", "sql", "d", "objc", "perl":
		// These grammars rely heavily on tree-sitter's binary repeat helper
		// shape. Keeping the upstream lowering avoids large state blowups and
		// preserves upstream ambiguity handling for imported grammars.
		g.BinaryRepeatMode = true
		if g.Name == "javascript" {
			g.FlattenGeneratedRepeatAux = true
			g.ReuseRepeatAuxForParents = []string{"jsx_opening_element", "jsx_self_closing_element"}
		}
	case "powershell":
		// PowerShell's string/command repeat helpers carry broad content-token
		// lookaheads. The upstream binary repeat shape keeps those lookaheads
		// from leaking into plain variable assignment lex modes.
		g.BinaryRepeatMode = true
	case "promql":
		// PromQL's operator grammar relies on hidden pass-through reductions
		// through _query and _series_matcher to complete higher-precedence
		// operands before lower-precedence operator shifts.
		g.PreserveHiddenChoicePassthrough = []string{"_query", "_series_matcher"}
	}
}

func applyImportGrammarPostShapeHints(g *Grammar) {
	if g == nil {
		return
	}
	switch g.Name {
	case "perl":
		if _, ok := g.Rules["heredoc_content"]; ok {
			// Perl models heredoc bodies as grammar extras that start and end in
			// the external scanner. Importing the full interpolation grammar into
			// a nonterminal-extra chain makes table generation unbounded for this
			// shape; keep the scanner-delimited body path and leave richer heredoc
			// interpolation structure to a scanner-aware follow-up.
			g.Rules["heredoc_content"] = Seq(
				Sym("_heredoc_start"),
				Repeat(Choice(
					Sym("_heredoc_middle"),
					Sym("escape_sequence"),
				)),
				Sym("heredoc_end"),
			)
		}
	}
}

// ImportGrammarJSON parses a tree-sitter grammar.json file (the canonical
// resolved form generated by `tree-sitter generate`) and returns a Grammar IR.
// This is more reliable than ImportGrammarJS because grammar.json has no
// require() calls, helper functions, or other JavaScript-specific constructs.
func ImportGrammarJSON(data []byte) (*Grammar, error) {
	var raw jsonGrammar
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse grammar.json: %w", err)
	}

	g := NewGrammar(raw.Name)
	applyImportGrammarShapeHints(g)

	if raw.Gotreesitter != nil {
		g.WantsForest = raw.Gotreesitter.WantsForest
	}

	// Build named precedence → numeric value mapping from the precedences array.
	// Each level is an ordered list from highest to lowest precedence.
	// STRING entries define named precedence values.
	namedPrecs := buildNamedPrecMap(raw.Precedences)

	// Store the full precedences table (including SYMBOL entries) for use
	// during LR conflict resolution. SYMBOL entries define rule-level
	// precedence ordering (e.g. update_expression > logical_and) that
	// cannot be captured by numeric prec values alone.
	g.Precedences = importPrecedenceLevels(raw.Precedences)

	conv := &jsonConverter{namedPrecs: namedPrecs}

	// Import rules in order.
	for _, name := range raw.ruleOrder {
		rule, err := conv.convertRule(raw.Rules[name])
		if err != nil {
			return nil, fmt.Errorf("rule %q: %w", name, err)
		}
		g.Define(name, rule)
	}

	// Import extras.
	for _, extra := range raw.Extras {
		rule, err := conv.convertRule(extra)
		if err != nil {
			return nil, fmt.Errorf("extras: %w", err)
		}
		g.Extras = append(g.Extras, rule)
	}

	// Import conflicts.
	for _, group := range raw.Conflicts {
		var names []string
		names = append(names, group...)
		if len(names) > 0 {
			g.Conflicts = append(g.Conflicts, names)
		}
	}

	// Import externals.
	for _, ext := range raw.Externals {
		rule, err := conv.convertRule(ext)
		if err != nil {
			return nil, fmt.Errorf("externals: %w", err)
		}
		g.Externals = append(g.Externals, rule)
	}

	// Import inline rules.
	g.Inline = raw.Inline

	// Import word token.
	if raw.Word != "" {
		g.Word = raw.Word
	}

	// Import reserved word sets in source order. The first set is the global
	// reserved set in tree-sitter's grammar.json format.
	for _, name := range raw.reservedOrder {
		members := raw.Reserved[name]
		rules := make([]*Rule, 0, len(members))
		for _, member := range members {
			rule, err := conv.convertRule(member)
			if err != nil {
				return nil, fmt.Errorf("reserved %q: %w", name, err)
			}
			rules = append(rules, rule)
		}
		g.ReservedWordSets = append(g.ReservedWordSets, ReservedWordSet{
			Name:  name,
			Rules: rules,
		})
	}

	// Import supertypes.
	g.Supertypes = raw.Supertypes

	// Reserved word sets are only meaningful when productions actually use
	// per-context RESERVED wrappers. In tree-sitter's semantic, without any
	// RESERVED usage every production step gets the default reserved_word_set_id,
	// which results in no per-state filtering at runtime. Our generator's
	// buildReservedWordTables currently applies the global reserved set more
	// aggressively than tree-sitter would (treating it as a universal filter),
	// which breaks grammars like Go where all reserved words are hard keywords
	// handled directly by the LR grammar. Drop the sets when no RESERVED node
	// was encountered so the runtime path falls back to the default (no filter)
	// behavior that matches the DSL-built language.
	if !conv.sawReservedNode {
		g.ReservedWordSets = nil
	}

	applyImportGrammarPostShapeHints(g)

	return g, nil
}

// buildPrecMaps builds two precedence maps from the precedences array:
//   - namedPrecs: STRING entry name → numeric value (for resolving named precs
//     like "logical_and" used in PREC_LEFT("logical_and", ...))
//   - symbolPrecs: SYMBOL entry name → numeric value (for overriding the outer
//     PREC value of entire rules like update_expression)
//
// buildNamedPrecMap builds a mapping from named precedence strings to numeric
// values. Levels are ordered from highest to lowest precedence; within each
// level, earlier entries have higher precedence. Values are assigned globally
// across all levels so that entries in earlier levels always outrank entries
// in later levels.
func buildNamedPrecMap(rawLevels []json.RawMessage) map[string]int {
	// First pass: collect all STRING entries across all levels in order.
	type precEntry struct {
		name      string
		globalIdx int
	}
	var all []precEntry
	for _, rawLevel := range rawLevels {
		var entries []jsonPrecEntry
		if err := json.Unmarshal(rawLevel, &entries); err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Type == "STRING" && entry.Value != "" {
				all = append(all, precEntry{name: entry.Value, globalIdx: len(all)})
			}
		}
	}

	// Second pass: assign values so first entry gets highest value.
	m := make(map[string]int, len(all))
	total := len(all)
	for _, e := range all {
		val := total - 1 - e.globalIdx
		if existing, ok := m[e.name]; !ok || val > existing {
			m[e.name] = val
		}
	}
	return m
}

// importPrecedenceLevels converts raw JSON precedence levels into the Grammar
// IR's PrecEntry format.
func importPrecedenceLevels(rawLevels []json.RawMessage) [][]PrecEntry {
	var levels [][]PrecEntry
	for _, rawLevel := range rawLevels {
		var entries []jsonPrecEntry
		if err := json.Unmarshal(rawLevel, &entries); err != nil {
			continue
		}
		var level []PrecEntry
		for _, e := range entries {
			switch {
			case e.Type == "STRING" && e.Value != "":
				level = append(level, PrecEntry{Name: e.Value})
			case e.Type == "SYMBOL" && e.Name != "":
				level = append(level, PrecEntry{IsSymbol: true, Name: e.Name})
			}
		}
		if len(level) > 0 {
			levels = append(levels, level)
		}
	}
	return levels
}

// grammarJSONExtensions holds gotreesitter-specific settings that extend the
// standard tree-sitter grammar.json under a "gotreesitter" object. tree-sitter's
// own tooling ignores this key; gotreesitter reads it during ImportGrammarJSON
// and writes it back during ExportGrammarJSON (only when set).
type grammarJSONExtensions struct {
	// WantsForest opts the assembled Language into the GSS-forest GLR fast path
	// (see gotreesitter.Language.WantsForest). Lets an existing grammar enable
	// forest declaratively via grammar.json — no code change, no fork.
	WantsForest bool `json:"wantsForest,omitempty"`
}

// jsonGrammar is the top-level structure of a grammar.json file.
type jsonGrammar struct {
	Name          string                       `json:"name"`
	Rules         map[string]json.RawMessage   `json:"rules"`
	Extras        []json.RawMessage            `json:"extras"`
	Conflicts     [][]string                   `json:"conflicts"`
	Externals     []json.RawMessage            `json:"externals"`
	Inline        []string                     `json:"inline"`
	Supertypes    []string                     `json:"supertypes"`
	Word          string                       `json:"word"`
	Reserved      map[string][]json.RawMessage `json:"reserved"`
	Precedences   []json.RawMessage            `json:"precedences"`
	Gotreesitter  *grammarJSONExtensions       `json:"gotreesitter,omitempty"`
	ruleOrder     []string                     // populated during UnmarshalJSON
	reservedOrder []string                     // populated during UnmarshalJSON
}

// jsonPrecEntry is an entry in the precedences array.
type jsonPrecEntry struct {
	Type  string `json:"type"`
	Value string `json:"value"` // for STRING entries
	Name  string `json:"name"`  // for SYMBOL entries
}

// UnmarshalJSON implements custom unmarshaling to preserve rule order.
func (g *jsonGrammar) UnmarshalJSON(data []byte) error {
	// First pass: use a decoder to extract rule order.
	type Alias jsonGrammar
	if err := json.Unmarshal(data, (*Alias)(g)); err != nil {
		return err
	}

	// Second pass: extract rule order from the raw JSON.
	// json.Decoder preserves object key order.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	if rulesRaw, ok := raw["rules"]; ok {
		g.ruleOrder = extractKeyOrder(rulesRaw)
	}
	if reservedRaw, ok := raw["reserved"]; ok {
		g.reservedOrder = extractKeyOrder(reservedRaw)
	}

	return nil
}

// extractKeyOrder extracts the key order from a JSON object.
func extractKeyOrder(data json.RawMessage) []string {
	dec := json.NewDecoder(jsonReader(data))
	t, err := dec.Token()
	if err != nil || t != json.Delim('{') {
		return nil
	}

	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			break
		}
		key, ok := t.(string)
		if !ok {
			break
		}
		keys = append(keys, key)
		// Skip the value.
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			break
		}
	}
	return keys
}

// jsonReader wraps a byte slice to implement io.Reader.
type jsonReaderImpl struct {
	data []byte
	pos  int
}

func jsonReader(data []byte) *jsonReaderImpl {
	return &jsonReaderImpl{data: data}
}

func (r *jsonReaderImpl) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, fmt.Errorf("EOF")
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// jsonRuleNode is the JSON representation of a rule node.
type jsonRuleNode struct {
	Type    string            `json:"type"`
	Value   interface{}       `json:"value"`   // string for STRING/PATTERN, int for PREC*
	Name    string            `json:"name"`    // for SYMBOL, FIELD
	Content json.RawMessage   `json:"content"` // for wrappers (TOKEN, PREC*, REPEAT, etc.)
	Members []json.RawMessage `json:"members"` // for SEQ, CHOICE
	Named   bool              `json:"named"`   // for ALIAS
	Flags   string            `json:"flags"`   // for PATTERN: "i" = case-insensitive
}

// jsonConverter holds context for converting grammar.json rules.
type jsonConverter struct {
	namedPrecs      map[string]int // named precedence → numeric value
	sawReservedNode bool           // tracks whether any RESERVED wrapper was encountered
}

// convertRule converts a grammar.json rule node to a Grammar Rule.
func (c *jsonConverter) convertRule(data json.RawMessage) (*Rule, error) {
	var node jsonRuleNode
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("unmarshal rule: %w", err)
	}

	switch node.Type {
	case "STRING":
		val, ok := node.Value.(string)
		if !ok {
			return nil, fmt.Errorf("STRING value not a string: %v", node.Value)
		}
		return Str(val), nil

	case "PATTERN":
		val, ok := node.Value.(string)
		if !ok {
			return nil, fmt.Errorf("PATTERN value not a string: %v", node.Value)
		}
		if strings.Contains(node.Flags, "i") {
			val = makeCaseInsensitivePattern(val)
		}
		return Pat(val), nil

	case "SYMBOL":
		return Sym(node.Name), nil

	case "BLANK":
		return Blank(), nil

	case "SEQ":
		children, err := c.convertRuleList(node.Members)
		if err != nil {
			return nil, fmt.Errorf("SEQ: %w", err)
		}
		return Seq(children...), nil

	case "CHOICE":
		children, err := c.convertRuleList(node.Members)
		if err != nil {
			return nil, fmt.Errorf("CHOICE: %w", err)
		}
		return Choice(children...), nil

	case "REPEAT":
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("REPEAT: %w", err)
		}
		return Repeat(child), nil

	case "REPEAT1":
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("REPEAT1: %w", err)
		}
		return Repeat1(child), nil

	case "TOKEN":
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("TOKEN: %w", err)
		}
		return Token(child), nil

	case "IMMEDIATE_TOKEN":
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("IMMEDIATE_TOKEN: %w", err)
		}
		return ImmToken(child), nil

	case "PREC":
		return c.convertPrecRule(node, func(n int, r *Rule) *Rule {
			return Prec(n, r)
		})

	case "PREC_LEFT":
		return c.convertPrecRule(node, func(n int, r *Rule) *Rule {
			return PrecLeft(n, r)
		})

	case "PREC_RIGHT":
		return c.convertPrecRule(node, func(n int, r *Rule) *Rule {
			return PrecRight(n, r)
		})

	case "PREC_DYNAMIC":
		return c.convertPrecRule(node, func(n int, r *Rule) *Rule {
			return PrecDynamic(n, r)
		})

	case "FIELD":
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("FIELD: %w", err)
		}
		return Field(node.Name, child), nil

	case "ALIAS":
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("ALIAS: %w", err)
		}
		val, ok := node.Value.(string)
		if !ok {
			return nil, fmt.Errorf("ALIAS value not a string: %v", node.Value)
		}
		return Alias(child, val, node.Named), nil

	case "RESERVED":
		// RESERVED wraps a rule with context-dependent keyword reservation.
		// We unwrap it — the structural grammar encodes the context, and our
		// runtime parser handles keyword promotion separately. We track this
		// so we can tell whether per-context reserved sets are actually used.
		c.sawReservedNode = true
		child, err := c.convertRule(node.Content)
		if err != nil {
			return nil, fmt.Errorf("RESERVED: %w", err)
		}
		return child, nil

	default:
		return nil, fmt.Errorf("unsupported rule type %q", node.Type)
	}
}

// convertPrecRule handles PREC/PREC_LEFT/PREC_RIGHT/PREC_DYNAMIC nodes.
// Resolves named precedence strings to numeric values via the namedPrecs map.
func (c *jsonConverter) convertPrecRule(node jsonRuleNode, make_ func(int, *Rule) *Rule) (*Rule, error) {
	prec := 0
	switch v := node.Value.(type) {
	case float64:
		prec = int(v)
	case int:
		prec = v
	case string:
		if val, ok := c.namedPrecs[v]; ok {
			prec = val
		}
	}

	child, err := c.convertRule(node.Content)
	if err != nil {
		return nil, err
	}
	return make_(prec, child), nil
}

// convertRuleList converts a list of JSON rule nodes to Rules.
func (c *jsonConverter) convertRuleList(members []json.RawMessage) ([]*Rule, error) {
	rules := make([]*Rule, len(members))
	for i, m := range members {
		r, err := c.convertRule(m)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		rules[i] = r
	}
	return rules, nil
}

// makeCaseInsensitivePattern converts a regex pattern to case-insensitive form
// by expanding ASCII letters to character classes. For example, "DUP" becomes
// "[Dd][Uu][Pp]". Characters inside character classes are expanded in-place.
func makeCaseInsensitivePattern(pattern string) string {
	var b strings.Builder
	b.Grow(len(pattern) * 4)
	inClass := false
	escaped := false
	for _, ch := range pattern {
		if escaped {
			b.WriteRune(ch)
			escaped = false
			continue
		}
		if ch == '\\' {
			b.WriteRune(ch)
			escaped = true
			continue
		}
		if ch == '[' {
			inClass = true
			b.WriteRune(ch)
			continue
		}
		if ch == ']' {
			inClass = false
			b.WriteRune(ch)
			continue
		}
		if isASCIILetter(ch) {
			if inClass {
				// Inside a character class, add both cases
				lo := rune(ch | 0x20)
				up := rune(ch &^ 0x20)
				b.WriteRune(lo)
				b.WriteRune(up)
			} else {
				// Outside a class, wrap in a class
				lo := rune(ch | 0x20)
				up := rune(ch &^ 0x20)
				b.WriteRune('[')
				b.WriteRune(lo)
				b.WriteRune(up)
				b.WriteRune(']')
			}
		} else {
			b.WriteRune(ch)
		}
	}
	return b.String()
}

func isASCIILetter(ch rune) bool {
	return (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}
