package ccore

// FieldEdgeAnswer preserves both the ordinary and cursor field of one visible
// child occurrence. Zero is an explicit answer, not an absence marker.
type FieldEdgeAnswer struct{ Field, CursorField uint16 }

// FieldEdgeOf reads the ordinary and cursor fields of this exact occurrence.
func FieldEdgeOf(n Node) FieldEdgeAnswer { return FieldEdgeAnswer{n.Field, n.CField} }

// FieldLookupAnswer records a requested field's answer. Found=false is an
// explicit miss. A found path occupies PathStart:PathEnd in the path buffer.
type FieldLookupAnswer struct {
	Field              uint16
	Found              bool
	PathStart, PathEnd int
}

// FieldAnswers snapshots answers for this parent occurrence into caller-owned
// buffers. The reader and its store must remain retained and unchanged during
// the call. It does not authenticate mutable tables or cache handle identities.
// A false result declines the whole snapshot; buffer contents are unspecified.
// maxWork bounds traversal and field-entry visits, then divides the remaining
// budget equally across lookups. The result aliases caller buffers.
func (r NodeReader) FieldAnswers(parent Node, fields []uint16, maxWork int, edges []FieldEdgeAnswer, lookups []FieldLookupAnswer, paths []uint32) ([]FieldEdgeAnswer, []FieldLookupAnswer, []uint32, bool) {
	if r.S == nil || r.T == nil || r.T.Base == nil || parent.H == 0 || maxWork <= 0 {
		return nil, nil, nil, false
	}
	count := r.ChildCount(parent)
	if uint64(count) > uint64(cap(edges)) || len(fields) > cap(lookups) {
		return nil, nil, nil, false
	}
	edges = edges[:int(count)]
	lookups = lookups[:len(fields)]
	paths = paths[:0]
	// A missing production-zero slice is valid and denotes no fields. All
	// other missing or out-of-range slices make the snapshot incomplete.
	validProduction := func(h Handle) bool {
		prod := r.S.ProductionID(h)
		if int(prod) >= len(r.T.FieldSlices) {
			return prod == 0
		}
		sl := r.T.FieldSlices[prod]
		return int(sl[0])+int(sl[1]) <= len(r.T.FieldEntries)
	}
	if !validProduction(parent.H) {
		return nil, nil, nil, false
	}
	type frame struct {
		it                childIter
		inherited, cursor uint16
	}
	var stack [64]frame
	stack[0] = frame{it: r.iterate(parent)}
	depth, used, out := 1, 0, 0
	for depth > 0 {
		top := &stack[depth-1]
		if top.it.idx >= top.it.count {
			depth--
			continue
		}
		if used >= maxWork {
			return nil, nil, nil, false
		}
		used++
		var child Node
		if !top.it.next(&child) {
			return nil, nil, nil, false
		}
		visible := r.relevant(&child, true)
		if !visible && r.relevantCount(&child, true) == 0 {
			continue
		}
		var field uint16
		if !r.S.extra(child.H) || !visible {
			prod := top.it.prod
			if int(prod) < len(r.T.FieldSlices) {
				sl := r.T.FieldSlices[prod]
				entries := int(sl[1])
				if entries > maxWork-used {
					return nil, nil, nil, false
				}
				used += entries
			}
			field = r.ownField(prod, int(top.it.structural)-1)
		}
		if visible {
			if out >= len(edges) {
				return nil, nil, nil, false
			}
			if !r.S.extra(child.H) {
				child.Field, child.CField = field, field
				if field == 0 {
					child.Field, child.CField = top.inherited, top.cursor
				}
			}
			edges[out] = FieldEdgeOf(child)
			out++
			continue
		}
		inherited, cursor := top.inherited, top.cursor
		if field != 0 {
			inherited = field
		}
		if r.S.extra(child.H) {
			cursor = 0
		} else if field != 0 {
			cursor = field
		}
		if depth == len(stack) || !validProduction(child.H) {
			return nil, nil, nil, false
		}
		stack[depth] = frame{it: r.iterate(child), inherited: inherited, cursor: cursor}
		depth++
	}
	if out != len(edges) {
		return nil, nil, nil, false
	}
	remaining := maxWork - used
	if len(fields) > remaining {
		return nil, nil, nil, false
	}
	perLookup := 0
	if len(fields) > 0 {
		perLookup = remaining / len(fields)
	}
	for i, field := range fields {
		start := len(paths)
		path, found, ok := r.FieldLookupPath(parent, field, perLookup, paths[start:])
		if !ok {
			return nil, nil, nil, false
		}
		paths = paths[:start+len(path)]
		lookups[i] = FieldLookupAnswer{field, found, start, len(paths)}
	}
	return edges, lookups, paths, true
}
