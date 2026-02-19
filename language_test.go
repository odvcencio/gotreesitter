package gotreesitter

import "testing"

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

// TestMinimalLanguage constructs a minimal 3-symbol, 2-state grammar
// and verifies that all fields are correctly defined and accessible.
func TestMinimalLanguage(t *testing.T) {
	// Symbols: 0=ERROR, 1=identifier (terminal), 2=expression (nonterminal)
	lang := Language{
		Name:              "test",
		SymbolCount:       3,
		TokenCount:        2,
		ExternalTokenCount: 0,
		StateCount:        2,
		LargeStateCount:   0,
		FieldCount:        1,
		ProductionIDCount: 1,

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

func (m *mockExternalScanner) Create() interface{} {
	m.created = true
	return &struct{ state int }{state: 0}
}

func (m *mockExternalScanner) Destroy(payload interface{}) {
	m.destroyed = true
}

func (m *mockExternalScanner) Serialize(payload interface{}, buf []byte) int {
	if len(buf) > 0 {
		buf[0] = 42
		return 1
	}
	return 0
}

func (m *mockExternalScanner) Deserialize(payload interface{}, buf []byte) {
	// no-op for test
}

func (m *mockExternalScanner) Scan(payload interface{}, lexer interface{}, validSymbols []bool) bool {
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
