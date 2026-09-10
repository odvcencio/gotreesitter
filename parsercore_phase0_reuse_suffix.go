//go:build !gts_no_parsercorephase0

package gotreesitter

// prepareSuffixProof authenticates every byte from the common suffix through EOF.
// One backward scan covers all later candidates. Polling bounds cancellation delay.
func (s *compactIncrementalReuseSession) prepareSuffixProof(poll func() error) error {
	if s.suffixReady {
		return nil
	}
	if s.oldTree == nil || s.scheduler == nil || s.scheduler.tokenSource == nil ||
		!s.scheduler.tokenSource.compactReuseForwardDependenciesOnly() ||
		len(s.oldTree.includedRanges) != 0 || len(s.scheduler.options.includedRanges) != 0 ||
		s.oldTree.sourceEncoding != InputEncodingUTF8 {
		s.suffixReady = true
		return nil
	}
	oldEnd, newEnd := len(s.cursor.oldSource), len(s.cursor.newSource)
	steps := 0
	for oldEnd > 0 && newEnd > 0 && s.cursor.oldSource[oldEnd-1] == s.cursor.newSource[newEnd-1] {
		if steps&4095 == 0 && poll != nil {
			if err := poll(); err != nil {
				return err
			}
		}
		oldEnd--
		newEnd--
		steps++
	}
	if poll != nil {
		if err := poll(); err != nil {
			return err
		}
	}
	s.suffixOldStart, s.suffixNewStart = uint32(oldEnd), uint32(newEnd)
	s.suffixValid = true
	s.suffixReady = true
	return nil
}

// suffixNodeEligible includes all forward lexer and reduction lookahead.
// Reverse mapping binds the retained node to the authenticated old suffix.
func (s *compactIncrementalReuseSession) suffixNodeEligible(p *Parser, n *Node) bool {
	if !s.suffixReady || !s.suffixValid || n == nil || s.oldTree == nil ||
		n == s.oldTree.root || n.ChildCount() == 0 || !n.isCompactMaterialized() ||
		p == nil || p.language == nil || uint32(n.symbol) < p.language.TokenCount || !p.isVisibleSymbol(n.symbol) ||
		!compactNodeStateProofAvailable(n) || n.StartByte() < s.suffixNewStart ||
		n.EndByte() > uint32(len(s.cursor.newSource)) || n.EndByte() <= n.StartByte() {
		return false
	}
	oldStart, ok := s.cursor.oldByteForNew(n.StartByte())
	if !ok || uint64(oldStart) != uint64(s.suffixOldStart)+uint64(n.StartByte()-s.suffixNewStart) {
		return false
	}
	oldEnd, ok := s.cursor.oldByteForNew(n.EndByte())
	return ok && uint64(oldEnd) == uint64(s.suffixOldStart)+uint64(n.EndByte()-s.suffixNewStart)
}

func (s *compactIncrementalReuseSession) nodeMayBeReused(p *Parser, n *Node) bool {
	if compactNodeMayBeReused(n) {
		return true
	}
	return s.suffixNodeEligible(p, n) && !compactNodeRecoveryBearingWithAncestors(n, false)
}
