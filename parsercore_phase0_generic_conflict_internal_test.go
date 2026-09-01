//go:build gts_parsercorephase0

package gotreesitter

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	core "github.com/odvcencio/gotreesitter/internal/parsercorephase0"
)

func TestDiagnosticParserCoreGenericWorkSaturatesCausalCounters(t *testing.T) {
	work := DiagnosticParserCoreGenericWork{
		ConflictActionArmsAdmitted: math.MaxUint64 - 1,
		CausalConflictForks:        math.MaxUint64 - 2,
		RepetitionFolds:            math.MaxUint64,
	}
	work.add(&work.ConflictActionArmsAdmitted, 2)
	work.add(&work.CausalConflictForks, 3)
	work.add(&work.RepetitionFolds, 1)
	if work.ConflictActionArmsAdmitted != math.MaxUint64 || work.CausalConflictForks != math.MaxUint64 ||
		work.RepetitionFolds != math.MaxUint64 || !work.Overflow {
		t.Fatalf("scheduler causal saturation=%+v", work)
	}
}

func TestDiagnosticParserCoreGenericRepetitionFoldMatchesProductionScope(t *testing.T) {
	actions := core.NewActionRow([]core.Action{
		{Type: core.ActionShift, State: 2, Repetition: true},
		{Type: core.ActionReduce, Symbol: 4},
	}, false)
	if ordinal, ok := diagnosticParserCoreRepetitionFoldOrdinal(&Language{Name: "bash"}, actions); !ok || ordinal != 1 {
		t.Fatalf("bash repetition fold ordinal=%d ok=%t, want 1 true", ordinal, ok)
	}
	if _, ok := diagnosticParserCoreRepetitionFoldOrdinal(&Language{Name: "dart"}, actions); ok {
		t.Fatal("dart repetition fold bypassed the production opt-out")
	}
	if _, ok := diagnosticParserCoreRepetitionFoldOrdinal(&Language{Name: "markdown_inline"}, actions); ok {
		t.Fatal("Markdown Inline repetition fold bypassed the compact frontier opt-out")
	}
	if _, ok := diagnosticParserCoreRepetitionFoldOrdinal(nil, actions); ok {
		t.Fatal("nil-language repetition fold was admitted")
	}
	malformed := []core.ActionRow{
		core.NewActionRow([]core.Action{{Type: core.ActionShift, State: 2, Repetition: true}}, false),
		core.NewActionRow([]core.Action{
			{Type: core.ActionShift, State: 2},
			{Type: core.ActionReduce, Symbol: 4},
		}, false),
		core.NewActionRow([]core.Action{
			{Type: core.ActionShift, State: 2, Repetition: true},
			{Type: core.ActionReduce, Symbol: 4},
			{Type: core.ActionReduce, Symbol: 5},
		}, false),
	}
	for index, row := range malformed {
		if _, ok := diagnosticParserCoreRepetitionFoldOrdinal(&Language{Name: "bash"}, row); ok {
			t.Fatalf("malformed repetition row %d was admitted", index)
		}
	}
}

func TestDiagnosticParserCoreGenericRepetitionFoldReducesWithoutFork(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 9}: {
				{Type: core.ActionShift, State: 3, Repetition: true},
				{Type: core.ActionReduce, Symbol: 4},
			},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 4}: 2,
		},
	}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: &dfaTokenSource{language: &Language{Name: "bash"}},
		headers:     []diagnosticParserCoreHeader{{head: head}},
		token:       Token{Symbol: 9, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 8,
			ReceiptMode:   DiagnosticParserCoreReceiptFull,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil {
		t.Fatalf("repetition fold stop=%+v err=%v", stop, err)
	}
	state, _, err := compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if state != 2 || scheduler.dispatches != 1 || scheduler.work.RepetitionFolds != 1 ||
		scheduler.work.Reductions != 1 || scheduler.work.Conflicts != 0 || scheduler.work.Forks != 0 {
		t.Fatalf("repetition fold state=%d dispatches=%d work=%+v", state, scheduler.dispatches, scheduler.work)
	}
	if len(scheduler.receipt.Rounds) != 1 || len(scheduler.receipt.Rounds[0].Actions) != 1 {
		t.Fatalf("repetition fold rounds=%+v", scheduler.receipt.Rounds)
	}
	action := scheduler.receipt.Rounds[0].Actions[0]
	if action.Ordinal != 1 || action.Action.Type != ParseActionReduce {
		t.Fatalf("repetition fold action=%+v, want reduction ordinal 1", action)
	}
	derivations, err := compact.Derivations(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if len(derivations) != 1 || len(derivations[0].Payloads) != 1 {
		t.Fatalf("repetition fold derivations=%+v, want one reduced payload", derivations)
	}
	view, err := compact.MaterializationView(derivations[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if !view.Fragile {
		t.Fatal("repetition fold reduction lost its conflict fragility")
	}
}

func TestDiagnosticParserCoreGenericRepetitionForkMatchesProductionOptOut(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 9}: {
				{Type: core.ActionShift, State: 3, Repetition: true},
				{Type: core.ActionReduce, Symbol: 4},
			},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 4}: 2,
		},
	}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: &dfaTokenSource{language: &Language{Name: "haskell"}},
		headers:     []diagnosticParserCoreHeader{{head: head}},
		token:       Token{Symbol: 9, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 8,
			ReceiptMode:   DiagnosticParserCoreReceiptFull,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil {
		t.Fatalf("repetition fork stop=%+v err=%v", stop, err)
	}
	if len(scheduler.headers) != 2 || scheduler.work.Conflicts != 1 ||
		scheduler.work.Forks != 1 || scheduler.work.RepetitionFolds != 0 {
		t.Fatalf("repetition fork headers=%d work=%+v", len(scheduler.headers), scheduler.work)
	}
	got := make(map[StateID]uint32, 2)
	for _, header := range scheduler.headers {
		state, byteOffset, err := compact.Boundary(header.head)
		if err != nil {
			t.Fatal(err)
		}
		got[StateID(state)] = byteOffset
	}
	if got[2] != 0 || got[3] != 1 {
		t.Fatalf("repetition fork boundaries=%v, want reduce state 2 at byte 0 and shift state 3 at byte 1", got)
	}
}

func TestDiagnosticParserCoreGenericConflictPolicySelectsRepetitionReduce(t *testing.T) {
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 9}: {
				{Type: core.ActionShift, State: 3, Repetition: true},
				{Type: core.ActionReduce, Symbol: 4},
			},
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 4}: 2,
		},
	}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{
		Name: "haskell",
		ConflictPolicies: []ConflictPolicy{{
			State: 1, Lookahead: 9, Kind: ConflictPolicyRepetitionReduce,
			ReduceSymbols: []Symbol{4},
		}},
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: &dfaTokenSource{language: language},
		headers:     []diagnosticParserCoreHeader{{head: head}},
		token:       Token{Symbol: 9, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 8,
			ReceiptMode:   DiagnosticParserCoreReceiptFull,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil {
		t.Fatalf("conflict policy reduce stop=%+v err=%v", stop, err)
	}
	state, _, err := compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if state != 2 || scheduler.work.RepetitionFolds != 1 ||
		scheduler.work.Reductions != 1 || scheduler.work.Conflicts != 0 ||
		scheduler.work.Forks != 0 {
		t.Fatalf("conflict policy reduce state=%d work=%+v", state, scheduler.work)
	}
	action := scheduler.receipt.Rounds[0].Actions[0]
	if action.Ordinal != 1 || action.Action.Type != ParseActionReduce {
		t.Fatalf("conflict policy reduce action=%+v", action)
	}
}

func TestDiagnosticParserCoreGenericConflictPolicySelectsRepetitionShift(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionReduce, Symbol: 4},
		{Type: core.ActionShift, State: 3, Repetition: true},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	language := &Language{
		Name: "dart",
		ConflictPolicies: []ConflictPolicy{{
			State: 1, Lookahead: 9, Kind: ConflictPolicyRepetitionShift,
			ReduceSymbols: []Symbol{4},
		}},
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact:     compact,
		tokenSource: &dfaTokenSource{language: language},
		headers:     []diagnosticParserCoreHeader{{head: head}},
		token:       Token{Symbol: 9, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{
			MaxDispatches: 8,
			ReceiptMode:   DiagnosticParserCoreReceiptFull,
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop != nil {
		t.Fatalf("conflict policy shift stop=%+v err=%v", stop, err)
	}
	state, byteOffset, err := compact.Boundary(scheduler.headers[0].head)
	if err != nil {
		t.Fatal(err)
	}
	if state != 3 || byteOffset != 1 || scheduler.work.RepetitionFolds != 0 ||
		scheduler.work.OrdinaryShifts != 1 || scheduler.work.Conflicts != 0 ||
		scheduler.work.Forks != 0 {
		t.Fatalf("conflict policy shift state=%d byte=%d work=%+v", state, byteOffset, scheduler.work)
	}
	action := scheduler.receipt.Rounds[0].Actions[0]
	if action.Ordinal != 1 || action.Action.Type != ParseActionShift || !action.Action.Repetition {
		t.Fatalf("conflict policy shift action=%+v", action)
	}
}

func TestDiagnosticParserCoreGenericConflictPolicyRequiresExactRow(t *testing.T) {
	actions := core.NewActionRow([]core.Action{
		{Type: core.ActionShift, State: 3, Repetition: true},
		{Type: core.ActionReduce, Symbol: 4},
	}, false)
	language := &Language{
		Name: "haskell",
		ConflictPolicies: []ConflictPolicy{{
			State: 2, Lookahead: 9, Kind: ConflictPolicyRepetitionReduce,
			ReduceSymbols: []Symbol{4},
		}},
	}
	if _, ok := diagnosticParserCoreConflictPolicyOrdinal(language, Token{Symbol: 9}, 1, actions, 1); ok {
		t.Fatal("conflict policy admitted an unmatched state")
	}
	language.ConflictPolicies[0].State = 1
	language.ConflictPolicies[0].ReduceSymbols = []Symbol{5}
	if _, ok := diagnosticParserCoreConflictPolicyOrdinal(language, Token{Symbol: 9}, 1, actions, 1); ok {
		t.Fatal("conflict policy admitted an unmatched reduce symbol")
	}
}

func TestDiagnosticParserCoreGenericConflictIgnoresNoArmsWhenRepetitionDeclinesCell(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3, Repetition: true},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	head, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	header := diagnosticParserCoreHeader{head: head}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, header, 9)
	unsupported := diagnosticParserCoreGenericUnsupportedCell(0, Token{Symbol: 9, EndByte: 1}, cell.actions())
	if unsupported == nil || unsupported.boundary != DiagnosticParserCoreRoute || !strings.Contains(unsupported.detail, "repetition") {
		t.Fatalf("repetition conflict gate=%+v", unsupported)
	}
	scheduler := diagnosticParserCoreGenericScheduler{compact: compact, headers: []diagnosticParserCoreHeader{header}}
	if scheduler.work != (DiagnosticParserCoreGenericWork{}) || compact.Work() != (core.Work{}) {
		t.Fatalf("declined repetition cell published work: scheduler=%+v core=%+v", scheduler.work, compact.Work())
	}
}

func TestDiagnosticParserCoreDescriptorPreservesUnsupportedOrdinalOrdering(t *testing.T) {
	tests := []struct {
		name    string
		actions []core.Action
		detail  string
	}{
		{
			name: "ordinary shift precedes static repetition",
			actions: []core.Action{
				{Type: core.ActionShift, State: 2},
				{Type: core.ActionReduce, Repetition: true},
			},
			detail: "repetition",
		},
		{
			name: "static repetition precedes dynamic shift",
			actions: []core.Action{
				{Type: core.ActionReduce, Repetition: true},
				{Type: core.ActionShift, State: 2},
			},
			detail: "repetition",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := core.NewActionRow(test.actions, false)
			if got := row.Descriptor().Kind(); got != core.ActionRowUnsupported {
				t.Fatalf("descriptor=%v, want unsupported", got)
			}
			unsupported := diagnosticParserCoreGenericUnsupportedCell(3, Token{Symbol: 9}, row)
			if unsupported == nil || unsupported.headerIndex != 3 || !strings.Contains(unsupported.detail, test.detail) {
				t.Fatalf("unsupported=%+v, want first ordinal detail %q", unsupported, test.detail)
			}
		})
	}
}

func TestDiagnosticParserCoreDescriptorAdmitsZeroWidthOrdinaryShift(t *testing.T) {
	token := Token{Symbol: 9, StartByte: 4, EndByte: 4}
	tests := []struct {
		name    string
		actions []core.Action
	}{
		{name: "single shift", actions: []core.Action{{Type: core.ActionShift, State: 2}}},
		{name: "conflict shift", actions: []core.Action{{Type: core.ActionShift, State: 2}, {Type: core.ActionReduce, Symbol: 4}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			row := core.NewActionRow(test.actions, false)
			if unsupported := diagnosticParserCoreGenericUnsupportedCell(0, token, row); unsupported != nil {
				t.Fatalf("zero-width ordinary shift declined: %+v", unsupported)
			}
		})
	}
}

func TestDiagnosticParserCoreDescriptorValidatesCompleteFrontierBeforeDispatch(t *testing.T) {
	table := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 9}: {{Type: core.ActionShift, State: 3}},
		{state: 2, symbol: 9}: {{Type: core.ActionShift, State: 4, Repetition: true}},
	}}
	compact, err := core.New(table, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact,
		headers: []diagnosticParserCoreHeader{{head: first}, {head: second}},
		token:   Token{Symbol: 9, EndByte: 1},
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 8},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil || stop == nil || !strings.Contains(stop.detail, "repetition") {
		t.Fatalf("dispatch stop=%+v err=%v", stop, err)
	}
	if scheduler.dispatches != 0 || scheduler.work.OrdinaryShifts != 0 || scheduler.work.ExtraShifts != 0 || scheduler.work.Reductions != 0 || compact.Work() != (core.Work{}) {
		t.Fatalf("unsupported later cell allowed mutation: dispatches=%d scheduler=%+v core=%+v", scheduler.dispatches, scheduler.work, compact.Work())
	}
}

func TestDiagnosticParserCorePointIndexPollsBeforeScanning(t *testing.T) {
	want := errors.New("stop")
	polls := 0
	index, err := newDiagnosticParserCorePointIndex([]byte("first\nsecond\n"), func() error {
		polls++
		return want
	})
	if !errors.Is(err, want) || polls != 1 || index.lineStarts != nil {
		t.Fatalf("stopped point index=%+v err=%v polls=%d", index, err, polls)
	}
}

func TestDiagnosticParserCorePointIndexCachesExactArbitraryOffsets(t *testing.T) {
	index, err := newDiagnosticParserCorePointIndex([]byte("a\nbc\n\nxyz"), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		offset uint32
		want   Point
	}{
		{offset: 0, want: Point{Row: 0, Column: 0}},
		{offset: 1, want: Point{Row: 0, Column: 1}},
		{offset: 2, want: Point{Row: 1, Column: 0}},
		{offset: 4, want: Point{Row: 1, Column: 2}},
		{offset: 5, want: Point{Row: 2, Column: 0}},
		{offset: 6, want: Point{Row: 3, Column: 0}},
		{offset: 9, want: Point{Row: 3, Column: 3}},
		{offset: 12, want: Point{Row: 3, Column: 6}},
	}
	for _, test := range tests {
		got, hit := index.pointCached(test.offset)
		if hit || got != test.want {
			t.Fatalf("first point(%d)=%+v hit=%t, want %+v miss", test.offset, got, hit, test.want)
		}
		got, hit = index.pointCached(test.offset)
		if !hit || got != test.want {
			t.Fatalf("cached point(%d)=%+v hit=%t, want %+v hit", test.offset, got, hit, test.want)
		}
	}
	other, err := newDiagnosticParserCorePointIndex([]byte("x\ny"), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, hit := other.pointCached(2); hit {
		t.Fatal("point cache leaked across materializations")
	}
}

func TestDiagnosticParserCorePointIndexRejectsCollidingOffset(t *testing.T) {
	index, err := newDiagnosticParserCorePointIndex([]byte("abcdefghijklmn"), func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if point, hit := index.pointCached(0); hit || point != (Point{}) {
		t.Fatalf("first point(0)=%+v hit=%t, want origin miss", point, hit)
	}
	if point, hit := index.pointCached(13); hit || point != (Point{Column: 13}) {
		t.Fatalf("colliding point(13)=%+v hit=%t, want column 13 miss", point, hit)
	}
	if point, hit := index.pointCached(0); hit || point != (Point{}) {
		t.Fatalf("evicted point(0)=%+v hit=%t, want origin miss", point, hit)
	}
}

type genericConflictTable struct {
	actions []core.Action
	cells   map[genericConflictCell][]core.Action
	gotos   map[genericConflictCell]core.StateID
}

func (t *genericConflictTable) Actions(state core.StateID, symbol core.Symbol) (core.ActionRow, error) {
	if actions := t.cells[genericConflictCell{state: state, symbol: symbol}]; actions != nil {
		return core.NewActionRow(actions, false), nil
	}
	if state == 1 && symbol == 9 {
		return core.NewActionRow(t.actions, false), nil
	}
	return core.ActionRow{}, nil
}

type genericConflictCell struct {
	state  core.StateID
	symbol core.Symbol
}

func (t *genericConflictTable) Goto(state core.StateID, symbol core.Symbol) (core.StateID, error) {
	return t.gotos[genericConflictCell{state: state, symbol: symbol}], nil
}

func (*genericConflictTable) ProductionFields(uint16, int) ([]core.FieldMapEntry, error) {
	return nil, nil
}

func (*genericConflictTable) ProductionAliases(uint16, int) ([]core.Symbol, error) {
	return nil, nil
}

func TestDiagnosticParserCoreRecoverEOFAcceptHeaderTopologyRejectsUnsupportedProvenance(t *testing.T) {
	base := diagnosticParserCoreHeader{}
	if !recoverEOFAcceptHeaderTopologyEligible(base) {
		t.Fatal("plain singleton header was rejected")
	}
	tests := []struct {
		name   string
		mutate func(*diagnosticParserCoreHeader)
	}{
		{name: "accepted", mutate: func(header *diagnosticParserCoreHeader) { header.accepted = true }},
		{name: "shifted", mutate: func(header *diagnosticParserCoreHeader) { header.shifted = true }},
		{name: "paused", mutate: func(header *diagnosticParserCoreHeader) { header.paused = true }},
		{name: "alternative-set", mutate: func(header *diagnosticParserCoreHeader) { header.altSet = core.NewAlternativeSetMember(7, 0) }},
		{name: "converged-split", mutate: func(header *diagnosticParserCoreHeader) { header.convergedReductionSplit = true }},
		{name: "resurrection-unproved", mutate: func(header *diagnosticParserCoreHeader) { header.resurrectionUnproved = true }},
		{name: "blended", mutate: func(header *diagnosticParserCoreHeader) { header.blended = true }},
		{name: "recovery-lineage", mutate: func(header *diagnosticParserCoreHeader) { header.markRecoveryLineage() }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			header := base
			test.mutate(&header)
			if recoverEOFAcceptHeaderTopologyEligible(header) {
				t.Fatalf("unsupported header topology was admitted: %+v", header)
			}
		})
	}
}

func TestDiagnosticParserCoreRecoverEOFAcceptPayloadShapeAdmitsSingletonAndRejectsFragile(t *testing.T) {
	plainTable := &genericConflictTable{cells: map[genericConflictCell][]core.Action{
		{state: 1, symbol: 1}: {{Type: core.ActionShift, State: 2}},
	}}
	plain, err := core.New(plainTable, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	seed, err := plain.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	plainHead, err := plain.Shift(seed, 1, 0, core.Token{Symbol: 1, StartByte: 0, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	plainPaths, err := plain.Derivations(plainHead)
	if err != nil {
		t.Fatal(err)
	}
	if len(plainPaths) != 1 || len(plainPaths[0].Payloads) != 1 {
		t.Fatalf("plain singleton derivations=%+v", plainPaths)
	}
	if _, ok, err := recoverEOFAcceptPayloadShapeEligible(plain, plainPaths[0].Payloads[0]); err != nil || !ok {
		t.Fatalf("plain singleton shape eligible=%t err=%v", ok, err)
	}

	fragileTable := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 1}: {{Type: core.ActionShift, State: 2}},
			{state: 2, symbol: 2}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 4, ChildCount: 2, ProductionID: 1}},
		},
		gotos: map[genericConflictCell]core.StateID{{state: 1, symbol: 4}: 4},
	}
	fragile, err := core.New(fragileTable, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	fragileSeed, err := fragile.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	fragileHead, err := fragile.Shift(fragileSeed, 1, 0, core.Token{Symbol: 1, StartByte: 0, EndByte: 1}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	fragileHead, err = fragile.Shift(fragileHead, 2, 0, core.Token{Symbol: 2, StartByte: 1, EndByte: 2}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	fragile.SetReduceConflictContext(true)
	fragileFrontier, err := fragile.Reduce(fragileHead, 9, 0, core.ForkOrder{})
	fragile.SetReduceConflictContext(false)
	if err != nil || len(fragileFrontier) != 1 {
		t.Fatalf("fragile reduction frontier=%v err=%v", fragileFrontier, err)
	}
	fragilePaths, err := fragile.Derivations(fragileFrontier[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(fragilePaths) != 1 || len(fragilePaths[0].Payloads) != 1 {
		t.Fatalf("fragile derivations=%+v", fragilePaths)
	}
	fragileView, err := fragile.MaterializationView(fragilePaths[0].Payloads[0])
	if err != nil {
		t.Fatal(err)
	}
	if !fragileView.Fragile {
		t.Fatalf("fixture reduction was not fragile: %+v", fragileView)
	}
	if _, ok, err := recoverEOFAcceptPayloadShapeEligible(fragile, fragilePaths[0].Payloads[0]); err != nil || ok {
		t.Fatalf("fragile payload shape eligible=%t err=%v", ok, err)
	}
}

func TestDiagnosticParserCoreRecoverEOFAcceptPriorityFreeProbesAreReadOnly(t *testing.T) {
	tests := []struct {
		name      string
		cells     map[genericConflictCell][]core.Action
		gotos     map[genericConflictCell]core.StateID
		shiftHead bool
		wantAdmit bool
	}{
		{name: "no-earlier-mechanism", wantAdmit: true},
		{name: "any-lookahead-reduction", cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 2}: {{Type: core.ActionReduce, Symbol: 4, ChildCount: 1}},
		}},
		{name: "missing-token-opportunity", cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 2}: {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 9}: {{Type: core.ActionReduce, Symbol: 4, ChildCount: 1}},
		}},
		{name: "ancestor-eof-action", cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 0}: {{Type: core.ActionShift, State: 4}},
			{state: 1, symbol: 1}: {{Type: core.ActionShift, State: 2}},
		}, shiftHead: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{cells: test.cells, gotos: test.gotos}, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			head, err := compact.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			if test.shiftHead {
				head, err = compact.Shift(head, 1, 0, core.Token{Symbol: 1, StartByte: 0, EndByte: 1}, core.ForkOrder{})
				if err != nil {
					t.Fatal(err)
				}
			}
			before, err := compact.Stats(head)
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact:     compact,
				tokenSource: &dfaTokenSource{language: &Language{TokenCount: 4}},
				token:       Token{Symbol: 9, StartByte: 1, EndByte: 1},
			}
			got, err := scheduler.recoverEOFAcceptPriorityFree(head)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.wantAdmit {
				t.Fatalf("priority-free=%t, want %t", got, test.wantAdmit)
			}
			after, err := compact.Stats(head)
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("priority probes mutated Core: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestDiagnosticParserCoreGenericConflictArbitraryNOrdering(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3},
		{Type: core.ActionShift, State: 4},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := compact.Seed(9, 0)
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	suffix, err := compact.Seed(8, 0)
	if err != nil {
		t.Fatal(err)
	}
	headers := []diagnosticParserCoreHeader{
		{head: prefix, creationSeq: 2},
		{head: prefix, creationSeq: 3},
		{head: incoming, creationSeq: 4},
		{head: suffix, creationSeq: 6},
	}
	headers[2].markRecoveryLineage()
	before, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: headers, token: Token{Symbol: 9, StartByte: 0, EndByte: 1, ExternalScannerToken: true, ExternalScannerStartByte: 0},
		branchOrder: 7, nextSeq: 10, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
		electionIndex: 44,
		currentElection: DiagnosticParserCoreElection{
			Token:         Token{Symbol: 9, StartByte: 0, EndByte: 1, ExternalScannerToken: true, ExternalScannerStartByte: 0},
			ScannerBefore: DiagnosticParserCoreScannerCheckpoint{Length: 1, SHA256: [32]byte{1}},
			ScannerAfter:  DiagnosticParserCoreScannerCheckpoint{Length: 2, SHA256: [32]byte{2}},
		},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 2, headers[2], 9)
	if err := scheduler.applyGenericConflict(before, cell); err != nil {
		t.Fatal(err)
	}
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	var states []StateID
	var sequences []uint64
	for _, receipt := range receipts {
		states = append(states, receipt.State)
		sequences = append(sequences, receipt.CreationSeq)
	}
	if !reflect.DeepEqual(states, []StateID{9, 2, 8, 3, 4}) || !reflect.DeepEqual(sequences, []uint64{2, 4, 6, 10, 11}) {
		t.Fatalf("physical conflict order states=%v sequences=%v", states, sequences)
	}
	for index := range scheduler.headers {
		wantRecovery := index == 1 || index == 3 || index == 4
		if scheduler.headers[index].isRecoveryLineage() != wantRecovery {
			t.Fatalf("conflict output %d recovery marker=%t, want %t", index, scheduler.headers[index].isRecoveryLineage(), wantRecovery)
		}
		if wantRecovery && !scheduler.headers[index].isRecoveryCosted() {
			t.Fatalf("conflict output %d lost recovery cost provenance", index)
		}
	}
	if scheduler.branchOrder != 9 || scheduler.nextSeq != 12 || scheduler.dispatches != 1 || scheduler.work != (DiagnosticParserCoreGenericWork{
		Dispatches: 1, Conflicts: 1, ConflictActions: 3, Forks: 2, ConflictHeads: 3,
		ConflictActionArmsAdmitted: 3, CausalConflictForks: 2,
		RecoveryAmbiguityForks: 1, Canonicalizations: 1, PeakHeaders: 5,
	}) {
		t.Fatalf("scheduler allocation/work drift: order=%d seq=%d dispatches=%d work=%+v", scheduler.branchOrder, scheduler.nextSeq, scheduler.dispatches, scheduler.work)
	}
	conflict := scheduler.receipt.Conflicts[0]
	if len(conflict.Round.Actions) != 3 || conflict.Round.Actions[0].Ordinal != 1 || conflict.Round.Actions[0].BranchOrder != 8 ||
		conflict.Round.Actions[1].Ordinal != 2 || conflict.Round.Actions[1].BranchOrder != 9 || conflict.Round.Actions[2].Ordinal != 0 ||
		len(conflict.Prefix) != 2 || conflict.PrimaryOutput.State != 2 || len(conflict.OriginalSuffix) != 1 ||
		len(conflict.SecondaryArms) != 2 || len(conflict.SecondaryArms[0].Outputs) != 1 || len(conflict.SecondaryArms[1].Outputs) != 1 ||
		len(conflict.AdditionalPrimaryOutputs) != 0 || len(conflict.After) != 5 {
		t.Fatalf("three-action conflict receipt drifted: %+v", conflict)
	}
	if len(scheduler.receipt.ExternalShifts) != 1 {
		t.Fatalf("external conflict receipts=%+v, want one round", scheduler.receipt.ExternalShifts)
	}
	external := scheduler.receipt.ExternalShifts[0]
	if external.ElectionIndex != 44 || external.RoundIndex != 0 || !reflect.DeepEqual(external.Token, scheduler.currentElection.Token) ||
		external.ScannerBefore != scheduler.currentElection.ScannerBefore || external.ScannerAfter != scheduler.currentElection.ScannerAfter || len(external.Payloads) != 3 {
		t.Fatalf("external conflict receipt drifted: %+v", external)
	}
	for index, payload := range external.Payloads {
		if payload.ID != uint32(index+1) || payload.Symbol != 9 || payload.StartByte != 0 || payload.EndByte != 1 ||
			payload.ProductionID != 0 || payload.DynamicPrecedence != 0 || len(payload.Children) != 0 || len(payload.Fields) != 0 || len(payload.Aliases) != 0 ||
			!payload.Terminal || !payload.External || payload.Extra {
			t.Fatalf("external conflict payload %d=%+v", index, payload)
		}
	}
}

func TestDiagnosticParserCoreGenericConflictMultiOutputSequencing(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionReduce, Symbol: 7, ChildCount: 1},
		{Type: core.ActionShift, State: 11},
		{Type: core.ActionReduce, Symbol: 8, ChildCount: 1},
	}
	table := &genericConflictTable{
		cells: map[genericConflictCell][]core.Action{
			{state: 1, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
			{state: 2, symbol: 9}:  {{Type: core.ActionShift, State: 3}},
			{state: 3, symbol: 10}: actions,
		},
		gotos: map[genericConflictCell]core.StateID{
			{state: 1, symbol: 7}: 4,
			{state: 2, symbol: 7}: 5,
			{state: 1, symbol: 8}: 6,
			{state: 2, symbol: 8}: 7,
		},
	}
	compact, err := core.New(table, core.Limits{MaxDerivations: 8, MaxPopPaths: 8})
	if err != nil {
		t.Fatal(err)
	}
	first, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := compact.Seed(2, 0)
	if err != nil {
		t.Fatal(err)
	}
	firstShifted, err := compact.Shift(first, 9, 0, core.Token{Symbol: 9, EndByte: 1}, core.ForkOrder{Present: true, Value: 1})
	if err != nil {
		t.Fatal(err)
	}
	secondShifted, err := compact.Shift(second, 9, 0, core.Token{Symbol: 9, EndByte: 1}, core.ForkOrder{Present: true, Value: 2})
	if err != nil {
		t.Fatal(err)
	}
	shiftedStats, err := compact.Stats(secondShifted)
	if err != nil {
		t.Fatal(err)
	}
	if firstShifted == secondShifted || shiftedStats.CurrentExactPaths != 2 {
		t.Fatalf("second immutable head did not retain both converged paths: first=%v second=%v stats=%+v", firstShifted, secondShifted, shiftedStats)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	prefix, err := compact.Seed(9, 1)
	if err != nil {
		t.Fatal(err)
	}
	suffix, err := compact.Seed(8, 1)
	if err != nil {
		t.Fatal(err)
	}
	headers := []diagnosticParserCoreHeader{
		{head: prefix, creationSeq: 2},
		{head: secondShifted, creationSeq: 4},
		{head: suffix, creationSeq: 6},
	}
	before, err := diagnosticParserCoreHeaderReceipts(compact, headers)
	if err != nil {
		t.Fatal(err)
	}
	beforeStats, err := compact.Stats(secondShifted)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name        string
		branchOrder uint64
		nextSeq     uint64
	}{
		{name: "branch-order-overflow", branchOrder: math.MaxUint64 - 1, nextSeq: 10},
		{name: "creation-sequence-overflow", branchOrder: 7, nextSeq: math.MaxUint64 - 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			receipt := &DiagnosticParserCoreGenericScheduler{}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: append([]diagnosticParserCoreHeader(nil), headers...),
				token:                Token{Symbol: 10, StartByte: 1, EndByte: 2},
				branchOrder:          test.branchOrder,
				nextSeq:              test.nextSeq,
				nextCleanPathLineage: 1,
				options:              DiagnosticParserCorePrefixOptions{MaxDispatches: 100}, receipt: receipt,
			}
			cell := mustDiagnosticParserCoreGenericCell(t, compact, 1, headers[1], 10)
			err := scheduler.applyGenericConflict(before, cell)
			if err == nil {
				t.Fatal("overflowing conflict unexpectedly succeeded")
			}
			afterStats, statsErr := compact.Stats(secondShifted)
			if statsErr != nil {
				t.Fatal(statsErr)
			}
			if beforeStats != afterStats || !reflect.DeepEqual(scheduler.headers, headers) ||
				scheduler.branchOrder != test.branchOrder || scheduler.nextSeq != test.nextSeq ||
				scheduler.nextCleanPathLineage != 1 ||
				scheduler.dispatches != 0 || scheduler.work != (DiagnosticParserCoreGenericWork{}) ||
				!reflect.DeepEqual(receipt, &DiagnosticParserCoreGenericScheduler{}) {
				t.Fatalf("overflow rollback leaked: before=%+v after=%+v scheduler=%+v receipt=%+v", beforeStats, afterStats, scheduler, receipt)
			}
		})
	}
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: headers, token: Token{Symbol: 10, StartByte: 1, EndByte: 2},
		branchOrder: 7, nextSeq: 10, nextCleanPathLineage: 1,
		options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100}, receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	cell := mustDiagnosticParserCoreGenericCell(t, compact, 1, headers[1], 10)
	if err := scheduler.applyGenericConflict(before, cell); err != nil {
		t.Fatal(err)
	}
	receipts, err := diagnosticParserCoreHeaderReceipts(compact, scheduler.headers)
	if err != nil {
		t.Fatal(err)
	}
	var states []StateID
	var sequences []uint64
	for _, receipt := range receipts {
		states = append(states, receipt.State)
		sequences = append(sequences, receipt.CreationSeq)
	}
	if !reflect.DeepEqual(states, []StateID{9, 4, 8, 11, 6, 7, 5}) ||
		!reflect.DeepEqual(sequences, []uint64{2, 4, 6, 10, 11, 12, 13}) {
		t.Fatalf("multi-output order states=%v sequences=%v", states, sequences)
	}
	conflict := scheduler.receipt.Conflicts[0]
	if scheduler.branchOrder != 9 || scheduler.nextSeq != 14 || conflict.PrimaryOutput.State != 4 || len(conflict.AdditionalPrimaryOutputs) != 1 || conflict.AdditionalPrimaryOutputs[0].State != 5 ||
		len(conflict.SecondaryArms) != 2 || len(conflict.SecondaryArms[0].Outputs) != 1 || len(conflict.SecondaryArms[1].Outputs) != 2 ||
		len(conflict.Round.Actions) != 3 || conflict.Round.Actions[0].Ordinal != 1 || conflict.Round.Actions[1].Ordinal != 2 || conflict.Round.Actions[2].Ordinal != 0 {
		t.Fatalf("multi-output conflict allocation drifted: order=%d seq=%d receipt=%+v", scheduler.branchOrder, scheduler.nextSeq, conflict)
	}
}

func TestDiagnosticParserCoreHeaderPathsMarksBoundedObservation(t *testing.T) {
	table := &genericConflictTable{cells: make(map[genericConflictCell][]core.Action)}
	for state := core.StateID(1); state <= 64; state++ {
		target := core.StateID(100 + (state-1)/8)
		table.cells[genericConflictCell{state: state, symbol: 1}] = []core.Action{{Type: core.ActionShift, State: target}}
		table.cells[genericConflictCell{target, 2}] = []core.Action{{Type: core.ActionShift, State: 200}}
	}
	table.cells[genericConflictCell{state: 200, symbol: 3}] = []core.Action{{Type: core.ActionShift, State: 300}}
	table.cells[genericConflictCell{state: 1000, symbol: 3}] = []core.Action{{Type: core.ActionShift, State: 300}}

	compact, err := core.New(table, core.Limits{MaxDerivations: 64})
	if err != nil {
		t.Fatal(err)
	}
	groups := make([]core.Head, 8)
	for state := core.StateID(1); state <= 64; state++ {
		seed, seedErr := compact.Seed(state, 0)
		if seedErr != nil {
			t.Fatal(seedErr)
		}
		group, shiftErr := compact.Shift(seed, 1, 0, core.Token{Symbol: 1, EndByte: 1}, core.ForkOrder{})
		if shiftErr != nil {
			t.Fatal(shiftErr)
		}
		groups[(state-1)/8] = group
	}
	spare, err := compact.Seed(1000, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	var sixtyFour core.Head
	for _, group := range groups {
		sixtyFour, err = compact.Shift(group, 2, 0, core.Token{Symbol: 2, StartByte: 1, EndByte: 2}, core.ForkOrder{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := compact.BeginFrontier(); err != nil {
		t.Fatal(err)
	}
	head, err := compact.Shift(sixtyFour, 3, 0, core.Token{Symbol: 3, StartByte: 2, EndByte: 3}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	head, err = compact.Shift(spare, 3, 0, core.Token{Symbol: 3, StartByte: 2, EndByte: 3}, core.ForkOrder{})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := diagnosticParserCoreHeaderPaths(compact, diagnosticParserCoreHeader{head: head})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Header.ExactPaths != 65 || !receipt.DerivationsTruncated || len(receipt.Derivations) != 0 {
		t.Fatalf("bounded path receipt=%+v, want 65 paths with explicitly truncated derivations", receipt)
	}
}

func TestDiagnosticParserCoreGenericConflictArenaCapsRollback(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3},
		{Type: core.ActionShift, State: 4},
	}
	for _, test := range []struct {
		name        string
		maxSubtrees uint32
	}{
		{name: "partial-secondary", maxSubtrees: 1},
		{name: "partial-primary", maxSubtrees: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{MaxSubtrees: test.maxSubtrees})
			if err != nil {
				t.Fatal(err)
			}
			incoming, err := compact.Seed(1, 0)
			if err != nil {
				t.Fatal(err)
			}
			before, err := compact.Stats(incoming)
			if err != nil {
				t.Fatal(err)
			}
			header := diagnosticParserCoreHeader{head: incoming, creationSeq: 4}
			receipt, err := diagnosticParserCoreHeaderReceipt(compact, header)
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: []diagnosticParserCoreHeader{header},
				token: Token{Symbol: 9, StartByte: 0, EndByte: 1}, branchOrder: 7, nextSeq: 10,
				options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100}, receipt: &DiagnosticParserCoreGenericScheduler{},
			}
			cell := mustDiagnosticParserCoreGenericCell(t, compact, 0, header, 9)
			if err := compact.ApplyAtomic(func() error {
				return scheduler.applyGenericConflict([]DiagnosticParserCoreHeaderReceipt{receipt}, cell)
			}); err == nil {
				t.Fatal("capped conflict unexpectedly succeeded")
			}
			after, err := compact.Stats(incoming)
			if err != nil {
				t.Fatal(err)
			}
			if after != before {
				t.Fatalf("capped conflict leaked graph mutation: before=%+v after=%+v", before, after)
			}
			if scheduler.dispatches != 0 || scheduler.branchOrder != 7 || scheduler.nextSeq != 10 || scheduler.work != (DiagnosticParserCoreGenericWork{}) ||
				len(scheduler.headers) != 1 || scheduler.headers[0] != header || !reflect.DeepEqual(scheduler.receipt, &DiagnosticParserCoreGenericScheduler{}) {
				t.Fatalf("capped conflict leaked scheduler allocation/receipt state: %+v receipt=%+v", scheduler, scheduler.receipt)
			}
		})
	}
}

func TestDiagnosticParserCoreGenericConflictRejectsUnsupportedArmBeforeMutation(t *testing.T) {
	actions := []core.Action{
		{Type: core.ActionShift, State: 2},
		{Type: core.ActionShift, State: 3, Extra: true},
	}
	compact, err := core.New(&genericConflictTable{actions: actions}, core.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	incoming, err := compact.Seed(1, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := compact.Stats(incoming)
	scheduler := &diagnosticParserCoreGenericScheduler{
		compact: compact, headers: []diagnosticParserCoreHeader{{head: incoming}},
		token: Token{Symbol: 9, StartByte: 0, EndByte: 1}, options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
		receipt: &DiagnosticParserCoreGenericScheduler{},
	}
	stop, err := scheduler.dispatchPass()
	if err != nil {
		t.Fatal(err)
	}
	if stop == nil || stop.boundary != DiagnosticParserCoreExtra || scheduler.dispatches != 0 || scheduler.work.ActionLookups != 1 {
		t.Fatalf("unsupported-arm preflight stop=%+v dispatches=%d work=%+v", stop, scheduler.dispatches, scheduler.work)
	}
	after, _ := compact.Stats(incoming)
	if after != before {
		t.Fatalf("unsupported-arm preflight mutated graph: before=%+v after=%+v", before, after)
	}
}

func TestDiagnosticParserCoreGenericAcceptRequiresSoleFrontierBeforeMutation(t *testing.T) {
	accept := []core.Action{{Type: core.ActionAccept}}
	for _, test := range []struct {
		name          string
		cells         map[genericConflictCell][]core.Action
		headerStates  []core.StateID
		configureHead func(*diagnosticParserCoreHeader)
	}{
		{
			name:         "accept-and-no-action",
			cells:        map[genericConflictCell][]core.Action{{state: 1, symbol: 0}: accept},
			headerStates: []core.StateID{1, 2},
		},
		{
			name:          "accept-and-shifted-sibling",
			cells:         map[genericConflictCell][]core.Action{{state: 1, symbol: 0}: accept},
			headerStates:  []core.StateID{1, 2},
			configureHead: func(header *diagnosticParserCoreHeader) { header.shifted = true },
		},
		{
			name: "accept-and-reduction-sibling",
			cells: map[genericConflictCell][]core.Action{
				{state: 1, symbol: 0}: accept,
				{state: 2, symbol: 0}: {{Type: core.ActionReduce, Symbol: 3}},
			},
			headerStates: []core.StateID{1, 2},
		},
		{
			name: "multiple-accepts",
			cells: map[genericConflictCell][]core.Action{
				{state: 1, symbol: 0}: accept,
				{state: 2, symbol: 0}: accept,
			},
			headerStates: []core.StateID{1, 2},
		},
		{
			name: "accept-in-fork",
			cells: map[genericConflictCell][]core.Action{
				{state: 1, symbol: 0}: {{Type: core.ActionAccept}, {Type: core.ActionReduce, Symbol: 3}},
			},
			headerStates: []core.StateID{1},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			compact, err := core.New(&genericConflictTable{cells: test.cells}, core.Limits{})
			if err != nil {
				t.Fatal(err)
			}
			headers := make([]diagnosticParserCoreHeader, len(test.headerStates))
			for index, state := range test.headerStates {
				head, seedErr := compact.Seed(state, 0)
				if seedErr != nil {
					t.Fatal(seedErr)
				}
				headers[index] = diagnosticParserCoreHeader{head: head, creationSeq: uint64(index)}
			}
			if test.configureHead != nil {
				test.configureHead(&headers[len(headers)-1])
			}
			beforeHeaders := append([]diagnosticParserCoreHeader(nil), headers...)
			beforeStats, err := compact.Stats(headers[0].head)
			if err != nil {
				t.Fatal(err)
			}
			scheduler := &diagnosticParserCoreGenericScheduler{
				compact: compact, headers: headers,
				token:   Token{Symbol: 0, StartByte: 0, EndByte: 0},
				options: DiagnosticParserCorePrefixOptions{MaxDispatches: 100},
				receipt: &DiagnosticParserCoreGenericScheduler{},
			}
			stop, err := scheduler.dispatchPass()
			if err != nil {
				t.Fatal(err)
			}
			if stop == nil {
				t.Fatal("mixed accept frontier unexpectedly dispatched")
			}
			afterStats, err := compact.Stats(headers[0].head)
			if err != nil {
				t.Fatal(err)
			}
			if afterStats != beforeStats || !reflect.DeepEqual(scheduler.headers, beforeHeaders) || scheduler.dispatches != 0 || scheduler.work.Accepts != 0 || len(scheduler.receipt.Rounds) != 0 || scheduler.acceptedHead.Node != 0 {
				t.Fatalf("mixed accept mutated state: stop=%+v beforeStats=%+v afterStats=%+v scheduler=%+v", stop, beforeStats, afterStats, scheduler)
			}
		})
	}
}
