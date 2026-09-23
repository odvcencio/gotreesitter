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
