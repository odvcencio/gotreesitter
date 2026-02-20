package gotreesitter

import (
	"bytes"
	"sync"
)

// Parser reads parse tables from a Language and produces a syntax tree.
// It supports GLR parsing: when a (state, symbol) pair maps to multiple
// actions, the parser forks the stack and explores all alternatives in
// parallel, merging stacks that converge on the same state and picking
// the highest dynamic-precedence winner for ambiguities.
type Parser struct {
	language     *Language
	reuseCursor  reuseCursor
	reuseScratch reuseScratch
	reuseMu      sync.Mutex
}

type parserScratch struct {
	merge   glrMergeScratch
	entries glrEntryScratch
}

var parserScratchPool = sync.Pool{
	New: func() any {
		return &parserScratch{}
	},
}

func acquireParserScratch() *parserScratch {
	return parserScratchPool.Get().(*parserScratch)
}

func releaseParserScratch(s *parserScratch) {
	if s == nil {
		return
	}
	s.merge.result = s.merge.result[:0]
	s.entries.reset()
	parserScratchPool.Put(s)
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

// ByteSkippableTokenSource can jump to a byte offset and return the first
// token at or after that position.
type ByteSkippableTokenSource interface {
	TokenSource
	SkipToByte(offset uint32) Token
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
	if !p.languageCompatible() {
		return NewTree(nil, source, p.language)
	}
	if !p.canUseDFALexer() {
		return NewTree(nil, source, p.language)
	}
	lexer := NewLexer(p.language.LexStates, source)
	ts := &dfaTokenSource{
		lexer:             lexer,
		language:          p.language,
		lookupActionIndex: p.lookupActionIndex,
	}
	if p.language.ExternalScanner != nil {
		ts.externalPayload = p.language.ExternalScanner.Create()
	}
	return p.parseInternal(source, ts, nil, nil, arenaClassFull)
}

// ParseWithTokenSource parses source using a custom token source.
// This is used for real grammars where the lexer DFA isn't available
// as data tables (e.g., Go grammar using go/scanner as a bridge).
func (p *Parser) ParseWithTokenSource(source []byte, ts TokenSource) *Tree {
	if !p.languageCompatible() {
		return NewTree(nil, source, p.language)
	}
	return p.parseInternal(source, ts, nil, nil, arenaClassFull)
}

// ParseIncremental re-parses source after edits were applied to oldTree.
// It reuses unchanged subtrees from the old tree for better performance.
// Call oldTree.Edit() for each edit before calling this method.
func (p *Parser) ParseIncremental(source []byte, oldTree *Tree) *Tree {
	if !p.languageCompatible() {
		return NewTree(nil, source, p.language)
	}
	if !p.canUseDFALexer() {
		return NewTree(nil, source, p.language)
	}
	lexer := NewLexer(p.language.LexStates, source)
	ts := &dfaTokenSource{
		lexer:             lexer,
		language:          p.language,
		lookupActionIndex: p.lookupActionIndex,
	}
	if p.language.ExternalScanner != nil {
		ts.externalPayload = p.language.ExternalScanner.Create()
	}
	return p.parseIncrementalInternal(source, oldTree, ts)
}

// ParseIncrementalWithTokenSource is like ParseIncremental but uses a custom
// token source.
func (p *Parser) ParseIncrementalWithTokenSource(source []byte, oldTree *Tree, ts TokenSource) *Tree {
	if !p.languageCompatible() {
		return NewTree(nil, source, p.language)
	}
	return p.parseIncrementalInternal(source, oldTree, ts)
}

func (p *Parser) canUseDFALexer() bool {
	return p.language != nil && len(p.language.LexStates) > 0
}

func (p *Parser) languageCompatible() bool {
	return p.language != nil && p.language.CompatibleWithRuntime()
}

func (p *Parser) parseIncrementalInternal(source []byte, oldTree *Tree, ts TokenSource) *Tree {
	// Fast path: unchanged source and no recorded edits.
	if oldTree != nil &&
		oldTree.language == p.language &&
		len(oldTree.edits) == 0 &&
		bytes.Equal(oldTree.source, source) {
		return oldTree
	}

	p.reuseMu.Lock()
	defer p.reuseMu.Unlock()

	reuse := p.reuseCursor.reset(oldTree, source, &p.reuseScratch)
	tree := p.parseInternal(source, ts, reuse, oldTree, arenaClassIncremental)
	if reuse != nil {
		reuse.commitScratch(&p.reuseScratch)
	}
	return tree
}

// dfaTokenSource wraps the built-in DFA Lexer as a TokenSource.
// It tracks the current parser state to select the correct lex mode.
type dfaTokenSource struct {
	lexer    *Lexer
	language *Language
	state    StateID

	lookupActionIndex func(state StateID, sym Symbol) uint16
	externalPayload   any
	externalValid     []bool
}

func (d *dfaTokenSource) Close() {
	if d.language == nil || d.language.ExternalScanner == nil || d.externalPayload == nil {
		return
	}
	d.language.ExternalScanner.Destroy(d.externalPayload)
	d.externalPayload = nil
}

func (d *dfaTokenSource) Next() Token {
	if tok, ok := d.nextExternalToken(); ok {
		return tok
	}

	lexState := uint16(0)
	if int(d.state) < len(d.language.LexModes) {
		lexState = d.language.LexModes[d.state].LexState
	}
	return d.lexer.Next(lexState)
}

func (d *dfaTokenSource) SkipToByte(offset uint32) Token {
	target := int(offset)
	if target < d.lexer.pos {
		// Rewind isn't supported for DFA token sources during parse.
		return d.Next()
	}
	for d.lexer.pos < target {
		d.lexer.skipOneRune()
	}
	return d.Next()
}

func (d *dfaTokenSource) nextExternalToken() (Token, bool) {
	if d.language == nil || d.language.ExternalScanner == nil || d.lookupActionIndex == nil {
		return Token{}, false
	}
	if len(d.language.ExternalSymbols) == 0 {
		return Token{}, false
	}

	if cap(d.externalValid) < len(d.language.ExternalSymbols) {
		d.externalValid = make([]bool, len(d.language.ExternalSymbols))
	}
	valid := d.externalValid[:len(d.language.ExternalSymbols)]
	for i := range valid {
		valid[i] = false
	}

	anyValid := false
	for i, sym := range d.language.ExternalSymbols {
		if d.lookupActionIndex(d.state, sym) != 0 {
			valid[i] = true
			anyValid = true
		}
	}
	if !anyValid {
		return Token{}, false
	}

	el := newExternalLexer(d.lexer.source, d.lexer.pos, d.lexer.row, d.lexer.col)
	if !RunExternalScanner(d.language, d.externalPayload, el, valid) {
		return Token{}, false
	}
	tok, ok := el.token()
	if !ok {
		return Token{}, false
	}

	d.lexer.pos = int(tok.EndByte)
	d.lexer.row = tok.EndPoint.Row
	d.lexer.col = tok.EndPoint.Column
	return tok, true
}

// parseIterations returns the iteration limit scaled to input size.
// A correctly-parsed file needs roughly (tokens * grammar_depth) iterations.
// For typical source (~5 bytes/token, ~10 reduce depth), that's sourceLen*2.
// We use sourceLen*20 as a generous upper bound that still prevents runaway
// parsing from OOMing the machine.
func parseIterations(sourceLen int) int {
	return max(10_000, sourceLen*20)
}

// parseStackDepth returns the stack depth limit scaled to input size.
func parseStackDepth(sourceLen int) int {
	return max(1_000, sourceLen*2)
}

// parseNodeLimit returns the maximum number of Node allocations allowed.
// This is the hard ceiling that prevents OOM regardless of iteration count.
func parseNodeLimit(sourceLen int) int {
	return max(50_000, sourceLen*10)
}

// parseInternal is the core GLR parsing loop shared by Parse and
// ParseWithTokenSource.
//
// It maintains a set of parse stacks. For unambiguous grammars (single
// action per table entry), there is exactly one stack and the algorithm
// reduces to standard LR parsing. When multiple actions exist for a
// (state, symbol) pair, the parser forks: one stack per alternative.
// Stacks that error out are dropped. Stacks that converge to the same
// top state are merged, keeping the highest dynamic-precedence version.
func (p *Parser) parseInternal(source []byte, ts TokenSource, reuse *reuseCursor, oldTree *Tree, arenaClass arenaClass) *Tree {
	if closer, ok := ts.(interface{ Close() }); ok {
		defer closer.Close()
	}
	scratch := acquireParserScratch()
	defer releaseParserScratch(scratch)

	arena := acquireNodeArena(arenaClass)
	reusedAny := false

	finalize := func(stacks []glrStack) *Tree {
		return p.buildResultFromGLR(stacks, source, arena, oldTree, reusedAny)
	}

	stacks := []glrStack{newGLRStackWithScratch(p.language.InitialState, &scratch.entries)}

	maxIter := parseIterations(len(source))
	maxDepth := parseStackDepth(len(source))
	maxNodes := parseNodeLimit(len(source))
	nodeCount := 0

	needToken := true
	var tok Token

	// Per-primary-stack infinite-reduce detection.
	var lastReduceState StateID
	var consecutiveReduces int

	for iter := 0; iter < maxIter; iter++ {
		// Prune dead stacks and merge stacks with identical top states.
		stacks = mergeStacksWithScratch(stacks, &scratch.merge)
		if len(stacks) == 0 {
			arena.Release()
			return NewTree(nil, source, p.language)
		}

		// Cap the number of parallel stacks to prevent combinatorial explosion.
		const maxStacks = 64
		if len(stacks) > maxStacks {
			stacks = stacks[:maxStacks]
		}

		// Safety: if the primary stack has grown beyond the depth cap,
		// or we've allocated too many nodes, return what we have.
		if len(stacks[0].entries) > maxDepth || nodeCount > maxNodes {
			return finalize(stacks)
		}

		// Use the primary (first) stack's state for DFA lex mode selection.
		if dts, ok := ts.(*dfaTokenSource); ok {
			dts.state = stacks[0].top().state
		}

		if needToken {
			tok = ts.Next()
		}

		// Incremental parsing fast-path: when there is a single active stack,
		// try to reuse an unchanged subtree starting at the current token.
		if reuse != nil && len(stacks) == 1 && !stacks[0].dead && tok.Symbol != 0 {
			if nextTok, ok := p.tryReuseSubtree(&stacks[0], tok, ts, reuse, &scratch.entries); ok {
				reusedAny = true
				tok = nextTok
				needToken = false
				consecutiveReduces = 0
				continue
			}
		}

		// Process all alive stacks for this token.
		// We iterate by index because forks may append to `stacks`.
		numStacks := len(stacks)
		anyReduced := false

		for si := 0; si < numStacks; si++ {
			s := &stacks[si]
			if s.dead {
				continue
			}

			currentState := s.top().state
			action := p.lookupAction(currentState, tok.Symbol)

			// --- Extra token handling (comments, whitespace) ---
			if action != nil && len(action.Actions) > 0 &&
				action.Actions[0].Type == ParseActionShift && action.Actions[0].Extra {
				named := p.isNamedSymbol(tok.Symbol)
				leaf := newLeafNodeInArena(arena, tok.Symbol, named,
					tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
				s.push(currentState, leaf, &scratch.entries)
				nodeCount++
				needToken = true
				continue
			}

			// --- No action: error handling ---
			if action == nil || len(action.Actions) == 0 {
				if tok.Symbol == 0 {
					if tok.StartByte == tok.EndByte {
						// True EOF. If this is the only stack, return result.
						if len(stacks) == 1 {
							return finalize(stacks)
						}
						// Multiple stacks at EOF: this one is done.
						// Mark dead so merge picks the best remaining.
						s.dead = true
						continue
					}
					// Zero-symbol width token: skip.
					needToken = true
					continue
				}

				// Try grammar-directed recovery by searching the stack for
				// the nearest state that can recover on this lookahead.
				if depth, recoverAct, ok := p.findRecoverActionOnStack(s, tok.Symbol); ok {
					s.entries = s.entries[:depth+1]
					p.applyAction(s, recoverAct, tok, &anyReduced, &nodeCount, arena, &scratch.entries)
					needToken = true
					continue
				}

				// If other stacks have valid actions, kill this one.
				if len(stacks) > 1 {
					s.dead = true
					continue
				}

				// Only stack: error recovery — wrap token in error node.
				if len(s.entries) == 0 {
					return finalize(stacks)
				}
				errNode := newLeafNodeInArena(arena, errorSymbol, false,
					tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
				errNode.hasError = true
				s.push(currentState, errNode, &scratch.entries)
				nodeCount++
				needToken = true
				continue
			}

			// --- GLR: fork for multiple actions ---
			// For single-action entries (the common case), no fork occurs.
			// For multi-action entries, clone the stack for each alternative.
			actions := action.Actions
			if len(actions) > 1 {
				// Save state before applying any action.
				saved := s.cloneWithScratch(&scratch.entries)
				// Apply first action to the original stack.
				p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries)
				// Clone for each additional action.
				for ai := 1; ai < len(actions); ai++ {
					fork := saved.cloneWithScratch(&scratch.entries)
					p.applyAction(&fork, actions[ai], tok, &anyReduced, &nodeCount, arena, &scratch.entries)
					stacks = append(stacks, fork)
				}
			} else {
				p.applyAction(s, actions[0], tok, &anyReduced, &nodeCount, arena, &scratch.entries)
			}
		}

		// After processing all stacks: determine whether to advance the
		// token. If any stack reduced, reuse the same token (the reducing
		// stacks have new top states and need to re-check the action for
		// the current lookahead). Otherwise, advance to next token.
		if anyReduced {
			needToken = false

			// Infinite-reduce detection (for the primary stack).
			if len(stacks) > 0 && !stacks[0].dead {
				topState := stacks[0].top().state
				if topState == lastReduceState {
					consecutiveReduces++
				} else {
					lastReduceState = topState
					consecutiveReduces = 1
				}
				if consecutiveReduces > 10 {
					needToken = true
					consecutiveReduces = 0
				}
			}
		} else {
			needToken = true
			consecutiveReduces = 0
		}

		// Check for accept on any stack.
		for si := range stacks {
			if stacks[si].accepted {
				return finalize(stacks[si : si+1])
			}
		}
	}

	// Iteration limit reached.
	return finalize(stacks)
}

// applyAction applies a single parse action to a GLR stack.
func (p *Parser) applyAction(s *glrStack, act ParseAction, tok Token, anyReduced *bool, nodeCount *int, arena *nodeArena, entryScratch *glrEntryScratch) {
	switch act.Type {
	case ParseActionShift:
		named := p.isNamedSymbol(tok.Symbol)
		leaf := newLeafNodeInArena(arena, tok.Symbol, named,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		s.push(act.State, leaf, entryScratch)
		*nodeCount++

	case ParseActionReduce:
		childCount := int(act.ChildCount)
		if childCount > len(s.entries)-1 {
			// Not enough stack entries — kill this stack version.
			s.dead = true
			return
		}

		children := arena.allocNodeSlice(childCount)
		for i := childCount - 1; i >= 0; i-- {
			children[i] = s.entries[len(s.entries)-1].node
			s.entries = s.entries[:len(s.entries)-1]
		}

		named := p.isNamedSymbol(act.Symbol)
		fieldIDs := p.buildFieldIDs(childCount, act.ProductionID, arena)
		parent := newParentNodeInArena(arena, act.Symbol, named, children, fieldIDs, act.ProductionID)
		*nodeCount++

		topState := s.entries[len(s.entries)-1].state
		gotoState := p.lookupGoto(topState, act.Symbol)

		if gotoState != 0 {
			s.push(gotoState, parent, entryScratch)
		} else {
			s.push(topState, parent, entryScratch)
		}

		s.score += int(act.DynamicPrecedence)
		*anyReduced = true

	case ParseActionAccept:
		s.accepted = true

	case ParseActionRecover:
		if tok.Symbol == 0 && tok.StartByte == tok.EndByte {
			s.accepted = true
			return
		}
		errNode := newLeafNodeInArena(arena, errorSymbol, false,
			tok.StartByte, tok.EndByte, tok.StartPoint, tok.EndPoint)
		errNode.hasError = true
		recoverState := s.top().state
		if act.State != 0 {
			recoverState = act.State
		}
		s.push(recoverState, errNode, entryScratch)
		*nodeCount++
	}
}

func recoverAction(entry *ParseActionEntry) (ParseAction, bool) {
	if entry == nil {
		return ParseAction{}, false
	}
	for _, act := range entry.Actions {
		if act.Type == ParseActionRecover {
			return act, true
		}
	}
	return ParseAction{}, false
}

func (p *Parser) findRecoverActionOnStack(s *glrStack, sym Symbol) (int, ParseAction, bool) {
	if s == nil || len(s.entries) == 0 {
		return 0, ParseAction{}, false
	}
	for depth := len(s.entries) - 1; depth >= 0; depth-- {
		state := s.entries[depth].state
		action := p.lookupAction(state, sym)
		if act, ok := recoverAction(action); ok {
			return depth, act, true
		}
	}
	return 0, ParseAction{}, false
}

// buildFieldIDs creates the field ID slice for a reduce action.
func (p *Parser) buildFieldIDs(childCount int, productionID uint16, arena *nodeArena) []FieldID {
	if childCount <= 0 || len(p.language.FieldMapEntries) == 0 {
		return nil
	}

	pid := int(productionID)
	if pid >= len(p.language.FieldMapSlices) {
		return nil
	}

	fm := p.language.FieldMapSlices[pid]
	count := int(fm[1])
	if count == 0 {
		return nil
	}

	fieldIDs := arena.allocFieldIDSlice(childCount)
	start := int(fm[0])
	assigned := false
	for i := 0; i < count; i++ {
		entryIdx := start + i
		if entryIdx >= len(p.language.FieldMapEntries) {
			break
		}
		entry := p.language.FieldMapEntries[entryIdx]
		if int(entry.ChildIndex) < len(fieldIDs) {
			fieldIDs[entry.ChildIndex] = entry.FieldID
			assigned = true
		}
	}

	if !assigned {
		return nil
	}
	return fieldIDs
}

// buildResultFromGLR picks the best stack and constructs the final tree.
// Prefers accepted stacks, then highest score, then most entries.
func (p *Parser) buildResultFromGLR(stacks []glrStack, source []byte, arena *nodeArena, oldTree *Tree, reusedAny bool) *Tree {
	if len(stacks) == 0 {
		arena.Release()
		return NewTree(nil, source, p.language)
	}

	best := 0
	for i := 1; i < len(stacks); i++ {
		if stacks[i].dead && !stacks[best].dead {
			continue
		}
		if !stacks[i].dead && stacks[best].dead {
			best = i
			continue
		}
		if stacks[i].accepted && !stacks[best].accepted {
			best = i
			continue
		}
		if stacks[i].score > stacks[best].score {
			best = i
			continue
		}
		if stacks[i].score == stacks[best].score && len(stacks[i].entries) > len(stacks[best].entries) {
			best = i
		}
	}

	return p.buildResult(stacks[best].entries, source, arena, oldTree, reusedAny)
}

// lookupAction looks up the parse action for the given state and symbol.
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
func (p *Parser) lookupGoto(state StateID, sym Symbol) StateID {
	raw := p.lookupActionIndex(state, sym)
	if raw == 0 {
		return 0
	}

	// ts2go-generated grammars encode nonterminal GOTO values directly as
	// parser state IDs. Hand-built grammars encode parse-action indices.
	if p.language.TokenCount > 0 &&
		uint32(sym) >= p.language.TokenCount &&
		p.language.StateCount > 0 &&
		(p.language.LargeStateCount > 0 || len(p.language.SmallParseTableMap) > 0) {
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

// isNamedSymbol checks whether a symbol is a named symbol.
func (p *Parser) isNamedSymbol(sym Symbol) bool {
	if int(sym) < len(p.language.SymbolMetadata) {
		return p.language.SymbolMetadata[sym].Named
	}
	return false
}

// buildResult constructs the final Tree from a stack of entries.
func (p *Parser) buildResult(stack []stackEntry, source []byte, arena *nodeArena, oldTree *Tree, reusedAny bool) *Tree {
	var nodes []*Node
	for _, entry := range stack {
		if entry.node != nil {
			nodes = append(nodes, entry.node)
		}
	}

	if len(nodes) == 0 {
		arena.Release()
		return NewTree(nil, source, p.language)
	}

	if arena != nil && arena.used == 0 {
		arena.Release()
		arena = nil
	}

	borrowed := retainBorrowedArenas(oldTree, reusedAny)

	if len(nodes) == 1 {
		return newTreeWithArenas(nodes[0], source, p.language, arena, borrowed)
	}

	root := newParentNodeInArena(arena, nodes[len(nodes)-1].symbol, true, nodes, nil, 0)
	root.hasError = true
	return newTreeWithArenas(root, source, p.language, arena, borrowed)
}

func retainBorrowedArenas(oldTree *Tree, reusedAny bool) []*nodeArena {
	if !reusedAny || oldTree == nil {
		return nil
	}
	refs := oldTree.referencedArenas()
	if len(refs) == 0 {
		return nil
	}
	for _, a := range refs {
		a.Retain()
	}
	return refs
}
