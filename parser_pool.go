package gotreesitter

import "sync"

// ParserPoolOption configures a ParserPool.
type ParserPoolOption func(*parserPoolConfig)

type parserPoolConfig struct {
	logger           ParserLogger
	timeoutMicros    uint64
	included         []Range
	glrTrace         bool
	ambiguityProfile *AmbiguityProfile
}

// WithParserPoolLogger sets the logger applied to pooled parser instances.
func WithParserPoolLogger(logger ParserLogger) ParserPoolOption {
	return func(cfg *parserPoolConfig) {
		cfg.logger = logger
	}
}

// WithParserPoolTimeoutMicros sets the parse timeout for pooled parsers.
func WithParserPoolTimeoutMicros(timeoutMicros uint64) ParserPoolOption {
	return func(cfg *parserPoolConfig) {
		cfg.timeoutMicros = timeoutMicros
	}
}

// WithParserPoolIncludedRanges sets default include ranges for pooled parsers.
func WithParserPoolIncludedRanges(ranges []Range) ParserPoolOption {
	return func(cfg *parserPoolConfig) {
		cfg.included = normalizeIncludedRanges(ranges)
	}
}

// WithParserPoolGLRTrace toggles GLR trace logs on pooled parser instances.
func WithParserPoolGLRTrace(enabled bool) ParserPoolOption {
	return func(cfg *parserPoolConfig) {
		cfg.glrTrace = enabled
	}
}

// WithParserPoolAmbiguityProfile installs an optional diagnostic ambiguity
// profile on checked-out parsers.
func WithParserPoolAmbiguityProfile(profile *AmbiguityProfile) ParserPoolOption {
	return func(cfg *parserPoolConfig) {
		cfg.ambiguityProfile = profile
	}
}

// ParserPool provides concurrency-safe parsing by reusing Parser instances.
//
// ParserPool is safe for concurrent use. Each call checks out one parser from
// an internal sync.Pool, applies configured defaults, runs the parse, and
// returns the parser to the pool.
//
// Mutable parser state (logger, timeout, cancellation flag, included ranges,
// GLR trace) is reset on checkout so request-local state cannot bleed across
// callers.
type ParserPool struct {
	language         *Language
	logger           ParserLogger
	timeoutMicros    uint64
	included         []Range
	glrTrace         bool
	ambiguityProfile *AmbiguityProfile
	pool             sync.Pool
}

// NewParserPool creates a concurrency-safe parser pool for lang.
func NewParserPool(lang *Language, opts ...ParserPoolOption) *ParserPool {
	cfg := parserPoolConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	pp := &ParserPool{
		language:         lang,
		logger:           cfg.logger,
		timeoutMicros:    cfg.timeoutMicros,
		included:         append([]Range(nil), cfg.included...),
		glrTrace:         cfg.glrTrace,
		ambiguityProfile: cfg.ambiguityProfile,
	}
	pp.pool.New = func() any {
		p := NewParser(pp.language)
		pp.applyDefaults(p)
		return p
	}
	return pp
}

// Language returns the pool's configured language.
func (pp *ParserPool) Language() *Language {
	if pp == nil {
		return nil
	}
	return pp.language
}

func (pp *ParserPool) applyDefaults(p *Parser) {
	if p == nil {
		return
	}
	p.SetLogger(pp.logger)
	p.SetTimeoutMicros(pp.timeoutMicros)
	p.SetCancellationFlag(nil)
	p.SetIncludedRanges(pp.included)
	p.SetGLRTrace(pp.glrTrace)
	p.SetAmbiguityProfile(pp.ambiguityProfile)
	p.SetParsePhaseTiming(false)
	p.noTreeBenchmarkOnly = false
	p.noTreeCheckpointBenchmarkOnly = false
	p.compactNoTreeShiftLeaves = false
	p.compactFullShiftLeaves = false
	p.pendingFullParents = false
	p.skipInvisibleFullLeafCheckpoints = false
	p.noResultCompatibilityBenchmarkOnly = false
	// Scrub the Phase-3 admission switch state so a pooled parser follows the
	// process-wide default again and never carries a stale override, suppression
	// depth, or cached compact runner across checkouts.
	p.admissionCandidateRoute = admissionRouteFollowDefault
	p.admissionRouteSuppressed = 0
	p.admissionCandidateRunner = nil
}

func (pp *ParserPool) checkout() *Parser {
	if pp == nil {
		return nil
	}
	v := pp.pool.Get()
	p, _ := v.(*Parser)
	if p == nil || p.Language() != pp.language {
		p = NewParser(pp.language)
	}
	pp.applyDefaults(p)
	return p
}

func (pp *ParserPool) release(p *Parser) {
	if pp == nil || p == nil {
		return
	}
	// Keep parser state scrubbed before re-adding to the shared pool.
	pp.applyDefaults(p)
	// Release *Node refs so the last-used arena can be GC'd.
	p.reuseCursor.releaseNodeRefs()
	p.reuseScratch.releaseNodeRefs()
	// Return the recovery sub-parser so its reuseCursor *Node refs (and the
	// arena they pin) are released while preserving its language-level caches.
	p.clearRecoveryParser()
	p.releaseCompatibilityBorrowedArenas()
	pp.pool.Put(p)
}

// Parse delegates to a pooled Parser.Parse call.
func (pp *ParserPool) Parse(source []byte) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.Parse(source)
}

// ParseStrict delegates to a pooled Parser.ParseStrict call.
func (pp *ParserPool) ParseStrict(source []byte) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseStrict(source)
}

// ParseNoTreeBenchmarkOnly delegates to Parser.ParseNoTreeBenchmarkOnly.
// It is intended only for parser-loop performance experiments; the returned
// tree is not API-compatible.
func (pp *ParserPool) ParseNoTreeBenchmarkOnly(source []byte) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseNoTreeBenchmarkOnly(source)
}

// ParseNoTreeWithExternalCheckpointsBenchmarkOnly delegates to
// Parser.ParseNoTreeWithExternalCheckpointsBenchmarkOnly. It is intended only
// for parser performance attribution; the returned tree is not API-compatible.
func (pp *ParserPool) ParseNoTreeWithExternalCheckpointsBenchmarkOnly(source []byte) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseNoTreeWithExternalCheckpointsBenchmarkOnly(source)
}

// ParseNoResultCompatibilityBenchmarkOnly delegates to
// Parser.ParseNoResultCompatibilityBenchmarkOnly. It is intended only for
// performance attribution. The result tree is materialized, but other
// diagnostic materialization strategies may still key off this mode, and the
// returned tree is not API-compatible.
func (pp *ParserPool) ParseNoResultCompatibilityBenchmarkOnly(source []byte) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseNoResultCompatibilityBenchmarkOnly(source)
}

// ParseUTF16 delegates to a pooled Parser.ParseUTF16 call.
func (pp *ParserPool) ParseUTF16(source []uint16) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseUTF16(source)
}

// ParseUTF16Bytes delegates to a pooled Parser.ParseUTF16Bytes call.
func (pp *ParserPool) ParseUTF16Bytes(source []byte, order UTF16ByteOrder) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseUTF16Bytes(source, order)
}

// ParseWithTokenSource delegates to a pooled Parser.ParseWithTokenSource call.
func (pp *ParserPool) ParseWithTokenSource(source []byte, ts TokenSource) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWithTokenSource(source, ts)
}

// ParseWithTokenSourceStrict delegates to a pooled
// Parser.ParseWithTokenSourceStrict call.
func (pp *ParserPool) ParseWithTokenSourceStrict(source []byte, ts TokenSource) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWithTokenSourceStrict(source, ts)
}

// ParseWithTokenSourceFactory delegates to a pooled
// Parser.ParseWithTokenSourceFactory call.
func (pp *ParserPool) ParseWithTokenSourceFactory(source []byte, factory TokenSourceFactory) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWithTokenSourceFactory(source, factory)
}

// ParseWithTokenSourceFactoryStrict delegates to a pooled
// Parser.ParseWithTokenSourceFactoryStrict call.
func (pp *ParserPool) ParseWithTokenSourceFactoryStrict(source []byte, factory TokenSourceFactory) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWithTokenSourceFactoryStrict(source, factory)
}

// ParseUTF16WithTokenSourceFactory delegates to a pooled
// Parser.ParseUTF16WithTokenSourceFactory call.
func (pp *ParserPool) ParseUTF16WithTokenSourceFactory(source []uint16, factory TokenSourceFactory) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseUTF16WithTokenSourceFactory(source, factory)
}

// ParseUTF16BytesWithTokenSourceFactory delegates to a pooled
// Parser.ParseUTF16BytesWithTokenSourceFactory call.
func (pp *ParserPool) ParseUTF16BytesWithTokenSourceFactory(source []byte, order UTF16ByteOrder, factory TokenSourceFactory) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseUTF16BytesWithTokenSourceFactory(source, order, factory)
}

// ParseIncrementalUTF16 delegates to a pooled Parser.ParseIncrementalUTF16 call.
func (pp *ParserPool) ParseIncrementalUTF16(source []uint16, oldTree *Tree) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseIncrementalUTF16(source, oldTree)
}

// ParseIncrementalUTF16Bytes delegates to a pooled
// Parser.ParseIncrementalUTF16Bytes call.
func (pp *ParserPool) ParseIncrementalUTF16Bytes(source []byte, oldTree *Tree, order UTF16ByteOrder) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseIncrementalUTF16Bytes(source, oldTree, order)
}

// ParseIncrementalUTF16WithTokenSourceFactory delegates to a pooled
// Parser.ParseIncrementalUTF16WithTokenSourceFactory call.
func (pp *ParserPool) ParseIncrementalUTF16WithTokenSourceFactory(source []uint16, oldTree *Tree, factory TokenSourceFactory) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseIncrementalUTF16WithTokenSourceFactory(source, oldTree, factory)
}

// ParseIncrementalUTF16BytesWithTokenSourceFactory delegates to a pooled
// Parser.ParseIncrementalUTF16BytesWithTokenSourceFactory call.
func (pp *ParserPool) ParseIncrementalUTF16BytesWithTokenSourceFactory(source []byte, oldTree *Tree, order UTF16ByteOrder, factory TokenSourceFactory) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseIncrementalUTF16BytesWithTokenSourceFactory(source, oldTree, order, factory)
}

// ParseWith delegates to a pooled Parser.ParseWith call.
func (pp *ParserPool) ParseWith(source []byte, opts ...ParseOption) (ParseResult, error) {
	p := pp.checkout()
	if p == nil {
		return ParseResult{}, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWith(source, opts...)
}

// ParseWithStrict delegates to a pooled Parser.ParseWithStrict call.
func (pp *ParserPool) ParseWithStrict(source []byte, opts ...ParseOption) (ParseResult, error) {
	p := pp.checkout()
	if p == nil {
		return ParseResult{}, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWithStrict(source, opts...)
}
