package queryexec

// Expansion records unfinished greedy-run prefixes for capture iteration.
type Expansion struct {
	Run     uint32
	Matches []uint32
}

func (b *Budget) Track(expansion *Expansion) {
	b.expansion = expansion
	expansion.Run = 0
	expansion.Matches = expansion.Matches[:0]
}

func (b *Budget) SetExpansion(count uint32) uint32 {
	if b == nil || b.expansion == nil {
		return 0
	}
	previous := b.expansion.Run
	b.expansion.Run = count
	return previous
}

func (b *Budget) EmitMatch() {
	if b != nil && b.expansion != nil {
		b.expansion.Matches = append(b.expansion.Matches, b.expansion.Run)
	}
}
