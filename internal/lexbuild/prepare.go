package lexbuild

import (
	"fmt"
	"sort"
	"strings"
)

// ExternalToken is grammars.rs: struct ExternalToken.
type ExternalToken struct {
	Ident                 string
	Kind                  VariableType
	CorrespondingInternal Symbol
	HasCorresponding      bool
}

// LexicalToken is extract_tokens.rs: struct LexicalToken.
type LexicalToken struct {
	Ident string
	Kind  VariableType
	Root  RuleID
}

type externalName struct {
	name    string
	hasName bool
	kind    VariableType
}

// internedMeta is intern_symbols.rs: struct InternedGrammarMeta.
type internedMeta struct {
	kinds          []VariableType
	externalTokens []externalName
	supertypes     []Symbol
	conflicts      [][]Symbol
	inline         []Symbol
	word           Symbol
	hasWord        bool
}

// extractedMeta is extract_tokens.rs: struct ExtractedGrammarMeta.
type extractedMeta struct {
	kinds            []VariableType
	lexicalVariables []LexicalToken
	separatorRoots   []RuleID
	extraSymbols     []Symbol
	externalTokens   []ExternalToken
	supertypes       []Symbol
	conflicts        [][]Symbol
	inline           []Symbol
	word             Symbol
	hasWord          bool
	reservedSets     []reservedSet
}

type reservedSet struct {
	name    string
	symbols []Symbol
}

// variableTypeForName is intern_symbols.rs: fn variable_type_for_name.
func variableTypeForName(name string) VariableType {
	if strings.HasPrefix(name, "_") {
		return VarHidden
	}
	return VarNamed
}

// internSymbols is intern_symbols.rs: fn intern_symbols. Diagnostics are not
// produced.
func internSymbols(g *InputGrammar) (*internedMeta, error) {
	pool := g.Pool
	if len(g.Variables) == 0 {
		return nil, fmt.Errorf("lexbuild: the grammar has no rules")
	}
	nameOfSymbol := map[string]Symbol{}
	for i, v := range g.Variables {
		nameOfSymbol[v.Ident] = nonTerminalSym(i)
	}
	for i, root := range g.ExternalRoots {
		if n := pool.node(root); n.Kind == RNamedSymbol {
			if _, ok := nameOfSymbol[n.Str]; !ok { // entry(sid).or_insert_with
				nameOfSymbol[n.Str] = externalSym(i)
			}
		}
	}
	if variableTypeForName(g.Variables[0].Ident) == VarHidden {
		return nil, fmt.Errorf("A grammar's start rule must be visible.")
	}
	m := &internedMeta{}
	for _, root := range g.ExternalRoots {
		if n := pool.node(root); n.Kind == RNamedSymbol {
			m.externalTokens = append(m.externalTokens, externalName{n.Str, true, variableTypeForName(n.Str)})
		} else {
			m.externalTokens = append(m.externalTokens, externalName{"", false, VarAnonymous})
		}
	}
	for _, v := range g.Variables {
		m.kinds = append(m.kinds, variableTypeForName(v.Ident))
	}
	var stack []RuleID
	for _, v := range g.Variables {
		if err := internRoot(pool, v.Root, nameOfSymbol, &stack); err != nil {
			return nil, err
		}
	}
	for _, root := range g.ExternalRoots {
		if err := internRoot(pool, root, nameOfSymbol, &stack); err != nil {
			return nil, err
		}
	}
	for _, root := range g.ExtraRoots {
		if err := internRoot(pool, root, nameOfSymbol, &stack); err != nil {
			return nil, err
		}
	}
	for _, set := range g.ReservedSets {
		for _, root := range set.Roots {
			if err := internRoot(pool, root, nameOfSymbol, &stack); err != nil {
				return nil, err
			}
		}
	}
	var supertypes []Symbol
	for _, s := range g.SupertypeNames {
		sym, ok := nameOfSymbol[s]
		if !ok {
			return nil, fmt.Errorf("Undefined symbol `%s` in grammar's supertypes array", s)
		}
		supertypes = append(supertypes, sym)
	}
	for _, c := range g.ConflictNames {
		var row []Symbol
		for _, s := range c {
			sym, ok := nameOfSymbol[s]
			if !ok {
				return nil, fmt.Errorf("Undefined symbol `%s` in grammar's conflicts array", s)
			}
			row = append(row, sym)
		}
		m.conflicts = append(m.conflicts, row)
	}
	for _, s := range g.InlineNames {
		if sym, ok := nameOfSymbol[s]; ok {
			m.inline = append(m.inline, sym)
		}
	}
	for _, sym := range supertypes {
		inlined := false
		for _, in := range m.inline {
			if in == sym {
				inlined = true
			}
		}
		if !inlined {
			m.supertypes = append(m.supertypes, sym)
		}
	}
	if g.HasWord {
		sym, ok := nameOfSymbol[g.WordName]
		if !ok {
			return nil, fmt.Errorf("Undefined symbol `%s` as grammar's word token", g.WordName)
		}
		m.word, m.hasWord = sym, true
	}
	for _, s := range m.supertypes {
		if s.isNonTerminal() {
			m.kinds[s.Index] = VarHidden
		}
	}
	return m, nil
}

// internRoot is intern_symbols.rs: fn intern_root. The walk order does not
// reach the output (each NamedSymbol node is rewritten in place).
func internRoot(pool *RulePool, root RuleID, names map[string]Symbol, stack *[]RuleID) error {
	*stack = append((*stack)[:0], root)
	for len(*stack) > 0 {
		id := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		n := pool.node(id)
		switch n.Kind {
		case RNamedSymbol:
			s, ok := names[n.Str]
			if !ok {
				return fmt.Errorf("Undefined symbol `%s`", n.Str)
			}
			pool.setNode(id, symRule(s))
		case RSeq, RChoice:
			base := len(*stack)
			*stack = append(*stack, n.Children...)
			reverseIDs((*stack)[base:])
		case RRepeat, RMetadata, RReserved:
			*stack = append(*stack, n.Inner)
		}
	}
	return nil
}

// tokenExtractor is extract_tokens.rs: struct TokenExtractor. The Rust memo
// is a hash table from the subtree hash to candidate indices; a linear scan
// in index order finds the same first equal entry, because equal subtrees have
// equal hashes.
type tokenExtractor struct {
	lexical     []LexicalToken
	usageCounts []uint32
}

// extractToken is extract_tokens.rs: fn extract_token.
func (t *tokenExtractor) extractToken(pool *RulePool, tokenRoot RuleID, stringName string, hasString bool, varName string, auxTokenCount *uint32, isFirst bool) (uint32, error) {
	for i := range t.lexical {
		if pool.subtreeEq(t.lexical[i].Root, tokenRoot) {
			t.usageCounts[i]++
			return uint32(i), nil
		}
	}
	var name string
	var kind VariableType
	if hasString {
		if stringName == "" && !isFirst {
			return 0, fmt.Errorf("The rule `%s` contains an empty string.\n\nTree-sitter does not support syntactic rules that contain an empty string\nunless they are used only as the grammar's start rule.\n", varName)
		}
		name, kind = stringName, VarAnonymous
	} else {
		*auxTokenCount++
		name = fmt.Sprintf("%s_token%d", varName, *auxTokenCount)
		kind = VarAuxiliary
	}
	index := uint32(len(t.lexical))
	root := pool.pushNode(pool.node(tokenRoot))
	t.lexical = append(t.lexical, LexicalToken{name, kind, root})
	t.usageCounts = append(t.usageCounts, 1)
	return index, nil
}

// extractInRoot is extract_tokens.rs: fn extract_in_root. The walk is
// pre-order, first child first, as in Rust; this order fixes the token indices.
func (t *tokenExtractor) extractInRoot(pool *RulePool, root RuleID, varName string, isFirst bool, stack *[]RuleID) error {
	var aux uint32
	*stack = append((*stack)[:0], root)
	for len(*stack) > 0 {
		id := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		n := pool.node(id)
		switch n.Kind {
		case RString:
			i, err := t.extractToken(pool, id, n.Str, true, varName, &aux, isFirst)
			if err != nil {
				return err
			}
			pool.setNode(id, symRule(terminalSym(int(i))))
		case RPattern:
			i, err := t.extractToken(pool, id, "", false, varName, &aux, isFirst)
			if err != nil {
				return err
			}
			pool.setNode(id, symRule(terminalSym(int(i))))
		case RMetadata:
			p := pool.paramsOf(n.Params)
			if p.IsToken {
				cleaned := p
				cleaned.IsToken = false
				inner := pool.node(n.Inner)
				stringName, hasString := "", false
				if inner.Kind == RString {
					stringName, hasString = inner.Str, true
				}
				tokenRoot := id
				if cleaned == (MetadataParams{}) {
					tokenRoot = n.Inner // only a token: drop the wrapper
				}
				i, err := t.extractToken(pool, tokenRoot, stringName, hasString, varName, &aux, isFirst)
				if err != nil {
					return err
				}
				pool.setNode(id, symRule(terminalSym(int(i))))
			} else {
				*stack = append(*stack, n.Inner)
			}
		case RSeq, RChoice:
			base := len(*stack)
			*stack = append(*stack, n.Children...)
			reverseIDs((*stack)[base:])
		case RRepeat, RReserved:
			*stack = append(*stack, n.Inner)
		}
	}
	return nil
}

// find is extract_tokens.rs: fn find.
func (t *tokenExtractor) find(pool *RulePool, root RuleID) (uint32, bool) {
	for i := range t.lexical {
		if pool.subtreeEq(t.lexical[i].Root, root) {
			return uint32(i), true
		}
	}
	return 0, false
}

// extractTokens is extract_tokens.rs: fn extract_tokens.
func extractTokens(g *InputGrammar, in *internedMeta) (*extractedMeta, error) {
	pool := g.Pool
	ex := &tokenExtractor{}
	var stack []RuleID
	for i, v := range g.Variables {
		if err := ex.extractInRoot(pool, v.Root, v.Ident, i == 0, &stack); err != nil {
			return nil, err
		}
	}
	for i, root := range g.ExternalRoots {
		if err := ex.extractInRoot(pool, root, in.externalTokens[i].name, false, &stack); err != nil {
			return nil, err
		}
	}
	oldLen := len(g.Variables)
	replacements := map[uint32]uint32{}
	var retained []Variable
	var kinds []VariableType
	retained = append(retained, g.Variables[0])
	kinds = append(kinds, in.kinds[0])
	for i := 1; i < len(g.Variables); i++ {
		v := g.Variables[i]
		if sym, ok := pool.node(v.Root).symbol(); ok && sym.IsTerminal() && ex.usageCounts[sym.Index] == 1 {
			lex := &ex.lexical[sym.Index]
			if lex.Kind == VarAuxiliary || in.kinds[i] != VarHidden {
				lex.Kind = in.kinds[i]
				lex.Ident = v.Ident
				replacements[uint32(i)] = sym.Index
				continue
			}
		}
		retained = append(retained, v)
		kinds = append(kinds, in.kinds[i])
	}
	g.Variables = retained

	shift := make([]uint32, oldLen)
	var removed uint32
	for i := range shift {
		shift[i] = removed
		if _, ok := replacements[uint32(i)]; ok {
			removed++
		}
	}
	replaceSymbol := func(s Symbol) Symbol {
		if !s.isNonTerminal() {
			return s
		}
		if r, ok := replacements[s.Index]; ok {
			return terminalSym(int(r))
		}
		return nonTerminalSym(int(s.Index - shift[s.Index]))
	}
	for _, v := range g.Variables {
		renumberRoot(pool, v.Root, replaceSymbol, &stack)
	}
	for _, root := range g.ExternalRoots {
		renumberRoot(pool, root, replaceSymbol, &stack)
	}
	m := &extractedMeta{kinds: kinds}
	for _, c := range in.conflicts {
		var row []Symbol
		for _, s := range c {
			row = append(row, replaceSymbol(s))
		}
		sort.Slice(row, func(a, b int) bool { return symbolLess(row[a], row[b]) })
		var dd []Symbol
		for i, s := range row {
			if i == 0 || s != row[i-1] {
				dd = append(dd, s)
			}
		}
		m.conflicts = append(m.conflicts, dd)
	}
	for _, s := range in.supertypes {
		sym := replaceSymbol(s)
		if sym.IsTerminal() {
			return nil, fmt.Errorf("Terminal rule '%s' cannot be used as a supertype", ex.lexical[sym.Index].Ident)
		}
		m.supertypes = append(m.supertypes, sym)
	}
	for _, s := range in.inline {
		m.inline = append(m.inline, replaceSymbol(s))
	}
	for _, root := range g.ExtraRoots {
		if s, ok := pool.node(root).symbol(); ok {
			m.extraSymbols = append(m.extraSymbols, replaceSymbol(s))
		} else if i, ok := ex.find(pool, root); ok {
			m.extraSymbols = append(m.extraSymbols, terminalSym(int(i)))
		} else {
			m.separatorRoots = append(m.separatorRoots, root)
		}
	}
	for i, root := range g.ExternalRoots {
		s, ok := pool.node(root).symbol()
		if !ok {
			return nil, fmt.Errorf("Non-symbol rules cannot be used as external tokens")
		}
		if s.isNonTerminal() {
			return nil, fmt.Errorf("Rule '%s' cannot be used as both an external token and a non-terminal rule", g.Variables[s.Index].Ident)
		}
		nm := in.externalTokens[i]
		if s.isExternal() {
			if !nm.hasName {
				return nil, fmt.Errorf("Non-symbol rules cannot be used as external tokens")
			}
			m.externalTokens = append(m.externalTokens, ExternalToken{Ident: nm.name, Kind: nm.kind})
		} else {
			m.externalTokens = append(m.externalTokens, ExternalToken{Ident: ex.lexical[s.Index].Ident, Kind: nm.kind, CorrespondingInternal: s, HasCorresponding: true})
		}
	}
	if in.hasWord {
		token := replaceSymbol(in.word)
		if token.isNonTerminal() {
			return nil, fmt.Errorf("Non-terminal symbol '%s' cannot be used as the word token", g.Variables[token.Index].Ident)
		}
		m.word, m.hasWord = token, true
	}
	for _, set := range g.ReservedSets {
		var syms []Symbol
		for _, root := range set.Roots {
			if s, ok := pool.node(root).symbol(); ok {
				r := replaceSymbol(s)
				if r.isNonTerminal() {
					// Go shape: the generator inserts the non-terminal into a token set and
					// panics (flatten_grammar.rs and rules.rs: TokenSet::insert); here it is an error.
					return nil, fmt.Errorf("Reserved word '%s' must be a token", g.Variables[r.Index].Ident)
				}
				syms = append(syms, r)
			} else if i, ok := ex.find(pool, root); ok {
				syms = append(syms, terminalSym(int(i)))
			} else {
				inner := pool.node(root)
				if inner.Kind == RMetadata {
					inner = pool.node(inner.Inner)
				}
				tokenName := "unknown"
				if inner.Kind == RString || inner.Kind == RPattern {
					tokenName = inner.Str
				}
				return nil, fmt.Errorf("Reserved word '%s' must be a token", tokenName)
			}
		}
		m.reservedSets = append(m.reservedSets, reservedSet{set.Ident, syms})
	}
	m.lexicalVariables = ex.lexical
	return m, nil
}

// renumberRoot is extract_tokens.rs: fn renumber_root.
func renumberRoot(pool *RulePool, root RuleID, replace func(Symbol) Symbol, stack *[]RuleID) {
	*stack = append((*stack)[:0], root)
	for len(*stack) > 0 {
		id := (*stack)[len(*stack)-1]
		*stack = (*stack)[:len(*stack)-1]
		n := pool.node(id)
		switch n.Kind {
		case RSym:
			pool.setNode(id, symRule(replace(Symbol{n.SymKind, n.Index})))
		case RSeq, RChoice:
			*stack = append(*stack, n.Children...)
		case RRepeat, RMetadata, RReserved:
			*stack = append(*stack, n.Inner)
		}
	}
}

// width is expand_repeats.rs: struct Width.
type width struct{ nullable, eofNullable bool }

func (w *width) merge(o width) bool {
	before := *w
	w.nullable = w.nullable || o.nullable
	w.eofNullable = w.eofNullable || o.eofNullable
	return *w != before
}

// zeroWidth is expand_repeats.rs: struct ZeroWidth.
//
// Go shape: eval is recursive. The Rust version uses an explicit stack with
// the same per-node rules; Choice and Seq combine their children's results
// with commutative operations, so the visiting order does not reach the output.
type zeroWidth struct{ byVariable []width }

// newZeroWidth is expand_repeats.rs: fn ZeroWidth::new.
func newZeroWidth(pool *RulePool, vars []Variable) *zeroWidth {
	z := &zeroWidth{byVariable: make([]width, len(vars))}
	for {
		changed := false
		for i, v := range vars {
			w := z.eval(pool, v.Root)
			if z.byVariable[i].merge(w) {
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	return z
}

// eval is expand_repeats.rs: fn ZeroWidth::eval.
func (z *zeroWidth) eval(pool *RulePool, id RuleID) width {
	n := pool.node(id)
	switch n.Kind {
	case RSeq:
		nullable, anyEOF, allZero := true, false, true
		for _, c := range n.Children {
			w := z.eval(pool, c)
			nullable = nullable && w.nullable
			anyEOF = anyEOF || w.eofNullable
			allZero = allZero && (w.nullable || w.eofNullable)
		}
		return width{nullable, allZero && anyEOF}
	case RChoice:
		var r width
		for _, c := range n.Children {
			w := z.eval(pool, c)
			r.nullable = r.nullable || w.nullable
			r.eofNullable = r.eofNullable || w.eofNullable
		}
		return r
	case RRepeat, RMetadata, RReserved:
		return z.eval(pool, n.Inner)
	case RBlank:
		return width{nullable: true}
	case RString:
		return width{nullable: n.Str == ""}
	case REof:
		return width{eofNullable: true}
	case RSym:
		switch n.SymKind {
		case End:
			return width{eofNullable: true}
		case NonTerminal:
			if int(n.Index) < len(z.byVariable) {
				return z.byVariable[n.Index]
			}
			return width{}
		case External:
			return width{nullable: true}
		}
		return width{}
	}
	panic("lexbuild: zeroWidth.eval: unexpected rule")
}

type repeatMemo struct {
	node RuleID
	sym  Symbol
}

type expander struct {
	preceding int
	aux       []Variable
	memo      []repeatMemo
	zw        *zeroWidth
}

type task struct {
	expand  bool
	id      RuleID
	content RuleID
}

// expandRoot is expand_repeats.rs: fn Expander::expand_root.
func (e *expander) expandRoot(pool *RulePool, root RuleID, varName string, auxRepeatCounter *uint32) error {
	stack := []task{{id: root}}
	for len(stack) > 0 {
		t := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if !t.expand {
			n := pool.node(t.id)
			switch n.Kind {
			case RRepeat:
				stack = append(stack, task{expand: true, id: t.id, content: n.Inner})
				stack = append(stack, task{id: n.Inner})
			case RSeq, RChoice:
				base := len(stack)
				for _, c := range n.Children {
					stack = append(stack, task{id: c})
				}
				for i, j := base, len(stack)-1; i < j; i, j = i+1, j-1 {
					stack[i], stack[j] = stack[j], stack[i]
				}
			case RMetadata:
				stack = append(stack, task{id: n.Inner})
			}
			continue
		}
		w := e.zw.eval(pool, t.content)
		if w.eofNullable {
			return fmt.Errorf("Rule `%s` contains a repetition that can match the empty string at end of input", varName)
		}
		found := false
		for _, m := range e.memo {
			if pool.subtreeEq(m.node, t.content) {
				pool.setNode(t.id, symRule(m.sym))
				found = true
				break
			}
		}
		if found {
			continue
		}
		*auxRepeatCounter++
		name := fmt.Sprintf("%s_repeat%d", varName, *auxRepeatCounter)
		sym := nonTerminalSym(e.preceding + len(e.aux))
		e.zw.byVariable = append(e.zw.byVariable, w)
		e.memo = append(e.memo, repeatMemo{t.content, sym})
		root := wrapInBinaryTree(pool, sym, t.content)
		e.aux = append(e.aux, Variable{Ident: name, Root: root})
		pool.setNode(t.id, symRule(sym))
	}
	return nil
}

// wrapInBinaryTree is expand_repeats.rs: fn wrap_in_binary_tree.
func wrapInBinaryTree(pool *RulePool, sym Symbol, inner RuleID) RuleID {
	s1 := pool.pushNode(symRule(sym))
	s2 := pool.pushNode(symRule(sym))
	seq := pool.pushNode(Rule{Kind: RSeq, Children: []RuleID{s1, s2}})
	elements := []RuleID{seq}
	stack := []RuleID{inner}
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if n := pool.node(id); n.Kind == RChoice {
			base := len(stack)
			stack = append(stack, n.Children...)
			reverseIDs(stack[base:])
		} else {
			dup := false
			for _, e := range elements {
				if pool.subtreeEq(e, id) {
					dup = true
					break
				}
			}
			if !dup {
				elements = append(elements, id)
			}
		}
	}
	if len(elements) == 1 {
		return elements[0]
	}
	return pool.pushNode(Rule{Kind: RChoice, Children: elements})
}

// expandRepeats is expand_repeats.rs: fn expand_repeats.
func expandRepeats(g *InputGrammar, m *extractedMeta) error {
	pool := g.Pool
	e := &expander{preceding: len(g.Variables), zw: newZeroWidth(pool, g.Variables)}
	n := len(g.Variables)
	for i := 0; i < n; i++ {
		v := g.Variables[i]
		var auxCount uint32
		if rn := pool.node(v.Root); m.kinds[i] == VarHidden && rn.Kind == RRepeat {
			if e.zw.eval(pool, rn.Inner).eofNullable {
				return fmt.Errorf("Rule `%s` contains a repetition that can match the empty string at end of input", v.Ident)
			}
			if err := e.expandRoot(pool, rn.Inner, v.Ident, &auxCount); err != nil {
				return err
			}
			g.Variables[i].Root = wrapInBinaryTree(pool, nonTerminalSym(i), rn.Inner)
			m.kinds[i] = VarAuxiliary
			var in []Symbol
			for _, s := range m.inline {
				if s != nonTerminalSym(i) {
					in = append(in, s)
				}
			}
			m.inline = in
			continue
		}
		if err := e.expandRoot(pool, v.Root, v.Ident, &auxCount); err != nil {
			return err
		}
	}
	for _, v := range e.aux {
		g.Variables = append(g.Variables, v)
		m.kinds = append(m.kinds, VarAuxiliary)
	}
	return nil
}

// ProductionStep is grammars.rs: struct ProductionStep, unpacked. Reserved is
// the reserved word set id of the step (0 without reserved words).
type ProductionStep struct {
	Symbol        Symbol
	Precedence    Precedence
	Associativity Associativity
	Alias         Alias
	Field         string
	HasField      bool
	Reserved      uint16
}

// Production is grammars.rs: struct Production, with its steps held inline.
type Production struct {
	Steps             []ProductionStep
	DynamicPrecedence int32
	RequiresEOF       bool
}

// SyntaxVariable is grammars.rs: struct SyntaxVariable.
type SyntaxVariable struct {
	Ident string
	Kind  VariableType
}

// SyntaxGrammar is grammars.rs: struct SyntaxGrammar.
type SyntaxGrammar struct {
	Variables           []SyntaxVariable
	ExtraSymbols        []Symbol
	ExpectedConflicts   [][]Symbol
	ExternalTokens      []ExternalToken
	SupertypeSymbols    []Symbol
	VariablesToInline   []Symbol
	WordToken           Symbol
	HasWord             bool
	PrecedenceOrderings [][]PrecedenceEntry
	// Productions holds every production; VarProds holds, for each variable,
	// the half-open range of its productions.
	Productions []Production
	VarProds    [][2]int
	// ReservedWordSets holds the tokens of each reserved word set; set 0 is
	// the global set (one empty set when the grammar declares none).
	ReservedWordSets []TokenSet
}

// choiceCursor is flatten_grammar.rs: struct ChoiceCursor.
type choiceCursor struct {
	decisions []uint32
	arities   []uint32
	depth     int
}

func (c *choiceCursor) reset() { c.decisions, c.arities, c.depth = c.decisions[:0], c.arities[:0], 0 }
func (c *choiceCursor) beginPath() {
	c.arities, c.depth = c.arities[:0], 0
}
func (c *choiceCursor) selectChild(n uint32) uint32 {
	var sel uint32
	if c.depth < len(c.decisions) {
		sel = c.decisions[c.depth]
	}
	c.arities = append(c.arities, n)
	c.depth++
	return sel
}
func (c *choiceCursor) advance() bool {
	for depth := len(c.arities) - 1; depth >= 0; depth-- {
		var sel uint32
		if depth < len(c.decisions) {
			sel = c.decisions[depth]
		}
		if sel+1 < c.arities[depth] {
			for len(c.decisions) < depth+1 {
				c.decisions = append(c.decisions, 0)
			}
			c.decisions = c.decisions[:depth+1]
			c.decisions[depth] = sel + 1
			return true
		}
	}
	return false
}

// flattenCtx is flatten_grammar.rs: struct FlattenCtx.
type flattenCtx struct {
	prec     Precedence
	assoc    Associativity
	alias    Alias
	field    string
	hasField bool
	reserved uint16
}

// flattenState is flatten_grammar.rs: struct FlattenState.
type flattenState struct {
	reservedIDs map[string]uint16
	steps       []ProductionStep
	failed      error
	choices     choiceCursor
	dynPrec     int32
	dead        bool
}

func (s *flattenState) resetVariable() { s.choices.reset(); s.resetPath() }
func (s *flattenState) resetPath() {
	s.steps, s.dynPrec, s.dead = s.steps[:0], 0, false
	s.choices.beginPath()
}

func (s *flattenState) pushStep(sym Symbol, c flattenCtx) {
	s.steps = append(s.steps, ProductionStep{Symbol: sym, Precedence: c.prec, Associativity: c.assoc, Alias: c.alias, Field: c.field, HasField: c.hasField, Reserved: c.reserved})
}

func abs32(x int32) int32 {
	if x < 0 {
		return -x
	}
	return x
}

// apply is flatten_grammar.rs: fn apply.
func (s *flattenState) apply(pool *RulePool, id RuleID, ctx flattenCtx, atEnd bool) bool {
	n := pool.node(id)
	switch n.Kind {
	case RSym:
		s.pushStep(Symbol{n.SymKind, n.Index}, ctx)
		return true
	case RSeq:
		did := false
		for i, c := range n.Children {
			if s.apply(pool, c, ctx, atEnd && i+1 == len(n.Children)) {
				did = true
			}
			if s.dead {
				break
			}
		}
		return did
	case RChoice:
		if len(n.Children) == 0 {
			s.dead = true
			return false
		}
		sel := s.choices.selectChild(uint32(len(n.Children)))
		return s.apply(pool, n.Children[sel], ctx, atEnd)
	case REof:
		s.pushStep(endSym(), ctx)
		return true
	case RMetadata:
		p := pool.paramsOf(n.Params)
		inner := ctx
		if p.Precedence.Kind != PrecNone {
			inner.prec = p.Precedence
		}
		if p.Associativity != AssocNone {
			inner.assoc = p.Associativity
		}
		if p.Alias.Set {
			inner.alias = p.Alias
		}
		if p.FieldSet {
			inner.field, inner.hasField = p.Field, true
		}
		if abs32(p.DynamicPrecedence) > abs32(s.dynPrec) {
			s.dynPrec = p.DynamicPrecedence
		}
		did := s.apply(pool, n.Inner, inner, atEnd)
		// A step's precedence and associativity govern the position just after
		// it; if more steps follow the region, that gap reverts to the outer
		// context.
		if did && !atEnd {
			if p.Precedence.Kind != PrecNone {
				s.steps[len(s.steps)-1].Precedence = ctx.prec
			}
			if p.Associativity != AssocNone {
				s.steps[len(s.steps)-1].Associativity = ctx.assoc
			}
		}
		return did
	case RReserved:
		id, ok := s.reservedIDs[n.Str]
		if !ok {
			s.failed = fmt.Errorf("No such reserved word set: %s", n.Str)
			return false
		}
		inner := ctx
		inner.reserved = id
		return s.apply(pool, n.Inner, inner, atEnd)
	}
	return false
}

func stepsEqual(a, b []ProductionStep) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// emit is flatten_grammar.rs: fn emit.
func (s *flattenState) emit(out *SyntaxGrammar, prodStart int) bool {
	eofIndex := -1
	for i, st := range s.steps {
		if st.Symbol == endSym() {
			eofIndex = i
			break
		}
	}
	if eofIndex < 0 {
		return s.emitReady(out, prodStart, false)
	}
	if eofIndex != len(s.steps)-1 {
		return false
	}
	s.steps = s.steps[:len(s.steps)-1]
	return s.emitReady(out, prodStart, true)
}

// emitReady is flatten_grammar.rs: fn emit_ready.
func (s *flattenState) emitReady(out *SyntaxGrammar, prodStart int, requiresEOF bool) bool {
	for _, p := range out.Productions[prodStart:] {
		if p.DynamicPrecedence == s.dynPrec && p.RequiresEOF == requiresEOF && stepsEqual(p.Steps, s.steps) {
			return true
		}
	}
	out.Productions = append(out.Productions, Production{append([]ProductionStep(nil), s.steps...), s.dynPrec, requiresEOF})
	return true
}

// flattenGrammar is flatten_grammar.rs: fn flatten_grammar (with fn check).
func flattenGrammar(g *InputGrammar, m *extractedMeta) (*SyntaxGrammar, error) {
	out := &SyntaxGrammar{}
	st := &flattenState{reservedIDs: map[string]uint16{}}
	if len(m.reservedSets) > 65535 {
		return nil, fmt.Errorf("Reserved word set count %d exceeds the maximum of 65535", len(m.reservedSets))
	}
	for i, rs := range m.reservedSets {
		st.reservedIDs[rs.name] = uint16(i)
	}
	for _, v := range g.Variables {
		prodStart := len(out.Productions)
		dropped := false
		st.resetVariable()
		for {
			st.apply(g.Pool, v.Root, flattenCtx{}, true)
			if st.failed != nil {
				return nil, st.failed
			}
			if !st.dead {
				if !st.emit(out, prodStart) {
					dropped = true
				}
			}
			if !st.choices.advance() {
				break
			}
			st.resetPath()
		}
		if dropped && prodStart == len(out.Productions) {
			return nil, fmt.Errorf("Rule `%s` has no reachable productions.", v.Ident)
		}
		out.VarProds = append(out.VarProds, [2]int{prodStart, len(out.Productions)})
	}
	for i, vp := range out.VarProds {
		sym := nonTerminalSym(i)
		used := false
		for _, p := range out.Productions {
			for _, s := range p.Steps {
				if s.Symbol == sym {
					used = true
				}
			}
		}
		inlined := false
		for _, s := range m.inline {
			if s == sym {
				inlined = true
			}
		}
		for _, p := range out.Productions[vp[0]:vp[1]] {
			if used && len(p.Steps) == 0 && !p.RequiresEOF {
				return nil, fmt.Errorf("The rule `%s` matches the empty string.\n\nTree-sitter does not support syntactic rules that match the empty string\nunless they are used only as the grammar's start rule.\n", g.Variables[i].Ident)
			}
			if inlined {
				for _, s := range p.Steps {
					if s.Symbol == sym {
						return nil, fmt.Errorf("Rule `%s` cannot be inlined because it contains a reference to itself", g.Variables[i].Ident)
					}
				}
			}
		}
	}
	// flatten_grammar.rs: fn assemble_syntax_grammar (reduced).
	for i, v := range g.Variables {
		out.Variables = append(out.Variables, SyntaxVariable{v.Ident, m.kinds[i]})
	}
	for _, rs := range m.reservedSets {
		var ts TokenSet
		for _, sy := range rs.symbols {
			ts.Insert(sy)
		}
		out.ReservedWordSets = append(out.ReservedWordSets, ts)
	}
	if len(out.ReservedWordSets) == 0 {
		out.ReservedWordSets = append(out.ReservedWordSets, TokenSet{})
	}
	out.ExtraSymbols = m.extraSymbols
	out.ExpectedConflicts = m.conflicts
	out.ExternalTokens = m.externalTokens
	out.SupertypeSymbols = m.supertypes
	out.VariablesToInline = m.inline
	out.PrecedenceOrderings = g.PrecedenceOrderings
	out.WordToken, out.HasWord = m.word, m.hasWord
	return out, nil
}
