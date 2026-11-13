package lexbuild

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/odvcencio/gotreesitter/internal/lexexact"
)

// refEnv names the folder with the reference files. Each grammar has a
// folder <name>/ in it with src/grammar.json, parser.c and tree_sitter/parser.h
// (parser.c written by tree-sitter 0.27.1 from that grammar.json).
const refEnv = "LEXBUILD_REF"

func refDir(t *testing.T) string {
	t.Helper()
	d := os.Getenv(refEnv)
	if d == "" {
		t.Skipf("%s is not set: no reference parser.c files to compare with", refEnv)
	}
	return d
}

// refCell is one terminal cell of a parse state in the reference parse table.
type refCell struct {
	sym      int // C symbol id
	reusable bool
	recover  bool // the action list is exactly RECOVER()
}

// refGrammar is what the tests read from a reference parser.c.
type refGrammar struct {
	parserC    string
	src        *lexexact.Source
	tables     *lexexact.Tables
	symNames   map[int]string
	enumNames  map[int]string
	reserved   map[int][]int // reserved word set id -> C symbol ids (ts_reserved_words)
	stateRes   []int         // reserved word set id of each parse state
	extIDs     []int         // C symbol id of each external token (ts_external_scanner_symbol_map)
	tokenCount int
	extCount   int
	lexModes   []int // -1 for (TSStateId)(-1)
	cells      [][]refCell
	jsonBytes  []byte
}

var rePragma = regexp.MustCompile(`(?m)^#pragma GCC optimize [^\n]*\n`)

var (
	reDefine = regexp.MustCompile(`(?m)^#define (\w+) (\d+)$`)
	reEnum   = regexp.MustCompile(`(?m)^\s+(\w+) = (\d+),`)
	reName   = regexp.MustCompile(`(?m)^\s+\[(\w+)\] = "((?:[^"\\]|\\.)*)",$`)
	reMode   = regexp.MustCompile(`(?m)^\s+\[(\d+)\] = \{(.*)\},?$`)
	reAct    = regexp.MustCompile(`(?m)^\s+\[(\d+)\] = \{\.entry = \{\.count = (\d+), \.reusable = (true|false)\}\}(.*)$`)
)

func section(text, start string) string {
	i := strings.Index(text, start)
	if i < 0 {
		return ""
	}
	rest := text[i:]
	j := strings.Index(rest, "\n};")
	if j < 0 {
		return rest
	}
	return rest[:j]
}

func unescapeC(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// tb is the part of testing.TB that loadRef uses, so that the census can
// load a reference with a panicking implementation.
type tb interface {
	Helper()
	Fatalf(format string, args ...any)
}

func loadRef(t tb, root, name string) *refGrammar {
	t.Helper()
	dir := filepath.Join(root, name)
	read := func(p string) []byte {
		b, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("%v", err)
		}
		return b
	}
	pc := string(read("parser.c"))
	ph := string(read("tree_sitter/parser.h"))
	// Large lex functions get a compiler pragma line (render.rs: lex states > 300)
	// that lexexact does not read. It changes no step, so it is removed here.
	src, err := lexexact.Parse(rePragma.ReplaceAllString(pc, ""), ph)
	if err != nil {
		t.Fatalf("lexexact.Parse: %v", err)
	}
	tabs, err := lexexact.BuildUnchecked(src, pc)
	if err != nil {
		t.Fatalf("lexexact.BuildUnchecked: %v", err)
	}
	g := &refGrammar{parserC: pc, src: src, tables: tabs, symNames: map[int]string{}, enumNames: map[int]string{}, jsonBytes: read("src/grammar.json")}
	g.readTables(t, pc)
	return g
}

// symbolMap maps the reference's C symbol ids of tokens to this package's
// symbols. The generator numbers the used terminals in index order, except that
// the word token gets id 1 (build_tables.rs: populate_used_symbols). The match
// follows that order. A token is compatible with a C symbol when its name
// equals the symbol's enum name without the sym_ or aux_sym_ prefix, or (for
// an anonymous token) the symbol's name string. A C symbol that no later token
// is compatible with takes the next token: the generator renames a token that
// every use aliases to one name.
func (g *refGrammar) symbolMap(p *Prepared) (toSym map[int]Symbol, toID map[Symbol]int, err error) {
	toSym = map[int]Symbol{0: endSym()}
	toID = map[Symbol]int{endSym(): 0}
	// An external token that stands for an internal token shares that token's
	// symbol id; the others have ids after the terminals.
	nOwn := 0
	for _, et := range p.Syntax.ExternalTokens {
		if !et.HasCorresponding {
			nOwn++
		}
	}
	nTerm := g.tokenCount - nOwn
	vars := p.Lex.Variables
	compatible := func(id, i int) bool {
		en, name := g.enumNames[id], g.symNames[id]
		if strings.HasPrefix(en, "sym_") && vars[i].Ident == strings.TrimPrefix(en, "sym_") {
			return true
		}
		if strings.HasPrefix(en, "aux_sym_") && vars[i].Ident == strings.TrimPrefix(en, "aux_sym_") {
			return true
		}
		return strings.HasPrefix(en, "anon_sym_") && vars[i].Kind == VarAnonymous && vars[i].Ident == name
	}
	set := func(id, i int) {
		sy := terminalSym(i)
		toSym[id], toID[sy] = sy, id
	}
	skip := -1
	first := 1
	if p.Syntax.HasWord && p.Syntax.WordToken.IsTerminal() && nTerm > 1 {
		w := int(p.Syntax.WordToken.Index)
		if compatible(1, w) {
			set(1, w)
			skip, first = w, 2
		}
	}
	ptr := 0
	for id := first; id < nTerm; id++ {
		for ptr == skip {
			ptr++
		}
		found := -1
		for j := ptr; j < len(vars); j++ {
			if j != skip && compatible(id, j) {
				found = j
				break
			}
		}
		if found < 0 {
			found = ptr
		}
		if found >= len(vars) {
			return nil, nil, fmt.Errorf("C symbol %d %q has no token left", id, g.symNames[id])
		}
		set(id, found)
		ptr = found + 1
	}
	for k, et := range p.Syntax.ExternalTokens {
		if et.HasCorresponding || k >= len(g.extIDs) {
			continue
		}
		s := externalSym(k)
		toSym[g.extIDs[k]] = s
		toID[s] = g.extIDs[k]
	}
	return toSym, toID, nil
}

// parseStates converts the reference parse table into the valid tokens of
// each parse state. Externals are kept; they do not reach the lexer.
func (g *refGrammar) parseStates(toSym map[int]Symbol) []ParseState {
	out := make([]ParseState, len(g.cells))
	for i, cs := range g.cells {
		for _, c := range cs {
			if c.sym < g.tokenCount {
				out[i].Tokens.Insert(toSym[c.sym])
			}
		}
	}
	return out
}

// readTables reads the symbol names, lex modes and parse table cells of a parser.c text.
func (g *refGrammar) readTables(t tb, pc string) {
	defs := map[string]int{}
	for _, m := range reDefine.FindAllStringSubmatch(pc, -1) {
		v, _ := strconv.Atoi(m[2])
		defs[m[1]] = v
	}
	g.tokenCount, g.extCount = defs["TOKEN_COUNT"], defs["EXTERNAL_TOKEN_COUNT"]
	stateCount := defs["STATE_COUNT"]
	enum := map[string]int{"ts_builtin_sym_end": 0}
	for _, m := range reEnum.FindAllStringSubmatch(section(pc, "enum ts_symbol_identifiers"), -1) {
		v, _ := strconv.Atoi(m[2])
		enum[m[1]] = v
		g.enumNames[v] = m[1]
	}
	for _, m := range regexp.MustCompile(`(?m)^\s+\[ts_external_token_\w+\] = (\w+),$`).FindAllStringSubmatch(section(pc, "ts_external_scanner_symbol_map[EXTERNAL_TOKEN_COUNT]"), -1) {
		g.extIDs = append(g.extIDs, enum[m[1]])
	}
	for _, m := range reName.FindAllStringSubmatch(section(pc, "static const char * const ts_symbol_names[]"), -1) {
		if id, ok := enum[m[1]]; ok {
			g.symNames[id] = unescapeC(m[2])
		}
	}
	// lex modes
	g.reserved = map[int][]int{}
	if sec := section(pc, "ts_reserved_words["); sec != "" {
		cur := -1
		for _, line := range strings.Split(sec, "\n") {
			if m := regexp.MustCompile(`^\s+\[(\d+)\] = \{$`).FindStringSubmatch(line); m != nil {
				cur, _ = strconv.Atoi(m[1])
				continue
			}
			if m := regexp.MustCompile(`^\s+(\w+),$`).FindStringSubmatch(line); m != nil && cur >= 0 {
				if id, ok := enum[m[1]]; ok {
					g.reserved[cur] = append(g.reserved[cur], id)
				}
			}
		}
	}
	g.stateRes = make([]int, stateCount)
	g.lexModes = make([]int, stateCount)
	for _, m := range reMode.FindAllStringSubmatch(section(pc, "ts_lex_modes[STATE_COUNT]"), -1) {
		i, _ := strconv.Atoi(m[1])
		if strings.Contains(m[2], "(-1)") {
			g.lexModes[i] = -1
			continue
		}
		mm := regexp.MustCompile(`\.lex_state = (\d+)`).FindStringSubmatch(m[2])
		if mm == nil {
			t.Fatalf("lex mode %d: %q", i, m[2])
		}
		g.lexModes[i], _ = strconv.Atoi(mm[1])
		if rm := regexp.MustCompile(`\.reserved_word_set_id = (\d+)`).FindStringSubmatch(m[2]); rm != nil {
			g.stateRes[i], _ = strconv.Atoi(rm[1])
		}
	}
	// parse actions
	type act struct{ reusable, recover bool }
	acts := map[int]act{}
	for _, m := range reAct.FindAllStringSubmatch(section(pc, "static const TSParseActionEntry ts_parse_actions[]"), -1) {
		i, _ := strconv.Atoi(m[1])
		rest := strings.TrimSpace(m[4])
		acts[i] = act{m[3] == "true", rest == ", RECOVER()," || rest == ", RECOVER()"}
	}
	g.cells = make([][]refCell, stateCount)
	addCell := func(state int, sym int, actIdx int) {
		a, ok := acts[actIdx]
		if !ok {
			t.Fatalf("state %d: no parse action entry %d", state, actIdx)
		}
		g.cells[state] = append(g.cells[state], refCell{sym, a.reusable, a.recover})
	}
	reLargeState := regexp.MustCompile(`^\s+\[STATE\((\d+)\)\] = \{$`)
	reCell := regexp.MustCompile(`^\s+\[(\w+)\] = (ACTIONS|STATE)\((\d+)\),$`)
	cur := -1
	for _, line := range strings.Split(section(pc, "static const uint16_t ts_parse_table[LARGE_STATE_COUNT][SYMBOL_COUNT]"), "\n") {
		if m := reLargeState.FindStringSubmatch(line); m != nil {
			cur, _ = strconv.Atoi(m[1])
			continue
		}
		if m := reCell.FindStringSubmatch(line); m != nil && cur >= 0 && m[2] == "ACTIONS" {
			n, _ := strconv.Atoi(m[3])
			id, ok := enum[m[1]]
			if !ok {
				t.Fatalf("unknown symbol %s", m[1])
			}
			addCell(cur, id, n)
		}
	}
	// small parse table
	small := section(pc, "static const uint16_t ts_small_parse_table[]")
	offsets := map[int]int{} // offset -> state
	for _, m := range regexp.MustCompile(`\[SMALL_STATE\((\d+)\)\] = (\d+),`).FindAllStringSubmatch(section(pc, "static const uint32_t ts_small_parse_table_map[]"), -1) {
		s, _ := strconv.Atoi(m[1])
		o, _ := strconv.Atoi(m[2])
		offsets[o] = s
	}
	reHead := regexp.MustCompile(`^\s+\[(\d+)\] = (\d+),$`)
	reGroup := regexp.MustCompile(`^\s+(ACTIONS|STATE)\((\d+)\), (\d+),$`)
	reSym := regexp.MustCompile(`^\s+(\w+),$`)
	lines := strings.Split(small, "\n")
	for i := 0; i < len(lines); i++ {
		hm := reHead.FindStringSubmatch(lines[i])
		if hm == nil {
			continue
		}
		off, _ := strconv.Atoi(hm[1])
		groups, _ := strconv.Atoi(hm[2])
		state, ok := offsets[off]
		if !ok {
			t.Fatalf("small parse table offset %d has no state", off)
		}
		for gi := 0; gi < groups; gi++ {
			i++
			gm := reGroup.FindStringSubmatch(lines[i])
			if gm == nil {
				t.Fatalf("small parse table: line %q", lines[i])
			}
			n, _ := strconv.Atoi(gm[2])
			cnt, _ := strconv.Atoi(gm[3])
			for k := 0; k < cnt; k++ {
				i++
				sm := reSym.FindStringSubmatch(lines[i])
				if sm == nil {
					t.Fatalf("small parse table: symbol line %q", lines[i])
				}
				id, ok := enum[sm[1]]
				if !ok {
					t.Fatalf("unknown symbol %s", sm[1])
				}
				if gm[1] == "ACTIONS" {
					addCell(state, id, n)
				}
			}
		}
	}
	for s := range g.cells {
		sort.Slice(g.cells[s], func(a, b int) bool { return g.cells[s][a].sym < g.cells[s][b].sym })
	}
}

// coincidentStates returns the valid tokens of each parse state of the parse
// table written with --disable-optimizations (file parser-noopt.c next to
// parser.c). The generator builds its coincident token index from the table
// before parse states are merged, and that table is this one. State 0 is taken
// before the error state gets its recovery entries. ok is false when the file
// is missing or its symbols differ from those of parser.c.
func coincidentStates(t tb, root, name string, main *refGrammar, toSym map[int]Symbol) (sets []TokenSet, ok bool) {
	b, err := os.ReadFile(filepath.Join(root, name, "parser-noopt.c"))
	if err != nil {
		return nil, false
	}
	g := &refGrammar{symNames: map[int]string{}, enumNames: map[int]string{}}
	g.readTables(t, string(b))
	if g.tokenCount != main.tokenCount || len(g.enumNames) != len(main.enumNames) {
		return nil, false
	}
	for id, n := range main.enumNames {
		if g.enumNames[id] != n {
			return nil, false
		}
	}
	sets = make([]TokenSet, len(g.cells))
	for i, cs := range g.cells {
		for _, c := range cs {
			if c.sym < g.tokenCount && !(i == 0 && c.recover) {
				sets[i].Insert(toSym[c.sym])
			}
		}
	}
	return sets, true
}

// withReserved adds to the valid tokens of each parse state the tokens of its
// reserved word set. The generator builds the lex table from both
// (build_lex_table.rs: terminal_entries chained with reserved_words) and builds
// its other tables from the terminal entries only.
func (g *refGrammar) withReserved(final []TokenSet, toSym map[int]Symbol) []TokenSet {
	out := make([]TokenSet, len(final))
	for i := range final {
		out[i] = final[i].clone()
		for _, id := range g.reserved[g.stateRes[i]] {
			if s, ok := toSym[id]; ok {
				out[i].Insert(s)
			}
		}
	}
	return out
}
