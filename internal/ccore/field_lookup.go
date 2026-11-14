package ccore

// FieldLookupPath resolves a field to visible child ordinals relative to parent.
// found distinguishes an absent field from an incomplete search. ok is false on
// invalid input or exhausted work/depth budget; callers must then decline.
// dst is caller-owned; capacity exhaustion declines. On failure its contents
// are unspecified. Internal search scratch is bounded to 64 raw path entries.
// The returned path aliases dst. The reader requires the tree to remain retained.
func (r NodeReader) FieldLookupPath(parent Node, field uint16, maxWork int, dst []uint32) (path []uint32, found, ok bool) {
	if r.S == nil || r.T == nil || r.T.Base == nil || parent.H == 0 || maxWork <= 0 {
		return nil, false, false
	}
	s := fieldPathSearch{r: r, remaining: maxWork}
	if field == 0 {
		return nil, false, true
	}
	var rawBuf [64]uint32
	raw, found := s.find(parent.H, field, rawBuf[:0], 0)
	if s.bad {
		return nil, false, false
	}
	if !found {
		return nil, false, true
	}
	p, good := s.project(parent.H, raw, dst[:0], 0)
	if s.bad || !good {
		return nil, false, false
	}
	return p, true, true
}

type fieldPathSearch struct {
	r         NodeReader
	remaining int
	bad       bool
}

func (s *fieldPathSearch) tick(depth int) bool {
	if depth >= 64 || s.remaining <= 0 {
		s.bad = true
		return false
	}
	s.remaining--
	return true
}
func (s *fieldPathSearch) entries(h Handle) ([]FieldEntry, bool) {
	p := s.r.S.ProductionID(h)
	if int(p) >= len(s.r.T.FieldSlices) {
		if p == 0 {
			return nil, true
		}
		s.bad = true
		return nil, false
	}
	sl := s.r.T.FieldSlices[p]
	end := int(sl[0]) + int(sl[1])
	if end > len(s.r.T.FieldEntries) {
		s.bad = true
		return nil, false
	}
	return s.r.T.FieldEntries[int(sl[0]):end], true
}
func (s *fieldPathSearch) relevant(h Handle, prod uint16, index int) bool {
	if s.r.S.Visible(h) {
		return true
	}
	return prod != 0 && index >= 0 && int(prod) < len(s.r.T.Base.Alias) && index < len(s.r.T.Base.Alias[prod]) && s.r.T.Base.Alias[prod][index] != 0
}
func (s *fieldPathSearch) first(h Handle, raw []uint32, depth int) ([]uint32, bool) {
	if !s.tick(depth) {
		return nil, false
	}
	prod := s.r.S.ProductionID(h)
	str := 0
	for j, c := range s.r.S.Children(h) {
		if !s.tick(depth) {
			return nil, false
		}
		i := -1
		if !s.r.S.Extra(c) {
			i = str
			str++
		}
		if len(raw) == cap(raw) {
			s.bad = true
			return nil, false
		}
		p := append(raw, uint32(j))
		if s.relevant(c, prod, i) {
			return p, true
		}
		if s.r.S.ChildCount(c) > 0 {
			if out, ok := s.first(c, p, depth+1); ok {
				return out, true
			}
			if s.bad {
				return nil, false
			}
		}
	}
	return nil, false
}
func (s *fieldPathSearch) find(h Handle, f uint16, raw []uint32, depth int) ([]uint32, bool) {
	if !s.tick(depth) {
		return nil, false
	}
	entries, ok := s.entries(h)
	if !ok {
		return nil, false
	}
	var storage [64]FieldEntry
	matches := storage[:0]
	var prior uint16
	for i, e := range entries {
		if !s.tick(depth) {
			return nil, false
		}
		if i > 0 && e.FieldID < prior {
			s.bad = true
			return nil, false
		}
		prior = e.FieldID
		if e.FieldID == f {
			if len(matches) > 0 && e.ChildIndex < matches[len(matches)-1].ChildIndex {
				s.bad = true
				return nil, false
			}
			if len(matches) == len(storage) {
				s.bad = true
				return nil, false
			}
			matches = append(matches, e)
		}
	}
	if len(matches) == 0 {
		return nil, false
	}
	prod := s.r.S.ProductionID(h)
	str, k := 0, 0
	for j, c := range s.r.S.Children(h) {
		if !s.tick(depth) {
			return nil, false
		}
		if s.r.S.Extra(c) {
			continue
		}
		idx := str
		str++
		if idx < int(matches[k].ChildIndex) {
			continue
		}
		if len(raw) == cap(raw) {
			s.bad = true
			return nil, false
		}
		p := append(raw, uint32(j))
		if matches[k].Inherited {
			if out, hit := s.find(c, f, p, depth+1); hit {
				return out, true
			}
			if s.bad {
				return nil, false
			}
			k++
			if k == len(matches) {
				return nil, false
			}
			continue
		}
		if s.relevant(c, prod, idx) {
			return p, true
		}
		if s.r.S.ChildCount(c) > 0 {
			if out, hit := s.first(c, p, depth+1); hit {
				return out, true
			}
			if s.bad {
				return nil, false
			}
		}
		k++
		if k == len(matches) {
			return nil, false
		}
	}
	if k < len(matches) && int(matches[k].ChildIndex) >= str {
		s.bad = true
	}
	return nil, false
}
func (s *fieldPathSearch) count(h Handle, depth int) (uint32, bool) {
	if !s.tick(depth) {
		return 0, false
	}
	var n uint32
	prod := s.r.S.ProductionID(h)
	str := 0
	for _, c := range s.r.S.Children(h) {
		if !s.tick(depth) {
			return 0, false
		}
		i := -1
		if !s.r.S.Extra(c) {
			i = str
			str++
		}
		if s.relevant(c, prod, i) {
			if n == ^uint32(0) {
				s.bad = true
				return 0, false
			}
			n++
		} else if s.r.S.ChildCount(c) > 0 {
			m, ok := s.count(c, depth+1)
			if !ok || m > ^uint32(0)-n {
				s.bad = true
				return 0, false
			}
			n += m
		}
	}
	return n, true
}
func (s *fieldPathSearch) locate(h Handle, raw []uint32, ordinal *uint32, depth int) (Handle, []uint32, bool) {
	if !s.tick(depth) || len(raw) == 0 {
		return 0, nil, false
	}
	prod := s.r.S.ProductionID(h)
	str := 0
	for j, c := range s.r.S.Children(h) {
		if !s.tick(depth) {
			return 0, nil, false
		}
		i := -1
		if !s.r.S.Extra(c) {
			i = str
			str++
		}
		rel := s.relevant(c, prod, i)
		if uint32(j) == raw[0] {
			if rel {
				return c, raw[1:], true
			}
			if len(raw) == 1 {
				return 0, nil, false
			}
			return s.locate(c, raw[1:], ordinal, depth+1)
		}
		if rel {
			if *ordinal == ^uint32(0) {
				s.bad = true
				return 0, nil, false
			}
			*ordinal++
		} else if s.r.S.ChildCount(c) > 0 {
			n, ok := s.count(c, depth+1)
			if !ok || n > ^uint32(0)-*ordinal {
				s.bad = true
				return 0, nil, false
			}
			*ordinal += n
		}
	}
	return 0, nil, false
}
func (s *fieldPathSearch) project(h Handle, raw, path []uint32, depth int) ([]uint32, bool) {
	for len(raw) > 0 {
		if !s.tick(depth) {
			return nil, false
		}
		var ordinal uint32
		next, rest, ok := s.locate(h, raw, &ordinal, depth+1)
		if !ok {
			return nil, false
		}
		if len(path) == cap(path) {
			s.bad = true
			return nil, false
		}
		path = append(path, ordinal)
		h, raw = next, rest
		depth++
	}
	return path, true
}
