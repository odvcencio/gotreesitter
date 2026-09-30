package main

import (
	"bytes"
	"encoding/json"
	"strconv"
	"unicode/utf16"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
	"github.com/odvcencio/gotreesitter/internal/treewalk"
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
	makeNode := func(n *gotreesitter.Node, field string) *jsonNode {
		b.remaining--
		start16, end16 := spans16(tree, n.StartByte(), n.EndByte())
		return &jsonNode{Type: n.Type(lang), Start: n.StartByte(), End: n.EndByte(), Start16: start16, End16: end16, Named: n.IsNamed(), Missing: n.IsMissing(), Error: n.IsError(), Field: field}
	}
	root := makeNode(n, field)
	var inline [32]treewalk.FoldFrame[*gotreesitter.Node, *jsonNode]
	frames := append(inline[:0], treewalk.FoldFrame[*gotreesitter.Node, *jsonNode]{Node: n, Value: root})
	for len(frames) > 0 {
		f := &frames[len(frames)-1]
		if f.NextChild == f.Node.ChildCount() {
			frames = frames[:len(frames)-1]
			continue
		}
		if b.remaining <= 0 {
			b.omitted = true
			break
		}
		i := f.NextChild
		f.NextChild++
		child := f.Node.Child(i)
		if child == nil {
			continue
		}
		out := makeNode(child, f.Node.FieldNameForChild(i, lang))
		f.Value.Children = append(f.Value.Children, out)
		frames = append(frames, treewalk.FoldFrame[*gotreesitter.Node, *jsonNode]{Node: child, Value: out})
	}
	return root
}

func marshalJSONTree(tree *gotreesitter.Tree, lang *gotreesitter.Language, root *gotreesitter.Node, limit int) ([]byte, error) {
	jsonRoot, _ := buildJSONTree(tree, lang, root, limit)
	if jsonRoot == nil {
		return []byte("null"), nil
	}
	// encoding/json recursively encodes pointer trees. Encode each header on
	// its own and write the child arrays with explicit frames instead.
	var out bytes.Buffer
	quoted := make(map[string][]byte)
	writeString := func(value string) error {
		encoded, ok := quoted[value]
		if !ok {
			var err error
			encoded, err = json.Marshal(value)
			if err != nil {
				return err
			}
			quoted[value] = encoded
		}
		out.Write(encoded)
		return nil
	}
	var digits [20]byte
	writeUint := func(value uint32) { out.Write(strconv.AppendUint(digits[:0], uint64(value), 10)) }
	var inline [32]treewalk.ChildFrame[*jsonNode]
	frames := append(inline[:0], treewalk.ChildFrame[*jsonNode]{Node: jsonRoot, NextChild: -1})
	for len(frames) > 0 {
		f := &frames[len(frames)-1]
		if f.NextChild == -1 {
			n := f.Node
			out.WriteString(`{"type":`)
			if err := writeString(n.Type); err != nil {
				return nil, err
			}
			out.WriteString(`,"start":`)
			writeUint(n.Start)
			out.WriteString(`,"end":`)
			writeUint(n.End)
			out.WriteString(`,"start16":`)
			writeUint(n.Start16)
			out.WriteString(`,"end16":`)
			writeUint(n.End16)
			out.WriteString(`,"named":`)
			if n.Named {
				out.WriteString("true")
			} else {
				out.WriteString("false")
			}
			if n.Missing {
				out.WriteString(`,"missing":true`)
			}
			if n.Error {
				out.WriteString(`,"error":true`)
			}
			if n.Field != "" {
				out.WriteString(`,"field":`)
				if err := writeString(n.Field); err != nil {
					return nil, err
				}
			}

			if len(f.Node.Children) > 0 {
				out.WriteString(`,"children":[`)
			}
			f.NextChild = 0
		}
		if f.NextChild == len(f.Node.Children) {
			if len(f.Node.Children) > 0 {
				out.WriteByte(']')
			}
			if f.Node.Truncated {
				out.WriteString(`,"truncated":true`)
			}
			out.WriteByte('}')
			frames = frames[:len(frames)-1]
			continue
		}
		i := f.NextChild
		f.NextChild++
		if i > 0 {
			out.WriteByte(',')
		}
		frames = append(frames, treewalk.ChildFrame[*jsonNode]{Node: f.Node.Children[i], NextChild: -1})
	}
	return out.Bytes(), nil
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
				startByte, endByte = capture.ByteRange()
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
