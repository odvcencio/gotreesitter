package ref

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	gts "github.com/odvcencio/gotreesitter"
)

// Extra holds what a generated parser.c has and a Language does not. A nil
// slice or a negative count means the source did not have it: the records of
// that section are then not written, and a comparison that needs them must
// say so, because equality with missing data is not evidence.
type Extra struct {
	SymbolMap              []int // ts_symbol_map
	PrimaryStateIDs        []int // ts_primary_state_ids; nil: Language.PrimaryStateIDs
	MaxAliasSequenceLength int   // MAX_ALIAS_SEQUENCE_LENGTH; -1 when unknown
}

// NoExtra is the Extra of a source that has none of its data.
var NoExtra = Extra{MaxAliasSequenceLength: -1}

// line joins the fields of one record.
func line(fields ...string) string { return strings.Join(fields, "\t") }

func itoa(v int) string { return strconv.Itoa(v) }

func joinInts[T ~int | ~uint16 | ~uint32](xs []T, sep string) string {
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = strconv.FormatInt(int64(x), 10)
	}
	return strings.Join(parts, sep)
}

// LanguageRecords returns the records of a Language in C's numbering. With
// tables "L4" it writes layers L1, L2, L4, L5 and L6; with "L3" (a table
// written without merging, parser-noopt.c) it writes only the L3 tables.
func LanguageRecords(lang *gts.Language, x Extra, tables string) ([]string, error) {
	if tables != "L3" && tables != "L4" {
		return nil, fmt.Errorf("tables layer %q: want L3 or L4", tables)
	}
	cells, err := stateCells(lang)
	if err != nil {
		return nil, err
	}
	var out []string
	if tables == "L4" {
		out = append(out, preparedRecords(lang)...)
		out = append(out, numberingRecords(lang, x, cells)...)
	}
	out = append(out, tableRecords(lang, x, cells, tables)...)
	if tables == "L3" {
		return out, nil
	}
	lex, err := lexRecords(lang, cells)
	if err != nil {
		return nil, err
	}
	out = append(out, lex...)
	return append(out, languageRecords(lang)...), nil
}

// cell is one non-empty entry of the parse table: an action list offset for a
// token, a goto state for a non-terminal.
type cell struct {
	sym, val int
}

// stateCells reads every non-empty cell of every state, large and small, in
// symbol order.
func stateCells(lang *gts.Language) ([][]cell, error) {
	if len(lang.LargeStateGotos) != 0 {
		return nil, fmt.Errorf("LargeStateGotos is not read by the records")
	}
	n, large := int(lang.StateCount), int(lang.LargeStateCount)
	if large > n || len(lang.ParseTable) < large {
		return nil, fmt.Errorf("large states %d, dense rows %d, states %d", large, len(lang.ParseTable), n)
	}
	if large < n && len(lang.SmallParseTableMap) != n-large {
		return nil, fmt.Errorf("small parse table map has %d entries for %d small states", len(lang.SmallParseTableMap), n-large)
	}
	out := make([][]cell, n)
	for s := 0; s < large; s++ {
		for sym, v := range lang.ParseTable[s] {
			if v != 0 {
				out[s] = append(out[s], cell{sym, int(v)})
			}
		}
	}
	t := lang.SmallParseTable
	for i := 0; large+i < n; i++ {
		s, pos := large+i, int(lang.SmallParseTableMap[i])
		if pos >= len(t) {
			return nil, fmt.Errorf("state %d: small table offset %d past %d", s, pos, len(t))
		}
		groups := int(t[pos])
		pos++
		for g := 0; g < groups; g++ {
			if pos+1 >= len(t) {
				return nil, fmt.Errorf("state %d: small table group %d past the end", s, g)
			}
			val, count := int(t[pos]), int(t[pos+1])
			pos += 2
			if pos+count > len(t) {
				return nil, fmt.Errorf("state %d: small table group %d past the end", s, g)
			}
			for k := 0; k < count; k++ {
				out[s] = append(out[s], cell{int(t[pos+k]), val})
			}
			pos += count
		}
	}
	for s := range out {
		cs := out[s]
		sort.SliceStable(cs, func(a, b int) bool { return cs[a].sym < cs[b].sym })
		for k := 1; k < len(cs); k++ {
			if cs[k].sym == cs[k-1].sym {
				return nil, fmt.Errorf("state %d: symbol %d has two cells", s, cs[k].sym)
			}
		}
	}
	return out, nil
}

func actionText(a gts.ParseAction) string {
	switch a.Type {
	case gts.ParseActionShift:
		s := "S" + itoa(int(a.State))
		if a.Extra {
			s += "x"
		}
		if a.Repetition {
			s += "r"
		}
		return s
	case gts.ParseActionReduce:
		return fmt.Sprintf("R%d/%d/%d/%d", a.Symbol, a.ChildCount, a.DynamicPrecedence, a.ProductionID)
	case gts.ParseActionAccept:
		return "A"
	case gts.ParseActionRecover:
		return "E"
	}
	return fmt.Sprintf("?%d", a.Type)
}

// cellText is one field of a state record: "sym=A<offset>[actions]" for a
// token, "sym=G<state>" for a non-terminal.
func cellText(lang *gts.Language, c cell) string {
	if c.sym >= int(lang.TokenCount) {
		return fmt.Sprintf("%d=G%d", c.sym, c.val)
	}
	if c.val >= len(lang.ParseActions) {
		return fmt.Sprintf("%d=A%d[missing]", c.sym, c.val)
	}
	acts := lang.ParseActions[c.val].Actions
	parts := make([]string, len(acts))
	for i, a := range acts {
		parts[i] = actionText(a)
	}
	return fmt.Sprintf("%d=A%d[%s]", c.sym, c.val, strings.Join(parts, " "))
}

func tableRecords(lang *gts.Language, x Extra, cells [][]cell, layer string) []string {
	out := []string{line(layer, "counts",
		"states="+itoa(int(lang.StateCount)), "large="+itoa(int(lang.LargeStateCount)),
		"symbols="+itoa(int(lang.SymbolCount)), "tokens="+itoa(int(lang.TokenCount)),
		"externals="+itoa(int(lang.ExternalTokenCount)), "fields="+itoa(int(lang.FieldCount)),
		"productions="+itoa(int(lang.ProductionIDCount)))}
	for s, cs := range cells {
		f := []string{layer, "state", itoa(s)}
		for _, c := range cs {
			f = append(f, cellText(lang, c))
		}
		out = append(out, line(f...))
	}
	primary := x.PrimaryStateIDs
	if primary == nil && lang.PrimaryStateIDs != nil {
		primary = make([]int, len(lang.PrimaryStateIDs))
		for i, p := range lang.PrimaryStateIDs {
			primary[i] = int(p)
		}
	}
	if primary != nil {
		out = append(out, line(layer, "primary-states", joinInts(primary, " ")))
	}
	if layer == "L4" && x.SymbolMap != nil {
		out = append(out, line(layer, "symbol-map", joinInts(x.SymbolMap, " ")))
	}
	return out
}

func symbolMeta(lang *gts.Language, i int) string {
	var m gts.SymbolMetadata
	if i < len(lang.SymbolMetadata) {
		m = lang.SymbolMetadata[i]
	}
	return fmt.Sprintf("visible=%v\tnamed=%v\tsupertype=%v", m.Visible, m.Named, m.Supertype)
}

// fieldMapText is the field map of one production id: field:child, with "i"
// for an inherited entry, in the stored order.
func fieldMapText(lang *gts.Language, id int) string {
	if id >= len(lang.FieldMapSlices) {
		return ""
	}
	sl := lang.FieldMapSlices[id]
	var parts []string
	for k := int(sl[0]); k < int(sl[0])+int(sl[1]) && k < len(lang.FieldMapEntries); k++ {
		e := lang.FieldMapEntries[k]
		p := fmt.Sprintf("%d:%d", e.FieldID, e.ChildIndex)
		if e.Inherited {
			p += "i"
		}
		parts = append(parts, p)
	}
	return strings.Join(parts, " ")
}

// aliasText is the alias sequence of one production id without trailing
// zeros (no alias at that child).
func aliasText(lang *gts.Language, id int) string {
	if id >= len(lang.AliasSequences) {
		return ""
	}
	seq := lang.AliasSequences[id]
	n := len(seq)
	for n > 0 && seq[n-1] == 0 {
		n--
	}
	return joinInts(seq[:n], " ")
}

func sortedSet(lines []string) []string {
	sort.Strings(lines)
	out := lines[:0]
	for i, l := range lines {
		if i == 0 || l != lines[i-1] {
			out = append(out, l)
		}
	}
	return out
}

func preparedRecords(lang *gts.Language) []string {
	var syms, aliases, fieldMaps []string
	for i, name := range lang.SymbolNames {
		syms = append(syms, line("L1", "symbol", strconv.Quote(name), symbolMeta(lang, i)))
	}
	for id := 0; id < int(lang.ProductionIDCount); id++ {
		if a := aliasText(lang, id); a != "" {
			aliases = append(aliases, line("L1", "alias-sequence", a))
		}
		if f := fieldMapText(lang, id); f != "" {
			fieldMaps = append(fieldMaps, line("L1", "field-map", f))
		}
	}
	out := sortedSet(syms)
	for i := 1; i < len(lang.FieldNames); i++ {
		out = append(out, line("L1", "field", itoa(i), strconv.Quote(lang.FieldNames[i])))
	}
	out = append(out, sortedSet(aliases)...)
	out = append(out, sortedSet(fieldMaps)...)
	for sym, as := range lang.NonTerminalAliasMap {
		if len(as) > 0 {
			out = append(out, line("L1", "non-terminal-alias", itoa(sym), joinInts(as, " ")))
		}
	}
	for _, st := range lang.SupertypeSymbols {
		members := ""
		if int(st) < len(lang.SupertypeMapSlices) {
			sl := lang.SupertypeMapSlices[st]
			end := int(sl[0]) + int(sl[1])
			if end <= len(lang.SupertypeMapEntries) {
				members = joinInts(lang.SupertypeMapEntries[sl[0]:end], " ")
			}
		}
		out = append(out, line("L1", "supertype", itoa(int(st)), members))
	}
	return out
}

func numberingRecords(lang *gts.Language, x Extra, cells [][]cell) []string {
	var out []string
	for i, name := range lang.SymbolNames {
		out = append(out, line("L2", "symbol", itoa(i), strconv.Quote(name)))
	}
	if x.MaxAliasSequenceLength >= 0 {
		out = append(out, line("L2", "max-alias-sequence-length", itoa(x.MaxAliasSequenceLength)))
	}
	for id := 0; id < int(lang.ProductionIDCount); id++ {
		out = append(out, line("L2", "production", itoa(id), "aliases="+aliasText(lang, id), "fields="+fieldMapText(lang, id)))
	}
	if len(cells) > 0 {
		f := []string{"L2", "error-state"}
		for _, c := range cells[0] {
			if c.sym < int(lang.TokenCount) {
				f = append(f, cellText(lang, c))
			}
		}
		out = append(out, line(f...))
	}
	return out
}

// seg is one range of a lex state after first-match resolution.
type seg struct {
	lo, hi rune
	next   int
	skip   bool
}

// canonicalRanges resolves the transitions of a lex state in first-match
// order (the order the lexer reads them) and merges neighbouring ranges with
// the same target and skip flag. Two lists that the lexer cannot tell apart
// give the same result.
func canonicalRanges(ts []gts.LexTransition) []seg {
	var pts []rune
	for _, t := range ts {
		if t.Lo <= t.Hi {
			pts = append(pts, t.Lo, t.Hi+1)
		}
	}
	sort.Slice(pts, func(a, b int) bool { return pts[a] < pts[b] })
	var out []seg
	for i := 0; i+1 < len(pts); i++ {
		lo, hi := pts[i], pts[i+1]-1
		if lo > hi {
			continue
		}
		for _, t := range ts {
			if t.Lo <= lo && lo <= t.Hi {
				if n := len(out); n > 0 && out[n-1].hi+1 == lo && out[n-1].next == t.NextState && out[n-1].skip == t.Skip {
					out[n-1].hi = hi
				} else {
					out = append(out, seg{lo, hi, t.NextState, t.Skip})
				}
				break
			}
		}
	}
	return out
}

func lexStateRecord(kind string, i int, st gts.LexState) string {
	f := []string{"L5", kind, itoa(i),
		"accept=" + itoa(int(st.AcceptToken)), fmt.Sprintf("end=%v", st.AcceptEOF), fmt.Sprintf("skip=%v", st.Skip),
		"priority=" + itoa(int(st.AcceptPriority)), "eof=" + itoa(st.EOF), "default=" + itoa(st.Default)}
	for _, r := range canonicalRanges(st.Transitions) {
		s := fmt.Sprintf("%d-%d:%d", r.lo, r.hi, r.next)
		if r.skip {
			s += "s"
		}
		f = append(f, s)
	}
	return line(f...)
}

func lexRecords(lang *gts.Language, cells [][]cell) ([]string, error) {
	if len(lang.LexModes) != int(lang.StateCount) {
		return nil, fmt.Errorf("%d lex modes for %d states", len(lang.LexModes), lang.StateCount)
	}
	var out []string
	for s, m := range lang.LexModes {
		lex := "none"
		if i := m.LexStateIndex(); i != ^uint32(0) {
			lex = strconv.FormatUint(uint64(i), 10)
		}
		f := []string{"L5", "state", itoa(s), "lex=" + lex, "external=" + itoa(int(m.ExternalLexState)), "reserved=" + itoa(int(m.ReservedWordSetID))}
		for _, c := range cells[s] {
			if c.sym >= int(lang.TokenCount) {
				continue
			}
			r := "?"
			if c.val < len(lang.ParseActions) {
				r = "n"
				if lang.ParseActions[c.val].Reusable {
					r = "r"
				}
			}
			f = append(f, itoa(c.sym)+"="+r)
		}
		out = append(out, line(f...))
	}
	for i, st := range lang.LexStates {
		out = append(out, lexStateRecord("lex", i, st))
	}
	for i, st := range lang.KeywordLexStates {
		out = append(out, lexStateRecord("keyword-lex", i, st))
	}
	if len(lang.KeywordLexStates) > 0 {
		out = append(out, line("L5", "keyword-capture", itoa(int(lang.KeywordCaptureToken))))
	}
	if size := int(lang.MaxReservedWordSetSize); size > 0 {
		f := []string{"L5", "reserved-words"}
		for start := 0; start+size <= len(lang.ReservedWords); start += size {
			set := lang.ReservedWords[start : start+size]
			n := 0
			for n < len(set) && set[n] != 0 {
				n++
			}
			f = append(f, "{"+joinInts(set[:n], " ")+"}")
		}
		out = append(out, line(f...))
	}
	if lang.ExternalTokenCount > 0 {
		out = append(out, line("L5", "external-symbols", joinInts(lang.ExternalSymbols, " ")))
		for row, bits := range lang.ExternalLexStates {
			var on []int
			for k, b := range bits {
				if b {
					on = append(on, k)
				}
			}
			out = append(out, line("L5", "external-state", itoa(row), joinInts(on, " ")))
		}
	}
	return out, nil
}
