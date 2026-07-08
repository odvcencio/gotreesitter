package gotreesitter

import (
	"errors"
	"fmt"
	"time"
	"unsafe"
)

type parseConfig struct {
	oldTree     *Tree
	tokenSource TokenSource
	profiling   bool
}

// ParseStoppedEarlyError reports a parse that returned a tree but stopped
// before accepting the input. The returned tree is still available to callers
// that want diagnostics or partial output.
type ParseStoppedEarlyError struct {
	Reason  ParseStopReason
	Runtime ParseRuntime
}

func (e *ParseStoppedEarlyError) Error() string {
	if e == nil {
		return ErrParseStoppedEarly.Error()
	}
	reason := e.Reason
	if reason == "" {
		reason = ParseStopNone
	}
	return fmt.Sprintf("%s: %s", ErrParseStoppedEarly, reason)
}

func (e *ParseStoppedEarlyError) Is(target error) bool {
	return target == ErrParseStoppedEarly
}

func parseStoppedEarlyError(tree *Tree) error {
	if tree == nil || !tree.ParseStoppedEarly() {
		return nil
	}
	rt := tree.ParseRuntime()
	reason := rt.StopReason
	if reason == "" {
		reason = tree.ParseStopReason()
	}
	return &ParseStoppedEarlyError{
		Reason:  reason,
		Runtime: rt,
	}
}

func strictParseResult(tree *Tree, err error) (*Tree, error) {
	if err != nil {
		return tree, err
	}
	if stoppedErr := parseStoppedEarlyError(tree); stoppedErr != nil {
		return tree, stoppedErr
	}
	return tree, nil
}

// TokenSourceFactory builds a token source for parser source bytes.
type TokenSourceFactory func(source []byte) (TokenSource, error)

// ParserLogType categorizes parser log messages.
type ParserLogType uint8

const (
	// ParserLogParse emits parser-loop lifecycle and control-flow logs.
	ParserLogParse ParserLogType = iota
	// ParserLogLex emits token-source and token-consumption logs.
	ParserLogLex
)

// ParserLogger receives parser debug logs when configured via SetLogger.
type ParserLogger func(kind ParserLogType, message string)

func normalizeReturnedTree(root *Node, source []byte, lang *Language) {
	if root == nil || lang == nil {
		return
	}
	switch lang.Name {
	case "scala":
		normalizeScalaTemplateBodyObjectFragments(root, source, nil, lang)
		normalizeScalaRecoveredObjectTemplateBodies(root, source, nil, lang)
		normalizeScalaDefinitionFields(root, source, lang)
		normalizeScalaTemplateBodyFunctionAnnotations(root, source, nil, lang)
		normalizeScalaTemplateBodyFunctionEnds(root, source, lang)
		normalizeScalaCaseClauseEnds(root, source, lang)
		normalizeRootEOFNewlineSpan(root, source, lang)
	case "html":
		normalizeHTMLRecoveredNestedCustomTagRanges(root, source, lang)
	case "javascript":
		normalizeJavaScriptProgramEnd(root, source, lang)
	}
}

func shouldNormalizeIncrementalReturnedTree(tree, oldTree *Tree) bool {
	if tree == nil {
		return false
	}
	if tree.ParseStoppedEarly() {
		return false
	}
	if oldTree == nil {
		return true
	}
	return rawRootOrNil(tree) != rawRootOrNil(oldTree)
}

func (p *Parser) normalizeReturnedIncrementalTree(tree, oldTree *Tree, source []byte) {
	if !shouldNormalizeIncrementalReturnedTree(tree, oldTree) {
		return
	}
	if tree.resultCompatibilityPending {
		finalizeDeferredReturnedTreeTruncation(tree, source)
		return
	}
	if reason := p.normalizeReturnedTree(rawRootOrNil(tree), source); parseStopReasonIsTerminal(reason) {
		tree.setParseStopReason(reason)
		return
	}
	finalizeReturnedTreeRootSpan(tree, source)
}

func shouldNormalizeReturnedTree(tree *Tree) bool {
	return tree != nil && !tree.ParseStoppedEarly()
}

func (p *Parser) normalizeReturnedTreeForParse(tree *Tree, source []byte) {
	if !shouldNormalizeReturnedTree(tree) {
		return
	}
	if tree.resultCompatibilityPending {
		finalizeDeferredReturnedTreeTruncation(tree, source)
		return
	}
	if reason := p.normalizeReturnedTree(rawRootOrNil(tree), source); parseStopReasonIsTerminal(reason) {
		tree.setParseStopReason(reason)
		return
	}
	finalizeReturnedTreeRootSpan(tree, source)
}

// finalizeDeferredReturnedTreeTruncation enforces the silent-truncation contract
// for a returned tree whose result-compatibility normalization is deferred
// (ini/typescript/tsx via shouldDeferResultCompatibility). finalizeReturnedTreeRootSpan
// is skipped for these trees, so without this a truncated typescript parse
// (checker.ts / dom.generated.d.ts) returns a SILENT prefix-only program root
// (root.HasError()==false, ParseStoppedEarly()==false). The deferred trees
// already carry a correct rt.Truncated (recordParseRuntimeRootStats computes it,
// trivia tail included); only the consumer-visible HasError signal is missing.
// Clean deferred trees (rt.Truncated==false, the common typescript path) are
// untouched and keep their lazy compat.
func finalizeDeferredReturnedTreeTruncation(tree *Tree, _ []byte) {
	if tree == nil || !tree.parseRuntime.Truncated {
		return
	}
	// Flush the deferred compat normalization first (only for the rare truncated
	// case) so the HasError mark lands on the final normalized root and cannot be
	// dropped if a normalizer rebuilds the root. ensureResultCompatibility clears
	// resultCompatibilityPending.
	tree.ensureResultCompatibility()
	markTruncatedTreeHasError(tree.parseRuntime, rawRootOrNil(tree))
}

const forestIncrementalReuseUnsupportedReason = "old tree was built by GSS forest fast path"

func oldTreeDisablesIncrementalReuse(oldTree *Tree) bool {
	return oldTree != nil && oldTree.incrementalReuseDisabled
}

func (p *Parser) tryTokenInvariantReuseForDisabledOldTree(source []byte, oldTree *Tree, timing *incrementalParseTiming) (*Tree, bool) {
	if !oldTreeDisablesIncrementalReuse(oldTree) {
		return nil, false
	}
	if p == nil || p.language == nil {
		return nil, false
	}
	if !p.disabledOldTreeTokenInvariantLeafAllowed(source, oldTree) {
		return nil, false
	}
	if p.checkDFALexer() != nil {
		return nil, false
	}
	prevFactory := p.reparseFactory
	p.reparseFactory = nil
	defer func() {
		p.reparseFactory = prevFactory
	}()
	ts := p.acquireParserDFATokenSource(source)
	defer ts.Close()
	tree, ok := p.tryTokenInvariantLeafEdit(source, oldTree, p.wrapIncludedRanges(ts), timing)
	if !ok {
		return nil, false
	}
	p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	return tree, true
}

func (p *Parser) disabledOldTreeTokenInvariantLeafAllowed(source []byte, oldTree *Tree) bool {
	root, edit, ok := p.tokenInvariantLeafEditCandidate(source, oldTree)
	if !ok {
		return false
	}
	node := oldTree.lastEditedLeaf
	if node == nil || !node.containsByteRange(edit.StartByte, edit.OldEndByte) {
		node = root.DescendantForByteRange(edit.StartByte, edit.OldEndByte)
	}
	if p.canReuseLanguageTextInvariantNode(source, oldTree, node, edit) {
		return true
	}
	if !tokenInvariantLeafReusable(node) {
		return false
	}
	switch p.language.Name {
	case "go":
		return true
	case "css", "scss":
		return cssDisabledTreeTokenInvariantLeafAllowed(oldTree, node)
	case "c_sharp":
		return csharpDisabledTreeTokenInvariantLeafAllowed(p.language, node, oldTree.source, source)
	default:
		return false
	}
}

func cssDisabledTreeTokenInvariantLeafAllowed(oldTree *Tree, leaf *Node) bool {
	return oldTree != nil && oldTree.forestFastPath && tokenInvariantLeafReusable(leaf)
}

func csharpDisabledTreeTokenInvariantLeafAllowed(lang *Language, leaf *Node, oldSource, newSource []byte) bool {
	switch leaf.Type(lang) {
	case "integer_literal", "real_literal":
		return true
	case "identifier":
		return csharpTokenInvariantIdentifierText(oldSource, leaf) &&
			csharpTokenInvariantIdentifierText(newSource, leaf)
	default:
		return false
	}
}

func csharpTokenInvariantIdentifierText(source []byte, leaf *Node) bool {
	if leaf == nil || leaf.startByte >= leaf.endByte || int(leaf.endByte) > len(source) {
		return false
	}
	text := source[leaf.startByte:leaf.endByte]
	if !csharpSimpleIdentifierBytes(text) {
		return false
	}
	return !csharpTokenInvariantIdentifierKeyword(string(text))
}

func csharpSimpleIdentifierBytes(text []byte) bool {
	if len(text) == 0 {
		return false
	}
	if !csharpIdentifierStartByte(text[0]) {
		return false
	}
	for _, b := range text[1:] {
		if !csharpIdentifierContinueByte(b) {
			return false
		}
	}
	return true
}

func csharpTokenInvariantIdentifierKeyword(text string) bool {
	switch text {
	case "abstract", "add", "alias", "as", "ascending", "async", "await", "base",
		"bool", "break", "by", "byte", "case", "catch", "char", "checked",
		"class", "const", "continue", "decimal", "default", "delegate",
		"descending", "do", "double", "dynamic", "else", "enum", "equals",
		"event", "explicit", "extern", "false", "file", "finally", "fixed",
		"float", "for", "foreach", "from", "get", "global", "goto", "group",
		"if", "implicit", "in", "init", "int", "interface", "internal", "into",
		"is", "join", "let", "lock", "long", "namespace", "new", "notnull",
		"null", "object", "on", "operator", "orderby", "out", "override",
		"params", "partial", "private", "protected", "public", "readonly",
		"record", "ref", "remove", "return", "sbyte", "scoped", "sealed",
		"select", "set", "short", "sizeof", "stackalloc", "static", "string",
		"struct", "switch", "this", "throw", "true", "try", "typeof", "uint",
		"ulong", "unchecked", "unsafe", "ushort", "using", "var", "virtual",
		"void", "volatile", "when", "where", "while", "with", "yield":
		return true
	default:
		return false
	}
}

func profileFreshParseFallback(start time.Time, tree *Tree, reason string) IncrementalParseProfile {
	profile := IncrementalParseProfile{
		ReparseNanos:           time.Since(start).Nanoseconds(),
		ReuseUnsupported:       true,
		ReuseUnsupportedReason: reason,
	}
	if tree == nil {
		return profile
	}
	timing := &incrementalParseTiming{totalNanos: profile.ReparseNanos}
	copyParseRuntimeToTiming(timing, tree.ParseRuntime())
	profile = timing.toProfile()
	profile.ReparseNanos = time.Since(start).Nanoseconds()
	profile.ReuseUnsupported = true
	profile.ReuseUnsupportedReason = reason
	return profile
}

func (p *Parser) normalizeReturnedTree(root *Node, source []byte) ParseStopReason {
	if p == nil || p.language == nil || root == nil || p.noResultCompatibilityBenchmarkOnly {
		return ParseStopNone
	}
	if reason := p.parseStopReasonNow(); parseStopReasonIsTerminal(reason) {
		return reason
	}
	normalizeResultCompatibility(root, source, p)
	return p.parseStopReasonNow()
}

func finalizeReturnedTreeRootSpan(tree *Tree, source []byte) {
	if tree == nil {
		return
	}
	root := rawRootOrNil(tree)
	if root == nil {
		return
	}
	rt := tree.parseRuntime
	if rt.StopReason == ParseStopAccepted {
		extendRootToAcceptedCleanTail(root, source, rt.ExpectedEOFByte, tree.includedRanges)
	}
	rt.RootEndByte = root.endByte
	rt.Truncated = rt.ExpectedEOFByte > root.endByte
	tailStart := root.endByte
	if rt.LastTokenWasEOF && rt.LastTokenEndByte > tailStart && rt.LastTokenEndByte <= rt.ExpectedEOFByte {
		tailStart = rt.LastTokenEndByte
	}
	if rt.Truncated && parserTailAllowsCleanAcceptance(source, tailStart, rt.ExpectedEOFByte, tree.includedRanges) {
		rt.Truncated = false
	}
	markTruncatedTreeHasError(rt, root)
	tree.setParseRuntime(rt)
}

// markTruncatedTreeHasError repairs the wave2b SILENT-TRUNCATION contract: a
// returned tree whose root does not span the input (rt.Truncated, a real
// non-trivia tail) MUST carry a reliable, consumer-visible error signal. C's
// recovery guarantees its returned tree always spans the input and reports
// HasError() for these inputs (the C-oracle confirms every one of the 14 known
// members has a full-span C root; 7 are ERROR-bearing). When our GLR frontier
// dies mid-file (ParseStopNoStacksAlive) after the live stack already reduced to
// a clean start symbol, we return a prefix-only root with hasError=false and
// ParseStoppedEarly()==false — the swallowed-error class (crystal string.cr,
// typescript checker.ts / dom.generated.d.ts; the _pydecimal.py family). The
// tree is definitionally erroneous (real source past root.EndByte was dropped),
// so surface root.HasError()==true. The tree is left honestly TRUNCATED (root
// span unchanged, rt.Truncated preserved) — matching the established contract
// that a non-clean tail is never silently extended
// (TestFinalizeReturnedTreeRootSpanDoesNotExtendNonCleanTail); making the whole
// span cover the input is the GLR/dispatch layer's job (siblings), not the
// result-reporting layer's.
//
// Only ever SETS the flag (never clears): a truncated tree that already reports
// HasError (11 of the 14 members) is untouched. Trivia-only tails already
// cleared rt.Truncated above, and early-stop trees (node_limit / memory_budget /
// timeout) never reach this finalizer, so hard budget aborts keep their honest
// Truncated flag without a synthetic error mark.
func markTruncatedTreeHasError(rt ParseRuntime, root *Node) {
	if !rt.Truncated || root == nil || root.hasError() {
		return
	}
	root.setHasError(true)
}

func extendRootToAcceptedCleanTail(root *Node, source []byte, expectedEOFByte uint32, included []Range) bool {
	if root == nil || expectedEOFByte <= root.endByte {
		return false
	}
	if !parserTailAllowsCleanAcceptance(source, root.endByte, expectedEOFByte, included) {
		return false
	}
	extendNodeEndToByte(root, source, expectedEOFByte)
	return root.endByte >= expectedEOFByte
}

func extendNodeEndToByte(n *Node, source []byte, endByte uint32) {
	if n == nil || endByte <= n.endByte || int(endByte) > len(source) {
		return
	}
	n.endPoint = advancePointByBytes(n.endPoint, source[n.endByte:endByte])
	n.endByte = endByte
}

func (p *Parser) dfaReparseFactory() TokenSourceFactory {
	if p == nil || p.language == nil || len(p.language.LexStates) == 0 {
		return nil
	}
	return func(source []byte) (TokenSource, error) {
		return p.newDFAReparseTokenSource(source), nil
	}
}

func (p *Parser) newDFAReparseTokenSource(source []byte) TokenSource {
	if p == nil || p.language == nil || len(p.language.LexStates) == 0 {
		return nil
	}
	lexer := NewLexer(p.language.LexStates, source)
	return newDFATokenSourceDirectWithCRecovery(lexer, p.language, p.lookupActionIndexFunc(), p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState, p.errorCostCompetitionEnabled())
}

// acquireParserDFATokenSource returns a pooled dfaTokenSource wired to p's
// language tables, reusing the pooled source's retained lexer so steady-state
// parses allocate neither a token source nor a lexer. The caller must Close()
// the returned source (Close returns it to the pool), and its lifetime must
// not extend past that Close. Returns nil when p has no DFA lex tables.
func (p *Parser) acquireParserDFATokenSource(source []byte) *dfaTokenSource {
	if p == nil || p.language == nil || len(p.language.LexStates) == 0 {
		return nil
	}
	return acquireDFATokenSourceReusingLexer(source, p.language, p.lookupActionIndexFunc(), p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState, p.errorCostCompetitionEnabled())
}

func (p *Parser) tokenSourceReparseFactory(ts TokenSource) TokenSourceFactory {
	if rebuilder, ok := ts.(TokenSourceRebuilder); ok {
		return func(source []byte) (TokenSource, error) {
			return rebuilder.RebuildTokenSource(source, p.language)
		}
	}
	return nil
}

func (p *Parser) parseForRecovery(source []byte) (*Tree, error) {
	if p == nil || p.language == nil {
		return nil, ErrNoLanguage
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return nil, &ParseStoppedEarlyError{Reason: reason}
	}
	parser := p.recoveryParser
	if parser == nil || parser.language != p.language {
		if parser != nil {
			releaseSnippetParser(parser)
		}
		parser = acquireSnippetParser(p.language)
		if parser == nil {
			return nil, ErrNoLanguage
		}
		p.recoveryParser = parser
	}
	parser.skipRecoveryReparse = true
	parser.timeoutMicros = p.remainingTimeoutMicros()
	parser.cancellationFlag = p.cancellationFlag
	if p.reparseFactory != nil {
		ts, err := p.reparseFactory(source)
		if err != nil {
			return nil, err
		}
		tree, err := parser.ParseWithTokenSource(source, ts)
		if tree != nil && parseStopReasonIsActive(tree.ParseStopReason()) {
			p.markActiveParseStopped(tree.ParseStopReason())
		}
		return tree, err
	}
	tree, err := parser.Parse(source)
	if tree != nil && parseStopReasonIsActive(tree.ParseStopReason()) {
		p.markActiveParseStopped(tree.ParseStopReason())
	}
	return tree, err
}

func (p *Parser) clearRecoveryParser() {
	if p == nil || p.recoveryParser == nil {
		return
	}
	releaseSnippetParser(p.recoveryParser)
	p.recoveryParser = nil
}

func (p *Parser) borrowCompatibilityArena(arena *nodeArena) {
	if p == nil || arena == nil {
		return
	}
	for _, existing := range p.compatibilityBorrowedArenas {
		if existing == arena {
			return
		}
	}
	arena.Retain()
	p.compatibilityBorrowedArenas = append(p.compatibilityBorrowedArenas, arena)
}

func (p *Parser) borrowCompatibilityTreeArenas(tree *Tree) {
	if p == nil || tree == nil {
		return
	}
	p.borrowCompatibilityArena(tree.arena)
	for _, arena := range tree.borrowedArena {
		p.borrowCompatibilityArena(arena)
	}
}

func (p *Parser) takeCompatibilityBorrowedArenas() []*nodeArena {
	if p == nil || len(p.compatibilityBorrowedArenas) == 0 {
		return nil
	}
	out := p.compatibilityBorrowedArenas
	p.compatibilityBorrowedArenas = nil
	return out
}

func (p *Parser) releaseCompatibilityBorrowedArenas() {
	if p == nil || len(p.compatibilityBorrowedArenas) == 0 {
		return
	}
	for _, arena := range p.compatibilityBorrowedArenas {
		if arena != nil {
			arena.Release()
		}
	}
	clear(p.compatibilityBorrowedArenas)
	p.compatibilityBorrowedArenas = nil
}

// parseWithSnippetParser runs a recovery snippet parse. timeoutMicros is
// optional so callers can inherit a parent parser timeout when needed.
func parseWithSnippetParser(lang *Language, source []byte, timeoutMicros ...uint64) (*Tree, error) {
	return parseWithSnippetParserInheriting(lang, source, nil, timeoutMicros...)
}

func parseWithSnippetParserInheriting(lang *Language, source []byte, parent *Parser, timeoutMicros ...uint64) (*Tree, error) {
	parser := acquireSnippetParser(lang)
	if parser == nil {
		return nil, ErrNoLanguage
	}
	defer releaseSnippetParser(parser)
	if parent != nil {
		parser.timeoutMicros = parent.remainingTimeoutMicros()
		parser.cancellationFlag = parent.cancellationFlag
		if reason := parent.activeParseStopReason(); parseStopReasonIsActive(reason) {
			parser.parseStoppedReason = reason
			parser.parseBudgetDepth = 1
		}
	}
	if parent == nil && len(timeoutMicros) > 0 && timeoutMicros[0] > 0 {
		parser.timeoutMicros = timeoutMicros[0]
	}
	tree, err := parser.Parse(source)
	if parent != nil && tree != nil && parseStopReasonIsActive(tree.ParseStopReason()) {
		parent.markActiveParseStopped(tree.ParseStopReason())
	}
	return tree, err
}

type closeableTokenSource interface {
	Close()
}

func manageTokenSourceLifetime(ts TokenSource) func() {
	closer, ok := ts.(closeableTokenSource)
	if !ok {
		return func() {}
	}
	return closer.Close
}

func (p *Parser) parseWithTokenSource(source []byte, ts TokenSource, reparseFactory TokenSourceFactory) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if ts == nil {
		return nil, ErrNoTokenSource
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	p.fullParseRetryPassesTaken = 0
	p.releaseCompatibilityBorrowedArenas()
	p.clearRecoveryParser()
	defer p.clearRecoveryParser()
	releaseTS := manageTokenSourceLifetime(ts)
	defer releaseTS()
	prevFactory := p.reparseFactory
	p.reparseFactory = reparseFactory
	defer func() {
		p.reparseFactory = prevFactory
	}()
	deterministicExternalConflicts := fullParseUsesDeterministicExternalConflicts(p.language)
	initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth)
	tree := p.parseInternal(source, p.wrapIncludedRanges(ts), nil, nil, arenaClassFull, nil, initialMaxStacks, 0, 0, deterministicExternalConflicts)
	if tree != nil && !tree.ParseStoppedEarly() && !parseStopReasonIsActive(p.activeParseStopReason()) {
		tree = p.retryFullParseWithTokenSource(source, ts, initialMaxStacks, deterministicExternalConflicts, tree)
		if tree != nil && !tree.ParseStoppedEarly() && !parseStopReasonIsActive(p.activeParseStopReason()) && shouldRepeatExternalScannerFullParse(p.language, tree) {
			tree = p.retryFullParseWithTokenSource(source, ts, initialMaxStacks, deterministicExternalConflicts, tree)
		}
	}
	p.normalizeReturnedTreeForParse(tree, source)
	return tree, nil
}

func (p *Parser) parseIncrementalWithTokenSource(source []byte, oldTree *Tree, ts TokenSource, reparseFactory TokenSourceFactory) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if ts == nil {
		return nil, ErrNoTokenSource
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	releaseTS := manageTokenSourceLifetime(ts)
	defer releaseTS()
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, nil
	}
	return p.parseIncrementalWithTokenSourceChanged(source, oldTree, ts, reparseFactory)
}

func (p *Parser) parseIncrementalWithTokenSourceChanged(source []byte, oldTree *Tree, ts TokenSource, reparseFactory TokenSourceFactory) (*Tree, error) {
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
	p.fullParseRetryPassesTaken = 0
	prevFactory := p.reparseFactory
	p.reparseFactory = reparseFactory
	defer func() {
		p.reparseFactory = prevFactory
	}()
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), nil)
	// Never hand oldTree to the retry helper: it releases the losing tree,
	// and oldTree is owned by the caller.
	if initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth); tree != oldTree && shouldRetryIncrementalParseAsFull(tree, len(source), initialMaxStacks) {
		tree = p.retryIncrementalParseAsFullWithTokenSource(source, ts, initialMaxStacks, tree, nil)
	}
	p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	return tree, nil
}

// ParseOption configures ParseWith behavior.
type ParseOption func(*parseConfig)

// WithOldTree enables incremental parsing against an edited prior tree.
func WithOldTree(oldTree *Tree) ParseOption {
	return func(c *parseConfig) {
		c.oldTree = oldTree
	}
}

// WithTokenSource provides a custom token source for parsing.
func WithTokenSource(ts TokenSource) ParseOption {
	return func(c *parseConfig) {
		c.tokenSource = ts
	}
}

// WithProfiling enables incremental parse attribution in ParseResult.Profile.
func WithProfiling() ParseOption {
	return func(c *parseConfig) {
		c.profiling = true
	}
}

// ParseResult is returned by ParseWith.
type ParseResult struct {
	Tree *Tree
	// Profile is populated only when ParseWith uses WithProfiling for
	// incremental parsing.
	Profile IncrementalParseProfile
	// ProfileAvailable reports whether Profile contains attribution data.
	ProfileAvailable bool
}

// Language returns the parser's configured language.
func (p *Parser) Language() *Language {
	if p == nil {
		return nil
	}
	return p.language
}

// SetGLRTrace enables verbose GLR stack tracing to stdout (debug only).
func (p *Parser) SetGLRTrace(enabled bool) {
	if p == nil {
		return
	}
	p.glrTrace = enabled
}

// SetAmbiguityProfile installs an optional diagnostic ambiguity profile.
// The profile receives parser state/lookahead/action counters for GLR-heavy
// benchmark runs. Pass nil to disable profiling.
func (p *Parser) SetAmbiguityProfile(profile *AmbiguityProfile) {
	if p == nil {
		return
	}
	p.ambiguityProfile = profile
}

// SetLogger installs a parser debug logger. Pass nil to disable logging.
func (p *Parser) SetLogger(logger ParserLogger) {
	if p == nil {
		return
	}
	p.logger = logger
}

// Logger returns the currently configured parser debug logger.
func (p *Parser) Logger() ParserLogger {
	if p == nil {
		return nil
	}
	return p.logger
}

// SetTimeoutMicros configures a per-parse timeout in microseconds. A value of 0
// disables timeout checks. Parse methods preserve tree-sitter's partial-tree
// behavior on timeout: they return a tree and nil error, with
// tree.ParseStopReason() == ParseStopTimeout and tree.ParseStoppedEarly() true.
// Use ParseStrict or another strict parse method to treat early stops as errors.
func (p *Parser) SetTimeoutMicros(timeoutMicros uint64) {
	if p == nil {
		return
	}
	p.timeoutMicros = timeoutMicros
}

// TimeoutMicros returns the parser timeout in microseconds.
func (p *Parser) TimeoutMicros() uint64 {
	if p == nil {
		return 0
	}
	return p.timeoutMicros
}

// SetCancellationFlag configures a caller-owned cancellation flag.
// Parsing stops when the pointed value becomes non-zero.
func (p *Parser) SetCancellationFlag(flag *uint32) {
	if p == nil {
		return
	}
	p.cancellationFlag = flag
}

// CancellationFlag returns the parser's current cancellation flag pointer.
func (p *Parser) CancellationFlag() *uint32 {
	if p == nil {
		return nil
	}
	return p.cancellationFlag
}

// SetIncludedRanges configures parser include ranges.
// Tokens outside these ranges are skipped.
func (p *Parser) SetIncludedRanges(ranges []Range) {
	if p == nil {
		return
	}
	p.included = normalizeIncludedRanges(ranges)
}

// SetIncludedUTF16Ranges configures parser include ranges from UTF-16
// code-unit ranges. Internal parser points are derived from source as UTF-8
// columns.
func (p *Parser) SetIncludedUTF16Ranges(source []uint16, ranges []UTF16Range) bool {
	converted, ok := IncludedRangesForUTF16(source, ranges)
	if !ok {
		return false
	}
	p.SetIncludedRanges(converted)
	return true
}

// SetIncludedUTF16ByteRanges configures parser include ranges from
// endian-specific UTF-16 bytes.
func (p *Parser) SetIncludedUTF16ByteRanges(source []byte, order UTF16ByteOrder, ranges []UTF16Range) error {
	converted, err := IncludedRangesForUTF16Bytes(source, order, ranges)
	if err != nil {
		return err
	}
	p.SetIncludedRanges(converted)
	return nil
}

// IncludedRanges returns a copy of the configured include ranges.
func (p *Parser) IncludedRanges() []Range {
	if p == nil || len(p.included) == 0 {
		return nil
	}
	out := make([]Range, len(p.included))
	copy(out, p.included)
	return out
}

func (p *Parser) wrapIncludedRanges(ts TokenSource) TokenSource {
	if p == nil || len(p.included) == 0 || ts == nil {
		return ts
	}
	return newIncludedRangeTokenSource(ts, p.included)
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

// TokenSourceRebuilder is an optional extension for token sources that can
// build a fresh equivalent token source for another source buffer. Result
// normalization uses this to reparse isolated fragments with the same lexer
// backend as the original parse.
type TokenSourceRebuilder interface {
	RebuildTokenSource(source []byte, lang *Language) (TokenSource, error)
}

// ByteSkippableTokenSource can jump to a byte offset and return the first
// token at or after that position.
type ByteSkippableTokenSource interface {
	TokenSource
	SkipToByte(offset uint32) Token
}

// PointSkippableTokenSource extends ByteSkippableTokenSource with a hint-based
// skip that avoids recomputing row/column from byte offset. During incremental
// parsing the reused node already carries its endpoint, so passing it directly
// eliminates the O(n) offset-to-point scan.
type PointSkippableTokenSource interface {
	ByteSkippableTokenSource
	SkipToByteWithPoint(offset uint32, pt Point) Token
}

// tokenSourceRelexer is an internal parser-loop extension for token sources
// that can re-read a token from its original start after parser state changes.
type tokenSourceRelexer interface {
	CanRelexFromTokenStart(tok Token) bool
	RelexFromTokenStart(tok Token) (Token, bool)
}

// IncrementalReuseTokenSource is an opt-in marker for custom token sources
// that are safe for incremental subtree reuse. Implementations must provide
// stable token boundaries across edits and support deterministic SkipToByte*
// behavior so reused-tree fast-forwarding remains correct.
type IncrementalReuseTokenSource interface {
	TokenSource
	SupportsIncrementalReuse() bool
}

type parserStateTokenSource interface {
	SetParserState(state StateID)
	// SetGLRStates provides all active GLR stack states so the token source
	// can compute valid external symbols as the union across all stacks.
	// This is critical for grammars with external scanners and GLR conflicts.
	SetGLRStates(states []StateID)
}

// errorModeLexingTokenSource is implemented by token sources that honor
// SetParserState(0) with C-equivalent ERROR-state lexing (LexModes[0], the
// most permissive mode). The faithful C recovery port uses it to know whether
// a token lexed while every live stack was absorbing already carries the
// error-mode identity C's ts_parser__lex would produce (see
// cRecoverElectionLookaheadSymbol).
type errorModeLexingTokenSource interface {
	lexesErrorModeAtErrorState() bool
}

// stackEntry is a single parser LR-stack entry. The hot path stores real
// public tree nodes directly; compact parse modes can tag the same 16-byte slot
// as a noTreeNode, compact leaf, or pending-parent payload.
type stackEntry struct {
	node  unsafe.Pointer
	state StateID
	kind  uint32
}

// errorSymbol is the well-known symbol ID used for error nodes.
const errorSymbol = Symbol(65535)

// Parse tokenizes and parses source using the built-in DFA lexer, returning
// a syntax tree. This works for hand-built grammars that provide LexStates.
// For real grammars that need a custom lexer, use ParseWithTokenSource.
// If the input is empty, it returns a tree with a nil root and no error.
func (p *Parser) Parse(source []byte) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if err := p.checkDFALexer(); err != nil {
		return nil, err
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	p.fullParseRetryPassesTaken = 0
	progress := newParseProgressTelemetry(p, len(source), uint32(len(source)), time.Now())
	if progress.enabled {
		progress.emit(time.Now(), "parse_entry", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
		defer progress.emit(time.Now(), "parse_return", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, "")
	}
	// GSS-forest fast path for languages whose production GLR parse blows up on
	// deep stack-equivalence (e.g. bash). Returns nil to fall back to the
	// production parser on any failure, error, or truncation. Off unless
	// GOT_GLR_FOREST is set; see tryForestFastPath.
	if progress.enabled {
		progress.emit(time.Now(), "forest_fast_path_begin", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	if tree := p.tryForestFastPath(source); tree != nil {
		if progress.enabled {
			progress.emit(time.Now(), "forest_fast_path_end", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, "used=true")
		}
		return tree, nil
	}
	if progress.enabled {
		progress.emit(time.Now(), "forest_fast_path_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "used=false")
	}
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
	p.releaseCompatibilityBorrowedArenas()
	p.clearRecoveryParser()
	defer p.clearRecoveryParser()
	prevFactory := p.reparseFactory
	if p.noResultCompatibilityBenchmarkOnly {
		p.reparseFactory = nil
	} else {
		p.reparseFactory = p.dfaReparseFactory()
	}
	defer func() {
		p.reparseFactory = prevFactory
	}()
	if progress.enabled {
		progress.emit(time.Now(), "token_source_setup_begin", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	ts := p.acquireParserDFATokenSource(source)
	if progress.enabled {
		progress.emit(time.Now(), "token_source_setup_end", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, "")
	}
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
		ts.usesExternalCheckpoints = false
	}
	defer ts.Close()
	deterministicExternalConflicts := fullParseUsesDeterministicExternalConflicts(p.language)
	initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth)
	if progress.enabled {
		progress.emit(time.Now(), "parse_internal_begin", 0, 0, Token{}, false, nil, 0, 0, 0, true, 0, 0, fmt.Sprintf("initial_max_stacks=%d deterministic_external_conflicts=%t", initialMaxStacks, deterministicExternalConflicts))
	}
	tree := p.parseInternal(source, p.wrapIncludedRanges(ts), nil, nil, arenaClassFull, nil, initialMaxStacks, 0, 0, deterministicExternalConflicts)
	if progress.enabled {
		progress.emit(time.Now(), "parse_internal_end", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, "")
	}
	if !p.noTreeBenchmarkOnly {
		if progress.enabled {
			progress.emit(time.Now(), "retry_begin", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, "")
		}
		if tree != nil && !tree.ParseStoppedEarly() && !parseStopReasonIsActive(p.activeParseStopReason()) {
			tree = p.retryFullParseWithDFA(source, initialMaxStacks, deterministicExternalConflicts, tree)
			if tree != nil && !tree.ParseStoppedEarly() && !parseStopReasonIsActive(p.activeParseStopReason()) && shouldRepeatExternalScannerFullParse(p.language, tree) {
				tree = p.retryFullParseWithDFA(source, initialMaxStacks, deterministicExternalConflicts, tree)
			}
		}
		if progress.enabled {
			progress.emit(time.Now(), "retry_end", 0, 0, Token{}, false, nil, 0, 0, 0, false, 0, 0, "")
		}
		p.normalizeReturnedTreeForParse(tree, source)
		tree = p.resolveCRecoverySwallowedError(source, tree)
	}
	return tree, nil
}

// resolveCRecoverySwallowedError is a language-agnostic safety net for a
// defect class in the faithful C error-recovery port (parser_recover_c.go):
// for a confirmed set of inputs (java/php/gomod malformed-input regressions;
// see parser_resync_recovery_test.go and
// grammars/php_parse_regression_test.go,
// grammars/gomod_parse_regression_test.go) the GLR engine's condense-step
// cost competition (ts_parser__compare_versions) can select a final result
// whose lineage discarded C-recovery-owned content in favor of a marker-free
// sibling, or that itself created a real ERROR node and was never
// re-validated by another cost competition — see cRecoveryDroppedErrorForClean
// and cRecoveryUnvalidatedMarker. The real tree-sitter C oracle (verified
// against the exact pinned grammar commits in grammars/languages.lock) never
// produces such a marker-free result from a version that needed recovery:
// every path out of ts_parser__handle_error/ts_parser__recover
// (recover_to_state, skip_token, recover_eof, missing-token insertion) wraps
// something in ERROR or MISSING. When that happens and it is the last thing
// standing between the parse and a clean result, HasError() silently goes
// false for genuinely malformed input.
//
// The check is deliberately narrow and cannot invent new errors or regress
// existing passing results:
//   - It fires only when the SELECTED result's own lineage carries the
//     signal (CRecoveryDroppedErrorForClean, set exclusively in
//     buildResultFromGLR for the stack that is actually returned — never for
//     a drop or fork that happened on some other, discarded lineage
//     elsewhere in the parse) AND the resulting tree is nonetheless
//     completely clean. Ordinary "recovered cleanly, nothing left over"
//     results (eds, ledger, authzed, dart, ...) never carry the signal.
//   - Even then, it only adopts the resync-based fallback's verdict if the
//     fallback itself completed normally (no timeout/cancellation/truncation)
//     and its own disagreement stays small in absolute byte size — see
//     crecoverySwallowedErrorMaxFallbackErrorBytes.
//
// In that specific combination this re-parses with the C-recovery gate
// disabled for this Parser instance (matching GOT_C_RECOVERY=0, i.e. the
// resync-based engine path that predates the C-recovery default-enablement)
// and adopts that result only if it actually reports an error; otherwise the
// original, C-recovery-produced tree is kept unchanged.
//
// Scope note: this guarantees HasError() correctness only, not tree shape.
// The adopted resync fallback can be structurally coarser than what a true,
// local C-recovery fix would have produced (e.g. the java fixture's adopted
// tree unwinds much of the method body into top-level ERROR nodes, where the
// real tree-sitter C oracle produces a narrow (ERROR (type_identifier))) —
// resync's whole-span ERROR-wrap heuristic (tryOpportunisticTopLevelResyncRecovery)
// is coarser than C-recovery's local cost competition by design. Fixing that
// shape gap would require a true local fix inside the C-recovery port itself,
// which this safety net does not attempt.
func (p *Parser) resolveCRecoverySwallowedError(source []byte, tree *Tree) *Tree {
	if p == nil || tree == nil {
		return tree
	}
	if !p.errorCostCompetitionEnabled() {
		return tree
	}
	if p.crecoverySwallowedErrorCheckActive {
		return tree
	}
	if tree.ParseStoppedEarly() {
		return tree
	}
	// Read the signal from the tree's OWN captured ParseRuntime, not the live
	// p.crecoveryEnteredErrorState/p.crecoveryDroppedErrorForClean parser
	// fields: parseInternal can run more than once per Parse() call (DFA
	// retries — see retryFullParseWithDFA), and a discarded retry attempt
	// would otherwise leak its recovery history into this check even though
	// it has nothing to do with the tree actually being returned.
	rt := tree.ParseRuntime()
	if !rt.CRecoveryEnteredErrorState || !rt.CRecoveryDroppedErrorForClean {
		// Either this tree never itself hit ts_parser__handle_error, or
		// recovery resolved cleanly without the selected lineage ever
		// discarding recovery-owned content for a marker-free sibling.
		// Nothing to double-check.
		return tree
	}
	root := tree.RootNode()
	if root == nil || root.HasError() {
		return tree
	}

	// defer (not a plain post-call restore) so a panic inside the fallback
	// Parse call can never leave this Parser instance permanently stuck with
	// C-recovery disabled.
	prevGate := p.errorCostCompetition
	prevCheckActive := p.crecoverySwallowedErrorCheckActive
	defer func() {
		p.errorCostCompetition = prevGate
		p.crecoverySwallowedErrorCheckActive = prevCheckActive
	}()
	p.errorCostCompetition = false
	p.crecoverySwallowedErrorCheckActive = true
	fallback, err := p.Parse(source)
	if err != nil || fallback == nil {
		return tree
	}
	markCRecoverySwallowedErrorFallbackAttempted(tree)
	// A caller-shared timeout/cancellation can trip while the (slower)
	// resync fallback is running. Adopting a truncated fallback tree with
	// err == nil would be worse than the swallowed-error bug this safety net
	// exists to fix, so reject it outright and keep the original result.
	if fallback.ParseStoppedEarly() || parseStopReasonIsActive(p.activeParseStopReason()) {
		fallback.Release()
		return tree
	}
	fallbackRoot := fallback.RootNode()
	if fallbackRoot == nil || !fallbackRoot.HasError() {
		// The resync-based path agrees the input is clean (or itself
		// couldn't build a root); keep the original C-recovery result.
		fallback.Release()
		return tree
	}
	// The resync-based path disagrees, but resync is a coarser, whole-span
	// ERROR-wrap heuristic (see tryOpportunisticTopLevelResyncRecovery) that
	// is known to sometimes unwind a large, otherwise-locally-recoverable
	// span into one ERROR instead of the narrow local recovery C-recovery's
	// cost competition would have produced — exactly why C-recovery owns
	// these dead ends by default for highly ambiguous grammars (kotlin's
	// generic-vs-comparison forks are the confirmed example: resync there
	// also disagrees, but by unwinding hundreds of bytes of a legitimately
	// valid, deeply nested lambda expression). Only trust resync's verdict
	// when its own ERROR/MISSING content stays small in absolute size: the
	// confirmed defect-class fixtures (java/php/gomod) all fit within a few
	// dozen bytes — a single malformed token or directive — while resync's
	// destructive whole-span unwinds run into the hundreds of bytes. A
	// fraction-of-source threshold does not separate these (java's own
	// malformed method body is itself a large fraction of its short test
	// source), so this is deliberately an absolute bound.
	if errorByteCoverage(fallbackRoot) > crecoverySwallowedErrorMaxFallbackErrorBytes {
		fallback.Release()
		return tree
	}
	markCRecoverySwallowedErrorFallbackAttempted(fallback)
	tree.Release()
	return fallback
}

// markCRecoverySwallowedErrorFallbackAttempted stamps t's ParseRuntime with
// CRecoverySwallowedErrorFallbackAttempted=true. Diagnostic bookkeeping only;
// see the field doc comment in tree.go.
func markCRecoverySwallowedErrorFallbackAttempted(t *Tree) {
	if t == nil {
		return
	}
	rt := t.ParseRuntime()
	rt.CRecoverySwallowedErrorFallbackAttempted = true
	t.setParseRuntime(rt)
}

// crecoverySwallowedErrorMaxFallbackErrorBytes bounds how many source bytes
// the resync fallback's own ERROR/MISSING content may cover before
// resolveCRecoverySwallowedError distrusts it as too coarse to adopt. Tuned
// against the confirmed defect-class fixtures — java 44 bytes, gomod 5 bytes,
// php 1 byte (a zero-width MISSING plus its immediate span) — versus the
// confirmed destructive resync unwind on ambiguous kotlin input (285 bytes on
// a 371-byte source). Kept well below that gap.
const crecoverySwallowedErrorMaxFallbackErrorBytes = 128

// errorByteCoverage returns the number of source bytes covered by the
// outermost ERROR/MISSING nodes under root (children of an ERROR/MISSING
// node are not double-counted; their span is already included).
func errorByteCoverage(root *Node) uint32 {
	if root == nil {
		return 0
	}
	var covered uint32
	var walk func(n *Node)
	walk = func(n *Node) {
		if n == nil {
			return
		}
		if n.IsError() || n.IsMissing() {
			span := n.EndByte() - n.StartByte()
			covered += span
			return
		}
		for i := 0; i < n.ChildCount(); i++ {
			walk(n.Child(i))
		}
	}
	walk(root)
	return covered
}

// ParseStrict is like Parse, but returns ErrParseStoppedEarly when parsing
// returns a partial tree due to timeout, cancellation, token-source EOF, or a
// parser safety limit. The partial tree is returned alongside the error.
func (p *Parser) ParseStrict(source []byte) (*Tree, error) {
	return strictParseResult(p.Parse(source))
}

// ParseNoTreeBenchmarkOnly parses source while suppressing parent/child tree
// materialization in reduce actions. It is intended only for parser-loop
// performance experiments; the returned tree is not API-compatible.
func (p *Parser) ParseNoTreeBenchmarkOnly(source []byte) (*Tree, error) {
	if p == nil {
		return nil, ErrNoLanguage
	}
	prev := p.noTreeBenchmarkOnly
	prevCheckpoints := p.noTreeCheckpointBenchmarkOnly
	p.noTreeBenchmarkOnly = true
	p.noTreeCheckpointBenchmarkOnly = false
	defer func() {
		p.noTreeBenchmarkOnly = prev
		p.noTreeCheckpointBenchmarkOnly = prevCheckpoints
	}()
	return p.Parse(source)
}

// ParseNoTreeWithExternalCheckpointsBenchmarkOnly parses source while
// suppressing parent/child tree materialization in reduce actions but keeping
// external-scanner checkpoint capture enabled. It is intended only for parser
// performance attribution; the returned tree is not API-compatible.
func (p *Parser) ParseNoTreeWithExternalCheckpointsBenchmarkOnly(source []byte) (*Tree, error) {
	if p == nil {
		return nil, ErrNoLanguage
	}
	prevNoTree := p.noTreeBenchmarkOnly
	prevCheckpoints := p.noTreeCheckpointBenchmarkOnly
	p.noTreeBenchmarkOnly = true
	p.noTreeCheckpointBenchmarkOnly = true
	defer func() {
		p.noTreeBenchmarkOnly = prevNoTree
		p.noTreeCheckpointBenchmarkOnly = prevCheckpoints
	}()
	return p.Parse(source)
}

// ParseNoResultCompatibilityBenchmarkOnly parses source while suppressing
// language-specific result compatibility rewrites. It is intended only for
// performance attribution; the returned tree is not API-compatible.
func (p *Parser) ParseNoResultCompatibilityBenchmarkOnly(source []byte) (*Tree, error) {
	if p == nil {
		return nil, ErrNoLanguage
	}
	prevNoResult := p.noResultCompatibilityBenchmarkOnly
	prevNoTree := p.noTreeBenchmarkOnly
	prevCheckpoints := p.noTreeCheckpointBenchmarkOnly
	p.noResultCompatibilityBenchmarkOnly = true
	p.noTreeBenchmarkOnly = true
	p.noTreeCheckpointBenchmarkOnly = false
	defer func() {
		p.noResultCompatibilityBenchmarkOnly = prevNoResult
		p.noTreeBenchmarkOnly = prevNoTree
		p.noTreeCheckpointBenchmarkOnly = prevCheckpoints
	}()
	return p.Parse(source)
}

// ParseUTF16 parses UTF-16 source represented as Go UTF-16 code units.
//
// The parser core uses a canonical UTF-8 view internally so existing byte-based
// APIs remain unchanged. The returned tree retains the original UTF-16 source
// and can convert node ranges back to UTF-16 code-unit coordinates.
func (p *Parser) ParseUTF16(source []uint16) (*Tree, error) {
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	tree, err := p.Parse(utf8Source)
	if err != nil {
		return nil, err
	}
	attachUTF16Source(tree, source, sourceMap)
	return tree, nil
}

// ParseUTF16Bytes parses UTF-16 source encoded as bytes with an explicit byte
// order.
func (p *Parser) ParseUTF16Bytes(source []byte, order UTF16ByteOrder) (*Tree, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return p.ParseUTF16(units)
}

// ParseUTF16WithTokenSourceFactory parses UTF-16 source using a token source
// built from the parser's canonical UTF-8 source view.
func (p *Parser) ParseUTF16WithTokenSourceFactory(source []uint16, factory TokenSourceFactory) (*Tree, error) {
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	tree, err := p.ParseWithTokenSourceFactory(utf8Source, factory)
	if err != nil {
		return nil, err
	}
	attachUTF16Source(tree, source, sourceMap)
	return tree, nil
}

// ParseUTF16BytesWithTokenSourceFactory parses UTF-16 bytes using a token
// source built from the parser's canonical UTF-8 source view.
func (p *Parser) ParseUTF16BytesWithTokenSourceFactory(source []byte, order UTF16ByteOrder, factory TokenSourceFactory) (*Tree, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return p.ParseUTF16WithTokenSourceFactory(units, factory)
}

// ParseWithTokenSource parses source using a custom token source.
// This is used for real grammars where the lexer DFA isn't available
// as data tables (e.g., Go grammar using go/scanner as a bridge).
func (p *Parser) ParseWithTokenSource(source []byte, ts TokenSource) (*Tree, error) {
	return p.parseWithTokenSource(source, ts, p.tokenSourceReparseFactory(ts))
}

// ParseWithTokenSourceStrict is like ParseWithTokenSource, but returns
// ErrParseStoppedEarly when parsing returns a partial tree.
func (p *Parser) ParseWithTokenSourceStrict(source []byte, ts TokenSource) (*Tree, error) {
	return strictParseResult(p.ParseWithTokenSource(source, ts))
}

// ParseWithTokenSourceFactory parses source using a freshly built custom token
// source. The factory is also retained for recovery reparses.
func (p *Parser) ParseWithTokenSourceFactory(source []byte, factory TokenSourceFactory) (*Tree, error) {
	if factory == nil {
		return nil, ErrNoTokenSourceFactory
	}
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	ts, err := factory(source)
	if err != nil {
		return nil, err
	}
	return p.parseWithTokenSource(source, ts, factory)
}

// ParseWithTokenSourceFactoryStrict is like ParseWithTokenSourceFactory, but
// returns ErrParseStoppedEarly when parsing returns a partial tree.
func (p *Parser) ParseWithTokenSourceFactoryStrict(source []byte, factory TokenSourceFactory) (*Tree, error) {
	return strictParseResult(p.ParseWithTokenSourceFactory(source, factory))
}

// ParseIncremental re-parses source after edits were applied to oldTree.
// It reuses unchanged subtrees from the old tree for better performance.
// Call oldTree.Edit() for each edit before calling this method.
func (p *Parser) ParseIncremental(source []byte, oldTree *Tree) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, nil
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	return p.parseIncrementalChanged(source, oldTree)
}

func (p *Parser) parseIncrementalChanged(source []byte, oldTree *Tree) (*Tree, error) {
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
	p.fullParseRetryPassesTaken = 0
	if oldTreeDisablesIncrementalReuse(oldTree) {
		if tree, ok := p.tryTokenInvariantReuseForDisabledOldTree(source, oldTree, nil); ok {
			return tree, nil
		}
		return p.Parse(source)
	}
	if err := p.checkDFALexer(); err != nil {
		return nil, err
	}
	prevFactory := p.reparseFactory
	p.reparseFactory = nil
	defer func() {
		p.reparseFactory = prevFactory
	}()
	ts := p.acquireParserDFATokenSource(source)
	defer ts.Close()
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), nil)
	p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	return tree, nil
}

// ParseIncrementalStrict is like ParseIncremental, but returns
// ErrParseStoppedEarly when parsing returns a partial tree.
func (p *Parser) ParseIncrementalStrict(source []byte, oldTree *Tree) (*Tree, error) {
	return strictParseResult(p.ParseIncremental(source, oldTree))
}

// ParseIncrementalUTF16 re-parses UTF-16 source after edits were applied to
// oldTree. oldTree should have been produced by ParseUTF16, and UTF-16 edits
// can be recorded with Tree.EditUTF16.
func (p *Parser) ParseIncrementalUTF16(source []uint16, oldTree *Tree) (*Tree, error) {
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	tree, err := p.ParseIncremental(utf8Source, oldTree)
	if err != nil {
		return nil, err
	}
	attachUTF16Source(tree, source, sourceMap)
	return tree, nil
}

// ParseIncrementalUTF16Bytes re-parses UTF-16 bytes after edits were applied
// to oldTree.
func (p *Parser) ParseIncrementalUTF16Bytes(source []byte, oldTree *Tree, order UTF16ByteOrder) (*Tree, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return p.ParseIncrementalUTF16(units, oldTree)
}

// ParseIncrementalUTF16WithTokenSourceFactory re-parses UTF-16 source using a
// token source built from the parser's canonical UTF-8 source view.
func (p *Parser) ParseIncrementalUTF16WithTokenSourceFactory(source []uint16, oldTree *Tree, factory TokenSourceFactory) (*Tree, error) {
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	tree, err := p.ParseIncrementalWithTokenSourceFactory(utf8Source, oldTree, factory)
	if err != nil {
		return nil, err
	}
	attachUTF16Source(tree, source, sourceMap)
	return tree, nil
}

// ParseIncrementalUTF16BytesWithTokenSourceFactory re-parses UTF-16 bytes using
// a token source built from the parser's canonical UTF-8 source view.
func (p *Parser) ParseIncrementalUTF16BytesWithTokenSourceFactory(source []byte, oldTree *Tree, order UTF16ByteOrder, factory TokenSourceFactory) (*Tree, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return p.ParseIncrementalUTF16WithTokenSourceFactory(units, oldTree, factory)
}

// ParseIncrementalWithTokenSource is like ParseIncremental but uses a custom
// token source.
func (p *Parser) ParseIncrementalWithTokenSource(source []byte, oldTree *Tree, ts TokenSource) (*Tree, error) {
	return p.parseIncrementalWithTokenSource(source, oldTree, ts, p.tokenSourceReparseFactory(ts))
}

// ParseIncrementalWithTokenSourceStrict is like ParseIncrementalWithTokenSource,
// but returns ErrParseStoppedEarly when parsing returns a partial tree.
func (p *Parser) ParseIncrementalWithTokenSourceStrict(source []byte, oldTree *Tree, ts TokenSource) (*Tree, error) {
	return strictParseResult(p.ParseIncrementalWithTokenSource(source, oldTree, ts))
}

// ParseIncrementalWithTokenSourceFactory is like ParseWithTokenSourceFactory
// for an edited old tree.
func (p *Parser) ParseIncrementalWithTokenSourceFactory(source []byte, oldTree *Tree, factory TokenSourceFactory) (*Tree, error) {
	if factory == nil {
		return nil, ErrNoTokenSourceFactory
	}
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	ts, err := factory(source)
	if err != nil {
		return nil, err
	}
	return p.parseIncrementalWithTokenSource(source, oldTree, ts, factory)
}

// ParseIncrementalWithTokenSourceFactoryStrict is like
// ParseIncrementalWithTokenSourceFactory, but returns ErrParseStoppedEarly when
// parsing returns a partial tree.
func (p *Parser) ParseIncrementalWithTokenSourceFactoryStrict(source []byte, oldTree *Tree, factory TokenSourceFactory) (*Tree, error) {
	return strictParseResult(p.ParseIncrementalWithTokenSourceFactory(source, oldTree, factory))
}

func attachUTF16Source(tree *Tree, source []uint16, sourceMap *utf16SourceMap) {
	if tree == nil {
		return
	}
	tree.sourceEncoding = InputEncodingUTF16
	tree.sourceUTF16 = source
	tree.utf16Map = sourceMap
}

// ParseIncrementalProfiled is like ParseIncremental and also returns runtime
// attribution for incremental reuse work vs parse/rebuild work.
func (p *Parser) ParseIncrementalProfiled(source []byte, oldTree *Tree) (*Tree, IncrementalParseProfile, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, IncrementalParseProfile{}, err
	}
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, IncrementalParseProfile{}, nil
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	return p.parseIncrementalChangedProfiled(source, oldTree)
}

func (p *Parser) parseIncrementalChangedProfiled(source []byte, oldTree *Tree) (*Tree, IncrementalParseProfile, error) {
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
	p.fullParseRetryPassesTaken = 0
	if oldTreeDisablesIncrementalReuse(oldTree) {
		timing := &incrementalParseTiming{}
		if tree, ok := p.tryTokenInvariantReuseForDisabledOldTree(source, oldTree, timing); ok {
			return tree, timing.toProfile(), nil
		}
		start := time.Now()
		tree, err := p.Parse(source)
		return tree, profileFreshParseFallback(start, tree, forestIncrementalReuseUnsupportedReason), err
	}
	if err := p.checkDFALexer(); err != nil {
		return nil, IncrementalParseProfile{}, err
	}
	prevFactory := p.reparseFactory
	p.reparseFactory = nil
	defer func() {
		p.reparseFactory = prevFactory
	}()
	ts := p.acquireParserDFATokenSource(source)
	defer ts.Close()
	timing := &incrementalParseTiming{}
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), timing)
	p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	return tree, timing.toProfile(), nil
}

// ParseIncrementalWithTokenSourceProfiled is like ParseIncrementalWithTokenSource
// and also returns runtime attribution for incremental reuse work vs parse/rebuild work.
func (p *Parser) ParseIncrementalWithTokenSourceProfiled(source []byte, oldTree *Tree, ts TokenSource) (*Tree, IncrementalParseProfile, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, IncrementalParseProfile{}, err
	}
	endBudget := p.beginParseOperationBudget()
	defer endBudget()
	releaseTS := manageTokenSourceLifetime(ts)
	defer releaseTS()
	if canReuseUnchangedTree(source, oldTree, p.language) {
		return oldTree, IncrementalParseProfile{}, nil
	}
	return p.parseIncrementalWithTokenSourceChangedProfiled(source, oldTree, ts)
}

func (p *Parser) parseIncrementalWithTokenSourceChangedProfiled(source []byte, oldTree *Tree, ts TokenSource) (*Tree, IncrementalParseProfile, error) {
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
	p.fullParseRetryPassesTaken = 0
	prevFactory := p.reparseFactory
	p.reparseFactory = p.tokenSourceReparseFactory(ts)
	defer func() {
		p.reparseFactory = prevFactory
	}()
	timing := &incrementalParseTiming{}
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), timing)
	// Never hand oldTree to the retry helper: it releases the losing tree,
	// and oldTree is owned by the caller.
	if initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth); tree != oldTree && shouldRetryIncrementalParseAsFull(tree, len(source), initialMaxStacks) {
		tree = p.retryIncrementalParseAsFullWithTokenSource(source, ts, initialMaxStacks, tree, timing)
	}
	p.normalizeReturnedIncrementalTree(tree, oldTree, source)
	return tree, timing.toProfile(), nil
}

// ParseWith parses source using option-based configuration.
func (p *Parser) ParseWith(source []byte, opts ...ParseOption) (ParseResult, error) {
	var cfg parseConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	if cfg.profiling {
		if cfg.oldTree != nil {
			if cfg.tokenSource != nil {
				tree, profile, err := p.ParseIncrementalWithTokenSourceProfiled(source, cfg.oldTree, cfg.tokenSource)
				return ParseResult{Tree: tree, Profile: profile, ProfileAvailable: true}, err
			}
			tree, profile, err := p.ParseIncrementalProfiled(source, cfg.oldTree)
			return ParseResult{Tree: tree, Profile: profile, ProfileAvailable: true}, err
		}
		// Full parses do not currently expose attribution data.
		if cfg.tokenSource != nil {
			tree, err := p.ParseWithTokenSource(source, cfg.tokenSource)
			return ParseResult{Tree: tree, ProfileAvailable: false}, err
		}
		tree, err := p.Parse(source)
		return ParseResult{Tree: tree, ProfileAvailable: false}, err
	}

	if cfg.oldTree != nil {
		if cfg.tokenSource != nil {
			tree, err := p.ParseIncrementalWithTokenSource(source, cfg.oldTree, cfg.tokenSource)
			return ParseResult{Tree: tree, ProfileAvailable: false}, err
		}
		tree, err := p.ParseIncremental(source, cfg.oldTree)
		return ParseResult{Tree: tree, ProfileAvailable: false}, err
	}

	if cfg.tokenSource != nil {
		tree, err := p.ParseWithTokenSource(source, cfg.tokenSource)
		return ParseResult{Tree: tree, ProfileAvailable: false}, err
	}
	tree, err := p.Parse(source)
	return ParseResult{Tree: tree, ProfileAvailable: false}, err
}

// ParseWithStrict is like ParseWith, but returns ErrParseStoppedEarly when
// parsing returns a partial tree. The ParseResult still carries that tree.
func (p *Parser) ParseWithStrict(source []byte, opts ...ParseOption) (ParseResult, error) {
	result, err := p.ParseWith(source, opts...)
	if err != nil {
		return result, err
	}
	if stoppedErr := parseStoppedEarlyError(result.Tree); stoppedErr != nil {
		return result, stoppedErr
	}
	return result, nil
}

// ErrNoLanguage is returned when a Parser has no language configured.
var ErrNoLanguage = errors.New("parser has no language configured")

// ErrParseStoppedEarly is matched by ParseStoppedEarlyError when a strict parse
// returns a partial tree.
var ErrParseStoppedEarly = errors.New("parse stopped before accepting input")

// ErrNoTokenSourceFactory is returned when a factory-based parse is called
// without a token source factory.
var ErrNoTokenSourceFactory = errors.New("parser has no token source factory")

// ErrNoTokenSource is returned when a token-source parse is called without a
// token source.
var ErrNoTokenSource = errors.New("parser has no token source")

// checkLanguageCompatible returns an error if the parser's language is nil or
// incompatible with the runtime.
func (p *Parser) checkLanguageCompatible() error {
	if p.language == nil {
		return ErrNoLanguage
	}
	if !p.language.CompatibleWithRuntime() {
		return fmt.Errorf("language version %d incompatible with parser", p.language.LanguageVersion)
	}
	return nil
}

// checkDFALexer returns an error if the parser's language has no DFA lexer tables.
func (p *Parser) checkDFALexer() error {
	if p.language == nil || len(p.language.LexStates) == 0 {
		return fmt.Errorf("no DFA lexer available for language (use ParseWithTokenSource instead)")
	}
	return nil
}
