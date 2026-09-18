package main

import (
	"testing"

	gts "github.com/odvcencio/gotreesitter"
)

func TestImportedEOFAcceptSurvivesBlob(t *testing.T) {
	const source = `static bool ts_lex(TSLexer *lexer, TSStateId state) {
		switch (state) {
		case 0:
			ACCEPT_TOKEN(1);
			if (eof) ADVANCE(1);
			if (lookahead == 'a') ADVANCE(2);
			END_STATE();
		case 1: ACCEPT_TOKEN(ts_builtin_sym_end); END_STATE();
		case 2: ACCEPT_TOKEN(1); END_STATE();
		case 3: END_STATE();
		}
	}`
	states, err := extractLexFunctionStates(source, "ts_lex", map[string]int{"ts_builtin_sym_end": 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := make([]LexStateEntry, len(states))
	for i, state := range states {
		entries[i] = LexStateEntry{Accept: state.Accept, HasAccept: state.HasAccept, EOF: state.EOF}
		for _, tr := range state.Transitions {
			entries[i].Transitions = append(entries[i].Transitions, LexTransitionEntry{Lo: tr.Lo, Hi: tr.Hi, Next: tr.Next, Skip: tr.Skip})
		}
	}
	lang := &gts.Language{LexStates: convertLexStates(entries)}
	blob, err := EncodeLanguageBlob(lang)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := gts.LoadLanguage(blob)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []*gts.Language{lang, loaded} {
		if !candidate.LexStates[1].AcceptEOF || candidate.LexStates[3].AcceptEOF {
			t.Fatal("end acceptance was lost or assigned to a nonaccepting state")
		}
		lex := gts.NewLexer(candidate.LexStates, []byte("a"))
		if token := lex.Next(0); token.Symbol != 1 || token.EndByte != 1 {
			t.Fatalf("text token=%+v", token)
		}
		if token := lex.Next(0); token.Symbol != 0 || token.StartByte != 1 || token.EndByte != 1 {
			t.Fatalf("end token=%+v", token)
		}
	}
}

func TestImportedZeroLookaheadEOF(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		eof        int
	}{
		{"map", "ADVANCE_MAP(0, 1); END_STATE();", 1},
		{"comparison", "if (lookahead == 0) ADVANCE(1); END_STATE();", 1},
		{"guard", "if (eof) ADVANCE(2); ADVANCE_MAP(0, 1); END_STATE();", 2},
		{"non_eof", "if (!eof && lookahead == 0) ADVANCE(1); END_STATE();", -1},
		{"stop", "if (eof) END_STATE(); ADVANCE_MAP(0, 1); END_STATE();", -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row, err := parseLexCaseBlock(tc.body, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if row.EOF != tc.eof {
				t.Fatalf("EOF=%d want %d", row.EOF, tc.eof)
			}
			states := []gts.LexState{{EOF: row.EOF, Default: -1}, {AcceptToken: 1, EOF: -1, Default: -1}, {AcceptToken: 2, EOF: -1, Default: -1}}
			for _, tr := range row.Transitions {
				states[0].Transitions = append(states[0].Transitions, gts.LexTransition{Lo: tr.Lo, Hi: tr.Hi, NextState: tr.Next, Skip: tr.Skip})
			}

			literal := gts.NewLexer(states, []byte{0}).Next(0)
			if literal.Symbol != 1 || literal.StartByte != 0 || literal.EndByte != 1 {
				t.Fatalf("literal NUL token=%+v", literal)
			}
		})
	}
}

func TestImportedLookaheadConditionsAtEOF(t *testing.T) {
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{"lookahead == 0", true}, {"lookahead != 0", false}, {"lookahead < 1", true}, {"0 == lookahead", true}, {"!eof && lookahead == 0", false}, {"eof || lookahead == 'x'", true},
	} {
		got, err := parseConditionExpression(tc.expr, nil)
		if err != nil || got.eof != tc.want {
			t.Fatalf("%s: eof=%v err=%v", tc.expr, got.eof, err)
		}
	}
}

func TestImportedEOFRejectsUnsupportedActions(t *testing.T) {
	for _, body := range []string{
		"SKIP(1);", "if (eof) SKIP(1);", "if (lookahead == 0) SKIP(1);",
		"if (lookahead == 'a') END_STATE(); ADVANCE(1);",
	} {
		if _, err := parseLexCaseBlock(body, nil, nil); err == nil {
			t.Errorf("accepted unsupported action: %s", body)
		}
	}
	for _, body := range []string{
		"if (eof) ADVANCE(1); SKIP(2);",
		"if (eof) END_STATE(); if (lookahead == 0) SKIP(2);",
		"if (!eof && lookahead == 0) SKIP(2);",
	} {
		if _, err := parseLexCaseBlock(body, nil, nil); err != nil {
			t.Errorf("rejected guarded action: %s: %v", body, err)
		}
	}
}

func TestImportedEOFChainValidation(t *testing.T) {
	for _, tc := range []struct {
		name, cases string
		reject      bool
	}{
		{"end", "case 0: if (lookahead == 0) ADVANCE(1); END_STATE(); case 1: ACCEPT_TOKEN(ts_builtin_sym_end); END_STATE();", false},
		{"sparse", "case 0: if (eof) ADVANCE(2); END_STATE(); case 2: ACCEPT_TOKEN(1); END_STATE();", false},
		{"token", "case 0: if (lookahead == 0) ADVANCE(1); END_STATE(); case 1: ACCEPT_TOKEN(1); END_STATE();", false},
		{"consumed_prefix", "case 0: if (lookahead == 'a') ADVANCE(1); END_STATE(); case 1: if (lookahead == 0) ADVANCE(2); END_STATE(); case 2: END_STATE();", true},
		{"self_cycle", "case 0: if (lookahead == 0) ADVANCE(0); END_STATE();", true},
		{"cycle", "case 0: if (lookahead == 0) ADVANCE(1); END_STATE(); case 1: if (eof) ADVANCE(0); END_STATE();", true},
		{"missing", "case 0: if (eof) ADVANCE(1); END_STATE(); case 2: ACCEPT_TOKEN(1); END_STATE();", true},
		{"out_of_bounds", "case 0: if (eof) ADVANCE(42); END_STATE();", true},
		{"nonaccepting_suffix", "case 0: ACCEPT_TOKEN(1); if (eof) ADVANCE(1); END_STATE(); case 1: if (eof) ADVANCE(2); END_STATE(); case 2: END_STATE();", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, function := range []string{"ts_lex", "ts_lex_keywords"} {
				source := "static bool " + function + "(TSLexer *lexer, TSStateId state) { switch (state) {" + tc.cases + "} }"
				_, err := extractLexFunctionStates(source, function, map[string]int{"ts_builtin_sym_end": 0}, nil)
				if (err != nil) != tc.reject {
					t.Fatalf("%s: err=%v want rejection=%v", function, err, tc.reject)
				}
			}
		})
	}
}
