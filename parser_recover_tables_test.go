package gotreesitter

import "testing"

func TestBuildStateRecoverTableNilWhenNoRecoverActions(t *testing.T) {
	lang := buildArithmeticLanguage()
	table := buildStateRecoverTable(lang)
	if table != nil {
		t.Fatalf("expected nil recover table when grammar has no recover actions, got len=%d", len(table))
	}
}

func TestBuildStateRecoverTableMarksRecoverStates(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	table := buildStateRecoverTable(lang)
	if len(table) == 0 {
		t.Fatal("expected recover table to be populated")
	}
	if len(table) != int(lang.StateCount) {
		t.Fatalf("recover table len = %d, want %d", len(table), lang.StateCount)
	}
	if table[0] {
		t.Fatal("state 0 should not be marked recoverable")
	}
	if !table[2] {
		t.Fatal("state 2 should be marked recoverable")
	}
}

func TestBuildKeywordStatesDense(t *testing.T) {
	lang := buildKeywordStateLanguageDense()
	table := buildKeywordStates(lang)
	if len(table) != int(lang.StateCount) {
		t.Fatalf("keyword state table len = %d, want %d", len(table), lang.StateCount)
	}
	if !table[0] {
		t.Fatal("state 0 should allow keyword promotion")
	}
	if table[1] {
		t.Fatal("state 1 should not allow keyword promotion")
	}
	if !table[2] {
		t.Fatal("state 2 should allow keyword promotion")
	}
}

func TestBuildKeywordStatesSmall(t *testing.T) {
	lang := buildKeywordStateLanguageSmall()
	table := buildKeywordStates(lang)
	if len(table) != int(lang.StateCount) {
		t.Fatalf("keyword state table len = %d, want %d", len(table), lang.StateCount)
	}
	if table[0] {
		t.Fatal("state 0 should not allow keyword promotion")
	}
	if !table[1] {
		t.Fatal("state 1 should allow keyword promotion from small parse table")
	}
}

func TestBuildKeywordStatesNilWhenNoKeywordActions(t *testing.T) {
	lang := buildKeywordStateLanguageDense()
	lang.ParseTable[0][2] = 0
	lang.ParseTable[2][2] = 0
	table := buildKeywordStates(lang)
	if table != nil {
		t.Fatalf("expected nil keyword state table, got len=%d", len(table))
	}
}

func TestBuildRecoverActionsByStateMarksRecoverSymbols(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	_, _, symbols := buildRecoverActionsByState(lang)
	if len(symbols) == 0 {
		t.Fatal("expected recover symbol table to be populated")
	}
	if !symbols[3] { // STAR
		t.Fatal("expected STAR to be marked as recoverable lookahead")
	}
	if symbols[1] { // NUMBER
		t.Fatal("did not expect NUMBER to be marked as recoverable lookahead")
	}
}

func TestFindRecoverActionOnStackUsesNearestAncestor(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	parser := NewParser(lang)
	s := newGLRStack(lang.InitialState)
	s.push(2, nil, nil, nil)
	s.push(3, nil, nil, nil)

	depth, act, ok := parser.findRecoverActionOnStack(&s, Symbol(3), nil) // STAR
	if !ok {
		t.Fatal("expected recover action on stack for STAR")
	}
	if depth != 1 {
		t.Fatalf("recover depth = %d, want 1 (state 2)", depth)
	}
	if act.Type != ParseActionRecover {
		t.Fatalf("recover action type = %d, want %d", act.Type, ParseActionRecover)
	}
	if act.State != 3 {
		t.Fatalf("recover state = %d, want 3", act.State)
	}
}

func TestRecoverActionForStateUsesSymbolSpecificTable(t *testing.T) {
	lang := buildArithmeticRecoverLanguage()
	parser := NewParser(lang)

	if _, ok := parser.recoverActionForState(2, Symbol(3)); !ok {
		t.Fatal("expected recover action for state 2 on STAR")
	}
	if _, ok := parser.recoverActionForState(2, Symbol(1)); ok {
		t.Fatal("did not expect recover action for state 2 on NUMBER")
	}
}
