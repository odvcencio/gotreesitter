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
	// Override via GTS_DECODE_TARGETS="c:43,31;cpp:52,50"
	if env := os.Getenv("GTS_DECODE_TARGETS"); env != "" {
		targets = nil
		for _, spec := range splitCSVSemi(env) {
			parts := splitOnColon(spec)
			if len(parts) != 2 {
				continue
			}
			tg := target{blob: parts[0]}
			for _, s := range splitCSV(parts[1]) {
				var n int
				fmt.Sscanf(s, "%d", &n)
				tg.states = append(tg.states, StateID(n))
			}
			targets = append(targets, tg)
		}
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

// TestScanRepeatForkStates (gated GTS_DECODE=1) scans every state of a blob for
// the fork-reduction shape — a conflict with exactly one repetition shift and
// one or more reduces — and tallies, per (state, reduced-symbol), how many
// lookaheads carry it. States with many lookaheads at a single *_repeat1 reduce
// are the structural fork-reduction candidates (the JS state-9 / rust state-83
// shape). Cross-reference the output with parse_gap surviving-fork hotness to
// pick targets.
//
//	GTS_DECODE=1 go test . -run TestScanRepeatForkStates -v
func TestScanRepeatForkStates(t *testing.T) {
	if os.Getenv("GTS_DECODE") == "" {
		t.Skip("diagnostic; set GTS_DECODE=1 to run")
	}
	blobs := []string{"c", "cpp", "go", "java", "c_sharp"}
	if env := os.Getenv("GTS_DECODE_BLOBS"); env != "" {
		blobs = splitCSV(env)
	}
	for _, name := range blobs {
		lang := loadBlobForDecode(t, name)
		p := NewParser(lang)
		type key struct {
			state  StateID
			reduce string
		}
		laCount := map[key]int{}
		stateCount := int(lang.StateCount)
		if stateCount == 0 {
			stateCount = len(lang.ParseTable) + len(lang.SmallParseTableMap)
		}
		for st := 0; st < stateCount; st++ {
			p.forEachActionIndexInState(StateID(st), func(sym Symbol, idx uint16) bool {
				if int(idx) >= len(lang.ParseActions) {
					return true
				}
				acts := lang.ParseActions[idx].Actions
				reduceSym, ok := repetitionShiftVsReduceShape(acts)
				if !ok {
					return true
				}
				laCount[key{StateID(st), symName(lang, reduceSym)}]++
				return true
			})
		}
		type row struct {
			state  StateID
			reduce string
			las    int
		}
		var rows []row
		for k, c := range laCount {
			rows = append(rows, row{k.state, k.reduce, c})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].las > rows[j].las })
		t.Logf("\n===== %s: top repetition-shift-vs-reduce fork states (by #lookaheads) =====", name)
		limit := 20
		for i, r := range rows {
			if i >= limit {
				break
			}
			t.Logf("  state %-5d reduce=%-40s lookaheads=%d", r.state, r.reduce, r.las)
		}
	}
}

func splitCSVSemi(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ';' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func splitOnColon(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == ':' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

func splitCSV(s string) []string {
	var out []string
	cur := ""
	for _, c := range s {
		if c == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(c)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

// repetitionShiftVsReduceShape returns the (first) reduced symbol when actions
// match the collapse shape: exactly one repetition shift + >=1 reduce, nothing
// else. This is the same precondition repetitionShiftConflictChoice enforces.
func repetitionShiftVsReduceShape(actions []ParseAction) (Symbol, bool) {
	if len(actions) < 2 {
		return 0, false
	}
	shifts, reduces := 0, 0
	var reduceSym Symbol
	for _, a := range actions {
		switch a.Type {
		case ParseActionShift:
			if !a.Repetition {
				return 0, false
			}
			shifts++
		case ParseActionReduce:
			if reduces == 0 {
				reduceSym = a.Symbol
			}
			reduces++
		default:
			return 0, false
		}
	}
	if shifts != 1 || reduces < 1 {
		return 0, false
	}
	return reduceSym, true
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
