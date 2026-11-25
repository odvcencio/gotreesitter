package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// At the end of input, C's ts_lex reads lookahead 0 and eof true, and takes
// the first step whose condition holds. A lex state's EOF edge must be that
// step. Each state below holds a condition form a generator writes.
const lexEOFEdgeSource = `#include "tree_sitter/parser.h"
enum ts_symbol_identifiers {
  sym_word = 1,
};

static inline bool sym_word_character_set_1(int32_t c) {
  return (c < ' '
    ? c == 0
    : (c <= ' ' || c == '"'));
}

static bool ts_lex(TSLexer *lexer, TSStateId state) {
  START_LEXER();
  eof = lexer->eof(lexer);
  switch (state) {
    case 0:
      ACCEPT_TOKEN(sym_word);
      if (!sym_word_character_set_1(lookahead)) ADVANCE(0);
      END_STATE();
    case 1:
      if (lookahead == 0) ADVANCE(2);
      if (lookahead == 'a') ADVANCE(0);
      END_STATE();
    case 2:
      ACCEPT_TOKEN(ts_builtin_sym_end);
      END_STATE();
    case 3:
      if (eof) ADVANCE(2);
      if (lookahead != 0 &&
          lookahead != '"') ADVANCE(0);
      END_STATE();
    case 4:
      if ((!eof && lookahead == 00)) ADVANCE(0);
      END_STATE();
    case 5:
      if (lookahead < 'a') ADVANCE(0);
      END_STATE();
    case 6:
      ADVANCE(0);
      END_STATE();
    default:
      return false;
  }
}
`

// cEOFSteps returns, per main lex state, the target of the step C takes at the
// end of input, or -1. It reads parserC with internal/lexexact, which
// evaluates the C conditions with lookahead 0 and eof true.
func cEOFSteps(t *testing.T, parserC, parserH string) []int {
	t.Helper()
	src, err := lexexact.Parse(parserC, parserH)
	if err != nil {
		t.Fatalf("lexexact.Parse: %v", err)
	}
	rec, err := lexexact.BuildUnchecked(src, parserC)
	if err != nil {
		t.Fatalf("lexexact.BuildUnchecked: %v", err)
	}
	out := make([]int, len(rec.Main.OnEOF))
	for i, code := range rec.Main.OnEOF {
		out[i] = -1
		if step, ok := lexexact.UnpackStep(code); ok {
			out[i] = step.Target
		}
	}
	return out
}

func extractedEOFEdges(t *testing.T, parserC string) []int {
	t.Helper()
	dfa, err := ExtractLexDFA(parserC)
	if err != nil {
		t.Fatalf("ExtractLexDFA: %v", err)
	}
	out := make([]int, len(dfa.States))
	for i, st := range dfa.States {
		out[i] = st.EOF
	}
	return out
}

func TestLexEOFEdgeFollowsCAtEndOfInput(t *testing.T) {
	got := extractedEOFEdges(t, lexEOFEdgeSource)
	for _, tc := range []struct {
		state, want int
		form        string
	}{
		{0, -1, "!helper(lookahead), NUL in the helper's set (generators 0.16.8 to 0.22.2)"},
		{1, 2, "lookahead == 0 (end of input before generator 0.16)"},
		{2, -1, "no step"},
		{3, 2, "if (eof) (generator 0.16 on), then a class guarded by lookahead != 0"},
		{4, -1, "NUL in an included class, guarded by !eof"},
		{5, 0, "a comparison that holds for lookahead 0"},
		{6, 0, "unconditional ADVANCE"},
	} {
		if got[tc.state] != tc.want {
			t.Errorf("state %d (%s): EOF edge %d, want %d", tc.state, tc.form, got[tc.state], tc.want)
		}
	}
	want := cEOFSteps(t, lexEOFEdgeSource, lexexact.StandardHeader)
	if len(want) != len(got) {
		t.Fatalf("C has %d lex states, the extractor %d", len(want), len(got))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("state %d: EOF edge %d, C steps to %d at the end of input", i, got[i], want[i])
		}
	}
}

// TestLexEOFEdgeFishExcerpt checks the fish lexer states that gave the shipped
// fish blob an end-of-input edge to state 142 (word) where C takes no step:
// `set x 1` without a final newline lexed 1 as word instead of integer. The
// expected edges are C's: the exact-lexer record of the full pinned parser.c
// (grammars/runtime/lex_exact/fish.lexx) holds C's step at the end of input
// for every fish lex state.
func TestLexEOFEdgeFishExcerpt(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "fish_lex_eof_excerpt.c"))
	if err != nil {
		t.Fatal(err)
	}
	excerpt := string(data)
	got := extractedEOFEdges(t, excerpt)
	wantByState := map[int]int{30: -1, 31: 33, 33: -1, 54: -1, 61: -1, 63: -1, 141: -1, 142: -1}

	record, err := os.ReadFile(filepath.Join("..", "..", "grammars", "runtime", "lex_exact", "fish.lexx"))
	if err != nil {
		t.Fatal(err)
	}
	rec, err := lexexact.Decode(record)
	if err != nil {
		t.Fatal(err)
	}
	for state, want := range wantByState {
		fullC := -1
		if step, ok := lexexact.UnpackStep(rec.Main.OnEOF[state]); ok {
			fullC = step.Target
		}
		if fullC != want {
			t.Fatalf("state %d: expected edge %d disagrees with C's step %d in fish.lexx", state, want, fullC)
		}
		if got[state] != want {
			t.Errorf("state %d: EOF edge %d, C steps to %d at the end of input", state, got[state], want)
		}
	}
}

// TestLexEOFEdgesMatchCPinnedCorpus compares, for every pinned parser.c under
// $TS2GO_PARSER_REPOS, the extractor's end-of-input edge of every main lex
// state with the step that C takes at the end of input (internal/lexexact on
// the grammar's own parser.h).
func TestLexEOFEdgesMatchCPinnedCorpus(t *testing.T) {
	files, _ := pinnedParserSources(t)
	grammars, states, differ, refused := 0, 0, 0, 0
	for _, name := range sortedKeys(files) {
		parserC, err := os.ReadFile(files[name])
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		parserH, err := os.ReadFile(filepath.Join(filepath.Dir(files[name]), "tree_sitter", "parser.h"))
		if err != nil {
			t.Logf("%s: no parser.h: %v", name, err)
			refused++
			continue
		}
		src, err := lexexact.Parse(string(parserC), string(parserH))
		if err != nil {
			t.Logf("%s: lexexact refuses the source: %v", name, err)
			refused++
			continue
		}
		rec, err := lexexact.BuildUnchecked(src, string(parserC))
		if err != nil {
			t.Logf("%s: lexexact refuses the source: %v", name, err)
			refused++
			continue
		}
		got := extractedEOFEdges(t, string(parserC))
		grammars++
		n := 0
		for i, code := range rec.Main.OnEOF {
			want := -1
			if step, ok := lexexact.UnpackStep(code); ok {
				want = step.Target
			}
			if i < len(got) && got[i] != want {
				if n < 3 {
					t.Errorf("%s: state %d: EOF edge %d, C steps to %d at the end of input", name, i, got[i], want)
				}
				n++
			}
		}
		states += len(rec.Main.OnEOF)
		differ += n
	}
	t.Logf("CORPUS grammars=%d lex_states=%d eof_edges_differ=%d refused=%d", grammars, states, differ, refused)
}
