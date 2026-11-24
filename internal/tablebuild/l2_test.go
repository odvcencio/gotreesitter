package tablebuild

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/tablebuild/ref"
)

// Layer L2 compares three facts of the raw parse table, each of which
// depends on the whole canonical LR(1) search, with the parser.c that the
// unmodified generator v0.27.1 writes from the same grammar.json:
//
//   - the symbols in id order (populate_used_symbols, then the aliases that
//     match no symbol, as render.rs: fn init numbers them);
//   - the production infos in id order (get_production_id numbers them as
//     add_actions first reduces each one) and MAX_ALIAS_SEQUENCE_LENGTH;
//   - the terminal cells of the error state (populate_error_state).
//
// The records are those of internal/tablebuild/ref (section "L2 ..."), so the
// port's lines and the reference's lines compare field by field.
//
// Two modes:
//
//  1. Pure-Go (default): the port's records are compared with committed
//     digests, the files of stage 0 (testdata/<grammar>/digests.json[.gz]),
//     of which only the L2 sections are read.
//  2. Reference mode, with TABLEBUILD_REF set to a reference set written by
//     cgo_harness/docker/tablebuild_reference.sh: the test reads
//     <grammar>/parser.c through cmd/ts2go (TABLEBUILD_TS2GO names a built
//     ts2go binary; without it the test runs go run ./cmd/ts2go), compares
//     full lines and prints both sides of the first differing record.
//
// The records are numbered by the port of render.rs (layout.go): the raw
// table goes through the passes that BuildTables runs without merging, and
// Render numbers it. The symbols, the production infos and the error state
// are the same in the merged table of parser.c: the passes change neither
// the symbols nor the production infos, the error state merges with no other
// state and stays state 0, and render.rs gives the action lists of state 0
// the first offsets. A grammar whose error state shifts (one with
// non-terminal extras) would print the unmerged targets; the reference
// prints the merged ones.

const (
	l2RefEnv   = "TABLEBUILD_REF"
	l2TS2GoEnv = "TABLEBUILD_TS2GO"
)

var l2Grammars = []string{"json", "go"}

// l2Build is the raw table build of one grammar and what L2 derives from it.
type l2Build struct {
	raw    *RawTables
	tables *Tables
	layout *Layout
}

func readTestdataGrammar(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name, "grammar.json"))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// buildRaw prepares a grammar and builds its raw tables.
func buildRaw(t testing.TB, grammarJSON []byte, order closureOrder) (*PreparedGrammar, []VariableInfo, *RawTables) {
	t.Helper()
	g, err := Prepare(grammarJSON)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	info, err := GetVariableInfo(g.Syntax, g.Lex, g.DefaultAliases)
	if err != nil {
		t.Fatalf("variable info: %v", err)
	}
	raw, err := buildRawTables(g, info, order)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return g, info, raw
}

func newL2Build(t testing.TB, grammarJSON []byte) *l2Build {
	t.Helper()
	g, info, raw := buildRaw(t, grammarJSON, ascendingVariables)
	tables, err := finishTables(g, raw, 0)
	if err != nil {
		t.Fatalf("tables: %v", err)
	}
	// The ABI version reaches neither the symbols, the production infos nor
	// the error state.
	layout, err := Render(tables.ParseTable, g, GetSupertypeSymbolMap(g.Syntax, g.DefaultAliases, info), ABIVersionMax)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return &l2Build{raw: raw, tables: tables, layout: layout}
}

func l2Line(fields ...string) string { return strings.Join(fields, "\t") }

// records writes the port's L2 records.
func (b *l2Build) records() []string {
	l := b.layout
	var out []string
	for i, name := range l.SymbolNames {
		out = append(out, l2Line("L2", "symbol", strconv.Itoa(i), strconv.Quote(name)))
	}
	out = append(out, l2Line("L2", "max-alias-sequence-length", strconv.Itoa(l.MaxAliasSequenceLength)))
	for id := 0; id < l.ProductionIDCount; id++ {
		var aliases, fields []string
		if id < len(l.AliasSequences) {
			seq := l.AliasSequences[id]
			n := len(seq)
			for n > 0 && seq[n-1] == 0 {
				n--
			}
			for _, a := range seq[:n] {
				aliases = append(aliases, strconv.Itoa(int(a)))
			}
		}
		if id < len(l.FieldMapSlices) {
			sl := l.FieldMapSlices[id]
			for _, e := range l.FieldMapEntries[sl[0] : sl[0]+sl[1]] {
				f := fmt.Sprintf("%d:%d", e.FieldID, e.ChildIndex)
				if e.Inherited {
					f += "i"
				}
				fields = append(fields, f)
			}
		}
		out = append(out, l2Line("L2", "production", strconv.Itoa(id), "aliases="+strings.Join(aliases, " "), "fields="+strings.Join(fields, " ")))
	}
	return append(out, b.errorStateRecord())
}

// renderedActionText is the text of one action in internal/tablebuild/ref.
func renderedActionText(a RenderedAction) string {
	switch a.Kind {
	case ActionShift:
		s := "S" + strconv.Itoa(int(a.State))
		if a.IsRepetition {
			s += "r"
		}
		return s
	case ActionShiftExtra:
		return "S0x"
	case ActionReduce:
		return fmt.Sprintf("R%d/%d/%d/%d", a.Symbol, a.ChildCount, a.DynamicPrecedence, a.ProductionID)
	case ActionAccept:
		return "A"
	}
	return "E"
}

// errorStateRecord is the "L2 error-state" record: the terminal cells of
// state 0 in symbol id order, each with its action list offset.
func (b *l2Build) errorStateRecord() string {
	l := b.layout
	lists := map[uint32]ActionListEntry{}
	for _, e := range l.ParseActions {
		lists[e.Offset] = e
	}
	f := []string{"L2", "error-state"}
	for sym := 0; sym < l.TokenCount; sym++ {
		off := l.ParseTable[0][sym]
		if off == 0 {
			continue
		}
		var acts []string
		for _, a := range lists[uint32(off)].Actions {
			acts = append(acts, renderedActionText(a))
		}
		f = append(f, fmt.Sprintf("%d=A%d[%s]", sym, off, strings.Join(acts, " ")))
	}
	return l2Line(f...)
}

func isL2(section string) bool { return strings.HasPrefix(section, "L2 ") }

// l2Digests returns the committed L2 digests of a grammar.
func l2Digests(t *testing.T, name string) map[string][]string {
	t.Helper()
	dir := filepath.Join("testdata", name)
	f, _, err := ref.ReadDigestDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{}
	for s, d := range f.Sections {
		if isL2(s) {
			want[s] = d
		}
	}
	if len(want) == 0 {
		t.Fatalf("%s has no L2 sections", dir)
	}
	return want
}

func TestL2(t *testing.T) {
	refRoot := os.Getenv(l2RefEnv)
	for _, name := range l2Grammars {
		t.Run(name, func(t *testing.T) {
			grammarJSON := readTestdataGrammar(t, name)
			b := newL2Build(t, grammarJSON)
			port := b.records()
			logL2(t, b, port)
			if refRoot == "" {
				if d := ref.CompareDigests(port, l2Digests(t, name)); d != nil {
					t.Fatalf("L2 differs from the committed digests:\n%s", d)
				}
				return
			}
			refL2 := ref.Without(l2Reference(t, refRoot, name, grammarJSON), func(s string) bool { return !isL2(s) })
			if d := ref.Compare(port, refL2); d != nil {
				t.Errorf("L2 differs from %s:\n%s", filepath.Join(refRoot, name, "parser.c"), d)
			}
			if d := ref.CompareDigests(refL2, l2Digests(t, name)); d != nil {
				t.Errorf("the committed L2 digests differ from the reference (reference shown as port):\n%s", d)
			}
		})
	}
}

func logL2(t *testing.T, b *l2Build, port []string) {
	t.Helper()
	secs := ref.Sections(port)
	t.Logf("raw states %d, production infos %d, symbols %d (+%d aliases), closure insertions %d; records: symbol %d, production %d, error-state %d",
		b.tables.Stats.RawStates, b.layout.ProductionIDCount, b.layout.SymbolCount, b.layout.AliasCount, b.raw.ClosureEntries,
		len(secs["L2 symbol"]), len(secs["L2 production"]), len(secs["L2 error-state"]))
	var peaks []string
	for _, bg := range allBudgets {
		peaks = append(peaks, fmt.Sprintf("%s: %d", bg.what, bg.peak.Load()))
	}
	t.Logf("budget peaks: %s", strings.Join(peaks, "; "))
}

// l2Reference reads the records of <root>/<name>/parser.c through cmd/ts2go.
func l2Reference(t *testing.T, root, name string, grammarJSON []byte) []string {
	return layerReference(t, root, name, grammarJSON, "parser.c", "L4")
}

// layerReference reads the records of <root>/<name>/<file> through
// cmd/ts2go, with -tablebuild-tables tables (L4 for parser.c, L3 for
// parser-noopt.c), after checking that the reference set was built from the
// grammar.json of testdata.
func layerReference(t *testing.T, root, name string, grammarJSON []byte, file, tables string) []string {
	t.Helper()
	dir := filepath.Join(root, name)
	refJSON, err := os.ReadFile(filepath.Join(dir, "src", "grammar.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(refJSON) != string(grammarJSON) {
		t.Fatalf("testdata/%s/grammar.json differs from %s", name, filepath.Join(dir, "src", "grammar.json"))
	}
	out := filepath.Join(t.TempDir(), "records.txt")
	args := []string{"-input", filepath.Join(dir, file), "-name", name, "-tablebuild-records", out, "-tablebuild-tables", tables}
	var cmd *exec.Cmd
	if bin := os.Getenv(l2TS2GoEnv); bin != "" {
		cmd = exec.Command(bin, args...)
	} else {
		cmd = exec.Command("go", append([]string{"run", "./cmd/ts2go"}, args...)...)
		cmd.Dir = filepath.Join("..", "..")
	}
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("ts2go: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
}
