package gotreesitter

import (
	"bytes"
	"testing"
	"time"
)

type parserTestUnsafeExternalScanner struct{}

func (parserTestUnsafeExternalScanner) Create() any                           { return nil }
func (parserTestUnsafeExternalScanner) Destroy(payload any)                   {}
func (parserTestUnsafeExternalScanner) Serialize(payload any, buf []byte) int { return 0 }
func (parserTestUnsafeExternalScanner) Deserialize(payload any, buf []byte)   {}
func (parserTestUnsafeExternalScanner) Scan(payload any, lexer *ExternalLexer, validSymbols []bool) bool {
	return false
}

type parserTestSafeExternalScanner struct {
	parserTestUnsafeExternalScanner
}

func (parserTestSafeExternalScanner) SupportsIncrementalReuse() bool { return true }

func TestRepetitionShiftConflictChoice(t *testing.T) {
	chosen, ok := repetitionShiftConflictChoice([]ParseAction{
		{Type: ParseActionReduce, Symbol: 191, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	})
	if !ok {
		t.Fatal("repetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 1245 || !chosen.Repetition {
		t.Fatalf("repetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestRepetitionShiftConflictChoiceRejectsNonRepetitionShift(t *testing.T) {
	if _, ok := repetitionShiftConflictChoice([]ParseAction{
		{Type: ParseActionReduce, Symbol: 191, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: false},
	}); ok {
		t.Fatal("repetitionShiftConflictChoice = true, want false")
	}
}

func TestCSharpRepetitionShiftConflictChoice(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "this", "block_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	}

	chosen, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 2, Text: "this"}, actions)
	if !ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 1245 || !chosen.Repetition {
		t.Fatalf("csharpRepetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestCSharpRepetitionShiftConflictChoiceRejectsScopedContextualIdentifier(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "this", "block_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	}

	if _, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 1, Text: "scoped"}, actions); ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = true, want false")
	}
}

func TestCSharpRepetitionShiftConflictChoiceAllowsDeclarationLists(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "private", "declaration_list_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 3237, Repetition: true},
	}

	chosen, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 1, Text: "private"}, actions)
	if !ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 3237 || !chosen.Repetition {
		t.Fatalf("csharpRepetitionShiftConflictChoice picked %+v, want declaration-list shift", chosen)
	}
}

func TestCSharpRepetitionShiftConflictChoiceRejectsOtherRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "this", "argument_list_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 1245, Repetition: true},
	}

	if _, ok := csharpRepetitionShiftConflictChoice(lang, Token{Symbol: 1, Text: "this"}, actions); ok {
		t.Fatal("csharpRepetitionShiftConflictChoice = true, want false")
	}
}

func TestRRepetitionShiftConflictChoiceAllowsHotRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "program_repeat1", "braced_expression_repeat1"}}
	for _, tc := range []struct {
		name       string
		state      StateID
		reduceSym  Symbol
		shiftState StateID
	}{
		{name: "program", state: 448, reduceSym: 2, shiftState: 446},
		{name: "braced_expression", state: 445, reduceSym: 3, shiftState: 446},
	} {
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSym, ChildCount: 2},
			{Type: ParseActionShift, State: tc.shiftState, Repetition: true},
		}
		chosen, ok := rRepetitionShiftConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("rRepetitionShiftConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionShift || chosen.State != tc.shiftState || !chosen.Repetition {
			t.Fatalf("rRepetitionShiftConflictChoice(%s) picked %+v, want repetition shift", tc.name, chosen)
		}
	}
}

func TestRRepetitionShiftConflictChoiceRejectsWrongReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "program_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 446, Repetition: true},
	}
	if _, ok := rRepetitionShiftConflictChoice(lang, 448, actions); ok {
		t.Fatal("rRepetitionShiftConflictChoice = true, want false for wrong repeat symbol")
	}
}

func TestTypeScriptRepetitionShiftConflictChoiceAllowsHotProgramRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "function", "identifier", "const", "return", "if", "export", "program_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 7, ChildCount: 2},
		{Type: ParseActionShift, State: 3693, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5, 6} {
		chosen, ok := typescriptRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 9, actions)
		if !ok {
			t.Fatalf("typescriptRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 3693 || !chosen.Repetition {
			t.Fatalf("typescriptRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestTypeScriptRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "function", "program_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 3693, Repetition: true},
	}

	if _, ok := typescriptRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 10, actions); ok {
		t.Fatal("typescriptRepetitionShiftConflictChoice = true, want false")
	}
}

func TestPythonRepetitionShiftConflictChoiceAllowsHotModuleRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "def", "module_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 616, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2} {
		chosen, ok := pythonRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 72, actions)
		if !ok {
			t.Fatalf("pythonRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 616 || !chosen.Repetition {
			t.Fatalf("pythonRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestPythonRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "module_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 616, Repetition: true},
	}

	if _, ok := pythonRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 73, actions); ok {
		t.Fatal("pythonRepetitionShiftConflictChoice = true, want false")
	}
}

func TestPHPRepetitionShiftConflictChoiceAllowsProgramRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "namespace", "\\", "name", "use", "new", "program_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 6, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5} {
		chosen, ok := phpRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 2, actions)
		if !ok {
			t.Fatalf("phpRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 1846 || !chosen.Repetition {
			t.Fatalf("phpRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestPHPRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "namespace", "program_repeat1", "arguments_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}
	if _, ok := phpRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 2, actions); ok {
		t.Fatal("phpRepetitionShiftConflictChoice = true, want false")
	}
}

func TestPHPRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "namespace", "program_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 1846, Repetition: true},
	}
	if _, ok := phpRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 3, actions); ok {
		t.Fatal("phpRepetitionShiftConflictChoice = true, want false")
	}
}

func TestPerlRepetitionShiftConflictChoiceAllowsHotRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "source_file_repeat1", "heredoc_content_repeat1"}}
	for _, tc := range []struct {
		name       string
		state      StateID
		reduceSym  Symbol
		shiftState StateID
	}{
		{name: "source_file", state: 27, reduceSym: 1, shiftState: 28},
		{name: "heredoc_content", state: 2853, reduceSym: 2, shiftState: 2854},
	} {
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSym, ChildCount: 2},
			{Type: ParseActionShift, State: tc.shiftState, Repetition: true},
		}
		chosen, ok := perlRepetitionShiftConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("perlRepetitionShiftConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionShift || chosen.State != tc.shiftState || !chosen.Repetition {
			t.Fatalf("perlRepetitionShiftConflictChoice(%s) picked %+v, want repetition shift", tc.name, chosen)
		}
	}
}

func TestPerlRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "source_file_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 28, Repetition: true},
	}
	if _, ok := perlRepetitionShiftConflictChoice(lang, 27, actions); ok {
		t.Fatal("perlRepetitionShiftConflictChoice = true, want false")
	}
}

func TestPerlRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 2},
		{Type: ParseActionShift, State: 28, Repetition: true},
	}
	if _, ok := perlRepetitionShiftConflictChoice(lang, 28, actions); ok {
		t.Fatal("perlRepetitionShiftConflictChoice = true, want false")
	}
}

func TestPerlRepetitionShiftConflictChoiceRejectsGrammargenLanguage(t *testing.T) {
	lang := &Language{
		GeneratedByGrammargen: true,
		SymbolNames:           []string{"end", "source_file_repeat1"},
	}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 2},
		{Type: ParseActionShift, State: 28, Repetition: true},
	}
	if _, ok := perlRepetitionShiftConflictChoice(lang, 27, actions); ok {
		t.Fatal("perlRepetitionShiftConflictChoice = true, want false for grammargen language")
	}
}

func TestSQLRepetitionShiftConflictChoiceAllowsSelectClauseComma(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "select_clause_body_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 7016, Repetition: true},
	}

	chosen, ok := sqlRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 10852, actions)
	if !ok {
		t.Fatal("sqlRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 7016 || !chosen.Repetition {
		t.Fatalf("sqlRepetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestSQLRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "select_clause_body_repeat1", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 7016, Repetition: true},
	}

	if _, ok := sqlRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 10852, actions); ok {
		t.Fatal("sqlRepetitionShiftConflictChoice = true, want false")
	}
}

func TestSQLRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "select_clause_body_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 7016, Repetition: true},
	}

	if _, ok := sqlRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 10853, actions); ok {
		t.Fatal("sqlRepetitionShiftConflictChoice = true, want false")
	}
}

func TestDartRepetitionShiftConflictChoiceAllowsHotRepeats(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state        StateID
		reduceSymbol Symbol
	}{
		{name: "enum_body_repeat2", state: 596, reduceSymbol: 1},
		{name: "extension_body_repeat1", state: 602, reduceSymbol: 2},
		{name: "program_repeat4", state: 479, reduceSymbol: 3},
	} {
		lang := &Language{SymbolNames: []string{"end", "enum_body_repeat2", "extension_body_repeat1", "program_repeat4"}}
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSymbol, ChildCount: 2},
			{Type: ParseActionShift, State: 9, Repetition: true},
		}

		chosen, ok := dartRepetitionShiftConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("dartRepetitionShiftConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionShift || chosen.State != 9 || !chosen.Repetition {
			t.Fatalf("dartRepetitionShiftConflictChoice(%s) picked %+v, want repetition shift", tc.name, chosen)
		}
	}
}

func TestDartRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "enum_body_repeat2", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 9, Repetition: true},
	}
	if _, ok := dartRepetitionShiftConflictChoice(lang, 596, actions); ok {
		t.Fatal("dartRepetitionShiftConflictChoice = true, want false")
	}
}

func TestHCLRepetitionShiftConflictChoiceAllowsTemplateLiteralRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "_template_literal_chunk", "template_literal_repeat1"}}
	for _, state := range []StateID{426, 541} {
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
			{Type: ParseActionShift, State: state, Repetition: true},
		}
		chosen, ok := hclRepetitionShiftConflictChoice(lang, state, actions)
		if !ok {
			t.Fatalf("hclRepetitionShiftConflictChoice(state %d) = false, want true", state)
		}
		if chosen.Type != ParseActionShift || chosen.State != state || !chosen.Repetition {
			t.Fatalf("hclRepetitionShiftConflictChoice(state %d) picked %+v, want repetition shift", state, chosen)
		}
	}
}

func TestHCLRepetitionShiftConflictChoiceAllowsBodyRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "body_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 406, Repetition: true},
	}
	chosen, ok := hclRepetitionShiftConflictChoice(lang, 408, actions)
	if !ok {
		t.Fatal("hclRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 406 || !chosen.Repetition {
		t.Fatalf("hclRepetitionShiftConflictChoice picked %+v, want body repeat shift", chosen)
	}
}

func TestHCLRepetitionShiftConflictChoiceRejectsOtherBodyReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "template_literal_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 406, Repetition: true},
	}
	if _, ok := hclRepetitionShiftConflictChoice(lang, 408, actions); ok {
		t.Fatal("hclRepetitionShiftConflictChoice = true, want false for wrong body reduce")
	}
}

func TestHaskellRepeatBoundaryConflictChoiceAllowsHotRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "_cond_layout_semicolon", ",", "declarations_repeat1", "exports_repeat1"}}
	for _, tc := range []struct {
		name         string
		state        StateID
		reduceSymbol Symbol
		shiftState   StateID
	}{
		{name: "declarations", state: 9609, reduceSymbol: 3, shiftState: 21},
		{name: "exports", state: 10984, reduceSymbol: 4, shiftState: 7942},
	} {
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSymbol, ChildCount: 2},
			{Type: ParseActionShift, State: tc.shiftState, Repetition: true},
		}
		chosen, ok := haskellRepeatBoundaryConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("haskellRepeatBoundaryConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionReduce || chosen.Symbol != tc.reduceSymbol || chosen.Repetition {
			t.Fatalf("haskellRepeatBoundaryConflictChoice(%s) picked %+v, want repeat reduce", tc.name, chosen)
		}
	}
}

func TestHaskellRepeatBoundaryConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "declarations_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 21, Repetition: true},
	}
	if _, ok := haskellRepeatBoundaryConflictChoice(lang, 9609, actions); ok {
		t.Fatal("haskellRepeatBoundaryConflictChoice = true, want false for wrong repeat symbol")
	}
}

func TestHaskellRepeatBoundaryConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "declarations_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 2},
		{Type: ParseActionShift, State: 21, Repetition: true},
	}
	if _, ok := haskellRepeatBoundaryConflictChoice(lang, 9610, actions); ok {
		t.Fatal("haskellRepeatBoundaryConflictChoice = true, want false for wrong state")
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceAllowsHotRR(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "{", "_simple_user_type", "_expression"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	}

	chosen, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 2815, actions)
	if !ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 2 {
		t.Fatalf("swiftBraceTypeExpressionConflictChoice picked %+v, want _simple_user_type reduce", chosen)
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "{", "_simple_user_type", "_expression"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	}

	if _, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 2816, actions); ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice = true, want false")
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceRejectsOtherReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "{", "_simple_user_type", "other"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
	}

	if _, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 2815, actions); ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice = true, want false")
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceAllowsNavigableDotReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ".", "_navigable_type_expression"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionShift, State: 201},
	}

	chosen, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 72, actions)
	if !ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice(dot) = false, want true")
	}
	if chosen.Type != ParseActionReduce || chosen.Symbol != 2 {
		t.Fatalf("swiftBraceTypeExpressionConflictChoice(dot) picked %+v, want _navigable_type_expression reduce", chosen)
	}
}

func TestSwiftBraceTypeExpressionConflictChoiceRejectsOtherDotReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ".", "_navigable_type_expression", "other"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 201},
	}

	if _, ok := swiftBraceTypeExpressionConflictChoice(lang, Token{Symbol: 1}, 72, actions); ok {
		t.Fatal("swiftBraceTypeExpressionConflictChoice(dot) = true, want false")
	}
}

func TestDRepetitionShiftConflictChoiceAllowsDeclarationsAndStatements(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "_declarations_and_statements"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 1, ChildCount: 2},
		{Type: ParseActionShift, State: 9, Repetition: true},
	}

	chosen, ok := dRepetitionShiftConflictChoice(lang, 118, actions)
	if !ok {
		t.Fatal("dRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 9 || !chosen.Repetition {
		t.Fatalf("dRepetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestDRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "_declarations_and_statements", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 9, Repetition: true},
	}
	if _, ok := dRepetitionShiftConflictChoice(lang, 118, actions); ok {
		t.Fatal("dRepetitionShiftConflictChoice = true, want false")
	}
}

func TestClojureRepetitionShiftConflictChoiceAllowsHotRepeats(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state        StateID
		reduceSymbol Symbol
	}{
		{name: "source_repeat1", state: 20, reduceSymbol: 1},
		{name: "_bare_list_lit_repeat1", state: 2, reduceSymbol: 2},
	} {
		lang := &Language{SymbolNames: []string{"end", "source_repeat1", "_bare_list_lit_repeat1"}}
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSymbol, ChildCount: 2},
			{Type: ParseActionShift, State: 9, Repetition: true},
		}

		chosen, ok := clojureRepetitionShiftConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("clojureRepetitionShiftConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionShift || chosen.State != 9 || !chosen.Repetition {
			t.Fatalf("clojureRepetitionShiftConflictChoice(%s) picked %+v, want repetition shift", tc.name, chosen)
		}
	}
}

func TestClojureRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "source_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 9, Repetition: true},
	}
	if _, ok := clojureRepetitionShiftConflictChoice(lang, 20, actions); ok {
		t.Fatal("clojureRepetitionShiftConflictChoice = true, want false")
	}
}

func TestAwkRepetitionShiftConflictChoiceAllowsHotRepeats(t *testing.T) {
	for _, tc := range []struct {
		name         string
		state        StateID
		reduceSymbol Symbol
	}{
		{name: "block_repeat1", state: 8, reduceSymbol: 1},
		{name: "program_repeat1", state: 303, reduceSymbol: 2},
		{name: "_regex_bracket_exp_repeat1", state: 2107, reduceSymbol: 3},
		{name: "regex_pattern_repeat1", state: 2120, reduceSymbol: 4},
	} {
		lang := &Language{SymbolNames: []string{"end", "block_repeat1", "program_repeat1", "_regex_bracket_exp_repeat1", "regex_pattern_repeat1"}}
		actions := []ParseAction{
			{Type: ParseActionReduce, Symbol: tc.reduceSymbol, ChildCount: 2},
			{Type: ParseActionShift, State: 9, Repetition: true},
		}

		chosen, ok := awkRepetitionShiftConflictChoice(lang, tc.state, actions)
		if !ok {
			t.Fatalf("awkRepetitionShiftConflictChoice(%s) = false, want true", tc.name)
		}
		if chosen.Type != ParseActionShift || chosen.State != 9 || !chosen.Repetition {
			t.Fatalf("awkRepetitionShiftConflictChoice(%s) picked %+v, want repetition shift", tc.name, chosen)
		}
	}
}

func TestAwkRepetitionShiftConflictChoiceRejectsOtherRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "program_repeat1", "other_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 9, Repetition: true},
	}
	if _, ok := awkRepetitionShiftConflictChoice(lang, 303, actions); ok {
		t.Fatal("awkRepetitionShiftConflictChoice = true, want false")
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsSourceFileRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", ";", "..", "source_file_repeat1", "_non_special_token_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 4, ChildCount: 2},
		{Type: ParseActionShift, State: 535, Repetition: true},
	}

	for _, sym := range []Symbol{2} {
		chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 12, actions)
		if !ok {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 535 || !chosen.Repetition {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsNonSpecialTokenRepeat(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "..", "_non_special_token_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 232, Repetition: true},
	}

	chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 193, actions)
	if !ok {
		t.Fatal("rustRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 232 || !chosen.Repetition {
		t.Fatalf("rustRepetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestTSXRepetitionReduceConflictChoiceAllowsHotRepeats(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", ";", "_jsx_start_opening_element_repeat1", "object_type_repeat1", "const", "let", "program_repeat1", "import", ",", "object_pattern_repeat1"}}
	for _, tc := range []struct {
		name      string
		state     StateID
		lookahead Symbol
		reduceSym Symbol
		wantType  ParseActionType
	}{
		{name: "jsx opening element", state: 3468, lookahead: 1, reduceSym: 3, wantType: ParseActionReduce},
		{name: "object type semicolon", state: 3885, lookahead: 2, reduceSym: 4, wantType: ParseActionReduce},
		{name: "program const", state: 9, lookahead: 5, reduceSym: 7, wantType: ParseActionReduce},
		{name: "program let", state: 9, lookahead: 6, reduceSym: 7, wantType: ParseActionReduce},
		{name: "program import", state: 9, lookahead: 8, reduceSym: 7, wantType: ParseActionReduce},
		{name: "object pattern comma", state: 4615, lookahead: 9, reduceSym: 10, wantType: ParseActionShift},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actions := []ParseAction{
				{Type: ParseActionReduce, Symbol: tc.reduceSym, ChildCount: 2},
				{Type: ParseActionShift, State: 3552, Repetition: true},
			}
			chosen, ok := tsxRepetitionReduceConflictChoice(lang, Token{Symbol: tc.lookahead}, tc.state, actions)
			if !ok {
				t.Fatal("tsxRepetitionReduceConflictChoice = false, want true")
			}
			if chosen.Type != tc.wantType {
				t.Fatalf("tsxRepetitionReduceConflictChoice picked %+v, want type %v", chosen, tc.wantType)
			}
			if tc.wantType == ParseActionReduce && chosen.Symbol != tc.reduceSym {
				t.Fatalf("tsxRepetitionReduceConflictChoice picked %+v, want reduce symbol %d", chosen, tc.reduceSym)
			}
			if tc.wantType == ParseActionShift && !chosen.Repetition {
				t.Fatalf("tsxRepetitionReduceConflictChoice picked %+v, want repetition shift", chosen)
			}
		})
	}
}

func TestTSXRepetitionReduceConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "_jsx_start_opening_element_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 3552, Repetition: true},
	}
	if _, ok := tsxRepetitionReduceConflictChoice(lang, Token{Symbol: 1}, 3469, actions); ok {
		t.Fatal("tsxRepetitionReduceConflictChoice = true, want false")
	}
}

func TestTSXRepetitionReduceConflictChoiceRejectsWrongReduce(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "_jsx_start_opening_element_repeat1", "wrong_repeat"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 2},
		{Type: ParseActionShift, State: 3552, Repetition: true},
	}
	if _, ok := tsxRepetitionReduceConflictChoice(lang, Token{Symbol: 1}, 3468, actions); ok {
		t.Fatal("tsxRepetitionReduceConflictChoice = true, want false")
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsTopLevelItemStarts(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "pub", "#", "impl", "fn", "mod", "use", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 7, ChildCount: 2},
		{Type: ParseActionShift, State: 2039, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5, 6} {
		chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 7, actions)
		if !ok {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 2039 || !chosen.Repetition {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestRustRepetitionShiftConflictChoiceRejectsOtherState(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "pub", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 2039, Repetition: true},
	}

	if _, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 8, actions); ok {
		t.Fatal("rustRepetitionShiftConflictChoice = true, want false")
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsTokenTreeRepeat(t *testing.T) {
	// state 83 = delim_token_tree_repeat1 (macro token-tree contents). The
	// collapse is gated on the reduce symbol, not the lookahead, so it covers
	// every continuation token — identifiers, the old listed punctuation, AND
	// previously-uncovered operators ("+", "<<") — that continue the tree.
	lang := &Language{SymbolNames: []string{"end", "identifier", ",", "(", "primitive_type", "::", ".", ";", "delim_token_tree_repeat1", "+", "<<", "block_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 8, ChildCount: 2},
		{Type: ParseActionShift, State: 246, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2, 3, 4, 5, 6, 7, 9, 10} {
		chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: sym}, 83, actions)
		if !ok {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 246 || !chosen.Repetition {
			t.Fatalf("rustRepetitionShiftConflictChoice(%q) picked %+v, want repetition shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsSourceFileRepeatAtTokenTreeBoundary(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "token_tree", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 165, Repetition: true},
	}

	chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 83, actions)
	if !ok {
		t.Fatal("rustRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 165 || !chosen.Repetition {
		t.Fatalf("rustRepetitionShiftConflictChoice picked %+v, want repetition shift", chosen)
	}
}

func TestRustRepetitionShiftConflictChoiceAllowsSourceFileRepeatAtCommentBoundary(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "line_comment", "source_file_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 165, Repetition: true},
	}

	for _, state := range []StateID{175, 205} {
		chosen, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, state, actions)
		if !ok {
			t.Fatalf("rustRepetitionShiftConflictChoice(state %d) = false, want true", state)
		}
		if chosen.Type != ParseActionShift || chosen.State != 165 || !chosen.Repetition {
			t.Fatalf("rustRepetitionShiftConflictChoice(state %d) picked %+v, want repetition shift", state, chosen)
		}
	}
}

// TestRustRepetitionShiftConflictChoiceRejectsNonTokenTreeReduceAtState83 proves
// the state-83 collapse stays scoped to known Rust repeat boundaries: a conflict
// whose reduce closes a different repeat must NOT be collapsed.
func TestRustRepetitionShiftConflictChoiceRejectsNonTokenTreeReduceAtState83(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", ",", "(", "primitive_type", "::", ".", ";", "delim_token_tree_repeat1", "+", "<<", "block_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 11, ChildCount: 2}, // block_repeat1, not delim_token_tree_repeat1
		{Type: ParseActionShift, State: 246, Repetition: true},
	}
	if _, ok := rustRepetitionShiftConflictChoice(lang, Token{Symbol: 1}, 83, actions); ok {
		t.Fatal("rustRepetitionShiftConflictChoice collapsed a non-token-tree reduce at state 83, want false")
	}
}

func TestJavaRepetitionShiftConflictChoiceAllowsStringLiteralContinuation(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "escape_sequence", "string_fragment", "_string_literal_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 3, ChildCount: 1},
		{Type: ParseActionShift, State: 983, Repetition: true},
	}

	for _, sym := range []Symbol{1, 2} {
		chosen, ok := javaRepetitionShiftConflictChoice(lang, nil, Token{Symbol: sym}, 983, actions)
		if !ok {
			t.Fatalf("javaRepetitionShiftConflictChoice(%q) = false, want true", lang.SymbolNames[sym])
		}
		if chosen.Type != ParseActionShift || chosen.State != 983 || !chosen.Repetition {
			t.Fatalf("javaRepetitionShiftConflictChoice(%q) picked %+v, want string repeat shift", lang.SymbolNames[sym], chosen)
		}
	}
}

func TestJavaRepetitionShiftConflictChoiceRejectsOtherStringLiteralLookahead(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", "identifier", "_string_literal_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 1},
		{Type: ParseActionShift, State: 983, Repetition: true},
	}

	if _, ok := javaRepetitionShiftConflictChoice(lang, nil, Token{Symbol: 1}, 983, actions); ok {
		t.Fatal("javaRepetitionShiftConflictChoice = true, want false")
	}
}

func TestJavaRepetitionShiftConflictChoiceAllowsArrayInitializerSeparator(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "array_initializer_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 145, Repetition: true},
	}
	source := []byte(`class T { int[] values = { 1, /* keep going */ 2 }; }`)
	comma := uint32(bytes.IndexByte(source, ',') + 1)

	chosen, ok := javaRepetitionShiftConflictChoice(lang, source, Token{Symbol: 1, EndByte: comma}, 1104, actions)
	if !ok {
		t.Fatal("javaRepetitionShiftConflictChoice = false, want true")
	}
	if chosen.Type != ParseActionShift || chosen.State != 145 || !chosen.Repetition {
		t.Fatalf("javaRepetitionShiftConflictChoice picked %+v, want array initializer comma shift", chosen)
	}
}

func TestJavaRepetitionShiftConflictChoiceRejectsArrayInitializerTrailingComma(t *testing.T) {
	lang := &Language{SymbolNames: []string{"end", ",", "array_initializer_repeat1"}}
	actions := []ParseAction{
		{Type: ParseActionReduce, Symbol: 2, ChildCount: 2},
		{Type: ParseActionShift, State: 145, Repetition: true},
	}
	source := []byte("class T { int[] values = { 1, // trailing\n}; }")
	comma := uint32(bytes.IndexByte(source, ',') + 1)

	if _, ok := javaRepetitionShiftConflictChoice(lang, source, Token{Symbol: 1, EndByte: comma}, 1104, actions); ok {
		t.Fatal("javaRepetitionShiftConflictChoice = true, want false for trailing comma")
	}
}

func TestShouldRetryNodeLimitParse(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      300_000,
			NodesAllocated: 300_001,
		},
	}

	if !shouldRetryNodeLimitParse(tree, 4096) {
		t.Fatal("shouldRetryNodeLimitParse = false, want true")
	}
}

func TestShouldNotRetryNodeLimitParseForLargeSource(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      300_000,
			NodesAllocated: 300_001,
		},
	}

	if shouldRetryNodeLimitParse(tree, fullParseRetryMaxSourceBytes+1) {
		t.Fatal("shouldRetryNodeLimitParse = true, want false")
	}
}

func TestShouldNotRetryMemoryBudgetParse(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason: ParseStopMemoryBudget,
		},
	}

	if shouldRetryNodeLimitParse(tree, 4096) {
		t.Fatal("shouldRetryNodeLimitParse = true, want false for memory budget stop")
	}
}

func TestFullParseRetryNodeLimitOverride(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      300_000,
			NodesAllocated: 300_001,
		},
	}

	got := fullParseRetryNodeLimitOverride(tree, 4096)
	want := 600_000
	if got != want {
		t.Fatalf("fullParseRetryNodeLimitOverride = %d, want %d", got, want)
	}
}

func TestFullParseRetrySecondaryNodeLimitOverride(t *testing.T) {
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason:     ParseStopNodeLimit,
			NodeLimit:      600_000,
			NodesAllocated: 600_001,
		},
	}

	got := fullParseRetrySecondaryNodeLimitOverride(tree, 4096)
	want := 1_800_000
	if got != want {
		t.Fatalf("fullParseRetrySecondaryNodeLimitOverride = %d, want %d", got, want)
	}
}

func TestShouldRunInitialFullParseMergeRetry(t *testing.T) {
	if shouldRunInitialFullParseMergeRetry(nil) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(nil) = true, want false")
	}
	tree := &Tree{
		parseRuntime: ParseRuntime{
			StopReason: ParseStopNodeLimit,
		},
	}
	if shouldRunInitialFullParseMergeRetry(tree) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(node_limit) = true, want false")
	}
	tree.parseRuntime.StopReason = ParseStopNoStacksAlive
	if !shouldRunInitialFullParseMergeRetry(tree) {
		t.Fatal("shouldRunInitialFullParseMergeRetry(no_stacks_alive) = false, want true")
	}
}

func TestCppAcceptedErrorRetrySkipsCompleteTree(t *testing.T) {
	tree := &Tree{
		language: &Language{Name: "cpp"},
		root: &Node{
			endByte: 128,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
			MaxStacksSeen:   18,
		},
	}

	if shouldRetryAcceptedErrorParse(tree, 128, 18) {
		t.Fatal("shouldRetryAcceptedErrorParse(cpp complete accepted error) = true, want false")
	}
	if got := fullParseRetryMergePerKeyOverride(tree, 128, 18); got != 0 {
		t.Fatalf("fullParseRetryMergePerKeyOverride(cpp complete accepted error) = %d, want 0", got)
	}
}

func TestCppAcceptedErrorRetryPreservesTruncatedMergeRetry(t *testing.T) {
	tree := &Tree{
		language: &Language{Name: "cpp"},
		root: &Node{
			endByte: 96,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     96,
			Truncated:       true,
			MaxStacksSeen:   18,
		},
	}

	if got := fullParseRetryMergePerKeyOverride(tree, 128, 18); got != fullParseRetryMaxMergePerKey {
		t.Fatalf("fullParseRetryMergePerKeyOverride(cpp truncated accepted error) = %d, want %d", got, fullParseRetryMaxMergePerKey)
	}
}

func TestJavaAcceptedErrorRetryUsesWideMergeRetry(t *testing.T) {
	errChild := &Node{
		symbol: errorSymbol,
		flags:  nodeFlagHasError,
	}
	tree := &Tree{
		language: &Language{Name: "java"},
		root: &Node{
			endByte: 128,
			children: []*Node{
				errChild,
			},
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 128,
			RootEndByte:     128,
			MaxStacksSeen:   1,
		},
	}

	if got := fullParseRetryMergePerKeyOverride(tree, 128, 6); got != javaFullParseRetryMaxMergePerKey {
		t.Fatalf("fullParseRetryMergePerKeyOverride(java accepted error) = %d, want %d", got, javaFullParseRetryMaxMergePerKey)
	}
}

func TestShouldRepeatExternalScannerFullParseSkipsDart(t *testing.T) {
	tree := &Tree{
		root: &Node{
			flags: nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason: ParseStopAccepted,
		},
	}
	scanner := parserTestUnsafeExternalScanner{}

	if shouldRepeatExternalScannerFullParse(&Language{Name: "dart", ExternalScanner: scanner}, tree) {
		t.Fatal("shouldRepeatExternalScannerFullParse(dart accepted error) = true, want false")
	}
	if shouldRepeatExternalScannerFullParse(&Language{Name: "python", ExternalScanner: scanner}, tree) {
		t.Fatal("shouldRepeatExternalScannerFullParse(python accepted error) = true, want false")
	}
	if !shouldRepeatExternalScannerFullParse(&Language{Name: "ruby", ExternalScanner: scanner}, tree) {
		t.Fatal("shouldRepeatExternalScannerFullParse(ruby accepted error) = false, want true")
	}
}

func TestRetryFullParseStopsSchedulingRetriesAfterTimeout(t *testing.T) {
	parser := &Parser{timeoutMicros: 500}
	source := []byte("1+")
	initial := &Tree{
		root: &Node{
			endByte: 1,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: uint32(len(source)),
			MaxStacksSeen:   2,
			NodesAllocated:  20,
		},
	}
	retry := &Tree{
		root: &Node{
			endByte: 2,
			flags:   nodeFlagHasError,
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: uint32(len(source)),
			MaxStacksSeen:   2,
			NodesAllocated:  10,
		},
	}
	calls := 0

	got := parser.retryFullParse(source, 2, initial, func(maxStacks, maxMergePerKeyOverride, maxNodes int) *Tree {
		calls++
		if calls != 1 {
			t.Fatalf("runRetry called %d times, want exactly one retry before timeout cutoff", calls)
		}
		if maxMergePerKeyOverride == 0 {
			t.Fatalf("first retry maxMergePerKeyOverride = 0, want initial merge retry")
		}
		time.Sleep(2 * time.Millisecond)
		return retry
	})

	if got != retry {
		t.Fatalf("retryFullParse returned %p, want retry tree %p", got, retry)
	}
	if calls != 1 {
		t.Fatalf("runRetry calls = %d, want 1", calls)
	}
}

func TestParseForRecoveryReusesRecoveryParser(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	tree, err := parser.parseForRecovery([]byte("1+2"))
	if err != nil {
		t.Fatalf("first parseForRecovery error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("first parseForRecovery returned nil tree/root")
	}
	tree.Release()

	first := parser.recoveryParser
	if first == nil {
		t.Fatal("recoveryParser = nil after first parseForRecovery")
	}
	if !first.skipRecoveryReparse {
		t.Fatal("recoveryParser.skipRecoveryReparse = false, want true")
	}

	tree, err = parser.parseForRecovery([]byte("3+4"))
	if err != nil {
		t.Fatalf("second parseForRecovery error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("second parseForRecovery returned nil tree/root")
	}
	tree.Release()

	if parser.recoveryParser != first {
		t.Fatal("parseForRecovery did not reuse recoveryParser instance")
	}
}

func TestResetSnippetParserClearsTransientState(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.reparseFactory = func(source []byte) (TokenSource, error) { return nil, nil }
	parser.recoveryParser = NewParser(buildArithmeticLanguage())
	parser.skipRecoveryReparse = true
	parser.fullArenaHint = 123
	parser.compactFullArenaHint = 456
	parser.included = []Range{{StartByte: 1, EndByte: 2}}
	parser.logger = func(kind ParserLogType, message string) {}
	parser.glrTrace = true
	parser.timeoutMicros = 99
	flag := uint32(1)
	parser.cancellationFlag = &flag
	parser.parseBudgetDepth = 1
	parser.parseDeadline = time.Now()
	parser.parseStoppedReason = ParseStopTimeout

	resetSnippetParser(parser)

	if parser.reparseFactory != nil {
		t.Fatal("resetSnippetParser did not clear reparseFactory")
	}
	if parser.recoveryParser != nil {
		t.Fatal("resetSnippetParser did not clear recoveryParser")
	}
	if parser.skipRecoveryReparse {
		t.Fatal("resetSnippetParser did not clear skipRecoveryReparse")
	}
	if parser.fullArenaHint != 0 {
		t.Fatal("resetSnippetParser did not clear fullArenaHint")
	}
	if parser.compactFullArenaHint != 0 {
		t.Fatal("resetSnippetParser did not clear compactFullArenaHint")
	}
	if len(parser.included) != 0 {
		t.Fatal("resetSnippetParser did not clear included ranges")
	}
	if parser.logger != nil {
		t.Fatal("resetSnippetParser did not clear logger")
	}
	if parser.glrTrace {
		t.Fatal("resetSnippetParser did not clear glrTrace")
	}
	if parser.timeoutMicros != 0 {
		t.Fatal("resetSnippetParser did not clear timeoutMicros")
	}
	if parser.cancellationFlag != nil {
		t.Fatal("resetSnippetParser did not clear cancellationFlag")
	}
	if parser.parseBudgetDepth != 0 {
		t.Fatal("resetSnippetParser did not clear parseBudgetDepth")
	}
	if !parser.parseDeadline.IsZero() {
		t.Fatal("resetSnippetParser did not clear parseDeadline")
	}
	if parser.parseStoppedReason != ParseStopNone {
		t.Fatal("resetSnippetParser did not clear parseStoppedReason")
	}
}

func TestParseWithSnippetParserParsesSource(t *testing.T) {
	tree, err := parseWithSnippetParser(buildArithmeticLanguage(), []byte("1+2"))
	if err != nil {
		t.Fatalf("parseWithSnippetParser error: %v", err)
	}
	if tree == nil || tree.RootNode() == nil {
		t.Fatal("parseWithSnippetParser returned nil tree/root")
	}
	tree.Release()
}

func TestParserParseClearsRecoveryParserAcrossTopLevelParses(t *testing.T) {
	parser := NewParser(buildArithmeticLanguage())
	parser.recoveryParser = NewParser(buildArithmeticLanguage())

	if _, err := parser.Parse([]byte("1+2")); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if parser.recoveryParser != nil {
		t.Fatal("Parse retained recoveryParser after top-level parse")
	}
}

func TestPreferRetryTreePrefersFurtherAcceptedProgress(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte:  100,
			flags:    nodeFlagHasError,
			children: []*Node{{}, {}, {}},
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopNoStacksAlive,
			ExpectedEOFByte: 200,
			Truncated:       true,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte:  200,
			flags:    nodeFlagHasError,
			children: []*Node{{}, {}},
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
		},
	}

	if !preferRetryTree(candidate, incumbent) {
		t.Fatal("preferRetryTree = false, want true for accepted full-length retry")
	}
}

func TestPreferRetryTreePrefersFewerChildrenOnEqualErrorTrees(t *testing.T) {
	incumbent := &Tree{
		root: &Node{
			endByte:  200,
			flags:    nodeFlagHasError,
			children: make([]*Node, 12),
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
			NodesAllocated:  1200,
		},
	}
	candidate := &Tree{
		root: &Node{
			endByte:  200,
			flags:    nodeFlagHasError,
			children: make([]*Node, 4),
		},
		parseRuntime: ParseRuntime{
			StopReason:      ParseStopAccepted,
			ExpectedEOFByte: 200,
			NodesAllocated:  800,
		},
	}

	if !preferRetryTree(candidate, incumbent) {
		t.Fatal("preferRetryTree = false, want true for smaller equal-span error tree")
	}
}

func TestGLRStackCullTrigger(t *testing.T) {
	if got := glrStackCullTrigger(8, arenaClassFull, "go"); got != 12 {
		t.Fatalf("glrStackCullTrigger(full, go) = %d, want 12", got)
	}
	if got := glrStackCullTrigger(8, arenaClassFull, "c_sharp"); got != 8 {
		t.Fatalf("glrStackCullTrigger(full, c_sharp) = %d, want 8", got)
	}
	if got := glrStackCullTrigger(8, arenaClassIncremental, "go"); got != 8 {
		t.Fatalf("glrStackCullTrigger(incremental, go) = %d, want 8", got)
	}
	maxInt := int(^uint(0) >> 1)
	if got := glrStackCullTrigger(maxInt, arenaClassFull, "go"); got != maxInt {
		t.Fatalf("glrStackCullTrigger(maxInt) = %d, want %d", got, maxInt)
	}
}

func TestResolveParseMaxStacks(t *testing.T) {
	if got, retry := resolveParseMaxStacks(6, 0, 2); got != 6 || retry {
		t.Fatalf("resolveParseMaxStacks(default) = (%d, %t), want (6, false)", got, retry)
	}
	if got, retry := resolveParseMaxStacks(6, 2, 2); got != 2 || retry {
		t.Fatalf("resolveParseMaxStacks(low override) = (%d, %t), want (2, false)", got, retry)
	}
	if got, retry := resolveParseMaxStacks(6, 32, 2); got != 32 || !retry {
		t.Fatalf("resolveParseMaxStacks(retry widen) = (%d, %t), want (32, true)", got, retry)
	}
	if got, retry := resolveParseMaxStacks(6, 2, 4); got != 4 || retry {
		t.Fatalf("resolveParseMaxStacks(conflict floor) = (%d, %t), want (4, false)", got, retry)
	}
}

func TestEffectiveFullParseInitialMaxStacks(t *testing.T) {
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "bash"}, maxGLRStacks); got != 256 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(bash) = %d, want 256", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "css"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(css) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "scss"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(scss) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "hcl"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(hcl) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "javascript"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(javascript) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "typescript"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(typescript) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "tsx"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(tsx) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "dart"}, maxGLRStacks); got != 6 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(dart) = %d, want 6", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "python"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(python) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "rust"}, maxGLRStacks); got != 2 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(rust) = %d, want 2", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "php"}, maxGLRStacks); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(php) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "go"}, maxGLRStacks); got != 32 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(go) = %d, want 32", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "markdown"}, maxGLRStacks); got != 5 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(markdown) = %d, want 5", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "markdown_inline"}, maxGLRStacks); got != 4 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(markdown_inline) = %d, want 4", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "css"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(css, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "javascript"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(javascript, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "typescript"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(typescript, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "tsx"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(tsx, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "dart"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(dart, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "rust"}, 16); got != 16 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(rust, explicit override) = %d, want 16", got)
	}
	if got := effectiveFullParseInitialMaxStacks(&Language{Name: "php"}, 32); got != 32 {
		t.Fatalf("effectiveFullParseInitialMaxStacks(php, explicit override) = %d, want 32", got)
	}
}

func TestParseMaxMergePerKeyValue(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "3")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := parseMaxMergePerKeyValue(); got != 3 {
		t.Fatalf("parseMaxMergePerKeyValue() = %d, want 3", got)
	}
}

func TestLanguageDefersExactDedupe(t *testing.T) {
	for _, name := range []string{"dart", "java", "typescript", "tsx", "rust"} {
		if !languageDefersExactDedupe(&Language{Name: name}, false) {
			t.Fatalf("languageDefersExactDedupe(%s, full tree) = false, want true", name)
		}
		if languageDefersExactDedupe(&Language{Name: name}, true) {
			t.Fatalf("languageDefersExactDedupe(%s, no-tree) = true, want false", name)
		}
	}
	for _, name := range []string{"go", "python", "javascript"} {
		if languageDefersExactDedupe(&Language{Name: name}, false) {
			t.Fatalf("languageDefersExactDedupe(%s, full tree) = true, want false", name)
		}
	}
	if languageDefersExactDedupe(nil, false) {
		t.Fatal("languageDefersExactDedupe(nil, full tree) = true, want false")
	}
}

func TestParsePreMaterializationDiagEnabled(t *testing.T) {
	t.Setenv("GOT_GLR_V2_PRE_MATERIALIZATION_DIAG", "1")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if !parsePreMaterializationDiagEnabled() {
		t.Fatal("parsePreMaterializationDiagEnabled() = false, want true")
	}
}

func TestEffectiveParseMergePerKeyCap(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, maxStacksPerMergeKey, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, default, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "go"}, maxStacksPerMergeKey, false); got != 3 {
		t.Fatalf("effectiveParseMergePerKeyCap(go, default, full) = %d, want 3", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "starlark"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(starlark, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, false, 128*1024); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, large default, full) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, false, 128*1024); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, large default, full) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, false, javaTightMergeCapSourceLen); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, large default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, false); got != 3 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, default, full) = %d, want 3", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(c, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "cpp"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(cpp, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(json, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "php"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(php, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "sql"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(sql, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "r"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(r, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scala"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(scala, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "powershell"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(powershell, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "graphql"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(graphql, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "haskell"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(haskell, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "make"}, maxStacksPerMergeKey, false); got != 2 {
		t.Fatalf("effectiveParseMergePerKeyCap(make, default, full) = %d, want 2", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "lua"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(lua, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ruby"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(ruby, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "rust"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(rust, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "svelte"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(svelte, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "xml"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(xml, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "toml"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(toml, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "nix"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(nix, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ocaml"}, maxStacksPerMergeKey, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(ocaml, default, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, 1, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(json, 1, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, 1, false); got != 1 {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, 1, full) = %d, want 1", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, 2, false); got != 2 {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, 2, full) = %d, want 2", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, 3, false); got != 3 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, 3, full) = %d, want 3", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "json"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(json, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "kotlin"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(kotlin, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "javascript"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(javascript, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "starlark"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(starlark, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "elixir"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(elixir, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "typescript"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(typescript, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(java, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(c, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "cpp"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(cpp, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "tsx"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(tsx, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, true, dartIncrementalReuseMaxSourceBytes); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, small incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, maxStacksPerMergeKey, true, dartIncrementalReuseMaxSourceBytes+1); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, large incremental fallback) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "php"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(php, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "sql"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(sql, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "r"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(r, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "scala"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(scala, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "powershell"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(powershell, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "graphql"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(graphql, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "haskell"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(haskell, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "make"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(make, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "lua"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(lua, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ruby"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(ruby, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "rust"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(rust, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "svelte"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(svelte, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "xml"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(xml, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "toml"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(toml, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "nix"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(nix, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ocaml"}, maxStacksPerMergeKey, true); got != maxStacksPerMergeKey {
		t.Fatalf("effectiveParseMergePerKeyCap(ocaml, default, incremental) = %d, want %d", got, maxStacksPerMergeKey)
	}
}

func TestEffectiveParseMergePerKeyCapJavaExplicitOverride(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "4")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "java"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(java, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "c"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(c, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "cpp"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(cpp, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "rust"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(rust, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "nix"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(nix, explicit, full) = %d, want 4", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "ocaml"}, 4, false); got != 4 {
		t.Fatalf("effectiveParseMergePerKeyCap(ocaml, explicit, full) = %d, want 4", got)
	}
}

func TestEffectiveParseMergePerKeyCapDartExplicitOverride(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "8")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, 8, false); got != 8 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, explicit, full) = %d, want 8", got)
	}
	if got := effectiveParseMergePerKeyCap(&Language{Name: "dart"}, 8, true, dartIncrementalReuseMaxSourceBytes+1); got != 8 {
		t.Fatalf("effectiveParseMergePerKeyCap(dart, explicit, large incremental fallback) = %d, want 8", got)
	}
}

func TestJavaAnnotationInterfaceSourceUsesWideMergeCap(t *testing.T) {
	t.Setenv("GOT_GLR_MAX_MERGE_PER_KEY", "")
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()

	if !javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("@interface Demo {}"), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth = false, want true")
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte(`class Demo { @Generated("x") void f() {} }`), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth(@Generated) = true, want false")
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("class Demo {}"), nil) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth(class) = true, want false")
	}
	if javaFullParseNeedsAnnotationDeclarationMergeWidth(&Language{Name: "java"}, []byte("@interface Demo {}"), &reuseCursor{}) {
		t.Fatal("javaFullParseNeedsAnnotationDeclarationMergeWidth(incremental) = true, want false")
	}
}

func TestNoteRepeatedReduceChainSignatureDetectsCycle(t *testing.T) {
	sig := reduceChainSignature{
		state:        2016,
		depth:        171,
		symbol:       216,
		childCount:   1,
		productionID: 42,
	}
	var prev reduceChainSignature
	count := 0
	cycle := false
	for i := 0; i <= maxRepeatedReduceChainSignature; i++ {
		prev, count, cycle = noteRepeatedReduceChainSignature(prev, count, sig)
	}
	if !cycle {
		t.Fatal("noteRepeatedReduceChainSignature did not report a repeated cycle")
	}
	if prev != sig {
		t.Fatalf("noteRepeatedReduceChainSignature signature = %+v, want %+v", prev, sig)
	}
	if count != maxRepeatedReduceChainSignature+1 {
		t.Fatalf("noteRepeatedReduceChainSignature count = %d, want %d", count, maxRepeatedReduceChainSignature+1)
	}
}

func TestNoteRepeatedReduceChainSignatureResetsOnChange(t *testing.T) {
	first := reduceChainSignature{state: 10, depth: 3, symbol: 7, childCount: 1, productionID: 2}
	second := reduceChainSignature{state: 11, depth: 3, symbol: 7, childCount: 1, productionID: 2}

	prev, count, cycle := noteRepeatedReduceChainSignature(reduceChainSignature{}, 0, first)
	if cycle || count != 1 || prev != first {
		t.Fatalf("first signature = (%+v, %d, %t), want (%+v, 1, false)", prev, count, cycle, first)
	}

	prev, count, cycle = noteRepeatedReduceChainSignature(prev, count, second)
	if cycle {
		t.Fatal("changed signature incorrectly reported a cycle")
	}
	if count != 1 || prev != second {
		t.Fatalf("changed signature = (%+v, %d), want (%+v, 1)", prev, count, second)
	}
}

func TestShouldNormalizeIncrementalReturnedTree(t *testing.T) {
	root := &Node{symbol: 1}
	oldTree := &Tree{root: root}
	reusedTree := &Tree{root: root}
	newRootTree := &Tree{root: &Node{symbol: 1}}

	if shouldNormalizeIncrementalReturnedTree(nil, oldTree) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(nil, oldTree) = true, want false")
	}
	if shouldNormalizeIncrementalReturnedTree(reusedTree, oldTree) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(reusedTree, oldTree) = true, want false")
	}
	if !shouldNormalizeIncrementalReturnedTree(newRootTree, oldTree) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(newRootTree, oldTree) = false, want true")
	}
	if !shouldNormalizeIncrementalReturnedTree(reusedTree, nil) {
		t.Fatal("shouldNormalizeIncrementalReturnedTree(reusedTree, nil) = false, want true")
	}
}

func TestLanguageSupportsIncrementalReuse(t *testing.T) {
	if languageSupportsIncrementalReuse(nil) {
		t.Fatal("languageSupportsIncrementalReuse(nil) = true, want false")
	}
	if !languageSupportsIncrementalReuse(&Language{}) {
		t.Fatal("languageSupportsIncrementalReuse(no scanner) = false, want true")
	}
	if languageSupportsIncrementalReuse(&Language{ExternalScanner: parserTestUnsafeExternalScanner{}}) {
		t.Fatal("languageSupportsIncrementalReuse(unsafe scanner) = true, want false")
	}
	if !languageSupportsIncrementalReuse(&Language{ExternalScanner: parserTestSafeExternalScanner{}}) {
		t.Fatal("languageSupportsIncrementalReuse(safe scanner) = false, want true")
	}
}

func TestIncrementalReuseUnavailableReason(t *testing.T) {
	if got := incrementalReuseUnavailableReason(nil); got != "token_source_nil" {
		t.Fatalf("incrementalReuseUnavailableReason(nil) = %q, want %q", got, "token_source_nil")
	}
	unsafeTS := &dfaTokenSource{language: &Language{ExternalScanner: parserTestUnsafeExternalScanner{}}}
	if got := incrementalReuseUnavailableReason(unsafeTS); got != "external_scanner_unsupported" {
		t.Fatalf("incrementalReuseUnavailableReason(unsafe external scanner) = %q, want %q", got, "external_scanner_unsupported")
	}
	safeTS := &dfaTokenSource{language: &Language{ExternalScanner: parserTestSafeExternalScanner{}}}
	if got := incrementalReuseUnavailableReason(safeTS); got != "" {
		t.Fatalf("incrementalReuseUnavailableReason(safe external scanner) = %q, want empty", got)
	}

	dartSmallTS := &dfaTokenSource{
		language: &Language{Name: "dart", ExternalScanner: parserTestSafeExternalScanner{}},
		lexer:    &Lexer{source: make([]byte, dartIncrementalReuseMaxSourceBytes)},
	}
	if !tokenSourceSupportsIncrementalReuse(dartSmallTS) {
		t.Fatal("tokenSourceSupportsIncrementalReuse(small dart safe scanner) = false, want true")
	}
	if got := incrementalReuseUnavailableReason(dartSmallTS); got != "" {
		t.Fatalf("incrementalReuseUnavailableReason(small dart safe scanner) = %q, want empty", got)
	}

	dartLargeTS := &dfaTokenSource{
		language: &Language{Name: "dart", ExternalScanner: parserTestSafeExternalScanner{}},
		lexer:    &Lexer{source: make([]byte, dartIncrementalReuseMaxSourceBytes+1)},
	}
	if tokenSourceSupportsIncrementalReuse(dartLargeTS) {
		t.Fatal("tokenSourceSupportsIncrementalReuse(large dart safe scanner) = true, want false")
	}
	if got := incrementalReuseUnavailableReason(dartLargeTS); got != "dart_large_external_scanner_unsupported" {
		t.Fatalf("incrementalReuseUnavailableReason(large dart safe scanner) = %q, want %q", got, "dart_large_external_scanner_unsupported")
	}
}

func TestParseFullArenaNodeCapacityCapsStaleLargeHintBySourceSize(t *testing.T) {
	sourceLen := 32 * 1024
	staleLargeHint := parseNodeLimit(2 * 1024 * 1024)

	got := parseFullArenaNodeCapacity(sourceLen, staleLargeHint)
	limit := parseFullArenaHintLimit(sourceLen)
	if got != limit {
		t.Fatalf("parseFullArenaNodeCapacity(%d, stale large hint) = %d, want source-sized limit %d", sourceLen, got, limit)
	}
	if got >= staleLargeHint {
		t.Fatalf("parseFullArenaNodeCapacity kept stale large hint: got %d, stale hint %d", got, staleLargeHint)
	}
}

func TestParseFullArenaNodeCapacityKeepsUsefulSameSizeHint(t *testing.T) {
	sourceLen := 128 * 1024
	initial := parseFullArenaInitialNodeCapacity(sourceLen)
	limit := parseFullArenaHintLimit(sourceLen)
	if initial >= limit {
		t.Fatalf("test setup invalid: initial=%d limit=%d", initial, limit)
	}
	hint := initial + (limit-initial)/2

	got := parseFullArenaNodeCapacity(sourceLen, hint)
	if got != hint {
		t.Fatalf("parseFullArenaNodeCapacity(%d, useful hint %d) = %d, want hint", sourceLen, hint, got)
	}
}

func TestParseFullArenaInitialNodeCapacityScalesForLargeSources(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parseFullArenaInitialNodeCapacity(sourceLen)
	want := 1_500_000
	if got != want {
		t.Fatalf("parseFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseFullArenaInitialNodeCapacityPreallocatesMediumSources(t *testing.T) {
	sourceLen := 192 * 1024
	got := parseFullArenaInitialNodeCapacity(sourceLen)
	want := sourceLen * 2 / 3
	if got != want {
		t.Fatalf("parseFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParsePendingFullArenaInitialNodeCapacityUsesLowerLargeSourceFloor(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	want := sourceLen / 2
	if got != want {
		t.Fatalf("parsePendingFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParsePendingFullArenaInitialNodeCapacityCapsHugeSourceFloor(t *testing.T) {
	sourceLen := 3 * 1024 * 1024
	got := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	want := 1_050_000
	if got != want {
		t.Fatalf("parsePendingFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseCompactFullArenaInitialNodeCapacityUsesCompactLargeSourceFloor(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	want := sourceLen / 4
	if got != want {
		t.Fatalf("parseCompactFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseCompactFullArenaInitialNodeCapacityCapsHugeSourceFloor(t *testing.T) {
	sourceLen := 4 * 1024 * 1024
	got := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	want := 750_000
	if got != want {
		t.Fatalf("parseCompactFullArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParseFinalChildRefArenaInitialNodeCapacityUsesSmallerFloor(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	got := parseFinalChildRefArenaInitialNodeCapacity(sourceLen)
	want := 64 * 1024
	if got != want {
		t.Fatalf("parseFinalChildRefArenaInitialNodeCapacity(%d) = %d, want %d", sourceLen, got, want)
	}
}

func TestParsePendingFullArenaNodeCapacityUsesCloseWarmHint(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	initial := parsePendingFullArenaInitialNodeCapacity(sourceLen)
	hint := initial - initial/16
	got := parsePendingFullArenaNodeCapacity(sourceLen, hint)
	if got != hint {
		t.Fatalf("parsePendingFullArenaNodeCapacity(%d, %d) = %d, want hint", sourceLen, hint, got)
	}
}

func TestParseCompactFullArenaNodeCapacityUsesWarmHintBelowInitial(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	initial := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	hint := initial * 3 / 4
	got := parseCompactFullArenaNodeCapacity(sourceLen, hint)
	if got != hint {
		t.Fatalf("parseCompactFullArenaNodeCapacity(%d, %d) = %d, want hint", sourceLen, hint, got)
	}
}

func TestParseCompactFullArenaNodeCapacityRejectsTinyStaleHint(t *testing.T) {
	sourceLen := 2 * 1024 * 1024
	initial := parseCompactFullArenaInitialNodeCapacity(sourceLen)
	hint := initial/2 - 1
	got := parseCompactFullArenaNodeCapacity(sourceLen, hint)
	if got != initial {
		t.Fatalf("parseCompactFullArenaNodeCapacity(%d, %d) = %d, want initial %d", sourceLen, hint, got, initial)
	}
}

func TestParsePendingFullArenaHintHeadroomIsTighterForLargeSources(t *testing.T) {
	used := 1_200_000
	got := parsePendingFullArenaHintHeadroom(used)
	want := 32 * 1024
	if got != want {
		t.Fatalf("parsePendingFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseCompactFullArenaHintHeadroomIsTighterForLargeSources(t *testing.T) {
	used := 1_200_000
	got := parseCompactFullArenaHintHeadroom(used)
	want := 32 * 1024
	if got != want {
		t.Fatalf("parseCompactFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseShouldUsePendingFullParentsDefaultsForLargePythonNoCompat(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "python"},
		noResultCompatibilityBenchmarkOnly: true,
	}

	if !parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = false, want true for large Python no-compat")
	}

	t.Setenv("GOT_GLR_V2_PENDING_PARENTS", "0")
	if parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = true, want explicit env disable")
	}
}

func TestParseShouldUsePendingFullParentsKeepsEnvOptInForOtherLargeSources(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language: &Language{Name: "java"},
	}

	if parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = true, want false without env for Java")
	}

	t.Setenv("GOT_GLR_V2_PENDING_PARENTS", "1")
	if !parseShouldUsePendingFullParents(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUsePendingFullParents = false, want env opt-in")
	}
}

func TestParseShouldUseCompactFullShiftLeavesDefaultsForLargePythonNoCompat(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "python"},
		noResultCompatibilityBenchmarkOnly: true,
	}

	if !parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = false, want true for large Python no-compat")
	}

	t.Setenv("GOT_GLR_V2_COMPACT_FULL_LEAVES", "0")
	if parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = true, want explicit env disable")
	}
}

func TestParseShouldUseCompactFullShiftLeavesKeepsEnvOptInForOtherLargeSources(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "java"},
		noResultCompatibilityBenchmarkOnly: true,
	}

	if parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = true, want false without env for Java")
	}

	t.Setenv("GOT_GLR_V2_COMPACT_FULL_LEAVES", "1")
	if !parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = false, want env opt-in")
	}

	parser.noResultCompatibilityBenchmarkOnly = false
	if parseShouldUseCompactFullShiftLeaves(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseCompactFullShiftLeaves = true, want no-compat-only gate")
	}
}

func TestParseShouldUseFinalChildRefsDefaultsForLargePythonNoCompat(t *testing.T) {
	source := make([]byte, 256*1024)
	parser := &Parser{
		language:                           &Language{Name: "python"},
		pendingFullParents:                 true,
		noResultCompatibilityBenchmarkOnly: true,
	}
	if !parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = false, want default large Python no-compat pending full parse")
	}

	parser.pendingFullParents = false
	if parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = true, want pending-parent gate")
	}

	parser.pendingFullParents = true
	parser.noResultCompatibilityBenchmarkOnly = false
	if parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = true, want no-compat-only gate")
	}

	parser.noResultCompatibilityBenchmarkOnly = true
	t.Setenv("GOT_GLR_V2_FINAL_CHILD_REFS", "0")
	if parseShouldUseFinalChildRefs(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldUseFinalChildRefs = true, want explicit env disable")
	}
}

func TestParserShouldDeferResultParentLinksForNoCompatBenchmark(t *testing.T) {
	arena := acquireNodeArena(arenaClassFull)
	defer arena.Release()

	root := newLeafNodeInArena(arena, Symbol(1), true, 0, 1, Point{}, Point{Column: 1})
	parser := &Parser{
		language:                           &Language{Name: "go"},
		noResultCompatibilityBenchmarkOnly: true,
	}
	if !parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = false, want true for no-compat benchmark parse")
	}

	parser.noTreeBenchmarkOnly = true
	if parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = true, want false for no-tree benchmark parse")
	}

	parser.noTreeBenchmarkOnly = false
	parser.noResultCompatibilityBenchmarkOnly = false
	if parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = true, want false for normal Go parse")
	}

	parser.language = &Language{Name: "java"}
	if !parser.shouldDeferResultParentLinks(root) {
		t.Fatal("shouldDeferResultParentLinks = false, want true for normal Java parse")
	}
}

func TestParseFullEntryScratchCapacityCapsLargePrealloc(t *testing.T) {
	got := parseFullEntryScratchCapacity(2 * 1024 * 1024)
	want := 64 * 1024
	if got != want {
		t.Fatalf("parseFullEntryScratchCapacity large source = %d, want %d", got, want)
	}
}

func TestParseFullArenaHintHeadroomIsBoundedForLargeSources(t *testing.T) {
	used := 1_500_000
	got := parseFullArenaHintHeadroom(used)
	want := 64 * 1024
	if got != want {
		t.Fatalf("parseFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseFullArenaHintHeadroomIsTighterForMediumSources(t *testing.T) {
	used := 128 * 1024
	got := parseFullArenaHintHeadroom(used)
	want := used / 16
	if got != want {
		t.Fatalf("parseFullArenaHintHeadroom(%d) = %d, want %d", used, got, want)
	}
}

func TestParseFullExternalScannerCheckpointCapacityUsesNodeHeadroom(t *testing.T) {
	const nodeCapacity = 1_500_000
	const sourceLen = 2 * 1024 * 1024
	got := parseFullExternalScannerCheckpointCapacity(sourceLen, nodeCapacity)
	want := sourceLen * 3 / 8
	if got != want {
		t.Fatalf("parseFullExternalScannerCheckpointCapacity = %d, want %d", got, want)
	}
	if got := parseFullExternalScannerCheckpointCapacity(8*1024*1024, nodeCapacity); got != nodeCapacity {
		t.Fatalf("capped checkpoint capacity = %d, want node capacity %d", got, nodeCapacity)
	}
	if got := parseFullExternalScannerCheckpointCapacity(256*1024-1, nodeCapacity); got != 0 {
		t.Fatalf("small-source checkpoint capacity = %d, want 0", got)
	}
}

func TestParseShouldSkipInvisibleFullLeafCheckpointsIsNarrow(t *testing.T) {
	parser := &Parser{
		language:                           &Language{Name: "python"},
		noResultCompatibilityBenchmarkOnly: true,
	}
	largeSource := make([]byte, 256*1024)
	if !parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = false, want true for large Python no-compat full parse")
	}
	parser.noResultCompatibilityBenchmarkOnly = false
	if parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = true for normal parse")
	}
	parser.noResultCompatibilityBenchmarkOnly = true
	if parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource[:len(largeSource)-1], nil, nil, arenaClassFull) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = true for sub-threshold source")
	}
	if parseShouldSkipInvisibleFullLeafCheckpoints(parser, largeSource, nil, nil, arenaClassIncremental) {
		t.Fatal("parseShouldSkipInvisibleFullLeafCheckpoints = true for incremental arena")
	}
}

func TestParseShouldCaptureFullMaterializationTimingIsNarrow(t *testing.T) {
	parser := &Parser{language: &Language{Name: "python"}}
	largeSource := make([]byte, 256*1024)
	if !parseShouldCaptureFullMaterializationTiming(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = false, want true for large Python full parse")
	}
	if parseShouldCaptureFullMaterializationTiming(parser, largeSource[:len(largeSource)-1], nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = true for sub-threshold source")
	}
	if parseShouldCaptureFullMaterializationTiming(parser, largeSource, nil, nil, arenaClassIncremental) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = true for incremental arena")
	}
	parser.language.Name = "go"
	if parseShouldCaptureFullMaterializationTiming(parser, largeSource, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureFullMaterializationTiming = true for non-Python language")
	}
}

func TestParseShouldCaptureMaterializationTimingEnv(t *testing.T) {
	ResetParseEnvConfigCacheForTests()
	defer ResetParseEnvConfigCacheForTests()
	t.Setenv("GOT_PARSE_PHASE_TIMING", "1")
	parser := &Parser{language: &Language{Name: "go"}}
	source := []byte("package p\n")
	if !parseShouldCaptureMaterializationTiming(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureMaterializationTiming = false, want env-enabled full timing")
	}
	if !parseShouldCaptureMaterializationTiming(parser, source, &reuseCursor{}, nil, arenaClassIncremental) {
		t.Fatal("parseShouldCaptureMaterializationTiming = false, want env-enabled incremental timing")
	}
	parser.noTreeBenchmarkOnly = true
	if parseShouldCaptureMaterializationTiming(parser, source, nil, nil, arenaClassFull) {
		t.Fatal("parseShouldCaptureMaterializationTiming = true for no-tree benchmark mode")
	}
}
