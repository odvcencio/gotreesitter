package tablebuild

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
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
//     digests: testdata/json/digests.json (the reference set of stage 0) and
//     testdata/go/l2.digests.json (the L2 sections only; Go's full digest
//     file would hold every state of the unmerged table).
//  2. Reference mode, with TABLEBUILD_REF set to a reference set written by
//     cgo_harness/docker/tablebuild_reference.sh: the test reads
//     <grammar>/parser.c through cmd/ts2go (TABLEBUILD_TS2GO names a built
//     ts2go binary; without it the test runs go run ./cmd/ts2go), compares
//     full lines and prints both sides of the first differing record. With
//     -l2.update it writes testdata/go/l2.digests.json.
//
// Numbering the symbols, aliases and fields of the records takes parts of
// render.rs that a later stage ports in full (fn init, symbols_for_alias,
// add_field_sequences, get_parse_action_list_id); until then this file uses
// the test-side copies that layer L1 uses (l1port_test.go). The error state's
// action list offsets need the reusable flag of each entry, which
// mark_fragile_tokens sets after the merge; the merge does not change the
// error state, so the flags are read from lexbuild's port of that function
// on the raw error state.

const (
	l2RefEnv   = "TABLEBUILD_REF"
	l2TS2GoEnv = "TABLEBUILD_TS2GO"
)

var l2Update = flag.Bool("l2.update", false, "rewrite testdata/go/l2.digests.json from the reference set (needs "+l2RefEnv+")")

var l2Grammars = []string{"json", "go"}

// l2Build is the raw table build of one grammar and what L2 derives from it.
type l2Build struct {
	port *l1Port
	raw  *RawTables
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
	p := &l1Port{g: g, info: info}
	p.symbols = raw.Table.Symbols
	// render.rs: fn init: assign_symbol_id for each symbol in order.
	used := map[string]bool{}
	p.ids = map[Symbol]string{}
	for _, s := range p.symbols {
		id := p.symbolID(s, used)
		used[id] = true
		p.ids[s] = id
	}
	p.ids[lexbuild.EndOfNonTerminalExtraSym()] = p.ids[lexbuild.EndSym()]
	p.computeSymbolMap()
	for i := range raw.Table.ProductionInfos {
		p.infos = append(p.infos, l1Info(&raw.Table.ProductionInfos[i]))
	}
	p.computeAliasIDs()
	return &l2Build{port: p, raw: raw}
}

// l1Info converts a production info into the form of l1port_test.go: the
// fields in the order of render.rs: fn add_field_sequences (stably sorted by
// name; one name's locations in the order they were pushed).
func l1Info(pi *ProductionInfo) productionInfo {
	out := productionInfo{aliases: append([]Alias(nil), pi.AliasSequence...)}
	for _, name := range pi.FieldNames() {
		for _, loc := range pi.FieldMap[name] {
			out.fields = append(out.fields, fieldLocation{name, int(loc.Index), loc.Inherited})
		}
	}
	return out
}

func l2Line(fields ...string) string { return strings.Join(fields, "\t") }

// records writes the port's L2 records.
func (b *l2Build) records() []string {
	p, table := b.port, b.raw.Table
	symbolIndex := map[Symbol]int{}
	for i, s := range p.symbols {
		symbolIndex[s] = i
	}
	var out []string
	for i, s := range p.symbols {
		name, _ := p.metadataForSymbol(s)
		if a, ok := p.g.DefaultAliases.get(s); ok {
			name = a.Value
		}
		out = append(out, l2Line("L2", "symbol", strconv.Itoa(i), strconv.Quote(name)))
	}
	for k, a := range p.uniqueAliases {
		out = append(out, l2Line("L2", "symbol", strconv.Itoa(len(p.symbols)+k), strconv.Quote(a.Value)))
	}
	out = append(out, l2Line("L2", "max-alias-sequence-length", strconv.Itoa(table.MaxAliasedProductionLength)))

	aliasID := func(a Alias) int {
		if existing := p.symbolsForAlias(a); len(existing) > 0 {
			return symbolIndex[p.symbolMap[existing[0]]]
		}
		for k, u := range p.uniqueAliases {
			if aliasEq(u, a) {
				return len(p.symbols) + k
			}
		}
		panic(fmt.Sprintf("alias %q has no id", a.Value))
	}
	fieldNames := map[string]bool{}
	for i := range table.ProductionInfos {
		for name := range table.ProductionInfos[i].FieldMap {
			fieldNames[name] = true
		}
	}
	sortedFields := make([]string, 0, len(fieldNames))
	for name := range fieldNames {
		sortedFields = append(sortedFields, name)
	}
	sort.Strings(sortedFields)
	fieldID := map[string]int{}
	for i, name := range sortedFields {
		fieldID[name] = i + 1
	}
	for id, info := range p.infos {
		var aliases, fields []string
		for _, a := range info.aliases {
			if a.Set {
				aliases = append(aliases, strconv.Itoa(aliasID(a)))
			} else {
				aliases = append(aliases, "0")
			}
		}
		for _, f := range info.fields {
			s := fmt.Sprintf("%d:%d", fieldID[f.name], f.index)
			if f.inherited {
				s += "i"
			}
			fields = append(fields, s)
		}
		out = append(out, l2Line("L2", "production", strconv.Itoa(id), "aliases="+strings.Join(aliases, " "), "fields="+strings.Join(fields, " ")))
	}
	return append(out, b.errorStateRecord(symbolIndex))
}

// actionText is the text of one action in internal/tablebuild/ref. Shift
// targets are raw state ids: a grammar with non-terminal extras has shifts
// in the error state, and their targets are renumbered by the merge.
func actionText(a ParseAction) string {
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
		return fmt.Sprintf("R?%d/%d/%d", a.ChildCount, a.DynamicPrecedence, a.ProductionID)
	case ActionAccept:
		return "A"
	}
	return "E"
}

// errorStateRecord is the "L2 error-state" record: the terminal cells of
// state 0 in symbol id order, each with its action list offset. Offsets are
// those of render.rs: fn add_parse_table, which visits state 0 first: offset
// 0 is the empty list, and each new (actions, reusable) pair takes the next
// offset, advancing by 1 plus its length.
func (b *l2Build) errorStateRecord(symbolIndex map[Symbol]int) string {
	state := &b.raw.Table.States[0]
	var tokens lexbuild.TokenSet
	for _, e := range state.TerminalEntries {
		tokens.Insert(e.Symbol)
	}
	fragile := b.raw.Analysis.Fragile(&tokens)
	entries := append([]TerminalEntry(nil), state.TerminalEntries...)
	sort.Slice(entries, func(i, j int) bool { return symbolIndex[entries[i].Symbol] < symbolIndex[entries[j].Symbol] })
	offsets := map[string]int{}
	next := 1
	f := []string{"L2", "error-state"}
	for _, e := range entries {
		reusable := e.Entry.Reusable && !(e.Symbol.IsTerminal() && fragile.Contains(e.Symbol))
		var acts []string
		for _, a := range e.Entry.Actions {
			acts = append(acts, actionText(a))
		}
		key := fmt.Sprintf("%s|%v", strings.Join(acts, " "), reusable)
		off, ok := offsets[key]
		if !ok {
			off = next
			offsets[key] = off
			next += 1 + len(e.Entry.Actions)
		}
		f = append(f, fmt.Sprintf("%d=A%d[%s]", symbolIndex[e.Symbol], off, strings.Join(acts, " ")))
	}
	return l2Line(f...)
}

func isL2(section string) bool { return strings.HasPrefix(section, "L2 ") }

// l2Digests returns the committed L2 digests of a grammar.
func l2Digests(t *testing.T, name string) map[string][]string {
	t.Helper()
	path := filepath.Join("testdata", name, "digests.json")
	if _, err := os.Stat(path); err != nil {
		path = filepath.Join("testdata", name, "l2.digests.json")
	}
	f, err := ref.ReadDigestFile(path)
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
		t.Fatalf("%s has no L2 sections", path)
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
				if *l2Update {
					t.Fatalf("-l2.update needs %s", l2RefEnv)
				}
				if d := ref.CompareDigests(port, l2Digests(t, name)); d != nil {
					t.Fatalf("L2 differs from the committed digests:\n%s", d)
				}
				return
			}
			refL2 := ref.Without(l2Reference(t, refRoot, name, grammarJSON), func(s string) bool { return !isL2(s) })
			if d := ref.Compare(port, refL2); d != nil {
				t.Errorf("L2 differs from %s:\n%s", filepath.Join(refRoot, name, "parser.c"), d)
			}
			if *l2Update && name != "json" {
				writeL2Digests(t, refRoot, name, refL2)
				return
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
		len(b.raw.Table.States), len(b.raw.Table.ProductionInfos), len(b.port.symbols), len(b.port.uniqueAliases), b.raw.ClosureEntries,
		len(secs["L2 symbol"]), len(secs["L2 production"]), len(secs["L2 error-state"]))
	var peaks []string
	for _, bg := range allBudgets {
		peaks = append(peaks, fmt.Sprintf("%s: %d", bg.what, bg.peak.Load()))
	}
	t.Logf("budget peaks: %s", strings.Join(peaks, "; "))
}

// l2Reference reads the records of <root>/<name>/parser.c through cmd/ts2go.
func l2Reference(t *testing.T, root, name string, grammarJSON []byte) []string {
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
	args := []string{"-input", filepath.Join(dir, "parser.c"), "-name", name, "-tablebuild-records", out, "-tablebuild-tables", "L4"}
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

// writeL2Digests writes testdata/<name>/l2.digests.json: the L2 sections of
// the reference with the header of the reference set.
func writeL2Digests(t *testing.T, root, name string, refL2 []string) {
	t.Helper()
	dir := filepath.Join(root, name)
	manifest, err := os.ReadFile(filepath.Join(dir, "MANIFEST"))
	if err != nil {
		t.Fatal(err)
	}
	f := &ref.DigestFile{Grammar: name, Sources: map[string]string{}, Sections: ref.SectionDigests(refL2)}
	for _, l := range strings.Split(string(manifest), "\n") {
		fs := strings.Fields(l)
		switch {
		case len(fs) >= 2 && fs[0] == "commit":
			f.Commit = fs[1]
		case len(fs) >= 3 && fs[0] == "cli" && fs[1] == "0.27.1":
			f.Generator = strings.Join(fs[2:], " ")
		}
	}
	for _, src := range []string{"parser.c", "parser-pinned.c", "src/grammar.json"} {
		b, err := os.ReadFile(filepath.Join(dir, src))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		f.Sources[src] = hex.EncodeToString(sum[:])
	}
	pinned, err := os.ReadFile(filepath.Join(dir, "parser-pinned.c"))
	if err != nil {
		t.Fatal(err)
	}
	generated, err := os.ReadFile(filepath.Join(dir, "parser.c"))
	if err != nil {
		t.Fatal(err)
	}
	f.Eligible = ref.CompareGenerated(pinned, generated).Equal
	path := filepath.Join("testdata", name, "l2.digests.json")
	if err := os.WriteFile(path, f.Marshal(), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%d records)", path, len(refL2))
}
