//go:build js && wasm

package main

import (
	"regexp"
	"strconv"
	"syscall/js"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

type runtimeDocument struct {
	runtime  runtimeLanguage
	source   []uint16
	parser   *gotreesitter.Parser
	tree     *gotreesitter.Tree
	revision uint64
}

func main() {
	runtime := js.Global().Get("Object").New()
	runtime.Set("parse", js.FuncOf(parse))
	runtime.Set("query", js.FuncOf(query))
	runtime.Set("highlight", js.FuncOf(highlight))
	runtime.Set("loadBlob", js.FuncOf(loadBlob))
	runtime.Set("open", js.FuncOf(openDocument))
	runtime.Set("update", js.FuncOf(updateDocument))
	runtime.Set("close", js.FuncOf(closeDocument))
	runtime.Set("queryDocument", js.FuncOf(queryDocument))
	runtime.Set("version", "0.2.0-runtime")
	runtime.Set("mode", "runtime")
	js.Global().Set("gotreesitter", runtime)
	select {}
}

var languages = map[string]runtimeLanguage{}
var highlighters = map[string]*gotreesitter.Highlighter{}
var documents = map[string]*runtimeDocument{}

func loadBlob(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return err("usage: loadBlob(name, blobUint8Array, highlightQuery, [tagsQuery])")
	}
	name := args[0].String()
	jsArr := args[1]
	queryText := args[2].String()
	tagsText := ""
	if len(args) >= 4 && args[3].Type() == js.TypeString {
		tagsText = args[3].String()
	}

	length := jsArr.Get("length").Int()
	blob := make([]byte, length)
	js.CopyBytesToGo(blob, jsArr)

	lang, langErr := grammars.LoadLanguage(name, blob)
	if langErr != nil {
		return err("load blob: " + langErr.Error())
	}
	loaded := newRuntimeLanguage(name, lang)

	var hl *gotreesitter.Highlighter
	if queryText != "" {
		var hlErr error
		hl, hlErr = loaded.newHighlighter(queryText)
		if hlErr != nil {
			return err("highlighter: " + hlErr.Error())
		}
	}
	var tagger *gotreesitter.Tagger
	if tagsText != "" {
		tagger, langErr = loaded.newTagger(tagsText)
		if langErr != nil {
			return err("tagger: " + langErr.Error())
		}
	}
	loaded.highlighter = hl
	loaded.tagger = tagger

	// Publish the language and its optional highlighter together only after all
	// validation succeeds. Reloading without a query intentionally clears an
	// older highlighter bound to the previous language instance.
	languages[name] = loaded
	if hl != nil {
		highlighters[name] = hl
	} else {
		delete(highlighters, name)
	}

	return ok(map[string]interface{}{
		"name":         name,
		"highlighting": hl != nil,
		"tags":         tagger != nil,
	})
}

func parse(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return err("usage: parse(name, source)")
	}
	loaded, has := languages[args[0].String()]
	if !has {
		return err("language not loaded: " + args[0].String())
	}
	tree, parseErr := loaded.parseUTF16(args[1].String())
	if parseErr != nil {
		return err(parseErr.Error())
	}
	if tree == nil {
		return err("parse returned no tree")
	}
	defer tree.Release()

	result, marshalErr := buildJSONParseResult(tree, loaded.language, tree.RootNode(), maxTreeNodes)
	if marshalErr != nil {
		return err("tree marshal: " + marshalErr.Error())
	}

	return ok(map[string]interface{}{
		"sexp":     result.SExpr,
		"hasError": result.HasError,
		// One JSON string over the JS boundary; the client JSON.parses it.
		"tree": result.Tree,
	})
}

// queryErrorPos extracts the byte offset embedded in NewQuery error text
// ("... at position 17 ..."). The engine reports positions inside the
// message rather than as a structured field; most compile errors carry one.
var queryErrorPos = regexp.MustCompile(`at position (\d+)`)

func query(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return err("usage: query(name, source, queryText)")
	}
	loaded, has := languages[args[0].String()]
	if !has {
		return err("language not loaded: " + args[0].String())
	}

	q, qErr := gotreesitter.NewQuery(args[2].String(), loaded.language)
	if qErr != nil {
		res := map[string]interface{}{"ok": false, "error": qErr.Error()}
		if m := queryErrorPos.FindStringSubmatch(qErr.Error()); m != nil {
			if off, convErr := strconv.Atoi(m[1]); convErr == nil {
				res["errorOffset"] = off
			}
		}
		return jsObject(res)
	}

	tree, parseErr := loaded.parseUTF16(args[1].String())
	if parseErr != nil {
		return err(parseErr.Error())
	}
	if tree == nil {
		return err("parse returned no tree")
	}
	defer tree.Release()

	matches, truncated := executeQueryJSON(q, tree, loaded.language, maxQueryMatches)

	return ok(map[string]interface{}{
		"matches":   queryMatchesForJS(matches),
		"truncated": truncated,
	})
}

func highlight(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return err("usage: highlight(name, source)")
	}
	hl, has := highlighters[args[0].String()]
	if !has {
		return err("no highlighter for: " + args[0].String())
	}
	ranges := hl.Highlight([]byte(args[1].String()))
	jsRanges := make([]interface{}, len(ranges))
	for i, r := range ranges {
		jsRanges[i] = map[string]interface{}{
			"startByte": r.StartByte,
			"endByte":   r.EndByte,
			"capture":   r.Capture,
		}
	}
	return ok(map[string]interface{}{"ranges": jsRanges})
}

func openDocument(this js.Value, args []js.Value) interface{} {
	if len(args) < 3 {
		return err("usage: open(name, documentID, source)")
	}
	name, documentID := args[0].String(), args[1].String()
	if documentID == "" {
		return err("document id is required")
	}
	loaded, has := languages[name]
	if !has {
		return err("language not loaded: " + name)
	}

	source := toUTF16(args[2].String())
	document := &runtimeDocument{
		runtime:  loaded,
		source:   source,
		parser:   gotreesitter.NewParser(loaded.language),
		revision: 1,
	}
	var parseErr error
	document.tree, parseErr = loaded.parseUTF16Units(document.parser, source, nil)
	if parseErr != nil {
		if document.tree != nil {
			document.tree.Release()
		}
		return err(parseErr.Error())
	}
	if document.tree == nil {
		return err("parse returned no tree")
	}
	if previous := documents[documentID]; previous != nil {
		previous.release()
	}
	documents[documentID] = document
	return document.analysis(false, nil)
}

func updateDocument(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return err("usage: update(documentID, source)")
	}
	documentID := args[0].String()
	document, has := documents[documentID]
	if !has {
		return err("document not open: " + documentID)
	}
	newSource := toUTF16(args[1].String())
	edit, changed := utf16EditBetween(document.source, newSource)
	if !changed {
		return document.analysis(true, &edit)
	}
	if !document.tree.EditUTF16(edit, newSource) {
		return err("edit does not align to a UTF-16 boundary")
	}
	oldTree := document.tree
	newTree, parseErr := document.runtime.parseUTF16Units(document.parser, newSource, oldTree)
	if parseErr != nil || newTree == nil {
		if newTree != nil && newTree != oldTree {
			newTree.Release()
		}
		// Tree.EditUTF16 has already mutated oldTree. A full reparse gives the
		// document a transactional fallback for parser errors and the defensive
		// nil-tree/no-error edge instead of retaining a half-updated tree.
		freshParser := gotreesitter.NewParser(document.runtime.language)
		newTree, parseErr = document.runtime.parseUTF16Units(freshParser, newSource, nil)
		if parseErr == nil && newTree != nil {
			document.parser = freshParser
		} else {
			if newTree != nil {
				newTree.Release()
			}
			oldTree.Release()
			document.tree = nil
			delete(documents, documentID)
			if parseErr != nil {
				return err("update failed and the document was closed: " + parseErr.Error())
			}
			return err("update returned no tree and the document was closed")
		}
	}
	document.tree = newTree
	document.source = newSource
	document.revision++
	if oldTree != newTree {
		oldTree.Release()
	}
	return document.analysis(true, &edit)
}

func closeDocument(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return err("usage: close(documentID)")
	}
	documentID := args[0].String()
	document, has := documents[documentID]
	if has {
		document.release()
		delete(documents, documentID)
	}
	return ok(map[string]interface{}{"closed": has})
}

func queryDocument(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return err("usage: queryDocument(documentID, queryText)")
	}
	document, has := documents[args[0].String()]
	if !has {
		return err("document not open: " + args[0].String())
	}
	q, queryErr := gotreesitter.NewQuery(args[1].String(), document.runtime.language)
	if queryErr != nil {
		return err(queryErr.Error())
	}
	matches, truncated := executeQueryJSON(q, document.tree, document.runtime.language, maxQueryMatches)
	return ok(map[string]interface{}{
		"matches":   queryMatchesForJS(matches),
		"truncated": truncated,
	})
}

func (document *runtimeDocument) analysis(incremental bool, edit *gotreesitter.UTF16Edit) interface{} {
	root := document.tree.RootNode()
	result := map[string]interface{}{
		"revision":    document.revision,
		"incremental": incremental,
		"hasError":    root != nil && root.HasError(),
		"highlights":  []interface{}{},
		"tags":        []interface{}{},
	}
	if document.runtime.highlighter != nil {
		result["highlights"] = highlightRanges(document.runtime.highlighter.HighlightTreeUTF16(document.tree))
	}
	if document.runtime.tagger != nil {
		result["tags"] = tagRanges(document.runtime.tagger.TagTreeUTF16(document.tree))
	}
	if edit != nil {
		result["edit"] = map[string]interface{}{
			"start16":  edit.StartCodeUnit,
			"oldEnd16": edit.OldEndCodeUnit,
			"newEnd16": edit.NewEndCodeUnit,
		}
	}
	return ok(result)
}

func (document *runtimeDocument) release() {
	if document != nil && document.tree != nil {
		document.tree.Release()
		document.tree = nil
	}
}

func highlightRanges(ranges []gotreesitter.UTF16HighlightRange) []interface{} {
	result := make([]interface{}, 0, len(ranges))
	for _, highlight := range ranges {
		result = append(result, map[string]interface{}{
			"start16":      highlight.StartCodeUnit,
			"end16":        highlight.EndCodeUnit,
			"startPoint":   jsPoint(highlight.StartPoint),
			"endPoint":     jsPoint(highlight.EndPoint),
			"capture":      highlight.Capture,
			"patternIndex": highlight.PatternIndex,
		})
	}
	return result
}

func tagRanges(tags []gotreesitter.UTF16Tag) []interface{} {
	result := make([]interface{}, 0, len(tags))
	for _, tag := range tags {
		result = append(result, map[string]interface{}{
			"kind":      tag.Kind,
			"name":      tag.Name,
			"range":     jsRange(tag.Range),
			"nameRange": jsRange(tag.NameRange),
		})
	}
	return result
}

func jsRange(sourceRange gotreesitter.UTF16Range) map[string]interface{} {
	return map[string]interface{}{
		"start16":    sourceRange.StartCodeUnit,
		"end16":      sourceRange.EndCodeUnit,
		"startPoint": jsPoint(sourceRange.StartPoint),
		"endPoint":   jsPoint(sourceRange.EndPoint),
	}
}

func jsPoint(point gotreesitter.Point) map[string]interface{} {
	return map[string]interface{}{"row": point.Row, "column": point.Column}
}

func ok(extra map[string]interface{}) interface{} {
	extra["ok"] = true
	return jsObject(extra)
}

func err(msg string) interface{} {
	return jsObject(map[string]interface{}{"ok": false, "error": msg})
}

// TinyGo's syscall/js.ValueOf intentionally accepts only primitive Go values.
// Build compound values explicitly so the same runtime works under TinyGo and
// the standard Go js/wasm target.
func jsObject(fields map[string]interface{}) js.Value {
	object := js.Global().Get("Object").New()
	for key, value := range fields {
		object.Set(key, jsValue(value))
	}
	return object
}

func jsArray(values []interface{}) js.Value {
	array := js.Global().Get("Array").New(len(values))
	for index, value := range values {
		array.SetIndex(index, jsValue(value))
	}
	return array
}

func jsValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return jsObject(typed)
	case []interface{}:
		return jsArray(typed)
	case uint:
		return float64(typed)
	case uint8:
		return float64(typed)
	case uint16:
		return float64(typed)
	case uint32:
		return float64(typed)
	case uint64:
		return float64(typed)
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return float64(typed)
	default:
		return value
	}
}
