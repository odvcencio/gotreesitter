package tablebuild

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	gts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

// Layer L4 compares the parse table that this package builds with the merge
// of compatible states (BuildTables with OptMergeStates, then Render) with
// the parser.c that the unmodified generator v0.27.1 writes from the same
// grammar.json: the counts (the state count and the large state count among
// them), every cell of every state (the action list offset and the ordered
// actions of a token, the state of a non-terminal), the primary state ids and
// ts_symbol_map. The records are the "L4 ..." sections of
// internal/tablebuild/ref, written from the Language that Layout.Language
// returns, so the port and the reference pass through the same record
// writer.
//
// Two modes, as for layers L2 and L3:
//
//  1. Pure-Go (default): the records are compared with the L4 sections of
//     the committed digest file of the reference set (testdata/<grammar>,
//     read with ref.ReadDigestDir).
//  2. Reference mode, with TABLEBUILD_REF set to a reference set written by
//     cgo_harness/docker/tablebuild_reference.sh: the test reads
//     <grammar>/parser.c through cmd/ts2go (TABLEBUILD_TS2GO names a built
//     ts2go binary), compares full lines and prints both sides of the first
//     differing record, then checks that the committed digests equal the
//     reference.
//
// TABLEBUILD_L4_OUT names a folder: the test writes the port's records there
// as <grammar>.l4.txt.

const l4OutEnv = "TABLEBUILD_L4_OUT"

// l4Build is the merged table build of one grammar.
type l4Build struct {
	tables *Tables
	layout *Layout
	lang   *gts.Language
	// search is the time of Prepare through BuildRawTables; passes the time
	// of the passes after it (the merge among them), Render and
	// Layout.Language.
	search, passes time.Duration
	// heapAfter is the heap in use after the build, in bytes.
	heapAfter uint64
}

func newL4Build(t testing.TB, name string, grammarJSON []byte, abi int) *l4Build {
	t.Helper()
	start := time.Now()
	g, info, raw := buildRaw(t, grammarJSON, ascendingVariables)
	search := time.Since(start)
	tables, err := finishTables(g, raw, OptMergeStates)
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
	passes := time.Since(start) - search
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return &l4Build{tables: tables, layout: layout, lang: lang, search: search, passes: passes, heapAfter: ms.HeapInuse}
}

func isL4(section string) bool { return strings.HasPrefix(section, "L4 ") }

// records writes the port's L4 sections. The record writer reads a lex mode
// per state; the lex modes are not built yet, so they are zero here and the
// sections that hold them are dropped.
func (b *l4Build) records(t testing.TB) []string {
	t.Helper()
	b.lang.LexModes = make([]gts.LexMode, b.lang.StateCount)
	symbolMap := make([]int, len(b.layout.SymbolMap))
	for i, s := range b.layout.SymbolMap {
		symbolMap[i] = int(s)
	}
	lines, err := ref.LanguageRecords(b.lang, ref.Extra{SymbolMap: symbolMap, MaxAliasSequenceLength: -1}, "L4")
	if err != nil {
		t.Fatalf("records: %v", err)
	}
	return ref.Without(lines, func(s string) bool { return !isL4(s) })
}

// l4Digests returns the committed digests of a grammar's L4 sections.
func l4Digests(t *testing.T, name string) map[string][]string {
	t.Helper()
	dir := filepath.Join("testdata", name)
	f, _, err := ref.ReadDigestDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for s, d := range f.Sections {
		if isL4(s) {
			want[s] = d
		}
	}
	if len(want) == 0 {
		t.Fatalf("%s has no L4 sections", dir)
	}
	return want
}

func TestL4(t *testing.T) {
	refRoot := os.Getenv(l2RefEnv)
	for _, gr := range l3Grammars {
		name := gr.name
		t.Run(name, func(t *testing.T) {
			grammarJSON := readTestdataGrammar(t, name)
			b := newL4Build(t, name, grammarJSON, gr.abi)
			port := b.records(t)
			logL4(t, b, port)
			if dir := os.Getenv(l4OutEnv); dir != "" {
				if err := os.WriteFile(filepath.Join(dir, name+".l4.txt"), []byte(strings.Join(port, "\n")+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if refRoot == "" {
				if d := ref.CompareDigests(port, l4Digests(t, name)); d != nil {
					t.Fatalf("L4 differs from the committed digests:\n%s", d)
				}
				return
			}
			refLines := ref.Without(layerReference(t, refRoot, name, grammarJSON, "parser.c", "L4"), func(s string) bool { return !isL4(s) })
			if d := ref.Compare(port, refLines); d != nil {
				t.Errorf("L4 differs from %s:\n%s", filepath.Join(refRoot, name, "parser.c"), d)
			}
			if d := ref.CompareDigests(refLines, l4Digests(t, name)); d != nil {
				t.Errorf("the committed L4 digests differ from the reference (reference shown as port):\n%s", d)
			}
		})
	}
}

func logL4(t *testing.T, b *l4Build, port []string) {
	t.Helper()
	s := b.tables.Stats
	t.Logf("states: raw %d; merge: core groups %d, groups after states_conflict %d, state_successors_differ calls %d, after merge %d; unit-reduction states %d, after remove_unused_states %d, final %d (large %d)",
		s.RawStates, s.CoreGroups, s.GroupsAfterConflicts, s.SuccessorPasses, s.StatesAfterMerge,
		s.UnitReductionStates, s.AfterUnusedStates, s.StatesAfterMinimizing, b.layout.LargeStateCount)
	t.Logf("action lists: after intern_table %d, after remove_unit_reductions %d, after canonicalize %d; fragile entries %d; action list slots %d; merge work %d (limit %d)",
		s.RawActionLists, s.AfterUnitReductions, s.CanonicalActionLists, s.FragileEntries, len(b.lang.ParseActions), s.MergeWork, bMergeWork.limit)
	t.Logf("%d records; prepare and search %v, passes and render %v; heap in use after the build %d MB",
		len(port), b.search.Round(time.Millisecond), b.passes.Round(100*time.Microsecond), b.heapAfter>>20)
}

// TestL4TableInvariants checks what the records cannot show, on the merged
// table: see checkTableInvariants. It also checks that the merge leaves
// every state of a group with one core id, so that each primary state id
// names a state with the same core id.
func TestL4TableInvariants(t *testing.T) {
	b := newL4Build(t, "json", readTestdataGrammar(t, "json"), 14)
	checkTableInvariants(t, b.tables, b.layout)
	table := b.tables.ParseTable
	for i, p := range b.layout.PrimaryStateIDs {
		if table.States[p].CoreID != table.States[i].CoreID {
			t.Errorf("state %d (core %d) has primary state %d (core %d)", i, table.States[i].CoreID, p, table.States[p].CoreID)
		}
	}
	if b.tables.Stats.StatesAfterMerge >= b.tables.Stats.RawStates {
		t.Errorf("the merge kept %d of %d states", b.tables.Stats.StatesAfterMerge, b.tables.Stats.RawStates)
	}
}
