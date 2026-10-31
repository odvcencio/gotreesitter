package incr

// ReuseActionMemo caches only immutable action-table facts for one parse.
// The caller must clear it when the parse or language changes and must still
// evaluate every node-specific FirstLeaf condition on each reuse candidate.
type ReuseActionMemo struct {
	state                       uint32
	symbol                      uint16
	valid, hasActions, reusable bool
}

func (m *ReuseActionMemo) Lookup(state uint32, symbol uint16) (hasActions, reusable, ok bool) {
	if !m.valid || m.state != state || m.symbol != symbol {
		return false, false, false
	}
	return m.hasActions, m.reusable, true
}

func (m *ReuseActionMemo) Store(state uint32, symbol uint16, hasActions, reusable bool) {
	*m = ReuseActionMemo{state: state, symbol: symbol, valid: true, hasActions: hasActions, reusable: reusable}
}
