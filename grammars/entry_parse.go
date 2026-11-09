package grammars

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/odvcencio/gotreesitter"
)

var errEntryWithoutLanguage = errors.New("grammars: language entry has no loader")

// NewParser returns a parser for the entry's language. The parser has no token
// source of its own, so parser.Parse always uses the DFA lexer. Call
// entry.Parse to use the registered token source (c, cpp, java, authzed).
func (e *LangEntry) NewParser() *gotreesitter.Parser {
	return gotreesitter.NewParser(e.Language())
}

// Parse parses src using the registered token source, or the DFA lexer if none
// is registered. Release every non-nil returned tree.
func (e *LangEntry) Parse(src []byte) (*gotreesitter.Tree, error) {
	return e.parse(context.Background(), src, false)
}

// ParseContext is like Parse, with cooperative cancellation and deadlines.
// ctx must be non-nil. Cancellation cannot interrupt a blocking language
// loader, scanner or token source. A context-induced stop returns ctx.Err()
// and any partial tree. Other early stops retain Parser.Parse semantics; use
// ParseStrict to report those as errors. Release every non-nil returned tree,
// including when err is non-nil.
//
// +spec=`Context belongs to one parse; completed trees are not invalidated by late cancellation.`
func (e *LangEntry) ParseContext(ctx context.Context, src []byte) (*gotreesitter.Tree, error) {
	return e.parse(ctx, src, false)
}

// ParseStrict is like ParseContext, but reports every early stop as an error. A
// context-induced stop also matches gotreesitter.ErrParseStoppedEarly when the
// parser returns a partial tree. Cancellation before parsing returns only
// ctx.Err(). Syntax errors remain represented by the tree, as in Parser.ParseStrict.
func (e *LangEntry) ParseStrict(ctx context.Context, src []byte) (*gotreesitter.Tree, error) {
	return e.parse(ctx, src, true)
}

func (e *LangEntry) parse(ctx context.Context, src []byte, strict bool) (*gotreesitter.Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if e.Language == nil {
		return nil, errEntryWithoutLanguage
	}
	lang := e.Language()
	parser := gotreesitter.NewParser(lang)
	var ts gotreesitter.TokenSource
	if e.TokenSourceFactory != nil {
		ts = e.TokenSourceFactory(src, lang)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if ctx.Done() != nil {
		var cancelled uint32
		parser.SetCancellationFlag(&cancelled)
		stop := context.AfterFunc(ctx, func() { atomic.StoreUint32(&cancelled, 1) })
		defer stop()
	}

	var tree *gotreesitter.Tree
	var err error
	if e.TokenSourceFactory != nil {
		if strict {
			tree, err = parser.ParseWithTokenSourceStrict(src, ts)
		} else {
			tree, err = parser.ParseWithTokenSource(src, ts)
		}
	} else if strict {
		tree, err = parser.ParseStrict(src)
	} else {
		tree, err = parser.Parse(src)
	}
	return entryParseResult(tree, err, ctx)
}

// +why=`The parser stop reason, not a late context notification, owns the outcome.`
func entryParseResult(tree *gotreesitter.Tree, err error, ctx context.Context) (*gotreesitter.Tree, error) {
	if tree != nil && tree.ParseStopReason() == gotreesitter.ParseStopCancelled {
		return tree, errors.Join(err, ctx.Err())
	}
	return tree, err
}

// NewHighlighter returns a highlighter built from the entry's language and
// HighlightQuery. It passes the entry's TokenSourceFactory to the highlighter
// when there is one; a WithTokenSourceFactory option in opts overrides it. Take
// the entry from DetectLanguage, DetectLanguageByName or AllLanguages, so that
// HighlightQuery holds the composed query.
func (e *LangEntry) NewHighlighter(opts ...gotreesitter.HighlighterOption) (*gotreesitter.Highlighter, error) {
	if e.Language == nil {
		return nil, errEntryWithoutLanguage
	}
	lang := e.Language()
	if factory := e.TokenSourceFactory; factory != nil {
		opts = append([]gotreesitter.HighlighterOption{
			gotreesitter.WithTokenSourceFactory(func(src []byte) gotreesitter.TokenSource {
				return factory(src, lang)
			}),
		}, opts...)
	}
	return gotreesitter.NewHighlighter(lang, e.HighlightQuery, opts...)
}
