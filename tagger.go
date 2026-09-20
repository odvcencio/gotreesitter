package gotreesitter

// Tag represents a tagged symbol in source code, extracted by a Tagger.
// Kind follows tree-sitter convention: "definition.function", "reference.call", etc.
// Name is the captured symbol text (e.g., the function name).
type Tag struct {
	Kind      string // e.g. "definition.function", "reference.call"
	Name      string // the captured symbol text
	Range     Range  // full span of the tagged node
	NameRange Range  // span of the @name capture
}

// UTF16Tag represents a tagged symbol with ranges in UTF-16 code-unit
// coordinates.
type UTF16Tag struct {
	Kind      string
	Name      string
	Range     UTF16Range
	NameRange UTF16Range
}

// Tagger extracts symbol definitions and references from source code using
// tree-sitter tags queries. It is the tagging counterpart to Highlighter.
//
// Tags queries use a convention where captures follow the pattern:
//   - @name captures the symbol name (e.g., function identifier)
//   - @definition.X or @reference.X captures the kind
//
// Example query:
//
//	(function_declaration name: (identifier) @name) @definition.function
//	(call_expression function: (identifier) @name) @reference.call
type Tagger struct {
	parser             *Parser
	query              *Query
	lang               *Language
	tokenSourceFactory func(source []byte) TokenSource
	// matchesBuf is reused across Tag calls to eliminate per-call slice allocation.
	matchesBuf []QueryMatch
}

// TaggerOption configures a Tagger.
type TaggerOption func(*Tagger)

// WithTaggerTokenSourceFactory sets a factory function that creates a TokenSource
// for each Tag call.
func WithTaggerTokenSourceFactory(factory func(source []byte) TokenSource) TaggerOption {
	return func(tg *Tagger) {
		tg.tokenSourceFactory = factory
	}
}

// WithTaggerTimeoutMicros bounds every full and incremental parse performed
// by the tagger. A value of zero disables timeout checks.
func WithTaggerTimeoutMicros(timeoutMicros uint64) TaggerOption {
	return func(tagger *Tagger) {
		tagger.parser.SetTimeoutMicros(timeoutMicros)
	}
}

// NewTagger creates a Tagger for the given language and tags query.
func NewTagger(lang *Language, tagsQuery string, opts ...TaggerOption) (*Tagger, error) {
	q, err := NewQuery(tagsQuery, lang)
	if err != nil {
		return nil, err
	}

	tg := &Tagger{
		parser: NewParser(lang),
		query:  q,
		lang:   lang,
	}
	for _, opt := range opts {
		opt(tg)
	}
	return tg, nil
}

// Tag parses source and returns all tags.
func (tg *Tagger) Tag(source []byte) []Tag {
	if len(source) == 0 {
		return nil
	}

	tree := tg.parse(source, nil)
	if tree.RootNode() == nil {
		return nil
	}
	defer tree.Release()

	return tg.tagTree(tree)
}

// TagStrict is like Tag, but rejects a partial tree when parsing stops before
// it accepts all input. It releases the partial tree and returns an error that
// wraps ErrParseStoppedEarly.
func (tg *Tagger) TagStrict(source []byte) ([]Tag, error) {
	if len(source) == 0 {
		return nil, nil
	}

	tree, err := tg.parseStrict(source, nil)
	if err != nil {
		if tree != nil {
			tree.Release()
		}
		return nil, err
	}
	if tree == nil {
		return nil, nil
	}
	defer tree.Release()
	if tree.RootNode() == nil {
		return nil, nil
	}

	return tg.tagTree(tree), nil
}

// TagUTF16 parses UTF-16 source and returns all tags with UTF-16 ranges.
func (tg *Tagger) TagUTF16(source []uint16) []UTF16Tag {
	if len(source) == 0 {
		return nil
	}

	tree := tg.parseUTF16(source, nil)
	if tree.RootNode() == nil {
		return nil
	}
	defer tree.Release()

	return tg.tagTreeUTF16(tree)
}

// TagUTF16Bytes is like TagUTF16 for endian-specific UTF-16 bytes.
func (tg *Tagger) TagUTF16Bytes(source []byte, order UTF16ByteOrder) ([]UTF16Tag, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return tg.TagUTF16(units), nil
}

// TagTree extracts tags from an already-parsed tree.
func (tg *Tagger) TagTree(tree *Tree) []Tag {
	if tree == nil || tree.RootNode() == nil {
		return nil
	}
	return tg.tagTree(tree)
}

// TagTreeUTF16 extracts tags from an already-parsed UTF-16 tree.
func (tg *Tagger) TagTreeUTF16(tree *Tree) []UTF16Tag {
	if tree == nil || tree.RootNode() == nil {
		return nil
	}
	return tg.tagTreeUTF16(tree)
}

// TagIncremental re-tags source after edits to oldTree.
// Returns the tags and the new tree for subsequent incremental calls.
func (tg *Tagger) TagIncremental(source []byte, oldTree *Tree) ([]Tag, *Tree) {
	if len(source) == 0 {
		return nil, NewTree(nil, source, tg.lang)
	}

	tree := tg.parse(source, oldTree)
	if tree.RootNode() == nil {
		return nil, tree
	}

	return tg.tagTree(tree), tree
}

// TagIncrementalStrict is like TagIncremental, but reports
// ErrParseStoppedEarly and skips query execution when parsing is stopped by a
// timeout, cancellation, token-source EOF, or parser safety limit. The partial
// tree is returned so callers can release it or use it for diagnostics.
func (tg *Tagger) TagIncrementalStrict(source []byte, oldTree *Tree) ([]Tag, *Tree, error) {
	if len(source) == 0 {
		return nil, NewTree(nil, source, tg.lang), nil
	}

	tree := tg.parse(source, oldTree)
	if err := parseStoppedEarlyError(tree); err != nil {
		return nil, tree, err
	}
	if tree.RootNode() == nil {
		return nil, tree, nil
	}

	return tg.tagTree(tree), tree, nil
}

// TagIncrementalUTF16 re-tags UTF-16 source after edits to oldTree. Call
// oldTree.EditUTF16 before calling this.
func (tg *Tagger) TagIncrementalUTF16(source []uint16, oldTree *Tree) ([]UTF16Tag, *Tree) {
	if len(source) == 0 {
		tree := dispatchParseUTF16(tg.parser, source, nil, tg.tokenSourceFactory, tg.lang)
		return nil, tree
	}

	tree := tg.parseUTF16(source, oldTree)
	if tree.RootNode() == nil {
		return nil, tree
	}

	return tg.tagTreeUTF16(tree), tree
}

// TagIncrementalUTF16Bytes is like TagIncrementalUTF16 for endian-specific
// UTF-16 bytes.
func (tg *Tagger) TagIncrementalUTF16Bytes(source []byte, oldTree *Tree, order UTF16ByteOrder) ([]UTF16Tag, *Tree, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, nil, err
	}
	tags, tree := tg.TagIncrementalUTF16(units, oldTree)
	return tags, tree, nil
}

func (tg *Tagger) parse(source []byte, oldTree *Tree) *Tree {
	return dispatchParse(tg.parser, source, oldTree, tg.tokenSourceFactory, tg.lang)
}

func (tg *Tagger) parseStrict(source []byte, oldTree *Tree) (*Tree, error) {
	return dispatchParseStrict(tg.parser, source, oldTree, tg.tokenSourceFactory)
}

func (tg *Tagger) parseUTF16(source []uint16, oldTree *Tree) *Tree {
	return dispatchParseUTF16(tg.parser, source, oldTree, tg.tokenSourceFactory, tg.lang)
}

func (tg *Tagger) tagTree(tree *Tree) []Tag {
	// Reuse the matches buffer across calls to eliminate the per-call
	// []QueryMatch allocation. ExecuteInto appends into the pre-allocated slice.
	tg.matchesBuf = tg.query.ExecuteInto(tree, tg.matchesBuf[:0])
	if len(tg.matchesBuf) == 0 {
		return nil
	}

	// Pre-size to match count. Tags queries emit at most one tag per match,
	// so this is the tightest possible upper bound without a pre-pass.
	tags := make([]Tag, 0, len(tg.matchesBuf))
	for _, m := range tg.matchesBuf {
		tag := tg.extractTag(m, tree.Source())
		if tag.Kind != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func (tg *Tagger) tagTreeUTF16(tree *Tree) []UTF16Tag {
	tags := tg.tagTree(tree)
	if len(tags) == 0 {
		return nil
	}
	out := make([]UTF16Tag, 0, len(tags))
	for _, tag := range tags {
		tagRange, ok := tree.UTF16RangeForRange(tag.Range)
		if !ok {
			continue
		}
		nameRange, ok := tree.UTF16RangeForRange(tag.NameRange)
		if !ok {
			continue
		}
		out = append(out, UTF16Tag{
			Kind:      tag.Kind,
			Name:      tag.Name,
			Range:     tagRange,
			NameRange: nameRange,
		})
	}
	return out
}

func (tg *Tagger) extractTag(m QueryMatch, source []byte) Tag {
	var tag Tag
	for _, c := range m.Captures {
		switch {
		case c.Name == "name":
			tag.Name = c.Text(source)
			tag.NameRange = c.Range()
		case len(c.Name) > 11 && c.Name[:11] == "definition." ||
			len(c.Name) > 10 && c.Name[:10] == "reference.":
			tag.Kind = c.Name
			tag.Range = c.Range()
		}
	}
	if tag.Kind != "" && tag.Name == "" {
		tag.Name = string(source[tag.Range.StartByte:tag.Range.EndByte])
		tag.NameRange = tag.Range
	}
	return tag
}
