package lexbuild

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// diffCount builds the lexer of grammar text gj from the reference parse
// states of the json reference and counts how it differs from the reference
// lexer: a changed state count, or states whose accept, end-of-input step or
// invalid-input step differ.
func diffCount(t *testing.T, ref *refGrammar, gj []byte, dropEnd bool) int {
	p, err := Prepare(gj)
	if err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	toSym, _, err := ref.symbolMap(p)
	if err != nil {
		return 1000
	}
	states := ref.parseStates(toSym)
	final := make([]TokenSet, len(states))
	pre := make([]TokenSet, len(states))
	for i := range states {
		final[i] = states[i].Tokens
		pre[i] = states[i].Tokens
	}
	if dropEnd {
		for i := range final {
			final[i].eof = false
		}
	}
	a, err := p.Analyze(pre)
	if err != nil {
		t.Fatal(err)
	}
	lt, err := p.BuildLexTables(a, final)
	if err != nil {
		t.Fatal(err)
	}
	ids := p.TokenSymbolIDs(final)
	rec, err := lt.Main.Record(func(s Symbol) (uint32, bool) { id, ok := ids[s]; return id, ok }, lt.LexState)
	if err != nil {
		return 1000
	}
	want := ref.tables.Main
	if len(rec.Accept) != len(want.Accept) {
		return 1000
	}
	n := 0
	for i := range rec.Accept {
		if rec.Accept[i] != want.Accept[i] || rec.OnEOF[i] != want.OnEOF[i] || rec.OnInvalid[i] != want.OnInvalid[i] {
			n++
		}
	}
	return n
}

// TestControlsDetectDifferences: the comparison is not blind. Two inputs that
// are altered on purpose must show differences against the reference.
func TestControlsDetectDifferences(t *testing.T) {
	root := refDir(t)
	ref := loadRef(t, root, "json")
	if n := diffCount(t, ref, ref.jsonBytes, false); n != 0 {
		t.Fatalf("unaltered input differs in %d states", n)
	}
	if n := diffCount(t, ref, ref.jsonBytes, true); n == 0 {
		t.Error("control 1 (end token removed from every parse state) shows no difference")
	}
	altered := bytes.Replace(ref.jsonBytes, []byte(`"[^\\\\\"\\n]+"`), []byte(`"[^\\\\\"]+"`), 1)
	if bytes.Equal(altered, ref.jsonBytes) {
		t.Fatal("control 2 did not alter the grammar text")
	}
	if n := diffCount(t, ref, altered, false); n == 0 {
		t.Error("control 2 (string content pattern altered) shows no difference")
	}
}

// buildLexers builds the main and keyword lexers of one reference grammar.
func buildLexers(t *testing.T, root, name string) (*refGrammar, lexexactRecs) {
	t.Helper()
	ref := loadRef(t, root, name)
	p, err := Prepare(ref.jsonBytes)
	if err != nil {
		t.Fatal(err)
	}
	toSym, _, err := ref.symbolMap(p)
	if err != nil {
		t.Fatal(err)
	}
	states := ref.parseStates(toSym)
	final := make([]TokenSet, len(states))
	pre := make([]TokenSet, len(states))
	for i := range states {
		final[i], pre[i] = states[i].Tokens, states[i].Tokens
	}
	coinc, ok := coincidentStates(t, root, name, ref, toSym)
	if !ok {
		coinc = pre
	}
	a, err := p.Analyze(coinc)
	if err != nil {
		t.Fatal(err)
	}
	ids := p.TokenSymbolIDs(final)
	symID := func(s Symbol) (uint32, bool) { id, ok := ids[s]; return id, ok }
	lt, err := p.BuildLexTables(a, ref.withReserved(final, toSym))
	if err != nil {
		t.Fatal(err)
	}
	var recs lexexactRecs
	if recs.main, err = lt.Main.Record(symID, lt.LexState); err != nil {
		t.Fatal(err)
	}
	if lt.Keyword != nil {
		kr, err := lt.Keyword.Record(symID, nil)
		if err != nil {
			t.Fatal(err)
		}
		recs.kw, recs.hasKW = kr, true
	}
	return ref, recs
}

type lexexactRecs struct {
	main  lexexact.Func
	kw    lexexact.Func
	hasKW bool
}

// TestControlsOnTransitions: the complete-transition check is not blind. Changing one target of
// one range of the main lexer, and of the keyword lexer, must be reported against the reference.
func TestControlsOnTransitions(t *testing.T) {
	root := refDir(t)
	checked := map[string]bool{}
	for _, name := range []string{"json", "awk"} {
		if _, err := os.Stat(filepath.Join(root, name, "parser.c")); err != nil {
			continue
		}
		ref, recs := buildLexers(t, root, name)
		if err := ref.src.Main.CheckTransitions(blobOf(recs.main)); err != nil {
			t.Errorf("%s: unaltered main lexer: %v", name, err)
		}
		blob := blobOf(recs.main)
		changed := false
		for i := range blob {
			if len(blob[i]) > 0 {
				blob[i][0].Next++
				changed = true
				break
			}
		}
		if changed && ref.src.Main.CheckTransitions(blob) == nil {
			t.Errorf("%s: a changed target of the main lexer went unnoticed", name)
		}
		checked["main"] = true
		if recs.hasKW && ref.src.Keyword != nil {
			if err := ref.src.Keyword.CheckTransitions(blobOf(recs.kw)); err != nil {
				t.Errorf("%s: unaltered keyword lexer: %v", name, err)
			}
			kb := blobOf(recs.kw)
			for i := range kb {
				if len(kb[i]) > 0 {
					kb[i][0].Lo++ // moves one edge of one keyword state
					break
				}
			}
			if ref.src.Keyword.CheckTransitions(kb) == nil {
				t.Errorf("%s: a moved edge of the keyword lexer went unnoticed", name)
			}
			checked["keyword"] = true
		}
	}
	if !checked["main"] {
		t.Skip("no reference grammar for the control")
	}
}
