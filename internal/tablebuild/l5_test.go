package tablebuild

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/lexexact"
	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

// Layer L5 compares the lex half of the tables that this package builds
// (BuildTables with OptMergeStates, then RenderTables) with the parser.c that
// the unmodified generator v0.27.1 writes from the same grammar.json: per
// parse state the lex state, the external lex state, the reserved word set
// and the reusable flag of each terminal cell; every state of the main and
// keyword lexers (accept token, end-of-input edge, ranges in first-match
// order); the keyword capture token; the reserved word sets; the external
// scanner symbol map and states. The records are the "L5 ..." sections of
// internal/tablebuild/ref, written from the Language that Layout.Language
// returns, so the port and the reference pass through the same record
// writer.
//
// Two modes, as for the other layers:
//
//  1. Pure-Go (default): the records are compared with the L5 sections of
//     the digest files of stage 0 (testdata/<grammar>/digests.json[.gz]).
//  2. Reference mode, with TABLEBUILD_REF set to a reference set written by
//     cgo_harness/docker/tablebuild_reference.sh: the test reads
//     <grammar>/parser.c through cmd/ts2go (TABLEBUILD_TS2GO names a built
//     ts2go binary), compares full lines and prints both sides of the first
//     differing record. It then reads the lex functions of parser.c with
//     internal/lexexact, which follows the C code exactly, and requires per
//     lex state the same accept code, the same step at the end of input and
//     on an ill-formed byte, and the same step on every code point where a
//     range of either side starts or ends (lexexact's CheckTransitions).
//     cmd/ts2go reads the end-of-input edge from "if (eof) ADVANCE(n);"
//     only; this check covers the steps that C takes without it.
//
// TABLEBUILD_L5_OUT names a folder: the test writes the port's records there
// as <grammar>.l5.txt.

const l5OutEnv = "TABLEBUILD_L5_OUT"

// l5Build is the whole table build of one grammar.
type l5Build struct {
	tables *Tables
	layout *Layout
	lang   *gts.Language
	// search is the time of Prepare through BuildRawTables; passes the time
	// of the passes after it (the lex stage among them), RenderTables and
	// Layout.Language.
	search, passes time.Duration
	// allocated is the heap allocated by the passes after the search, in
	// bytes.
	allocated uint64
}

func newL5Build(t testing.TB, name string, grammarJSON []byte, abi int) *l5Build {
	t.Helper()
	start := time.Now()
	g, info, raw := buildRaw(t, grammarJSON, ascendingVariables)
	search := time.Since(start)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	passStart := time.Now()
	tables, err := finishTables(g, raw, OptMergeStates)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	layout, err := RenderTables(tables, g, GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info), abi)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	lang, err := layout.Language(name)
	if err != nil {
		t.Fatalf("language: %v", err)
	}
	passes := time.Since(passStart)
	runtime.ReadMemStats(&after)
	return &l5Build{tables: tables, layout: layout, lang: lang, search: search, passes: passes, allocated: after.TotalAlloc - before.TotalAlloc}
}

func isL5(section string) bool { return strings.HasPrefix(section, "L5 ") }

// records writes the port's L5 sections.
func (b *l5Build) records(t testing.TB) []string {
	t.Helper()
	symbolMap := make([]int, len(b.layout.SymbolMap))
	for i, s := range b.layout.SymbolMap {
		symbolMap[i] = int(s)
	}
	lines, err := ref.LanguageRecords(b.lang, ref.Extra{SymbolMap: symbolMap, MaxAliasSequenceLength: -1}, "L4")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	return ref.Without(lines, func(s string) bool { return !isL5(s) })
}

// l5Digests returns the committed digests of a grammar's L5 sections.
func l5Digests(t *testing.T, name string) map[string][]string {
	t.Helper()
	dir := filepath.Join("testdata", name)
	f, _, err := ref.ReadDigestDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for s, d := range f.Sections {
		if isL5(s) {
			want[s] = d
		}
	}
	if len(want) == 0 {
		t.Fatalf("%s has no L5 sections", dir)
	}
	return want
}

func TestL5(t *testing.T) {
	refRoot := os.Getenv(l2RefEnv)
	for _, gr := range l3Grammars {
		name := gr.name
		t.Run(name, func(t *testing.T) {
			grammarJSON := readTestdataGrammar(t, name)
			b := newL5Build(t, name, grammarJSON, gr.abi)
			port := b.records(t)
			logL5(t, b, port)
			if dir := os.Getenv(l5OutEnv); dir != "" {
				if err := os.WriteFile(filepath.Join(dir, name+".l5.txt"), []byte(strings.Join(port, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if refRoot == "" {
				if d := ref.CompareDigests(port, l5Digests(t, name)); d != nil {
					t.Fatalf("L5 differs from the committed digests:\n%s", d)
				}
				return
			}
			refLines := ref.Without(layerReference(t, refRoot, name, grammarJSON, "parser.c", "L4"), func(s string) bool { return !isL5(s) })
			if d := ref.Compare(port, refLines); d != nil {
				t.Errorf("L5 differs from %s:\n%s", filepath.Join(refRoot, name, "parser.c"), d)
			}
			if d := ref.CompareDigests(refLines, l5Digests(t, name)); d != nil {
				t.Errorf("the committed L5 digests differ from the reference (reference shown as port):\n%s", d)
			}
			checkLexAgainstC(t, b.layout, filepath.Join(refRoot, name))
		})
	}
}

func logL5(t *testing.T, b *l5Build, port []string) {
	t.Helper()
	s := b.tables.Stats
	perLex := map[uint32]int{}
	largest := 0
	for _, m := range b.layout.LexModes {
		if m.LexState == NoLexState {
			continue
		}
		perLex[m.LexState]++
		largest = max(largest, perLex[m.LexState])
	}
	noStep := 0 // lex states where C steps at the end of input without "if (eof) ADVANCE(n);"
	for _, states := range [][]LexState{b.layout.MainLex, b.layout.KeywordLex} {
		for _, st := range states {
			step, ok := lexexact.UnpackStep(st.OnEOF)
			if ok != (st.EOFAction >= 0) || (ok && step.Target != int(st.EOFAction)) {
				noStep++
			}
		}
	}
	t.Logf("parse states %d; token sets: distinct %d, after merge_token_set %d; lex states: main %d, keyword %d, start states used %d, parse states on the largest %d; external lex states %d",
		len(b.layout.LexModes), s.LexTokenSets, s.LexGroups, s.MainLexStates, s.KeywordLexStates, len(perLex), largest, s.ExternalLexStates)
	t.Logf("fragile entries %d, fragile work %d (limit %d); lex states whose C end-of-input step is not their if (eof) edge %d",
		s.FragileEntries, s.FragileWork, bFragileWork.limit, noStep)
	t.Logf("%d records; prepare and search %v; passes, render and conversion %v (lex stage %v); allocated by the passes %d KB",
		len(port), b.search.Round(time.Millisecond), b.passes.Round(100*time.Microsecond), s.LexTime.Round(100*time.Microsecond), b.allocated>>10)
}

// rePragma matches the compiler pragma line that render.rs writes before a
// large lex function; lexexact does not read it, and it changes no step.
var rePragma = regexp.MustCompile(`(?m)^#pragma GCC optimize [^\n]*\n`)

// checkLexAgainstC compares the port's lex functions with those of
// <dir>/parser.c as internal/lexexact reads them.
func checkLexAgainstC(t *testing.T, layout *Layout, dir string) {
	t.Helper()
	pc, err := os.ReadFile(filepath.Join(dir, "parser.c"))
	if err != nil {
		t.Fatal(err)
	}
	ph, err := os.ReadFile(filepath.Join(dir, "tree_sitter", "parser.h"))
	if err != nil {
		t.Fatal(err)
	}
	src, err := lexexact.Parse(rePragma.ReplaceAllString(string(pc), ""), string(ph))
	if err != nil {
		t.Fatalf("lexexact.Parse: %v", err)
	}
	want, err := lexexact.BuildUnchecked(src, string(pc))
	if err != nil {
		t.Fatalf("lexexact.BuildUnchecked: %v", err)
	}
	got := layout.LexExact()
	for _, fn := range []struct {
		name      string
		port      []LexState
		got, want *lexexact.Func
		c         *lexexact.Lexer
	}{
		{"ts_lex", layout.MainLex, &got.Main, &want.Main, src.Main},
		{"ts_lex_keywords", layout.KeywordLex, &got.Keyword, &want.Keyword, src.Keyword},
	} {
		if len(fn.got.Accept) != len(fn.want.Accept) {
			t.Errorf("%s: %d states, C %d", fn.name, len(fn.got.Accept), len(fn.want.Accept))
			continue
		}
		for i := range fn.got.Accept {
			if fn.got.Accept[i] != fn.want.Accept[i] || fn.got.OnEOF[i] != fn.want.OnEOF[i] || fn.got.OnInvalid[i] != fn.want.OnInvalid[i] {
				t.Errorf("%s state %d: accept, end-of-input step, invalid-input step: port %d %d %d, C %d %d %d", fn.name, i,
					fn.got.Accept[i], fn.got.OnEOF[i], fn.got.OnInvalid[i], fn.want.Accept[i], fn.want.OnEOF[i], fn.want.OnInvalid[i])
				break
			}
		}
		if fn.c == nil {
			if len(fn.port) > 0 {
				t.Errorf("%s: the port has %d states, parser.c has no such function", fn.name, len(fn.port))
			}
			continue
		}
		blob := make([][]lexexact.Transition, len(fn.port))
		for i, st := range fn.port {
			for _, r := range st.Ranges {
				blob[i] = append(blob[i], lexexact.Transition{Lo: rune(r.Lo), Hi: rune(r.Hi), Next: int(r.Target), Skip: r.Skip})
			}
		}
		if err := fn.c.CheckTransitions(blob); err != nil {
			t.Errorf("%s: the port's ranges differ from parser.c: %v", fn.name, err)
		}
		t.Logf("%s: %d states equal to parser.c in accept, end-of-input and invalid-input steps and ranges", fn.name, len(fn.got.Accept))
	}
}

// TestL5RenderWithoutLexTables: Render, which has no lex tables, writes the
// lex modes, the reserved words and the external scanner tables as
// RenderTables does, and no lex function.
func TestL5RenderWithoutLexTables(t *testing.T) {
	grammarJSON := readTestdataGrammar(t, "json")
	b := newL5Build(t, "json", grammarJSON, 14)
	g, info, raw := buildRaw(t, grammarJSON, ascendingVariables)
	tables, err := finishTables(g, raw, OptMergeStates)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := Render(tables.ParseTable, g, GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info), 14)
	if err != nil {
		t.Fatal(err)
	}
	if layout.MainLex != nil || layout.KeywordLex != nil || layout.LexExact() != nil {
		t.Errorf("Render wrote lex functions")
	}
	if fmt.Sprint(layout.LexModes) != fmt.Sprint(b.layout.LexModes) {
		t.Errorf("lex modes differ:\n Render       %v\n RenderTables %v", layout.LexModes, b.layout.LexModes)
	}
	if _, err := RenderTables(&Tables{ParseTable: tables.ParseTable}, g, nil, 14); err == nil {
		t.Errorf("RenderTables without lex tables: no error")
	}
}
