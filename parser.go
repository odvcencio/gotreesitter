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

// stackEntry is a single entry on the parser's LR stack, pairing a parser
// state with the syntax tree node that was shifted or reduced into that state.
type stackEntry struct {
	state StateID
	node  *Node
}

// errorSymbol is the well-known symbol ID used for error nodes.
const errorSymbol = Symbol(65535)

// Parse tokenizes and parses source, returning a syntax tree.
// If the input is empty, it returns a tree with a nil root.
func (p *Parser) Parse(source []byte) *Tree {
	lexer := NewLexer(p.language.LexStates, source)

	stack := []stackEntry{{state: 0, node: nil}}

	// needToken tracks whether we need to lex the next token.
	// After a reduce, we reuse the current lookahead.
	needToken := true
	var tok Token

	for {
		currentState := stack[len(stack)-1].state

		// Lex the next token if needed.
		if needToken {
			lexState := uint16(0)
			if int(currentState) < len(p.language.LexModes) {
				lexState = p.language.LexModes[currentState].LexState
			}
			tok = lexer.Next(lexState)
			needToken = true // default: consume after processing
		}

		action := p.lookupAction(currentState, tok.Symbol)
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

			// Look up the GOTO action for (new top state, reduced symbol).
			topState := stack[len(stack)-1].state
			gotoAction := p.lookupAction(topState, act.Symbol)
			if gotoAction != nil && len(gotoAction.Actions) > 0 && gotoAction.Actions[0].Type == ParseActionShift {
				gotoState := gotoAction.Actions[0].State
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

// lookupAction looks up the parse action for the given state and symbol
// using the dense parse table. Compressed (small) parse table support
// will be added in a later task.
func (p *Parser) lookupAction(state StateID, sym Symbol) *ParseActionEntry {
	if int(state) < len(p.language.ParseTable) {
		row := p.language.ParseTable[state]
		if int(sym) < len(row) {
			idx := row[sym]
			if int(idx) < len(p.language.ParseActions) {
				return &p.language.ParseActions[idx]
			}
		}
	}
	return nil
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
