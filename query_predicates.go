package gotreesitter

import (
	"unicode"
	"unicode/utf8"
)

func (q *Query) matchesPredicates(predicates []QueryPredicate, captures []QueryCapture, lang *Language, source []byte) bool {
	return matchesPredicatesWithReader(q, predicates, captures, lang, source, publicQueryReader{})
}

func (q *Query) predicatesStillViable(predicates []QueryPredicate, captures []QueryCapture, source []byte) bool {
	return predicatesStillViableWithReader(q, predicates, captures, source, publicQueryReader{})
}

func (q *Query) applyDirectives(predicates []QueryPredicate, captures []QueryCapture, source []byte) []QueryCapture {
	return applyDirectivesWithReader(q, predicates, captures, source, publicQueryReader{})
}

func matchesPredicatesWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, predicates []QueryPredicate, captures []C, lang *Language, source []byte, reader R) bool {
	for _, pred := range predicates {
		if !matchesPredicateWithReader(q, pred, captures, lang, source, reader) {
			return false
		}
	}
	return true
}

func matchesPredicateWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, pred QueryPredicate, captures []C, lang *Language, source []byte, reader R) bool {
	switch pred.kind {
	case predicateEq:
		return textEqualityPredicateMatchesWithReader(pred, captures, source, true, reader)
	case predicateNotEq:
		return textEqualityPredicateMatchesWithReader(pred, captures, source, false, reader)
	case predicateMatch, predicateLuaMatch:
		return regexPredicateMatchesWithReader(pred, captures, source, false, reader)
	case predicateNotMatch:
		return regexPredicateMatchesWithReader(pred, captures, source, true, reader)
	case predicateAnyEq:
		return anyCaptureTextEqualsWithReader(pred, captures, source, true, reader)
	case predicateAnyNotEq:
		return anyCaptureTextEqualsWithReader(pred, captures, source, false, reader)
	case predicateAnyMatch:
		return anyCaptureRegexMatchesWithReader(pred, captures, source, false, reader)
	case predicateAnyNotMatch:
		return anyCaptureRegexMatchesWithReader(pred, captures, source, true, reader)
	case predicateAnyOf:
		left, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
		return (ok && stringInList(left, pred.values)) || (!ok && pred.allowMissing)
	case predicateNotAnyOf:
		left, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
		return (ok && !stringInList(left, pred.values)) || (!ok && pred.allowMissing)
	case predicateHasAncestor:
		return ancestorPredicateMatchesWithReader(pred, captures, lang, false, reader)
	case predicateNotHasAncestor:
		return ancestorPredicateMatchesWithReader(pred, captures, lang, true, reader)
	case predicateHasParent:
		return parentPredicateMatchesWithReader(pred, captures, lang, false, reader)
	case predicateNotHasParent:
		return parentPredicateMatchesWithReader(pred, captures, lang, true, reader)
	case predicateIs, predicateIsNot:
		return true
	case predicateCount:
		return countPredicateMatchesWithReader(pred, captures, reader)
	case predicateIsExported:
		text, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
		return ok && textIsExported(text)
	case predicateSet, predicateOffset, predicateSelectAdjacent, predicateStrip:
		return true
	default:
		return false
	}
}

func predicatesStillViableWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, predicates []QueryPredicate, captures []C, source []byte, reader R) bool {
	for _, pred := range predicates {
		switch pred.kind {
		case predicateEq, predicateNotEq:
			left, leftOK := captureTextWithReader(pred.leftCapture, captures, source, reader)
			right, rightOK := predicateRightTextWithReader(pred, captures, source, reader)
			if leftOK && rightOK && ((left == right) != (pred.kind == predicateEq)) {
				return false
			}
		case predicateMatch, predicateLuaMatch, predicateNotMatch:
			left, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
			if ok {
				matched := pred.regex != nil && pred.regex.MatchString(left)
				if matched != (pred.kind != predicateNotMatch) {
					return false
				}
			}
		case predicateAnyOf, predicateNotAnyOf:
			left, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
			if ok && (stringInList(left, pred.values) != (pred.kind == predicateAnyOf)) {
				return false
			}
		case predicateIsExported:
			text, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
			if ok && !textIsExported(text) {
				return false
			}
		}
	}
	return true
}

func predicatesCanRejectMatch(predicates []QueryPredicate) bool {
	for _, pred := range predicates {
		switch pred.kind {
		case predicateSet, predicateOffset, predicateSelectAdjacent, predicateStrip, predicateIs, predicateIsNot:
		default:
			return true
		}
	}
	return false
}

func applyDirectivesWithReader[N comparable, C any, R queryNodeReader[N, C]](q *Query, predicates []QueryPredicate, captures []C, source []byte, reader R) []C {
	for _, pred := range predicates {
		switch pred.kind {
		case predicateSelectAdjacent:
			captures = applySelectAdjacentWithReader(pred, captures, reader)
		case predicateStrip:
			captures = applyStripWithReader(pred, captures, source, reader)
		}
	}
	return captures
}

type captureBoundary struct{ start, end uint32 }

func applySelectAdjacent(pred QueryPredicate, captures []QueryCapture) []QueryCapture {
	return applySelectAdjacentWithReader(pred, captures, publicQueryReader{})
}

func applySelectAdjacentWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, reader R) []C {
	var inline [8]captureBoundary
	anchors := inline[:0]
	for _, capture := range captures {
		node := reader.CaptureNode(capture)
		if reader.CaptureName(capture) == pred.rightCapture && !reader.IsNil(node) {
			anchors = append(anchors, captureBoundary{reader.StartByte(node), reader.EndByte(node)})
		}
	}
	out := captures[:0]
	for _, capture := range captures {
		if reader.CaptureName(capture) != pred.leftCapture {
			out = append(out, capture)
			continue
		}
		node := reader.CaptureNode(capture)
		adjacent := false
		if !reader.IsNil(node) {
			start, end := reader.StartByte(node), reader.EndByte(node)
			for _, anchor := range anchors {
				if end == anchor.start || start == anchor.end {
					adjacent = true
					break
				}
			}
		}
		if adjacent {
			out = append(out, capture)
		}
	}
	return out
}

func applyStrip(pred QueryPredicate, captures []QueryCapture, source []byte) []QueryCapture {
	return applyStripWithReader(pred, captures, source, publicQueryReader{})
}

func applyStripWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, reader R) []C {
	if pred.regex == nil {
		return captures
	}
	for i := range captures {
		node := reader.CaptureNode(captures[i])
		if reader.CaptureName(captures[i]) != pred.leftCapture || reader.IsNil(node) {
			continue
		}
		text := reader.Text(node, source)
		if stripped := pred.regex.ReplaceAllString(text, ""); stripped != text {
			reader.SetCaptureTextOverride(&captures[i], stripped)
		}
	}
	return captures
}

func predicateRightTextWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, reader R) (string, bool) {
	if pred.rightCapture == "" {
		return pred.literal, true
	}
	return captureTextWithReader(pred.rightCapture, captures, source, reader)
}

func textEqualityPredicateMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, wantEqual bool, reader R) bool {
	left, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
	if !ok {
		return pred.allowMissing
	}
	right, ok := predicateRightTextWithReader(pred, captures, source, reader)
	return ok && ((left == right) == wantEqual)
}

func regexPredicateMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, negated bool, reader R) bool {
	left, ok := captureTextWithReader(pred.leftCapture, captures, source, reader)
	if !ok {
		return pred.allowMissing
	}
	matched := pred.regex != nil && pred.regex.MatchString(left)
	return matched != negated
}

func anyCaptureTextEqualsWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, wantEqual bool, reader R) bool {
	right, rightOK := predicateRightTextWithReader(pred, captures, source, reader)
	found := false
	for _, capture := range captures {
		node := reader.CaptureNode(capture)
		if reader.CaptureName(capture) != pred.leftCapture || reader.IsNil(node) {
			continue
		}
		found = true
		if rightOK && ((reader.Text(node, source) == right) == wantEqual) {
			return true
		}
	}
	return !found && pred.allowMissing
}

func anyCaptureRegexMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, source []byte, negated bool, reader R) bool {
	found := false
	for _, capture := range captures {
		node := reader.CaptureNode(capture)
		if reader.CaptureName(capture) != pred.leftCapture || reader.IsNil(node) {
			continue
		}
		found = true
		if pred.regex == nil {
			continue
		}
		matched := pred.regex != nil && pred.regex.MatchString(reader.Text(node, source))
		if matched != negated {
			return true
		}
	}
	return !found && pred.allowMissing
}

func ancestorPredicateMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, lang *Language, negated bool, reader R) bool {
	return captureNodePredicateMatchesWithReader(pred.leftCapture, captures, negated, reader, func(node N) bool {
		for parent, ok := reader.Parent(node); ok; parent, ok = reader.Parent(parent) {
			if nodeTypeMatchesAnyWithReader(parent, pred.values, lang, reader) {
				return true
			}
		}
		return false
	})
}

func parentPredicateMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, lang *Language, negated bool, reader R) bool {
	return captureNodePredicateMatchesWithReader(pred.leftCapture, captures, negated, reader, func(node N) bool {
		parent, ok := reader.Parent(node)
		return ok && nodeTypeMatchesAnyWithReader(parent, pred.values, lang, reader)
	})
}

func captureNodePredicateMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](name string, captures []C, negated bool, reader R, matches func(N) bool) bool {
	found, matched := false, false
	for _, capture := range captures {
		node := reader.CaptureNode(capture)
		if reader.CaptureName(capture) != name || reader.IsNil(node) {
			continue
		}
		found = true
		if matches(node) {
			matched = true
			break
		}
	}
	return found && (matched != negated)
}

func countPredicateMatchesWithReader[N comparable, C any, R queryNodeReader[N, C]](pred QueryPredicate, captures []C, reader R) bool {
	count := 0
	for _, capture := range captures {
		if reader.CaptureName(capture) == pred.leftCapture && !reader.IsNil(reader.CaptureNode(capture)) {
			count++
		}
	}
	switch pred.countOp {
	case ">":
		return count > pred.countValue
	case "<":
		return count < pred.countValue
	case ">=":
		return count >= pred.countValue
	case "<=":
		return count <= pred.countValue
	case "==":
		return count == pred.countValue
	case "!=":
		return count != pred.countValue
	default:
		return false
	}
}

func captureTextWithReader[N comparable, C any, R queryNodeReader[N, C]](name string, captures []C, source []byte, reader R) (string, bool) {
	if source == nil {
		return "", false
	}
	for _, capture := range captures {
		if reader.CaptureName(capture) != name {
			continue
		}
		if override := reader.CaptureTextOverride(capture); override != "" {
			return override, true
		}
		node := reader.CaptureNode(capture)
		if reader.IsNil(node) {
			return "", false
		}
		return reader.Text(node, source), true
	}
	return "", false
}

func stringInList(value string, values []string) bool {
	for _, item := range values {
		if value == item {
			return true
		}
	}
	return false
}

func textIsExported(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	return r != utf8.RuneError && unicode.IsUpper(r)
}

func typeNameMatchesAny(typeName string, names []string) bool {
	for _, name := range names {
		if name == typeName {
			return true
		}
	}
	return false
}

func nodeTypeMatchesAnyWithReader[N comparable, C any, R queryNodeReader[N, C]](node N, names []string, lang *Language, reader R) bool {
	if reader.IsNil(node) || lang == nil {
		return false
	}
	if typeNameMatchesAny(reader.Type(node, lang), names) {
		return true
	}
	internal := reader.Symbol(node)
	public := lang.PublicSymbol(internal)
	for _, name := range names {
		if nodeSymbolMatchesTypeName(internal, public, name, lang) {
			return true
		}
	}
	return false
}

func nodeSymbolMatchesTypeName(nodeInternal, nodePublic Symbol, typeName string, lang *Language) bool {
	symbol, ok := lang.SymbolByName(typeName)
	if !ok {
		return false
	}
	if nodeInternal == symbol || nodePublic == symbol {
		return true
	}
	if !lang.IsSupertype(symbol) {
		return false
	}
	for _, child := range lang.SupertypeChildren(symbol) {
		if child == nodeInternal || lang.PublicSymbol(child) == nodePublic {
			return true
		}
	}
	return false
}
