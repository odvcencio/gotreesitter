package gotreesitter

import "sync"

// ParserPoolOption configures a ParserPool.
type ParserPoolOption func(*parserPoolConfig)

type parserPoolConfig struct {
	logger        ParserLogger
	timeoutMicros uint64
	included      []Range
	glrTrace      bool
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
	language      *Language
	logger        ParserLogger
	timeoutMicros uint64
	included      []Range
	glrTrace      bool
	pool          sync.Pool
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
		language:      lang,
		logger:        cfg.logger,
		timeoutMicros: cfg.timeoutMicros,
		included:      append([]Range(nil), cfg.included...),
		glrTrace:      cfg.glrTrace,
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
	// Drop the recovery sub-parser so its reuseCursor *Node refs (and the
	// arena they pin) are released.  It is cheap to recreate on next use.
	p.recoveryParser = nil
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

// ParseWithTokenSource delegates to a pooled Parser.ParseWithTokenSource call.
func (pp *ParserPool) ParseWithTokenSource(source []byte, ts TokenSource) (*Tree, error) {
	p := pp.checkout()
	if p == nil {
		return nil, ErrNoLanguage
	}
	defer pp.release(p)
	return p.ParseWithTokenSource(source, ts)
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
