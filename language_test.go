package gotreesitter

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/odvcencio/gotreesitter/internal/ccore"
)

// TestParseActionTypeConstants verifies the iota-generated constants.
func TestParseActionTypeConstants(t *testing.T) {
	if ParseActionShift != 0 {
		t.Errorf("ParseActionShift = %d, want 0", ParseActionShift)
	}
	if ParseActionReduce != 1 {
		t.Errorf("ParseActionReduce = %d, want 1", ParseActionReduce)
	}
	if ParseActionAccept != 2 {
		t.Errorf("ParseActionAccept = %d, want 2", ParseActionAccept)
	}
	if ParseActionRecover != 3 {
		t.Errorf("ParseActionRecover = %d, want 3", ParseActionRecover)
	}
}

func TestLanguageVersionCompatibility(t *testing.T) {
	lang := &Language{}
	if got := lang.Version(); got != 0 {
		t.Fatalf("Version: got %d, want 0", got)
	}
	if !lang.CompatibleWithRuntime() {
		t.Fatal("expected unspecified language version to be compatible")
	}

	lang.LanguageVersion = RuntimeLanguageVersion
	if !lang.CompatibleWithRuntime() {
		t.Fatalf("expected runtime version %d to be compatible", RuntimeLanguageVersion)
	}

	lang.LanguageVersion = MinCompatibleLanguageVersion
	if !lang.CompatibleWithRuntime() {
		t.Fatalf("expected min compatible version %d to be compatible", MinCompatibleLanguageVersion)
	}

	lang.LanguageVersion = MinCompatibleLanguageVersion - 1
	if lang.CompatibleWithRuntime() {
		t.Fatal("expected version below minimum to be incompatible")
	}

	lang.LanguageVersion = RuntimeLanguageVersion + 1
	if lang.CompatibleWithRuntime() {
		t.Fatal("expected version above runtime maximum to be incompatible")
	}
}

func TestLanguageSizeReportsDecodedTables(t *testing.T) {
	var nilLang *Language
	if got := nilLang.Size(); got != 0 {
		t.Fatalf("nil Language Size = %d, want 0", got)
	}

	base := (&Language{Name: "tiny"}).Size()
	lang := &Language{
		Name:        "larger",
		SymbolNames: []string{"end", "identifier", "expression"},
		ParseTable: [][]uint16{
			{0, 1, 2},
			{3, 4, 5},
		},
		LexStates: []LexState{
			{Transitions: []LexTransition{{Lo: 'a', Hi: 'z', NextState: 1}}},
		},
		ParseActions: []ParseActionEntry{
			{Actions: []ParseAction{{Type: ParseActionShift, State: 1}}},
		},
	}
	if got := lang.Size(); got <= base {
		t.Fatalf("populated Language Size = %d, want > base %d", got, base)
	}

	beforeMaps := lang.Size()
	if _, ok := lang.SymbolByName("identifier"); !ok {
		t.Fatal("SymbolByName(identifier) = false")
	}
	if got := lang.Size(); got <= beforeMaps {
		t.Fatalf("Language Size after lazy maps = %d, want > %d", got, beforeMaps)
	}
}

func TestLexAsciiTableFillsRangesPreservingFirstMatch(t *testing.T) {
	lang := &Language{
		LexStates: []LexState{
			{
				Transitions: []LexTransition{
					{Lo: 'a', Hi: 'z', NextState: 1},
					{Lo: 'm', Hi: 'm', NextState: 2},
					{Lo: '0', Hi: '9', NextState: 3, Skip: true},
					{Lo: 0, Hi: 255, NextState: 4},
				},
			},
		},
	}

	row := lang.LexAsciiTable()[0]
	if got, want := row['m'], int32(1); got != want {
		t.Fatalf("overlapping first match = %d, want %d", got, want)
	}
	if got, want := row['0'], int32(3)|lexAsciiSkipBit; got != want {
		t.Fatalf("skip transition = %d, want %d", got, want)
	}
	if got, want := row['A'], int32(4); got != want {
		t.Fatalf("fallback range match = %d, want %d", got, want)
	}
}

func TestInferGeneratedRepeatAuxMetadata(t *testing.T) {
	lang := &Language{
		TokenCount: 2,
		SymbolNames: []string{
			"end",
			"token_repeat1",
			"module_repeat1",
			"visible_repeat2",
			"named_repeat3",
			"not_repeat",
			"item_repeatx",
			"_statement_repeat12",
			"super_repeat4",
		},
		SymbolMetadata: []SymbolMetadata{
			{Name: "end"},
			{},
			{},
			{Visible: true},
			{Named: true},
			{},
			{},
			{Name: "_statement_repeat12"},
			{Supertype: true},
		},
	}

	InferGeneratedRepeatAuxMetadata(lang)

	if !lang.SymbolMetadata[2].GeneratedRepeatAux {
		t.Fatal("invisible anonymous module_repeat1 was not marked GeneratedRepeatAux")
	}
	if !lang.SymbolMetadata[7].GeneratedRepeatAux {
		t.Fatal("invisible anonymous _statement_repeat12 was not marked GeneratedRepeatAux")
	}
	for _, idx := range []int{1, 3, 4, 5, 6, 8} {
		if lang.SymbolMetadata[idx].GeneratedRepeatAux {
			t.Fatalf("SymbolMetadata[%d] GeneratedRepeatAux = true, want false", idx)
		}
	}
}

// TestMinimalLanguage constructs a minimal 3-symbol, 2-state grammar
// and verifies that all fields are correctly defined and accessible.
func TestMinimalLanguage(t *testing.T) {
	// Symbols: 0=ERROR, 1=identifier (terminal), 2=expression (nonterminal)
	lang := Language{
		Name:               "test",
		SymbolCount:        3,
		TokenCount:         2,
		ExternalTokenCount: 0,
		StateCount:         2,
		LargeStateCount:    0,
		FieldCount:         1,
		ProductionIDCount:  1,

		SymbolNames: []string{"ERROR", "identifier", "expression"},
		SymbolMetadata: []SymbolMetadata{
			{Name: "ERROR", Visible: false, Named: false, Supertype: false},
			{Name: "identifier", Visible: true, Named: true, Supertype: false},
			{Name: "expression", Visible: true, Named: true, Supertype: false},
		},
		FieldNames: []string{"", "name"},

		// State 0: shift to state 1 on symbol 1 (identifier)
		// State 1: reduce to symbol 2 (expression)
		ParseTable: [][]uint16{
			{0, 1}, // state 0
			{0, 0}, // state 1
		},
		ParseActions: []ParseActionEntry{
			{
				Reusable: false,
				Actions: []ParseAction{
					{
						Type:  ParseActionShift,
						State: 1,
					},
				},
			},
			{
				Reusable: false,
				Actions: []ParseAction{
					{
						Type:              ParseActionReduce,
						Symbol:            2,
						ChildCount:        1,
						DynamicPrecedence: 0,
						ProductionID:      0,
					},
				},
			},
		},

		LexModes: []LexMode{
			{LexState: 0, ExternalLexState: 0},
			{LexState: 1, ExternalLexState: 0},
		},
		LexStates: []LexState{
			{
				AcceptToken: 0,
				Skip:        true,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'a', Hi: 'z', NextState: 1},
				},
			},
			{
				AcceptToken: 1,
				Skip:        false,
				Default:     -1,
				EOF:         -1,
				Transitions: []LexTransition{
					{Lo: 'a', Hi: 'z', NextState: 1},
				},
			},
		},

		KeywordLexStates:    nil,
		KeywordCaptureToken: 0,

		FieldMapSlices: [][2]uint16{
			{0, 1},
		},
		FieldMapEntries: []FieldMapEntry{
			{FieldID: 1, ChildIndex: 0, Inherited: false},
		},

		AliasSequences:  nil,
		PrimaryStateIDs: []StateID{0, 1},
		ExternalScanner: nil,
	}

	// Verify basic counts.
	if lang.SymbolCount != 3 {
		t.Errorf("SymbolCount = %d, want 3", lang.SymbolCount)
	}
	if lang.TokenCount != 2 {
		t.Errorf("TokenCount = %d, want 2", lang.TokenCount)
	}
	if lang.StateCount != 2 {
		t.Errorf("StateCount = %d, want 2", lang.StateCount)
	}
	if lang.FieldCount != 1 {
		t.Errorf("FieldCount = %d, want 1", lang.FieldCount)
	}
	if lang.Name != "test" {
		t.Errorf("Name = %q, want %q", lang.Name, "test")
	}

	// Verify symbol metadata.
	if len(lang.SymbolMetadata) != 3 {
		t.Fatalf("len(SymbolMetadata) = %d, want 3", len(lang.SymbolMetadata))
	}
	if lang.SymbolMetadata[1].Name != "identifier" {
		t.Errorf("SymbolMetadata[1].Name = %q, want %q", lang.SymbolMetadata[1].Name, "identifier")
	}
	if !lang.SymbolMetadata[1].Visible {
		t.Error("SymbolMetadata[1].Visible = false, want true")
	}
	if !lang.SymbolMetadata[1].Named {
		t.Error("SymbolMetadata[1].Named = false, want true")
	}
	if lang.SymbolMetadata[0].Visible {
		t.Error("SymbolMetadata[0].Visible = true, want false (ERROR)")
	}

	// Verify field names.
	if len(lang.FieldNames) != 2 {
		t.Fatalf("len(FieldNames) = %d, want 2", len(lang.FieldNames))
	}
	if lang.FieldNames[0] != "" {
		t.Errorf("FieldNames[0] = %q, want empty string", lang.FieldNames[0])
	}
	if lang.FieldNames[1] != "name" {
		t.Errorf("FieldNames[1] = %q, want %q", lang.FieldNames[1], "name")
	}

	// Verify parse actions.
	if len(lang.ParseActions) != 2 {
		t.Fatalf("len(ParseActions) = %d, want 2", len(lang.ParseActions))
	}
	shift := lang.ParseActions[0].Actions[0]
	if shift.Type != ParseActionShift {
		t.Errorf("shift action type = %d, want %d", shift.Type, ParseActionShift)
	}
	if shift.State != 1 {
		t.Errorf("shift target state = %d, want 1", shift.State)
	}

	reduce := lang.ParseActions[1].Actions[0]
	if reduce.Type != ParseActionReduce {
		t.Errorf("reduce action type = %d, want %d", reduce.Type, ParseActionReduce)
	}
	if reduce.Symbol != 2 {
		t.Errorf("reduce symbol = %d, want 2", reduce.Symbol)
	}
	if reduce.ChildCount != 1 {
		t.Errorf("reduce child count = %d, want 1", reduce.ChildCount)
	}

	// Verify lex states.
	if len(lang.LexStates) != 2 {
		t.Fatalf("len(LexStates) = %d, want 2", len(lang.LexStates))
	}
	if lang.LexStates[0].AcceptToken != 0 {
		t.Errorf("LexStates[0].AcceptToken = %d, want 0", lang.LexStates[0].AcceptToken)
	}
	if !lang.LexStates[0].Skip {
		t.Error("LexStates[0].Skip = false, want true")
	}
	if lang.LexStates[1].AcceptToken != 1 {
		t.Errorf("LexStates[1].AcceptToken = %d, want 1", lang.LexStates[1].AcceptToken)
	}
	if lang.LexStates[0].Default != -1 {
		t.Errorf("LexStates[0].Default = %d, want -1", lang.LexStates[0].Default)
	}

	// Verify lex transitions.
	if len(lang.LexStates[0].Transitions) != 1 {
		t.Fatalf("len(LexStates[0].Transitions) = %d, want 1", len(lang.LexStates[0].Transitions))
	}
	tr := lang.LexStates[0].Transitions[0]
	if tr.Lo != 'a' || tr.Hi != 'z' {
		t.Errorf("transition range = [%c,%c], want [a,z]", tr.Lo, tr.Hi)
	}
	if tr.NextState != 1 {
		t.Errorf("transition next state = %d, want 1", tr.NextState)
	}

	// Verify lex modes.
	if len(lang.LexModes) != 2 {
		t.Fatalf("len(LexModes) = %d, want 2", len(lang.LexModes))
	}

	// Verify field map.
	if len(lang.FieldMapSlices) != 1 {
		t.Fatalf("len(FieldMapSlices) = %d, want 1", len(lang.FieldMapSlices))
	}
	if lang.FieldMapSlices[0] != [2]uint16{0, 1} {
		t.Errorf("FieldMapSlices[0] = %v, want [0 1]", lang.FieldMapSlices[0])
	}
	if len(lang.FieldMapEntries) != 1 {
		t.Fatalf("len(FieldMapEntries) = %d, want 1", len(lang.FieldMapEntries))
	}
	fme := lang.FieldMapEntries[0]
	if fme.FieldID != 1 {
		t.Errorf("FieldMapEntries[0].FieldID = %d, want 1", fme.FieldID)
	}
	if fme.ChildIndex != 0 {
		t.Errorf("FieldMapEntries[0].ChildIndex = %d, want 0", fme.ChildIndex)
	}
	if fme.Inherited {
		t.Error("FieldMapEntries[0].Inherited = true, want false")
	}

	// Verify primary state IDs.
	if len(lang.PrimaryStateIDs) != 2 {
		t.Fatalf("len(PrimaryStateIDs) = %d, want 2", len(lang.PrimaryStateIDs))
	}

	// Verify nil optional fields.
	if lang.ExternalScanner != nil {
		t.Error("ExternalScanner should be nil for this grammar")
	}
	if lang.KeywordLexStates != nil {
		t.Error("KeywordLexStates should be nil for this grammar")
	}
	if lang.AliasSequences != nil {
		t.Error("AliasSequences should be nil for this grammar")
	}
}

// mockExternalScanner is a minimal ExternalScanner implementation for testing.
type mockExternalScanner struct {
	created   bool
	destroyed bool
	scanned   bool
}

func (m *mockExternalScanner) Create() any {
	m.created = true
	return &struct{ state int }{state: 0}
}

func (m *mockExternalScanner) Destroy(payload any) {
	m.destroyed = true
}

func (m *mockExternalScanner) Serialize(payload any, buf []byte) int {
	if len(buf) > 0 {
		buf[0] = 42
		return 1
	}
	return 0
}

func (m *mockExternalScanner) Deserialize(payload any, buf []byte) {
	// no-op for test
}

func (m *mockExternalScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	m.scanned = true
	return false
}

// TestExternalScannerInterface verifies that ExternalScanner is a proper
// interface: it can be nil on Language, and can be assigned a mock.
func TestExternalScannerInterface(t *testing.T) {
	// A language with no external scanner.
	lang := Language{
		Name: "no_scanner",
	}
	if lang.ExternalScanner != nil {
		t.Fatal("ExternalScanner should be nil by default")
	}

	// Assign a mock scanner.
	mock := &mockExternalScanner{}
	lang.ExternalScanner = mock
	if lang.ExternalScanner == nil {
		t.Fatal("ExternalScanner should not be nil after assignment")
	}

	// Exercise the interface methods.
	payload := lang.ExternalScanner.Create()
	if !mock.created {
		t.Error("Create was not called")
	}
	if payload == nil {
		t.Error("Create returned nil payload")
	}

	buf := make([]byte, 16)
	n := lang.ExternalScanner.Serialize(payload, buf)
	if n != 1 || buf[0] != 42 {
		t.Errorf("Serialize returned n=%d, buf[0]=%d; want n=1, buf[0]=42", n, buf[0])
	}

	lang.ExternalScanner.Deserialize(payload, buf[:n])

	result := lang.ExternalScanner.Scan(payload, nil, []bool{true, false})
	if result {
		t.Error("Scan returned true, want false")
	}
	if !mock.scanned {
		t.Error("Scan was not called")
	}

	lang.ExternalScanner.Destroy(payload)
	if !mock.destroyed {
		t.Error("Destroy was not called")
	}
}

// TestParseActionFields verifies that ParseAction fields for shift and reduce
// actions work correctly with their respective field combinations.
func TestParseActionFields(t *testing.T) {
	shift := ParseAction{
		Type:       ParseActionShift,
		State:      42,
		Extra:      true,
		Repetition: false,
	}
	if shift.State != 42 {
		t.Errorf("shift.State = %d, want 42", shift.State)
	}
	if !shift.Extra {
		t.Error("shift.Extra = false, want true")
	}

	reduce := ParseAction{
		Type:              ParseActionReduce,
		Symbol:            10,
		ChildCount:        3,
		DynamicPrecedence: -5,
		ProductionID:      7,
	}
	if reduce.Symbol != 10 {
		t.Errorf("reduce.Symbol = %d, want 10", reduce.Symbol)
	}
	if reduce.ChildCount != 3 {
		t.Errorf("reduce.ChildCount = %d, want 3", reduce.ChildCount)
	}
	if reduce.DynamicPrecedence != -5 {
		t.Errorf("reduce.DynamicPrecedence = %d, want -5", reduce.DynamicPrecedence)
	}
	if reduce.ProductionID != 7 {
		t.Errorf("reduce.ProductionID = %d, want 7", reduce.ProductionID)
	}

	accept := ParseAction{Type: ParseActionAccept}
	if accept.Type != ParseActionAccept {
		t.Errorf("accept.Type = %d, want %d", accept.Type, ParseActionAccept)
	}

	recover := ParseAction{Type: ParseActionRecover, State: 99}
	if recover.State != 99 {
		t.Errorf("recover.State = %d, want 99", recover.State)
	}
}

func TestLanguageIsSupertype(t *testing.T) {
	lang := &Language{
		SupertypeSymbols: []Symbol{10, 20},
	}
	if !lang.IsSupertype(Symbol(10)) {
		t.Error("10 should be supertype")
	}
	if !lang.IsSupertype(Symbol(20)) {
		t.Error("20 should be supertype")
	}
	if lang.IsSupertype(Symbol(5)) {
		t.Error("5 should not be supertype")
	}
	// nil language edge case
	var nilLang *Language
	if nilLang.IsSupertype(Symbol(1)) {
		t.Error("nil language should return false")
	}
}

func TestLanguageSupertypeChildren(t *testing.T) {
	lang := &Language{
		SupertypeMapSlices:  make([][2]uint16, 11),
		SupertypeMapEntries: []Symbol{20, 21, 22},
	}
	lang.SupertypeMapSlices[10] = [2]uint16{0, 3}

	children := lang.SupertypeChildren(Symbol(10))
	if len(children) != 3 {
		t.Fatalf("got %d children, want 3", len(children))
	}
	if children[0] != 20 || children[1] != 21 || children[2] != 22 {
		t.Errorf("wrong children: %v", children)
	}

	// Out of range
	if got := lang.SupertypeChildren(Symbol(999)); got != nil {
		t.Errorf("out of range should return nil, got %v", got)
	}

	// No entries (zero-value slice at index 5)
	if got := lang.SupertypeChildren(Symbol(5)); got != nil {
		t.Errorf("no entries should return nil, got %v", got)
	}

	// nil language edge case
	var nilLang *Language
	if got := nilLang.SupertypeChildren(Symbol(1)); got != nil {
		t.Errorf("nil language should return nil, got %v", got)
	}
}

// TestTypeAliases verifies that Symbol, StateID, and FieldID are distinct
// types based on uint16, ensuring type safety at compile time.
func TestTypeAliases(t *testing.T) {
	var s Symbol = 100
	var st StateID = 200
	var f FieldID = 50

	// Verify they hold the expected values.
	if s != 100 {
		t.Errorf("Symbol = %d, want 100", s)
	}
	if st != 200 {
		t.Errorf("StateID = %d, want 200", st)
	}
	if f != 50 {
		t.Errorf("FieldID = %d, want 50", f)
	}

	// Verify they can be converted to uint16.
	if uint16(s) != 100 {
		t.Error("Symbol to uint16 conversion failed")
	}
	if uint16(st) != 200 {
		t.Error("StateID to uint16 conversion failed")
	}
	if uint16(f) != 50 {
		t.Error("FieldID to uint16 conversion failed")
	}
}

func TestCcoreTablesOwnedByLanguage(t *testing.T) {
	l := &Language{SymbolNames: []string{"first"}, SymbolMetadata: []SymbolMetadata{{Visible: true}}}
	base := ccoreTablesFor(l)
	nodes := ccoreNodeTablesFor(l)
	if l.ccoreTables == nil || l.ccoreTables.owner != l || l.ccoreTables.base != base || l.ccoreTables.nodes != nodes {
		t.Fatal("cache is not owned by the language")
	}
	// Simulate an inherited cache without copying Language's initialized
	// locks; copying a Language after initialization is unsupported.
	copyLanguage := Language{SymbolNames: l.SymbolNames, SymbolMetadata: l.SymbolMetadata, ccoreTables: l.ccoreTables}
	copiedBase := ccoreTablesFor(&copyLanguage)
	copiedNodes := ccoreNodeTablesFor(&copyLanguage)
	if copiedBase == base || copiedNodes == nodes || copiedNodes.Base != copiedBase {
		t.Fatal("copied language reused original tables")
	}
	if ccoreTablesFor(l) != base || ccoreNodeTablesFor(l) != nodes {
		t.Fatal("original cache changed")
	}
}

func TestCcoreTablesConcurrentInitialization(t *testing.T) {
	l := &Language{SymbolNames: []string{"name"}}
	const workers = 16
	bases := make([]*ccore.Tables, workers)
	nodes := make([]*ccore.NodeTables, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) { defer wg.Done(); bases[i] = ccoreTablesFor(l); nodes[i] = ccoreNodeTablesFor(l) }(i)
	}
	wg.Wait()
	for i := range bases {
		if bases[i] != bases[0] || nodes[i] != nodes[0] || nodes[i].Base != bases[i] {
			t.Fatal("concurrent initializers did not share one consistent cache")
		}
	}
}

//go:noinline
func makeCcoreLanguageLifetimeSentinel(done chan<- struct{}) {
	backing := new([128]SymbolMetadata)
	runtime.SetFinalizer(backing, func(*[128]SymbolMetadata) { close(done) })
	lang := &Language{SymbolMetadata: backing[:]}
	cfg, err := ccoreConfig(lang)
	if err != nil {
		panic(err)
	}
	runtime.KeepAlive(cfg)
	runtime.KeepAlive(lang)
}

func TestCcoreTablesDoNotRetainLanguage(t *testing.T) {
	done := make(chan struct{})
	makeCcoreLanguageLifetimeSentinel(done)
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		runtime.GC()
		select {
		case <-done:
			return
		case <-deadline.C:
			t.Fatal("ccore tables retained the language's backing data, or its finalizer did not run")
		case <-time.After(20 * time.Millisecond):
		}
	}
}
