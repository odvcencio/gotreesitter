package gotreesitter

import "strings"

func (h *Highlighter) appendInjectedRanges(tree *Tree, source []byte, ranges []HighlightRange) []HighlightRange {
	if h == nil || tree == nil || h.injectionQuery == nil || h.injectionResolver == nil || len(source) == 0 {
		return ranges
	}
	root := tree.RootNode()
	if root == nil {
		return ranges
	}

	cursor := h.injectionQuery.Exec(root, h.lang, source)
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		ctx, ok := h.injectedHighlightContext(match, source)
		if !ok {
			continue
		}
		childRanges := h.collectInjectedHighlightRanges(ctx)
		ranges = appendOffsetHighlightRanges(ranges, childRanges, ctx.start)
	}

	return ranges
}

type injectionMatchCaptures struct {
	contentNode *Node
	startNode   *Node
	langHint    string
}

type injectedHighlightContext struct {
	lang               *Language
	querySource        string
	tokenSourceFactory func(source []byte) TokenSource
	cacheKey           string
	source             []byte
	start              uint32
}

func (h *Highlighter) injectedHighlightContext(match QueryMatch, source []byte) (injectedHighlightContext, bool) {
	captures := h.collectInjectionCaptures(match, source)
	if captures.contentNode == nil {
		return injectedHighlightContext{}, false
	}
	normalizedHint := normalizeInjectionLanguageHint(captures.langHint)
	if normalizedHint == "" {
		return injectedHighlightContext{}, false
	}

	childLang, childQueryStr, childTSFactory, ok := h.injectionResolver(normalizedHint)
	if !ok || childLang == nil || strings.TrimSpace(childQueryStr) == "" {
		return injectedHighlightContext{}, false
	}

	start, end, ok := injectedContentByteRange(captures.contentNode, captures.startNode, len(source))
	if !ok {
		return injectedHighlightContext{}, false
	}

	return injectedHighlightContext{
		lang:               childLang,
		querySource:        childQueryStr,
		tokenSourceFactory: childTSFactory,
		cacheKey:           injectedHighlightCacheKey(childLang, normalizedHint),
		source:             source[start:end],
		start:              start,
	}, true
}

func (h *Highlighter) collectInjectionCaptures(match QueryMatch, source []byte) injectionMatchCaptures {
	captures := injectionMatchCaptures{}
	if vals := match.SetValues(h.injectionQuery, "injection.language"); len(vals) > 0 {
		captures.langHint = vals[0]
	}
	for _, c := range match.Captures {
		switch c.Name {
		case "injection.content":
			captures.contentNode = c.Node
		case "injection.start":
			captures.startNode = c.Node
		case "injection.language":
			if captures.langHint == "" && c.Node != nil {
				captures.langHint = c.Node.Text(source)
			}
		}
	}
	return captures
}

func injectedContentByteRange(contentNode, startNode *Node, sourceLen int) (uint32, uint32, bool) {
	start := contentNode.StartByte()
	if startNode != nil && startNode.StartByte() < start {
		start = startNode.StartByte()
	}
	end := contentNode.EndByte()
	return start, end, end > start && int(end) <= sourceLen
}

func injectedHighlightCacheKey(lang *Language, fallback string) string {
	if lang.Name != "" {
		return lang.Name
	}
	return fallback
}

func (h *Highlighter) collectInjectedHighlightRanges(ctx injectedHighlightContext) []HighlightRange {
	childTree, err := h.parseInjectedTree(ctx.lang, ctx.tokenSourceFactory, ctx.source)
	if err != nil || childTree == nil || childTree.RootNode() == nil {
		if childTree != nil {
			childTree.Release()
		}
		return nil
	}
	defer childTree.Release()

	childQuery, ok := h.childHighlightQuery(ctx.cacheKey, ctx.querySource, ctx.lang)
	if !ok {
		return nil
	}
	childRanges := collectHighlightRanges(childQuery, childTree)
	if len(childRanges) == 0 && ctx.cacheKey == "go" {
		childRanges = h.collectWrappedGoHighlightRanges(ctx, childQuery)
	}
	return childRanges
}

func (h *Highlighter) childHighlightQuery(cacheKey string, querySource string, lang *Language) (*Query, bool) {
	// Combine the language resolver key and the exact query source so queries
	// compiled from different sources are never confused for one another.
	compositeKey := cacheKey + "\x00" + querySource
	childQuery := h.childQueries[compositeKey]
	if childQuery != nil {
		return childQuery, true
	}
	childQuery, err := NewQuery(querySource, lang)
	if err != nil {
		return nil, false
	}
	h.childQueries[compositeKey] = childQuery
	return childQuery, true
}

func (h *Highlighter) collectWrappedGoHighlightRanges(ctx injectedHighlightContext, childQuery *Query) []HighlightRange {
	const prefix = "package main\nfunc __gts_markdown_fence__() {\n"
	const suffix = "\n}\n"

	wrapped := make([]byte, 0, len(prefix)+len(ctx.source)+len(suffix))
	wrapped = append(wrapped, []byte(prefix)...)
	wrapped = append(wrapped, ctx.source...)
	wrapped = append(wrapped, []byte(suffix)...)

	wrappedTree, wrappedErr := h.parseInjectedTree(ctx.lang, ctx.tokenSourceFactory, wrapped)
	if wrappedErr != nil || wrappedTree == nil || wrappedTree.RootNode() == nil {
		if wrappedTree != nil {
			wrappedTree.Release()
		}
		return nil
	}
	defer wrappedTree.Release()

	offset := uint32(len(prefix))
	endOffset := offset + uint32(len(ctx.source))
	childRanges := make([]HighlightRange, 0)
	for _, r := range collectHighlightRanges(childQuery, wrappedTree) {
		if r.StartByte < offset || r.EndByte > endOffset {
			continue
		}
		childRanges = append(childRanges, HighlightRange{
			StartByte: r.StartByte - offset,
			EndByte:   r.EndByte - offset,
			Capture:   r.Capture,
		})
	}
	return childRanges
}

func appendOffsetHighlightRanges(dst []HighlightRange, ranges []HighlightRange, offset uint32) []HighlightRange {
	for _, r := range ranges {
		dst = append(dst, HighlightRange{
			StartByte: r.StartByte + offset,
			EndByte:   r.EndByte + offset,
			Capture:   r.Capture,
		})
	}
	return dst
}

func (h *Highlighter) parseInjectedTree(lang *Language, tokenSourceFactory func(source []byte) TokenSource, source []byte) (*Tree, error) {
	childParser := NewParser(lang)
	// Injected highlight subtrees are outside the admission scorecard's validated
	// surface: keep the child parser on production regardless of the default.
	childParser.pinToProductionRoute()
	if tokenSourceFactory != nil {
		return childParser.ParseWithTokenSource(source, tokenSourceFactory(source))
	}
	return childParser.Parse(source)
}

func collectHighlightRanges(q *Query, tree *Tree) []HighlightRange {
	if q == nil || tree == nil {
		return nil
	}
	matches := q.Execute(tree)
	if len(matches) == 0 {
		return nil
	}
	ranges := make([]HighlightRange, 0, len(matches)*2)
	for _, m := range matches {
		for _, c := range m.Captures {
			if c.Node.StartByte() == c.Node.EndByte() {
				continue
			}
			ranges = append(ranges, HighlightRange{
				StartByte: c.Node.StartByte(),
				EndByte:   c.Node.EndByte(),
				Capture:   c.Name,
			})
		}
	}
	return ranges
}
