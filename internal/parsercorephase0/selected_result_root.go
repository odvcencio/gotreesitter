package parsercorephase0

import "errors"

// normalizeResultRootSourceStart implements public root padding ownership for
// authenticated result framing. A retained leading child can own trivia;
// otherwise the root starts at the first source byte that is not padding.
// A gap before all retained records could instead be a lexer-owned skipped
// prefix. That provenance is not represented here, so decline rather than
// extend the root without its proof.
func (s *SelectedStore) normalizeResultRootSourceStart(policy SelectedStorePolicy, source []byte) error {
	if !policy.ProjectSyntheticSiblingRoot || len(source) == 0 {
		return nil
	}
	root := &s.records[s.root-1]
	first := selectedResultFirstSourceByte(source)
	if first == root.StartByte {
		return nil
	}
	if first < root.StartByte {
		return errors.New("parser-core phase zero: selected root source-prefix ownership is outside admitted scope")
	}
	if first == 0 || root.ChildCount == 0 {
		return nil
	}
	child := s.records[s.children[root.FirstChild]-1]
	if child.StartByte == first {
		root.StartByte = first
	}
	return nil
}

// Match the public framing rule's UTF-8 BOM and ASCII padding treatment.
// All-padding input retains the established zero start.
func selectedResultFirstSourceByte(source []byte) uint32 {
	start := 0
	if len(source) >= 3 && source[0] == 0xef && source[1] == 0xbb && source[2] == 0xbf {
		start = 3
	}
	for index := start; index < len(source); index++ {
		switch source[index] {
		case ' ', '\t', '\n', '\r', '\f':
			continue
		default:
			return uint32(index)
		}
	}
	return 0
}
