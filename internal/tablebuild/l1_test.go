package tablebuild

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexbuild"
)

// Layer L1 compares the prepared grammar with what the generator wrote from
// it: the public name and metadata of each symbol (default aliases decide
// them), the aliases that match no symbol, the production infos as a set
// (alias sequences and field maps, without their ids), the non-terminal alias
// map, the supertype map (ABI 15), and the fields, children and subtypes of
// each node type in node-types.json.
//
// Three modes:
//
//  1. Pinned mode, with TABLEBUILD_L1_REF set to a folder that holds
//     <grammar>/src/grammar.json, node-types.json and parser.c as pinned (the
//     src folder of the grammar repository at its locked commit). The test
//     reads the records of the reference, compares them with the port's and
//     prints each differing record with both sides. The records that differ
//     must be exactly those of l1KnownDifferences. It also checks that the
//     committed digests equal the reference's; -l1.update rewrites them.
//  2. Generator mode, with TABLEBUILD_L1_GEN set to a folder of the same
//     layout written by the unmodified tree-sitter CLI v0.27.1 from the same
//     grammar.json (tree-sitter generate --abi 15). No record may differ.
//  3. Pure-Go mode, with neither variable set: the port's records are
//     compared with testdata/<grammar>/l1.digests (the first 8 bytes of
//     SHA-256 of each pinned record line). A difference names the record key
//     and prints the port's line; l1KnownDifferences applies as in mode 1.
//
// The port names its symbols as the generator does once the used symbols are
// known; which symbols the parse table uses is L2's business. In reference
// mode the used symbols are aligned with the reference's identifiers; in
// pure-Go mode every terminal, external and non-inlined non-terminal is taken
// as used, and reference mode reports a grammar for which that differs.
//
// This test keeps its own records and digests. Moving it onto the shared
// comparer of internal/tablebuild/ref, which also defines L1 records, is
// follow-up work.

const (
	l1RefEnv = "TABLEBUILD_L1_REF"
	l1GenEnv = "TABLEBUILD_L1_GEN"
)

var l1Update = flag.Bool("l1.update", false, "rewrite testdata/<grammar>/l1.digests from the pinned reference (needs "+l1RefEnv+")")

var l1Grammars = []string{"json", "go", "markdown", "markdown_inline"}

// l1Difference is a set of records where the pinned reference differs from
// the port for a known reason.
type l1Difference struct {
	reason string
	keys   []string
}

// l1KnownDifferences lists, for each grammar, the record keys where the pinned
// node-types.json or parser.c differs from the port, with the reason. Each
// listed key must differ and no other key may; the list may only shrink.
var l1KnownDifferences = map[string]l1Difference{
	"go": {
		reason: "The pinned node-types.json was written by generator v0.25.8, which keeps `_type` as a supertype " +
			"although the grammar also inlines it. v0.27.1 drops such a supertype (intern_symbols.rs: fn intern_symbols; " +
			"the CLI warns \"rule `_type` is both a supertype and inlined. the supertype is ignored.\"), as lexbuild's port does. " +
			"The v0.27.1 CLI writes the pinned parser.c again from the pinned grammar.json (apart from the version comment and " +
			"the metadata version), and its node-types.json gives no differing record (TABLEBUILD_L1_GEN).",
		keys: []string{
			`node "_type"/named`,
			`node "_type"/named subtypes`,
			`node "argument_list"/named children`,
			`node "array_type"/named field element`,
			`node "channel_type"/named field value`,
			`node "const_spec"/named field type`,
			`node "field_declaration"/named field type`,
			`node "implicit_length_array_type"/named field element`,
			`node "map_type"/named field key`,
			`node "map_type"/named field value`,
			`node "negated_type"/named children`,
			`node "parameter_declaration"/named field type`,
			`node "parenthesized_type"/named children`,
			`node "pointer_type"/named children`,
			`node "slice_type"/named field element`,
			`node "type_alias"/named field type`,
			`node "type_assertion_expression"/named field type`,
			`node "type_case"/named field type`,
			`node "type_constraint"/named children`,
			`node "type_conversion_expression"/named field type`,
			`node "type_elem"/named children`,
			`node "type_instantiation_expression"/named children`,
			`node "type_instantiation_expression"/named field type`,
			`node "type_spec"/named field type`,
			`node "var_spec"/named field type`,
			`node "variadic_parameter_declaration"/named field type`,
		},
	},
}

func knownKeys(name string) map[string]bool {
	m := map[string]bool{}
	for _, k := range l1KnownDifferences[name].keys {
		m[k] = true
	}
	return m
}

func TestL1(t *testing.T) {
	refRoot, genRoot := os.Getenv(l1RefEnv), os.Getenv(l1GenEnv)
	for _, name := range l1Grammars {
		t.Run(name, func(t *testing.T) {
			grammarJSON, err := os.ReadFile(filepath.Join("testdata", name, "grammar.json"))
			if err != nil {
				t.Fatal(err)
			}
			newPort := func(t *testing.T) *l1Port {
				for _, b := range allBudgets {
					b.peak.Store(0)
				}
				port, err := newL1Port(grammarJSON)
				if err != nil {
					t.Fatalf("prepare: %v", err)
				}
				return port
			}
			digestPath := filepath.Join("testdata", name, "l1.digests")
			if refRoot == "" && genRoot == "" {
				if *l1Update {
					t.Fatalf("-l1.update needs %s", l1RefEnv)
				}
				port := newPort(t)
				port.alignAll()
				want, abi, err := readDigests(digestPath)
				if err != nil {
					t.Fatal(err)
				}
				compareDigests(t, port.records(abi >= 15), want, knownKeys(name))
				logPort(t, port)
				return
			}
			if genRoot != "" {
				t.Run("v0.27.1", func(t *testing.T) {
					port := newPort(t)
					ref := loadL1Ref(t, port, genRoot, name, grammarJSON)
					compareRecords(t, port.records(ref.defines["LANGUAGE_VERSION"] >= 15), ref.records(), nil)
					logPort(t, port)
				})
			}
			if refRoot != "" {
				t.Run("pinned", func(t *testing.T) {
					port := newPort(t)
					ref := loadL1Ref(t, port, refRoot, name, grammarJSON)
					want := ref.records()
					compareRecords(t, port.records(ref.defines["LANGUAGE_VERSION"] >= 15), want, knownKeys(name))
					refDigests, refABI := digestsOf(want), ref.defines["LANGUAGE_VERSION"]
					if *l1Update {
						if err := writeDigests(digestPath, refDigests, refABI); err != nil {
							t.Fatal(err)
						}
						t.Logf("wrote %s (%d records)", digestPath, len(want))
					} else if committed, abi, err := readDigests(digestPath); err != nil {
						t.Errorf("%v (run with -l1.update to write it)", err)
					} else if !equalDigests(committed, refDigests) || abi != refABI {
						t.Errorf("%s differs from the reference's digests (run with -l1.update)", digestPath)
					}
					logPort(t, port)
				})
			}
		})
	}
}

// loadL1Ref reads the reference of one grammar under root and aligns the
// port's symbols with it.
func loadL1Ref(t *testing.T, port *l1Port, root, name string, grammarJSON []byte) *l1Ref {
	t.Helper()
	dir := filepath.Join(root, name, "src")
	refJSON, err := os.ReadFile(filepath.Join(dir, "grammar.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(refJSON, grammarJSON) {
		t.Fatalf("testdata/%s/grammar.json differs from %s", name, filepath.Join(dir, "grammar.json"))
	}
	ref := readL1Ref(t, dir)
	if err := port.alignSymbols(ref.idents[:ref.defines["SYMBOL_COUNT"]]); err != nil {
		t.Fatal(err)
	}
	if len(port.unused) > 0 {
		var names []string
		for _, s := range port.unused {
			n, _ := port.metadataForSymbol(s)
			names = append(names, fmt.Sprintf("%v %q", s, n))
		}
		t.Logf("port symbols with no reference symbol id (unused): %s", strings.Join(names, ", "))
	}
	for _, s := range port.unused {
		if !(s.IsNonTerminal() && containsSymbol(port.g.Syntax.VariablesToInline, s)) {
			t.Errorf("unused symbol %v is not an inlined rule: the pure-Go mode takes it as used", s)
		}
	}
	return ref
}

func logPort(t *testing.T, p *l1Port) {
	t.Helper()
	sg := p.g.Syntax
	first := sg.VarProds[len(sg.VarProds)-1][1]
	var peaks []string
	for _, b := range allBudgets {
		peaks = append(peaks, fmt.Sprintf("%s: %d", b.what, b.peak.Load()))
	}
	t.Logf("default aliases %d, inline map entries %d, productions %d (+%d built by inlining), production infos %d, unique aliases %d; budget peaks: %s",
		len(p.g.DefaultAliases), p.g.Inlines.Len(), first, len(sg.Productions)-first, len(p.infos), len(p.uniqueAliases), strings.Join(peaks, "; "))
}

// alignAll names the port's symbols with every terminal, external and
// non-inlined non-terminal taken as used.
func (p *l1Port) alignAll() {
	sg, lg := p.g.Syntax, p.g.Lex
	var syms []Symbol
	if sg.HasWord && sg.WordToken.IsTerminal() {
		syms = append(syms, sg.WordToken)
	}
	for i := range lg.Variables {
		if s := lexbuild.TerminalSym(i); !(sg.HasWord && sg.WordToken == s) {
			syms = append(syms, s)
		}
	}
	for i := range sg.ExternalTokens {
		syms = append(syms, lexbuild.ExternalSym(i))
	}
	for i := range sg.Variables {
		if s := lexbuild.NonTerminalSym(i); !containsSymbol(sg.VariablesToInline, s) {
			syms = append(syms, s)
		}
	}
	used := map[string]bool{"ts_builtin_sym_end": true}
	idents := []string{"ts_builtin_sym_end"}
	for _, s := range syms {
		id := p.symbolID(s, used)
		used[id] = true
		idents = append(idents, id)
	}
	if err := p.alignSymbols(idents); err != nil {
		panic(err)
	}
}

// record is one L1 record line: a key that names it and a value.
type record struct{ key, value string }

// records lists the port's records. withSupertypes adds the supertype map
// (render.rs writes it for ABI 15 and later).
func (p *l1Port) records(withSupertypes bool) []record {
	var out []record
	add := func(k, v string) { out = append(out, record{k, v}) }
	for _, s := range p.symbols[1:] {
		add("symbol "+p.ids[s], p.symbolRecord(s))
	}
	for _, a := range p.uniqueAliases {
		add("alias "+p.aliasIDs[a], fmt.Sprintf("name=%q named=%t", a.Value, a.Named))
	}
	fieldNames := map[string]bool{}
	for _, info := range p.infos {
		add("production_info", info.key())
		for _, f := range info.fields {
			fieldNames[f.name] = true
		}
	}
	add("field_names", sortedKeys(fieldNames))
	for sym, list := range p.nonTerminalAliasMap() {
		add("alias_map "+sym, strings.Join(list, " "))
	}
	if withSupertypes {
		for sym, list := range p.supertypeMap() {
			add("supertype "+sym, strings.Join(list, " "))
		}
	}
	nodes, err := p.generateNodeTypes()
	if err != nil {
		add("node_types error", err.Error())
	} else {
		nodeRecords(nodes, add)
	}
	return out
}

func sortedKeys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, " ")
}

// l1Ref is what L1 reads from a reference parser.c and node-types.json.
type l1Ref struct {
	defines    map[string]int
	idents     []string // symbol id -> identifier
	names      map[string]string
	meta       map[string][3]bool // visible, named, supertype
	fieldNames map[string]string  // field identifier -> name
	infos      []string
	aliasMap   map[string][]string
	supertypes map[string][]string
	nodes      []nodeJSON
}

type nodeJSON struct {
	Type     string                   `json:"type"`
	Named    bool                     `json:"named"`
	Fields   map[string]fieldJSONType `json:"fields"`
	Children *fieldJSONType           `json:"children"`
	Subtypes []nodeRefJSON            `json:"subtypes"`
}

type fieldJSONType struct {
	Multiple bool          `json:"multiple"`
	Required bool          `json:"required"`
	Types    []nodeRefJSON `json:"types"`
}

type nodeRefJSON struct {
	Type  string `json:"type"`
	Named bool   `json:"named"`
}

var (
	reL1Define = regexp.MustCompile(`(?m)^#define (\w+) (\d+)$`)
	reL1Enum   = regexp.MustCompile(`^\s+(\w+) = (\d+),$`)
	reL1Name   = regexp.MustCompile(`^\s+\[(\w+)\] = "((?:[^"\\]|\\.)*)",$`)
	reL1Open   = regexp.MustCompile(`^\s+\[(\w+)\] = \{$`)
	reL1Flag   = regexp.MustCompile(`^\s+\.(\w+) = (true|false),$`)
	reL1Slice  = regexp.MustCompile(`^\s+\[(\w+)\] = \{\.index = (\d+), \.length = (\d+)\},$`)
	reL1Row    = regexp.MustCompile(`^\s+\[(\d+)\] =$`)
	reL1Field  = regexp.MustCompile(`^\s+\{(\w+), (\d+)(, \.inherited = true)?\},$`)
	reL1Seq    = regexp.MustCompile(`^\s+\[(\d+)\] = (\w+),$`)
	reL1Ident  = regexp.MustCompile(`^\s+(\w+),$`)
)

// l1Section returns the text from the first line that holds start up to the
// closing "};" of that definition.
func l1Section(text, start string) []string {
	i := strings.Index(text, start)
	if i < 0 {
		return nil
	}
	rest := text[i:]
	if j := strings.Index(rest, "\n};"); j >= 0 {
		rest = rest[:j]
	}
	lines := strings.Split(rest, "\n")
	return lines[1:]
}

func readL1Ref(t *testing.T, dir string) *l1Ref {
	t.Helper()
	pcBytes, err := os.ReadFile(filepath.Join(dir, "parser.c"))
	if err != nil {
		t.Fatal(err)
	}
	pc := string(pcBytes)
	r := &l1Ref{defines: map[string]int{}, names: map[string]string{}, meta: map[string][3]bool{}, fieldNames: map[string]string{}, aliasMap: map[string][]string{}}
	for _, m := range reL1Define.FindAllStringSubmatch(pc, -1) {
		r.defines[m[1]], _ = strconv.Atoi(m[2])
	}
	n := r.defines["SYMBOL_COUNT"] + r.defines["ALIAS_COUNT"]
	r.idents = make([]string, n)
	r.idents[0] = "ts_builtin_sym_end"
	for _, line := range l1Section(pc, "enum ts_symbol_identifiers {") {
		if m := reL1Enum.FindStringSubmatch(line); m != nil {
			id, _ := strconv.Atoi(m[2])
			if id >= n {
				t.Fatalf("symbol id %d out of range", id)
			}
			r.idents[id] = m[1]
		}
	}
	for id, s := range r.idents {
		if s == "" {
			t.Fatalf("symbol id %d has no identifier", id)
		}
	}
	for _, line := range l1Section(pc, "ts_symbol_names[] = {") {
		if m := reL1Name.FindStringSubmatch(line); m != nil {
			r.names[m[1]] = unescapeCString(t, m[2])
		}
	}
	cur := ""
	for _, line := range l1Section(pc, "ts_symbol_metadata[] = {") {
		if m := reL1Open.FindStringSubmatch(line); m != nil {
			cur = m[1]
			r.meta[cur] = [3]bool{}
			continue
		}
		if m := reL1Flag.FindStringSubmatch(line); m != nil && cur != "" {
			v := r.meta[cur]
			switch m[1] {
			case "visible":
				v[0] = m[2] == "true"
			case "named":
				v[1] = m[2] == "true"
			case "supertype":
				v[2] = m[2] == "true"
			}
			r.meta[cur] = v
		}
	}
	for _, line := range l1Section(pc, "ts_field_names[] = {") {
		if m := reL1Name.FindStringSubmatch(line); m != nil {
			r.fieldNames[m[1]] = unescapeCString(t, m[2])
		}
	}
	aliasRefOf := func(ident string) string {
		name, ok := r.names[ident]
		if !ok {
			t.Fatalf("alias identifier %s has no name", ident)
		}
		return aliasRef(name, r.meta[ident][1])
	}

	// Production infos: alias sequences and field maps, by production id.
	count := r.defines["PRODUCTION_ID_COUNT"]
	aliasSeqs := make([][]string, count)
	cur = ""
	for _, line := range l1Section(pc, "ts_alias_sequences[PRODUCTION_ID_COUNT]") {
		if m := reL1Open.FindStringSubmatch(line); m != nil {
			cur = m[1]
			continue
		}
		if m := reL1Seq.FindStringSubmatch(line); m != nil && cur != "" {
			p, _ := strconv.Atoi(cur)
			aliasSeqs[p] = append(aliasSeqs[p], m[1]+":"+aliasRefOf(m[2]))
		}
	}
	var flat []string
	for _, line := range l1Section(pc, "ts_field_map_entries[] = {") {
		if m := reL1Row.FindStringSubmatch(line); m != nil {
			if row, _ := strconv.Atoi(m[1]); row != len(flat) {
				t.Fatalf("field map row %d at entry %d", row, len(flat))
			}
			continue
		}
		if m := reL1Field.FindStringSubmatch(line); m != nil {
			name, ok := r.fieldNames[m[1]]
			if !ok {
				t.Fatalf("field identifier %s has no name", m[1])
			}
			e := name + ":" + m[2]
			if m[3] != "" {
				e += "*"
			}
			flat = append(flat, e)
		}
	}
	fieldMaps := make([][]string, count)
	for _, line := range l1Section(pc, "ts_field_map_slices[") {
		if m := reL1Slice.FindStringSubmatch(line); m != nil {
			p, _ := strconv.Atoi(m[1])
			idx, _ := strconv.Atoi(m[2])
			ln, _ := strconv.Atoi(m[3])
			fieldMaps[p] = flat[idx : idx+ln]
		}
	}
	for p := 0; p < count; p++ {
		r.infos = append(r.infos, "aliases=["+strings.Join(aliasSeqs[p], " ")+"] fields=["+strings.Join(fieldMaps[p], " ")+"]")
	}

	// ts_non_terminal_alias_map: symbol, count, then count identifiers (the
	// public symbol first), then 0.
	var toks []string
	for _, line := range l1Section(pc, "ts_non_terminal_alias_map[] = {") {
		for _, f := range strings.Split(line, ",") {
			if f = strings.TrimSpace(f); f != "" {
				toks = append(toks, f)
			}
		}
	}
	for k := 0; k < len(toks) && toks[k] != "0"; {
		sym := toks[k]
		c, err := strconv.Atoi(toks[k+1])
		if err != nil || k+2+c > len(toks) {
			t.Fatalf("ts_non_terminal_alias_map at %s: %v", sym, toks[k:])
		}
		var list []string
		for _, id := range toks[k+3 : k+2+c] {
			list = append(list, aliasRefOf(id))
		}
		sort.Strings(list)
		r.aliasMap[sym] = list
		k += 2 + c
	}

	// ts_supertype_map (ABI 15).
	if sec := l1Section(pc, "ts_supertype_map_entries[] = {"); sec != nil {
		r.supertypes = map[string][]string{}
		var entries []string
		for _, line := range sec {
			if m := reL1Ident.FindStringSubmatch(line); m != nil {
				entries = append(entries, m[1])
			}
		}
		for _, line := range l1Section(pc, "ts_supertype_map_slices[] = {") {
			if m := reL1Slice.FindStringSubmatch(line); m != nil {
				idx, _ := strconv.Atoi(m[2])
				ln, _ := strconv.Atoi(m[3])
				list := append([]string(nil), entries[idx:idx+ln]...)
				sort.Strings(list)
				r.supertypes[m[1]] = list
			}
		}
	}

	nt, err := os.ReadFile(filepath.Join(dir, "node-types.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(nt, &r.nodes); err != nil {
		t.Fatalf("node-types.json: %v", err)
	}
	return r
}

// records lists the reference's records, in the forms of l1Port.records.
func (r *l1Ref) records() []record {
	var out []record
	add := func(k, v string) { out = append(out, record{k, v}) }
	sc := r.defines["SYMBOL_COUNT"]
	for id := 1; id < len(r.idents); id++ {
		ident := r.idents[id]
		m := r.meta[ident]
		if id < sc {
			add("symbol "+ident, symbolRecordValue(r.names[ident], m[0], m[1], m[2]))
		} else {
			add("alias "+ident, fmt.Sprintf("name=%q named=%t", r.names[ident], m[1]))
		}
	}
	fieldNames := map[string]bool{}
	for _, info := range r.infos {
		add("production_info", info)
	}
	for _, name := range r.fieldNames {
		fieldNames[name] = true
	}
	add("field_names", sortedKeys(fieldNames))
	for sym, list := range r.aliasMap {
		add("alias_map "+sym, strings.Join(list, " "))
	}
	for sym, list := range r.supertypes {
		add("supertype "+sym, strings.Join(list, " "))
	}
	nodes := map[nodeTypeRef]*nodeInfoJSON{}
	conv := func(list []nodeRefJSON) []nodeTypeRef {
		out := []nodeTypeRef{}
		for _, x := range list {
			out = append(out, nodeTypeRef{x.Type, x.Named})
		}
		return out
	}
	convField := func(f *fieldJSONType) *fieldInfoJSON {
		return &fieldInfoJSON{multiple: f.Multiple, required: f.Required, types: conv(f.Types)}
	}
	for _, n := range r.nodes {
		e := &nodeInfoJSON{ref: nodeTypeRef{n.Type, n.Named}}
		if n.Subtypes != nil {
			e.subtypes, e.hasSub = conv(n.Subtypes), true
		}
		if n.Fields != nil {
			e.fields = map[string]*fieldInfoJSON{}
			for k, f := range n.Fields {
				e.fields[k] = convField(&f)
			}
		}
		if n.Children != nil {
			e.children = convField(n.Children)
		}
		nodes[e.ref] = e
	}
	nodeRecords(nodes, add)
	return out
}

// unescapeCString reads a string literal written by render.rs: fn sanitize_string.
func unescapeCString(t *testing.T, s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			t.Fatalf("string ends in a backslash: %q", s)
		}
		hex := func(n int) {
			if i+1+n > len(s) {
				t.Fatalf("short escape in %q", s)
			}
			v, err := strconv.ParseUint(s[i+1:i+1+n], 16, 32)
			if err != nil {
				t.Fatalf("escape in %q: %v", s, err)
			}
			b.WriteRune(rune(v))
			i += n
		}
		switch s[i] {
		case 'a':
			b.WriteByte(7)
		case 'b':
			b.WriteByte(8)
		case 'v':
			b.WriteByte(0x0b)
		case 'f':
			b.WriteByte(0x0c)
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '0':
			b.WriteByte(0)
		case 'x':
			hex(2)
		case 'u':
			hex(4)
		case 'U':
			hex(8)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// group collects the values of each key, sorted.
func group(recs []record) map[string][]string {
	m := map[string][]string{}
	for _, r := range recs {
		m[r.key] = append(m[r.key], r.value)
	}
	for _, v := range m {
		sort.Strings(v)
	}
	return m
}

func unionKeys[V any](a, b map[string]V) []string {
	set := map[string]bool{}
	for k := range a {
		set[k] = true
	}
	for k := range b {
		set[k] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// minus returns the members of a that b lacks (as multisets of sorted lists).
func minus(a, b []string) []string {
	count := map[string]int{}
	for _, x := range b {
		count[x]++
	}
	var out []string
	for _, x := range a {
		if count[x] > 0 {
			count[x]--
			continue
		}
		out = append(out, x)
	}
	return out
}

// compareRecords reports each key whose values differ, with both sides. A
// key in known must differ (it is logged, the first with both sides); any
// other differing key is an error.
func compareRecords(t *testing.T, port, ref []record, known map[string]bool) {
	t.Helper()
	p, r := group(port), group(ref)
	differing, knownSeen, reported := 0, map[string]bool{}, 0
	for _, k := range unionKeys(p, r) {
		onlyPort, onlyRef := minus(p[k], r[k]), minus(r[k], p[k])
		if len(onlyPort) == 0 && len(onlyRef) == 0 {
			continue
		}
		differing++
		msg := fmt.Sprintf("record %q differs:\n  port:      %s\n  reference: %s", k, strings.Join(onlyPort, "\n             "), strings.Join(onlyRef, "\n             "))
		if known[k] {
			if len(knownSeen) == 0 {
				t.Logf("known difference (first of %d): %s", len(known), msg)
			}
			knownSeen[k] = true
			continue
		}
		if reported++; reported <= 40 {
			t.Error(msg)
		}
	}
	checkKnownSeen(t, known, knownSeen)
	t.Logf("L1: %d keys, %d port records, %d reference records, %d differing keys (%d known)", len(unionKeys(p, r)), len(port), len(ref), differing, len(knownSeen))
}

// checkKnownSeen fails for each known difference that no longer differs.
func checkKnownSeen(t *testing.T, known, seen map[string]bool) {
	t.Helper()
	var gone []string
	for k := range known {
		if !seen[k] {
			gone = append(gone, k)
		}
	}
	sort.Strings(gone)
	for _, k := range gone {
		t.Errorf("known difference %q no longer differs: remove it from l1KnownDifferences", k)
	}
}

// digest is the first 8 bytes of SHA-256 of a record line, in hex.
func digest(r record) string {
	sum := sha256.Sum256([]byte(r.key + "\t" + r.value))
	return hex.EncodeToString(sum[:8])
}

func digestsOf(recs []record) map[string][]string {
	m := map[string][]string{}
	for _, r := range recs {
		m[r.key] = append(m[r.key], digest(r))
	}
	for _, v := range m {
		sort.Strings(v)
	}
	return m
}

func equalDigests(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if strings.Join(v, ",") != strings.Join(b[k], ",") {
			return false
		}
	}
	return true
}

// readDigests reads a digests file and the LANGUAGE_VERSION of the parser.c
// it was written from (the supertype map is a record only from version 15).
func readDigests(path string) (map[string][]string, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	m, abi := map[string][]string{}, 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<16), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "# LANGUAGE_VERSION "); ok {
			if abi, err = strconv.Atoi(v); err != nil {
				return nil, 0, fmt.Errorf("%s: %q", path, line)
			}
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, d, ok := strings.Cut(line, "\t")
		if !ok {
			return nil, 0, fmt.Errorf("%s: bad line %q", path, line)
		}
		m[k] = append(m[k], d)
	}
	for _, v := range m {
		sort.Strings(v)
	}
	if abi == 0 {
		return nil, 0, fmt.Errorf("%s: no LANGUAGE_VERSION line", path)
	}
	return m, abi, sc.Err()
}

func writeDigests(path string, m map[string][]string, abi int) error {
	var b strings.Builder
	b.WriteString("# Layer L1 digests: record key, tab, first 8 bytes of SHA-256 of the record line (key, tab, value).\n")
	b.WriteString("# Written by TestL1 with -l1.update from the pinned parser.c and node-types.json.\n")
	fmt.Fprintf(&b, "# LANGUAGE_VERSION %d\n", abi)
	for _, k := range unionKeys(m, m) {
		for _, d := range m[k] {
			b.WriteString(k + "\t" + d + "\n")
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// compareDigests checks the port's records against the committed digests and
// names each differing key with the port's lines. known is as in compareRecords.
func compareDigests(t *testing.T, port []record, want map[string][]string, known map[string]bool) {
	t.Helper()
	got := digestsOf(port)
	lines := map[string][]string{}
	for _, r := range port {
		lines[digest(r)] = append(lines[digest(r)], r.key+"\t"+r.value)
	}
	differing, knownSeen, reported := 0, map[string]bool{}, 0
	for _, k := range unionKeys(got, want) {
		onlyPort, onlyRef := minus(got[k], want[k]), minus(want[k], got[k])
		if len(onlyPort) == 0 && len(onlyRef) == 0 {
			continue
		}
		differing++
		if known[k] {
			knownSeen[k] = true
			continue
		}
		if reported++; reported > 40 {
			continue
		}
		var portLines []string
		for _, d := range onlyPort {
			portLines = append(portLines, lines[d]...)
		}
		t.Errorf("record %q differs from the committed digests:\n  port lines: %s\n  reference digests missing from the port: %v", k, strings.Join(portLines, "\n              "), onlyRef)
	}
	checkKnownSeen(t, known, knownSeen)
	t.Logf("L1 (digests): %d keys, %d port records, %d differing keys (%d known)", len(unionKeys(got, want)), len(port), differing, len(knownSeen))
}
