//go:build cgo && treesitter_c_parity

package cgoharness

// C-side predicate reassociation for the outline C-oracle differential
// (outline_differential_test.go, spec.outline-api.v1 gate G-2).
//
// Root cause: a resolved tags query authored the way
// grammars/tags_query_infer.go writes elixir/commonlisp/scheme (clojure's
// own override happens to nest correctly already) writes predicates AFTER a
// pattern's own closing paren --
//
//	(call (identifier) @_head (arguments (alias) @name)) @definition.module (#eq? @_head "defmodule")
//
// -- instead of nested inside it. gotreesitter's own top-level parser
// intentionally tolerates that: a standalone top-level "(#...)" form always
// attaches as an extra predicate on the MOST RECENTLY completed top-level
// pattern (query_compile.go's parse loop, the "ch == '(' && next == '#'"
// branch), so gotreesitter compiles the line above into exactly ONE pattern
// carrying an #eq? predicate. The real C library (github.com/tree-sitter/
// go-tree-sitter's cgo binding around tree-sitter's own ts_query_new) has no
// such leniency: every top-level, paren-balanced S-expression -- predicate
// calls included -- is its OWN independent compiled pattern
// (ts_query_pattern_count / ts_query_predicates_for_pattern). A standalone
// "(#eq? @_head \"defmodule\")" pattern has no structural steps of its own,
// so it is unrooted and carries zero captures; its OWN capture-name lookups
// are always empty, which makes go-tree-sitter's built-in
// QueryMatch.SatisfiesTextPredicate vacuously true, and an unrooted,
// always-true pattern matches once at every eligible tree position. That is
// the entire gap: not that the C side skips predicate filtering (the
// go-tree-sitter binding already runs #eq?/#not-eq?/#any-eq?/#any-not-eq?/
// #match?/#not-match?/#any-match?/#any-not-match?/#any-of?/#not-any-of? as
// TextPredicates automatically, see QueryMatches.Next), but that the
// predicate ends up detached from the ONE compiled pattern that actually
// carries the "_head" capture, so it filters nothing and the phantom
// pattern's own noise inflates the raw match count (elixir: go_matches=5,
// c_matches=49 before this file).
//
// The fix reproduces gotreesitter's own top-level grouping rule using only
// public go-tree-sitter API (Query.PatternCount/StartByteForPattern/
// EndByteForPattern/TextPredicates/GeneralPredicates), re-attaches each
// trailing predicate-only pattern to the structural pattern immediately
// before it in source order, evaluates that predicate against the
// STRUCTURAL match's own captures (the ones that actually bind "_head"),
// and remaps the surviving structural pattern indices to the same dense,
// source-order numbering gotreesitter itself assigns
// (outlineDiffGoQueryMatches's PatternIndex). Phantom predicate-only
// patterns never become their own row in the compared capture stream.
//
// Every resolved tags query in this repository was grepped for predicate
// operator names (grammars/tags_query_infer.go, the only source of
// predicates in any registered ResolveTagsQuery output): only #eq? and
// #any-of? are used anywhere in the fleet today. This file still implements
// the small remainder of gotreesitter's predicate surface
// (#has-ancestor?/#has-parent?/#count?/#is-exported?, and the #strip!
// directive) because they are cheap, direct ports of query_predicates.go's
// own algorithms and this differential's fleet census runs across the
// entire language registry under GTS_PARITY_MODE=exhaustive -- silently
// no-op'ing a predicate kind gotreesitter actually filters on would trade
// today's false "diverged" for a future false "equal". #lua-match? is the
// one exception: gotreesitter compiles Lua pattern syntax through a
// dedicated translator (compileLuaPattern, query_compile_predicates.go)
// before evaluating it as a Go regexp, and no query anywhere uses it today,
// so a phantom #lua-match? predicate fails the comparison loudly (skipped,
// with a detail string) rather than risk a silently wrong translation.

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// outlineCPredicateGroup is one gotreesitter-equivalent top-level pattern:
// the C-compiled structural pattern index that actually carries captures,
// plus every trailing top-level "(#...)" pattern index gotreesitter's own
// parser would fold into it. ordinal is the dense, 0-based, source-order
// index that lines up with the PatternIndex outlineDiffGoQueryMatches
// records for the same query.
type outlineCPredicateGroup struct {
	structuralIdx int
	ordinal       int
	predicateIdx  []int
}

// outlineCPredicateGroups scans a compiled C query's own pattern source
// spans (Query.StartByteForPattern/EndByteForPattern) and reproduces
// gotreesitter's top-level grouping rule (query_compile.go: a standalone
// "(#...)" pattern always attaches to the most recently completed
// preceding pattern) using only that span's text: a pattern whose trimmed
// source starts with "(#" is a predicate-only pattern; everything else
// starts a new group. tagsQuery must be the exact string both engines
// compiled (ResolveTagsQuery's output), since pattern byte offsets are
// relative to it.
func outlineCPredicateGroups(q *sitter.Query, tagsQuery string) ([]outlineCPredicateGroup, error) {
	count := int(q.PatternCount())
	groups := make([]outlineCPredicateGroup, 0, count)
	lastOrdinal := -1

	for i := 0; i < count; i++ {
		start := q.StartByteForPattern(uint(i))
		end := q.EndByteForPattern(uint(i))
		if int(start) > len(tagsQuery) || int(end) > len(tagsQuery) || start > end {
			return nil, fmt.Errorf("pattern %d span [%d,%d) out of range for a %d-byte query", i, start, end, len(tagsQuery))
		}
		span := strings.TrimSpace(tagsQuery[start:end])

		if strings.HasPrefix(span, "(#") {
			if lastOrdinal < 0 {
				return nil, fmt.Errorf("predicate pattern %d (%s) precedes any structural pattern", i, outlineDiffTruncate(span, 40))
			}
			groups[lastOrdinal].predicateIdx = append(groups[lastOrdinal].predicateIdx, i)
			continue
		}

		groups = append(groups, outlineCPredicateGroup{structuralIdx: i, ordinal: len(groups)})
		lastOrdinal = len(groups) - 1
	}

	return groups, nil
}

// outlineDiffCQueryMatchesWithPredicates is outlineDiffCQueryMatches's
// predicate-aware counterpart: it runs the same C query cursor, drops every
// phantom predicate-only match (see the file comment), and, for a
// structural match, applies every predicate gotreesitter itself would have
// folded into that same pattern before deciding whether the match survives.
// Surviving matches carry the dense, gotreesitter-equivalent PatternIndex
// (outlineCPredicateGroup.ordinal), making the result directly comparable
// (reflect.DeepEqual) to outlineDiffGoQueryMatches's output for the
// identical query text. An error means a predicate form this evaluator does
// not implement was found on a phantom pattern; the caller must treat that
// as "cannot compare," never as silent equality or a fabricated divergence.
func outlineDiffCQueryMatchesWithPredicates(q *sitter.Query, tree *sitter.Tree, source []byte, tagsQuery string) ([]exactQueryMatch, error) {
	groups, err := outlineCPredicateGroups(q, tagsQuery)
	if err != nil {
		return nil, err
	}
	groupByStructuralIdx := make(map[int]outlineCPredicateGroup, len(groups))
	for _, g := range groups {
		groupByStructuralIdx[g.structuralIdx] = g
	}

	names := q.CaptureNames()
	cursor := sitter.NewQueryCursor()
	defer cursor.Close()
	iter := cursor.Matches(q, tree.RootNode(), source)

	var matches []exactQueryMatch
	for {
		m := iter.Next()
		if m == nil {
			break
		}
		group, isStructural := groupByStructuralIdx[int(m.PatternIndex)]
		if !isStructural {
			// A trailing top-level "(#...)" pattern: no captures of its own,
			// no counterpart PatternIndex on the Go side. Never its own row.
			continue
		}

		ok, err := outlineCMatchSatisfiesPredicates(q, group, m, source)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		snap := exactQueryMatch{PatternIndex: group.ordinal}
		for _, c := range m.Captures {
			name := ""
			if int(c.Index) < len(names) {
				name = names[c.Index]
			}
			start := uint32(c.Node.StartByte())
			end := uint32(c.Node.EndByte())
			snap.Captures = append(snap.Captures, exactQueryCapture{
				Name:      name,
				Type:      c.Node.Kind(),
				Named:     c.Node.IsNamed(),
				StartByte: start,
				EndByte:   end,
				Text:      string(source[start:end]),
			})
		}
		outlineCApplyStripDirectives(q, group, &snap)
		matches = append(matches, snap)
	}
	return matches, nil
}

// outlineCMatchSatisfiesPredicates mirrors gotreesitter's
// Query.matchesPredicates (query_predicates.go) using the C library's own
// compiled predicate data for every phantom pattern folded into group,
// evaluated against the STRUCTURAL match's captures (the ones that actually
// bind the predicate's capture references).
func outlineCMatchSatisfiesPredicates(q *sitter.Query, group outlineCPredicateGroup, m *sitter.QueryMatch, source []byte) (bool, error) {
	for _, predIdx := range group.predicateIdx {
		// #eq?/#not-eq?/#any-eq?/#any-not-eq?/#match?/#not-match?/
		// #any-match?/#any-not-match?/#any-of?/#not-any-of? are already
		// parsed by the C library into Query.TextPredicates. Evaluate them
		// with a synthetic match that borrows the phantom pattern's own
		// predicate definition (TextPredicates is indexed by PatternIndex)
		// but the structural match's real captures -- the exact function
		// QueryMatches.Next() would already have used had the compiler kept
		// this predicate on the structural pattern instead of splitting it
		// into its own unrooted, zero-capture pattern (see the file
		// comment).
		if len(q.TextPredicates[predIdx]) > 0 {
			synthetic := sitter.QueryMatch{Captures: m.Captures, PatternIndex: uint(predIdx)}
			if !synthetic.SatisfiesTextPredicate(q, nil, nil, source) {
				return false, nil
			}
		}

		for _, gp := range q.GeneralPredicates(uint(predIdx)) {
			ok, err := outlineCGeneralPredicateSatisfied(gp, m, source)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
	}
	return true, nil
}

// outlineCGeneralPredicateSatisfied evaluates one predicate the C binding
// did not classify as a TextPredicate (Query.GeneralPredicates), matching
// query_predicates.go's matchesPredicateWithReader case for the same
// predicate kind. gotreesitter's own compiler (query_compile_predicates.go)
// already rejects any query using a predicate name outside this set before
// this differential ever reaches C-side comparison, so this switch is
// exhaustive over what can arrive here.
func outlineCGeneralPredicateSatisfied(gp sitter.QueryPredicate, m *sitter.QueryMatch, source []byte) (bool, error) {
	switch gp.Operator {
	case "has-ancestor?", "not-has-ancestor?", "has-parent?", "not-has-parent?":
		return outlineCAncestorOrParentPredicateSatisfied(gp, m), nil
	case "count?":
		return outlineCCountPredicateSatisfied(gp, m), nil
	case "is-exported?":
		return outlineCIsExportedPredicateSatisfied(gp, m, source), nil
	case "offset!", "select-adjacent!", "strip!":
		// Directives, not filters: query_predicates.go's
		// predicatesCanRejectMatch excludes exactly these (plus #set!/#is?/
		// #is-not?, which the C binding classifies as property
		// predicates/settings and never surfaces as a GeneralPredicate at
		// all). #strip! rewrites captured TEXT after a match already
		// survives filtering; see outlineCApplyStripDirectives.
		// #select-adjacent! narrows the captures a match reports but never
		// rejects the match itself; no resolved tags query uses it today,
		// so it is intentionally not reproduced beyond "never reject."
		return true, nil
	case "lua-match?":
		return false, fmt.Errorf("#lua-match? predicate has no C-side evaluator in this differential (gotreesitter translates Lua pattern syntax before matching; verified unused by every resolved tags query today)")
	default:
		return false, fmt.Errorf("unsupported predicate operator %q on a phantom C-compiled pattern", gp.Operator)
	}
}

// outlineCAncestorOrParentPredicateSatisfied ports
// query_predicates.go's ancestorPredicateMatchesWithReader /
// parentPredicateMatchesWithReader / captureNodePredicateMatchesWithReader:
// the predicate FAILS outright if its capture never occurs in the match
// (not merely "allowMissing" the way the text-equality predicates behave),
// otherwise it succeeds once ANY occurrence's parent chain (has-ancestor?)
// or immediate parent (has-parent?) contains a listed type name, XORed with
// negation for the not-* forms. Node-type matching here is a direct Kind()
// string comparison; gotreesitter's supertype-aware match
// (nodeTypeMatchesAnyWithReader) is not reproduced since no resolved tags
// query anywhere uses either predicate today (a real future use that relies
// on supertype matching would under-match here, which fails toward a
// visible "diverged" row, never a silently wrong "equal").
func outlineCAncestorOrParentPredicateSatisfied(gp sitter.QueryPredicate, m *sitter.QueryMatch) bool {
	leftCapture, values, ok := outlineCCaptureAndValues(gp)
	if !ok {
		return true
	}
	negated := gp.Operator == "not-has-ancestor?" || gp.Operator == "not-has-parent?"
	ancestorMode := gp.Operator == "has-ancestor?" || gp.Operator == "not-has-ancestor?"

	found, matched := false, false
	for _, c := range m.Captures {
		if uint(c.Index) != leftCapture {
			continue
		}
		found = true
		node := c.Node
		if ancestorMode {
			for p := node.Parent(); p != nil; p = p.Parent() {
				if stringInListC(p.Kind(), values) {
					matched = true
					break
				}
			}
		} else if p := node.Parent(); p != nil && stringInListC(p.Kind(), values) {
			matched = true
		}
		if matched {
			break
		}
	}
	return found && (matched != negated)
}

// outlineCCountPredicateSatisfied ports countPredicateMatchesWithReader:
// count every capture in the match bound to the predicate's capture name,
// compare against the literal operand with the literal operator.
func outlineCCountPredicateSatisfied(gp sitter.QueryPredicate, m *sitter.QueryMatch) bool {
	if len(gp.Args) != 3 || gp.Args[0].CaptureId == nil || gp.Args[1].String == nil || gp.Args[2].String == nil {
		return true
	}
	leftCapture := *gp.Args[0].CaptureId
	op := *gp.Args[1].String
	value, err := strconv.Atoi(*gp.Args[2].String)
	if err != nil {
		return true
	}

	count := 0
	for _, c := range m.Captures {
		if uint(c.Index) == leftCapture {
			count++
		}
	}
	switch op {
	case ">":
		return count > value
	case "<":
		return count < value
	case ">=":
		return count >= value
	case "<=":
		return count <= value
	case "==":
		return count == value
	case "!=":
		return count != value
	default:
		return false
	}
}

// outlineCIsExportedPredicateSatisfied ports query_predicates.go's
// #is-exported? handling: fails if the capture is absent (no allowMissing),
// otherwise true iff the FIRST capture's raw source text starts with an
// upper-case rune (textIsExported).
func outlineCIsExportedPredicateSatisfied(gp sitter.QueryPredicate, m *sitter.QueryMatch, source []byte) bool {
	if len(gp.Args) != 1 || gp.Args[0].CaptureId == nil {
		return true
	}
	leftCapture := *gp.Args[0].CaptureId
	for _, c := range m.Captures {
		if uint(c.Index) != leftCapture {
			continue
		}
		text := string(source[c.Node.StartByte():c.Node.EndByte()])
		return outlineCTextIsExported(text)
	}
	return false
}

func outlineCTextIsExported(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	return r != utf8.RuneError && unicode.IsUpper(r)
}

// outlineCApplyStripDirectives ports applyStripWithReader: for every
// #strip! predicate folded into group, replace every match of its regex
// with "" in the raw text of every capture bound to its target capture
// name. Runs only after the match has already survived
// outlineCMatchSatisfiesPredicates, matching gotreesitter's own order
// (directives apply after filtering, never before -- Query.matchPatternAll).
func outlineCApplyStripDirectives(q *sitter.Query, group outlineCPredicateGroup, snap *exactQueryMatch) {
	names := q.CaptureNames()
	for _, predIdx := range group.predicateIdx {
		for _, gp := range q.GeneralPredicates(uint(predIdx)) {
			if gp.Operator != "strip!" || len(gp.Args) != 2 || gp.Args[0].CaptureId == nil || gp.Args[1].String == nil {
				continue
			}
			rx, err := regexp.Compile(*gp.Args[1].String)
			if err != nil {
				continue
			}
			leftName := ""
			if id := *gp.Args[0].CaptureId; int(id) < len(names) {
				leftName = names[id]
			}
			for i := range snap.Captures {
				if snap.Captures[i].Name != leftName {
					continue
				}
				if stripped := rx.ReplaceAllString(snap.Captures[i].Text, ""); stripped != snap.Captures[i].Text {
					snap.Captures[i].Text = stripped
				}
			}
		}
	}
}

// outlineCCaptureAndValues extracts a "(#pred? @capture value ...)"-shaped
// general predicate's left capture id and literal value list. ok is false
// for a malformed arg list, which gotreesitter's own compiler would already
// have rejected before this differential's C-side comparison ever runs.
func outlineCCaptureAndValues(gp sitter.QueryPredicate) (leftCapture uint, values []string, ok bool) {
	if len(gp.Args) < 2 || gp.Args[0].CaptureId == nil {
		return 0, nil, false
	}
	for _, a := range gp.Args[1:] {
		if a.String != nil {
			values = append(values, *a.String)
		}
	}
	return *gp.Args[0].CaptureId, values, true
}

func stringInListC(value string, values []string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}
