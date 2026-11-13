package lexbuild

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

var update = flag.Bool("update", false, "rewrite testdata/small/states.json and record.golden from the reference (needs "+refEnv+")")

// goldenStates is testdata/small/states.json: the parse states of the small
// grammar as the reference parse table has them, by token name.
type goldenStates struct {
	States []goldenState `json:"states"`
	// Coincident holds the valid tokens of each parse state of the table written
	// without merging parse states, from which the generator builds its coincident
	// token index.
	Coincident [][]string `json:"coincident,omitempty"`
}

type goldenState struct {
	Lex         int      `json:"lex"`
	Tokens      []string `json:"tokens"`
	Recover     []string `json:"recover,omitempty"`
	NotReusable []string `json:"notReusable,omitempty"`
}

const endName = "<end>"

func tokenName(p *Prepared, s Symbol) string {
	switch s.Kind {
	case End:
		return endName
	case Terminal:
		return p.Lex.Variables[s.Index].Ident
	case External:
		return fmt.Sprintf("<external %d>", s.Index)
	}
	return "<other>"
}

func tokenByName(p *Prepared, name string) (Symbol, error) {
	if name == endName {
		return endSym(), nil
	}
	var found []Symbol
	for i, v := range p.Lex.Variables {
		if v.Ident == name {
			found = append(found, terminalSym(i))
		}
	}
	if len(found) != 1 {
		return Symbol{}, fmt.Errorf("token name %q matches %d tokens", name, len(found))
	}
	return found[0], nil
}

// formatRecord writes the lex tables and the facts about parse states as text.
func formatRecord(lt *LexTables, rec lexexact.Func, gs goldenStates) string {
	var b strings.Builder
	fmt.Fprintf(&b, "lex states %d\n", len(rec.Accept))
	for i := range rec.Accept {
		fmt.Fprintf(&b, "state %d accept=%d eof=%d invalid=%d\n", i, rec.Accept[i], rec.OnEOF[i], rec.OnInvalid[i])
		for _, r := range rec.Trans[i] {
			skip := ""
			if r.Skip {
				skip = " skip"
			}
			fmt.Fprintf(&b, "  %X-%X -> %d%s\n", r.Lo, r.Hi, r.Target, skip)
		}
	}
	for i, st := range gs.States {
		fmt.Fprintf(&b, "parse %d lex=%d notReusable=%d\n", i, lt.LexState[i], len(st.NotReusable))
	}
	return b.String()
}

// runSmall builds the lexer of the small grammar from grammar.json and the
// parse-state facts and returns the built tables and the record.
func runSmall(t *testing.T, dir string, gs goldenStates) (*LexTables, lexexact.Func, *Analysis, []TokenSet) {
	t.Helper()
	gj, err := os.ReadFile(filepath.Join(dir, "grammar.json"))
	if err != nil {
		t.Fatal(err)
	}
	p, err := Prepare(gj)
	if err != nil {
		t.Fatal(err)
	}
	final := make([]TokenSet, len(gs.States))
	pre := make([]TokenSet, len(gs.States))
	for i, st := range gs.States {
		rec := map[string]bool{}
		for _, n := range st.Recover {
			rec[n] = true
		}
		for _, n := range st.Tokens {
			s, err := tokenByName(p, n)
			if err != nil {
				t.Fatal(err)
			}
			final[i].Insert(s)
			if !rec[n] {
				pre[i].Insert(s)
			}
		}
	}
	if gs.Coincident != nil {
		pre = make([]TokenSet, len(gs.Coincident))
		for i, names := range gs.Coincident {
			for _, n := range names {
				s, err := tokenByName(p, n)
				if err != nil {
					t.Fatal(err)
				}
				pre[i].Insert(s)
			}
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
		t.Fatal(err)
	}
	return lt, rec, a, final
}

// TestGoldenSmallGrammar needs no reference files. testdata/small holds a
// grammar.json (the grammar of tree-sitter-json at its pinned commit, MIT
// licence, see testdata/small/LICENSE), the parse states of the reference
// parse table by token name, and the record that this package builds. The
// golden record was checked equal to the lexer of the reference parser.c by
// TestLexbuildRecordEqualsC when it was written.
func TestGoldenSmallGrammar(t *testing.T) {
	dir := filepath.Join("testdata", "small")
	if *update {
		root := refDir(t)
		ref := loadRef(t, root, "json")
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
		for i := range states {
			final[i] = states[i].Tokens
		}
		var gs goldenStates
		for i, st := range states {
			g := goldenState{Lex: ref.lexModes[i]}
			for _, s := range st.Tokens.symbols() {
				g.Tokens = append(g.Tokens, tokenName(p, s))
			}
			for _, c := range ref.cells[i] {
				if c.sym >= ref.tokenCount {
					continue
				}
				n := tokenName(p, toSym[c.sym])
				if c.recover {
					g.Recover = append(g.Recover, n)
				}
				if !c.reusable {
					g.NotReusable = append(g.NotReusable, n)
				}
			}
			sort.Strings(g.Recover)
			sort.Strings(g.NotReusable)
			gs.States = append(gs.States, g)
		}
		if cs, ok := coincidentStates(t, root, "json", ref, toSym); ok {
			for _, set := range cs {
				var names []string
				for _, sy := range set.symbols() {
					names = append(names, tokenName(p, sy))
				}
				gs.Coincident = append(gs.Coincident, names)
			}
		}
		b, _ := json.MarshalIndent(gs, "", " ")
		if err := os.WriteFile(filepath.Join(dir, "grammar.json"), ref.jsonBytes, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "states.json"), append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		lt, rec, _, _ := runSmall(t, dir, gs)
		if err := os.WriteFile(filepath.Join(dir, "record.golden"), []byte(formatRecord(lt, rec, gs)), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(filepath.Join(dir, "states.json"))
	if err != nil {
		t.Fatal(err)
	}
	var gs goldenStates
	if err := json.Unmarshal(raw, &gs); err != nil {
		t.Fatal(err)
	}
	lt, rec, a, final := runSmall(t, dir, gs)
	want, err := os.ReadFile(filepath.Join(dir, "record.golden"))
	if err != nil {
		t.Fatal(err)
	}
	if got := formatRecord(lt, rec, gs); got != string(want) {
		t.Errorf("record differs from testdata/small/record.golden (first lines of a diff):\n%s", firstDiff(got, string(want)))
	}
	// lex starts and reusable flags against the facts taken from the reference
	for i, st := range gs.States {
		if st.Lex >= 0 && int(lt.LexState[i]) != st.Lex {
			t.Errorf("parse state %d: lex start %d, reference %d", i, lt.LexState[i], st.Lex)
		}
		frag := a.Fragile(&final[i])
		notReusable := map[string]bool{}
		for _, n := range st.NotReusable {
			notReusable[n] = true
		}
		rec := map[string]bool{}
		for _, n := range st.Recover {
			rec[n] = true
		}
		gp, err := os.ReadFile(filepath.Join(dir, "grammar.json"))
		if err != nil {
			t.Fatal(err)
		}
		p, _ := Prepare(gp)
		for _, n := range st.Tokens {
			s, _ := tokenByName(p, n)
			if s.Kind != Terminal && s.Kind != End {
				continue
			}
			got := !(rec[n] || (s.IsTerminal() && frag.Contains(s)))
			if got == notReusable[n] {
				t.Errorf("parse state %d token %q: reusable %v, reference %v", i, n, got, !notReusable[n])
			}
		}
	}
}

func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d:\n got  %q\n want %q", i+1, al[i], bl[i])
		}
	}
	return fmt.Sprintf("lengths differ: got %d lines, want %d lines", len(al), len(bl))
}
