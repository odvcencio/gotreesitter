package grammargen

import (
	"context"
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
	LHS  int   // symbol index
	RHS  []int // symbol indices
	Prec int
	// HasExplicitPrec distinguishes an explicit compile-time precedence wrapper
	// (including prec(0, ...)) from the default implicit zero precedence.
	HasExplicitPrec bool
	Assoc           Assoc
	DynPrec         int
	ProductionID    int
	Fields          []FieldAssign // per-RHS-position field assignments
	Aliases         []AliasInfo   // per-RHS-position alias info
	IsExtra         bool          // true if this production belongs to a nonterminal extra
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
	// ReservedWordSets stores token symbol IDs for each imported reserved word
	// set. The first set is the global set from grammar.json. Current
	// generation derives per-state subsets from that global set.
	ReservedWordSets [][]int

	// External scanner support (populated when Grammar.Externals is set).
	ExternalSymbols []int // external token index → symbol ID

	// PrecedenceOrder stores the symbol-level precedence ordering from the
	// grammar's precedences table. Maps a rule name to its numeric position
	// (higher = higher priority) and whether it's a SYMBOL or STRING entry.
	// Used during conflict resolution to compare a reduce production's LHS
	// against the named precedence of a competing shift action.
	PrecedenceOrder *precOrderTable

	// conflictCache is built lazily by LR conflict resolution so repeated
	// resolveActionConflict calls can reuse the same reverse indexes.
	conflictCache *conflictResolutionCache
}

// symbolTable is used during normalization.
type symbolTable struct {
	byName              map[string]int // terminal name → symbol ID
	nontermByName       map[string]int // nonterminal name → symbol ID
	namedTokenByName    map[string]int // named token name → symbol ID (when distinct from anonymous)
	symbols             []SymbolInfo
	fieldMap            map[string]int
	fields              []string
	binaryRepeatMode    bool // use tree-sitter binary repeat helper shape
	choiceLiftThreshold int  // if >0, lift inline CHOICE nodes exceeding this width
}

const inlinePatternSymbolPrefix = "\x00inline_pattern:"

func inlinePatternSymbolKey(pattern string) string {
	return inlinePatternSymbolPrefix + pattern
}

func newSymbolTable() *symbolTable {
	st := &symbolTable{
		byName:           make(map[string]int),
		nontermByName:    make(map[string]int),
		namedTokenByName: make(map[string]int),
		fieldMap:         make(map[string]int),
		fields:           []string{""}, // index 0 is always ""
	}
	// Symbol 0 = "end" (EOF). Tree-sitter C marks this Named=true.
	st.addSymbol("end", SymbolInfo{
		Name:    "end",
		Visible: false,
		Named:   true,
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

// lookupNonterm returns the nonterminal symbol ID. Falls back to
// namedTokenByName (for named tokens that are distinct from anonymous
// terminals with the same name), then to byName.
func (st *symbolTable) lookupNonterm(name string) (int, bool) {
	if id, ok := st.nontermByName[name]; ok {
		return id, ok
	}
	// Prefer the named token when it was split from an anonymous terminal.
	// This ensures Sym("number") in literal_type resolves to the named
	// TOKEN symbol, not the anonymous keyword string.
	if id, ok := st.namedTokenByName[name]; ok {
		return id, ok
	}
	return st.lookup(name)
}

// lookupNamedToken returns the named token symbol ID. If the named token
// was split from an anonymous terminal, returns the split ID; otherwise
// falls back to byName.
func (st *symbolTable) lookupNamedToken(name string) (int, bool) {
	if id, ok := st.namedTokenByName[name]; ok {
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

	st := newSymbolTable()
	st.binaryRepeatMode = g.BinaryRepeatMode
	st.choiceLiftThreshold = g.ChoiceLiftThreshold
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
	// Collect which patterns are aliased, and to what name/named status.
	// Tree-sitter C bakes aliases into symbol names/metadata (e.g., pattern
	// [^\[\]]+ in ALIAS(..., "text") becomes a symbol named "text" with
	// visible=true, named=true). Grammargen must match this so the parser
	// creates properly typed nodes even when reductions don't follow the
	// expected path.
	aliasedPatterns := collectAliasedPatterns(g)
	for _, pat := range inlinePatterns {
		name := inlinePatternSymbolKey(pat)
		if _, ok := st.lookup(name); ok {
			continue // already registered
		}
		displayName := pat
		visible := false
		named := false
		if ai, ok := aliasedPatterns[pat]; ok {
			displayName = ai.name
			visible = true
			named = ai.named
		}
		st.addSymbol(name, SymbolInfo{
			Name:    displayName,
			Visible: visible,
			Named:   named,
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
		// When a complex TOKEN rule (not a string-only token) has the same
		// name as an already-registered anonymous string literal, they must
		// get distinct symbol IDs. Example: TS has STRING "number" in
		// predefined_type (the keyword) and TOKEN(CHOICE(...)) number (the
		// numeric literal pattern). Tree-sitter C gives them different IDs;
		// without this split, productions for predefined_type and literal_type
		// become indistinguishable.
		if named && kind == SymbolNamedToken {
			if existingID, exists := st.lookup(name); exists &&
				!st.symbols[existingID].Named && st.symbols[existingID].Kind == SymbolTerminal {
				rule := g.Rules[name]
				if rule != nil && !isStringOnlyToken(rule) {
					// Allocate a new symbol for the named token, keeping
					// the anonymous terminal at its original ID for string
					// literal references.
					newID := len(st.symbols)
					st.namedTokenByName[name] = newID
					st.symbols = append(st.symbols, SymbolInfo{
						Name:    displayName,
						Visible: visible,
						Named:   named,
						Kind:    kind,
					})
					continue
				}
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
		// When a hidden rule's entire body is repeat1, tree-sitter converts
		// the rule itself into the repetition binary tree instead of
		// introducing another auxiliary rule.
		rule = expandTopLevelRepeat(cloneRule(rule), name, st.binaryRepeatMode)
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

	// Tree-sitter expands repeats before flattening hidden pass-through
	// alternatives. Running this after prepareRule lets us flatten the cc=1
	// branches introduced by repeat lowering, especially for hidden repeat
	// helpers and top-level hidden repeat1 rules.
	processedRules, auxRules = flattenPreparedRules(g.Name, nonterminals, processedRules, auxRules, g.Supertypes)

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
		// Use lookupNamedToken so that if the word token was split from
		// an anonymous terminal (e.g. identifier TOKEN colliding with
		// "identifier" string), we get the named token's symbol ID.
		wordSymbolID, _ = st.lookupNamedToken(g.Word)
		keywordSet, keywordSymbols, keywordEntries = identifyKeywords(g, st, stringLiterals, namedTokens)
	}

	// Phase 6: Extract terminal patterns for DFA generation.
	terminals, err := extractTerminals(g, st, stringLiterals, namedTokens, inlinePatterns, inlineTokens, keywordSet)
	if err != nil {
		return nil, fmt.Errorf("extract terminals: %w", err)
	}

	reservedWordSets, err := resolveReservedWordSets(g.ReservedWordSets, st, terminals)
	if err != nil {
		return nil, fmt.Errorf("resolve reserved words: %w", err)
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

	// Phase 8b: Extend conflict groups to include auxiliary repeat rules.
	// When a grammar rule X is in a conflict group, auxiliary repeat rules
	// originating from X (e.g. X_repeat52) should inherit that membership.
	// Without this, R/R conflicts between a parent rule and its repeat
	// auxiliary are resolved deterministically (killing the repeat path)
	// instead of being kept as GLR as tree-sitter C intends.
	{
		// Build reverse map: grammar rule name → set of conflict group indices.
		conflictGroupsByName := make(map[string][]int)
		for gi, cgroup := range g.Conflicts {
			for _, name := range cgroup {
				conflictGroupsByName[name] = append(conflictGroupsByName[name], gi)
			}
		}
		// For each auxiliary rule, check if its originating rule is in any
		// conflict group. If so, add the auxiliary's symbol ID to that group.
		for auxName, originName := range auxOrigin {
			gis := conflictGroupsByName[originName]
			if len(gis) == 0 {
				continue
			}
			auxID, ok := st.lookupNonterm(auxName)
			if !ok {
				continue
			}
			for _, gi := range gis {
				if gi < len(conflicts) {
					conflicts[gi] = append(conflicts[gi], auxID)
				}
			}
		}
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

	promoteDefaultAliases(st.symbols, productions, extraSymbols)
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
	ng.ReservedWordSets = reservedWordSets
	ng.ExternalSymbols = externalSymbols
	ng.PrecedenceOrder = buildPrecOrderTable(g.Precedences, buildNamedPrecMapFromLevels(g.Precedences))

	// Set tokenCount boundary on symbols so assembly knows where terminals end.
	_ = tokenCount

	return ng, nil
}

func resolveReservedWordSets(sets []ReservedWordSet, st *symbolTable, terminals []TerminalPattern) ([][]int, error) {
	if len(sets) == 0 {
		return nil, nil
	}

	out := make([][]int, 0, len(sets))
	for _, set := range sets {
		seen := make(map[int]bool, len(set.Rules))
		resolved := make([]int, 0, len(set.Rules))
		for _, rule := range set.Rules {
			symID, ok := resolveReservedWordSymbol(rule, st, terminals)
			if !ok {
				return nil, fmt.Errorf("set %q: unsupported reserved rule %s", set.Name, describeRule(rule))
			}
			if !seen[symID] {
				seen[symID] = true
				resolved = append(resolved, symID)
			}
		}
		out = append(out, resolved)
	}
	return out, nil
}

func resolveReservedWordSymbol(rule *Rule, st *symbolTable, terminals []TerminalPattern) (int, bool) {
	if rule == nil {
		return 0, false
	}

	rule = unwrapReservedWordRule(rule)
	switch rule.Kind {
	case RuleString:
		id, ok := st.lookup(rule.Value)
		if ok && id >= 0 && id < len(st.symbols) && st.symbols[id].Kind != SymbolNonterminal {
			return id, true
		}
	case RuleSymbol:
		id, ok := st.lookupNamedToken(rule.Value)
		if ok && id >= 0 && id < len(st.symbols) && st.symbols[id].Kind != SymbolNonterminal {
			return id, true
		}
	}

	for _, term := range terminals {
		if rulesEqual(unwrapReservedWordRule(term.Rule), rule) {
			return term.SymbolID, true
		}
	}
	return 0, false
}

func unwrapReservedWordRule(rule *Rule) *Rule {
	for rule != nil {
		switch rule.Kind {
		case RuleToken, RuleImmToken, RuleField, RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic, RuleAlias:
			if len(rule.Children) == 0 {
				return rule
			}
			rule = rule.Children[0]
		default:
			return rule
		}
	}
	return nil
}

func describeRule(rule *Rule) string {
	if rule == nil {
		return "<nil>"
	}
	switch rule.Kind {
	case RuleString:
		return fmt.Sprintf("STRING(%q)", rule.Value)
	case RulePattern:
		return fmt.Sprintf("PATTERN(%q)", rule.Value)
	case RuleSymbol:
		return fmt.Sprintf("SYMBOL(%q)", rule.Value)
	case RuleToken:
		return fmt.Sprintf("TOKEN(%s)", describeRule(firstRuleChild(rule)))
	case RuleImmToken:
		return fmt.Sprintf("IMMEDIATE_TOKEN(%s)", describeRule(firstRuleChild(rule)))
	case RuleAlias:
		return fmt.Sprintf("ALIAS(%s, %q)", describeRule(firstRuleChild(rule)), rule.Value)
	default:
		return fmt.Sprintf("kind=%d", rule.Kind)
	}
}

func firstRuleChild(rule *Rule) *Rule {
	if rule == nil || len(rule.Children) == 0 {
		return nil
	}
	return rule.Children[0]
}

func promoteDefaultAliases(symbols []SymbolInfo, productions []Production, extraSymbols []int) {
	type aliasKey struct {
		name  string
		named bool
	}
	type aliasCount struct {
		key       aliasKey
		count     int
		firstSeen int
	}
	type symbolStatus struct {
		aliases          map[aliasKey]*aliasCount
		appearsUnaliased bool
	}

	if len(symbols) == 0 || len(productions) == 0 {
		return
	}

	statuses := make([]symbolStatus, len(symbols))
	seenOrder := 0

	for _, prod := range productions {
		if len(prod.RHS) == 0 {
			continue
		}
		aliasByChild := make(map[int]AliasInfo, len(prod.Aliases))
		for _, ai := range prod.Aliases {
			aliasByChild[ai.ChildIndex] = ai
		}
		for childIdx, symID := range prod.RHS {
			if symID < 0 || symID >= len(statuses) {
				continue
			}
			ai, ok := aliasByChild[childIdx]
			if !ok || ai.Name == "" {
				statuses[symID].appearsUnaliased = true
				continue
			}
			ak := aliasKey{name: ai.Name, named: ai.Named}
			if statuses[symID].aliases == nil {
				statuses[symID].aliases = make(map[aliasKey]*aliasCount)
			}
			entry, ok := statuses[symID].aliases[ak]
			if !ok {
				entry = &aliasCount{key: ak, firstSeen: seenOrder}
				statuses[symID].aliases[ak] = entry
			}
			entry.count++
			seenOrder++
		}
	}

	for _, symID := range extraSymbols {
		if symID >= 0 && symID < len(statuses) {
			// Extras that appear only with aliases (e.g., ALIAS-wrapper nonterminals
			// like COBOL's _LINE_COMMENT_ALIAS) should still be eligible for alias
			// promotion. Only mark as unaliased if it has no recorded aliases at all.
			if len(statuses[symID].aliases) == 0 {
				statuses[symID].appearsUnaliased = true
			}
		}
	}

	defaultAliases := make(map[int]aliasKey)
	for symID, status := range statuses {
		if status.appearsUnaliased || len(status.aliases) == 0 {
			continue
		}
		var best *aliasCount
		for _, entry := range status.aliases {
			if best == nil ||
				entry.count > best.count ||
				(entry.count == best.count && entry.firstSeen < best.firstSeen) {
				best = entry
			}
		}
		if best == nil {
			continue
		}
		defaultAliases[symID] = best.key
		symbols[symID].Name = best.key.name
		symbols[symID].Visible = true
		symbols[symID].Named = best.key.named
	}

	if len(defaultAliases) == 0 {
		return
	}

	for i := range productions {
		if len(productions[i].Aliases) == 0 {
			continue
		}
		filtered := productions[i].Aliases[:0]
		for _, ai := range productions[i].Aliases {
			if ai.ChildIndex < 0 || ai.ChildIndex >= len(productions[i].RHS) {
				filtered = append(filtered, ai)
				continue
			}
			symID := productions[i].RHS[ai.ChildIndex]
			if def, ok := defaultAliases[symID]; ok && def.name == ai.Name && def.named == ai.Named {
				continue
			}
			filtered = append(filtered, ai)
		}
		if len(filtered) == 0 {
			productions[i].Aliases = nil
			continue
		}
		productions[i].Aliases = append([]AliasInfo(nil), filtered...)
	}
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

// aliasInfo records the alias target for a pattern.
type aliasInfo struct {
	name  string
	named bool
}

// collectAliasedPatterns scans the grammar for ALIAS nodes wrapping PATTERN
// leaves. Returns a map from pattern value to its alias info. When a pattern
// is aliased to different names in different places, the first alias wins
// (tree-sitter C uses the first occurrence).
func collectAliasedPatterns(g *Grammar) map[string]aliasInfo {
	result := make(map[string]aliasInfo)

	var walk func(r *Rule)
	walk = func(r *Rule) {
		if r == nil {
			return
		}
		if r.Kind == RuleAlias && len(r.Children) > 0 {
			child := r.Children[0]
			if child != nil && child.Kind == RulePattern {
				if _, exists := result[child.Value]; !exists {
					result[child.Value] = aliasInfo{
						name:  r.Value,
						named: r.Named,
					}
				}
			}
		}
		for _, c := range r.Children {
			walk(c)
		}
	}

	for _, name := range g.RuleOrder {
		rule := g.Rules[name]
		if !isTerminalRule(rule) {
			walk(rule)
		}
	}
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
			// Visible (non-underscore) bare-STRING rules become nonterminals,
			// matching tree-sitter C behavior: the string becomes an anonymous
			// terminal, and the visible rule wraps it with a production
			// (e.g. pass_statement → "pass"). Without this, the keyword promotion
			// system produces the anonymous terminal which has no parse table action,
			// because actions are on the named token symbol that the DFA never emits.
			isVisible := !strings.HasPrefix(name, "_")
			isBareString := terminalStringValue(rule) != ""
			if (isVisible && isBareString) || (isBareString && sharedStrings[terminalStringValue(rule)]) {
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

	// Recurse into children — copy-on-write to avoid mutating the original rule tree.
	var newChildren []*Rule
	for i, c := range r.Children {
		nc := liftTokensInRule(c, parentName, st, entries, counter, dedup)
		if nc != c && newChildren == nil {
			newChildren = make([]*Rule, len(r.Children))
			copy(newChildren, r.Children)
		}
		if newChildren != nil {
			newChildren[i] = nc
		}
	}
	if newChildren != nil {
		out := *r
		out.Children = newChildren
		return &out
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
		preparedInner := prepareRule(cloneRule(r.Children[0]), parentName, st, auxRules, counter)
		// If the inner rule is a wide CHOICE and we have a lift threshold,
		// extract it into an auxiliary nonterminal to prevent the repeat
		// helper from creating N² productions (where N = choice width).
		preparedInner = maybeExtractWideChoice(preparedInner, parentName, st, auxRules, counter)
		if st.binaryRepeatMode {
			auxName := ensureRepeatAuxBinary(parentName, preparedInner, st, auxRules, counter)
			return Choice(Sym(auxName), Blank())
		}
		auxName := ensureRepeatAuxLinear(parentName, preparedInner, st, auxRules, counter)
		return Choice(Blank(), cloneRule(preparedInner), Sym(auxName))

	case RuleRepeat1:
		preparedInner := prepareRule(cloneRule(r.Children[0]), parentName, st, auxRules, counter)
		preparedInner = maybeExtractWideChoice(preparedInner, parentName, st, auxRules, counter)
		if st.binaryRepeatMode {
			auxName := ensureRepeatAuxBinary(parentName, preparedInner, st, auxRules, counter)
			return Sym(auxName)
		}
		auxName := ensureRepeatAuxLinear(parentName, preparedInner, st, auxRules, counter)
		return Choice(cloneRule(preparedInner), Sym(auxName))

	case RuleOptional:
		// optional(x) → choice(x, blank)
		inner := prepareRule(r.Children[0], parentName, st, auxRules, counter)
		return Choice(inner, Blank())

	}

	// Recurse into children.
	for i, c := range r.Children {
		r.Children[i] = prepareRule(c, parentName, st, auxRules, counter)
	}

	// For SEQ nodes in grammars with ChoiceLiftThreshold: lift inline CHOICE
	// children whose width exceeds the threshold into auxiliary nonterminals.
	// This prevents Cartesian product explosion in production extraction.
	// Only enabled for grammars that explicitly opt in (e.g. COBOL with 1071 rules).
	if r.Kind == RuleSeq && st.choiceLiftThreshold > 0 {
		r = liftLargeSeqChoices(r, parentName, st, auxRules, counter)
	}

	return r
}

// liftLargeSeqChoices lifts CHOICE children of a SEQ node into auxiliary
// hidden nonterminals when the Cartesian product of alternatives would
// exceed a production budget. The budget is choiceLiftThreshold^2 (e.g.
// threshold=8 → budget=64 productions per SEQ). Choices are lifted
// widest-first until the product is within budget.
// maybeExtractWideChoice checks if r is a CHOICE with more children than
// choiceLiftThreshold. If so, it extracts the CHOICE into an auxiliary
// nonterminal and returns a symbol reference. This prevents repeat helpers
// from creating N² productions when their body is a wide CHOICE.
func maybeExtractWideChoice(r *Rule, parentName string, st *symbolTable, auxRules map[string]*Rule, counter *int) *Rule {
	if st.choiceLiftThreshold <= 0 || r == nil || r.Kind != RuleChoice {
		return r
	}
	if len(r.Children) <= st.choiceLiftThreshold {
		return r
	}
	*counter++
	auxName := fmt.Sprintf("_%s_choice_lift%d", parentName, *counter)
	if _, exists := st.lookupNonterm(auxName); !exists {
		st.addSymbol(auxName, SymbolInfo{
			Name: auxName, Visible: false, Named: false, Kind: SymbolNonterminal,
		})
		auxRules[auxName] = r
	}
	return Sym(auxName)
}

// estimateAlternativeCount returns the number of flat alternatives that
// enumerateAlternatives would produce for a rule.
func estimateAlternativeCount(r *Rule) int {
	if r == nil {
		return 1
	}
	switch r.Kind {
	case RuleChoice:
		n := 0
		for _, c := range r.Children {
			n += estimateAlternativeCount(c)
		}
		if n == 0 {
			return 1
		}
		return n
	case RuleSeq:
		product := 1
		for _, c := range r.Children {
			product *= estimateAlternativeCount(c)
			if product > 10000 {
				return product
			}
		}
		return product
	case RuleBlank:
		return 1
	case RuleField, RuleAlias, RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			return estimateAlternativeCount(r.Children[0])
		}
		return 1
	default:
		return 1
	}
}

// liftLargeSeqChoices examines a SEQ node and lifts CHOICE children into
// auxiliary nonterminals when the total Cartesian product exceeds the
// choiceLiftThreshold. Only active when st.choiceLiftThreshold > 0.
func liftLargeSeqChoices(seq *Rule, parentName string, st *symbolTable, auxRules map[string]*Rule, counter *int) *Rule {
	threshold := st.choiceLiftThreshold
	if threshold <= 0 {
		return seq
	}

	total := estimateAlternativeCount(seq)
	if total <= threshold {
		return seq
	}

	newChildren := make([]*Rule, len(seq.Children))
	copy(newChildren, seq.Children)

	for total > threshold {
		// Find the widest liftable CHOICE child.
		bestIdx := -1
		bestAlts := 0
		for i, c := range newChildren {
			if c == nil {
				continue
			}
			alts := estimateAlternativeCount(c)
			if alts > bestAlts && alts >= 2 {
				inner := c
				for inner != nil && (inner.Kind == RuleField || inner.Kind == RuleAlias ||
					inner.Kind == RulePrec || inner.Kind == RulePrecLeft ||
					inner.Kind == RulePrecRight || inner.Kind == RulePrecDynamic) {
					if len(inner.Children) > 0 {
						inner = inner.Children[0]
					} else {
						break
					}
				}
				if inner != nil && inner.Kind == RuleChoice {
					bestIdx = i
					bestAlts = alts
				}
			}
		}
		if bestIdx < 0 || bestAlts <= 1 {
			break
		}

		*counter++
		auxName := fmt.Sprintf("_%s_choice_lift%d", parentName, *counter)
		if _, exists := st.lookupNonterm(auxName); !exists {
			st.addSymbol(auxName, SymbolInfo{
				Name: auxName, Visible: false, Named: false, Kind: SymbolNonterminal,
			})
			auxRules[auxName] = cloneRule(newChildren[bestIdx])
		}
		newChildren[bestIdx] = Sym(auxName)

		newSeq := &Rule{Kind: RuleSeq, Children: newChildren}
		total = estimateAlternativeCount(newSeq)
	}

	result := *seq
	result.Children = newChildren
	return &result
}

func ensureRepeatAuxLinear(parentName string, inner *Rule, st *symbolTable, auxRules map[string]*Rule, counter *int) string {
	*counter++
	auxName := fmt.Sprintf("%s_repeat%d", parentName, *counter)
	if _, exists := st.lookupNonterm(auxName); !exists {
		st.addSymbol(auxName, SymbolInfo{
			Name: auxName, Visible: false, Named: false, Kind: SymbolNonterminal,
		})
		auxRules[auxName] = Choice(
			Seq(Sym(auxName), cloneRule(inner)),
			Seq(cloneRule(inner), cloneRule(inner)),
		)
	}
	return auxName
}

func ensureRepeatAuxBinary(parentName string, inner *Rule, st *symbolTable, auxRules map[string]*Rule, counter *int) string {
	*counter++
	auxName := fmt.Sprintf("%s_repeat%d", parentName, *counter)
	if _, exists := st.lookupNonterm(auxName); !exists {
		st.addSymbol(auxName, SymbolInfo{
			Name: auxName, Visible: false, Named: false, Kind: SymbolNonterminal,
		})
		auxRules[auxName] = Choice(
			Seq(Sym(auxName), Sym(auxName)),
			cloneRule(inner),
		)
	}
	return auxName
}

// expandTopLevelRepeat expands repeat1 at the top level of a hidden rule into
// the same binary-tree shape tree-sitter uses for repetition auxiliaries.
//
// Only applies when the ENTIRE rule body is a repeat1 (possibly wrapped in
// precedence). Zero-or-more is handled as choice(repeat1, blank), matching
// tree-sitter's grammar.json lowering. Nested repeats inside seq/choice are handled
// normally by prepareRule (which creates aux rules).
func expandTopLevelRepeat(r *Rule, ruleName string, binaryMode bool) *Rule {
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

	if inner.Kind != RuleRepeat1 {
		return r
	}
	x := inner.Children[0]
	var expanded *Rule
	if binaryMode {
		expanded = Choice(
			Seq(Sym(ruleName), Sym(ruleName)),
			cloneRule(x),
		)
	} else {
		expanded = Choice(
			cloneRule(x),
			Seq(cloneRule(x), cloneRule(x)),
			Seq(Sym(ruleName), cloneRule(x)),
		)
	}

	// Re-wrap with precedence if there were any wrappers.
	for i := len(precWrappers) - 1; i >= 0; i-- {
		w := precWrappers[i]
		expanded = &Rule{Kind: w.Kind, Prec: w.Prec, Children: []*Rule{expanded}}
	}
	return expanded
}

func flattenPreparedRules(grammarName string, nonterminals []string, processedRules, auxRules map[string]*Rule, supertypes []string) (map[string]*Rule, map[string]*Rule) {
	tmp := NewGrammar(grammarName)
	tmp.Supertypes = append(tmp.Supertypes, supertypes...)

	for _, name := range nonterminals {
		if rule := processedRules[name]; rule != nil {
			tmp.Define(name, rule)
		}
	}

	auxNames := make([]string, 0, len(auxRules))
	for name := range auxRules {
		auxNames = append(auxNames, name)
	}
	sort.Strings(auxNames)
	for _, name := range auxNames {
		if rule := auxRules[name]; rule != nil {
			tmp.Define(name, rule)
		}
	}

	flattened := flattenHiddenChoiceAlts(tmp)
	if flattened == nil {
		return processedRules, auxRules
	}

	nextProcessed := make(map[string]*Rule, len(processedRules))
	for _, name := range nonterminals {
		nextProcessed[name] = flattened.Rules[name]
	}

	nextAux := make(map[string]*Rule, len(auxRules))
	for _, name := range auxNames {
		nextAux[name] = flattened.Rules[name]
	}

	return nextProcessed, nextAux
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

	// All non-immediate terminals use prec-based priority: -prec*1000.
	// This matches tree-sitter C where tokens at the same precedence level share
	// the same AcceptPriority and the runtime's greedy longest-match tiebreaker
	// decides among them. Tokens with a negative explicit prec (like diff's
	// token(prec(-1,...))) get a higher priority number (= worse priority) so
	// they lose to prec=0 tokens regardless of match length. IMMTOKEN terminals
	// that have no longer non-immediate sibling get an additional -10000 bonus.

	// String literals (prec is always 0 for bare string literals).
	for _, s := range stringLits {
		id, ok := st.lookup(s)
		if !ok {
			continue
		}
		// Skip keywords — they're recognized via the word token + keyword DFA.
		if keywordSet != nil && keywordSet[id] {
			continue
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID: id,
			Rule:     Str(s),
			Priority: 0,
		})
	}

	// Named tokens: split into string-only and non-string groups for ordering,
	// but both use the same prec-based priority scheme.
	var stringNamedTokens, patternNamedTokens []string
	for _, name := range namedTokens {
		rule := g.Rules[name]
		if isStringOnlyToken(rule) {
			stringNamedTokens = append(stringNamedTokens, name)
		} else {
			patternNamedTokens = append(patternNamedTokens, name)
		}
	}

	// String-only named tokens: prec-based priority, greedy decides ties.
	for _, name := range stringNamedTokens {
		id, ok := st.lookupNamedToken(name)
		if !ok {
			continue
		}
		rule := g.Rules[name]
		expanded, imm, prec, err := expandTokenRule(rule)
		if err != nil {
			return nil, fmt.Errorf("expand token %q: %w", name, err)
		}
		adjustedPriority := -prec * 1000
		if imm {
			adjustedPriority -= 10000
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID:  id,
			Rule:      expanded,
			Priority:  adjustedPriority,
			Immediate: imm,
		})
	}

	// Inline patterns (regex appearing directly in non-terminal rules, not in token()).
	// These have no explicit prec, so priority is 0.
	for _, pat := range inlinePatterns {
		id, ok := st.lookup(inlinePatternSymbolKey(pat))
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
			Priority: 0,
		})
	}

	// Non-string named tokens: prec-based priority, greedy decides ties.
	// Skip keywords — they're recognized via the word token + keyword DFA.
	for _, name := range patternNamedTokens {
		id, ok := st.lookupNamedToken(name)
		if !ok {
			continue
		}
		// Skip pattern-based keywords (e.g. COBOL case-insensitive keywords).
		if keywordSet != nil && keywordSet[id] {
			continue
		}
		rule := g.Rules[name]
		expanded, imm, prec, err := expandTokenRule(rule)
		if err != nil {
			return nil, fmt.Errorf("expand token %q: %w", name, err)
		}
		adjustedPriority := -prec * 1000
		if imm {
			adjustedPriority -= 10000
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID:  id,
			Rule:      expanded,
			Priority:  adjustedPriority,
			Immediate: imm,
		})
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
		adjustedPriority := -prec * 1000
		if entry.immediate {
			if expanded.Kind == RuleString && hasLongerStringPrefixPattern(patterns, expanded.Value) {
				// IMMTOKEN "#" has a longer non-immediate sibling "#)".
				// Don't apply bonus; use same prec-based priority so greedy
				// picks the longer non-immediate string over this IMMTOKEN.
			} else if !isStringOnlyRule(expanded) {
				// Pattern-based IMMTOKEN (e.g. [^\n'] for char content):
				// use a modest -500 bonus so it beats regular tokens (prio 0)
				// but loses to tokens with explicit PREC(1) (prio -1000).
				// This prevents broad catch-all IMMTOKENs from defeating
				// more specific TOKEN(PREC(1,...)) patterns like escape_sequence.
				adjustedPriority -= 500
			} else {
				// String-based IMMTOKEN: use the full -10000 bonus.
				// String IMMTOKENs are specific and should always beat
				// non-immediate tokens sharing the same lex mode.
				adjustedPriority -= 10000
			}
		}
		patterns = append(patterns, TerminalPattern{
			SymbolID:  id,
			Rule:      expanded,
			Priority:  adjustedPriority,
			Immediate: entry.immediate,
		})
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
				Priority: 2000, // worst priority: only consumed when no grammar token matches
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

// identifyKeywords determines which terminals are keywords.
// A keyword is a terminal whose accepted strings all match the word token's
// pattern. This includes both string literals (e.g. "if") and pattern-based
// terminals (e.g. [iI][fF] for case-insensitive keywords like COBOL).
// Returns the keyword set, ordered symbol IDs, and terminal patterns
// for keyword DFA construction.
func identifyKeywords(g *Grammar, st *symbolTable, stringLits []string, namedTokens []string) (map[int]bool, []int, []TerminalPattern) {
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

	wordDFA, _ := subsetConstruction(context.Background(), &nfa{states: b.states, start: frag.start})

	keywordSet := make(map[int]bool)
	var keywordSyms []int
	var keywordEntries []TerminalPattern

	for _, s := range stringLits {
		id, ok := st.lookup(s)
		if !ok {
			continue
		}
		// Treat only identifier-like literals as keyword candidates.
		// Some grammars have broad `word` tokens that also match punctuation
		// literals (e.g. //, $$), which should remain regular terminals.
		if matchesDFA(wordDFA, s) && isIdentifierLikeKeywordLiteral(s) {
			keywordSet[id] = true
			keywordSyms = append(keywordSyms, id)
			// All keyword entries use the same priority (0) so the DFA
			// lexer's greedy longest-match tiebreaker selects correctly.
			// Sequential priorities would cause shorter prefix keywords
			// (e.g. "as") to beat longer ones (e.g. "assert") because the
			// lexer prefers lower priority numbers.
			keywordEntries = append(keywordEntries, TerminalPattern{
				SymbolID: id,
				Rule:     Str(s),
				Priority: 0,
			})
		}
	}

	// Also check named pattern tokens for keyword candidacy. This handles
	// grammars like COBOL where keywords are case-insensitive patterns
	// (e.g. _IDENTIFICATION = [iI][dD][eE]...) rather than string literals.
	// A pattern terminal is a keyword if every string it accepts is also
	// accepted by the word token's pattern.
	for _, name := range namedTokens {
		// Skip the word token itself.
		if name == g.Word {
			continue
		}
		rule := g.Rules[name]
		if rule == nil {
			continue
		}
		// Only consider hidden pattern terminals (starting with "_").
		// Visible named tokens are nonterminals wrapping productions.
		if !strings.HasPrefix(name, "_") {
			continue
		}
		// Only consider pure pattern rules (not token() or token.immediate() wrappers,
		// which may have precedence semantics that matter for lexing).
		if rule.Kind != RulePattern {
			continue
		}
		id, ok := st.lookupNamedToken(name)
		if !ok {
			if id2, ok2 := st.lookup(name); ok2 {
				id = id2
			} else {
				continue
			}
		}
		// Skip if already identified (shouldn't happen, but be safe).
		if keywordSet[id] {
			continue
		}
		// Build a DFA for this candidate pattern and check if its language
		// is a subset of the word token's language.
		candExpanded, err := expandPatternRule(rule.Value)
		if err != nil {
			continue
		}
		cb := newNFABuilder()
		candFrag, err := cb.buildFromRule(candExpanded)
		if err != nil {
			continue
		}
		cb.states[candFrag.end].accept = 1
		cb.states[candFrag.end].priority = 0
		candDFA, err := subsetConstruction(context.Background(), &nfa{states: cb.states, start: candFrag.start})
		if err != nil || len(candDFA) == 0 {
			continue
		}
		if dfaAcceptsSubsetOf(candDFA, wordDFA) {
			keywordSet[id] = true
			keywordSyms = append(keywordSyms, id)
			keywordEntries = append(keywordEntries, TerminalPattern{
				SymbolID: id,
				Rule:     candExpanded,
				Priority: 0,
			})
		}
	}

	// Named token rules that expand to a finite set of identifier-like string
	// literals can also ride the keyword DFA path. This captures visible tokens
	// like C#'s predefined_type = token(choice("int", "string", ...)).
	for _, name := range namedTokens {
		if name == g.Word {
			continue
		}
		rule := g.Rules[name]
		if rule == nil {
			continue
		}
		id, ok := st.lookupNamedToken(name)
		if !ok {
			if id2, ok2 := st.lookup(name); ok2 {
				id = id2
			} else {
				continue
			}
		}
		if keywordSet[id] {
			continue
		}
		expanded, _, _, err := expandTokenRule(rule)
		if err != nil || !isStringOnlyRule(expanded) {
			continue
		}
		lits, ok := collectStringOnlyRuleLiterals(expanded)
		if !ok || len(lits) == 0 {
			continue
		}
		allKeyword := true
		for _, lit := range lits {
			if !matchesDFA(wordDFA, lit) || !isIdentifierLikeKeywordLiteral(lit) {
				allKeyword = false
				break
			}
		}
		if !allKeyword {
			continue
		}
		keywordSet[id] = true
		keywordSyms = append(keywordSyms, id)
		keywordEntries = append(keywordEntries, TerminalPattern{
			SymbolID: id,
			Rule:     expanded,
			Priority: 0,
		})
	}

	return keywordSet, keywordSyms, keywordEntries
}

func collectStringOnlyRuleLiterals(r *Rule) ([]string, bool) {
	if r == nil {
		return nil, false
	}
	switch r.Kind {
	case RuleString:
		return []string{r.Value}, true
	case RuleSeq:
		out := []string{""}
		for _, child := range r.Children {
			childLits, ok := collectStringOnlyRuleLiterals(child)
			if !ok {
				return nil, false
			}
			next := make([]string, 0, len(out)*len(childLits))
			for _, prefix := range out {
				for _, lit := range childLits {
					next = append(next, prefix+lit)
				}
			}
			out = next
		}
		return out, len(out) > 0
	case RuleChoice:
		out := make([]string, 0, len(r.Children))
		for _, child := range r.Children {
			childLits, ok := collectStringOnlyRuleLiterals(child)
			if !ok {
				return nil, false
			}
			out = append(out, childLits...)
		}
		return out, len(out) > 0
	default:
		return nil, false
	}
}

// dfaAcceptsSubsetOf checks whether every string accepted by DFA 'candidate'
// is also accepted by DFA 'word'. Uses a product automaton approach: explores
// all reachable (candidate_state, word_state) pairs. If any pair is reached
// where 'candidate' accepts but 'word' does not, returns false.
// A word_state of -1 means "dead state" (no transition found in word DFA).
func dfaAcceptsSubsetOf(candidate, word []dfaState) bool {
	if len(candidate) == 0 {
		return true
	}
	if len(word) == 0 {
		// Word DFA is empty — candidate can only be a subset if it also accepts nothing.
		for _, s := range candidate {
			if s.accept > 0 {
				return false
			}
		}
		return true
	}

	// Product state: (candidate state, word state). word state -1 = dead.
	type pair struct{ c, w int }
	visited := make(map[pair]bool)
	stack := []pair{{0, 0}}
	visited[pair{0, 0}] = true

	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		// Check accept: if candidate accepts here, word must also accept.
		if candidate[cur.c].accept > 0 {
			if cur.w < 0 || word[cur.w].accept == 0 {
				return false
			}
		}

		// Explore transitions from candidate state.
		for _, ct := range candidate[cur.c].transitions {
			// For each character in candidate's transition range, find
			// the corresponding word DFA transition (or dead state).
			// Since character ranges can overlap partially, split the
			// candidate range against word transitions.
			nextC := ct.nextState
			wTransitions := wordTransitionsForRange(word, cur.w, ct.lo, ct.hi)
			for _, wt := range wTransitions {
				next := pair{nextC, wt.nextState}
				if !visited[next] {
					visited[next] = true
					stack = append(stack, next)
				}
			}
		}
	}
	return true
}

// wordTransEntry maps a sub-range to a word DFA next state (-1 = dead).
type wordTransEntry struct {
	lo, hi    rune
	nextState int // -1 = dead
}

// wordTransitionsForRange returns the word DFA transitions covering the
// character range [lo, hi]. Each piece of the range maps to either a word
// DFA state or -1 (dead). If wordState is already -1 (dead), all
// transitions go to -1.
func wordTransitionsForRange(word []dfaState, wordState int, lo, hi rune) []wordTransEntry {
	if wordState < 0 {
		return []wordTransEntry{{lo, hi, -1}}
	}
	var result []wordTransEntry
	pos := lo
	for _, wt := range word[wordState].transitions {
		if wt.hi < pos {
			continue
		}
		if wt.lo > hi {
			break
		}
		// Gap before this word transition: [pos, wt.lo-1] → dead
		if wt.lo > pos {
			gapHi := wt.lo - 1
			if gapHi > hi {
				gapHi = hi
			}
			result = append(result, wordTransEntry{pos, gapHi, -1})
		}
		// Overlap: [max(pos, wt.lo), min(hi, wt.hi)] → wt.nextState
		overlapLo := pos
		if wt.lo > overlapLo {
			overlapLo = wt.lo
		}
		overlapHi := hi
		if wt.hi < overlapHi {
			overlapHi = wt.hi
		}
		if overlapLo <= overlapHi {
			result = append(result, wordTransEntry{overlapLo, overlapHi, wt.nextState})
		}
		pos = overlapHi + 1
		if pos > hi {
			break
		}
	}
	// Remaining range after all word transitions: dead
	if pos <= hi {
		result = append(result, wordTransEntry{pos, hi, -1})
	}
	return result
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
	prec, assoc, dynPrec, hasPrec, inner := unwrapPrec(r)

	switch inner.Kind {
	case RuleChoice:
		var prods []Production
		for _, alt := range inner.Children {
			altPrec, altAssoc, altDyn, altHasPrec, altInner := unwrapPrec(alt)
			// Direct blank alternatives should not inherit an outer explicit
			// zero-precedence wrapper here. Otherwise constructs like
			// prec.right(0, choice(statement_block, blank)) manufacture an
			// explicit epsilon reduce that can beat the concrete branch too
			// early (e.g. TypeScript `namespace ts { ... }` reducing the body
			// to blank before seeing `{`). Positive precedence is still copied
			// to epsilon arms below via the sibling-propagation step.
			inheritOuterToBlank := altInner.Kind != RuleBlank
			if altPrec == 0 && inheritOuterToBlank {
				altPrec = prec
			}
			if !altHasPrec && inheritOuterToBlank {
				altHasPrec = hasPrec
			}
			if altAssoc == AssocNone && inheritOuterToBlank {
				altAssoc = assoc
			}
			if altDyn == 0 && inheritOuterToBlank {
				altDyn = dynPrec
			}
			// Recursively flatten — alternatives may contain more choices.
			altProds := flattenRule2(altInner, lhsID, st, prodIDCounter)
			for i := range altProds {
				suppressExplicitZeroWrapperAssoc := false
				if altPrec == 0 && altHasPrec && altAssoc != AssocNone && altDyn == 0 &&
					len(altProds[i].RHS) == 1 && len(altProds[i].Fields) == 0 && len(altProds[i].Aliases) == 0 {
					rhsSym := altProds[i].RHS[0]
					if rhsSym >= 0 && rhsSym < len(st.symbols) && st.symbols[rhsSym].Kind == SymbolNonterminal {
						// A top-level prec.left/right(0, choice(...)) wrapper around a
						// single nonterminal pass-through should not manufacture an
						// explicit zero-precedence reduce on the wrapper production
						// itself. That metadata belongs on real sequence productions;
						// attaching it to wrapper reductions like `_string ->
						// concatenated_string` can make C GNU asm prefer the wrapper
						// reduce over shifting the following operand-list colon.
						suppressExplicitZeroWrapperAssoc = true
					}
				}
				if altProds[i].Prec == 0 {
					altProds[i].Prec = altPrec
				}
				if !suppressExplicitZeroWrapperAssoc && !altProds[i].HasExplicitPrec && altHasPrec {
					altProds[i].HasExplicitPrec = true
				}
				if !suppressExplicitZeroWrapperAssoc && altProds[i].Assoc == AssocNone {
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
		lhsName := ""
		if lhsID >= 0 && lhsID < len(st.symbols) {
			lhsName = st.symbols[lhsID].Name
		}
		if maxPrec > 0 && !strings.Contains(lhsName, "_choice_lift") {
			for i := range prods {
				if prods[i].Prec == 0 && len(prods[i].RHS) == 0 {
					prods[i].Prec = maxPrec
					prods[i].HasExplicitPrec = true
					if prods[i].Assoc == AssocNone {
						prods[i].Assoc = maxAssoc
					}
				}
			}
		}
		return prods

	case RuleBlank:
		prod := Production{
			LHS:             lhsID,
			Prec:            prec,
			HasExplicitPrec: hasPrec,
			Assoc:           assoc,
			DynPrec:         dynPrec,
			ProductionID:    *prodIDCounter,
		}
		*prodIDCounter++
		return []Production{prod}

	default:
		// Enumerate all alternatives from Choice-within-Seq by expanding
		// the rule into a list of "flat" RHS sequences.
		alternatives := enumerateAlternatives(inner)
		trailingAutoSemiOptional := hasTrailingAutomaticSemicolonOptional(inner)
		var prods []Production
		for _, alt := range alternatives {
			// Compute per-alternative prec: use rightmost element's prec
			// (matching tree-sitter's behavior where the rightmost prec
			// wrapper in a production wins). Fall back to the outer prec
			// from unwrapPrec, then scanInnerPrec as last resort.
			altPrec, altAssoc, altDyn, altHasPrec := prec, assoc, dynPrec, hasPrec
			altIncludesBlankChoice := false
			altHasInnerPrec := false
			for _, elem := range alt {
				if elem.blank {
					altIncludesBlankChoice = true
					continue
				}
				if elem.hasPrec {
					altPrec = elem.prec
					altHasPrec = true
					altHasInnerPrec = true
				}
				if elem.assoc != AssocNone {
					altAssoc = elem.assoc
					altHasInnerPrec = true
				}
				if elem.dynPrec != 0 {
					altDyn = elem.dynPrec
					altHasInnerPrec = true
				}
			}
			trailingAutoSemiBlankOmission := trailingAutoSemiOptional && len(alt) > 0 && alt[len(alt)-1].blank
			if altIncludesBlankChoice && !altHasInnerPrec && prec == 0 && dynPrec == 0 && !trailingAutoSemiBlankOmission {
				// Alternatives expanded from a blank choice arm should not inherit
				// an outer explicit zero-precedence wrapper. Otherwise optional
				// suffixes like seq(name, choice(statement_block, blank)) get a
				// synthetic explicit epsilon-style reduce that can beat shifting
				// the concrete branch too early (TypeScript internal_module).
				altPrec = 0
				altHasPrec = false
				altAssoc = AssocNone
			}
			if !altHasPrec && altPrec == 0 && altAssoc == AssocNone && altDyn == 0 {
				altPrec, altAssoc, altDyn, altHasPrec = scanInnerPrec(inner)
			}

			prod := Production{
				LHS:             lhsID,
				Prec:            altPrec,
				HasExplicitPrec: altHasPrec,
				Assoc:           altAssoc,
				DynPrec:         altDyn,
				ProductionID:    *prodIDCounter,
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

func hasTrailingAutomaticSemicolonOptional(r *Rule) bool {
	if r == nil || r.Kind != RuleSeq || len(r.Children) == 0 {
		return false
	}

	last := r.Children[len(r.Children)-1]
	for last != nil {
		switch last.Kind {
		case RuleField, RuleAlias, RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
			if len(last.Children) == 0 {
				return false
			}
			last = last.Children[0]
		default:
			goto unwrapped
		}
	}

unwrapped:
	if last == nil || last.Kind != RuleChoice {
		return false
	}

	hasBlank := false
	hasAutoSemi := false
	for _, child := range last.Children {
		inner := child
		for inner != nil {
			switch inner.Kind {
			case RuleField, RuleAlias, RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
				if len(inner.Children) == 0 {
					return false
				}
				inner = inner.Children[0]
			default:
				goto childUnwrapped
			}
		}

	childUnwrapped:
		if inner == nil {
			return false
		}
		switch inner.Kind {
		case RuleBlank:
			hasBlank = true
		case RuleSymbol:
			if inner.Value != "_automatic_semicolon" {
				return false
			}
			hasAutoSemi = true
		case RuleString:
			if inner.Value != ";" {
				return false
			}
			hasAutoSemi = true
		default:
			return false
		}
	}

	return hasBlank && hasAutoSemi
}

// rhsElement is a single element in a flattened RHS.
type rhsElement struct {
	rule       *Rule
	blank      bool   // marker for an expanded blank choice arm; not emitted into RHS
	fieldName  string // non-empty if wrapped in a Field
	aliasName  string // non-empty if wrapped in an Alias
	aliasNamed bool   // true if alias is a named symbol ($.name form)
	prec       int    // precedence from enclosing prec wrapper (0 = none)
	hasPrec    bool   // true if an explicit compile-time precedence wrapper was present
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
		// The outer alias must override any inner alias metadata; otherwise nested
		// alias forms like alias(_jsx_identifier, "property_identifier") let the
		// inner alias on _jsx_identifier shadow the outer one and lose the
		// intended surface node type after default-alias cleanup.
		innerAlts := enumerateAlternatives(r.Children[0])
		var result [][]*rhsElement
		for _, alt := range innerAlts {
			tagged := make([]*rhsElement, len(alt))
			for i, elem := range alt {
				cp := *elem
				cp.aliasName = r.Value
				cp.aliasNamed = r.Named
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
						elem.hasPrec = true
						elem.assoc = AssocLeft
					case RulePrecRight:
						elem.prec = r.Prec
						elem.hasPrec = true
						elem.assoc = AssocRight
					case RulePrecDynamic:
						elem.dynPrec = r.Prec
					default: // RulePrec
						elem.prec = r.Prec
						elem.hasPrec = true
						elem.assoc = AssocNone
					}
				}
			}
			return innerAlts
		}
		return [][]*rhsElement{{}}

	case RuleBlank:
		// Epsilon — carry a marker so expanded seq alternatives can detect that
		// they came from a blank choice arm without emitting an RHS symbol.
		return [][]*rhsElement{{&rhsElement{blank: true}}}

	default:
		// Leaf node (String, Symbol, etc.) — single element.
		return [][]*rhsElement{{&rhsElement{rule: r}}}
	}
}

// collectLinearRHS converts a flat list of rhsElements into symbol IDs, field assignments, and alias info.
func collectLinearRHS(elems []*rhsElement, st *symbolTable, rhs *[]int, fields *[]FieldAssign, aliases *[]AliasInfo) {
	for _, elem := range elems {
		if elem.blank {
			continue
		}
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
		// Inline patterns use a distinct internal symbol key so they do not
		// collide with anonymous string terminals that share the same display
		// text (for example SQL's literal ".*" and pg_command regex /.*/).
		if id, ok := st.lookup(inlinePatternSymbolKey(r.Value)); ok {
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

		if len(pt) == 0 {
			continue
		}

		// Cap pass-through count to avoid Cartesian product explosion.
		// When a hidden rule with N pass-through alts is referenced in a Seq
		// with another such rule, production extraction creates N*M alternatives.
		if len(pt) > 8 {
			continue
		}

		// For all-pass-through hidden rules, don't rewrite the rule itself;
		// just inline its direct alternatives at reference sites. This
		// approximates tree-sitter's transitive single-symbol closure.
		if len(compound) == 0 {
			selfRef := false
			for _, alt := range pt {
				if ruleReferencesSym(alt, name) {
					selfRef = true
					break
				}
			}
			if selfRef {
				continue
			}
			flattenMap[name] = &flattenInfo{
				passThrough: pt,
				replaceRule: false,
			}
			continue
		}

		// Skip arbitrary self-recursive flattening, but allow the exact
		// repetition shape introduced by repeat lowering:
		//   choice(seq(self, inner), seq(inner, inner), passthrough...)
		// Tree-sitter flattens those generated pass-through branches after
		// repeat expansion, and doing the same removes the extra cc=1
		// reductions from hidden repeat helpers.
		unsafeSelfRef := false
		allCompoundsAreSelfRecursive := true
		for _, c := range compound {
			if ruleReferencesSym(c, name) {
				if !isRepeatSelfRef(c, name) {
					unsafeSelfRef = true
					break
				}
			} else {
				allCompoundsAreSelfRecursive = false
			}
		}
		if unsafeSelfRef {
			continue
		}

		// When ALL compound alternatives are self-recursive (e.g.,
		// _string_content → _string_content _string_content), stripping
		// the pass-through base cases (string_content, escape_sequence)
		// would leave the rule unreachable — it can only reduce from
		// itself, with no way to create a base instance. Skip flattening
		// for these rules to preserve their base cases.
		if allCompoundsAreSelfRecursive {
			continue
		}

		// When some compounds are self-recursive and the non-self-recursive
		// compounds do NOT share symbols with the pass-through alternatives,
		// the base cases are qualitatively different from the pass-throughs.
		// Stripping the pass-throughs leaves the self-recursive productions
		// reachable only through the non-self-recursive base (e.g. parens),
		// not through the pass-through paths (e.g. variable_expr).
		//
		// Example: HCL's _expr_term has pass-through alts (variable_expr,
		// literal_value, ...) and compounds (_expr_term get_attr,
		// _expr_term index, "(" expression ")"). The non-self-recursive
		// compound "(" expression ")" doesn't share symbols with the
		// pass-throughs, so stripping them means `var.field` can never
		// parse — variable_expr no longer reduces to _expr_term.
		//
		// Contrast with repeat helpers like _items -> item | item item |
		// _items item where the non-self-recursive compound (item item)
		// shares `item` with the pass-through, making flattening safe.
		if !allCompoundsAreSelfRecursive {
			hasSelfRecursiveCompound := false
			for _, c := range compound {
				if ruleReferencesSym(c, name) {
					hasSelfRecursiveCompound = true
					break
				}
			}
			if hasSelfRecursiveCompound {
				ptSyms := make(map[string]bool)
				for _, p := range pt {
					for _, ref := range collectSymbolRefs(p) {
						ptSyms[ref] = true
					}
				}
				nonRecSharesPassthrough := false
				for _, c := range compound {
					if ruleReferencesSym(c, name) {
						continue
					}
					for _, ref := range collectSymbolRefs(c) {
						if ptSyms[ref] {
							nonRecSharesPassthrough = true
							break
						}
					}
					if nonRecSharesPassthrough {
						break
					}
				}
				if !nonRecSharesPassthrough {
					continue
				}
			}
		}

		flattenMap[name] = &flattenInfo{
			passThrough: pt,
			compound:    compound,
			replaceRule: true,
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
		if fi, ok := flattenMap[name]; ok && fi.replaceRule {
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
	out.ReservedWordSets = cloneReservedWordSets(g.ReservedWordSets)
	out.Supertypes = g.Supertypes
	out.Inline = g.Inline
	return out
}

// getTopLevelChoiceAlts returns the rule's top-level choice alternatives,
// recursively flattening nested Choice nodes and re-wrapping alternatives
// through metadata wrappers like precedence, alias, and field.
// If the rule is not a choice, returns nil.
func getTopLevelChoiceAlts(r *Rule) []*Rule {
	if r == nil {
		return nil
	}

	switch r.Kind {
	case RuleChoice:
		var alts []*Rule
		for _, child := range r.Children {
			if childAlts := getTopLevelChoiceAlts(child); childAlts != nil {
				alts = append(alts, childAlts...)
			} else {
				alts = append(alts, child)
			}
		}
		return alts

	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic, RuleAlias, RuleField:
		if len(r.Children) == 0 {
			return nil
		}
		childAlts := getTopLevelChoiceAlts(r.Children[0])
		if childAlts == nil {
			return nil
		}
		alts := make([]*Rule, 0, len(childAlts))
		for _, alt := range childAlts {
			out := *r
			out.Children = []*Rule{alt}
			alts = append(alts, &out)
		}
		return alts
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

func isRepeatSelfRef(r *Rule, name string) bool {
	if r == nil {
		return false
	}
	for {
		switch r.Kind {
		case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
			if len(r.Children) == 0 {
				return false
			}
			r = r.Children[0]
			continue
		}
		break
	}
	if r.Kind != RuleSeq || len(r.Children) != 2 {
		return false
	}
	// Linear shape: seq(self, non-self)
	if isDirectSelfRef(r.Children[0], name) && !ruleReferencesSym(r.Children[1], name) {
		return true
	}
	// Binary shape: seq(self, self)
	if isDirectSelfRef(r.Children[0], name) && isDirectSelfRef(r.Children[1], name) {
		return true
	}
	return false
}

func isDirectSelfRef(r *Rule, name string) bool {
	if r == nil {
		return false
	}
	for {
		switch r.Kind {
		case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
			if len(r.Children) == 0 {
				return false
			}
			r = r.Children[0]
			continue
		}
		break
	}
	return r.Kind == RuleSymbol && r.Value == name
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
	return inlinePassthroughRefsCtx(r, flattenMap, false)
}

// inlinePassthroughRefsCtx is inlinePassthroughRefs with ALIAS context tracking.
// When insideAlias is true, pass-through alternatives that carry their own ALIAS
// have the inner ALIAS stripped so the outer alias can correctly tag the result.
// Without this, enumerateAlternatives would let the inner alias shadow the outer
// one (e.g., YAML's plain_scalar blocking flow_node).
func inlinePassthroughRefsCtx(r *Rule, flattenMap map[string]*flattenInfo, insideAlias bool) *Rule {
	return inlinePassthroughRefsCtxSeen(r, flattenMap, insideAlias, make(map[string]bool))
}

func inlinePassthroughRefsCtxSeen(r *Rule, flattenMap map[string]*flattenInfo, insideAlias bool, expanding map[string]bool) *Rule {
	if r == nil {
		return nil
	}

	// If this is a symbol reference to a flattened hidden rule, expand it.
	if r.Kind == RuleSymbol {
		fi, ok := flattenMap[r.Value]
		if !ok {
			return r
		}
		if insideAlias && !fi.replaceRule {
			return r
		}
		if expanding[r.Value] {
			return r
		}
		expanding[r.Value] = true
		defer delete(expanding, r.Value)
		// Create Choice(original_ref, passthrough_alt1, passthrough_alt2, ...)
		alts := make([]*Rule, 0, len(fi.passThrough)+1)
		alts = append(alts, r) // keep original ref for compound alts
		for _, pt := range fi.passThrough {
			c := cloneRule(pt)
			// When expanding inside an outer ALIAS context, strip inner
			// ALIAS wrappers so the outer alias can tag the result. Without
			// this, the inner alias shadows the outer in enumerateAlternatives
			// (which checks cp.aliasName == "" before applying outer alias).
			if insideAlias {
				c = stripTopAlias(c)
			}
			c = inlinePassthroughRefsCtxSeen(c, flattenMap, insideAlias, expanding)
			alts = append(alts, c)
		}
		return Choice(alts...)
	}

	// Track ALIAS context for children.
	inAlias := insideAlias || r.Kind == RuleAlias

	// Recurse into children.
	if len(r.Children) == 0 {
		return r
	}
	changed := false
	newChildren := make([]*Rule, len(r.Children))
	for i, c := range r.Children {
		nc := inlinePassthroughRefsCtxSeen(c, flattenMap, inAlias, expanding)
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

// stripTopAlias removes a top-level ALIAS wrapper, unwrapping through
// precedence wrappers to find it. Returns the inner rule without the alias.
func stripTopAlias(r *Rule) *Rule {
	if r == nil {
		return nil
	}
	if r.Kind == RuleAlias && len(r.Children) > 0 {
		return r.Children[0]
	}
	// Unwrap precedence wrappers.
	switch r.Kind {
	case RulePrec, RulePrecLeft, RulePrecRight, RulePrecDynamic:
		if len(r.Children) > 0 {
			inner := stripTopAlias(r.Children[0])
			if inner != r.Children[0] {
				out := *r
				out.Children = []*Rule{inner}
				return &out
			}
		}
	}
	return r
}

type flattenInfo struct {
	passThrough []*Rule
	compound    []*Rule
	replaceRule bool
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
	out.ReservedWordSets = cloneReservedWordSets(g.ReservedWordSets)
	out.Supertypes = g.Supertypes
	out.Precedences = g.Precedences
	out.BinaryRepeatMode = g.BinaryRepeatMode
	out.EnableLRSplitting = g.EnableLRSplitting
	out.ChoiceLiftThreshold = g.ChoiceLiftThreshold
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
func scanInnerPrec(r *Rule) (prec int, assoc Assoc, dynPrec int, hasPrec bool) {
	if r == nil {
		return 0, AssocNone, 0, false
	}
	switch r.Kind {
	case RulePrec:
		prec = r.Prec
		hasPrec = true
		assoc = AssocNone
	case RulePrecLeft:
		prec = r.Prec
		hasPrec = true
		assoc = AssocLeft
	case RulePrecRight:
		prec = r.Prec
		hasPrec = true
		assoc = AssocRight
	case RulePrecDynamic:
		dynPrec = r.Prec
	}
	for _, child := range r.Children {
		cp, ca, cd, childHasPrec := scanInnerPrec(child)
		if childHasPrec {
			prec = cp
			hasPrec = true
			assoc = ca
		}
		if cd != 0 {
			dynPrec = cd
		}
	}
	return
}

// unwrapPrec strips precedence/associativity wrappers from a rule.
func unwrapPrec(r *Rule) (prec int, assoc Assoc, dynPrec int, hasPrec bool, inner *Rule) {
	for r != nil {
		switch r.Kind {
		case RulePrec:
			prec = r.Prec
			hasPrec = true
			assoc = AssocNone
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
		case RulePrecLeft:
			prec = r.Prec
			hasPrec = true
			assoc = AssocLeft
			if len(r.Children) > 0 {
				r = r.Children[0]
				continue
			}
		case RulePrecRight:
			prec = r.Prec
			hasPrec = true
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
	return prec, assoc, dynPrec, hasPrec, r
}
