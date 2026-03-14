package gotreesitter

import "testing"

func TestApplyActionTerminalExtraShiftPreservesCurrentState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: 0,
		Extra: true,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got, want := s.top().state, lang.InitialState; got != want {
		t.Fatalf("top state = %d, want %d", got, want)
	}
	if got, want := s.top().node.parseState, lang.InitialState; got != want {
		t.Fatalf("extra leaf parseState = %d, want %d", got, want)
	}
	if got, want := s.top().node.preGotoState, lang.InitialState; got != want {
		t.Fatalf("extra leaf preGotoState = %d, want %d", got, want)
	}
}

func TestApplyActionNonterminalExtraShiftUsesActionState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     2,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
		Extra: true,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got := s.top().state; got != targetState {
		t.Fatalf("top state = %d, want %d", got, targetState)
	}
	if got := s.top().node.parseState; got != targetState {
		t.Fatalf("extra leaf parseState = %d, want %d", got, targetState)
	}
	if got, want := s.top().node.preGotoState, lang.InitialState; got != want {
		t.Fatalf("extra leaf preGotoState = %d, want %d", got, want)
	}
}

func TestApplyActionNonExtraShiftUsesActionState(t *testing.T) {
	lang := buildArithmeticLanguage()
	parser := NewParser(lang)

	s := newGLRStack(lang.InitialState)
	anyReduced := false
	nodeCount := 0
	targetState := lang.InitialState + 7
	tok := Token{
		Symbol:     1,
		StartByte:  0,
		EndByte:    1,
		StartPoint: Point{},
		EndPoint:   Point{Row: 0, Column: 1},
	}

	parser.applyAction(&s, ParseAction{
		Type:  ParseActionShift,
		State: targetState,
	}, tok, &anyReduced, &nodeCount, nil, nil, nil, nil, false, nil)

	if got := s.top().state; got != targetState {
		t.Fatalf("top state = %d, want %d", got, targetState)
	}
	if got := s.top().node.parseState; got != targetState {
		t.Fatalf("leaf parseState = %d, want %d", got, targetState)
	}
}
