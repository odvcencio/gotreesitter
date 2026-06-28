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
	case "go":
		normalizeGoCompatibility(root, source, lang)
		normalizeRootEOFNewlineSpan(root, source, lang)
	case "scala":
		normalizeScalaTemplateBodyObjectFragments(root, source, lang)
		normalizeScalaRecoveredObjectTemplateBodies(root, source, lang)
		normalizeScalaDefinitionFields(root, source, lang)
		normalizeScalaTemplateBodyFunctionAnnotations(root, source, lang)
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
	if oldTree == nil {
		return true
	}
	return rawRootOrNil(tree) != rawRootOrNil(oldTree)
}

func normalizeReturnedIncrementalTree(tree, oldTree *Tree, source []byte, lang *Language) {
	if !shouldNormalizeIncrementalReturnedTree(tree, oldTree) {
		return
	}
	normalizeReturnedTree(rawRootOrNil(tree), source, lang)
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
	p.reparseFactory = p.dfaReparseFactory()
	defer func() {
		p.reparseFactory = prevFactory
	}()
	lexer := NewLexer(p.language.LexStates, source)
	ts := acquireDFATokenSource(lexer, p.language, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState)
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

func (p *Parser) normalizeReturnedTree(root *Node, source []byte) {
	if p != nil && p.noResultCompatibilityBenchmarkOnly {
		return
	}
	if reason := p.activeParseStopReason(); parseStopReasonIsActive(reason) {
		return
	}
	normalizeReturnedTree(root, source, p.language)
}

func (p *Parser) normalizeReturnedParseTree(tree *Tree, source []byte) {
	if tree == nil || tree.ParseStoppedEarly() {
		return
	}
	p.normalizeReturnedTree(rawRootOrNil(tree), source)
	p.applyActiveStopReasonToTree(tree)
}

func (p *Parser) normalizeReturnedIncrementalTree(tree, oldTree *Tree, source []byte) {
	if !shouldNormalizeIncrementalReturnedTree(tree, oldTree) {
		return
	}
	p.normalizeReturnedParseTree(tree, source)
}

func (p *Parser) dfaReparseFactory() TokenSourceFactory {
	if p == nil || p.language == nil || len(p.language.LexStates) == 0 {
		return nil
	}
	return func(source []byte) (TokenSource, error) {
		lexer := NewLexer(p.language.LexStates, source)
		return newDFATokenSourceDirect(lexer, p.language, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState), nil
	}
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
	var tree *Tree
	var err error
	if p.reparseFactory != nil {
		ts, err := p.reparseFactory(source)
		if err != nil {
			return nil, err
		}
		tree, err = parser.ParseWithTokenSource(source, ts)
	} else {
		tree, err = parser.Parse(source)
	}
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
	parser := acquireSnippetParser(lang)
	if parser == nil {
		return nil, ErrNoLanguage
	}
	defer releaseSnippetParser(parser)
	if len(timeoutMicros) > 0 && timeoutMicros[0] > 0 {
		parser.timeoutMicros = timeoutMicros[0]
	}
	return parser.Parse(source)
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
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
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
	p.normalizeReturnedParseTree(tree, source)
	return tree, nil
}

func (p *Parser) parseIncrementalWithTokenSource(source []byte, oldTree *Tree, ts TokenSource, reparseFactory TokenSourceFactory) (*Tree, error) {
	if err := p.checkLanguageCompatible(); err != nil {
		return nil, err
	}
	if ts == nil {
		return nil, ErrNoTokenSource
	}
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
	prevFactory := p.reparseFactory
	p.reparseFactory = reparseFactory
	defer func() {
		p.reparseFactory = prevFactory
	}()
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), nil)
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
	// GSS-forest fast path for languages whose production GLR parse blows up on
	// deep stack-equivalence (e.g. bash). Returns nil to fall back to the
	// production parser on any failure, error, or truncation. Off unless
	// GOT_GLR_FOREST is set; see tryForestFastPath.
	if tree := p.tryForestFastPath(source); tree != nil {
		return tree, nil
	}
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
	p.releaseCompatibilityBorrowedArenas()
	p.clearRecoveryParser()
	defer p.clearRecoveryParser()
	prevFactory := p.reparseFactory
	p.reparseFactory = p.dfaReparseFactory()
	defer func() {
		p.reparseFactory = prevFactory
	}()
	lexer := NewLexer(p.language.LexStates, source)
	ts := acquireDFATokenSource(lexer, p.language, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState)
	if p.noTreeBenchmarkOnly && !p.noTreeCheckpointBenchmarkOnly {
		ts.usesExternalCheckpoints = false
	}
	defer ts.Close()
	deterministicExternalConflicts := fullParseUsesDeterministicExternalConflicts(p.language)
	initialMaxStacks := fullParseInitialMaxStacks(p.language, p.maxConflictWidth)
	tree := p.parseInternal(source, p.wrapIncludedRanges(ts), nil, nil, arenaClassFull, nil, initialMaxStacks, 0, 0, deterministicExternalConflicts)
	if !p.noTreeBenchmarkOnly {
		if tree != nil && !tree.ParseStoppedEarly() && !parseStopReasonIsActive(p.activeParseStopReason()) {
			tree = p.retryFullParseWithDFA(source, initialMaxStacks, deterministicExternalConflicts, tree)
			if tree != nil && !tree.ParseStoppedEarly() && !parseStopReasonIsActive(p.activeParseStopReason()) && shouldRepeatExternalScannerFullParse(p.language, tree) {
				tree = p.retryFullParseWithDFA(source, initialMaxStacks, deterministicExternalConflicts, tree)
			}
		}
		p.normalizeReturnedParseTree(tree, source)
	}
	return tree, nil
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
	prev := p.noResultCompatibilityBenchmarkOnly
	p.noResultCompatibilityBenchmarkOnly = true
	defer func() {
		p.noResultCompatibilityBenchmarkOnly = prev
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
	return p.parseIncrementalChanged(source, oldTree)
}

func (p *Parser) parseIncrementalChanged(source []byte, oldTree *Tree) (*Tree, error) {
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
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
	p.reparseFactory = p.dfaReparseFactory()
	defer func() {
		p.reparseFactory = prevFactory
	}()
	lexer := NewLexer(p.language.LexStates, source)
	ts := acquireDFATokenSource(lexer, p.language, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState)
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
	return p.parseIncrementalChangedProfiled(source, oldTree)
}

func (p *Parser) parseIncrementalChangedProfiled(source []byte, oldTree *Tree) (*Tree, IncrementalParseProfile, error) {
	endParseBudget := p.enterParseBudget()
	defer endParseBudget()
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
	p.reparseFactory = p.dfaReparseFactory()
	defer func() {
		p.reparseFactory = prevFactory
	}()
	lexer := NewLexer(p.language.LexStates, source)
	ts := acquireDFATokenSource(lexer, p.language, p.lookupActionIndex, p.hasKeywordState, p.externalValidByState, p.externalValidMaskByState)
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
	prevFactory := p.reparseFactory
	p.reparseFactory = p.tokenSourceReparseFactory(ts)
	defer func() {
		p.reparseFactory = prevFactory
	}()
	timing := &incrementalParseTiming{}
	tree := p.parseIncrementalInternal(source, oldTree, p.wrapIncludedRanges(ts), timing)
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
