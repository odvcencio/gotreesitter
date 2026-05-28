package gotreesitter

import (
	"sort"
	"sync"
)

// AmbiguityProfile aggregates parser states/lookaheads that contribute to GLR
// fanout. It is intended for diagnostics and benchmark runs, not normal API use.
type AmbiguityProfile struct {
	mu            sync.Mutex
	entries       map[AmbiguityKey]*AmbiguityStat
	reduceEntries map[AmbiguityKey]*AmbiguityStat
	chainEntries  map[AmbiguityKey]*AmbiguityStat
	mergeEntries  map[StateID]*AmbiguityStat
}

// AmbiguityKey identifies one parse-table ambiguity bucket.
type AmbiguityKey struct {
	State                          StateID
	Lookahead                      Symbol
	ActionCount                    uint8
	ShiftCount                     uint8
	ReduceCount                    uint8
	ReduceSymbol                   Symbol
	ChildCount                     uint8
	ProductionID                   uint16
	ReduceChainTerminalState       StateID
	ReduceChainTerminalActionClass uint8
}

// AmbiguityStat is a snapshot row from AmbiguityProfile.
type AmbiguityStat struct {
	State                          StateID
	Lookahead                      Symbol
	ActionCount                    uint8
	ShiftCount                     uint8
	ReduceCount                    uint8
	ReduceSymbol                   Symbol
	ChildCount                     uint8
	ProductionID                   uint16
	Actions                        []ParseAction
	Hits                           uint64
	Forks                          uint64
	MultiStackHits                 uint64
	StackInTotal                   uint64
	StackInMax                     int
	ReduceChainHits                uint64
	ReduceChainSteps               uint64
	ReduceChainMaxLen              int
	ReduceChainNanos               int64
	ReduceChainRuns                uint64
	ReduceChainClassHits           uint64
	ReduceChainStopNoAction        uint64
	ReduceChainStopMulti           uint64
	ReduceChainStopShift           uint64
	ReduceChainStopAccept          uint64
	ReduceChainStopDead            uint64
	ReduceChainStopCycle           uint64
	ReduceChainStopLimit           uint64
	ReduceChainTerminalState       StateID
	ReduceChainTerminalActionClass uint8
	ActionNanos                    int64
	ExtraShiftNanos                int64
	NoActionNanos                  int64
	ConflictChoiceNanos            int64
	ConflictForkNanos              int64
	SingleShiftNanos               int64
	SingleReduceNanos              int64
	SingleAcceptNanos              int64
	SingleRecoverNanos             int64
	SingleOtherNanos               int64
	MergeCalls                     uint64
	MergeStacksIn                  uint64
	MergeStacksOut                 uint64
	MergeStacksInMax               int
	MergeStacksOutMax              int
}

type ambiguityActionTimingKind uint8

const (
	ambiguityActionExtraShift ambiguityActionTimingKind = iota + 1
	ambiguityActionNoAction
	ambiguityActionConflictChoice
	ambiguityActionConflictFork
	ambiguityActionSingleShift
	ambiguityActionSingleReduce
	ambiguityActionSingleAccept
	ambiguityActionSingleRecover
	ambiguityActionSingleOther
)

type reduceChainStopReason uint8

const (
	reduceChainStopNoAction reduceChainStopReason = iota + 1
	reduceChainStopMulti
	reduceChainStopShift
	reduceChainStopAccept
	reduceChainStopDead
	reduceChainStopCycle
	reduceChainStopLimit
)

// NewAmbiguityProfile creates an empty GLR ambiguity profile.
func NewAmbiguityProfile() *AmbiguityProfile {
	return &AmbiguityProfile{
		entries:       make(map[AmbiguityKey]*AmbiguityStat),
		reduceEntries: make(map[AmbiguityKey]*AmbiguityStat),
		chainEntries:  make(map[AmbiguityKey]*AmbiguityStat),
		mergeEntries:  make(map[StateID]*AmbiguityStat),
	}
}

// Reset clears all accumulated ambiguity counters.
func (p *AmbiguityProfile) Reset() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.entries)
	clear(p.reduceEntries)
	clear(p.chainEntries)
	clear(p.mergeEntries)
}

// SnapshotTop returns the highest-impact ambiguity buckets ordered by stack
// pressure, then hit count.
func (p *AmbiguityProfile) SnapshotTop(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.entries))
	for _, stat := range p.entries {
		copied := *stat
		if len(stat.Actions) > 0 {
			copied.Actions = append([]ParseAction(nil), stat.Actions...)
		}
		out = append(out, copied)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].ActionNanos != out[j].ActionNanos && (out[i].ActionNanos != 0 || out[j].ActionNanos != 0) {
			return out[i].ActionNanos > out[j].ActionNanos
		}
		if out[i].StackInTotal != out[j].StackInTotal {
			return out[i].StackInTotal > out[j].StackInTotal
		}
		if out[i].Hits != out[j].Hits {
			return out[i].Hits > out[j].Hits
		}
		if out[i].StackInMax != out[j].StackInMax {
			return out[i].StackInMax > out[j].StackInMax
		}
		if out[i].State != out[j].State {
			return out[i].State < out[j].State
		}
		return out[i].Lookahead < out[j].Lookahead
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SnapshotTopReduceChains returns the parser states/lookaheads that spent the
// most time in deterministic reduce-chain fusion.
func (p *AmbiguityProfile) SnapshotTopReduceChains(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.reduceEntries))
	for _, stat := range p.reduceEntries {
		out = append(out, *stat)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].ReduceChainNanos != out[j].ReduceChainNanos && (out[i].ReduceChainNanos != 0 || out[j].ReduceChainNanos != 0) {
			return out[i].ReduceChainNanos > out[j].ReduceChainNanos
		}
		if out[i].ReduceChainSteps != out[j].ReduceChainSteps {
			return out[i].ReduceChainSteps > out[j].ReduceChainSteps
		}
		if out[i].ReduceChainHits != out[j].ReduceChainHits {
			return out[i].ReduceChainHits > out[j].ReduceChainHits
		}
		if out[i].ReduceChainMaxLen != out[j].ReduceChainMaxLen {
			return out[i].ReduceChainMaxLen > out[j].ReduceChainMaxLen
		}
		if out[i].State != out[j].State {
			return out[i].State < out[j].State
		}
		return out[i].Lookahead < out[j].Lookahead
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SnapshotTopReduceChainRuns returns the starting states/lookaheads that begin
// the most expensive deterministic reduce chains.
func (p *AmbiguityProfile) SnapshotTopReduceChainRuns(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.chainEntries))
	for _, stat := range p.chainEntries {
		out = append(out, *stat)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].ReduceChainNanos != out[j].ReduceChainNanos && (out[i].ReduceChainNanos != 0 || out[j].ReduceChainNanos != 0) {
			return out[i].ReduceChainNanos > out[j].ReduceChainNanos
		}
		if out[i].ReduceChainSteps != out[j].ReduceChainSteps {
			return out[i].ReduceChainSteps > out[j].ReduceChainSteps
		}
		if out[i].ReduceChainRuns != out[j].ReduceChainRuns {
			return out[i].ReduceChainRuns > out[j].ReduceChainRuns
		}
		if out[i].ReduceChainMaxLen != out[j].ReduceChainMaxLen {
			return out[i].ReduceChainMaxLen > out[j].ReduceChainMaxLen
		}
		if out[i].State != out[j].State {
			return out[i].State < out[j].State
		}
		return out[i].Lookahead < out[j].Lookahead
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// SnapshotReduceChainTotals returns aggregate deterministic reduce-chain run
// counters across all profiled start states/lookaheads.
func (p *AmbiguityProfile) SnapshotReduceChainTotals() AmbiguityStat {
	if p == nil {
		return AmbiguityStat{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	var out AmbiguityStat
	for _, stat := range p.chainEntries {
		out.ReduceChainRuns += stat.ReduceChainRuns
		out.ReduceChainSteps += stat.ReduceChainSteps
		out.ReduceChainClassHits += stat.ReduceChainClassHits
		out.ReduceChainNanos += stat.ReduceChainNanos
		if stat.ReduceChainMaxLen > out.ReduceChainMaxLen {
			out.ReduceChainMaxLen = stat.ReduceChainMaxLen
		}
		out.ReduceChainStopNoAction += stat.ReduceChainStopNoAction
		out.ReduceChainStopMulti += stat.ReduceChainStopMulti
		out.ReduceChainStopShift += stat.ReduceChainStopShift
		out.ReduceChainStopAccept += stat.ReduceChainStopAccept
		out.ReduceChainStopDead += stat.ReduceChainStopDead
		out.ReduceChainStopCycle += stat.ReduceChainStopCycle
		out.ReduceChainStopLimit += stat.ReduceChainStopLimit
	}
	return out
}

// SnapshotTopMergeStates returns parser states that most often participate in
// multi-stack merge passes. These rows are keyed by state only, because merge
// happens before the next lookahead dispatch.
func (p *AmbiguityProfile) SnapshotTopMergeStates(limit int) []AmbiguityStat {
	if p == nil || limit == 0 {
		return nil
	}
	p.mu.Lock()
	out := make([]AmbiguityStat, 0, len(p.mergeEntries))
	for _, stat := range p.mergeEntries {
		out = append(out, *stat)
	}
	p.mu.Unlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].MergeStacksIn != out[j].MergeStacksIn {
			return out[i].MergeStacksIn > out[j].MergeStacksIn
		}
		if out[i].MergeCalls != out[j].MergeCalls {
			return out[i].MergeCalls > out[j].MergeCalls
		}
		if out[i].MergeStacksOut != out[j].MergeStacksOut {
			return out[i].MergeStacksOut > out[j].MergeStacksOut
		}
		return out[i].State < out[j].State
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (p *AmbiguityProfile) record(state StateID, lookahead Symbol, actions []ParseAction, stackCount int) {
	if p == nil || (len(actions) <= 1 && stackCount <= 1) {
		return
	}
	var shifts, reduces int
	for _, action := range actions {
		switch action.Type {
		case ParseActionShift:
			shifts++
		case ParseActionReduce:
			reduces++
		}
	}
	key := AmbiguityKey{
		State:       state,
		Lookahead:   lookahead,
		ActionCount: saturatingUint8(len(actions)),
		ShiftCount:  saturatingUint8(shifts),
		ReduceCount: saturatingUint8(reduces),
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries == nil {
		p.entries = make(map[AmbiguityKey]*AmbiguityStat)
	}
	stat := p.entries[key]
	if stat == nil {
		stat = &AmbiguityStat{
			State:       state,
			Lookahead:   lookahead,
			ActionCount: key.ActionCount,
			ShiftCount:  key.ShiftCount,
			ReduceCount: key.ReduceCount,
			Actions:     append([]ParseAction(nil), actions...),
		}
		p.entries[key] = stat
	}
	stat.Hits++
	if len(actions) > 1 {
		stat.Forks++
	}
	if stackCount > 1 {
		stat.MultiStackHits++
	}
	if stackCount > stat.StackInMax {
		stat.StackInMax = stackCount
	}
	if stackCount > 0 {
		stat.StackInTotal += uint64(stackCount)
	}
}

func (p *AmbiguityProfile) recordActionTiming(state StateID, lookahead Symbol, actions []ParseAction, kind ambiguityActionTimingKind, nanos int64) {
	if p == nil || nanos <= 0 {
		return
	}
	var shifts, reduces int
	for _, action := range actions {
		switch action.Type {
		case ParseActionShift:
			shifts++
		case ParseActionReduce:
			reduces++
		}
	}
	key := AmbiguityKey{
		State:       state,
		Lookahead:   lookahead,
		ActionCount: saturatingUint8(len(actions)),
		ShiftCount:  saturatingUint8(shifts),
		ReduceCount: saturatingUint8(reduces),
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.entries == nil {
		p.entries = make(map[AmbiguityKey]*AmbiguityStat)
	}
	stat := p.entries[key]
	if stat == nil {
		stat = &AmbiguityStat{
			State:       state,
			Lookahead:   lookahead,
			ActionCount: key.ActionCount,
			ShiftCount:  key.ShiftCount,
			ReduceCount: key.ReduceCount,
			Actions:     append([]ParseAction(nil), actions...),
		}
		p.entries[key] = stat
	}
	if len(actions) == 1 && actions[0].Type == ParseActionReduce {
		stat.ReduceSymbol = actions[0].Symbol
		stat.ChildCount = actions[0].ChildCount
		stat.ProductionID = actions[0].ProductionID
	}
	stat.ActionNanos += nanos
	switch kind {
	case ambiguityActionExtraShift:
		stat.ExtraShiftNanos += nanos
	case ambiguityActionNoAction:
		stat.NoActionNanos += nanos
	case ambiguityActionConflictChoice:
		stat.ConflictChoiceNanos += nanos
	case ambiguityActionConflictFork:
		stat.ConflictForkNanos += nanos
	case ambiguityActionSingleShift:
		stat.SingleShiftNanos += nanos
	case ambiguityActionSingleReduce:
		stat.SingleReduceNanos += nanos
	case ambiguityActionSingleAccept:
		stat.SingleAcceptNanos += nanos
	case ambiguityActionSingleRecover:
		stat.SingleRecoverNanos += nanos
	case ambiguityActionSingleOther:
		stat.SingleOtherNanos += nanos
	}
}

func (p *AmbiguityProfile) recordReduceChainStep(state StateID, lookahead Symbol, act ParseAction, chainLen int, nanos int64) {
	if p == nil {
		return
	}
	key := AmbiguityKey{
		State:        state,
		Lookahead:    lookahead,
		ActionCount:  1,
		ReduceCount:  1,
		ReduceSymbol: act.Symbol,
		ChildCount:   act.ChildCount,
		ProductionID: act.ProductionID,
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.reduceEntries == nil {
		p.reduceEntries = make(map[AmbiguityKey]*AmbiguityStat)
	}
	stat := p.reduceEntries[key]
	if stat == nil {
		stat = &AmbiguityStat{
			State:        state,
			Lookahead:    lookahead,
			ActionCount:  key.ActionCount,
			ReduceCount:  key.ReduceCount,
			ReduceSymbol: act.Symbol,
			ChildCount:   act.ChildCount,
			ProductionID: act.ProductionID,
			Actions:      []ParseAction{act},
		}
		p.reduceEntries[key] = stat
	}
	stat.ReduceChainHits++
	stat.ReduceChainSteps++
	if nanos > 0 {
		stat.ReduceChainNanos += nanos
	}
	if chainLen > stat.ReduceChainMaxLen {
		stat.ReduceChainMaxLen = chainLen
	}
}

func (p *AmbiguityProfile) recordReduceChainRun(state StateID, lookahead Symbol, terminalState StateID, terminalClass classifiedParseActionClass, steps, maxLen, classHits int, nanos int64, stop reduceChainStopReason) {
	if p == nil {
		return
	}
	key := AmbiguityKey{
		State:                          state,
		Lookahead:                      lookahead,
		ActionCount:                    1,
		ReduceCount:                    1,
		ReduceChainTerminalState:       terminalState,
		ReduceChainTerminalActionClass: uint8(terminalClass),
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.chainEntries == nil {
		p.chainEntries = make(map[AmbiguityKey]*AmbiguityStat)
	}
	stat := p.chainEntries[key]
	if stat == nil {
		stat = &AmbiguityStat{
			State:                          state,
			Lookahead:                      lookahead,
			ActionCount:                    key.ActionCount,
			ReduceCount:                    key.ReduceCount,
			ReduceChainTerminalState:       terminalState,
			ReduceChainTerminalActionClass: uint8(terminalClass),
		}
		p.chainEntries[key] = stat
	}
	stat.ReduceChainRuns++
	stat.ReduceChainSteps += uint64(max(steps, 0))
	stat.ReduceChainClassHits += uint64(max(classHits, 0))
	if nanos > 0 {
		stat.ReduceChainNanos += nanos
	}
	if maxLen > stat.ReduceChainMaxLen {
		stat.ReduceChainMaxLen = maxLen
	}
	switch stop {
	case reduceChainStopNoAction:
		stat.ReduceChainStopNoAction++
	case reduceChainStopMulti:
		stat.ReduceChainStopMulti++
	case reduceChainStopShift:
		stat.ReduceChainStopShift++
	case reduceChainStopAccept:
		stat.ReduceChainStopAccept++
	case reduceChainStopDead:
		stat.ReduceChainStopDead++
	case reduceChainStopCycle:
		stat.ReduceChainStopCycle++
	case reduceChainStopLimit:
		stat.ReduceChainStopLimit++
	}
}

func (p *AmbiguityProfile) recordMergeBefore(stacks []glrStack) {
	if p == nil || len(stacks) <= 1 {
		return
	}
	counts := map[StateID]int{}
	for i := range stacks {
		if stacks[i].dead || len(stacks[i].entries) == 0 {
			continue
		}
		counts[stacks[i].top().state]++
	}
	if len(counts) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mergeEntries == nil {
		p.mergeEntries = make(map[StateID]*AmbiguityStat)
	}
	for state, count := range counts {
		stat := p.mergeEntries[state]
		if stat == nil {
			stat = &AmbiguityStat{State: state}
			p.mergeEntries[state] = stat
		}
		stat.MergeCalls++
		stat.MergeStacksIn += uint64(count)
		if count > stat.MergeStacksInMax {
			stat.MergeStacksInMax = count
		}
	}
}

func (p *AmbiguityProfile) recordMergeAfter(stacks []glrStack) {
	if p == nil || len(stacks) == 0 {
		return
	}
	counts := map[StateID]int{}
	for i := range stacks {
		if stacks[i].dead || len(stacks[i].entries) == 0 {
			continue
		}
		counts[stacks[i].top().state]++
	}
	if len(counts) == 0 {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mergeEntries == nil {
		p.mergeEntries = make(map[StateID]*AmbiguityStat)
	}
	for state, count := range counts {
		stat := p.mergeEntries[state]
		if stat == nil {
			stat = &AmbiguityStat{State: state}
			p.mergeEntries[state] = stat
		}
		stat.MergeStacksOut += uint64(count)
		if count > stat.MergeStacksOutMax {
			stat.MergeStacksOutMax = count
		}
	}
}

func saturatingUint8(n int) uint8 {
	if n <= 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return uint8(n)
}
