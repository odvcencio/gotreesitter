package tablebuild

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

// Layer L3 compares the parse table that this package builds without merging
// (BuildTables with OptLevel 0, then Render) with the parser-noopt.c that the
// unmodified generator v0.27.1 writes with --disable-optimizations from the
// same grammar.json: the counts, every cell of every state (the action list
// offset and the ordered actions of a token, the state of a non-terminal)
// and the primary state ids. The records are those of
// internal/tablebuild/ref (sections "L3 ..."), written from the Language that
// Layout.Language returns, so the port and the reference pass through the
// same record writer.
//
// Two modes, as for layer L2:
//
//  1. Pure-Go (default): the records are compared with committed digests:
//     the digest files of stage 0 (testdata/<grammar>/digests.json[.gz]),
//     of which the L3 and numbering sections are read.
//  2. Reference mode, with TABLEBUILD_REF set to a reference set written by
//     cgo_harness/docker/tablebuild_reference.sh: the test reads
//     <grammar>/parser-noopt.c through cmd/ts2go (TABLEBUILD_TS2GO names a
//     built ts2go binary), compares full lines and prints both sides of the
//     first differing record.
//
// The test also checks the numbers that Render gives and that the merge
// does not change, against parser.c (the merged table): the symbol names
// and metadata, the field names, the alias sequences and field maps (as
// sets), the non-terminal alias map, the supertype map and ts_symbol_map
// (isNumbering lists the sections). The committed digest files hold those
// sections too.
//
// TABLEBUILD_L3_OUT names a folder: the test writes the port's records there
// as <grammar>.l3.txt.

const l3OutEnv = "TABLEBUILD_L3_OUT"

// l3Grammars are the grammars of the layer and the ABI version of their
// reference sets (each MANIFEST records it; it reaches no L3 record).
var l3Grammars = []struct {
	name string
	abi  int
}{{"json", 14}, {"go", 15}}

// l3Build is the unmerged table build of one grammar.
type l3Build struct {
	tables *Tables
	layout *Layout
	lang   *gts.Language
	// search is the time of Prepare through BuildRawTables; passes the time
	// of the passes after it, Render and Layout.Language.
	search, passes time.Duration
}

func newL3Build(t testing.TB, name string, grammarJSON []byte, abi int, order closureOrder) *l3Build {
	t.Helper()
	start := time.Now()
	g, info, raw := buildRaw(t, grammarJSON, order)
	search := time.Since(start)
	tables, err := finishTables(g, raw, 0)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	layout, err := Render(tables.ParseTable, g, GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info), abi)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	lang, err := layout.Language(name)
	if err != nil {
		t.Fatalf("language: %v", err)
	}
	return &l3Build{tables: tables, layout: layout, lang: lang, search: search, passes: time.Since(start) - search}
}

func (b *l3Build) records(t testing.TB) []string {
	t.Helper()
	lines, err := ref.LanguageRecords(b.lang, ref.NoExtra, "L3")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	return lines
}

// isNumbering reports whether a section of parser.c's records holds numbers
// that Render gives and that the merge of states does not change.
func isNumbering(section string) bool {
	switch section {
	case "L1 symbol", "L1 field", "L1 alias-sequence", "L1 field-map", "L1 non-terminal-alias", "L1 supertype", "L4 symbol-map":
		return true
	}
	return false
}

// numberingRecords writes the isNumbering sections of the port. The record
// writer reads a lex mode per state; they are zero here, and the sections
// that hold them are dropped.
func (b *l3Build) numberingRecords(t testing.TB) []string {
	t.Helper()
	lang, err := b.layout.Language(b.lang.Name)
	if err != nil {
		t.Fatalf("language: %v", err)
	}
	lang.LexModes = make([]gts.LexMode, lang.StateCount)
	symbolMap := make([]int, len(b.layout.SymbolMap))
	for i, s := range b.layout.SymbolMap {
		symbolMap[i] = int(s)
	}
	lines, err := ref.LanguageRecords(lang, ref.Extra{SymbolMap: symbolMap, MaxAliasSequenceLength: -1}, "L4")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	return ref.Without(lines, func(s string) bool { return !isNumbering(s) })
}

// l3Digests returns the committed digests of a grammar's L3 and numbering
// sections.
func l3Digests(t *testing.T, name string) map[string][]string {
	t.Helper()
	dir := filepath.Join("testdata", name)
	f, _, err := ref.ReadDigestDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for s, d := range f.Sections {
		if isL3(s) || isNumbering(s) {
			want[s] = d
		}
	}
	if len(want) == 0 {
		t.Fatalf("%s has no L3 sections", dir)
	}
	return want
}

func isL3(section string) bool { return strings.HasPrefix(section, "L3 ") }

func TestL3(t *testing.T) {
	refRoot := os.Getenv(l2RefEnv)
	for _, gr := range l3Grammars {
		name := gr.name
		t.Run(name, func(t *testing.T) {
			grammarJSON := readTestdataGrammar(t, name)
			b := newL3Build(t, name, grammarJSON, gr.abi, ascendingVariables)
			port := b.records(t)
			logL3(t, b, port)
			if dir := os.Getenv(l3OutEnv); dir != "" {
				if err := os.WriteFile(filepath.Join(dir, name+".l3.txt"), []byte(strings.Join(port, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			port = append(port, b.numberingRecords(t)...)
			if refRoot == "" {
				if d := ref.CompareDigests(port, l3Digests(t, name)); d != nil {
					t.Fatalf("L3 differs from the committed digests:\n%s", d)
				}
				return
			}
			refLines := layerReference(t, refRoot, name, grammarJSON, "parser-noopt.c", "L3")
			numbering := ref.Without(layerReference(t, refRoot, name, grammarJSON, "parser.c", "L4"), func(s string) bool { return !isNumbering(s) })
			refLines = append(refLines, numbering...)
			if d := ref.Compare(port, refLines); d != nil {
				t.Errorf("L3 differs from %s (L3 sections) and parser.c (the others):\n%s", filepath.Join(refRoot, name, "parser-noopt.c"), d)
			}
			if d := ref.CompareDigests(refLines, l3Digests(t, name)); d != nil {
				t.Errorf("the committed L3 digests differ from the reference (reference shown as port):\n%s", d)
			}
		})
	}
}

func logL3(t *testing.T, b *l3Build, port []string) {
	t.Helper()
	s := b.tables.Stats
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	t.Logf("states: raw %d, unit-reduction states %d, after remove_unused_states %d, final %d (large %d)",
		s.RawStates, s.UnitReductionStates, s.AfterUnusedStates, s.StatesAfterMinimizing, b.layout.LargeStateCount)
	t.Logf("action lists: after intern_table %d, after remove_unit_reductions %d, after canonicalize %d; fragile entries %d; action list slots %d",
		s.RawActionLists, s.AfterUnitReductions, s.CanonicalActionLists, s.FragileEntries, len(b.lang.ParseActions))
	t.Logf("symbols %d (+%d aliases), tokens %d, fields %d, production ids %d; %d records; prepare and search %v, passes and render %v; heap in use %d MB, from the system %d MB",
		b.layout.SymbolCount, b.layout.AliasCount, b.layout.TokenCount, b.layout.FieldCount, b.layout.ProductionIDCount,
		len(port), b.search.Round(time.Millisecond), b.passes.Round(100*time.Microsecond), ms.HeapInuse>>20, ms.Sys>>20)
}

// TestL3ClosureOrderDoesNotMatter builds the unmerged table of each grammar
// with the closure's additions visited in ascending and in descending
// variable order (the generator fills them from a hash map; see closure.go)
// and requires equal L3 records.
func TestL3ClosureOrderDoesNotMatter(t *testing.T) {
	for _, gr := range l3Grammars {
		t.Run(gr.name, func(t *testing.T) {
			if gr.name != "json" && testing.Short() {
				t.Skip("builds the table twice")
			}
			grammarJSON := readTestdataGrammar(t, gr.name)
			up := newL3Build(t, gr.name, grammarJSON, gr.abi, ascendingVariables).records(t)
			down := newL3Build(t, gr.name, grammarJSON, gr.abi, descendingVariables).records(t)
			if d := ref.Compare(down, up); d != nil {
				t.Fatalf("descending order differs from ascending:\n%s", d)
			}
		})
	}
}

// TestL3TableInvariants checks what the records cannot show: the state ids
// that the actions and gotos name exist, states 0 and 1 stay first, the
// other states are ordered by descending entry count, and every action list
// offset names a list.
func TestL3TableInvariants(t *testing.T) {
	b := newL3Build(t, "json", readTestdataGrammar(t, "json"), 14, ascendingVariables)
	table := b.tables.ParseTable
	n := uint32(len(table.States))
	prev := -1
	for i := range table.States {
		s := &table.States[i]
		for _, e := range s.TerminalEntries {
			for _, a := range table.ActionLists.Get(e.ID) {
				if a.Kind == ActionShift && a.State >= n {
					t.Errorf("state %d shifts to %d of %d", i, a.State, n)
				}
			}
		}
		for _, e := range s.NonterminalEntries {
			if !e.Action.ShiftExtra && e.Action.State >= n {
				t.Errorf("state %d goes to %d of %d", i, e.Action.State, n)
			}
		}
		if i < 2 {
			if s.ID != uint32(i) {
				t.Errorf("state %d has search id %d", i, s.ID)
			}
			continue
		}
		size := len(s.TerminalEntries) + len(s.NonterminalEntries)
		if prev >= 0 && size > prev {
			t.Errorf("state %d has %d entries after a state with %d", i, size, prev)
		}
		prev = size
	}
	offsets := map[uint32]bool{}
	for _, e := range b.layout.ParseActions {
		offsets[e.Offset] = true
	}
	for i, row := range b.layout.ParseTable {
		for sym, v := range row {
			if sym < b.layout.TokenCount && v != 0 && !offsets[uint32(v)] {
				t.Errorf("state %d symbol %d names offset %d, which starts no list", i, sym, v)
			}
		}
	}
	if fmt.Sprint(b.layout.ParseActions[0]) != fmt.Sprint(ActionListEntry{Actions: []RenderedAction{}}) {
		t.Errorf("offset 0 is %+v, want the empty list", b.layout.ParseActions[0])
	}
}
