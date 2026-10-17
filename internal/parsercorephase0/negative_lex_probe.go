package parsercorephase0

// NegativeLexProbeMemo holds failed external-token probes for one frozen
// election. The scheduler clears it when that election snapshot changes and
// authenticates the scanner/checkpoint before consuming a hit. No token,
// scanner payload, tree, or input reference is retained.
// A slot collision only discards a reusable result.
type NegativeLexProbeMemo struct {
	slots [16]negativeLexProbe
	valid uint16
}
type negativeLexProbe struct {
	start, span, produced uint32
	state                 StateID
}

func (m *NegativeLexProbeMemo) Clear() { m.valid = 0 }
func (m *NegativeLexProbeMemo) Lookup(state StateID, start uint32) (span, produced uint32, ok bool) {
	i := state & 15
	e := &m.slots[i]
	if m.valid&(1<<i) == 0 || e.state != state || e.start != start {
		return 0, 0, false
	}
	return e.span, e.produced, true
}
func (m *NegativeLexProbeMemo) Record(state StateID, start, span, produced uint32) {
	i := state & 15
	m.slots[i] = negativeLexProbe{start: start, span: span, produced: produced, state: state}
	m.valid |= 1 << i
}

// CaptureLexProbeReadSpan separates this probe's read dependency from earlier
// reads. The aggregate is restored monotonically, including on panic.
func CaptureLexProbeReadSpan[T any](span *uint32, probe func() T) (value T, observed uint32) {
	before := *span
	*span = 0
	defer func() { *span = max(before, *span) }()
	value = probe()
	return value, *span
}
