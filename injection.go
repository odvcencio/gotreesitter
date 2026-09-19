package gotreesitter

import (
	"fmt"
	"strings"
)

// InjectionResult holds parse results for a multi-language document.
type InjectionResult struct {
	// Tree is the parent language's parse tree.
	Tree *Tree
	// Injections contains child language parse results, ordered by position.
	Injections []Injection
}

// UTF16InjectionResult holds parse results for a UTF-16 multi-language
// document. Injection ranges are expressed in UTF-16 code units.
type UTF16InjectionResult struct {
	// Tree is the parent language's parse tree.
	Tree *Tree
	// Injections contains child language parse results, ordered by position.
	Injections []UTF16Injection

	byteResult *InjectionResult
}

// Injection is a single embedded language region.
type Injection struct {
	// Language is the detected language name (e.g., "javascript").
	Language string
	// Tree is the parse tree for this region, or nil if the language
	// was not registered.
	Tree *Tree
	// Ranges are the source ranges this tree covers.
	Ranges []Range
	// Node is the parent tree node that triggered the injection.
	Node *Node
}

// UTF16Injection is a single embedded language region with ranges in UTF-16
// code-unit coordinates.
type UTF16Injection struct {
	// Language is the detected language name (e.g., "javascript").
	Language string
	// Tree is the parse tree for this region, or nil if the language
	// was not registered.
	Tree *Tree
	// Ranges are the source ranges this tree covers in UTF-16 code units.
	Ranges []UTF16Range
	// Node is the parent tree node that triggered the injection.
	Node *Node
}

// InjectionParser parses documents with embedded languages.
//
// InjectionParser is not safe for concurrent use. It caches child parsers and
// mutates shared maps during parse operations.
type InjectionParser struct {
	// languages maps language name -> Language.
	languages map[string]*Language
	// injectionQueries maps parent language name -> compiled injection query.
	injectionQueries map[string]*Query
	// parsers caches Parser instances per language for reuse.
	parsers map[string]*Parser
	// maxDepth limits nested injection recursion. Zero means use default.
	maxDepth int
	// prevResult holds the previous parse result for reuse.
	prevResult *InjectionResult
	// timeoutMicros bounds every parse this InjectionParser performs, root
	// and nested. Zero disables timeout checks. See SetTimeoutMicros.
	timeoutMicros uint64
	// cancellationFlag stops every parse this InjectionParser performs, root
	// and nested, when the pointed value becomes non-zero. See
	// SetCancellationFlag.
	cancellationFlag *uint32
}

// NewInjectionParser creates an InjectionParser.
func NewInjectionParser() *InjectionParser {
	return &InjectionParser{
		languages:        make(map[string]*Language),
		injectionQueries: make(map[string]*Query),
		parsers:          make(map[string]*Parser),
	}
}

// RegisterLanguage adds a language that can be used as parent or child.
func (ip *InjectionParser) RegisterLanguage(name string, lang *Language) {
	ip.languages[name] = lang
}

// SetTimeoutMicros configures a per-parse timeout in microseconds that
// bounds every parse this InjectionParser performs: the root parse and
// every nested injected parse, at every recursion depth. A value of zero
// disables timeout checks. It applies immediately to already-cached child
// parsers and to any parser created afterward.
func (ip *InjectionParser) SetTimeoutMicros(timeoutMicros uint64) {
	if ip == nil {
		return
	}
	ip.timeoutMicros = timeoutMicros
	for _, p := range ip.parsers {
		p.SetTimeoutMicros(timeoutMicros)
	}
}

// SetCancellationFlag configures a caller-owned cancellation flag that stops
// every parse this InjectionParser performs — the root parse and every
// nested injected parse, at every recursion depth — when the pointed value
// becomes non-zero. It applies immediately to already-cached child parsers
// and to any parser created afterward.
func (ip *InjectionParser) SetCancellationFlag(flag *uint32) {
	if ip == nil {
		return
	}
	ip.cancellationFlag = flag
	for _, p := range ip.parsers {
		p.SetCancellationFlag(flag)
	}
}

// RegisterInjectionQuery sets the injection query for a parent language.
// The query should use @injection.content and #set! injection.language
// conventions. It is compiled against the registered parent language.
func (ip *InjectionParser) RegisterInjectionQuery(parentLang string, query string) error {
	lang, ok := ip.languages[parentLang]
	if !ok {
		return fmt.Errorf("injection: parent language %q not registered", parentLang)
	}
	q, err := NewQuery(query, lang)
	if err != nil {
		return fmt.Errorf("injection: compiling query for %q: %w", parentLang, err)
	}
	ip.injectionQueries[parentLang] = q
	return nil
}

// releaseResult releases all parse trees held by r. Safe to call with nil.
func releaseResult(r *InjectionResult) {
	if r == nil {
		return
	}
	r.Tree.Release()
	for _, inj := range r.Injections {
		if inj.Tree != nil {
			inj.Tree.Release()
		}
	}
}

func releaseResultExcept(r, keep *InjectionResult) {
	if r == nil {
		return
	}
	if keep == nil {
		releaseResult(r)
		return
	}

	keptTrees := map[*Tree]struct{}{}
	if keep.Tree != nil {
		keptTrees[keep.Tree] = struct{}{}
	}
	for _, inj := range keep.Injections {
		if inj.Tree != nil {
			keptTrees[inj.Tree] = struct{}{}
		}
	}

	if _, ok := keptTrees[r.Tree]; !ok {
		r.Tree.Release()
	}
	for _, inj := range r.Injections {
		if _, ok := keptTrees[inj.Tree]; ok {
			continue
		}
		if inj.Tree != nil {
			inj.Tree.Release()
		}
	}
}

// Parse parses source as parentLang, then recursively parses injected regions.
func (ip *InjectionParser) Parse(source []byte, parentLang string) (*InjectionResult, error) {
	// Release previous result to allow arena reuse.
	releaseResult(ip.prevResult)
	ip.prevResult = nil

	lang, ok := ip.languages[parentLang]
	if !ok {
		return nil, fmt.Errorf("injection: language %q not registered", parentLang)
	}

	parser := ip.getParser(parentLang, lang)
	tree, err := parser.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("injection: parsing %q: %w", parentLang, err)
	}

	injections, err := ip.findAndParseInjections(source, parentLang, tree, 0)
	if err != nil {
		return nil, err
	}

	ip.prevResult = &InjectionResult{
		Tree:       tree,
		Injections: injections,
	}

	return ip.prevResult, nil
}

// ParseUTF16 parses UTF-16 source as parentLang, then recursively parses
// injected regions. The returned injection ranges are in UTF-16 code units.
func (ip *InjectionParser) ParseUTF16(source []uint16, parentLang string) (*UTF16InjectionResult, error) {
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	result, err := ip.Parse(utf8Source, parentLang)
	if err != nil {
		return nil, err
	}
	attachUTF16SourceToInjectionResult(result, source, sourceMap)
	return ip.utf16InjectionResult(result)
}

// ParseUTF16Bytes is like ParseUTF16 for endian-specific UTF-16 bytes.
func (ip *InjectionParser) ParseUTF16Bytes(source []byte, parentLang string, order UTF16ByteOrder) (*UTF16InjectionResult, error) {
	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return ip.ParseUTF16(units, parentLang)
}

// ParseIncremental re-parses after edits, reusing unchanged child trees.
//
// It detects and parses injections the same way Parse does — recursing into
// each injected language's own injection query, and routing single-range
// injections through the slice-and-rebase path — so a ParseIncremental call
// produces the same nested injection tree as a fresh Parse of the same
// (edited) source. The only difference is that a detected injection whose
// byte range does not overlap any changed range reuses its old child tree
// instead of reparsing it.
func (ip *InjectionParser) ParseIncremental(source []byte, parentLang string,
	oldResult *InjectionResult) (*InjectionResult, error) {

	if oldResult == nil {
		return ip.Parse(source, parentLang)
	}

	// Detach prevResult now; release it after parsing so that oldResult.Tree
	// (which may be the same object) remains valid throughout the parse.
	prev := ip.prevResult
	ip.prevResult = nil
	defer func() {
		releaseResultExcept(prev, ip.prevResult)
	}()

	lang, ok := ip.languages[parentLang]
	if !ok {
		return nil, fmt.Errorf("injection: language %q not registered", parentLang)
	}

	parser := ip.getParser(parentLang, lang)
	newTree, err := parser.ParseIncremental(source, oldResult.Tree)
	if err != nil {
		return nil, fmt.Errorf("injection: incremental parsing %q: %w", parentLang, err)
	}

	// Determine which ranges changed between old and new parent trees.
	changedRanges := DiffChangedRanges(oldResult.Tree, newTree)

	injections, err := ip.findAndReuseInjections(source, parentLang, newTree, 0, oldResult, changedRanges)
	if err != nil {
		return nil, err
	}

	ip.prevResult = &InjectionResult{
		Tree:       newTree,
		Injections: injections,
	}

	return ip.prevResult, nil
}

// ParseIncrementalUTF16 re-parses UTF-16 source after edits, reusing unchanged
// child trees. Call oldResult.Tree.EditUTF16 before calling this.
func (ip *InjectionParser) ParseIncrementalUTF16(source []uint16, parentLang string,
	oldResult *UTF16InjectionResult) (*UTF16InjectionResult, error) {

	if oldResult == nil {
		return ip.ParseUTF16(source, parentLang)
	}
	utf8Source, sourceMap := encodeUTF16ToUTF8WithMap(source)
	byteResult := oldResult.byteResult
	if byteResult == nil {
		byteResult = oldResult.toByteResult()
	}
	result, err := ip.ParseIncremental(utf8Source, parentLang, byteResult)
	if err != nil {
		return nil, err
	}
	attachUTF16SourceToInjectionResult(result, source, sourceMap)
	return ip.utf16InjectionResult(result)
}

// ParseIncrementalUTF16Bytes is like ParseIncrementalUTF16 for endian-specific
// UTF-16 bytes.
func (ip *InjectionParser) ParseIncrementalUTF16Bytes(source []byte, parentLang string,
	oldResult *UTF16InjectionResult, order UTF16ByteOrder) (*UTF16InjectionResult, error) {

	units, err := DecodeUTF16Bytes(source, order)
	if err != nil {
		return nil, err
	}
	return ip.ParseIncrementalUTF16(units, parentLang, oldResult)
}

// defaultMaxInjectionDepth limits recursion to prevent infinite loops.
const defaultMaxInjectionDepth = 10

// SetMaxDepth overrides the nested injection recursion limit.
// Depth values <= 0 restore the default limit.
func (ip *InjectionParser) SetMaxDepth(depth int) {
	if ip == nil {
		return
	}
	if depth <= 0 {
		ip.maxDepth = 0
		return
	}
	ip.maxDepth = depth
}

func (ip *InjectionParser) effectiveMaxDepth() int {
	if ip == nil || ip.maxDepth <= 0 {
		return defaultMaxInjectionDepth
	}
	return ip.maxDepth
}

// findAndParseInjections detects injections in tree and parses them, recursing.
func (ip *InjectionParser) findAndParseInjections(source []byte, parentLang string,
	tree *Tree, depth int) ([]Injection, error) {

	if depth >= ip.effectiveMaxDepth() {
		return nil, nil
	}

	detected, err := ip.detectInjections(source, parentLang, tree)
	if err != nil {
		return nil, err
	}

	result := make([]Injection, 0, len(detected))
	for _, det := range detected {
		if det.Language == "" {
			result = append(result, det)
			continue
		}

		childLang, ok := ip.languages[det.Language]
		if !ok {
			// Language not registered — record injection without tree.
			result = append(result, det)
			continue
		}

		childParser := ip.getParser(det.Language, childLang)

		childTree, err := ip.parseInjectionRegion(source, childParser, det.Ranges)
		if err != nil {
			result = append(result, det)
			continue
		}
		det.Tree = childTree

		// Recurse: check if this child language has injection queries too.
		if _, hasQuery := ip.injectionQueries[det.Language]; hasQuery {
			nested, err := ip.findAndParseInjections(source, det.Language, childTree, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, det)
			result = append(result, nested...)
		} else {
			result = append(result, det)
		}
	}

	return result, nil
}

// parseInjectionRegion parses one detected injection's byte ranges with
// childParser. For a single-range injection (the common case), it parses
// only the range bytes via ParseIncremental(rangeBytes, nil). This lets the
// parser use an incremental-class arena (16 KB slab vs 2 MB for full parse),
// which is orders of magnitude cheaper when there are many small
// injections. It rebases the resulting tree back into document coordinates
// so callers still see the same byte/point space as the included-range
// path. A multi-range injection, or a single-range parse that fails or
// stops early, falls back to the full-source path with SetIncludedRanges,
// since the lexer needs non-contiguous byte ranges.
//
// findAndParseInjections and findAndReuseInjections both call this so a
// freshly-parsed injection takes the same route regardless of which one
// found it.
func (ip *InjectionParser) parseInjectionRegion(source []byte, childParser *Parser, ranges []Range) (*Tree, error) {
	if len(ranges) == 1 {
		r := ranges[0]
		if r.StartByte <= r.EndByte && int(r.EndByte) <= len(source) {
			rangeSource := source[r.StartByte:r.EndByte]
			childTree, err := childParser.ParseIncremental(rangeSource, nil)
			if err == nil && childTree != nil && !childTree.ParseStoppedEarly() {
				rebaseInjectionTree(childTree, source, r)
				return childTree, nil
			}
		}
	}
	childParser.SetIncludedRanges(ranges)
	return childParser.Parse(source)
}

// injectionRangesOverlapChanged reports whether any of ranges overlaps any
// of changedRanges.
func injectionRangesOverlapChanged(ranges, changedRanges []Range) bool {
	for _, cr := range changedRanges {
		for _, r := range ranges {
			if r.StartByte < cr.EndByte && r.EndByte > cr.StartByte {
				return true
			}
		}
	}
	return false
}

// findAndReuseInjections mirrors findAndParseInjections's detection,
// per-region parse route, and recursion into nested injection queries, but
// reuses an old child tree instead of reparsing wherever a region's byte
// range does not overlap changedRanges and oldResult has a matching old
// injection (same language and ranges). This keeps ParseIncremental's
// injection tree the same shape as a fresh Parse of the same edited source,
// while still avoiding redundant reparses of untouched regions.
func (ip *InjectionParser) findAndReuseInjections(source []byte, parentLang string,
	tree *Tree, depth int, oldResult *InjectionResult, changedRanges []Range) ([]Injection, error) {

	if depth >= ip.effectiveMaxDepth() {
		return nil, nil
	}

	detected, err := ip.detectInjections(source, parentLang, tree)
	if err != nil {
		return nil, err
	}

	result := make([]Injection, 0, len(detected))
	for _, det := range detected {
		if det.Language == "" {
			result = append(result, det)
			continue
		}

		childLang, ok := ip.languages[det.Language]
		if !ok {
			// Language not registered — record injection without tree.
			result = append(result, det)
			continue
		}

		var childTree *Tree
		if oldResult != nil && !injectionRangesOverlapChanged(det.Ranges, changedRanges) {
			childTree = ip.findOldInjection(oldResult, det.Language, det.Ranges)
		}

		if childTree == nil {
			childParser := ip.getParser(det.Language, childLang)
			childTree, err = ip.parseInjectionRegion(source, childParser, det.Ranges)
			if err != nil {
				// If child parse fails, record injection without tree.
				result = append(result, det)
				continue
			}
		}
		det.Tree = childTree

		// Recurse: check if this child language has injection queries too.
		if _, hasQuery := ip.injectionQueries[det.Language]; hasQuery {
			nested, err := ip.findAndReuseInjections(source, det.Language, childTree, depth+1, oldResult, changedRanges)
			if err != nil {
				return nil, err
			}
			result = append(result, det)
			result = append(result, nested...)
		} else {
			result = append(result, det)
		}
	}

	return result, nil
}

// detectInjections runs the injection query for parentLang against tree
// and returns detected injection regions (without parsing them).
func (ip *InjectionParser) detectInjections(source []byte, parentLang string,
	tree *Tree) ([]Injection, error) {

	q, ok := ip.injectionQueries[parentLang]
	if !ok {
		return nil, nil
	}

	lang := ip.languages[parentLang]
	root := tree.RootNode()
	if root == nil {
		return nil, nil
	}

	cursor := q.Exec(root, lang, source)

	// Collect injection regions grouped by language.
	type injectionEntry struct {
		language string
		ranges   []Range
		node     *Node
	}
	var entries []injectionEntry

	for {
		match, ok := cursor.NextMatch()
		if !ok {
			break
		}

		var contentNode *Node
		var langName string

		// Check for static language from #set! injection.language.
		if vals := match.SetValues(q, "injection.language"); len(vals) > 0 {
			langName = vals[0]
		}

		for _, cap := range match.Captures {
			switch cap.Name {
			case "injection.content":
				contentNode = cap.Node
			case "injection.language":
				// Dynamic language detection: node text is the language name.
				if langName == "" && cap.Node != nil {
					langName = strings.TrimSpace(cap.Node.Text(source))
				}
			}
		}

		if contentNode == nil {
			continue
		}

		entries = append(entries, injectionEntry{
			language: langName,
			ranges:   []Range{contentNode.Range()},
			node:     contentNode,
		})
	}

	// Convert entries to Injection structs.
	injections := make([]Injection, len(entries))
	for i, e := range entries {
		injections[i] = Injection{
			Language: e.language,
			Ranges:   e.ranges,
			Node:     e.node,
		}
	}

	return injections, nil
}

// findOldInjection searches oldResult for a matching injection by language and ranges.
func (ip *InjectionParser) findOldInjection(oldResult *InjectionResult, lang string, ranges []Range) *Tree {
	for _, old := range oldResult.Injections {
		if old.Language != lang || old.Tree == nil || len(old.Ranges) != len(ranges) {
			continue
		}
		match := true
		for i, r := range old.Ranges {
			if r != ranges[i] {
				match = false
				break
			}
		}
		if match {
			return old.Tree
		}
	}
	return nil
}

func (ip *InjectionParser) utf16InjectionResult(result *InjectionResult) (*UTF16InjectionResult, error) {
	if result == nil {
		return nil, nil
	}
	out := &UTF16InjectionResult{
		Tree:       result.Tree,
		Injections: make([]UTF16Injection, 0, len(result.Injections)),
		byteResult: result,
	}
	for _, inj := range result.Injections {
		converted := UTF16Injection{
			Language: inj.Language,
			Tree:     inj.Tree,
			Node:     inj.Node,
		}
		if len(inj.Ranges) > 0 {
			converted.Ranges = make([]UTF16Range, 0, len(inj.Ranges))
			for _, r := range inj.Ranges {
				utf16Range, ok := result.Tree.UTF16RangeForRange(r)
				if !ok {
					return nil, ErrInvalidUTF16Range
				}
				converted.Ranges = append(converted.Ranges, utf16Range)
			}
		}
		out.Injections = append(out.Injections, converted)
	}
	return out, nil
}

func attachUTF16SourceToInjectionResult(result *InjectionResult, source []uint16, sourceMap *utf16SourceMap) {
	if result == nil {
		return
	}
	attachUTF16Source(result.Tree, source, sourceMap)
	for _, inj := range result.Injections {
		attachUTF16Source(inj.Tree, source, sourceMap)
	}
}

func (r *UTF16InjectionResult) toByteResult() *InjectionResult {
	if r == nil {
		return nil
	}
	out := &InjectionResult{
		Tree:       r.Tree,
		Injections: make([]Injection, 0, len(r.Injections)),
	}
	if r.Tree == nil || r.Tree.utf16Map == nil {
		return out
	}
	sourceMap := r.Tree.utf16Map
	for _, inj := range r.Injections {
		byteInjection := Injection{
			Language: inj.Language,
			Tree:     inj.Tree,
			Node:     inj.Node,
		}
		for _, rng := range inj.Ranges {
			startByte, ok := r.Tree.UTF8ByteForUTF16Offset(rng.StartCodeUnit)
			if !ok {
				continue
			}
			endByte, ok := r.Tree.UTF8ByteForUTF16Offset(rng.EndCodeUnit)
			if !ok {
				continue
			}
			startPoint, ok := sourceMap.pointForUTF8Byte(startByte)
			if !ok {
				continue
			}
			endPoint, ok := sourceMap.pointForUTF8Byte(endByte)
			if !ok {
				continue
			}
			byteInjection.Ranges = append(byteInjection.Ranges, Range{
				StartByte:  startByte,
				EndByte:    endByte,
				StartPoint: startPoint,
				EndPoint:   endPoint,
			})
		}
		out.Injections = append(out.Injections, byteInjection)
	}
	return out
}

func rebaseInjectionTree(tree *Tree, source []byte, span Range) {
	if tree == nil {
		return
	}
	if tree.root != nil {
		if !shiftNodeBytes(tree.root, int64(span.StartByte)) {
			tree.root = tree.RootNodeWithOffset(span.StartByte, span.StartPoint)
		} else {
			shiftNodePoints(tree.root, span.StartPoint)
		}
	}
	tree.source = source

	rt := tree.ParseRuntime()
	rt.SourceLen = uint32(len(source))
	rt.ExpectedEOFByte = addUint32Delta(rt.ExpectedEOFByte, int64(span.StartByte))
	if rt.LastTokenEndByte != 0 {
		rt.LastTokenEndByte = addUint32Delta(rt.LastTokenEndByte, int64(span.StartByte))
	}
	if tree.root != nil {
		rt.RootEndByte = tree.root.EndByte()
	} else {
		rt.RootEndByte = span.StartByte
	}
	rt.Truncated = rt.RootEndByte < rt.ExpectedEOFByte
	tree.setParseRuntime(rt)
}

func shiftNodePoints(root *Node, offset Point) {
	if root == nil || offset == (Point{}) {
		return
	}
	baseRow := root.startPoint.Row
	stack := []*Node{root}
	for len(stack) > 0 {
		last := len(stack) - 1
		n := stack[last]
		stack = stack[:last]

		startRow := n.startPoint.Row
		n.startPoint.Row = addUint32Delta(n.startPoint.Row, int64(offset.Row))
		if offset.Row == 0 || startRow == baseRow {
			n.startPoint.Column = addUint32Delta(n.startPoint.Column, int64(offset.Column))
		}

		endRow := n.endPoint.Row
		n.endPoint.Row = addUint32Delta(n.endPoint.Row, int64(offset.Row))
		if offset.Row == 0 || endRow == baseRow {
			n.endPoint.Column = addUint32Delta(n.endPoint.Column, int64(offset.Column))
		}

		childCount := nodeChildCountNoMaterialize(n)
		for i := 0; i < childCount; i++ {
			if child := nodeChildAtForReason(n, i, materializeForParentAPI); child != nil {
				stack = append(stack, child)
			}
		}
	}
}

// getParser returns a cached Parser for the language, creating one if needed.
func (ip *InjectionParser) getParser(name string, lang *Language) *Parser {
	if p, ok := ip.parsers[name]; ok {
		// Reset included ranges for fresh use.
		p.SetIncludedRanges(nil)
		return p
	}
	p := NewParser(lang)
	// Injection child parsers build injection subtrees the admission scorecard
	// never validated: keep them on production regardless of the global default.
	p.pinToProductionRoute()
	p.SetTimeoutMicros(ip.timeoutMicros)
	p.SetCancellationFlag(ip.cancellationFlag)
	ip.parsers[name] = p
	return p
}
