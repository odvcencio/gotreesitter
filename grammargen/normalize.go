package grammargen

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Assoc is the associativity of a production.
type Assoc int

const (
	AssocNone Assoc = iota
	AssocLeft
	AssocRight
)

// SymbolKind classifies a grammar symbol.
type SymbolKind int

const (
	SymbolTerminal    SymbolKind = iota // anonymous terminal like "{"
	SymbolNamedToken                    // named terminal like number, string_content
	SymbolExternal                      // external scanner token
	SymbolNonterminal                   // nonterminal rule
)

// SymbolInfo describes a grammar symbol.
type SymbolInfo struct {
	Name      string
	Visible   bool
	Named     bool
	Supertype bool
	Kind      SymbolKind
	IsExtra   bool
	Immediate bool // token.immediate — no preceding whitespace skip
}

// Production is a single LHS → RHS production with metadata.
type Production struct {
	LHS          int   // symbol index
	RHS          []int // symbol indices
	Prec         int
	Assoc        Assoc
	DynPrec      int
	ProductionID int
	Fields       []FieldAssign // per-RHS-position field assignments
	Aliases      []AliasInfo   // per-RHS-position alias info
	IsExtra      bool          // true if this production belongs to a nonterminal extra
}

// FieldAssign maps a child position in a production to a field name.
type FieldAssign struct {
	ChildIndex int
	FieldName  string
}

// AliasInfo stores alias information for a child position.
type AliasInfo struct {
	ChildIndex int
	Name       string
	Named      bool
}

// TerminalPattern describes a terminal symbol's match pattern for DFA generation.
type TerminalPattern struct {
	SymbolID  int
	Rule      *Rule // the flattened rule tree for NFA construction
	Priority  int   // lower = higher priority (wins on tie)
	Immediate bool  // token.immediate
}

// NormalizedGrammar is the output of the normalize step.
type NormalizedGrammar struct {
	Symbols       []SymbolInfo
	Productions   []Production
	Terminals     []TerminalPattern
	ExtraSymbols  []int    // symbol indices of extras
	FieldNames    []string // index 0 is always ""
	Conflicts     [][]int  // symbol index groups
	Supertypes    []int    // symbol indices
	StartSymbol   int
	AugmentProdID int // production index for S' → S

	// Keyword support (populated when Grammar.Word is set).
	KeywordSymbols []int             // symbol IDs that are keywords
	WordSymbolID   int               // word token symbol ID (e.g., identifier)
	KeywordEntries []TerminalPattern // keyword patterns for keyword DFA

	// External scanner support (populated when Grammar.Externals is set).
	ExternalSymbols []int // external token index → symbol ID

	// conflictCache is built lazily by LR conflict resolution so repeated
	// resolveActionConflict calls can reuse the same reverse indexes.
	conflictCache *conflictResolutionCache
}

// symbolTable is used during normalization.
type symbolTable struct {
	byName        map[string]int // terminal name → symbol ID
	nontermByName map[string]int // nonterminal name → symbol ID
	symbols       []SymbolInfo
	nextID        int
	fieldMap      map[string]int
	fields        []string
}

func newSymbolTable() *symbolTable {
	st := &symbolTable{
		byName:        make(map[string]int),
		nontermByName: make(map[string]int),
		fieldMap:      make(map[string]int),
		fields:        []string{""}, // index 0 is always ""
	}
	// Symbol 0 = "end" (EOF)
	st.addSymbol("end", SymbolInfo{
		Name:    "end",
		Visible: false,
		Named:   false,
		Kind:    SymbolTerminal,
	})
	return st
}

func (st *symbolTable) addSymbol(name string, info SymbolInfo) int {
	isNonterm := info.Kind == SymbolNonterminal

	if isNonterm {
		// Nonterminals use a separate namespace. A nonterminal named "type"
		// and a string literal "type" are distinct symbols.
		if id, ok := st.nontermByName[name]; ok {
			return id
		}
		id := len(st.symbols)
		st.nontermByName[name] = id
		st.symbols = append(st.symbols, info)
		// Also register in byName if no terminal with this name exists,
		// so Sym("type") lookups work when there's no collision.
		if _, exists := st.byName[name]; !exists {
			st.byName[name] = id
		}
		return id
	}

	// Terminals (anonymous, named tokens, externals).
	if id, ok := st.byName[name]; ok {
		// Symbol 0 is reserved for EOF ("end"). Never reuse it for a
		// grammar terminal (e.g., jq's "end" keyword).
		if id == 0 {
			newID := len(st.symbols)
			st.byName[name] = newID
			st.symbols = append(st.symbols, info)
			return newID
		}
		// If re-registering as a named token (e.g., true: "true"),
		// upgrade the existing entry from anonymous to named,
		// but only if it's still a terminal (not a nonterminal).
		if info.Named && !st.symbols[id].Named && st.symbols[id].Kind != SymbolNonterminal {
			st.symbols[id].Named = true
			st.symbols[id].Kind = info.Kind
		}
		return id
	}
	id := len(st.symbols)
	st.byName[name] = id
	st.symbols = append(st.symbols, info)
	return id
}

func (st *symbolTable) getOrAdd(name string, info SymbolInfo) int {
	return st.addSymbol(name, info)
}

// lookup returns the symbol ID for a name. For ambiguous names where both
// a terminal and nonterminal exist, it returns the terminal (use lookupNonterm
// for nonterminals).
func (st *symbolTable) lookup(name string) (int, bool) {
	id, ok := st.byName[name]
	return id, ok
}

// lookupNonterm returns the nonterminal symbol ID. Falls back to byName
// for named tokens that are treated like nonterminals in Sym() references.
func (st *symbolTable) lookupNonterm(name string) (int, bool) {
	if id, ok := st.nontermByName[name]; ok {
		return id, ok
	}
	return st.lookup(name)
}

func (st *symbolTable) fieldID(name string) int {
	if id, ok := st.fieldMap[name]; ok {
		return id
	}
	id := len(st.fields)
	st.fieldMap[name] = id
	st.fields = append(st.fields, name)
	return id
}

// Normalize transforms a Grammar into a NormalizedGrammar.
func Normalize(g *Grammar) (*NormalizedGrammar, error) {
	if len(g.RuleOrder) == 0 {
		return nil, fmt.Errorf("grammar has no rules")
	}

	// Shallow-clone g so later phases (e.g. liftInlineTokens) that write
	// back into g.Rules don't mutate the caller's Grammar.
	gCopy := *g
	gCopy.Rules = make(map[string]*Rule, len(g.Rules))
	for k, v := range g.Rules {
		gCopy.Rules[k] = v
	}
	g = &gCopy

	// Phase 0: Expand inline rules. Rules listed in Grammar.Inline are replaced
	// at all usage sites with their rule body, then removed as nonterminals.
	// This must happen before symbol assignment since inline rules don't get IDs.
	if len(g.Inline) > 0 {
		g = expandInlineRules(g)
	}

	// Phase 0b: Flatten hidden rule pass-through alternatives.
	// Tree-sitter C's flatten_grammar.cc auto-inlines single-symbol alternatives
	// of hidden nonterminals into parent rules' choice contexts. This removes
	// cc=1 productions from hidden rules and distributes them as direct children
	// in parent rules. Without this, grammargen generates extra cc=1 reduce
	// actions that tree-sitter C doesn't have (~10 grammars affected).
	g = flattenHiddenChoiceAlts(g)

	st := newSymbolTable()
	ng := &NormalizedGrammar{}

	// Phase 1: Collect all string literals and register terminal symbols.
	// Walk all rules to find string literals (anonymous terminals).
	stringLiterals := collectStringLiterals(g)
	for _, s := range stringLiterals {
		st.addSymbol(s, SymbolInfo{
			Name:    escapeAnonymousName(s),
			Visible: true,
			Named:   false,
			Kind:    SymbolTerminal,
		})
	}

	// Phase 1b: Collect inline patterns (regex nodes inside non-terminal rules
	// that are NOT wrapped in token()). These become anonymous terminal symbols.
	// Unlike string literals (which are Visible=true), inline patterns are
	// Visible=false to match tree-sitter C behavior: pattern alternatives inside
	// nonterminal rules (e.g. core: choice(/DUP/i, /DROP/i, ...)) produce
	// invisible child tokens. The parent nonterminal thus has 0 visible children,
	// matching the reference parser's child count.
	inlinePatterns := collectInlinePatterns(g)
	for _, pat := range inlinePatterns {
		name := pat // use pattern value as key for lookup
		if _, ok := st.lookup(name); ok {
			continue // already registered
		}
		st.addSymbol(name, SymbolInfo{
			Name:    name,
			Visible: false,
			Named:   false,
			Kind:    SymbolTerminal,
		})
	}

	// Phase 1c: Lift inline Token/ImmToken nodes from nonterminal rules.
	// These become anonymous terminal symbols (e.g. _rule_token1) so they
	// get terminal IDs before nonterminals are registered.
	inlineTokens := liftInlineTokens(g, st)

	// Phase 2: Register named terminals (rules that are token() or token.immediate()
	// or simple patterns, and rules that resolve to string literals like "true").
	// Also register nonterminals.
	namedTokens, nonterminals := classifyRules(g)

	for _, name := range namedTokens {
		visible := !strings.HasPrefix(name, "_")
		displayName := name
		named := true
		kind := SymbolKind(SymbolNamedToken)
		// Hidden named tokens that are pure STRING literals should be treated
		// as anonymous visible terminals matching tree-sitter's behavior:
		// _end = "/" becomes the visible "/" terminal, not an invisible _end token.
		if !visible {
			if rule, ok := g.Rules[name]; ok && rule != nil && rule.Kind == RuleString {
				displayName = rule.Value
				visible = true
				named = false
				kind = SymbolTerminal
			}
		}
		st.addSymbol(name, SymbolInfo{
			Name:    displayName,
			Visible: visible,
			Named:   named,
			Kind:    kind,
		})
	}

	// Phase 2b: Register extra terminal symbols (e.g. whitespace pattern)
	// BEFORE nonterminals so all terminals have contiguous low IDs.
	registerExtraTerminals(g, st)

	// Phase 2c: Register external scanner symbols.
	var externalSymbols []int
	if len(g.Externals) > 0 {
		externalSymbols = registerExternalSymbols(g, st)
	}

	// Record token count (terminals end here, before nonterminals).
	tokenCount := len(st.symbols)

	// Phase 3: Register nonterminal symbols.
	for _, name := range nonterminals {
		visible := !strings.HasPrefix(name, "_")
		isSupertype := false
		for _, s := range g.Supertypes {
			if s == name {
				isSupertype = true
				break
			}
		}
		// Supertypes are transparent wrappers — tree-sitter marks them
		// Visible=false so they don't appear as explicit tree nodes.
		if isSupertype {
			visible = false
		}
		st.addSymbol(name, SymbolInfo{
			Name:      name,
			Visible:   visible,
			Named:     true,
			Kind:      SymbolNonterminal,
			Supertype: isSupertype,
		})
	}

	// Phase 4: Pre-process rules — expand Optional, lift Repeat/Repeat1
	// into auxiliary nonterminals at ALL levels (including top-level).
	auxCounter := 0
	processedRules := make(map[string]*Rule)
	auxRules := make(map[string]*Rule)
	auxOrigin := make(map[string]string) // aux rule name → originating grammar rule name

	for _, name := range nonterminals {
		rule := g.Rules[name]
		if rule == nil {
			continue
		}
		// When a hidden rule's entire body is repeat/repeat1, expand to a
		// self-referencing choice BEFORE prepareRule, matching tree-sitter's
		// behavior (tree-sitter makes the rule self-recursive rather than
		// creating a separate aux rule).
		rule = expandTopLevelRepeat(cloneRule(rule), name)
		prevAuxCount := len(auxRules)
		processed := prepareRule(rule, name, st, auxRules, &auxCounter)
		processedRules[name] = processed
		// Track which grammar rule each new auxiliary rule originates from.
		if len(auxRules) > prevAuxCount {
			for auxName := range auxRules {
				if _, tracked := auxOrigin[auxName]; !tracked {
					auxOrigin[auxName] = name
				}
			}
		}
	}

	// Phase 5: Mark extra symbols.
	extraSymbols := resolveExtras(g, st)
	for _, eid := range extraSymbols {
		st.symbols[eid].IsExtra = true
	}

	// Phase 5b: Identify keywords when a word token is declared.
	var keywordSet map[int]bool
	var keywordSymbols []int
	var keywordEntries []TerminalPattern
	var wordSymbolID int
	if g.Word != "" {
		wordSymbolID, _ = st.lookup(g.Word)
		keywordSet, keywordSymbols, keywordEntries = identifyKeywords(g, st, stringLiterals)
	}

	// Phase 6: Extract terminal patterns for DFA generation.
	terminals, err := extractTerminals(g, st, stringLiterals, namedTokens, inlinePatterns, inlineTokens, keywordSet)
	if err != nil {
		return nil, fmt.Errorf("extract terminals: %w", err)
	}

	// Phase 7: Extract productions from each nonterminal rule.
	var productions []Production
	prodIDCounter := 0

	// Add augmented start production: S' → startRule
	startName := g.RuleOrder[0]
	startSym, _ := st.lookupNonterm(startName)
	augStartSym := st.addSymbol("_start", SymbolInfo{
		Name:    "_start",
		Visible: false,
		Named:   false,
		Kind:    SymbolNonterminal,
	})

	augProd := Production{
		LHS:          augStartSym,
		RHS:          []int{startSym},
		ProductionID: prodIDCounter,
	}
	productions = append(productions, augProd)
	prodIDCounter++

	// Extract productions for each nonterminal rule.
	for _, name := range nonterminals {
		rule := processedRules[name]
		if rule == nil {
			continue
		}
		symID, _ := st.lookupNonterm(name)
		prods := flattenRule2(rule, symID, st, &prodIDCounter)
		productions = append(productions, prods...)
	}

	// Extract productions for auxiliary rules.
	// Sort by originating grammar rule's definition order first, then by name.
	// This ensures that auxiliary rules from earlier-defined grammar rules get
	// lower production indices, matching tree-sitter's conflict resolution behavior.
	ruleOrderIdx := make(map[string]int, len(nonterminals))
	for i, name := range nonterminals {
		ruleOrderIdx[name] = i
	}
	auxNames := make([]string, 0, len(auxRules))
	for name := range auxRules {
		auxNames = append(auxNames, name)
	}
	sort.Slice(auxNames, func(i, j int) bool {
		oi := ruleOrderIdx[auxOrigin[auxNames[i]]]
		oj := ruleOrderIdx[auxOrigin[auxNames[j]]]
		if oi != oj {
			return oi < oj
		}
		return auxNames[i] < auxNames[j]
	})
	for _, name := range auxNames {
		rule := auxRules[name]
		symID, _ := st.lookupNonterm(name)
		prods := flattenRule2(rule, symID, st, &prodIDCounter)
		productions = append(productions, prods...)
	}

	// Phase 7b: Deduplicate productions.
	// enumerateAlternatives can produce duplicate productions when Choice
	// alternatives overlap after expansion. Tree-sitter deduplicates these.
	// Extra duplicates cause spurious reduce-reduce conflicts.
	productions = deduplicateProductions(productions)

	// Phase 8: Resolve conflicts.
	var conflicts [][]int
	for _, cgroup := range g.Conflicts {
		var syms []int
		for _, name := range cgroup {
			if id, ok := st.lookupNonterm(name); ok {
				syms = append(syms, id)
			}
		}
		conflicts = append(conflicts, syms)
	}

	// Phase 9: Resolve supertypes.
	var supertypes []int
	for _, name := range g.Supertypes {
		if id, ok := st.lookupNonterm(name); ok {
			supertypes = append(supertypes, id)
		}
	}

	// Mark productions belonging to nonterminal extras.
	extraNTSet := make(map[int]bool)
	for _, e := range extraSymbols {
		if e >= tokenCount {
			extraNTSet[e] = true
		}
	}
	if len(extraNTSet) > 0 {
		for i := range productions {
			if extraNTSet[productions[i].LHS] {
				productions[i].IsExtra = true
			}
		}
	}

	canonicalizeAliasedExternalSymbols(st.symbols, productions, externalSymbols)

	ng.Symbols = st.symbols
	ng.Productions = productions
	ng.Terminals = terminals
	ng.ExtraSymbols = extraSymbols
	ng.FieldNames = st.fields
	ng.Conflicts = conflicts
	ng.Supertypes = supertypes
	ng.StartSymbol = augStartSym
	ng.AugmentProdID = 0
	ng.KeywordSymbols = keywordSymbols
	ng.WordSymbolID = wordSymbolID
	ng.KeywordEntries = keywordEntries
	ng.ExternalSymbols = externalSymbols

	// Set tokenCount boundary on symbols so assembly knows where terminals end.
	_ = tokenCount

	return ng, nil
}

func canonicalizeAliasedExternalSymbols(symbols []SymbolInfo, productions []Production, externalSymbols []int) {
	if len(externalSymbols) == 0 {
		return
	}

	type aliasShape struct {
		name  string
		named bool
	}

	for _, symID := range externalSymbols {
		if symID < 0 || symID >= len(symbols) || symbols[symID].Kind != SymbolExternal {
			continue
		}

		var canonical aliasShape
		hasCanonical := false
		seenUse := false
		ok := true

		for _, prod := range productions {
			for childIdx, rhsSym := range prod.RHS {
				if rhsSym != symID {
					continue
				}
				seenUse = true
				var alias *AliasInfo
				for i := range prod.Aliases {
					if prod.Aliases[i].ChildIndex == childIdx {
						alias = &prod.Aliases[i]
						break
					}
				}
				if alias == nil {
					ok = false
					break
				}
				cur := aliasShape{name: alias.Name, named: alias.Named}
				if !hasCanonical {
					canonical = cur
					hasCanonical = true
					continue
				}
				if canonical != cur {
					ok = false
					break
				}
			}
			if !ok {
				break
			}
		}

		if !ok || !seenUse || !hasCanonical || canonical.name == "" {
			continue
		}

		symbols[symID].Name = canonical.name
		symbols[symID].Named = canonical.named
		symbols[symID].Visible = !strings.HasPrefix(canonical.name, "_")
	}
}

// TokenCount returns the number of terminal symbols (including symbol 0 = end).
func (ng *NormalizedGrammar) TokenCount() int {
	count := 0
	for _, s := range ng.Symbols {
		if s.Kind == SymbolTerminal || s.Kind == SymbolNamedToken || s.Kind == SymbolExternal {
			count++
		}
	}
	return count
}

// collectStringLiterals walks all rules and collects unique string literals
// in order of first appearance.
func collectStringLiterals(g *Grammar) []string {
	seen := make(map[string]bool)
	var result []string

	var walk func(r *Rule, inToken bool)
	walk = func(r *Rule, inToken bool) {
		if r == nil {
			return
		}
		switch r.Kind {
		case RuleString:
			if !inToken && !seen[r.Value] {
				seen[r.Value] = true
				result = append(result, r.Value)
			}
		case RuleToken, RuleImmToken:
			// String literals inside token() are part of the token pattern,
			// not standalone terminals.
			for _, c := range r.Children {
				walk(c, true)
			}
			return
		}
		for _, c := range r.Children {
			walk(c, inToken)
		}
	}

	// Walk extras first (they may contain patterns).
	for _, e := range g.Extras {
		walk(e, false)
	}
	// Walk rules in definition order.
	for _, name := range g.RuleOrder {
		walk(g.Rules[name], false)
	}
	return result
}

// collectInlinePatterns walks all non-terminal rules and collects RulePattern
// nodes that appear inline (not inside Token() wrappers and not as top-level
// terminal rules). These anonymous regex patterns need their own terminal symbols.
func collectInlinePatterns(g *Grammar) []string {
	seen := make(map[string]bool)
	var result []string

	var walk func(r *Rule, inToken bool)
	walk = func(r *Rule, inToken bool) {
		if r == nil {
			return
		}
		switch r.Kind {
		case RulePattern:
			if !inToken && !seen[r.Value] {
				seen[r.Value] = true
				result = append(result, r.Value)
			}
			return
		case RuleToken, RuleImmToken:
			// Patterns inside token() are handled as part of the token, not inline.
			return
		}
		for _, c := range r.Children {
			walk(c, inToken)
		}
	}

	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if !isTerminalRule(rule) {
			walk(rule, false)
		}
	}
	// NOTE: We intentionally skip walking g.Extras here.
	// Pattern extras (like /\s/) are handled by registerExtraTerminals which
	// creates the _whitespace symbol. Walking extras here would create a
	// DUPLICATE terminal (e.g., both "\s" and "_whitespace") for the same
	// pattern, inflating TokenCount. Symbol extras (like comment) are
	// nonterminals resolved via resolveExtras.
	return result
}

// classifyRules separates rule names into named tokens (terminal rules)
// and nonterminals. A rule is a "named token" if its definition is:
//   - wrapped in token() or token.immediate()
//   - a pattern
//   - a string literal ONLY when no other rule shares the same string value
//     (if multiple named rules define the same STRING, or the STRING is used
//     inline in nonterminal rules, the named rule becomes a nonterminal
//     wrapping the shared anonymous terminal — matching tree-sitter C behavior).
func classifyRules(g *Grammar) (tokens, nonterms []string) {
	// Count how many distinct sources each STRING value has.
	// Sources: named bare-STRING rules + inline STRING usage in nonterminal rules.
	sharedStrings := computeSharedStrings(g)

	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if isTerminalRule(rule) {
			// Check if this is a bare STRING rule whose value is shared.
			if sv := terminalStringValue(rule); sv != "" && sharedStrings[sv] {
				nonterms = append(nonterms, name)
			} else {
				tokens = append(tokens, name)
			}
		} else {
			nonterms = append(nonterms, name)
		}
	}
	return
}

// computeSharedStrings identifies STRING values that are used by multiple
// sources. A STRING value is "shared" when it appears in more than one named
// bare-STRING rule, or when it appears both in a named bare-STRING rule AND
// as an inline string in a nonterminal rule.
func computeSharedStrings(g *Grammar) map[string]bool {
	// Count named bare-STRING rules per string value.
	namedUses := make(map[string]int)
	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if sv := terminalStringValue(rule); sv != "" {
			namedUses[sv]++
		}
	}

	// Count inline STRING usage in nonterminal rules.
	inlineUses := make(map[string]bool)
	var walkInline func(r *Rule, inToken bool)
	walkInline = func(r *Rule, inToken bool) {
		if r == nil {
			return
		}
		switch r.Kind {
		case RuleString:
			if !inToken {
				inlineUses[r.Value] = true
			}
			return
		case RuleToken, RuleImmToken:
			return // strings inside token() don't count as separate inline usage
		}
		for _, c := range r.Children {
			walkInline(c, inToken)
		}
	}
	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if !isTerminalRule(rule) {
			walkInline(rule, false)
		}
	}
	for _, e := range g.Extras {
		walkInline(e, false)
	}

	shared := make(map[string]bool)
	for sv, count := range namedUses {
		if count > 1 || inlineUses[sv] {
			shared[sv] = true
		}
	}
	return shared
}

// terminalStringValue returns the string value of a bare STRING terminal rule
// (including through prec wrappers). Returns "" for non-STRING terminals
// (TOKEN, PATTERN, etc.) or non-terminal rules.
func terminalStringValue(r *Rule) string {
	if r == nil {
		return ""
	}
	switch r.Kind {
	case RuleString:
		return r.Value
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			return terminalStringValue(r.Children[0])
		}
	}
	return ""
}

// isTerminalRule returns true if the rule defines a terminal token.
func isTerminalRule(r *Rule) bool {
	if r == nil {
		return false
	}
	switch r.Kind {
	case RuleString:
		return true
	case RulePattern:
		return true
	case RuleToken, RuleImmToken:
		return true
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			return isTerminalRule(r.Children[0])
		}
	}
	return false
}

// inlineTokenEntry stores information about an inline Token/ImmToken found
// inside a nonterminal rule tree. These need anonymous terminal symbols.
type inlineTokenEntry struct {
	name      string // anonymous terminal name, e.g. "_rule_token1"
	rule      *Rule  // the original Token/ImmToken node (for pattern extraction)
	immediate bool   // true if ImmToken
}

// liftInlineTokens walks nonterminal rules in the grammar, finds inline
// Token/ImmToken nodes (not at the rule top level), registers anonymous
// terminal symbols for them, and replaces them with Sym references.
// This must run before tokenCount is recorded so inline tokens get terminal IDs.
//
// Inline tokens with identical patterns are deduplicated to share a single
// symbol, matching tree-sitter C behavior. Without this, synonym tokens
// (e.g. _variable_assignment_token4 and _RECIPEPREFIX_assignment_token2 both
// matching "\n") cause parse failures when the DFA picks the wrong synonym
// and the parser can't find an action for it after a reduce chain.
func liftInlineTokens(g *Grammar, st *symbolTable) []inlineTokenEntry {
	var entries []inlineTokenEntry
	counter := make(map[string]int)  // per-parent-rule counters
	dedup := make(map[string]string) // canonical pattern key → symbol name

	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if isTerminalRule(rule) {
			continue
		}
		g.Rules[name] = liftTokensInRule(rule, name, st, &entries, counter, dedup)
	}

	return entries
}

// canonicalTokenKey computes a canonical string key for an inline token's
// pattern, used to deduplicate tokens with identical matching behavior.
// Precedence wrappers are stripped since they affect conflict resolution,
// not pattern matching. Token vs ImmToken are distinguished since they
// have different lexing semantics.
func canonicalTokenKey(r *Rule) string {
	if r == nil {
		return ""
	}
	var sb strings.Builder
	switch r.Kind {
	case RuleToken:
		sb.WriteString("T:")
	case RuleImmToken:
		sb.WriteString("I:")
	default:
		return ""
	}
	if len(r.Children) > 0 {
		writeCanonicalInner(r.Children[0], &sb)
	}
	return sb.String()
}

// writeCanonicalInner writes a canonical representation of a rule subtree,
// stripping precedence/field wrappers that don't affect pattern matching.
func writeCanonicalInner(r *Rule, sb *strings.Builder) {
	if r == nil {
		return
	}
	switch r.Kind {
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			writeCanonicalInner(r.Children[len(r.Children)-1], sb)
		}
	case RuleField, RuleAlias:
		if len(r.Children) > 0 {
			writeCanonicalInner(r.Children[0], sb)
		}
	case RuleString:
		sb.WriteString("s:")
		sb.WriteString(r.Value)
	case RulePattern:
		sb.WriteString("p:")
		sb.WriteString(r.Value)
	case RuleSeq:
		sb.WriteString("q(")
		for i, c := range r.Children {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeCanonicalInner(c, sb)
		}
		sb.WriteByte(')')
	case RuleChoice:
		sb.WriteString("c(")
		for i, c := range r.Children {
			if i > 0 {
				sb.WriteByte(',')
			}
			writeCanonicalInner(c, sb)
		}
		sb.WriteByte(')')
	case RuleBlank:
		sb.WriteString("b")
	case RuleRepeat:
		sb.WriteString("*(")
		if len(r.Children) > 0 {
			writeCanonicalInner(r.Children[0], sb)
		}
		sb.WriteByte(')')
	case RuleRepeat1:
		sb.WriteString("+(")
		if len(r.Children) > 0 {
			writeCanonicalInner(r.Children[0], sb)
		}
		sb.WriteByte(')')
	case RuleSymbol:
		sb.WriteString("r:")
		sb.WriteString(r.Value)
	default:
		fmt.Fprintf(sb, "?%d:%s", r.Kind, r.Value)
	}
}

// liftTokensInRule recursively walks a rule tree, replacing inline Token/ImmToken
// nodes with Sym references to newly-registered anonymous terminal symbols.
// Tokens with identical patterns are deduplicated via the dedup map.
func liftTokensInRule(r *Rule, parentName string, st *symbolTable, entries *[]inlineTokenEntry, counter map[string]int, dedup map[string]string) *Rule {
	if r == nil {
		return r
	}

	switch r.Kind {
	case RuleToken, RuleImmToken:
		// Inline Token/ImmToken inside a nonterminal rule.

		// For non-immediate Token wrapping a simple STRING (possibly through
		// prec wrappers), reuse the bare string symbol if it was already
		// registered in Phase 1. This matches tree-sitter C which unifies
		// token("x") and token(prec(N, "x")) with the bare "x" terminal.
		if r.Kind == RuleToken {
			if sv := extractTokenStringValue(r); sv != "" {
				if _, exists := st.lookup(sv); exists {
					key := canonicalTokenKey(r)
					dedup[key] = sv
					return Sym(sv)
				}
			}
		}

		// Check if an identical pattern was already registered.
		key := canonicalTokenKey(r)
		if existingName, ok := dedup[key]; ok {
			return Sym(existingName)
		}

		// Create an anonymous terminal symbol for it.
		// Visibility matches tree-sitter: STRING-based tokens are visible
		// (delimiters like quotes, brackets), PATTERN-based tokens are invisible
		// (internal content matchers).
		counter[parentName]++
		visible := isStringOnlyToken(r)
		regKey := fmt.Sprintf("_%s_token%d", parentName, counter[parentName])
		displayName := regKey
		if visible {
			if s := extractTokenStringValue(r); s != "" {
				displayName = escapeAnonymousName(s)
			}
		}

		st.addSymbol(regKey, SymbolInfo{
			Name:    displayName,
			Visible: visible,
			Named:   false,
			Kind:    SymbolTerminal,
		})

		dedup[key] = regKey

		*entries = append(*entries, inlineTokenEntry{
			name:      regKey,
			rule:      r,
			immediate: r.Kind == RuleImmToken,
		})

		return Sym(regKey)

	case RuleString, RulePattern, RuleSymbol, RuleBlank:
		// Leaf nodes — no Token/ImmToken inside.
		return r
	}

	// Recurse into children.
	for i, c := range r.Children {
		r.Children[i] = liftTokensInRule(c, parentName, st, entries, counter, dedup)
	}
	return r
}

// isStringOnlyToken returns true if a Token/ImmToken wraps a STRING literal
// (possibly through prec wrappers). Such tokens represent visible delimiters
// (quotes, brackets) that appear as children in the parse tree.
func isStringOnlyToken(r *Rule) bool {
	if r == nil {
		return false
	}
	// Unwrap Token/ImmToken wrapper
	if r.Kind == RuleToken || r.Kind == RuleImmToken {
		if len(r.Children) > 0 {
			return isStringOnlyToken(r.Children[0])
		}
		return false
	}
	// Unwrap precedence and alias wrappers
	if r.Kind == RulePrec || r.Kind == RulePrecLeft || r.Kind == RulePrecRight || r.Kind == RulePrecDynamic || r.Kind == RuleAlias {
		if len(r.Children) > 0 {
			return isStringOnlyToken(r.Children[0])
		}
		return false
	}
	return r.Kind == RuleString
}

// extractTokenStringValue returns the string literal value from a Token/ImmToken
// that wraps a STRING, or "" if it's not a simple string token.
func extractTokenStringValue(r *Rule) string {
	if r == nil {
		return ""
	}
	if r.Kind == RuleToken || r.Kind == RuleImmToken ||
		r.Kind == RulePrec || r.Kind == RulePrecLeft || r.Kind == RulePrecRight || r.Kind == RulePrecDynamic ||
		r.Kind == RuleAlias {
		if len(r.Children) > 0 {
			return extractTokenStringValue(r.Children[0])
		}
		return ""
	}
	if r.Kind == RuleString {
		return r.Value
	}
	return ""
}

// escapeAnonymousName escapes special characters in anonymous terminal display
// names to match tree-sitter C behavior. Currently only ? is escaped to \?.
func escapeAnonymousName(s string) string {
	return strings.ReplaceAll(s, "?", `\?`)
}

// prepareRule normalizes a rule tree for production extraction:
// - Expands Optional(x) → Choice(x, Blank())
// - Replaces Repeat(x) and Repeat1(x) with auxiliary nonterminal symbols
// This handles repeat/repeat1 at ALL levels including the root.
func prepareRule(r *Rule, parentName string, st *symbolTable, auxRules map[string]*Rule, counter *int) *Rule {
	if r == nil {
		return r
	}
	// Don't descend into token boundaries.
	if r.Kind == RuleToken || r.Kind == RuleImmToken {
		return r
	}

	// Handle the current node.
	switch r.Kind {
	case RuleRepeat:
		// repeat(x) = optional(repeat1(x)) — matches tree-sitter's lowering.
		// Creates a repeat1-style aux rule (always matches at least one x),
		// then wraps the reference in choice(aux, blank()) so the parent
		// gets both "with repeat" and "without repeat" production variants.
		*counter++
		auxName := fmt.Sprintf("_%s_repeat%d", parentName, *counter)
		if _, exists := st.lookupNonterm(auxName); !exists {
			st.addSymbol(auxName, SymbolInfo{
				Name: auxName, Visible: false, Named: false, Kind: SymbolNonterminal,
			})
			inner := r.Children[0]
			preparedInner := prepareRule(cloneRule(inner), parentName, st, auxRules, counter)
			auxRules[auxName] = Choice(
				Seq(Sym(auxName), cloneRule(preparedInner)),
				cloneRule(preparedInner),
			)
		}
		return Choice(Sym(auxName), Blank())

	case RuleRepeat1:
		*counter++
		auxName := fmt.Sprintf("_%s_repeat1_%d", parentName, *counter)
		if _, exists := st.lookupNonterm(auxName); !exists {
			st.addSymbol(auxName, SymbolInfo{
				Name: auxName, Visible: false, Named: false, Kind: SymbolNonterminal,
			})
			inner := r.Children[0]
			preparedInner := prepareRule(cloneRule(inner), parentName, st, auxRules, counter)
			auxRules[auxName] = Choice(
				Seq(Sym(auxName), cloneRule(preparedInner)),
				cloneRule(preparedInner),
			)
		}
		return Sym(auxName)

	case RuleOptional:
		// optional(x) → choice(x, blank)
		inner := prepareRule(r.Children[0], parentName, st, auxRules, counter)
		return Choice(inner, Blank())

	}

	// Recurse into children.
	for i, c := range r.Children {
		r.Children[i] = prepareRule(c, parentName, st, auxRules, counter)
	}
	return r
}

// expandTopLevelRepeat expands repeat/repeat1 at the top level of a hidden
// rule into a self-referencing choice. This matches tree-sitter's behavior:
// when a hidden rule IS a repeat, tree-sitter makes the rule self-recursive
// (e.g., _a_list → _a_list item | item) rather than creating a separate aux.
//
// Only applies when the ENTIRE rule body is a repeat/repeat1 (possibly
// wrapped in precedence). Nested repeats inside seq/choice are handled
// normally by prepareRule (which creates aux rules).
func expandTopLevelRepeat(r *Rule, ruleName string) *Rule {
	if r == nil {
		return r
	}
	// Only inline for hidden rules (underscore-prefixed). Visible rules
	// must use aux rules to keep flat structure — self-recursion in a
	// visible rule creates deeply nested nodes in the parse tree.
	if !strings.HasPrefix(ruleName, "_") {
		return r
	}
	// Unwrap precedence wrappers to check the inner structure.
	inner := r
	var precWrappers []*Rule
	for inner.Kind == RulePrec || inner.Kind == RulePrecLeft ||
		inner.Kind == RulePrecRight || inner.Kind == RulePrecDynamic {
		precWrappers = append(precWrappers, inner)
		if len(inner.Children) == 0 {
			return r
		}
		inner = inner.Children[0]
	}

	var expanded *Rule
	switch inner.Kind {
	case RuleRepeat1:
		// repeat1(x) → choice(seq(self, x), x)
		x := inner.Children[0]
		expanded = Choice(Seq(Sym(ruleName), cloneRule(x)), cloneRule(x))
	case RuleRepeat:
		// repeat(x) → choice(blank(), seq(self, x))
		// Matches tree-sitter's expansion. The standalone x alternative is
		// redundant (seq(self,x) with self→blank already covers it) and
		// causes spurious R/R conflicts.
		x := inner.Children[0]
		expanded = Choice(Blank(), Seq(Sym(ruleName), cloneRule(x)))
	default:
		return r
	}

	// Re-wrap with precedence if there were any wrappers.
	for i := len(precWrappers) - 1; i >= 0; i-- {
		w := precWrappers[i]
		expanded = &Rule{Kind: w.Kind, Prec: w.Prec, Children: []*Rule{expanded}}
	}
	return expanded
}

// registerExtraTerminals pre-registers terminal symbols from extras
// so they get contiguous IDs before nonterminals.
func registerExtraTerminals(g *Grammar, st *symbolTable) {
	for _, e := range g.Extras {
		if e == nil {
			continue
		}
		if e.Kind == RulePattern {
			st.getOrAdd("_whitespace", SymbolInfo{
				Name: "_whitespace", Visible: false, Named: false, Kind: SymbolTerminal,
			})
		}
	}
}

// registerExternalSymbols registers external scanner symbols from g.Externals.
// Each external token gets a symbol ID with Kind=SymbolExternal.
// Returns the mapping: external token index → symbol ID.
func registerExternalSymbols(g *Grammar, st *symbolTable) []int {
	var extSyms []int
	anonPatternCount := 0
	for _, ext := range g.Externals {
		if ext == nil {
			continue
		}
		name := ""
		named := true
		switch ext.Kind {
		case RuleSymbol:
			name = ext.Value
		case RuleString:
			// External STRING tokens are anonymous structural delimiters
			// (like "/>"), equivalent to inline string literals. They must
			// be Named=false so the parser treats them as anonymous tokens
			// that don't count as named children.
			name = ext.Value
			named = false
		case RulePattern:
			// Anonymous external patterns are emitted by tree-sitter with
			// synthetic names like _token1. Keep them in the external-symbol
			// table so scanner adaptation can preserve external token order.
			anonPatternCount++
			name = fmt.Sprintf("_token%d", anonPatternCount)
			named = false
		default:
			continue
		}
		visible := !strings.HasPrefix(name, "_")
		id := st.addSymbol(name, SymbolInfo{
			Name:    name,
			Visible: visible,
			Named:   named,
			Kind:    SymbolExternal,
		})
		extSyms = append(extSyms, id)
	}
	return extSyms
}

// resolveExtras returns symbol IDs for the extra rules.
func resolveExtras(g *Grammar, st *symbolTable) []int {
	var extras []int
	addExtra := func(id int) {
		for _, cur := range extras {
			if cur == id {
				return
			}
		}
		extras = append(extras, id)
	}
	appendMatchingExternalPatternExtras := func(pattern string) {
		anonPatternCount := 0
		for _, ext := range g.Externals {
			if ext == nil || ext.Kind != RulePattern {
				continue
			}
			anonPatternCount++
			if ext.Value != pattern {
				continue
			}
			name := fmt.Sprintf("_token%d", anonPatternCount)
			if id, ok := st.lookup(name); ok {
				addExtra(id)
			}
		}
	}
	for _, e := range g.Extras {
		if e == nil {
			continue
		}
		switch e.Kind {
		case RulePattern:
			if id, ok := st.lookup("_whitespace"); ok {
				addExtra(id)
			}
			appendMatchingExternalPatternExtras(e.Value)
		case RuleSymbol:
			if id, ok := st.lookupNonterm(e.Value); ok {
				addExtra(id)
			}
		case RuleString:
			if id, ok := st.lookup(e.Value); ok {
				addExtra(id)
			}
		}
	}
	return extras
}

// extractTerminals builds TerminalPattern entries for DFA generation.
// When keywordSet is non-nil, string terminals that are keywords are excluded
// from the main DFA (they're handled by the keyword DFA instead).
func extractTerminals(g *Grammar, st *symbolTable, stringLits []string, namedTokens []string, inlinePatterns []string, inlineTokens []inlineTokenEntry, keywordSet map[int]bool) ([]TerminalPattern, error) {
	var patterns []TerminalPattern
	priority := 0

	// String literals become simple string-match patterns.
	for _, s := range stringLits {
		id, ok := st.lookup(s)
		if !ok {
			continue
		}
		// Skip keywords — they're recognized via the word token + keyword DFA.
		if keywordSet != nil && keywordSet[id] {
			priority++
			continue
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID: id,
			Rule:     Str(s),
			Priority: priority,
		})
		priority++
	}

	// Named tokens: split into string-only and non-string groups.
	// String-only named tokens get string-tier priority (right after inline
	// string literals), matching tree-sitter C where string terminals always
	// have lower symbol IDs and thus higher lexer priority than patterns.
	var stringNamedTokens, patternNamedTokens []string
	for _, name := range namedTokens {
		rule := g.Rules[name]
		if isStringOnlyToken(rule) {
			stringNamedTokens = append(stringNamedTokens, name)
		} else {
			patternNamedTokens = append(patternNamedTokens, name)
		}
	}

	// String-only named tokens (string-tier priority, right after inline strings).
	for _, name := range stringNamedTokens {
		id, ok := st.lookup(name)
		if !ok {
			continue
		}
		rule := g.Rules[name]
		expanded, imm, prec, err := expandTokenRule(rule)
		if err != nil {
			return nil, fmt.Errorf("expand token %q: %w", name, err)
		}
		adjustedPriority := priority - prec*1000
		patterns = append(patterns, TerminalPattern{
			SymbolID:  id,
			Rule:      expanded,
			Priority:  adjustedPriority,
			Immediate: imm,
		})
		priority++
	}

	// Inline patterns (regex appearing directly in non-terminal rules, not in token()).
	for _, pat := range inlinePatterns {
		id, ok := st.lookup(pat)
		if !ok {
			continue
		}
		expanded, err := expandPatternRule(pat)
		if err != nil {
			return nil, fmt.Errorf("expand inline pattern %q: %w", pat, err)
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID: id,
			Rule:     expanded,
			Priority: priority,
		})
		priority++
	}

	// Non-string named tokens (after inline patterns).
	for _, name := range patternNamedTokens {
		id, ok := st.lookup(name)
		if !ok {
			continue
		}
		rule := g.Rules[name]
		expanded, imm, prec, err := expandTokenRule(rule)
		if err != nil {
			return nil, fmt.Errorf("expand token %q: %w", name, err)
		}
		adjustedPriority := priority - prec*1000
		patterns = append(patterns, TerminalPattern{
			SymbolID:  id,
			Rule:      expanded,
			Priority:  adjustedPriority,
			Immediate: imm,
		})
		priority++
	}

	// Inline token patterns (Token/ImmToken found inside nonterminal rules).
	for _, entry := range inlineTokens {
		id, ok := st.lookup(entry.name)
		if !ok {
			continue
		}
		expanded, _, prec, err := expandTokenRule(entry.rule)
		if err != nil {
			return nil, fmt.Errorf("expand inline token %q: %w", entry.name, err)
		}
		adjustedPriority := priority - prec*1000
		if entry.immediate {
			// Immediate inline tokens must outrank overlapping non-immediate
			// patterns in the same lex mode (e.g. dockerfile env_pair "=" value).
			// But do not force a shorter immediate string literal ahead of a
			// longer non-immediate string terminal that shares its prefix (e.g.
			// "#" vs "#)"). Tree-sitter still needs the longer literal to win.
			if !(expanded.Kind == RuleString && hasLongerStringPrefixPattern(patterns, expanded.Value)) {
				adjustedPriority -= 10000
			}
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID:  id,
			Rule:      expanded,
			Priority:  adjustedPriority,
			Immediate: entry.immediate,
		})
		priority++
	}

	// Extra patterns (like /\s/).
	for _, e := range g.Extras {
		if e != nil && e.Kind == RulePattern {
			id, ok := st.lookup("_whitespace")
			if !ok {
				continue
			}
			expanded, err := expandPatternRule(e.Value)
			if err != nil {
				return nil, fmt.Errorf("expand extra pattern: %w", err)
			}
			patterns = append(patterns, TerminalPattern{
				SymbolID: id,
				Rule:     expanded,
				Priority: priority + 1000, // lowest priority (high number = low priority in DFA)
			})
		}
	}

	return patterns, nil
}

func hasLongerStringPrefixPattern(patterns []TerminalPattern, prefix string) bool {
	for _, pat := range patterns {
		if pat.Immediate || pat.Rule == nil || pat.Rule.Kind != RuleString {
			continue
		}
		if len(pat.Rule.Value) <= len(prefix) {
			continue
		}
		if strings.HasPrefix(pat.Rule.Value, prefix) {
			return true
		}
	}
	return false
}

// identifyKeywords determines which string terminals are keywords.
// A keyword is a string terminal whose characters all match the word token's
// pattern. Returns the keyword set, ordered symbol IDs, and terminal patterns
// for keyword DFA construction.
func identifyKeywords(g *Grammar, st *symbolTable, stringLits []string) (map[int]bool, []int, []TerminalPattern) {
	wordRule := g.Rules[g.Word]
	if wordRule == nil {
		return nil, nil, nil
	}

	// Build a test DFA from the word pattern.
	expanded, _, _, err := expandTokenRule(wordRule)
	if err != nil {
		return nil, nil, nil
	}
	b := newNFABuilder()
	frag, err := b.buildFromRule(expanded)
	if err != nil {
		return nil, nil, nil
	}
	b.states[frag.end].accept = 1 // any non-zero accept
	b.states[frag.end].priority = 0

	dfa := subsetConstruction(&nfa{states: b.states, start: frag.start})

	keywordSet := make(map[int]bool)
	var keywordSyms []int
	var keywordEntries []TerminalPattern
	priority := 0

	for _, s := range stringLits {
		id, ok := st.lookup(s)
		if !ok {
			continue
		}
		// Treat only identifier-like literals as keyword candidates.
		// Some grammars have broad `word` tokens that also match punctuation
		// literals (e.g. //, $$), which should remain regular terminals.
		if matchesDFA(dfa, s) && isIdentifierLikeKeywordLiteral(s) {
			keywordSet[id] = true
			keywordSyms = append(keywordSyms, id)
			keywordEntries = append(keywordEntries, TerminalPattern{
				SymbolID: id,
				Rule:     Str(s),
				Priority: priority,
			})
			priority++
		}
	}

	return keywordSet, keywordSyms, keywordEntries
}

func isIdentifierLikeKeywordLiteral(s string) bool {
	if s == "" {
		return false
	}
	hasLetter := false
	for i, r := range s {
		if i == 0 {
			if r == '_' {
				continue
			}
			if unicode.IsLetter(r) {
				hasLetter = true
				continue
			}
			return false
		}
		if r == '_' || unicode.IsDigit(r) {
			continue
		}
		if unicode.IsLetter(r) {
			hasLetter = true
			continue
		}
		return false
	}
	return hasLetter
}

// matchesDFA tests if a string is fully accepted by a DFA.
func matchesDFA(dfa []dfaState, s string) bool {
	state := 0
	for _, ch := range s {
		found := false
		for _, t := range dfa[state].transitions {
			if ch >= t.lo && ch <= t.hi {
				state = t.nextState
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return dfa[state].accept != 0
}

// expandTokenRule flattens a token rule into a Rule tree suitable for
// NFA construction. Returns the expanded rule, whether it's immediate,
// and the lexer precedence bias (from TOKEN(PREC(n, ...))).
func expandTokenRule(r *Rule) (*Rule, bool, int, error) {
	if r == nil {
		return Blank(), false, 0, nil
	}
	switch r.Kind {
	case RuleString:
		return Str(r.Value), false, 0, nil
	case RulePattern:
		expanded, err := expandPatternRule(r.Value)
		return expanded, false, 0, err
	case RuleToken:
		inner, prec, err := flattenTokenInnerPrec(r.Children[0])
		return inner, false, prec, err
	case RuleImmToken:
		inner, prec, err := flattenTokenInnerPrec(r.Children[0])
		return inner, true, prec, err
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			rule, imm, _, err := expandTokenRule(r.Children[0])
			return rule, imm, r.Prec, err
		}
		return Blank(), false, 0, nil
	default:
		return Blank(), false, 0, fmt.Errorf("unexpected rule kind %d in token position", r.Kind)
	}
}

// flattenTokenInnerPrec extracts precedence from the top-level PREC wrapper
// inside a token() rule, then flattens the rest for NFA construction.
func flattenTokenInnerPrec(r *Rule) (*Rule, int, error) {
	if r == nil {
		rule, err := flattenTokenInner(r)
		return rule, 0, err
	}
	switch r.Kind {
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			rule, err := flattenTokenInner(r.Children[0])
			return rule, r.Prec, err
		}
		return Blank(), r.Prec, nil
	default:
		rule, err := flattenTokenInner(r)
		return rule, 0, err
	}
}

// flattenTokenInner expands the interior of a token() rule for NFA construction.
// Inside a token, everything is part of one lexer pattern.
func flattenTokenInner(r *Rule) (*Rule, error) {
	if r == nil {
		return Blank(), nil
	}
	switch r.Kind {
	case RuleString:
		return Str(r.Value), nil
	case RulePattern:
		return expandPatternRule(r.Value)
	case RuleSeq:
		children := make([]*Rule, len(r.Children))
		for i, c := range r.Children {
			exp, err := flattenTokenInner(c)
			if err != nil {
				return nil, err
			}
			children[i] = exp
		}
		return Seq(children...), nil
	case RuleChoice:
		children := make([]*Rule, len(r.Children))
		for i, c := range r.Children {
			exp, err := flattenTokenInner(c)
			if err != nil {
				return nil, err
			}
			children[i] = exp
		}
		return Choice(children...), nil
	case RuleRepeat:
		inner, err := flattenTokenInner(r.Children[0])
		if err != nil {
			return nil, err
		}
		return Repeat(inner), nil
	case RuleRepeat1:
		inner, err := flattenTokenInner(r.Children[0])
		if err != nil {
			return nil, err
		}
		return Repeat1(inner), nil
	case RuleOptional:
		inner, err := flattenTokenInner(r.Children[0])
		if err != nil {
			return nil, err
		}
		return Optional(inner), nil
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			return flattenTokenInner(r.Children[0])
		}
		return Blank(), nil
	case RuleBlank:
		return Blank(), nil
	case RuleToken, RuleImmToken:
		// Nested token() inside token() — just unwrap.
		if len(r.Children) > 0 {
			return flattenTokenInner(r.Children[0])
		}
		return Blank(), nil
	case RuleSymbol:
		// Symbol reference inside token — this typically means the token
		// references another rule. Return as-is; the caller should resolve.
		return r, nil
	case RuleAlias:
		// Alias inside token — strip the alias metadata and flatten the content.
		if len(r.Children) > 0 {
			return flattenTokenInner(r.Children[0])
		}
		return Blank(), nil
	case RuleField:
		// Field inside token — strip the field metadata and flatten the content.
		if len(r.Children) > 0 {
			return flattenTokenInner(r.Children[0])
		}
		return Blank(), nil
	default:
		return nil, fmt.Errorf("unexpected rule kind %d inside token", r.Kind)
	}
}

// flattenRule2 extracts all productions from a prepared rule tree.
// It properly handles Choice at any level by enumerating all alternatives.
func flattenRule2(r *Rule, lhsID int, st *symbolTable, prodIDCounter *int) []Production {
	if r == nil {
		return nil
	}

	// Unwrap precedence/assoc wrappers at the top level.
	prec, assoc, dynPrec, inner := unwrapPrec(r)

	switch inner.Kind {
	case RuleChoice:
		var prods []Production
		for _, alt := range inner.Children {
			altPrec, altAssoc, altDyn, altInner := unwrapPrec(alt)
			if altPrec == 0 {
				altPrec = prec
			}
			if altAssoc == AssocNone {
				altAssoc = assoc
			}
			if altDyn == 0 {
				altDyn = dynPrec
			}
			// Recursively flatten — alternatives may contain more choices.
			altProds := flattenRule2(altInner, lhsID, st, prodIDCounter)
			for i := range altProds {
				if altProds[i].Prec == 0 {
					altProds[i].Prec = altPrec
				}
				if altProds[i].Assoc == AssocNone {
					altProds[i].Assoc = altAssoc
				}
				if altProds[i].DynPrec == 0 {
					altProds[i].DynPrec = altDyn
				}
			}
			prods = append(prods, altProds...)
		}
		// Propagate prec from non-epsilon siblings to epsilon productions.
		// In tree-sitter, all alternatives of a choice share the same
		// precedence context; epsilon (blank) alternatives should inherit
		// the prec from their non-epsilon siblings. This matters for repeat
		// helpers where the epsilon reduce must compete with shift actions
		// from inner nonterminals (e.g., array's comma vs sequence_expression).
		var maxPrec int
		var maxAssoc Assoc
		for _, p := range prods {
			if p.Prec > maxPrec {
				maxPrec = p.Prec
				maxAssoc = p.Assoc
			}
		}
		if maxPrec > 0 {
			for i := range prods {
				if prods[i].Prec == 0 && len(prods[i].RHS) == 0 {
					prods[i].Prec = maxPrec
					if prods[i].Assoc == AssocNone {
						prods[i].Assoc = maxAssoc
					}
				}
			}
		}
		return prods

	case RuleBlank:
		prod := Production{
			LHS:          lhsID,
			Prec:         prec,
			Assoc:        assoc,
			DynPrec:      dynPrec,
			ProductionID: *prodIDCounter,
		}
		*prodIDCounter++
		return []Production{prod}

	default:
		// Enumerate all alternatives from Choice-within-Seq by expanding
		// the rule into a list of "flat" RHS sequences.
		alternatives := enumerateAlternatives(inner)
		var prods []Production
		for _, alt := range alternatives {
			// Compute per-alternative prec: use rightmost element's prec
			// (matching tree-sitter's behavior where the rightmost prec
			// wrapper in a production wins). Fall back to the outer prec
			// from unwrapPrec, then scanInnerPrec as last resort.
			altPrec, altAssoc, altDyn := prec, assoc, dynPrec
			for _, elem := range alt {
				if elem.prec != 0 {
					altPrec = elem.prec
				}
				if elem.assoc != AssocNone {
					altAssoc = elem.assoc
				}
				if elem.dynPrec != 0 {
					altDyn = elem.dynPrec
				}
			}
			if altPrec == 0 && altAssoc == AssocNone && altDyn == 0 {
				altPrec, altAssoc, altDyn = scanInnerPrec(inner)
			}

			prod := Production{
				LHS:          lhsID,
				Prec:         altPrec,
				Assoc:        altAssoc,
				DynPrec:      altDyn,
				ProductionID: *prodIDCounter,
			}
			*prodIDCounter++

			var rhs []int
			var fields []FieldAssign
			var aliases []AliasInfo
			collectLinearRHS(alt, st, &rhs, &fields, &aliases)
			prod.RHS = rhs
			prod.Fields = fields
			prod.Aliases = aliases
			prods = append(prods, prod)
		}
		return prods
	}
}

// rhsElement is a single element in a flattened RHS.
type rhsElement struct {
	rule       *Rule
	fieldName  string // non-empty if wrapped in a Field
	aliasName  string // non-empty if wrapped in an Alias
	aliasNamed bool   // true if alias is a named symbol ($.name form)
	prec       int    // precedence from enclosing prec wrapper (0 = none)
	assoc      Assoc  // associativity from enclosing prec wrapper
	dynPrec    int    // dynamic precedence from enclosing prec_dynamic wrapper
}

// enumerateAlternatives expands a rule containing inline Choice nodes
// into multiple flat sequences (one per alternative combination).
func enumerateAlternatives(r *Rule) [][]*rhsElement {
	if r == nil {
		return [][]*rhsElement{{}}
	}
	switch r.Kind {
	case RuleChoice:
		var all [][]*rhsElement
		for _, child := range r.Children {
			all = append(all, enumerateAlternatives(child)...)
		}
		return all

	case RuleSeq:
		// Start with one empty sequence.
		result := [][]*rhsElement{{}}
		for _, child := range r.Children {
			childAlts := enumerateAlternatives(child)
			var newResult [][]*rhsElement
			for _, existing := range result {
				for _, childAlt := range childAlts {
					combined := make([]*rhsElement, len(existing)+len(childAlt))
					copy(combined, existing)
					copy(combined[len(existing):], childAlt)
					newResult = append(newResult, combined)
				}
			}
			result = newResult
		}
		return result

	case RuleField:
		if len(r.Children) == 0 {
			return [][]*rhsElement{{}}
		}
		// Enumerate alternatives inside the field, tagging each with the field name.
		innerAlts := enumerateAlternatives(r.Children[0])
		var result [][]*rhsElement
		for _, alt := range innerAlts {
			tagged := make([]*rhsElement, len(alt))
			for i, elem := range alt {
				cp := *elem
				if cp.fieldName == "" {
					cp.fieldName = r.Value
				}
				tagged[i] = &cp
			}
			result = append(result, tagged)
		}
		return result

	case RuleAlias:
		if len(r.Children) == 0 {
			return [][]*rhsElement{{}}
		}
		// Enumerate alternatives inside the alias, tagging each with the alias name.
		innerAlts := enumerateAlternatives(r.Children[0])
		var result [][]*rhsElement
		for _, alt := range innerAlts {
			tagged := make([]*rhsElement, len(alt))
			for i, elem := range alt {
				cp := *elem
				if cp.aliasName == "" {
					cp.aliasName = r.Value
					cp.aliasNamed = r.Named
				}
				tagged[i] = &cp
			}
			result = append(result, tagged)
		}
		return result

	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			innerAlts := enumerateAlternatives(r.Children[0])
			// Tag all elements in each alternative with the prec info.
			for _, alt := range innerAlts {
				for _, elem := range alt {
					switch r.Kind {
					case RulePrecLeft:
						elem.prec = r.Prec
						elem.assoc = AssocLeft
					case RulePrecRight:
						elem.prec = r.Prec
						elem.assoc = AssocRight
					case RulePrecDynamic:
						elem.dynPrec = r.Prec
					default: // RulePrec
						elem.prec = r.Prec
					}
				}
			}
			return innerAlts
		}
		return [][]*rhsElement{{}}

	case RuleBlank:
		// Epsilon — empty sequence.
		return [][]*rhsElement{{}}

	default:
		// Leaf node (String, Symbol, etc.) — single element.
		return [][]*rhsElement{{&rhsElement{rule: r}}}
	}
}

// collectLinearRHS converts a flat list of rhsElements into symbol IDs, field assignments, and alias info.
func collectLinearRHS(elems []*rhsElement, st *symbolTable, rhs *[]int, fields *[]FieldAssign, aliases *[]AliasInfo) {
	for _, elem := range elems {
		childIdx := len(*rhs)
		addRuleSymbol(elem.rule, st, rhs)
		if elem.fieldName != "" && len(*rhs) > childIdx {
			st.fieldID(elem.fieldName)
			*fields = append(*fields, FieldAssign{
				ChildIndex: childIdx,
				FieldName:  elem.fieldName,
			})
		}
		if elem.aliasName != "" && len(*rhs) > childIdx {
			*aliases = append(*aliases, AliasInfo{
				ChildIndex: childIdx,
				Name:       elem.aliasName,
				Named:      elem.aliasNamed,
			})
		}
	}
}

// addRuleSymbol resolves a rule to a symbol ID and appends it to rhs.
func addRuleSymbol(r *Rule, st *symbolTable, rhs *[]int) {
	if r == nil {
		return
	}
	switch r.Kind {
	case RuleString:
		if id, ok := st.lookup(r.Value); ok {
			*rhs = append(*rhs, id)
		}
	case RuleSymbol:
		// Sym("type") should resolve to the nonterminal "type" when it exists,
		// not the string literal "type". This handles grammars where a rule
		// name collides with a string literal (e.g., graphql's "type" keyword
		// vs. type rule).
		if id, ok := st.lookupNonterm(r.Value); ok {
			*rhs = append(*rhs, id)
		}
	case RulePattern:
		// Inline patterns are registered by their pattern value.
		if id, ok := st.lookup(r.Value); ok {
			*rhs = append(*rhs, id)
		}
	}
}

// deduplicateProductions removes duplicate productions that have the same
// LHS, RHS, fields, and aliases. Keeps the first occurrence (lowest production
// ID). Reassigns production IDs to be contiguous.
func deduplicateProductions(prods []Production) []Production {
	type prodKey struct {
		lhs    int
		rhs    string // fmt.Sprint of RHS slice
		prec   int
		assoc  Assoc
		dynP   int
		fields string
		alias  string
	}

	seen := make(map[prodKey]bool, len(prods))
	result := make([]Production, 0, len(prods))

	for _, p := range prods {
		k := prodKey{
			lhs:    p.LHS,
			rhs:    fmt.Sprint(p.RHS),
			prec:   p.Prec,
			assoc:  p.Assoc,
			dynP:   p.DynPrec,
			fields: fmt.Sprint(p.Fields),
			alias:  fmt.Sprint(p.Aliases),
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		result = append(result, p)
	}

	// Reassign contiguous production IDs.
	for i := range result {
		result[i].ProductionID = i
	}
	return result
}

// flattenHiddenChoiceAlts inlines single-symbol (pass-through) alternatives
// of hidden nonterminals into parent rules at the rule-tree level.
//
// For example, if hidden rule _H has:
//
//	_H → Choice(X, Y, Seq(P, Q))
//
// And parent rule A has:
//
//	A → Choice(_H, Z)
//
// After flattening:
//
//	_H → Seq(P, Q)                   (only compound alts kept)
//	A → Choice(_H, X, Y, Z)          (pass-through alts inlined)
//
// This matches tree-sitter C's flatten_grammar.cc behavior.
func flattenHiddenChoiceAlts(g *Grammar) *Grammar {
	// 1. Identify hidden nonterminals with mixed pass-through and compound alts.
	flattenMap := make(map[string]*flattenInfo)

	for _, name := range g.RuleOrder {
		if !strings.HasPrefix(name, "_") {
			continue // only hidden rules
		}
		// Skip supertypes.
		isSupertype := false
		for _, s := range g.Supertypes {
			if s == name {
				isSupertype = true
				break
			}
		}
		if isSupertype {
			continue
		}

		rule := g.Rules[name]
		if rule == nil {
			continue
		}

		alts := getTopLevelChoiceAlts(rule)
		if len(alts) <= 1 {
			continue
		}

		var pt, compound []*Rule
		for _, alt := range alts {
			if isSingleSymRef(alt) {
				pt = append(pt, alt)
			} else {
				compound = append(compound, alt)
			}
		}

		if len(pt) == 0 || len(compound) == 0 {
			continue // nothing to split, or all pass-through
		}

		// Cap pass-through count to avoid Cartesian product explosion.
		// When a hidden rule with N pass-through alts is referenced in a Seq
		// with another such rule, production extraction creates N*M alternatives.
		if len(pt) > 8 {
			continue
		}

		// Skip if the hidden rule is directly self-referencing in compound alts.
		selfRef := false
		for _, c := range compound {
			if ruleReferencesSym(c, name) {
				selfRef = true
				break
			}
		}
		if selfRef {
			continue
		}

		flattenMap[name] = &flattenInfo{
			passThrough: pt,
			compound:    compound,
		}
	}

	if len(flattenMap) == 0 {
		return g
	}

	// 2. Build new grammar with flattened rules.
	out := NewGrammar(g.Name)
	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if rule == nil {
			continue
		}

		// If this IS a flattened hidden rule, replace with compound-only CHOICE.
		if fi, ok := flattenMap[name]; ok {
			var newRule *Rule
			if len(fi.compound) == 1 {
				newRule = fi.compound[0]
			} else {
				newRule = Choice(fi.compound...)
			}
			out.Define(name, newRule)
			continue
		}

		// For all other rules, inline pass-through alternatives at reference sites.
		out.Define(name, inlinePassthroughRefs(rule, flattenMap))
	}

	// Copy other fields.
	for _, extra := range g.Extras {
		out.Extras = append(out.Extras, inlinePassthroughRefs(extra, flattenMap))
	}
	for _, group := range g.Conflicts {
		out.Conflicts = append(out.Conflicts, group)
	}
	for _, ext := range g.Externals {
		out.Externals = append(out.Externals, ext)
	}
	out.Word = g.Word
	out.Supertypes = g.Supertypes
	out.Inline = g.Inline
	return out
}

// getTopLevelChoiceAlts unwraps precedence and returns the top-level CHOICE
// alternatives. If the rule is not a choice, returns nil.
func getTopLevelChoiceAlts(r *Rule) []*Rule {
	if r == nil {
		return nil
	}
	// Unwrap precedence wrappers.
	for r.Kind == RulePrec || r.Kind == RulePrecLeft || r.Kind == RulePrecRight || r.Kind == RulePrecDynamic {
		if len(r.Children) > 0 {
			r = r.Children[0]
		} else {
			return nil
		}
	}
	if r.Kind == RuleChoice {
		return r.Children
	}
	return nil
}

// isSingleSymRef returns true if the rule is a plain symbol reference,
// possibly wrapped in precedence.
func isSingleSymRef(r *Rule) bool {
	if r == nil {
		return false
	}
	// Unwrap precedence, alias, and field wrappers — these all produce cc=1
	// productions (they attach metadata but don't add child count).
	for {
		switch r.Kind {
		case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic, RuleAlias, RuleField:
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
			return false
		}
		break
	}
	// A single symbol, pattern, or string literal all produce cc=1 productions.
	return r.Kind == RuleSymbol || r.Kind == RulePattern || r.Kind == RuleString
}

// ruleReferencesSym returns true if the rule tree contains a Sym reference
// to the named symbol.
func ruleReferencesSym(r *Rule, name string) bool {
	if r == nil {
		return false
	}
	if r.Kind == RuleSymbol {
		return r.Value == name
	}
	for _, c := range r.Children {
		if ruleReferencesSym(c, name) {
			return true
		}
	}
	return false
}

// inlinePassthroughRefs walks a rule tree and, at every Sym reference to a
// flattened hidden rule, wraps it in a Choice that includes the pass-through
// alternatives alongside the original reference. This preserves the original
// reference for compound alternatives while adding direct paths for cc=1 targets.
func inlinePassthroughRefs(r *Rule, flattenMap map[string]*flattenInfo) *Rule {
	if r == nil {
		return nil
	}

	// If this is a symbol reference to a flattened hidden rule, expand it.
	if r.Kind == RuleSymbol {
		fi, ok := flattenMap[r.Value]
		if !ok {
			return r
		}
		// Create Choice(original_ref, passthrough_alt1, passthrough_alt2, ...)
		alts := make([]*Rule, 0, len(fi.passThrough)+1)
		alts = append(alts, r) // keep original ref for compound alts
		for _, pt := range fi.passThrough {
			alts = append(alts, cloneRule(pt))
		}
		return Choice(alts...)
	}

	// Recurse into children.
	if len(r.Children) == 0 {
		return r
	}
	changed := false
	newChildren := make([]*Rule, len(r.Children))
	for i, c := range r.Children {
		nc := inlinePassthroughRefs(c, flattenMap)
		if nc != c {
			changed = true
		}
		newChildren[i] = nc
	}
	if !changed {
		return r
	}
	out := *r
	out.Children = newChildren
	return &out
}

type flattenInfo struct {
	passThrough []*Rule
	compound    []*Rule
}

// expandInlineRules returns a copy of the grammar with all inline rule
// references replaced by the rule body. Inline rules are then removed from
// the rule set so they don't create nonterminal symbols.
func expandInlineRules(g *Grammar) *Grammar {
	inlineSet := make(map[string]bool, len(g.Inline))
	for _, name := range g.Inline {
		inlineSet[name] = true
	}

	// Build lookup for inline rule bodies.
	// Expand inline rules with reasonable width. Very wide choices (>16
	// alternatives) can cause Cartesian product explosion when the rule is used
	// in multiple positions of a sequence. Tree-sitter handles this but some
	// grammars' inline rules need the nonterminal wrapper for correct GLR
	// conflict resolution in grammargen's current LR table builder.
	inlineBodies := make(map[string]*Rule)
	// For inline rules too wide to expand, rename them to be hidden (prefix '_')
	// so they don't create visible nodes in the parse tree.
	hiddenRenames := make(map[string]string)
	for _, name := range g.Inline {
		if rule, ok := g.Rules[name]; ok {
			if choiceWidth(rule) <= 16 {
				inlineBodies[name] = rule
			} else if !strings.HasPrefix(name, "_") {
				// Too wide to inline but currently visible — make hidden.
				hiddenRenames[name] = "_" + name
			}
		}
	}

	// First pass: expand inline refs in all rules.
	expandedRules := make(map[string]*Rule)
	for _, name := range g.RuleOrder {
		if inlineSet[name] && inlineBodies[name] != nil {
			continue // will be dropped
		}
		expandedRules[name] = substituteInlineRefs(g.Rules[name], inlineBodies)
	}
	var expandedExtras []*Rule
	for _, extra := range g.Extras {
		expandedExtras = append(expandedExtras, substituteInlineRefs(extra, inlineBodies))
	}
	var expandedExternals []*Rule
	for _, ext := range g.Externals {
		expandedExternals = append(expandedExternals, substituteInlineRefs(ext, inlineBodies))
	}

	// Scan expanded rules for remaining references to inline rules that
	// weren't fully expanded (depth limit hit). These inline rules must
	// be preserved as hidden rules to prevent dangling symbol references
	// that would become epsilon productions.
	stillReferenced := make(map[string]bool)
	for _, rule := range expandedRules {
		collectInlineRefs(rule, inlineBodies, stillReferenced)
	}
	for _, extra := range expandedExtras {
		collectInlineRefs(extra, inlineBodies, stillReferenced)
	}
	for _, ext := range expandedExternals {
		collectInlineRefs(ext, inlineBodies, stillReferenced)
	}
	// For any inline rule still referenced, add it to hiddenRenames so it's
	// kept as a hidden rule in the output grammar.
	for name := range stillReferenced {
		if _, already := hiddenRenames[name]; !already {
			if !strings.HasPrefix(name, "_") {
				hiddenRenames[name] = "_" + name
			}
			// else: already hidden, no rename needed, just don't delete it
		}
	}

	// Create a new grammar without the fully-inlined rules.
	out := NewGrammar(g.Name)
	for _, name := range g.RuleOrder {
		if inlineSet[name] && inlineBodies[name] != nil && !stillReferenced[name] {
			continue // drop fully inlined rules
		}
		outName := name
		if renamed, ok := hiddenRenames[name]; ok {
			outName = renamed
		}
		rule := expandedRules[name]
		if rule == nil {
			// This is an inline rule still referenced — use its original body
			// with inline refs substituted (and handle its own nested refs).
			rule = substituteInlineRefs(g.Rules[name], inlineBodies)
		}
		rule = applyHiddenRenames(rule, hiddenRenames)
		out.Define(outName, rule)
	}

	// Copy other fields.
	for _, extra := range expandedExtras {
		out.Extras = append(out.Extras, applyHiddenRenames(extra, hiddenRenames))
	}
	// Rename conflict group entries too.
	for _, group := range g.Conflicts {
		outGroup := make([]string, len(group))
		for i, name := range group {
			if renamed, ok := hiddenRenames[name]; ok {
				outGroup[i] = renamed
			} else {
				outGroup[i] = name
			}
		}
		out.Conflicts = append(out.Conflicts, outGroup)
	}
	for _, ext := range expandedExternals {
		out.Externals = append(out.Externals, applyHiddenRenames(ext, hiddenRenames))
	}
	out.Word = g.Word
	out.Supertypes = g.Supertypes
	// Don't propagate Inline — they've been expanded.

	return out
}

// choiceWidth returns the number of top-level Choice alternatives in a rule.
// For non-Choice rules, returns 1.
func choiceWidth(r *Rule) int {
	if r == nil {
		return 1
	}
	// Unwrap precedence wrappers.
	for r.Kind == RulePrec || r.Kind == RulePrecLeft || r.Kind == RulePrecRight || r.Kind == RulePrecDynamic {
		if len(r.Children) > 0 {
			r = r.Children[0]
		} else {
			return 1
		}
	}
	if r.Kind == RuleChoice {
		return len(r.Children)
	}
	return 1
}

// substituteInlineRefs replaces RuleSymbol references to inline rules with
// cloned copies of the inline rule body. Recursion depth is bounded to
// prevent Cartesian product explosion in grammars with deep inline chains
// (e.g. Haskell with 17 nested chains across 51 inline rules).
func substituteInlineRefs(r *Rule, inlineBodies map[string]*Rule) *Rule {
	return substituteInlineRefsDepth(r, inlineBodies, 0)
}

const maxInlineSubstDepth = 2

func substituteInlineRefsDepth(r *Rule, inlineBodies map[string]*Rule, depth int) *Rule {
	if r == nil {
		return nil
	}
	if r.Kind == RuleSymbol {
		if body, ok := inlineBodies[r.Value]; ok {
			clone := cloneRule(body)
			// Recursively substitute nested inline refs up to a bounded
			// depth to avoid exponential production explosion.
			if depth < maxInlineSubstDepth {
				return substituteInlineRefsDepth(clone, inlineBodies, depth+1)
			}
			return clone
		}
		return r
	}
	if len(r.Children) == 0 {
		return r
	}
	out := *r
	out.Children = make([]*Rule, len(r.Children))
	for i, c := range r.Children {
		out.Children[i] = substituteInlineRefsDepth(c, inlineBodies, depth)
	}
	return &out
}

// collectInlineRefs finds any symbol references in r that point to inline rules
// in inlineBodies. These are refs that weren't expanded due to depth limiting.
func collectInlineRefs(r *Rule, inlineBodies map[string]*Rule, out map[string]bool) {
	if r == nil {
		return
	}
	if r.Kind == RuleSymbol {
		if _, ok := inlineBodies[r.Value]; ok {
			out[r.Value] = true
		}
		return
	}
	for _, c := range r.Children {
		collectInlineRefs(c, inlineBodies, out)
	}
}

// applyHiddenRenames renames symbol references according to the hidden renames map.
func applyHiddenRenames(r *Rule, renames map[string]string) *Rule {
	if r == nil || len(renames) == 0 {
		return r
	}
	if r.Kind == RuleSymbol {
		if newName, ok := renames[r.Value]; ok {
			cp := *r
			cp.Value = newName
			return &cp
		}
		return r
	}
	if len(r.Children) == 0 {
		return r
	}
	out := *r
	out.Children = make([]*Rule, len(r.Children))
	changed := false
	for i, c := range r.Children {
		out.Children[i] = applyHiddenRenames(c, renames)
		if out.Children[i] != c {
			changed = true
		}
	}
	if !changed {
		return r
	}
	return &out
}

// scanInnerPrec walks a rule tree looking for prec wrappers inside seq elements.
// In tree-sitter, prec.left(N, $.symbol) inside a seq propagates the precedence
// to the containing production. Returns the last (rightmost) prec/assoc/dynPrec found.
func scanInnerPrec(r *Rule) (prec int, assoc Assoc, dynPrec int) {
	if r == nil {
		return 0, AssocNone, 0
	}
	switch r.Kind {
	case RulePrec:
		prec = r.Prec
	case RulePrecLeft:
		prec = r.Prec
		assoc = AssocLeft
	case RulePrecRight:
		prec = r.Prec
		assoc = AssocRight
	case RulePrecDynamic:
		dynPrec = r.Prec
	}
	for _, child := range r.Children {
		cp, ca, cd := scanInnerPrec(child)
		if cp != 0 {
			prec = cp
		}
		if ca != AssocNone {
			assoc = ca
		}
		if cd != 0 {
			dynPrec = cd
		}
	}
	return
}

// unwrapPrec strips precedence/associativity wrappers from a rule.
func unwrapPrec(r *Rule) (prec int, assoc Assoc, dynPrec int, inner *Rule) {
	for r != nil {
		switch r.Kind {
		case RulePrec:
			prec = r.Prec
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
		case RulePrecLeft:
			prec = r.Prec
			assoc = AssocLeft
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
		case RulePrecRight:
			prec = r.Prec
			assoc = AssocRight
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
		case RulePrecDynamic:
			dynPrec = r.Prec
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
		}
		break
	}
	return prec, assoc, dynPrec, r
}
