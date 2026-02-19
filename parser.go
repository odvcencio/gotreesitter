package gotreesitter

// Parser is an LR(1) parser that reads parse tables from a Language and
// produces a syntax tree. This is the core of the tree-sitter runtime.
//
// The current implementation supports a single parse stack (SLR/LALR/LR(1)).
// GLR support (multiple stack versions) and incremental reparsing will be
// added in later tasks.
type Parser struct {
	language *Language
}

// NewParser creates a new Parser for the given language.
func NewParser(lang *Language) *Parser {
	return &Parser{language: lang}
}

// TokenSource provides tokens to the parser. This interface abstracts over
// different lexer implementations: the built-in DFA lexer (for hand-built
// grammars) or custom bridges like GoTokenSource (for real grammars where
// we can't extract the C lexer DFA).
type TokenSource interface {
	// Next returns the next token. It should skip whitespace and comments
	// as appropriate for the language. Returns a zero-Symbol token at EOF.
	Next() Token
}

// stackEntry is a single entry on the parser's LR stack, pairing a parser
// state with the syntax tree node that was shifted or reduced into that state.
type stackEntry struct {
	state StateID
	node  *Node
}

// errorSymbol is the well-known symbol ID used for error nodes.
const errorSymbol = Symbol(65535)

// Parse tokenizes and parses source using the built-in DFA lexer, returning
// a syntax tree. This works for hand-built grammars that provide LexStates.
// For real grammars that need a custom lexer, use ParseWithTokenSource.
// If the input is empty, it returns a tree with a nil root.
func (p *Parser) Parse(source []byte) *Tree {
	lexer := NewLexer(p.language.LexStates, source)
	ts := &dfaTokenSource{lexer: lexer, language: p.language}
	return p.parseInternal(source, ts)
}

// ParseWithTokenSource parses source using a custom token source.
// This is used for real grammars where the lexer DFA isn't available
// as data tables (e.g., Go grammar using go/scanner as a bridge).
func (p *Parser) ParseWithTokenSource(source []byte, ts TokenSource) *Tree {
	return p.parseInternal(source, ts)
}

// dfaTokenSource wraps the built-in DFA Lexer as a TokenSource.
// It tracks the current parser state to select the correct lex mode.
type dfaTokenSource struct {
	lexer    *Lexer
	language *Language
	state    StateID
}

func (d *dfaTokenSource) Next() Token {
	lexState := uint16(0)
	if int(d.state) < len(d.language.LexModes) {
		lexState = d.language.LexModes[d.state].LexState
	}
	return d.lexer.Next(lexState)
}

// parseInternal is the core LR parsing loop shared by Parse and
// ParseWithTokenSource.
func (p *Parser) parseInternal(source []byte, ts TokenSource) *Tree {
	stack := []stackEntry{{state: p.language.InitialState, node: nil}}

	// needToken tracks whether we need to lex the next token.
	// After a reduce, we reuse the current lookahead.
	needToken := true
	var tok Token

	for {
		currentState := stack[len(stack)-1].state

		// Update the DFA token source's state if applicable.
		if dts, ok := ts.(*dfaTokenSource); ok {
			dts.state = currentState
		}

		// Lex the next token if needed.
		if needToken {
			tok = ts.Next()
			needToken = true // default: consume after processing
		}

		action := p.lookupAction(currentState, tok.Symbol)

		// Handle extra tokens (like comments). If the action says this is
		// an extra/shift-extra, we consume it but don't change state.
		if action != nil && len(action.Actions) > 0 && action.Actions[0].Type == ParseActionShift && action.Actions[0].Extra {
			// Create a leaf for the extra token and attach it but keep parsing.
			named := p.isNamedSymbol(tok.Symbol)
			leaf := NewLeafNode(
				tok.Symbol,
				named,
				tok.StartByte, tok.EndByte,
				tok.StartPoint, tok.EndPoint,
			)
			stack = append(stack, stackEntry{state: currentState, node: leaf})
			needToken = true
			continue
		}

		if action == nil || len(action.Actions) == 0 {
			// Error recovery: wrap the current token in an error node and skip it.
			if tok.Symbol == 0 {
				// EOF with no valid action — we're done. Return whatever we have.
				return p.buildResult(stack, source)
			}
			errNode := NewLeafNode(
				errorSymbol,
				false,
				tok.StartByte, tok.EndByte,
				tok.StartPoint, tok.EndPoint,
			)
			errNode.hasError = true
			// Push the error node in the current state (don't change state).
			stack = append(stack, stackEntry{state: currentState, node: errNode})
			needToken = true
			continue
		}

		// Take the first action (GLR would fork here).
		act := action.Actions[0]

		switch act.Type {
		case ParseActionShift:
			// Create a leaf node from the token.
			named := p.isNamedSymbol(tok.Symbol)
			leaf := NewLeafNode(
				tok.Symbol,
				named,
				tok.StartByte, tok.EndByte,
				tok.StartPoint, tok.EndPoint,
			)
			stack = append(stack, stackEntry{state: act.State, node: leaf})
			needToken = true

		case ParseActionReduce:
			childCount := int(act.ChildCount)
			children := make([]*Node, childCount)

			// Pop childCount entries from the stack.
			for i := childCount - 1; i >= 0; i-- {
				children[i] = stack[len(stack)-1].node
				stack = stack[:len(stack)-1]
			}

			// Create a parent node for the reduced symbol.
			named := p.isNamedSymbol(act.Symbol)
			parent := NewParentNode(act.Symbol, named, children, nil, act.ProductionID)

			// Look up the GOTO for (new top state, reduced symbol).
			// For nonterminal symbols (>= TokenCount), the parse table value
			// is the target state directly (not an action index).
			// For terminal symbols, it's an index into ParseActions.
			topState := stack[len(stack)-1].state
			gotoState := p.lookupGoto(topState, act.Symbol)
			if gotoState != 0 {
				stack = append(stack, stackEntry{state: gotoState, node: parent})
			} else {
				// No GOTO found — push with current top state as fallback.
				stack = append(stack, stackEntry{state: topState, node: parent})
			}

			// After a reduce, reuse the same lookahead token.
			needToken = false

		case ParseActionAccept:
			// The top of stack should contain the root node.
			return p.buildResult(stack, source)

		default:
			// Unknown action type — skip token as error recovery.
			needToken = true
		}
	}
}

// lookupAction looks up the parse action for the given state and symbol.
// For states < LargeStateCount, it uses the dense ParseTable.
// For states >= LargeStateCount, it uses the compressed SmallParseTable.
// Both return an index into ParseActions.
func (p *Parser) lookupAction(state StateID, sym Symbol) *ParseActionEntry {
	idx := p.lookupActionIndex(state, sym)
	if idx == 0 {
		return nil
	}
	if int(idx) < len(p.language.ParseActions) {
		return &p.language.ParseActions[idx]
	}
	return nil
}

// lookupActionIndex returns the parse action index for (state, symbol).
// Returns 0 (the error/no-action entry) if not found.
func (p *Parser) lookupActionIndex(state StateID, sym Symbol) uint16 {
	// Use dense table for states in the dense table range.
	// When LargeStateCount is 0 and ParseTable is non-empty, that means
	// the grammar uses a dense table for all states (hand-built grammars).
	useDense := false
	if p.language.LargeStateCount > 0 {
		useDense = uint32(state) < p.language.LargeStateCount
	} else if len(p.language.ParseTable) > 0 {
		useDense = int(state) < len(p.language.ParseTable)
	}

	if useDense {
		if int(state) < len(p.language.ParseTable) {
			row := p.language.ParseTable[state]
			if int(sym) < len(row) {
				return row[sym]
			}
		}
		return 0
	}

	// Small (compressed sparse) table lookup.
	smallIdx := int(state) - int(p.language.LargeStateCount)
	if smallIdx < 0 || smallIdx >= len(p.language.SmallParseTableMap) {
		return 0
	}
	offset := p.language.SmallParseTableMap[smallIdx]
	table := p.language.SmallParseTable
	if int(offset) >= len(table) {
		return 0
	}

	groupCount := table[offset]
	pos := int(offset) + 1
	for i := uint16(0); i < groupCount; i++ {
		if pos+1 >= len(table) {
			break
		}
		sectionValue := table[pos]
		symbolCount := table[pos+1]
		pos += 2
		for j := uint16(0); j < symbolCount; j++ {
			if pos >= len(table) {
				break
			}
			if table[pos] == uint16(sym) {
				return sectionValue
			}
			pos++
		}
	}
	return 0
}

// lookupGoto returns the GOTO target state for a nonterminal symbol.
//
// In real tree-sitter grammars (InitialState > 0), nonterminal symbols
// (>= TokenCount) store the target state directly in the parse table,
// not an action index. Terminal symbols use action indices.
//
// In hand-built grammars (InitialState == 0), ALL parse table values are
// action indices regardless of symbol type.
func (p *Parser) lookupGoto(state StateID, sym Symbol) StateID {
	raw := p.lookupActionIndex(state, sym)
	if raw == 0 {
		return 0
	}

	// Real tree-sitter grammars: nonterminal GOTO values are state IDs.
	if p.language.InitialState > 0 && p.language.TokenCount > 0 && uint32(sym) >= p.language.TokenCount {
		return StateID(raw)
	}

	// Hand-built grammar or terminal symbol: look up in parse actions.
	if int(raw) < len(p.language.ParseActions) {
		entry := &p.language.ParseActions[raw]
		if len(entry.Actions) > 0 && entry.Actions[0].Type == ParseActionShift {
			return entry.Actions[0].State
		}
	}
	return 0
}

// isNamedSymbol checks whether a symbol is a named symbol using the
// language's symbol metadata.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	if int(sym) < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[sym].Named
	}
	return false
}

// buildResult constructs the final Tree from the parser stack.
// It finds the topmost non-nil node on the stack and uses it as root.
// If multiple nodes remain (due to error recovery or incomplete parse),
// they are gathered under a synthetic root.
func (p *Parser) buildResult(stack []stackEntry, source []byte) *Tree {
	// Collect all non-nil nodes from the stack.
	var nodes []*Node
	for _, entry := range stack {
		if entry.node != nil {
			nodes = append(nodes, entry.node)
		}
	}

	if len(nodes) == 0 {
		return NewTree(nil, source, p.language)
	}

	if len(nodes) == 1 {
		return NewTree(nodes[0], source, p.language)
	}

	// Multiple nodes on stack — wrap them in a parent.
	// Use the symbol of the last node (most likely the intended root symbol).
	root := NewParentNode(nodes[len(nodes)-1].symbol, true, nodes, nil, 0)
	root.hasError = true
	return NewTree(root, source, p.language)
}
