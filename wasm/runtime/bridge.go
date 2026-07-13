package main

import (
	"encoding/json"
	"unicode/utf16"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type runtimeLanguage struct {
	language           *gotreesitter.Language
	tokenSourceFactory func([]byte) gotreesitter.TokenSource
	highlighter        *gotreesitter.Highlighter
	tagger             *gotreesitter.Tagger
}

func newRuntimeLanguage(name string, lang *gotreesitter.Language) runtimeLanguage {
	loaded := runtimeLanguage{language: lang}
	if lang == nil {
		return loaded
	}
	entry := grammars.DetectLanguageByName(name)
	if entry != nil && entry.TokenSourceFactory != nil {
		loaded.tokenSourceFactory = func(source []byte) gotreesitter.TokenSource {
			return entry.TokenSourceFactory(source, lang)
		}
	}
	return loaded
}

func (l runtimeLanguage) parseUTF16(source string) (*gotreesitter.Tree, error) {
	parser := gotreesitter.NewParser(l.language)
	return l.parseUTF16Units(parser, toUTF16(source), nil)
}

func (l runtimeLanguage) parseUTF16Units(parser *gotreesitter.Parser, source []uint16, oldTree *gotreesitter.Tree) (*gotreesitter.Tree, error) {
	if parser == nil {
		parser = gotreesitter.NewParser(l.language)
	}
	if l.tokenSourceFactory == nil {
		if oldTree != nil {
			return parser.ParseIncrementalUTF16(source, oldTree)
		}
		return parser.ParseUTF16(source)
	}
	factory := func(source []byte) (gotreesitter.TokenSource, error) {
		return l.tokenSourceFactory(source), nil
	}
	if oldTree != nil {
		return parser.ParseIncrementalUTF16WithTokenSourceFactory(source, oldTree, factory)
	}
	return parser.ParseUTF16WithTokenSourceFactory(source, factory)
}

func (l runtimeLanguage) newHighlighter(query string) (*gotreesitter.Highlighter, error) {
	if l.tokenSourceFactory == nil {
		return gotreesitter.NewHighlighter(l.language, query)
	}
	return gotreesitter.NewHighlighter(
		l.language,
		query,
		gotreesitter.WithTokenSourceFactory(l.tokenSourceFactory),
	)
}

func (l runtimeLanguage) newTagger(query string) (*gotreesitter.Tagger, error) {
	if l.tokenSourceFactory == nil {
		return gotreesitter.NewTagger(l.language, query)
	}
	return gotreesitter.NewTagger(
		l.language,
		query,
		gotreesitter.WithTaggerTokenSourceFactory(l.tokenSourceFactory),
	)
}

func toUTF16(source string) []uint16 {
	return utf16.Encode([]rune(source))
}

// jsonNode is the wire shape of one syntax node in the structured tree the
// parse global returns. start/end are canonical UTF-8 byte offsets;
// start16/end16 are UTF-16 code-unit offsets into the original JS string.
type jsonNode struct {
	Type      string      `json:"type"`
	Start     uint32      `json:"start"`
	End       uint32      `json:"end"`
	Start16   uint32      `json:"start16"`
	End16     uint32      `json:"end16"`
	Named     bool        `json:"named"`
	Missing   bool        `json:"missing,omitempty"`
	Error     bool        `json:"error,omitempty"`
	Field     string      `json:"field,omitempty"`
	Children  []*jsonNode `json:"children,omitempty"`
	Truncated bool        `json:"truncated,omitempty"` // set on the root only when nodes were omitted
}

// maxTreeNodes caps the structured tree payload. Past this we stop
// descending and mark the root truncated; the client renders what it got.
const maxTreeNodes = 20000

func spans16(tree *gotreesitter.Tree, startByte, endByte uint32) (uint32, uint32) {
	if tree != nil {
		if rng, has := tree.UTF16RangeForByteRange(startByte, endByte); has {
			return rng.StartCodeUnit, rng.EndCodeUnit
		}
	}
	// A parsed UTF-16 tree should always have a source map. For nil trees or a
	// missing map entry, degrade to byte offsets rather than lying with zeros.
	return startByte, endByte
}

type jsonTreeBuilder struct {
	remaining int
	omitted   bool
}

func buildJSONTree(tree *gotreesitter.Tree, lang *gotreesitter.Language, root *gotreesitter.Node, limit int) (*jsonNode, bool) {
	builder := jsonTreeBuilder{remaining: limit}
	jsonRoot := builder.build(tree, lang, root, "")
	if jsonRoot != nil {
		jsonRoot.Truncated = builder.omitted
	}
	return jsonRoot, builder.omitted
}

func (b *jsonTreeBuilder) build(tree *gotreesitter.Tree, lang *gotreesitter.Language, n *gotreesitter.Node, field string) *jsonNode {
	if n == nil {
		return nil
	}
	if b.remaining <= 0 {
		b.omitted = true
		return nil
	}
	b.remaining--

	start16, end16 := spans16(tree, n.StartByte(), n.EndByte())
	out := &jsonNode{
		Type:    n.Type(lang),
		Start:   n.StartByte(),
		End:     n.EndByte(),
		Start16: start16,
		End16:   end16,
		Named:   n.IsNamed(),
		Missing: n.IsMissing(),
		Error:   n.IsError(),
		Field:   field,
	}
	count := n.ChildCount()
	for i := 0; i < count; i++ {
		if b.remaining <= 0 {
			b.omitted = true
			break
		}
		child := b.build(tree, lang, n.Child(i), n.FieldNameForChild(i, lang))
		if child != nil {
			out.Children = append(out.Children, child)
		}
	}
	return out
}

func marshalJSONTree(tree *gotreesitter.Tree, lang *gotreesitter.Language, root *gotreesitter.Node, limit int) ([]byte, error) {
	jsonRoot, _ := buildJSONTree(tree, lang, root, limit)
	return json.Marshal(jsonRoot)
}

type jsonParseResult struct {
	SExpr    string
	HasError bool
	Tree     string
}

func buildJSONParseResult(tree *gotreesitter.Tree, lang *gotreesitter.Language, root *gotreesitter.Node, limit int) (jsonParseResult, error) {
	treeJSON, err := marshalJSONTree(tree, lang, root, limit)
	if err != nil {
		return jsonParseResult{}, err
	}
	result := jsonParseResult{Tree: string(treeJSON)}
	if root != nil {
		result.SExpr = root.SExpr(lang)
		result.HasError = root.HasError()
	}
	return result, nil
}

// maxQueryMatches caps the query result count retained by the bridge.
// QueryCursor probes at most one additional
// match so it can distinguish exactly-at-limit from omitted results.
const maxQueryMatches = 500

type jsonQueryCapture struct {
	Name    string
	Type    string
	Start   uint32
	End     uint32
	Start16 uint32
	End16   uint32
	Text    string
}

type jsonQueryMatch struct {
	Pattern  int
	Captures []jsonQueryCapture
}

func executeQueryJSON(q *gotreesitter.Query, tree *gotreesitter.Tree, lang *gotreesitter.Language, limit uint32) ([]jsonQueryMatch, bool) {
	if q == nil || tree == nil || lang == nil {
		return nil, false
	}

	root := tree.RootNode()
	if root == nil {
		return nil, false
	}
	cursor := q.Exec(root, lang, tree.Source())
	cursor.SetMatchLimit(limit)

	capacity := int(limit)
	if capacity == 0 || capacity > maxQueryMatches {
		capacity = maxQueryMatches
	}
	matches := make([]jsonQueryMatch, 0, capacity)
	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}
		captures := make([]jsonQueryCapture, len(match.Captures))
		for i, capture := range match.Captures {
			var startByte, endByte uint32
			var nodeType, text string
			if capture.Node != nil {
				startByte, endByte = capture.Node.StartByte(), capture.Node.EndByte()
				nodeType = capture.Node.Type(lang)
				text = capture.Text(tree.Source())
			}
			start16, end16 := spans16(tree, startByte, endByte)
			captures[i] = jsonQueryCapture{
				Name:    capture.Name,
				Type:    nodeType,
				Start:   startByte,
				End:     endByte,
				Start16: start16,
				End16:   end16,
				Text:    text,
			}
		}
		matches = append(matches, jsonQueryMatch{
			Pattern:  match.PatternIndex,
			Captures: captures,
		})
	}
	return matches, cursor.DidExceedMatchLimit()
}

func queryMatchesForJS(matches []jsonQueryMatch) []interface{} {
	jsMatches := make([]interface{}, len(matches))
	for i, match := range matches {
		jsCaptures := make([]interface{}, len(match.Captures))
		for j, capture := range match.Captures {
			item := map[string]interface{}{
				"name":    capture.Name,
				"start":   capture.Start,
				"end":     capture.End,
				"start16": capture.Start16,
				"end16":   capture.End16,
				"text":    capture.Text,
			}
			if capture.Type != "" {
				item["type"] = capture.Type
			}
			jsCaptures[j] = item
		}
		jsMatches[i] = map[string]interface{}{
			"pattern":  match.Pattern,
			"captures": jsCaptures,
		}
	}
	return jsMatches
}
