package gotreesitter

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"os"
	"sort"
	"testing"
)

// TestDecodeConflictStates is a diagnostic (gated on GTS_DECODE=1) that decodes
// the GLR conflict action tables at specific (state, lookahead) pairs for a
// grammar blob. It is the "decode conflict actions" step of the fork-reduction
// playbook: load the shipped table, print the shift/reduce actions at a hot
// fork state, so we can compare against tree-sitter C's parser.c and decide
// whether the reduce is a zero-progress dead-end safe to collapse to the shift.
//
//	GTS_DECODE=1 go test . -run TestDecodeConflictStates -v
func TestDecodeConflictStates(t *testing.T) {
	if os.Getenv("GTS_DECODE") == "" {
		t.Skip("diagnostic; set GTS_DECODE=1 to run")
	}

	type target struct {
		blob   string
		states []StateID
	}
	targets := []target{
		{blob: "python", states: []StateID{72, 2309, 1334, 1367, 1725}},
		{blob: "rust", states: []StateID{83, 3095, 486, 246}},
	}

	for _, tg := range targets {
		lang := loadBlobForDecode(t, tg.blob)
		p := NewParser(lang)
		t.Logf("\n========== %s (states=%d, symbols=%d, largeStateCount=%d) ==========",
			tg.blob, lang.StateCount, len(lang.SymbolNames), lang.LargeStateCount)
		for _, st := range tg.states {
			dumpConflictState(t, p, lang, st)
		}
	}
}

func loadBlobForDecode(t *testing.T, name string) *Language {
	t.Helper()
	data, err := os.ReadFile(fmt.Sprintf("grammars/grammar_blobs/%s.bin", name))
	if err != nil {
		t.Skipf("blob %s not present: %v", name, err)
	}
	gzr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: gzip: %v", name, err)
	}
	defer gzr.Close()
	var lang Language
	if err := gob.NewDecoder(gzr).Decode(&lang); err != nil {
		t.Fatalf("%s: gob: %v", name, err)
	}
	return &lang
}

func symName(lang *Language, s Symbol) string {
	if int(s) < len(lang.SymbolNames) {
		return lang.SymbolNames[s]
	}
	return fmt.Sprintf("sym#%d", s)
}

func fmtAction(lang *Language, a ParseAction) string {
	switch a.Type {
	case ParseActionShift:
		flags := ""
		if a.Repetition {
			flags += " REPETITION"
		}
		if a.Extra {
			flags += " extra"
		}
		return fmt.Sprintf("SHIFT -> state %d%s", a.State, flags)
	case ParseActionReduce:
		return fmt.Sprintf("REDUCE %s (childCount=%d prod=%d dynPrec=%d)",
			symName(lang, a.Symbol), a.ChildCount, a.ProductionID, a.DynamicPrecedence)
	case ParseActionAccept:
		return "ACCEPT"
	default:
		return fmt.Sprintf("type=%d", a.Type)
	}
}

// dumpConflictState walks every symbol that has an action at `state` and prints
// the ones with >1 action (the genuine forks), tagging the lookahead name.
func dumpConflictState(t *testing.T, p *Parser, lang *Language, state StateID) {
	t.Logf("--- state %d ---", state)
	type row struct {
		sym     Symbol
		actions []ParseAction
	}
	var conflicts []row
	p.forEachActionIndexInState(state, func(sym Symbol, idx uint16) bool {
		if int(idx) >= len(lang.ParseActions) {
			return true
		}
		acts := lang.ParseActions[idx].Actions
		if len(acts) > 1 {
			conflicts = append(conflicts, row{sym: sym, actions: acts})
		}
		return true
	})
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].sym < conflicts[j].sym })
	if len(conflicts) == 0 {
		t.Logf("  (no multi-action conflicts at this state)")
		return
	}
	for _, c := range conflicts {
		t.Logf("  lookahead %q (sym %d): %d actions", symName(lang, c.sym), c.sym, len(c.actions))
		for _, a := range c.actions {
			t.Logf("      %s", fmtAction(lang, a))
		}
	}
}
